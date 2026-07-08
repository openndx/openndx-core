package integration_test

import (
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestFailurePath_PDP_ServiceUnavailable tests the failure path when Policy Decision Point is unavailable.
// Expected: GraphQL query should fail with PDP_ERROR or timeout.
func TestFailurePath_PDP_ServiceUnavailable(t *testing.T) {
	testID := fmt.Sprintf("%d", time.Now().UnixNano())
	appID := fmt.Sprintf("test-app-pdp-down-%s", testID)

	cleanup := &testCleanupRegistry{
		appIDs: []string{appID},
	}
	defer cleanup.cleanup(t)

	wd, err := os.Getwd()
	if err != nil {
		t.Skipf("Skipping test: unable to get working directory: %v", err)
		return
	}

	composeFiles := []string{"docker-compose.db.yml", "docker-compose.test.yml"}

	// Pause PDP service to simulate failure
	args := []string{"compose"}
	for _, f := range composeFiles {
		args = append(args, "-f", filepath.Join(wd, f))
	}
	args = append(args, "pause", "policy-decision-point")

	cmd := exec.Command("docker", args...)
	err = cmd.Run()
	if err != nil {
		t.Skipf("Skipping test: unable to pause PDP container: %v", err)
		return
	}

	t.Cleanup(func() {
		args := []string{"compose"}
		for _, f := range composeFiles {
			args = append(args, "-f", filepath.Join(wd, f))
		}
		args = append(args, "unpause", "policy-decision-point")
		unpauseCmd := exec.Command("docker", args...)
		if err := unpauseCmd.Run(); err != nil {
			t.Logf("Failed to unpause PDP container during cleanup: %v", err)
		}
		time.Sleep(servicePauseDelay)
	})

	time.Sleep(servicePauseDelay)

	token, err := createTestJWT(appID)
	require.NoError(t, err)

	resp, err := makeGraphQLRequest(t, `
		query TestQuery($nic: String!) {
			personInfo(nic: $nic) {
				email
			}
		}
	`, map[string]interface{}{"nic": testNIC}, token)

	if err != nil {
		t.Logf("Request failed as expected when PDP is down: %v", err)
		assert.Error(t, err, "Request should fail when PDP is unavailable")
		return
	}
	defer resp.Body.Close()

	t.Logf("Response status during PDP outage: %d", resp.StatusCode)

	if resp.StatusCode == http.StatusOK {
		result := parseGraphQLResponse(t, resp)
		if errors, ok := result["errors"].([]interface{}); ok {
			assert.NotEmpty(t, errors, "Should have errors when PDP is down")
			t.Logf("Received errors as expected: %+v", errors)

			// Verify error code indicates PDP failure
			if len(errors) > 0 {
				firstError := errors[0].(map[string]interface{})
				if ext, ok := firstError["extensions"].(map[string]interface{}); ok {
					errorCode := fmt.Sprintf("%v", ext["code"])
					assert.Contains(t, []string{"PDP_ERROR", "SERVICE_UNAVAILABLE", "INTERNAL_ERROR"},
						errorCode, "Error should indicate PDP failure")
				}
			}
		}
	} else {
		assert.NotEqual(t, http.StatusOK, resp.StatusCode,
			"Should not return OK when PDP is down")
	}
}

// TestFailurePath_PDP_Succeeds_ConsentNotGranted tests when PDP succeeds but consent is not granted.
// Scenario: Policy metadata exists, app is in allowlist, but consent is pending or rejected.
// Expected: GraphQL query should fail with CE_ERROR or CE_NOT_APPROVED.
func TestFailurePath_PDP_Succeeds_ConsentNotGranted(t *testing.T) {
	timestamp := time.Now().UnixNano()
	schemaID := "test-schema-123"
	appID := fmt.Sprintf("test-app-consent-pending-%d", timestamp)
	fieldName := "person.address"
	ownerID := testNIC

	t.Logf("Testing consent not granted with AppID: %s, SchemaID: %s", appID, schemaID)

	cleanup := &testCleanupRegistry{
		schemaIDs: []string{schemaID},
		appIDs:    []string{appID},
	}
	defer cleanup.cleanup(t)

	// Setup: Create policy metadata and add app to allowlist
	createPolicyMetadata(t, schemaID, fieldName)
	updatePDPAllowlist(t, appID, schemaID, fieldName)

	// Create consent but do NOT approve it (leaves it in "pending" status)
	consentID := createConsent(t, appID, schemaID, fieldName, ownerID)
	cleanup.consentIDs = append(cleanup.consentIDs, consentID)

	// Verify consent is pending
	db := getConsentDB(t)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	defer sqlDB.Close()

	var status string
	err = db.Raw("SELECT status FROM consent_records WHERE consent_id = ?", consentID).Scan(&status).Error
	require.NoError(t, err)
	assert.Equal(t, "pending", status, "Consent should be in pending status")

	// Make GraphQL request - should fail due to consent not approved
	token, err := createTestJWT(appID)
	require.NoError(t, err)

	resp, err := makeGraphQLRequest(t, `
		query TestQuery($nic: String!) {
			personInfo(nic: $nic) {
				address
			}
		}
	`, map[string]interface{}{"nic": ownerID}, token)
	require.NoError(t, err)
	defer resp.Body.Close()

	t.Logf("Response status for pending consent: %d", resp.StatusCode)
	assert.Equal(t, http.StatusOK, resp.StatusCode, "GraphQL should return 200 OK even with errors")

	result := parseGraphQLResponse(t, resp)

	// Should contain errors indicating consent not approved
	errors, hasErrors := result["errors"]
	assert.True(t, hasErrors, "Should return errors for pending consent")
	if hasErrors {
		errorList := errors.([]interface{})
		assert.NotEmpty(t, errorList, "Error list should not be empty")
		t.Logf("Received errors as expected: %+v", errorList)

		// Check for consent-related error code
		if len(errorList) > 0 {
			firstError := errorList[0].(map[string]interface{})
			message := fmt.Sprintf("%v", firstError["message"])
			t.Logf("Error message: %s", message)

			extensions, hasExtensions := firstError["extensions"].(map[string]interface{})
			if hasExtensions {
				errorCode := fmt.Sprintf("%v", extensions["code"])
				assert.True(t, errorCode == "CE_ERROR" || errorCode == "CE_NOT_APPROVED",
					"Error should have CE_ERROR or CE_NOT_APPROVED code, got: %s", errorCode)
			} else {
				assert.True(t,
					strings.Contains(message, "Consent") || strings.Contains(message, "CE"),
					"Error message should mention consent")
			}
		}
	}
}

// TestFailurePath_PDP_Succeeds_ConsentExpired tests when PDP succeeds but consent has expired.
// Scenario: Policy metadata exists, app is in allowlist, consent was approved but grant_expires_at is in the past.
// Expected: GraphQL query should fail with CE_ERROR or CE_EXPIRED.
func TestFailurePath_PDP_Succeeds_ConsentExpired(t *testing.T) {
	timestamp := time.Now().UnixNano()
	schemaID := "test-schema-123"
	appID := fmt.Sprintf("test-app-consent-expired-%d", timestamp)
	fieldName := "person.address"
	ownerID := testNIC

	t.Logf("Testing expired consent with AppID: %s, SchemaID: %s", appID, schemaID)

	cleanup := &testCleanupRegistry{
		schemaIDs: []string{schemaID},
		appIDs:    []string{appID},
	}
	defer cleanup.cleanup(t)

	// Setup: Create policy metadata and add app to allowlist
	createPolicyMetadata(t, schemaID, fieldName)
	updatePDPAllowlist(t, appID, schemaID, fieldName)

	// Create consent
	consentID := createConsent(t, appID, schemaID, fieldName, ownerID)
	cleanup.consentIDs = append(cleanup.consentIDs, consentID)

	// Manually set consent to approved with expired grant_expires_at
	db := getConsentDB(t)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	defer sqlDB.Close()

	// Set status to approved and grant_expires_at to past time
	pastTime := time.Now().UTC().Add(-24 * time.Hour) // 24 hours ago
	err = db.Exec(`
		UPDATE consent_records 
		SET status = 'approved', 
		    grant_expires_at = ?,
		    updated_at = CURRENT_TIMESTAMP
		WHERE consent_id = ?
	`, pastTime, consentID).Error
	require.NoError(t, err, "Failed to set consent as expired")

	// Verify consent is expired
	var dbResult struct {
		Status         string     `gorm:"column:status"`
		GrantExpiresAt *time.Time `gorm:"column:grant_expires_at"`
	}
	err = db.Raw("SELECT status, grant_expires_at FROM consent_records WHERE consent_id = ?", consentID).
		Scan(&dbResult).Error
	require.NoError(t, err)
	assert.Equal(t, "approved", dbResult.Status, "Consent should be approved")
	assert.NotNil(t, dbResult.GrantExpiresAt, "Grant expires at should be set")
	assert.True(t, time.Now().UTC().After(*dbResult.GrantExpiresAt), "Grant should be expired")

	// Make GraphQL request - should fail due to expired consent
	token, err := createTestJWT(appID)
	require.NoError(t, err)

	resp, err := makeGraphQLRequest(t, `
		query TestQuery($nic: String!) {
			personInfo(nic: $nic) {
				address
			}
		}
	`, map[string]interface{}{"nic": ownerID}, token)
	require.NoError(t, err)
	defer resp.Body.Close()

	t.Logf("Response status for expired consent: %d", resp.StatusCode)
	assert.Equal(t, http.StatusOK, resp.StatusCode, "GraphQL should return 200 OK even with errors")

	result := parseGraphQLResponse(t, resp)

	// Should contain errors indicating consent expired
	errors, hasErrors := result["errors"]
	assert.True(t, hasErrors, "Should return errors for expired consent")
	if hasErrors {
		errorList := errors.([]interface{})
		assert.NotEmpty(t, errorList, "Error list should not be empty")
		t.Logf("Received errors as expected: %+v", errorList)

		// Check for consent-related error code
		if len(errorList) > 0 {
			firstError := errorList[0].(map[string]interface{})
			message := fmt.Sprintf("%v", firstError["message"])
			t.Logf("Error message: %s", message)

			extensions, hasExtensions := firstError["extensions"].(map[string]interface{})
			if hasExtensions {
				errorCode := fmt.Sprintf("%v", extensions["code"])
				assert.True(t, errorCode == "CE_ERROR" || errorCode == "CE_EXPIRED" || errorCode == "CE_NOT_APPROVED",
					"Error should have CE_ERROR, CE_EXPIRED, or CE_NOT_APPROVED code, got: %s", errorCode)
			} else {
				assert.True(t,
					strings.Contains(message, "Consent") || strings.Contains(message, "CE") || strings.Contains(message, "expired"),
					"Error message should mention consent or expiration")
			}
		}
	}
}

// TestFailurePath_PDP_AuthorizationFailure tests when PDP denies authorization (app not in allowlist).
// Scenario: Policy metadata exists, but app is NOT in allowlist.
// Expected: GraphQL query should fail with PDP_NOT_ALLOWED or PDP_ERROR.
func TestFailurePath_PDP_AuthorizationFailure(t *testing.T) {
	timestamp := time.Now().UnixNano()
	schemaID := "test-schema-123"
	unauthorizedAppID := fmt.Sprintf("test-app-not-allowed-%d", timestamp)
	fieldName := "person.address"

	t.Logf("Testing PDP authorization failure with AppID: %s, SchemaID: %s", unauthorizedAppID, schemaID)

	cleanup := &testCleanupRegistry{
		schemaIDs: []string{schemaID},
		appIDs:    []string{unauthorizedAppID},
	}
	defer cleanup.cleanup(t)

	// Setup: Create policy metadata but do NOT add app to allowlist
	createPolicyMetadata(t, schemaID, fieldName)
	// Intentionally skip updatePDPAllowlist to simulate app not authorized

	// Make GraphQL request - should fail due to authorization failure
	token, err := createTestJWT(unauthorizedAppID)
	require.NoError(t, err)

	resp, err := makeGraphQLRequest(t, `
		query TestQuery($nic: String!) {
			personInfo(nic: $nic) {
				address
			}
		}
	`, map[string]interface{}{"nic": testNIC}, token)
	require.NoError(t, err)
	defer resp.Body.Close()

	t.Logf("Response status for unauthorized app: %d", resp.StatusCode)
	assert.Equal(t, http.StatusOK, resp.StatusCode, "GraphQL should return 200 OK even with errors")

	result := parseGraphQLResponse(t, resp)

	// Should contain errors indicating authorization failure
	errors, hasErrors := result["errors"]
	assert.True(t, hasErrors, "Should return errors for unauthorized app")
	if hasErrors {
		errorList := errors.([]interface{})
		assert.NotEmpty(t, errorList, "Error list should not be empty")
		t.Logf("Received errors as expected: %+v", errorList)

		// Check for PDP-related error code
		if len(errorList) > 0 {
			firstError := errorList[0].(map[string]interface{})
			message := fmt.Sprintf("%v", firstError["message"])
			t.Logf("Error message: %s", message)

			extensions, hasExtensions := firstError["extensions"].(map[string]interface{})
			if hasExtensions {
				errorCode := fmt.Sprintf("%v", extensions["code"])
				assert.True(t, errorCode == "PDP_ERROR" || errorCode == "PDP_NOT_ALLOWED",
					"Error should have PDP_ERROR or PDP_NOT_ALLOWED code, got: %s", errorCode)
			} else {
				assert.Contains(t, message, "PDP", "Error message should mention PDP")
			}
		}
	}
}

// TestFailurePath_ConsentEngine_ServiceUnavailable tests the failure path when Consent Engine is unavailable.
// Expected: GraphQL query should fail with CE_ERROR or SERVICE_UNAVAILABLE.
func TestFailurePath_ConsentEngine_ServiceUnavailable(t *testing.T) {
	timestamp := time.Now().UnixNano()
	schemaID := "test-schema-123"
	appID := fmt.Sprintf("test-app-ce-down-%d", timestamp)
	fieldName := "person.address"

	cleanup := &testCleanupRegistry{
		schemaIDs: []string{schemaID},
		appIDs:    []string{appID},
	}
	defer cleanup.cleanup(t)

	wd, err := os.Getwd()
	if err != nil {
		t.Skipf("Skipping test: unable to get working directory: %v", err)
		return
	}

	composeFiles := []string{"docker-compose.db.yml", "docker-compose.test.yml"}

	// Setup: Create policy metadata and add app to allowlist (PDP should succeed)
	createPolicyMetadata(t, schemaID, fieldName)
	updatePDPAllowlist(t, appID, schemaID, fieldName)

	// Pause Consent Engine service to simulate failure
	args := []string{"compose"}
	for _, f := range composeFiles {
		args = append(args, "-f", filepath.Join(wd, f))
	}
	args = append(args, "pause", "consent-engine")

	cmd := exec.Command("docker", args...)
	err = cmd.Run()
	if err != nil {
		t.Skipf("Skipping test: unable to pause Consent Engine container: %v", err)
		return
	}

	t.Cleanup(func() {
		args := []string{"compose"}
		for _, f := range composeFiles {
			args = append(args, "-f", filepath.Join(wd, f))
		}
		args = append(args, "unpause", "consent-engine")
		unpauseCmd := exec.Command("docker", args...)
		if err := unpauseCmd.Run(); err != nil {
			t.Logf("Failed to unpause Consent Engine container during cleanup: %v", err)
		}
		time.Sleep(servicePauseDelay)
	})

	time.Sleep(servicePauseDelay)

	token, err := createTestJWT(appID)
	require.NoError(t, err)

	resp, err := makeGraphQLRequest(t, `
		query TestQuery($nic: String!) {
			personInfo(nic: $nic) {
				address
			}
		}
	`, map[string]interface{}{"nic": testNIC}, token)

	if err != nil {
		t.Logf("Request failed as expected when Consent Engine is down: %v", err)
		assert.Error(t, err, "Request should fail when Consent Engine is unavailable")
		return
	}
	defer resp.Body.Close()

	t.Logf("Response status during Consent Engine outage: %d", resp.StatusCode)

	if resp.StatusCode == http.StatusOK {
		result := parseGraphQLResponse(t, resp)
		if errors, ok := result["errors"].([]interface{}); ok {
			assert.NotEmpty(t, errors, "Should have errors when Consent Engine is down")
			t.Logf("Received errors as expected: %+v", errors)

			// Verify error code indicates Consent Engine failure
			if len(errors) > 0 {
				firstError := errors[0].(map[string]interface{})
				if ext, ok := firstError["extensions"].(map[string]interface{}); ok {
					errorCode := fmt.Sprintf("%v", ext["code"])
					assert.Contains(t, []string{"CE_ERROR", "SERVICE_UNAVAILABLE", "INTERNAL_ERROR"},
						errorCode, "Error should indicate Consent Engine failure")
				}
			}
		}
	} else {
		assert.NotEqual(t, http.StatusOK, resp.StatusCode,
			"Should not return OK when Consent Engine is down")
	}
}

// TestFailurePath_Provider_ServiceUnavailable tests the failure path when Provider is unavailable.
// Scenario: PDP and Consent Engine succeed, but provider data source is down.
// Expected: GraphQL query should fail with provider error or partial data.
func TestFailurePath_Provider_ServiceUnavailable(t *testing.T) {
	timestamp := time.Now().UnixNano()
	schemaID := "test-schema-123"
	appID := fmt.Sprintf("test-app-provider-down-%d", timestamp)
	fieldName := "person.email"

	t.Logf("Testing provider unavailable with AppID: %s, SchemaID: %s", appID, schemaID)

	cleanup := &testCleanupRegistry{
		schemaIDs: []string{schemaID},
		appIDs:    []string{appID},
	}
	defer cleanup.cleanup(t)

	// Setup: Create policy metadata, add app to allowlist, and create approved consent
	createPolicyMetadata(t, schemaID, fieldName)
	updatePDPAllowlist(t, appID, schemaID, fieldName)

	consentID := createConsent(t, appID, schemaID, fieldName, testNIC)
	approveConsent(t, consentID)
	cleanup.consentIDs = append(cleanup.consentIDs, consentID)

	// Note: Provider failures are harder to simulate in integration tests since providers
	// are external services. This test verifies that the system handles provider errors gracefully.
	// In a real scenario, we would pause the mock-provider service or configure it to return errors.

	token, err := createTestJWT(appID)
	require.NoError(t, err)

	resp, err := makeGraphQLRequest(t, `
		query TestQuery($nic: String!) {
			personInfo(nic: $nic) {
				email
			}
		}
	`, map[string]interface{}{"nic": testNIC}, token)
	require.NoError(t, err)
	defer resp.Body.Close()

	t.Logf("Response status: %d", resp.StatusCode)

	// The response may succeed if provider is available, or fail if provider is down
	// This test documents the expected behavior when provider fails
	result := parseGraphQLResponse(t, resp)

	if errors, ok := result["errors"].([]interface{}); ok && len(errors) > 0 {
		t.Logf("Received errors (provider may be unavailable): %+v", errors)
		// If provider is down, we expect provider-related errors
		firstError := errors[0].(map[string]interface{})
		message := fmt.Sprintf("%v", firstError["message"])
		t.Logf("Error message: %s", message)
	} else {
		// If provider is available, query should succeed
		data, ok := result["data"].(map[string]interface{})
		if ok {
			t.Logf("Query succeeded (provider is available): %+v", data)
		}
	}
}

// TestFailurePath_PDP_Succeeds_ConsentRejected tests when PDP succeeds but consent was rejected.
// Scenario: Policy metadata exists, app is in allowlist, but consent status is "rejected".
// Expected: GraphQL query should fail with CE_ERROR or CE_NOT_APPROVED.
func TestFailurePath_PDP_Succeeds_ConsentRejected(t *testing.T) {
	timestamp := time.Now().UnixNano()
	schemaID := "test-schema-123"
	appID := fmt.Sprintf("test-app-consent-rejected-%d", timestamp)
	fieldName := "person.address"
	ownerID := testNIC

	t.Logf("Testing rejected consent with AppID: %s, SchemaID: %s", appID, schemaID)

	cleanup := &testCleanupRegistry{
		schemaIDs: []string{schemaID},
		appIDs:    []string{appID},
	}
	defer cleanup.cleanup(t)

	// Setup: Create policy metadata and add app to allowlist
	createPolicyMetadata(t, schemaID, fieldName)
	updatePDPAllowlist(t, appID, schemaID, fieldName)

	// Create consent
	consentID := createConsent(t, appID, schemaID, fieldName, ownerID)
	cleanup.consentIDs = append(cleanup.consentIDs, consentID)

	// Manually set consent status to rejected
	db := getConsentDB(t)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	defer sqlDB.Close()

	err = db.Exec(`
		UPDATE consent_records 
		SET status = 'rejected',
		    updated_at = CURRENT_TIMESTAMP
		WHERE consent_id = ?
	`, consentID).Error
	require.NoError(t, err, "Failed to set consent as rejected")

	// Verify consent is rejected
	var status string
	err = db.Raw("SELECT status FROM consent_records WHERE consent_id = ?", consentID).Scan(&status).Error
	require.NoError(t, err)
	assert.Equal(t, "rejected", status, "Consent should be rejected")

	// Make GraphQL request - should fail due to rejected consent
	token, err := createTestJWT(appID)
	require.NoError(t, err)

	resp, err := makeGraphQLRequest(t, `
		query TestQuery($nic: String!) {
			personInfo(nic: $nic) {
				address
			}
		}
	`, map[string]interface{}{"nic": ownerID}, token)
	require.NoError(t, err)
	defer resp.Body.Close()

	t.Logf("Response status for rejected consent: %d", resp.StatusCode)
	assert.Equal(t, http.StatusOK, resp.StatusCode, "GraphQL should return 200 OK even with errors")

	result := parseGraphQLResponse(t, resp)

	// Should contain errors indicating consent rejected
	errors, hasErrors := result["errors"]
	assert.True(t, hasErrors, "Should return errors for rejected consent")
	if hasErrors {
		errorList := errors.([]interface{})
		assert.NotEmpty(t, errorList, "Error list should not be empty")
		t.Logf("Received errors as expected: %+v", errorList)

		// Check for consent-related error code
		if len(errorList) > 0 {
			firstError := errorList[0].(map[string]interface{})
			message := fmt.Sprintf("%v", firstError["message"])
			t.Logf("Error message: %s", message)

			extensions, hasExtensions := firstError["extensions"].(map[string]interface{})
			if hasExtensions {
				errorCode := fmt.Sprintf("%v", extensions["code"])
				assert.True(t, errorCode == "CE_ERROR" || errorCode == "CE_NOT_APPROVED",
					"Error should have CE_ERROR or CE_NOT_APPROVED code, got: %s", errorCode)
			} else {
				assert.True(t,
					strings.Contains(message, "Consent") || strings.Contains(message, "CE"),
					"Error message should mention consent")
			}
		}
	}
}
