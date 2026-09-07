package config

import (
	"testing"
)

func TestLoad_MetadataBackupEmbedDefault(t *testing.T) {
	// viper.SetEnvPrefix is "APP" and AutomaticEnv; a bare Config struct cannot
	// reflect those defaults. We exercise Load() with a minimal valid env.
	base := map[string]string{
		"APP_TEMP_DIR":             "/tmp",
		"APP_METADATA_DB_HOST":     "localhost",
		"APP_METADATA_DB_PORT":     "5432",
		"APP_METADATA_DB_USER":     "u",
		"APP_METADATA_DB_PASSWORD": "p",
		"APP_METADATA_DB_DBNAME":   "db",
		"APP_METADATA_DB_SSLMODE":  "disable",
	}

	setEnv := func(t *testing.T) {
		t.Helper()
		for k, v := range base {
			t.Setenv(k, v)
		}
	}

	t.Run("unset uses default true", func(t *testing.T) {
		setEnv(t)
		cfg, err := Load()
		if err != nil {
			t.Fatalf("Load() error: %v", err)
		}
		if !cfg.MetadataBackupEmbed {
			t.Errorf("expected MetadataBackupEmbed=true by default, got false")
		}
	})

	t.Run("explicitly disabled via env", func(t *testing.T) {
		setEnv(t)
		t.Setenv("APP_METADATA_BACKUP_EMBED", "false")
		cfg, err := Load()
		if err != nil {
			t.Fatalf("Load() error: %v", err)
		}
		if cfg.MetadataBackupEmbed {
			t.Errorf("expected MetadataBackupEmbed=false when env is false, got true")
		}
	})
}

func TestConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		config  Config
		wantErr bool
	}{
		{
			name: "Valid metadata DB, missing target DB should pass startup validation",
			config: Config{
				TempDir: "/tmp",
				MetadataDB: DatabaseConfig{
					Host:     "localhost",
					Port:     "5432",
					User:     "user",
					Password: "pass",
					DBName:   "db",
					SSLMode:  "disable",
				},
			},
			wantErr: false,
		},
		{
			name: "Missing metadata DB should fail startup validation",
			config: Config{
				TempDir: "/tmp",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.config.Validate(); (err != nil) != tt.wantErr {
				t.Errorf("Config.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestConfig_AdminKeyRequiredInProduction(t *testing.T) {
	valid := Config{
		TempDir: "/tmp",
		Env:     "production",
		MetadataDB: DatabaseConfig{
			Host: "localhost", Port: "5432", User: "u", Password: "p", DBName: "db", SSLMode: "disable",
		},
	}

	if err := valid.Validate(); err == nil {
		t.Fatal("expected production config without AdminKey to fail validation")
	}

	valid.AdminKey = "secret"
	if err := valid.Validate(); err != nil {
		t.Fatalf("production config with AdminKey should pass: %v", err)
	}
}

func TestParseList(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  []string
	}{
		{name: "empty", value: "", want: nil},
		{name: "single", value: "http://localhost:3000", want: []string{"http://localhost:3000"}},
		{name: "multiple with spaces", value: "http://a.com, http://b.com ,", want: []string{"http://a.com", "http://b.com"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseList(tt.value)
			if len(got) != len(tt.want) {
				t.Fatalf("parseList(%q) = %v, want %v", tt.value, got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("parseList(%q)[%d] = %q, want %q", tt.value, i, got[i], tt.want[i])
				}
			}
		})
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
	tests := []struct {
		name    string
		config  Config
		wantErr bool
	}{
		{
			name: "Complete target DB should pass",
			config: Config{
				TargetDB: DatabaseConfig{
					Host:     "1.2.3.4",
					Port:     "5435",
					User:     "user",
					Password: "pass",
					DBName:   "db",
					SSLMode:  "disable",
				},
			},
			wantErr: false,
		},
		{
			name: "Incomplete target DB should fail",
			config: Config{
				TargetDB: DatabaseConfig{
					Host: "localhost",
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.config.ValidateTargetDB(); (err != nil) != tt.wantErr {
				t.Errorf("Config.ValidateTargetDB() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
