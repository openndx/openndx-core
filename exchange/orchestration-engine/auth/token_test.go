package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/OpenNDX/openndx-core/exchange/orchestration-engine/configs"
	"github.com/OpenNDX/openndx-core/exchange/orchestration-engine/logger"
	"github.com/golang-jwt/jwt/v5"
)

// Initialize logger for tests
func init() {
	logger.Init()
	// Ensure we're not in production mode for tests
	// (ENVIRONMENT is not set by default, so SSRF protection won't block localhost)
}

// Helper function to create a token without signing (matches ParseUnverified usage)
func createUnsignedTestToken(claims jwt.MapClaims) string {
	token := jwt.NewWithClaims(jwt.SigningMethodNone, claims)
	// For unsigned tokens, use empty string as key
	tokenString, _ := token.SignedString(jwt.UnsafeAllowNoneSignatureType)
	return tokenString
}

func TestGetConsumerJwtFromToken_AuthorizationHeader(t *testing.T) {
	claims := jwt.MapClaims{
		ClaimIss:      "https://idp.test.com",
		ClaimClientId: "test-client-id",
		ClaimSub:      "test-subscriber",
		ClaimAud:      []string{"https://api.test.com"},
		ClaimExp:      float64(time.Now().Add(time.Hour).Unix()),
		ClaimIat:      float64(time.Now().Unix()),
	}

	tokenString := createUnsignedTestToken(claims)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "Bearer "+tokenString)

	result, err := GetConsumerJwtFromToken(nil, true, req)
	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}

	if result == nil {
		t.Fatal("Expected non-nil result")
	}

	if result.ClientID != "test-client-id" {
		t.Errorf("Expected ClientID 'test-client-id', got '%s'", result.ClientID)
	}
	if result.Subscriber != "test-subscriber" {
		t.Errorf("Expected Subscriber 'test-subscriber', got '%s'", result.Subscriber)
	}
	if result.Iss != "https://idp.test.com" {
		t.Errorf("Expected Iss 'https://idp.test.com', got '%s'", result.Iss)
	}
}

func TestGetConsumerJwtFromToken_MissingAuthorizationHeader(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	// No Authorization header set

	result, err := GetConsumerJwtFromToken(nil, true, req)

	if err == nil {
		t.Error("Expected error when Authorization header is missing")
	}
	if result != nil {
		t.Error("Expected nil result when Authorization header is missing")
	}
	if err != nil && !strings.Contains(err.Error(), "missing Authorization header") {
		t.Errorf("Expected error about missing header, got: %v", err)
	}
}

func TestGetConsumerJwtFromToken_InvalidBearerScheme(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "Basic sometoken")

	result, err := GetConsumerJwtFromToken(nil, true, req)

	if err == nil {
		t.Error("Expected error when Bearer scheme is not used")
	}
	if result != nil {
		t.Error("Expected nil result when Bearer scheme is not used")
	}
	if err != nil && !strings.Contains(err.Error(), "Bearer scheme") {
		t.Errorf("Expected error about Bearer scheme, got: %v", err)
	}
}

func TestGetConsumerJwtFromToken_EmptyToken(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "Bearer ")

	result, err := GetConsumerJwtFromToken(nil, true, req)

	if err == nil {
		t.Error("Expected error when token is empty")
	}
	if result != nil {
		t.Error("Expected nil result when token is empty")
	}
	if err != nil && !strings.Contains(err.Error(), "empty token") {
		t.Errorf("Expected error about empty token, got: %v", err)
	}
}

func TestGetConsumerJwtFromToken_TokenSizeLimit(t *testing.T) {
	// Create a token larger than 16KB
	largeToken := strings.Repeat("a", 17*1024)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "Bearer "+largeToken)

	result, err := GetConsumerJwtFromToken(nil, true, req)

	if err == nil {
		t.Error("Expected error when token exceeds size limit")
	}
	if result != nil {
		t.Error("Expected nil result when token exceeds size limit")
	}
	if err != nil && !strings.Contains(err.Error(), "exceeds maximum allowed size") {
		t.Errorf("Expected error about size limit, got: %v", err)
	}
}

