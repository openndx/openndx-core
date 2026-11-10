package policy

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/OpenNDX/openndx-core/tests/integration/testutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPolicy_CreateMetadata_TransactionRollback tests that invalid requests
// are properly rejected and don't leave partial data in the database.
// This requires a real PostgreSQL database to test transaction rollback behavior.
func TestPolicy_CreateMetadata_TransactionRollback(t *testing.T) {
	schemaID := generateTestID("test-schema-rollback")

	// The second record names an unknown policy class, so the whole request fails
	req := PolicyMetadataCreateRequest{
		SchemaID: schemaID,
		Records: []PolicyMetadataCreateRequestRecord{
			{
				FieldName:   "field1",
				Source:      "primary",
				PolicyClass: "allowlist",
			},
			{
				FieldName:   "field2",
				Source:      "primary",
				PolicyClass: "no_such_class",
			},
		},
	}

	reqBody, err := json.Marshal(req)
	require.NoError(t, err)

	resp, err := http.Post(pdpBaseURL+"/api/v1/policy/metadata", "application/json", bytes.NewBuffer(reqBody))
	require.NoError(t, err)
	defer resp.Body.Close()

	// Should return error due to validation failure
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "Should reject invalid request")

	// Verify transaction was rolled back - no records should exist
	cleanup := testutils.WithTestDBEnv(t, testutils.DBConfig{
		Port:     "5433",
		Database: "policy_db",
		Password: "password",
	})
	defer cleanup()

	db := testutils.SetupPDPDB(t)
	if db != nil {
		var count int64
		db.Table("policy_metadata").Where("schema_id = ?", schemaID).Count(&count)
		assert.Equal(t, int64(0), count, "Transaction should have been rolled back, no records should exist")
	}

	// Cleanup
	t.Cleanup(func() {
		cleanupPolicyMetadata(t, schemaID)
	})
}

// TestPolicy_CreateMetadata_UniqueConstraintViolation tests that duplicate records
// are handled correctly (should update existing, not create duplicate).
// This tests the unique constraint (schema_id, field_name) behavior with PostgreSQL.
func TestPolicy_CreateMetadata_UniqueConstraintViolation(t *testing.T) {
	schemaID := generateTestID("test-schema-unique")
	fieldName := "person.fullName"

	// Create initial record
	createReq := PolicyMetadataCreateRequest{
		SchemaID: schemaID,
		Records: []PolicyMetadataCreateRequestRecord{
			{
				FieldName:   fieldName,
				Source:      "primary",
				PolicyClass: "allowlist",
			},
		},
	}

	createBody, err := json.Marshal(createReq)
	require.NoError(t, err)

	resp1, err := http.Post(pdpBaseURL+"/api/v1/policy/metadata", "application/json", bytes.NewBuffer(createBody))
	require.NoError(t, err)
	defer resp1.Body.Close()
	assert.Equal(t, http.StatusCreated, resp1.StatusCode)

	var createResponse1 PolicyMetadataCreateResponse
	err = json.NewDecoder(resp1.Body).Decode(&createResponse1)
	require.NoError(t, err)
	require.Len(t, createResponse1.Records, 1)
	originalID := createResponse1.Records[0].ID

	// Create same record again - should update, not create duplicate
	displayName := "Updated Name"
	updateReq := PolicyMetadataCreateRequest{
		SchemaID: schemaID,
		Records: []PolicyMetadataCreateRequestRecord{
			{
				FieldName:   fieldName,
				DisplayName: &displayName,
				Source:      "primary",
				PolicyClass: "allowlist_consent", // Changed
			},
		},
	}

	updateBody, err := json.Marshal(updateReq)
	require.NoError(t, err)

	resp2, err := http.Post(pdpBaseURL+"/api/v1/policy/metadata", "application/json", bytes.NewBuffer(updateBody))
	require.NoError(t, err)
	defer resp2.Body.Close()
	assert.Equal(t, http.StatusCreated, resp2.StatusCode)

	var createResponse2 PolicyMetadataCreateResponse
	err = json.NewDecoder(resp2.Body).Decode(&createResponse2)
	require.NoError(t, err)
	require.Len(t, createResponse2.Records, 1)

	// Verify it's the same record (same ID) but with updated values
	assert.Equal(t, originalID, createResponse2.Records[0].ID, "Should update existing record, not create duplicate")
	assert.Equal(t, "Updated Name", *createResponse2.Records[0].DisplayName)
	assert.Equal(t, "allowlist_consent", createResponse2.Records[0].PolicyClass)

	// Verify only one record exists (no duplicates) by checking database directly
	cleanup := testutils.WithTestDBEnv(t, testutils.DBConfig{
		Port:     "5433",
		Database: "policy_db",
		Password: "password",
	})
	defer cleanup()

	db := testutils.SetupPDPDB(t)
	if db != nil {
		var count int64
		db.Table("policy_metadata").Where("schema_id = ? AND field_name = ?", schemaID, fieldName).Count(&count)
		assert.Equal(t, int64(1), count, "Should have exactly one record, no duplicates")
	}

	// Cleanup
	t.Cleanup(func() {
		cleanupPolicyMetadata(t, schemaID)
	})
}

