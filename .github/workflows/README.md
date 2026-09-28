# GitHub Actions Workflows

This directory contains GitHub Actions workflows for CI/CD, validation, and publishing.

## Workflow Types

### 1. Validation Workflows (Pull Requests)

Run on every PR when service code changes. Perform code quality checks and tests.

| Workflow                             | Service               | Triggers On                                    |
| ------------------------------------ | --------------------- | ---------------------------------------------- |
| `consent-engine-validate.yml`        | Consent Engine        | Changes to `cmd/ce/**`, `internal/ce/**`       |
| `orchestration-engine-validate.yml`  | Orchestration Engine  | Changes to `cmd/oe/**`, `internal/oe/**`       |
| `policy-decision-point-validate.yml` | Policy Decision Point | Changes to `cmd/pdp/**`, `internal/pdp/**`     |
| `portal-backend-validate.yml`        | Portal Backend        | Changes to `cmd/pb/**`, `internal/pb/**`       |
| `integration-tests.yml`              | All Services          | Manual or scheduled                            |

**What they do:**

- Go mod tidy check
- Go build
- Unit & integration tests
- TruffleHog secret scanning
- Docker image build validation
- Trivy vulnerability scanning, with results uploaded to the GitHub Security tab

### 2. Publish Workflows (Production)

Build and publish Docker images to GitHub Container Registry when code is merged to main.

| Workflow                            | Service               | Image                                                       |
|-------------------------------------|-----------------------|-------------------------------------------------------------|
| `consent-engine-publish.yml`        | Consent Engine        | `ghcr.io/{owner}/consent-engine`                            |
| `orchestration-engine-publish.yml`  | Orchestration Engine  | `ghcr.io/{owner}/orchestration-engine`                      |
| `policy-decision-point-publish.yml` | Policy Decision Point | `ghcr.io/{owner}/policy-decision-point`                     |
| `portal-backend-publish.yml`        | Portal Backend        | `ghcr.io/{owner}/portal-backend`                            |
| `release.yml`                       | All Services, `ondx`  | Builds all services + `ondx` CLI binaries with version tags |

**Triggers:**

- Push to main (when service code changes)
- Manual dispatch from GitHub Actions UI

**Process:**

1. Builds Docker image
2. Tags with `latest` and commit SHA
3. Scans for vulnerabilities (Trivy)
4. Publishes to GHCR

**Image Tags:**

- `latest` - Latest build from main
- `{branch}-{sha}` - Specific commit (e.g., `main-abc123`)

## Quick Test

```bash
# Test local build (from the repository root)
docker build -f cmd/ce/Dockerfile \
  --build-arg SERVICE_PATH=cmd/ce \
  --build-arg BUILD_VERSION=test \
  --build-arg BUILD_TIME=$(date -u +"%Y-%m-%dT%H:%M:%SZ") \
  --build-arg GIT_COMMIT=test \
  -t consent-engine:test .

# Test image runs
docker run --rm -p 8081:8081 \
  -e ENVIRONMENT=local -e PORT=8081 \
  consent-engine:test
```

## Security Scanning

All workflows include Trivy scanning:

- Scans images after build
- Fails on CRITICAL/HIGH vulnerabilities
- Results in GitHub Security tab

**View results:** Repository → Security → Code scanning alerts

## Using Published Images

Update `../../compose.yml`:

```yaml
services:
  consent-engine:
    image: ghcr.io/openndx/consent-engine:latest
    # Remove 'build:' section
```

Then:

```bash
docker compose pull
docker compose up -d
```

## Troubleshooting

**Validation workflow doesn't trigger:**

- Check if files in the service directory changed
- Verify the PR is targeting the correct branch

**Build fails:**

- Test build locally first
- Check Dockerfile and dependencies
- Review test database configuration

**Image not found:**

- Check image visibility in GitHub package settings
- Login: `echo $GITHUB_TOKEN | docker login ghcr.io -u {username} --password-stdin`

## Resources

- [Release Guide](RELEASE_GUIDE.md) - How to create releases with version tags
- [GitHub Container Registry Docs](https://docs.github.com/en/packages/working-with-a-github-packages-registry/working-with-the-container-registry)