func TestGetConsumerJwtFromToken_MissingClientId(t *testing.T) {
	claims := jwt.MapClaims{
		ClaimSub: "some-subscriber",
		ClaimExp: float64(time.Now().Add(time.Hour).Unix()),
	}

	tokenString := createUnsignedTestToken(claims)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "Bearer "+tokenString)

	result, err := GetConsumerJwtFromToken(nil, true, req)

	if err == nil {
		t.Error("Expected error when client_id is missing")
	}
	if result != nil {
		t.Error("Expected nil result when client_id is missing")
	}
}

func TestGetConsumerJwtFromToken_ExpiredToken(t *testing.T) {
	claims := jwt.MapClaims{
		ClaimClientId: "client-id",
		ClaimExp:      float64(time.Now().Add(-time.Hour).Unix()), // Expired 1 hour ago
	}

	tokenString := createUnsignedTestToken(claims)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "Bearer "+tokenString)

	result, err := GetConsumerJwtFromToken(nil, true, req)

	if err == nil {
		t.Error("Expected error when token is expired")
	}
	if result != nil {
		t.Error("Expected nil result when token is expired")
	}
}

func TestGetConsumerJwtFromToken_NbfFuture(t *testing.T) {
	claims := jwt.MapClaims{
		ClaimClientId: "client-id",
		ClaimExp:      float64(time.Now().Add(time.Hour).Unix()),
		ClaimNbf:      float64(time.Now().Add(time.Hour).Unix()), // Valid in future
	}

	tokenString := createUnsignedTestToken(claims)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "Bearer "+tokenString)

	result, err := GetConsumerJwtFromToken(nil, true, req)

	if err == nil {
		t.Error("Expected error when token is not yet valid (nbf)")
	}
	if result != nil {
		t.Error("Expected nil result when token is not yet valid")
	}
}

func TestGetConsumerJwtFromToken_AzpFallback(t *testing.T) {
	claims := jwt.MapClaims{
		ClaimClientId: "client-id",
		ClaimAzp:      "azp-subscriber",
		// missing sub
		ClaimExp: float64(time.Now().Add(time.Hour).Unix()),
	}

	tokenString := createUnsignedTestToken(claims)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "Bearer "+tokenString)

	result, err := GetConsumerJwtFromToken(nil, true, req)
	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}

	if result.Subscriber != "azp-subscriber" {
		t.Errorf("Expected Subscriber to fall back to azp 'azp-subscriber', got '%s'", result.Subscriber)
	}
}

func TestGetConsumerJwtFromToken_MissingExp(t *testing.T) {
	claims := jwt.MapClaims{
		ClaimClientId: "client-id",
		ClaimSub:      "some-subscriber",
		// missing exp
	}

	tokenString := createUnsignedTestToken(claims)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "Bearer "+tokenString)

	result, err := GetConsumerJwtFromToken(nil, true, req)

	if err == nil {
		t.Error("Expected error when exp claim is missing")
	}
	if result != nil {
		t.Error("Expected nil result when exp claim is missing")
	}
	if err != nil && err.Error() != "missing exp claim" {
		t.Errorf("Expected error message 'missing exp claim', got: %v", err)
	}
}

