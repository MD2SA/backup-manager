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

# Explicit command (e.g. metadata-restore, keygen, config check) wins.
if [ "$#" -gt 0 ]; then
    exec /usr/local/bin/backup-manager "$@"
fi

CONFIG_PATH=${APP_CONFIG_PATH:-/etc/backup-manager/config.yaml}
if [ ! -f "$CONFIG_PATH" ]; then
    echo "Error: configuration file not found at $CONFIG_PATH"
    echo
    echo "Create one with:"
    echo "  docker run --rm -it -u \"\$(id -u):\$(id -g)\" -v \"\$PWD:/work\" -w /work <image> init --docker"
    echo
    echo "Then mount config.yaml at $CONFIG_PATH and secrets.env next to it."
    exit 1
fi

echo "Starting Backup Manager..."
exec /usr/local/bin/backup-manager serve
