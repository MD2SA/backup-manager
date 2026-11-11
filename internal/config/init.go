package config

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	appcrypto "github.com/MD2SA/backup-manager/internal/pkg/crypto"
	"gopkg.in/yaml.v3"
)

// InitOptions controls `backup-manager init`. Empty fields receive safe
// defaults; secrets that are not provided are generated when it is safe to do
// so. Database passwords are never invented silently.
type InitOptions struct {
	ConfigPath  string
	SecretsPath string
	Force       bool

	Env         string
	Port        string
	LogLevel    string
	StoragePath string
	TempDir     string

	CORSOrigins    []string
	TrustedProxies []string

	MetadataHost     string
	MetadataPort     string
	MetadataUser     string
	MetadataDBName   string
	MetadataSSLMode  string
	MetadataPassword string

	TargetHost     string
	TargetPort     string
	TargetUser     string
	TargetDBName   string
	TargetSSLMode  string
	TargetPassword string

	EncryptionMode       string // "passphrase", "keypair" or "none"
	EncryptionPassphrase string
	AgePublicKey         string
	AgePrivateKey        string

	AdminKey         string
	ConfigEncryptKey string

	MetadataBackupSchedule   string
	MetadataBackupPassphrase string
	MetadataBackupRetention  int
	MetadataBackupEmbed      bool

	AllowUnencryptedBackups bool
}

// InitResult reports the files written and the bootstrap admin key so the CLI
// can print a single, explicit next step.
type InitResult struct {
	ConfigPath  string
	SecretsPath string
	AdminKey    string
}

// Init generates a ready-to-use config.yaml and secrets.env pair.
func Init(opts InitOptions) (InitResult, error) {
	applyInitDefaults(&opts)

	configPath, err := normalizeOutputPath(opts.ConfigPath, DefaultConfigPath)
	if err != nil {
		return InitResult{}, err
	}
	opts.ConfigPath = configPath

	if opts.SecretsPath == "" {
		opts.SecretsPath = filepath.Join(filepath.Dir(configPath), DefaultSecretsFile)
	}
	secretsPath, err := normalizeOutputPath(opts.SecretsPath, DefaultSecretsFile)
	if err != nil {
		return InitResult{}, err
	}
	opts.SecretsPath = secretsPath

	if configPath == secretsPath {
		return InitResult{}, fmt.Errorf("config and secrets cannot be the same file (%s)", configPath)
	}

	configExisted := fileExists(configPath)
	secretsExisted := fileExists(secretsPath)
	if !opts.Force {
		if configExisted {
			return InitResult{}, fmt.Errorf("%s already exists (use --force to overwrite)", configPath)
		}
		if secretsExisted {
			return InitResult{}, fmt.Errorf("%s already exists (use --force to overwrite)", secretsPath)
		}
	}

	if err := prepareSecrets(&opts); err != nil {
		return InitResult{}, err
	}
	if err := validateInitDatabase(opts); err != nil {
		return InitResult{}, err
	}

	cfg := Config{
		ConfigPath:               configPath,
		SecretsPath:              secretsPath,
		Port:                     opts.Port,
		LogLevel:                 opts.LogLevel,
		Env:                      opts.Env,
		TempDir:                  opts.TempDir,
		StoragePath:              opts.StoragePath,
		AgePublicKey:             opts.AgePublicKey,
		AgePrivateKey:            opts.AgePrivateKey,
		EncryptionPassphrase:     opts.EncryptionPassphrase,
		AdminKey:                 opts.AdminKey,
		ConfigEncryptionKey:      opts.ConfigEncryptKey,
		RateLimitRequests:        100,
		RateLimitWindow:          time.Minute,
		CORSOrigins:              opts.CORSOrigins,
		TrustedProxies:           opts.TrustedProxies,
		MetadataBackupSchedule:   opts.MetadataBackupSchedule,
		MetadataBackupPassphrase: opts.MetadataBackupPassphrase,
		MetadataBackupRetention:  opts.MetadataBackupRetention,
		MetadataBackupEmbed:      opts.MetadataBackupEmbed,
		AllowUnencryptedBackups:  opts.AllowUnencryptedBackups,
		OverdueFactor:            2.0,
		OverdueGrace:             15 * time.Minute,
		AuditRetentionDays:       90,
		AuditCleanupSchedule:     "0 4 * * *",
		MetadataDB: DatabaseConfig{
			Host:     opts.MetadataHost,
			Port:     opts.MetadataPort,
			User:     opts.MetadataUser,
			Password: opts.MetadataPassword,
			DBName:   opts.MetadataDBName,
			SSLMode:  opts.MetadataSSLMode,
		},
		TargetDB: DatabaseConfig{
			Host:     opts.TargetHost,
			Port:     opts.TargetPort,
			User:     opts.TargetUser,
			Password: opts.TargetPassword,
			DBName:   opts.TargetDBName,
			SSLMode:  opts.TargetSSLMode,
		},
	}
	if err := cfg.validate(true); err != nil {
		return InitResult{}, fmt.Errorf("generated configuration is invalid: %w", err)
	}

	secretsFile := secretsPath
	if filepath.Dir(configPath) == filepath.Dir(secretsPath) {
		secretsFile = filepath.Base(secretsPath)
	}

	configData := renderConfig(opts, secretsFile)
	secretsData := renderSecrets(opts)

	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		return InitResult{}, fmt.Errorf("create config directory: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(secretsPath), 0o755); err != nil {
		return InitResult{}, fmt.Errorf("create secrets directory: %w", err)
	}

	if err := writeFileAtomic(secretsPath, []byte(secretsData), 0o600); err != nil {
		return InitResult{}, fmt.Errorf("write secrets: %w", err)
	}
	if err := writeFileAtomic(configPath, []byte(configData), 0o644); err != nil {
		if !secretsExisted {
			_ = os.Remove(secretsPath)
		}
		return InitResult{}, fmt.Errorf("write config: %w", err)
	}

	// Reload from disk so init validates exactly what the service will read.
	if _, err := loadFrom(configPath, secretsPath, true); err != nil {
		if !configExisted {
			_ = os.Remove(configPath)
		}
		if !secretsExisted {
			_ = os.Remove(secretsPath)
		}
		return InitResult{}, fmt.Errorf("generated configuration failed validation: %w", err)
	}

	return InitResult{
		ConfigPath:  configPath,
		SecretsPath: secretsPath,
		AdminKey:    opts.AdminKey,
	}, nil
}

