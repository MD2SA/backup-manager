package retention

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/MD2SA/backup-manager/internal/repository/db"
	"github.com/MD2SA/backup-manager/internal/storage"
	"github.com/jackc/pgx/v5/pgtype"
)

type Policy struct {
	KeepHourly  int
	KeepDaily   int
	KeepWeekly  int
	KeepMonthly int
	KeepYearly  int
	YearlyMonth int
}

// Repository is the subset of the repository used by the retention engine.
// Keeping it local makes the GFS logic easy to test without a database.
type Repository interface {
	ListExecutionsByProfileForRetention(ctx context.Context, profileID pgtype.UUID) ([]db.Execution, error)
	ListExecutionStorage(ctx context.Context, executionID pgtype.UUID) ([]db.ExecutionStorage, error)
	DeleteExecutionStorage(ctx context.Context, arg db.DeleteExecutionStorageParams) error
	DeleteExecutionIfNoStorage(ctx context.Context, id pgtype.UUID) (int64, error)
}

type Engine struct {
	repo   Repository
	logger *slog.Logger
}

func New(repo Repository, logger *slog.Logger) *Engine {
	if logger == nil {
		logger = slog.Default()
	}
	return &Engine{repo: repo, logger: logger}
}

// Apply removes executions that fall outside the policy. It is called once per
// backup after all uploads have completed, so it can delete every stored copy
// of an execution and only then remove the database record.
func (e *Engine) Apply(
	ctx context.Context,
	profileID pgtype.UUID,
	policy Policy,
	providers map[pgtype.UUID]storage.StorageProvider,
) error {
	executions, err := e.repo.ListExecutionsByProfileForRetention(ctx, profileID)
	if err != nil {
		return err
	}
	if len(executions) == 0 {
		return nil
	}

	// GFS only considers successful backups. A failed execution must not occupy
	// the "latest" slot and cause older, usable backups to be deleted.
	successful := make([]db.Execution, 0, len(executions))
	for _, execution := range executions {
		if execution.Status == "success" {
			successful = append(successful, execution)
		}
	}
	if len(successful) == 0 {
		return nil
	}
	executions = successful

	toKeep := make(map[pgtype.UUID]bool)

	// Always keep the latest one.
	toKeep[executions[0].ID] = true

	hourly := make(map[string]pgtype.UUID)
	daily := make(map[string]pgtype.UUID)
	weekly := make(map[string]pgtype.UUID)
	monthly := make(map[string]pgtype.UUID)
	yearly := make(map[string]pgtype.UUID)

	for _, exec := range executions {
		if exec.IsPinned {
			toKeep[exec.ID] = true
			continue
		}
		if !exec.StartTime.Valid {
			continue
		}
		t := exec.StartTime.Time

		hKey := t.Format("2006-01-02-15")
		dKey := t.Format("2006-01-02")
		_, wWeek := t.ISOWeek()
		wKey := fmt.Sprintf("%s-W%d", t.Format("2006"), wWeek)
		mKey := t.Format("2006-01")
		yKey := t.Format("2006")

		if _, ok := hourly[hKey]; !ok && len(hourly) < policy.KeepHourly {
			hourly[hKey] = exec.ID
			toKeep[exec.ID] = true
		}
		if _, ok := daily[dKey]; !ok && len(daily) < policy.KeepDaily {
			daily[dKey] = exec.ID
			toKeep[exec.ID] = true
		}
		if _, ok := weekly[wKey]; !ok && len(weekly) < policy.KeepWeekly {
			weekly[wKey] = exec.ID
			toKeep[exec.ID] = true
		}
		if _, ok := monthly[mKey]; !ok && len(monthly) < policy.KeepMonthly {
			monthly[mKey] = exec.ID
			toKeep[exec.ID] = true
		}
		if int(t.Month()) == policy.YearlyMonth {
			if _, ok := yearly[yKey]; !ok && len(yearly) < policy.KeepYearly {
				yearly[yKey] = exec.ID
				toKeep[exec.ID] = true
			}
		}
	}

	for _, exec := range executions {
		if toKeep[exec.ID] {
			continue
		}
		if err := e.deleteExecution(ctx, exec, providers); err != nil {
			e.logger.Warn("Retention could not delete execution", "execution_id", exec.ID, "error", err)
		}
	}
	return nil
}

func (e *Engine) deleteExecution(
	ctx context.Context,
	exec db.Execution,
	providers map[pgtype.UUID]storage.StorageProvider,
) error {
	records, err := e.repo.ListExecutionStorage(ctx, exec.ID)
	if err != nil {
		return fmt.Errorf("list execution storage: %w", err)
	}

	if len(records) == 0 {
		// Legacy execution: only executions.storage_path is known. Delete it
		// from every configured provider; this is safe because the key is
		// namespaced by profile and execution. If no path is recorded, the
		// execution has no artifact and can be removed directly.
		if !exec.StoragePath.Valid {
			_, err := e.repo.DeleteExecutionIfNoStorage(ctx, exec.ID)
			return err
		}
		if len(providers) == 0 {
			return fmt.Errorf("no storage providers available to delete legacy artifact")
		}
		var deleteErrs []error
		for providerID, provider := range providers {
			if err := provider.Delete(ctx, exec.StoragePath.String); err != nil && !os.IsNotExist(err) {
				deleteErrs = append(deleteErrs, fmt.Errorf("provider %s: %w", providerID, err))
			}
		}
		if len(deleteErrs) > 0 {
			return fmt.Errorf("delete legacy artifact: %v", deleteErrs)
		}
		_, err := e.repo.DeleteExecutionIfNoStorage(ctx, exec.ID)
		return err
	}

	for _, record := range records {
		provider, ok := providers[record.ProviderID]
		if !ok {
			// Provider was deleted or cannot be resolved; keep the record so a
			// later run can retry.
			e.logger.Warn("Retention skipped storage copy: provider unavailable",
				"execution_id", exec.ID, "provider_id", record.ProviderID)
			continue
		}
		if err := provider.Delete(ctx, record.StoragePath); err != nil && !os.IsNotExist(err) {
			e.logger.Warn("Retention failed to delete storage object",
				"execution_id", exec.ID, "provider_id", record.ProviderID, "path", record.StoragePath, "error", err)
			continue
		}
		if err := e.repo.DeleteExecutionStorage(ctx, db.DeleteExecutionStorageParams{
			ExecutionID: exec.ID,
			ProviderID:  record.ProviderID,
		}); err != nil {
			return fmt.Errorf("delete execution storage record: %w", err)
		}
	}

	// The execution row is only removed when no copies remain and no restore
	// is active. If a provider was unavailable, this returns 0 and we retry
	// on the next retention run.
	_, err = e.repo.DeleteExecutionIfNoStorage(ctx, exec.ID)
	return err
}
