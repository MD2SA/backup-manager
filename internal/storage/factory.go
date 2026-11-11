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
		endpoint, _ := config["endpoint"].(string)
		forcePathStyle, _ := config["force_path_style"].(bool)
		useSSL := true
		if value, ok := config["use_ssl"].(bool); ok {
			useSSL = value
		}

		if region == "" || bucket == "" || accessKey == "" || secretKey == "" {
			return nil, fmt.Errorf("s3 storage requires region, bucket, access_key, and secret_key")
		}

		return s3.New(ctx, s3.Options{
			Region:         region,
			Bucket:         bucket,
			AccessKey:      accessKey,
			SecretKey:      secretKey,
			Endpoint:       endpoint,
			ForcePathStyle: forcePathStyle,
			UseSSL:         useSSL,
		})
	default:
		return nil, fmt.Errorf("unknown storage type: %s", pType)
	}
}
