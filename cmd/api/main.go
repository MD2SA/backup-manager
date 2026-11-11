package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/MD2SA/backup-manager/internal/app"
	"github.com/MD2SA/backup-manager/internal/config"
	"github.com/MD2SA/backup-manager/internal/database"
	"github.com/MD2SA/backup-manager/internal/logger"
	"github.com/MD2SA/backup-manager/internal/metarestore"
	"github.com/MD2SA/backup-manager/internal/pkg/crypto"
)

var version = "dev"

const defaultMigrationsDir = "sql/migrations"

// @title Backup Manager API
// @version 1.0
// @description This is a backup management server for Postgres databases.
// @host localhost:8080
// @BasePath /api/v1
// @securityDefinitions.apikey ApiKeyAuth
// @in header
// @name X-API-Key

func main() {
	if len(os.Args) < 2 {
		runServe(nil)
		return
	}

	switch os.Args[1] {
	case "serve":
		runServe(os.Args[2:])
	case "init":
		runInit(os.Args[2:])
	case "config":
		runConfig(os.Args[2:])
	case "migrate":
		runMigrate(os.Args[2:])
	case "keygen":
		runKeygen()
	case "metadata-restore":
		runMetadataRestore(os.Args[2:])
	case "user":
		runUser(os.Args[2:])
	case "key":
		runKey(os.Args[2:])
	case "version", "--version":
		fmt.Println(version)
	case "help", "-h", "--help":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n", os.Args[1])
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `Backup Manager

Usage:
  backup-manager [serve] [--config PATH] [--no-migrate] [--migrations-dir DIR]
  backup-manager init [--config PATH] [--secrets PATH] [--non-interactive] [flags]
  backup-manager config check [--config PATH] [--connect]
  backup-manager migrate [--config PATH] [--migrations-dir DIR]
  backup-manager metadata-restore [--config PATH] [--file PATH] [flags]
  backup-manager user <create|list> [flags]
  backup-manager key create --email EMAIL --name NAME [flags]
  backup-manager keygen
  backup-manager version

The default command is "serve". Configuration is read from config.yaml
(or APP_CONFIG_PATH) and secrets from its companion secrets file.
`)
}

func runServe(args []string) {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	configPath := fs.String("config", "", "path to config.yaml (default: APP_CONFIG_PATH or ./config.yaml)")
	noMigrate := fs.Bool("no-migrate", false, "do not apply metadata database migrations on startup")
	migrationsDir := fs.String("migrations-dir", envOr("APP_MIGRATIONS_DIR", defaultMigrationsDir), "path to SQL migrations")
	_ = fs.Parse(args)

	cfg, err := config.Load(*configPath)
	if err != nil {
		fatal("Configuration error:\n%v", err)
	}

	log := logger.New(cfg.LogLevel)
	warnings, checkErr := config.Check(cfg, false)
	for _, warning := range warnings {
		log.Warn("Configuration warning", "warning", warning)
	}
	if checkErr != nil {
		fatal("Configuration check failed:\n%v", checkErr)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	if !*noMigrate {
		if err := database.Migrate(ctx, cfg.MetadataDB, *migrationsDir, 60*time.Second); err != nil {
			fatal("Database migration failed: %v", err)
		}
	}

	application, err := app.New(ctx, cfg)
	if err != nil {
		fatal("Fatal error while starting the application: %v", err)
	}
	defer application.DB.Close()
	defer application.Stop()

	application.Start(ctx)

	server := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           application.Router,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Minute,
		IdleTimeout:       2 * time.Minute,
		MaxHeaderBytes:    1 << 20,
	}

	serverErrors := make(chan error, 1)
	go func() {
		application.Logger.Info("Starting server", "port", cfg.Port, "env", cfg.Env, "config", cfg.ConfigPath)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			serverErrors <- err
		}
	}()

	select {
	case err := <-serverErrors:
		application.Logger.Error("Critical error in HTTP server", "error", err)
		os.Exit(1)
	case <-ctx.Done():
		application.Logger.Info("Shutting down server gracefully...")

		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer shutdownCancel()

		if err := server.Shutdown(shutdownCtx); err != nil {
			application.Logger.Error("Graceful shutdown failed", "error", err)
			_ = server.Close()
		}

		application.Logger.Info("Server shut down successfully")
	}
}

