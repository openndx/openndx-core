package handler

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

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

// TestRespondWithPolicyServiceError_HidesInternalDetails verifies that
// unexpected service errors are logged without exposing details to clients.
func TestRespondWithPolicyServiceError_HidesInternalDetails(t *testing.T) {
	w := httptest.NewRecorder()
	internalErr := errors.New("database connection failed: password=secret")

	respondWithPolicyServiceError(w, internalErr)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.JSONEq(t, `{"error":"internal server error"}`, w.Body.String())
	assert.NotContains(t, w.Body.String(), internalErr.Error())
}

func TestHandler_CreatePolicyMetadata_InvalidJSON(t *testing.T) {
	// This test doesn't need a database - it only tests JSON parsing
	db := setupTestDB(t)
	handler := NewHandler(db)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/policy/metadata", bytes.NewBufferString("invalid json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.CreatePolicyMetadata(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandler_CreateAccessGrants_InvalidJSON(t *testing.T) {
	// This test doesn't need a database - it only tests JSON parsing
	db := setupTestDB(t)
	handler := NewHandler(db)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/policy/grants", bytes.NewBufferString("invalid json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.CreateAccessGrants(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandler_GetPolicyDecision_InvalidJSON(t *testing.T) {
	// This test doesn't need a database - it only tests JSON parsing
	db := setupTestDB(t)
	handler := NewHandler(db)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/policy/decide", bytes.NewBufferString("invalid json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.GetPolicyDecision(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandler_NewHandler(t *testing.T) {
	db := setupTestDB(t)
	handler := NewHandler(db)
	assert.NotNil(t, handler)
	assert.NotNil(t, handler.policyService)
}

func TestHandler_CreatePolicyMetadata(t *testing.T) {
	tests := []struct {
		name           string
		requestBody    models.PolicyMetadataCreateRequest
		expectedStatus int
		validateFunc   func(t *testing.T, response *httptest.ResponseRecorder)
	}{
		{
			name: "Create new policy metadata successfully",
			requestBody: models.PolicyMetadataCreateRequest{
				SchemaID: "schema-123",
				Records: []models.PolicyMetadataCreateRequestRecord{
					{
						FieldName:         "person.fullName",
						DisplayName:       testhelpers.StringPtr("Full Name"),
						Description:       testhelpers.StringPtr("Complete name"),
						Source:            models.SourcePrimary,
						RequiresAllowlist: new(true), RequiresConsent: new(false),
					},
				},
			},
			expectedStatus: http.StatusCreated,
			validateFunc: func(t *testing.T, w *httptest.ResponseRecorder) {
				var resp models.PolicyMetadataCreateResponse
				if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
					t.Fatalf("Failed to decode response: %v", err)
				}
				if len(resp.Records) != 1 {
					t.Errorf("Expected 1 record, got %d", len(resp.Records))
				}
				if resp.Records[0].FieldName != "person.fullName" {
					t.Errorf("Expected fieldName person.fullName, got %s", resp.Records[0].FieldName)
				}
			},
		},
		{
			name: "Update existing policy metadata",
			requestBody: models.PolicyMetadataCreateRequest{
				SchemaID: "schema-123",
				Records: []models.PolicyMetadataCreateRequestRecord{
					{
						FieldName:         "person.fullName",
						DisplayName:       testhelpers.StringPtr("Full Name Updated"),
						Description:       testhelpers.StringPtr("Updated description"),
						Source:            models.SourcePrimary,
						RequiresAllowlist: new(true), RequiresConsent: new(true),
					},
				},
			},
			expectedStatus: http.StatusCreated,
			validateFunc: func(t *testing.T, w *httptest.ResponseRecorder) {
				var resp models.PolicyMetadataCreateResponse
				if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
					t.Fatalf("Failed to decode response: %v", err)
				}
				if len(resp.Records) != 1 {
					t.Errorf("Expected 1 record, got %d", len(resp.Records))
				}
				assert.True(t, resp.Records[0].RequiresAllowlist)
				assert.True(t, resp.Records[0].RequiresConsent)
			},
		},
		{
			name: "Empty request body",
			requestBody: models.PolicyMetadataCreateRequest{
				SchemaID: "",
				Records:  []models.PolicyMetadataCreateRequestRecord{},
			},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name: "Missing policy flag",
			requestBody: models.PolicyMetadataCreateRequest{
				SchemaID: "schema-123",
				Records: []models.PolicyMetadataCreateRequestRecord{
					{
						FieldName:         "person.invalid",
						Source:            models.SourcePrimary,
						RequiresAllowlist: new(false),
					},
				},
			},
			expectedStatus: http.StatusBadRequest,
			validateFunc: func(t *testing.T, w *httptest.ResponseRecorder) {
				assert.Contains(t, w.Body.String(), "requiresConsent")
				assert.Contains(t, w.Body.String(), "person.invalid")
			},
		},
		{
			name: "Create with multiple records",
			requestBody: models.PolicyMetadataCreateRequest{
				SchemaID: "schema-123",
				Records: []models.PolicyMetadataCreateRequestRecord{
					{
						FieldName:         "person.fullName",
						DisplayName:       testhelpers.StringPtr("Full Name"),
						Source:            models.SourcePrimary,
						RequiresAllowlist: new(true), RequiresConsent: new(false),
					},
					{
						FieldName:         "person.email",
						DisplayName:       testhelpers.StringPtr("Email"),
						Source:            models.SourcePrimary,
						RequiresAllowlist: new(true), RequiresConsent: new(false),
					},
				},
			},
			expectedStatus: http.StatusCreated,
			validateFunc: func(t *testing.T, w *httptest.ResponseRecorder) {
				var resp models.PolicyMetadataCreateResponse
				if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
					t.Fatalf("Failed to decode response: %v", err)
				}
				if len(resp.Records) != 2 {
					t.Errorf("Expected 2 records, got %d", len(resp.Records))
				}
			},
		},
		{
			name: "Create with fallback source",
			requestBody: models.PolicyMetadataCreateRequest{
				SchemaID: "schema-123",
				Records: []models.PolicyMetadataCreateRequestRecord{
					{
						FieldName:         "person.fullName",
						Source:            models.SourceFallback,
						RequiresAllowlist: new(true), RequiresConsent: new(false),
					},
				},
			},
			expectedStatus: http.StatusCreated,
			validateFunc: func(t *testing.T, w *httptest.ResponseRecorder) {
				var resp models.PolicyMetadataCreateResponse
				if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
					t.Fatalf("Failed to decode response: %v", err)
				}
				if resp.Records[0].Source != models.SourceFallback {
					t.Errorf("Expected source fallback, got %s", resp.Records[0].Source)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset database for each test
			db := setupTestDB(t)
			handler := NewHandler(db)

			// Create initial record for update test
			if tt.name == "Update existing policy metadata" {
				initialReq := models.PolicyMetadataCreateRequest{
					SchemaID: "schema-123",
					Records: []models.PolicyMetadataCreateRequestRecord{
						{
							FieldName:         "person.fullName",
							DisplayName:       testhelpers.StringPtr("Full Name"),
							Source:            models.SourcePrimary,
							RequiresAllowlist: new(true), RequiresConsent: new(false),
						},
					},
				}
				_, err := handler.policyService.CreatePolicyMetadata(&initialReq)
				require.NoError(t, err)
			}

			body, _ := json.Marshal(tt.requestBody)
			req := httptest.NewRequest(http.MethodPost, "/api/v1/policy/metadata", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			handler.CreatePolicyMetadata(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("Expected status %d, got %d. Body: %s", tt.expectedStatus, w.Code, w.Body.String())
			}

			if tt.validateFunc != nil {
				tt.validateFunc(t, w)
			}
		})
	}
}

func TestHandler_ListPolicyMetadata(t *testing.T) {
	db := setupTestDB(t)
	handler := NewHandler(db)

	createResp, err := handler.policyService.CreatePolicyMetadata(&models.PolicyMetadataCreateRequest{
		SchemaID: "schema-123",
		Records: []models.PolicyMetadataCreateRequestRecord{
			{
				FieldName:         "person.name",
				DisplayName:       testhelpers.StringPtr("Name"),
				Source:            models.SourcePrimary,
				RequiresAllowlist: new(true), RequiresConsent: new(false),
			},
			{
				FieldName:         "person.email",
				DisplayName:       testhelpers.StringPtr("Email"),
				Source:            models.SourcePrimary,
				RequiresAllowlist: new(true), RequiresConsent: new(false),
			},
		},
	})
	require.NoError(t, err)
	require.Len(t, createResp.Records, 2)

	_, err = handler.policyService.CreatePolicyMetadata(&models.PolicyMetadataCreateRequest{
		SchemaID: "other-schema",
		Records: []models.PolicyMetadataCreateRequestRecord{
			{
				FieldName:         "other.field",
				Source:            models.SourcePrimary,
				RequiresAllowlist: new(true), RequiresConsent: new(false),
			},
		},
	})
	require.NoError(t, err)

	allReq := httptest.NewRequest(
		http.MethodGet,
		"/api/v1/policy/metadata",
		nil,
	)
	allRecorder := httptest.NewRecorder()

	handler.ListPolicyMetadata(allRecorder, allReq)

	require.Equal(t, http.StatusOK, allRecorder.Code)

	var allResp models.PolicyMetadataListResponse
	require.NoError(t, json.NewDecoder(allRecorder.Body).Decode(&allResp))
	require.Len(t, allResp.Records, 3)
	assert.Equal(t, "other-schema", allResp.Records[0].SchemaID)
	assert.Equal(t, "schema-123", allResp.Records[1].SchemaID)
	assert.Equal(t, "schema-123", allResp.Records[2].SchemaID)

	filteredReq := httptest.NewRequest(
		http.MethodGet,
		"/api/v1/policy/metadata?schemaId=schema-123",
		nil,
	)
	filteredRecorder := httptest.NewRecorder()

	handler.ListPolicyMetadata(filteredRecorder, filteredReq)

	require.Equal(t, http.StatusOK, filteredRecorder.Code)

	var filteredResp models.PolicyMetadataListResponse
	require.NoError(t, json.NewDecoder(filteredRecorder.Body).Decode(&filteredResp))
	require.Len(t, filteredResp.Records, 2)
	assert.Equal(t, "person.email", filteredResp.Records[0].FieldName)
	assert.Equal(t, "person.name", filteredResp.Records[1].FieldName)
}

func TestHandler_PatchPolicyMetadata(t *testing.T) {
	db := setupTestDB(t)
	handler := NewHandler(db)

	createResp, err := handler.policyService.CreatePolicyMetadata(&models.PolicyMetadataCreateRequest{
		SchemaID: "schema-123",
		Records: []models.PolicyMetadataCreateRequestRecord{
			{
				FieldName:         "person.name",
				DisplayName:       testhelpers.StringPtr("Name"),
				Source:            models.SourcePrimary,
				RequiresAllowlist: new(true), RequiresConsent: new(false),
			},
			{
				FieldName:         "person.email",
				DisplayName:       testhelpers.StringPtr("Email"),
				Source:            models.SourcePrimary,
				RequiresAllowlist: new(true), RequiresConsent: new(false),
			},
		},
	})
	require.NoError(t, err)
	require.Len(t, createResp.Records, 2)

	emailID := createResp.Records[1].ID

	req := httptest.NewRequest(
		http.MethodPatch,
		"/api/v1/policy/metadata/"+emailID,
		bytes.NewBufferString(`{"displayName":"Primary Email"}`),
	)
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("id", emailID)
	w := httptest.NewRecorder()

	handler.PatchPolicyMetadata(w, req)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var resp models.PolicyMetadataResponse
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	require.NotNil(t, resp.DisplayName)
	assert.Equal(t, "Primary Email", *resp.DisplayName)

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
}

func TestHandler_PatchPolicyMetadata_Flags(t *testing.T) {
	db := setupTestDB(t)
	handler := NewHandler(db)

	createResp, err := handler.policyService.CreatePolicyMetadata(&models.PolicyMetadataCreateRequest{
		SchemaID: "schema-123",
		Records: []models.PolicyMetadataCreateRequestRecord{
			{FieldName: "person.email", Source: models.SourcePrimary, RequiresAllowlist: new(true), RequiresConsent: new(false)},
		},
	})
	require.NoError(t, err)
	id := createResp.Records[0].ID

	tests := []struct {
		name        string
		body        string
		wantStatus  int
		wantConsent bool
	}{
		{"null flag", `{"requiresConsent":null}`, http.StatusBadRequest, false},
		{"flag of the wrong type", `{"requiresConsent":"yes"}`, http.StatusBadRequest, false},
		{"set a flag", `{"requiresConsent":true}`, http.StatusOK, true},
		{"removed policyClass", `{"policyClass":"allowlist_consent"}`, http.StatusBadRequest, false},
		{"removed accessControlType", `{"accessControlType":"restricted"}`, http.StatusBadRequest, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPatch, "/api/v1/policy/metadata/"+id, bytes.NewBufferString(tt.body))
			req.SetPathValue("id", id)
			w := httptest.NewRecorder()

			handler.PatchPolicyMetadata(w, req)

			require.Equal(t, tt.wantStatus, w.Code, w.Body.String())
			if tt.wantStatus == http.StatusOK {
				var resp models.PolicyMetadataResponse
				require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
				assert.True(t, resp.RequiresAllowlist, "an omitted flag is unchanged")
				assert.Equal(t, tt.wantConsent, resp.RequiresConsent)
			}
		})
	}
}

func TestHandler_DeletePolicyMetadata(t *testing.T) {
	db := setupTestDB(t)
	handler := NewHandler(db)

	createResp, err := handler.policyService.CreatePolicyMetadata(&models.PolicyMetadataCreateRequest{
		SchemaID: "schema-123",
		Records: []models.PolicyMetadataCreateRequestRecord{
			{
				FieldName:         "person.name",
				Source:            models.SourcePrimary,
				RequiresAllowlist: new(true), RequiresConsent: new(false),
			},
			{
				FieldName:         "person.email",
				Source:            models.SourcePrimary,
				RequiresAllowlist: new(true), RequiresConsent: new(false),
			},
		},
	})
	require.NoError(t, err)
	require.Len(t, createResp.Records, 2)

	req := httptest.NewRequest(
		http.MethodDelete,
		"/api/v1/policy/metadata/"+createResp.Records[1].ID,
		nil,
	)
	req.SetPathValue("id", createResp.Records[1].ID)
	w := httptest.NewRecorder()

	handler.DeletePolicyMetadata(w, req)

	assert.Equal(t, http.StatusNoContent, w.Code)
	assert.Empty(t, w.Body.String())

	var records []models.PolicyMetadata
	require.NoError(t, db.Where("schema_id = ?", "schema-123").Find(&records).Error)
	require.Len(t, records, 1)
	assert.Equal(t, "person.name", records[0].FieldName)

	unknownReq := httptest.NewRequest(
		http.MethodDelete,
		"/api/v1/policy/metadata/not-a-uuid",
		nil,
	)
	unknownReq.SetPathValue("id", "not-a-uuid")
	unknownRecorder := httptest.NewRecorder()

	handler.DeletePolicyMetadata(unknownRecorder, unknownReq)

	assert.Equal(t, http.StatusNotFound, unknownRecorder.Code)
}

func TestHandler_RevokeAccessGrant(t *testing.T) {
	db := setupTestDB(t)
	handler := NewHandler(db)

	_, err := handler.policyService.CreatePolicyMetadata(&models.PolicyMetadataCreateRequest{
		SchemaID: "schema-123",
		Records: []models.PolicyMetadataCreateRequestRecord{
			{FieldName: "person.email", Source: models.SourcePrimary, RequiresAllowlist: new(true), RequiresConsent: new(false)},
		},
	})
	require.NoError(t, err)

	grants, err := handler.policyService.CreateAccessGrants(&models.AccessGrantCreateRequest{
		ApplicationID: "app-123",
		GrantDuration: models.GrantDurationTypeOneMonth,
		Records:       []models.AccessGrantCreateRequestRecord{{FieldName: "person.email", SchemaID: "schema-123"}},
	})
	require.NoError(t, err)
	grantID := grants.Records[0].ID

	revoke := func(id string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/policy/grants/"+id+"/revoke", nil)
		req.SetPathValue("id", id)
		w := httptest.NewRecorder()
		handler.RevokeAccessGrant(w, req)
		return w
	}

	w := revoke(grantID)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var resp models.AccessGrantResponse
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	assert.Equal(t, grantID, resp.ID)
	assert.Equal(t, models.GrantStatusRevoked, resp.Status)

	// A revoked grant can't be revoked again.
	assert.Equal(t, http.StatusNotFound, revoke(grantID).Code)
	assert.Equal(t, http.StatusNotFound, revoke("not-a-uuid").Code)
}

func TestHandler_ConsentBypassGrants(t *testing.T) {
	db := setupTestDB(t)
	handler := NewHandler(db)

	_, err := handler.policyService.CreatePolicyMetadata(&models.PolicyMetadataCreateRequest{
		SchemaID: "schema-123",
		Records: []models.PolicyMetadataCreateRequestRecord{
			{FieldName: "person.nic", Source: models.SourcePrimary, RequiresAllowlist: new(false), RequiresConsent: new(true)},
			{FieldName: "person.name", Source: models.SourcePrimary, RequiresAllowlist: new(true), RequiresConsent: new(false)},
		},
	})
	require.NoError(t, err)

	serve := func(method, path, body string, h http.HandlerFunc, pathValues ...string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
		for i := 0; i+1 < len(pathValues); i += 2 {
			req.SetPathValue(pathValues[i], pathValues[i+1])
		}
		w := httptest.NewRecorder()
		h(w, req)
		return w
	}
	grant := func(body string) *httptest.ResponseRecorder {
		return serve(http.MethodPost, "/api/v1/policy/grants", body, handler.CreateAccessGrants)
	}
	bypass := func(field, legalBasis string) *httptest.ResponseRecorder {
		return grant(`{"applicationId":"police","grantDuration":"30d","consentBypass":true,"legalBasis":"` + legalBasis +
			`","records":[{"schemaId":"schema-123","fieldName":"` + field + `"}]}`)
	}

	w := bypass("person.nic", "Police Act s.12")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var created models.AccessGrantListResponse
	require.NoError(t, json.NewDecoder(w.Body).Decode(&created))
	require.Len(t, created.Records, 1)
	id := created.Records[0].ID
	assert.Equal(t, models.GrantStatusActive, created.Records[0].Status)
	assert.True(t, created.Records[0].ConsentBypass)

	assert.Equal(t, http.StatusBadRequest, bypass("person.nic", " ").Code, "legal basis required")
	assert.Equal(t, http.StatusBadRequest, bypass("person.name", "Police Act s.12").Code, "allowlist field asks for no consent")
	assert.Equal(t, http.StatusNotFound, bypass("person.missing", "Police Act s.12").Code)
	assert.Equal(t, http.StatusBadRequest,
		grant(`{"applicationId":"police","grantDuration":"30d","legalBasis":"Police Act s.12",`+
			`"records":[{"schemaId":"schema-123","fieldName":"person.nic"}]}`).Code,
		"legal basis on a plain grant")

	w = serve(http.MethodGet, "/api/v1/policy/grants/"+id, "", handler.GetAccessGrant, "id", id)
	require.Equal(t, http.StatusOK, w.Code)
	var got models.AccessGrantResponse
	require.NoError(t, json.NewDecoder(w.Body).Decode(&got))
	assert.True(t, got.ConsentBypass)
	require.NotNil(t, got.LegalBasis)
	assert.Equal(t, "Police Act s.12", *got.LegalBasis)
	assert.Equal(t, http.StatusNotFound,
		serve(http.MethodGet, "/api/v1/policy/grants/x", "", handler.GetAccessGrant, "id", "not-a-uuid").Code)

	w = serve(http.MethodGet, "/api/v1/policy/grants?consentBypass=true", "", handler.ListAccessGrants)
	require.Equal(t, http.StatusOK, w.Code)
	var listed models.AccessGrantListResponse
	require.NoError(t, json.NewDecoder(w.Body).Decode(&listed))
	require.Len(t, listed.Records, 1)
	assert.Equal(t, id, listed.Records[0].ID)
}

func TestHandler_ListAccessGrants(t *testing.T) {
	db := setupTestDB(t)
	handler := NewHandler(db)

	_, err := handler.policyService.CreatePolicyMetadata(&models.PolicyMetadataCreateRequest{
		SchemaID: "schema-123",
		Records: []models.PolicyMetadataCreateRequestRecord{
			{FieldName: "person.email", Source: models.SourcePrimary, RequiresAllowlist: new(true), RequiresConsent: new(false)},
		},
	})
	require.NoError(t, err)
	for _, applicationID := range []string{"app-123", "app-456"} {
		_, err = handler.policyService.CreateAccessGrants(&models.AccessGrantCreateRequest{
			ApplicationID: applicationID,
			GrantDuration: models.GrantDurationTypeOneMonth,
			Records:       []models.AccessGrantCreateRequestRecord{{FieldName: "person.email", SchemaID: "schema-123"}},
		})
		require.NoError(t, err)
	}

	list := func(query string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/policy/grants"+query, nil)
		w := httptest.NewRecorder()
		handler.ListAccessGrants(w, req)
		return w
	}

	w := list("?applicationId=app-456&schemaId=schema-123&status=active")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var resp models.AccessGrantListResponse
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	require.Len(t, resp.Records, 1)
	assert.Equal(t, "app-456", resp.Records[0].ApplicationID)
	assert.Equal(t, "person.email", resp.Records[0].FieldName)

	assert.Equal(t, http.StatusBadRequest, list("?status=pending").Code)
	assert.Equal(t, http.StatusBadRequest, list("?consentBypass=maybe").Code)
}

func TestHandler_CreateAccessGrants(t *testing.T) {
	db := setupTestDB(t)
	handler := NewHandler(db)

	// Create initial policy metadata
	createReq := models.PolicyMetadataCreateRequest{
		SchemaID: "schema-123",
		Records: []models.PolicyMetadataCreateRequestRecord{
			{
				FieldName:         "person.fullName",
				DisplayName:       testhelpers.StringPtr("Full Name"),
				Source:            models.SourcePrimary,
				RequiresAllowlist: new(true), RequiresConsent: new(false),
			},
		},
	}
	_, err := handler.policyService.CreatePolicyMetadata(&createReq)
	require.NoError(t, err)

	tests := []struct {
		name           string
		requestBody    models.AccessGrantCreateRequest
		expectedStatus int
		validateFunc   func(t *testing.T, response *httptest.ResponseRecorder)
	}{
		{
			name: "Grant access successfully",
			requestBody: models.AccessGrantCreateRequest{
				ApplicationID: "app-123",
				GrantDuration: models.GrantDurationTypeOneMonth,
				Records: []models.AccessGrantCreateRequestRecord{
					{
						FieldName: "person.fullName",
						SchemaID:  "schema-123",
					},
				},
			},
			expectedStatus: http.StatusOK,
			validateFunc: func(t *testing.T, w *httptest.ResponseRecorder) {
				var resp models.AccessGrantListResponse
				require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
				require.Len(t, resp.Records, 1)
				assert.NotEmpty(t, resp.Records[0].ID)
				assert.Equal(t, "person.fullName", resp.Records[0].FieldName)
				assert.Equal(t, "app-123", resp.Records[0].ApplicationID)
				assert.Equal(t, models.GrantStatusActive, resp.Records[0].Status)
				assert.NotEmpty(t, resp.Records[0].ExpiresAt)
			},
		},
		{
			name: "Field not found",
			requestBody: models.AccessGrantCreateRequest{
				ApplicationID: "app-123",
				GrantDuration: models.GrantDurationTypeOneMonth,
				Records: []models.AccessGrantCreateRequestRecord{
					{
						FieldName: "person.nonexistent",
						SchemaID:  "schema-123",
					},
				},
			},
			expectedStatus: http.StatusNotFound,
		},
		{
			name: "Missing application ID",
			requestBody: models.AccessGrantCreateRequest{
				ApplicationID: "",
				GrantDuration: models.GrantDurationTypeOneMonth,
				Records:       []models.AccessGrantCreateRequestRecord{},
			},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name: "No records",
			requestBody: models.AccessGrantCreateRequest{
				ApplicationID: "app-123",
				GrantDuration: models.GrantDurationTypeOneMonth,
				Records:       []models.AccessGrantCreateRequestRecord{},
			},
			expectedStatus: http.StatusOK,
		},
		{
			name: "Invalid grant duration",
			requestBody: models.AccessGrantCreateRequest{
				ApplicationID: "app-123",
				GrantDuration: "invalid-duration", // Invalid grant duration
				Records: []models.AccessGrantCreateRequestRecord{
					{
						FieldName: "person.fullName",
						SchemaID:  "schema-123",
					},
				},
			},
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(tt.requestBody)
			req := httptest.NewRequest(http.MethodPost, "/api/v1/policy/grants", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			handler.CreateAccessGrants(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("Expected status %d, got %d. Body: %s", tt.expectedStatus, w.Code, w.Body.String())
			}

			if tt.validateFunc != nil {
				tt.validateFunc(t, w)
			}
		})
	}
}

func TestHandler_GetPolicyDecision(t *testing.T) {
	db := setupTestDB(t)
	handler := NewHandler(db)

	// Create policy metadata with allow list
	createReq := models.PolicyMetadataCreateRequest{
		SchemaID: "schema-123",
		Records: []models.PolicyMetadataCreateRequestRecord{
			{
				FieldName:         "person.fullName",
				DisplayName:       testhelpers.StringPtr("Full Name"),
				Source:            models.SourcePrimary,
				RequiresAllowlist: new(true), RequiresConsent: new(false),
			},
			{
				FieldName:         "person.nic",
				DisplayName:       testhelpers.StringPtr("NIC"),
				Source:            models.SourcePrimary,
				RequiresAllowlist: new(true), RequiresConsent: new(true),
				Owner: testhelpers.OwnerPtr(models.OwnerCitizen),
			},
		},
	}
	_, err := handler.policyService.CreatePolicyMetadata(&createReq)
	if err != nil {
		t.Fatalf("Failed to create policy metadata: %v", err)
	}

	// Update allow list for authorized field
	updateReq := models.AccessGrantCreateRequest{
		ApplicationID: "app-123",
		GrantDuration: models.GrantDurationTypeOneMonth,
		Records: []models.AccessGrantCreateRequestRecord{
			{
				FieldName: "person.fullName",
				SchemaID:  "schema-123",
			},
		},
	}
	_, err = handler.policyService.CreateAccessGrants(&updateReq)
	if err != nil {
		t.Fatalf("Failed to update allow list: %v", err)
	}

	tests := []struct {
		name           string
		requestBody    models.PolicyDecisionRequest
		expectedStatus int
		validateFunc   func(t *testing.T, response *httptest.ResponseRecorder)
	}{
		{
			name: "Authorized request",
			requestBody: models.PolicyDecisionRequest{
				ApplicationID: "app-123",
				RequiredFields: []models.PolicyDecisionRequestRecord{
					{
						FieldName: "person.fullName",
						SchemaID:  "schema-123",
					},
				},
			},
			expectedStatus: http.StatusOK,
			validateFunc: func(t *testing.T, w *httptest.ResponseRecorder) {
				var resp models.PolicyDecisionResponse
				if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
					t.Fatalf("Failed to decode response: %v", err)
				}
				if !resp.AppAuthorized {
					t.Error("Expected appAuthorized to be true")
				}
				if len(resp.UnauthorizedFields) > 0 {
					t.Errorf("Expected no unauthorized fields, got %d", len(resp.UnauthorizedFields))
				}
			},
		},
		{
			name: "Unauthorized request - not in allow list",
			requestBody: models.PolicyDecisionRequest{
				ApplicationID: "app-456", // Different app, not in allow list
				RequiredFields: []models.PolicyDecisionRequestRecord{
					{
						FieldName: "person.fullName",
						SchemaID:  "schema-123",
					},
				},
			},
			expectedStatus: http.StatusOK,
			validateFunc: func(t *testing.T, w *httptest.ResponseRecorder) {
				var resp models.PolicyDecisionResponse
				if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
					t.Fatalf("Failed to decode response: %v", err)
				}
				if resp.AppAuthorized {
					t.Error("Expected appAuthorized to be false")
				}
				if len(resp.UnauthorizedFields) != 1 {
					t.Errorf("Expected 1 unauthorized field, got %d", len(resp.UnauthorizedFields))
				}
			},
		},
		{
			name: "Unauthorized request - restricted field not in allow list",
			requestBody: models.PolicyDecisionRequest{
				ApplicationID: "app-123",
				RequiredFields: []models.PolicyDecisionRequestRecord{
					{
						FieldName: "person.nic",
						SchemaID:  "schema-123",
					},
				},
			},
			expectedStatus: http.StatusOK,
			validateFunc: func(t *testing.T, w *httptest.ResponseRecorder) {
				var resp models.PolicyDecisionResponse
				if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
					t.Fatalf("Failed to decode response: %v", err)
				}
				if resp.AppAuthorized {
					t.Error("Expected appAuthorized to be false")
				}
				if len(resp.UnauthorizedFields) != 1 {
					t.Errorf("Expected 1 unauthorized field, got %d", len(resp.UnauthorizedFields))
				}
			},
		},
		{
			name: "Consent required - restricted field in allow list",
			requestBody: models.PolicyDecisionRequest{
				ApplicationID: "app-123",
				RequiredFields: []models.PolicyDecisionRequestRecord{
					{
						FieldName: "person.nic",
						SchemaID:  "schema-123",
					},
				},
			},
			expectedStatus: http.StatusOK,
			validateFunc: func(t *testing.T, w *httptest.ResponseRecorder) {
				var resp models.PolicyDecisionResponse
				if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
					t.Fatalf("Failed to decode response: %v", err)
				}
				if !resp.AppAuthorized {
					t.Error("Expected appAuthorized to be true (field is in allow list)")
				}
				if !resp.AppRequiresOwnerConsent {
					t.Error("Expected appRequiresOwnerConsent to be true")
				}
				if len(resp.ConsentRequiredFields) != 1 {
					t.Errorf("Expected 1 consent required field, got %d", len(resp.ConsentRequiredFields))
				}
				if len(resp.UnauthorizedFields) > 0 {
					t.Errorf("Expected no unauthorized fields, got %d", len(resp.UnauthorizedFields))
				}
			},
		},
		{
			name: "Field not found",
			requestBody: models.PolicyDecisionRequest{
				ApplicationID: "app-123",
				RequiredFields: []models.PolicyDecisionRequestRecord{
					{
						FieldName: "person.nonexistent",
						SchemaID:  "schema-123",
					},
				},
			},
			expectedStatus: http.StatusInternalServerError,
		},
		{
			name: "Empty request body",
			requestBody: models.PolicyDecisionRequest{
				ApplicationID:  "",
				RequiredFields: []models.PolicyDecisionRequestRecord{},
			},
			expectedStatus: http.StatusBadRequest, // Handler validates required fields
		},
		{
			name: "Service error - schema not found",
			requestBody: models.PolicyDecisionRequest{
				ApplicationID: "app-123",
				RequiredFields: []models.PolicyDecisionRequestRecord{
					{
						FieldName: "person.fullName",
						SchemaID:  "nonexistent-schema",
					},
				},
			},
			expectedStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup: Update allow list for consent-required test case before making the request
			if tt.name == "Consent required - restricted field in allow list" {
				updateReq := models.AccessGrantCreateRequest{
					ApplicationID: "app-123",
					GrantDuration: models.GrantDurationTypeOneMonth,
					Records: []models.AccessGrantCreateRequestRecord{
						{
							FieldName: "person.nic",
							SchemaID:  "schema-123",
						},
					},
				}
				_, err := handler.policyService.CreateAccessGrants(&updateReq)
				if err != nil {
					t.Fatalf("Failed to update allow list for test setup: %v", err)
				}
			}

			// Use the same handler instance for all operations
			body, _ := json.Marshal(tt.requestBody)
			req := httptest.NewRequest(http.MethodPost, "/api/v1/policy/decide", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			handler.GetPolicyDecision(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("Expected status %d, got %d. Body: %s", tt.expectedStatus, w.Code, w.Body.String())
				return // Skip validation if status doesn't match
			}

			if tt.validateFunc != nil {
				tt.validateFunc(t, w)
			}
		})
	}
}
