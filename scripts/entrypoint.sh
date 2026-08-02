#!/bin/sh
set -e

mkdir -p /backups

DETECTED_UID=$(stat -c '%u' /backups)
DETECTED_GID=$(stat -c '%g' /backups)

USER_ID=${PUID:-$DETECTED_UID}
GROUP_ID=${PGID:-$DETECTED_GID}

# Safety check: if for some reason we still have 0, force safe appuser defaults
if [ "$USER_ID" = "0" ]; then USER_ID=100; fi
if [ "$GROUP_ID" = "0" ]; then GROUP_ID=101; fi

if [ "$(id -u)" = "0" ]; then
    echo "Identity: Auto-adapting to match host user (UID: $USER_ID, GID: $GROUP_ID)"

    # Update appuser and appgroup to match the desired identity
    groupmod -o -g "$GROUP_ID" appgroup
    usermod -o -u "$USER_ID" appuser

    # Ensure /backups is owned by our target identity
    chown "$USER_ID:$GROUP_ID" /backups

    # Drop privileges and re-run this script as appuser
    exec su-exec appuser "$0" "$@"
fi

# Construct Metadata Database URL for migrations
DB_USER=$APP_METADATA_DB_USER
DB_PASSWORD=$APP_METADATA_DB_PASSWORD
DB_HOST=$APP_METADATA_DB_HOST
DB_PORT=$APP_METADATA_DB_PORT
DB_NAME=$APP_METADATA_DB_DBNAME
DB_SSLMODE=$APP_METADATA_DB_SSLMODE

# Validate mandatory configuration
if [ -z "$DB_USER" ] || [ -z "$DB_PASSWORD" ] || [ -z "$DB_HOST" ] || [ -z "$DB_PORT" ] || [ -z "$DB_NAME" ] || [ -z "$DB_SSLMODE" ]; then
  echo "Error: Mandatory metadata database configuration is missing."
  exit 1
fi

DATABASE_URL="postgres://$DB_USER:$DB_PASSWORD@$DB_HOST:$DB_PORT/$DB_NAME?sslmode=$DB_SSLMODE"

echo "Database: Waiting for metadata database at $DB_HOST:$DB_PORT..."
while ! pg_isready -h "$DB_HOST" -p "$DB_PORT" -U "$DB_USER" > /dev/null 2>&1; do
  sleep 1
done

echo "Database: Running migrations..."
/usr/local/bin/goose -dir /app/sql/migrations postgres "$DATABASE_URL" up

echo "Starting Backup Manager..."
exec /usr/local/bin/backup-manager
