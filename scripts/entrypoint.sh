#!/bin/sh
set -e

# Construct Metadata Database URL for migrations
DB_USER=$APP_METADATA_DB_USER
DB_PASSWORD=$APP_METADATA_DB_PASSWORD
DB_HOST=$APP_METADATA_DB_HOST
DB_PORT=$APP_METADATA_DB_PORT
DB_NAME=$APP_METADATA_DB_DBNAME
DB_SSLMODE=$APP_METADATA_DB_SSLMODE

# Validate that all mandatory metadata DB variables are provided
if [ -z "$DB_USER" ] || [ -z "$DB_PASSWORD" ] || [ -z "$DB_HOST" ] || [ -z "$DB_PORT" ] || [ -z "$DB_NAME" ] || [ -z "$DB_SSLMODE" ]; then
  echo "Error: Mandatory metadata database configuration is missing."
  echo "Please ensure all APP_METADATA_DB_* variables are set."
  exit 1
fi

DATABASE_URL="postgres://$DB_USER:$DB_PASSWORD@$DB_HOST:$DB_PORT/$DB_NAME?sslmode=$DB_SSLMODE"

echo "Waiting for metadata database at $DB_HOST:$DB_PORT..."
until pg_isready -h "$DB_HOST" -p "$DB_PORT" -U "$DB_USER"; do
  echo "Database is unavailable - sleeping"
  sleep 2
done

echo "Metadata database is up. Running migrations..."
/usr/local/bin/goose -dir /app/sql/migrations postgres "$DATABASE_URL" up

echo "Migrations completed successfully. Starting Backup Manager..."
exec /usr/local/bin/backup-manager
