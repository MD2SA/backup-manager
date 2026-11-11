package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/MD2SA/backup-manager/internal/backup"
	"github.com/MD2SA/backup-manager/internal/config"
	"github.com/MD2SA/backup-manager/internal/notification"
	"github.com/MD2SA/backup-manager/internal/pkg/pgutil"
	"github.com/MD2SA/backup-manager/internal/repository"
	"github.com/MD2SA/backup-manager/internal/repository/db"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

var (
	ErrRestoreConflict     = errors.New("a restore is already active for this execution")
	ErrRestoreQueueFull    = backup.ErrRestoreQueueFull
	ErrTargetNotConfigured = errors.New("target database is not configured")
	ErrTargetNotReady      = errors.New("target database is not ready for restore")
	ErrRestoreNoArtifact   = errors.New("execution has no stored backup artifact")
	ErrRestoreProvider     = errors.New("the requested storage provider is not linked to this execution")
)

// RestoreOptions controls a restore job.
type RestoreOptions struct {
	ProviderID           pgtype.UUID
	TerminateConnections bool
	VerifyOnly           bool
	SingleTransaction    bool
	RequestID            string
	UserID               pgtype.UUID
	KeyID                pgtype.UUID
}

type RestoreService struct {
	repo            repository.Repository
	providerService *ProviderService
	config          config.Config
	logger          *slog.Logger
	gate            *backup.Gate
	notifier        *Notifier
	queue           func(pgtype.UUID) error
	checkTarget     func(context.Context, backup.RestoreTarget) error
}

func NewRestoreService(
	repo repository.Repository,
	providerService *ProviderService,
	cfg config.Config,
	logger *slog.Logger,
	gate *backup.Gate,
	notifier *Notifier,
) *RestoreService {
	return &RestoreService{
		repo:            repo,
		providerService: providerService,
		config:          cfg,
		logger:          logger,
		gate:            gate,
		notifier:        notifier,
		checkTarget: func(ctx context.Context, target backup.RestoreTarget) error {
			return target.CheckEmpty(ctx)
		},
	}
}

// SetQueue wires the asynchronous runner after construction.
func (s *RestoreService) SetQueue(queue func(pgtype.UUID) error) {
	s.queue = queue
}

// Enqueue validates the request, creates a pending restore row and queues it.
func (s *RestoreService) Enqueue(ctx context.Context, executionID pgtype.UUID, opts RestoreOptions) (db.Restore, error) {
	execution, err := s.repo.GetExecution(ctx, executionID)
	if err != nil {
		return db.Restore{}, err
	}

	if !opts.VerifyOnly && !s.config.IsTargetDBConfigured() {
		return db.Restore{}, ErrTargetNotConfigured
	}

	profile, err := s.repo.GetProfile(ctx, execution.ProfileID)
	if err != nil {
		return db.Restore{}, err
	}

	records, err := s.repo.ListExecutionStorage(ctx, executionID)
	if err != nil {
		return db.Restore{}, err
	}
	if len(records) == 0 && !execution.StoragePath.Valid {
		return db.Restore{}, ErrRestoreNoArtifact
	}
	if opts.ProviderID.Valid {
		found := false
		for _, record := range records {
			if record.ProviderID == opts.ProviderID {
				found = true
				break
			}
		}
		if len(records) > 0 && !found {
			return db.Restore{}, ErrRestoreProvider
		}
		if len(records) == 0 && !providerLinked(profile.StorageProviderIDs, opts.ProviderID) {
			return db.Restore{}, ErrRestoreProvider
		}
	}

	if _, err := s.repo.GetActiveRestoreByExecution(ctx, executionID); err == nil {
		return db.Restore{}, ErrRestoreConflict
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return db.Restore{}, err
	}

	if !opts.VerifyOnly {
		target := backup.RestoreTarget{
			Host:     s.config.TargetDB.Host,
			Port:     s.config.TargetDB.Port,
			User:     s.config.TargetDB.User,
			Password: s.config.TargetDB.Password,
			DBName:   s.config.TargetDB.DBName,
			SSLMode:  s.config.TargetDB.SSLMode,
		}
		if s.checkTarget != nil {
			if err := s.checkTarget(ctx, target); err != nil {
				return db.Restore{}, fmt.Errorf("%w: %v", ErrTargetNotReady, err)
			}
		}
	}

	payload, err := json.Marshal(restoreOptionsPayload{
		ProviderID:           pgutil.UUIDToString(opts.ProviderID),
		TerminateConnections: opts.TerminateConnections,
		VerifyOnly:           opts.VerifyOnly,
		SingleTransaction:    opts.SingleTransaction,
	})
	if err != nil {
		return db.Restore{}, err
	}

	restore, err := s.repo.CreateRestore(ctx, db.CreateRestoreParams{
		ExecutionID:       executionID,
		Status:            "pending",
		RequestID:         pgutil.ToText(opts.RequestID),
		Options:           payload,
		TargetDb:          pgutil.ToText(s.config.TargetDB.DBName),
		ProviderID:        opts.ProviderID,
		StoragePath:       pgtype.Text{},
		RequestedByUserID: opts.UserID,
		RequestedByKeyID:  opts.KeyID,
	})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return db.Restore{}, ErrRestoreConflict
		}
		return db.Restore{}, err
	}

	if s.queue == nil {
		_ = s.finishFailed(ctx, restore, time.Now(), nil, pgtype.UUID{}, "", errors.New("restore runner is not configured"))
		return restore, errors.New("restore runner is not configured")
	}
	if err := s.queue(restore.ID); err != nil {
		_ = s.finishFailed(ctx, restore, time.Now(), nil, pgtype.UUID{}, "", err)
		return restore, err
	}

	return restore, nil
}

