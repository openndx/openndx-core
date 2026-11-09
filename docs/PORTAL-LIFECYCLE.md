# Portal lifecycle decision

Status: accepted for the first release made after this change is merged.

## Context

OpenNDX is still in pre-release development. The Consent Portal is in use and
actively maintained. The Admin and Member Portals have no current customers,
development is inactive, and there is no allocated capacity to bring them up to
date. Their READMEs document incompatibilities with the current management API
and identity provider. Publishing fresh images alongside maintained components
misrepresents their readiness.

## Decision

| Component | Status | Release treatment |
|---|---|---|
| Consent Portal | Actively maintained | Continue validation and image publication |
| Admin Portal | Development paused; unsupported prototype | Exclude from release images and automatic publication |
| Member Portal | Development paused; unsupported prototype | Exclude from release images and automatic publication |
| Portal Backend and `ondx` | Actively maintained management API and CLI | Continue validation and publication |

Retain Admin and Member Portal source in `portals/apps/admin` and
`portals/apps/member` for reference and possible future development. Their
documentation describes the existing prototypes, rather than a supported
deployment. There is no commitment to fixes, dependency updates, backend
compatibility, or a delivery date while development is paused.

Remove both apps from the release image matrix and automatic publishing.
Validation workflows remain available through manual dispatch without publishing
images. Default Make batch commands and automatic pre-commit validation exclude
these apps; explicit local commands remain available. Repository secret scanning
still applies to all source.

Keep the Consent Portal, Portal Backend, and other maintained components in the
release. This decision does not remove management APIs or submission workflows.

## Current management path

Use the [`ondx` CLI](../cmd/ondx/README.md) to log in, register and inspect members,
schemas and applications, and grant applications access to fields. The
[end-to-end walkthrough](../dev/WALKTHROUGH.md) and
[`ondx` tutorial](TUTORIAL-ondx-cli.md) describe the current registration and
consent flow.

The CLI does not cover every management operation. Use the authenticated
[Portal Backend API](../cmd/pb/README.md) and its
[OpenAPI contract](../cmd/pb/openapi.yaml) for operations such as member updates
and submission review. Calls require the appropriate
[scopes and member access](../cmd/pb/AUTHORIZATION.md); use the API rather than
editing database records directly. Identity-provider organizations and users
must be configured separately, as described in the CLI documentation.

## Release communication and existing artifacts

The first release containing this change excludes Admin and Member Portal images.
The release-note template announces their status and directs operators to the CLI
and management API. Previously published images are historical, unsupported
artifacts; a tag such as `latest` on an old image does not imply current support.
Existing images, release records, and Git tags are retained. This change does not
delete deployments or migrate data.

There are no known customers to migrate from these portals. Anyone evaluating an
older image should use the management path above for a current deployment.

## Ownership and review

OpenNDX maintainers own this decision and should review it during release planning
when new adopter demand or development capacity becomes available. Review does
not imply a commitment to resume development.

Before resuming either portal, require:

1. A concrete adopter and agreed user workflows.
2. A named owner and allocated implementation and maintenance capacity.
3. Resolution of the app's documented authentication, authorization, API, and
   runtime integration gaps.
4. Validation of complete user journeys against the current stack, including
   organization access boundaries and failure handling.
5. An explicit support status, updated documentation, and restored validation
   before reintroducing published images.
