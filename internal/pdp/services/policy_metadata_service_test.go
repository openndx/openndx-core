package services

import (
	"testing"
	"time"

	"github.com/openndx/openndx-core/internal/pdp/engine"
	"github.com/openndx/openndx-core/internal/pdp/models"
	"github.com/openndx/openndx-core/internal/pdp/testhelpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// setupTestDB creates an in-memory SQLite database for unit testing.
func setupTestDB(t *testing.T) *gorm.DB {
	return testhelpers.SetupTestDB(t)
}

func TestNewPolicyMetadataService(t *testing.T) {
	db := setupTestDB(t)
	service := NewPolicyMetadataService(db, testhelpers.Engine(t))
	assert.NotNil(t, service)
	assert.NotNil(t, service.db)
}

func TestPolicyMetadataService_ListPolicyMetadata(t *testing.T) {
	db := setupTestDB(t)
	service := NewPolicyMetadataService(db, testhelpers.Engine(t))

	_, err := service.CreatePolicyMetadata(&models.PolicyMetadataCreateRequest{
		SchemaID: "schema-123",
		Records: []models.PolicyMetadataCreateRequestRecord{
			{
				FieldName:   "person.name",
				Source:      models.SourcePrimary,
				PolicyClass: engine.ClassAllowlist,
			},
			{
				FieldName:   "person.email",
				Source:      models.SourcePrimary,
				PolicyClass: engine.ClassAllowlist,
			},
		},
	})
	require.NoError(t, err)

	_, err = service.CreatePolicyMetadata(&models.PolicyMetadataCreateRequest{
		SchemaID: "other-schema",
		Records: []models.PolicyMetadataCreateRequestRecord{
			{
				FieldName:   "other.field",
				Source:      models.SourcePrimary,
				PolicyClass: engine.ClassAllowlist,
			},
		},
	})
	require.NoError(t, err)

	resp, err := service.ListPolicyMetadata("schema-123")
	require.NoError(t, err)
	require.Len(t, resp.Records, 2)
	assert.Equal(t, "person.email", resp.Records[0].FieldName)
	assert.Equal(t, "person.name", resp.Records[1].FieldName)
	assert.Equal(t, "schema-123", resp.Records[0].SchemaID)
	assert.Equal(t, "schema-123", resp.Records[1].SchemaID)

	allResp, err := service.ListPolicyMetadata("")
	require.NoError(t, err)
	require.Len(t, allResp.Records, 3)
	assert.Equal(t, "other-schema", allResp.Records[0].SchemaID)
	assert.Equal(t, "schema-123", allResp.Records[1].SchemaID)
	assert.Equal(t, "schema-123", allResp.Records[2].SchemaID)

	emptyResp, err := service.ListPolicyMetadata("missing-schema")
	require.NoError(t, err)
	assert.Empty(t, emptyResp.Records)
}

func TestPolicyMetadataService_PatchPolicyMetadata(t *testing.T) {
	db := setupTestDB(t)
	service := NewPolicyMetadataService(db, testhelpers.Engine(t))

	createResp, err := service.CreatePolicyMetadata(&models.PolicyMetadataCreateRequest{
		SchemaID: "schema-123",
		Records: []models.PolicyMetadataCreateRequestRecord{
			{
				FieldName:   "person.name",
				DisplayName: testhelpers.StringPtr("Name"),
				Source:      models.SourcePrimary,
				PolicyClass: engine.ClassAllowlist,
			},
			{
				FieldName:   "person.email",
				DisplayName: testhelpers.StringPtr("Email"),
				Description: testhelpers.StringPtr("Old description"),
				Source:      models.SourcePrimary,
				PolicyClass: engine.ClassAllowlist,
			},
		},
	})
	require.NoError(t, err)
	require.Len(t, createResp.Records, 2)

	_, err = service.UpdateAllowList(&models.AllowListUpdateRequest{
		ApplicationID: "app-123",
		GrantDuration: models.GrantDurationTypeOneMonth,
		Records: []models.AllowListUpdateRequestRecord{
			{FieldName: "person.email", SchemaID: "schema-123"},
		},
	})
	require.NoError(t, err)

	id := createResp.Records[1].ID

	consentClass := engine.ClassAllowlistConsent
	params := models.PolicyParams{"purpose": "tax"}
	resp, err := service.PatchPolicyMetadata(id, &models.PolicyMetadataPatchRequest{
		DisplayName: models.OptionalPatchField[string]{
			Set:   true,
			Value: testhelpers.StringPtr("Primary Email"),
		},
		Description: models.OptionalPatchField[string]{
			Set:   true,
			Value: nil,
		},
		PolicyClass: models.OptionalPatchField[string]{
			Set:   true,
			Value: &consentClass,
		},
		PolicyParams: models.OptionalPatchField[models.PolicyParams]{
			Set:   true,
			Value: &params,
		},
	})
	require.NoError(t, err)
	require.NotNil(t, resp.DisplayName)
	assert.Equal(t, "Primary Email", *resp.DisplayName)
	assert.Nil(t, resp.Description)
	assert.Equal(t, engine.ClassAllowlistConsent, resp.PolicyClass)
	assert.Equal(t, params, resp.PolicyParams)
	assert.Contains(t, resp.AllowList, "app-123")

	var count int64
	require.NoError(t, db.Model(&models.PolicyMetadata{}).
		Where("schema_id = ?", "schema-123").
		Count(&count).Error)
	assert.Equal(t, int64(2), count)

	var nameRecord models.PolicyMetadata
	require.NoError(t, db.Where(
		"schema_id = ? AND field_name = ?",
		"schema-123",
		"person.name",
	).First(&nameRecord).Error)
	require.NotNil(t, nameRecord.DisplayName)
	assert.Equal(t, "Name", *nameRecord.DisplayName)

	// Reject any SQL statement that updates policy_class. A display-only
	// patch must still succeed because it should update only the supplied field.
	require.NoError(t, db.Exec(`
		CREATE TRIGGER reject_policy_class_update
		BEFORE UPDATE OF policy_class ON policy_metadata
		BEGIN
			SELECT RAISE(FAIL, 'policy_class was unexpectedly updated');
		END;
	`).Error)

	displayOnlyResp, err := service.PatchPolicyMetadata(id, &models.PolicyMetadataPatchRequest{
		DisplayName: models.OptionalPatchField[string]{
			Set:   true,
			Value: testhelpers.StringPtr("Contact Email"),
		},
	})
	require.NoError(t, err)
	require.NotNil(t, displayOnlyResp.DisplayName)
	assert.Equal(t, "Contact Email", *displayOnlyResp.DisplayName)
	assert.Equal(t, engine.ClassAllowlistConsent, displayOnlyResp.PolicyClass)
	assert.Equal(t, params, displayOnlyResp.PolicyParams)

	_, err = service.PatchPolicyMetadata("not-a-uuid", &models.PolicyMetadataPatchRequest{})
	assert.ErrorIs(t, err, ErrPolicyMetadataNotFound)
}

func TestPolicyMetadataService_PatchPolicyMetadata_PolicyClass(t *testing.T) {
	db := setupTestDB(t)
	service := NewPolicyMetadataService(db, testhelpers.Engine(t))

	createResp, err := service.CreatePolicyMetadata(&models.PolicyMetadataCreateRequest{
		SchemaID: "schema-123",
		Records: []models.PolicyMetadataCreateRequestRecord{
			{
				FieldName:    "person.email",
				Source:       models.SourcePrimary,
				PolicyClass:  engine.ClassAllowlist,
				PolicyParams: models.PolicyParams{"purpose": "tax"},
			},
		},
	})
	require.NoError(t, err)
	id := createResp.Records[0].ID

	unknown := "no_such_class"
	_, err = service.PatchPolicyMetadata(id, &models.PolicyMetadataPatchRequest{
		PolicyClass: models.OptionalPatchField[string]{Set: true, Value: &unknown},
	})
	assert.ErrorIs(t, err, ErrUnknownPolicyClass)

	_, err = service.PatchPolicyMetadata(id, &models.PolicyMetadataPatchRequest{
		PolicyClass: models.OptionalPatchField[string]{Set: true},
	})
	assert.ErrorIs(t, err, ErrUnknownPolicyClass)

	// null clears the parameters
	resp, err := service.PatchPolicyMetadata(id, &models.PolicyMetadataPatchRequest{
		PolicyParams: models.OptionalPatchField[models.PolicyParams]{Set: true},
	})
	require.NoError(t, err)
	assert.Equal(t, engine.ClassAllowlist, resp.PolicyClass)
	assert.Equal(t, models.PolicyParams{}, resp.PolicyParams)
}

func TestPolicyMetadataService_DeletePolicyMetadata(t *testing.T) {
	db := setupTestDB(t)
	service := NewPolicyMetadataService(db, testhelpers.Engine(t))

	createResp, err := service.CreatePolicyMetadata(&models.PolicyMetadataCreateRequest{
		SchemaID: "schema-123",
		Records: []models.PolicyMetadataCreateRequestRecord{
			{
				FieldName:   "person.name",
				Source:      models.SourcePrimary,
				PolicyClass: engine.ClassAllowlist,
			},
			{
				FieldName:   "person.email",
				Source:      models.SourcePrimary,
				PolicyClass: engine.ClassAllowlist,
			},
		},
	})
	require.NoError(t, err)
	require.Len(t, createResp.Records, 2)

	id := createResp.Records[1].ID
	require.NoError(t, service.DeletePolicyMetadata(id))

	var records []models.PolicyMetadata
	require.NoError(t, db.Where("schema_id = ?", "schema-123").Find(&records).Error)
	require.Len(t, records, 1)
	assert.Equal(t, "person.name", records[0].FieldName)

	err = service.DeletePolicyMetadata(id)
	assert.ErrorIs(t, err, ErrPolicyMetadataNotFound)
}

func TestPolicyMetadataService_RevokeAllowListEntry(t *testing.T) {
	db := setupTestDB(t)
	service := NewPolicyMetadataService(db, testhelpers.Engine(t))

	createResp, err := service.CreatePolicyMetadata(&models.PolicyMetadataCreateRequest{
		SchemaID: "schema-123",
		Records: []models.PolicyMetadataCreateRequestRecord{
			{
				FieldName:   "person.email",
				Source:      models.SourcePrimary,
				PolicyClass: engine.ClassAllowlist,
			},
		},
	})
	require.NoError(t, err)
	require.Len(t, createResp.Records, 1)

	for _, applicationID := range []string{"app-123", "app-456"} {
		_, err = service.UpdateAllowList(&models.AllowListUpdateRequest{
			ApplicationID: applicationID,
			GrantDuration: models.GrantDurationTypeOneMonth,
			Records: []models.AllowListUpdateRequestRecord{
				{FieldName: "person.email", SchemaID: "schema-123"},
			},
		})
		require.NoError(t, err)
	}

	id := createResp.Records[0].ID
	require.NoError(t, service.RevokeAllowListEntry(id, "app-123"))

	var policyMetadata models.PolicyMetadata
	require.NoError(t, db.First(&policyMetadata, "id = ?", id).Error)
	assert.NotContains(t, policyMetadata.AllowList, "app-123")
	assert.Contains(t, policyMetadata.AllowList, "app-456")

	err = service.RevokeAllowListEntry(id, "app-123")
	assert.ErrorIs(t, err, ErrAllowListEntryNotFound)

	err = service.RevokeAllowListEntry("not-a-uuid", "app-456")
	assert.ErrorIs(t, err, ErrPolicyMetadataNotFound)
}

// TestCreatePolicyMetadata_DoesNotUpdateAllowList verifies that schema
// synchronization cannot restore a stale allow-list entry.
func TestCreatePolicyMetadata_DoesNotUpdateAllowList(t *testing.T) {
	db := setupTestDB(t)
	service := NewPolicyMetadataService(db, testhelpers.Engine(t))

	_, err := service.CreatePolicyMetadata(&models.PolicyMetadataCreateRequest{
		SchemaID: "schema-123",
		Records: []models.PolicyMetadataCreateRequestRecord{
			{
				FieldName:   "person.email",
				DisplayName: testhelpers.StringPtr("Email"),
				Source:      models.SourcePrimary,
				PolicyClass: engine.ClassAllowlist,
			},
		},
	})
	require.NoError(t, err)

	_, err = service.UpdateAllowList(&models.AllowListUpdateRequest{
		ApplicationID: "app-A",
		GrantDuration: models.GrantDurationTypeOneMonth,
		Records: []models.AllowListUpdateRequestRecord{
			{FieldName: "person.email", SchemaID: "schema-123"},
		},
	})
	require.NoError(t, err)

	// Simulate a concurrent revoke after schema synchronization has read the
	// stale row but immediately before it writes its metadata update.
	require.NoError(t, db.Exec(`
		CREATE TRIGGER revoke_before_schema_sync_update
		BEFORE UPDATE OF display_name ON policy_metadata
		BEGIN
			UPDATE policy_metadata
			SET allow_list = '{}'
			WHERE id = OLD.id;
		END;
	`).Error)

	_, err = service.CreatePolicyMetadata(&models.PolicyMetadataCreateRequest{
		SchemaID: "schema-123",
		Records: []models.PolicyMetadataCreateRequestRecord{
			{
				FieldName:   "person.email",
				DisplayName: testhelpers.StringPtr("Primary Email"),
				Source:      models.SourcePrimary,
				PolicyClass: engine.ClassAllowlist,
			},
		},
	})
	require.NoError(t, err)

	var policyMetadata models.PolicyMetadata
	require.NoError(t, db.Where(
		"schema_id = ? AND field_name = ?",
		"schema-123",
		"person.email",
	).First(&policyMetadata).Error)
	require.NotNil(t, policyMetadata.DisplayName)
	assert.Equal(t, "Primary Email", *policyMetadata.DisplayName)
	assert.NotContains(t, policyMetadata.AllowList, "app-A")
	assert.Empty(t, policyMetadata.AllowList)
}

func TestPolicyMetadataService_CreatePolicyMetadata_EdgeCases(t *testing.T) {
	t.Run("CreatePolicyMetadata_EmptyRecords", func(t *testing.T) {
		db := setupTestDB(t)
		service := NewPolicyMetadataService(db, testhelpers.Engine(t))

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
		service := NewPolicyMetadataService(db, testhelpers.Engine(t))

		// Create initial records
		initialReq := &models.PolicyMetadataCreateRequest{
			SchemaID: "schema-123",
			Records: []models.PolicyMetadataCreateRequestRecord{
				{
					FieldName:   "field1",
					Source:      models.SourcePrimary,
					PolicyClass: engine.ClassAllowlist,
				},
				{
					FieldName:   "field2",
					Source:      models.SourcePrimary,
					PolicyClass: engine.ClassAllowlist,
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
					FieldName:   "field1",
					Source:      models.SourcePrimary,
					PolicyClass: engine.ClassAllowlist,
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
		service := NewPolicyMetadataService(db, testhelpers.Engine(t))

		// One unknown class rejects the whole request before anything is written
		req := &models.PolicyMetadataCreateRequest{
			SchemaID: "schema-123",
			Records: []models.PolicyMetadataCreateRequestRecord{
				{
					FieldName:   "field1",
					Source:      models.SourcePrimary,
					PolicyClass: engine.ClassAllowlist,
				},
				{
					FieldName:   "field2",
					Source:      models.SourcePrimary,
					PolicyClass: "no_such_class",
				},
			},
		}

		_, err := service.CreatePolicyMetadata(req)
		assert.ErrorIs(t, err, ErrUnknownPolicyClass)
		assert.ErrorContains(t, err, "field2")

		var count int64
		db.Model(&models.PolicyMetadata{}).Count(&count)
		assert.Equal(t, int64(0), count)
	})

	t.Run("CreatePolicyMetadata_StoresPolicyParams", func(t *testing.T) {
		db := setupTestDB(t)
		service := NewPolicyMetadataService(db, testhelpers.Engine(t))

		resp, err := service.CreatePolicyMetadata(&models.PolicyMetadataCreateRequest{
			SchemaID: "schema-123",
			Records: []models.PolicyMetadataCreateRequestRecord{
				{
					FieldName:    "field1",
					Source:       models.SourcePrimary,
					PolicyClass:  engine.ClassAllowlistConsent,
					PolicyParams: models.PolicyParams{"purpose": "tax"},
				},
				{
					FieldName:   "field2",
					Source:      models.SourcePrimary,
					PolicyClass: engine.ClassAllowlist,
				},
			},
		})
		require.NoError(t, err)
		require.Len(t, resp.Records, 2)

		var records []models.PolicyMetadata
		require.NoError(t, db.Order("field_name").Find(&records).Error)
		require.Len(t, records, 2)
		assert.Equal(t, engine.ClassAllowlistConsent, records[0].PolicyClass)
		assert.Equal(t, models.PolicyParams{"purpose": "tax"}, records[0].PolicyParams)
		assert.Equal(t, engine.ClassAllowlist, records[1].PolicyClass)
		assert.Equal(t, models.PolicyParams{}, records[1].PolicyParams)
	})

	t.Run("CreatePolicyMetadata_MixedNewAndUpdated", func(t *testing.T) {
		db := setupTestDB(t)
		service := NewPolicyMetadataService(db, testhelpers.Engine(t))

		// Create initial record
		initialReq := &models.PolicyMetadataCreateRequest{
			SchemaID: "schema-123",
			Records: []models.PolicyMetadataCreateRequestRecord{
				{
					FieldName:   "field1",
					Source:      models.SourcePrimary,
					PolicyClass: engine.ClassAllowlist,
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
					FieldName:   "field1",
					DisplayName: testhelpers.StringPtr("Updated Field 1"),
					Source:      models.SourcePrimary,
					PolicyClass: engine.ClassAllowlist,
				},
				{
					FieldName:   "field2",
					DisplayName: testhelpers.StringPtr("New Field 2"),
					Source:      models.SourcePrimary,
					PolicyClass: engine.ClassAllowlist,
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
		service := NewPolicyMetadataService(db, testhelpers.Engine(t))

		req := &models.PolicyMetadataCreateRequest{
			SchemaID: "schema-123",
			Records: []models.PolicyMetadataCreateRequestRecord{
				{
					FieldName:   "field1",
					Source:      models.SourcePrimary,
					Owner:       testhelpers.OwnerPtr(models.OwnerCitizen),
					PolicyClass: engine.ClassAllowlistConsent,
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
		service := NewPolicyMetadataService(db, testhelpers.Engine(t))

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
		service := NewPolicyMetadataService(db, testhelpers.Engine(t))

		// Create policy metadata first
		createReq := &models.PolicyMetadataCreateRequest{
			SchemaID: "schema-123",
			Records: []models.PolicyMetadataCreateRequestRecord{
				{
					FieldName:   "field1",
					Source:      models.SourcePrimary,
					PolicyClass: engine.ClassAllowlist,
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
		service := NewPolicyMetadataService(db, testhelpers.Engine(t))

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
		service := NewPolicyMetadataService(db, testhelpers.Engine(t))

		// Create policy metadata first
		createReq := &models.PolicyMetadataCreateRequest{
			SchemaID: "schema-123",
			Records: []models.PolicyMetadataCreateRequestRecord{
				{
					FieldName:   "field1",
					Source:      models.SourcePrimary,
					PolicyClass: engine.ClassAllowlist,
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
		service := NewPolicyMetadataService(db, testhelpers.Engine(t))

		// Create policy metadata first using the service
		createReq := &models.PolicyMetadataCreateRequest{
			SchemaID: "schema-123",
			Records: []models.PolicyMetadataCreateRequestRecord{
				{
					FieldName:   "field1",
					Source:      models.SourcePrimary,
					PolicyClass: engine.ClassAllowlist,
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
		service := NewPolicyMetadataService(db, testhelpers.Engine(t))

		// Create policy metadata first
		createReq := &models.PolicyMetadataCreateRequest{
			SchemaID: "schema-123",
			Records: []models.PolicyMetadataCreateRequestRecord{
				{
					FieldName:   "field1",
					Source:      models.SourcePrimary,
					PolicyClass: engine.ClassAllowlist,
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
		service := NewPolicyMetadataService(db, testhelpers.Engine(t))

		// Create policy metadata for multiple fields
		createReq := &models.PolicyMetadataCreateRequest{
			SchemaID: "schema-123",
			Records: []models.PolicyMetadataCreateRequestRecord{
				{
					FieldName:   "field1",
					Source:      models.SourcePrimary,
					PolicyClass: engine.ClassAllowlist,
				},
				{
					FieldName:   "field2",
					Source:      models.SourcePrimary,
					PolicyClass: engine.ClassAllowlist,
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
		service := NewPolicyMetadataService(db, testhelpers.Engine(t))

		// Create policy metadata
		createReq := &models.PolicyMetadataCreateRequest{
			SchemaID: "schema-123",
			Records: []models.PolicyMetadataCreateRequestRecord{
				{
					FieldName:   "field1",
					Source:      models.SourcePrimary,
					PolicyClass: engine.ClassAllowlist,
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

		resp, err := service.GetPolicyDecision(t.Context(), req)
		assert.NoError(t, err)
		assert.NotNil(t, resp)
		assert.True(t, resp.AppAccessExpired)
		assert.Equal(t, 1, len(resp.ExpiredFields))
	})

	t.Run("GetPolicyDecision_FieldNotFound", func(t *testing.T) {
		db := setupTestDB(t)
		service := NewPolicyMetadataService(db, testhelpers.Engine(t))

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
		service := NewPolicyMetadataService(db, testhelpers.Engine(t))

		// Create public field
		createReq := &models.PolicyMetadataCreateRequest{
			SchemaID: "schema-123",
			Records: []models.PolicyMetadataCreateRequestRecord{
				{
					FieldName:   "field1",
					Source:      models.SourcePrimary,
					PolicyClass: engine.ClassAllowlist,
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

		resp, err := service.GetPolicyDecision(t.Context(), req)
		assert.NoError(t, err)
		assert.NotNil(t, resp)
		assert.True(t, resp.AppAuthorized)
		assert.False(t, resp.AppRequiresOwnerConsent) // Public field doesn't require consent
		assert.Equal(t, 0, len(resp.ConsentRequiredFields))
	})

	t.Run("GetPolicyDecision_MultipleSchemas", func(t *testing.T) {
		db := setupTestDB(t)
		service := NewPolicyMetadataService(db, testhelpers.Engine(t))

		// Create metadata for multiple schemas
		createReq1 := &models.PolicyMetadataCreateRequest{
			SchemaID: "schema-1",
			Records: []models.PolicyMetadataCreateRequestRecord{
				{
					FieldName:   "field1",
					Source:      models.SourcePrimary,
					PolicyClass: engine.ClassAllowlist,
				},
			},
		}
		_, err := service.CreatePolicyMetadata(createReq1)
		assert.NoError(t, err)

		createReq2 := &models.PolicyMetadataCreateRequest{
			SchemaID: "schema-2",
			Records: []models.PolicyMetadataCreateRequestRecord{
				{
					FieldName:   "field2",
					Source:      models.SourcePrimary,
					PolicyClass: engine.ClassAllowlist,
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

		resp, err := service.GetPolicyDecision(t.Context(), req)
		assert.NoError(t, err)
		assert.NotNil(t, resp)
		assert.True(t, resp.AppAuthorized)
		assert.Equal(t, 0, len(resp.UnauthorizedFields))
	})

	t.Run("GetPolicyDecision_AllowlistConsentFieldRequiresConsent", func(t *testing.T) {
		db := setupTestDB(t)
		service := NewPolicyMetadataService(db, testhelpers.Engine(t))

		createReq := &models.PolicyMetadataCreateRequest{
			SchemaID: "schema-123",
			Records: []models.PolicyMetadataCreateRequestRecord{
				{
					FieldName:   "field1",
					Source:      models.SourcePrimary,
					Owner:       testhelpers.OwnerPtr(models.OwnerCitizen),
					PolicyClass: engine.ClassAllowlistConsent,
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
		service := NewPolicyMetadataService(db, testhelpers.Engine(t))

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
		service := NewPolicyMetadataService(db, testhelpers.Engine(t))

		// Create multiple fields with different states
		createReq := &models.PolicyMetadataCreateRequest{
			SchemaID: "schema-123",
			Records: []models.PolicyMetadataCreateRequestRecord{
				{
					FieldName:   "authorized",
					Source:      models.SourcePrimary,
					PolicyClass: engine.ClassAllowlist,
				},
				{
					FieldName:   "expired",
					Source:      models.SourcePrimary,
					PolicyClass: engine.ClassAllowlist,
				},
				{
					FieldName:   "unauthorized",
					Source:      models.SourcePrimary,
					PolicyClass: engine.ClassAllowlist,
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

// Error path tests for UpdateAllowList
// Note: Tests that require closing database connections to simulate errors have been removed
// as they cannot be properly mocked with SQLite. These error scenarios should be tested
// in integration tests with a real PostgreSQL database.

// Error path tests for GetPolicyDecision
// Note: Tests that require closing database connections to simulate errors have been removed
// as they cannot be properly mocked with SQLite. These error scenarios should be tested
// in integration tests with a real PostgreSQL database.
