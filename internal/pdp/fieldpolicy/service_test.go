package fieldpolicy

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/openndx/openndx-core/internal/pdp/pdptest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// setupTestDB creates an in-memory SQLite database for unit testing.
func setupTestDB(t *testing.T) *gorm.DB {
	return pdptest.SetupTestDB(t)
}

// insertGrant stores an active access grant for a field, as the grant context
// would, to check what happens to grants when fields change.
func insertGrant(t *testing.T, db *gorm.DB, schemaID, fieldName, applicationID string) {
	t.Helper()
	now := time.Now().UTC()
	require.NoError(t, db.Exec(
		`INSERT INTO access_grants
			(id, schema_id, field_name, application_id, expires_at, status, consent_bypass, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, 'active', false, ?, ?)`,
		uuid.NewString(), schemaID, fieldName, applicationID, now.AddDate(0, 1, 0), now, now,
	).Error)
}

// grantedFields returns the names of the fields the application holds active
// grants for, in order.
func grantedFields(t *testing.T, db *gorm.DB, applicationID string) []string {
	t.Helper()
	var fields []string
	require.NoError(t, db.Table("access_grants").
		Where("application_id = ? AND status = ?", applicationID, "active").
		Order("field_name").
		Pluck("field_name", &fields).Error)
	return fields
}

func TestNewService(t *testing.T) {
	db := setupTestDB(t)
	service := NewService(db)
	assert.NotNil(t, service)
	assert.NotNil(t, service.db)
}

