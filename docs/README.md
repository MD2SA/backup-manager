# Documentation

Canonical documentation lives in this directory and is published at
<https://md2sa.github.io/backup-manager/>.

| Document | Contents |
|---|---|
| [Configuration](guides/CONFIGURATION.md) | all fields, secrets, production checklist, PostgreSQL compatibility |
| [Operations](guides/OPERATIONS.md) | monitoring, upgrades, backups, restores, recovery after host loss |
| [API](guides/API.md) | endpoints, authentication, pagination and examples |
| [Architecture](guides/ARCHITECTURE.md) | modules, backup lifecycle, design decisions |
| [Testing](guides/TESTING.md) | unit, integration, S3 and container recovery drill |
| [Releasing](guides/RELEASING.md) | versioning, tags, GHCR publishing, verification and rollback |
| [Observability](../deploy/observability/README.md) | Prometheus rules and the Grafana dashboard |

Swagger UI is served by the running service at `/swagger/index.html`.

## Local preview

```bash
python3 -m venv .venv
. .venv/bin/activate
pip install -r requirements-docs.txt
mkdocs serve
```

Build the same site CI publishes:

```bash
mkdocs build --strict
```

## Contributing to the docs

- Keep source-of-truth detail in this directory, not only in the README.
- Update the README when a change affects installation or compatibility.
- Do not duplicate long procedures; link between documents.
