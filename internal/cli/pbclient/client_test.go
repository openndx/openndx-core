package pbclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	models2 "github.com/openndx/openndx-core/internal/pb/models"
	"github.com/stretchr/testify/assert"
)

func TestCreateSchema_Success(t *testing.T) {
	var capturedMethod, capturedPath string
	var capturedBody models2.CreateSchemaRequest

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedMethod = r.Method
		capturedPath = r.URL.Path
		assert.NoError(t, json.NewDecoder(r.Body).Decode(&capturedBody))

		resp := models2.SchemaResponse{
			SchemaID:   "sch_new",
			SchemaName: capturedBody.SchemaName,
			Endpoint:   capturedBody.Endpoint,
			MemberID:   capturedBody.MemberID,
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated) // PB returns 201 for creation
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient(server.URL, "token")
	req := &models2.CreateSchemaRequest{
		SchemaName: "Citizen Info",
		Endpoint:   "http://example.com/graphql",
		MemberID:   "member-1",
		Fields: []models2.PolicyMetadataCreateRequestRecord{
			{FieldName: "email", AccessControlType: models2.AccessControlTypePublic, Source: models2.SourcePrimary},
		},
	}

	resp, err := client.CreateSchema(context.Background(), req)

	assert.NoError(t, err)
	if assert.NotNil(t, resp) {
		assert.Equal(t, "sch_new", resp.SchemaID)
	}
	assert.Equal(t, http.MethodPost, capturedMethod)
	assert.Equal(t, "/api/v1/schemas", capturedPath)
	if assert.Len(t, capturedBody.Fields, 1) {
		assert.Equal(t, "email", capturedBody.Fields[0].FieldName)
	}
}

func TestCreateSchema_ErrorResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "exactly one of sdl or fields must be provided"})
	}))
	defer server.Close()

	client := NewClient(server.URL, "token")
	resp, err := client.CreateSchema(context.Background(), &models2.CreateSchemaRequest{
		SchemaName: "Bad Schema",
		Endpoint:   "http://example.com/graphql",
		MemberID:   "member-1",
	})

	assert.Error(t, err)
	assert.Nil(t, resp)
	assert.ErrorContains(t, err, "status 400")
}

func TestCreateMember_Success(t *testing.T) {
	var capturedMethod, capturedPath string
	var capturedBody models2.CreateMemberRequest

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedMethod = r.Method
		capturedPath = r.URL.Path
		assert.NoError(t, json.NewDecoder(r.Body).Decode(&capturedBody))

		resp := models2.MemberResponse{
			MemberID: "mem_new",
			Name:     capturedBody.Name,
			Email:    capturedBody.Email,
		}
		if capturedBody.IdpUserID != nil {
			resp.IdpUserID = *capturedBody.IdpUserID
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated) // PB returns 201 for creation
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient(server.URL, "token")
	idpUserID := "thunder-user-1"
	req := &models2.CreateMemberRequest{
		Name:        "New Member",
		Email:       "new@example.com",
		PhoneNumber: "+1234567890",
		IdpUserID:   &idpUserID,
	}

	resp, err := client.CreateMember(context.Background(), req)

	assert.NoError(t, err)
	if assert.NotNil(t, resp) {
		assert.Equal(t, "mem_new", resp.MemberID)
		assert.Equal(t, idpUserID, resp.IdpUserID)
	}
	assert.Equal(t, http.MethodPost, capturedMethod)
	assert.Equal(t, "/api/v1/members", capturedPath)
	assert.Equal(t, idpUserID, *capturedBody.IdpUserID)
}

func TestCreateMember_ErrorResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "Insufficient permissions"})
	}))
	defer server.Close()

	client := NewClient(server.URL, "token")
	resp, err := client.CreateMember(context.Background(), &models2.CreateMemberRequest{
		Name:        "New Member",
		Email:       "new@example.com",
		PhoneNumber: "+1234567890",
	})

	assert.Error(t, err)
	assert.Nil(t, resp)
	assert.ErrorContains(t, err, "status 403")
}

