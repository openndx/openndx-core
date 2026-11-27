# Privacy and Data Protection

This statement covers the personal data that the OpenNDX software handles, how its design protects that data, and who is responsible for it.

It applies to the components released from this repository:
- the Orchestration Engine;
- the Policy Decision Point;
- the Consent Engine;
- the Portal Backend;
- the Consent Portal;
- the `ondx` CLI;
- the Helm chart.

The Admin and Member portals are unreleased prototypes and aren't covered.

## Who is responsible

OpenNDX is self-hosted software.
- **The operator is responsible.** The operator is the government agency or other organization that deploys an exchange. It runs OpenNDX on its own infrastructure, decides which providers, fields and applications take part, and is the data controller under the law that applies to it.
- **Providers keep responsibility for their records.** Each data provider stays responsible for the records it holds.

Lanka Software Foundation (LSF) develops OpenNDX.
- **LSF doesn't operate exchanges that others deploy.** It receives no data from them.
- **OpenNDX sends nothing to LSF.** The software sends no usage data, telemetry or crash reports to LSF or to anyone else. Audit records go only to the OpenTelemetry endpoint the operator configures, and the Consent Engine's metrics only to the operator's own monitoring.

## Personal data OpenNDX handles

| Where | Personal data | Purpose | How long it is kept |
|---|---|---|---|
| Orchestration Engine | The fields a consumer requests, fetched from providers, such as a person's name or address | To answer the request | Not stored. The Orchestration Engine has no database. It doesn't log or audit field values or query text. |
| Consent Engine database | Consent records. Each one holds: <ul><li>the data owner's identifier;</li><li>the requesting application's ID and name;</li><li>the fields requested;</li><li>the status and timestamps;</li><li>the grant duration and expiry;</li><li>who last updated the record.</li></ul> | To ask for, record and enforce the owner's consent | Kept after a consent expires, is rejected or is revoked, as the history of the owner's decisions |
| Portal Backend database | Each member organization's name, contact email address and phone number | To register members and manage their schemas and applications | Until the operator changes or removes them |
| Policy Decision Point database | No personal data. It holds field policies and access grants, including the legal basis recorded for a consent bypass. | To decide which application may read which field | Until the operator changes them |
| Identity provider (ThunderID in the default setup, or the deployment's own identity provider) | Accounts of data owners who sign in to the Consent Portal, and of operator and member staff | Sign-in | As configured in the identity provider, which is a separate project |
| Audit records | <ul><li>The event type, status, actor and target.</li><li>The application ID and the fields requested.</li><li>For a consent check, the data owner's identifier and the consent ID.</li><li>When a provider returns errors, the first few error messages, exactly as the provider sent them. Providers shouldn't put personal data in error messages.</li></ul> | Accountability for each exchange and each management action | Exported over OpenTelemetry (OTLP) to the endpoint the operator configures, and kept as long as the operator's system keeps them. If no endpoint is configured, they are dropped. |
| Consent Engine metrics | No personal data. They count and time requests per route template, such as `/api/v1/consents/:id`, without IDs. | Monitoring | Served at the Consent Engine's `/metrics` endpoint, or pushed to an endpoint the operator configures, and kept as long as the operator's monitoring system keeps them |
| `ondx` CLI | The operator's own sign-in tokens, on the operator's computer | To call the management API | In `~/.openndx`, in a file created so that only that user can read it |

The data owner's identifier is the value of the identity claim that the operator sets in the Consent Engine's
`idp.subjectClaim`, such as a national identity number.

## How the design protects personal data

- **No central copy.**
  - Records stay with the providers that hold them.
  - OpenNDX fetches only the fields a request needs and returns them to the consumer.
- **Field-level minimization.**
  - Every field has a policy.
  - An application receives only the fields it holds an unexpired grant for, and only with the owner's consent where the field requires it.
  - See the [policy model](cmd/pdp/POLICY_MODEL.md).
- **Consent the owner controls.**
  - The Consent Portal shows the owner which application is asking, which fields it wants and for how long.
  - Only a person whose identity claim matches the record's owner can approve, reject or revoke a consent.
  - Approved consent expires. Consents that the Orchestration Engine creates grant one hour of access.
  - A revocation applies from the next request.
  - See the [consent lifecycle](cmd/ce/CONSENT_LIFECYCLE.md).
- **Exceptions need a legal basis.**
  - An administrator can let an application read a field without consent only through a consent bypass grant. The grant records its legal basis and expiry.
  - The Orchestration Engine flags each such read in its audit events.
  - See [consent bypass](cmd/pdp/POLICY_MODEL.md#consent-bypass).
- **Authenticated access.**
  - Consumers, members and data owners present JSON Web Tokens, which are verified against the identity provider's keys.
  - The management API also checks OAuth scopes.
  - The Policy Decision Point's API and the Consent Engine's internal API have no authentication of their own. The network policies in the [Helm chart](deploy/helm/openndx/README.md) let only the services that call them reach them.
- **Separation.**
  - Each service has its own database.
  - Secrets come from environment variables or files, not from code.
- **Secure development.** Vulnerabilities are reported privately and fixed as described in [SECURITY.md](SECURITY.md).

## Applicable laws

The operator determines which laws apply to its exchange and is responsible for meeting them. That includes any data
protection impact assessment.

OpenNDX is designed to support operators' obligations under the Sri Lanka Personal Data Protection Act, No. 9 of 2022,
and under the EU General Data Protection Regulation where it applies. In particular it supports:
- **lawful processing:** owner consent, or a recorded legal basis, for each consent-protected field;
- **purpose limitation and data minimization:** grants per application and per field;
- **accountability:** audit records of each exchange and each management action;
- **security of processing:** the controls described above.

## Requests from data owners

Requests about a person's data, such as access, correction or deletion, go to the organization that runs the exchange
or to the provider that holds the record. OpenNDX helps operators answer them:
- **Data owners can manage their own consents.** In the Consent Portal they can see the consents they have given and revoke them.
- **Operators can find an owner's records.** Consent records are in the Consent Engine database, and the related audit records carry the same identifier.
- **Corrections happen at the provider.** OpenNDX holds no copy of the providers' records.

## Deletion and retention

OpenNDX doesn't delete personal data on a schedule or through its APIs.
- **Consent records are kept as history** after they expire, are rejected or are revoked. An operator who needs to delete them, for example to answer an erasure request, deletes them in the Consent Engine database.
- **Member contact details** can be changed through the management API and deleted in the Portal Backend database.
- **Audit records are kept as long as the operator's OpenTelemetry backend keeps them.** Set retention there.

## Contact

| Topic | Contact |
|---|---|
| A deployment, or your own data in it | The organization that runs the exchange |
| How OpenNDX is designed to protect privacy | contact@opensource.lk |
| A security vulnerability | Follow [SECURITY.md](SECURITY.md). Don't open a public issue. |
