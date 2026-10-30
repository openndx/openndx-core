// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Lanka Software Foundation

package authn

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/golang-jwt/jwt/v5"
)

func TestTokenExtractor_ExtractPrincipalFromHeader(t *testing.T) {
	extractor, privateKey, cleanup := newTokenExtractor(t)
	defer cleanup()

	validUserToken := newUserToken(t, privateKey)
	validClientToken := newClientToken(t, privateKey)

	tests := []struct {
		name        string
		authHeader  string
		wantSubject string
		wantErr     bool
	}{
		{name: "valid user token", authHeader: "Bearer " + validUserToken, wantSubject: testUserID},
		{name: "valid client token", authHeader: "Bearer " + validClientToken, wantSubject: "FCAU_TO_NSW"},
		{name: "missing header", authHeader: "", wantErr: true},
		{name: "invalid token", authHeader: "Bearer invalid.jwt.token", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			principal, err := extractor.ExtractPrincipalFromHeader(tt.authHeader)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ExtractPrincipalFromHeader() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if principal.Subject() != tt.wantSubject || principal.ClientID() != testClientID {
				t.Fatalf("unexpected principal: got (%q, %q) want (%q, %q)",
					principal.Subject(), principal.ClientID(), tt.wantSubject, testClientID)
			}
		})
	}
}

func TestTokenExtractor_ExtractPrincipalFromHeader_InvalidHeaderFormats(t *testing.T) {
	extractor, _, cleanup := newTokenExtractor(t)
	defer cleanup()

	invalidHeaders := []string{
		"Bearer",
		"Bearer ",
		"Bearer\t",
		"Basic abc.def",
		"Token abc.def",
	}

	for _, header := range invalidHeaders {
		if _, err := extractor.ExtractPrincipalFromHeader(header); err == nil {
			t.Fatalf("expected error for header %q", header)
		}
	}
}

func TestTokenExtractor_ExtractPrincipalFromHeader_MissingClaims(t *testing.T) {
	extractor, privateKey, cleanup := newTokenExtractor(t)
	defer cleanup()

	baseClaims := func() jwt.MapClaims {
		claims := newBaseClaims()
		claims["sub"] = testUserID
		claims["email"] = testEmail
		claims["phone_number"] = testPhone
		claims["ouId"] = testOUID
		claims["ouHandle"] = testOUHandle
		claims["roles"] = []string{"exporter"}
		return claims
	}

	tests := []struct {
		name      string
		mutate    func(jwt.MapClaims)
		errSubstr string
	}{
		{
			name: "missing exp",
			mutate: func(claims jwt.MapClaims) {
				delete(claims, "exp")
			},
			errSubstr: "jwt missing exp claim",
		},
		{
			name: "missing client_id",
			mutate: func(claims jwt.MapClaims) {
				delete(claims, "client_id")
			},
			errSubstr: "jwt missing client_id claim",
		},
		{
			name: "missing sub",
			mutate: func(claims jwt.MapClaims) {
				delete(claims, "sub")
			},
			errSubstr: "jwt missing sub claim",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			claims := baseClaims()
			tt.mutate(claims)
			token := signToken(t, privateKey, claims)
			_, err := extractor.ExtractPrincipalFromHeader("Bearer " + token)
			if err == nil || !strings.Contains(err.Error(), tt.errSubstr) {
				t.Fatalf("expected error containing %q, got %v", tt.errSubstr, err)
			}
		})
	}
}

// TestTokenExtractor_IgnoresGrantType locks in that how a token was obtained
// has no bearing on whether it is accepted or on the Principal it yields:
// grant_type used to dispatch between user and client principals and reject
// anything else.
func TestTokenExtractor_IgnoresGrantType(t *testing.T) {
	extractor, privateKey, cleanup := newTokenExtractor(t)
	defer cleanup()

	for _, grantType := range []any{nil, "authorization_code", "client_credentials", "password", 42} {
		claims := newBaseClaims()
		if grantType != nil {
			claims["grant_type"] = grantType
		}
		principal, err := extractor.ExtractPrincipalFromHeader("Bearer " + signToken(t, privateKey, claims))
		if err != nil {
			t.Fatalf("grant_type %v: expected success, got %v", grantType, err)
		}
		if principal.Subject() != testUserID || principal.ClientID() != testClientID {
			t.Fatalf("grant_type %v: unexpected principal %#v", grantType, principal)
		}
	}
}

// TestTokenExtractor_GrantTypeIsAnOrdinaryClaim covers the other half of
// dropping dispatch: grant_type is no longer reserved, so it passes through to
// Claims() like any other claim for a consumer that cares.
func TestTokenExtractor_GrantTypeIsAnOrdinaryClaim(t *testing.T) {
	extractor, privateKey, cleanup := newTokenExtractor(t)
	defer cleanup()

	claims := newBaseClaims()
	claims["grant_type"] = "client_credentials"

	principal, err := extractor.ExtractPrincipalFromHeader("Bearer " + signToken(t, privateKey, claims))
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if got := principal.Claims().String("grant_type"); got != "client_credentials" {
		t.Fatalf("Claims grant_type = %q, want client_credentials", got)
	}
}

