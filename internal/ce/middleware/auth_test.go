package middleware

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/openndx/openndx-core/internal/authn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testIssuer   = "http://idp.test"
	testAudience = "CONSENT_PORTAL"
	testKid      = "test-kid"
)

// newTestAuthn returns authn's RequireAuthMiddleware, backed by a JWKS server
// that serves the public half of the returned signing key.
func newTestAuthn(t *testing.T) (func(http.Handler) http.Handler, *rsa.PrivateKey) {
	t.Helper()
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	jwks := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"keys": []map[string]any{{
				"kid": testKid,
				"kty": "RSA",
				"alg": "RS256",
				"use": "sig",
				"n":   base64.RawURLEncoding.EncodeToString(privateKey.N.Bytes()),
				"e":   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(privateKey.E)).Bytes()),
			}},
		})
	}))
	t.Cleanup(jwks.Close)

	manager, err := authn.NewManager(authn.Config{JWKSURL: jwks.URL, Issuer: testIssuer, Audience: testAudience})
	require.NoError(t, err)
	return manager.RequireAuthMiddleware(), privateKey
}

// signToken returns a consent-portal token signed with key, with extra merged
// into its claims.
func signToken(t *testing.T, key *rsa.PrivateKey, extra jwt.MapClaims) string {
	t.Helper()
	claims := jwt.MapClaims{
		"iss":       testIssuer,
		"aud":       testAudience,
		"sub":       "user-123",
		"client_id": "CONSENT_PORTAL",
		"exp":       time.Now().Add(10 * time.Minute).Unix(),
	}
	for k, v := range extra {
		claims[k] = v
	}
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = testKid
	signed, err := token.SignedString(key)
	require.NoError(t, err)
	return signed
}

// serveAuthenticated sends a request through authenticate and OwnerSubject(claim)
// and returns the response and the owner subject the handler saw.
func serveAuthenticated(t *testing.T, authenticate func(http.Handler) http.Handler, claim, authorization string) (*httptest.ResponseRecorder, string) {
	t.Helper()
	var seen string
	handler := authenticate(OwnerSubject(claim)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		subject, ok := GetOwnerSubjectFromContext(r.Context())
		require.True(t, ok)
		seen = subject
		w.WriteHeader(http.StatusOK)
	})))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/consents", nil)
	if authorization != "" {
		req.Header.Set("Authorization", authorization)
	}
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	return w, seen
}

func TestOwnerSubject_UsesTokenSubject(t *testing.T) {
	authenticate, key := newTestAuthn(t)

	w, subject := serveAuthenticated(t, authenticate, SubjectClaim, "Bearer "+signToken(t, key, nil))

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "user-123", subject)
}

func TestOwnerSubject_UsesConfiguredClaim(t *testing.T) {
	authenticate, key := newTestAuthn(t)

	w, subject := serveAuthenticated(t, authenticate, "nic", "Bearer "+signToken(t, key, jwt.MapClaims{"nic": "199012345678"}))

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "199012345678", subject)
}

func TestOwnerSubject_UsesStandardClaims(t *testing.T) {
	authenticate, key := newTestAuthn(t)
	for _, tc := range []struct {
		claim string
		value string
		want  string
	}{
		{"email", "  owner@example.com \n", "owner@example.com"},
		{"ouId", " OU-123 ", "OU-123"},
		{"ouHandle", " organization ", "organization"},
		{"client_id", "CONSENT_PORTAL", "CONSENT_PORTAL"},
		{"jti", "token-123", "token-123"},
	} {
		t.Run(tc.claim, func(t *testing.T) {
			w, subject := serveAuthenticated(t, authenticate, tc.claim,
				"Bearer "+signToken(t, key, jwt.MapClaims{tc.claim: tc.value}))
			assert.Equal(t, http.StatusOK, w.Code)
			assert.Equal(t, tc.want, subject)
		})
	}
}

