package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/MicahParks/keyfunc/v3"
	"github.com/OpenNDX/openndx-core/exchange/orchestration-engine/configs"
	"github.com/OpenNDX/openndx-core/exchange/orchestration-engine/logger"
	"github.com/golang-jwt/jwt/v5"
)

// httpClient is a shared HTTP client with reasonable timeouts
var httpClient = &http.Client{
	Timeout: 10 * time.Second,
	Transport: &http.Transport{
		MaxIdleConns:        100,
		IdleConnTimeout:     90 * time.Second,
		TLSHandshakeTimeout: 10 * time.Second,
	},
}

// filteringTransport wraps an http.RoundTripper to filter x5c certificates from JWKS responses.
// This is necessary because some identity providers include x5c certificates that cause parsing errors.
type filteringTransport struct {
	base http.RoundTripper
}

// RoundTrip implements http.RoundTripper interface
func (t *filteringTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Use base transport to make the request
	resp, err := t.base.RoundTrip(req)
	if err != nil {
		return resp, err
	}

	// Only filter JWKS responses (application/json)
	if resp.StatusCode != http.StatusOK {
		return resp, nil
	}

	// Read the response body
	body, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	// Try to parse and filter JWKS
	var jwks map[string]interface{}
	if err := json.Unmarshal(body, &jwks); err != nil {
		// Not valid JSON or not a JWKS, return original response
		resp.Body = io.NopCloser(strings.NewReader(string(body)))
		return resp, nil
	}

	// Check if this is a JWKS response
	keys, ok := jwks["keys"].([]interface{})
	if !ok {
		// Not a JWKS, return original response
		resp.Body = io.NopCloser(strings.NewReader(string(body)))
		return resp, nil
	}

	// Filter x5c from keys
	filteredKeys := make([]interface{}, 0, len(keys))
	for _, key := range keys {
		keyMap, ok := key.(map[string]interface{})
		if !ok {
			continue
		}

		// Validate minimum required fields
		kid, hasKid := keyMap["kid"].(string)
		kty, hasKty := keyMap["kty"].(string)
		if !hasKid || !hasKty || kid == "" || kty == "" {
			continue
		}

		// Remove x5c field if present
		delete(keyMap, "x5c")

		filteredKeys = append(filteredKeys, keyMap)
	}

	jwks["keys"] = filteredKeys

	// Marshal back to JSON
	filteredBody, err := json.Marshal(jwks)
	if err != nil {
		// If marshaling fails, return original response
		resp.Body = io.NopCloser(strings.NewReader(string(body)))
		return resp, nil
	}

	// Create new response with filtered body
	resp.Body = io.NopCloser(strings.NewReader(string(filteredBody)))
	resp.ContentLength = int64(len(filteredBody))

	return resp, nil
}

// TokenValidator handles JWT token validation with auto-refreshing JWKS.
// The JWKS is automatically refreshed in the background to handle key rotation.
type TokenValidator struct {
	jwks    keyfunc.Keyfunc // Keyfunc with background refresh goroutine
	jwksURL string
}

// isPrivateIP checks if the given host is a private, loopback, or link-local IP address.
// This is used to prevent SSRF attacks by blocking requests to internal networks.
func isPrivateIP(host string) bool {
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast()
}

