# authn

JWT-based authentication. Validates tokens against a JWKS endpoint, extracts identity claims, and provides HTTP middleware that injects an `AuthContext` into each request's context.

## Configuration

```go
import "github.com/OpenNSW/core/authn"

manager, err := authn.NewManager(authn.Config{
    JWKSURL:  "https://idp.example.com/.well-known/jwks.json",
    Issuer:   "https://idp.example.com",
    Audience: "my-api",

    // Optional: make standard identity claims required. See "Standard identity claims" below.
    Claims: authn.ClaimsConfig{
        OUID: authn.ClaimConfig{Required: true},
    },
})
```

`Config` carries `yaml` struct tags, so it can be embedded in a larger application config struct and populated generically (e.g. via `yaml.Unmarshal`):

```yaml
authn:
  jwksURL: https://idp.example.com/.well-known/jwks.json
  issuer: https://idp.example.com
  audience: my-api
  claims:
    ouId:
      required: true
```

A token is accepted when its signature, `iss`, `aud` and `exp` check out, it carries non-empty `sub` and `client_id` claims, and it carries every standard identity claim configured as required. How the token was obtained (`grant_type`) plays no part: a person signing in through a client and a system using client credentials get the same `Principal`, distinguished only by the roles, scopes and claims they carry. Every other claim in the token is passed through as `ExtraClaims` — nothing has to be declared.

> **Note:** `audience` is what binds a token to this service: any client the IdP issues a token for this audience is accepted. Which clients may call the service is authorization policy, not this package's concern — restrict it in authz or the application using `AuthContext.ClientID()`. If your IdP sets `aud` to the requesting client rather than the API, configure an audience / API resource for the service in the IdP.

## Middleware

```go
// 401 if no valid token
mux.Handle("/api/v1/tasks", manager.RequireAuthMiddleware()(handler))

// Proceeds with or without a token; handler checks presence itself
mux.Handle("/api/v1/public", manager.OptionalAuthMiddleware()(handler))
```

`authn` only authenticates. Anything your application does with the validated principal — provisioning a user record, loading a profile, attaching tenant data — goes in your own middleware, chained after `authn`'s:

```go
// in the application, not in authn
func ProvisionUser(users *user.Service) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            authCtx := authn.GetAuthContext(r.Context())
            u, err := users.GetOrCreate(r.Context(), authCtx.Subject(), authCtx.Email(), authCtx.OUID())
            if err != nil {
                http.Error(w, "user provisioning failed", http.StatusInternalServerError)
                return
            }
            next.ServeHTTP(w, r.WithContext(user.NewContext(r.Context(), u)))
        })
    }
}

// wrap all protected routes once, not per route
api := http.NewServeMux()
api.HandleFunc("POST /api/x", handlerX)

root := http.NewServeMux()
root.Handle("/api/", manager.RequireAuthMiddleware()(ProvisionUser(users)(api)))
```

The application owns that policy: who gets a record, what goes on the context, and whether a failure rejects the request.

## Reading identity in handlers

```go
authCtx := authn.GetAuthContext(r.Context())
if authCtx == nil {
    // no token present (only possible with OptionalAuthMiddleware)
    return
}

fmt.Println(authCtx.Subject())                        // JWT "sub"
fmt.Println(authCtx.ClientID())                       // JWT "client_id"
fmt.Println(authCtx.Roles())                          // role claims
fmt.Println(authCtx.Scopes())                         // scope claims
fmt.Println(authCtx.Email())                          // "email", "" if absent
fmt.Println(authCtx.OUID(), authCtx.OUHandle())       // "ouId", "ouHandle", "" if absent
fmt.Println(authCtx.ExtraClaims().String("given_name")) // any other claim — see "Extra claims"
```

The accessors are nil-safe, so they can be called on a nil `*AuthContext`:

```go
email := authn.GetAuthContext(r.Context()).Email()
```

The same values are available as fields on `authCtx.Principal`.

## Standard identity claims

`email`, `ouId` and `ouHandle` have dedicated fields: `Principal.Email`, `Principal.OUID` and `Principal.OUHandle`, also available as `Email()`, `OUID()` and `OUHandle()` on `AuthContext`. Values are whitespace-trimmed.

