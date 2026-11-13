package grant

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/openndx/openndx-core/internal/pdp/decision"
	"github.com/openndx/openndx-core/internal/pdp/pdptest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// setupTestDB creates an in-memory SQLite database for unit testing.
func setupTestDB(t *testing.T) *gorm.DB {
	return pdptest.SetupTestDB(t)
}

// insertField stores a field's policy, as the field policy context
// would, so that grants can be made for it.
func insertField(t *testing.T, db *gorm.DB, schemaID, fieldName string, requiresAllowlist, requiresConsent bool) {
	t.Helper()
	require.NoError(t, db.Exec(
		`INSERT INTO policy_metadata (id, schema_id, field_name, source, requires_allowlist, requires_consent)
			VALUES (?, ?, ?, 'primary', ?, ?)`,
		uuid.NewString(), schemaID, fieldName, requiresAllowlist, requiresConsent,
	).Error)
}

// insertGrant stores an active grant directly, for example one that has
// already expired.
func insertGrant(t *testing.T, db *gorm.DB, schemaID, fieldName, applicationID string, expiresAt time.Time) {
	t.Helper()
	now := time.Now().UTC()
	require.NoError(t, db.Create(&AccessGrant{
		ID:            uuid.New(),
		SchemaID:      schemaID,
		FieldName:     fieldName,
		ApplicationID: applicationID,
		ExpiresAt:     expiresAt,
		Status:        GrantStatusActive,
		CreatedAt:     now,
		UpdatedAt:     now,
	}).Error)
}

func plainRequest(applicationID string, duration GrantDurationType, records ...AccessGrantCreateRequestRecord) *AccessGrantCreateRequest {
	return &AccessGrantCreateRequest{ApplicationID: applicationID, GrantDuration: duration, Records: records}
}

func TestNewService(t *testing.T) {
	db := setupTestDB(t)
	service := NewService(db)
	assert.NotNil(t, service)
	assert.NotNil(t, service.db)
}

func TestService_RevokeAccessGrant(t *testing.T) {
	db := setupTestDB(t)
	service := NewService(db)
	insertField(t, db, "schema-123", "person.email", true, false)
	email := AccessGrantCreateRequestRecord{FieldName: "person.email", SchemaID: "schema-123"}

	grantIDs := map[string]string{}
	for _, applicationID := range []string{"app-123", "app-456"} {
		resp, err := service.CreateAccessGrants(plainRequest(applicationID, GrantDurationTypeOneMonth, email))
		require.NoError(t, err)
		require.Len(t, resp.Records, 1)
		grantIDs[applicationID] = resp.Records[0].ID
	}

	revoked, err := service.RevokeAccessGrant(grantIDs["app-123"])
	require.NoError(t, err)
	assert.Equal(t, GrantStatusRevoked, revoked.Status)

	active, err := service.ListAccessGrants(AccessGrantFilter{Status: GrantStatusActive})
	require.NoError(t, err)
	require.Len(t, active.Records, 1)
	assert.Equal(t, "app-456", active.Records[0].ApplicationID)

	// The revoked grant is kept as a record.
	revokedOnly, err := service.ListAccessGrants(AccessGrantFilter{Status: GrantStatusRevoked})
	require.NoError(t, err)
	require.Len(t, revokedOnly.Records, 1)
	assert.Equal(t, "app-123", revokedOnly.Records[0].ApplicationID)

	_, err = service.RevokeAccessGrant(grantIDs["app-123"])
	assert.ErrorIs(t, err, ErrAccessGrantNotFound)

	_, err = service.RevokeAccessGrant("not-a-uuid")
	assert.ErrorIs(t, err, ErrAccessGrantNotFound)

	// Granting again after a revoke creates a new active grant.
	resp, err := service.CreateAccessGrants(plainRequest("app-123", GrantDurationTypeOneMonth, email))
	require.NoError(t, err)
	assert.NotEqual(t, grantIDs["app-123"], resp.Records[0].ID)
}

func TestService_GetAccessGrant(t *testing.T) {
	db := setupTestDB(t)
	service := NewService(db)
	insertField(t, db, "schema-123", "person.email", true, false)

	created, err := service.CreateAccessGrants(plainRequest("app-123", GrantDurationTypeOneMonth,
		AccessGrantCreateRequestRecord{FieldName: "person.email", SchemaID: "schema-123"}))
	require.NoError(t, err)

	got, err := service.GetAccessGrant(created.Records[0].ID)
	require.NoError(t, err)
	assert.Equal(t, created.Records[0], *got)

	_, err = service.GetAccessGrant(uuid.NewString())
	assert.ErrorIs(t, err, ErrAccessGrantNotFound)
	_, err = service.GetAccessGrant("not-a-uuid")
	assert.ErrorIs(t, err, ErrAccessGrantNotFound)
}

