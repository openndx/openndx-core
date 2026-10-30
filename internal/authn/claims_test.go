// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Lanka Software Foundation

package authn

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/golang-jwt/jwt/v5"
)

func TestClaims_ValidatedToken(t *testing.T) {
	extractor, key, cleanup := newTokenExtractor(t)
	defer cleanup()

	values := newBaseClaims()
	values["email"] = "  " + testEmail + " \n"
	values["ouId"] = "  " + testOUID + " "
	values["ouHandle"] = "  " + testOUHandle + " "
	values["roles"] = []string{" Customs Officer ", "exporter"}
	values["scope"] = " read   write "
	values["jti"] = "token-1"
	values["nic"] = "  199012345678 "
	values["https://example.com/department"] = " Customs "
	p, err := extractor.ExtractPrincipalFromHeader("Bearer " + signToken(t, key, values))
	if err != nil {
		t.Fatal(err)
	}
	ctx := &AuthContext{Principal: *p}
	claims := ctx.Claims()
	for name, want := range map[string]string{
		"sub": testUserID, "client_id": testClientID, "email": testEmail,
		"ouId": testOUID, "ouHandle": testOUHandle, "iss": testIssuer,
		"aud": testClientID, "jti": "token-1", "nic": "199012345678",
		"https://example.com/department": "Customs", "scope": " read   write ",
	} {
		if got := claims.String(name); got != want {
			t.Errorf("String(%q) = %q, want %q", name, got, want)
		}
	}
	for _, name := range []string{"exp", "nbf", "iat"} {
		value, present := claims.Lookup(name)
		if !present || value != float64(values[name].(int64)) || claims.String(name) != "" {
			t.Errorf("registered numeric claim %s = %#v, present=%v", name, value, present)
		}
	}
	for name, typed := range map[string]string{
		"sub": ctx.Subject(), "client_id": ctx.ClientID(), "email": ctx.Email(),
		"ouId": ctx.OUID(), "ouHandle": ctx.OUHandle(),
	} {
		if claims.String(name) != typed {
			t.Errorf("typed and generic %s disagree", name)
		}
	}
	if !reflect.DeepEqual(ctx.Roles(), []string{"Customs Officer", "exporter"}) ||
		!reflect.DeepEqual(ctx.Roles(), claims.Strings("roles")) {
		t.Errorf("roles disagree: %v / %v", ctx.Roles(), claims.Strings("roles"))
	}
	if !reflect.DeepEqual(ctx.Scopes(), []string{"read", "write"}) ||
		!reflect.DeepEqual(ctx.Scopes(), claims.Strings("scope")) {
		t.Errorf("scopes disagree: %v / %v", ctx.Scopes(), claims.Strings("scope"))
	}
}

func TestClaims_PresenceAndTypes(t *testing.T) {
	extractor, key, cleanup := newTokenExtractor(t)
	defer cleanup()
	values := newBaseClaims()
	for name, value := range map[string]any{
		"null": nil, "blank": " \t ", "number": 42, "false": false,
		"empty": []any{}, "list": []any{" a ", "b "}, "mixed": []any{"a", 1},
		"object": map[string]any{"nested": "  untouched  "}, "free_text": "Customs Officer",
	} {
		values[name] = value
	}
	p, err := extractor.ExtractPrincipalFromHeader("Bearer " + signToken(t, key, values))
	if err != nil {
		t.Fatal(err)
	}
	claims := p.Claims()
	for _, tc := range []struct {
		name    string
		value   any
		present bool
		str     string
		list    []string
	}{
		{"missing", nil, false, "", nil},
		{"null", nil, true, "", nil},
		{"blank", "", true, "", nil},
		{"number", 42.0, true, "", nil},
		{"false", false, true, "", nil},
		{"empty", []any{}, true, "", []string{}},
		{"list", []any{"a", "b"}, true, "", []string{"a", "b"}},
		{"mixed", []any{"a", 1.0}, true, "", nil},
		{"object", map[string]any{"nested": "  untouched  "}, true, "", nil},
		{"free_text", "Customs Officer", true, "Customs Officer", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			value, present := claims.Lookup(tc.name)
			if present != tc.present || !reflect.DeepEqual(value, tc.value) {
				t.Errorf("Lookup = (%#v, %v), want (%#v, %v)", value, present, tc.value, tc.present)
			}
			if claims.String(tc.name) != tc.str || !reflect.DeepEqual(claims.Strings(tc.name), tc.list) {
				t.Errorf("String/Strings = %q / %#v", claims.String(tc.name), claims.Strings(tc.name))
			}
		})
	}
}

