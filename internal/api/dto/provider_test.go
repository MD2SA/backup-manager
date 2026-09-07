package dto

import (
	"encoding/json"
	"testing"

	"github.com/MD2SA/backup-manager/internal/repository/db"
	"github.com/jackc/pgx/v5/pgtype"
)

func TestToStorageProviderResponse_MasksS3Secrets(t *testing.T) {
	cfg := `{"region":"us-east-1","bucket":"b","access_key":"AKIA123","secret_key":"supersecret","endpoint":"s3.amazonaws.com"}`
	p := db.StorageProvider{
		ID:     pgtype.UUID{},
		Name:   "S3",
		Type:   "s3",
		Config: []byte(cfg),
	}

	resp := ToStorageProviderResponse(p)

	config, ok := resp.Config.(map[string]any)
	if !ok {
		t.Fatalf("expected map config, got %T", resp.Config)
	}

	if config["secret_key"] != maskedSecret {
		t.Errorf("secret_key should be masked, got %v", config["secret_key"])
	}
	if config["access_key"] != maskedSecret {
		t.Errorf("access_key should be masked, got %v", config["access_key"])
	}
	if config["region"] != "us-east-1" {
		t.Errorf("region should be preserved, got %v", config["region"])
	}
}

func TestToNotificationProviderResponse_MasksWebhook(t *testing.T) {
	cfg := `{"webhook_url":"https://discord.com/api/webhooks/abc"}`
	p := db.NotificationProvider{
		ID:     pgtype.UUID{},
		Name:   "Discord",
		Type:   "discord",
		Config: []byte(cfg),
	}

	resp := ToNotificationProviderResponse(p)

	config, ok := resp.Config.(map[string]any)
	if !ok {
		t.Fatalf("expected map config, got %T", resp.Config)
	}

	if config["webhook_url"] != maskedSecret {
		t.Errorf("webhook_url should be masked, got %v", config["webhook_url"])
	}
}

func TestMergeProviderSecrets(t *testing.T) {
	incoming := map[string]any{
		"region":     "us-east-1",
		"bucket":     "b",
		"access_key": maskedSecret,
		"secret_key": maskedSecret,
		"endpoint":   "new",
	}
	existing := map[string]any{
		"region":     "us-east-1",
		"bucket":     "b",
		"access_key": "AKIA123",
		"secret_key": "supersecret",
		"endpoint":   "old",
	}

	merged := MergeProviderSecrets(incoming, existing, "s3")

	if merged["secret_key"] != "supersecret" {
		t.Errorf("secret_key should be preserved, got %v", merged["secret_key"])
	}
	if merged["access_key"] != "AKIA123" {
		t.Errorf("access_key should be preserved, got %v", merged["access_key"])
	}
	if merged["endpoint"] != "new" {
		t.Errorf("other fields should be updated, got %v", merged["endpoint"])
	}
}

func TestMaskConfigJSONRoundtrip(t *testing.T) {
	m := map[string]any{
		"a": "1",
		"b": maskedSecret,
	}
	raw, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var back map[string]any
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if back["b"] != maskedSecret {
		t.Errorf("expected masked secret to survive roundtrip, got %v", back["b"])
	}
}