func TestCreateApplication_Success(t *testing.T) {
	var capturedMethod, capturedPath string
	var capturedBody models2.CreateApplicationRequest

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedMethod = r.Method
		capturedPath = r.URL.Path
		assert.NoError(t, json.NewDecoder(r.Body).Decode(&capturedBody))

		resp := models2.ApplicationResponse{
			ApplicationID:   "app_new",
			ApplicationName: capturedBody.ApplicationName,
			SelectedFields:  capturedBody.SelectedFields,
			MemberID:        capturedBody.MemberID,
		}
		if capturedBody.IdpClientID != nil {
			resp.IdpClientID = capturedBody.IdpClientID
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated) // PB returns 201 for creation
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient(server.URL, "token")
	idpAppID := "thunder-app-1"
	idpClientID := "THUNDER_CLIENT"
	req := &models2.CreateApplicationRequest{
		ApplicationName: "New App",
		SelectedFields: []models2.SelectedFieldRecord{
			{FieldName: "email", SchemaID: "schema-1"},
		},
		MemberID:         "member-1",
		IdpApplicationID: &idpAppID,
		IdpClientID:      &idpClientID,
	}

	resp, err := client.CreateApplication(context.Background(), req)

	assert.NoError(t, err)
	if assert.NotNil(t, resp) {
		assert.Equal(t, "app_new", resp.ApplicationID)
		assert.Equal(t, idpClientID, *resp.IdpClientID)
	}
	assert.Equal(t, http.MethodPost, capturedMethod)
	assert.Equal(t, "/api/v1/applications", capturedPath)
	assert.Equal(t, idpAppID, *capturedBody.IdpApplicationID)
}

func TestCreateApplication_ErrorResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "idpApplicationId and idpClientId must both be provided together, or both omitted"})
	}))
	defer server.Close()

	client := NewClient(server.URL, "token")
	resp, err := client.CreateApplication(context.Background(), &models2.CreateApplicationRequest{
		ApplicationName: "New App",
		SelectedFields:  []models2.SelectedFieldRecord{{FieldName: "email", SchemaID: "schema-1"}},
		MemberID:        "member-1",
	})

	assert.Error(t, err)
	assert.Nil(t, resp)
	assert.ErrorContains(t, err, "status 400")
}

func TestGetApplication_Success(t *testing.T) {
	var capturedAuth, capturedPath, capturedMethod string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedAuth = r.Header.Get("Authorization")
		capturedPath = r.URL.Path
		capturedMethod = r.Method

		resp := models2.ApplicationResponse{
			ApplicationID:   "app_123",
			ApplicationName: "Test App",
			SelectedFields: []models2.SelectedFieldRecord{
				{FieldName: "email", SchemaID: "schema-1"},
			},
			MemberID: "member-1",
			Version:  "1.0.0",
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-token")
	resp, err := client.GetApplication(context.Background(), "app_123")

	assert.NoError(t, err)
	if assert.NotNil(t, resp) {
		assert.Equal(t, "app_123", resp.ApplicationID)
		assert.Equal(t, "Test App", resp.ApplicationName)
		assert.Len(t, resp.SelectedFields, 1)
	}

	assert.Equal(t, "Bearer test-token", capturedAuth)
	assert.Equal(t, "/api/v1/applications/app_123", capturedPath)
	assert.Equal(t, http.MethodGet, capturedMethod)
}

func TestGetApplication_NotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "application not found"})
	}))
	defer server.Close()

	client := NewClient(server.URL, "token")
	resp, err := client.GetApplication(context.Background(), "does-not-exist")

	assert.Error(t, err)
	assert.Nil(t, resp)
	assert.ErrorContains(t, err, "status 404")
}

func TestListApplications_Success(t *testing.T) {
	var capturedPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.String()
		resp := map[string]any{
			"items": []models2.ApplicationResponse{
				{ApplicationID: "app_1", ApplicationName: "App One", MemberID: "member-1"},
				{ApplicationID: "app_2", ApplicationName: "App Two", MemberID: "member-1"},
			},
			"count": 2,
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient(server.URL, "token")
	apps, err := client.ListApplications(context.Background(), nil)

	assert.NoError(t, err)
	assert.Equal(t, "/api/v1/applications", capturedPath)
	assert.Equal(t, 2, apps.Count)
	if assert.Len(t, apps.Items, 2) {
		assert.Equal(t, "app_1", apps.Items[0].ApplicationID)
		assert.Equal(t, "app_2", apps.Items[1].ApplicationID)
	}
}

func TestListApplications_WithMemberFilter(t *testing.T) {
	var capturedPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.String()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"items": []models2.ApplicationResponse{}, "count": 0})
	}))
	defer server.Close()

	client := NewClient(server.URL, "token")
	memberID := "member-42"
	_, err := client.ListApplications(context.Background(), &memberID)

	assert.NoError(t, err)
	assert.Equal(t, "/api/v1/applications?memberId=member-42", capturedPath)
}