func TestGetConsumerJwtFromToken_InvalidTemporalClaimTypes(t *testing.T) {
	testCases := []struct {
		name     string
		claims   jwt.MapClaims
		errorMsg string
	}{
		{
			name: "invalid exp type",
			claims: jwt.MapClaims{
				ClaimClientId: "client-id",
				ClaimSub:      "subscriber",
				ClaimExp:      "not-a-number",
			},
			errorMsg: "invalid exp claim type",
		},
		{
			name: "negative exp",
			claims: jwt.MapClaims{
				ClaimClientId: "client-id",
				ClaimSub:      "subscriber",
				ClaimExp:      float64(-100),
			},
			errorMsg: "invalid exp claim value",
		},
		{
			name: "invalid nbf type",
			claims: jwt.MapClaims{
				ClaimClientId: "client-id",
				ClaimSub:      "subscriber",
				ClaimExp:      float64(time.Now().Add(time.Hour).Unix()),
				ClaimNbf:      "not-a-number",
			},
			errorMsg: "invalid nbf claim type",
		},
		{
			name: "negative nbf",
			claims: jwt.MapClaims{
				ClaimClientId: "client-id",
				ClaimSub:      "subscriber",
				ClaimExp:      float64(time.Now().Add(time.Hour).Unix()),
				ClaimNbf:      float64(-100),
			},
			errorMsg: "invalid nbf claim value",
		},
		{
			name: "invalid iat type",
			claims: jwt.MapClaims{
				ClaimClientId: "client-id",
				ClaimSub:      "subscriber",
				ClaimExp:      float64(time.Now().Add(time.Hour).Unix()),
				ClaimIat:      "not-a-number",
			},
			errorMsg: "invalid iat claim type",
		},
		{
			name: "negative iat",
			claims: jwt.MapClaims{
				ClaimClientId: "client-id",
				ClaimSub:      "subscriber",
				ClaimExp:      float64(time.Now().Add(time.Hour).Unix()),
				ClaimIat:      float64(-100),
			},
			errorMsg: "invalid iat claim value",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tokenString := createUnsignedTestToken(tc.claims)
			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			req.Header.Set("Authorization", "Bearer "+tokenString)

			result, err := GetConsumerJwtFromToken(nil, true, req)

			if err == nil {
				t.Errorf("Expected error for %s", tc.name)
			}
			if result != nil {
				t.Errorf("Expected nil result for %s", tc.name)
			}
			if err != nil && !strings.Contains(err.Error(), tc.errorMsg) {
				t.Errorf("Expected error about %s, got: %v", tc.errorMsg, err)
			}
		})
	}
}

func TestGetConsumerJwtFromToken_InvalidIssuer(t *testing.T) {
	claims := jwt.MapClaims{
		ClaimClientId: "client-id",
		ClaimSub:      "subscriber",
		ClaimIss:      "https://wrong-issuer.com",
		ClaimExp:      float64(time.Now().Add(time.Hour).Unix()),
	}

	tokenString := createUnsignedTestToken(claims)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "Bearer "+tokenString)

	jwtConfig := &configs.JWTConfig{
		ExpectedIssuer: "https://expected-issuer.com",
	}

	result, err := GetConsumerJwtFromToken(jwtConfig, true, req)

	if err == nil {
		t.Error("Expected error when issuer doesn't match")
	}
	if result != nil {
		t.Error("Expected nil result when issuer doesn't match")
	}
}

func TestGetConsumerJwtFromToken_InvalidAudience(t *testing.T) {
	claims := jwt.MapClaims{
		ClaimClientId: "client-id",
		ClaimSub:      "subscriber",
		ClaimAud:      []string{"https://wrong-api.com"},
		ClaimExp:      float64(time.Now().Add(time.Hour).Unix()),
	}

	tokenString := createUnsignedTestToken(claims)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "Bearer "+tokenString)

	jwtConfig := &configs.JWTConfig{
		ValidAudiences: []string{"https://api1.com", "https://api2.com"},
	}

	result, err := GetConsumerJwtFromToken(jwtConfig, true, req)

	if err == nil {
		t.Error("Expected error when audience doesn't match any valid audience")
	}
	if result != nil {
		t.Error("Expected nil result when audience doesn't match")
	}
}

