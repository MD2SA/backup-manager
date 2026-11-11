#!/usr/bin/env bash
#
# End-to-end disaster-recovery drill.
#
# Creates a disposable PostgreSQL database, seeds deterministic data, runs a
# real backup through the container image, destroys both the metadata database
# and the target database, then recovers everything through the CLI.
#
# Requirements: docker, curl, python3.
#
set -Eeuo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
IMAGE=${DRILL_IMAGE:-backup-manager-drill:local}
USERS=${DRILL_USERS:-5000}
KEEP=${DRILL_KEEP:-0}
BUILD=${DRILL_BUILD:-1}

usage() {
  cat <<EOF
Usage: scripts/recovery-drill.sh [options]

Options:
  --image IMAGE   Container image to test (default: $IMAGE)
  --users N       Number of synthetic users (default: $USERS)
  --no-build      Use the image as-is instead of building it
  --keep          Keep the temporary work directory and containers' data
  -h, --help      Show this help

Environment:
  DRILL_IMAGE, DRILL_USERS, DRILL_BUILD=0, DRILL_KEEP=1

The script exits non-zero on the first data or schema mismatch.
EOF
}

while [ $# -gt 0 ]; do
  case "$1" in
    --image) IMAGE=$2; shift 2 ;;
    --users) USERS=$2; shift 2 ;;
    --no-build) BUILD=0; shift ;;
    --keep) KEEP=1; shift ;;
    -h|--help) usage; exit 0 ;;
    *) echo "unknown option: $1" >&2; usage >&2; exit 2 ;;
  esac
done

case "$USERS" in
  ''|*[!0-9]*) echo "--users must be a positive integer" >&2; exit 2 ;;
esac

for tool in docker curl python3; do
  command -v "$tool" >/dev/null 2>&1 || { echo "required tool not found: $tool" >&2; exit 2; }
done
docker info >/dev/null 2>&1 || { echo "docker daemon is not available" >&2; exit 2; }

ACCOUNTS=$((USERS * 3 / 2))
ORDERS=$((USERS * 4))
ITEMS=$((USERS * 10))
EVENTS=$((USERS * 6))

WORKDIR=$(mktemp -d "${TMPDIR:-/tmp}/backup-manager-drill.XXXXXX")
NET="bm-drill-net-$$"
PG="bm-drill-pg-$$"
APP="bm-drill-app-$$"
APP_PORT=8080

now_ms() { python3 -c 'import time; print(int(time.time() * 1000))'; }
jget() { python3 -c "$1"; }

cleanup() {
  local status=$?
  trap - EXIT
  if [ "$status" -ne 0 ]; then
    echo >&2
    echo "drill failed; recent application logs:" >&2
    docker logs "$APP" 2>&1 | tail -60 >&2 || true
    echo "work directory: $WORKDIR" >&2
    for log in metadata-recover data-recover file-recover verify; do
      if [ -f "$WORKDIR/$log.log" ]; then
        echo "--- $log.log ---" >&2
        tail -20 "$WORKDIR/$log.log" >&2 || true
      fi
    done
  fi
  docker rm -f "$APP" "$PG" >/dev/null 2>&1 || true
  docker network rm "$NET" >/dev/null 2>&1 || true
  if [ "$status" -eq 0 ] && [ "$KEEP" != "1" ]; then
    rm -rf "$WORKDIR"
  fi
  exit "$status"
}
trap cleanup EXIT

echo "=== 1/11 build image ==="
if [ "$BUILD" = "1" ]; then
  docker build -t "$IMAGE" "$ROOT" >/dev/null
fi

echo "=== 2/11 disposable PostgreSQL ==="
docker network create "$NET" >/dev/null
docker run -d --rm --name "$PG" --network "$NET" \
  -e POSTGRES_PASSWORD=testpass -e POSTGRES_DB=postgres \
  postgres:17-alpine >/dev/null
for _ in $(seq 1 60); do
  docker exec "$PG" pg_isready -U postgres >/dev/null 2>&1 && break
  sleep 1
done
sleep 2
for _ in $(seq 1 60); do
  docker exec "$PG" psql -U postgres -d postgres -tAc 'select 1' >/dev/null 2>&1 && break
  sleep 1
done
for db in bm_meta appdb appdb_file bm_meta_new; do
  docker exec "$PG" psql -U postgres -d postgres -c "CREATE DATABASE $db;" >/dev/null