// Execute runs a queued restore. It is the callback used by RestoreRunner.
func (s *RestoreService) Execute(ctx context.Context, restoreID pgtype.UUID) error {
	restore, err := s.repo.GetRestore(ctx, restoreID)
	if err != nil {
		return err
	}
	if restore.Status != "pending" && restore.Status != "running" {
		return fmt.Errorf("restore %s is not pending", restoreID)
	}

	execution, err := s.repo.GetExecution(ctx, restore.ExecutionID)
	if err != nil {
		return s.finishFailed(ctx, restore, time.Now(), nil, pgtype.UUID{}, "", err)
	}
	profile, err := s.repo.GetProfile(ctx, execution.ProfileID)
	if err != nil {
		return s.finishFailed(ctx, restore, time.Now(), nil, pgtype.UUID{}, "", err)
	}

	opts := parseRestoreOptions(restore.Options)
	started := time.Now()
	logs := []string{}
	logf := func(format string, args ...any) {
		message := fmt.Sprintf(format, args...)
		logs = append(logs, time.Now().Format(time.RFC3339)+": "+message)
		s.logger.Info("Restore", "restore_id", restore.ID, "execution_id", execution.ID, "message", message)
	}

	restore, err = s.update(ctx, db.UpdateRestoreParams{
		ID:        restore.ID,
		Status:    "running",
		StartedAt: pgutil.ToTimestamptz(started),
		Logs:      marshalLogs(logs),
	})
	if err != nil {
		return err
	}

	s.notifyAll(ctx, profile.NotificationProviderIDs, notification.EventStarted,
		fmt.Sprintf("Restore started for execution: %s", pgutil.UUIDToString(execution.ID)), nil)

	providerID, storagePath, artifact, err := s.prepareArtifact(ctx, execution, profile, opts, logf)
	if err != nil {
		return s.finishFailed(ctx, restore, started, logs, pgtype.UUID{}, "", err)
	}
	defer artifact.Cleanup()

	if opts.VerifyOnly {
		logf("verify-only request: artifact prepared and checksum checked; target database untouched")
		if err := s.finishSuccess(ctx, restore, started, logs, artifact, providerID, storagePath, profile); err != nil {
			return err
		}
		return nil
	}

	if s.gate != nil {
		logf("waiting for the target database gate")
		if err := s.gate.Acquire(ctx); err != nil {
			return s.finishFailed(ctx, restore, started, logs, providerID, storagePath, err)
		}
		defer s.gate.Release()
	}

	target := backup.RestoreTarget{
		Host:     s.config.TargetDB.Host,
		Port:     s.config.TargetDB.Port,
		User:     s.config.TargetDB.User,
		Password: s.config.TargetDB.Password,
		DBName:   s.config.TargetDB.DBName,
		SSLMode:  s.config.TargetDB.SSLMode,
	}

	logf("preflight target database %q", target.DBName)
	if err := target.Preflight(ctx, opts.TerminateConnections); err != nil {
		return s.finishFailed(ctx, restore, started, logs, providerID, storagePath, err)
	}

	logf("applying artifact to target database")
	output, applyErr := backup.ApplyArtifact(ctx, artifact, target, opts.SingleTransaction)
	appendOutput(&logs, output)
	if applyErr != nil {
		return s.finishFailed(ctx, restore, started, logs, providerID, storagePath, applyErr)
	}
	logf("restore applied successfully")

	if err := s.finishSuccess(ctx, restore, started, logs, artifact, providerID, storagePath, profile); err != nil {
		return err
	}
	return nil
}

