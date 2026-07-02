package router

import (
	"net/http"

	"github.com/gov-dx-sandbox/exchange/consent-engine/v1/auth"
	"github.com/gov-dx-sandbox/exchange/consent-engine/v1/handlers"
	"github.com/gov-dx-sandbox/exchange/consent-engine/v1/middleware"

	sharedUtils "github.com/OpenDIF/opendif-core/exchange/shared/utils"
)

// V1Router handles all V1 API route registration
type V1Router struct {
	internalHandler *handlers.InternalHandler
	portalHandler   *handlers.PortalHandler
	authMiddleware  *middleware.JWTAuthMiddleware
	corsMiddleware  func(http.Handler) http.Handler
}

// NewV1Router creates a new V1 router with all dependencies
func NewV1Router(
	allowedOrigins string,
	internalHandler *handlers.InternalHandler,
	portalHandler *handlers.PortalHandler,
	jwtVerifier *auth.JWTVerifier,
) *V1Router {
	return &V1Router{
		internalHandler: internalHandler,
		portalHandler:   portalHandler,
		authMiddleware:  middleware.NewJWTAuthMiddleware(jwtVerifier),
		corsMiddleware:  middleware.NewCORSMiddleware(allowedOrigins),
	}
}

// RegisterRoutes registers all V1 API routes to the provided mux
func (r *V1Router) RegisterRoutes(mux *http.ServeMux) {
	r.registerInternalRoutes(mux)
	r.registerPortalRoutes(mux)
}

// registerInternalRoutes registers internal API routes (no authentication required)
func (r *V1Router) registerInternalRoutes(mux *http.ServeMux) {
	// Health check
	mux.Handle("/internal/api/v1/health",
		sharedUtils.PanicRecoveryMiddleware(http.HandlerFunc(r.internalHandler.HealthCheck)))

	// Consents endpoint
	mux.Handle("GET /internal/api/v1/consents",
		sharedUtils.PanicRecoveryMiddleware(http.HandlerFunc(r.internalHandler.GetConsent)))
	mux.Handle("POST /internal/api/v1/consents",
		sharedUtils.PanicRecoveryMiddleware(http.HandlerFunc(r.internalHandler.CreateConsent)))
}

// registerPortalRoutes registers portal API routes (authentication required for protected endpoints)
func (r *V1Router) registerPortalRoutes(mux *http.ServeMux) {
	// Health check endpoint (public - no authentication per OpenAPI spec)
	mux.Handle("/api/v1/health",
		sharedUtils.PanicRecoveryMiddleware(http.HandlerFunc(r.portalHandler.HealthCheck)))

	// Consent endpoints (authentication required)
	mux.Handle("GET /api/v1/consents/{consentId}",
		sharedUtils.PanicRecoveryMiddleware(
			r.authMiddleware.Authenticate(http.HandlerFunc(r.portalHandler.GetConsent))))
	mux.Handle("PUT /api/v1/consents/{consentId}",
		sharedUtils.PanicRecoveryMiddleware(
			r.authMiddleware.Authenticate(http.HandlerFunc(r.portalHandler.UpdateConsent))))
}

// ApplyCORS wraps a handler with CORS middleware
func (r *V1Router) ApplyCORS(handler http.Handler) http.Handler {
	return r.corsMiddleware(handler)
}
