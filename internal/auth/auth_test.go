package auth

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/MD2SA/backup-manager/internal/repository"
	"github.com/MD2SA/backup-manager/internal/repository/db"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

type fakeRepo struct {
	repository.Repository

	users          map[pgtype.UUID]db.User
	usersByEmail   map[string]pgtype.UUID
	keys           map[pgtype.UUID]db.ApiKey
	keysByHash     map[string]pgtype.UUID
	bootstrapCalls int
	bootstrapHash  []byte
	bootstrapPref  string
	bootstrapOK    bool
	lastKeyUsed    pgtype.UUID
	auditEvents    []db.CreateAuditEntryParams
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{
		users:        make(map[pgtype.UUID]db.User),
		usersByEmail: make(map[string]pgtype.UUID),
		keys:         make(map[pgtype.UUID]db.ApiKey),
		keysByHash:   make(map[string]pgtype.UUID),
	}
}

func uuid(n byte) pgtype.UUID {
	return pgtype.UUID{Bytes: [16]byte{n}, Valid: true}
}

func (f *fakeRepo) BootstrapAdmin(_ context.Context, keyHash []byte, keyPrefix string) (bool, error) {
	f.bootstrapCalls++
	f.bootstrapHash = keyHash
	f.bootstrapPref = keyPrefix
	if f.bootstrapOK {
		user := db.User{ID: uuid(1), Email: "admin@localhost", Name: "Admin", Active: true}
		f.users[user.ID] = user
		f.usersByEmail[user.Email] = user.ID
	}
	return f.bootstrapOK, nil
}

func (f *fakeRepo) CountUsers(context.Context) (int64, error) {
	return int64(len(f.users)), nil
}

func (f *fakeRepo) CountActiveUsers(context.Context) (int64, error) {
	var count int64
	for _, user := range f.users {
		if user.Active {
			count++
		}
	}
	return count, nil
}

func (f *fakeRepo) CreateUser(_ context.Context, arg db.CreateUserParams) (db.User, error) {
	if _, exists := f.usersByEmail[arg.Email]; exists {
		return db.User{}, &pgconn.PgError{Code: "23505"}
	}
	user := db.User{
		ID:        uuid(byte(len(f.users) + 10)),
		Email:     arg.Email,
		Name:      arg.Name,
		Active:    true,
		CreatedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true},
		UpdatedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true},
	}
	f.users[user.ID] = user
	f.usersByEmail[user.Email] = user.ID
	return user, nil
}

func (f *fakeRepo) GetUser(_ context.Context, id pgtype.UUID) (db.User, error) {
	user, ok := f.users[id]
	if !ok {
		return db.User{}, pgx.ErrNoRows
	}
	return user, nil
}

func (f *fakeRepo) GetUserByEmail(_ context.Context, email string) (db.User, error) {
	id, ok := f.usersByEmail[email]
	if !ok {
		return db.User{}, pgx.ErrNoRows
	}
	return f.users[id], nil
}

func (f *fakeRepo) ListUsers(context.Context) ([]db.User, error) {
	var out []db.User
	for _, user := range f.users {
		out = append(out, user)
	}
	return out, nil
}

func (f *fakeRepo) UpdateUser(_ context.Context, arg db.UpdateUserParams) (db.User, error) {
	user, ok := f.users[arg.ID]
	if !ok {
		return db.User{}, pgx.ErrNoRows
	}
	user.Email, user.Name, user.Active = arg.Email, arg.Name, arg.Active
	user.UpdatedAt = pgtype.Timestamptz{Time: time.Now(), Valid: true}
	f.users[arg.ID] = user
	f.usersByEmail[arg.Email] = arg.ID
	return user, nil
}

func (f *fakeRepo) DeactivateUser(_ context.Context, id pgtype.UUID) (db.User, error) {
	user, ok := f.users[id]
	if !ok {
		return db.User{}, pgx.ErrNoRows
	}
	user.Active = false
	f.users[id] = user
	return user, nil
}

func (f *fakeRepo) CreateAPIKey(_ context.Context, arg db.CreateAPIKeyParams) (db.ApiKey, error) {
	key := db.ApiKey{
		ID:        uuid(byte(len(f.keys) + 50)),
		UserID:    arg.UserID,
		Name:      arg.Name,
		KeyPrefix: arg.KeyPrefix,
		KeyHash:   arg.KeyHash,
		ExpiresAt: arg.ExpiresAt,
		CreatedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true},
	}
	f.keys[key.ID] = key
	f.keysByHash[string(arg.KeyHash)] = key.ID
	return key, nil
}

func (f *fakeRepo) GetAPIKey(_ context.Context, id pgtype.UUID) (db.ApiKey, error) {
	key, ok := f.keys[id]
	if !ok {
		return db.ApiKey{}, pgx.ErrNoRows
	}
	return key, nil
}

