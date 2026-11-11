package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/MD2SA/backup-manager/internal/config"
	"golang.org/x/term"
)

func runInit(args []string) {
	opts := config.InitOptions{
		ConfigPath:              "config.yaml",
		Env:                     "development",
		Port:                    "8080",
		LogLevel:                "info",
		StoragePath:             "/backups",
		TempDir:                 "/tmp",
		CORSOrigins:             []string{"http://localhost:3000"},
		MetadataHost:            "localhost",
		MetadataPort:            "5432",
		MetadataUser:            "postgres",
		MetadataDBName:          "backup_manager",
		MetadataSSLMode:         "disable",
		TargetPort:              "5432",
		TargetSSLMode:           "disable",
		MetadataBackupRetention: 14,
		MetadataBackupEmbed:     true,
	}

	// Presets make the Docker quick start a single command. Explicit flags still
	// win because they are parsed after these defaults are applied.
	dockerPreset := hasTrueFlag(args, "--docker")
	devPreset := hasTrueFlag(args, "--dev")
	if dockerPreset || devPreset {
		opts.MetadataHost = "metadata-db"
		opts.StoragePath = "/backups"
		opts.TempDir = "/tmp"
		// When run through the Makefile, the bundled database credentials are
		// passed explicitly so the generated files match Compose.
		opts.MetadataUser = envOr("POSTGRES_USER", opts.MetadataUser)
		opts.MetadataDBName = envOr("POSTGRES_DB", opts.MetadataDBName)
		opts.MetadataPassword = envOr("POSTGRES_PASSWORD", opts.MetadataPassword)
	}
	if devPreset {
		opts.TargetHost = "target-db"
		opts.TargetPort = "5432"
		opts.TargetUser = envOr("POSTGRES_USER", "postgres")
		opts.TargetDBName = envOr("TARGET_DB_NAME", "project_db")
		opts.TargetSSLMode = "disable"
		opts.TargetPassword = envOr("POSTGRES_PASSWORD", opts.TargetPassword)
	}

	fs := flag.NewFlagSet("init", flag.ExitOnError)
	fs.StringVar(&opts.ConfigPath, "config", opts.ConfigPath, "path of the config.yaml to create")
	fs.StringVar(&opts.SecretsPath, "secrets", opts.SecretsPath, "path of the secrets file (default: secrets.env next to config)")
	fs.BoolVar(&opts.Force, "force", false, "overwrite existing files")
	fs.BoolVar(&opts.AllowUnencryptedBackups, "allow-unencrypted", false, "allow production without backup encryption")
	fs.Bool("docker", false, "preset for the bundled Docker Compose stack (metadata-db, /backups, /tmp)")
	fs.Bool("dev", false, "preset for docker-compose.dev.yml (adds the target-db service)")

	fs.StringVar(&opts.Env, "env", opts.Env, "server.env (development, staging, production)")
	fs.StringVar(&opts.Port, "port", opts.Port, "HTTP port")
	fs.StringVar(&opts.LogLevel, "log-level", opts.LogLevel, "debug, info, warn or error")
	fs.StringVar(&opts.StoragePath, "storage-path", opts.StoragePath, "directory where backups are stored")
	fs.StringVar(&opts.TempDir, "temp-dir", opts.TempDir, "temporary directory for staging backups")
	cors := fs.String("cors-origins", strings.Join(opts.CORSOrigins, ","), "comma-separated browser origins")
	proxies := fs.String("trusted-proxies", strings.Join(opts.TrustedProxies, ","), "comma-separated proxy IPs/CIDRs")

	fs.StringVar(&opts.MetadataHost, "metadata-host", opts.MetadataHost, "metadata database host")
	fs.StringVar(&opts.MetadataPort, "metadata-port", opts.MetadataPort, "metadata database port")
	fs.StringVar(&opts.MetadataUser, "metadata-user", opts.MetadataUser, "metadata database user")
	fs.StringVar(&opts.MetadataDBName, "metadata-db", opts.MetadataDBName, "metadata database name")
	fs.StringVar(&opts.MetadataSSLMode, "metadata-sslmode", opts.MetadataSSLMode, "metadata database sslmode")
	fs.StringVar(&opts.MetadataPassword, "metadata-password", opts.MetadataPassword, "metadata database password")

	fs.StringVar(&opts.TargetHost, "target-host", opts.TargetHost, "target database host (optional)")
	fs.StringVar(&opts.TargetPort, "target-port", opts.TargetPort, "target database port")
	fs.StringVar(&opts.TargetUser, "target-user", opts.TargetUser, "target database user")
	fs.StringVar(&opts.TargetDBName, "target-db", opts.TargetDBName, "target database name")
	fs.StringVar(&opts.TargetSSLMode, "target-sslmode", opts.TargetSSLMode, "target database sslmode")
	fs.StringVar(&opts.TargetPassword, "target-password", opts.TargetPassword, "target database password")

	fs.StringVar(&opts.EncryptionMode, "encryption", opts.EncryptionMode, "backup encryption: keypair, passphrase or none")
	fs.StringVar(&opts.EncryptionPassphrase, "encryption-passphrase", opts.EncryptionPassphrase, "encryption passphrase (generated when empty)")
	fs.StringVar(&opts.AgePublicKey, "age-public-key", opts.AgePublicKey, "Age X25519 public key")
	fs.StringVar(&opts.AgePrivateKey, "age-private-key", opts.AgePrivateKey, "Age X25519 private key")
	fs.StringVar(&opts.AdminKey, "admin-key", opts.AdminKey, "bootstrap admin API key (generated when empty)")
	fs.StringVar(&opts.ConfigEncryptKey, "config-key", opts.ConfigEncryptKey, "provider config encryption key (generated when empty)")

	fs.StringVar(&opts.MetadataBackupSchedule, "metadata-backup-schedule", opts.MetadataBackupSchedule, "cron for local metadata self-backup")
	fs.StringVar(&opts.MetadataBackupPassphrase, "metadata-backup-passphrase", opts.MetadataBackupPassphrase, "metadata snapshot passphrase (generated when empty)")
	fs.IntVar(&opts.MetadataBackupRetention, "metadata-backup-retention", opts.MetadataBackupRetention, "metadata snapshots to keep")
	fs.BoolVar(&opts.MetadataBackupEmbed, "metadata-backup-embed", opts.MetadataBackupEmbed, "embed encrypted metadata snapshots in backups")

	interactive := fs.Bool("interactive", term.IsTerminal(int(os.Stdin.Fd())), "ask questions interactively")
	nonInteractive := fs.Bool("non-interactive", false, "never prompt; all required values must be provided as flags")
	_ = fs.Parse(args)

	opts.CORSOrigins = parseCSV(*cors)
	opts.TrustedProxies = parseCSV(*proxies)
	if opts.EncryptionMode == "" {
		if opts.AgePublicKey != "" || opts.AgePrivateKey != "" {
			opts.EncryptionMode = "keypair"
		} else {
			opts.EncryptionMode = "passphrase"
		}
	}
	if *nonInteractive {
		*interactive = false
	}
	if *interactive {
		promptInit(&opts)
	}

	result, err := config.Init(opts)
	if err != nil {
		fatal("Setup failed: %v", err)
	}

	fmt.Println()
	fmt.Println("Configuration created:")
	fmt.Printf("  config:  %s\n", result.ConfigPath)
	fmt.Printf("  secrets: %s (chmod 600)\n", result.SecretsPath)
	fmt.Printf("  admin API key: %s\n", result.AdminKey)
	fmt.Println()
	fmt.Println("Next steps:")
	fmt.Println("  1. Review the config file and keep the secrets file private.")
	fmt.Println("  2. Validate: backup-manager config check --connect")
	fmt.Println("     Docker: docker compose run --rm backup-manager config check --connect")
	fmt.Println("  3. Start the service: backup-manager serve")
	fmt.Println("  (Docker: mount config.yaml at /etc/backup-manager/config.yaml and secrets.env alongside it.)")
}

