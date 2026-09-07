package config

import (
	"errors"
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
	Port                 string
	LogLevel             string
	TempDir              string
	StoragePath          string
	AgePublicKey         string
	AgePrivateKey        string
	EncryptionPassphrase string
	AdminKey             string
	RateLimitRequests    int
	RateLimitWindow      time.Duration
	MetadataDB           DatabaseConfig
	TargetDB             DatabaseConfig
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
	viper.SetDefault("storage_path", "/backups")
	viper.SetDefault("rate_limit_requests", 100)
	viper.SetDefault("rate_limit_window", "1m")

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
		Port:                 viper.GetString("port"),
		LogLevel:             viper.GetString("log_level"),
		TempDir:              viper.GetString("temp_dir"),
		StoragePath:          viper.GetString("storage_path"),
		AgePublicKey:         viper.GetString("age_public_key"),
		AgePrivateKey:        viper.GetString("age_private_key"),
		EncryptionPassphrase: viper.GetString("encryption_passphrase"),
		AdminKey:             viper.GetString("admin_key"),
		RateLimitRequests:    viper.GetInt("rate_limit_requests"),
		RateLimitWindow:      viper.GetDuration("rate_limit_window"),
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

	// Validate Target DB
	if c.TargetDB.Host == "" {
		return errors.New("target database host is required (APP_TARGET_DB_HOST)")
	}
	if c.TargetDB.Port == "" {
		return errors.New("target database port is required (APP_TARGET_DB_PORT)")
	}
	if c.TargetDB.User == "" {
		return errors.New("target database user is required (APP_TARGET_DB_USER)")
	}
	if c.TargetDB.Password == "" {
		return errors.New("target database password is required (APP_TARGET_DB_PASSWORD)")
	}
	if c.TargetDB.DBName == "" {
		return errors.New("target database name is required (APP_TARGET_DB_DBNAME)")
	}
	if c.TargetDB.SSLMode == "" {
		return errors.New("target database SSL mode is required (APP_TARGET_DB_SSLMODE)")
	}

	return nil
}