func TestClaims_ImmutableCollections(t *testing.T) {
	extractor, key, cleanup := newTokenExtractor(t)
	defer cleanup()
	values := newBaseClaims()
	values["roles"] = []string{"exporter"}
	values["scope"] = []string{"read"}
	values["groups"] = []string{"group-1"}
	values["aud"] = []string{testClientID, "other-api"}
	values["meta"] = map[string]any{"items": []any{map[string]any{"value": "original"}}}
	p, err := extractor.ExtractPrincipalFromHeader("Bearer " + signToken(t, key, values))
	if err != nil {
		t.Fatal(err)
	}
	ctx := &AuthContext{Principal: *p}
	ctx.Roles()[0] = "admin"
	ctx.Scopes()[0] = "write"
	ctx.Claims().Strings("roles")[0] = "admin"
	ctx.Claims().Strings("scope")[0] = "write"
	ctx.Claims().Strings("groups")[0] = "group-2"
	roles, _ := ctx.Claims().Lookup("roles")
	roles.([]any)[0] = "admin"
	audiences, _ := ctx.Claims().Lookup("aud")
	audiences.([]any)[0] = "other-api"
	meta, _ := ctx.Claims().Lookup("meta")
	meta.(map[string]any)["items"].([]any)[0].(map[string]any)["value"] = "changed"
	if !reflect.DeepEqual(ctx.Roles(), []string{"exporter"}) ||
		!reflect.DeepEqual(ctx.Scopes(), []string{"read"}) ||
		!reflect.DeepEqual(ctx.Claims().Strings("groups"), []string{"group-1"}) {
		t.Fatal("returned slices mutated the identity")
	}
	meta, _ = ctx.Claims().Lookup("meta")
	if !reflect.DeepEqual(meta, values["meta"]) {
		t.Fatalf("nested mutation leaked: %#v", meta)
	}
	if !reflect.DeepEqual(ctx.Claims().Strings("aud"), []string{testClientID, "other-api"}) {
		t.Fatal("registered claim array mutation leaked")
	}
}

func TestClaims_CaseAndExactNames(t *testing.T) {
	extractor, key, cleanup := newTokenExtractor(t)
	defer cleanup()
	values := newBaseClaims()
	delete(values, "sub")
	values["SUB"] = testUserID
	values["EMAIL"] = " " + testEmail + " "
	values["OUID"] = testOUID
	values["Roles"] = []string{"exporter"}
	values["NIC"] = "upper"
	values["nic"] = "lower"
	values[" sub "] = "custom"
	p, err := extractor.ExtractPrincipalFromHeader("Bearer " + signToken(t, key, values))
	if err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]string{
		"sub": testUserID, "email": testEmail, "ouId": testOUID,
		"NIC": "upper", "nic": "lower", " sub ": "custom",
	} {
		if p.Claims().String(name) != want {
			t.Errorf("String(%q) = %q, want %q", name, p.Claims().String(name), want)
		}
	}
	for _, name := range []string{"SUB", "EMAIL", "OUID", "Roles", "Nic", "roles[0]"} {
		if _, present := p.Claims().Lookup(name); present {
			t.Errorf("lookup must use exact canonical name: %q", name)
		}
	}
	if p.Subject() != p.Claims().String("sub") || p.Email() != p.Claims().String("email") ||
		!reflect.DeepEqual(p.Roles(), p.Claims().Strings("roles")) {
		t.Fatal("case handling made typed and generic accessors disagree")
	}
}

