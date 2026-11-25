// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Lanka Software Foundation

// Package caller is the Portal Backend's view of who is calling: the OAuth2
// scopes its routes require, the scopes and organization unit internal/authn
// validated, and the member that organization unit belongs to. Middleware
// puts the member on the request context; handlers read it from there.
package caller

import (
	"context"
	"slices"

	"github.com/OpenNSW/core/authz"
	"github.com/openndx/openndx-core/internal/authn"
)

type memberIDKey struct{}

// PrincipalFromContext is the authz.Extractor for PB: the principal
// internal/authn validated, or false when the request is unauthenticated.
func PrincipalFromContext(ctx context.Context) (authz.Principal, bool) {
	authCtx := authn.GetAuthContext(ctx)
	if authCtx == nil {
		return nil, false
	}
	return authCtx, true
}

// HasScope reports whether the caller's token was granted scope.
func HasScope(ctx context.Context, scope string) bool {
	return slices.Contains(authn.GetAuthContext(ctx).Scopes(), scope)
}

// OUID returns the caller's organization unit, or "" when it has none.
func OUID(ctx context.Context) string {
	return authn.GetAuthContext(ctx).OUID()
}

// WithMemberID returns ctx carrying memberID as the caller's member.
func WithMemberID(ctx context.Context, memberID string) context.Context {
	return context.WithValue(ctx, memberIDKey{}, memberID)
}

// MemberID returns the member the caller's organization unit belongs to, or
// false when the caller has no organization unit or it has no member.
func MemberID(ctx context.Context) (string, bool) {
	memberID, ok := ctx.Value(memberIDKey{}).(string)
	return memberID, ok && memberID != ""
}

// CanAccess reports whether the caller may act on a record owned by ownerID:
// it holds allScope, which covers every member, or ownerID is its own member.
func CanAccess(ctx context.Context, allScope, ownerID string) bool {
	if HasScope(ctx, allScope) {
		return true
	}
	memberID, ok := MemberID(ctx)
	return ok && memberID == ownerID
}
