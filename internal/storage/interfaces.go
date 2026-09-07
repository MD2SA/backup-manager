package storage

import (
	"context"
	"io"
)

// StorageProvider defines the behavior for persisting and retrieving backup artifacts.
// Implementations can target local disks, cloud buckets (S3), or other remote protocols.
type StorageProvider interface {
	// Name returns a unique identifier for the provider type (e.g., "s3", "local").
	Name() string

	// Upload streams data from the reader to the specified key in the storage.
	Upload(ctx context.Context, key string, reader io.Reader) error

	// Download returns a reader for the artifact at the specified key.
	// The caller is responsible for closing the reader.
	Download(ctx context.Context, key string) (io.ReadCloser, error)

	// Delete removes the artifact at the specified key.
	Delete(ctx context.Context, key string) error

	// Exists checks if an artifact is present at the specified key.
	Exists(ctx context.Context, key string) (bool, error)

	// List returns the keys under prefix, sorted lexically.
	// Keys are relative to the provider's root and prefixed with the given prefix.
	List(ctx context.Context, prefix string) ([]string, error)
}
