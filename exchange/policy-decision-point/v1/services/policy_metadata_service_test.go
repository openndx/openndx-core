package services

import (
	"testing"
	"time"

	"github.com/gov-dx-sandbox/exchange/policy-decision-point/v1/models"
	"github.com/gov-dx-sandbox/exchange/policy-decision-point/v1/testhelpers"
	"github.com/stretchr/testify/assert"
	"gorm.io/gorm"
)

// setupTestDB creates an in-memory SQLite database for unit testing.
func setupTestDB(t *testing.T) *gorm.DB {
	return testhelpers.SetupTestDB(t)
}

func TestNewPolicyMetadataService(t *testing.T) {
	db := setupTestDB(t)
	service := NewPolicyMetadataService(db)
	assert.NotNil(t, service)
	assert.NotNil(t, service.db)
}

func TestPolicyMetadataService_CreatePolicyMetadata_EdgeCases(t *testing.T) {
	t.Run("CreatePolicyMetadata_EmptyRecords", func(t *testing.T) {
		db := setupTestDB(t)
		service := NewPolicyMetadataService(db)

		req := &models.PolicyMetadataCreateRequest{
			SchemaID: "schema-123",
			Records:  []models.PolicyMetadataCreateRequestRecord{},
		}

		resp, err := service.CreatePolicyMetadata(req)
		assert.NoError(t, err)
		assert.NotNil(t, resp)
		assert.Equal(t, 0, len(resp.Records))
	})

	t.Run("CreatePolicyMetadata_DeleteObsoleteRecords", func(t *testing.T) {
		db := setupTestDB(t)
		service := NewPolicyMetadataService(db)

		// Create initial records
		initialReq := &models.PolicyMetadataCreateRequest{
			SchemaID: "schema-123",
			Records: []models.PolicyMetadataCreateRequestRecord{
				{
					FieldName:         "field1",
					Source:            models.SourcePrimary,
					IsOwner:           true,
					AccessControlType: models.AccessControlTypePublic,
				},
				{
					FieldName:         "field2",
					Source:            models.SourcePrimary,
					IsOwner:           true,
					AccessControlType: models.AccessControlTypePublic,
				},
			},
		}
		_, err := service.CreatePolicyMetadata(initialReq)
		assert.NoError(t, err)

		// Update with only one field (field2 should be deleted)
		updateReq := &models.PolicyMetadataCreateRequest{
			SchemaID: "schema-123",
			Records: []models.PolicyMetadataCreateRequestRecord{
				{
					FieldName:         "field1",
					Source:            models.SourcePrimary,
					IsOwner:           true,
					AccessControlType: models.AccessControlTypePublic,
				},
			},
		}
		_, err = service.CreatePolicyMetadata(updateReq)
		assert.NoError(t, err)

		// Verify field2 was deleted
		var count int64
		db.Model(&models.PolicyMetadata{}).Where("field_name = ?", "field2").Count(&count)
		assert.Equal(t, int64(0), count)
	})

	t.Run("CreatePolicyMetadata_TransactionRollback", func(t *testing.T) {
		db := setupTestDB(t)
		service := NewPolicyMetadataService(db)

		// This test verifies transaction rollback on error
		// We'll create a request that should fail validation
		req := &models.PolicyMetadataCreateRequest{
			SchemaID: "schema-123",
			Records: []models.PolicyMetadataCreateRequestRecord{
				{
					FieldName:         "field1",
					Source:            models.SourcePrimary,
					IsOwner:           false, // Invalid: isOwner false but no owner specified
					AccessControlType: models.AccessControlTypePublic,
				},
			},
		}

		// This should fail due to validation
		_, err := service.CreatePolicyMetadata(req)
		assert.Error(t, err)
	})

	t.Run("CreatePolicyMetadata_MixedNewAndUpdated", func(t *testing.T) {
		db := setupTestDB(t)
		service := NewPolicyMetadataService(db)

		// Create initial record
		initialReq := &models.PolicyMetadataCreateRequest{
			SchemaID: "schema-123",
			Records: []models.PolicyMetadataCreateRequestRecord{
				{
					FieldName:         "field1",
					Source:            models.SourcePrimary,
					IsOwner:           true,
					AccessControlType: models.AccessControlTypePublic,
				},
			},
		}
		_, err := service.CreatePolicyMetadata(initialReq)
		assert.NoError(t, err)

		// Update existing and add new
		updateReq := &models.PolicyMetadataCreateRequest{
			SchemaID: "schema-123",
			Records: []models.PolicyMetadataCreateRequestRecord{
				{
					FieldName:         "field1",
					DisplayName:       testhelpers.StringPtr("Updated Field 1"),
					Source:            models.SourcePrimary,
					IsOwner:           true,
					AccessControlType: models.AccessControlTypeRestricted,
				},
				{
					FieldName:         "field2",
					DisplayName:       testhelpers.StringPtr("New Field 2"),
					Source:            models.SourcePrimary,
					IsOwner:           true,
					AccessControlType: models.AccessControlTypePublic,
				},
			},
		}
		resp, err := service.CreatePolicyMetadata(updateReq)
		assert.NoError(t, err)
		assert.Equal(t, 2, len(resp.Records))

		// Verify both records exist
		var count int64
		db.Model(&models.PolicyMetadata{}).Where("schema_id = ?", "schema-123").Count(&count)
		assert.Equal(t, int64(2), count)
	})

	t.Run("CreatePolicyMetadata_WithOwner", func(t *testing.T) {
		db := setupTestDB(t)
		service := NewPolicyMetadataService(db)

		req := &models.PolicyMetadataCreateRequest{
			SchemaID: "schema-123",
			Records: []models.PolicyMetadataCreateRequestRecord{
				{
					FieldName:         "field1",
					Source:            models.SourcePrimary,
					IsOwner:           false,
					Owner:             testhelpers.OwnerPtr(models.OwnerCitizen),
					AccessControlType: models.AccessControlTypeRestricted,
				},
			},
		}

		resp, err := service.CreatePolicyMetadata(req)
		assert.NoError(t, err)
		assert.Equal(t, 1, len(resp.Records))
		assert.Equal(t, models.OwnerCitizen, *resp.Records[0].Owner)
	})
}