func TestService_ListPolicyMetadata(t *testing.T) {
	db := setupTestDB(t)
	service := NewService(db)

	_, err := service.CreatePolicyMetadata(&PolicyMetadataCreateRequest{
		SchemaID: "schema-123",
		Records: []PolicyMetadataCreateRequestRecord{
			{
				FieldName:         "person.name",
				Source:            SourcePrimary,
				RequiresAllowlist: new(true), RequiresConsent: new(false),
			},
			{
				FieldName:         "person.email",
				Source:            SourcePrimary,
				RequiresAllowlist: new(true), RequiresConsent: new(false),
			},
		},
	})
	require.NoError(t, err)

	_, err = service.CreatePolicyMetadata(&PolicyMetadataCreateRequest{
		SchemaID: "other-schema",
		Records: []PolicyMetadataCreateRequestRecord{
			{
				FieldName:         "other.field",
				Source:            SourcePrimary,
				RequiresAllowlist: new(true), RequiresConsent: new(false),
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

func TestService_FieldsByKeys(t *testing.T) {
	db := setupTestDB(t)
	service := NewService(db)
	ctx := context.Background()

	_, err := service.CreatePolicyMetadata(&PolicyMetadataCreateRequest{
		SchemaID: "schema-123",
		Records: []PolicyMetadataCreateRequestRecord{
			{FieldName: "person.name", DisplayName: new("Name"), Source: SourcePrimary, RequiresAllowlist: new(true), RequiresConsent: new(false)},
			{FieldName: "person.email", Source: SourcePrimary, Owner: new(OwnerCitizen), RequiresAllowlist: new(false), RequiresConsent: new(true)},
		},
	})
	require.NoError(t, err)
	_, err = service.CreatePolicyMetadata(&PolicyMetadataCreateRequest{
		SchemaID: "other-schema",
		Records: []PolicyMetadataCreateRequestRecord{
			{FieldName: "person.name", Source: SourcePrimary, RequiresAllowlist: new(false), RequiresConsent: new(false)},
		},
	})
	require.NoError(t, err)

	fields, err := service.FieldsByKeys(ctx, []FieldKey{
		{SchemaID: "schema-123", FieldName: "person.email"},
		{SchemaID: "other-schema", FieldName: "person.name"},
		{SchemaID: "schema-123", FieldName: "person.missing"},
	})
	require.NoError(t, err)
	require.Len(t, fields, 2, "only the named fields that exist are returned")

	byKey := make(map[FieldKey]PolicyMetadata, len(fields))
	for _, f := range fields {
		byKey[FieldKey{SchemaID: f.SchemaID, FieldName: f.FieldName}] = f
	}
	email := byKey[FieldKey{SchemaID: "schema-123", FieldName: "person.email"}]
	assert.False(t, email.RequiresAllowlist)
	assert.True(t, email.RequiresConsent)
	require.NotNil(t, email.Owner)
	assert.Equal(t, OwnerCitizen, *email.Owner)
	other, found := byKey[FieldKey{SchemaID: "other-schema", FieldName: "person.name"}]
	require.True(t, found, "other-schema/person.name must be returned")
	assert.False(t, other.RequiresAllowlist, "a field is matched on schema and name together")

	none, err := service.FieldsByKeys(ctx, nil)
	require.NoError(t, err)
	assert.Empty(t, none)
}

func TestService_PatchPolicyMetadata(t *testing.T) {
	db := setupTestDB(t)
	service := NewService(db)

	createResp, err := service.CreatePolicyMetadata(&PolicyMetadataCreateRequest{
		SchemaID: "schema-123",
		Records: []PolicyMetadataCreateRequestRecord{
			{
				FieldName:         "person.name",
				DisplayName:       new("Name"),
				Source:            SourcePrimary,
				RequiresAllowlist: new(true), RequiresConsent: new(false),
			},
			{
				FieldName:         "person.email",
				DisplayName:       new("Email"),
				Description:       new("Old description"),
				Source:            SourcePrimary,
				RequiresAllowlist: new(true), RequiresConsent: new(false),
			},
		},
	})
	require.NoError(t, err)
	require.Len(t, createResp.Records, 2)

	insertGrant(t, db, "schema-123", "person.email", "app-123")

	id := createResp.Records[1].ID

	resp, err := service.PatchPolicyMetadata(id, &PolicyMetadataPatchRequest{
		DisplayName: OptionalPatchField[string]{
			Set:   true,
			Value: new("Primary Email"),
		},
		Description: OptionalPatchField[string]{
			Set:   true,
			Value: nil,
		},
		RequiresConsent: OptionalPatchField[bool]{
			Set:   true,
			Value: new(true),
		},
	})
	require.NoError(t, err)
	require.NotNil(t, resp.DisplayName)
	assert.Equal(t, "Primary Email", *resp.DisplayName)
	assert.Nil(t, resp.Description)
	assert.True(t, resp.RequiresAllowlist, "an omitted flag is unchanged")
	assert.True(t, resp.RequiresConsent)

	// Patching a field keeps its grants.
	assert.Equal(t, []string{"person.email"}, grantedFields(t, db, "app-123"))

	var count int64
	require.NoError(t, db.Model(&PolicyMetadata{}).
		Where("schema_id = ?", "schema-123").
		Count(&count).Error)
	assert.Equal(t, int64(2), count)

	var nameRecord PolicyMetadata
	require.NoError(t, db.Where(
		"schema_id = ? AND field_name = ?",
		"schema-123",
		"person.name",
	).First(&nameRecord).Error)
	require.NotNil(t, nameRecord.DisplayName)
	assert.Equal(t, "Name", *nameRecord.DisplayName)

	// Reject any SQL statement that updates the policy flags. A display-only
	// patch must still succeed because it should update only the supplied field.
	require.NoError(t, db.Exec(`
		CREATE TRIGGER reject_policy_flags_update
		BEFORE UPDATE OF requires_allowlist, requires_consent ON policy_metadata
		BEGIN
			SELECT RAISE(FAIL, 'policy flags were unexpectedly updated');
		END;
	`).Error)

	displayOnlyResp, err := service.PatchPolicyMetadata(id, &PolicyMetadataPatchRequest{
		DisplayName: OptionalPatchField[string]{
			Set:   true,
			Value: new("Contact Email"),
		},
	})
	require.NoError(t, err)
	require.NotNil(t, displayOnlyResp.DisplayName)
	assert.Equal(t, "Contact Email", *displayOnlyResp.DisplayName)
	assert.True(t, displayOnlyResp.RequiresAllowlist)
	assert.True(t, displayOnlyResp.RequiresConsent)

	_, err = service.PatchPolicyMetadata("not-a-uuid", &PolicyMetadataPatchRequest{})
	assert.ErrorIs(t, err, ErrPolicyMetadataNotFound)
}

func TestService_PatchPolicyMetadata_Flags(t *testing.T) {
	db := setupTestDB(t)
	service := NewService(db)

	createResp, err := service.CreatePolicyMetadata(&PolicyMetadataCreateRequest{
		SchemaID: "schema-123",
		Records: []PolicyMetadataCreateRequestRecord{
			{
				FieldName:         "person.email",
				Source:            SourcePrimary,
				RequiresAllowlist: new(true),
				RequiresConsent:   new(true),
			},
		},
	})
	require.NoError(t, err)
	id := createResp.Records[0].ID

	// A flag can be changed but not cleared.
	_, err = service.PatchPolicyMetadata(id, &PolicyMetadataPatchRequest{
		RequiresAllowlist: OptionalPatchField[bool]{Set: true},
	})
	assert.ErrorIs(t, err, ErrInvalidPolicy)

	// Setting both flags to false makes the field public.
	resp, err := service.PatchPolicyMetadata(id, &PolicyMetadataPatchRequest{
		RequiresAllowlist: OptionalPatchField[bool]{Set: true, Value: new(false)},
		RequiresConsent:   OptionalPatchField[bool]{Set: true, Value: new(false)},
	})
	require.NoError(t, err)
	assert.False(t, resp.RequiresAllowlist)
	assert.False(t, resp.RequiresConsent)

	var stored PolicyMetadata
	require.NoError(t, db.First(&stored, "field_name = ?", "person.email").Error)
	assert.False(t, stored.RequiresAllowlist, "false is written, not skipped")
	assert.False(t, stored.RequiresConsent, "false is written, not skipped")
}

func TestService_DeletePolicyMetadata(t *testing.T) {
	db := setupTestDB(t)
	service := NewService(db)

	createResp, err := service.CreatePolicyMetadata(&PolicyMetadataCreateRequest{
		SchemaID: "schema-123",
		Records: []PolicyMetadataCreateRequestRecord{
			{
				FieldName:         "person.name",
				Source:            SourcePrimary,
				RequiresAllowlist: new(true), RequiresConsent: new(false),
			},
			{
				FieldName:         "person.email",
				Source:            SourcePrimary,
				RequiresAllowlist: new(true), RequiresConsent: new(false),
			},
		},
	})
	require.NoError(t, err)
	require.Len(t, createResp.Records, 2)

	insertGrant(t, db, "schema-123", "person.name", "app-123")
	insertGrant(t, db, "schema-123", "person.email", "app-123")

	id := createResp.Records[1].ID
	require.NoError(t, service.DeletePolicyMetadata(id))

	var records []PolicyMetadata
	require.NoError(t, db.Where("schema_id = ?", "schema-123").Find(&records).Error)
	require.Len(t, records, 1)
	assert.Equal(t, "person.name", records[0].FieldName)

	// Deleting a field deletes its grants.
	assert.Equal(t, []string{"person.name"}, grantedFields(t, db, "app-123"))

	err = service.DeletePolicyMetadata(id)
	assert.ErrorIs(t, err, ErrPolicyMetadataNotFound)
}

// TestCreatePolicyMetadata_KeepsAndDropsGrants verifies that schema
// synchronization keeps the grants of fields it updates and deletes the
// grants of fields it removes.
func TestCreatePolicyMetadata_KeepsAndDropsGrants(t *testing.T) {
	db := setupTestDB(t)
	service := NewService(db)

	_, err := service.CreatePolicyMetadata(&PolicyMetadataCreateRequest{
		SchemaID: "schema-123",
		Records: []PolicyMetadataCreateRequestRecord{
			{FieldName: "person.email", DisplayName: new("Email"), Source: SourcePrimary, RequiresAllowlist: new(true), RequiresConsent: new(false)},
			{FieldName: "person.phone", Source: SourcePrimary, RequiresAllowlist: new(true), RequiresConsent: new(false)},
		},
	})
	require.NoError(t, err)

	insertGrant(t, db, "schema-123", "person.email", "app-A")
	insertGrant(t, db, "schema-123", "person.phone", "app-A")

	_, err = service.CreatePolicyMetadata(&PolicyMetadataCreateRequest{
		SchemaID: "schema-123",
		Records: []PolicyMetadataCreateRequestRecord{
			{FieldName: "person.email", DisplayName: new("Primary Email"), Source: SourcePrimary, RequiresAllowlist: new(true), RequiresConsent: new(false)},
		},
	})
	require.NoError(t, err)

	assert.Equal(t, []string{"person.email"}, grantedFields(t, db, "app-A"))
}

func TestService_CreatePolicyMetadata_EdgeCases(t *testing.T) {
	t.Run("CreatePolicyMetadata_EmptyRecords", func(t *testing.T) {
		db := setupTestDB(t)
		service := NewService(db)

		req := &PolicyMetadataCreateRequest{
			SchemaID: "schema-123",
			Records:  []PolicyMetadataCreateRequestRecord{},
		}

		resp, err := service.CreatePolicyMetadata(req)
		assert.NoError(t, err)
		assert.NotNil(t, resp)
		assert.Equal(t, 0, len(resp.Records))
	})

	t.Run("CreatePolicyMetadata_DeleteObsoleteRecords", func(t *testing.T) {
		db := setupTestDB(t)
		service := NewService(db)

		// Create initial records
		initialReq := &PolicyMetadataCreateRequest{
			SchemaID: "schema-123",
			Records: []PolicyMetadataCreateRequestRecord{
				{
					FieldName:         "field1",
					Source:            SourcePrimary,
					RequiresAllowlist: new(true), RequiresConsent: new(false),
				},
				{
					FieldName:         "field2",
					Source:            SourcePrimary,
					RequiresAllowlist: new(true), RequiresConsent: new(false),
				},
			},
		}
		_, err := service.CreatePolicyMetadata(initialReq)
		assert.NoError(t, err)

		// Update with only one field (field2 should be deleted)
		updateReq := &PolicyMetadataCreateRequest{
			SchemaID: "schema-123",
			Records: []PolicyMetadataCreateRequestRecord{
				{
					FieldName:         "field1",
					Source:            SourcePrimary,
					RequiresAllowlist: new(true), RequiresConsent: new(false),
				},
			},
		}
		_, err = service.CreatePolicyMetadata(updateReq)
		assert.NoError(t, err)

		// Verify field2 was deleted
		var count int64
		db.Model(&PolicyMetadata{}).Where("field_name = ?", "field2").Count(&count)
		assert.Equal(t, int64(0), count)
	})

	t.Run("CreatePolicyMetadata_TransactionRollback", func(t *testing.T) {
		db := setupTestDB(t)
		service := NewService(db)

		// One record without a flag rejects the whole request before anything is written
		req := &PolicyMetadataCreateRequest{
			SchemaID: "schema-123",
			Records: []PolicyMetadataCreateRequestRecord{
				{
					FieldName:         "field1",
					Source:            SourcePrimary,
					RequiresAllowlist: new(true), RequiresConsent: new(false),
				},
				{
					FieldName:       "field2",
					Source:          SourcePrimary,
					RequiresConsent: new(false),
				},
			},
		}

		_, err := service.CreatePolicyMetadata(req)
		assert.ErrorIs(t, err, ErrInvalidPolicy)
		assert.ErrorContains(t, err, "field2")

		var count int64
		db.Model(&PolicyMetadata{}).Count(&count)
		assert.Equal(t, int64(0), count)
	})

	t.Run("CreatePolicyMetadata_StoresFlags", func(t *testing.T) {
		db := setupTestDB(t)
		service := NewService(db)

		flags := []struct{ allowlist, consent bool }{{true, true}, {true, false}, {false, true}, {false, false}}
		records := make([]PolicyMetadataCreateRequestRecord, 0, len(flags))
		for i, f := range flags {
			records = append(records, PolicyMetadataCreateRequestRecord{
				FieldName:         fmt.Sprintf("field%d", i),
				Source:            SourcePrimary,
				RequiresAllowlist: new(f.allowlist),
				RequiresConsent:   new(f.consent),
			})
		}
		_, err := service.CreatePolicyMetadata(&PolicyMetadataCreateRequest{SchemaID: "schema-123", Records: records})
		require.NoError(t, err)

		var stored []PolicyMetadata
		require.NoError(t, db.Order("field_name").Find(&stored).Error)
		require.Len(t, stored, len(flags))
		for i, f := range flags {
			assert.Equal(t, f.allowlist, stored[i].RequiresAllowlist, stored[i].FieldName)
			assert.Equal(t, f.consent, stored[i].RequiresConsent, stored[i].FieldName)
		}
	})

	t.Run("CreatePolicyMetadata_MixedNewAndUpdated", func(t *testing.T) {
		db := setupTestDB(t)
		service := NewService(db)

		// Create initial record
		initialReq := &PolicyMetadataCreateRequest{
			SchemaID: "schema-123",
			Records: []PolicyMetadataCreateRequestRecord{
				{
					FieldName:         "field1",
					Source:            SourcePrimary,
					RequiresAllowlist: new(true), RequiresConsent: new(false),
				},
			},
		}
		_, err := service.CreatePolicyMetadata(initialReq)
		assert.NoError(t, err)

		// Update existing and add new
		updateReq := &PolicyMetadataCreateRequest{
			SchemaID: "schema-123",
			Records: []PolicyMetadataCreateRequestRecord{
				{
					FieldName:         "field1",
					DisplayName:       new("Updated Field 1"),
					Source:            SourcePrimary,
					RequiresAllowlist: new(true), RequiresConsent: new(false),
				},
				{
					FieldName:         "field2",
					DisplayName:       new("New Field 2"),
					Source:            SourcePrimary,
					RequiresAllowlist: new(true), RequiresConsent: new(false),
				},
			},
		}
		resp, err := service.CreatePolicyMetadata(updateReq)
		assert.NoError(t, err)
		assert.Equal(t, 2, len(resp.Records))

		// Verify both records exist
		var count int64
		db.Model(&PolicyMetadata{}).Where("schema_id = ?", "schema-123").Count(&count)
		assert.Equal(t, int64(2), count)
	})

	t.Run("CreatePolicyMetadata_WithOwner", func(t *testing.T) {
		db := setupTestDB(t)
		service := NewService(db)

		req := &PolicyMetadataCreateRequest{
			SchemaID: "schema-123",
			Records: []PolicyMetadataCreateRequestRecord{
				{
					FieldName:         "field1",
					Source:            SourcePrimary,
					Owner:             new(OwnerCitizen),
					RequiresAllowlist: new(true), RequiresConsent: new(true),
				},
			},
		}

		resp, err := service.CreatePolicyMetadata(req)
		assert.NoError(t, err)
		assert.Equal(t, 1, len(resp.Records))
		assert.Equal(t, OwnerCitizen, *resp.Records[0].Owner)
	})
}
