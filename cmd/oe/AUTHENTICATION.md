# OE authentication

How the Orchestration Engine authenticates data consumers on `POST /public/graphql`. The other routes (`/health`, `/sdl*`) are not authenticated. The overview is in the [README](README.md).

## What a consumer must present

An access token in the `Authorization: Bearer <token>` header, issued by the identity provider to a machine client. In the bundled stack that is a ThunderID `client_credentials` token; see the example in [dev/README.md](../../dev/README.md).

The token must carry:

| Claim | Requirement |
|---|---|
| `exp` | Present and in the future. If `nbf` is present it must not be in the future; `iat`, if present, must be a number. |
| `client_id` | Present. It becomes the application's identity (`applicationId`) for the PDP and Consent Engine. |
| `sub` or `azp` | At least one present. |
| `iss` | Must equal `jwt.expectedIssuer`, when that is configured. |
| `aud` | Must contain one of `jwt.validAudiences`, when that is configured. |

A token that fails any check gets `401 Unauthorized: invalid or expired token`. The reason is logged, not returned.

## Signature verification: `trustUpstream`

| `trustUpstream` | Behavior |
|---|---|
| `false` | The signature is verified against the keys at `jwt.jwksUrl`. `jwt.jwksUrl` is required and the service exits at startup if it is missing or the first fetch fails. **Use this in production.** |
| `true` | The token is parsed **without verifying its signature**. The claims above are still required and checked. Use only when something in front of the OE has already verified the token, or for local development. |

The sample configs ([config.example.json](config.example.json), [config.docker.json](config.docker.json)) set `trustUpstream: true`. With `true` and a `jwt.jwksUrl`, the OE still loads the keys, but a failure to load them is only a warning.

There is no `environment`-based bypass. Setting `environment` to `development` does not skip any token check.

## JWKS handling

With a JWKS URL configured, the OE fetches the key set at startup and refreshes it in the background, so key rotation needs no restart. Keys that carry `x5c` certificate chains are dropped before parsing. The response is capped at 1 MB.

## JWKS URL checks

The URL must be `http` or `https` and have a host. A non-HTTPS URL logs a warning. When `environment` is `production` or `staging`, the OE also refuses a URL whose host is a private, loopback or link-local IP address, or contains `169.254.169.254`. These checks apply only to this one fetch, and only to IP literals: a hostname such as `localhost` is not resolved or blocked. With any other `environment` value the checks are off and a warning is logged.

## Production checklist

- `trustUpstream: false`, with `jwt.jwksUrl`, `jwt.expectedIssuer` and `jwt.validAudiences` set.
- `environment: "production"`.
- Terminate TLS in front of the OE, and expose only `POST /public/graphql`.
