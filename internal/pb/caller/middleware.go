// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Lanka Software Foundation

package caller

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/openndx/openndx-core/internal/utils"
)

// MemberLookup returns the ID of the member linked to an organization unit,
// or "" when no member is.
type MemberLookup func(ctx context.Context, ouID string) (string, error)

// Middleware puts the caller's member on the request context. Chain it after
// internal/authn. A caller without an organization unit, or whose
// organization unit has no member, proceeds without one; handlers then limit
// it to nothing unless it holds the matching -all scope.
func Middleware(lookup MemberLookup) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ouID := OUID(r.Context())
			if ouID == "" {
				next.ServeHTTP(w, r)
				return
			}

			memberID, err := lookup(r.Context(), ouID)
			if err != nil {
				slog.ErrorContext(r.Context(), "Failed to look up the caller's member", "ouId", ouID, "error", err)
				utils.RespondWithError(w, http.StatusInternalServerError, "Failed to look up the caller's member")
				return
			}
			if memberID == "" {
				next.ServeHTTP(w, r)
				return
			}

			next.ServeHTTP(w, r.WithContext(WithMemberID(r.Context(), memberID)))
		})
	}
}