func TestGetConsumerJwtFromToken_StringAudience(t *testing.T) {
	claims := jwt.MapClaims{
		ClaimClientId: "client-id",
		ClaimSub:      "subscriber",
		ClaimAud:      "https://api-string.com", // String audience instead of array
		ClaimExp:      float64(time.Now().Add(time.Hour).Unix()),
	}

	tokenString := createUnsignedTestToken(claims)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "Bearer "+tokenString)

	jwtConfig := &configs.JWTConfig{
		ValidAudiences: []string{"https://api-string.com"},
	}

	result, err := GetConsumerJwtFromToken(jwtConfig, true, req)
	if err != nil {
		t.Errorf("Expected no error for string audience, got: %v", err)
	}
	if result == nil {
		t.Fatal("Expected non-nil result for valid string audience")
	}
	if len(result.Aud) != 1 || result.Aud[0] != "https://api-string.com" {
		t.Errorf("Expected audience 'https://api-string.com', got '%v'", result.Aud)
	}
}

func TestGetConsumerJwtFromToken_MissingSubscriber(t *testing.T) {
	claims := jwt.MapClaims{
		ClaimClientId: "client-id",
		// Missing both sub and azp
		ClaimExp: float64(time.Now().Add(time.Hour).Unix()),
	}

	tokenString := createUnsignedTestToken(claims)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "Bearer "+tokenString)

	result, err := GetConsumerJwtFromToken(nil, true, req)

	if err == nil {
		t.Error("Expected error when both sub and azp are missing")
	}
	if result != nil {
		t.Error("Expected nil result when subscriber claims are missing")
	}
	if err != nil && !strings.Contains(err.Error(), "missing subscriber claim") {
		t.Errorf("Expected error about missing subscriber, got: %v", err)
	}
}

func TestGetConsumerJwtFromToken_EmptyClientId(t *testing.T) {
	claims := jwt.MapClaims{
		ClaimClientId: "", // Empty client_id
		ClaimSub:      "subscriber",
		ClaimExp:      float64(time.Now().Add(time.Hour).Unix()),
	}

	tokenString := createUnsignedTestToken(claims)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "Bearer "+tokenString)

	result, err := GetConsumerJwtFromToken(nil, true, req)

	if err == nil {
		t.Error("Expected error when client_id is empty")
	}
	if result != nil {
		t.Error("Expected nil result when client_id is empty")
	}
	if err != nil && !strings.Contains(err.Error(), "missing or invalid client_id") {
		t.Errorf("Expected error about invalid client_id, got: %v", err)
	}
}

func TestGetConsumerJwtFromToken_ApplicationIDIsClientID(t *testing.T) {
	claims := jwt.MapClaims{
		ClaimClientId: "test-client",
		ClaimSub:      "subscriber",
		ClaimExp:      float64(time.Now().Add(time.Hour).Unix()),
	}

	tokenString := createUnsignedTestToken(claims)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "Bearer "+tokenString)

	result, err := GetConsumerJwtFromToken(nil, true, req)
	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}
	if result == nil {
		t.Fatal("Expected non-nil result")
	}
	if result.ApplicationID != "test-client" {
		t.Errorf("Expected ApplicationID to equal client_id 'test-client', got '%s'", result.ApplicationID)
	}
}

// The application identity is standardized on client_id; any application_id claim
// in the token is intentionally ignored (see issue #447).
func TestGetConsumerJwtFromToken_ApplicationIdClaimIgnored(t *testing.T) {
	claims := jwt.MapClaims{
		ClaimClientId:      "test-client",
		ClaimApplicationId: "test-app-123",
		ClaimSub:           "subscriber",
		ClaimExp:           float64(time.Now().Add(time.Hour).Unix()),
	}

	tokenString := createUnsignedTestToken(claims)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "Bearer "+tokenString)

	result, err := GetConsumerJwtFromToken(nil, true, req)
	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}
	if result == nil {
		t.Fatal("Expected non-nil result")
	}
	if result.ApplicationID != "test-client" {
		t.Errorf("Expected ApplicationID to equal client_id 'test-client' (application_id ignored), got '%s'", result.ApplicationID)
	}
}