// fetchAndFilterJWKS fetches JWKS from URL and removes keys with x5c certificates to avoid parsing errors
// environment parameter controls SSRF protection: only enforced in production/staging environments
func fetchAndFilterJWKS(ctx context.Context, jwksURL string, environment string) (json.RawMessage, error) {
	// Validate URL format before attempting to fetch
	parsedURL, err := url.Parse(jwksURL)
	if err != nil {
		return nil, fmt.Errorf("invalid JWKS URL format: %w", err)
	}

	// Validate URL scheme
	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		return nil, fmt.Errorf("invalid JWKS URL scheme: expected http or https, got %s", parsedURL.Scheme)
	}

	// Warn if not using HTTPS (security best practice)
	if parsedURL.Scheme != "https" {
		logger.Log.Warn("JWKS URL does not use HTTPS", "jwksUrl", jwksURL)
	}

	// Validate URL has a host
	if parsedURL.Host == "" {
		return nil, fmt.Errorf("invalid JWKS URL: missing host")
	}

	// SSRF Protection: Validate hostname to prevent attacks on private networks
	// Only enforce in production/staging environments
	enforceSSRFProtection := environment == "production" || environment == "staging"
	hostname := parsedURL.Hostname()

	if enforceSSRFProtection {
		// Block cloud metadata endpoints (AWS, GCP, Azure use 169.254.169.254)
		if strings.Contains(hostname, "169.254.169.254") {
			return nil, fmt.Errorf("JWKS URL cannot point to cloud metadata endpoint")
		}

		// Block private/internal network addresses
		if isPrivateIP(hostname) {
			return nil, fmt.Errorf("JWKS URL cannot point to private/internal network")
		}
	} else {
		logger.Log.Warn("SSRF protection disabled in non-production environment", "environment", environment)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, jwksURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch JWKS: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("JWKS endpoint returned status %d", resp.StatusCode)
	}

	// Limit response size to prevent memory exhaustion attacks
	limitedReader := io.LimitReader(resp.Body, 1<<20) // 1MB limit
	body, err := io.ReadAll(limitedReader)
	if err != nil {
		return nil, fmt.Errorf("failed to read JWKS response: %w", err)
	}

	// Parse the JWKS to filter out problematic keys
	var jwks map[string]interface{}
	if err := json.Unmarshal(body, &jwks); err != nil {
		return nil, fmt.Errorf("failed to parse JWKS JSON: %w", err)
	}

	keys, ok := jwks["keys"].([]interface{})
	if !ok {
		return nil, fmt.Errorf("JWKS missing 'keys' array")
	}

	if len(keys) == 0 {
		return nil, fmt.Errorf("JWKS contains no keys")
	}

	// Filter out keys with x5c (X.509 certificate chain) to avoid certificate parsing errors
	filteredKeys := make([]interface{}, 0, len(keys))
	skippedCount := 0
	for _, key := range keys {
		keyMap, ok := key.(map[string]interface{})
		if !ok {
			continue
		}

		// Validate that key has minimum required fields
		kid, hasKid := keyMap["kid"].(string)
		kty, hasKty := keyMap["kty"].(string)
		if !hasKid || !hasKty || kid == "" || kty == "" {
			logger.Log.Warn("Skipping key with missing kid or kty")
			continue
		}

		// Remove x5c field if present (this causes the certificate parsing error)
		if _, hasX5c := keyMap["x5c"]; hasX5c {
			delete(keyMap, "x5c")
			skippedCount++
			logger.Log.Warn("Removed x5c certificate from key to avoid parsing errors", "kid", kid)
		}
		filteredKeys = append(filteredKeys, keyMap)
	}

	if len(filteredKeys) == 0 {
		return nil, fmt.Errorf("no valid keys remaining after filtering")
	}

	if skippedCount > 0 {
		logger.Log.Info("Filtered keys with x5c certificates from JWKS", "count", skippedCount)
	}

	jwks["keys"] = filteredKeys
	return json.Marshal(jwks)
}

