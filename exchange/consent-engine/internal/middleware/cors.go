package middleware

import (
	"net/http"
	"strconv"
	"strings"
)

// CORSConfig holds the CORS configuration
type CORSConfig struct {
	AllowedOrigins   []string
	AllowedMethods   []string
	AllowedHeaders   []string
	ExposedHeaders   []string
	AllowCredentials bool
	MaxAge           int
}

// DefaultCORSConfig returns the default CORS configuration.
// The allowed origins can be overridden by setting the CORS_ALLOWED_ORIGINS environment variable (comma-separated).
func DefaultCORSConfig(allowedOrigins string) CORSConfig {
	// Get allowed origins from environment variable, default to localhost:5173
	var allowedOriginsArr []string
	if envOrigins := allowedOrigins; envOrigins != "" {
		envOriginsList := strings.Split(envOrigins, ",")
		// Append environment origins to the default
		for _, origin := range envOriginsList {
			trimmed := strings.TrimSpace(origin)
			if trimmed != "" {
				allowedOriginsArr = append(allowedOriginsArr, trimmed)
			}
		}
	}

	return CORSConfig{
		AllowedOrigins: allowedOriginsArr,
		AllowedMethods: []string{
			"GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS",
		},
		AllowedHeaders: []string{
			"Origin", "Content-Type", "Accept", "Authorization",
			"X-Requested-With", "X-CSRF-Token", "X-Request-ID",
		},
		ExposedHeaders: []string{
			"Content-Length", "X-Request-ID",
		},
		AllowCredentials: true,
		MaxAge:           86400, // 24 hours
	}
}

// CORSMiddleware creates a CORS middleware with the given configuration
func CORSMiddleware(config CORSConfig) func(http.Handler) http.Handler {
	// Validate configuration: wildcard origin is incompatible with credentials
	if config.AllowCredentials {
		for _, origin := range config.AllowedOrigins {
			if origin == "*" {
				panic("CORS configuration error: wildcard origin (*) cannot be used with AllowCredentials=true")
			}
		}
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")

			// Check if the origin is allowed
			var allowedOrigin string
			for _, allowedOrig := range config.AllowedOrigins {
				if allowedOrig == "*" || allowedOrig == origin {
					allowedOrigin = allowedOrig
					break
				}
			}

			// If origin is allowed or we allow all origins
			if allowedOrigin != "" {
				// Always add Vary: Origin to prevent cache poisoning
				w.Header().Add("Vary", "Origin")

				// Set CORS headers
				// Note: wildcard with credentials is prevented at configuration time
				if allowedOrigin == "*" {
					w.Header().Set("Access-Control-Allow-Origin", "*")
				} else {
					w.Header().Set("Access-Control-Allow-Origin", origin)
				}

				w.Header().Set("Access-Control-Allow-Methods", strings.Join(config.AllowedMethods, ", "))
				w.Header().Set("Access-Control-Allow-Headers", strings.Join(config.AllowedHeaders, ", "))

				if len(config.ExposedHeaders) > 0 {
					w.Header().Set("Access-Control-Expose-Headers", strings.Join(config.ExposedHeaders, ", "))
				}

				if config.AllowCredentials {
					w.Header().Set("Access-Control-Allow-Credentials", "true")
				}

				if config.MaxAge > 0 {
					w.Header().Set("Access-Control-Max-Age", strconv.Itoa(config.MaxAge))
				}
			}

			// Handle preflight requests
			if r.Method == "OPTIONS" {
				// Add Vary headers for preflight requests to prevent cache poisoning
				w.Header().Add("Vary", "Access-Control-Request-Method")
				w.Header().Add("Vary", "Access-Control-Request-Headers")
				w.WriteHeader(http.StatusOK)
				return
			}

			// Continue with the next handler
			next.ServeHTTP(w, r)
		})
	}
}

// NewCORSMiddleware creates a CORS middleware with default configuration
func NewCORSMiddleware(allowedOrigins string) func(http.Handler) http.Handler {
	return CORSMiddleware(DefaultCORSConfig(allowedOrigins))
}