func applyInitDefaults(opts *InitOptions) {
	if opts.Env == "" {
		opts.Env = "development"
	}
	if opts.Port == "" {
		opts.Port = "8080"
	}
	if opts.LogLevel == "" {
		opts.LogLevel = "info"
	}
	if opts.StoragePath == "" {
		opts.StoragePath = "/backups"
	}
	if opts.TempDir == "" {
		opts.TempDir = "/tmp"
	}
	if len(opts.CORSOrigins) == 0 {
		opts.CORSOrigins = []string{"http://localhost:3000"}
	}
	if opts.MetadataHost == "" {
		opts.MetadataHost = "localhost"
	}
	if opts.MetadataPort == "" {
		opts.MetadataPort = "5432"
	}
	if opts.MetadataUser == "" {
		opts.MetadataUser = "postgres"
	}
	if opts.MetadataDBName == "" {
		opts.MetadataDBName = "backup_manager"
	}
	if opts.MetadataSSLMode == "" {
		opts.MetadataSSLMode = "disable"
	}
	if opts.TargetPort == "" {
		opts.TargetPort = "5432"
	}
	if opts.TargetSSLMode == "" {
		opts.TargetSSLMode = "disable"
	}
	if opts.EncryptionMode == "" {
		opts.EncryptionMode = "passphrase"
	}
}

