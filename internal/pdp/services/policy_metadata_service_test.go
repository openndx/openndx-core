package services

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/openndx/openndx-core/internal/pdp/fieldpolicy"
	"github.com/openndx/openndx-core/internal/pdp/models"
	"github.com/openndx/openndx-core/internal/pdp/pdptest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// setupTestDB creates an in-memory SQLite database for unit testing.
func setupTestDB(t *testing.T) *gorm.DB {
	return pdptest.SetupTestDB(t)
}

func TestNewPolicyMetadataService(t *testing.T) {
	db := setupTestDB(t)
	service := NewPolicyMetadataService(db, fieldpolicy.NewService(db))
	assert.NotNil(t, service)
	assert.NotNil(t, service.db)
	assert.NotNil(t, service.fields)
}

func TestPolicyMetadataService_RevokeAccessGrant(t *testing.T) {
	db := setupTestDB(t)
	service := NewPolicyMetadataService(db, fieldpolicy.NewService(db))

	_, err := fieldpolicy.NewService(db).CreatePolicyMetadata(&fieldpolicy.PolicyMetadataCreateRequest{
		SchemaID: "schema-123",
		Records: []fieldpolicy.PolicyMetadataCreateRequestRecord{
			{FieldName: "person.email", Source: fieldpolicy.SourcePrimary, RequiresAllowlist: new(true), RequiresConsent: new(false)},
		},
	})
	require.NoError(t, err)

	grantIDs := map[string]string{}
	for _, applicationID := range []string{"app-123", "app-456"} {
		resp, err := service.CreateAccessGrants(&models.AccessGrantCreateRequest{
			ApplicationID: applicationID,
			GrantDuration: models.GrantDurationTypeOneMonth,
			Records: []models.AccessGrantCreateRequestRecord{
				{FieldName: "person.email", SchemaID: "schema-123"},
			},
		})
		require.NoError(t, err)
		require.Len(t, resp.Records, 1)
		grantIDs[applicationID] = resp.Records[0].ID
	}

	revoked, err := service.RevokeAccessGrant(grantIDs["app-123"])
	require.NoError(t, err)
	assert.Equal(t, models.GrantStatusRevoked, revoked.Status)

	decide := func(applicationID string) []models.PolicyDecisionFieldOutcome {
		resp, err := service.GetPolicyDecision(t.Context(), &models.PolicyDecisionRequest{
			ApplicationID:  applicationID,
			RequiredFields: []models.PolicyDecisionRequestRecord{{FieldName: "person.email", SchemaID: "schema-123"}},
		})
		require.NoError(t, err)
		return resp.Fields
	}
	assert.Equal(t, "deny", decide("app-123")[0].Outcome)
	assert.Equal(t, "allow", decide("app-456")[0].Outcome)

	// The revoked grant is kept as a record.
	revokedOnly, err := service.ListAccessGrants(models.AccessGrantFilter{Status: models.GrantStatusRevoked})
	require.NoError(t, err)
	require.Len(t, revokedOnly.Records, 1)
	assert.Equal(t, "app-123", revokedOnly.Records[0].ApplicationID)

	_, err = service.RevokeAccessGrant(grantIDs["app-123"])
	assert.ErrorIs(t, err, ErrAccessGrantNotFound)

	_, err = service.RevokeAccessGrant("not-a-uuid")
	assert.ErrorIs(t, err, ErrAccessGrantNotFound)

	// Granting again after a revoke creates a new active grant.
	resp, err := service.CreateAccessGrants(&models.AccessGrantCreateRequest{
		ApplicationID: "app-123",
		GrantDuration: models.GrantDurationTypeOneMonth,
		Records:       []models.AccessGrantCreateRequestRecord{{FieldName: "person.email", SchemaID: "schema-123"}},
	})
	require.NoError(t, err)
	assert.NotEqual(t, grantIDs["app-123"], resp.Records[0].ID)
	assert.Equal(t, "allow", decide("app-123")[0].Outcome)
}