Each is **optional by default**: a token without the claim (or with a JSON-null or blank value) is accepted and the field is `""`. Configure a claim as required to reject such tokens instead:

```yaml
claims:
  email: {}              # optional (same as omitting it)
  ouId:
    required: true
  ouHandle:
    required: false
```

```go
authn.Config{ /* ... */ Claims: authn.ClaimsConfig{OUID: authn.ClaimConfig{Required: true}} }
// or authn.WithClaims(authn.ClaimsConfig{OUID: authn.ClaimConfig{Required: true}})
```

- **Required applies to every token**, including client-credentials tokens issued to systems, which typically carry no email or organization unit. Only require a claim every caller's token carries; check anything route-specific in your own middleware or handler.
- A present claim that is not a JSON string rejects the token.

## Extra claims (beyond the fixed schema)

`authn`'s fixed schema is deliberately small: `sub`, `client_id`, `scope`, `roles`, the standard identity claims `email`, `ouId` and `ouHandle`, plus the JWT registered claims `iss`, `aud`, `exp`, `nbf`, `iat`, `jti`. Every other claim in the token — `phone_number`, `grant_type`, an IdP-specific claim like `given_name`, a namespaced claim like `https://app.example.com/department` — is passed through in `ExtraClaims`, keyed by its exact name.

```go
authCtx := authn.GetAuthContext(r.Context())
name   := authCtx.ExtraClaims().String("given_name")  // "" if absent or not a string
groups := authCtx.ExtraClaims().Strings("groups")     // JSON array of strings, or a space-delimited string
dept   := authCtx.ExtraClaims()["department"]         // any other shape: index the map directly
```

`authn` does not decide which of these a request needs. If your application depends on one, check it in your own middleware or handler.

**Semantics:**
- Claims `authn` binds itself never appear in `ExtraClaims`, so each value has one home: read `Subject()`, not `ExtraClaims()["sub"]`. Case variants of fixed-schema names (`Roles`, `SUB`) are excluded too, because `encoding/json` matches struct tags case-insensitively and binds them to the fixed field.
- A claim whose value is JSON null or an empty/whitespace-only string is dropped, so it reads the same as a claim the token never carried. A token with no other claims has a nil `ExtraClaims`.
- Claim names are matched **exactly** — JWT claim names are case-sensitive (RFC 7519 §4). Nested lookups are not supported: a name is a top-level key, which is what lets namespaced names like `https://app.example.com/roles` work verbatim.
- Values are the claim's JSON-decoded Go representation, with one normalization: every JSON string a value *directly* carries is whitespace-trimmed — the value itself, and the elements of a top-level array of strings. Nested objects are never rewritten. (Fixed-schema claims such as `sub` are decoded separately and are not trimmed.)
- Use `ExtraClaims.String(name)` for a plain string value and `ExtraClaims.Strings(name)` for a list-shaped claim. `Strings` accepts a JSON array of strings *or* a single space-delimited string (the convention the `scope` claim uses), so it whitespace-splits free text — use `String` for anything that is not list-shaped. Both are nil-safe and return the zero value for absent/wrong-shape claims; they never panic or error.

## Principal fields

| Field         | Claim       | Required?                    | Description                                                                  |
|---------------|-------------|------------------------------|------------------------------------------------------------------------------|
| `Subject`     | `sub`       | Always                       | The IdP's ID for the caller                                                  |
| `ClientID`    | `client_id` | Always                       | The OAuth2 client the token was issued to                                    |
| `Roles`       | `roles`     | No                           | Must be an array of strings if present; nil if absent                        |
| `Scopes`      | `scope`     | No                           | Space-delimited; nil if absent                                               |
| `Email`       | `email`     | Configurable (default: no)   | `""` if absent — see "Standard identity claims" above                        |
| `OUID`        | `ouId`      | Configurable (default: no)   | `""` if absent                                                               |
| `OUHandle`    | `ouHandle`  | Configurable (default: no)   | `""` if absent                                                               |
| `ExtraClaims` | —           | —                            | Every other claim in the token — see "Extra claims" above                    |

Validation also requires `exp`, and checks `iss` and `aud` against `issuer` and `audience`.

