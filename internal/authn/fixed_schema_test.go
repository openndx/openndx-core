// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Lanka Software Foundation

package authn

import (
	"encoding/json"
	"testing"
)

// TestTokenClaims_JSONBindingIsCaseInsensitive checks that standard claim
// canonicalization preserves encoding/json's case-insensitive struct binding.
func TestTokenClaims_JSONBindingIsCaseInsensitive(t *testing.T) {
	var c tokenClaims
	raw := `{"Roles":["admin"],"Client_ID":"evil","SCOPE":"a b"}`
	if err := json.Unmarshal([]byte(raw), &c); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	roles, err := parseRolesClaim(c.Roles.value)
	if err != nil {
		t.Fatalf("parseRolesClaim: %v", err)
	}
	if len(roles) != 1 || roles[0] != "admin" {
		t.Fatalf(`"Roles" did not bind to tokenClaims.Roles: %#v`, roles)
	}
	if c.ClientID != "evil" {
		t.Fatalf(`"Client_ID" did not bind to tokenClaims.ClientID: %q`, c.ClientID)
	}
	if len(c.Scopes) != 2 {
		t.Fatalf(`"SCOPE" did not bind to tokenClaims.Scopes: %#v`, c.Scopes)
	}
}

func TestParseRolesClaim(t *testing.T) {
	tests := []struct {
		name    string
		value   any
		want    []string
		wantNil bool // sameScopes cannot tell nil from an empty slice; assert it explicitly
		wantErr bool
	}{
		{name: "absent", value: nil, want: nil, wantNil: true},
		{name: "array of strings", value: []any{"a", "b"}, want: []string{"a", "b"}},
		{name: "elements are trimmed", value: []any{" a ", "b"}, want: []string{"a", "b"}},
		{name: "empty array", value: []any{}, want: []string{}},
		// Space-splitting a bare string would turn "Customs Officer"
		// into two roles. Only scope accepts a space-delimited string.
		{name: "space-delimited string", value: "a b", wantErr: true},
		{name: "bare string", value: "admin", wantErr: true},
		{name: "number", value: 42.0, wantErr: true},
		{name: "object", value: map[string]any{"a": true}, wantErr: true},
		{name: "mixed array", value: []any{"a", 1.0}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseRolesClaim(tt.value)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got %#v", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !sameScopes(got, tt.want) {
				t.Fatalf("got %#v, want %#v", got, tt.want)
			}
			if (got == nil) != tt.wantNil {
				t.Fatalf("got nil = %v, want nil = %v (%#v)", got == nil, tt.wantNil, got)
			}
		})
	}
}

// TestRolesClaim covers roles at the token level: parseRolesClaim's shape rules
// (see TestParseRolesClaim) decide whether the whole token is accepted.
func TestRolesClaim(t *testing.T) {
	extractor, privateKey, cleanup := newTokenExtractor(t)
	defer cleanup()

	t.Run("absent claim yields no roles, not an error", func(t *testing.T) {
		p, err := extractor.ExtractPrincipalFromHeader("Bearer " + signToken(t, privateKey, newBaseClaims()))
		if err != nil {
			t.Fatalf("expected a caller with no roles to authenticate, got %v", err)
		}
		if p.Roles() != nil {
			t.Fatalf("roles = %#v, want nil", p.Roles())
		}
	})

	t.Run("wrong shape rejects the token", func(t *testing.T) {
		// A silently empty Roles reads as "granted nothing", which fails OPEN
		// for any check phrased as "deny if the principal has role X".
		claims := newBaseClaims()
		claims["roles"] = "exporter broker"

		if _, err := extractor.ExtractPrincipalFromHeader("Bearer " + signToken(t, privateKey, claims)); err == nil {
			t.Fatalf("expected a space-delimited roles string to be rejected")
		}
	})
}
