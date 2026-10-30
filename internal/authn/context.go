// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Lanka Software Foundation

package authn

import (
	"context"
)

// AuthContext is the transient authentication context injected into each request
// by the auth middleware. It carries the Principal resolved from the request's
// token.
type AuthContext struct {
	Principal Principal
}

// ContextKey is a custom type for context keys to avoid collisions.
type ContextKey string

const AuthContextKey ContextKey = "authContext"

// GetAuthContext extracts the AuthContext from a request context.
// Returns nil if no auth context is available (for example: public route,
// missing auth header, or middleware not applied).
//
// Usage in handlers:
//
//	authCtx := auth.GetAuthContext(r.Context())
//	if authCtx == nil {
//	    // Handle unauthorized request
//	}
//	subject := authCtx.Subject()
func GetAuthContext(ctx context.Context) *AuthContext {
	authCtx, ok := ctx.Value(AuthContextKey).(*AuthContext)
	if !ok {
		return nil
	}
	return authCtx
}

// The methods below form a small, stable seam over the authenticated principal so
// that an authz layer can depend on a narrow interface it defines itself
// (e.g. interface{ Roles() []string; Scopes() []string; Subject() string }) which
// *AuthContext satisfies structurally. They are nil-safe.

// Subject returns the principal's JWT "sub" claim, or "" when unauthenticated.
func (a *AuthContext) Subject() string {
	if a == nil {
		return ""
	}
	return a.Principal.Subject()
}

// ClientID returns the OAuth2 client the principal's token was issued to, or ""
// when unauthenticated.
func (a *AuthContext) ClientID() string {
	if a == nil {
		return ""
	}
	return a.Principal.ClientID()
}

// Email returns the principal's "email" claim, or "" when absent or
// unauthenticated.
func (a *AuthContext) Email() string {
	if a == nil {
		return ""
	}
	return a.Principal.Email()
}

// OUID returns the principal's "ouId" claim, or "" when absent or
// unauthenticated.
func (a *AuthContext) OUID() string {
	if a == nil {
		return ""
	}
	return a.Principal.OUID()
}

// OUHandle returns the principal's "ouHandle" claim, or "" when absent or
// unauthenticated.
func (a *AuthContext) OUHandle() string {
	if a == nil {
		return ""
	}
	return a.Principal.OUHandle()
}

// Roles returns the granted roles for the principal, or nil.
func (a *AuthContext) Roles() []string {
	if a == nil {
		return nil
	}
	return a.Principal.Roles()
}

// Scopes returns the granted OAuth2 scopes for the principal, or nil.
func (a *AuthContext) Scopes() []string {
	if a == nil {
		return nil
	}
	return a.Principal.Scopes()
}

// Claims returns the unified, read-only claims view, or an empty view when
// unauthenticated. Claims' accessors are also safe on an empty view.
func (a *AuthContext) Claims() Claims {
	if a == nil {
		return Claims{}
	}
	return a.Principal.Claims()
}
