package config

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"filippo.io/age"
	"github.com/robfig/cron/v3"
	"gopkg.in/yaml.v3"
)

const (
	// CurrentVersion is the schema version accepted by this build.
	CurrentVersion = 1

	// DefaultConfigPath is used when neither --config nor APP_CONFIG_PATH is set.
	DefaultConfigPath = "config.yaml"

	// DefaultSecretsFile is used when the config does not set secrets_file.
	DefaultSecretsFile = "secrets.env"
)

// DatabaseConfig describes a PostgreSQL connection.
type DatabaseConfig struct {
	Host            string
	Port            string
	User            string
	Password        string
	DBName          string
	SSLMode         string
	RestoreUser     string
	RestorePassword string
}

// DSN renders the connection string used by pgx, psql and pg_dump helpers.
func (d DatabaseConfig) DSN() string {
	u := &url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(d.User, d.Password),
		Host:   net.JoinHostPort(d.Host, d.Port),
		Path:   "/" + d.DBName,
	}
	q := u.Query()
	q.Set("sslmode", d.SSLMode)
	u.RawQuery = q.Encode()
	return u.String()
}

func (d DatabaseConfig) configured() bool {
	return d.Host != "" || d.User != "" || d.DBName != "" || d.Password != ""
}

// Config is the resolved runtime configuration. It is loaded from a YAML file
// plus a companion secrets file; no value is taken from the process environment
// except the config/secrets paths.
type Config struct {
	ConfigPath  string
	SecretsPath string

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
	AllowUnencryptedBackups  bool
	OverdueFactor            float64
	OverdueGrace             time.Duration
	AuditRetentionDays       int
	AuditCleanupSchedule     string
	MetadataDB               DatabaseConfig
	TargetDB                 DatabaseConfig
}

// fileConfig mirrors the YAML document. Defaults are applied before decoding,
// so omitted fields keep their documented default.
type fileConfig struct {
	Version          int                      `yaml:"version"`
	Server           serverFileConfig         `yaml:"server"`
	Storage          storageFileConfig        `yaml:"storage"`
	SecretsFile      string                   `yaml:"secrets_file"`
	Secrets          secretRefs               `yaml:"secrets"`
	MetadataDatabase databaseFileConfig       `yaml:"metadata_database"`
	TargetDatabase   databaseFileConfig       `yaml:"target_database"`
	Encryption       encryptionFileConfig     `yaml:"encryption"`
	MetadataBackup   metadataBackupFileConfig `yaml:"metadata_backup"`
	Health           healthFileConfig         `yaml:"health"`
	Audit            auditFileConfig          `yaml:"audit"`
}

type serverFileConfig struct {
	Port           string              `yaml:"port"`
	Env            string              `yaml:"env"`
	LogLevel       string              `yaml:"log_level"`
	CORSOrigins    []string            `yaml:"cors_origins"`
	TrustedProxies []string            `yaml:"trusted_proxies"`
	RateLimit      rateLimitFileConfig `yaml:"rate_limit"`
}

type rateLimitFileConfig struct {
	Requests int    `yaml:"requests"`
	Window   string `yaml:"window"`
}

type storageFileConfig struct {
	Path    string `yaml:"path"`
	TempDir string `yaml:"temp_dir"`
}

type databaseFileConfig struct {
	Host                  string `yaml:"host"`
	Port                  string `yaml:"port"`
	User                  string `yaml:"user"`
	DBName                string `yaml:"dbname"`
	SSLMode               string `yaml:"sslmode"`
	RestoreUser           string `yaml:"restore_user"`
	RestorePasswordSecret string `yaml:"restore_password_secret"`
}

type encryptionFileConfig struct {
	AgePublicKey            string `yaml:"age_public_key"`
	AllowUnencryptedBackups bool   `yaml:"allow_unencrypted_backups"`
}

type metadataBackupFileConfig struct {
	Schedule  string `yaml:"schedule"`
	Retention int    `yaml:"retention"`
	Embed     bool   `yaml:"embed"`
}

type healthFileConfig struct {
	OverdueFactor float64 `yaml:"overdue_factor"`
	OverdueGrace  string  `yaml:"overdue_grace"`
}

type auditFileConfig struct {
	RetentionDays   int    `yaml:"retention_days"`
	CleanupSchedule string `yaml:"cleanup_schedule"`
}