// NewTokenValidator creates a new TokenValidator instance with automatic JWKS refresh.
// The provided context controls the lifecycle of the background refresh goroutine.
// When the context is cancelled, the refresh goroutine will stop.
//
// Implementation uses keyfunc.NewDefaultOverrideCtx with a custom HTTP client that:
//   - Filters x5c certificates from JWKS (prevents parsing errors)
//   - Fetches JWKS immediately (fail-fast validation)
//   - Starts background goroutine to refresh JWKS every hour
//   - Handles automatic key rotation
//   - Refreshes on unknown kid (rate-limited to prevent abuse)
//
// This ensures:
//  1. Service won't start if JWKS is unavailable (when trustUpstream=false)
//  2. Automatic key rotation handling without manual intervention
//  3. High availability during key rotation periods
//  4. No x5c certificate parsing errors
func NewTokenValidator(ctx context.Context, jwksURL string, environment string) (*TokenValidator, error) {
	if jwksURL == "" {
		return &TokenValidator{}, nil
	}

	// First, validate and fetch JWKS to ensure fail-fast behavior
	// This ensures the service won't start with an invalid JWKS URL
	// Use parent context with timeout to respect cancellation
	fetchCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	// Validate URL and perform initial fetch for fail-fast validation
	_, err := fetchAndFilterJWKS(fetchCtx, jwksURL, environment)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch and filter JWKS: %w", err)
	}

	// Create custom HTTP client with filtering transport to remove x5c certificates
	// This ensures keyfunc's auto-refresh also gets filtered JWKS
	filteringClient := &http.Client{
		Timeout: 10 * time.Second,
		Transport: &filteringTransport{
			base: &http.Transport{
				MaxIdleConns:        100,
				IdleConnTimeout:     90 * time.Second,
				TLSHandshakeTimeout: 10 * time.Second,
			},
		},
	}

	// Now create keyfunc with auto-refresh using the filtering HTTP client
	// This will:
	// - Fetch JWKS immediately (with x5c filtered)
	// - Start background refresh goroutine (default: every 1 hour)
	// - Refresh on unknown kid (rate-limited: once per 5 minutes)
	// - Stop refresh goroutine when ctx is cancelled
	jwks, err := keyfunc.NewDefaultOverrideCtx(ctx, []string{jwksURL}, keyfunc.Override{
		Client: filteringClient,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create JWKS with auto-refresh: %w", err)
	}

	logger.Log.Info("Successfully loaded JWKS from endpoint with auto-refresh enabled", "url", jwksURL, "refreshInterval", "1h")

	return &TokenValidator{
		jwks:    jwks,
		jwksURL: jwksURL,
	}, nil
}

// validateSignature validates the token signature using cached JWKS
func (v *TokenValidator) validateSignature(tokenString string) (*jwt.Token, error) {
	token, err := jwt.Parse(tokenString, v.jwks.Keyfunc)
	if err != nil {
		// Log detailed error with JWKS URL for debugging, but don't expose it in error messages
		switch {
		case errors.Is(err, jwt.ErrTokenSignatureInvalid):
			logger.Log.Error("Token signature verification failed", "jwksUrl", v.jwksURL, "error", err)
			return nil, fmt.Errorf("token signature verification failed: %w", err)
		case errors.Is(err, jwt.ErrTokenMalformed):
			return nil, fmt.Errorf("malformed token: %w", err)
		case errors.Is(err, jwt.ErrTokenUnverifiable):
			logger.Log.Error("Token key not found in JWKS", "jwksUrl", v.jwksURL, "error", err)
			return nil, fmt.Errorf("token key not found in JWKS: %w", err)
		default:
			logger.Log.Error("Failed to parse and verify token", "jwksUrl", v.jwksURL, "error", err)
			return nil, fmt.Errorf("failed to parse and verify token: %w", err)
		}
	}

	if !token.Valid {
		return nil, fmt.Errorf("token is invalid")
	}

	return token, nil
}

// isAudienceValid checks if at least one token audience matches valid audiences
func isAudienceValid(tokenAudiences []string, validAudiences []string) bool {
	validAudsSet := make(map[string]struct{}, len(validAudiences))
	for _, aud := range validAudiences {
		validAudsSet[aud] = struct{}{}
	}

	for _, tokenAud := range tokenAudiences {
		if _, ok := validAudsSet[tokenAud]; ok {
			return true
		}
	}
	return false
}

// extractAudience extracts the audience claim which can be a string or array of strings
func extractAudience(claims jwt.MapClaims) []string {
	var aud []string
	if audStr, ok := claims[ClaimAud].(string); ok {
		aud = []string{audStr}
	} else if audList, ok := claims[ClaimAud].([]interface{}); ok {
		for _, a := range audList {
			if s, ok := a.(string); ok {
				aud = append(aud, s)
			}
		}
	}
	return aud
}