done
docker exec "$PG" psql -U postgres -d postgres \
  -c "CREATE ROLE restore_admin WITH LOGIN SUPERUSER PASSWORD 'restorepass';" >/dev/null

echo "=== 3/11 synthetic data ($USERS users) ==="
docker exec -i "$PG" psql -U postgres -d appdb -v ON_ERROR_STOP=1 >/dev/null <<SQL
SELECT setseed(0.42);
CREATE TABLE users (
  id bigserial PRIMARY KEY,
  email text NOT NULL UNIQUE,
  name text NOT NULL,
  metadata jsonb NOT NULL,
  created_at timestamptz NOT NULL
);
CREATE TABLE accounts (
  id bigserial PRIMARY KEY,
  user_id bigint NOT NULL REFERENCES users(id),
  balance numeric(14,2) NOT NULL,
  status text NOT NULL,
  created_at timestamptz NOT NULL
);
CREATE TABLE orders (
  id bigserial PRIMARY KEY,
  account_id bigint NOT NULL REFERENCES accounts(id),
  total numeric(14,2) NOT NULL,
  status text NOT NULL,
  created_at timestamptz NOT NULL
);
CREATE TABLE order_items (
  id bigserial PRIMARY KEY,
  order_id bigint NOT NULL REFERENCES orders(id),
  sku text NOT NULL,
  quantity int NOT NULL,
  price numeric(12,2) NOT NULL
);
CREATE TABLE events (
  id bigserial PRIMARY KEY,
  user_id bigint,
  kind text NOT NULL,
  payload jsonb NOT NULL,
  created_at timestamptz NOT NULL
);
CREATE INDEX idx_users_created ON users(created_at);
CREATE INDEX idx_accounts_user ON accounts(user_id);
CREATE INDEX idx_orders_account_created ON orders(account_id, created_at);
CREATE INDEX idx_items_order ON order_items(order_id);
CREATE INDEX idx_events_kind_created ON events(kind, created_at);

INSERT INTO users (email, name, metadata, created_at)
SELECT 'user'||g||'@example.test', 'User '||g,
       jsonb_build_object('source','campaign-'||(g%20),'flags',jsonb_build_array(g%2=0,g%3=0)),
       now() - (g || ' minutes')::interval
FROM generate_series(1, $USERS) g;

INSERT INTO accounts (user_id, balance, status, created_at)
SELECT (g % $USERS) + 1, round((random()*50000)::numeric,2),
       CASE WHEN g % 7 = 0 THEN 'blocked' ELSE 'active' END,
       now() - (g || ' minutes')::interval
FROM generate_series(1, $ACCOUNTS) g;

INSERT INTO orders (account_id, total, status, created_at)
SELECT (g % $ACCOUNTS) + 1, round((random()*10000)::numeric,2),
       CASE g % 5 WHEN 0 THEN 'paid' WHEN 1 THEN 'pending' WHEN 2 THEN 'cancelled' ELSE 'paid' END,
       now() - (g || ' minutes')::interval
FROM generate_series(1, $ORDERS) g;

INSERT INTO order_items (order_id, sku, quantity, price)
SELECT (g % $ORDERS) + 1, 'SKU-'||(g % 5000), (g % 5) + 1,
       round((random()*500)::numeric,2)
FROM generate_series(1, $ITEMS) g;

INSERT INTO events (user_id, kind, payload, created_at)
SELECT (g % $USERS) + 1,
       CASE g % 4 WHEN 0 THEN 'login' WHEN 1 THEN 'purchase' WHEN 2 THEN 'logout' ELSE 'view' END,
       jsonb_build_object('path','/page/'||(g%100),'ok',g%11<>0,'ms',g%900),
       now() - (g || ' minutes')::interval
FROM generate_series(1, $EVENTS) g;
ANALYZE;
SQL
SOURCE_SIZE=$(docker exec "$PG" psql -U postgres -d appdb -tAc "SELECT pg_size_pretty(pg_database_size('appdb'))")

