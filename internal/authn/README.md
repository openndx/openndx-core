# authn

JWT-based authentication. Validates tokens against a JWKS endpoint, extracts identity claims, and provides HTTP middleware that injects an `AuthContext` into each request's context.

## Configuration

```go
import "github.com/openndx/openndx-core/internal/authn"

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

A token is accepted when its signature, `iss`, `aud` and `exp` check out, it carries non-empty `sub` and `client_id` claims, and it carries every standard identity claim configured as required. How the token was obtained (`grant_type`) plays no part: a person signing in through a client and a system using client credentials get the same `Principal`, distinguished only by the roles, scopes and claims they carry. All validated claims are available through `Claims()` — custom claims need no configuration.

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
fmt.Println(authCtx.Claims().String("given_name"))     // any validated claim
```

The accessors are nil-safe, so they can be called on a nil `*AuthContext`:

```go
email := authn.GetAuthContext(r.Context()).Email()
```

The same accessors are available on `authCtx.Principal`; its claim storage is private.

## Standard identity claims

`email`, `ouId` and `ouHandle` are available through `Email()`, `OUID()` and `OUHandle()` on `AuthContext` and `Principal`. These accessors derive their values from the same claim set as `Claims()`. Values are whitespace-trimmed: `Email()` and `Claims().String("email")` always agree.

Each is **optional by default**: a token without the claim (or with a JSON-null or blank value) is accepted and the string accessor returns `""`. Configure a claim as required to reject such tokens instead:

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

## Unified claims

`Claims()` exposes every claim from the validated token: `sub`, `client_id`, `email`, `ouId`, `ouHandle`, `roles`, `scope`, registered JWT claims (`iss`, `aud`, `exp`, `nbf`, `iat`, `jti`), and custom claims without any declaration.

```go
claims := authn.GetAuthContext(r.Context()).Claims()
ownerID := claims.String(configuredClaim) // works for "sub", "email", "nic", etc.
groups := claims.Strings("groups")        // JSON array of strings
value, present := claims.Lookup("department")
// Inspect value's Go type when missing, null and wrong-type values matter.
```

The view and all typed accessors are nil-safe. `Claims` has no public map or mutators. `Lookup`, `Strings`, `Roles` and `Scopes` return detached copies, including nested maps and slices. Mutating a returned collection cannot change authenticated identity.

| Accessor | Behavior |
|----------|----------|
| `String(name)` | Returns only a JSON string; missing, null and other types return `""`. Numbers, arrays and objects are never coerced into identifiers. |
| `Strings(name)` | Returns a JSON array of strings, or nil for a missing, null or wrong-type claim. An empty array returns a non-nil empty slice. Only `scope` also accepts a whitespace-delimited string and treats null as an empty slice. |
| `Lookup(name)` | Returns `(value, present)` using JSON-decoded types: `string`, `float64`, `bool`, `[]any`, `map[string]any`, or nil. Missing is `(nil, false)`; JSON null is `(nil, true)`. |

Lookup names are exact top-level keys, with no dotted-path or nested expansion. Custom names are case-sensitive, including namespaced names such as `https://app.example.com/department`; `nic` and `NIC` can coexist. To preserve existing standard-claim binding, case variants of standard names are canonicalized at decode time (`EMAIL` → `email`, `SUB` → `sub`, `OUID` → `ouId`). Access them using the canonical names above. Names are never whitespace-trimmed. Duplicate keys and multiple spellings of the same standard claim reject the token, preventing ambiguous identity or grants.

Normalization preserves the previous identity behavior:

- `email`, `ouId`, `ouHandle` and custom string claims are whitespace-trimmed, as are the elements of top-level arrays consisting entirely of strings. Nested objects and mixed arrays are unchanged. Null and blank claims remain present for inspection.
- `sub`, `client_id` and registered JWT claims retain the values used during validation, without trimming. Required `sub` and `client_id` retain their existing non-empty check. `aud` keeps its original string or array shape; numeric dates keep their decoded shape.
- `roles` must be an array of strings (or absent/null). Each role is trimmed. A role such as `"Customs Officer"` remains one role; a free-text `roles` string rejects the token.
- `scope` keeps its existing parsing: a whitespace-delimited string or an array of strings. Array entries are not trimmed; the existing decoder accepts null array entries as empty strings. A blank/null scope or empty array yields an empty slice; missing scope yields nil. `Scopes()` and `Claims().Strings("scope")` agree.