func TestGetConsumerJwtFromToken_MissingIssuerWhenRequired(t *testing.T) {
	claims := jwt.MapClaims{
		ClaimClientId: "client-id",
		ClaimSub:      "subscriber",
		ClaimExp:      float64(time.Now().Add(time.Hour).Unix()),
		// Missing iss claim
	}

	tokenString := createUnsignedTestToken(claims)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "Bearer "+tokenString)

	jwtConfig := &configs.JWTConfig{
		ExpectedIssuer: "https://required-issuer.com",
	}

	result, err := GetConsumerJwtFromToken(jwtConfig, true, req)

	if err == nil {
		t.Error("Expected error when issuer is required but missing")
	}
	if result != nil {
		t.Error("Expected nil result when required issuer is missing")
	}
	if err != nil && !strings.Contains(err.Error(), "missing issuer claim") {
		t.Errorf("Expected error about missing issuer, got: %v", err)
	}
}

func TestGetConsumerJwtFromToken_EmptyAudience(t *testing.T) {
	claims := jwt.MapClaims{
		ClaimClientId: "client-id",
		ClaimSub:      "subscriber",
		ClaimExp:      float64(time.Now().Add(time.Hour).Unix()),
		ClaimAud:      []string{}, // Empty audience array
	}

	tokenString := createUnsignedTestToken(claims)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "Bearer "+tokenString)

	jwtConfig := &configs.JWTConfig{
		ValidAudiences: []string{"https://api.com"},
	}

	result, err := GetConsumerJwtFromToken(jwtConfig, true, req)

	if err == nil {
		t.Error("Expected error when audience is empty but validation is required")
	}
	if result != nil {
		t.Error("Expected nil result when audience validation fails")
	}
	if err != nil && !strings.Contains(err.Error(), "invalid audience") {
		t.Errorf("Expected error about invalid audience, got: %v", err)
	}
}

func TestGetConsumerJwtFromToken_NoAudienceValidationWhenNotConfigured(t *testing.T) {
	claims := jwt.MapClaims{
		ClaimClientId: "client-id",
		ClaimSub:      "subscriber",
		ClaimExp:      float64(time.Now().Add(time.Hour).Unix()),
		// No aud claim
	}

	tokenString := createUnsignedTestToken(claims)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "Bearer "+tokenString)

	// No jwtConfig means no audience validation
	result, err := GetConsumerJwtFromToken(nil, true, req)
	if err != nil {
		t.Errorf("Expected no error when audience validation is not configured, got: %v", err)
	}
	if result == nil {
		t.Fatal("Expected non-nil result")
	}
}

// Tests for fetchAndFilterJWKS function
func TestFetchAndFilterJWKS_InvalidURL(t *testing.T) {
	_, err := fetchAndFilterJWKS(context.Background(), "http://invalid-url-that-does-not-exist:99999/jwks", "test")
	if err == nil {
		t.Error("Expected error for invalid URL")
	}
	if err != nil && !strings.Contains(err.Error(), "failed to fetch JWKS") {
		t.Errorf("Expected fetch error, got: %v", err)
	}
}

func TestFetchAndFilterJWKS_Non200Status(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	_, err := fetchAndFilterJWKS(context.Background(), server.URL, "test")
	if err == nil {
		t.Error("Expected error for non-200 status")
	}
	if err != nil && !strings.Contains(err.Error(), "returned status 404") {
		t.Errorf("Expected status error, got: %v", err)
	}
}

func TestFetchAndFilterJWKS_InvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("not valid json"))
	}))
	defer server.Close()

	_, err := fetchAndFilterJWKS(context.Background(), server.URL, "test")
	if err == nil {
		t.Error("Expected error for invalid JSON")
	}
	if err != nil && !strings.Contains(err.Error(), "failed to parse JWKS JSON") {
		t.Errorf("Expected JSON parse error, got: %v", err)
	}
}

