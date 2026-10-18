# PDP policy model

How the Policy Decision Point represents field policy and turns it into a decision. API shapes are in [openapi.yaml](openapi.yaml); the service overview is in the [README](README.md).

## Policy metadata

One record describes one field of one schema. `(schemaId, fieldName)` is unique.

| Field | Values | Meaning |
|---|---|---|
| `schemaId` | string | The provider schema the field belongs to. |
| `fieldName` | string | The field path, for example `person.fullName`. The Orchestration Engine sends its `@sourceInfo` `providerField` here. |
| `displayName`, `description` | optional text | Shown to the data owner on the consent screen. |
| `source` | `primary`, `fallback` | Whether the provider is the primary or a fallback source for the field. |
| `isOwner` | boolean | `true` when the field is owned by the provider itself, so no citizen consent applies. |
| `owner` | `citizen` | Who owns the field. Required when `isOwner` is `false` and must be absent when it is `true`. |
| `accessControlType` | `public`, `restricted` | `restricted` fields need owner consent from a non-owner caller. |
| `allowList` | map | Applications allowed to read the field, each with an expiry. Managed through the allow-list endpoints, never set when creating metadata. |

`POST /api/v1/policy/metadata` is a sync for one schema. It upserts the records you send and deletes the schema's records that you omit.

## Allow-list

The allow-list is a JSON map stored on each record. The key is the application's OIDC `client_id`; the value holds `expires_at` and `updated_at`.

`POST /api/v1/policy/update-allowlist` takes `grantDuration` of `30d` or `365d` and sets the expiry from the time of the call. An unknown `(schemaId, fieldName)` in the request fails with HTTP 500.

## Decision rules

`POST /api/v1/policy/decide` evaluates each requested field independently:

| Check | Result |
|---|---|
| No metadata record | The whole request fails with HTTP 500 and `policy metadata not found for schema_id ... and field_name ...`. |
| Application not in `allowList` | Field added to `unauthorizedFields`. |
| Entry present but past `expiresAt` | Field added to `expiredFields`. |
| Authorized, `isOwner` is `false`, `accessControlType` is `restricted` | Field added to `consentRequiredFields`. |
| Anything else | Field is allowed without consent. |

Response flags are derived from those lists:

- `appAuthorized` is `true` when `unauthorizedFields` is empty.
- `appAccessExpired` is `true` when `expiredFields` is not empty.
- `appRequiresOwnerConsent` is `true` when `consentRequiredFields` is not empty.

Making a field `public` does not open it to every application. It only removes the consent requirement for applications already on the allow-list.

## Table

`policy_metadata` (PostgreSQL, created by `RUN_MIGRATION=true`):

| Column | Type | Notes |
|---|---|---|
| `id` | uuid | Primary key, generated. |
| `schema_id` | varchar(255) | Part of the unique index. |
| `field_name` | text | Part of the unique index. |
| `display_name`, `description` | text | Optional. |
| `source` | `source_enum` | Default `fallback`. |
| `is_owner` | boolean | Default `false`. |
| `access_control_type` | `access_control_type_enum` | Default `restricted`. |
| `allow_list` | jsonb | Default `{}`. |
| `owner` | `owner_enum` | `citizen`, or null when `is_owner` is true. |
| `created_at`, `updated_at` | timestamp | |

Source: [`internal/pdp/models`](../../internal/pdp/models).
