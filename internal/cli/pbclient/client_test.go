package pbclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/openndx/openndx-core/internal/pb/application"
	"github.com/openndx/openndx-core/internal/pb/member"
	"github.com/openndx/openndx-core/internal/pb/policy"
	"github.com/openndx/openndx-core/internal/pb/schema"
	"github.com/stretchr/testify/assert"
)

func TestCreateSchema_Success(t *testing.T) {
	var capturedMethod, capturedPath string
	var capturedBody schema.CreateSchemaRequest

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedMethod = r.Method
		capturedPath = r.URL.Path
		assert.NoError(t, json.NewDecoder(r.Body).Decode(&capturedBody))

		resp := schema.SchemaResponse{
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
	req := &schema.CreateSchemaRequest{
		SchemaName: "Citizen Info",
		Endpoint:   "http://example.com/graphql",
		MemberID:   "member-1",
		Fields: []policy.PolicyMetadataCreateRequestRecord{
			{FieldName: "email", AccessControlType: policy.AccessControlTypePublic, Source: policy.SourcePrimary},
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
	resp, err := client.CreateSchema(context.Background(), &schema.CreateSchemaRequest{
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
	var capturedBody member.CreateMemberRequest

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedMethod = r.Method
		capturedPath = r.URL.Path
		assert.NoError(t, json.NewDecoder(r.Body).Decode(&capturedBody))

		resp := member.MemberResponse{
			MemberID: "mem_new",
			Name:     capturedBody.Name,
			Email:    capturedBody.Email,
			OuID:     capturedBody.OuID,
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated) // PB returns 201 for creation
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient(server.URL, "token")
	ouID := "thunder-ou-1"
	req := &member.CreateMemberRequest{
		Name:        "New Member",
		Email:       "new@example.com",
		PhoneNumber: "+1234567890",
		OuID:        ouID,
	}

	resp, err := client.CreateMember(context.Background(), req)

	assert.NoError(t, err)
	if assert.NotNil(t, resp) {
		assert.Equal(t, "mem_new", resp.MemberID)
		assert.Equal(t, ouID, resp.OuID)
	}
	assert.Equal(t, http.MethodPost, capturedMethod)
	assert.Equal(t, "/api/v1/members", capturedPath)
	assert.Equal(t, ouID, capturedBody.OuID)
}

func TestCreateMember_ErrorResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "Insufficient permissions"})
	}))
	defer server.Close()

	client := NewClient(server.URL, "token")
	resp, err := client.CreateMember(context.Background(), &member.CreateMemberRequest{
		Name:        "New Member",
		Email:       "new@example.com",
		PhoneNumber: "+1234567890",
	})

	assert.Error(t, err)
	assert.Nil(t, resp)
	assert.ErrorContains(t, err, "status 403")
}

func TestGetMember_Success(t *testing.T) {
	var capturedAuth, capturedPath, capturedMethod string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedAuth = r.Header.Get("Authorization")
		capturedPath = r.URL.Path
		capturedMethod = r.Method

		resp := member.MemberResponse{
			MemberID: "mem_123",
			Name:     "Test Member",
			OuID:     "thunder-ou-1",
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-token")
	resp, err := client.GetMember(context.Background(), "mem_123")

	assert.NoError(t, err)
	if assert.NotNil(t, resp) {
		assert.Equal(t, "mem_123", resp.MemberID)
		assert.Equal(t, "thunder-ou-1", resp.OuID)
	}

	assert.Equal(t, "Bearer test-token", capturedAuth)
	assert.Equal(t, "/api/v1/members/mem_123", capturedPath)
	assert.Equal(t, http.MethodGet, capturedMethod)
}

func TestGetMember_Forbidden(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "Access denied to this resource"})
	}))
	defer server.Close()

	client := NewClient(server.URL, "token")
	resp, err := client.GetMember(context.Background(), "mem_other")

	assert.Error(t, err)
	assert.Nil(t, resp)
	assert.ErrorContains(t, err, "status 403")
}

