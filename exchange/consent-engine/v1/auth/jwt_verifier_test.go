package auth

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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testKID = "test-key-1"

// newTestVerifier spins up a mock JWKS server serving the public half of a
// freshly generated RSA keypair, and returns a verifier wired to it plus the
// private key used to sign test tokens.
func newTestVerifier(t *testing.T, cfg JWTVerifierConfig) (*JWTVerifier, *rsa.PrivateKey) {
	t.Helper()

	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	jwks := JWKS{Keys: []JSONWebKey{{
		Kid: testKID,
		Kty: "RSA",
		Use: "sig",
		N:   base64.RawURLEncoding.EncodeToString(priv.N.Bytes()),
		E:   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(priv.E)).Bytes()),
	}}}
	body, err := json.Marshal(jwks)
	require.NoError(t, err)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)

	cfg.JWKSUrl = srv.URL
	v, err := NewJWTVerifier(cfg)
	require.NoError(t, err)
	return v, priv
}

// signToken signs claims with the given key and the test kid.
func signToken(t *testing.T, priv *rsa.PrivateKey, claims jwt.MapClaims) string {
	t.Helper()
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	tok.Header["kid"] = testKID
	s, err := tok.SignedString(priv)
	require.NoError(t, err)
	return s
}

// baseClaims returns a valid ThunderID-shaped claim set (string aud, no org_name).
func baseClaims() jwt.MapClaims {
	now := time.Now()
	return jwt.MapClaims{
		"iss":       "https://thunderid:8090",
		"aud":       "CONSENT_PORTAL_APP",
		"client_id": "CONSENT_PORTAL_APP",
		"email":     "nayana@opensource.lk",
		"sub":       "user-123",
		"iat":       now.Unix(),
		"nbf":       now.Unix(),
		"exp":       now.Add(time.Hour).Unix(),
	}
}

func fullConfig() JWTVerifierConfig {
	return JWTVerifierConfig{
		Issuer:   "https://thunderid:8090",
		Audience: "CONSENT_PORTAL_APP",
		ClientID: "CONSENT_PORTAL_APP",
	}
}

func TestVerifyToken_ThunderIDShapedTokenPasses(t *testing.T) {
	v, priv := newTestVerifier(t, fullConfig())
	email, err := v.VerifyTokenAndExtractEmail(signToken(t, priv, baseClaims()))
	require.NoError(t, err)
	assert.Equal(t, "nayana@opensource.lk", email)
}

func TestVerifyToken_AudienceAsArrayPasses(t *testing.T) {
	v, priv := newTestVerifier(t, fullConfig())
	claims := baseClaims()
	claims["aud"] = []string{"other-app", "CONSENT_PORTAL_APP"}
	_, err := v.VerifyToken(signToken(t, priv, claims))
	assert.NoError(t, err)
}

func TestVerifyToken_OrgClaimIsIgnored(t *testing.T) {
	// A token carrying an org_name claim must still be accepted — the verifier
	// is IdP-generic and does not look at organization.
	v, priv := newTestVerifier(t, fullConfig())
	claims := baseClaims()
	claims["org_name"] = "SomeOtherOrg"
	_, err := v.VerifyToken(signToken(t, priv, claims))
	assert.NoError(t, err)
}

func TestVerifyToken_WrongIssuerFails(t *testing.T) {
	v, priv := newTestVerifier(t, fullConfig())
	claims := baseClaims()
	claims["iss"] = "https://evil.example.com"
	_, err := v.VerifyToken(signToken(t, priv, claims))
	assert.Error(t, err)
}

func TestVerifyToken_WrongAudienceFails(t *testing.T) {
	v, priv := newTestVerifier(t, fullConfig())
	claims := baseClaims()
	claims["aud"] = "SOME_OTHER_APP"
	_, err := v.VerifyToken(signToken(t, priv, claims))
	assert.Error(t, err)
}

