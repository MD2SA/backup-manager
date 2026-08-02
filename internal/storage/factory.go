package storage

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/MD2SA/backup-manager/internal/storage/local"
	"github.com/MD2SA/backup-manager/internal/storage/s3"
)

func NewProvider(ctx context.Context, pType string, config map[string]interface{}, basePath string) (StorageProvider, error) {
	switch pType {
	case "local":
		path, _ := config["path"].(string)

		// Sanitize and join with basePath
		// 1. Remove leading slashes to prevent absolute paths
		path = strings.TrimPrefix(path, "/")
		// 2. Use Join and Clean to resolve ".." and relative segments
		finalPath := filepath.Join(basePath, path)
		// 3. Ensure the final path is still within the basePath
		rel, err := filepath.Rel(basePath, finalPath)
		if err != nil || strings.HasPrefix(rel, "..") {
			return nil, fmt.Errorf("invalid local path: must be within storage directory")
		}

		return local.New(finalPath)
	case "s3":
		region, _ := config["region"].(string)
		bucket, _ := config["bucket"].(string)
		accessKey, _ := config["access_key"].(string)
		secretKey, _ := config["secret_key"].(string)

		if region == "" || bucket == "" || accessKey == "" || secretKey == "" {
			return nil, fmt.Errorf("s3 storage requires region, bucket, access_key, and secret_key")
		}

		return s3.New(ctx, region, bucket, accessKey, secretKey)
	default:
		return nil, fmt.Errorf("Unknown storage type: %s", pType)
	}
}
