// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Lanka Software Foundation

package caller_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/openndx/openndx-core/internal/pb/caller"
	"github.com/openndx/openndx-core/internal/pb/caller/callertest"
	"github.com/stretchr/testify/assert"
)

func TestCanAccess(t *testing.T) {
	member := callertest.MemberUser.WithMember("mem_own")

	tests := []struct {
		name    string
		req     *http.Request
		ownerID string
		want    bool
	}{
		{"own member", callertest.NewAuthenticatedRequest(http.MethodGet, "/", nil, member), "mem_own", true},
		{"another member", callertest.NewAuthenticatedRequest(http.MethodGet, "/", nil, member), "mem_other", false},
		{"no member on the context", callertest.NewMemberRequest(http.MethodGet, "/", nil), "mem_own", false},
		{"no member never matches an empty owner", callertest.NewMemberRequest(http.MethodGet, "/", nil), "", false},
		{"-all scope covers another member", callertest.NewAdminRequest(http.MethodGet, "/", nil), "mem_other", true},
		{"unauthenticated", callertest.NewUnauthenticatedRequest(http.MethodGet, "/", nil), "mem_own", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, caller.CanAccess(tt.req.Context(), caller.ScopeSchemaReadAll, tt.ownerID))
		})
	}
}

func TestMemberID_IgnoresEmpty(t *testing.T) {
	_, ok := caller.MemberID(caller.WithMemberID(context.Background(), ""))
	assert.False(t, ok)
}

func TestMiddleware(t *testing.T) {
	lookup := func(_ context.Context, ouID string) (string, error) {
		switch ouID {
		case callertest.MemberUser.OuID:
			return "mem_456", nil
		case "broken-ou":
			return "", errors.New("database unavailable")
		default:
			return "", nil
		}
	}

	// serve runs req through the middleware and reports the member the next handler saw
	serve := func(req *http.Request) (int, string, bool) {
		var memberID string
		var found, called bool
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			called = true
			memberID, found = caller.MemberID(r.Context())
		})
		w := httptest.NewRecorder()
		caller.Middleware(lookup)(next).ServeHTTP(w, req)
		if !called {
			return w.Code, "", false
		}
		return w.Code, memberID, found
	}

	t.Run("caller whose organization unit has a member", func(t *testing.T) {
		code, memberID, found := serve(callertest.NewMemberRequest(http.MethodGet, "/", nil))
		assert.Equal(t, http.StatusOK, code)
		assert.True(t, found)
		assert.Equal(t, "mem_456", memberID)
	})

	t.Run("caller whose organization unit has no member", func(t *testing.T) {
		code, _, found := serve(callertest.NewAdminRequest(http.MethodGet, "/", nil))
		assert.Equal(t, http.StatusOK, code)
		assert.False(t, found)
	})

	t.Run("caller without an organization unit", func(t *testing.T) {
		service := callertest.CreateCustomTestUser("service-client", "", "", callertest.ReadAllScopes)
		code, _, found := serve(callertest.NewAuthenticatedRequest(http.MethodGet, "/", nil, service))
		assert.Equal(t, http.StatusOK, code)
		assert.False(t, found)
	})

	t.Run("unauthenticated request", func(t *testing.T) {
		code, _, found := serve(callertest.NewUnauthenticatedRequest(http.MethodGet, "/", nil))
		assert.Equal(t, http.StatusOK, code)
		assert.False(t, found)
	})

	t.Run("lookup failure", func(t *testing.T) {
		broken := callertest.CreateCustomTestUser("broken-user", "", "broken-ou", callertest.MemberScopes)
		code, _, found := serve(callertest.NewAuthenticatedRequest(http.MethodGet, "/", nil, broken))
		assert.Equal(t, http.StatusInternalServerError, code)
		assert.False(t, found)
	})
}
