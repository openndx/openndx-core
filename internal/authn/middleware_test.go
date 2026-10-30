// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Lanka Software Foundation

package authn

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestAuthMiddleware_NoToken tests middleware when no auth header provided
func TestAuthMiddleware_NoToken(t *testing.T) {
	// Create a test handler that checks for auth context
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authCtx := GetAuthContext(r.Context())
		if authCtx != nil {
			t.Error("expected no auth context when no token provided")
		}
		w.WriteHeader(http.StatusOK)
	})

	// A nil TokenExtractor is acceptable here: with no token the middleware
	// never uses it.
	middleware := Middleware(nil)
	handlerWithMiddleware := middleware(testHandler)

	// Make a test request without Authorization header
	req := httptest.NewRequest("GET", "http://example.com/test", nil)
	recorder := httptest.NewRecorder()

	handlerWithMiddleware.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", recorder.Code)
	}
}

// TestAuthMiddleware_UninitializedTokenExtractor tests middleware returns 500 when tokenExtractor is nil
func TestAuthMiddleware_UninitializedDependencies(t *testing.T) {
	testHandlerCalled := false
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testHandlerCalled = true
		w.WriteHeader(http.StatusOK)
	})

	// With nil tokenExtractor, middleware should return 500
	middleware := Middleware(nil)
	handlerWithMiddleware := middleware(testHandler)

	req := httptest.NewRequest("GET", "http://example.com/test", nil)
	req.Header.Set("Authorization", "Bearer invalid.jwt.token")
	recorder := httptest.NewRecorder()

	handlerWithMiddleware.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusInternalServerError {
		t.Errorf("expected status 500, got %d", recorder.Code)
	}
	if testHandlerCalled {
		t.Error("expected handler not to be called when tokenExtractor is nil")
	}
}

// TestAuthMiddleware_InvalidToken tests middleware returns 401 for invalid auth token
func TestAuthMiddleware_InvalidToken(t *testing.T) {
	testHandlerCalled := false
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testHandlerCalled = true
		w.WriteHeader(http.StatusOK)
	})

	tokenExtractor, _, cleanup := newTokenExtractor(t)
	defer cleanup()
	middleware := Middleware(tokenExtractor)
	handlerWithMiddleware := middleware(testHandler)

	req := httptest.NewRequest("GET", "http://example.com/test", nil)
	req.Header.Set("Authorization", "Bearer invalid.jwt.token")
	recorder := httptest.NewRecorder()

	handlerWithMiddleware.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusUnauthorized {
		t.Errorf("expected status 401, got %d", recorder.Code)
	}
	if testHandlerCalled {
		t.Error("expected handler not to be called for invalid token")
	}
}

func TestAuthMiddleware_ValidClientCredentialsToken(t *testing.T) {
	tokenExtractor, privateKey, cleanup := newTokenExtractor(t)
	defer cleanup()

	signedToken := newClientToken(t, privateKey)

	testHandlerCalled := false
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testHandlerCalled = true
		authCtx := GetAuthContext(r.Context())
		if authCtx == nil {
			t.Fatalf("expected auth context")
			return
		}
		if authCtx.ClientID() != testClientID {
			t.Fatalf("expected client id %s, got %q", testClientID, authCtx.ClientID())
		}
		if authCtx.Subject() != "FCAU_TO_NSW" {
			t.Fatalf("expected subject FCAU_TO_NSW, got %q", authCtx.Subject())
		}
		w.WriteHeader(http.StatusOK)
	})

	handlerWithMiddleware := Middleware(tokenExtractor)(testHandler)
	req := httptest.NewRequest(http.MethodGet, "http://example.com/test", nil)
	req.Header.Set("Authorization", "Bearer "+signedToken)
	recorder := httptest.NewRecorder()

	handlerWithMiddleware.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", recorder.Code)
	}
	if !testHandlerCalled {
		t.Fatalf("expected handler to be called for valid token")
	}
}

