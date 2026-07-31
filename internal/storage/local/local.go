package local

import (
	"context"
	"io"
	"os"
	"path/filepath"
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
	defer f.Close()

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