// TestTokenExtractor_UserToken_NoLongerRequiresEmailOrOU documents a
// deliberate behavior change: email/ouId/ouHandle used to be hard-required
// fixed-schema claims. They are now ordinary extra claims, so a token that
// omits them entirely is accepted; requiring them is the application's job.
func TestTokenExtractor_UserToken_NoLongerRequiresEmailOrOU(t *testing.T) {
	extractor, privateKey, cleanup := newTokenExtractor(t)
	defer cleanup()

	claims := newBaseClaims()
	claims["sub"] = testUserID
	// Deliberately omit email/phone_number/ouId/ouHandle/roles entirely.

	if _, err := extractor.ExtractPrincipalFromHeader("Bearer " + signToken(t, privateKey, claims)); err != nil {
		t.Fatalf("expected success: email/ouId/ouHandle are no longer hard-required, got %v", err)
	}
}

// TestTokenExtractor_AcceptsAnyClientIDForAudience documents that there is no
// client allowlist: aud binds a token to this service, and which clients may
// obtain such a token is the IdP's decision. Restricting callers by client_id
// is authorization policy, not authentication.
func TestTokenExtractor_AcceptsAnyClientIDForAudience(t *testing.T) {
	extractor, privateKey, cleanup := newTokenExtractor(t)
	defer cleanup()

	claims := newBaseClaims()
	claims["client_id"] = "SOME_OTHER_CLIENT"

	principal, err := extractor.ExtractPrincipalFromHeader("Bearer " + signToken(t, privateKey, claims))
	if err != nil {
		t.Fatalf("expected success for a token issued for this audience, got %v", err)
	}
	if principal.ClientID() != "SOME_OTHER_CLIENT" {
		t.Fatalf("unexpected principal: %#v", principal)
	}
}

func TestTokenExtractor_JWKSIsCached(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("failed to generate rsa key: %v", err)
	}

	var fetchCount int32
	jwksServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&fetchCount, 1)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"keys": []map[string]any{{
				"kid": "cache-kid",
				"kty": "RSA",
				"alg": "RS256",
				"use": "sig",
				"n":   base64.RawURLEncoding.EncodeToString(privateKey.N.Bytes()),
				"e":   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(privateKey.PublicKey.E)).Bytes()),
			}},
		})
	}))
	defer jwksServer.Close()

	claims := newBaseClaims()
	claims["sub"] = testUserID
	claims["email"] = testEmail
	claims["phone_number"] = testPhone
	claims["ouId"] = testOUID
	claims["ouHandle"] = testOUHandle
	claims["roles"] = []string{"exporter"}

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = "cache-kid"
	signedToken, err := token.SignedString(privateKey)
	if err != nil {
		t.Fatalf("failed to sign token: %v", err)
	}

	extractor, err := NewTokenExtractor(jwksServer.URL, testIssuer, testClientID)
	if err != nil {
		t.Fatalf("failed to create token extractor: %v", err)
	}

	if _, err := extractor.ExtractPrincipalFromHeader("Bearer " + signedToken); err != nil {
		t.Fatalf("first extract failed: %v", err)
	}
	if _, err := extractor.ExtractPrincipalFromHeader("Bearer " + signedToken); err != nil {
		t.Fatalf("second extract failed: %v", err)
	}

	if got := atomic.LoadInt32(&fetchCount); got != 1 {
		t.Fatalf("expected JWKS to be fetched once, got %d", got)
	}
}