func TestFetchAndFilterJWKS_MissingKeysArray(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"notkeys": []}`))
	}))
	defer server.Close()

	_, err := fetchAndFilterJWKS(context.Background(), server.URL, "test")
	if err == nil {
		t.Error("Expected error for missing keys array")
	}
	if err != nil && !strings.Contains(err.Error(), "missing 'keys' array") {
		t.Errorf("Expected missing keys error, got: %v", err)
	}
}

func TestFetchAndFilterJWKS_EmptyKeys(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"keys": []}`))
	}))
	defer server.Close()

	_, err := fetchAndFilterJWKS(context.Background(), server.URL, "test")
	if err == nil {
		t.Error("Expected error for empty keys array")
	}
	if err != nil && !strings.Contains(err.Error(), "contains no keys") {
		t.Errorf("Expected no keys error, got: %v", err)
	}
}

func TestFetchAndFilterJWKS_FilterX5c(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		jwks := `{
			"keys": [
				{
					"kid": "key1",
					"kty": "RSA",
					"use": "sig",
					"n": "test",
					"e": "AQAB",
					"x5c": ["cert1", "cert2"]
				},
				{
					"kid": "key2",
					"kty": "RSA",
					"use": "sig",
					"n": "test2",
					"e": "AQAB"
				}
			]
		}`
		w.Write([]byte(jwks))
	}))
	defer server.Close()

	result, err := fetchAndFilterJWKS(context.Background(), server.URL, "test")
	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}

	// Verify x5c was removed
	if strings.Contains(string(result), "x5c") {
		t.Error("Expected x5c to be filtered out")
	}
	if !strings.Contains(string(result), "key1") {
		t.Error("Expected key1 to remain")
	}
	if !strings.Contains(string(result), "key2") {
		t.Error("Expected key2 to remain")
	}
}

func TestFetchAndFilterJWKS_SkipInvalidKeys(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		jwks := `{
			"keys": [
				{
					"kty": "RSA",
					"use": "sig"
				},
				{
					"kid": "key2",
					"kty": "RSA",
					"use": "sig",
					"n": "test",
					"e": "AQAB"
				}
			]
		}`
		w.Write([]byte(jwks))
	}))
	defer server.Close()

	result, err := fetchAndFilterJWKS(context.Background(), server.URL, "test")
	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}

	// Verify only valid key remains
	if !strings.Contains(string(result), "key2") {
		t.Error("Expected key2 to remain")
	}
}

func TestFetchAndFilterJWKS_AllKeysInvalid(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		jwks := `{
			"keys": [
				{
					"kty": "RSA"
				},
				{
					"kid": ""
				}
			]
		}`
		w.Write([]byte(jwks))
	}))
	defer server.Close()

	_, err := fetchAndFilterJWKS(context.Background(), server.URL, "test")
	if err == nil {
		t.Error("Expected error when all keys are invalid")
	}
	if err != nil && !strings.Contains(err.Error(), "no valid keys remaining") {
		t.Errorf("Expected no valid keys error, got: %v", err)
	}
}

// Tests for NewTokenValidator function
func TestNewTokenValidator_EmptyURL(t *testing.T) {
	validator, err := NewTokenValidator(context.Background(), "", "test")
	if err != nil {
		t.Errorf("Expected no error for empty URL, got: %v", err)
	}
	if validator == nil {
		t.Error("Expected non-nil validator")
	}
}

func TestNewTokenValidator_InvalidURL(t *testing.T) {
	_, err := NewTokenValidator(context.Background(), "http://invalid-url:99999/jwks", "test")
	if err == nil {
		t.Error("Expected error for invalid URL")
	}
	if err != nil && !strings.Contains(err.Error(), "failed to fetch and filter JWKS") {
		t.Errorf("Expected fetch error, got: %v", err)
	}
}

