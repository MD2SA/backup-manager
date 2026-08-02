# API Reference

Backup Manager provides a RESTful API for management and monitoring. By default, the API is available at `http://localhost:8080/api/v1`.

## Documentation (Swagger)

The project includes built-in Swagger documentation. To view it:
1. Start the application.
2. Navigate to `http://localhost:8080/swagger/index.html`.

## Key Endpoints

### Profiles
*   `GET /profiles`: List all backup profiles.
*   `POST /profiles`: Create a new profile.
*   `GET /profiles/{id}`: Get profile details.
*   `PUT /profiles/{id}`: Update a profile.
*   `DELETE /profiles/{id}`: Delete a profile.
*   `POST /profiles/{id}/run`: Trigger a backup immediately.
*   `POST /profiles/{id}/activate`: Set a profile as the active scheduled job.

### Storage Providers
*   `GET /storage-providers`: List providers.
*   `POST /storage-providers`: Add a new provider (S3, Local, etc.).

### Notification Providers
*   `GET /notification-providers`: List providers.
*   `POST /notification-providers`: Add a new provider (Discord, Email, etc.).

### Executions (History)
*   `GET /executions`: List recent backup executions.
*   `GET /executions/{id}`: Get detailed logs and status of a specific execution.
*   `POST /executions/{id}/restore`: Trigger a database restore from this backup.

### Retention Policies
*   `GET /retention-policies`: List policies.
*   `POST /retention-policies`: Define GFS retention rules.

## Authentication
*(Currently not implemented - planned for future release)*
