package services

import (
	"testing"
	"time"

	"github.com/google/uuid"
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

	_, err = service.CreateAccessGrants(&models.AccessGrantCreateRequest{
		ApplicationID: "app-123",
		GrantDuration: models.GrantDurationTypeOneMonth,
		Records: []models.AccessGrantCreateRequestRecord{
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

	// Patching a field keeps its grants.
	grants, err := service.ListAccessGrants(models.AccessGrantFilter{ApplicationID: "app-123"})
	require.NoError(t, err)
	require.Len(t, grants.Records, 1)
	assert.Equal(t, "person.email", grants.Records[0].FieldName)

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

	_, err = service.CreateAccessGrants(&models.AccessGrantCreateRequest{
		ApplicationID: "app-123",
		GrantDuration: models.GrantDurationTypeOneMonth,
		Records: []models.AccessGrantCreateRequestRecord{
			{FieldName: "person.name", SchemaID: "schema-123"},
			{FieldName: "person.email", SchemaID: "schema-123"},
		},
	})
	require.NoError(t, err)

	id := createResp.Records[1].ID
	require.NoError(t, service.DeletePolicyMetadata(id))

	var records []models.PolicyMetadata
	require.NoError(t, db.Where("schema_id = ?", "schema-123").Find(&records).Error)
	require.Len(t, records, 1)
	assert.Equal(t, "person.name", records[0].FieldName)

	// Deleting a field deletes its grants.
	var grants []models.AccessGrant
	require.NoError(t, db.Find(&grants).Error)
	require.Len(t, grants, 1)
	assert.Equal(t, "person.name", grants[0].FieldName)

	err = service.DeletePolicyMetadata(id)
	assert.ErrorIs(t, err, ErrPolicyMetadataNotFound)
}

// TestCreatePolicyMetadata_KeepsAndDropsGrants verifies that schema
// synchronization keeps the grants of fields it updates and deletes the
// grants of fields it removes.
func TestCreatePolicyMetadata_KeepsAndDropsGrants(t *testing.T) {
	db := setupTestDB(t)
	service := NewPolicyMetadataService(db, testhelpers.Engine(t))

	_, err := service.CreatePolicyMetadata(&models.PolicyMetadataCreateRequest{
		SchemaID: "schema-123",
		Records: []models.PolicyMetadataCreateRequestRecord{
			{FieldName: "person.email", DisplayName: testhelpers.StringPtr("Email"), Source: models.SourcePrimary, PolicyClass: engine.ClassAllowlist},
			{FieldName: "person.phone", Source: models.SourcePrimary, PolicyClass: engine.ClassAllowlist},
		},
	})
	require.NoError(t, err)

	_, err = service.CreateAccessGrants(&models.AccessGrantCreateRequest{
		ApplicationID: "app-A",
		GrantDuration: models.GrantDurationTypeOneMonth,
		Records: []models.AccessGrantCreateRequestRecord{
			{FieldName: "person.email", SchemaID: "schema-123"},
			{FieldName: "person.phone", SchemaID: "schema-123"},
		},
	})
	require.NoError(t, err)

	_, err = service.CreatePolicyMetadata(&models.PolicyMetadataCreateRequest{
		SchemaID: "schema-123",
		Records: []models.PolicyMetadataCreateRequestRecord{
			{FieldName: "person.email", DisplayName: testhelpers.StringPtr("Primary Email"), Source: models.SourcePrimary, PolicyClass: engine.ClassAllowlist},
		},
	})
	require.NoError(t, err)

	grants, err := service.ListAccessGrants(models.AccessGrantFilter{ApplicationID: "app-A"})
	require.NoError(t, err)
	require.Len(t, grants.Records, 1)
	assert.Equal(t, "person.email", grants.Records[0].FieldName)
	assert.Equal(t, models.GrantStatusActive, grants.Records[0].Status)
}

func TestPolicyMetadataService_RevokeAccessGrant(t *testing.T) {
	db := setupTestDB(t)
	service := NewPolicyMetadataService(db, testhelpers.Engine(t))

	_, err := service.CreatePolicyMetadata(&models.PolicyMetadataCreateRequest{
		SchemaID: "schema-123",
		Records: []models.PolicyMetadataCreateRequestRecord{
			{FieldName: "person.email", Source: models.SourcePrimary, PolicyClass: engine.ClassAllowlist},
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
	service := NewPolicyMetadataService(db, testhelpers.Engine(t))

	for _, schemaID := range []string{"schema-1", "schema-2"} {
		_, err := service.CreatePolicyMetadata(&models.PolicyMetadataCreateRequest{
			SchemaID: schemaID,
			Records: []models.PolicyMetadataCreateRequestRecord{
				{FieldName: "field1", Source: models.SourcePrimary, PolicyClass: engine.ClassAllowlist},
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

func TestPolicyMetadataService_CreateAccessGrants_EdgeCases(t *testing.T) {
	t.Run("CreateAccessGrants_EmptyRecords", func(t *testing.T) {
		db := setupTestDB(t)
		service := NewPolicyMetadataService(db, testhelpers.Engine(t))

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
		service := NewPolicyMetadataService(db, testhelpers.Engine(t))

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
		assert.ErrorIs(t, err, ErrPolicyMetadataNotFound)
	})

	t.Run("CreateAccessGrants_OneYearDuration", func(t *testing.T) {
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
		service := NewPolicyMetadataService(db, testhelpers.Engine(t))

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
		service := NewPolicyMetadataService(db, testhelpers.Engine(t))

		owner := models.OwnerCitizen
		_, err := service.CreatePolicyMetadata(&models.PolicyMetadataCreateRequest{
			SchemaID: "schema-123",
			Records: []models.PolicyMetadataCreateRequestRecord{
				{FieldName: "field1", Source: models.SourcePrimary, PolicyClass: engine.ClassConsent, Owner: &owner},
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
		service := NewPolicyMetadataService(db, testhelpers.Engine(t))

		_, err := service.CreatePolicyMetadata(&models.PolicyMetadataCreateRequest{
			SchemaID: "schema-123",
			Records: []models.PolicyMetadataCreateRequestRecord{
				{FieldName: "field1", Source: models.SourcePrimary, PolicyClass: engine.ClassOpen},
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