func TestPolicyMetadataService_ListAccessGrants(t *testing.T) {
	db := setupTestDB(t)
	service := NewPolicyMetadataService(db, fieldpolicy.NewService(db))

	for _, schemaID := range []string{"schema-1", "schema-2"} {
		_, err := fieldpolicy.NewService(db).CreatePolicyMetadata(&fieldpolicy.PolicyMetadataCreateRequest{
			SchemaID: schemaID,
			Records: []fieldpolicy.PolicyMetadataCreateRequestRecord{
				{FieldName: "field1", Source: fieldpolicy.SourcePrimary, RequiresAllowlist: new(true), RequiresConsent: new(false)},
			},
		})
		require.NoError(t, err)
	}
	for _, applicationID := range []string{"app-A", "app-B"} {
		_, err := service.CreateAccessGrants(&models.AccessGrantCreateRequest{
			ApplicationID: applicationID,
			GrantDuration: models.GrantDurationTypeOneMonth,
			Records: []models.AccessGrantCreateRequestRecord{
				{FieldName: "field1", SchemaID: "schema-1"},
				{FieldName: "field1", SchemaID: "schema-2"},
			},
		})
		require.NoError(t, err)
	}

	all, err := service.ListAccessGrants(models.AccessGrantFilter{})
	require.NoError(t, err)
	assert.Len(t, all.Records, 4)

	byApp, err := service.ListAccessGrants(models.AccessGrantFilter{ApplicationID: "app-A"})
	require.NoError(t, err)
	require.Len(t, byApp.Records, 2)
	assert.Equal(t, "schema-1", byApp.Records[0].SchemaID)
	assert.Equal(t, "schema-2", byApp.Records[1].SchemaID)

	bySchemaAndApp, err := service.ListAccessGrants(models.AccessGrantFilter{ApplicationID: "app-B", SchemaID: "schema-2"})
	require.NoError(t, err)
	require.Len(t, bySchemaAndApp.Records, 1)
	assert.Equal(t, "app-B", bySchemaAndApp.Records[0].ApplicationID)

	none, err := service.ListAccessGrants(models.AccessGrantFilter{ApplicationID: "app-C"})
	require.NoError(t, err)
	assert.NotNil(t, none.Records, "records serializes as [] rather than null")
	assert.Empty(t, none.Records)

	_, err = service.ListAccessGrants(models.AccessGrantFilter{Status: "pending"})
	assert.ErrorIs(t, err, ErrInvalidGrantRequest)
}

// insertGrant stores an active grant directly, for example one that has
// already expired.
func insertGrant(t *testing.T, db *gorm.DB, schemaID, fieldName, applicationID string, expiresAt time.Time) {
	t.Helper()
	now := time.Now().UTC()
	require.NoError(t, db.Create(&models.AccessGrant{
		ID:            uuid.New(),
		SchemaID:      schemaID,
		FieldName:     fieldName,
		ApplicationID: applicationID,
		ExpiresAt:     expiresAt,
		Status:        models.GrantStatusActive,
		CreatedAt:     now,
		UpdatedAt:     now,
	}).Error)
}

