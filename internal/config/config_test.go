package config

import (
	"testing"
)

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
