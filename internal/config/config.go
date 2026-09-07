package config

import (
	"errors"
	"net/netip"
	"strings"
	"time"

	"github.com/joho/godotenv"
	"github.com/spf13/viper"
)

type DatabaseConfig struct {
	Host     string
	Port     string
	User     string
	Password string
	DBName   string
	SSLMode  string
}

type Config struct {
	Port                     string
	LogLevel                 string
	Env                      string
	TempDir                  string
	StoragePath              string
	AgePublicKey             string
	AgePrivateKey            string
	EncryptionPassphrase     string
	AdminKey                 string
	ConfigEncryptionKey      string
	RateLimitRequests        int
	RateLimitWindow          time.Duration
	CORSOrigins              []string
	TrustedProxies           []string
	MetadataBackupSchedule   string
	MetadataBackupPassphrase string
	MetadataBackupRetention  int
	MetadataBackupEmbed      bool
	MetadataDB               DatabaseConfig
	TargetDB                 DatabaseConfig
}

func Load() (Config, error) {
	// Load .env file if it exists (local development)
	_ = godotenv.Load()

	viper.SetEnvPrefix("APP")
	viper.AutomaticEnv()
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	// Global settings
	viper.SetDefault("port", "8080")
	viper.SetDefault("log_level", "info")
	viper.SetDefault("env", "development")
	viper.SetDefault("storage_path", "/backups")
	viper.SetDefault("rate_limit_requests", 100)
	viper.SetDefault("rate_limit_window", "1m")
	viper.SetDefault("cors_origins", "http://localhost:3000")
	viper.SetDefault("metadata_backup_retention", 14)
	viper.SetDefault("metadata_backup_embed", true)

	// Metadata Database (Internal state)
	metadataHost := viper.GetString("metadata_db.host")
	metadataPort := viper.GetString("metadata_db.port")
	metadataUser := viper.GetString("metadata_db.user")
	metadataPass := viper.GetString("metadata_db.password")
	metadataName := viper.GetString("metadata_db.dbname")
	metadataSSL := viper.GetString("metadata_db.sslmode")

	// Target Database (The one to be backed up)
	targetHost := viper.GetString("target_db.host")
	targetPort := viper.GetString("target_db.port")
	targetUser := viper.GetString("target_db.user")
	targetPass := viper.GetString("target_db.password")
	targetName := viper.GetString("target_db.dbname")
	targetSSL := viper.GetString("target_db.sslmode")

	cfg := Config{
		Port:                     viper.GetString("port"),
		LogLevel:                 viper.GetString("log_level"),
		Env:                      viper.GetString("env"),
		TempDir:                  viper.GetString("temp_dir"),
		StoragePath:              viper.GetString("storage_path"),
		AgePublicKey:             viper.GetString("age_public_key"),
		AgePrivateKey:            viper.GetString("age_private_key"),
		EncryptionPassphrase:     viper.GetString("encryption_passphrase"),
		AdminKey:                 viper.GetString("admin_key"),
		ConfigEncryptionKey:      viper.GetString("config_encrypt_key"),
		RateLimitRequests:        viper.GetInt("rate_limit_requests"),
		RateLimitWindow:          viper.GetDuration("rate_limit_window"),
		CORSOrigins:              parseList(viper.GetString("cors_origins")),
		TrustedProxies:           parseList(viper.GetString("trusted_proxies")),
		MetadataBackupSchedule:   viper.GetString("metadata_backup_schedule"),
		MetadataBackupPassphrase: viper.GetString("metadata_backup_passphrase"),
		MetadataBackupRetention:  viper.GetInt("metadata_backup_retention"),
		MetadataBackupEmbed:      viper.GetBool("metadata_backup_embed"),
		MetadataDB: DatabaseConfig{
			Host:     metadataHost,
			Port:     metadataPort,
			User:     metadataUser,
			Password: metadataPass,
			DBName:   metadataName,
			SSLMode:  metadataSSL,
		},
		TargetDB: DatabaseConfig{
			Host:     targetHost,
			Port:     targetPort,
			User:     targetUser,
			Password: targetPass,
			DBName:   targetName,
			SSLMode:  targetSSL,
		},
	}

	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

func (c *Config) Validate() error {
	// Global validation
	if c.TempDir == "" {
		return errors.New("temporary directory is required (APP_TEMP_DIR)")
	}

	// In production the API must never run without an admin key (Insecure Mode).
	if c.IsProduction() && c.AdminKey == "" {
		return errors.New("APP_ADMIN_KEY is required when APP_ENV=production")
	}

	if _, err := normalizeTrustedProxies(c.TrustedProxies); err != nil {
		return err
	}

	// Validate Metadata DB (Mandatory for service startup)
	if c.MetadataDB.Host == "" {
		return errors.New("metadata database host is required (APP_METADATA_DB_HOST)")
	}
	if c.MetadataDB.Port == "" {
		return errors.New("metadata database port is required (APP_METADATA_DB_PORT)")
	}
	if c.MetadataDB.User == "" {
		return errors.New("metadata database user is required (APP_METADATA_DB_USER)")
	}
	if c.MetadataDB.Password == "" {
		return errors.New("metadata database password is required (APP_METADATA_DB_PASSWORD)")
	}
	if c.MetadataDB.DBName == "" {
		return errors.New("metadata database name is required (APP_METADATA_DB_DBNAME)")
	}
	if c.MetadataDB.SSLMode == "" {
		return errors.New("metadata database SSL mode is required (APP_METADATA_DB_SSLMODE)")
	}

	// Note: Target DB validation is deferred to runtime (ValidateTargetDB)
	// to allow the service to start even if infrastructure is not yet fully configured.

	return nil
}

// IsProduction reports whether the service runs in production mode.
func (c *Config) IsProduction() bool {
	return c.Env == "production" || c.Env == "prod"
}

// TrustedProxyPrefixes normalizes the configured trusted proxies into
// netip.Prefix values, converting bare IPs into host prefixes.
func (c *Config) TrustedProxyPrefixes() ([]netip.Prefix, error) {
	return normalizeTrustedProxies(c.TrustedProxies)
}

// parseList splits a comma-separated env value into a slice of trimmed items.
func parseList(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if trimmed := strings.TrimSpace(p); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

// normalizeTrustedProxies parses and normalizes proxy prefixes, upgrading
// bare addresses (e.g. "127.0.0.1") to host prefixes ("127.0.0.1/32").
func normalizeTrustedProxies(proxies []string) ([]netip.Prefix, error) {
	prefixes := make([]netip.Prefix, 0, len(proxies))
	for _, raw := range proxies {
		p, err := parsePrefix(raw)
		if err != nil {
			return nil, err
		}
		prefixes = append(prefixes, p)
	}
	return prefixes, nil
}

func parsePrefix(raw string) (netip.Prefix, error) {
	if strings.Contains(raw, "/") {
		p, err := netip.ParsePrefix(raw)
		if err != nil {
			return netip.Prefix{}, errors.New("invalid proxy in APP_TRUSTED_PROXIES: " + raw)
		}
		return p, nil
	}

	addr, err := netip.ParseAddr(raw)
	if err != nil {
		return netip.Prefix{}, errors.New("invalid proxy in APP_TRUSTED_PROXIES: " + raw)
	}
	return netip.PrefixFrom(addr.Unmap(), addr.BitLen()), nil
}

// IsTargetDBConfigured returns true if all mandatory Target DB fields are present.
func (c *Config) IsTargetDBConfigured() bool {
	return c.TargetDB.Host != "" &&
		c.TargetDB.Port != "" &&
		c.TargetDB.User != "" &&
		c.TargetDB.Password != "" &&
		c.TargetDB.DBName != ""
}

// ValidateTargetDB ensures the target database configuration is complete.
// This should be called before any backup or restore operation.
func (c *Config) ValidateTargetDB() error {
	if c.TargetDB.Host == "" {
		return errors.New("target database host is missing (APP_TARGET_DB_HOST)")
	}
	if c.TargetDB.Port == "" {
		return errors.New("target database port is missing (APP_TARGET_DB_PORT)")
	}
	if c.TargetDB.User == "" {
		return errors.New("target database user is missing (APP_TARGET_DB_USER)")
	}
	if c.TargetDB.Password == "" {
		return errors.New("target database password is missing (APP_TARGET_DB_PASSWORD)")
	}
	if c.TargetDB.DBName == "" {
		return errors.New("target database name is missing (APP_TARGET_DB_DBNAME)")
	}
	if c.TargetDB.SSLMode == "" {
		return errors.New("target database SSL mode is missing (APP_TARGET_DB_SSLMODE)")
	}
	return nil
}
