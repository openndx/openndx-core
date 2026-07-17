package services

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"time"

	"github.com/OpenNDX/openndx-core/exchange/consent-engine/internal/models"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ConsentService provides business logic for consent operations
type ConsentService struct {
	db                   *gorm.DB
	consentPortalBaseURL string
}

// NewConsentService creates a new consent service
func NewConsentService(db *gorm.DB, consentPortalBaseURL string) (*ConsentService, error) {
	parsed, err := url.Parse(consentPortalBaseURL)
	if consentPortalBaseURL == "" || err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return nil, fmt.Errorf("invalid consentPortalBaseURL: must be a non-empty, valid URL with scheme and host")
	}
	return &ConsentService{
		db:                   db,
		consentPortalBaseURL: consentPortalBaseURL,
	}, nil
}

// CreateConsentRecord creates a new consent record in the database
func (s *ConsentService) CreateConsentRecord(ctx context.Context, req models.CreateConsentRequest) (*models.ConsentResponseInternalView, error) {
	// Validate input first
	if err := validateCreateConsentRequest(req); err != nil {
		return nil, fmt.Errorf("%w: %w", models.ErrConsentCreateFailed, err)
	}

	// First Check if a pending or approved consent already exists for the same (ownerID, appID)
	existingConsent, err := s.GetConsentInternalView(ctx, nil, &req.ConsentRequirement.OwnerID, &req.AppID)
	if err == nil {
		// An existing consent was found
		if existingConsent.Status == string(models.StatusPending) || existingConsent.Status == string(models.StatusApproved) {
			// Check if the fields match
			if areConsentFieldsEqual(existingConsent.Fields, &req.ConsentRequirement.Fields) {
				// Return the existing consent instead of creating a new one
				return existingConsent, nil
			}
			// Revoke the existing consent if fields do not match and create a new one
			// This operation must be transactional
			return s.revokeAndCreateConsent(ctx, existingConsent.ConsentID, req)
		}
	} else {
		// If the error is not "not found", return the error
		if !errors.Is(err, models.ErrConsentNotFound) {
			return nil, fmt.Errorf("%w: %w", models.ErrConsentCreateFailed, err)
		}
	}

	// Create new consent record
	consentRecord, err := s.buildConsentRecord(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", models.ErrConsentCreateFailed, err)
	}

	// Insert consent record
	if err := s.db.WithContext(ctx).Create(&consentRecord).Error; err != nil {
		return nil, fmt.Errorf("%w: %w", models.ErrConsentCreateFailed, err)
	}

	// Convert to internal view response
	internalView := consentRecord.ToConsentResponseInternalView()
	return &internalView, nil
}

// revokeAndCreateConsent revokes an existing consent and creates a new one in a single transaction
func (s *ConsentService) revokeAndCreateConsent(ctx context.Context, existingConsentID string, req models.CreateConsentRequest) (*models.ConsentResponseInternalView, error) {
	var newConsentRecord models.ConsentRecord

	// Execute revoke and create in a transaction
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Step 1: Revoke the existing consent
		var existingConsentRecord models.ConsentRecord
		parsedConsentID, err := uuid.Parse(existingConsentID)
		if err != nil {
			return fmt.Errorf("invalid consent ID: %w", err)
		}

		if err := tx.Where("consent_id = ?", parsedConsentID).First(&existingConsentRecord).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return fmt.Errorf("%w: %w", models.ErrConsentNotFound, err)
			}
			return fmt.Errorf("failed to find existing consent: %w", err)
		}

		if existingConsentRecord.Status != string(models.StatusApproved) && existingConsentRecord.Status != string(models.StatusPending) {
			return fmt.Errorf("only approved or pending consents can be revoked")
		}

		existingConsentRecord.Status = string(models.StatusRevoked)
		currentTime := time.Now().UTC()
		existingConsentRecord.UpdatedAt = currentTime
		revokedBy := models.RevokedByNewConsentWithDifferentFields
		existingConsentRecord.UpdatedBy = (*string)(&revokedBy)

		if err := tx.Save(&existingConsentRecord).Error; err != nil {
			return fmt.Errorf("failed to revoke existing consent: %w", err)
		}

		// Step 2: Create the new consent record
		newConsentRecordPtr, err := s.buildConsentRecord(req)
		if err != nil {
			return fmt.Errorf("failed to build new consent record: %w", err)
		}
		newConsentRecord = *newConsentRecordPtr

		if err := tx.Create(&newConsentRecord).Error; err != nil {
			return fmt.Errorf("failed to create new consent: %w", err)
		}

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("%w: %w", models.ErrConsentCreateFailed, err)
	}

	// Convert to internal view response
	internalView := newConsentRecord.ToConsentResponseInternalView()
	return &internalView, nil
}