func (f *fakeRepo) GetAPIKeyByHash(_ context.Context, hash []byte) (db.ApiKey, error) {
	id, ok := f.keysByHash[string(hash)]
	if !ok {
		return db.ApiKey{}, pgx.ErrNoRows
	}
	return f.keys[id], nil
}

func (f *fakeRepo) ListAPIKeysByUser(_ context.Context, userID pgtype.UUID) ([]db.ApiKey, error) {
	var out []db.ApiKey
	for _, key := range f.keys {
		if key.UserID == userID {
			out = append(out, key)
		}
	}
	return out, nil
}

func (f *fakeRepo) ListAPIKeys(context.Context) ([]db.ApiKey, error) {
	var out []db.ApiKey
	for _, key := range f.keys {
		out = append(out, key)
	}
	return out, nil
}

func (f *fakeRepo) RevokeAPIKey(_ context.Context, id pgtype.UUID) (db.ApiKey, error) {
	key, ok := f.keys[id]
	if !ok || key.RevokedAt.Valid {
		return db.ApiKey{}, pgx.ErrNoRows
	}
	key.RevokedAt = pgtype.Timestamptz{Time: time.Now(), Valid: true}
	f.keys[id] = key
	return key, nil
}

func (f *fakeRepo) RevokeAllUserAPIKeys(_ context.Context, userID pgtype.UUID) error {
	for id, key := range f.keys {
		if key.UserID == userID {
			key.RevokedAt = pgtype.Timestamptz{Time: time.Now(), Valid: true}
			f.keys[id] = key
		}
	}
	return nil
}

func (f *fakeRepo) TouchAPIKey(_ context.Context, id pgtype.UUID) error {
	f.lastKeyUsed = id
	return nil
}

func (f *fakeRepo) CreateAuditEntry(_ context.Context, arg db.CreateAuditEntryParams) (db.AuditLog, error) {
	f.auditEvents = append(f.auditEvents, arg)
	return db.AuditLog{ID: uuid(99), Metadata: arg.Metadata}, nil
}

func (f *fakeRepo) ListAuditEntries(context.Context, db.ListAuditEntriesParams) ([]db.AuditLog, error) {
	return nil, nil
}

func testAuthService(repo *fakeRepo) *Service {
	return NewService(repo, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func addUser(repo *fakeRepo, id pgtype.UUID, email string, active bool) db.User {
	user := db.User{
		ID:        id,
		Email:     email,
		Name:      "User",
		Active:    active,
		CreatedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true},
	}
	repo.users[id] = user
	repo.usersByEmail[email] = id
	return user
}

func TestGenerateAPIKey(t *testing.T) {
	raw, hash, prefix, err := GenerateAPIKey()
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) < 20 || raw[:3] != "bm_" {
		t.Fatalf("unexpected key format: %q", raw)
	}
	if len(hash) != 32 {
		t.Fatalf("hash length = %d", len(hash))
	}
	if prefix != raw[:12] {
		t.Fatalf("prefix = %q, want %q", prefix, raw[:12])
	}
}

func TestBootstrapCreatesAdmin(t *testing.T) {
	repo := newFakeRepo()
	repo.bootstrapOK = true
	service := testAuthService(repo)

	if err := service.Bootstrap(context.Background(), "bm_bootstrap_secret", false); err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}
	if repo.bootstrapCalls != 1 {
		t.Fatalf("bootstrap calls = %d", repo.bootstrapCalls)
	}
	if string(repo.bootstrapHash) != string(HashKey("bm_bootstrap_secret")) {
		t.Fatal("bootstrap hash mismatch")
	}
	if repo.bootstrapPref != KeyPrefix("bm_bootstrap_secret") {
		t.Fatal("bootstrap prefix mismatch")
	}
	if service.InsecureMode() {
		t.Fatal("insecure mode must not be enabled when ADMIN_KEY is set")
	}
}

func TestBootstrapInsecureDevelopmentMode(t *testing.T) {
	repo := newFakeRepo()
	service := testAuthService(repo)

	if err := service.Bootstrap(context.Background(), "", true); err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}
	if !service.InsecureMode() {
		t.Fatal("expected insecure mode")
	}
	principal, err := service.Authenticate(context.Background(), "")
	if err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
	if !principal.Insecure() {
		t.Fatal("expected synthetic insecure principal")
	}
}

func TestBootstrapWithoutKeyFailsWhenNotAllowed(t *testing.T) {
	service := testAuthService(newFakeRepo())
	if err := service.Bootstrap(context.Background(), "", false); !errors.Is(err, ErrNoBootstrapKey) {
		t.Fatalf("expected ErrNoBootstrapKey, got %v", err)
	}
}

