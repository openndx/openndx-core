# OE authentication

How the Orchestration Engine authenticates data consumers on `POST /public/graphql`. The only other routes, `/health`, `/health/liveness` and `/health/readiness`, are not authenticated. The overview is in the [README](README.md).

The OE uses the shared [`internal/authn`](../../internal/authn) package, like Portal Backend. Every token's signature is verified; there is no setting that skips verification.

## What a consumer must present

An access token in the `Authorization: Bearer <token>` header, issued by the identity provider to a machine client for the OE's audience. In the bundled stack that is a ThunderID `client_credentials` token requested with `resource=http://api.openndx.local`; see the example in [dev/README.md](../../dev/README.md).

The token must:

| Check       | Requirement                                                                                                                 |
|-------------|-----------------------------------------------------------------------------------------------------------------------------|
| Signature   | `RS256`, `RS384` or `RS512`, signed by a key in the JWKS at `idp.jwksURL`, with a `kid` header that names the key.          |
| `iss`       | Equals `idp.issuer`.                                                                                                        |
| `aud`       | Contains `idp.audience`.                                                                                                    |
| `exp`       | Present and in the future, with 30 seconds of leeway. `nbf`, when present, must not be in the future, with the same leeway. |
| `sub`       | Present.                                                                                                                    |
| `client_id` | Present. It becomes the application's identity (`applicationId`) for the PDP and Consent Engine.                            |

A request without a valid token gets `401` with a JSON body, for example `{"error":"unauthorized","message":"invalid authentication token"}`. The reason is logged at debug level, not returned.

## Configuration

The `idp` section of [config.yaml](CONFIGURATION.md#configyaml):

| Key                         | Default | Notes                                                                               |
|-----------------------------|---------|-------------------------------------------------------------------------------------|
| `idp.jwksURL`               |         | **Required.** Where the signing keys are fetched. Must be an `http` or `https` URL. |
| `idp.issuer`                |         | **Required.** The expected `iss`. Must be an `http` or `https` URL.                 |
| `idp.audience`              |         | **Required.** The expected `aud`, the OE's resource server identifier.              |
| `idp.insecureSkipTLSVerify` | `false` | `true` skips TLS certificate checks when fetching the JWKS. Local development only. |

The service exits at startup if `jwksURL`, `issuer` or `audience` is missing or invalid. `issuer` must equal the `iss` the IdP stamps into tokens. ThunderID's is its public URL (`http://localhost:8090` locally), not `/oauth2/token`. In Docker Compose, [config.docker.yaml](config.docker.yaml) reads the issuer from `IDP_PUBLIC_URL`, ThunderID's public URL, and the JWKS URL from `IDP_JWKS_URL`, its compose-network address.

## JWKS handling

The keys are fetched on the first request, not at startup, so the OE starts even when the IdP is not yet up. They are cached for 5 minutes. A token whose `kid` is not in the cached set triggers one refresh, so key rotation needs no restart.

## Production checklist

- Set `idp.jwksURL` and `idp.issuer` to the production IdP over `https`, and `idp.audience` to the OE's resource server.
- Leave `idp.insecureSkipTLSVerify` unset.
- Terminate TLS in front of the OE, and expose only `POST /public/graphql`.
