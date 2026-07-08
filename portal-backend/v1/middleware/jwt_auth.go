package middleware

import (
	"context"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	sharedutils "github.com/gov-dx-sandbox/portal-backend/shared/utils"
	"github.com/gov-dx-sandbox/portal-backend/v1/models"
	authutils "github.com/gov-dx-sandbox/portal-backend/v1/utils"
)

// JWKS represents the JSON Web Key Set structure
type JWKS struct {
	Keys []JWK `json:"keys"`
}

// JWK represents a single JSON Web Key
type JWK struct {
	Kty string `json:"kty"`
	Kid string `json:"kid"`
	Use string `json:"use"`
	Alg string `json:"alg"`
	N   string `json:"n"`
	E   string `json:"e"`
}

// JWTAuthMiddleware provides JWT authentication functionality
// Thread-safe: All methods can be called concurrently from multiple goroutines
type JWTAuthMiddleware struct {
	jwksURL        string
	expectedIssuer string
	validClientIDs []string
	httpClient     *http.Client

	// Protected by keysMutex to prevent race conditions
	// keysMutex guards both keys map and lastFetch time to ensure atomic updates
	keysMutex sync.RWMutex
	keys      map[string]*rsa.PublicKey
	lastFetch time.Time
}

// JWTAuthConfig contains configuration for JWT authentication
type JWTAuthConfig struct {
	JWKSURL        string
	ExpectedIssuer string
	ValidClientIDs []string // Multiple valid client IDs for different portals
	Timeout        time.Duration
}

// Validate checks if the JWT configuration is valid
func (c JWTAuthConfig) Validate() error {
	if c.JWKSURL == "" {
		return fmt.Errorf("JWKSURL is required for JWT authentication")
	}

	if c.ExpectedIssuer == "" {
		return fmt.Errorf("ExpectedIssuer is required for JWT authentication")
	}

	if len(c.ValidClientIDs) == 0 {
		return fmt.Errorf("at least one ValidClientID is required for JWT authentication")
	}

	// Check that all client IDs are non-empty
	for i, clientID := range c.ValidClientIDs {
		if strings.TrimSpace(clientID) == "" {
			return fmt.Errorf("ValidClientID at index %d is empty", i)
		}
	}

	return nil
}

// NewJWTAuthMiddleware creates a new JWT authentication middleware
func NewJWTAuthMiddleware(config JWTAuthConfig) *JWTAuthMiddleware {
	timeout := config.Timeout
	if timeout == 0 {
		timeout = 10 * time.Second
	}

	return &JWTAuthMiddleware{
		jwksURL:        config.JWKSURL,
		expectedIssuer: config.ExpectedIssuer,
		validClientIDs: config.ValidClientIDs,
		httpClient: &http.Client{
			Timeout: timeout,
		},
		keys: make(map[string]*rsa.PublicKey),
	}
}

