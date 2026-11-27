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

A token is accepted when it passes the [token checks](#token-checks). How the token was obtained (`grant_type`) plays no part: a person signing in through a client and a system using client credentials get the same `Principal`, distinguished only by the roles, scopes and claims they carry. All validated claims are available through `Claims()` — custom claims need no configuration.

> **Note:** `audience` is what binds a token to this service: any client the IdP issues a token for this audience is accepted. Which clients may call the service is authorization policy, not this package's concern — restrict it in authz or the application using `AuthContext.ClientID()`. If your IdP sets `aud` to the requesting client rather than the API, configure an audience / API resource for the service in the IdP.

## Token checks

A request sends the token as `Authorization: Bearer <token>`. The token must pass these checks:

| Check | Requirement |
|---|---|
| Signature | `RS256`, `RS384` or `RS512`, by a key in the JWKS at `jwksURL`. The token's `kid` header names the key. |
| `iss` | Equals `issuer`. |
| `aud` | Contains `audience`. |
| `exp` | Present and in the future, with 30 seconds of leeway. `nbf`, when present, must not be in the future, with the same leeway. |
| `sub`, `client_id` | Present and not empty. |
| `email`, `ouId`, `ouHandle` | Present and not blank, when [configured as required](#standard-identity-claims). |

A request without a valid token gets `401` with a JSON body: `{"error":"unauthorized","message":"authentication required"}` when it has no token, and `{"error":"unauthorized","message":"invalid authentication token"}` when the token fails a check. The reason is logged at debug level, not returned.

The keys are fetched from `jwksURL` on the first request, not at startup, so a service starts even while the identity provider is down. They are cached for 5 minutes, and a token whose `kid` isn't in the cache triggers one refresh, so key rotation needs no restart. `insecureSkipTLSVerify: true` skips TLS certificate checks on that fetch; use it only in local development.

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

Lookup names are exact top-level keys, with no dotted-path or nested expansion. Custom names are case-sensitive, including namespaced names such as `https://app.example.com/department`; `nic` and `NIC` can coexist. Case variants of standard names are canonicalized at decode time (`EMAIL` → `email`, `SUB` → `sub`, `OUID` → `ouId`). Access them using the canonical names above. Names are never whitespace-trimmed. Duplicate keys and multiple spellings of the same standard claim reject the token, preventing ambiguous identity or grants.

Values are normalized as follows:

- `email`, `ouId`, `ouHandle` and custom string claims are whitespace-trimmed, as are the elements of top-level arrays consisting entirely of strings. Nested objects and mixed arrays are unchanged. Null and blank claims remain present for inspection.
- `sub`, `client_id` and registered JWT claims retain the values used during validation, without trimming. `sub` and `client_id` must be non-empty. `aud` keeps its original string or array shape; numeric dates keep their decoded shape.
- `roles` must be an array of strings (or absent/null). Each role is trimmed. A role such as `"Customs Officer"` remains one role; a free-text `roles` string rejects the token.
- `scope` is a whitespace-delimited string or an array of strings. Array entries are not trimmed, and null array entries are read as empty strings. A blank/null scope or empty array yields an empty slice; missing scope yields nil. `Scopes()` and `Claims().Strings("scope")` agree.

Signature, issuer, audience, expiry, not-before, required-claim and standard-claim type checks run before any claims view is returned.

## Health check

```go
if err := manager.Health(); err != nil {
    // auth components are not initialized
    // (this does NOT check JWKS reachability)
}
```

## Authorization

`authn` handles identity only. For scope enforcement use the [`authz`](https://pkg.go.dev/github.com/OpenNSW/core/authz) package — `*AuthContext` satisfies `authz.Principal` directly.
