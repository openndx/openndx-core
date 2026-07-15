package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/OpenNDX/openndx-core/portal-backend/v1/models"
)

func TestAuthorizationMiddleware_HandleUndefinedEndpoint(t *testing.T) {
	// Create test users with different roles
	adminUser := &models.AuthenticatedUser{
		IdpUserID: "admin-123",
		Email:     "admin@example.com",
		Roles:     []models.Role{models.RoleAdmin},
	}

	memberUser := &models.AuthenticatedUser{
		IdpUserID: "member-123",
		Email:     "member@example.com",
		Roles:     []models.Role{models.RoleMember},
	}

	systemUser := &models.AuthenticatedUser{
		IdpUserID: "system-123",
		Email:     "system@example.com",
		Roles:     []models.Role{models.RoleSystem},
	}

	tests := []struct {
		name           string
		config         AuthorizationConfig
		user           *models.AuthenticatedUser
		expectedStatus int
		expectedStop   bool // true if response should be sent (request stops)
	}{
		{
			name: "FailClosed - Admin user denied",
			config: AuthorizationConfig{
				Mode:       models.AuthorizationModeFailClosed,
				StrictMode: false,
			},
			user:           adminUser,
			expectedStatus: http.StatusForbidden,
			expectedStop:   true,
		},
		{
			name: "FailClosed - Member user denied",
			config: AuthorizationConfig{
				Mode:       models.AuthorizationModeFailClosed,
				StrictMode: false,
			},
			user:           memberUser,
			expectedStatus: http.StatusForbidden,
			expectedStop:   true,
		},
		{
			name: "FailOpenAdmin - Admin user allowed",
			config: AuthorizationConfig{
				Mode:       models.AuthorizationModeFailOpenAdmin,
				StrictMode: false,
			},
			user:           adminUser,
			expectedStatus: 0, // No response sent
			expectedStop:   false,
		},
		{
			name: "FailOpenAdmin - Member user denied",
			config: AuthorizationConfig{
				Mode:       models.AuthorizationModeFailOpenAdmin,
				StrictMode: false,
			},
			user:           memberUser,
			expectedStatus: http.StatusForbidden,
			expectedStop:   true,
		},
		{
			name: "FailOpenAdmin - System user denied",
			config: AuthorizationConfig{
				Mode:       models.AuthorizationModeFailOpenAdmin,
				StrictMode: false,
			},
			user:           systemUser,
			expectedStatus: http.StatusForbidden,
			expectedStop:   true,
		},
		{
			name: "FailOpenAdminSystem - Admin user allowed",
			config: AuthorizationConfig{
				Mode:       models.AuthorizationModeFailOpenAdminSystem,
				StrictMode: false,
			},
			user:           adminUser,
			expectedStatus: 0, // No response sent
			expectedStop:   false,
		},
		{
			name: "FailOpenAdminSystem - System user allowed",
			config: AuthorizationConfig{
				Mode:       models.AuthorizationModeFailOpenAdminSystem,
				StrictMode: false,
			},
			user:           systemUser,
			expectedStatus: 0, // No response sent
			expectedStop:   false,
		},
		{
			name: "FailOpenAdminSystem - Member user denied",
			config: AuthorizationConfig{
				Mode:       models.AuthorizationModeFailOpenAdminSystem,
				StrictMode: false,
			},
			user:           memberUser,
			expectedStatus: http.StatusForbidden,
			expectedStop:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			middleware := NewAuthorizationMiddlewareWithConfig(tt.config)

			// Create test request and response recorder
			req := httptest.NewRequest("GET", "/api/v1/undefined-endpoint", nil)
			w := httptest.NewRecorder()

			// Test the handleUndefinedEndpoint method
			stop := middleware.handleUndefinedEndpoint(w, req, tt.user)

			// Check if the function returned the expected stop value
			if stop != tt.expectedStop {
				t.Errorf("handleUndefinedEndpoint() stop = %v, want %v", stop, tt.expectedStop)
			}

			// Check the response status if a response was expected
			if tt.expectedStop && tt.expectedStatus > 0 {
				if w.Code != tt.expectedStatus {
					t.Errorf("handleUndefinedEndpoint() status = %v, want %v", w.Code, tt.expectedStatus)
				}
			}

			// If not stopping, make sure no error response was written (body should be empty)
			if !tt.expectedStop {
				if w.Body.Len() > 0 {
					t.Errorf("handleUndefinedEndpoint() wrote response body when it shouldn't have, body: %s", w.Body.String())
				}
			}
		})
	}
}

func TestAuthorizationMiddleware_Configuration(t *testing.T) {
	// Test default configuration
	defaultMiddleware := NewAuthorizationMiddleware()
	if defaultMiddleware.config.Mode != models.AuthorizationModeFailOpenAdminSystem {
		t.Errorf("Default mode should be FailOpenAdminSystem, got %v", defaultMiddleware.config.Mode)
	}
	if defaultMiddleware.config.StrictMode != false {
		t.Errorf("Default strict mode should be false, got %v", defaultMiddleware.config.StrictMode)
	}

	// Test custom configuration
	customConfig := AuthorizationConfig{
		Mode:       models.AuthorizationModeFailClosed,
		StrictMode: true,
	}
	customMiddleware := NewAuthorizationMiddlewareWithConfig(customConfig)
	if customMiddleware.config.Mode != models.AuthorizationModeFailClosed {
		t.Errorf("Custom mode should be FailClosed, got %v", customMiddleware.config.Mode)
	}
	if customMiddleware.config.StrictMode != true {
		t.Errorf("Custom strict mode should be true, got %v", customMiddleware.config.StrictMode)
	}
}