// AuthenticateJWT returns a middleware function that validates JWT tokens
func (j *JWTAuthMiddleware) AuthenticateJWT(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Skip authentication for health and debug endpoints
		if j.shouldSkipAuth(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}

		// Extract token from Authorization header
		tokenString, err := authutils.ExtractBearerToken(r)
		if err != nil {
			slog.Warn("Failed to extract bearer token", "error", err, "path", r.URL.Path, "method", r.Method)
			sharedutils.RespondWithError(w, http.StatusUnauthorized, "Invalid or missing authorization header")
			return
		}

		// Validate and parse the token
		user, authCtx, err := j.validateToken(tokenString)
		if err != nil {
			slog.Warn("Token validation failed", "error", err, "path", r.URL.Path, "method", r.Method)
			sharedutils.RespondWithError(w, http.StatusUnauthorized, "Invalid access token")
			return
		}

		// Check if token is expired
		if user.IsTokenExpired() {
			slog.Warn("Token is expired", "expiry", user.ExpiresAt, "user", user.Email)
			sharedutils.RespondWithError(w, http.StatusUnauthorized, "Access token has expired")
			return
		}

		// Add user and auth context to request context
		ctx := authutils.SetAuthenticatedUser(r.Context(), user)
		ctx = authutils.SetAuthContext(ctx, authCtx)

		// Log successful authentication
		slog.Info("User authenticated successfully",
			"user_id", user.IdpUserID,
			"email", user.Email,
			"roles", user.Roles,
			"path", r.URL.Path,
			"method", r.Method)

		// Continue to the next handler with updated context
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// validateToken validates a JWT token and returns the authenticated user
func (j *JWTAuthMiddleware) validateToken(tokenString string) (*models.AuthenticatedUser, *models.AuthContext, error) {
	// Ensure we have fresh JWKS keys
	if err := j.ensureKeysFresh(); err != nil {
		return nil, nil, fmt.Errorf("failed to ensure fresh keys: %w", err)
	}

	// Parse and validate the token
	token, err := jwt.ParseWithClaims(tokenString, &models.UserClaims{}, func(token *jwt.Token) (interface{}, error) {
		// Verify signing method
		if _, ok := token.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}

		// Get key ID from token header
		kid, ok := token.Header["kid"].(string)
		if !ok {
			return nil, fmt.Errorf("missing 'kid' in token header")
		}

		// Find the public key with read lock
		j.keysMutex.RLock()
		publicKey, exists := j.keys[kid]
		j.keysMutex.RUnlock()

		if !exists {
			// Try to refresh keys once
			slog.Info("Key not found, refreshing JWKS", "kid", kid)
			if err := j.fetchJWKS(); err != nil {
				return nil, fmt.Errorf("failed to refresh JWKS: %w", err)
			}

			// Check again after refresh with read lock
			j.keysMutex.RLock()
			publicKey, exists = j.keys[kid]
			j.keysMutex.RUnlock()

			if !exists {
				return nil, fmt.Errorf("no public key found for kid: %s", kid)
			}
		}

		return publicKey, nil
	})
	if err != nil {
		return nil, nil, fmt.Errorf("failed to parse token: %w", err)
	}

	// Extract claims
	claims, ok := token.Claims.(*models.UserClaims)
	if !ok || !token.Valid {
		return nil, nil, fmt.Errorf("invalid token claims")
	}

	// Validate standard claims
	if err := j.validateStandardClaims(claims); err != nil {
		return nil, nil, fmt.Errorf("claim validation failed: %w", err)
	}

	// Create authenticated user from claims
	user, err := models.NewAuthenticatedUser(claims)
	if err != nil {
		return nil, nil, fmt.Errorf("user creation failed: %w", err)
	}

	// Create auth context
	authCtx := &models.AuthContext{
		User:        user,
		Token:       tokenString,
		IssuedBy:    claims.Issuer,
		Audience:    claims.Audience.ToStringSlice(),
		Permissions: user.GetPermissions(),
	}

	return user, authCtx, nil
}

// validateStandardClaims validates the standard JWT claims
func (j *JWTAuthMiddleware) validateStandardClaims(claims *models.UserClaims) error {
	now := time.Now()

	// Check if token is expired
	if claims.ExpiresAt != 0 && now.After(time.Unix(claims.ExpiresAt, 0)) {
		return fmt.Errorf("token is expired")
	}

	// Check not before time
	if claims.NotBefore != 0 && now.Before(time.Unix(claims.NotBefore, 0)) {
		return fmt.Errorf("token is not valid yet")
	}

	// Validate issuer
	if j.expectedIssuer != "" && claims.Issuer != j.expectedIssuer {
		return fmt.Errorf("invalid issuer: expected %s, got %s", j.expectedIssuer, claims.Issuer)
	}

	// Validate client ID (audience)
	if len(j.validClientIDs) > 0 && !j.containsValidClientID(claims.Audience) {
		return fmt.Errorf("invalid audience: expected one of %v, got %v", j.validClientIDs, claims.Audience)
	}

	// Validate required fields
	if claims.Email == "" {
		return fmt.Errorf("email claim is missing")
	}

	if claims.IdpUserID == "" {
		return fmt.Errorf("subject claim is missing")
	}

	return nil
}