// secretRefs maps logical secrets to keys inside the secrets file. Keeping the
// mapping explicit lets operators reuse an existing secret store naming scheme.
type secretRefs struct {
	MetadataDBPassword       string `yaml:"metadata_db_password"`
	TargetDBPassword         string `yaml:"target_db_password"`
	TargetDBRestorePassword  string `yaml:"target_db_restore_password"`
	AgePrivateKey            string `yaml:"age_private_key"`
	EncryptionPassphrase     string `yaml:"encryption_passphrase"`
	AdminKey                 string `yaml:"admin_key"`
	ConfigEncryptKey         string `yaml:"config_encrypt_key"`
	MetadataBackupPassphrase string `yaml:"metadata_backup_passphrase"`
}

func defaultSecretRefs() secretRefs {
	return secretRefs{
		MetadataDBPassword:       "METADATA_DB_PASSWORD",
		TargetDBPassword:         "TARGET_DB_PASSWORD",
		TargetDBRestorePassword:  "TARGET_DB_RESTORE_PASSWORD",
		AgePrivateKey:            "AGE_PRIVATE_KEY",
		EncryptionPassphrase:     "ENCRYPTION_PASSPHRASE",
		AdminKey:                 "ADMIN_KEY",
		ConfigEncryptKey:         "CONFIG_ENCRYPT_KEY",
		MetadataBackupPassphrase: "METADATA_BACKUP_PASSPHRASE",
	}
}

func defaultFileConfig() fileConfig {
	return fileConfig{
		Server: serverFileConfig{
			Port:        "8080",
			Env:         "development",
			LogLevel:    "info",
			CORSOrigins: []string{"http://localhost:3000"},
			RateLimit: rateLimitFileConfig{
				Requests: 100,
				Window:   "1m",
			},
		},
		Storage: storageFileConfig{
			Path:    "/backups",
			TempDir: "/tmp",
		},
		SecretsFile: DefaultSecretsFile,
		Secrets:     defaultSecretRefs(),
		MetadataDatabase: databaseFileConfig{
			Port:    "5432",
			SSLMode: "disable",
		},
		TargetDatabase: databaseFileConfig{
			Port:    "5432",
			SSLMode: "disable",
		},
		Encryption: encryptionFileConfig{
			AllowUnencryptedBackups: false,
		},
		MetadataBackup: metadataBackupFileConfig{
			Retention: 14,
			Embed:     true,
		},
		Health: healthFileConfig{
			OverdueFactor: 2.0,
			OverdueGrace:  "15m",
		},
		Audit: auditFileConfig{
			RetentionDays:   90,
			CleanupSchedule: "0 4 * * *",
		},
	}
}

// ResolvePath applies the CLI/env precedence for the config file location.
func ResolvePath(explicit string) string {
	if explicit != "" {
		return explicit
	}
	if p := os.Getenv("APP_CONFIG_PATH"); p != "" {
		return p
	}
	return DefaultConfigPath
}

// Load reads, validates and resolves the configuration.
func Load(path string) (Config, error) {
	return load(path, true)
}

// LoadLenient behaves like Load but skips rules that only apply to the API
// server (e.g. the mandatory admin key in production). It is used by
// maintenance commands such as migrate and metadata-restore.
func LoadLenient(path string) (Config, error) {
	return load(path, false)
}

func load(path string, requireAdminKey bool) (Config, error) {
	return loadFrom(path, "", requireAdminKey)
}