func TestPolicyMetadataService_CreateAccessGrants_EdgeCases(t *testing.T) {
	t.Run("CreateAccessGrants_EmptyRecords", func(t *testing.T) {
		db := setupTestDB(t)
		service := NewPolicyMetadataService(db, fieldpolicy.NewService(db))

		req := &models.AccessGrantCreateRequest{
			ApplicationID: "app-123",
			GrantDuration: models.GrantDurationTypeOneMonth,
			Records:       []models.AccessGrantCreateRequestRecord{},
		}

		resp, err := service.CreateAccessGrants(req)
		assert.NoError(t, err)
		assert.NotNil(t, resp)
		assert.Equal(t, 0, len(resp.Records))
	})

	t.Run("CreateAccessGrants_InvalidGrantDuration", func(t *testing.T) {
		db := setupTestDB(t)
		service := NewPolicyMetadataService(db, fieldpolicy.NewService(db))

		// Create policy metadata first
		createReq := &fieldpolicy.PolicyMetadataCreateRequest{
			SchemaID: "schema-123",
			Records: []fieldpolicy.PolicyMetadataCreateRequestRecord{
				{
					FieldName:         "field1",
					Source:            fieldpolicy.SourcePrimary,
					RequiresAllowlist: new(true), RequiresConsent: new(false),
				},
			},
		}
		_, err := fieldpolicy.NewService(db).CreatePolicyMetadata(createReq)
		assert.NoError(t, err)

		req := &models.AccessGrantCreateRequest{
			ApplicationID: "app-123",
			GrantDuration: "invalid-duration",
			Records: []models.AccessGrantCreateRequestRecord{
				{
					FieldName: "field1",
					SchemaID:  "schema-123",
				},
			},
		}

		_, err = service.CreateAccessGrants(req)
		assert.ErrorIs(t, err, ErrInvalidGrantRequest)
		assert.Contains(t, err.Error(), "invalid grant duration")
	})

	t.Run("CreateAccessGrants_FieldNotFound", func(t *testing.T) {
		db := setupTestDB(t)
		service := NewPolicyMetadataService(db, fieldpolicy.NewService(db))

		req := &models.AccessGrantCreateRequest{
			ApplicationID: "app-123",
			GrantDuration: models.GrantDurationTypeOneMonth,
			Records: []models.AccessGrantCreateRequestRecord{
				{
					FieldName: "nonexistent",
					SchemaID:  "schema-123",
				},
			},
		}

		_, err := service.CreateAccessGrants(req)
		assert.ErrorIs(t, err, fieldpolicy.ErrPolicyMetadataNotFound)
	})

	t.Run("CreateAccessGrants_OneYearDuration", func(t *testing.T) {
		db := setupTestDB(t)
		service := NewPolicyMetadataService(db, fieldpolicy.NewService(db))

		// Create policy metadata first
		createReq := &fieldpolicy.PolicyMetadataCreateRequest{
			SchemaID: "schema-123",
			Records: []fieldpolicy.PolicyMetadataCreateRequestRecord{
				{
					FieldName:         "field1",
					Source:            fieldpolicy.SourcePrimary,
					RequiresAllowlist: new(true), RequiresConsent: new(false),
				},
			},
		}
		_, err := fieldpolicy.NewService(db).CreatePolicyMetadata(createReq)
		assert.NoError(t, err)

		req := &models.AccessGrantCreateRequest{
			ApplicationID: "app-123",
			GrantDuration: models.GrantDurationTypeOneYear,
			Records: []models.AccessGrantCreateRequestRecord{
				{
					FieldName: "field1",
					SchemaID:  "schema-123",
				},
			},
		}

		resp, err := service.CreateAccessGrants(req)
		assert.NoError(t, err)
		assert.NotNil(t, resp)
		assert.Equal(t, 1, len(resp.Records))

		// Verify expiration is approximately 1 year from now
		expiresAt, _ := time.Parse(time.RFC3339, resp.Records[0].ExpiresAt)
		expectedExpiry := time.Now().AddDate(1, 0, 0)
		assert.WithinDuration(t, expectedExpiry, expiresAt, 5*time.Second)
	})

	t.Run("CreateAccessGrants_InvalidRequest", func(t *testing.T) {
		db := setupTestDB(t)
		service := NewPolicyMetadataService(db, fieldpolicy.NewService(db))

		_, err := service.CreateAccessGrants(&models.AccessGrantCreateRequest{
			ApplicationID: " ",
			GrantDuration: models.GrantDurationTypeOneMonth,
		})
		assert.ErrorIs(t, err, ErrInvalidGrantRequest)

		_, err = service.CreateAccessGrants(&models.AccessGrantCreateRequest{
			ApplicationID: "app-123",
			GrantDuration: models.GrantDurationTypeOneMonth,
			Records:       []models.AccessGrantCreateRequestRecord{{FieldName: "field1"}},
		})
		assert.ErrorIs(t, err, ErrInvalidGrantRequest)
	})

	t.Run("CreateAccessGrants_RenewsActiveGrant", func(t *testing.T) {
		db := setupTestDB(t)
		service := NewPolicyMetadataService(db, fieldpolicy.NewService(db))

		// Create policy metadata first
		createReq := &fieldpolicy.PolicyMetadataCreateRequest{
			SchemaID: "schema-123",
			Records: []fieldpolicy.PolicyMetadataCreateRequestRecord{
				{
					FieldName:         "field1",
					Source:            fieldpolicy.SourcePrimary,
					RequiresAllowlist: new(true), RequiresConsent: new(false),
				},
			},
		}
		_, err := fieldpolicy.NewService(db).CreatePolicyMetadata(createReq)
		assert.NoError(t, err)

		// First update
		req1 := &models.AccessGrantCreateRequest{
			ApplicationID: "app-123",
			GrantDuration: models.GrantDurationTypeOneMonth,
			Records: []models.AccessGrantCreateRequestRecord{
				{
					FieldName: "field1",
					SchemaID:  "schema-123",
				},
			},
		}
		resp1, err := service.CreateAccessGrants(req1)
		assert.NoError(t, err)
		assert.NotNil(t, resp1)

		// Re-update with different duration
		req2 := &models.AccessGrantCreateRequest{
			ApplicationID: "app-123",
			GrantDuration: models.GrantDurationTypeOneYear,
			Records: []models.AccessGrantCreateRequestRecord{
				{
					FieldName: "field1",
					SchemaID:  "schema-123",
				},
			},
		}
		resp2, err := service.CreateAccessGrants(req2)
		assert.NoError(t, err)
		assert.NotNil(t, resp2)
		assert.Equal(t, 1, len(resp2.Records))

		// The active grant is renewed, not duplicated.
		assert.Equal(t, resp1.Records[0].ID, resp2.Records[0].ID)
		assert.Equal(t, resp1.Records[0].CreatedAt, resp2.Records[0].CreatedAt)
		var count int64
		require.NoError(t, db.Model(&models.AccessGrant{}).Count(&count).Error)
		assert.Equal(t, int64(1), count)

		// Verify expiration is approximately 1 year from now
		expiresAt, _ := time.Parse(time.RFC3339, resp2.Records[0].ExpiresAt)
		expectedExpiry := time.Now().AddDate(1, 0, 0)
		assert.WithinDuration(t, expectedExpiry, expiresAt, 5*time.Second)
	})

	t.Run("CreateAccessGrants_MultipleFields", func(t *testing.T) {
		db := setupTestDB(t)
		service := NewPolicyMetadataService(db, fieldpolicy.NewService(db))

		// Create policy metadata for multiple fields
		createReq := &fieldpolicy.PolicyMetadataCreateRequest{
			SchemaID: "schema-123",
			Records: []fieldpolicy.PolicyMetadataCreateRequestRecord{
				{
					FieldName:         "field1",
					Source:            fieldpolicy.SourcePrimary,
					RequiresAllowlist: new(true), RequiresConsent: new(false),
				},
				{
					FieldName:         "field2",
					Source:            fieldpolicy.SourcePrimary,
					RequiresAllowlist: new(true), RequiresConsent: new(false),
				},
			},
		}
		_, err := fieldpolicy.NewService(db).CreatePolicyMetadata(createReq)
		assert.NoError(t, err)

		req := &models.AccessGrantCreateRequest{
			ApplicationID: "app-123",
			GrantDuration: models.GrantDurationTypeOneMonth,
			Records: []models.AccessGrantCreateRequestRecord{
				{
					FieldName: "field1",
					SchemaID:  "schema-123",
				},
				{
					FieldName: "field2",
					SchemaID:  "schema-123",
				},
			},
		}

		resp, err := service.CreateAccessGrants(req)
		assert.NoError(t, err)
		assert.NotNil(t, resp)
		assert.Equal(t, 2, len(resp.Records))
	})
}

