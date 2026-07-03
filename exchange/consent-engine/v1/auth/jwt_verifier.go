package auth

import (
	"context"
	"crypto/rsa"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math/big"
	"net/http"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// JWKS represents the JSON Web Key Set structure
type JWKS struct {
	Keys []JSONWebKey `json:"keys"`
}

// JSONWebKey represents a single key in the JWKS
type JSONWebKey struct {
	Kid string `json:"kid"` // Key ID
	Kty string `json:"kty"` // Key Type (e.g., "RSA")
	Use string `json:"use"` // Key Use (e.g., "sig")
	N   string `json:"n"`   // Modulus
	E   string `json:"e"`   // Exponent
}

// JWTVerifierConfig holds configuration for the JWT verifier.
// Each check is applied only when its value is configured (non-empty), so the
// verifier stays generic across identity providers.
type JWTVerifierConfig struct {
	JWKSUrl  string
	Issuer   string
	Audience string
	ClientID string
	// InsecureSkipVerify disables TLS certificate verification for the JWKS
	// fetch. Intended only for local/dev IdPs using self-signed certs; leave
	// false in production.
	InsecureSkipVerify bool
}

// JWTVerifier handles JWT token verification
type JWTVerifier struct {
	config        JWTVerifierConfig
	keys          map[string]*rsa.PublicKey
	keyMutex      sync.RWMutex
	lastFetchTime time.Time
	logger        *slog.Logger
	httpClient    *http.Client
}

// NewJWTVerifier creates a new JWT verifier instance
func NewJWTVerifier(config JWTVerifierConfig) (*JWTVerifier, error) {
	httpClient := &http.Client{Timeout: 10 * time.Second}
	if config.InsecureSkipVerify {
		// Clone the default transport to preserve connection pooling, keep-alives
		// and proxy settings, overriding only TLS verification.
		transport := http.DefaultTransport.(*http.Transport).Clone()
		// #nosec G402 -- opt-in via config, dev-only for self-signed IdP certs; default is false.
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
		httpClient.Transport = transport
	}

	verifier := &JWTVerifier{
		config:     config,
		keys:       make(map[string]*rsa.PublicKey),
		logger:     slog.Default(),
		httpClient: httpClient,
	}

	// Initial fetch of JWKS (non-blocking to prevent startup failure)
	go func() {
		if err := verifier.fetchJWKS(); err != nil {
			verifier.logger.Warn("Failed to perform initial JWKS fetch", "error", err)
		}
	}()

	return verifier, nil
}

// fetchJWKS retrieves and caches the public keys from the JWKS endpoint
func (jv *JWTVerifier) fetchJWKS() error {
	jv.keyMutex.Lock()

	// Check if we need to refresh (refresh every hour)
	if time.Since(jv.lastFetchTime) < time.Hour && len(jv.keys) > 0 {
		jv.keyMutex.Unlock()
		return nil
	}

	defer jv.keyMutex.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, jv.config.JWKSUrl, nil)
	if err != nil {
		return fmt.Errorf("failed to create JWKS request: %w", err)
	}

	resp, err := jv.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to fetch JWKS: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("JWKS endpoint returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read JWKS response: %w", err)
	}

	var jwks JWKS
	if err := json.Unmarshal(body, &jwks); err != nil {
		return fmt.Errorf("failed to unmarshal JWKS: %w", err)
	}

	// Clear old keys and add new ones
	jv.keys = make(map[string]*rsa.PublicKey)
	for _, key := range jwks.Keys {
		if key.Kty != "RSA" {
			continue
		}

		publicKey, err := jv.buildRSAPublicKey(key)
		if err != nil {
			jv.logger.Warn("failed to build RSA public key", "kid", key.Kid, "error", err)
			continue
		}

		jv.keys[key.Kid] = publicKey
	}

	jv.lastFetchTime = time.Now()
	jv.logger.Info("JWKS refreshed", "key_count", len(jv.keys))

	return nil
}

