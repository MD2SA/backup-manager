package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"log/slog"
	"net/mail"
	"strings"
	"time"

	"context"
	"github.com/MD2SA/backup-manager/internal/repository"
	"github.com/MD2SA/backup-manager/internal/repository/db"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

var (
	ErrUnauthorized   = errors.New("unauthorized")
	ErrUserExists     = errors.New("user already exists")
	ErrUserNotFound   = errors.New("user not found")
	ErrKeyNotFound    = errors.New("api key not found")
	ErrKeyForbidden   = errors.New("api key does not belong to this user")
	ErrLastActiveUser = errors.New("cannot deactivate the last active user")
	ErrInactiveUser   = errors.New("user is inactive")
	ErrNoBootstrapKey = errors.New("no users exist and ADMIN_KEY is empty")
	ErrNoActiveUsers  = errors.New("no active users exist; create one with the CLI")
	ErrInvalidUser    = errors.New("email and name are required")
	ErrInvalidKeyName = errors.New("API key name is required")
)

// Authenticator is implemented by Service and consumed by the HTTP middleware.
type Authenticator interface {
	Authenticate(ctx context.Context, rawKey string) (Principal, error)
}

// Service owns identity, API keys and audit records.
type Service struct {
	repo     repository.Repository
	logger   *slog.Logger
	insecure bool
}

func NewService(repo repository.Repository, logger *slog.Logger) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{repo: repo, logger: logger}
}

// Bootstrap creates the first admin user from ADMIN_KEY when no users exist.
// If both the database and ADMIN_KEY are empty, development may run in insecure
// mode; production configuration already refuses to start without a key.
func (s *Service) Bootstrap(ctx context.Context, adminKey string, allowInsecure bool) error {
	adminKey = strings.TrimSpace(adminKey)
	if adminKey != "" {
		created, err := s.repo.BootstrapAdmin(ctx, HashKey(adminKey), KeyPrefix(adminKey))
		if err != nil {
			return err
		}
		if created {
			s.logger.Info("Bootstrap admin created", "email", "admin@localhost")
		}
		return s.ensureActiveUser(ctx)
	}

	count, err := s.repo.CountUsers(ctx)
	if err != nil {
		return err
	}
	if count == 0 {
		if !allowInsecure {
			return ErrNoBootstrapKey
		}
		s.insecure = true
		s.logger.Warn("SECURITY WARNING: no users and no ADMIN_KEY; running in insecure mode")
		return nil
	}
	return s.ensureActiveUser(ctx)
}

func (s *Service) ensureActiveUser(ctx context.Context) error {
	if s.insecure {
		return nil
	}
	active, err := s.repo.CountActiveUsers(ctx)
	if err != nil {
		return err
	}
	if active == 0 {
		return ErrNoActiveUsers
	}
	return nil
}

func (s *Service) InsecureMode() bool {
	return s.insecure
}

// Authenticate resolves an API key into a principal.
func (s *Service) Authenticate(ctx context.Context, rawKey string) (Principal, error) {
	if s.insecure {
		return Principal{Email: "insecure@localhost", Name: "Insecure Mode"}, nil
	}

	rawKey = strings.TrimSpace(rawKey)
	if rawKey == "" {
		return Principal{}, ErrUnauthorized
	}

	key, err := s.repo.GetAPIKeyByHash(ctx, HashKey(rawKey))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Principal{}, ErrUnauthorized
		}
		return Principal{}, err
	}
	if key.RevokedAt.Valid {
		return Principal{}, ErrUnauthorized
	}
	if key.ExpiresAt.Valid && time.Now().After(key.ExpiresAt.Time) {
		return Principal{}, ErrUnauthorized
	}

	user, err := s.repo.GetUser(ctx, key.UserID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Principal{}, ErrUnauthorized
		}
		return Principal{}, err
	}
	if !user.Active {
		return Principal{}, ErrUnauthorized
	}

	_ = s.repo.TouchAPIKey(ctx, key.ID)

	return Principal{
		UserID:    user.ID,
		Email:     user.Email,
		Name:      user.Name,
		KeyID:     key.ID,
		KeyPrefix: key.KeyPrefix,
	}, nil
}

// GenerateAPIKey creates a new high-entropy key and returns the plaintext,
// hash and display prefix. The plaintext is never stored.
func GenerateAPIKey() (raw string, hash []byte, prefix string, err error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", nil, "", err
	}
	raw = "bm_" + base64.RawURLEncoding.EncodeToString(buf)
	return raw, HashKey(raw), KeyPrefix(raw), nil
}

func HashKey(raw string) []byte {
	sum := sha256.Sum256([]byte(raw))
	return sum[:]
}

func KeyPrefix(raw string) string {
	const prefixLen = 12
	if len(raw) <= prefixLen {
		return raw
	}
	return raw[:prefixLen]
}

func (s *Service) CreateUser(ctx context.Context, email, name string) (db.User, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	name = strings.TrimSpace(name)
	if email == "" || name == "" {
		return db.User{}, ErrInvalidUser
	}
	if _, err := mail.ParseAddress(email); err != nil {
		return db.User{}, ErrInvalidUser
	}
	user, err := s.repo.CreateUser(ctx, db.CreateUserParams{Email: email, Name: name})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return db.User{}, ErrUserExists
		}
		return db.User{}, err
	}
	return user, nil
}

