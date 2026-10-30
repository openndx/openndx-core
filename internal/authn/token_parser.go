// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Lanka Software Foundation

package authn

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// spaceDelimitedScope unmarshals the OAuth2 "scope" claim, which is a single
// space-delimited string (RFC 6749 §3.3), e.g. "nsw:task:read nsw:storage:read".
// It defensively also accepts a JSON array of strings. An absent claim leaves it
// nil; an empty string yields an empty slice.
type spaceDelimitedScope []string

func (s *spaceDelimitedScope) UnmarshalJSON(data []byte) error {
	var str string
	if err := json.Unmarshal(data, &str); err == nil {
		*s = strings.Fields(str)
		return nil
	}
	var arr []string
	if err := json.Unmarshal(data, &arr); err != nil {
		return fmt.Errorf("scope claim must be a string or array of strings: %w", err)
	}
	*s = arr
	return nil
}

// tokenClaims is deliberately small: the claims every Principal carries (sub
// and client_id, both required; scope for OAuth2 permission scoping; roles for
// the authz seam), the standard identity claims (email, ouId, ouHandle; each
// optional unless configured as required), plus the JWT registered claims
// (iss/aud/exp/...) needed for signature/expiry/issuer/audience validation.
// The full payload is retained privately for the unified Claims view.
//
// Roles is captured shape-free (see rolesValue); parseRolesClaim enforces its
// shape.
type tokenClaims struct {
	payload map[string]any
	jwt.RegisteredClaims
	ClientID string              `json:"client_id"`
	Roles    rolesValue          `json:"roles"`
	Scopes   spaceDelimitedScope `json:"scope,omitempty"`
	Email    string              `json:"email"`
	OUID     string              `json:"ouId"`
	OUHandle string              `json:"ouHandle"`
}

type jwksResponse struct {
	Keys []jwk `json:"keys"`
}

type jwk struct {
	Kid string `json:"kid"`
	Kty string `json:"kty"`
	Alg string `json:"alg"`
	Use string `json:"use"`
	N   string `json:"n"`
	E   string `json:"e"`
}

const defaultJWKSCacheTTL = 5 * time.Minute

// TokenExtractor handles token extraction and parsing from HTTP headers.
// It validates JWT signatures using JWKS and resolves the token's Principal.
type TokenExtractor struct {
	jwksURL     string
	expIssuer   string
	expAudience string
	httpClient  *http.Client

	// claims configures the standard identity claims; see WithClaims.
	claims ClaimsConfig

	cacheMu       sync.RWMutex
	cachedJWKS    *jwksResponse
	lastJWKSFetch time.Time
	jwksCacheTTL  time.Duration
}

func NewTokenExtractor(jwksURL, issuer, audience string, opts ...Option) (*TokenExtractor, error) {
	return newExtractor(jwksURL, issuer, audience, &http.Client{Timeout: 10 * time.Second}, opts...)
}

func NewTokenExtractorWithClient(jwksURL, issuer, audience string, httpClient *http.Client, opts ...Option) (*TokenExtractor, error) {
	if httpClient == nil {
		return NewTokenExtractor(jwksURL, issuer, audience, opts...)
	}
	return newExtractor(jwksURL, issuer, audience, httpClient, opts...)
}

// newExtractor is the single construction path, so a defaulted field
// (jwksCacheTTL) can only be forgotten in one place.
func newExtractor(jwksURL, issuer, audience string, httpClient *http.Client, opts ...Option) (*TokenExtractor, error) {
	extractor := &TokenExtractor{
		jwksURL:      strings.TrimSpace(jwksURL),
		expIssuer:    strings.TrimSpace(issuer),
		expAudience:  strings.TrimSpace(audience),
		jwksCacheTTL: defaultJWKSCacheTTL,
		httpClient:   httpClient,
	}

	for _, opt := range opts {
		opt(extractor)
	}

	if err := extractor.validateConfig(); err != nil {
		return nil, err
	}

	return extractor, nil
}

func (te *TokenExtractor) validateConfig() error {
	if te.jwksURL == "" {
		return fmt.Errorf("jwks url is not configured")
	}
	if te.expIssuer == "" {
		return fmt.Errorf("issuer is not configured")
	}
	if te.expAudience == "" {
		return fmt.Errorf("audience is not configured")
	}
	if te.httpClient == nil {
		return fmt.Errorf("http client is not configured")
	}
	return nil
}

