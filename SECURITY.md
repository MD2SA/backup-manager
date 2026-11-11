# Security Policy

## Supported versions

Security fixes are applied to the `main` branch and the latest published
release. Older releases may not receive fixes.

## Reporting a vulnerability

Please do **not** open a public issue for a vulnerability.

Use GitHub's private vulnerability reporting:

<https://github.com/MD2SA/backup-manager/security/advisories/new>

Include:

- affected version, commit or container tag;
- deployment model (Docker, binary, reverse proxy);
- a minimal reproduction;
- impact and any known workaround;
- relevant logs with secrets, connection strings and webhook URLs removed.

You will receive an acknowledgement when the report is triaged. Fixes and
disclosures are coordinated through the private advisory.

## Security model

- API authentication uses per-user API keys stored as SHA-256 hashes.
  Every user has the single admin role; keys provide attribution and revocation.
- `ADMIN_KEY` bootstraps the first admin and should be removed once another
  active user exists.
- `server.env: production` requires backup encryption and a provider-config
  encryption key.
- Provider credentials are encrypted at rest with AES-256-GCM when
  `CONFIG_ENCRYPT_KEY` is set.
- `config.yaml` is non-secret; `secrets.env` must be mode `0600` and must never
  be committed.
- The service serves HTTP. Terminate TLS in a reverse proxy and do not expose
  port `8080` directly to the public internet.
- Run one replica. The PostgreSQL advisory lock prevents two active schedulers.

## Out of scope

- PITR/WAL, database-engine plugins and multi-tenant access control are not
  implemented.
- Vulnerabilities in PostgreSQL, Docker, the operating system or a reverse proxy
  should be reported to their respective projects.
- Denial of service through a deliberately misconfigured deployment is not a
  vulnerability.