func (s *Service) GetUser(ctx context.Context, id pgtype.UUID) (db.User, error) {
	user, err := s.repo.GetUser(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return db.User{}, ErrUserNotFound
	}
	return user, err
}

func (s *Service) GetUserByEmail(ctx context.Context, email string) (db.User, error) {
	user, err := s.repo.GetUserByEmail(ctx, strings.ToLower(strings.TrimSpace(email)))
	if errors.Is(err, pgx.ErrNoRows) {
		return db.User{}, ErrUserNotFound
	}
	return user, err
}

func (s *Service) ListUsers(ctx context.Context) ([]db.User, error) {
	return s.repo.ListUsers(ctx)
}

func (s *Service) UpdateUser(ctx context.Context, id pgtype.UUID, email, name string, active bool) (db.User, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	name = strings.TrimSpace(name)
	if email == "" || name == "" {
		return db.User{}, ErrInvalidUser
	}
	if _, err := mail.ParseAddress(email); err != nil {
		return db.User{}, ErrInvalidUser
	}
	existing, err := s.repo.GetUser(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return db.User{}, ErrUserNotFound
	}
	if err != nil {
		return db.User{}, err
	}
	if existing.Active && !active {
		activeCount, err := s.repo.CountActiveUsers(ctx)
		if err != nil {
			return db.User{}, err
		}
		if activeCount <= 1 {
			return db.User{}, ErrLastActiveUser
		}
	}

	user, err := s.repo.UpdateUser(ctx, db.UpdateUserParams{
		ID:     id,
		Email:  email,
		Name:   name,
		Active: active,
	})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return db.User{}, ErrUserExists
		}
		if errors.Is(err, pgx.ErrNoRows) {
			return db.User{}, ErrUserNotFound
		}
		return db.User{}, err
	}
	if !active {
		if err := s.repo.RevokeAllUserAPIKeys(ctx, id); err != nil {
			return db.User{}, err
		}
	}
	return user, nil
}

func (s *Service) DeactivateUser(ctx context.Context, id pgtype.UUID) (db.User, error) {
	existing, err := s.repo.GetUser(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return db.User{}, ErrUserNotFound
	}
	if err != nil {
		return db.User{}, err
	}
	if !existing.Active {
		return existing, nil
	}

	activeCount, err := s.repo.CountActiveUsers(ctx)
	if err != nil {
		return db.User{}, err
	}
	if activeCount <= 1 {
		return db.User{}, ErrLastActiveUser
	}

	user, err := s.repo.DeactivateUser(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return db.User{}, ErrUserNotFound
	}
	if err != nil {
		return db.User{}, err
	}
	if err := s.repo.RevokeAllUserAPIKeys(ctx, id); err != nil {
		return db.User{}, err
	}
	return user, nil
}

// CreateAPIKey issues a key for a user and returns the plaintext exactly once.
func (s *Service) CreateAPIKey(
	ctx context.Context,
	userID pgtype.UUID,
	name string,
	expiresAt pgtype.Timestamptz,
) (db.ApiKey, string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return db.ApiKey{}, "", ErrInvalidKeyName
	}
	user, err := s.repo.GetUser(ctx, userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return db.ApiKey{}, "", ErrUserNotFound
	}
	if err != nil {
		return db.ApiKey{}, "", err
	}
	if !user.Active {
		return db.ApiKey{}, "", ErrInactiveUser
	}

	raw, hash, prefix, err := GenerateAPIKey()
	if err != nil {
		return db.ApiKey{}, "", err
	}

	key, err := s.repo.CreateAPIKey(ctx, db.CreateAPIKeyParams{
		UserID:    userID,
		Name:      name,
		KeyPrefix: prefix,
		KeyHash:   hash,
		ExpiresAt: expiresAt,
	})
	if err != nil {
		return db.ApiKey{}, "", err
	}
	return key, raw, nil
}

func (s *Service) ListAPIKeys(ctx context.Context, userID pgtype.UUID) ([]db.ApiKey, error) {
	return s.repo.ListAPIKeysByUser(ctx, userID)
}

func (s *Service) ListAllAPIKeys(ctx context.Context) ([]db.ApiKey, error) {
	return s.repo.ListAPIKeys(ctx)
}

func (s *Service) RevokeAPIKey(ctx context.Context, keyID pgtype.UUID) (db.ApiKey, error) {
	key, err := s.repo.RevokeAPIKey(ctx, keyID)
	if errors.Is(err, pgx.ErrNoRows) {
		return db.ApiKey{}, ErrKeyNotFound
	}
	return key, err
}

func (s *Service) RevokeAPIKeyForUser(ctx context.Context, keyID, userID pgtype.UUID) (db.ApiKey, error) {
	key, err := s.repo.GetAPIKey(ctx, keyID)
	if errors.Is(err, pgx.ErrNoRows) {
		return db.ApiKey{}, ErrKeyNotFound
	}
	if err != nil {
		return db.ApiKey{}, err
	}
	if key.UserID != userID {
		return db.ApiKey{}, ErrKeyForbidden
	}
	return s.RevokeAPIKey(ctx, keyID)
}
