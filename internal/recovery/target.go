package recovery

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/MD2SA/backup-manager/internal/backup"
	"github.com/MD2SA/backup-manager/internal/config"
	"github.com/jackc/pgx/v5"
)

// ParseTargetDSN converts a PostgreSQL DSN into a RestoreTarget.
func ParseTargetDSN(dsn string) (backup.RestoreTarget, error) {
	parsed, err := pgx.ParseConfig(dsn)
	if err != nil {
		return backup.RestoreTarget{}, err
	}
	sslMode := "disable"
	if u, parseErr := url.Parse(dsn); parseErr == nil {
		if mode := u.Query().Get("sslmode"); mode != "" {
			sslMode = mode
		}
	}
	port := int(parsed.Port)
	if port == 0 {
		port = 5432
	}
	return backup.RestoreTarget{
		Host:     parsed.Host,
		Port:     strconv.Itoa(port),
		User:     parsed.User,
		Password: parsed.Password,
		DBName:   parsed.Database,
		SSLMode:  sslMode,
	}, nil
}

// ResolveTarget returns the target from an explicit DSN or the config. It
// refuses to restore into the metadata database, even when the caller passes a
// DSN explicitly.
func ResolveTarget(dsn string, cfg config.Config) (backup.RestoreTarget, error) {
	var target backup.RestoreTarget
	if dsn != "" {
		parsed, err := ParseTargetDSN(dsn)
		if err != nil {
			return backup.RestoreTarget{}, err
		}
		target = parsed
	} else {
		target = backup.RestoreTarget{
			Host:     cfg.TargetDB.Host,
			Port:     cfg.TargetDB.Port,
			User:     cfg.TargetDB.User,
			Password: cfg.TargetDB.Password,
			DBName:   cfg.TargetDB.DBName,
			SSLMode:  cfg.TargetDB.SSLMode,
		}
		if cfg.TargetDB.RestoreUser != "" {
			target.User = cfg.TargetDB.RestoreUser
			target.Password = cfg.TargetDB.RestorePassword
		}
		if target.Host == "" || target.DBName == "" {
			return backup.RestoreTarget{}, fmt.Errorf("target database is not configured; use --target-dsn")
		}
	}

	if sameDatabaseTarget(target, cfg.MetadataDB) {
		return backup.RestoreTarget{}, fmt.Errorf("refusing to restore into the metadata database %q", target.DBName)
	}
	return target, nil
}

func sameDatabaseTarget(target backup.RestoreTarget, metadata config.DatabaseConfig) bool {
	return target.DBName != "" &&
		strings.EqualFold(target.Host, metadata.Host) &&
		target.Port == metadata.Port &&
		strings.EqualFold(target.DBName, metadata.DBName)
}