func TestListMembers_Success(t *testing.T) {
	var capturedPath, capturedMethod string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.String()
		capturedMethod = r.Method
		resp := map[string]any{
			"items": []member.MemberResponse{
				{MemberID: "mem_1", Name: "Member One", OuID: "ou-1"},
				{MemberID: "mem_2", Name: "Member Two", OuID: "ou-2"},
			},
			"count": 2,
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient(server.URL, "token")
	members, err := client.ListMembers(context.Background(), nil)

	assert.NoError(t, err)
	assert.Equal(t, "/api/v1/members", capturedPath)
	assert.Equal(t, http.MethodGet, capturedMethod)
	assert.Equal(t, 2, members.Count)
	if assert.Len(t, members.Items, 2) {
		assert.Equal(t, "mem_1", members.Items[0].MemberID)
		assert.Equal(t, "ou-2", members.Items[1].OuID)
	}
}

func TestListMembers_WithOUFilter(t *testing.T) {
	var capturedPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.String()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"items": []member.MemberResponse{}, "count": 0})
	}))
	defer server.Close()

	client := NewClient(server.URL, "token")
	ouID := "ou-42"
	members, err := client.ListMembers(context.Background(), &ouID)

	assert.NoError(t, err)
	assert.Equal(t, "/api/v1/members?ouId=ou-42", capturedPath)
	assert.Empty(t, members.Items)
}

func TestListMembers_ErrorResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "User has no organization unit"})
	}))
	defer server.Close()

	client := NewClient(server.URL, "token")
	members, err := client.ListMembers(context.Background(), nil)

	assert.Error(t, err)
	assert.Nil(t, members)
	assert.ErrorContains(t, err, "status 403")
	assert.ErrorContains(t, err, "User has no organization unit")
}

func TestGetSchema_Success(t *testing.T) {
	var capturedAuth, capturedPath, capturedMethod string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedAuth = r.Header.Get("Authorization")
		capturedPath = r.URL.Path
		capturedMethod = r.Method

		resp := schema.SchemaResponse{
			SchemaID:   "drp-schema-v1",
			MemberID:   "mem_123",
			SchemaName: "DRP Person",
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-token")
	resp, err := client.GetSchema(context.Background(), "drp-schema-v1")

	assert.NoError(t, err)
	if assert.NotNil(t, resp) {
		assert.Equal(t, "drp-schema-v1", resp.SchemaID)
		assert.Equal(t, "mem_123", resp.MemberID)
	}

	assert.Equal(t, "Bearer test-token", capturedAuth)
	assert.Equal(t, "/api/v1/schemas/drp-schema-v1", capturedPath)
	assert.Equal(t, http.MethodGet, capturedMethod)
}

func TestGetSchema_Forbidden(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "Access denied to this resource"})
	}))
	defer server.Close()

	client := NewClient(server.URL, "token")
	resp, err := client.GetSchema(context.Background(), "other-schema")

	assert.Error(t, err)
	assert.Nil(t, resp)
	assert.ErrorContains(t, err, "status 403")
}

func TestListSchemas_Success(t *testing.T) {
	var capturedPath, capturedMethod string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.String()
		capturedMethod = r.Method
		resp := map[string]any{
			"items": []schema.SchemaResponse{
				{SchemaID: "schema-1", SchemaName: "Schema One", MemberID: "mem_1"},
				{SchemaID: "schema-2", SchemaName: "Schema Two", MemberID: "mem_2"},
			},
			"count": 2,
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient(server.URL, "token")
	schemas, err := client.ListSchemas(context.Background(), nil)

	assert.NoError(t, err)
	assert.Equal(t, "/api/v1/schemas", capturedPath)
	assert.Equal(t, http.MethodGet, capturedMethod)
	assert.Equal(t, 2, schemas.Count)
	if assert.Len(t, schemas.Items, 2) {
		assert.Equal(t, "schema-1", schemas.Items[0].SchemaID)
		assert.Equal(t, "mem_2", schemas.Items[1].MemberID)
	}
}

func TestListSchemas_WithMemberFilter(t *testing.T) {
	var capturedPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.String()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"items": []schema.SchemaResponse{}, "count": 0})
	}))
	defer server.Close()

	client := NewClient(server.URL, "token")
	memberID := "mem_42"
	schemas, err := client.ListSchemas(context.Background(), &memberID)

	assert.NoError(t, err)
	assert.Equal(t, "/api/v1/schemas?memberId=mem_42", capturedPath)
	assert.Empty(t, schemas.Items)
}

func TestListSchemas_ErrorResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "User member record not found"})
	}))
	defer server.Close()

	client := NewClient(server.URL, "token")
	schemas, err := client.ListSchemas(context.Background(), nil)

	assert.Error(t, err)
	assert.Nil(t, schemas)
	assert.ErrorContains(t, err, "status 403")
	assert.ErrorContains(t, err, "User member record not found")
}