// containsValidClientID checks if the audience list contains any of the valid client IDs
func (j *JWTAuthMiddleware) containsValidClientID(audiences models.FlexibleStringSlice) bool {
	audienceSlice := audiences.ToStringSlice()
	for _, aud := range audienceSlice {
		for _, validClientID := range j.validClientIDs {
			if aud == validClientID {
				return true
			}
		}
	}
	return false
}

// fetchJWKS fetches the JWKS from the configured endpoint
// Thread-safe: Updates keys and lastFetch atomically under write lock
func (j *JWTAuthMiddleware) fetchJWKS() error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", j.jwksURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := j.httpClient.Do(req)
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
		return fmt.Errorf("failed to parse JWKS: %w", err)
	}

	// Build new keys map first, then update with write lock
	newKeys := make(map[string]*rsa.PublicKey)

	// Process each key
	for _, key := range jwks.Keys {
		if key.Kty == "RSA" && key.Use == "sig" {
			publicKey, err := j.buildRSAPublicKey(key.N, key.E)
			if err != nil {
				slog.Warn("Failed to build RSA public key", "kid", key.Kid, "error", err)
				continue
			}
			newKeys[key.Kid] = publicKey
		}
	}

	// Update keys and lastFetch atomically with write lock
	j.keysMutex.Lock()
	j.keys = newKeys
	j.lastFetch = time.Now()
	j.keysMutex.Unlock()

	slog.Info("Successfully fetched JWKS", "keys_count", len(newKeys))
	return nil
}

// buildRSAPublicKey constructs an RSA public key from modulus and exponent
func (j *JWTAuthMiddleware) buildRSAPublicKey(nStr, eStr string) (*rsa.PublicKey, error) {
	// Decode base64url encoded modulus
	nBytes, err := base64.RawURLEncoding.DecodeString(nStr)
	if err != nil {
		return nil, fmt.Errorf("failed to decode modulus: %w", err)
	}

	// Decode base64url encoded exponent
	eBytes, err := base64.RawURLEncoding.DecodeString(eStr)
	if err != nil {
		return nil, fmt.Errorf("failed to decode exponent: %w", err)
	}

	// Convert bytes to big integers
	n := new(big.Int).SetBytes(nBytes)
	e := new(big.Int).SetBytes(eBytes)

	// Validate RSA modulus size for cryptographic strength
	if n.BitLen() < 2048 {
		return nil, fmt.Errorf("RSA modulus too small: %d bits, minimum 2048 required", n.BitLen())
	}

	// Validate exponent
	if !e.IsInt64() || e.Int64() < 2 {
		return nil, fmt.Errorf("invalid exponent")
	}

	return &rsa.PublicKey{
		N: n,
		E: int(e.Int64()),
	}, nil
}

// ensureKeysFresh ensures we have fresh JWKS keys (refreshes if older than 1 hour)
// Thread-safe: Uses read lock to check freshness, delegates to fetchJWKS for updates
func (j *JWTAuthMiddleware) ensureKeysFresh() error {
	// Check if refresh is needed with read lock
	j.keysMutex.RLock()
	needsRefresh := len(j.keys) == 0 || time.Since(j.lastFetch) > time.Hour
	j.keysMutex.RUnlock()

	if needsRefresh {
		return j.fetchJWKS()
	}
	return nil
}

// shouldSkipAuth determines if authentication should be skipped for this path
func (j *JWTAuthMiddleware) shouldSkipAuth(path string) bool {
	skipPaths := []string{
		"/health",
		"/debug",
		"/openapi.yaml",
		"/favicon.ico",
	}

	for _, skipPath := range skipPaths {
		if strings.HasPrefix(path, skipPath) {
			return true
		}
	}
	return false
}