func TestAuthMiddleware_ValidUserToken(t *testing.T) {
	tokenExtractor, privateKey, cleanup := newTokenExtractor(t)
	defer cleanup()

	signedToken := newUserToken(t, privateKey)

	testHandlerCalled := false
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testHandlerCalled = true
		authCtx := GetAuthContext(r.Context())
		if authCtx == nil {
			t.Fatalf("expected auth context")
		}
		if authCtx.Subject() != testUserID {
			t.Fatalf("expected subject %s, got %s", testUserID, authCtx.Subject())
		}
		if authCtx.ClientID() != testClientID {
			t.Fatalf("expected client id %s, got %s", testClientID, authCtx.ClientID())
		}
		if roles := authCtx.Roles(); len(roles) != 1 || roles[0] != "exporter" {
			t.Fatalf("expected roles [exporter], got %v", roles)
		}
		w.WriteHeader(http.StatusOK)
	})

	handlerWithMiddleware := Middleware(tokenExtractor)(testHandler)
	req := httptest.NewRequest(http.MethodGet, "http://example.com/test", nil)
	req.Header.Set("Authorization", "Bearer "+signedToken)
	recorder := httptest.NewRecorder()

	handlerWithMiddleware.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", recorder.Code)
	}
	if !testHandlerCalled {
		t.Fatalf("expected handler to be called for valid token")
	}
}

func TestAuthMiddleware_IdentityClaims(t *testing.T) {
	tokenExtractor, privateKey, cleanup := newTokenExtractor(t)
	defer cleanup()

	signedToken := newUserToken(t, privateKey)

	testHandlerCalled := false
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testHandlerCalled = true
		authCtx := GetAuthContext(r.Context())
		if authCtx.Email() != testEmail || authCtx.OUID() != testOUID || authCtx.OUHandle() != testOUHandle {
			t.Fatalf("unexpected standard claims: %+v", authCtx.Principal)
		}
		if got := authCtx.Claims().String("phone_number"); got != testPhone {
			t.Fatalf("phone_number = %q, want %q", got, testPhone)
		}
		w.WriteHeader(http.StatusOK)
	})

	handlerWithMiddleware := Middleware(tokenExtractor)(testHandler)
	req := httptest.NewRequest(http.MethodGet, "http://example.com/test", nil)
	req.Header.Set("Authorization", "Bearer "+signedToken)
	recorder := httptest.NewRecorder()

	handlerWithMiddleware.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", recorder.Code)
	}
	if !testHandlerCalled {
		t.Fatalf("expected handler to be called for valid token")
	}
}

// TestRequireAuth_ChainedApplicationMiddleware locks in the extension point
// that replaced UserProfileService: an application middleware chained after
// RequireAuth sees the validated principal and can enrich the request context
// before the handler runs.
func TestRequireAuth_ChainedApplicationMiddleware(t *testing.T) {
	tokenExtractor, privateKey, cleanup := newTokenExtractor(t)
	defer cleanup()

	type userKey struct{}
	provision := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			subject := GetAuthContext(r.Context()).Subject()
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), userKey{}, "user-for-"+subject)))
		})
	}

	var got any
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Context().Value(userKey{})
		w.WriteHeader(http.StatusOK)
	})

	protected := RequireAuth(tokenExtractor)(provision(testHandler))
	req := httptest.NewRequest(http.MethodGet, "http://example.com/protected", nil)
	req.Header.Set("Authorization", "Bearer "+newUserToken(t, privateKey))
	recorder := httptest.NewRecorder()

	protected.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", recorder.Code)
	}
	if got != "user-for-"+testUserID {
		t.Fatalf("expected chained middleware value user-for-%s, got %v", testUserID, got)
	}
}

func TestRequireAuth_UnauthenticatedRequest(t *testing.T) {
	handlerCalled := false
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
		w.WriteHeader(http.StatusOK)
	})

	tokenExtractor, _, cleanup := newTokenExtractor(t)
	defer cleanup()
	protected := RequireAuth(tokenExtractor)(testHandler)
	req := httptest.NewRequest(http.MethodGet, "http://example.com/protected", nil)
	recorder := httptest.NewRecorder()

	protected.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401, got %d", recorder.Code)
	}
	if handlerCalled {
		t.Fatalf("expected protected handler not to be called")
	}
}

func TestRequireAuth_ValidClientCredentialsToken(t *testing.T) {
	tokenExtractor, privateKey, cleanup := newTokenExtractor(t)
	defer cleanup()

	signedToken := newClientToken(t, privateKey)

	handlerCalled := false
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
		w.WriteHeader(http.StatusOK)
	})

	protected := RequireAuth(tokenExtractor)(testHandler)
	req := httptest.NewRequest(http.MethodGet, "http://example.com/protected", nil)
	req.Header.Set("Authorization", "Bearer "+signedToken)
	recorder := httptest.NewRecorder()

	protected.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", recorder.Code)
	}
	if !handlerCalled {
		t.Fatalf("expected protected handler to be called")
	}
}
