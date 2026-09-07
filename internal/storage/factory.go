package storage

import (
	"context"
	"fmt"

	"github.com/MD2SA/backup-manager/internal/storage/local"
	"github.com/MD2SA/backup-manager/internal/storage/s3"
)

func NewProvider(ctx context.Context, pType string, config map[string]interface{}, basePath string) (StorageProvider, error) {
	switch pType {
	case "local":
		return local.New(basePath)
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
		return nil, fmt.Errorf("unknown storage type: %s", pType)
	}
}
