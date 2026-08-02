# API Documentation and Swagger Improvement Walkthrough

I have improved the API documentation by ensuring all routes are documented and refining the Swagger UI organization to match the logical groupings used in the project guides.

## Changes Made

### Documentation Completeness
- Updated [API.md](file:///home/manas/Documents/projects/backup-manager/backend/docs/guides/API.md) to include **all** available routes, including:
    - Health Summary and basic Health checks.
    - Full CRUD operations for Retention Policies, Storage Providers, and Notification Providers.
    - Execution pinning and profile-specific execution history.
    - Restore triggers.

### Swagger UI Organization
I have updated the Swagger tags in the Go handlers to provide a cleaner separation in the UI:
- **Separated Providers**: "Storage Providers" and "Notification Providers" now have their own distinct sections instead of being lumped into a generic "Providers" tag.
- **Consolidated Executions**: Moved the **Restore** endpoint under the `executions` tag, grouping it with other execution-related actions like fetching logs and pinning.
- **Unified Retention**: Renamed the tag for retention endpoints to `retention-policies` for consistency with the resource name.

### Technical Steps
1.  Modified `internal/api/handlers/providers.go`, `internal/api/handlers/restore.go`, and `internal/api/handlers/retention.go` to update Swagger annotations.
2.  Executed `make swagger` to regenerate `docs/swagger.json`, `docs/swagger.yaml`, and `docs/docs.go`.
3.  Verified that the project builds correctly with `go build ./...`.

## Verification Results
- All routes defined in `internal/api/router/router.go` are now reflected in [API.md](file:///home/manas/Documents/projects/backup-manager/backend/docs/guides/API.md).
- The Swagger UI (available at `/swagger/index.html` when running) now displays endpoints in their specific, logical groups.