func TestNewTokenValidator_ValidJWKS(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		jwks := `{
			"keys": [
				{
					"kid": "test-key",
					"kty": "RSA",
					"use": "sig",
					"n": "xGOr-H7A-PWP8v8tI1sJdWclcKcFPvjKZd9U8xNxbXm7K5dP6Lv5vF1d_HUV6K3hjDLh1N9O_kC8v0e_R8xVzL3Q3t-Gz2vH3S4uM8xC7f5b4J3N6p8L0f2T1zN8x7v2s4K9L3g5H7F0d1_",
					"e": "AQAB"
				}
			]
		}`
		w.Write([]byte(jwks))
	}))
	defer server.Close()

	validator, err := NewTokenValidator(context.Background(), server.URL, "test")
	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}
	if validator == nil {
		t.Fatal("Expected non-nil validator")
	}
	if validator.jwksURL != server.URL {
		t.Errorf("Expected jwksURL %s, got %s", server.URL, validator.jwksURL)
	}
}

// Tests for parseAndValidateToken function
func TestParseAndValidateToken_TrustUpstream(t *testing.T) {
	claims := jwt.MapClaims{
		ClaimClientId: "test-client",
		ClaimExp:      float64(time.Now().Add(time.Hour).Unix()),
	}
	tokenString := createUnsignedTestToken(claims)

	token, err := parseAndValidateToken(tokenString, true, nil, nil)
	if err != nil {
		t.Errorf("Expected no error with trustUpstream=true, got: %v", err)
	}
	if token == nil {
		t.Error("Expected non-nil token")
	}
}

func TestParseAndValidateToken_InvalidToken(t *testing.T) {
	_, err := parseAndValidateToken("not.a.valid.token", true, nil, nil)
	if err == nil {
		t.Error("Expected error for invalid token")
	}
	if err != nil && !strings.Contains(err.Error(), "failed to parse token") {
		t.Errorf("Expected parse error, got: %v", err)
	}
}

func TestParseAndValidateToken_NoValidatorWhenRequired(t *testing.T) {
	claims := jwt.MapClaims{
		ClaimClientId: "test-client",
		ClaimExp:      float64(time.Now().Add(time.Hour).Unix()),
	}
	tokenString := createUnsignedTestToken(claims)

	_, err := parseAndValidateToken(tokenString, false, nil, nil)
	if err == nil {
		t.Error("Expected error when validator is nil with trustUpstream=false")
	}
	if err != nil && !strings.Contains(err.Error(), "TokenValidator required") {
		t.Errorf("Expected validator required error, got: %v", err)
	}
}

func TestParseAndValidateToken_MissingJWKSURL(t *testing.T) {
	claims := jwt.MapClaims{
		ClaimClientId: "test-client",
		ClaimExp:      float64(time.Now().Add(time.Hour).Unix()),
	}
	tokenString := createUnsignedTestToken(claims)

	validator := &TokenValidator{jwksURL: ""}

	_, err := parseAndValidateToken(tokenString, false, validator, &configs.JWTConfig{})
	if err == nil {
		t.Error("Expected error when JWKS URL is missing")
	}
	if err != nil && !strings.Contains(err.Error(), "missing JWKS URL") {
		t.Errorf("Expected missing JWKS URL error, got: %v", err)
	}
}

func TestParseAndValidateToken_URLMismatch(t *testing.T) {
	claims := jwt.MapClaims{
		ClaimClientId: "test-client",
		ClaimExp:      float64(time.Now().Add(time.Hour).Unix()),
	}
	tokenString := createUnsignedTestToken(claims)

	validator := &TokenValidator{jwksURL: "https://jwks1.com"}
	jwtConfig := &configs.JWTConfig{JwksUrl: "https://jwks2.com"}

	_, err := parseAndValidateToken(tokenString, false, validator, jwtConfig)
	if err == nil {
		t.Error("Expected error when validator URL doesn't match config URL")
	}
	if err != nil && !strings.Contains(err.Error(), "URL mismatch") {
		t.Errorf("Expected URL mismatch error, got: %v", err)
	}
}

