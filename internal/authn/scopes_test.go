// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Lanka Software Foundation

package authn

import (
	"encoding/json"
	"testing"
)

func sameScopes(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// The OAuth2 "scope" claim is a single space-delimited string (RFC 6749 §3.3);
// it must unmarshal into a slice without error.
func TestSpaceDelimitedScope_Unmarshal(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want []string
	}{
		{"space-delimited string", `{"scope":"nsw:task:read nsw:storage:read"}`, []string{"nsw:task:read", "nsw:storage:read"}},
		{"extra whitespace", `{"scope":"  a   b "}`, []string{"a", "b"}},
		{"empty string", `{"scope":""}`, nil},
		{"absent", `{}`, nil},
		{"defensive array", `{"scope":["a","b"]}`, []string{"a", "b"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var c tokenClaims
			if err := json.Unmarshal([]byte(tc.in), &c); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if got := []string(c.Scopes); !sameScopes(got, tc.want) {
				t.Fatalf("scopes = %#v, want %#v", got, tc.want)
			}
		})
	}
}

func TestSpaceDelimitedScope_InvalidType(t *testing.T) {
	var c tokenClaims
	if err := json.Unmarshal([]byte(`{"scope":123}`), &c); err == nil {
		t.Fatal("expected error unmarshaling a numeric scope claim")
	}
}

// End-to-end through the parser: scope claim and client roles/scopes surface on
// the principal and flow through to the AuthContext.
func TestExtractPrincipal_SurfacesScopesAndRoles(t *testing.T) {
	extractor, privateKey, cleanup := newTokenExtractor(t)
	defer cleanup()

	t.Run("user token scopes", func(t *testing.T) {
		claims := newBaseClaims()
		claims["sub"] = testUserID
		claims["email"] = testEmail
		claims["ouId"] = testOUID
		claims["ouHandle"] = testOUHandle
		claims["roles"] = []string{"Trader"}
		claims["scope"] = "nsw:task:read nsw:storage:read"

		p, err := extractor.ExtractPrincipalFromHeader("Bearer " + signToken(t, privateKey, claims))
		if err != nil {
			t.Fatalf("extract: %v", err)
		}
		if !sameScopes(p.Scopes(), []string{"nsw:task:read", "nsw:storage:read"}) {
			t.Fatalf("user scopes = %v", p.Scopes())
		}
	})

	t.Run("client token roles and scopes", func(t *testing.T) {
		claims := newBaseClaims()
		claims["sub"] = testClientID
		claims["roles"] = []string{"AgencyM2M"}
		claims["scope"] = "nsw:task:write nsw:consignment:read"

		p, err := extractor.ExtractPrincipalFromHeader("Bearer " + signToken(t, privateKey, claims))
		if err != nil {
			t.Fatalf("extract: %v", err)
		}
		if !sameScopes(p.Scopes(), []string{"nsw:task:write", "nsw:consignment:read"}) {
			t.Fatalf("client scopes = %v", p.Scopes())
		}
		if !sameScopes(p.Roles(), []string{"AgencyM2M"}) {
			t.Fatalf("client roles = %v", p.Roles())
		}
	})
}

// Compile-time proof that *AuthContext satisfies the accessor seam used by authz.
var _ interface {
	Subject() string
	Roles() []string
	Scopes() []string
} = (*AuthContext)(nil)
