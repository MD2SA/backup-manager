# Releasing

This project uses annotated Git tags and GitHub Actions to publish container
images to the GitHub Container Registry (GHCR).

## Versioning

Follow [Semantic Versioning](https://semver.org/):

- `v1.2.3` — patch: fixes only;
- `v1.3.0` — minor: backwards-compatible features;
- `v2.0.0` — major: breaking API, configuration or storage changes.

Use `v0.y.z` only while the public API and configuration are still unstable.
This project is scoped as production-ready for a single PostgreSQL instance, so
`v1.0.0` is a reasonable first release.

Pre-release tags such as `v1.0.0-rc.1` publish only the exact version image;
they do not update `latest`, `1.0` or `1`.

## Before tagging

1. `main` is green and the release PR is merged.
2. Update `CHANGELOG.md`:
   - move `## [Unreleased]` to `## [1.0.0] - YYYY-MM-DD`;
   - add a new empty `## [Unreleased]` section at the top.
3. Confirm the documentation and examples point at the new version if needed.
4. Merge the release-prep PR through the protected `main` branch.

```bash
git switch main
git pull --ff-only
git switch -c release/v1.0.0
# edit CHANGELOG.md
git commit -am "docs: prepare v1.0.0"
git push -u origin release/v1.0.0
```

Open a pull request, wait for the required check, and squash-merge it.

## Tag and publish

Tag the exact commit that passed CI:

```bash
git switch main
git pull --ff-only

git tag -a v1.0.0 -m "Backup Manager v1.0.0"
git push origin v1.0.0
```

The tag push triggers `.github/workflows/ci.yml`:

1. the full test job runs;
2. the Docker job waits for it;
3. the image is built for `linux/amd64` and `linux/arm64`;
4. provenance and SBOM attestations are attached;
5. the image is pushed to GHCR;
6. stable tags are also pushed to Docker Hub when
   `DOCKERHUB_USERNAME` and `DOCKERHUB_TOKEN` are configured.

Expected image tags for `v1.0.0`:

| Tag | Meaning |
|---|---|
| `1.0.0` | exact release; use this in production |
| `1.0` | latest `1.0.x` |
| `1` | latest `1.x` |
| `latest` | latest stable release |
| `sha-<short>` | commit traceability |

`main` images are published on pushes to `main`. They are development builds;
do not deploy them to production.

## Docker Hub mirror

Docker Hub is an optional discovery mirror for **stable releases only**. It is
enabled when these repository secrets exist:

```text
DOCKERHUB_USERNAME
DOCKERHUB_TOKEN
```

Create a public Docker Hub repository named `backup-manager` under that
namespace and use an access token with read/write access. Development builds
(`main`, `sha-*`) stay on GHCR only.

## Verify the image

```bash
docker pull ghcr.io/md2sa/backup-manager:1.0.0

docker run --rm ghcr.io/md2sa/backup-manager:1.0.0 version
```

If Docker Hub is configured:

```bash
docker pull <dockerhub-namespace>/backup-manager:1.0.0
```

Check the manifest and attestations:

```bash
docker buildx imagetools inspect ghcr.io/md2sa/backup-manager:1.0.0
```

## Create the GitHub Release

The Git tag triggers the image build. The GitHub Release is the human-facing
record.

```bash
gh release create v1.0.0 \
  --title "Backup Manager v1.0.0" \
  --notes "See CHANGELOG.md for the full list of changes."
```

Or use the GitHub UI:

```text
Releases → Draft a new release → choose v1.0.0
```

Use the `[1.0.0]` section of `CHANGELOG.md` as the release notes.

## Registry visibility and authentication

GHCR packages can be public or private. If the package is private, deployment
hosts need a token with `read:packages`:

```bash
echo "$GHCR_TOKEN" | docker login ghcr.io -u YOUR_USER --password-stdin
```

Do not use the workflow's `GITHUB_TOKEN` on servers.

## Production deployment

Pin the exact version, not `latest`, `main`, `1` or `1.0`:

```env
IMAGE_TAG=ghcr.io/md2sa/backup-manager:1.0.0
```

For stronger immutability, pin the digest:

```env
IMAGE_TAG=ghcr.io/md2sa/backup-manager@sha256:...
```

After changing the image, run:

```bash
docker compose pull
docker compose up -d
curl -fsS http://localhost:8080/readyz
backup-manager config check --connect --config /etc/backup-manager/config.yaml
```

## Do not move published tags

Once a release is public, do not delete or force-update its tag. Publish a new
patch release instead. A tag ruleset should block tag deletion and force
updates for `refs/tags/v*`.

## Rollback

Roll back by deploying the previous known-good version:

```env
IMAGE_TAG=ghcr.io/md2sa/backup-manager:0.9.3
```

Database migrations are additive; newer schemas are ignored by older binaries.
Restoring a metadata snapshot requires `recover metadata` before starting the
older image.