func TestListSchemaPolicyMetadata_Success(t *testing.T) {
	var capturedPath, capturedMethod string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		capturedMethod = r.Method
		resp := map[string]any{
			"items": []policy.PolicyMetadataResponse{
				{ID: "pm_1", SchemaID: "drp-schema-v1", FieldName: "person.fullName", AccessControlType: policy.AccessControlTypePublic},
				{ID: "pm_2", SchemaID: "drp-schema-v1", FieldName: "person.nic", IsOwner: true},
			},
			"count": 2,
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient(server.URL, "token")
	fields, err := client.ListSchemaPolicyMetadata(context.Background(), "drp-schema-v1")

	assert.NoError(t, err)
	assert.Equal(t, "/api/v1/schemas/drp-schema-v1/policy-metadata", capturedPath)
	assert.Equal(t, http.MethodGet, capturedMethod)
	assert.Equal(t, 2, fields.Count)
	if assert.Len(t, fields.Items, 2) {
		assert.Equal(t, "person.fullName", fields.Items[0].FieldName)
		assert.True(t, fields.Items[1].IsOwner)
	}
}

func TestListSchemaPolicyMetadata_ErrorResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "schema not found"})
	}))
	defer server.Close()

	client := NewClient(server.URL, "token")
	fields, err := client.ListSchemaPolicyMetadata(context.Background(), "missing")

	assert.Error(t, err)
	assert.Nil(t, fields)
	assert.ErrorContains(t, err, "status 404")
}

func TestCreateApplication_Success(t *testing.T) {
	var capturedMethod, capturedPath string
	var capturedBody application.CreateApplicationRequest

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedMethod = r.Method
		capturedPath = r.URL.Path
		assert.NoError(t, json.NewDecoder(r.Body).Decode(&capturedBody))

		resp := application.ApplicationResponse{
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
	req := &application.CreateApplicationRequest{
		ApplicationName: "New App",
		SelectedFields: []policy.SelectedFieldRecord{
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
	resp, err := client.CreateApplication(context.Background(), &application.CreateApplicationRequest{
		ApplicationName: "New App",
		SelectedFields:  []policy.SelectedFieldRecord{{FieldName: "email", SchemaID: "schema-1"}},
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

		resp := application.ApplicationResponse{
			ApplicationID:   "app_123",
			ApplicationName: "Test App",
			SelectedFields: []policy.SelectedFieldRecord{
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
			"items": []application.ApplicationResponse{
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
		_ = json.NewEncoder(w).Encode(map[string]any{"items": []application.ApplicationResponse{}, "count": 0})
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
		_ = json.NewEncoder(w).Encode(map[string]any{"items": []application.ApplicationResponse{}, "count": 0})
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
	var capturedBody application.UpdateApplicationPolicyRequest

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedAuth = r.Header.Get("Authorization")
		capturedPath = r.URL.Path
		capturedMethod = r.Method
		assert.NoError(t, json.NewDecoder(r.Body).Decode(&capturedBody))

		resp := application.ApplicationResponse{
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
	grantDuration := policy.GrantDurationTypeOneYear
	req := &application.UpdateApplicationPolicyRequest{
		SelectedFields: []policy.SelectedFieldRecord{
			{FieldName: "email", SchemaID: "schema-1"},
		},
		GrantDuration: &grantDuration,
	}

	resp, err := client.UpdateApplicationPolicy(context.Background(), "app_123", req)

	assert.NoError(t, err)
	if assert.NotNil(t, resp) {
		assert.Equal(t, "app_123", resp.ApplicationID)
		assert.Equal(t, req.SelectedFields, []policy.SelectedFieldRecord(resp.SelectedFields))
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
		_ = json.NewEncoder(w).Encode(application.ApplicationResponse{ApplicationID: "app_1"})
	}))
	defer server.Close()

	client := NewClient(server.URL+"/", "token")
	_, err := client.UpdateApplicationPolicy(context.Background(), "app_1", &application.UpdateApplicationPolicyRequest{
		SelectedFields: []policy.SelectedFieldRecord{{FieldName: "f", SchemaID: "s"}},
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
	resp, err := client.UpdateApplicationPolicy(context.Background(), "app_1", &application.UpdateApplicationPolicyRequest{
		SelectedFields: []policy.SelectedFieldRecord{{FieldName: "f", SchemaID: "s"}},
	})

	assert.Error(t, err)
	assert.Nil(t, resp)
	assert.ErrorContains(t, err, "status 403")
	assert.ErrorContains(t, err, "Insufficient permissions")
}

func TestUpdateApplicationPolicy_Unreachable(t *testing.T) {
	client := NewClient("http://127.0.0.1:1", "token")
	resp, err := client.UpdateApplicationPolicy(context.Background(), "app_1", &application.UpdateApplicationPolicyRequest{
		SelectedFields: []policy.SelectedFieldRecord{{FieldName: "f", SchemaID: "s"}},
	})

	assert.Error(t, err)
	assert.Nil(t, resp)
}
