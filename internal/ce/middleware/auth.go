package middleware

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/openndx/openndx-core/internal/authn"
	"github.com/openndx/openndx-core/internal/ce/models"
	"github.com/openndx/openndx-core/internal/ce/utils"
)

// contextKey is a custom type for context keys used with context.WithValue.
// Defining a custom type helps avoid key collisions with context keys defined in other packages.
type contextKey string

const (
	// ownerSubjectKey is the context key for the authenticated owner's subject (UID)
	ownerSubjectKey contextKey = "ownerSubject"
)

// SubjectClaim is the standard token claim that identifies the user.
const SubjectClaim = "sub"

// OwnerSubject returns middleware that adds the data owner's UID to the request
// context. It must run after internal/authn's RequireAuthMiddleware, which has
// already validated the token. The UID is read from claim: "sub" is the token's
// subject; standard identity claims use their dedicated accessors, and other
// claims (e.g. "nic") are read from extra claims. A token without the claim is
// rejected with 401.
func OwnerSubject(claim string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authCtx := authn.GetAuthContext(r.Context())
			var subject string
			switch claim {
			case SubjectClaim:
				subject = authCtx.Subject()
			case "email":
				subject = authCtx.Email()
			case "ouId":
				subject = authCtx.OUID()
			case "ouHandle":
				subject = authCtx.OUHandle()
			default:
				subject = authCtx.ExtraClaims().String(claim)
			}
			if subject == "" {
				slog.Warn("Token has no owner subject claim", "claim", claim)
				utils.RespondWithError(w, http.StatusUnauthorized, models.ErrorCodeUnauthorized, "Token does not identify the data owner")
				return
			}

			slog.Debug("User authenticated", "subject", subject)
			next.ServeHTTP(w, r.WithContext(WithOwnerSubject(r.Context(), subject)))
		})
	}
}

// WithOwnerSubject returns a copy of ctx carrying the authenticated owner's subject (UID)
func WithOwnerSubject(ctx context.Context, subject string) context.Context {
	return context.WithValue(ctx, ownerSubjectKey, subject)
}

// GetOwnerSubjectFromContext extracts the authenticated owner's subject (UID) from the request context
func GetOwnerSubjectFromContext(ctx context.Context) (string, bool) {
	subject, ok := ctx.Value(ownerSubjectKey).(string)
	return subject, ok
}