// buildConsentRecord builds a ConsentRecord from the request
func (s *ConsentService) buildConsentRecord(req models.CreateConsentRequest) (*models.ConsentRecord, error) {
	// No need of Validate input
	// Validation is already performed by callers (CreateConsentRecord)

	consentID := uuid.New()
	currentTime := time.Now().UTC()

	if req.ConsentType == nil {
		defaultType := models.TypeRealtime
		req.ConsentType = &defaultType
	}
	pendingTimeout := parsePendingTimeoutDuration(*req.ConsentType)
	pendingExpiresAt := currentTime.Add(pendingTimeout)

	return &models.ConsentRecord{
		ConsentID:        consentID,
		OwnerID:          req.ConsentRequirement.OwnerID,
		AppID:            req.AppID,
		AppName:          req.AppName,
		Status:           string(models.StatusPending),
		Type:             string(*req.ConsentType),
		CreatedAt:        currentTime,
		UpdatedAt:        currentTime,
		GrantDuration:    string(getGrantDurationOrDefault((*models.GrantDuration)(req.GrantDuration))),
		Fields:           req.ConsentRequirement.Fields,
		ConsentPortalURL: fmt.Sprintf("%s?consentId=%s", s.consentPortalBaseURL, consentID.String()),
		PendingExpiresAt: &pendingExpiresAt,
	}, nil
}

// getGrantDurationOrDefault returns the provided grant duration or the default if empty
func getGrantDurationOrDefault(grantDuration *models.GrantDuration) models.GrantDuration {
	if grantDuration == nil || *grantDuration == "" {
		return models.DurationDefault
	}
	return *grantDuration
}

// GetConsentInternalView retrieves a consent record by ID or by (ownerID AND appID) and returns its internal view
func (s *ConsentService) GetConsentInternalView(ctx context.Context, consentID *string, ownerID *string, appID *string) (*models.ConsentResponseInternalView, error) {
	var consentRecord models.ConsentRecord
	query := s.db.WithContext(ctx).Model(&models.ConsentRecord{})

	if consentID != nil {
		parsedConsentID, err := uuid.Parse(*consentID)
		if err != nil {
			return nil, fmt.Errorf("%w: invalid consent ID", models.ErrConsentGetFailed)
		}
		query = query.Where("consent_id = ?", parsedConsentID)
	} else if ownerID != nil && appID != nil {
		query = query.Where("owner_id = ? AND app_id = ?", *ownerID, *appID)
	} else {
		return nil, fmt.Errorf("%w: either consentID or (ownerID and appID) must be provided", models.ErrConsentGetFailed)
	}

	// There is a possibility of multiple records for (ownerID, appID) if previous consents exist.
	// We fetch the latest one based on CreatedAt timestamp.
	// We can safely assume CreatedAt is unique for active consents due to the conditional unique constraint.
	query = query.Order("created_at DESC").Limit(1)

	if err := query.First(&consentRecord).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("%w: %w", models.ErrConsentNotFound, err)
		}
		return nil, fmt.Errorf("%w: %w", models.ErrConsentGetFailed, err)
	}

	// Either PendingExpiresAt or GrantExpiresAt will be nil depending on status
	// Check and update status to expired if necessary
	if consentRecord.PendingExpiresAt != nil && time.Now().UTC().After(*consentRecord.PendingExpiresAt) && consentRecord.Status == string(models.StatusPending) {
		consentRecord.Status = string(models.StatusExpired)
		if err := s.db.WithContext(ctx).Save(&consentRecord).Error; err != nil {
			return nil, fmt.Errorf("%w: %w", models.ErrConsentGetFailed, err)
		}
	} else if consentRecord.GrantExpiresAt != nil && time.Now().UTC().After(*consentRecord.GrantExpiresAt) && consentRecord.Status == string(models.StatusApproved) {
		consentRecord.Status = string(models.StatusExpired)
		if err := s.db.WithContext(ctx).Save(&consentRecord).Error; err != nil {
			return nil, fmt.Errorf("%w: %w", models.ErrConsentGetFailed, err)
		}
	}

	internalView := consentRecord.ToConsentResponseInternalView()
	return &internalView, nil
}