func prepareSecrets(opts *InitOptions) error {
	if opts.MetadataPassword == "" {
		return fmt.Errorf("metadata database password is required (--metadata-password or interactive prompt)")
	}

	switch opts.EncryptionMode {
	case "none":
		if opts.IsProduction() && !opts.AllowUnencryptedBackups {
			return fmt.Errorf("encryption mode \"none\" is not allowed in production unless --allow-unencrypted is set")
		}
	case "passphrase":
		if opts.EncryptionPassphrase == "" {
			token, err := randomToken(32)
			if err != nil {
				return err
			}
			opts.EncryptionPassphrase = token
		}
	case "keypair":
		if opts.AgePublicKey == "" && opts.AgePrivateKey == "" {
			pub, priv, err := appcrypto.GenerateX25519KeyPair()
			if err != nil {
				return fmt.Errorf("generate Age keypair: %w", err)
			}
			opts.AgePublicKey = pub
			opts.AgePrivateKey = priv
		} else if opts.AgePublicKey == "" || opts.AgePrivateKey == "" {
			return fmt.Errorf("both --age-public-key and --age-private-key are required when supplying a keypair")
		}
	default:
		return fmt.Errorf("unknown encryption mode %q (expected passphrase, keypair or none)", opts.EncryptionMode)
	}

	if opts.AdminKey == "" {
		token, err := randomToken(32)
		if err != nil {
			return err
		}
		opts.AdminKey = "bm_" + token
	}
	if opts.ConfigEncryptKey == "" {
		token, err := randomToken(32)
		if err != nil {
			return err
		}
		opts.ConfigEncryptKey = token
	}
	if opts.MetadataBackupEmbed && opts.MetadataBackupPassphrase == "" {
		token, err := randomToken(32)
		if err != nil {
			return err
		}
		opts.MetadataBackupPassphrase = token
	}
	return nil
}

func validateInitDatabase(opts InitOptions) error {
	targetConfigured := opts.TargetHost != "" || opts.TargetUser != "" || opts.TargetDBName != "" || opts.TargetPassword != ""
	if targetConfigured && opts.TargetPassword == "" {
		return fmt.Errorf("target database password is required when a target database is configured")
	}
	return nil
}

// IsProduction mirrors Config.IsProduction for init-time checks.
func (o InitOptions) IsProduction() bool {
	return o.Env == "production" || o.Env == "prod"
}

