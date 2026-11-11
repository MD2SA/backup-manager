# Testing

Backup Manager uses three layers of tests. The fast layer runs on every pull
request; the integration layer uses a disposable PostgreSQL database; the
container-level recovery drill is run before releases and when changing the
recovery path.

## Fast tests

No database, Docker or network access required.

```bash
make test
```

This runs `go test ./...`, including:

- configuration parsing and validation;
- backup pipeline and artifact format detection;
- encryption/decryption;
- metadata snapshot selection and multi-passphrase decryption;
- S3 provider behaviour against an `httptest` server;
- the `internal/recovery` unit tests.

Run with the race detector:

```bash
go test ./... -race -count=1
```

## PostgreSQL integration tests

These tests create and drop disposable databases. Point
`BACKUP_MANAGER_TEST_DSN` at an **empty, disposable** PostgreSQL instance that
the test user can create databases on. Never use a production DSN.

```bash
export BACKUP_MANAGER_TEST_DSN='postgres://postgres:postgres@127.0.0.1:5432/postgres?sslmode=disable'
make test-integration
```

The test suite includes:

- restore and artifact integration;
- authentication integration;
- pagination integration;
- the single-instance advisory lock;
- the full recovery contract in `internal/recovery`:
  1. seed a target database with tables, indexes, foreign keys and sequences;
  2. run a real backup with metadata embedding;
  3. destroy both the metadata and target databases;
  4. recover metadata from the embedded snapshot;
  5. verify that the restored execution has a valid checksum and storage path;
  6. run `--verify-only` and confirm the target is untouched;
  7. restore the data and compare checksums, sequences, indexes and foreign
     keys with the original database;
  8. restore from the artifact alone with `--file`;
  9. confirm that the metadata database cannot be used as a target.

The recovery integration test runs twice: once with the profile encryption
passphrase and once with an Age X25519 identity.

Requirements: `pg_dump`, `pg_restore` and `psql` in `PATH`, plus permission to
create databases.

## Container recovery drill

The final layer runs the real container image. It builds a disposable
PostgreSQL instance and a synthetic database, runs a backup, destroys both the
metadata database and the target database, and recovers everything through the
CLI.

```bash
make recovery-drill
```

Useful options:

```bash
scripts/recovery-drill.sh --users 50000   # larger dataset
scripts/recovery-drill.sh --keep          # keep the work directory
scripts/recovery-drill.sh --no-build      # reuse an existing image
```

The drill asserts:

- metadata recovery restores a successful execution with checksum and storage path;
- `--verify-only` does not touch the target;
- row-level checksums match the source for every table;
- sequence values, indexes and foreign keys match;
- artifact-only recovery (`--file`) produces the same data;
- restoring into the metadata database is rejected;
- restoring without `--confirm` is rejected.

Requirements: Docker, `curl` and `python3`. The script finds free ports and uses
a dedicated Docker network; no fixed host ports are required.

## CI

The `test` job runs the fast suite with `-race`, then the PostgreSQL integration
tests against a `postgres:16-alpine` service container. The recovery integration
test is a required step, so a change that breaks metadata recovery or data
restore fails the pull request.

A separate `Recovery drill` workflow runs the container drill:

- weekly (`schedule`);
- on demand (`workflow_dispatch`);
- on pull requests that touch recovery, backup, storage, config, metadata
  restore, the CLI, the drill script or the Dockerfile.

The drill log is uploaded as an artifact when it fails.

## Safety rules for integration tests

- Use a dedicated disposable PostgreSQL instance.
- Never point `BACKUP_MANAGER_TEST_DSN` at production.
- Tests create databases with random suffixes and drop them in `t.Cleanup`.
- Never commit credentials or generated keys.
- Use `t.TempDir()` for storage and temporary artifacts.

## Adding tests

- Prefer behaviour assertions over implementation details.
- For recovery, assert data invariants: row counts, per-row checksums,
  sequences, indexes and foreign keys.
- Gate tests that need infrastructure behind `BACKUP_MANAGER_TEST_DSN` and
  `t.Skip` when it is absent.
- Keep the fast suite independent of Docker and external services.