// GetConsentPortalView retrieves a consent record by ID and returns its portal view
func (s *ConsentService) GetConsentPortalView(ctx context.Context, consentID string) (*models.ConsentResponsePortalView, error) {
	var consentRecord models.ConsentRecord
	parsedConsentID, err := uuid.Parse(consentID)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid consent ID", models.ErrConsentGetFailed)
	}

	if err := s.db.WithContext(ctx).Where("consent_id = ?", parsedConsentID).First(&consentRecord).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("%w: %w", models.ErrConsentNotFound, err)
		}
		return nil, fmt.Errorf("%w: %w", models.ErrConsentGetFailed, err)
	}

	portalView := consentRecord.ToConsentResponsePortalView()
	return &portalView, nil
}

// UpdateConsentStatusByPortalAction updates the consent status based on user action from the consent portal
func (s *ConsentService) UpdateConsentStatusByPortalAction(ctx context.Context, req models.ConsentPortalActionRequest) error {
	// Validate action
	if !isValidConsentPortalAction(req.Action) {
		return fmt.Errorf("%w: invalid action: %s", models.ErrPortalRequestFailed, req.Action)
	}

	var consentRecord models.ConsentRecord
	parsedConsentID, err := uuid.Parse(req.ConsentID)
	if err != nil {
		return fmt.Errorf("%w: invalid consent ID", models.ErrPortalRequestFailed)
	}

	if err := s.db.WithContext(ctx).Where("consent_id = ?", parsedConsentID).First(&consentRecord).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("%w: %w", models.ErrConsentNotFound, err)
		}
		return fmt.Errorf("%w: %w", models.ErrConsentUpdateFailed, err)
	}

	currentTime := time.Now().UTC()
	consentRecord.UpdatedAt = currentTime
	consentRecord.UpdatedBy = &req.UpdatedBy

	switch req.Action {
	case models.ActionApprove:
		consentRecord.Status = string(models.StatusApproved)
		grantExpiresAt := currentTime.Add(parseGrantDuration((models.GrantDuration)(consentRecord.GrantDuration)))
		consentRecord.GrantExpiresAt = &grantExpiresAt
		consentRecord.PendingExpiresAt = nil
	case models.ActionReject:
		consentRecord.Status = string(models.StatusRejected)
		// Do not set GrantExpiresAt on rejection - only approval gets a grant expiry
		consentRecord.PendingExpiresAt = nil
	default:
		return fmt.Errorf("%w: invalid action: %s", models.ErrPortalRequestFailed, req.Action)
	}

	if err := s.db.WithContext(ctx).Save(&consentRecord).Error; err != nil {
		return fmt.Errorf("%w: %w", models.ErrConsentUpdateFailed, err)
	}

	return nil
}