func TestClaims_NilSafe(t *testing.T) {
	var ctx *AuthContext
	var principal *Principal
	for _, claims := range []Claims{ctx.Claims(), (&AuthContext{}).Claims(), principal.Claims(), Claims{}} {
		if claims.String("sub") != "" || claims.Strings("roles") != nil {
			t.Error("empty claims must return zero values")
		}
		if value, present := claims.Lookup("sub"); present || value != nil {
			t.Error("empty claims must report missing values")
		}
	}
	if principal.Subject() != "" || principal.ClientID() != "" || principal.Email() != "" ||
		principal.OUID() != "" || principal.OUHandle() != "" || principal.Roles() != nil ||
		principal.Scopes() != nil {
		t.Error("nil principal must return zero values")
	}
}

// rawTokenClaims allows signing payloads with duplicate keys for regression tests.
type rawTokenClaims struct {
	jwt.RegisteredClaims
	raw []byte
}

func (c rawTokenClaims) MarshalJSON() ([]byte, error) { return c.raw, nil }

func TestClaims_RejectDuplicateNames(t *testing.T) {
	extractor, key, cleanup := newTokenExtractor(t)
	defer cleanup()
	base, err := json.Marshal(newBaseClaims())
	if err != nil {
		t.Fatal(err)
	}
	for _, extra := range []string{
		`,"sub":"other"`, `,"SUB":"other"`, `,"client_ID":"other"`,
		`,"email":"a","EMAIL":"b"`, `,"roles":["admin"],"Roles":["reader"]`,
		`,"custom":"a","custom":"b"`,
	} {
		t.Run(extra, func(t *testing.T) {
			raw := append(append([]byte{}, base[:len(base)-1]...), []byte(extra+"}")...)
			token := jwt.NewWithClaims(jwt.SigningMethodRS256, rawTokenClaims{raw: raw})
			token.Header["kid"] = testKid
			signed, err := token.SignedString(key)
			if err != nil {
				t.Fatal(err)
			}
			if p, err := extractor.ExtractPrincipalFromHeader("Bearer " + signed); err == nil || p != nil {
				t.Fatalf("ambiguous token accepted: principal=%v error=%v", p, err)
			}
		})
	}
}

func TestClaims_StandardTypesStillValidated(t *testing.T) {
	extractor, key, cleanup := newTokenExtractor(t)
	defer cleanup()
	for _, name := range []string{"sub", "client_id", "email", "ouId", "ouHandle", "iss", "aud", "jti", "exp", "nbf", "iat", "scope", "roles"} {
		t.Run(name, func(t *testing.T) {
			values := newBaseClaims()
			values[name] = map[string]any{"wrong": "shape"}
			if p, err := extractor.ExtractPrincipalFromHeader("Bearer " + signToken(t, key, values)); err == nil || p != nil {
				t.Fatalf("wrong-type %s accepted: principal=%v error=%v", name, p, err)
			}
		})
	}
}

func TestClaims_ScopeParsing(t *testing.T) {
	extractor, key, cleanup := newTokenExtractor(t)
	defer cleanup()
	for _, tc := range []struct {
		name  string
		value any
		want  []string
	}{
		{"null", nil, []string{}},
		{"blank", " \t ", []string{}},
		{"string", " a  b ", []string{"a", "b"}},
		{"empty array", []any{}, []string{}},
		{"array", []any{" a ", "b", nil}, []string{" a ", "b", ""}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			values := newBaseClaims()
			values["scope"] = tc.value
			p, err := extractor.ExtractPrincipalFromHeader("Bearer " + signToken(t, key, values))
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(p.Scopes(), tc.want) || !reflect.DeepEqual(p.Claims().Strings("scope"), tc.want) {
				t.Fatalf("Scopes = %#v, generic = %#v, want %#v", p.Scopes(), p.Claims().Strings("scope"), tc.want)
			}
		})
	}
}