func promptInit(opts *config.InitOptions) {
	reader := bufio.NewReader(os.Stdin)

	fmt.Println("Backup Manager interactive setup")
	fmt.Println("Press Enter to accept the value in brackets.")
	fmt.Println()

	opts.MetadataHost = promptString(reader, "Metadata DB host", opts.MetadataHost)
	opts.MetadataPort = promptString(reader, "Metadata DB port", opts.MetadataPort)
	opts.MetadataUser = promptString(reader, "Metadata DB user", opts.MetadataUser)
	opts.MetadataDBName = promptString(reader, "Metadata DB name", opts.MetadataDBName)
	opts.MetadataSSLMode = promptString(reader, "Metadata DB sslmode", opts.MetadataSSLMode)
	if opts.MetadataPassword == "" {
		opts.MetadataPassword = promptPassword("Metadata DB password")
	}

	if opts.TargetHost != "" || promptBool(reader, "Configure the target database now?", false) {
		opts.TargetHost = promptString(reader, "Target DB host", opts.TargetHost)
		opts.TargetPort = promptString(reader, "Target DB port", opts.TargetPort)
		opts.TargetUser = promptString(reader, "Target DB user", opts.TargetUser)
		opts.TargetDBName = promptString(reader, "Target DB name", opts.TargetDBName)
		opts.TargetSSLMode = promptString(reader, "Target DB sslmode", opts.TargetSSLMode)
		if opts.TargetPassword == "" {
			opts.TargetPassword = promptPassword("Target DB password")
		}
	} else {
		opts.TargetHost = ""
		opts.TargetUser = ""
		opts.TargetDBName = ""
		opts.TargetPassword = ""
	}

	opts.StoragePath = promptString(reader, "Backup storage path (use /backups in Docker)", opts.StoragePath)
	opts.TempDir = promptString(reader, "Temporary directory", opts.TempDir)
	opts.Env = promptChoice(reader, "Environment", []string{"development", "staging", "production"}, opts.Env)
	opts.EncryptionMode = promptChoice(reader, "Backup encryption", []string{"keypair", "passphrase", "none"}, opts.EncryptionMode)

	if opts.EncryptionMode == "passphrase" && opts.EncryptionPassphrase == "" {
		value := promptPassword("Encryption passphrase (leave empty to generate one)")
		if value != "" {
			opts.EncryptionPassphrase = value
		}
	}

	opts.MetadataBackupSchedule = promptString(reader, "Local metadata self-backup cron (empty to disable)", opts.MetadataBackupSchedule)
	cors := promptString(reader, "Allowed CORS origins (comma-separated)", strings.Join(opts.CORSOrigins, ","))
	opts.CORSOrigins = parseCSV(cors)
	proxies := promptString(reader, "Trusted proxy IPs/CIDRs (comma-separated)", strings.Join(opts.TrustedProxies, ","))
	opts.TrustedProxies = parseCSV(proxies)
}