func TestService_ListAccessGrants(t *testing.T) {
	db := setupTestDB(t)
	service := NewService(db)

	for _, schemaID := range []string{"schema-1", "schema-2"} {
		insertField(t, db, schemaID, "field1", true, false)
	}
	for _, applicationID := range []string{"app-A", "app-B"} {
		_, err := service.CreateAccessGrants(plainRequest(applicationID, GrantDurationTypeOneMonth,
			AccessGrantCreateRequestRecord{FieldName: "field1", SchemaID: "schema-1"},
			AccessGrantCreateRequestRecord{FieldName: "field1", SchemaID: "schema-2"},
		))
		require.NoError(t, err)
	}

	all, err := service.ListAccessGrants(AccessGrantFilter{})
	require.NoError(t, err)
	assert.Len(t, all.Records, 4)

	byApp, err := service.ListAccessGrants(AccessGrantFilter{ApplicationID: "app-A"})
	require.NoError(t, err)
	require.Len(t, byApp.Records, 2)
	assert.Equal(t, "schema-1", byApp.Records[0].SchemaID)
	assert.Equal(t, "schema-2", byApp.Records[1].SchemaID)

	bySchemaAndApp, err := service.ListAccessGrants(AccessGrantFilter{ApplicationID: "app-B", SchemaID: "schema-2"})
	require.NoError(t, err)
	require.Len(t, bySchemaAndApp.Records, 1)
	assert.Equal(t, "app-B", bySchemaAndApp.Records[0].ApplicationID)

	none, err := service.ListAccessGrants(AccessGrantFilter{ApplicationID: "app-C"})
	require.NoError(t, err)
	assert.NotNil(t, none.Records, "records serializes as [] rather than null")
	assert.Empty(t, none.Records)

	_, err = service.ListAccessGrants(AccessGrantFilter{Status: "pending"})
	assert.ErrorIs(t, err, ErrInvalidGrantRequest)
}

func TestService_ActiveGrants(t *testing.T) {
	db := setupTestDB(t)
	service := NewService(db)
	for _, schemaID := range []string{"schema-1", "schema-2"} {
		insertField(t, db, schemaID, "field1", true, true)
		insertField(t, db, schemaID, "field2", true, true)
	}

	expired := time.Now().Add(-time.Hour)
	insertGrant(t, db, "schema-1", "field1", "app-A", expired)
	_, err := service.CreateAccessGrants(&AccessGrantCreateRequest{
		ApplicationID: "app-A",
		GrantDuration: GrantDurationTypeOneMonth,
		Records:       []AccessGrantCreateRequestRecord{{SchemaID: "schema-1", FieldName: "field1"}},
		ConsentBypass: true,
		LegalBasis:    "Police Act s.12",
	})
	require.NoError(t, err)
	revoke, err := service.CreateAccessGrants(plainRequest("app-A", GrantDurationTypeOneMonth,
		AccessGrantCreateRequestRecord{SchemaID: "schema-1", FieldName: "field2"}))
	require.NoError(t, err)
	_, err = service.RevokeAccessGrant(revoke.Records[0].ID)
	require.NoError(t, err)
	// Same field name in another schema, and a grant for another application.
	insertGrant(t, db, "schema-2", "field1", "app-A", time.Now().AddDate(0, 1, 0))
	insertGrant(t, db, "schema-1", "field1", "app-B", time.Now().AddDate(0, 1, 0))

	grants, err := service.ActiveGrants(t.Context(), "app-A", []decision.FieldKey{
		{SchemaID: "schema-1", FieldName: "field1"},
		{SchemaID: "schema-1", FieldName: "field2"},
	})
	require.NoError(t, err)

	// Both active grants for schema-1.field1, the expired plain one included;
	// not the revoked grant, other schemas' fields or other applications.
	require.Len(t, grants, 2)
	var bypass, plain int
	for _, g := range grants {
		assert.Equal(t, decision.FieldKey{SchemaID: "schema-1", FieldName: "field1"}, g.FieldKey)
		if g.Grant.ConsentBypass {
			bypass++
		} else {
			plain++
			assert.WithinDuration(t, expired, g.Grant.ExpiresAt, time.Second)
		}
	}
	assert.Equal(t, 1, bypass)
	assert.Equal(t, 1, plain)

	none, err := service.ActiveGrants(t.Context(), "app-A", nil)
	require.NoError(t, err)
	assert.Empty(t, none)
}

