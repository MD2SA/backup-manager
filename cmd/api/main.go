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
	"github.com/MD2SA/backup-manager/internal/logger"
	"github.com/MD2SA/backup-manager/internal/metarestore"
	"github.com/MD2SA/backup-manager/internal/pkg/crypto"
)

// @title Backup Manager API
// @version 1.0
// @description This is a backup management server for Postgres databases.
// @host localhost:8080
// @BasePath /api/v1

func main() {
	// Simple command line interface
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "keygen":
			pub, priv, err := crypto.GenerateX25519KeyPair()
			if err != nil {
				fmt.Fprintf(os.Stderr, "Failed to generate keys: %v\n", err)
				os.Exit(1)
			}
			fmt.Println("--- Backup Manager Encryption Keys ---")
			fmt.Printf("APP_AGE_PUBLIC_KEY:  %s\n", pub)
			fmt.Printf("APP_AGE_PRIVATE_KEY: %s\n", priv)
			fmt.Println("---------------------------------------")
			fmt.Println("CRITICAL: Save the private key! You cannot restore backups without it.")
			return
		case "metadata-restore":
			metadataRestore()
			return
		}
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	application, err := app.New(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Fatal error while starting the application: %v\n", err)
		os.Exit(1)
	}

	defer application.DB.Close()

	application.Start(ctx)

	server := &http.Server{
		Addr:    ":" + application.Config.Port,
		Handler: application.Router,
	}

	serverErrors := make(chan error, 1)

	go func() {
		application.Logger.Info("Starting server", "port", application.Config.Port)
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

		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()

		if err := server.Shutdown(shutdownCtx); err != nil {
			application.Logger.Error("Graceful shutdown failed", "error", err)
			_ = server.Close()
		}

		application.Logger.Info("Server shut down successfully")
	}
}

// metadataRestore implements the `backup-manager metadata-restore` maintenance
// command. It decrypts a metadata snapshot (explicit --file or the newest local
// snapshot) and restores it into the metadata database with pg_restore.
func metadataRestore() {
	fs := flag.NewFlagSet("metadata-restore", flag.ExitOnError)
	file := fs.String("file", "", "path to a snapshot file (default: newest snapshot in APP_STORAGE_PATH/metadata)")
	passphrase := fs.String("passphrase", "", "override APP_METADATA_BACKUP_PASSPHRASE")
	identity := fs.String("identity", "", "override APP_AGE_PRIVATE_KEY")
	replace := fs.Bool("replace", false, "restore over an existing database (pg_restore --clean --if-exists)")
	dryRun := fs.Bool("dry-run", false, "only validate the snapshot, write nothing")
	_ = fs.Parse(os.Args[2:])

	cfg, err := config.LoadLenient()
	if err != nil {
		fatal("Configuration error: %v", err)
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

func fatal(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