func TestAuthenticateValidKey(t *testing.T) {
	repo := newFakeRepo()
	userID := uuid(1)
	addUser(repo, userID, "admin@example.com", true)
	raw, hash, prefix, _ := GenerateAPIKey()
	key := db.ApiKey{ID: uuid(2), UserID: userID, KeyPrefix: prefix, KeyHash: hash}
	repo.keys[key.ID] = key
	repo.keysByHash[string(hash)] = key.ID

	service := testAuthService(repo)
	principal, err := service.Authenticate(context.Background(), raw)
	if err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
	if principal.UserID != userID || principal.Email != "admin@example.com" || principal.KeyID != key.ID {
		t.Fatalf("unexpected principal: %+v", principal)
	}
	if repo.lastKeyUsed != key.ID {
		t.Fatal("last_used_at was not touched")
	}
}

func TestAuthenticateRejectsRevokedExpiredAndInactive(t *testing.T) {
	userID := uuid(1)
	raw, hash, prefix, _ := GenerateAPIKey()

	tests := []struct {
		name   string
		key    db.ApiKey
		active bool
	}{
		{name: "revoked", key: db.ApiKey{ID: uuid(2), UserID: userID, KeyHash: hash, KeyPrefix: prefix, RevokedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true}}, active: true},
		{name: "expired", key: db.ApiKey{ID: uuid(3), UserID: userID, KeyHash: hash, KeyPrefix: prefix, ExpiresAt: pgtype.Timestamptz{Time: time.Now().Add(-time.Hour), Valid: true}}, active: true},
		{name: "inactive user", key: db.ApiKey{ID: uuid(4), UserID: userID, KeyHash: hash, KeyPrefix: prefix}, active: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newFakeRepo()
			addUser(repo, userID, "user@example.com", tt.active)
			repo.keys[tt.key.ID] = tt.key
			repo.keysByHash[string(hash)] = tt.key.ID
			service := testAuthService(repo)

			if _, err := service.Authenticate(context.Background(), raw); !errors.Is(err, ErrUnauthorized) {
				t.Fatalf("expected ErrUnauthorized, got %v", err)
			}
		})
	}
}

func TestCreateUserDuplicate(t *testing.T) {
	repo := newFakeRepo()
	service := testAuthService(repo)
	if _, err := service.CreateUser(context.Background(), "a@example.com", "A"); err != nil {
		t.Fatal(err)
	}
	if _, err := service.CreateUser(context.Background(), "a@example.com", "A"); !errors.Is(err, ErrUserExists) {
		t.Fatalf("expected ErrUserExists, got %v", err)
	}
}

func TestDeactivateLastActiveUser(t *testing.T) {
	repo := newFakeRepo()
	addUser(repo, uuid(1), "a@example.com", true)
	addUser(repo, uuid(2), "b@example.com", false)
	service := testAuthService(repo)

	if _, err := service.DeactivateUser(context.Background(), uuid(1)); !errors.Is(err, ErrLastActiveUser) {
		t.Fatalf("expected ErrLastActiveUser, got %v", err)
	}
}

func TestCreateAPIKeyRejectsInactiveUser(t *testing.T) {
	repo := newFakeRepo()
	addUser(repo, uuid(1), "a@example.com", false)
	service := testAuthService(repo)

	if _, _, err := service.CreateAPIKey(context.Background(), uuid(1), "key", pgtype.Timestamptz{}); !errors.Is(err, ErrInactiveUser) {
		t.Fatalf("expected ErrInactiveUser, got %v", err)
	}
}

func TestRevokeAPIKeyForWrongUser(t *testing.T) {
	repo := newFakeRepo()
	repo.keys[uuid(2)] = db.ApiKey{ID: uuid(2), UserID: uuid(1)}
	service := testAuthService(repo)

	if _, err := service.RevokeAPIKeyForUser(context.Background(), uuid(2), uuid(9)); !errors.Is(err, ErrKeyForbidden) {
		t.Fatalf("expected ErrKeyForbidden, got %v", err)
	}
}

func TestRecordAudit(t *testing.T) {
	repo := newFakeRepo()
	service := testAuthService(repo)

	err := service.RecordAudit(context.Background(), AuditEvent{
		Principal:    Principal{UserID: uuid(1), KeyID: uuid(2)},
		Action:       "POST /api/v1/users",
		ResourceType: "users",
		Metadata:     map[string]any{"status": 201},
	})
	if err != nil {
		t.Fatalf("RecordAudit: %v", err)
	}
	if len(repo.auditEvents) != 1 {
		t.Fatalf("audit events = %d", len(repo.auditEvents))
	}
	var metadata map[string]any
	if err := json.Unmarshal(repo.auditEvents[0].Metadata, &metadata); err != nil {
		t.Fatal(err)
	}
	if metadata["status"] != float64(201) {
		t.Fatalf("unexpected metadata: %v", metadata)
	}
}
