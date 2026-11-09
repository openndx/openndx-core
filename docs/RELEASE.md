# Release Guide

Releases are automated via GitHub Actions when a semantic version tag is pushed (e.g., `v1.0.0`).

## How to Release

```bash
# 1. Checkout main
git checkout main && git pull

# 2. Create & Push Tag
git tag v1.0.0
git push origin v1.0.0
```

This triggers the **Release** workflow which:
1.  Builds Docker images for the **5 maintained components** listed below.
2.  Pushes version tags (for example `1.0.0`, `1.0`, `1`), `sha-<commit>`, and `latest` for versions without a pre-release suffix.
3.  Scans images with Trivy.
4.  Builds the `ondx` CLI binaries and creates a GitHub Release with changelogs and download artifacts.

## Manual Release
Go to **Actions** → **Release - Build and Publish Maintained Components** → **Run workflow** → Enter version (e.g., `v1.0.0`). A manual run on `main` publishes images; the CLI and GitHub Release job runs only for tag pushes.

## Artifacts

All images are published to **ghcr.io/openndx/** (previously `ghcr.io/openndx/openndx-core/`; existing tags under the old path remain available but are no longer updated):

| Category     | Service               | Image Name              |
|:-------------|:----------------------|:------------------------|
| **Backend**  | Portal Backend        | `portal-backend`        |
|              | Policy Decision Point | `policy-decision-point` |
|              | Consent Engine        | `consent-engine`        |
|              | Orchestration Engine  | `orchestration-engine`  |
| **Frontend** | Consent Portal        | `consent-portal`        |

## Paused portals

Starting with the first release containing this change, Admin and Member Portals
are excluded from release images and automatic publishing. Their source remains
available as unsupported prototypes; there is no committed maintenance or backend
compatibility. Previously published images and release tags are retained as
historical artifacts, and their old `latest` tags do not indicate current support.

Use the `ondx` CLI or Portal Backend API for management. The Consent Portal and
Portal Backend remain maintained and released. See the
[portal lifecycle decision](PORTAL-LIFECYCLE.md) for scope, management alternatives,
and restart criteria. The GoReleaser release footer includes this notice in future
release notes.

## Verification
```bash
docker pull ghcr.io/openndx/portal-backend:1.0.0
```
