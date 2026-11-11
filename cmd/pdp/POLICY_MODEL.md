# PDP policy model

How the Policy Decision Point represents field policy and turns it into a decision. API shapes are in [openapi.yaml](openapi.yaml); the service overview is in the [README](README.md).

## Policy metadata

One record describes one field of one schema. `(schemaId, fieldName)` is unique.

| Field                        | Values                 | Meaning                                                                                                                                  |
|------------------------------|------------------------|------------------------------------------------------------------------------------------------------------------------------------------|
| `schemaId`                   | string                 | The provider schema the field belongs to.                                                                                                |
| `fieldName`                  | string                 | The field path, for example `person.fullName`. The Orchestration Engine sends its `@sourceInfo` `providerField` here.                    |
| `displayName`, `description` | optional text          | Shown to the data owner on the consent screen.                                                                                           |
| `source`                     | `primary`, `fallback`  | Whether the provider is the primary or a fallback source for the field.                                                                  |
| `requiresAllowlist`          | boolean                | The application needs an unexpired [access grant](#access-grants) for the field. Required.                                               |
| `requiresConsent`            | boolean                | The owner must consent, unless the application holds a [consent bypass](#consent-bypass) grant. Required.                                |
| `owner`                      | `citizen`, optional    | Whose consent is asked for the field. Shown on the consent screen.                                                                       |

`POST /api/v1/policy/metadata` is a sync for one schema. It upserts the records you send and deletes the schema's records that you omit, together with their [access grants](#access-grants). Records it updates keep their grants. If any record leaves out `requiresAllowlist` or `requiresConsent`, nothing is written and the request fails with HTTP 400. A missing flag is never read as `false`, which would open the field.

## Field policy

Two flags describe who may read a field:

|                               | **`requiresConsent: true`**           | **`requiresConsent: false`**   |
|-------------------------------|---------------------------------------|--------------------------------|
| **`requiresAllowlist: true`** | Allow-listed applications, with owner consent | Allow-listed applications, no consent |
| **`requiresAllowlist: false`**| Any authenticated application, with owner consent | Any authenticated application (public) |

A field that requires consent but not the allow-list lets any authenticated application ask the owner, for data the owner alone should control. An admin doesn't have to put the application on the allow-list first, and an expired allow-list entry makes no difference.

A field that requires neither is public. Any authenticated application reads it without an allow-list entry or owner consent. The request still needs an application ID, so every access is attributed to an application in the audit trail.

## Access grants

An access grant gives one application access to one field until it expires. A grant is either **plain** or a **consent bypass** grant. A field's allow-list is the set of applications that hold an `active` plain grant for it. The application is identified by its OIDC `client_id`.

| Property        | Meaning                                                                                              |
|-----------------|------------------------------------------------------------------------------------------------------|
| `id`            | Grant ID, used to revoke it.                                                                         |
| `schemaId`, `fieldName` | The field. It must have policy metadata.                                                     |
| `applicationId` | The application's OIDC `client_id`.                                                                  |
| `expiresAt`     | After this time the field is `expired` for the application until the grant is renewed.              |
| `status`        | `active` or `revoked`. A field has at most one `active` plain grant and one `active` bypass grant per application. |
| `consentBypass` | `true` for a consent bypass grant.                                                                   |
| `legalBasis`    | Set for consent bypass grants: why the application may skip consent.                                 |
| `createdAt`, `updatedAt` | `updatedAt` changes when the grant is renewed or revoked.                                   |

| Endpoint                                   | Behavior                                                                                                                                                                               |
|--------------------------------------------|----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `POST /api/v1/policy/grants`               | Takes `applicationId`, `records` and `grantDuration` of `30d` or `365d`, and sets the expiry from the time of the call. Renews the application's active grant of the same kind for a field, or creates one. With `consentBypass: true` and a `legalBasis` it grants consent bypass; see [Consent bypass](#consent-bypass). A field without metadata fails with HTTP 404 and nothing is written. |
| `GET /api/v1/policy/grants`                | Lists grants, filtered by any of `applicationId`, `schemaId`, `status` and `consentBypass`.                                                                                            |
| `GET /api/v1/policy/grants/{id}`           | Reads one grant.                                                                                                                                                                       |
| `POST /api/v1/policy/grants/{id}/revoke`   | Sets an active grant's status to `revoked`. The grant is kept as a record. Revoking a grant that is not active fails with HTTP 404. Granting the field again creates a new grant.       |

Deleting a field's policy metadata deletes its grants. An expired grant stays `active`: the decision reports the field as `expired`, which tells the application to renew access rather than ask for it.

### Consent bypass

A consent bypass grant lets one application, such as a legally authorized police system, read a field that requires the owner's consent without asking. It also counts as the application's allow-list entry for the field.

- **Grant.** `POST /api/v1/policy/grants` with `consentBypass: true` and a non-empty `legalBasis` creates an active bypass grant per field, or renews the application's existing one with the new expiry and legal basis. A field that doesn't require consent fails the request with HTTP 400, and nothing is written. A `legalBasis` without `consentBypass` is rejected too.
- **Kinds stay apart.** Plain and bypass grants are renewed separately, so re-granting an application's plain access never creates or extends a bypass.
- **Revoke.** `POST /api/v1/policy/grants/{id}/revoke` takes effect immediately.

The PDP doesn't authenticate callers or decide who may grant a bypass. That is a management decision: Portal Backend exposes it to administrators and records who granted it. The PDP API must be reachable only by trusted services.

## Decision rules

`POST /api/v1/policy/decide` evaluates each requested field independently, from the field's two flags and the application's active grant for it, if any. When the application holds both kinds of grant, the bypass grant is used while it is unexpired, otherwise the plain one. The rules are in [`internal/pdp/decision`](../../internal/pdp/decision) and apply in order:

| # | Check                                                              | Outcome            | Reason             |
|---|--------------------------------------------------------------------|--------------------|--------------------|
| 1 | No application ID                                                  | `deny`             | `no_application`   |
| 2 | `requiresAllowlist` and no active grant                            | `deny`             | `not_allowlisted`  |
| 3 | `requiresAllowlist` and the grant is past its `expiresAt`          | `expired`          | `grant_expired`    |
| 4 | `requiresConsent` and an unexpired consent bypass grant            | `allow`, with the `audit.consent_bypassed` obligation | `consent_bypassed` |
| 5 | `requiresConsent`                                                  | `consent_required` |                    |
| 6 | Otherwise                                                          | `allow`            |                    |

A grant is valid up to and including its `expiresAt`. A field that doesn't require the allow-list ignores the grant's expiry: an expired grant neither denies it nor marks it `expired`.

A requested field without a metadata record fails the whole request with HTTP 500 and `policy metadata not found for schema_id ... and field_name ...`.

### Field outcomes

`fields` in the response holds one entry per requested field, in request order:

```json
{"schemaId": "drp", "fieldName": "person.fullName", "outcome": "deny", "reason": "not_allowlisted"}
```

| Property      | Notes                                                                                                                         |
|---------------|-------------------------------------------------------------------------------------------------------------------------------|
| `outcome`     | One of `allow`, `deny`, `expired`, `consent_required`.                                                                        |
| `reason`      | Optional. Why the PDP chose the outcome: one of the reasons in [Decision rules](#decision-rules).                             |
| `obligations` | Optional. Extra actions the Orchestration Engine must take. The only one is `audit.consent_bypassed`.                         |

The Orchestration Engine records every field's outcome, reason and obligations in its `POLICY_CHECK` audit event, under `responseMetadata.fields`. The `audit.consent_bypassed` obligation is set when a bypass grant lets the application skip consent.

The lists group the fields by outcome: `deny` goes to `unauthorizedFields`, `expired` to `expiredFields` and `consent_required` to `consentRequiredFields`, each with the field's display metadata. Response flags are derived from those lists:

- `appAuthorized` is `true` when `unauthorizedFields` is empty.
- `appAccessExpired` is `true` when `expiredFields` is not empty.
- `appRequiresOwnerConsent` is `true` when `consentRequiredFields` is not empty.

Only a field with both flags `false` lets every authenticated application read it without an allow-list entry or consent. Dropping `requiresConsent` alone only removes consent for applications already on the allow-list, and dropping `requiresAllowlist` alone opens a field to every application only once its owner consents.

## Tables

`policy_metadata` (PostgreSQL, created by [schema.sql](schema.sql)):

| Column                        | Type          | Notes                                         |
|-------------------------------|---------------|-----------------------------------------------|
| `id`                          | uuid          | Primary key, generated.                       |
| `schema_id`                   | varchar(255)  | Part of the unique index.                     |
| `field_name`                  | text          | Part of the unique index.                     |
| `display_name`, `description` | text          | Optional.                                     |
| `source`                      | `source_enum` | Default `fallback`.                           |
| `requires_allowlist`          | boolean       | Required, no default.                         |
| `requires_consent`            | boolean       | Required, no default.                         |
| `owner`                       | `owner_enum`  | `citizen`, or null.                           |
| `created_at`, `updated_at`    | timestamp     |                                               |

`access_grants`:

| Column                          | Type         | Notes                                                                                     |
|---------------------------------|--------------|-------------------------------------------------------------------------------------------|
| `id`                            | uuid         | Primary key, generated.                                                                   |
| `schema_id`, `field_name`       | varchar(255), text | Foreign key to `policy_metadata`, `ON DELETE CASCADE`.                              |
| `application_id`                | text         | OIDC `client_id`.                                                                         |
| `expires_at`                    | timestamptz  | Required.                                                                                 |
| `status`                        | text         | `active` (default) or `revoked`, enforced by `access_grants_status_check`.               |
| `consent_bypass`                | boolean      | Default `false`.                                                                          |
| `legal_basis`                   | text         | Required for bypass grants, by `access_grants_bypass_check`.                             |
| `created_at`, `updated_at`      | timestamptz  |                                                                                           |

A partial unique index on `(schema_id, field_name, application_id, consent_bypass) WHERE status = 'active'` keeps one active grant of each kind per application and field.

Source: [`internal/pdp/models`](../../internal/pdp/models).