// loadFrom is load with an explicit secrets path override. Init uses it to
// validate the files it just wrote even when APP_SECRETS_FILE is set in the
// environment for the running service.
func loadFrom(path, secretsPathOverride string, requireAdminKey bool) (Config, error) {
	path = ResolvePath(path)

	raw, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("read config %s: %w", path, err)
	}

	file := defaultFileConfig()
	dec := yaml.NewDecoder(bytes.NewReader(raw))
	dec.KnownFields(true)
	if err := dec.Decode(&file); err != nil {
		return Config{}, fmt.Errorf("parse config %s: %w", path, err)
	}
	if file.Version != CurrentVersion {
		return Config{}, fmt.Errorf(
			"unsupported config version %d in %s (this build supports version %d)",
			file.Version, path, CurrentVersion,
		)
	}

	secretsPath := secretsPathOverride
	if secretsPath == "" {
		secretsPath = os.Getenv("APP_SECRETS_FILE")
	}
	if secretsPath == "" {
		secretsPath = file.SecretsFile
	}
	if secretsPath == "" {
		secretsPath = DefaultSecretsFile
	}
	if !filepath.IsAbs(secretsPath) {
		secretsPath = filepath.Join(filepath.Dir(path), secretsPath)
	}
	secretsPath = filepath.Clean(secretsPath)

	secrets, err := loadSecrets(secretsPath)
	if err != nil {
		return Config{}, err
	}

	refs := file.Secrets
	applySecretDefaults(&refs)

	window, err := time.ParseDuration(file.Server.RateLimit.Window)
	if err != nil {
		return Config{}, fmt.Errorf("server.rate_limit.window: %w", err)
	}
	overdueGrace, err := time.ParseDuration(file.Health.OverdueGrace)
	if err != nil {
		return Config{}, fmt.Errorf("health.overdue_grace: %w", err)
	}

	cfg := Config{
		ConfigPath:               path,
		SecretsPath:              secretsPath,
		Port:                     file.Server.Port,
		LogLevel:                 file.Server.LogLevel,
		Env:                      file.Server.Env,
		TempDir:                  file.Storage.TempDir,
		StoragePath:              file.Storage.Path,
		AgePublicKey:             file.Encryption.AgePublicKey,
		AgePrivateKey:            secrets[refs.AgePrivateKey],
		EncryptionPassphrase:     secrets[refs.EncryptionPassphrase],
		AdminKey:                 secrets[refs.AdminKey],
		ConfigEncryptionKey:      secrets[refs.ConfigEncryptKey],
		RateLimitRequests:        file.Server.RateLimit.Requests,
		RateLimitWindow:          window,
		CORSOrigins:              file.Server.CORSOrigins,
		TrustedProxies:           file.Server.TrustedProxies,
		MetadataBackupSchedule:   file.MetadataBackup.Schedule,
		MetadataBackupPassphrase: secrets[refs.MetadataBackupPassphrase],
		MetadataBackupRetention:  file.MetadataBackup.Retention,
		MetadataBackupEmbed:      file.MetadataBackup.Embed,
		AllowUnencryptedBackups:  file.Encryption.AllowUnencryptedBackups,
		OverdueFactor:            file.Health.OverdueFactor,
		OverdueGrace:             overdueGrace,
		AuditRetentionDays:       file.Audit.RetentionDays,
		AuditCleanupSchedule:     file.Audit.CleanupSchedule,
		MetadataDB: DatabaseConfig{
			Host:     file.MetadataDatabase.Host,
			Port:     file.MetadataDatabase.Port,
			User:     file.MetadataDatabase.User,
			Password: secrets[refs.MetadataDBPassword],
			DBName:   file.MetadataDatabase.DBName,
			SSLMode:  file.MetadataDatabase.SSLMode,
		},
		TargetDB: DatabaseConfig{
			Host:            file.TargetDatabase.Host,
			Port:            file.TargetDatabase.Port,
			User:            file.TargetDatabase.User,
			Password:        secrets[refs.TargetDBPassword],
			DBName:          file.TargetDatabase.DBName,
			SSLMode:         file.TargetDatabase.SSLMode,
			RestoreUser:     file.TargetDatabase.RestoreUser,
			RestorePassword: secrets[refs.TargetDBRestorePassword],
		},
	}

	if err := errors.Join(
		cfg.validate(requireAdminKey),
		cfg.validateSecrets(secretsPath, secrets, refs, requireAdminKey),
	); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

func applySecretDefaults(refs *secretRefs) {
	defaults := defaultSecretRefs()
	if refs.MetadataDBPassword == "" {
		refs.MetadataDBPassword = defaults.MetadataDBPassword
	}
	if refs.TargetDBPassword == "" {
		refs.TargetDBPassword = defaults.TargetDBPassword
	}
	if refs.TargetDBRestorePassword == "" {
		refs.TargetDBRestorePassword = defaults.TargetDBRestorePassword
	}
	if refs.AgePrivateKey == "" {
		refs.AgePrivateKey = defaults.AgePrivateKey
	}
	if refs.EncryptionPassphrase == "" {
		refs.EncryptionPassphrase = defaults.EncryptionPassphrase
	}
	if refs.AdminKey == "" {
		refs.AdminKey = defaults.AdminKey
	}
	if refs.ConfigEncryptKey == "" {
		refs.ConfigEncryptKey = defaults.ConfigEncryptKey
	}
	if refs.MetadataBackupPassphrase == "" {
		refs.MetadataBackupPassphrase = defaults.MetadataBackupPassphrase
	}
}

