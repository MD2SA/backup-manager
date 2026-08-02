package config

import (
	"strings"

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
	Port       string
	MetadataDB DatabaseConfig
	TargetDB   DatabaseConfig
}

func Load() Config {
	// Load .env file if it exists (local development)
	_ = godotenv.Load()

	viper.SetEnvPrefix("APP")
	viper.AutomaticEnv()
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	// Default port
	viper.SetDefault("port", "8080")

	// Metadata Database Defaults (Internal state)
	// We check for both APP_METADATA_DB_* and legacy APP_DATABASE_* for compatibility
	metadataHost := getEnvWithFallback("metadata_db.host", "database.host", "localhost")
	metadataPort := getEnvWithFallback("metadata_db.port", "database.port", "5432")
	metadataUser := getEnvWithFallback("metadata_db.user", "database.user", "postgres")
	metadataPass := getEnvWithFallback("metadata_db.password", "database.password", "postgres")
	metadataName := getEnvWithFallback("metadata_db.dbname", "database.dbname", "backup_manager")
	metadataSSL := getEnvWithFallback("metadata_db.sslmode", "database.sslmode", "disable")

	// Target Database Defaults (The one to be backed up)
	viper.SetDefault("target_db.host", "localhost")
	viper.SetDefault("target_db.port", "5432")
	viper.SetDefault("target_db.user", "postgres")
	viper.SetDefault("target_db.password", "postgres")
	viper.SetDefault("target_db.dbname", "target_db")
	viper.SetDefault("target_db.sslmode", "disable")

	return Config{
		Port: viper.GetString("port"),
		MetadataDB: DatabaseConfig{
			Host:     metadataHost,
			Port:     metadataPort,
			User:     metadataUser,
			Password: metadataPass,
			DBName:   metadataName,
			SSLMode:  metadataSSL,
		},
		TargetDB: DatabaseConfig{
			Host:     viper.GetString("target_db.host"),
			Port:     viper.GetString("target_db.port"),
			User:     viper.GetString("target_db.user"),
			Password: viper.GetString("target_db.password"),
			DBName:   viper.GetString("target_db.dbname"),
			SSLMode:  viper.GetString("target_db.sslmode"),
		},
	}
}

func getEnvWithFallback(key, fallbackKey, defaultValue string) string {
	viper.SetDefault(key, "")
	viper.SetDefault(fallbackKey, defaultValue)

	val := viper.GetString(key)
	if val == "" {
		return viper.GetString(fallbackKey)
	}
	return val
}