func normalizeOutputPath(path, fallback string) (string, error) {
	if path == "" {
		path = fallback
	}
	info, err := os.Stat(path)
	if err == nil && info.IsDir() {
		path = filepath.Join(path, fallback)
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	return abs, nil
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func writeFileAtomic(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".backup-manager-init-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Chmod(mode); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		return err
	}
	// Rename preserves the temp file mode, but enforce it explicitly for
	// filesystems or umasks that behave differently.
	return os.Chmod(path, mode)
}

func randomToken(bytes int) (string, error) {
	buf := make([]byte, bytes)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func renderSecrets(opts InitOptions) string {
	var b strings.Builder
	b.WriteString("# Backup Manager secrets.\n")
	b.WriteString("# Keep this file private (chmod 600) and never commit it.\n")
	b.WriteString("# Keys are referenced from config.yaml under `secrets:`.\n\n")
	writeSecret(&b, "METADATA_DB_PASSWORD", opts.MetadataPassword)
	writeSecret(&b, "TARGET_DB_PASSWORD", opts.TargetPassword)
	writeSecret(&b, "AGE_PRIVATE_KEY", opts.AgePrivateKey)
	writeSecret(&b, "ENCRYPTION_PASSPHRASE", opts.EncryptionPassphrase)
	writeSecret(&b, "ADMIN_KEY", opts.AdminKey)
	writeSecret(&b, "CONFIG_ENCRYPT_KEY", opts.ConfigEncryptKey)
	writeSecret(&b, "METADATA_BACKUP_PASSPHRASE", opts.MetadataBackupPassphrase)
	return b.String()
}

func writeSecret(b *strings.Builder, key, value string) {
	b.WriteString(key)
	b.WriteByte('=')
	b.WriteString(EncodeSecretValue(value))
	b.WriteByte('\n')
}

func renderConfig(opts InitOptions, secretsFile string) string {
	var b strings.Builder

	b.WriteString("# Backup Manager configuration (schema version 1).\n")
	b.WriteString("#\n")
	b.WriteString("# This file contains no secrets. Secret values are read from the file\n")
	b.WriteString("# referenced by `secrets_file` (chmod 600, never commit it).\n")
	b.WriteString("#\n")
	b.WriteString("# Validate before deploying:\n")
	b.WriteString("#   backup-manager config check --connect\n\n")

	b.WriteString("version: 1\n\n")

	b.WriteString("server:\n")
	fmt.Fprintf(&b, "  port: %s\n", yamlScalar(opts.Port))
	fmt.Fprintf(&b, "  env: %s\n", yamlScalar(opts.Env))
	fmt.Fprintf(&b, "  log_level: %s\n", yamlScalar(opts.LogLevel))
	writeYAMLList(&b, "  cors_origins", opts.CORSOrigins)
	writeYAMLList(&b, "  trusted_proxies", opts.TrustedProxies)
	b.WriteString("  rate_limit:\n")
	b.WriteString("    requests: 100\n")
	b.WriteString("    window: \"1m\"\n\n")

	b.WriteString("storage:\n")
	fmt.Fprintf(&b, "  path: %s\n", yamlScalar(opts.StoragePath))
	fmt.Fprintf(&b, "  temp_dir: %s\n\n", yamlScalar(opts.TempDir))

	fmt.Fprintf(&b, "secrets_file: %s\n\n", yamlScalar(secretsFile))

	b.WriteString("# Logical secret names -> keys in secrets_file.\n")
	b.WriteString("secrets:\n")
	b.WriteString("  metadata_db_password: \"METADATA_DB_PASSWORD\"\n")
	b.WriteString("  target_db_password: \"TARGET_DB_PASSWORD\"\n")
	b.WriteString("  age_private_key: \"AGE_PRIVATE_KEY\"\n")
	b.WriteString("  encryption_passphrase: \"ENCRYPTION_PASSPHRASE\"\n")
	b.WriteString("  admin_key: \"ADMIN_KEY\"\n")
	b.WriteString("  config_encrypt_key: \"CONFIG_ENCRYPT_KEY\"\n")
	b.WriteString("  metadata_backup_passphrase: \"METADATA_BACKUP_PASSPHRASE\"\n\n")

	b.WriteString("metadata_database:\n")
	writeDatabaseConfig(&b, opts.MetadataHost, opts.MetadataPort, opts.MetadataUser, opts.MetadataDBName, opts.MetadataSSLMode)
	b.WriteString("\n")

	b.WriteString("# Leave host/user/dbname empty to disable backups until a target is ready.\n")
	b.WriteString("target_database:\n")
	writeDatabaseConfig(&b, opts.TargetHost, opts.TargetPort, opts.TargetUser, opts.TargetDBName, opts.TargetSSLMode)
	b.WriteString("\n")

	b.WriteString("encryption:\n")
	fmt.Fprintf(&b, "  age_public_key: %s\n", yamlScalar(opts.AgePublicKey))
	fmt.Fprintf(&b, "  allow_unencrypted_backups: %t\n\n", opts.AllowUnencryptedBackups)

	b.WriteString("metadata_backup:\n")
	fmt.Fprintf(&b, "  schedule: %s\n", yamlScalar(opts.MetadataBackupSchedule))
	fmt.Fprintf(&b, "  retention: %d\n", opts.MetadataBackupRetention)
	fmt.Fprintf(&b, "  embed: %t\n\n", opts.MetadataBackupEmbed)

	b.WriteString("health:\n")
	b.WriteString("  overdue_factor: 2.0\n")
	b.WriteString("  overdue_grace: \"15m\"\n\n")

	b.WriteString("audit:\n")
	b.WriteString("  retention_days: 90\n")
	b.WriteString("  cleanup_schedule: \"0 4 * * *\"\n")

	return b.String()
}

func writeDatabaseConfig(b *strings.Builder, host, port, user, dbname, sslmode string) {
	fmt.Fprintf(b, "  host: %s\n", yamlScalar(host))
	fmt.Fprintf(b, "  port: %s\n", yamlScalar(port))
	fmt.Fprintf(b, "  user: %s\n", yamlScalar(user))
	fmt.Fprintf(b, "  dbname: %s\n", yamlScalar(dbname))
	fmt.Fprintf(b, "  sslmode: %s\n", yamlScalar(sslmode))
}

func writeYAMLList(b *strings.Builder, key string, values []string) {
	if len(values) == 0 {
		fmt.Fprintf(b, "%s: []\n", key)
		return
	}
	fmt.Fprintf(b, "%s:\n", key)
	for _, value := range values {
		fmt.Fprintf(b, "    - %s\n", yamlScalar(value))
	}
}

func yamlScalar(value string) string {
	encoded, err := yaml.Marshal(value)
	if err != nil {
		return fmt.Sprintf("%q", value)
	}
	return strings.TrimSuffix(string(encoded), "\n")
}