// validateSecrets reports missing secret values using both the logical name and
// the file where they were expected, which is far more actionable than an empty
// password error later.
func (c *Config) validateSecrets(path string, secrets map[string]string, refs secretRefs, requireAdminKey bool) error {
	var errs []error

	require := func(ref, what string) {
		if secrets[ref] == "" {
			errs = append(errs, fmt.Errorf("%s: secret %q is missing or empty in %s", what, ref, path))
		}
	}

	require(refs.MetadataDBPassword, "metadata_database")
	if c.TargetDB.configured() {
		require(refs.TargetDBPassword, "target_database")
		if c.TargetDB.RestoreUser != "" {
			require(refs.TargetDBRestorePassword, "target_database restore")
		}
	}
	if requireAdminKey && c.IsProduction() {
		require(refs.ConfigEncryptKey, "secrets.config_encrypt_key")
	}

	return errors.Join(errs...)
}

// Validate checks the configuration as used by the API server.
func (c *Config) Validate() error {
	return c.validate(true)
}

func (c *Config) validate(requireAdminKey bool) error {
	var errs []error
	add := func(format string, args ...any) {
		errs = append(errs, fmt.Errorf(format, args...))
	}

	if c.Port == "" {
		add("server.port is required")
	} else if p, err := strconv.Atoi(c.Port); err != nil || p < 1 || p > 65535 {
		add("server.port must be a number between 1 and 65535")
	}

	switch c.LogLevel {
	case "debug", "info", "warn", "error":
	default:
		add("server.log_level must be one of debug, info, warn, error")
	}

	if c.Env == "" {
		add("server.env is required (development, staging or production)")
	}

	if c.RateLimitRequests <= 0 {
		add("server.rate_limit.requests must be greater than zero")
	}
	if c.RateLimitWindow <= 0 {
		add("server.rate_limit.window must be a positive duration (e.g. 1m)")
	}

	if c.StoragePath == "" {
		add("storage.path is required")
	}
	if c.TempDir == "" {
		add("storage.temp_dir is required")
	}

	validateDatabase("metadata_database", c.MetadataDB, add)
	if c.TargetDB.configured() {
		validateDatabase("target_database", c.TargetDB, add)
		if sameDatabase(c.MetadataDB, c.TargetDB) {
			add("target_database must not point to the same database as metadata_database")
		}
		if c.TargetDB.RestoreUser != "" && c.TargetDB.RestorePassword == "" {
			add("target_database restore password is required when restore_user is set")
		}
	}

	if c.MetadataBackupRetention < 0 {
		add("metadata_backup.retention must be zero or greater")
	}
	if c.OverdueFactor < 1 {
		add("health.overdue_factor must be at least 1")
	}
	if c.OverdueGrace < 0 {
		add("health.overdue_grace must be zero or greater")
	}
	if c.AuditRetentionDays < 0 {
		add("audit.retention_days must be zero or greater")
	}
	if c.AuditRetentionDays > 0 {
		if c.AuditCleanupSchedule == "" {
			add("audit.cleanup_schedule is required when audit.retention_days > 0")
		} else if _, err := cron.ParseStandard(c.AuditCleanupSchedule); err != nil {
			add("audit.cleanup_schedule is not a valid 5-field cron expression: %v", err)
		}
	}
	if c.MetadataBackupSchedule != "" {
		if _, err := cron.ParseStandard(c.MetadataBackupSchedule); err != nil {
			add("metadata_backup.schedule is not a valid 5-field cron expression: %v", err)
		}
	}

	if _, err := normalizeTrustedProxies(c.TrustedProxies); err != nil {
		add("%v", err)
	}

	if c.AgePublicKey != "" {
		if _, err := age.ParseX25519Recipient(c.AgePublicKey); err != nil {
			add("encryption.age_public_key is not a valid Age X25519 recipient: %v", err)
		}
	}
	if c.AgePrivateKey != "" {
		identity, err := age.ParseX25519Identity(c.AgePrivateKey)
		if err != nil {
			add("encryption age private key is not a valid Age X25519 identity: %v", err)
		} else if c.AgePublicKey != "" && identity.Recipient().String() != c.AgePublicKey {
			add("encryption.age_public_key does not match the configured age private key")
		}
	}

	if requireAdminKey && c.IsProduction() {
		if c.ConfigEncryptionKey == "" {
			add("the provider config encryption key is required when server.env=production (secrets.config_encrypt_key)")
		}
		if !c.AllowUnencryptedBackups && c.AgePublicKey == "" && c.EncryptionPassphrase == "" {
			add("production requires backup encryption (set encryption.age_public_key or the encryption passphrase); set encryption.allow_unencrypted_backups=true to override")
		}
	}

	return errors.Join(errs...)
}

