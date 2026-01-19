package policy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/gov-dx-sandbox/tests/integration/testutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPolicy_CreateMetadata_TransactionRollback tests that invalid requests
// are properly rejected and don't leave partial data in the database.
// This requires a real PostgreSQL database to test transaction rollback behavior.
func TestPolicy_CreateMetadata_TransactionRollback(t *testing.T) {
	schemaID := generateTestID("test-schema-rollback")

	// Create a request that should fail validation (isOwner false but no owner specified)
	req := PolicyMetadataCreateRequest{
		SchemaID: schemaID,
		Records: []PolicyMetadataCreateRequestRecord{
			{
				FieldName:         "field1",
				Source:            "primary",
				IsOwner:           false, // Invalid: isOwner false but no owner specified
				AccessControlType: "public",
			},
		},
	}

	reqBody, err := json.Marshal(req)
	require.NoError(t, err)

	resp, err := http.Post(pdpBaseURL+"/api/v1/policy/metadata", "application/json", bytes.NewBuffer(reqBody))
	require.NoError(t, err)
	defer resp.Body.Close()

	// Should return error due to validation failure
	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode, "Should reject invalid request")

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
				FieldName:         fieldName,
				Source:            "primary",
				IsOwner:           true,
				AccessControlType: "public",
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
				FieldName:         fieldName,
				DisplayName:       &displayName,
				Source:            "primary",
				IsOwner:           true,
				AccessControlType: "restricted", // Changed
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
	assert.Equal(t, "restricted", createResponse2.Records[0].AccessControlType)

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

// TestPolicy_UpdateAllowList_FieldNotFound tests error handling when
// trying to update allow list for a field that doesn't exist.
func TestPolicy_UpdateAllowList_FieldNotFound(t *testing.T) {
	req := AllowListUpdateRequest{
		ApplicationID: "app-123",
		Records: []AllowListUpdateRequestRecord{
			{
				FieldName: "nonexistent-field",
				SchemaID:  "nonexistent-schema",
			},
		},
		GrantDuration: "30d",
	}

	reqBody, err := json.Marshal(req)
	require.NoError(t, err)

	resp, err := http.Post(pdpBaseURL+"/api/v1/policy/update-allowlist", "application/json", bytes.NewBuffer(reqBody))
	require.NoError(t, err)
	defer resp.Body.Close()

	// Should return error for field not found
	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode, "Should return error for field not found")
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

// TestPolicy_UpdateAllowList_InvalidGrantDuration tests error handling
// for invalid grant duration values.
func TestPolicy_UpdateAllowList_InvalidGrantDuration(t *testing.T) {
	schemaID := generateTestID("test-schema-invalid-duration")

	// Create policy metadata first
	createReq := PolicyMetadataCreateRequest{
		SchemaID: schemaID,
		Records: []PolicyMetadataCreateRequestRecord{
			{
				FieldName:         "field1",
				Source:            "primary",
				IsOwner:           true,
				AccessControlType: "public",
			},
		},
	}

	createBody, err := json.Marshal(createReq)
	require.NoError(t, err)

	resp, err := http.Post(pdpBaseURL+"/api/v1/policy/metadata", "application/json", bytes.NewBuffer(createBody))
	require.NoError(t, err)
	resp.Body.Close()
	assert.Equal(t, http.StatusCreated, resp.StatusCode)

	// Try to update with invalid grant duration
	updateReq := AllowListUpdateRequest{
		ApplicationID: "app-123",
		GrantDuration: "invalid-duration",
		Records: []AllowListUpdateRequestRecord{
			{
				FieldName: "field1",
				SchemaID:  schemaID,
			},
		},
	}

	updateBody, err := json.Marshal(updateReq)
	require.NoError(t, err)

	updateResp, err := http.Post(pdpBaseURL+"/api/v1/policy/update-allowlist", "application/json", bytes.NewBuffer(updateBody))
	require.NoError(t, err)
	defer updateResp.Body.Close()

	// Should return error for invalid grant duration
	assert.Equal(t, http.StatusInternalServerError, updateResp.StatusCode, "Should return error for invalid grant duration")

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
				FieldName:         fieldName,
				Source:            "primary",
				IsOwner:           true,
				AccessControlType: "public",
			},
		},
	}

	createBody, err := json.Marshal(createReq)
	require.NoError(t, err)

	resp, err := http.Post(pdpBaseURL+"/api/v1/policy/metadata", "application/json", bytes.NewBuffer(createBody))
	require.NoError(t, err)
	resp.Body.Close()
	assert.Equal(t, http.StatusCreated, resp.StatusCode)

	// Update allow list
	updateReq := AllowListUpdateRequest{
		ApplicationID: appID,
		GrantDuration: "30d",
		Records: []AllowListUpdateRequestRecord{
			{
				FieldName: fieldName,
				SchemaID:  schemaID,
			},
		},
	}

	updateBody, err := json.Marshal(updateReq)
	require.NoError(t, err)

	updateResp, err := http.Post(pdpBaseURL+"/api/v1/policy/update-allowlist", "application/json", bytes.NewBuffer(updateBody))
	require.NoError(t, err)
	updateResp.Body.Close()
	assert.Equal(t, http.StatusOK, updateResp.StatusCode)

	// Manually set expired allow list entry via database (simulating expired access)
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

	// Get the record and manually expire it
	var result struct {
		AllowList string `gorm:"column:allow_list"`
	}
	err = db.Table("policy_metadata").
		Where("schema_id = ? AND field_name = ?", schemaID, fieldName).
		Select("allow_list").
		Scan(&result).Error
	require.NoError(t, err)

	// Update with expired timestamp (yesterday)
	expiredTime := time.Now().AddDate(0, 0, -1).Format(time.RFC3339)
	expiredJSON := fmt.Sprintf(`{"%s":{"expires_at":"%s","updated_at":"%s"}}`, appID, expiredTime, time.Now().Format(time.RFC3339))
	err = db.Table("policy_metadata").
		Where("schema_id = ? AND field_name = ?", schemaID, fieldName).
		Update("allow_list", expiredJSON).Error
	require.NoError(t, err)

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