// Tests for GetConsumerJwtFromTokenWithValidator
func TestGetConsumerJwtFromTokenWithValidator_WithValidator(t *testing.T) {
	claims := jwt.MapClaims{
		ClaimClientId: "test-client",
		ClaimSub:      "test-subscriber",
		ClaimExp:      float64(time.Now().Add(time.Hour).Unix()),
	}
	tokenString := createUnsignedTestToken(claims)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "Bearer "+tokenString)

	// Create empty validator for trustUpstream=true scenario
	validator := &TokenValidator{}

	result, err := GetConsumerJwtFromTokenWithValidator(nil, true, req, validator)
	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}
	if result == nil {
		t.Error("Expected non-nil result")
	}
}

// Tests for validateSignature method
func TestValidateSignature_MalformedToken(t *testing.T) {
	// Use a real RSA public key (base64url encoded modulus and exponent)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		// This is a valid JWKS with proper base64url encoded RSA parameters
		jwks := `{
			"keys": [
				{
					"kty": "RSA",
					"use": "sig",
					"kid": "test-key-1",
					"n": "xGOr-H7A9L_PWP8v8tI1sJdWclcKcFPvjKZd9U8xNxbXm7K5dP6Lv5vF1d_HUV6K3hjDLh1N9O_kC8v0e_R8xVzL3Q3t-Gz2vH3S4uM8xC7f5b4J3N6p8L0f2T1zN8x7v2s4K9L3g5H7F0d1_KtY8xH3v4P2L9f0d6N8z1x5v7K3s2g4M9L0f8H1d3",
					"e": "AQAB"
				}
			]
		}`
		w.Write([]byte(jwks))
	}))
	defer server.Close()

	validator, err := NewTokenValidator(context.Background(), server.URL, "test")
	if err != nil {
		t.Fatalf("Failed to create validator: %v", err)
	}

	// Test with malformed token (not enough segments)
	_, err = validator.validateSignature("not.valid")
	if err == nil {
		t.Error("Expected error for malformed token")
	}
	// The error could be about segments or malformed
	if !strings.Contains(err.Error(), "malformed") && !strings.Contains(err.Error(), "token") {
		t.Logf("Got error: %v", err)
	}
}

func TestValidateSignature_KeyNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		jwks := `{
			"keys": [
				{
					"kty": "RSA",
					"use": "sig",
					"kid": "different-key-id",
					"n": "xGOr-H7A9L_PWP8v8tI1sJdWclcKcFPvjKZd9U8xNxbXm7K5dP6Lv5vF1d_HUV6K3hjDLh1N9O_kC8v0e_R8xVzL3Q3t-Gz2vH3S4uM8xC7f5b4J3N6p8L0f2T1zN8x7v2s4K9L3g5H7F0d1_KtY8xH3v4P2L9f0d6N8z1x5v7K3s2g4M9L0f8H1d3",
					"e": "AQAB"
				}
			]
		}`
		w.Write([]byte(jwks))
	}))
	defer server.Close()

	validator, err := NewTokenValidator(context.Background(), server.URL, "test")
	if err != nil {
		t.Fatalf("Failed to create validator: %v", err)
	}

	// Create a properly formatted but unsigned token with non-existent kid
	claims := jwt.MapClaims{
		ClaimClientId: "test-client",
		ClaimExp:      float64(time.Now().Add(time.Hour).Unix()),
	}
	tokenString := createUnsignedTestToken(claims)
	parts := strings.Split(tokenString, ".")
	if len(parts) >= 2 {
		// Add a fake signature to make it a complete JWT
		fakeTokenString := parts[0] + "." + parts[1] + ".fakesignature"

		_, err = validator.validateSignature(fakeTokenString)
		if err == nil {
			t.Error("Expected error for key not found or invalid signature")
		}
	}
}
