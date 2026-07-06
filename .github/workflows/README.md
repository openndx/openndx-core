# GitHub Actions Workflows

This directory contains GitHub Actions workflows for CI/CD, validation, and publishing.

## Workflow Types

### 1. Validation Workflows (Pull Requests)

Run on every PR when service code changes. Perform code quality checks and tests.

| Workflow                             | Service               | Triggers On                                    |
| ------------------------------------ | --------------------- | ---------------------------------------------- |
| `consent-engine-validate.yml`        | Consent Engine        | Changes to `exchange/consent-engine/**`        |
| `orchestration-engine-validate.yml`  | Orchestration Engine  | Changes to `exchange/orchestration-engine/**`  |
| `policy-decision-point-validate.yml` | Policy Decision Point | Changes to `exchange/policy-decision-point/**` |
| `portal-backend-validate.yml`        | Portal Backend        | Changes to `portal-backend/**`                 |
| `integration-tests.yml`              | All Services          | Manual or scheduled                            |

**What they do:**

- Go mod tidy check
- Go build
- Unit & integration tests
- TruffleHog secret scanning

### 2. Docker Validation Workflows (Dockerfile Changes Only)

Run only when Dockerfiles are modified. Optimizes CI time by skipping Docker builds on code-only changes.

| Workflow                                    | Triggers On                                            |
| ------------------------------------------- | ------------------------------------------------------ |
| `consent-engine-docker-validate.yml`        | Changes to `exchange/consent-engine/Dockerfile`        |
| `orchestration-engine-docker-validate.yml`  | Changes to `exchange/orchestration-engine/Dockerfile`  |
| `policy-decision-point-docker-validate.yml` | Changes to `exchange/policy-decision-point/Dockerfile` |
| `portal-backend-docker-validate.yml`        | Changes to `portal-backend/Dockerfile`                 |

**What they do:**

- Docker image build validation
- Trivy security vulnerability scanning
- Upload results to GitHub Security tab

### 3. Publish Workflows (Production)

Build and publish Docker images to GitHub Container Registry when code is merged to main.

| Workflow                            | Service               | Image                                          |
| ----------------------------------- | --------------------- | ---------------------------------------------- |
| `consent-engine-publish.yml`        | Consent Engine        | `ghcr.io/{owner}/{repo}/consent-engine`        |
| `orchestration-engine-publish.yml`  | Orchestration Engine  | `ghcr.io/{owner}/{repo}/orchestration-engine`  |
| `policy-decision-point-publish.yml` | Policy Decision Point | `ghcr.io/{owner}/{repo}/policy-decision-point` |
| `portal-backend-publish.yml`        | Portal Backend        | `ghcr.io/{owner}/{repo}/portal-backend`        |
| `release.yml`                       | All Services          | Builds all services with version tags          |

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
# Test local build
cd exchange
docker build -f consent-engine/Dockerfile \
  --build-arg SERVICE_PATH=consent-engine \
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

Update `docker-compose.yml`:

```yaml
services:
  consent-engine:
    image: ghcr.io/{owner}/{repo}/consent-engine:latest
    # Remove 'build:' section
```

Then:

```bash
docker compose pull
docker compose up -d
```

## Workflow Optimization

### Why Separate Docker Validation?

Docker validation workflows are separated from code validation to:

- **Reduce CI time**: Skip Docker builds when only code changes
- **Faster feedback**: Get test results quicker on code-only PRs
- **Resource efficiency**: Save GitHub Actions minutes
- **Cleaner PR checks**: Only relevant checks appear (no skipped jobs)

Docker validation only runs when Dockerfiles are modified, as Docker layer caching makes rebuilds fast and build failures due to code changes are caught by Go build steps.

## Troubleshooting

**Validation workflow doesn't trigger:**

- Check if files in the service directory changed
- Verify the PR is targeting the correct branch

**Docker validation workflow doesn't trigger:**

- This is expected if you only changed code files
- Docker validation only runs when the Dockerfile is modified
- Use workflow_dispatch to manually trigger if needed

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