CHECKSUM_SQL="
SELECT 'accounts', count(*), md5(string_agg(md5(t::text), '' ORDER BY id)) FROM accounts t
UNION ALL SELECT 'events', count(*), md5(string_agg(md5(t::text), '' ORDER BY id)) FROM events t
UNION ALL SELECT 'order_items', count(*), md5(string_agg(md5(t::text), '' ORDER BY id)) FROM order_items t
UNION ALL SELECT 'orders', count(*), md5(string_agg(md5(t::text), '' ORDER BY id)) FROM orders t
UNION ALL SELECT 'users', count(*), md5(string_agg(md5(t::text), '' ORDER BY id)) FROM users t
ORDER BY 1;"
SEQUENCE_SQL="
SELECT 'events_id_seq', last_value FROM events_id_seq
UNION ALL SELECT 'orders_id_seq', last_value FROM orders_id_seq
UNION ALL SELECT 'users_id_seq', last_value FROM users_id_seq
ORDER BY 1;"
SCHEMA_SQL="
SELECT 'indexes', count(*) FROM pg_indexes WHERE schemaname='public'
UNION ALL SELECT 'foreign_keys', count(*) FROM pg_constraint WHERE contype='f'
ORDER BY 1;"

docker exec "$PG" psql -U postgres -d appdb -tAc "$CHECKSUM_SQL" >"$WORKDIR/source-checksums.txt"
docker exec "$PG" psql -U postgres -d appdb -tAc "$SEQUENCE_SQL" >"$WORKDIR/source-sequences.txt"
docker exec "$PG" psql -U postgres -d appdb -tAc "$SCHEMA_SQL" >"$WORKDIR/source-schema.txt"
echo "source: $SOURCE_SIZE"

echo "=== 4/11 configuration (Age encryption) ==="
docker run --rm -u "$(id -u):$(id -g)" -v "$WORKDIR:/work" -w /work "$IMAGE" init \
  --non-interactive --force --config /work/config.yaml --secrets /work/secrets.env \
  --storage-path /backups --temp-dir /tmp --port "$APP_PORT" \
  --metadata-host "$PG" --metadata-port 5432 --metadata-user postgres \
  --metadata-db bm_meta --metadata-password testpass \
  --target-host "$PG" --target-port 5432 --target-user postgres \
  --target-db appdb --target-password testpass --encryption keypair >/dev/null
python3 - "$WORKDIR" <<'PY'
import sys
from pathlib import Path
work = Path(sys.argv[1])
config = work / "config.yaml"
secrets = work / "secrets.env"
config.write_text(config.read_text().replace('  restore_user: ""', '  restore_user: "restore_admin"'))
lines = []
for line in secrets.read_text().splitlines():
    if line.startswith("ENCRYPTION_PASSPHRASE="):
        line = "ENCRYPTION_PASSPHRASE="
    elif line.startswith("TARGET_DB_RESTORE_PASSWORD="):
        line = "TARGET_DB_RESTORE_PASSWORD=restorepass"
    lines.append(line)
secrets.write_text("\n".join(lines) + "\n")
new_config = config.read_text().replace('  dbname: bm_meta\n', '  dbname: bm_meta_new\n', 1)
(work / "config-new.yaml").write_text(new_config)
PY
mkdir -p "$WORKDIR/backups"
docker run --rm -u "$(id -u):$(id -g)" --network "$NET" \
  -v "$WORKDIR/backups:/backups" -v "$WORKDIR:/work" "$IMAGE" \
  config check --connect --config /work/config.yaml >/dev/null
KEY=$(grep '^ADMIN_KEY=' "$WORKDIR/secrets.env" | cut -d= -f2-)

echo "=== 5/11 start service and run backup ==="
docker run -d --rm --name "$APP" --network "$NET" -p "127.0.0.1::$APP_PORT" \
  -v "$WORKDIR/backups:/backups" \
  -v "$WORKDIR/config.yaml:/etc/backup-manager/config.yaml:ro" \
  -v "$WORKDIR/secrets.env:/etc/backup-manager/secrets.env:ro" \
  "$IMAGE" >/dev/null
HOST_PORT=$(docker inspect -f '{{(index (index .NetworkSettings.Ports "8080/tcp") 0).HostPort}}' "$APP")
API="http://127.0.0.1:$HOST_PORT/api/v1"
H="X-API-Key: $KEY"
READY=0
for _ in $(seq 1 60); do
  if curl -fsS "http://127.0.0.1:$HOST_PORT/healthz" >/dev/null 2>&1; then
    READY=1
    break
  fi
  sleep 1
done
if [ "$READY" != "1" ]; then
  echo "service did not become healthy on port $HOST_PORT" >&2
  exit 1
fi

