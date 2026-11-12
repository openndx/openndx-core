package fieldpolicy

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/openndx/openndx-core/internal/pdp/pdptest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRespondWithServiceError_HidesInternalDetails verifies that unexpected
// service errors are logged without exposing details to clients.
func TestRespondWithServiceError_HidesInternalDetails(t *testing.T) {
	w := httptest.NewRecorder()
	internalErr := errors.New("database connection failed: password=secret")

	respondWithServiceError(w, internalErr)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.JSONEq(t, `{"error":"internal server error"}`, w.Body.String())
	assert.NotContains(t, w.Body.String(), internalErr.Error())
}

func TestHandler_CreatePolicyMetadata_InvalidJSON(t *testing.T) {
	// This test doesn't need a database - it only tests JSON parsing
	db := setupTestDB(t)
	handler := NewHandler(NewService(db))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/policy/metadata", bytes.NewBufferString("invalid json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.CreatePolicyMetadata(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandler_NewHandler(t *testing.T) {
	db := setupTestDB(t)
	handler := NewHandler(NewService(db))
	assert.NotNil(t, handler)
	assert.NotNil(t, handler.service)
}

func TestHandler_CreatePolicyMetadata(t *testing.T) {
	tests := []struct {
		name           string
		requestBody    PolicyMetadataCreateRequest
		expectedStatus int
		validateFunc   func(t *testing.T, response *httptest.ResponseRecorder)
	}{
		{
			name: "Create new policy metadata successfully",
			requestBody: PolicyMetadataCreateRequest{
				SchemaID: "schema-123",
				Records: []PolicyMetadataCreateRequestRecord{
					{
						FieldName:         "person.fullName",
						DisplayName:       pdptest.StringPtr("Full Name"),
						Description:       pdptest.StringPtr("Complete name"),
						Source:            SourcePrimary,
						RequiresAllowlist: new(true), RequiresConsent: new(false),
					},
				},
			},
			expectedStatus: http.StatusCreated,
			validateFunc: func(t *testing.T, w *httptest.ResponseRecorder) {
				var resp PolicyMetadataCreateResponse
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
			requestBody: PolicyMetadataCreateRequest{
				SchemaID: "schema-123",
				Records: []PolicyMetadataCreateRequestRecord{
					{
						FieldName:         "person.fullName",
						DisplayName:       pdptest.StringPtr("Full Name Updated"),
						Description:       pdptest.StringPtr("Updated description"),
						Source:            SourcePrimary,
						RequiresAllowlist: new(true), RequiresConsent: new(true),
					},
				},
			},
			expectedStatus: http.StatusCreated,
			validateFunc: func(t *testing.T, w *httptest.ResponseRecorder) {
				var resp PolicyMetadataCreateResponse
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
			requestBody: PolicyMetadataCreateRequest{
				SchemaID: "",
				Records:  []PolicyMetadataCreateRequestRecord{},
			},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name: "Missing policy flag",
			requestBody: PolicyMetadataCreateRequest{
				SchemaID: "schema-123",
				Records: []PolicyMetadataCreateRequestRecord{
					{
						FieldName:         "person.invalid",
						Source:            SourcePrimary,
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
			requestBody: PolicyMetadataCreateRequest{
				SchemaID: "schema-123",
				Records: []PolicyMetadataCreateRequestRecord{
					{
						FieldName:         "person.fullName",
						DisplayName:       pdptest.StringPtr("Full Name"),
						Source:            SourcePrimary,
						RequiresAllowlist: new(true), RequiresConsent: new(false),
					},
					{
						FieldName:         "person.email",
						DisplayName:       pdptest.StringPtr("Email"),
						Source:            SourcePrimary,
						RequiresAllowlist: new(true), RequiresConsent: new(false),
					},
				},
			},
			expectedStatus: http.StatusCreated,
			validateFunc: func(t *testing.T, w *httptest.ResponseRecorder) {
				var resp PolicyMetadataCreateResponse
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
			requestBody: PolicyMetadataCreateRequest{
				SchemaID: "schema-123",
				Records: []PolicyMetadataCreateRequestRecord{
					{
						FieldName:         "person.fullName",
						Source:            SourceFallback,
						RequiresAllowlist: new(true), RequiresConsent: new(false),
					},
				},
			},
			expectedStatus: http.StatusCreated,
			validateFunc: func(t *testing.T, w *httptest.ResponseRecorder) {
				var resp PolicyMetadataCreateResponse
				if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
					t.Fatalf("Failed to decode response: %v", err)
				}
				if resp.Records[0].Source != SourceFallback {
					t.Errorf("Expected source fallback, got %s", resp.Records[0].Source)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset database for each test
			db := setupTestDB(t)
			handler := NewHandler(NewService(db))

			// Create initial record for update test
			if tt.name == "Update existing policy metadata" {
				initialReq := PolicyMetadataCreateRequest{
					SchemaID: "schema-123",
					Records: []PolicyMetadataCreateRequestRecord{
						{
							FieldName:         "person.fullName",
							DisplayName:       pdptest.StringPtr("Full Name"),
							Source:            SourcePrimary,
							RequiresAllowlist: new(true), RequiresConsent: new(false),
						},
					},
				}
				_, err := handler.service.CreatePolicyMetadata(&initialReq)
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
	handler := NewHandler(NewService(db))

	createResp, err := handler.service.CreatePolicyMetadata(&PolicyMetadataCreateRequest{
		SchemaID: "schema-123",
		Records: []PolicyMetadataCreateRequestRecord{
			{
				FieldName:         "person.name",
				DisplayName:       pdptest.StringPtr("Name"),
				Source:            SourcePrimary,
				RequiresAllowlist: new(true), RequiresConsent: new(false),
			},
			{
				FieldName:         "person.email",
				DisplayName:       pdptest.StringPtr("Email"),
				Source:            SourcePrimary,
				RequiresAllowlist: new(true), RequiresConsent: new(false),
			},
		},
	})
	require.NoError(t, err)
	require.Len(t, createResp.Records, 2)

	_, err = handler.service.CreatePolicyMetadata(&PolicyMetadataCreateRequest{
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

	allReq := httptest.NewRequest(
		http.MethodGet,
		"/api/v1/policy/metadata",
		nil,
	)
	allRecorder := httptest.NewRecorder()

	handler.ListPolicyMetadata(allRecorder, allReq)

	require.Equal(t, http.StatusOK, allRecorder.Code)

	var allResp PolicyMetadataListResponse
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

	var filteredResp PolicyMetadataListResponse
	require.NoError(t, json.NewDecoder(filteredRecorder.Body).Decode(&filteredResp))
	require.Len(t, filteredResp.Records, 2)
	assert.Equal(t, "person.email", filteredResp.Records[0].FieldName)
	assert.Equal(t, "person.name", filteredResp.Records[1].FieldName)
}

func TestHandler_PatchPolicyMetadata(t *testing.T) {
	db := setupTestDB(t)
	handler := NewHandler(NewService(db))

	createResp, err := handler.service.CreatePolicyMetadata(&PolicyMetadataCreateRequest{
		SchemaID: "schema-123",
		Records: []PolicyMetadataCreateRequestRecord{
			{
				FieldName:         "person.name",
				DisplayName:       pdptest.StringPtr("Name"),
				Source:            SourcePrimary,
				RequiresAllowlist: new(true), RequiresConsent: new(false),
			},
			{
				FieldName:         "person.email",
				DisplayName:       pdptest.StringPtr("Email"),
				Source:            SourcePrimary,
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

	var resp PolicyMetadataResponse
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	require.NotNil(t, resp.DisplayName)
	assert.Equal(t, "Primary Email", *resp.DisplayName)

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
}

func TestHandler_PatchPolicyMetadata_Flags(t *testing.T) {
	db := setupTestDB(t)
	handler := NewHandler(NewService(db))

	createResp, err := handler.service.CreatePolicyMetadata(&PolicyMetadataCreateRequest{
		SchemaID: "schema-123",
		Records: []PolicyMetadataCreateRequestRecord{
			{FieldName: "person.email", Source: SourcePrimary, RequiresAllowlist: new(true), RequiresConsent: new(false)},
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
				var resp PolicyMetadataResponse
				require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
				assert.True(t, resp.RequiresAllowlist, "an omitted flag is unchanged")
				assert.Equal(t, tt.wantConsent, resp.RequiresConsent)
			}
		})
	}
}

func TestHandler_DeletePolicyMetadata(t *testing.T) {
	db := setupTestDB(t)
	handler := NewHandler(NewService(db))

	createResp, err := handler.service.CreatePolicyMetadata(&PolicyMetadataCreateRequest{
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

	var records []PolicyMetadata
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
