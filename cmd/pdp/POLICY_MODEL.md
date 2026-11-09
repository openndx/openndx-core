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
| `policyClass`                | class name             | The [policy class](#policy-classes) that decides access to the field. Required. An unknown class is rejected with HTTP 400.              |
| `policyParams`               | object                 | Parameters for the policy class, passed to it as `field.params`. Optional, defaults to `{}`. The built-in classes ignore it.             |
| `owner`                      | `citizen`, optional    | Whose consent is asked for the field. Shown on the consent screen.                                                                       |
| `allowList`                  | map                    | Applications allowed to read the field, each with an expiry. Managed through the allow-list endpoints, never set when creating metadata. |

`POST /api/v1/policy/metadata` is a sync for one schema. It upserts the records you send and deletes the schema's records that you omit. If any record names an unknown policy class, nothing is written.

## Policy classes

| Class               | App must be on the allow-list? | Owner consent? |
|---------------------|--------------------------------|----------------|
| `allowlist`         | Yes                            | No             |
| `allowlist_consent` | Yes                            | Yes            |
| `consent`           | No                             | Yes            |

`consent` lets any authenticated application ask the owner, for data the owner alone should control. An admin doesn't have to put the application on the allow-list first, and an expired allow-list entry makes no difference.

Each class is a Rego package `openndx.classes.<name>` under [`internal/pdp/engine/policies/classes`](../../internal/pdp/engine/policies/classes). The PDP accepts exactly the classes it has loaded.

## Allow-list

The allow-list is a JSON map stored on each record. The key is the application's OIDC `client_id`; the value holds `expires_at` and `updated_at`.

`POST /api/v1/policy/update-allowlist` takes `grantDuration` of `30d` or `365d` and sets the expiry from the time of the call. An unknown `(schemaId, fieldName)` in the request fails with HTTP 500.

## Decision rules

`POST /api/v1/policy/decide` evaluates each requested field independently. The rules are Rego policies in [`internal/pdp/engine/policies`](../../internal/pdp/engine/policies), embedded in the PDP at build time, compiled at startup and evaluated with the embedded OPA library. The PDP does not start if they fail to compile.

Each field is evaluated by its `policyClass`. `allowlist` and `allowlist_consent` deny an application that is not on the allow-list, report an expired entry, and otherwise allow the field (`allowlist`) or require owner consent (`allowlist_consent`). `consent` skips the allow-list and always requires owner consent. An unknown class or a failed evaluation denies the field, which then appears in `unauthorizedFields`.

| Check                                                | Result                                                                                                      |
|------------------------------------------------------|-------------------------------------------------------------------------------------------------------------|
| No metadata record                                   | The whole request fails with HTTP 500 and `policy metadata not found for schema_id ... and field_name ...`. |
| `policyClass` is `consent`                           | Field added to `consentRequiredFields`, whatever the allow-list says.                                       |
| Application not in `allowList`                       | Field added to `unauthorizedFields`.                                                                        |
| Entry present but past `expiresAt`                   | Field added to `expiredFields`.                                                                             |
| Authorized, `policyClass` is `allowlist_consent`     | Field added to `consentRequiredFields`.                                                                     |
| Authorized, `policyClass` is `allowlist`             | Field is allowed without consent.                                                                           |

### Field outcomes

`fields` in the response holds one entry per requested field, in request order:

```json
{"schemaId": "drp", "fieldName": "person.fullName", "outcome": "deny", "reason": "not_allowlisted"}
```

| Property      | Notes                                                                                                                         |
|---------------|-------------------------------------------------------------------------------------------------------------------------------|
| `outcome`     | One of `allow`, `deny`, `expired`, `consent_required`. The set is fixed: a class that returns anything else denies the field. |
| `reason`      | Optional. Why the class chose the outcome, for example `not_allowlisted`, `grant_expired` or `unknown_policy_class`.          |
| `obligations` | Optional. Extra actions the class asks the Orchestration Engine to take, for example `audit.consent_bypassed`.                |

A class adds obligations by returning them, as a list or a set of strings, next to the outcome. A malformed `obligations` value denies the field, since the Orchestration Engine could not act on it. The Orchestration Engine records every field's outcome, reason and obligations in its `POLICY_CHECK` audit event, under `responseMetadata.fields`. The built-in classes return no obligations.

The lists group the fields by outcome: `deny` goes to `unauthorizedFields`, `expired` to `expiredFields` and `consent_required` to `consentRequiredFields`, each with the field's display metadata. Response flags are derived from those lists:

- `appAuthorized` is `true` when `unauthorizedFields` is empty.
- `appAccessExpired` is `true` when `expiredFields` is not empty.
- `appRequiresOwnerConsent` is `true` when `consentRequiredFields` is not empty.

No built-in class lets every application read a field without consent. `allowlist` only removes the consent requirement for applications already on the allow-list, and `consent` opens a field to every application only once its owner consents.

## Table

`policy_metadata` (PostgreSQL, created by [schema.sql](schema.sql)):

| Column                        | Type          | Notes                                         |
|-------------------------------|---------------|-----------------------------------------------|
| `id`                          | uuid          | Primary key, generated.                       |
| `schema_id`                   | varchar(255)  | Part of the unique index.                     |
| `field_name`                  | text          | Part of the unique index.                     |
| `display_name`, `description` | text          | Optional.                                     |
| `source`                      | `source_enum` | Default `fallback`.                           |
| `policy_class`                | text          | Required. Checked against the loaded classes. |
| `policy_params`               | jsonb         | Default `{}`.                                 |
| `allow_list`                  | jsonb         | Default `{}`.                                 |
| `owner`                       | `owner_enum`  | `citizen`, or null.                           |
| `created_at`, `updated_at`    | timestamp     |                                               |

Applying `schema.sql` to a v0.5 database converts `access_control_type` and `is_owner` to `policy_class` and drops them: `restricted` fields with `is_owner` false become `allowlist_consent`, and every other field becomes `allowlist`.

Source: [`internal/pdp/models`](../../internal/pdp/models).