func TestOwnerSubject_RejectsNonStringIdentifiers(t *testing.T) {
	authenticate, key := newTestAuthn(t)
	for name, value := range map[string]any{
		"null": nil, "blank": " \t ", "number": 123, "boolean": true,
		"array": []string{"199012345678"}, "object": map[string]any{"nic": "199012345678"},
	} {
		t.Run(name, func(t *testing.T) {
			w, subject := serveAuthenticated(t, authenticate, "nic",
				"Bearer "+signToken(t, key, jwt.MapClaims{"nic": value}))
			assert.Equal(t, http.StatusUnauthorized, w.Code)
			assert.Empty(t, subject)
		})
	}
	// Registered numeric claims must not be converted into owner IDs either.
	w, subject := serveAuthenticated(t, authenticate, "exp", "Bearer "+signToken(t, key, nil))
	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Empty(t, subject)
}

func TestOwnerSubject_UsesExactClaimName(t *testing.T) {
	authenticate, key := newTestAuthn(t)
	token := "Bearer " + signToken(t, key, jwt.MapClaims{"NIC": "upper", "nic": "lower"})
	for claim, want := range map[string]string{"NIC": "upper", "nic": "lower"} {
		w, subject := serveAuthenticated(t, authenticate, claim, token)
		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, want, subject)
	}
	w, subject := serveAuthenticated(t, authenticate, "Nic", token)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Empty(t, subject)
}

func TestOwnerSubject_RejectsTokenWithoutClaim(t *testing.T) {
	authenticate, key := newTestAuthn(t)

	w, subject := serveAuthenticated(t, authenticate, "nic", "Bearer "+signToken(t, key, nil))

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Contains(t, w.Body.String(), "UNAUTHORIZED")
	assert.Empty(t, subject)
}

func TestOwnerSubject_RejectsRequestsAuthnRejects(t *testing.T) {
	authenticate, key := newTestAuthn(t)
	otherKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	for name, authorization := range map[string]string{
		"missing header":  "",
		"not a bearer":    "Basic dXNlcjpwYXNz",
		"malformed token": "Bearer not-a-jwt",
		"wrong key":       "Bearer " + signToken(t, otherKey, nil),
		"wrong audience":  "Bearer " + signToken(t, key, jwt.MapClaims{"aud": "OTHER"}),
		"wrong issuer":    "Bearer " + signToken(t, key, jwt.MapClaims{"iss": "http://other.test"}),
		"expired":         "Bearer " + signToken(t, key, jwt.MapClaims{"exp": time.Now().Add(-time.Hour).Unix()}),
	} {
		t.Run(name, func(t *testing.T) {
			w, subject := serveAuthenticated(t, authenticate, SubjectClaim, authorization)

			assert.Equal(t, http.StatusUnauthorized, w.Code)
			assert.Empty(t, subject)
		})
	}
}

func TestOwnerSubject_RejectsUnauthenticatedContext(t *testing.T) {
	// Without authn in front, there is no principal to read the subject from.
	handler := OwnerSubject(SubjectClaim)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler must not run")
	}))
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/consents", nil))

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestGetOwnerSubjectFromContext(t *testing.T) {
	ctx := WithOwnerSubject(context.Background(), "user-123")
	subject, ok := GetOwnerSubjectFromContext(ctx)
	assert.True(t, ok)
	assert.Equal(t, "user-123", subject)
}

func TestGetOwnerSubjectFromContext_NotFound(t *testing.T) {
	subject, ok := GetOwnerSubjectFromContext(context.Background())
	assert.False(t, ok)
	assert.Empty(t, subject)
}

func TestGetOwnerSubjectFromContext_WrongType(t *testing.T) {
	ctx := context.WithValue(context.Background(), ownerSubjectKey, 123)
	subject, ok := GetOwnerSubjectFromContext(ctx)
	assert.False(t, ok)
	assert.Empty(t, subject)
}