func TestPolicyMetadataService_GetPolicyDecision_EdgeCases(t *testing.T) {
	t.Run("GetPolicyDecision_ExpiredAccess", func(t *testing.T) {
		db := setupTestDB(t)
		service := NewPolicyMetadataService(db, fieldpolicy.NewService(db))

		// Create policy metadata
		createReq := &fieldpolicy.PolicyMetadataCreateRequest{
			SchemaID: "schema-123",
			Records: []fieldpolicy.PolicyMetadataCreateRequestRecord{
				{
					FieldName:         "field1",
					Source:            fieldpolicy.SourcePrimary,
					RequiresAllowlist: new(true), RequiresConsent: new(false),
				},
			},
		}
		_, err := fieldpolicy.NewService(db).CreatePolicyMetadata(createReq)
		assert.NoError(t, err)

		// A grant that expired yesterday
		insertGrant(t, db, "schema-123", "field1", "app-123", time.Now().AddDate(0, 0, -1))

		req := &models.PolicyDecisionRequest{
			ApplicationID: "app-123",
			RequiredFields: []models.PolicyDecisionRequestRecord{
				{
					FieldName: "field1",
					SchemaID:  "schema-123",
				},
			},
		}

		resp, err := service.GetPolicyDecision(t.Context(), req)
		assert.NoError(t, err)
		assert.NotNil(t, resp)
		assert.True(t, resp.AppAccessExpired)
		assert.Equal(t, 1, len(resp.ExpiredFields))
	})

	t.Run("GetPolicyDecision_FieldNotFound", func(t *testing.T) {
		db := setupTestDB(t)
		service := NewPolicyMetadataService(db, fieldpolicy.NewService(db))

		req := &models.PolicyDecisionRequest{
			ApplicationID: "app-123",
			RequiredFields: []models.PolicyDecisionRequestRecord{
				{
					FieldName: "nonexistent",
					SchemaID:  "schema-123",
				},
			},
		}

		_, err := service.GetPolicyDecision(t.Context(), req)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "policy metadata not found")
	})

	t.Run("GetPolicyDecision_PublicFieldNoConsent", func(t *testing.T) {
		db := setupTestDB(t)
		service := NewPolicyMetadataService(db, fieldpolicy.NewService(db))

		// Create public field
		createReq := &fieldpolicy.PolicyMetadataCreateRequest{
			SchemaID: "schema-123",
			Records: []fieldpolicy.PolicyMetadataCreateRequestRecord{
				{
					FieldName:         "field1",
					Source:            fieldpolicy.SourcePrimary,
					RequiresAllowlist: new(true), RequiresConsent: new(false),
				},
			},
		}
		_, err := fieldpolicy.NewService(db).CreatePolicyMetadata(createReq)
		assert.NoError(t, err)

		// Add to allow list
		updateReq := &models.AccessGrantCreateRequest{
			ApplicationID: "app-123",
			GrantDuration: models.GrantDurationTypeOneMonth,
			Records: []models.AccessGrantCreateRequestRecord{
				{
					FieldName: "field1",
					SchemaID:  "schema-123",
				},
			},
		}
		_, err = service.CreateAccessGrants(updateReq)
		assert.NoError(t, err)

		req := &models.PolicyDecisionRequest{
			ApplicationID: "app-123",
			RequiredFields: []models.PolicyDecisionRequestRecord{
				{
					FieldName: "field1",
					SchemaID:  "schema-123",
				},
			},
		}

		resp, err := service.GetPolicyDecision(t.Context(), req)
		assert.NoError(t, err)
		assert.NotNil(t, resp)
		assert.True(t, resp.AppAuthorized)
		assert.False(t, resp.AppRequiresOwnerConsent) // Public field doesn't require consent
		assert.Equal(t, 0, len(resp.ConsentRequiredFields))
	})

	t.Run("GetPolicyDecision_MultipleSchemas", func(t *testing.T) {
		db := setupTestDB(t)
		service := NewPolicyMetadataService(db, fieldpolicy.NewService(db))

		// Create metadata for multiple schemas
		createReq1 := &fieldpolicy.PolicyMetadataCreateRequest{
			SchemaID: "schema-1",
			Records: []fieldpolicy.PolicyMetadataCreateRequestRecord{
				{
					FieldName:         "field1",
					Source:            fieldpolicy.SourcePrimary,
					RequiresAllowlist: new(true), RequiresConsent: new(false),
				},
			},
		}
		_, err := fieldpolicy.NewService(db).CreatePolicyMetadata(createReq1)
		assert.NoError(t, err)

		createReq2 := &fieldpolicy.PolicyMetadataCreateRequest{
			SchemaID: "schema-2",
			Records: []fieldpolicy.PolicyMetadataCreateRequestRecord{
				{
					FieldName:         "field2",
					Source:            fieldpolicy.SourcePrimary,
					RequiresAllowlist: new(true), RequiresConsent: new(false),
				},
			},
		}
		_, err = fieldpolicy.NewService(db).CreatePolicyMetadata(createReq2)
		assert.NoError(t, err)

		// Add both to allow list
		updateReq := &models.AccessGrantCreateRequest{
			ApplicationID: "app-123",
			GrantDuration: models.GrantDurationTypeOneMonth,
			Records: []models.AccessGrantCreateRequestRecord{
				{FieldName: "field1", SchemaID: "schema-1"},
				{FieldName: "field2", SchemaID: "schema-2"},
			},
		}
		_, err = service.CreateAccessGrants(updateReq)
		assert.NoError(t, err)

		req := &models.PolicyDecisionRequest{
			ApplicationID: "app-123",
			RequiredFields: []models.PolicyDecisionRequestRecord{
				{FieldName: "field1", SchemaID: "schema-1"},
				{FieldName: "field2", SchemaID: "schema-2"},
			},
		}

		resp, err := service.GetPolicyDecision(t.Context(), req)
		assert.NoError(t, err)
		assert.NotNil(t, resp)
		assert.True(t, resp.AppAuthorized)
		assert.Equal(t, 0, len(resp.UnauthorizedFields))
	})

	t.Run("GetPolicyDecision_ConsentFieldWithoutAllowList", func(t *testing.T) {
		db := setupTestDB(t)
		service := NewPolicyMetadataService(db, fieldpolicy.NewService(db))

		owner := fieldpolicy.OwnerCitizen
		_, err := fieldpolicy.NewService(db).CreatePolicyMetadata(&fieldpolicy.PolicyMetadataCreateRequest{
			SchemaID: "schema-123",
			Records: []fieldpolicy.PolicyMetadataCreateRequestRecord{
				{FieldName: "field1", Source: fieldpolicy.SourcePrimary, RequiresAllowlist: new(false), RequiresConsent: new(true), Owner: &owner},
			},
		})
		require.NoError(t, err)

		// The app was never added to the allow-list.
		resp, err := service.GetPolicyDecision(t.Context(), &models.PolicyDecisionRequest{
			ApplicationID:  "app-not-allowlisted",
			RequiredFields: []models.PolicyDecisionRequestRecord{{FieldName: "field1", SchemaID: "schema-123"}},
		})
		require.NoError(t, err)
		assert.True(t, resp.AppAuthorized)
		assert.True(t, resp.AppRequiresOwnerConsent)
		assert.Empty(t, resp.UnauthorizedFields)
		require.Len(t, resp.ConsentRequiredFields, 1)
		assert.Equal(t, &owner, resp.ConsentRequiredFields[0].Owner)
		assert.Equal(t, []models.PolicyDecisionFieldOutcome{
			{SchemaID: "schema-123", FieldName: "field1", Outcome: "consent_required"},
		}, resp.Fields)
	})

	t.Run("GetPolicyDecision_OpenFieldWithoutAllowList", func(t *testing.T) {
		db := setupTestDB(t)
		service := NewPolicyMetadataService(db, fieldpolicy.NewService(db))

		_, err := fieldpolicy.NewService(db).CreatePolicyMetadata(&fieldpolicy.PolicyMetadataCreateRequest{
			SchemaID: "schema-123",
			Records: []fieldpolicy.PolicyMetadataCreateRequestRecord{
				{FieldName: "field1", Source: fieldpolicy.SourcePrimary, RequiresAllowlist: new(false), RequiresConsent: new(false)},
			},
		})
		require.NoError(t, err)

		// The app was never added to the allow-list.
		resp, err := service.GetPolicyDecision(t.Context(), &models.PolicyDecisionRequest{
			ApplicationID:  "app-not-allowlisted",
			RequiredFields: []models.PolicyDecisionRequestRecord{{FieldName: "field1", SchemaID: "schema-123"}},
		})
		require.NoError(t, err)
		assert.True(t, resp.AppAuthorized)
		assert.False(t, resp.AppRequiresOwnerConsent)
		assert.False(t, resp.AppAccessExpired)
		assert.Empty(t, resp.UnauthorizedFields)
		assert.Empty(t, resp.ConsentRequiredFields)
		assert.Equal(t, []models.PolicyDecisionFieldOutcome{
			{SchemaID: "schema-123", FieldName: "field1", Outcome: "allow"},
		}, resp.Fields)
	})

	t.Run("GetPolicyDecision_AllowlistConsentFieldRequiresConsent", func(t *testing.T) {
		db := setupTestDB(t)
		service := NewPolicyMetadataService(db, fieldpolicy.NewService(db))

		createReq := &fieldpolicy.PolicyMetadataCreateRequest{
			SchemaID: "schema-123",
			Records: []fieldpolicy.PolicyMetadataCreateRequestRecord{
				{
					FieldName:         "field1",
					Source:            fieldpolicy.SourcePrimary,
					Owner:             new(fieldpolicy.OwnerCitizen),
					RequiresAllowlist: new(true), RequiresConsent: new(true),
				},
			},
		}
		_, err := fieldpolicy.NewService(db).CreatePolicyMetadata(createReq)
		assert.NoError(t, err)

		// Add to allow list
		updateReq := &models.AccessGrantCreateRequest{
			ApplicationID: "app-123",
			GrantDuration: models.GrantDurationTypeOneMonth,
			Records: []models.AccessGrantCreateRequestRecord{
				{
					FieldName: "field1",
					SchemaID:  "schema-123",
				},
			},
		}
		_, err = service.CreateAccessGrants(updateReq)
		assert.NoError(t, err)

		req := &models.PolicyDecisionRequest{
			ApplicationID: "app-123",
			RequiredFields: []models.PolicyDecisionRequestRecord{
				{
					FieldName: "field1",
					SchemaID:  "schema-123",
				},
			},
		}

		resp, err := service.GetPolicyDecision(t.Context(), req)
		assert.NoError(t, err)
		assert.NotNil(t, resp)
		assert.True(t, resp.AppAuthorized)
		assert.True(t, resp.AppRequiresOwnerConsent)
		assert.Equal(t, 1, len(resp.ConsentRequiredFields))
		assert.Equal(t, []models.PolicyDecisionFieldOutcome{
			{SchemaID: "schema-123", FieldName: "field1", Outcome: "consent_required"},
		}, resp.Fields)
	})

	t.Run("GetPolicyDecision_EmptyRequiredFields", func(t *testing.T) {
		db := setupTestDB(t)
		service := NewPolicyMetadataService(db, fieldpolicy.NewService(db))

		req := &models.PolicyDecisionRequest{
			ApplicationID:  "app-123",
			RequiredFields: []models.PolicyDecisionRequestRecord{},
		}

		resp, err := service.GetPolicyDecision(t.Context(), req)
		assert.NoError(t, err)
		assert.NotNil(t, resp)
		assert.True(t, resp.AppAuthorized) // No fields means authorized
		assert.Equal(t, 0, len(resp.UnauthorizedFields))
		assert.Equal(t, 0, len(resp.ConsentRequiredFields))
		assert.NotNil(t, resp.Fields, "fields serializes as [] rather than null")
		assert.Empty(t, resp.Fields)
	})

	t.Run("GetPolicyDecision_MixedAuthorizedUnauthorizedExpired", func(t *testing.T) {
		db := setupTestDB(t)
		service := NewPolicyMetadataService(db, fieldpolicy.NewService(db))

		// Create multiple fields with different states
		createReq := &fieldpolicy.PolicyMetadataCreateRequest{
			SchemaID: "schema-123",
			Records: []fieldpolicy.PolicyMetadataCreateRequestRecord{
				{
					FieldName:         "authorized",
					Source:            fieldpolicy.SourcePrimary,
					RequiresAllowlist: new(true), RequiresConsent: new(false),
				},
				{
					FieldName:         "expired",
					Source:            fieldpolicy.SourcePrimary,
					RequiresAllowlist: new(true), RequiresConsent: new(false),
				},
				{
					FieldName:         "unauthorized",
					Source:            fieldpolicy.SourcePrimary,
					RequiresAllowlist: new(true), RequiresConsent: new(false),
				},
			},
		}
		_, err := fieldpolicy.NewService(db).CreatePolicyMetadata(createReq)
		assert.NoError(t, err)

		// Add authorized field to allow list
		updateReq1 := &models.AccessGrantCreateRequest{
			ApplicationID: "app-123",
			GrantDuration: models.GrantDurationTypeOneMonth,
			Records: []models.AccessGrantCreateRequestRecord{
				{FieldName: "authorized", SchemaID: "schema-123"},
			},
		}
		_, err = service.CreateAccessGrants(updateReq1)
		assert.NoError(t, err)

		// A grant that expired yesterday
		insertGrant(t, db, "schema-123", "expired", "app-123", time.Now().AddDate(0, 0, -1))

		req := &models.PolicyDecisionRequest{
			ApplicationID: "app-123",
			RequiredFields: []models.PolicyDecisionRequestRecord{
				{FieldName: "authorized", SchemaID: "schema-123"},
				{FieldName: "expired", SchemaID: "schema-123"},
				{FieldName: "unauthorized", SchemaID: "schema-123"},
			},
		}

		resp, err := service.GetPolicyDecision(t.Context(), req)
		assert.NoError(t, err)
		assert.NotNil(t, resp)
		assert.False(t, resp.AppAuthorized) // Has unauthorized fields
		assert.True(t, resp.AppAccessExpired)
		assert.Equal(t, 1, len(resp.UnauthorizedFields))
		assert.Equal(t, 1, len(resp.ExpiredFields))
		assert.Equal(t, []models.PolicyDecisionFieldOutcome{
			{SchemaID: "schema-123", FieldName: "authorized", Outcome: "allow"},
			{SchemaID: "schema-123", FieldName: "expired", Outcome: "expired", Reason: "grant_expired"},
			{SchemaID: "schema-123", FieldName: "unauthorized", Outcome: "deny", Reason: "not_allowlisted"},
		}, resp.Fields)
	})
}

// Error path tests for CreatePolicyMetadata
// Note: Tests that require closing database connections to simulate errors have been removed
// as they cannot be properly mocked with SQLite. These error scenarios should be tested
// in integration tests with a real PostgreSQL database.

// Error path tests for CreateAccessGrants
// Note: Tests that require closing database connections to simulate errors have been removed
// as they cannot be properly mocked with SQLite. These error scenarios should be tested
// in integration tests with a real PostgreSQL database.

// Error path tests for GetPolicyDecision
// Note: Tests that require closing database connections to simulate errors have been removed
// as they cannot be properly mocked with SQLite. These error scenarios should be tested
// in integration tests with a real PostgreSQL database.
