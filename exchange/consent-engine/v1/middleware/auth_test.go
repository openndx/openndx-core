package middleware

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/gov-dx-sandbox/exchange/consent-engine/v1/auth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func createTestJWTVerifier(t *testing.T, privateKey *rsa.PrivateKey, issuer, audience string) *auth.JWTVerifier {
	// Create a mock JWKS server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Create JWKS response
		nBytes := privateKey.PublicKey.N.Bytes()
		eBytes := make([]byte, 4)
		e := privateKey.PublicKey.E
		for i := len(eBytes) - 1; i >= 0; i-- {
			eBytes[i] = byte(e)
			e >>= 8
		}

		jwks := map[string]interface{}{
			"keys": []map[string]interface{}{
				{
					"kid": "test-key-id",
					"kty": "RSA",
					"use": "sig",
					"n":   base64.RawURLEncoding.EncodeToString(nBytes),
					"e":   base64.RawURLEncoding.EncodeToString(eBytes),
				},
			},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(jwks)
	}))
	t.Cleanup(server.Close)

	config := auth.JWTVerifierConfig{
		JWKSUrl:      server.URL,
		Issuer:       issuer,
		Audience:     audience,
		Organization: "test-org",
	}

	verifier, err := auth.NewJWTVerifier(config)
	require.NoError(t, err)

	// Wait for JWKS to be ready by attempting token verification with retry logic.
	// The getPublicKey() method will automatically trigger a JWKS fetch if keys aren't loaded yet.
	// This approach doesn't require any changes to jwt_verifier.go.
	// Create a test token inline to verify JWKS is loaded
	claims := jwt.MapClaims{
		"iss":      issuer,
		"aud":      audience,
		"email":    "test@example.com",
		"org_name": "test-org",
		"exp":      time.Now().Add(time.Hour).Unix(),
		"iat":      time.Now().Unix(),
	}
	testToken := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	testToken.Header["kid"] = "test-key-id"
	testTokenString, err := testToken.SignedString(privateKey)
	require.NoError(t, err)

	maxRetries := 10
	retryDelay := 50 * time.Millisecond

	for i := 0; i < maxRetries; i++ {
		_, err := verifier.VerifyToken(testTokenString)
		if err == nil {
			// JWKS is loaded and token verification succeeded
			return verifier
		}
		// Check if error is due to missing keys (will trigger fetch) or other issues
		if i < maxRetries-1 {
			time.Sleep(retryDelay)
		}
	}

	// Final attempt - if this fails, the test will fail with a clear error
	_, err = verifier.VerifyToken(testTokenString)
	require.NoError(t, err, "JWKS should be loaded and token should verify within timeout")

	return verifier
}

func createTestToken(t *testing.T, privateKey *rsa.PrivateKey, issuer, audience, email string) string {
	claims := jwt.MapClaims{
		"iss":      issuer,
		"aud":      audience,
		"email":    email,
		"org_name": "test-org",
		"exp":      time.Now().Add(time.Hour).Unix(),
		"iat":      time.Now().Unix(),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = "test-key-id"

	tokenString, err := token.SignedString(privateKey)
	require.NoError(t, err)

	return tokenString
}

func TestNewJWTAuthMiddleware(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	verifier := createTestJWTVerifier(t, privateKey, "test-issuer", "test-audience")
	middleware := NewJWTAuthMiddleware(verifier)

	assert.NotNil(t, middleware)
	assert.Equal(t, verifier, middleware.verifier)
}

func TestJWTAuthMiddleware_Authenticate_Success(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	verifier := createTestJWTVerifier(t, privateKey, "test-issuer", "test-audience")
	middleware := NewJWTAuthMiddleware(verifier)

	token := createTestToken(t, privateKey, "test-issuer", "test-audience", "user@example.com")

	req := httptest.NewRequest("GET", "/api/v1/consents/123", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()

	nextCalled := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
		email, ok := GetUserEmailFromContext(r.Context())
		assert.True(t, ok)
		assert.Equal(t, "user@example.com", email)
	})

	middleware.Authenticate(next).ServeHTTP(w, req)

	assert.True(t, nextCalled)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestJWTAuthMiddleware_Authenticate_MissingHeader(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	verifier := createTestJWTVerifier(t, privateKey, "test-issuer", "test-audience")
	middleware := NewJWTAuthMiddleware(verifier)

	req := httptest.NewRequest("GET", "/api/v1/consents/123", nil)
	w := httptest.NewRecorder()

	nextCalled := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
	})

	middleware.Authenticate(next).ServeHTTP(w, req)

	assert.False(t, nextCalled)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestJWTAuthMiddleware_Authenticate_InvalidFormat(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	verifier := createTestJWTVerifier(t, privateKey, "test-issuer", "test-audience")
	middleware := NewJWTAuthMiddleware(verifier)

	req := httptest.NewRequest("GET", "/api/v1/consents/123", nil)
	req.Header.Set("Authorization", "InvalidFormat token")
	w := httptest.NewRecorder()

	nextCalled := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
	})

	middleware.Authenticate(next).ServeHTTP(w, req)

	assert.False(t, nextCalled)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestJWTAuthMiddleware_Authenticate_EmptyToken(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	verifier := createTestJWTVerifier(t, privateKey, "test-issuer", "test-audience")
	middleware := NewJWTAuthMiddleware(verifier)

	req := httptest.NewRequest("GET", "/api/v1/consents/123", nil)
	req.Header.Set("Authorization", "Bearer ")
	w := httptest.NewRecorder()

	nextCalled := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
	})

	middleware.Authenticate(next).ServeHTTP(w, req)

	assert.False(t, nextCalled)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestJWTAuthMiddleware_Authenticate_InvalidToken(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	verifier := createTestJWTVerifier(t, privateKey, "test-issuer", "test-audience")
	middleware := NewJWTAuthMiddleware(verifier)

	req := httptest.NewRequest("GET", "/api/v1/consents/123", nil)
	req.Header.Set("Authorization", "Bearer invalid.token.here")
	w := httptest.NewRecorder()

	nextCalled := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
	})

	middleware.Authenticate(next).ServeHTTP(w, req)

	assert.False(t, nextCalled)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestGetUserEmailFromContext(t *testing.T) {
	ctx := context.WithValue(context.Background(), userEmailKey, "user@example.com")

	email, ok := GetUserEmailFromContext(ctx)
	assert.True(t, ok)
	assert.Equal(t, "user@example.com", email)
}

func TestGetUserEmailFromContext_NotFound(t *testing.T) {
	ctx := context.Background()

	email, ok := GetUserEmailFromContext(ctx)
	assert.False(t, ok)
	assert.Empty(t, email)
}

func TestGetUserEmailFromContext_WrongType(t *testing.T) {
	ctx := context.WithValue(context.Background(), userEmailKey, 123)

	email, ok := GetUserEmailFromContext(ctx)
	assert.False(t, ok)
	assert.Empty(t, email)
}

