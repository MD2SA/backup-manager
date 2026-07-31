package retention

import (
	"context"
	"fmt"

	"github.com/MD2SA/backup-manager/internal/repository"
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

type Engine struct {
	repo repository.Repository
}

func New(repo repository.Repository) *Engine {
	return &Engine{repo: repo}
}

func (e *Engine) Apply(ctx context.Context, profileID pgtype.UUID, policy Policy, provider storage.StorageProvider) error {
	executions, err := e.repo.ListExecutionsByProfile(ctx, profileID)
	if err != nil {
		return err
	}

	if len(executions) == 0 {
		return nil
	}

	toKeep := make(map[pgtype.UUID]bool)

	// Always keep the latest one
	toKeep[executions[0].ID] = true

	hourly := make(map[string]pgtype.UUID)
	daily := make(map[string]pgtype.UUID)
	weekly := make(map[string]pgtype.UUID)
	monthly := make(map[string]pgtype.UUID)
	yearly := make(map[string]pgtype.UUID)

	for _, exec := range executions {
		// Never purge pinned backups
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
		if !toKeep[exec.ID] {
			if exec.StoragePath.Valid && provider != nil {
				_ = provider.Delete(ctx, exec.StoragePath.String)
			}

			if err := e.repo.DeleteExecution(ctx, exec.ID); err != nil {
				return err
			}
		}
	}

	return nil
}
