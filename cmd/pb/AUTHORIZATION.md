# PB authentication and authorization

How the Portal Backend decides who a caller is and what they may do. The overview is in the [README](README.md); the route-by-route scopes are in its API table and in [openapi.yaml](openapi.yaml).

## Authentication

Every `/api/v1/*` request needs `Authorization: Bearer <access token>`. The token is validated by [`internal/authn`](../../internal/authn/README.md) against the identity provider's signing keys (`IDP_JWKS_URL`, cached for 5 minutes):

| Check | Requirement |
|---|---|
| Signature | Valid for a key in the JWKS. |
| `iss` | Equals `IDP_ISSUER`. |
| `aud` | Contains `IDP_AUDIENCE`, PB's resource-server identifier. A token for any other audience is rejected, whichever client it was issued to. Clients request the audience as `resource=<IDP_AUDIENCE>` when they log in. |
| `sub`, `client_id` | Both present. |
| `scope` | Space-separated scopes; see below. |
| `ouId` | Optional. The caller's organization unit, which links the caller to a member. `email` is read when present. |

A missing or invalid token returns `401` with `{"error": "unauthorized", "message": "..."}`.

## Scopes

Each `/api/v1` route requires exactly one scope from the token's `scope` claim; otherwise PB returns `403` with `{"error": "forbidden", "message": "insufficient scope"}`. The scope for each route is registered next to it in [main.go](main.go); the names are in [`internal/pb/caller/scopes.go`](../../internal/pb/caller/scopes.go).

A scope is `<resource>:<action>`:

| Resource | Own member | Every member |
|---|---|---|
| Members | `member:read`, `member:write` | `member:read-all`, `member:write-all` |
| Schemas | `schema:read`, `schema:write` | `schema:read-all`, `schema:write-all` |
| Schema submissions | `schema-submission:read`, `schema-submission:write` | `schema-submission:read-all`, `schema-submission:write-all` |
| Applications | `application:read`, `application:write` | `application:read-all`, `application:write-all` |
| Application submissions | `application-submission:read`, `application-submission:write` | `application-submission:read-all`, `application-submission:write-all` |

- `GET` routes require `<resource>:read`. `POST`, `PUT`, `PATCH` and `DELETE` routes require `<resource>:write`.
- The exception is `POST /api/v1/members`, which requires `member:write-all`.
- Policy-metadata routes under `/schemas/{schemaId}/` use the schema scopes, and `PUT /applications/{applicationId}/policy` uses the application scopes.
- The `-all` scopes never open a route by themselves. Handlers check them to widen access beyond the caller's own member. A caller that works across members therefore needs both, for example `schema:read` and `schema:read-all`.

## Whose records a caller can reach

After authentication PB looks up the member whose `ouId` matches the token's `ouId` and attaches it to the request. A caller with no `ouId`, or whose organization unit has no member, continues without one.

Handlers then apply one rule: a caller holding the resource's `-all` scope may reach every member's records. Any other caller reaches only its own member's records, and gets `403` for another member's record, or when it has no member.

For list endpoints this means:

- `GET /members`: with `member:read-all`, the `ouId` query parameter filters or is omitted to list everyone. Without it the result is the caller's own organization unit, and `403` if it has none.
- `GET /schemas`, `/applications` and the submission lists: `memberId` is honored only with the matching `-all` scope; otherwise the caller's own member is used.

For creating records, a caller without the matching `-all` write scope always creates for its own member: a schema or application is created for the caller's member whatever `memberId` says, and a submission with another member's `memberId` is rejected with `403`. A caller with no member gets `403`.

## Roles

PB never reads role names. ThunderID grants scopes through roles on PB's resource server ([thunderid/bootstrap/resource.yaml](../../thunderid/bootstrap/resource.yaml)), so changing what a role may do is a ThunderID change.

| Role | Scopes |
|---|---|
| `OpenNDX_Admin` | All twenty scopes |
| `OpenNDX_Member` | `read` and `write` on members, schemas, schema submissions, applications and application submissions (no `-all`) |

ThunderID issues only the scopes a client requests and the user's roles allow, so clients must request the scopes they need, together with `resource=<IDP_AUDIENCE>`. The `ondx` `local` profile requests all of them; the portals take theirs from their scope setting.

## Submission approval

A schema or application submission becomes a schema or application when its status is set to `approved` with `PUT /api/v1/schema-submissions/{submissionId}` (or the application equivalent). PB checks the scope and ownership described above and nothing more:

- Any caller allowed to update a submission can set its status, including to `approved`. There is no separate reviewer scope, so with the bundled roles a member can approve its own submission, and an admin holding the `-all` write scopes can approve any.
- Each `PUT` that sets `status` to `approved` creates a new schema or application, including when the submission is already approved.
- If the creation fails, the submission's status is set back to `pending`.

## Audit

Create, update and delete requests are sent to the Argus audit service (when configured) with the token's `sub` as actor and its `client_id` in the event metadata. The actor type is `ADMIN` for callers holding `member:write-all`, `MEMBER` for other callers whose token has an `ouId`, and `SYSTEM` otherwise.

## Unauthenticated routes

`GET /health`, `GET /debug`, `GET /debug/db` and `GET /internal/api/v1/applications` skip authentication and scope checks. Protect them at the network level.
