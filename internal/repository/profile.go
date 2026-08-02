package repository

import (
	"context"

	"github.com/MD2SA/backup-manager/internal/repository/db"
	"github.com/jackc/pgx/v5/pgtype"
)

type ProfileFull struct {
	db.Profile
	StorageProviderIDs      []pgtype.UUID
	NotificationProviderIDs []pgtype.UUID
}

type ProfileRepository interface {
	GetProfile(ctx context.Context, id pgtype.UUID) (ProfileFull, error)
	ListProfiles(ctx context.Context) ([]ProfileFull, error)
	CreateProfile(ctx context.Context, params db.CreateProfileParams, storageIDs []pgtype.UUID, notificationIDs []pgtype.UUID) (ProfileFull, error)
	UpdateProfile(ctx context.Context, params db.UpdateProfileParams, storageIDs []pgtype.UUID, notificationIDs []pgtype.UUID) (ProfileFull, error)
	DeleteProfile(ctx context.Context, id pgtype.UUID) error
	ActivateProfile(ctx context.Context, id pgtype.UUID) error
}

func (r *Postgres) GetProfile(ctx context.Context, id pgtype.UUID) (ProfileFull, error) {
	p, err := r.queries.GetProfile(ctx, id)
	if err != nil {
		return ProfileFull{}, err
	}

	storageIDs, err := r.queries.GetProfileStorageProviderIDs(ctx, id)
	if err != nil {
		return ProfileFull{}, err
	}

	notificationIDs, err := r.queries.GetProfileNotificationProviderIDs(ctx, id)
	if err != nil {
		return ProfileFull{}, err
	}

	return ProfileFull{
		Profile:                 p,
		StorageProviderIDs:      storageIDs,
		NotificationProviderIDs: notificationIDs,
	}, nil
}

func (r *Postgres) ListProfiles(ctx context.Context) ([]ProfileFull, error) {
	profiles, err := r.queries.ListProfiles(ctx)
	if err != nil {
		return nil, err
	}

	res := make([]ProfileFull, len(profiles))
	for i, p := range profiles {
		storageIDs, err := r.queries.GetProfileStorageProviderIDs(ctx, p.ID)
		if err != nil {
			return nil, err
		}

		notificationIDs, err := r.queries.GetProfileNotificationProviderIDs(ctx, p.ID)
		if err != nil {
			return nil, err
		}

		res[i] = ProfileFull{
			Profile:                 p,
			StorageProviderIDs:      storageIDs,
			NotificationProviderIDs: notificationIDs,
		}
	}

	return res, nil
}

func (r *Postgres) CreateProfile(ctx context.Context, params db.CreateProfileParams, storageIDs []pgtype.UUID, notificationIDs []pgtype.UUID) (ProfileFull, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return ProfileFull{}, err
	}
	defer tx.Rollback(ctx)

	q := r.queries.WithTx(tx)

	p, err := q.CreateProfile(ctx, params)
	if err != nil {
		return ProfileFull{}, err
	}

	for _, sID := range storageIDs {
		err = q.AddProfileStorageProvider(ctx, db.AddProfileStorageProviderParams{
			ProfileID:         p.ID,
			StorageProviderID: sID,
		})
		if err != nil {
			return ProfileFull{}, err
		}
	}

	for _, nID := range notificationIDs {
		err = q.AddProfileNotificationProvider(ctx, db.AddProfileNotificationProviderParams{
			ProfileID:              p.ID,
			NotificationProviderID: nID,
		})
		if err != nil {
			return ProfileFull{}, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return ProfileFull{}, err
	}

	return ProfileFull{
		Profile:                 p,
		StorageProviderIDs:      storageIDs,
		NotificationProviderIDs: notificationIDs,
	}, nil
}

func (r *Postgres) UpdateProfile(ctx context.Context, params db.UpdateProfileParams, storageIDs []pgtype.UUID, notificationIDs []pgtype.UUID) (ProfileFull, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return ProfileFull{}, err
	}
	defer tx.Rollback(ctx)

	q := r.queries.WithTx(tx)

	p, err := q.UpdateProfile(ctx, params)
	if err != nil {
		return ProfileFull{}, err
	}

	if err := q.ClearProfileStorageProviders(ctx, p.ID); err != nil {
		return ProfileFull{}, err
	}
	for _, sID := range storageIDs {
		err = q.AddProfileStorageProvider(ctx, db.AddProfileStorageProviderParams{
			ProfileID:         p.ID,
			StorageProviderID: sID,
		})
		if err != nil {
			return ProfileFull{}, err
		}
	}

	if err := q.ClearProfileNotificationProviders(ctx, p.ID); err != nil {
		return ProfileFull{}, err
	}
	for _, nID := range notificationIDs {
		err = q.AddProfileNotificationProvider(ctx, db.AddProfileNotificationProviderParams{
			ProfileID:              p.ID,
			NotificationProviderID: nID,
		})
		if err != nil {
			return ProfileFull{}, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return ProfileFull{}, err
	}

	return ProfileFull{
		Profile:                 p,
		StorageProviderIDs:      storageIDs,
		NotificationProviderIDs: notificationIDs,
	}, nil
}

func (r *Postgres) DeleteProfile(ctx context.Context, id pgtype.UUID) error {
	return r.queries.DeleteProfile(ctx, id)
}

func (r *Postgres) ActivateProfile(ctx context.Context, id pgtype.UUID) error {
	return r.queries.ActivateProfile(ctx, id)
}