func TestListApplications_EmptyResult(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"items": []models2.ApplicationResponse{}, "count": 0})
	}))
	defer server.Close()

	client := NewClient(server.URL, "token")
	apps, err := client.ListApplications(context.Background(), nil)

	assert.NoError(t, err)
	assert.Empty(t, apps.Items)
	assert.Equal(t, 0, apps.Count)
}

func TestUpdateApplicationPolicy_Success(t *testing.T) {
	var capturedAuth, capturedPath, capturedMethod string
	var capturedBody models2.UpdateApplicationPolicyRequest

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedAuth = r.Header.Get("Authorization")
		capturedPath = r.URL.Path
		capturedMethod = r.Method
		assert.NoError(t, json.NewDecoder(r.Body).Decode(&capturedBody))

		resp := models2.ApplicationResponse{
			ApplicationID:   "app_123",
			ApplicationName: "Test App",
			SelectedFields:  capturedBody.SelectedFields,
			MemberID:        "member-1",
			Version:         "1.0.0",
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-token")
	grantDuration := models2.GrantDurationTypeOneYear
	req := &models2.UpdateApplicationPolicyRequest{
		SelectedFields: []models2.SelectedFieldRecord{
			{FieldName: "email", SchemaID: "schema-1"},
		},
		GrantDuration: &grantDuration,
	}

	resp, err := client.UpdateApplicationPolicy(context.Background(), "app_123", req)

	assert.NoError(t, err)
	if assert.NotNil(t, resp) {
		assert.Equal(t, "app_123", resp.ApplicationID)
		assert.Equal(t, req.SelectedFields, []models2.SelectedFieldRecord(resp.SelectedFields))
	}

	assert.Equal(t, "Bearer test-token", capturedAuth)
	assert.Equal(t, "/api/v1/applications/app_123/policy", capturedPath)
	assert.Equal(t, http.MethodPut, capturedMethod)
	assert.Equal(t, req.SelectedFields, capturedBody.SelectedFields)
}

func TestUpdateApplicationPolicy_TrimsTrailingSlashInBaseURL(t *testing.T) {
	var capturedPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(models2.ApplicationResponse{ApplicationID: "app_1"})
	}))
	defer server.Close()

	client := NewClient(server.URL+"/", "token")
	_, err := client.UpdateApplicationPolicy(context.Background(), "app_1", &models2.UpdateApplicationPolicyRequest{
		SelectedFields: []models2.SelectedFieldRecord{{FieldName: "f", SchemaID: "s"}},
	})

	assert.NoError(t, err)
	assert.Equal(t, "/api/v1/applications/app_1/policy", capturedPath)
}

func TestUpdateApplicationPolicy_ErrorResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "Insufficient permissions"})
	}))
	defer server.Close()

	client := NewClient(server.URL, "token")
	resp, err := client.UpdateApplicationPolicy(context.Background(), "app_1", &models2.UpdateApplicationPolicyRequest{
		SelectedFields: []models2.SelectedFieldRecord{{FieldName: "f", SchemaID: "s"}},
	})

	assert.Error(t, err)
	assert.Nil(t, resp)
	assert.ErrorContains(t, err, "status 403")
	assert.ErrorContains(t, err, "Insufficient permissions")
}

func TestUpdateApplicationPolicy_Unreachable(t *testing.T) {
	client := NewClient("http://127.0.0.1:1", "token")
	resp, err := client.UpdateApplicationPolicy(context.Background(), "app_1", &models2.UpdateApplicationPolicyRequest{
		SelectedFields: []models2.SelectedFieldRecord{{FieldName: "f", SchemaID: "s"}},
	})

	assert.Error(t, err)
	assert.Nil(t, resp)
}