// extractTokenFromRequest extracts and validates the JWT token from the HTTP Authorization header
func extractTokenFromRequest(r *http.Request) (string, error) {
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		return "", fmt.Errorf("missing Authorization header")
	}

	// Remove "Bearer " prefix if present
	tokenString := strings.TrimPrefix(authHeader, "Bearer ")
	if tokenString == authHeader {
		// No "Bearer " prefix found
		return "", fmt.Errorf("authorization header must use Bearer scheme")
	}

	// Validate token length to prevent memory exhaustion attacks
	// JWT tokens are typically < 8KB, but allow up to 16KB for safety
	if len(tokenString) > 16*1024 {
		return "", fmt.Errorf("token exceeds maximum allowed size (16KB)")
	}

	if len(tokenString) == 0 {
		return "", fmt.Errorf("empty token")
	}

	return tokenString, nil
}

// validateTemporalClaims validates exp, nbf, and iat claims
func validateTemporalClaims(claims jwt.MapClaims, now int64) (exp int64, iat int64, err error) {
	// Validate exp claim - mandatory
	expVal, exists := claims[ClaimExp]
	if !exists {
		return 0, 0, fmt.Errorf("missing exp claim")
	}
	expFloat, ok := expVal.(float64)
	if !ok {
		return 0, 0, fmt.Errorf("invalid exp claim type: expected number, got %T", expVal)
	}
	if expFloat < 0 {
		return 0, 0, fmt.Errorf("invalid exp claim value: cannot be negative")
	}

	// nbf (not before) - validate before checking expiration
	if nbfVal, exists := claims[ClaimNbf]; exists {
		nbf, ok := nbfVal.(float64)
		if !ok {
			return 0, 0, fmt.Errorf("invalid nbf claim type: expected number, got %T", nbfVal)
		}
		if nbf < 0 {
			return 0, 0, fmt.Errorf("invalid nbf claim value: cannot be negative")
		}
		if now < int64(nbf) {
			return 0, 0, fmt.Errorf("token is not valid yet")
		}
	}

	// Check expiration
	if now > int64(expFloat) {
		return 0, 0, fmt.Errorf("token has expired")
	}

	// Extract iat (issued at) - optional but validate type if present
	var iatInt int64
	if iatVal, exists := claims[ClaimIat]; exists {
		iatFloat, ok := iatVal.(float64)
		if !ok {
			return 0, 0, fmt.Errorf("invalid iat claim type: expected number, got %T", iatVal)
		}
		if iatFloat < 0 {
			return 0, 0, fmt.Errorf("invalid iat claim value: cannot be negative")
		}
		iatInt = int64(iatFloat)
	}

	return int64(expFloat), iatInt, nil
}

// validateRequiredClaims validates client_id and subscriber (sub or azp)
func validateRequiredClaims(claims jwt.MapClaims) (clientID string, subscriber string, err error) {
	// client_id is required
	clientID, ok := claims[ClaimClientId].(string)
	if !ok || clientID == "" {
		return "", "", fmt.Errorf("missing or invalid client_id claim")
	}

	// sub or azp - at least one must be present
	subscriber, ok = claims[ClaimSub].(string)
	if !ok || subscriber == "" {
		// fallback to azp if sub is missing
		if azp, ok := claims[ClaimAzp].(string); ok && azp != "" {
			subscriber = azp
		}
	}
	if subscriber == "" {
		return "", "", fmt.Errorf("missing subscriber claim: both 'sub' and 'azp' are missing or empty")
	}

	return clientID, subscriber, nil
}