func TestTokenExtractor_RefreshesJWKSOnUnknownKid(t *testing.T) {
	privateKeyA, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("failed to generate first rsa key: %v", err)
	}
	privateKeyB, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("failed to generate second rsa key: %v", err)
	}

	var fetchCount int32
	var serveNewKey int32
	jwksServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&fetchCount, 1)
		w.Header().Set("Content-Type", "application/json")

		key := privateKeyA.PublicKey
		kid := "old-kid"
		if atomic.LoadInt32(&serveNewKey) == 1 {
			key = privateKeyB.PublicKey
			kid = "new-kid"
		}

		_ = json.NewEncoder(w).Encode(map[string]any{
			"keys": []map[string]any{{
				"kid": kid,
				"kty": "RSA",
				"alg": "RS256",
				"use": "sig",
				"n":   base64.RawURLEncoding.EncodeToString(key.N.Bytes()),
				"e":   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.E)).Bytes()),
			}},
		})
	}))
	defer jwksServer.Close()

	oldClaims := newBaseClaims()
	oldClaims["sub"] = testUserID
	oldClaims["email"] = testEmail
	oldClaims["phone_number"] = testPhone
	oldClaims["ouId"] = testOUID
	oldClaims["ouHandle"] = testOUHandle
	oldClaims["roles"] = []string{"exporter"}

	oldToken := jwt.NewWithClaims(jwt.SigningMethodRS256, oldClaims)
	oldToken.Header["kid"] = "old-kid"
	oldSignedToken, err := oldToken.SignedString(privateKeyA)
	if err != nil {
		t.Fatalf("failed to sign old token: %v", err)
	}

	newClaims := newBaseClaims()
	newClaims["sub"] = testUserID
	newClaims["email"] = testEmail
	newClaims["phone_number"] = testPhone
	newClaims["ouId"] = testOUID
	newClaims["ouHandle"] = testOUHandle
	newClaims["roles"] = []string{"exporter"}

	newToken := jwt.NewWithClaims(jwt.SigningMethodRS256, newClaims)
	newToken.Header["kid"] = "new-kid"
	newSignedToken, err := newToken.SignedString(privateKeyB)
	if err != nil {
		t.Fatalf("failed to sign new token: %v", err)
	}

	extractor, err := NewTokenExtractor(jwksServer.URL, testIssuer, testClientID)
	if err != nil {
		t.Fatalf("failed to create token extractor: %v", err)
	}
	if _, err := extractor.ExtractPrincipalFromHeader("Bearer " + oldSignedToken); err != nil {
		t.Fatalf("old token extract failed: %v", err)
	}

	atomic.StoreInt32(&serveNewKey, 1)
	if _, err := extractor.ExtractPrincipalFromHeader("Bearer " + newSignedToken); err != nil {
		t.Fatalf("new token extract failed after refresh: %v", err)
	}

	if got := atomic.LoadInt32(&fetchCount); got != 2 {
		t.Fatalf("expected JWKS fetches to be 2 (initial + refresh on unknown kid), got %d", got)
	}
}

func TestTokenExtractor_UnknownKidAfterRefresh(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("failed to generate rsa key: %v", err)
	}

	jwksServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"keys": []map[string]any{{
				"kid": "other-kid",
				"kty": "RSA",
				"alg": "RS256",
				"use": "sig",
				"n":   base64.RawURLEncoding.EncodeToString(privateKey.N.Bytes()),
				"e":   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(privateKey.PublicKey.E)).Bytes()),
			}},
		})
	}))
	defer jwksServer.Close()

	claims := newBaseClaims()
	claims["sub"] = testUserID
	claims["email"] = testEmail
	claims["phone_number"] = testPhone
	claims["ouId"] = testOUID
	claims["ouHandle"] = testOUHandle
	claims["roles"] = []string{"exporter"}

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = "missing-kid"
	signedToken, err := token.SignedString(privateKey)
	if err != nil {
		t.Fatalf("failed to sign token: %v", err)
	}

	extractor, err := NewTokenExtractor(jwksServer.URL, testIssuer, testClientID)
	if err != nil {
		t.Fatalf("failed to create token extractor: %v", err)
	}

	if _, err := extractor.ExtractPrincipalFromHeader("Bearer " + signedToken); err == nil || !strings.Contains(err.Error(), "no jwk found for kid") {
		t.Fatalf("expected missing kid error, got %v", err)
	}
}

func TestTokenExtractor_JWKSFetchError(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("failed to generate rsa key: %v", err)
	}

	jwksServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer jwksServer.Close()

	claims := newBaseClaims()
	claims["sub"] = testUserID
	claims["email"] = testEmail
	claims["phone_number"] = testPhone
	claims["ouId"] = testOUID
	claims["ouHandle"] = testOUHandle
	claims["roles"] = []string{"exporter"}

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = testKid
	signedToken, err := token.SignedString(privateKey)
	if err != nil {
		t.Fatalf("failed to sign token: %v", err)
	}

	extractor, err := NewTokenExtractor(jwksServer.URL, testIssuer, testClientID)
	if err != nil {
		t.Fatalf("failed to create token extractor: %v", err)
	}

	if _, err := extractor.ExtractPrincipalFromHeader("Bearer " + signedToken); err == nil || !strings.Contains(err.Error(), "jwks endpoint returned status") {
		t.Fatalf("expected jwks status error, got %v", err)
	}
}

func TestTokenExtractor_ValidateConfig_MissingHTTPClient(t *testing.T) {
	extractor := &TokenExtractor{
		jwksURL:     "https://localhost/jwks",
		expIssuer:   testIssuer,
		expAudience: testClientID,
		httpClient:  nil,
	}

	if err := extractor.validateConfig(); err == nil {
		t.Fatalf("expected error for missing http client")
	}
}

