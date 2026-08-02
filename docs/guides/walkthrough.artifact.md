# Configurable and Persistent Local Storage Walkthrough

I have updated the project to make the local storage path configurable and ensure that backups are correctly persisted when running inside Docker.

## Changes Made

### Configuration
- Added `StoragePath` to the global `Config` struct in `internal/config/config.go`.
- Bound the `APP_STORAGE_PATH` environment variable with a professional default: `/var/lib/backup-manager/storage`.
- Added validation to ensure the storage path is always provided.

### Service Layer
- Updated `ProviderService` to accept the global configuration.
- In `ResolveStorageProvider`, the system now uses `APP_STORAGE_PATH` as the fallback for local storage when no specific provider is configured, ensuring consistency with the infrastructure layer.
- Updated `App` initialization in `internal/app/app.go` to inject the configuration into the `ProviderService`.

### Infrastructure (Docker)
- Updated `docker-compose.yml` to include `APP_STORAGE_PATH`.
- Changed the `backup_storage` volume mapping to `/var/lib/backup-manager/storage`. This ensures that backups saved to the default path are automatically persisted on the host machine via the named Docker volume.

### Documentation
- Updated [CONFIGURATION.md](file:///home/manas/Documents/projects/backup-manager/backend/docs/guides/CONFIGURATION.md) to document the new `APP_STORAGE_PATH` variable.
- Added a critical note for Docker users explaining the relationship between the `local` storage provider's `path` and Docker volume mappings.

## Verification Results
- The project builds successfully with `go build ./...`.
- Verified that the `docker-compose.yml` and `config.go` are synchronized to use the same default persistent path.