func (s *RestoreService) prepareArtifact(
	ctx context.Context,
	execution db.Execution,
	profile repository.ProfileFull,
	opts restoreOptionsPayload,
	logf func(string, ...any),
) (pgtype.UUID, string, *backup.Artifact, error) {
	requestedProvider, err := pgutil.ParseUUID(opts.ProviderID)
	if err != nil {
		return pgtype.UUID{}, "", nil, fmt.Errorf("invalid provider id: %w", err)
	}

	type candidate struct {
		providerID  pgtype.UUID
		storagePath string
	}

	var candidates []candidate
	if requestedProvider.Valid {
		path := ""
		records, err := s.repo.ListExecutionStorage(ctx, execution.ID)
		if err != nil {
			return pgtype.UUID{}, "", nil, err
		}
		for _, record := range records {
			if record.ProviderID == requestedProvider {
				path = record.StoragePath
				break
			}
		}
		if path == "" && execution.StoragePath.Valid {
			path = execution.StoragePath.String
		}
		if path == "" {
			return pgtype.UUID{}, "", nil, ErrRestoreProvider
		}
		candidates = append(candidates, candidate{providerID: requestedProvider, storagePath: path})
	} else {
		records, err := s.repo.ListExecutionStorage(ctx, execution.ID)
		if err != nil {
			return pgtype.UUID{}, "", nil, err
		}
		for _, record := range records {
			candidates = append(candidates, candidate{providerID: record.ProviderID, storagePath: record.StoragePath})
		}
		if len(candidates) == 0 && execution.StoragePath.Valid {
			for _, providerID := range profile.StorageProviderIDs {
				candidates = append(candidates, candidate{providerID: providerID, storagePath: execution.StoragePath.String})
			}
		}
	}

	if len(candidates) == 0 {
		return pgtype.UUID{}, "", nil, ErrRestoreNoArtifact
	}

	var errs []error
	for _, candidate := range candidates {
		provider, resolveErr := s.providerService.ResolveStorageProvider(ctx, candidate.providerID)
		if resolveErr != nil {
			errs = append(errs, fmt.Errorf("provider %s: %w", pgutil.UUIDToString(candidate.providerID), resolveErr))
			logf("provider %s could not be resolved: %v", pgutil.UUIDToString(candidate.providerID), resolveErr)
			continue
		}

		logf("trying artifact on provider %s at %s", pgutil.UUIDToString(candidate.providerID), candidate.storagePath)
		artifact, prepErr := backup.PrepareArtifact(
			ctx,
			provider,
			candidate.storagePath,
			s.config.TempDir,
			execution.Checksum.String,
			execution.IsEncrypted,
			s.config.EncryptionPassphrase,
			s.config.AgePrivateKey,
			logf,
		)
		if prepErr != nil {
			errs = append(errs, fmt.Errorf("provider %s: %w", pgutil.UUIDToString(candidate.providerID), prepErr))
			logf("artifact on provider %s is unusable: %v", pgutil.UUIDToString(candidate.providerID), prepErr)
			continue
		}
		return candidate.providerID, candidate.storagePath, artifact, nil
	}

	return pgtype.UUID{}, "", nil, fmt.Errorf("no usable backup artifact: %w", errors.Join(errs...))
}