func TestParseRSAPublicKey_InvalidData(t *testing.T) {
	validN := base64.RawURLEncoding.EncodeToString([]byte{1})

	tests := []struct {
		name      string
		key       jwk
		errSubstr string
	}{
		{
			name:      "invalid key type",
			key:       jwk{Kty: "EC"},
			errSubstr: "unsupported jwk key type",
		},
		{
			name:      "invalid modulus",
			key:       jwk{Kty: "RSA", N: "@@", E: "AQAB"},
			errSubstr: "failed to decode jwk modulus",
		},
		{
			name:      "invalid exponent",
			key:       jwk{Kty: "RSA", N: validN, E: "@@"},
			errSubstr: "failed to decode jwk exponent",
		},
		{
			name:      "empty key data",
			key:       jwk{Kty: "RSA", N: "", E: "AQAB"},
			errSubstr: "invalid jwk key data",
		},
		{
			name:      "invalid exponent value",
			key:       jwk{Kty: "RSA", N: validN, E: "AA"},
			errSubstr: "invalid jwk exponent",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := parseRSAPublicKey(tt.key); err == nil || !strings.Contains(err.Error(), tt.errSubstr) {
				t.Fatalf("expected error containing %q, got %v", tt.errSubstr, err)
			}
		})
	}
}

func TestTokenExtractor_StandardClaims(t *testing.T) {
	extractor, privateKey, cleanup := newTokenExtractor(t)
	defer cleanup()

	t.Run("populated and trimmed", func(t *testing.T) {
		claims := newBaseClaims()
		claims["email"] = "  " + testEmail + " \n"
		claims["ouId"] = testOUID
		claims["ouHandle"] = testOUHandle

		p, err := extractor.ExtractPrincipalFromHeader("Bearer " + signToken(t, privateKey, claims))
		if err != nil {
			t.Fatalf("extract: %v", err)
		}
		if p.Email() != testEmail || p.OUID() != testOUID || p.OUHandle() != testOUHandle {
			t.Fatalf("unexpected standard claims: %+v", p)
		}
	})

	t.Run("optional by default: absent or blank reads as empty", func(t *testing.T) {
		claims := newBaseClaims()
		claims["ouId"] = "   "

		p, err := extractor.ExtractPrincipalFromHeader("Bearer " + signToken(t, privateKey, claims))
		if err != nil {
			t.Fatalf("expected success with no standard claims, got %v", err)
		}
		if p.Email() != "" || p.OUID() != "" || p.OUHandle() != "" {
			t.Fatalf("expected empty standard claims, got %+v", p)
		}
	})

	t.Run("a non-string value rejects the token", func(t *testing.T) {
		claims := newBaseClaims()
		claims["email"] = 42

		if _, err := extractor.ExtractPrincipalFromHeader("Bearer " + signToken(t, privateKey, claims)); err == nil {
			t.Fatal("expected error for a non-string email claim")
		}
	})
}

func TestTokenExtractor_RequiredStandardClaims(t *testing.T) {
	tests := []struct {
		name   string
		config ClaimsConfig
		claim  string
	}{
		{"email", ClaimsConfig{Email: ClaimConfig{Required: true}}, "email"},
		{"ouId", ClaimsConfig{OUID: ClaimConfig{Required: true}}, "ouId"},
		{"ouHandle", ClaimsConfig{OUHandle: ClaimConfig{Required: true}}, "ouHandle"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			extractor, privateKey, cleanup := newTokenExtractorWithOptions(t, WithClaims(tt.config))
			defer cleanup()

			for _, value := range []any{nil, "", "   "} {
				claims := newBaseClaims()
				if value != nil {
					claims[tt.claim] = value
				}
				_, err := extractor.ExtractPrincipalFromHeader("Bearer " + signToken(t, privateKey, claims))
				want := fmt.Sprintf("jwt missing or empty required claim %q", tt.claim)
				if err == nil || err.Error() != want {
					t.Fatalf("value %#v: expected %q, got %v", value, want, err)
				}
			}

			claims := newBaseClaims()
			claims[tt.claim] = "present"
			if _, err := extractor.ExtractPrincipalFromHeader("Bearer " + signToken(t, privateKey, claims)); err != nil {
				t.Fatalf("expected success when %s is present, got %v", tt.claim, err)
			}
		})
	}

	// Required applies to every token: a client-credentials token without an
	// email is rejected too.
	t.Run("applies to client tokens", func(t *testing.T) {
		extractor, privateKey, cleanup := newTokenExtractorWithOptions(t,
			WithClaims(ClaimsConfig{Email: ClaimConfig{Required: true}}))
		defer cleanup()

		if _, err := extractor.ExtractPrincipalFromHeader("Bearer " + newClientToken(t, privateKey)); err == nil {
			t.Fatal("expected a client token without email to be rejected")
		}
	})
}
