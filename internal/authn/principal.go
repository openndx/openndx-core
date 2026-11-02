// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Lanka Software Foundation

package authn

// Principal is the authenticated caller described by a validated token. All
// accessors derive their values from one private, immutable claim set.
// Principals are created by TokenExtractor, never from unverified payloads.
type Principal struct {
	claims Claims
}

// Claims returns all claims from the validated token, including registered,
// standard identity and custom claims. A nil principal returns an empty view.
func (p *Principal) Claims() Claims {
	if p == nil {
		return Claims{}
	}
	return p.claims
}

// Subject returns the JWT sub claim, or "" for a nil/empty principal.
func (p *Principal) Subject() string { return p.Claims().String("sub") }

// ClientID returns the OAuth2 client_id claim.
func (p *Principal) ClientID() string { return p.Claims().String("client_id") }

// Email returns the whitespace-trimmed email claim.
func (p *Principal) Email() string { return p.Claims().String("email") }

// OUID returns the whitespace-trimmed ouId claim.
func (p *Principal) OUID() string { return p.Claims().String("ouId") }

// OUHandle returns the whitespace-trimmed ouHandle claim.
func (p *Principal) OUHandle() string { return p.Claims().String("ouHandle") }

// Roles returns a copy of the granted roles, or nil when absent/null.
func (p *Principal) Roles() []string { return p.Claims().Strings("roles") }

// Scopes returns a copy of the OAuth2 scopes, or nil when absent.
func (p *Principal) Scopes() []string { return p.Claims().Strings("scope") }
