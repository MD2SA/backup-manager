package local

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"sort"
)

type LocalStorage struct {
	BasePath string
}

func New(basePath string) (*LocalStorage, error) {
	if err := os.MkdirAll(basePath, 0755); err != nil {
		return nil, err
	}
	return &LocalStorage{BasePath: basePath}, nil
}

func (s *LocalStorage) Name() string { return "local" }

func (s *LocalStorage) Upload(ctx context.Context, key string, reader io.Reader) error {
	fullPath := filepath.Join(s.BasePath, key)
	dir := filepath.Dir(fullPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	// Write to a temporary file and rename it into place. A crash during an
	// upload must never leave a partial object at the final key.
	tmp, err := os.CreateTemp(dir, ".upload-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
	}()

	if _, err := io.Copy(tmp, reader); err != nil {
		return err
	}
	if err := tmp.Sync(); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpName, 0o644); err != nil {
		return err
	}
	return os.Rename(tmpName, fullPath)
}

func (s *LocalStorage) Download(ctx context.Context, key string) (io.ReadCloser, error) {
	return os.Open(filepath.Join(s.BasePath, key))
}

func (s *LocalStorage) Delete(ctx context.Context, key string) error {
	return os.Remove(filepath.Join(s.BasePath, key))
}

func (s *LocalStorage) Exists(ctx context.Context, key string) (bool, error) {
	_, err := os.Stat(filepath.Join(s.BasePath, key))
	if os.IsNotExist(err) {
		return false, nil
	}
	return err == nil, err
}

func (s *LocalStorage) List(ctx context.Context, prefix string) ([]string, error) {
	root := filepath.Join(s.BasePath, prefix)
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var keys []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		keys = append(keys, filepath.Join(prefix, e.Name()))
	}

	sort.Strings(keys)
	return keys, nil
}

// ListRecursive returns every object under prefix, including nested profile
// directories. S3 treats a prefix as a key namespace; this method gives local
// storage the same semantics for operations such as metadata recovery.
func (s *LocalStorage) ListRecursive(ctx context.Context, prefix string) ([]string, error) {
	root := filepath.Join(s.BasePath, prefix)

	var keys []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if info.IsDir() {
			return nil
		}
		rel, relErr := filepath.Rel(s.BasePath, path)
		if relErr != nil {
			return relErr
		}
		keys = append(keys, rel)
		return nil
	})
	if err != nil {
		return nil, err
	}

	sort.Strings(keys)
	return keys, nil
}