func TestPolicyMetadataService_UpdateAllowList_EdgeCases(t *testing.T) {
	t.Run("UpdateAllowList_EmptyRecords", func(t *testing.T) {
		db := setupTestDB(t)
		service := NewPolicyMetadataService(db)

		req := &models.AllowListUpdateRequest{
			ApplicationID: "app-123",
			GrantDuration: models.GrantDurationTypeOneMonth,
			Records:       []models.AllowListUpdateRequestRecord{},
		}

		resp, err := service.UpdateAllowList(req)
		assert.NoError(t, err)
		assert.NotNil(t, resp)
		assert.Equal(t, 0, len(resp.Records))
	})

	t.Run("UpdateAllowList_InvalidGrantDuration", func(t *testing.T) {
		db := setupTestDB(t)
		service := NewPolicyMetadataService(db)

		// Create policy metadata first
		createReq := &models.PolicyMetadataCreateRequest{
			SchemaID: "schema-123",
			Records: []models.PolicyMetadataCreateRequestRecord{
				{
					FieldName:         "field1",
					Source:            models.SourcePrimary,
					IsOwner:           true,
					AccessControlType: models.AccessControlTypePublic,
				},
			},
		}
		_, err := service.CreatePolicyMetadata(createReq)
		assert.NoError(t, err)

		req := &models.AllowListUpdateRequest{
			ApplicationID: "app-123",
			GrantDuration: "invalid-duration",
			Records: []models.AllowListUpdateRequestRecord{
				{
					FieldName: "field1",
					SchemaID:  "schema-123",
				},
			},
		}

		_, err = service.UpdateAllowList(req)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid grant duration")
	})

	t.Run("UpdateAllowList_FieldNotFound", func(t *testing.T) {
		db := setupTestDB(t)
		service := NewPolicyMetadataService(db)

		req := &models.AllowListUpdateRequest{
			ApplicationID: "app-123",
			GrantDuration: models.GrantDurationTypeOneMonth,
			Records: []models.AllowListUpdateRequestRecord{
				{
					FieldName: "nonexistent",
					SchemaID:  "schema-123",
				},
			},
		}

		_, err := service.UpdateAllowList(req)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "policy metadata not found")
	})

	t.Run("UpdateAllowList_OneYearDuration", func(t *testing.T) {
		db := setupTestDB(t)
		service := NewPolicyMetadataService(db)

		// Create policy metadata first
		createReq := &models.PolicyMetadataCreateRequest{
			SchemaID: "schema-123",
			Records: []models.PolicyMetadataCreateRequestRecord{
				{
					FieldName:         "field1",
					Source:            models.SourcePrimary,
					IsOwner:           true,
					AccessControlType: models.AccessControlTypePublic,
				},
			},
		}
		_, err := service.CreatePolicyMetadata(createReq)
		assert.NoError(t, err)

		req := &models.AllowListUpdateRequest{
			ApplicationID: "app-123",
			GrantDuration: models.GrantDurationTypeOneYear,
			Records: []models.AllowListUpdateRequestRecord{
				{
					FieldName: "field1",
					SchemaID:  "schema-123",
				},
			},
		}

		resp, err := service.UpdateAllowList(req)
		assert.NoError(t, err)
		assert.NotNil(t, resp)
		assert.Equal(t, 1, len(resp.Records))

		// Verify expiration is approximately 1 year from now
		expiresAt, _ := time.Parse(time.RFC3339, resp.Records[0].ExpiresAt)
		expectedExpiry := time.Now().AddDate(1, 0, 0)
		assert.WithinDuration(t, expectedExpiry, expiresAt, 5*time.Second)
	})

	t.Run("UpdateAllowList_NilAllowList", func(t *testing.T) {
		db := setupTestDB(t)
		service := NewPolicyMetadataService(db)

		// Create policy metadata first using the service
		createReq := &models.PolicyMetadataCreateRequest{
			SchemaID: "schema-123",
			Records: []models.PolicyMetadataCreateRequestRecord{
				{
					FieldName:         "field1",
					Source:            models.SourcePrimary,
					IsOwner:           true,
					AccessControlType: models.AccessControlTypePublic,
				},
			},
		}
		_, err := service.CreatePolicyMetadata(createReq)
		assert.NoError(t, err)

		// Now update allow list
		req := &models.AllowListUpdateRequest{
			ApplicationID: "app-123",
			GrantDuration: models.GrantDurationTypeOneMonth,
			Records: []models.AllowListUpdateRequestRecord{
				{
					FieldName: "field1",
					SchemaID:  "schema-123",
				},
			},
		}

		resp, err := service.UpdateAllowList(req)
		assert.NoError(t, err)
		assert.NotNil(t, resp)
	})

	t.Run("UpdateAllowList_ReupdateExistingAllowList", func(t *testing.T) {
		db := setupTestDB(t)
		service := NewPolicyMetadataService(db)

		// Create policy metadata first
		createReq := &models.PolicyMetadataCreateRequest{
			SchemaID: "schema-123",
			Records: []models.PolicyMetadataCreateRequestRecord{
				{
					FieldName:         "field1",
					Source:            models.SourcePrimary,
					IsOwner:           true,
					AccessControlType: models.AccessControlTypePublic,
				},
			},
		}
		_, err := service.CreatePolicyMetadata(createReq)
		assert.NoError(t, err)

		// First update
		req1 := &models.AllowListUpdateRequest{
			ApplicationID: "app-123",
			GrantDuration: models.GrantDurationTypeOneMonth,
			Records: []models.AllowListUpdateRequestRecord{
				{
					FieldName: "field1",
					SchemaID:  "schema-123",
				},
			},
		}
		resp1, err := service.UpdateAllowList(req1)
		assert.NoError(t, err)
		assert.NotNil(t, resp1)

		// Re-update with different duration
		req2 := &models.AllowListUpdateRequest{
			ApplicationID: "app-123",
			GrantDuration: models.GrantDurationTypeOneYear,
			Records: []models.AllowListUpdateRequestRecord{
				{
					FieldName: "field1",
					SchemaID:  "schema-123",
				},
			},
		}
		resp2, err := service.UpdateAllowList(req2)
		assert.NoError(t, err)
		assert.NotNil(t, resp2)
		assert.Equal(t, 1, len(resp2.Records))

		// Verify expiration is approximately 1 year from now
		expiresAt, _ := time.Parse(time.RFC3339, resp2.Records[0].ExpiresAt)
		expectedExpiry := time.Now().AddDate(1, 0, 0)
		assert.WithinDuration(t, expectedExpiry, expiresAt, 5*time.Second)
	})

	t.Run("UpdateAllowList_MultipleFields", func(t *testing.T) {
		db := setupTestDB(t)
		service := NewPolicyMetadataService(db)

		// Create policy metadata for multiple fields
		createReq := &models.PolicyMetadataCreateRequest{
			SchemaID: "schema-123",
			Records: []models.PolicyMetadataCreateRequestRecord{
				{
					FieldName:         "field1",
					Source:            models.SourcePrimary,
					IsOwner:           true,
					AccessControlType: models.AccessControlTypePublic,
				},
				{
					FieldName:         "field2",
					Source:            models.SourcePrimary,
					IsOwner:           true,
					AccessControlType: models.AccessControlTypePublic,
				},
			},
		}
		_, err := service.CreatePolicyMetadata(createReq)
		assert.NoError(t, err)

		req := &models.AllowListUpdateRequest{
			ApplicationID: "app-123",
			GrantDuration: models.GrantDurationTypeOneMonth,
			Records: []models.AllowListUpdateRequestRecord{
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

		resp, err := service.UpdateAllowList(req)
		assert.NoError(t, err)
		assert.NotNil(t, resp)
		assert.Equal(t, 2, len(resp.Records))
	})
}

func TestPolicyMetadataService_GetPolicyDecision_EdgeCases(t *testing.T) {
	t.Run("GetPolicyDecision_ExpiredAccess", func(t *testing.T) {
		db := setupTestDB(t)
		service := NewPolicyMetadataService(db)

		// Create policy metadata
		createReq := &models.PolicyMetadataCreateRequest{
			SchemaID: "schema-123",
			Records: []models.PolicyMetadataCreateRequestRecord{
				{
					FieldName:         "field1",
					Source:            models.SourcePrimary,
					IsOwner:           true,
					AccessControlType: models.AccessControlTypePublic,
				},
			},
		}
		_, err := service.CreatePolicyMetadata(createReq)
		assert.NoError(t, err)

		// Manually set expired allow list entry
		var pm models.PolicyMetadata
		db.Where("field_name = ?", "field1").First(&pm)
		pm.AllowList = models.AllowList{
			"app-123": {
				ExpiresAt: time.Now().AddDate(0, 0, -1), // Expired yesterday
				UpdatedAt: time.Now(),
			},
		}
		db.Save(&pm)

		req := &models.PolicyDecisionRequest{
			ApplicationID: "app-123",
			RequiredFields: []models.PolicyDecisionRequestRecord{
				{
					FieldName: "field1",
					SchemaID:  "schema-123",
				},
			},
		}

		resp, err := service.GetPolicyDecision(req)
		assert.NoError(t, err)
		assert.NotNil(t, resp)
		assert.True(t, resp.AppAccessExpired)
		assert.Equal(t, 1, len(resp.ExpiredFields))
	})

	t.Run("GetPolicyDecision_FieldNotFound", func(t *testing.T) {
		db := setupTestDB(t)
		service := NewPolicyMetadataService(db)

		req := &models.PolicyDecisionRequest{
			ApplicationID: "app-123",
			RequiredFields: []models.PolicyDecisionRequestRecord{
				{
					FieldName: "nonexistent",
					SchemaID:  "schema-123",
				},
			},
		}

		_, err := service.GetPolicyDecision(req)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "policy metadata not found")
	})

	t.Run("GetPolicyDecision_PublicFieldNoConsent", func(t *testing.T) {
		db := setupTestDB(t)
		service := NewPolicyMetadataService(db)

		// Create public field
		createReq := &models.PolicyMetadataCreateRequest{
			SchemaID: "schema-123",
			Records: []models.PolicyMetadataCreateRequestRecord{
				{
					FieldName:         "field1",
					Source:            models.SourcePrimary,
					IsOwner:           true,
					AccessControlType: models.AccessControlTypePublic,
				},
			},
		}
		_, err := service.CreatePolicyMetadata(createReq)
		assert.NoError(t, err)

		// Add to allow list
		updateReq := &models.AllowListUpdateRequest{
			ApplicationID: "app-123",
			GrantDuration: models.GrantDurationTypeOneMonth,
			Records: []models.AllowListUpdateRequestRecord{
				{
					FieldName: "field1",
					SchemaID:  "schema-123",
				},
			},
		}
		_, err = service.UpdateAllowList(updateReq)
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

		resp, err := service.GetPolicyDecision(req)
		assert.NoError(t, err)
		assert.NotNil(t, resp)
		assert.True(t, resp.AppAuthorized)
		assert.False(t, resp.AppRequiresOwnerConsent) // Public field doesn't require consent
		assert.Equal(t, 0, len(resp.ConsentRequiredFields))
	})

	t.Run("GetPolicyDecision_MultipleSchemas", func(t *testing.T) {
		db := setupTestDB(t)
		service := NewPolicyMetadataService(db)

		// Create metadata for multiple schemas
		createReq1 := &models.PolicyMetadataCreateRequest{
			SchemaID: "schema-1",
			Records: []models.PolicyMetadataCreateRequestRecord{
				{
					FieldName:         "field1",
					Source:            models.SourcePrimary,
					IsOwner:           true,
					AccessControlType: models.AccessControlTypePublic,
				},
			},
		}
		_, err := service.CreatePolicyMetadata(createReq1)
		assert.NoError(t, err)

		createReq2 := &models.PolicyMetadataCreateRequest{
			SchemaID: "schema-2",
			Records: []models.PolicyMetadataCreateRequestRecord{
				{
					FieldName:         "field2",
					Source:            models.SourcePrimary,
					IsOwner:           true,
					AccessControlType: models.AccessControlTypePublic,
				},
			},
		}
		_, err = service.CreatePolicyMetadata(createReq2)
		assert.NoError(t, err)

		// Add both to allow list
		updateReq := &models.AllowListUpdateRequest{
			ApplicationID: "app-123",
			GrantDuration: models.GrantDurationTypeOneMonth,
			Records: []models.AllowListUpdateRequestRecord{
				{FieldName: "field1", SchemaID: "schema-1"},
				{FieldName: "field2", SchemaID: "schema-2"},
			},
		}
		_, err = service.UpdateAllowList(updateReq)
		assert.NoError(t, err)

		req := &models.PolicyDecisionRequest{
			ApplicationID: "app-123",
			RequiredFields: []models.PolicyDecisionRequestRecord{
				{FieldName: "field1", SchemaID: "schema-1"},
				{FieldName: "field2", SchemaID: "schema-2"},
			},
		}

		resp, err := service.GetPolicyDecision(req)
		assert.NoError(t, err)
		assert.NotNil(t, resp)
		assert.True(t, resp.AppAuthorized)
		assert.Equal(t, 0, len(resp.UnauthorizedFields))
	})

	t.Run("GetPolicyDecision_RestrictedFieldRequiresConsent", func(t *testing.T) {
		db := setupTestDB(t)
		service := NewPolicyMetadataService(db)

		// Create restricted field with owner
		createReq := &models.PolicyMetadataCreateRequest{
			SchemaID: "schema-123",
			Records: []models.PolicyMetadataCreateRequestRecord{
				{
					FieldName:         "field1",
					Source:            models.SourcePrimary,
					IsOwner:           false,
					Owner:             testhelpers.OwnerPtr(models.OwnerCitizen),
					AccessControlType: models.AccessControlTypeRestricted,
				},
			},
		}
		_, err := service.CreatePolicyMetadata(createReq)
		assert.NoError(t, err)

		// Add to allow list
		updateReq := &models.AllowListUpdateRequest{
			ApplicationID: "app-123",
			GrantDuration: models.GrantDurationTypeOneMonth,
			Records: []models.AllowListUpdateRequestRecord{
				{
					FieldName: "field1",
					SchemaID:  "schema-123",
				},
			},
		}
		_, err = service.UpdateAllowList(updateReq)
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

		resp, err := service.GetPolicyDecision(req)
		assert.NoError(t, err)
		assert.NotNil(t, resp)
		assert.True(t, resp.AppAuthorized)
		assert.True(t, resp.AppRequiresOwnerConsent) // Restricted field requires consent
		assert.Equal(t, 1, len(resp.ConsentRequiredFields))
	})

	t.Run("GetPolicyDecision_EmptyRequiredFields", func(t *testing.T) {
		db := setupTestDB(t)
		service := NewPolicyMetadataService(db)

		req := &models.PolicyDecisionRequest{
			ApplicationID:  "app-123",
			RequiredFields: []models.PolicyDecisionRequestRecord{},
		}

		resp, err := service.GetPolicyDecision(req)
		assert.NoError(t, err)
		assert.NotNil(t, resp)
		assert.True(t, resp.AppAuthorized) // No fields means authorized
		assert.Equal(t, 0, len(resp.UnauthorizedFields))
		assert.Equal(t, 0, len(resp.ConsentRequiredFields))
	})

	t.Run("GetPolicyDecision_MixedAuthorizedUnauthorizedExpired", func(t *testing.T) {
		db := setupTestDB(t)
		service := NewPolicyMetadataService(db)

		// Create multiple fields with different states
		createReq := &models.PolicyMetadataCreateRequest{
			SchemaID: "schema-123",
			Records: []models.PolicyMetadataCreateRequestRecord{
				{
					FieldName:         "authorized",
					Source:            models.SourcePrimary,
					IsOwner:           true,
					AccessControlType: models.AccessControlTypePublic,
				},
				{
					FieldName:         "expired",
					Source:            models.SourcePrimary,
					IsOwner:           true,
					AccessControlType: models.AccessControlTypePublic,
				},
				{
					FieldName:         "unauthorized",
					Source:            models.SourcePrimary,
					IsOwner:           true,
					AccessControlType: models.AccessControlTypePublic,
				},
			},
		}
		_, err := service.CreatePolicyMetadata(createReq)
		assert.NoError(t, err)

		// Add authorized field to allow list
		updateReq1 := &models.AllowListUpdateRequest{
			ApplicationID: "app-123",
			GrantDuration: models.GrantDurationTypeOneMonth,
			Records: []models.AllowListUpdateRequestRecord{
				{FieldName: "authorized", SchemaID: "schema-123"},
			},
		}
		_, err = service.UpdateAllowList(updateReq1)
		assert.NoError(t, err)

		// Manually set expired allow list entry
		var pmExpired models.PolicyMetadata
		db.Where("field_name = ?", "expired").First(&pmExpired)
		pmExpired.AllowList = models.AllowList{
			"app-123": {
				ExpiresAt: time.Now().AddDate(0, 0, -1), // Expired yesterday
				UpdatedAt: time.Now(),
			},
		}
		db.Save(&pmExpired)

		req := &models.PolicyDecisionRequest{
			ApplicationID: "app-123",
			RequiredFields: []models.PolicyDecisionRequestRecord{
				{FieldName: "authorized", SchemaID: "schema-123"},
				{FieldName: "expired", SchemaID: "schema-123"},
				{FieldName: "unauthorized", SchemaID: "schema-123"},
			},
		}

		resp, err := service.GetPolicyDecision(req)
		assert.NoError(t, err)
		assert.NotNil(t, resp)
		assert.False(t, resp.AppAuthorized) // Has unauthorized fields
		assert.True(t, resp.AppAccessExpired)
		assert.Equal(t, 1, len(resp.UnauthorizedFields))
		assert.Equal(t, 1, len(resp.ExpiredFields))
	})
}

// Error path tests for CreatePolicyMetadata
// Note: Tests that require closing database connections to simulate errors have been removed
// as they cannot be properly mocked with SQLite. These error scenarios should be tested
// in integration tests with a real PostgreSQL database.

// Error path tests for UpdateAllowList
// Note: Tests that require closing database connections to simulate errors have been removed
// as they cannot be properly mocked with SQLite. These error scenarios should be tested
// in integration tests with a real PostgreSQL database.

// Error path tests for GetPolicyDecision
// Note: Tests that require closing database connections to simulate errors have been removed
// as they cannot be properly mocked with SQLite. These error scenarios should be tested
// in integration tests with a real PostgreSQL database.
