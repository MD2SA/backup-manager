# API Reference

Backup Manager provides a RESTful API for management and monitoring. By default, the API is available at `http://localhost:8080/api/v1`.

## Documentation (Swagger)

The project includes built-in Swagger documentation. To view it:
1. Start the application.
2. Navigate to `http://localhost:8080/swagger/index.html`.

## Key Endpoints

### Health & Monitoring
*   `GET /health`: Basic health check (returns "OK").
*   `GET /health/summary`: Summary of system health and backup status.

### Profiles
*   `GET /profiles`: List all backup profiles.
*   `POST /profiles`: Create a new profile.
*   `GET /profiles/{id}`: Get profile details (includes linked providers).
*   `PUT /profiles/{id}`: Update an existing profile.
*   `DELETE /profiles/{id}`: Delete a profile.
*   `POST /profiles/{id}/run`: Trigger a backup execution immediately (asynchronous).
*   `POST /profiles/{id}/activate`: Set a profile as the active scheduled job.
*   `GET /profiles/{id}/executions`: List backup history for a specific profile.

### Executions
*   `GET /executions`: List all backup executions in the system.
*   `GET /executions/{id}`: Get detailed logs and status of a specific execution.
*   `POST /executions/{id}/restore`: Trigger a database restore from this specific backup.
*   `POST /executions/{id}/pin`: Pin an execution to prevent automatic cleanup (if retention supports it).

### Storage Providers
*   `GET /storage-providers`: List all configured storage providers.
*   `POST /storage-providers`: Add a new storage provider (e.g., S3, Local).
*   `PUT /storage-providers/{id}`: Update storage provider configuration.
*   `DELETE /storage-providers/{id}`: Remove a storage provider.

### Notification Providers
*   `GET /notification-providers`: List all configured notification providers.
*   `POST /notification-providers`: Add a new notification provider (e.g., Discord).
*   `PUT /notification-providers/{id}`: Update notification provider configuration.
*   `DELETE /notification-providers/{id}`: Remove a notification provider.

### Retention Policies
*   `GET /retention-policies`: List all GFS retention policies.
*   `POST /retention-policies`: Define a new retention policy.
*   `PUT /retention-policies/{id}`: Update an existing policy.
*   `DELETE /retention-policies/{id}`: Remove a retention policy.

## Authentication
*(Currently not implemented - planned for future release)*