POLICY_ID=$(curl -sS -H "$H" -H 'Content-Type: application/json' -X POST "$API/retention-policies" \
  -d '{"name":"daily","keep_hourly":0,"keep_daily":7,"keep_weekly":0,"keep_monthly":0,"keep_yearly":0,"yearly_month":1}' \
  | jget 'import sys,json; print(json.load(sys.stdin)["id"])')
PROVIDER_ID=$(curl -sS -H "$H" -H 'Content-Type: application/json' -X POST "$API/storage-providers" \
  -d '{"name":"local","type":"local","config":{}}' \
  | jget 'import sys,json; print(json.load(sys.stdin)["id"])')
PROFILE_ID=$(curl -sS -H "$H" -H 'Content-Type: application/json' -X POST "$API/profiles" \
  -d "{\"name\":\"drill\",\"enabled\":false,\"schedule\":\"0 0 * * *\",\"storage_provider_ids\":[\"$PROVIDER_ID\"],\"notification_provider_ids\":[],\"retention_policy_id\":\"$POLICY_ID\",\"compression_type\":\"gzip\",\"compression_level\":6,\"encryption_enabled\":true}" \
  | jget 'import sys,json; print(json.load(sys.stdin)["id"])')

T0=$(now_ms)
curl -sS -o /dev/null -H "$H" -X POST "$API/profiles/$PROFILE_ID/run"
EXEC_ID=""; STATUS=""
for _ in $(seq 1 180); do
  sleep 1
  ROW=$(curl -sS -H "$H" "$API/executions")
  EXEC_ID=$(echo "$ROW" | jget 'import sys,json; d=json.load(sys.stdin)["items"]; print(d[0]["id"] if d else "")')
  STATUS=$(echo "$ROW" | jget 'import sys,json; d=json.load(sys.stdin)["items"]; print(d[0]["status"] if d else "")')
  { [ "$STATUS" = "success" ] || [ "$STATUS" = "failed" ]; } && break
done
T1=$(now_ms)
BACKUP_MS=$((T1 - T0))
for _ in $(seq 1 120); do
  [ -n "$(find "$WORKDIR/backups/meta" -type f 2>/dev/null | head -1)" ] && break
  sleep 1
done
ARTIFACT_REL=$(find "$WORKDIR/backups" -type f \( -name '*.sql.age' -o -name '*.sql.gz' -o -name '*.sql' \) \
  | grep -v '/meta/' | head -1 | sed "s#^$WORKDIR/backups/##")
[ "$STATUS" = "success" ] || { echo "backup failed with status $STATUS" >&2; exit 1; }
[ -n "$ARTIFACT_REL" ] || { echo "backup artifact not found" >&2; exit 1; }
echo "backup=$STATUS wall_ms=$BACKUP_MS execution=$EXEC_ID"
echo "artifact=$ARTIFACT_REL"

echo "=== 6/11 destroy metadata and target ==="
docker rm -f "$APP" >/dev/null
sleep 2
docker exec "$PG" psql -U postgres -d postgres -c \
  "SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname IN ('appdb','bm_meta') AND pid <> pg_backend_pid();" >/dev/null
docker exec "$PG" psql -U postgres -d postgres -c 'DROP DATABASE appdb WITH (FORCE);' >/dev/null
docker exec "$PG" psql -U postgres -d postgres -c 'CREATE DATABASE appdb;' >/dev/null
docker exec "$PG" psql -U postgres -d postgres -c 'DROP DATABASE bm_meta WITH (FORCE);' >/dev/null
TABLES_AFTER_LOSS=$(docker exec "$PG" psql -U postgres -d appdb -tAc "select count(*) from pg_tables where schemaname='public'")
[ "$TABLES_AFTER_LOSS" = "0" ] || { echo "target was not emptied" >&2; exit 1; }

run_recover() {
  docker run --rm -u "$(id -u):$(id -g)" --network "$NET" \
    -v "$WORKDIR/backups:/backups" -v "$WORKDIR:/work" "$IMAGE" "$@"
}