// ExtractPrincipalFromHeader extracts the principal from Authorization header.
// Expected header format: "Bearer <jwt_token>".
// JWT signature is validated against configured JWKS endpoint, then claims are
// mapped into a Principal.
func (te *TokenExtractor) ExtractPrincipalFromHeader(authHeader string) (*Principal, error) {
	if authHeader == "" {
		return nil, fmt.Errorf("authorization header is empty")
	}
	parts := strings.Fields(strings.TrimSpace(authHeader))
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return nil, fmt.Errorf("invalid authorization header format: expected 'Bearer <token>'")
	}
	tokenString := strings.TrimSpace(parts[1])
	if tokenString == "" {
		return nil, fmt.Errorf("authorization token is empty")
	}

	claims := &tokenClaims{}
	parsedToken, err := jwt.ParseWithClaims(tokenString, claims, te.keyFunc,
		jwt.WithValidMethods([]string{"RS256", "RS384", "RS512"}),
		jwt.WithIssuer(te.expIssuer),
		jwt.WithAudience(te.expAudience),
		jwt.WithLeeway(30*time.Second),
	)
	if err != nil {
		return nil, fmt.Errorf("invalid jwt token: %w", err)
	}
	if !parsedToken.Valid {
		return nil, fmt.Errorf("invalid jwt token")
	}

	if claims.ExpiresAt == nil {
		return nil, fmt.Errorf("jwt missing exp claim")
	}

	if claims.Subject == "" {
		return nil, fmt.Errorf("jwt missing sub claim")
	}

	if claims.ClientID == "" {
		return nil, fmt.Errorf("jwt missing client_id claim")
	}

	email := strings.TrimSpace(claims.Email)
	ouID := strings.TrimSpace(claims.OUID)
	ouHandle := strings.TrimSpace(claims.OUHandle)
	for _, c := range []struct {
		name   string
		config ClaimConfig
		value  string
	}{
		{"email", te.claims.Email, email},
		{"ouId", te.claims.OUID, ouID},
		{"ouHandle", te.claims.OUHandle, ouHandle},
	} {
		if c.config.Required && c.value == "" {
			// Claim NAMES only, never values: this error reaches the
			// middleware's Debug log with request context.
			return nil, fmt.Errorf("jwt missing or empty required claim %q", c.name)
		}
	}

	if _, err := parseRolesClaim(claims.Roles.value); err != nil {
		return nil, err
	}

	return &Principal{claims: validatedClaims(claims)}, nil
}

func (te *TokenExtractor) keyFunc(token *jwt.Token) (interface{}, error) {
	if _, ok := token.Method.(*jwt.SigningMethodRSA); !ok {
		return nil, fmt.Errorf("unexpected signing method: %s", token.Method.Alg())
	}

	kidValue, ok := token.Header["kid"]
	if !ok {
		return nil, fmt.Errorf("token header missing kid")
	}
	kid, ok := kidValue.(string)
	if !ok || strings.TrimSpace(kid) == "" {
		return nil, fmt.Errorf("token header has invalid kid")
	}

	keySet, err := te.getJWKS(false)
	if err != nil {
		return nil, err
	}

	for _, key := range keySet.Keys {
		if key.Kid != kid {
			continue
		}
		publicKey, err := parseRSAPublicKey(key)
		if err != nil {
			return nil, err
		}
		return publicKey, nil
	}

	// Key rotation can result in unknown kid in cache; force a refresh and retry once.
	keySet, err = te.getJWKS(true)
	if err != nil {
		return nil, err
	}

	for _, key := range keySet.Keys {
		if key.Kid != kid {
			continue
		}
		publicKey, err := parseRSAPublicKey(key)
		if err != nil {
			return nil, err
		}
		return publicKey, nil
	}

	return nil, fmt.Errorf("no jwk found for kid: %s", kid)
}

func (te *TokenExtractor) getJWKS(forceRefresh bool) (*jwksResponse, error) {
	now := time.Now()

	te.cacheMu.RLock()
	cacheValid := te.cachedJWKS != nil && te.jwksCacheTTL > 0 && now.Sub(te.lastJWKSFetch) < te.jwksCacheTTL
	if !forceRefresh && cacheValid {
		cached := te.cachedJWKS
		te.cacheMu.RUnlock()
		return cached, nil
	}
	// Snapshot the fetch timestamp so we can detect if another goroutine refreshed
	// while we were waiting for the write lock.
	lastFetchSnapshot := te.lastJWKSFetch
	te.cacheMu.RUnlock()

	te.cacheMu.Lock()
	defer te.cacheMu.Unlock()

	// Re-check after acquiring write lock.
	now = time.Now()
	cacheValid = te.cachedJWKS != nil && te.jwksCacheTTL > 0 && now.Sub(te.lastJWKSFetch) < te.jwksCacheTTL
	if !forceRefresh && cacheValid {
		return te.cachedJWKS, nil
	}
	// For forced refreshes: if another goroutine already fetched while we were
	// waiting for the lock (lastJWKSFetch moved forward), reuse that result
	// instead of hammering the IdP again.
	if forceRefresh && te.cachedJWKS != nil && te.lastJWKSFetch.After(lastFetchSnapshot) {
		return te.cachedJWKS, nil
	}

	jwks, err := te.fetchJWKS()
	if err != nil {
		return nil, err
	}

	te.cachedJWKS = jwks
	te.lastJWKSFetch = now

	return te.cachedJWKS, nil
}

func (te *TokenExtractor) fetchJWKS() (*jwksResponse, error) {
	slog.Debug("fetching jwks", "url", te.jwksURL)
	request, err := http.NewRequest(http.MethodGet, te.jwksURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to build jwks request: %w", err)
	}

	response, err := te.httpClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch jwks: %w", err)
	}
	defer func() { _ = response.Body.Close() }()

	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("jwks endpoint returned status %d", response.StatusCode)
	}

	var jwks jwksResponse
	if err := json.NewDecoder(response.Body).Decode(&jwks); err != nil {
		return nil, fmt.Errorf("failed to decode jwks response: %w", err)
	}

	if len(jwks.Keys) == 0 {
		return nil, fmt.Errorf("jwks response has no keys")
	}

	return &jwks, nil
}