func validateDatabase(field string, db DatabaseConfig, add func(string, ...any)) {
	if db.Host == "" {
		add("%s.host is required", field)
	}
	if db.Port == "" {
		add("%s.port is required", field)
	}
	if db.User == "" {
		add("%s.user is required", field)
	}
	if db.Password == "" {
		add("%s password is required", field)
	}
	if db.DBName == "" {
		add("%s.dbname is required", field)
	}
	if db.SSLMode == "" {
		add("%s.sslmode is required", field)
	}
}

func sameDatabase(a, b DatabaseConfig) bool {
	return strings.EqualFold(a.Host, b.Host) &&
		a.Port == b.Port &&
		a.DBName == b.DBName
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
			return netip.Prefix{}, errors.New("invalid proxy in server.trusted_proxies: " + raw)
		}
		return p, nil
	}

	addr, err := netip.ParseAddr(raw)
	if err != nil {
		return netip.Prefix{}, errors.New("invalid proxy in server.trusted_proxies: " + raw)
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
	var errs []error
	if c.TargetDB.Host == "" {
		errs = append(errs, errors.New("target database host is missing (target_database.host)"))
	}
	if c.TargetDB.Port == "" {
		errs = append(errs, errors.New("target database port is missing (target_database.port)"))
	}
	if c.TargetDB.User == "" {
		errs = append(errs, errors.New("target database user is missing (target_database.user)"))
	}
	if c.TargetDB.Password == "" {
		errs = append(errs, errors.New("target database password is missing (target_database secret)"))
	}
	if c.TargetDB.DBName == "" {
		errs = append(errs, errors.New("target database name is missing (target_database.dbname)"))
	}
	if c.TargetDB.SSLMode == "" {
		errs = append(errs, errors.New("target database SSL mode is missing (target_database.sslmode)"))
	}
	return errors.Join(errs...)
}

// loadSecrets parses a KEY=VALUE file. Blank lines and lines starting with '#'
// are ignored. Values may be single- or double-quoted; inline '#' is kept as
// part of the value so generated passwords containing it survive.
func loadSecrets(path string) (map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("read secrets %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()

	out := make(map[string]string)
	scanner := bufio.NewScanner(f)
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		key, value, ok := strings.Cut(line, "=")
		if !ok {
			return nil, fmt.Errorf("%s:%d: expected KEY=VALUE", path, lineNo)
		}
		key = strings.TrimSpace(key)
		if key == "" {
			return nil, fmt.Errorf("%s:%d: empty secret name", path, lineNo)
		}
		if _, exists := out[key]; exists {
			return nil, fmt.Errorf("%s:%d: duplicate secret %q", path, lineNo, key)
		}

		out[key] = parseSecretValue(strings.TrimSpace(value))
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read secrets %s: %w", path, err)
	}
	return out, nil
}

func parseSecretValue(value string) string {
	if len(value) >= 2 && value[0] == '"' && value[len(value)-1] == '"' {
		if unquoted, err := strconv.Unquote(value); err == nil {
			return unquoted
		}
	}
	if len(value) >= 2 && value[0] == '\'' && value[len(value)-1] == '\'' {
		return value[1 : len(value)-1]
	}
	return value
}

// EncodeSecretValue renders a value for the secrets file, quoting it only when
// needed to preserve whitespace or leading '#'. Newlines are rejected by Init.
func EncodeSecretValue(value string) string {
	if value == "" {
		return ""
	}
	if strings.ContainsAny(value, " \t\"'") || strings.HasPrefix(value, "#") {
		return strconv.Quote(value)
	}
	return value
}
