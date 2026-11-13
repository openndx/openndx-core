package services

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/openndx/openndx-core/internal/pdp/fieldpolicy"
	"github.com/openndx/openndx-core/internal/pdp/grant"
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

// testService decides on the fields and grants in db, and grants access to
// set up the decisions.
type testService struct {
	*PolicyMetadataService
	*grant.Service
}

func newTestService(db *gorm.DB) testService {
	grants := grant.NewService(db)
	return testService{
		PolicyMetadataService: NewPolicyMetadataService(fieldpolicy.NewService(db), grants),
		Service:               grants,
	}
}

func TestNewPolicyMetadataService(t *testing.T) {
	db := setupTestDB(t)
	service := NewPolicyMetadataService(fieldpolicy.NewService(db), grant.NewService(db))
	assert.NotNil(t, service)
	assert.NotNil(t, service.fields)
	assert.NotNil(t, service.grants)
}

func TestPolicyMetadataService_RevokeAccessGrant(t *testing.T) {
	db := setupTestDB(t)
	service := newTestService(db)

	_, err := fieldpolicy.NewService(db).CreatePolicyMetadata(&fieldpolicy.PolicyMetadataCreateRequest{
		SchemaID: "schema-123",
		Records: []fieldpolicy.PolicyMetadataCreateRequestRecord{
			{FieldName: "person.email", Source: fieldpolicy.SourcePrimary, RequiresAllowlist: new(true), RequiresConsent: new(false)},
		},
	})
	require.NoError(t, err)

	grantIDs := map[string]string{}
	for _, applicationID := range []string{"app-123", "app-456"} {
		resp, err := service.CreateAccessGrants(&grant.AccessGrantCreateRequest{
			ApplicationID: applicationID,
			GrantDuration: grant.GrantDurationTypeOneMonth,
			Records: []grant.AccessGrantCreateRequestRecord{
				{FieldName: "person.email", SchemaID: "schema-123"},
			},
		})
		require.NoError(t, err)
		require.Len(t, resp.Records, 1)
		grantIDs[applicationID] = resp.Records[0].ID
	}

	_, err = service.RevokeAccessGrant(grantIDs["app-123"])
	require.NoError(t, err)

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

	// Granting again after a revoke creates a new active grant.
	_, err = service.CreateAccessGrants(&grant.AccessGrantCreateRequest{
		ApplicationID: "app-123",
		GrantDuration: grant.GrantDurationTypeOneMonth,
		Records:       []grant.AccessGrantCreateRequestRecord{{FieldName: "person.email", SchemaID: "schema-123"}},
	})
	require.NoError(t, err)
	assert.Equal(t, "allow", decide("app-123")[0].Outcome)
}

// insertGrant stores an active grant directly, for example one that has
// already expired.
func insertGrant(t *testing.T, db *gorm.DB, schemaID, fieldName, applicationID string, expiresAt time.Time) {
	t.Helper()
	now := time.Now().UTC()
	require.NoError(t, db.Create(&grant.AccessGrant{
		ID:            uuid.New(),
		SchemaID:      schemaID,
		FieldName:     fieldName,
		ApplicationID: applicationID,
		ExpiresAt:     expiresAt,
		Status:        grant.GrantStatusActive,
		CreatedAt:     now,
		UpdatedAt:     now,
	}).Error)
}

func TestPolicyMetadataService_GetPolicyDecision_EdgeCases(t *testing.T) {
	t.Run("GetPolicyDecision_ExpiredAccess", func(t *testing.T) {
		db := setupTestDB(t)
		service := newTestService(db)

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
		service := newTestService(db)

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
		service := newTestService(db)

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
		updateReq := &grant.AccessGrantCreateRequest{
			ApplicationID: "app-123",
			GrantDuration: grant.GrantDurationTypeOneMonth,
			Records: []grant.AccessGrantCreateRequestRecord{
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
		service := newTestService(db)

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
		updateReq := &grant.AccessGrantCreateRequest{
			ApplicationID: "app-123",
			GrantDuration: grant.GrantDurationTypeOneMonth,
			Records: []grant.AccessGrantCreateRequestRecord{
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
		service := newTestService(db)

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
		service := newTestService(db)

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
		service := newTestService(db)

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
		updateReq := &grant.AccessGrantCreateRequest{
			ApplicationID: "app-123",
			GrantDuration: grant.GrantDurationTypeOneMonth,
			Records: []grant.AccessGrantCreateRequestRecord{
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
		service := newTestService(db)

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
		service := newTestService(db)

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
		updateReq1 := &grant.AccessGrantCreateRequest{
			ApplicationID: "app-123",
			GrantDuration: grant.GrantDurationTypeOneMonth,
			Records: []grant.AccessGrantCreateRequestRecord{
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

// Error path tests for GetPolicyDecision
// Note: Tests that require closing database connections to simulate errors have been removed
// as they cannot be properly mocked with SQLite. These error scenarios should be tested
// in integration tests with a real PostgreSQL database.
