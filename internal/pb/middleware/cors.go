package middleware

import (
	"log/slog"
	"net/http"
	"os"
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

// DefaultCORSConfig returns the default CORS configuration
func DefaultCORSConfig() CORSConfig {
	// Get allowed origins from environment variable, default to localhost:5173
	allowedOrigins := []string{"http://localhost:5173"}
	if envOrigins := os.Getenv("CORS_ALLOWED_ORIGINS"); envOrigins != "" {
		allowedOrigins = strings.Split(envOrigins, ",")
		// Trim whitespace from each origin
		for i, origin := range allowedOrigins {
			allowedOrigins[i] = strings.TrimSpace(origin)
		}
	}

	config := CORSConfig{
		AllowedOrigins: allowedOrigins,
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

	// Browsers reject Access-Control-Allow-Origin: * with credentials. Drop wildcard
	// origins when credentials are enabled so deployers must list trusted origins.
	config.AllowedOrigins = sanitizeCORSOrigins(config.AllowedOrigins, config.AllowCredentials)

	return config
}

func sanitizeCORSOrigins(origins []string, allowCredentials bool) []string {
	if !allowCredentials {
		return origins
	}

	sanitized := make([]string, 0, len(origins))
	for _, origin := range origins {
		if origin == "*" {
			slog.Warn("Ignoring CORS origin '*' because AllowCredentials is enabled; list explicit origins instead")
			continue
		}
		sanitized = append(sanitized, origin)
	}
	if len(sanitized) == 0 {
		return []string{"http://localhost:5173"}
	}
	return sanitized
}

// CORSMiddleware creates a CORS middleware with the given configuration
func CORSMiddleware(config CORSConfig) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")

			// Check if the origin is allowed
			var allowedOrigin string
			for _, allowedOrig := range config.AllowedOrigins {
				if allowedOrig == "*" {
					// Never pair wildcard with credentials (invalid per the Fetch spec)
					if config.AllowCredentials {
						continue
					}
					allowedOrigin = "*"
					break
				}
				if allowedOrig == origin {
					allowedOrigin = allowedOrig
					break
				}
			}

			// If origin is allowed or we allow all origins
			if allowedOrigin != "" {
				// Set CORS headers
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
				w.WriteHeader(http.StatusOK)
				return
			}

			// Continue with the next handler
			next.ServeHTTP(w, r)
		})
	}
}

// NewCORSMiddleware creates a CORS middleware with default configuration
func NewCORSMiddleware() func(http.Handler) http.Handler {
	return CORSMiddleware(DefaultCORSConfig())
}