func TestService_CreateAccessGrants_EdgeCases(t *testing.T) {
	t.Run("CreateAccessGrants_EmptyRecords", func(t *testing.T) {
		db := setupTestDB(t)
		service := NewService(db)

		resp, err := service.CreateAccessGrants(plainRequest("app-123", GrantDurationTypeOneMonth))
		assert.NoError(t, err)
		assert.NotNil(t, resp)
		assert.Equal(t, 0, len(resp.Records))
	})

	t.Run("CreateAccessGrants_InvalidGrantDuration", func(t *testing.T) {
		db := setupTestDB(t)
		service := NewService(db)
		insertField(t, db, "schema-123", "field1", true, false)

		_, err := service.CreateAccessGrants(plainRequest("app-123", "invalid-duration",
			AccessGrantCreateRequestRecord{FieldName: "field1", SchemaID: "schema-123"}))
		assert.ErrorIs(t, err, ErrInvalidGrantRequest)
		assert.Contains(t, err.Error(), "invalid grant duration")
	})

	t.Run("CreateAccessGrants_FieldNotFound", func(t *testing.T) {
		db := setupTestDB(t)
		service := NewService(db)

		_, err := service.CreateAccessGrants(plainRequest("app-123", GrantDurationTypeOneMonth,
			AccessGrantCreateRequestRecord{FieldName: "nonexistent", SchemaID: "schema-123"}))
		assert.ErrorIs(t, err, ErrFieldNotFound)
		assert.EqualError(t, err, "policy metadata not found for schema_id schema-123 and field_name nonexistent")
	})

	t.Run("CreateAccessGrants_OneYearDuration", func(t *testing.T) {
		db := setupTestDB(t)
		service := NewService(db)
		insertField(t, db, "schema-123", "field1", true, false)

		resp, err := service.CreateAccessGrants(plainRequest("app-123", GrantDurationTypeOneYear,
			AccessGrantCreateRequestRecord{FieldName: "field1", SchemaID: "schema-123"}))
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
		service := NewService(db)

		_, err := service.CreateAccessGrants(plainRequest(" ", GrantDurationTypeOneMonth))
		assert.ErrorIs(t, err, ErrInvalidGrantRequest)

		_, err = service.CreateAccessGrants(plainRequest("app-123", GrantDurationTypeOneMonth,
			AccessGrantCreateRequestRecord{FieldName: "field1"}))
		assert.ErrorIs(t, err, ErrInvalidGrantRequest)
	})

	t.Run("CreateAccessGrants_RenewsActiveGrant", func(t *testing.T) {
		db := setupTestDB(t)
		service := NewService(db)
		insertField(t, db, "schema-123", "field1", true, false)
		field1 := AccessGrantCreateRequestRecord{FieldName: "field1", SchemaID: "schema-123"}

		resp1, err := service.CreateAccessGrants(plainRequest("app-123", GrantDurationTypeOneMonth, field1))
		assert.NoError(t, err)
		assert.NotNil(t, resp1)

		// Re-grant with a different duration
		resp2, err := service.CreateAccessGrants(plainRequest("app-123", GrantDurationTypeOneYear, field1))
		assert.NoError(t, err)
		assert.NotNil(t, resp2)
		assert.Equal(t, 1, len(resp2.Records))

		// The active grant is renewed, not duplicated.
		assert.Equal(t, resp1.Records[0].ID, resp2.Records[0].ID)
		assert.Equal(t, resp1.Records[0].CreatedAt, resp2.Records[0].CreatedAt)
		var count int64
		require.NoError(t, db.Model(&AccessGrant{}).Count(&count).Error)
		assert.Equal(t, int64(1), count)

		// Verify expiration is approximately 1 year from now
		expiresAt, _ := time.Parse(time.RFC3339, resp2.Records[0].ExpiresAt)
		expectedExpiry := time.Now().AddDate(1, 0, 0)
		assert.WithinDuration(t, expectedExpiry, expiresAt, 5*time.Second)
	})

	t.Run("CreateAccessGrants_MultipleFields", func(t *testing.T) {
		db := setupTestDB(t)
		service := NewService(db)
		insertField(t, db, "schema-123", "field1", true, false)
		insertField(t, db, "schema-123", "field2", true, false)

		resp, err := service.CreateAccessGrants(plainRequest("app-123", GrantDurationTypeOneMonth,
			AccessGrantCreateRequestRecord{FieldName: "field1", SchemaID: "schema-123"},
			AccessGrantCreateRequestRecord{FieldName: "field2", SchemaID: "schema-123"},
		))
		assert.NoError(t, err)
		assert.NotNil(t, resp)
		assert.Equal(t, 2, len(resp.Records))
	})
}