Roles populate `AuthContext.Roles()`, which is what the [`authz`](https://pkg.go.dev/github.com/OpenNSW/core/authz) seam consumes. They are read from a top-level `roles` claim only; a role claim under another name, or nested (such as `realm_access.roles`), has to be mapped to `roles` on the IdP side. A `roles` claim of any shape other than an array of strings rejects the token, rather than silently reading as "no roles".

## Migrating from user and client principals

Earlier versions split every caller into a user or a client principal by the token's `grant_type` claim. There is now one `Principal`.

| Before                                                                                                 | After                                                                                                                                                               |
|--------------------------------------------------------------------------------------------------------|---------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `NewManager(userProfileSvc, cfg)`                                                                      | `NewManager(cfg)`                                                                                                                                                   |
| `Middleware(userProfileSvc, extractor)` / `RequireAuth(userProfileSvc, extractor)`                     | `Middleware(extractor)` / `RequireAuth(extractor)`                                                                                                                  |
| `UserProfileService.GetOrCreateUser`                                                                   | Your own middleware chained after `authn` — see "Middleware"                                                                                                        |
| `authCtx.User.ID` (persisted ID)                                                                       | Whatever your provisioning middleware puts on the context                                                                                                           |
| `authCtx.User.IDPUserID`                                                                               | `authCtx.Subject()`                                                                                                                                                 |
| `authCtx.Client.ClientID`                                                                              | `authCtx.ClientID()`                                                                                                                                                |
| `authCtx.User.Roles` / `authCtx.Client.Roles` (and `Scopes`, `ExtraClaims`)                            | `authCtx.Roles()` (and `Scopes()`, `ExtraClaims()`)                                                                                                                 |
| `authCtx.Type()`, `UserPrincipalType`, `ClientPrincipalType`                                           | Removed — branch on roles, scopes or claims instead                                                                                                                 |
| `authCtx.User.ExtraClaims.String("email")` (and `"ouId"`, `"ouHandle"`)                                | `authCtx.Email()` (and `OUID()`, `OUHandle()`)                                                                                                                      |
| `UserClaims` / `ClientClaims` (`ClaimSpec{Optional, Required}`), `WithUserClaims` / `WithClientClaims` | `email` / `ouId` / `ouHandle`: `Claims` (`ClaimsConfig`) / `WithClaims`. Any other claim: `ExtraClaims`, no declaration; check required ones in your own middleware |
| `RolesClaim` / `WithRolesClaim`                                                                        | Removed — roles are always read from `roles`; map a differently-named roles claim to `roles` on the IdP side                                                        |

Behaviour changes the compiler will not catch:

- **`Subject()` now returns the JWT `sub` claim.** For user tokens it used to return the persisted user ID from `UserProfileService` (falling back to `sub`), and for client tokens it returned the `client_id`. Anything keyed by `Subject()` — stored records, audit entries, cache keys — must be checked.
- **`sub` is required on every token.** Client-credentials tokens without `sub` used to be accepted; they are now rejected.
- **`grant_type` is ignored.** Tokens without it, or with any value, are accepted. It now passes through in `ExtraClaims` like any other claim.
- **Only `email`, `ouId` and `ouHandle` can be required.** A token missing any other claim you previously listed as `Required` is now accepted; move that check into your own middleware. Requirements now apply to every token, not just user or client tokens.
- **`email`, `ouId` and `ouHandle` are no longer in `ExtraClaims`**: `ExtraClaims().String("email")` now returns `""`; use `Email()`.
- **`ExtraClaims` now holds every other claim**, not just declared ones. Code that treated "present in `ExtraClaims`" as "declared in config" must be checked.
- **A leftover `rolesClaim` key in a yaml config is silently ignored**, so roles are read from `roles` instead. If your IdP emits roles under another name, every caller now has no roles until the IdP maps them to `roles`.
- **The JSON shape of `AuthContext` changed**, for anything that serializes it.

## Health check

```go
if err := manager.Health(); err != nil {
    // auth components are not initialized
    // (this does NOT check JWKS reachability)
}
```

## Authorization

`authn` handles identity only. For scope enforcement use the [`authz`](https://pkg.go.dev/github.com/OpenNSW/core/authz) package — `*AuthContext` satisfies `authz.Principal` directly.
