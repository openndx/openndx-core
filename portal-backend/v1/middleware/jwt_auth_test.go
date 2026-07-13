package middleware

import (
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

	authutils "github.com/gov-dx-sandbox/portal-backend/v1/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func generateTestKeys(t *testing.T) (*rsa.PrivateKey, *rsa.PublicKey) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	return privateKey, &privateKey.PublicKey
}

func createJWKSResponse(t *testing.T, pubKey *rsa.PublicKey, kid string) []byte {
	n := base64.RawURLEncoding.EncodeToString(pubKey.N.Bytes())
	e := base64.RawURLEncoding.EncodeToString(big.NewInt(int64(pubKey.E)).Bytes())

	jwks := JWKS{
		Keys: []JWK{
			{
				Kty: "RSA",
				Kid: kid,
				Use: "sig",
				Alg: "RS256",
				N:   n,
				E:   e,
			},
		},
	}

	data, err := json.Marshal(jwks)
	require.NoError(t, err)
	return data
}

func TestJWTAuthConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		config  JWTAuthConfig
		wantErr bool
	}{
		{
			name: "Valid config",
			config: JWTAuthConfig{
				JWKSURL:        "https://example.com/jwks",
				ExpectedIssuer: "https://example.com",
				ValidClientIDs: []string{"client-1"},
			},
			wantErr: false,
		},
		{
			name: "Missing JWKS URL",
			config: JWTAuthConfig{
				ExpectedIssuer: "https://example.com",
				ValidClientIDs: []string{"client-1"},
			},
			wantErr: true,
		},
		{
			name: "Missing Issuer",
			config: JWTAuthConfig{
				JWKSURL:        "https://example.com/jwks",
				ValidClientIDs: []string{"client-1"},
			},
			wantErr: true,
		},
		{
			name: "Missing Client IDs",
			config: JWTAuthConfig{
				JWKSURL:        "https://example.com/jwks",
				ExpectedIssuer: "https://example.com",
			},
			wantErr: true,
		},
		{
			name: "Empty Client ID",
			config: JWTAuthConfig{
				JWKSURL:        "https://example.com/jwks",
				ExpectedIssuer: "https://example.com",
				ValidClientIDs: []string{""},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestJWTAuthMiddleware_AuthenticateJWT(t *testing.T) {
	privKey, pubKey := generateTestKeys(t)
	kid := "test-key-1"

	// Setup mock JWKS server
	jwksServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(createJWKSResponse(t, pubKey, kid))
	}))
	defer jwksServer.Close()

	config := JWTAuthConfig{
		JWKSURL:        jwksServer.URL,
		ExpectedIssuer: "https://example.com",
		ValidClientIDs: []string{"client-1"},
	}

	middleware := NewJWTAuthMiddleware(config)

	// Helper to create token
	createToken := func(claims jwt.MapClaims, signKey *rsa.PrivateKey, keyID string) string {
		token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
		token.Header["kid"] = keyID
		tokenString, err := token.SignedString(signKey)
		require.NoError(t, err)
		return tokenString
	}

	tests := []struct {
		name           string
		setupRequest   func() *http.Request
		expectedStatus int
	}{
		{
			name: "Success",
			setupRequest: func() *http.Request {
				claims := jwt.MapClaims{
					"iss":       "https://example.com",
					"aud":       "client-1",
					"email":     "test@example.com",
					"sub":       "user-1",
					"exp":       time.Now().Add(time.Hour).Unix(),
					"iat":       time.Now().Unix(),
					"scope":     "read write",
					"client_id": "client-1",
					"username":  "testuser",
					"roles":     []string{"OpenNDX_Member"},
				}
				token := createToken(claims, privKey, kid)
				req := httptest.NewRequest("GET", "/api/v1/resource", nil)
				req.Header.Set("Authorization", "Bearer "+token)
				return req
			},
			expectedStatus: http.StatusOK,
		},
		{
			name: "Missing Token",
			setupRequest: func() *http.Request {
				return httptest.NewRequest("GET", "/api/v1/resource", nil)
			},
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name: "Invalid Token",
			setupRequest: func() *http.Request {
				req := httptest.NewRequest("GET", "/api/v1/resource", nil)
				req.Header.Set("Authorization", "Bearer invalid-token")
				return req
			},
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name: "Expired Token",
			setupRequest: func() *http.Request {
				claims := jwt.MapClaims{
					"iss":   "https://example.com",
					"aud":   "client-1",
					"email": "test@example.com",
					"sub":   "user-1",
					"exp":   time.Now().Add(-time.Hour).Unix(),
				}
				token := createToken(claims, privKey, kid)
				req := httptest.NewRequest("GET", "/api/v1/resource", nil)
				req.Header.Set("Authorization", "Bearer "+token)
				return req
			},
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name: "Invalid Issuer",
			setupRequest: func() *http.Request {
				claims := jwt.MapClaims{
					"iss":   "https://wrong-issuer.com",
					"aud":   "client-1",
					"email": "test@example.com",
					"sub":   "user-1",
					"exp":   time.Now().Add(time.Hour).Unix(),
				}
				token := createToken(claims, privKey, kid)
				req := httptest.NewRequest("GET", "/api/v1/resource", nil)
				req.Header.Set("Authorization", "Bearer "+token)
				return req
			},
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name: "Invalid Audience",
			setupRequest: func() *http.Request {
				claims := jwt.MapClaims{
					"iss":   "https://example.com",
					"aud":   "wrong-client",
					"email": "test@example.com",
					"sub":   "user-1",
					"exp":   time.Now().Add(time.Hour).Unix(),
				}
				token := createToken(claims, privKey, kid)
				req := httptest.NewRequest("GET", "/api/v1/resource", nil)
				req.Header.Set("Authorization", "Bearer "+token)
				return req
			},
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name: "Skip Auth Path",
			setupRequest: func() *http.Request {
				return httptest.NewRequest("GET", "/health", nil)
			},
			expectedStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := tt.setupRequest()
			w := httptest.NewRecorder()

			handler := middleware.AuthenticateJWT(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)

				// Verify context is set for authenticated requests
				if tt.expectedStatus == http.StatusOK && req.URL.Path != "/health" {
					user, err := authutils.GetAuthenticatedUser(r.Context())
					assert.NoError(t, err)
					assert.NotNil(t, user)
					assert.Equal(t, "user-1", user.IdpUserID)
				}
			}))

			handler.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)
		})
	}
}