func runConfig(args []string) {
	if len(args) == 0 || args[0] != "check" {
		fmt.Fprintln(os.Stderr, "usage: backup-manager config check [--config PATH] [--connect]")
		os.Exit(2)
	}

	fs := flag.NewFlagSet("config check", flag.ExitOnError)
	configPath := fs.String("config", "", "path to config.yaml")
	connect := fs.Bool("connect", false, "also validate metadata and target database connectivity")
	_ = fs.Parse(args[1:])

	cfg, err := config.Load(*configPath)
	if err != nil {
		fatal("Configuration invalid:\n%v", err)
	}

	warnings, checkErr := config.Check(cfg, *connect)
	fmt.Print(cfg.RedactedSummary())
	for _, warning := range warnings {
		fmt.Fprintf(os.Stderr, "WARN: %s\n", warning)
	}
	if checkErr != nil {
		fatal("Configuration check failed:\n%v", checkErr)
	}
	fmt.Println("Configuration OK")
}

func runMigrate(args []string) {
	fs := flag.NewFlagSet("migrate", flag.ExitOnError)
	configPath := fs.String("config", "", "path to config.yaml")
	migrationsDir := fs.String("migrations-dir", envOr("APP_MIGRATIONS_DIR", defaultMigrationsDir), "path to SQL migrations")
	timeout := fs.Duration("timeout", 60*time.Second, "how long to wait for the metadata database")
	_ = fs.Parse(args)

	cfg, err := config.LoadLenient(*configPath)
	if err != nil {
		fatal("Configuration error:\n%v", err)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	if err := database.Migrate(ctx, cfg.MetadataDB, *migrationsDir, *timeout); err != nil {
		fatal("Database migration failed: %v", err)
	}
	fmt.Println("Migrations applied successfully")
}

func runKeygen() {
	pub, priv, err := crypto.GenerateX25519KeyPair()
	if err != nil {
		fatal("Failed to generate keys: %v", err)
	}
	fmt.Println("--- Backup Manager Encryption Keys ---")
	fmt.Printf("public key:  %s\n", pub)
	fmt.Printf("private key: %s\n", priv)
	fmt.Println("---------------------------------------")
	fmt.Println("Store the private key in your secrets file (AGE_PRIVATE_KEY).")
	fmt.Println("CRITICAL: without it you cannot restore Age-encrypted backups.")
}

func runMetadataRestore(args []string) {
	fs := flag.NewFlagSet("metadata-restore", flag.ExitOnError)
	configPath := fs.String("config", "", "path to config.yaml")
	file := fs.String("file", "", "path to a snapshot file (default: newest snapshot in storage.path/metadata)")
	passphrase := fs.String("passphrase", "", "override the metadata backup passphrase")
	identity := fs.String("identity", "", "override the Age private key")
	replace := fs.Bool("replace", false, "restore over an existing database (pg_restore --clean --if-exists)")
	dryRun := fs.Bool("dry-run", false, "only validate the snapshot, write nothing")
	_ = fs.Parse(args)

	cfg, err := config.LoadLenient(*configPath)
	if err != nil {
		fatal("Configuration error:\n%v", err)
	}

	log := logger.New(cfg.LogLevel)
	opts := metarestore.Options{
		File:       *file,
		Passphrase: *passphrase,
		Identity:   *identity,
		Replace:    *replace,
		DryRun:     *dryRun,
	}
	if err := metarestore.Run(log, cfg, opts); err != nil {
		fatal("Metadata restore failed: %v", err)
	}

	fmt.Println("Metadata restore completed")
}

func envOr(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func fatal(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