func promptString(reader *bufio.Reader, label, def string) string {
	if def != "" {
		fmt.Printf("%s [%s]: ", label, def)
	} else {
		fmt.Printf("%s: ", label)
	}
	line, _ := reader.ReadString('\n')
	line = strings.TrimSpace(line)
	if line == "" {
		return def
	}
	return line
}

func promptPassword(label string) string {
	fmt.Printf("%s: ", label)
	value, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Println()
	if err != nil {
		fatal("Could not read password: %v", err)
	}
	return strings.TrimSpace(string(value))
}

func promptBool(reader *bufio.Reader, label string, def bool) bool {
	for {
		suffix := "y/N"
		if def {
			suffix = "Y/n"
		}
		fmt.Printf("%s [%s]: ", label, suffix)
		line, _ := reader.ReadString('\n')
		switch strings.ToLower(strings.TrimSpace(line)) {
		case "":
			return def
		case "y", "yes":
			return true
		case "n", "no":
			return false
		}
		fmt.Println("Please answer yes or no.")
	}
}

func promptChoice(reader *bufio.Reader, label string, options []string, def string) string {
	for {
		fmt.Printf("%s (%s) [%s]: ", label, strings.Join(options, "/"), def)
		line, _ := reader.ReadString('\n')
		line = strings.ToLower(strings.TrimSpace(line))
		if line == "" {
			return def
		}
		for _, option := range options {
			if line == option {
				return option
			}
		}
		fmt.Println("Invalid choice.")
	}
}

func parseCSV(value string) []string {
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func hasTrueFlag(args []string, name string) bool {
	for _, arg := range args {
		if arg == name {
			return true
		}
		if strings.HasPrefix(arg, name+"=") {
			value := strings.TrimPrefix(arg, name+"=")
			return value == "true" || value == "1"
		}
	}
	return false
}
