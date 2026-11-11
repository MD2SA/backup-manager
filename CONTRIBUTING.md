# Contributing

Thanks for taking the time to improve Backup Manager. This project favours
small, reviewable changes with tests and documentation.

## Before you start

- Search existing issues and discussions.
- For a large feature or architectural change, open an issue first so the
  approach can be agreed before implementation.
- Do not report security vulnerabilities in public issues. See
  [SECURITY.md](SECURITY.md).

## Requirements

- Go 1.26+
- Docker and Docker Compose
- PostgreSQL client binaries matching the target version (`pg_dump`,
  `pg_restore`, `psql`)
- Python 3 only if you are working on the documentation site

## Setup

```bash
git clone https://github.com/MD2SA/backup-manager.git
cd backup-manager

make tools
make init
make config-check
```

`make init` writes `config.yaml` and `secrets.env`; both are ignored by git.
Never commit them.

## Tests

```bash
make test              # fast suite, no database required
make test-integration  # PostgreSQL integration tests
make recovery-drill    # real container disaster-recovery drill
```

For integration tests, point `BACKUP_MANAGER_TEST_DSN` at a disposable
PostgreSQL instance that the user can create databases on:

```bash
export BACKUP_MANAGER_TEST_DSN='postgres://postgres:postgres@127.0.0.1:5432/postgres?sslmode=disable'
make test-integration
```

Never use a production DSN.

The recovery drill requires Docker and exercises the real image: backup,
metadata loss, target loss, restore, checksums and safety guards.

## Code style

- Run `make fmt` before committing.
- Run `make lint` when possible.
- Keep packages focused; business logic belongs in `internal/`, not `cmd/`.
- Prefer explicit errors with context (`fmt.Errorf("...: %w", err)`).
- Do not log secrets, passwords, API keys or webhook URLs.
- Add tests for behaviour changes.
- Update `CHANGELOG.md` for user-visible changes.

## Commit messages

Use a short imperative subject with a conventional prefix:

```text
feat(recovery): add artifact-only restore
fix(runtime): reconcile interrupted backups
docs: document PostgreSQL compatibility
test(s3): cover list pagination
```

Keep commits logical. Do not mix refactors, formatting and behaviour changes in
one commit unless they are inseparable.

## Pull requests

A pull request should include:

- what changed and why;
- how it was tested;
- configuration, migration or breaking changes;
- documentation updates;
- a `CHANGELOG.md` entry for user-visible changes.

The PR template contains the checklist. CI must pass, including the PostgreSQL
matrix and recovery drill when the change touches recovery-critical paths.

## Documentation

- Canonical documentation lives in `docs/`.
- Update the README when installation, compatibility or the quick start changes.
- Preview the site with `make docs-serve`; build it strictly with
  `make docs-build`.
- Prefer links over duplicated procedures.

## License

By contributing, you agree that your contribution is licensed under the MIT
license.
