package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const minimalConfig = `version: 1
server:
  port: "9090"
  env: development
  log_level: debug
storage:
  path: /tmp/backups
  temp_dir: /tmp
metadata_database:
  host: localhost
  port: "5432"
  user: u
  dbname: db
  sslmode: disable
`

func writeTestFiles(t *testing.T, dir, cfg, secrets string) (string, string) {
	t.Helper()
	configPath := filepath.Join(dir, "config.yaml")
	secretsPath := filepath.Join(dir, "secrets.env")
	if err := os.WriteFile(configPath, []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(secretsPath, []byte(secrets), 0o600); err != nil {
		t.Fatal(err)
	}
	return configPath, secretsPath
}

func TestLoad_DefaultsAndSecrets(t *testing.T) {
	t.Setenv("APP_SECRETS_FILE", "")
	dir := t.TempDir()
	configPath, _ := writeTestFiles(t, dir, minimalConfig, "METADATA_DB_PASSWORD=secret\n")

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.Port != "9090" {
		t.Errorf("port = %q", cfg.Port)
	}
	if cfg.LogLevel != "debug" {
		t.Errorf("log level = %q", cfg.LogLevel)
	}
	if cfg.MetadataDB.Password != "secret" {
		t.Errorf("metadata password not resolved from secrets file")
	}
	if cfg.RateLimitRequests != 100 {
		t.Errorf("default rate limit requests = %d", cfg.RateLimitRequests)
	}
	if cfg.RateLimitWindow.String() != "1m0s" {
		t.Errorf("default rate limit window = %s", cfg.RateLimitWindow)
	}
	if cfg.MetadataBackupRetention != 14 {
		t.Errorf("default metadata retention = %d", cfg.MetadataBackupRetention)
	}
	if !cfg.MetadataBackupEmbed {
		t.Error("metadata embed should default to true")
	}
	if len(cfg.CORSOrigins) != 1 || cfg.CORSOrigins[0] != "http://localhost:3000" {
		t.Errorf("default CORS = %v", cfg.CORSOrigins)
	}
	if cfg.OverdueFactor != 2.0 {
		t.Errorf("default overdue factor = %v", cfg.OverdueFactor)
	}
	if cfg.OverdueGrace != 15*time.Minute {
		t.Errorf("default overdue grace = %v", cfg.OverdueGrace)
	}
	if cfg.BackupTimeout != 6*time.Hour {
		t.Errorf("default backup timeout = %v", cfg.BackupTimeout)
	}
	if cfg.RestoreTimeout != 2*time.Hour {
		t.Errorf("default restore timeout = %v", cfg.RestoreTimeout)
	}
	if cfg.OverdueNotifyInterval != 15*time.Minute {
		t.Errorf("default overdue notify interval = %v", cfg.OverdueNotifyInterval)
	}
	if cfg.OverdueNotifyRepeat != 24*time.Hour {
		t.Errorf("default overdue notify repeat = %v", cfg.OverdueNotifyRepeat)
	}
	if cfg.AuditRetentionDays != 90 {
		t.Errorf("default audit retention = %d", cfg.AuditRetentionDays)
	}
	if cfg.AuditCleanupSchedule != "0 4 * * *" {
		t.Errorf("default audit schedule = %q", cfg.AuditCleanupSchedule)
	}
}

func TestLoad_RejectsInvalidHealthAndAuditConfig(t *testing.T) {
	t.Setenv("APP_SECRETS_FILE", "")
	dir := t.TempDir()
	cfg := minimalConfig + `health:
  overdue_factor: 0.5
  overdue_grace: "-1m"
audit:
  retention_days: 30
  cleanup_schedule: "not a cron"
`
	configPath, _ := writeTestFiles(t, dir, cfg, "METADATA_DB_PASSWORD=secret\n")

	_, err := Load(configPath)
	if err == nil {
		t.Fatal("expected validation error")
	}
	msg := err.Error()
	for _, want := range []string{"health.overdue_factor", "health.overdue_grace", "audit.cleanup_schedule"} {
		if !strings.Contains(msg, want) {
			t.Errorf("expected error to mention %q, got %v", want, msg)
		}
	}
}

func TestLoad_RejectsUnknownFields(t *testing.T) {
	t.Setenv("APP_SECRETS_FILE", "")
	dir := t.TempDir()
	bad := strings.Replace(minimalConfig, "metadata_database:", "metdata_database:", 1)
	configPath, _ := writeTestFiles(t, dir, bad, "METADATA_DB_PASSWORD=secret\n")

	_, err := Load(configPath)
	if err == nil || !strings.Contains(err.Error(), "metdata_database") {
		t.Fatalf("expected unknown-field error mentioning the typo, got %v", err)
	}
}

func TestLoad_RequiresVersion(t *testing.T) {
	t.Setenv("APP_SECRETS_FILE", "")
	dir := t.TempDir()
	configPath, _ := writeTestFiles(t, dir, strings.Replace(minimalConfig, "version: 1\n", "", 1), "METADATA_DB_PASSWORD=secret\n")

	_, err := Load(configPath)
	if err == nil || !strings.Contains(err.Error(), "unsupported config version") {
		t.Fatalf("expected version error, got %v", err)
	}
}

func TestLoad_MissingSecretNamesTheKey(t *testing.T) {
	t.Setenv("APP_SECRETS_FILE", "")
	dir := t.TempDir()
	configPath, _ := writeTestFiles(t, dir, minimalConfig, "")

	_, err := Load(configPath)
	if err == nil || !strings.Contains(err.Error(), "METADATA_DB_PASSWORD") {
		t.Fatalf("expected missing METADATA_DB_PASSWORD error, got %v", err)
	}
}

func TestLoad_PartialTargetDatabaseFails(t *testing.T) {
	t.Setenv("APP_SECRETS_FILE", "")
	dir := t.TempDir()
	cfg := minimalConfig + `target_database:
  host: target
`
	configPath, _ := writeTestFiles(t, dir, cfg, "METADATA_DB_PASSWORD=secret\n")

	_, err := Load(configPath)
	if err == nil || !strings.Contains(err.Error(), "target_database") {
		t.Fatalf("expected target database validation error, got %v", err)
	}
}

func TestLoad_ProductionRequiresAdminAndEncryption(t *testing.T) {
	t.Setenv("APP_SECRETS_FILE", "")
	dir := t.TempDir()
	cfg := strings.Replace(minimalConfig, "env: development", "env: production", 1)
	configPath, _ := writeTestFiles(t, dir, cfg, "METADATA_DB_PASSWORD=secret\n")

	_, err := Load(configPath)
	if err == nil {
		t.Fatal("expected production validation to fail without admin key, config key and encryption")
	}
	msg := err.Error()
	for _, want := range []string{"CONFIG_ENCRYPT_KEY", "production requires backup encryption"} {
		if !strings.Contains(msg, want) {
			t.Errorf("expected error to mention %q, got %v", want, msg)
		}
	}
}

func TestLoad_ProductionWithExplicitOptOut(t *testing.T) {
	t.Setenv("APP_SECRETS_FILE", "")
	dir := t.TempDir()
	cfg := strings.Replace(minimalConfig, "env: development", "env: production", 1) + `encryption:
  allow_unencrypted_backups: true
`
	secrets := "METADATA_DB_PASSWORD=secret\nADMIN_KEY=admin\nCONFIG_ENCRYPT_KEY=configkey\n"
	configPath, _ := writeTestFiles(t, dir, cfg, secrets)

	if _, err := Load(configPath); err != nil {
		t.Fatalf("expected explicit opt-out to pass, got %v", err)
	}
}

func TestLoad_RejectsTargetSameAsMetadata(t *testing.T) {
	t.Setenv("APP_SECRETS_FILE", "")
	dir := t.TempDir()
	cfg := minimalConfig + `target_database:
  host: localhost
  port: "5432"
  user: u
  dbname: db
  sslmode: disable
`
	secrets := "METADATA_DB_PASSWORD=secret\nTARGET_DB_PASSWORD=secret\n"
	configPath, _ := writeTestFiles(t, dir, cfg, secrets)

	_, err := Load(configPath)
	if err == nil || !strings.Contains(err.Error(), "target_database must not point") {
		t.Fatalf("expected same-database protection, got %v", err)
	}
}

func TestLoad_RestoreCredentials(t *testing.T) {
	t.Setenv("APP_SECRETS_FILE", "")
	dir := t.TempDir()
	cfg := minimalConfig + `target_database:
  host: target
  port: "5432"
  user: backup_reader
  dbname: db
  sslmode: disable
  restore_user: restore_admin
`
	secrets := "METADATA_DB_PASSWORD=secret\nTARGET_DB_PASSWORD=readpass\nTARGET_DB_RESTORE_PASSWORD=writepass\n"
	configPath, _ := writeTestFiles(t, dir, cfg, secrets)

	loaded, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.TargetDB.RestoreUser != "restore_admin" || loaded.TargetDB.RestorePassword != "writepass" {
		t.Fatalf("unexpected restore credentials: %+v", loaded.TargetDB)
	}

	// Restore user without a restore password must fail.
	configPath, _ = writeTestFiles(t, dir, cfg, "METADATA_DB_PASSWORD=secret\nTARGET_DB_PASSWORD=readpass\n")
	if _, err := Load(configPath); err == nil {
		t.Fatal("expected missing restore password to fail")
	}
}

func TestLoad_OperationTimeouts(t *testing.T) {
	t.Setenv("APP_SECRETS_FILE", "")
	dir := t.TempDir()
	cfg := minimalConfig + `operations:
  backup_timeout: "12h"
  restore_timeout: "0"
health:
  notify_interval: "5m"
  notify_repeat: "48h"
`
	configPath, _ := writeTestFiles(t, dir, cfg, "METADATA_DB_PASSWORD=secret\n")
	loaded, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.BackupTimeout != 12*time.Hour || loaded.RestoreTimeout != 0 {
		t.Fatalf("unexpected operation timeouts: backup=%v restore=%v", loaded.BackupTimeout, loaded.RestoreTimeout)
	}
	if loaded.OverdueNotifyInterval != 5*time.Minute || loaded.OverdueNotifyRepeat != 48*time.Hour {
		t.Fatalf("unexpected notify settings: interval=%v repeat=%v", loaded.OverdueNotifyInterval, loaded.OverdueNotifyRepeat)
	}

	bad := minimalConfig + "operations:\n  backup_timeout: \"not-a-duration\"\n"
	configPath, _ = writeTestFiles(t, dir, bad, "METADATA_DB_PASSWORD=secret\n")
	if _, err := Load(configPath); err == nil {
		t.Fatal("expected invalid backup timeout to fail")
	}
}

func TestLoad_NegativeTimeoutFails(t *testing.T) {
	t.Setenv("APP_SECRETS_FILE", "")
	dir := t.TempDir()
	cfg := minimalConfig + "operations:\n  restore_timeout: \"-1h\"\n"
	configPath, _ := writeTestFiles(t, dir, cfg, "METADATA_DB_PASSWORD=secret\n")
	if _, err := Load(configPath); err == nil {
		t.Fatal("expected negative restore timeout to fail")
	}
}

func TestNormalizeTrustedProxies(t *testing.T) {
	prefixes, err := normalizeTrustedProxies([]string{"127.0.0.1", "10.0.0.0/8", "2001:db8::/32"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(prefixes) != 3 {
		t.Fatalf("expected 3 prefixes, got %d", len(prefixes))
	}
	if prefixes[0].String() != "127.0.0.1/32" {
		t.Errorf("bare IP not upgraded to host prefix: got %s", prefixes[0])
	}

	if _, err := normalizeTrustedProxies([]string{"not-an-ip"}); err == nil {
		t.Error("expected invalid proxy to fail")
	}
}

func TestConfig_ValidateTargetDB(t *testing.T) {
	complete := Config{
		TargetDB: DatabaseConfig{
			Host:     "1.2.3.4",
			Port:     "5435",
			User:     "user",
			Password: "pass",
			DBName:   "db",
			SSLMode:  "disable",
		},
	}
	if err := complete.ValidateTargetDB(); err != nil {
		t.Errorf("complete target DB should pass: %v", err)
	}

	incomplete := Config{TargetDB: DatabaseConfig{Host: "localhost"}}
	if err := incomplete.ValidateTargetDB(); err == nil {
		t.Error("incomplete target DB should fail")
	}
}

func TestInit_GeneratesLoadableConfig(t *testing.T) {
	t.Setenv("APP_SECRETS_FILE", "")
	dir := t.TempDir()

	result, err := Init(InitOptions{
		ConfigPath:              filepath.Join(dir, "config.yaml"),
		Env:                     "development",
		StoragePath:             filepath.Join(dir, "backups"),
		TempDir:                 dir,
		MetadataHost:            "localhost",
		MetadataPort:            "5432",
		MetadataUser:            "u",
		MetadataDBName:          "metadata",
		MetadataSSLMode:         "disable",
		MetadataPassword:        "metadata-secret",
		TargetHost:              "target",
		TargetPort:              "5432",
		TargetUser:              "tu",
		TargetDBName:            "targetdb",
		TargetSSLMode:           "require",
		TargetPassword:          "target-secret",
		EncryptionMode:          "keypair",
		MetadataBackupEmbed:     true,
		MetadataBackupRetention: 14,
	})
	if err != nil {
		t.Fatalf("Init() error: %v", err)
	}

	cfg, err := Load(result.ConfigPath)
	if err != nil {
		t.Fatalf("generated config does not load: %v", err)
	}
	if cfg.MetadataDB.Password != "metadata-secret" {
		t.Errorf("metadata password mismatch")
	}
	if cfg.TargetDB.Password != "target-secret" {
		t.Errorf("target password mismatch")
	}
	if cfg.AdminKey == "" || cfg.ConfigEncryptionKey == "" {
		t.Error("admin/config keys should be generated")
	}
	if cfg.AgePrivateKey == "" || cfg.AgePublicKey == "" {
		t.Error("Age keypair should be generated")
	}
	if cfg.MetadataBackupPassphrase == "" {
		t.Error("metadata backup passphrase should be generated when embedding is enabled")
	}

	info, err := os.Stat(result.SecretsPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("secrets mode = %04o, want 0600", info.Mode().Perm())
	}

	if _, err := Init(InitOptions{ConfigPath: result.ConfigPath, MetadataPassword: "x", EncryptionMode: "none"}); err == nil {
		t.Error("expected Init to refuse overwriting an existing config")
	}
}

func TestEncodeSecretValue_Roundtrip(t *testing.T) {
	values := []string{"simple", "with space", "#leading", `quote"inside`, ""}
	for _, value := range values {
		encoded := EncodeSecretValue(value)
		if value == "" {
			if encoded != "" {
				t.Errorf("empty value should encode to empty, got %q", encoded)
			}
			continue
		}
		decoded := parseSecretValue(encoded)
		if decoded != value {
			t.Errorf("roundtrip %q -> %q -> %q", value, encoded, decoded)
		}
	}
}
