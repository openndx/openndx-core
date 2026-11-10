package application

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/openndx/openndx-core/internal/pb/policy"
)

// ErrConsentBypassGrantNotFound is returned when a grant does not exist in the
// PDP or is not a consent bypass grant.
var ErrConsentBypassGrantNotFound = errors.New("consent bypass grant not found")

// ErrInvalidConsentBypassRequest is returned for a consent bypass request PB
// rejects before asking the PDP.
var ErrInvalidConsentBypassRequest = errors.New("invalid consent bypass request")

// GrantConsentBypass lets one application read fields without the owner's
// consent, for the requested duration and with a legal basis. The grants are
// active at once. A field the application already holds a bypass grant for
// has that grant renewed with the new duration and legal basis.
func (s *Service) GrantConsentBypass(ctx context.Context, applicationID string, req *ConsentBypassRequest) ([]ConsentBypassGrantResponse, error) {
	if len(req.Fields) == 0 {
		return nil, fmt.Errorf("%w: fields is required", ErrInvalidConsentBypassRequest)
	}
	if strings.TrimSpace(req.LegalBasis) == "" {
		return nil, fmt.Errorf("%w: legalBasis is required", ErrInvalidConsentBypassRequest)
	}
	if req.GrantDuration == "" {
		return nil, fmt.Errorf("%w: grantDuration is required", ErrInvalidConsentBypassRequest)
	}

	var application Application
	if err := s.db.WithContext(ctx).First(&application, "application_id = ?", applicationID).Error; err != nil {
		return nil, err
	}
	if application.IdpClientID == nil {
		return nil, fmt.Errorf("cannot grant consent bypass: application IdpClientID is nil")
	}

	// Grants are keyed by the IdP client_id, as for the allow-list.
	resp, err := s.policyService.CreateAccessGrants(policy.AccessGrantCreateRequest{
		ApplicationID: *application.IdpClientID,
		Records:       req.Fields,
		GrantDuration: req.GrantDuration,
		ConsentBypass: true,
		LegalBasis:    req.LegalBasis,
	})
	if err != nil {
		return nil, err
	}

	applicationIDs := map[string]string{*application.IdpClientID: application.ApplicationID}
	return toConsentBypassGrants(resp.Records, applicationIDs), nil
}

// ListConsentBypassGrants lists consent bypass grants, optionally for one
// application and with one status.
func (s *Service) ListConsentBypassGrants(ctx context.Context, applicationID, status string) ([]ConsentBypassGrantResponse, error) {
	bypass := true
	filter := policy.AccessGrantFilter{Status: status, ConsentBypass: &bypass}

	if applicationID != "" {
		var application Application
		if err := s.db.WithContext(ctx).First(&application, "application_id = ?", applicationID).Error; err != nil {
			return nil, err
		}
		if application.IdpClientID == nil {
			return []ConsentBypassGrantResponse{}, nil
		}
		filter.ApplicationID = *application.IdpClientID
	}

	resp, err := s.policyService.ListAccessGrants(filter)
	if err != nil {
		return nil, err
	}

	applicationIDs, err := s.applicationIDsByClientID(ctx, resp.Records)
	if err != nil {
		return nil, err
	}
	return toConsentBypassGrants(resp.Records, applicationIDs), nil
}

// RevokeConsentBypass revokes an active consent bypass grant. A plain grant's
// ID is not found, so this route can't revoke allow-list access.
func (s *Service) RevokeConsentBypass(ctx context.Context, grantID string) (*ConsentBypassGrantResponse, error) {
	existing, err := s.policyService.GetAccessGrant(grantID)
	var pdpErr *policy.Error
	if errors.As(err, &pdpErr) && pdpErr.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("%w: %s", ErrConsentBypassGrantNotFound, grantID)
	}
	if err != nil {
		return nil, err
	}
	if !existing.ConsentBypass {
		return nil, fmt.Errorf("%w: %s", ErrConsentBypassGrantNotFound, grantID)
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
	response := toConsentBypassGrants([]policy.AccessGrant{*grant}, applicationIDs)[0]
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

	var applications []Application
	if err := s.db.WithContext(ctx).
		Select("application_id", "idp_client_id").
		Where("idp_client_id IN ?", clientIDs).
		Find(&applications).Error; err != nil {
		return nil, fmt.Errorf("failed to look up applications: %w", err)
	}
	for _, application := range applications {
		if application.IdpClientID != nil {
			byClientID[*application.IdpClientID] = application.ApplicationID
		}
	}
	return byClientID, nil
}

func toConsentBypassGrants(grants []policy.AccessGrant, applicationIDs map[string]string) []ConsentBypassGrantResponse {
	responses := make([]ConsentBypassGrantResponse, 0, len(grants))
	for _, grant := range grants {
		responses = append(responses, ConsentBypassGrantResponse{
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