func TestVerifyToken_WrongClientIDFails(t *testing.T) {
	v, priv := newTestVerifier(t, fullConfig())
	claims := baseClaims()
	claims["client_id"] = "SOME_OTHER_APP"
	_, err := v.VerifyToken(signToken(t, priv, claims))
	assert.Error(t, err)
}

func TestVerifyToken_ClientIDAzpFallback(t *testing.T) {
	// When client_id is absent but azp carries it, the check should pass.
	v, priv := newTestVerifier(t, fullConfig())
	claims := baseClaims()
	delete(claims, "client_id")
	claims["azp"] = "CONSENT_PORTAL_APP"
	_, err := v.VerifyToken(signToken(t, priv, claims))
	assert.NoError(t, err)
}

func TestVerifyToken_ClientIDCheckSkippedWhenUnconfigured(t *testing.T) {
	cfg := fullConfig()
	cfg.ClientID = "" // not configured -> skip client_id check entirely
	v, priv := newTestVerifier(t, cfg)
	claims := baseClaims()
	claims["client_id"] = "anything-goes"
	_, err := v.VerifyToken(signToken(t, priv, claims))
	assert.NoError(t, err)
}

func TestVerifyToken_ExpiredTokenFails(t *testing.T) {
	v, priv := newTestVerifier(t, fullConfig())
	claims := baseClaims()
	claims["exp"] = time.Now().Add(-time.Hour).Unix()
	_, err := v.VerifyToken(signToken(t, priv, claims))
	assert.Error(t, err)
}

func TestVerifyToken_NonRSAMethodFails(t *testing.T) {
	v, _ := newTestVerifier(t, fullConfig())
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, baseClaims())
	tok.Header["kid"] = testKID
	signed, err := tok.SignedString([]byte("symmetric-secret"))
	require.NoError(t, err)
	_, err = v.VerifyToken(signed)
	assert.Error(t, err)
}

func TestVerifyTokenAndExtractEmail_MissingEmailFails(t *testing.T) {
	v, priv := newTestVerifier(t, fullConfig())
	claims := baseClaims()
	delete(claims, "email")
	_, err := v.VerifyTokenAndExtractEmail(signToken(t, priv, claims))
	assert.Error(t, err)
}

// newTLSTestVerifier serves the JWKS over a self-signed HTTPS server (like a
// dev IdP), so JWKS fetches fail TLS verification unless InsecureSkipVerify is set.
func newTLSTestVerifier(t *testing.T, cfg JWTVerifierConfig) (*JWTVerifier, *rsa.PrivateKey) {
	t.Helper()

	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	jwks := JWKS{Keys: []JSONWebKey{{
		Kid: testKID,
		Kty: "RSA",
		Use: "sig",
		N:   base64.RawURLEncoding.EncodeToString(priv.N.Bytes()),
		E:   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(priv.E)).Bytes()),
	}}}
	body, err := json.Marshal(jwks)
	require.NoError(t, err)

	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)

	cfg.JWKSUrl = srv.URL
	v, err := NewJWTVerifier(cfg)
	require.NoError(t, err)
	return v, priv
}

func TestVerifyToken_SelfSignedJWKSFailsWithoutSkipVerify(t *testing.T) {
	cfg := fullConfig() // InsecureSkipVerify defaults to false
	v, priv := newTLSTestVerifier(t, cfg)
	_, err := v.VerifyToken(signToken(t, priv, baseClaims()))
	assert.Error(t, err) // JWKS fetch fails TLS verification -> cannot verify
}

func TestVerifyToken_SelfSignedJWKSPassesWithSkipVerify(t *testing.T) {
	cfg := fullConfig()
	cfg.InsecureSkipVerify = true
	v, priv := newTLSTestVerifier(t, cfg)
	email, err := v.VerifyTokenAndExtractEmail(signToken(t, priv, baseClaims()))
	require.NoError(t, err)
	assert.Equal(t, "nayana@opensource.lk", email)
}