echo "=== 7/11 recover metadata ==="
run_recover recover metadata --config /work/config-new.yaml --list 2>&1 | tee "$WORKDIR/metadata-list.log"
T0=$(now_ms)
run_recover recover metadata --config /work/config-new.yaml 2>&1 | tee "$WORKDIR/metadata-recover.log"
T1=$(now_ms)
METADATA_MS=$((T1 - T0))
PROFILES=$(docker exec "$PG" psql -U postgres -d bm_meta_new -tAc 'select count(*) from profiles')
EXECUTIONS=$(docker exec "$PG" psql -U postgres -d bm_meta_new -tAc 'select count(*) from executions')
EXEC_STATE=$(docker exec "$PG" psql -U postgres -d bm_meta_new -tAc "select status || '|' || (checksum is not null)::text || '|' || (storage_path is not null)::text from executions")
[ "$PROFILES" = "1" ] || { echo "metadata profiles=$PROFILES" >&2; exit 1; }
[ "$EXECUTIONS" = "1" ] || { echo "metadata executions=$EXECUTIONS" >&2; exit 1; }
[ "$EXEC_STATE" = "success|true|true" ] || { echo "metadata execution is stale: $EXEC_STATE" >&2; exit 1; }
echo "metadata_ms=$METADATA_MS profiles=$PROFILES executions=$EXECUTIONS"

echo "=== 8/11 verify without touching the target ==="
T0=$(now_ms)
run_recover recover data --config /work/config-new.yaml --profile "$PROFILE_ID" --verify-only 2>&1 | tee "$WORKDIR/verify.log"
T1=$(now_ms)
VERIFY_MS=$((T1 - T0))
TARGET_TABLES=$(docker exec "$PG" psql -U postgres -d appdb -tAc "select count(*) from pg_tables where schemaname='public'")
[ "$TARGET_TABLES" = "0" ] || { echo "verify-only wrote to the target" >&2; exit 1; }
echo "verify_ms=$VERIFY_MS"

echo "=== 9/11 recover data ==="
T0=$(now_ms)
run_recover recover data --config /work/config-new.yaml --profile "$PROFILE_ID" --confirm 2>&1 | tee "$WORKDIR/data-recover.log"
T1=$(now_ms)
DATA_MS=$((T1 - T0))
docker exec "$PG" psql -U postgres -d appdb -tAc "$CHECKSUM_SQL" >"$WORKDIR/restored-checksums.txt"
docker exec "$PG" psql -U postgres -d appdb -tAc "$SEQUENCE_SQL" >"$WORKDIR/restored-sequences.txt"
docker exec "$PG" psql -U postgres -d appdb -tAc "$SCHEMA_SQL" >"$WORKDIR/restored-schema.txt"
diff -u "$WORKDIR/source-checksums.txt" "$WORKDIR/restored-checksums.txt"
diff -u "$WORKDIR/source-sequences.txt" "$WORKDIR/restored-sequences.txt"
diff -u "$WORKDIR/source-schema.txt" "$WORKDIR/restored-schema.txt"
echo "data_ms=$DATA_MS total_recovery_ms=$((METADATA_MS + DATA_MS))"

echo "=== 10/11 artifact-only recovery ==="
run_recover recover data --config /work/config-new.yaml \
  --file "/backups/$ARTIFACT_REL" \
  --target-dsn "postgres://restore_admin:restorepass@$PG:5432/appdb_file?sslmode=disable" \
  --confirm 2>&1 | tee "$WORKDIR/file-recover.log"
FILE_USERS=$(docker exec "$PG" psql -U postgres -d appdb_file -tAc 'select count(*) from users')
[ "$FILE_USERS" = "$USERS" ] || { echo "file recovery users=$FILE_USERS want=$USERS" >&2; exit 1; }

echo "=== 11/11 safety guards ==="
if run_recover recover data --config /work/config-new.yaml --profile "$PROFILE_ID" \
  --target-dsn "postgres://postgres:testpass@$PG:5432/bm_meta_new?sslmode=disable" --confirm >/dev/null 2>&1; then
  echo "guard failed: metadata database accepted as a target" >&2
  exit 1
fi
if run_recover recover data --config /work/config-new.yaml --profile "$PROFILE_ID" >/dev/null 2>&1; then
  echo "guard failed: restore without --confirm was accepted" >&2
  exit 1
fi

echo
echo "=== DRILL PASSED ==="
echo "source_db=$SOURCE_SIZE"
echo "backup_wall_ms=$BACKUP_MS"
echo "metadata_recover_ms=$METADATA_MS"
echo "verify_only_ms=$VERIFY_MS"
echo "data_recover_ms=$DATA_MS"
echo "total_recovery_ms=$((METADATA_MS + DATA_MS))"
echo "checksums, sequences, indexes and foreign keys match"