// buildRSAPublicKey constructs an RSA public key from a JWK
func (jv *JWTVerifier) buildRSAPublicKey(key JSONWebKey) (*rsa.PublicKey, error) {
	// Decode modulus
	nBytes, err := base64.RawURLEncoding.DecodeString(key.N)
	if err != nil {
		return nil, fmt.Errorf("failed to decode modulus: %w", err)
	}

	// Decode exponent
	eBytes, err := base64.RawURLEncoding.DecodeString(key.E)
	if err != nil {
		return nil, fmt.Errorf("failed to decode exponent: %w", err)
	}

	// Convert bytes to big.Int
	n := new(big.Int).SetBytes(nBytes)
	var e int
	for _, b := range eBytes {
		e = e<<8 + int(b)
	}

	return &rsa.PublicKey{
		N: n,
		E: e,
	}, nil
}

// getPublicKey retrieves the public key for a given key ID
func (jv *JWTVerifier) getPublicKey(kid string) (*rsa.PublicKey, error) {
	// First, try to get the key with a read lock
	jv.keyMutex.RLock()
	key, exists := jv.keys[kid]
	jv.keyMutex.RUnlock()

	if exists {
		return key, nil
	}

	// Key not found, acquire write lock to ensure only one goroutine refreshes
	jv.keyMutex.Lock()

	// Check again in case another goroutine already refreshed
	key, exists = jv.keys[kid]
	if exists {
		jv.keyMutex.Unlock()
		return key, nil
	}

	// Release the lock before fetching (fetchJWKS will acquire its own lock)
	jv.keyMutex.Unlock()

	// Refresh JWKS
	if err := jv.fetchJWKS(); err != nil {
		return nil, fmt.Errorf("failed to refresh JWKS: %w", err)
	}

	// Check one more time after refresh
	jv.keyMutex.RLock()
	key, exists = jv.keys[kid]
	jv.keyMutex.RUnlock()

	if !exists {
		return nil, fmt.Errorf("key with kid '%s' not found in JWKS", kid)
	}

	return key, nil
}

// VerifyToken verifies a JWT token and returns the parsed token.
//
// Standard registered claims are validated by the jwt/v5 parser: the RS256
// signing method, the signature (via the JWKS key func), expiry/not-before, and
// — when configured — the issuer and audience. Audience validation natively
// accepts both the string and array (RFC 7519) forms. The only non-standard,
// optional check is client_id; there is deliberately no IdP-specific claim (e.g.
// organization) so the verifier stays generic across identity providers.
func (jv *JWTVerifier) VerifyToken(tokenString string) (*jwt.Token, error) {
	keyFunc := func(token *jwt.Token) (interface{}, error) {
		kid, ok := token.Header["kid"].(string)
		if !ok {
			return nil, fmt.Errorf("token missing 'kid' header")
		}
		return jv.getPublicKey(kid)
	}

	// Build parser options; issuer/audience are enforced only when configured.
	opts := []jwt.ParserOption{jwt.WithValidMethods([]string{"RS256"})}
	if jv.config.Issuer != "" {
		opts = append(opts, jwt.WithIssuer(jv.config.Issuer))
	}
	if jv.config.Audience != "" {
		opts = append(opts, jwt.WithAudience(jv.config.Audience))
	}

	token, err := jwt.Parse(tokenString, keyFunc, opts...)
	if err != nil {
		return nil, fmt.Errorf("token verification failed: %w", err)
	}
	if !token.Valid {
		return nil, fmt.Errorf("token is invalid")
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return nil, fmt.Errorf("invalid token claims")
	}

	// Verify client_id when configured. Prefer the "client_id" claim, falling
	// back to "azp" for IdPs that use it.
	if jv.config.ClientID != "" {
		cid, _ := claims["client_id"].(string)
		if cid == "" {
			cid, _ = claims["azp"].(string)
		}
		if cid != jv.config.ClientID {
			return nil, fmt.Errorf("invalid client_id: expected %s, got %q", jv.config.ClientID, cid)
		}
	}

	return token, nil
}

// VerifyTokenAndExtractEmail verifies the token and extracts the email claim
func (jv *JWTVerifier) VerifyTokenAndExtractEmail(tokenString string) (string, error) {
	token, err := jv.VerifyToken(tokenString)
	if err != nil {
		return "", err
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return "", fmt.Errorf("invalid token claims")
	}

	email, ok := claims["email"].(string)
	if !ok || email == "" {
		return "", fmt.Errorf("email claim not found or empty in token")
	}

	return email, nil
}