// TestPolicy_CreateAccessGrants_FieldNotFound tests error handling when
// trying to grant access to a field that doesn't exist.
func TestPolicy_CreateAccessGrants_FieldNotFound(t *testing.T) {
	req := AccessGrantCreateRequest{
		ApplicationID: "app-123",
		Records: []AccessGrantCreateRequestRecord{
			{
				FieldName: "nonexistent-field",
				SchemaID:  "nonexistent-schema",
			},
		},
		GrantDuration: "30d",
	}

	reqBody, err := json.Marshal(req)
	require.NoError(t, err)

	resp, err := http.Post(pdpBaseURL+"/api/v1/policy/grants", "application/json", bytes.NewBuffer(reqBody))
	require.NoError(t, err)
	defer resp.Body.Close()

	// Should return not found for a field without policy metadata
	assert.Equal(t, http.StatusNotFound, resp.StatusCode, "Should return not found for field not found")
}

// TestPolicy_GetPolicyDecision_FieldNotFound tests error handling when
// trying to get policy decision for a field that doesn't exist.
func TestPolicy_GetPolicyDecision_FieldNotFound(t *testing.T) {
	req := PolicyDecisionRequest{
		ApplicationID: "app-123",
		RequiredFields: []PolicyDecisionRequestRecord{
			{
				FieldName: "nonexistent-field",
				SchemaID:  "nonexistent-schema",
			},
		},
	}

	reqBody, err := json.Marshal(req)
	require.NoError(t, err)

	resp, err := http.Post(pdpBaseURL+"/api/v1/policy/decide", "application/json", bytes.NewBuffer(reqBody))
	require.NoError(t, err)
	defer resp.Body.Close()

	// Should return error for field not found
	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode, "Should return error for field not found")
}

// TestPolicy_CreateAccessGrants_InvalidGrantDuration tests error handling
// for invalid grant duration values.
func TestPolicy_CreateAccessGrants_InvalidGrantDuration(t *testing.T) {
	schemaID := generateTestID("test-schema-invalid-duration")

	// Create policy metadata first
	createReq := PolicyMetadataCreateRequest{
		SchemaID: schemaID,
		Records: []PolicyMetadataCreateRequestRecord{
			{
				FieldName:   "field1",
				Source:      "primary",
				PolicyClass: "allowlist",
			},
		},
	}

	createBody, err := json.Marshal(createReq)
	require.NoError(t, err)

	resp, err := http.Post(pdpBaseURL+"/api/v1/policy/metadata", "application/json", bytes.NewBuffer(createBody))
	require.NoError(t, err)
	resp.Body.Close()
	assert.Equal(t, http.StatusCreated, resp.StatusCode)

	// Try to grant with invalid grant duration
	updateReq := AccessGrantCreateRequest{
		ApplicationID: "app-123",
		GrantDuration: "invalid-duration",
		Records: []AccessGrantCreateRequestRecord{
			{
				FieldName: "field1",
				SchemaID:  schemaID,
			},
		},
	}

	updateBody, err := json.Marshal(updateReq)
	require.NoError(t, err)

	updateResp, err := http.Post(pdpBaseURL+"/api/v1/policy/grants", "application/json", bytes.NewBuffer(updateBody))
	require.NoError(t, err)
	defer updateResp.Body.Close()

	// Should reject an invalid grant duration
	assert.Equal(t, http.StatusBadRequest, updateResp.StatusCode, "Should return bad request for invalid grant duration")

	// Cleanup
	t.Cleanup(func() {
		cleanupPolicyMetadata(t, schemaID)
	})
}