Signature, issuer, audience, expiry, not-before, required-claim and standard-claim type checks still run before any claims view is returned.

## Migrating to Claims()

`ExtraClaims` and `ExtraClaims()` have been removed. This is an internal package, so all repository callers migrate together to `Claims()` without a deprecation period.

| Before | After |
|--------|-------|
| `authCtx.ExtraClaims().String(name)` | `authCtx.Claims().String(name)` for any claim, including `email` and `sub` |
| `authCtx.ExtraClaims()[name]` | `value, present := authCtx.Claims().Lookup(name)` |
| `authCtx.ExtraClaims().Strings(name)` | `authCtx.Claims().Strings(name)` for an array; explicitly use `strings.Fields(claims.String(name))` if a custom string deliberately encodes a list |
| `principal.Subject`, `principal.Email`, etc. | `principal.Subject()`, `principal.Email()`, etc., or the existing `AuthContext` methods |
| `Principal{Subject: ..., Scopes: ...}` in tests | `authntest.MustPrincipal(map[string]any{"sub": ..., "scope": ...})` from `internal/authn/authntest`; it signs and validates fixtures in memory |

Do not use `Lookup` presence alone to validate an owner ID: null and blank claims are now distinguishable from missing claims. For an identifier, use `String(name)` and reject `""`. A custom free-text claim is no longer split by `Strings`; only `scope` has that behavior.

`Principal` no longer exports mutable identity fields. It is created by `TokenExtractor`, and its accessors read the private claim set. The old principal JSON representation, including `extraClaims`, has been removed. Callers needing a serialized representation should build an explicit response from the accessors.

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
| `authCtx.User.Roles` / `authCtx.Client.Roles` (and `Scopes`, `ExtraClaims`)                            | `authCtx.Roles()` (and `Scopes()`, `Claims()`)                                                                                                                 |
| `authCtx.Type()`, `UserPrincipalType`, `ClientPrincipalType`                                           | Removed — branch on roles, scopes or claims instead                                                                                                                 |
| `authCtx.User.ExtraClaims.String("email")` (and `"ouId"`, `"ouHandle"`)                                | `authCtx.Email()` (and `OUID()`, `OUHandle()`)                                                                                                                      |
| `UserClaims` / `ClientClaims` (`ClaimSpec{Optional, Required}`), `WithUserClaims` / `WithClientClaims` | `email` / `ouId` / `ouHandle`: `Claims` (`ClaimsConfig`) / `WithClaims`. Any other claim: `Claims()`, no declaration; check required ones in your own middleware |
| `RolesClaim` / `WithRolesClaim`                                                                        | Removed — roles are always read from `roles`; map a differently-named roles claim to `roles` on the IdP side                                                        |

Behaviour changes the compiler will not catch:

- **`Subject()` now returns the JWT `sub` claim.** For user tokens it used to return the persisted user ID from `UserProfileService` (falling back to `sub`), and for client tokens it returned the `client_id`. Anything keyed by `Subject()` — stored records, audit entries, cache keys — must be checked.
- **`sub` is required on every token.** Client-credentials tokens without `sub` used to be accepted; they are now rejected.
- **`grant_type` is ignored.** Tokens without it, or with any value, are accepted. It is available through `Claims()` like any other claim.
- **Only `email`, `ouId` and `ouHandle` can be required.** A token missing any other claim you previously listed as `Required` is now accepted; move that check into your own middleware. Requirements now apply to every token, not just user or client tokens.
- **Use `Claims()` for generic lookups**, including `email`, `ouId` and `ouHandle`, which were excluded by the removed `ExtraClaims()` API.
- **Custom claims need no declaration.** Code that treats claim presence as "declared in config" must be checked.
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