// RevokeConsent revokes an existing approved or pending consent
func (s *ConsentService) RevokeConsent(ctx context.Context, consentID string, revokedBy string) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var consentRecord models.ConsentRecord
		parsedConsentID, err := uuid.Parse(consentID)
		if err != nil {
			return fmt.Errorf("%w: invalid consent ID", models.ErrConsentRevokeFailed)
		}

		if err := tx.Where("consent_id = ?", parsedConsentID).First(&consentRecord).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return fmt.Errorf("%w: %w", models.ErrConsentNotFound, err)
			}
			return fmt.Errorf("%w: %w", models.ErrConsentRevokeFailed, err)
		}

		if consentRecord.Status != string(models.StatusApproved) && consentRecord.Status != string(models.StatusPending) {
			return fmt.Errorf("%w: only approved or pending consents can be revoked", models.ErrConsentRevokeFailed)
		}

		consentRecord.Status = string(models.StatusRevoked)
		currentTime := time.Now().UTC()
		consentRecord.UpdatedAt = currentTime
		consentRecord.UpdatedBy = &revokedBy

		if err := tx.Save(&consentRecord).Error; err != nil {
			return fmt.Errorf("%w: %w", models.ErrConsentRevokeFailed, err)
		}

		return nil
	})
}

// parseGrantDuration parses the grant duration string into a time.Duration
func parseGrantDuration(grantDuration models.GrantDuration) time.Duration {
	switch grantDuration {
	case models.DurationOneHour:
		return time.Hour
	case models.DurationSixHours:
		return 6 * time.Hour
	case models.DurationTwelveHours:
		return 12 * time.Hour
	case models.DurationOneDay:
		return 24 * time.Hour
	case models.DurationSevenDays:
		return 7 * 24 * time.Hour
	case models.DurationThirtyDays:
		return 30 * 24 * time.Hour
	default:
		return time.Hour // Default to 1 hour if unrecognized
	}
}

// parsePendingTimeoutDuration returns the pending timeout duration based on consent type
func parsePendingTimeoutDuration(consentType models.ConsentType) time.Duration {
	switch consentType {
	case models.TypeRealtime:
		return time.Hour // 1 hour for realtime
	case models.TypeOffline:
		return 24 * time.Hour // 1 day for offline
	default:
		return time.Hour // Default to 1 hour if unrecognized
	}
}

// areConsentFieldsEqual checks if two slices of ConsentField are equal
func areConsentFieldsEqual(a, b *[]models.ConsentField) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	if len(*a) != len(*b) {
		return false
	}

	for i := range *a {
		if !(*a)[i].Equals((*b)[i]) {
			return false
		}
	}
	return true
}

// validateCreateConsentRequest validates the create consent request input
func validateCreateConsentRequest(req models.CreateConsentRequest) error {
	if req.AppID == "" {
		return errors.New("appId is required")
	}

	// Validate grant duration if provided
	if req.GrantDuration != nil && *req.GrantDuration != "" {
		if !isValidGrantDuration(models.GrantDuration(*req.GrantDuration)) {
			return fmt.Errorf("invalid grantDuration: %s", *req.GrantDuration)
		}
	}

	if req.ConsentRequirement.OwnerID == "" {
		return fmt.Errorf("consentRequirement.ownerId is required")
	}
	if len(req.ConsentRequirement.Fields) == 0 {
		return fmt.Errorf("consentRequirement.fields cannot be empty")
	}

	// Validate each field
	for j, field := range req.ConsentRequirement.Fields {
		if field.FieldName == "" {
			return fmt.Errorf("consentRequirement.fields[%d].fieldName is required", j)
		}
		if field.SchemaID == "" {
			return fmt.Errorf("consentRequirement.fields[%d].schemaId is required", j)
		}
	}

	return nil
}

// isValidGrantDuration checks if a grant duration is valid
func isValidGrantDuration(grantDuration models.GrantDuration) bool {
	switch grantDuration {
	case models.DurationOneHour,
		models.DurationSixHours,
		models.DurationTwelveHours,
		models.DurationOneDay,
		models.DurationSevenDays,
		models.DurationThirtyDays:
		return true
	default:
		return false
	}
}

// isValidConsentPortalAction checks if a consent portal action is valid
func isValidConsentPortalAction(action models.ConsentPortalAction) bool {
	switch action {
	case models.ActionApprove, models.ActionReject:
		return true
	default:
		return false
	}
}
