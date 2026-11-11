package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/MD2SA/backup-manager/internal/auth"
	"github.com/MD2SA/backup-manager/internal/config"
	"github.com/MD2SA/backup-manager/internal/database"
	"github.com/MD2SA/backup-manager/internal/logger"
	"github.com/MD2SA/backup-manager/internal/pkg/crypto"
	"github.com/MD2SA/backup-manager/internal/repository"
	"github.com/jackc/pgx/v5/pgtype"
)

func runUser(args []string) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: backup-manager user <create|list> [flags]")
		os.Exit(2)
	}

	switch args[0] {
	case "create":
		userCreate(args[1:])
	case "list":
		userList(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "unknown user command %q\n", args[0])
		os.Exit(2)
	}
}

func userCreate(args []string) {
	fs := flag.NewFlagSet("user create", flag.ExitOnError)
	configPath := fs.String("config", "", "path to config.yaml")
	email := fs.String("email", "", "user email")
	name := fs.String("name", "", "display name")
	migrationsDir := fs.String("migrations-dir", envOr("APP_MIGRATIONS_DIR", defaultMigrationsDir), "path to SQL migrations")
	noMigrate := fs.Bool("no-migrate", false, "skip migrations")
	_ = fs.Parse(args)

	service, cleanup := authCLI(*configPath, *migrationsDir, *noMigrate)
	defer cleanup()

	user, err := service.CreateUser(context.Background(), *email, *name)
	if err != nil {
		fatal("Create user failed: %v", err)
	}
	fmt.Printf("user created: id=%s email=%s name=%s\n", user.ID, user.Email, user.Name)
}

func userList(args []string) {
	fs := flag.NewFlagSet("user list", flag.ExitOnError)
	configPath := fs.String("config", "", "path to config.yaml")
	migrationsDir := fs.String("migrations-dir", envOr("APP_MIGRATIONS_DIR", defaultMigrationsDir), "path to SQL migrations")
	noMigrate := fs.Bool("no-migrate", false, "skip migrations")
	_ = fs.Parse(args)

	service, cleanup := authCLI(*configPath, *migrationsDir, *noMigrate)
	defer cleanup()

	users, err := service.ListUsers(context.Background())
	if err != nil {
		fatal("List users failed: %v", err)
	}
	for _, user := range users {
		fmt.Printf("%s\t%s\t%s\tactive=%t\n", user.ID, user.Email, user.Name, user.Active)
	}
}

func runKey(args []string) {
	if len(args) == 0 || args[0] != "create" {
		fmt.Fprintln(os.Stderr, "usage: backup-manager key create --email EMAIL --name NAME [flags]")
		os.Exit(2)
	}
	keyCreate(args[1:])
}

func keyCreate(args []string) {
	fs := flag.NewFlagSet("key create", flag.ExitOnError)
	configPath := fs.String("config", "", "path to config.yaml")
	email := fs.String("email", "", "user email")
	name := fs.String("name", "", "key name")
	expiresDays := fs.Int("expires-days", 0, "expiry in days (0 = no expiry)")
	migrationsDir := fs.String("migrations-dir", envOr("APP_MIGRATIONS_DIR", defaultMigrationsDir), "path to SQL migrations")
	noMigrate := fs.Bool("no-migrate", false, "skip migrations")
	_ = fs.Parse(args)

	service, cleanup := authCLI(*configPath, *migrationsDir, *noMigrate)
	defer cleanup()

	user, err := service.GetUserByEmail(context.Background(), *email)
	if err != nil {
		fatal("User not found: %v", err)
	}

	key, raw, err := service.CreateAPIKey(context.Background(), user.ID, *name, expiresAtFromDays(*expiresDays))
	if err != nil {
		fatal("Create API key failed: %v", err)
	}

	fmt.Printf("API key created for %s\n", user.Email)
	fmt.Printf("  id:     %s\n", key.ID)
	fmt.Printf("  prefix: %s\n", key.KeyPrefix)
	fmt.Printf("  key:    %s\n", raw)
	fmt.Println("Store this key now; it will not be shown again.")
}

func expiresAtFromDays(days int) pgtype.Timestamptz {
	if days <= 0 {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: time.Now().AddDate(0, 0, days), Valid: true}
}

func authCLI(configPath, migrationsDir string, noMigrate bool) (*auth.Service, func()) {
	cfg, err := config.LoadLenient(configPath)
	if err != nil {
		fatal("Configuration error:\n%v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	if !noMigrate {
		if err := database.Migrate(ctx, cfg.MetadataDB, migrationsDir, 60*time.Second); err != nil {
			fatal("Database migration failed: %v", err)
		}
	}

	pool, err := database.New(ctx, cfg.MetadataDB)
	if err != nil {
		fatal("Database connection failed: %v", err)
	}

	var encKey []byte
	if cfg.ConfigEncryptionKey != "" {
		encKey = crypto.DeriveConfigKey(cfg.ConfigEncryptionKey)
	}
	repo := repository.NewPostgres(pool, encKey)
	service := auth.NewService(repo, logger.New(cfg.LogLevel))

	return service, func() { pool.Close() }
}