// TestPolicy_GetPolicyDecision_ExpiredAccess tests that expired access
// is properly detected. This requires real database to test time-based expiration.
func TestPolicy_GetPolicyDecision_ExpiredAccess(t *testing.T) {
	schemaID := generateTestID("test-schema-expired")
	fieldName := "person.fullName"
	appID := generateTestID("test-app-expired")

	// Create policy metadata
	createReq := PolicyMetadataCreateRequest{
		SchemaID: schemaID,
		Records: []PolicyMetadataCreateRequestRecord{
			{
				FieldName:   fieldName,
				Source:      "primary",
				PolicyClass: "allowlist",
			},
		},
	}

	createBody, err := json.Marshal(createReq)
	require.NoError(t, err)

	resp, err := http.Post(pdpBaseURL+"/api/v1/policy/metadata", "application/json", bytes.NewBuffer(createBody))
	require.NoError(t, err)
	resp.Body.Close()
	assert.Equal(t, http.StatusCreated, resp.StatusCode)

	// Grant access
	updateReq := AccessGrantCreateRequest{
		ApplicationID: appID,
		GrantDuration: "30d",
		Records: []AccessGrantCreateRequestRecord{
			{
				FieldName: fieldName,
				SchemaID:  schemaID,
			},
		},
	}

	updateBody, err := json.Marshal(updateReq)
	require.NoError(t, err)

	updateResp, err := http.Post(pdpBaseURL+"/api/v1/policy/grants", "application/json", bytes.NewBuffer(updateBody))
	require.NoError(t, err)
	updateResp.Body.Close()
	assert.Equal(t, http.StatusOK, updateResp.StatusCode)

	// Manually expire the grant via database (simulating expired access)
	cleanup := testutils.WithTestDBEnv(t, testutils.DBConfig{
		Port:     "5433",
		Database: "policy_db",
		Password: "password",
	})
	defer cleanup()

	db := testutils.SetupPDPDB(t)
	if db == nil {
		t.Skip("Database connection not available for manual expiration test")
		return
	}

	// Expire the grant (yesterday)
	result := db.Table("access_grants").
		Where("schema_id = ? AND field_name = ? AND application_id = ? AND status = 'active'", schemaID, fieldName, appID).
		Update("expires_at", time.Now().AddDate(0, 0, -1))
	require.NoError(t, result.Error)
	require.Equal(t, int64(1), result.RowsAffected)

	// Test policy decision with expired access
	decisionReq := PolicyDecisionRequest{
		ApplicationID: appID,
		RequiredFields: []PolicyDecisionRequestRecord{
			{
				FieldName: fieldName,
				SchemaID:  schemaID,
			},
		},
	}

	decisionBody, err := json.Marshal(decisionReq)
	require.NoError(t, err)

	decisionResp, err := http.Post(pdpBaseURL+"/api/v1/policy/decide", "application/json", bytes.NewBuffer(decisionBody))
	require.NoError(t, err)
	defer decisionResp.Body.Close()

	assert.Equal(t, http.StatusOK, decisionResp.StatusCode, "Should return 200 even with expired access")

	var decisionResponse PolicyDecisionResponse
	err = json.NewDecoder(decisionResp.Body).Decode(&decisionResponse)
	require.NoError(t, err)

	assert.True(t, decisionResponse.AppAccessExpired, "Access should be expired")
	assert.Equal(t, 1, len(decisionResponse.ExpiredFields), "Should have one expired field")

	// Cleanup
	t.Cleanup(func() {
		cleanupPolicyMetadata(t, schemaID)
	})
}
