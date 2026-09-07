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
	if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
		return err
	}

	f, err := os.Create(fullPath)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	_, err = io.Copy(f, reader)
	return err
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