// validateIssuerAndAudience validates issuer and audience claims against configuration
func validateIssuerAndAudience(claims jwt.MapClaims, jwtConfig *configs.JWTConfig) (iss string, aud []string, err error) {
	iss, _ = claims[ClaimIss].(string)

	// Validate iss (issuer) if configured
	if jwtConfig != nil && jwtConfig.ExpectedIssuer != "" {
		if iss == "" {
			return "", nil, fmt.Errorf("missing issuer claim")
		}
		if iss != jwtConfig.ExpectedIssuer {
			return "", nil, fmt.Errorf("invalid issuer: expected %s, got %s", jwtConfig.ExpectedIssuer, iss)
		}
	}

	// Extract audience claim (can be string or array of strings)
	aud = extractAudience(claims)

	// Validate aud (audience) if configured
	if jwtConfig != nil && len(jwtConfig.ValidAudiences) > 0 {
		if !isAudienceValid(aud, jwtConfig.ValidAudiences) {
			return "", nil, fmt.Errorf("invalid audience: expected one of %v, got %v", jwtConfig.ValidAudiences, aud)
		}
	}

	return iss, aud, nil
}

// parseAndValidateToken parses the token string and validates it (with or without signature verification)
func parseAndValidateToken(tokenString string, trustUpstream bool, validator *TokenValidator, jwtConfig *configs.JWTConfig) (*jwt.Token, error) {
	if trustUpstream {
		// If we trust upstream, we assume the token has been validated already
		token, _, err := jwt.NewParser().ParseUnverified(tokenString, jwt.MapClaims{})
		if err != nil {
			return nil, fmt.Errorf("failed to parse token: %w", err)
		}
		return token, nil
	}

	// If we do not trust upstream, we must validate the token signature
	if validator == nil {
		return nil, fmt.Errorf("TokenValidator required when trustUpstream is false")
	}
	if jwtConfig == nil || jwtConfig.JwksUrl == "" {
		return nil, fmt.Errorf("missing JWKS URL for signature validation")
	}
	if validator.jwksURL != jwtConfig.JwksUrl {
		return nil, fmt.Errorf("TokenValidator URL mismatch: expected %s, got %s", jwtConfig.JwksUrl, validator.jwksURL)
	}

	return validator.validateSignature(tokenString)
}

// GetConsumerJwtFromToken validates and parses JWT token from HTTP request
func GetConsumerJwtFromToken(jwtConfig *configs.JWTConfig, trustUpstream bool, r *http.Request) (*ConsumerAssertion, error) {
	return GetConsumerJwtFromTokenWithValidator(jwtConfig, trustUpstream, r, nil)
}

// GetConsumerJwtFromTokenWithValidator validates and parses JWT token with optional cached validator
func GetConsumerJwtFromTokenWithValidator(jwtConfig *configs.JWTConfig, trustUpstream bool, r *http.Request, validator *TokenValidator) (*ConsumerAssertion, error) {
	// Extract token from request
	tokenString, err := extractTokenFromRequest(r)
	if err != nil {
		return nil, err
	}

	// Parse and validate token signature (if not trusting upstream)
	token, err := parseAndValidateToken(tokenString, trustUpstream, validator, jwtConfig)
	if err != nil {
		return nil, err
	}

	// Extract claims
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return nil, fmt.Errorf("invalid claims format")
	}

	// Validate temporal claims (exp, nbf, iat)
	now := time.Now().Unix()
	exp, iat, err := validateTemporalClaims(claims, now)
	if err != nil {
		return nil, err
	}

	// Validate required claims (client_id, subscriber)
	clientID, subscriber, err := validateRequiredClaims(claims)
	if err != nil {
		return nil, err
	}

	// Validate issuer and audience
	iss, aud, err := validateIssuerAndAudience(claims, jwtConfig)
	if err != nil {
		return nil, err
	}

	// The application identity used across the exchange is standardized on the OIDC
	// client_id. The PDP allow-list is keyed by client_id (see portal-backend
	// application_service.go), so OE presents the same client_id when asking the PDP
	// for a decision. See issue #447.

	// Build and return ConsumerAssertion
	return &ConsumerAssertion{
		ApplicationID: clientID,
		ClientID:      clientID,
		Subscriber:    subscriber,
		Iss:           iss,
		Aud:           aud,
		Exp:           exp,
		Iat:           iat,
	}, nil
}
