// Package consentbypass is the Portal Backend's consent bypass bounded
// context: granting an application access to fields without the owner's
// consent, for a duration and with a legal basis, listing those grants and
// revoking them. The grants live in the PDP, keyed by the application's IdP
// client_id; PB stores nothing of its own and reads applications only to map
// between the PB application ID and the IdP client_id.
package consentbypass

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/openndx/openndx-core/internal/pb/application"
	"github.com/openndx/openndx-core/internal/pb/policy"
	"gorm.io/gorm"
)

// ErrGrantNotFound is returned when a grant does not exist in the PDP or is
// not a consent bypass grant.
var ErrGrantNotFound = errors.New("consent bypass grant not found")

// ErrInvalidRequest is returned for a consent bypass request PB rejects
// before asking the PDP.
var ErrInvalidRequest = errors.New("invalid consent bypass request")

// Service grants, lists and revokes consent bypass grants in the PDP.
type Service struct {
	db            *gorm.DB
	policyService *policy.Client
}

// NewService creates a consent bypass service.
func NewService(db *gorm.DB, policyService *policy.Client) *Service {
	return &Service{db: db, policyService: policyService}
}

// Grant lets one application read fields without the owner's consent, for
// the requested duration and with a legal basis. The grants are active at
// once. A field the application already holds a bypass grant for has that
// grant renewed with the new duration and legal basis.
func (s *Service) Grant(ctx context.Context, applicationID string, req *GrantRequest) ([]Grant, error) {
	if len(req.Fields) == 0 {
		return nil, fmt.Errorf("%w: fields is required", ErrInvalidRequest)
	}
	if strings.TrimSpace(req.LegalBasis) == "" {
		return nil, fmt.Errorf("%w: legalBasis is required", ErrInvalidRequest)
	}
	if req.GrantDuration == "" {
		return nil, fmt.Errorf("%w: grantDuration is required", ErrInvalidRequest)
	}

	var app application.Application
	if err := s.db.WithContext(ctx).First(&app, "application_id = ?", applicationID).Error; err != nil {
		return nil, err
	}
	if app.IdpClientID == nil {
		return nil, fmt.Errorf("cannot grant consent bypass: application IdpClientID is nil")
	}

	// Grants are keyed by the IdP client_id, as for the allow-list.
	resp, err := s.policyService.CreateAccessGrants(policy.AccessGrantCreateRequest{
		ApplicationID: *app.IdpClientID,
		Records:       req.Fields,
		GrantDuration: req.GrantDuration,
		ConsentBypass: true,
		LegalBasis:    req.LegalBasis,
	})
	if err != nil {
		return nil, err
	}

	applicationIDs := map[string]string{*app.IdpClientID: app.ApplicationID}
	return toGrants(resp.Records, applicationIDs), nil
}

// List lists consent bypass grants, optionally for one application and with
// one status.
func (s *Service) List(ctx context.Context, applicationID, status string) ([]Grant, error) {
	bypass := true
	filter := policy.AccessGrantFilter{Status: status, ConsentBypass: &bypass}

	if applicationID != "" {
		var app application.Application
		if err := s.db.WithContext(ctx).First(&app, "application_id = ?", applicationID).Error; err != nil {
			return nil, err
		}
		if app.IdpClientID == nil {
			return []Grant{}, nil
		}
		filter.ApplicationID = *app.IdpClientID
	}

	resp, err := s.policyService.ListAccessGrants(filter)
	if err != nil {
		return nil, err
	}

	applicationIDs, err := s.applicationIDsByClientID(ctx, resp.Records)
	if err != nil {
		return nil, err
	}
	return toGrants(resp.Records, applicationIDs), nil
}

// Revoke revokes an active consent bypass grant. A plain grant's ID is not
// found, so this can't revoke allow-list access.
func (s *Service) Revoke(ctx context.Context, grantID string) (*Grant, error) {
	existing, err := s.policyService.GetAccessGrant(grantID)
	var pdpErr *policy.Error
	if errors.As(err, &pdpErr) && pdpErr.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("%w: %s", ErrGrantNotFound, grantID)
	}
	if err != nil {
		return nil, err
	}
	if !existing.ConsentBypass {
		return nil, fmt.Errorf("%w: %s", ErrGrantNotFound, grantID)
	}

	grant, err := s.policyService.RevokeAccessGrant(grantID)
	if err != nil {
		return nil, err
	}

	// The PDP has revoked the grant. The lookup only fills the display-only
	// applicationId, so its failure must not report the revoke as failed.
	applicationIDs, err := s.applicationIDsByClientID(ctx, []policy.AccessGrant{*grant})
	if err != nil {
		slog.Warn("Consent bypass revoked, but application lookup failed", "grantId", grantID, "error", err)
		applicationIDs = map[string]string{}
	}
	response := toGrants([]policy.AccessGrant{*grant}, applicationIDs)[0]
	return &response, nil
}

// applicationIDsByClientID maps the grants' IdP client IDs to PB application IDs.
func (s *Service) applicationIDsByClientID(ctx context.Context, grants []policy.AccessGrant) (map[string]string, error) {
	clientIDs := make([]string, 0, len(grants))
	for _, grant := range grants {
		clientIDs = append(clientIDs, grant.ApplicationID)
	}
	byClientID := make(map[string]string, len(clientIDs))
	if len(clientIDs) == 0 {
		return byClientID, nil
	}

	var applications []application.Application
	if err := s.db.WithContext(ctx).
		Select("application_id", "idp_client_id").
		Where("idp_client_id IN ?", clientIDs).
		Find(&applications).Error; err != nil {
		return nil, fmt.Errorf("failed to look up applications: %w", err)
	}
	for _, app := range applications {
		if app.IdpClientID != nil {
			byClientID[*app.IdpClientID] = app.ApplicationID
		}
	}
	return byClientID, nil
}

func toGrants(grants []policy.AccessGrant, applicationIDs map[string]string) []Grant {
	responses := make([]Grant, 0, len(grants))
	for _, grant := range grants {
		responses = append(responses, Grant{
			ID:            grant.ID,
			ApplicationID: applicationIDs[grant.ApplicationID],
			IdpClientID:   grant.ApplicationID,
			SchemaID:      grant.SchemaID,
			FieldName:     grant.FieldName,
			Status:        grant.Status,
			LegalBasis:    grant.LegalBasis,
			ExpiresAt:     grant.ExpiresAt,
			CreatedAt:     grant.CreatedAt,
			UpdatedAt:     grant.UpdatedAt,
		})
	}
	return responses
}