func (s *RestoreService) finishSuccess(
	ctx context.Context,
	restore db.Restore,
	started time.Time,
	logs []string,
	artifact *backup.Artifact,
	providerID pgtype.UUID,
	storagePath string,
	profile repository.ProfileFull,
) error {
	finished := time.Now()
	if _, err := s.update(ctx, db.UpdateRestoreParams{
		ID:               restore.ID,
		Status:           "success",
		ProviderID:       providerID,
		StoragePath:      pgutil.ToText(storagePath),
		ChecksumVerified: artifact.ChecksumVerified,
		StartedAt:        pgutil.ToTimestamptz(started),
		FinishedAt:       pgutil.ToTimestamptz(finished),
		Duration:         pgutil.ToInterval(finished.Sub(started)),
		Logs:             marshalLogs(logs),
	}); err != nil {
		return err
	}

	s.notifyAll(ctx, profile.NotificationProviderIDs, notification.EventSuccess,
		fmt.Sprintf("Restore completed for execution: %s", pgutil.UUIDToString(restore.ExecutionID)),
		map[string]interface{}{
			"duration":          finished.Sub(started).String(),
			"checksum_verified": artifact.ChecksumVerified,
		})
	return nil
}

func (s *RestoreService) finishFailed(
	ctx context.Context,
	restore db.Restore,
	started time.Time,
	logs []string,
	providerID pgtype.UUID,
	storagePath string,
	cause error,
) error {
	finished := time.Now()
	logs = append(logs, time.Now().Format(time.RFC3339)+": restore failed: "+cause.Error())

	_, updateErr := s.update(ctx, db.UpdateRestoreParams{
		ID:           restore.ID,
		Status:       "failed",
		ProviderID:   providerID,
		StoragePath:  pgutil.ToText(storagePath),
		StartedAt:    pgutil.ToTimestamptz(started),
		FinishedAt:   pgutil.ToTimestamptz(finished),
		Duration:     pgutil.ToInterval(finished.Sub(started)),
		ErrorMessage: pgutil.ToText(cause.Error()),
		Logs:         marshalLogs(logs),
	})
	if updateErr != nil {
		s.logger.Error("Failed to persist restore failure", "restore_id", restore.ID, "error", updateErr)
	}

	// The notification needs the profile; tolerate a missing profile here.
	notifyCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	execution, execErr := s.repo.GetExecution(notifyCtx, restore.ExecutionID)
	if execErr == nil {
		if profile, profileErr := s.repo.GetProfile(notifyCtx, execution.ProfileID); profileErr == nil {
			s.notifyAll(notifyCtx, profile.NotificationProviderIDs, notification.EventFailed,
				fmt.Sprintf("Restore failed for execution: %s. Error: %v", pgutil.UUIDToString(restore.ExecutionID), cause), nil)
		}
	}

	return cause
}

// update uses a cancellation-independent context so the final state is persisted
// even when the HTTP client disconnects or the process receives SIGTERM.
func (s *RestoreService) update(ctx context.Context, params db.UpdateRestoreParams) (db.Restore, error) {
	dbCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	return s.repo.UpdateRestore(dbCtx, params)
}

func (s *RestoreService) notifyAll(ctx context.Context, providerIDs []pgtype.UUID, event notification.Event, message string, metadata map[string]interface{}) {
	if s.notifier != nil {
		s.notifier.All(ctx, providerIDs, event, message, metadata)
	}
}

type restoreOptionsPayload struct {
	ProviderID           string `json:"provider_id,omitempty"`
	TerminateConnections bool   `json:"terminate_connections"`
	VerifyOnly           bool   `json:"verify_only"`
	SingleTransaction    bool   `json:"single_transaction"`
}

func parseRestoreOptions(raw json.RawMessage) restoreOptionsPayload {
	opts := restoreOptionsPayload{SingleTransaction: true}
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &opts)
	}
	return opts
}

func providerLinked(ids []pgtype.UUID, target pgtype.UUID) bool {
	for _, id := range ids {
		if id == target {
			return true
		}
	}
	return false
}

func marshalLogs(logs []string) json.RawMessage {
	if logs == nil {
		logs = []string{}
	}
	encoded, _ := json.Marshal(logs)
	return encoded
}

func appendOutput(logs *[]string, output string) {
	output = strings.TrimSpace(output)
	if output == "" {
		return
	}
	lines := strings.Split(output, "\n")
	const maxLines = 200
	if len(lines) > maxLines {
		lines = append(lines[:maxLines], "[output truncated]")
	}
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		*logs = append(*logs, time.Now().Format(time.RFC3339)+": [database] "+line)
	}
}
