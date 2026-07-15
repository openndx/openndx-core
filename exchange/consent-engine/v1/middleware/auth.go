package middleware

import (
	"context"
	"log/slog"
	"net/http"
	"strings"

	"github.com/OpenNDX/openndx-core/exchange/consent-engine/v1/auth"
	"github.com/OpenNDX/openndx-core/exchange/consent-engine/v1/models"
	"github.com/OpenNDX/openndx-core/exchange/consent-engine/v1/utils"
)

// contextKey is a custom type for context keys used with context.WithValue.
// Defining a custom type helps avoid key collisions with context keys defined in other packages.
type contextKey string

const (
	// userEmailKey is the context key for user email
	userEmailKey contextKey = "userEmail"
)

// JWTAuthMiddleware provides HTTP middleware for JWT authentication
type JWTAuthMiddleware struct {
	verifier *auth.JWTVerifier
}

// NewJWTAuthMiddleware creates a new JWT authentication middleware
func NewJWTAuthMiddleware(verifier *auth.JWTVerifier) *JWTAuthMiddleware {
	return &JWTAuthMiddleware{
		verifier: verifier,
	}
}

// Authenticate is the middleware function that validates JWT tokens
func (m *JWTAuthMiddleware) Authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Extract Authorization header
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			utils.RespondWithError(w, http.StatusUnauthorized, models.ErrorCodeUnauthorized, "Authorization header is required")
			return
		}

		// Check Bearer prefix
		const bearerPrefix = "Bearer "
		if !strings.HasPrefix(authHeader, bearerPrefix) {
			utils.RespondWithError(w, http.StatusUnauthorized, models.ErrorCodeUnauthorized, "Invalid authorization format. Expected 'Bearer <token>'")
			return
		}

		// Extract token
		tokenString := strings.TrimPrefix(authHeader, bearerPrefix)
		if tokenString == "" {
			utils.RespondWithError(w, http.StatusUnauthorized, models.ErrorCodeUnauthorized, "Token is required")
			return
		}

		// Verify token and extract email
		email, err := m.verifier.VerifyTokenAndExtractEmail(tokenString)
		if err != nil {
			slog.Warn("Token verification failed", "error", err)
			utils.RespondWithError(w, http.StatusUnauthorized, models.ErrorCodeUnauthorized, "Invalid or expired token")
			return
		}

		// Add email to request context
		ctx := context.WithValue(r.Context(), userEmailKey, email)
		r = r.WithContext(ctx)

		slog.Debug("User authenticated", "email", email)

		// Call next handler
		next.ServeHTTP(w, r)
	})
}

// GetUserEmailFromContext extracts the user email from the request context
func GetUserEmailFromContext(ctx context.Context) (string, bool) {
	email, ok := ctx.Value(userEmailKey).(string)
	return email, ok
}
