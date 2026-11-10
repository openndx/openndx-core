package application

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/openndx/openndx-core/internal/pb/database/dbtest"
	"github.com/openndx/openndx-core/internal/pb/idp"
	"github.com/openndx/openndx-core/internal/pb/idp/idptest"
	"github.com/openndx/openndx-core/internal/pb/kernel"
	"github.com/openndx/openndx-core/internal/pb/policy"
	"github.com/stretchr/testify/assert"
	"gorm.io/gorm"
)

func TestApplicationService_CreateApplication(t *testing.T) {
	t.Run("CreateApplication_Success", func(t *testing.T) {
		db, mock, cleanup := dbtest.SetupMockDB(t)
		defer cleanup()

		// Mock PDP, capturing the grant request so we can assert the path and key.
		var capturedAllowList policy.AccessGrantCreateRequest
		var capturedRequest string
		mockTransport := &MockRoundTripper{
			RoundTripFunc: func(req *http.Request) (*http.Response, error) {
				capturedRequest = req.Method + " " + req.URL.Path
				if req.Body != nil {
					body, _ := io.ReadAll(req.Body)
					_ = json.Unmarshal(body, &capturedAllowList)
				}
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(bytes.NewBufferString(`{"records": [{"id": "policy_1"}]}`)),
					Header:     make(http.Header),
				}, nil
			},
		}
		pdpService := policy.NewClient("http://mock-pdp")
		pdpService.HTTPClient = &http.Client{Transport: mockTransport}

		var capturedIdpApp *idp.Application
		mockIDP := &idptest.Mock{
			CreateApplicationFunc: func(ctx context.Context, app *idp.Application) (*idp.CreatedApplication, error) {
				capturedIdpApp = app
				return &idp.CreatedApplication{Id: "mock-idp-app-id", ClientId: "mock-client-id", ClientSecret: "mock-client-secret"}, nil
			},
		}
		service := NewService(db, pdpService, mockIDP)

		desc := "Test Description"
		req := &CreateApplicationRequest{
			ApplicationName:        "Test Application",
			ApplicationDescription: &desc,
			SelectedFields: []policy.SelectedFieldRecord{
				{FieldName: "field1", SchemaID: "schema-123"},
			},
			MemberID: "member-123",
		}

		// Mock DB expectations
		expectMemberOuLookup(mock, "ou-123")
		mock.ExpectQuery(`INSERT INTO "applications"`).
			WillReturnRows(sqlmock.NewRows([]string{"application_id"}).AddRow("app_123"))

		// Act
		// Note: CreateApplication returns error if PDP fails. Here PDP succeeds.
		// However, CreateApplication returns nil, error if successful? No, it returns response, nil.
		// Wait, the original test expected error because PDP failed. Now we mock PDP success.
		// Let's check CreateApplication implementation.
		// It returns *ApplicationResponse, error.

		resp, err := service.CreateApplication(context.Background(), req)

		// Assert
		assert.NoError(t, err)
		assert.NotNil(t, resp)
		if resp != nil {
			assert.Equal(t, req.ApplicationName, resp.ApplicationName)
			assert.Equal(t, req.MemberID, resp.MemberID)
			// The IDP client secret is returned once, in the create response.
			if assert.NotNil(t, resp.IdpClientSecret) {
				assert.Equal(t, "mock-client-secret", *resp.IdpClientSecret)
			}
		}

		// The IDP application is created in the owning member's organization unit.
		if assert.NotNil(t, capturedIdpApp) {
			assert.Equal(t, "ou-123", capturedIdpApp.OuID)
		}

		// PDP grants must be keyed by the IdP OIDC client_id (the identity OE
		// presents from the token), not the portal's random ApplicationID UUID. See #447.
		assert.Equal(t, "POST /api/v1/policy/grants", capturedRequest)
		assert.Equal(t, "mock-client-id", capturedAllowList.ApplicationID)

		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("CreateApplication_PDPFailure_Compensation", func(t *testing.T) {
		db, mock, cleanup := dbtest.SetupMockDB(t)
		defer cleanup()

		// Mock PDP failure
		mockTransport := &MockRoundTripper{
			RoundTripFunc: func(req *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusInternalServerError,
					Body:       io.NopCloser(bytes.NewBufferString(`{"error": "pdp error"}`)),
					Header:     make(http.Header),
				}, nil
			},
		}
		pdpService := policy.NewClient("http://mock-pdp")
		pdpService.HTTPClient = &http.Client{Transport: mockTransport}

		mockIDP := &idptest.Mock{}
		service := NewService(db, pdpService, mockIDP)

		desc := "Test Description"
		req := &CreateApplicationRequest{
			ApplicationName:        "Test Application",
			ApplicationDescription: &desc,
			SelectedFields: []policy.SelectedFieldRecord{
				{FieldName: "field1", SchemaID: "schema-123"},
			},
			MemberID: "member-123",
		}

		// Mock DB expectations
		// 1. Create application
		expectMemberOuLookup(mock, "ou-123")
		mock.ExpectQuery(`INSERT INTO "applications"`).
			WillReturnRows(sqlmock.NewRows([]string{"application_id"}).AddRow("app_123"))

		// 2. Compensation: Delete application
		mock.ExpectExec(`DELETE FROM "applications"`).
			WillReturnResult(sqlmock.NewResult(0, 1))

		// Act
		resp, err := service.CreateApplication(context.Background(), req)

		// Assert
		assert.Error(t, err)
		assert.Nil(t, resp)
		assert.Contains(t, err.Error(), "failed to update allow list")

		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("CreateApplication_MemberNotFound", func(t *testing.T) {
		db, mock, cleanup := dbtest.SetupMockDB(t)
		defer cleanup()

		mockIDP := &idptest.Mock{
			CreateApplicationFunc: func(ctx context.Context, app *idp.Application) (*idp.CreatedApplication, error) {
				t.Fatal("CreateApplication should not be called when the member doesn't exist")
				return nil, nil
			},
		}
		service := NewService(db, policy.NewClient("http://mock-pdp"), mockIDP)

		mock.ExpectQuery(`SELECT "ou_id" FROM "members"`).
			WillReturnRows(sqlmock.NewRows([]string{"ou_id"}))

		resp, err := service.CreateApplication(context.Background(), &CreateApplicationRequest{
			ApplicationName: "Test Application",
			SelectedFields:  []policy.SelectedFieldRecord{{FieldName: "field1", SchemaID: "schema-123"}},
			MemberID:        "missing-member",
		})

		assert.Nil(t, resp)
		assert.EqualError(t, err, "member not found")
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestApplicationService_CreateApplication_ExternallyProvisioned(t *testing.T) {
	t.Run("Success_SkipsIdpCreation", func(t *testing.T) {
		db, mock, cleanup := dbtest.SetupMockDB(t)
		defer cleanup()

		var capturedAllowList policy.AccessGrantCreateRequest
		mockTransport := &MockRoundTripper{
			RoundTripFunc: func(req *http.Request) (*http.Response, error) {
				if req.Body != nil {
					body, _ := io.ReadAll(req.Body)
					_ = json.Unmarshal(body, &capturedAllowList)
				}
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(bytes.NewBufferString(`{"records": [{"id": "policy_1"}]}`)),
					Header:     make(http.Header),
				}, nil
			},
		}
		pdpService := policy.NewClient("http://mock-pdp")
		pdpService.HTTPClient = &http.Client{Transport: mockTransport}

		mockIDP := &idptest.Mock{
			// These must never be called for an externally provisioned application.
			CreateApplicationFunc: func(ctx context.Context, app *idp.Application) (*idp.CreatedApplication, error) {
				t.Fatal("CreateApplication should not be called for an externally provisioned application")
				return nil, nil
			},
			GetApplicationOIDCFunc: func(ctx context.Context, applicationID string) (*idp.ApplicationOIDCInfo, error) {
				t.Fatal("GetApplicationOIDC should not be called for an externally provisioned application")
				return nil, nil
			},
		}
		service := NewService(db, pdpService, mockIDP)

		idpAppID := "thunder-app-01900000-0000-7000-8000-0000000000c0"
		idpClientID := "THUNDER_PROVISIONED_CLIENT"
		req := &CreateApplicationRequest{
			ApplicationName: "Manually Onboarded App",
			SelectedFields: []policy.SelectedFieldRecord{
				{FieldName: "field1", SchemaID: "schema-123"},
			},
			MemberID:         "member-123",
			IdpApplicationID: &idpAppID,
			IdpClientID:      &idpClientID,
		}

		mock.ExpectQuery(`INSERT INTO "applications"`).
			WillReturnRows(sqlmock.NewRows([]string{"application_id"}).AddRow("app_123"))

		resp, err := service.CreateApplication(context.Background(), req)

		assert.NoError(t, err)
		assert.NotNil(t, resp)
		if resp != nil {
			assert.Equal(t, idpAppID, *resp.IdpApplicationID)
			assert.Equal(t, idpClientID, *resp.IdpClientID)
		}

		// The allow-list must be keyed by the caller-supplied client ID.
		assert.Equal(t, idpClientID, capturedAllowList.ApplicationID)

		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("Error_OnlyOneIDProvided", func(t *testing.T) {
		db, _, cleanup := dbtest.SetupMockDB(t)
		defer cleanup()

		pdpService := policy.NewClient("http://mock-pdp")
		mockIDP := &idptest.Mock{}
		service := NewService(db, pdpService, mockIDP)

		idpAppID := "thunder-app-01900000-0000-7000-8000-0000000000c0"
		req := &CreateApplicationRequest{
			ApplicationName: "Bad Request App",
			SelectedFields: []policy.SelectedFieldRecord{
				{FieldName: "field1", SchemaID: "schema-123"},
			},
			MemberID:         "member-123",
			IdpApplicationID: &idpAppID,
			// IdpClientID intentionally omitted
		}

		resp, err := service.CreateApplication(context.Background(), req)

		assert.Error(t, err)
		assert.Nil(t, resp)
		assert.Contains(t, err.Error(), "must both be provided together")
	})

	t.Run("Error_EmptyValues", func(t *testing.T) {
		db, _, cleanup := dbtest.SetupMockDB(t)
		defer cleanup()

		pdpService := policy.NewClient("http://mock-pdp")
		mockIDP := &idptest.Mock{}
		service := NewService(db, pdpService, mockIDP)

		empty := ""
		idpClientID := "some-client-id"
		req := &CreateApplicationRequest{
			ApplicationName: "Bad Request App",
			SelectedFields: []policy.SelectedFieldRecord{
				{FieldName: "field1", SchemaID: "schema-123"},
			},
			MemberID:         "member-123",
			IdpApplicationID: &empty,
			IdpClientID:      &idpClientID,
		}

		resp, err := service.CreateApplication(context.Background(), req)

		assert.Error(t, err)
		assert.Nil(t, resp)
		assert.Contains(t, err.Error(), "must not be empty")
	})

	t.Run("PDPFailure_DoesNotAttemptIdpDeletion", func(t *testing.T) {
		db, mock, cleanup := dbtest.SetupMockDB(t)
		defer cleanup()

		mockTransport := &MockRoundTripper{
			RoundTripFunc: func(req *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusInternalServerError,
					Body:       io.NopCloser(bytes.NewBufferString(`{"error": "pdp error"}`)),
					Header:     make(http.Header),
				}, nil
			},
		}
		pdpService := policy.NewClient("http://mock-pdp")
		pdpService.HTTPClient = &http.Client{Transport: mockTransport}

		mockIDP := &idptest.Mock{
			DeleteApplicationFunc: func(ctx context.Context, applicationID string) error {
				t.Fatal("DeleteApplication should not be called for an externally provisioned application")
				return nil
			},
		}
		service := NewService(db, pdpService, mockIDP)

		idpAppID := "thunder-app-01900000-0000-7000-8000-0000000000c0"
		idpClientID := "THUNDER_PROVISIONED_CLIENT"
		req := &CreateApplicationRequest{
			ApplicationName: "Manually Onboarded App",
			SelectedFields: []policy.SelectedFieldRecord{
				{FieldName: "field1", SchemaID: "schema-123"},
			},
			MemberID:         "member-123",
			IdpApplicationID: &idpAppID,
			IdpClientID:      &idpClientID,
		}

		mock.ExpectQuery(`INSERT INTO "applications"`).
			WillReturnRows(sqlmock.NewRows([]string{"application_id"}).AddRow("app_123"))
		// Compensation: DB delete only - no IDP call expected.
		mock.ExpectExec(`DELETE FROM "applications"`).
			WillReturnResult(sqlmock.NewResult(0, 1))

		resp, err := service.CreateApplication(context.Background(), req)

		assert.Error(t, err)
		assert.Nil(t, resp)
		assert.Contains(t, err.Error(), "failed to update allow list")

		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestApplicationService_UpdateApplication(t *testing.T) {
	t.Run("UpdateApplication_Success", func(t *testing.T) {
		db, mock, cleanup := dbtest.SetupMockDB(t)
		defer cleanup()

		pdpService := policy.NewClient("http://mock-pdp")
		mockIDP := &idptest.Mock{}
		service := NewService(db, pdpService, mockIDP)

		// Mock DB expectations
		// 1. First find the application
		mock.ExpectQuery(`SELECT .*`).
			WillReturnRows(sqlmock.NewRows([]string{"application_id", "application_name", "application_description", "member_id", "version"}).
				AddRow("app_123", "Original Name", "Original Description", "member-123", "v1"))

		// 2. Save the updated application
		mock.ExpectExec(`UPDATE "applications"`).
			WillReturnResult(sqlmock.NewResult(0, 1))

		newName := "Updated Name"
		newDesc := "Updated Description"
		req := &UpdateApplicationRequest{
			ApplicationName:        &newName,
			ApplicationDescription: &newDesc,
		}

		result, err := service.UpdateApplication(context.Background(), "app_123", req)

		assert.NoError(t, err)
		assert.NotNil(t, result)
		if result != nil {
			assert.Equal(t, newName, result.ApplicationName)
			if result.ApplicationDescription != nil {
				assert.Equal(t, newDesc, *result.ApplicationDescription)
			}
		}

		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("UpdateApplication_NotFound", func(t *testing.T) {
		db, mock, cleanup := dbtest.SetupMockDB(t)
		defer cleanup()

		pdpService := policy.NewClient("http://mock-pdp")
		mockIDP := &idptest.Mock{}
		service := NewService(db, pdpService, mockIDP)

		// Mock DB expectations - return no rows
		mock.ExpectQuery(`SELECT .*`).
			WillReturnError(gorm.ErrRecordNotFound)

		newName := "Updated Name"
		req := &UpdateApplicationRequest{
			ApplicationName: &newName,
		}

		result, err := service.UpdateApplication(context.Background(), "non-existent-id", req)

		assert.Error(t, err)
		assert.Nil(t, result)
		assert.ErrorIs(t, err, gorm.ErrRecordNotFound)

		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestApplicationService_UpdateApplicationPolicy(t *testing.T) {
	t.Run("UpdateApplicationPolicy_Success_DefaultGrantDuration", func(t *testing.T) {
		db, mock, cleanup := dbtest.SetupMockDB(t)
		defer cleanup()

		var capturedAllowList policy.AccessGrantCreateRequest
		mockTransport := &MockRoundTripper{
			RoundTripFunc: func(req *http.Request) (*http.Response, error) {
				if req.Body != nil {
					body, _ := io.ReadAll(req.Body)
					_ = json.Unmarshal(body, &capturedAllowList)
				}
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(bytes.NewBufferString(`{"records": [{"id": "policy_1"}]}`)),
					Header:     make(http.Header),
				}, nil
			},
		}
		pdpService := policy.NewClient("http://mock-pdp")
		pdpService.HTTPClient = &http.Client{Transport: mockTransport}

		mockIDP := &idptest.Mock{}
		service := NewService(db, pdpService, mockIDP)

		clientID := "idp-client-123"
		mock.ExpectQuery(`SELECT .*`).
			WillReturnRows(sqlmock.NewRows([]string{"application_id", "application_name", "member_id", "version", "idp_client_id"}).
				AddRow("app_123", "Test Application", "member-123", "v1", clientID))

		mock.ExpectExec(`UPDATE "applications"`).
			WillReturnResult(sqlmock.NewResult(0, 1))

		req := &UpdateApplicationPolicyRequest{
			SelectedFields: []policy.SelectedFieldRecord{
				{FieldName: "email", SchemaID: "schema-123"},
			},
		}

		result, err := service.UpdateApplicationPolicy(context.Background(), "app_123", req)

		assert.NoError(t, err)
		assert.NotNil(t, result)
		if result != nil {
			assert.Equal(t, req.SelectedFields, []policy.SelectedFieldRecord(result.SelectedFields))
		}

		// The allow-list is keyed by the IdP client_id, not the portal application ID.
		assert.Equal(t, clientID, capturedAllowList.ApplicationID)
		// GrantDuration defaults to one month when the request omits it.
		assert.Equal(t, policy.GrantDurationTypeOneMonth, capturedAllowList.GrantDuration)

		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("UpdateApplicationPolicy_Success_ExplicitGrantDuration", func(t *testing.T) {
		db, mock, cleanup := dbtest.SetupMockDB(t)
		defer cleanup()

		var capturedAllowList policy.AccessGrantCreateRequest
		mockTransport := &MockRoundTripper{
			RoundTripFunc: func(req *http.Request) (*http.Response, error) {
				if req.Body != nil {
					body, _ := io.ReadAll(req.Body)
					_ = json.Unmarshal(body, &capturedAllowList)
				}
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(bytes.NewBufferString(`{"records": [{"id": "policy_1"}]}`)),
					Header:     make(http.Header),
				}, nil
			},
		}
		pdpService := policy.NewClient("http://mock-pdp")
		pdpService.HTTPClient = &http.Client{Transport: mockTransport}

		mockIDP := &idptest.Mock{}
		service := NewService(db, pdpService, mockIDP)

		clientID := "idp-client-123"
		mock.ExpectQuery(`SELECT .*`).
			WillReturnRows(sqlmock.NewRows([]string{"application_id", "application_name", "member_id", "version", "idp_client_id"}).
				AddRow("app_123", "Test Application", "member-123", "v1", clientID))

		mock.ExpectExec(`UPDATE "applications"`).
			WillReturnResult(sqlmock.NewResult(0, 1))

		grantDuration := policy.GrantDurationTypeOneYear
		req := &UpdateApplicationPolicyRequest{
			SelectedFields: []policy.SelectedFieldRecord{
				{FieldName: "email", SchemaID: "schema-123"},
			},
			GrantDuration: &grantDuration,
		}

		result, err := service.UpdateApplicationPolicy(context.Background(), "app_123", req)

		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, policy.GrantDurationTypeOneYear, capturedAllowList.GrantDuration)

		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("UpdateApplicationPolicy_NotFound", func(t *testing.T) {
		db, mock, cleanup := dbtest.SetupMockDB(t)
		defer cleanup()

		pdpService := policy.NewClient("http://mock-pdp")
		mockIDP := &idptest.Mock{}
		service := NewService(db, pdpService, mockIDP)

		mock.ExpectQuery(`SELECT .*`).
			WillReturnError(gorm.ErrRecordNotFound)

		req := &UpdateApplicationPolicyRequest{
			SelectedFields: []policy.SelectedFieldRecord{
				{FieldName: "email", SchemaID: "schema-123"},
			},
		}

		result, err := service.UpdateApplicationPolicy(context.Background(), "non-existent-id", req)

		assert.Error(t, err)
		assert.Nil(t, result)
		assert.ErrorIs(t, err, gorm.ErrRecordNotFound)

		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("UpdateApplicationPolicy_NilIdpClientID", func(t *testing.T) {
		db, mock, cleanup := dbtest.SetupMockDB(t)
		defer cleanup()

		pdpService := policy.NewClient("http://mock-pdp")
		mockIDP := &idptest.Mock{}
		service := NewService(db, pdpService, mockIDP)

		// idp_client_id column omitted -> IdpClientID stays nil
		mock.ExpectQuery(`SELECT .*`).
			WillReturnRows(sqlmock.NewRows([]string{"application_id", "application_name", "member_id", "version"}).
				AddRow("app_123", "Test Application", "member-123", "v1"))

		req := &UpdateApplicationPolicyRequest{
			SelectedFields: []policy.SelectedFieldRecord{
				{FieldName: "email", SchemaID: "schema-123"},
			},
		}

		result, err := service.UpdateApplicationPolicy(context.Background(), "app_123", req)

		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "IdpClientID is nil")

		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("UpdateApplicationPolicy_PDPFailure_NoDBWrite", func(t *testing.T) {
		db, mock, cleanup := dbtest.SetupMockDB(t)
		defer cleanup()

		mockTransport := &MockRoundTripper{
			RoundTripFunc: func(req *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusInternalServerError,
					Body:       io.NopCloser(bytes.NewBufferString(`{"error": "pdp error"}`)),
					Header:     make(http.Header),
				}, nil
			},
		}
		pdpService := policy.NewClient("http://mock-pdp")
		pdpService.HTTPClient = &http.Client{Transport: mockTransport}

		mockIDP := &idptest.Mock{}
		service := NewService(db, pdpService, mockIDP)

		clientID := "idp-client-123"
		mock.ExpectQuery(`SELECT .*`).
			WillReturnRows(sqlmock.NewRows([]string{"application_id", "application_name", "member_id", "version", "idp_client_id"}).
				AddRow("app_123", "Test Application", "member-123", "v1", clientID))
		// No ExpectExec for UPDATE: a failed PDP call must not touch the DB.

		req := &UpdateApplicationPolicyRequest{
			SelectedFields: []policy.SelectedFieldRecord{
				{FieldName: "email", SchemaID: "schema-123"},
			},
		}

		result, err := service.UpdateApplicationPolicy(context.Background(), "app_123", req)

		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "failed to update allow list")

		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestApplicationService_GetApplication(t *testing.T) {
	t.Run("GetApplication_Success", func(t *testing.T) {
		db, mock, cleanup := dbtest.SetupMockDB(t)
		defer cleanup()

		pdpService := policy.NewClient("http://mock-pdp")
		mockIDP := &idptest.Mock{}
		service := NewService(db, pdpService, mockIDP)

		// Mock DB expectations
		mock.ExpectQuery(`SELECT .*`).
			WillReturnRows(sqlmock.NewRows([]string{"application_id", "application_name", "member_id", "version"}).
				AddRow("app_123", "Test Application", "member-123", "v1"))

		// Preload Member expectation
		mock.ExpectQuery(`SELECT .* FROM "members" WHERE "members"."member_id" = .*`).
			WithArgs("member-123").
			WillReturnRows(sqlmock.NewRows([]string{"member_id", "name"}).
				AddRow("member-123", "Test Member"))

		result, err := service.GetApplication(context.Background(), "app_123")

		assert.NoError(t, err)
		assert.NotNil(t, result)
		if result != nil {
			assert.Equal(t, "app_123", result.ApplicationID)
			assert.Equal(t, "Test Application", result.ApplicationName)
		}

		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("GetApplication_NotFound", func(t *testing.T) {
		db, mock, cleanup := dbtest.SetupMockDB(t)
		defer cleanup()

		pdpService := policy.NewClient("http://mock-pdp")
		mockIDP := &idptest.Mock{}
		service := NewService(db, pdpService, mockIDP)

		// Mock DB expectations
		mock.ExpectQuery(`SELECT .*`).
			WillReturnError(gorm.ErrRecordNotFound)

		result, err := service.GetApplication(context.Background(), "non-existent-id")

		assert.Error(t, err)
		assert.Nil(t, result)
		assert.ErrorIs(t, err, gorm.ErrRecordNotFound)

		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestApplicationService_GetApplications(t *testing.T) {
	t.Run("GetApplications_NoFilter", func(t *testing.T) {
		db, mock, cleanup := dbtest.SetupMockDB(t)
		defer cleanup()

		pdpService := policy.NewClient("http://mock-pdp")
		mockIDP := &idptest.Mock{}
		service := NewService(db, pdpService, mockIDP)

		// Mock DB expectations
		mock.ExpectQuery(`SELECT .* FROM "applications" ORDER BY created_at DESC`).
			WillReturnRows(sqlmock.NewRows([]string{"application_id", "application_name", "member_id", "version"}).
				AddRow("app_1", "App 1", "member-1", "v1").
				AddRow("app_2", "App 2", "member-2", "v1"))

		// Preload Member expectation (for each application)
		// Note: GORM might batch these or do them individually depending on version/config
		// With Preload, it typically does IN query
		mock.ExpectQuery(`SELECT .* FROM "members" WHERE "members"."member_id" IN .*`).
			WithArgs("member-1", "member-2").
			WillReturnRows(sqlmock.NewRows([]string{"member_id", "name"}).
				AddRow("member-1", "Member 1").
				AddRow("member-2", "Member 2"))

		result, err := service.GetApplications(context.Background(), nil)

		assert.NoError(t, err)
		assert.Len(t, result, 2)

		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("GetApplications_WithMemberIDFilter", func(t *testing.T) {
		db, mock, cleanup := dbtest.SetupMockDB(t)
		defer cleanup()

		pdpService := policy.NewClient("http://mock-pdp")
		mockIDP := &idptest.Mock{}
		service := NewService(db, pdpService, mockIDP)

		memberID := "member-123"

		// Mock DB expectations
		mock.ExpectQuery(`SELECT .* FROM "applications" WHERE member_id = .* ORDER BY created_at DESC`).
			WithArgs(memberID).
			WillReturnRows(sqlmock.NewRows([]string{"application_id", "application_name", "member_id", "version"}).
				AddRow("app_1", "App 1", memberID, "v1"))

		// Preload Member expectation
		mock.ExpectQuery(`SELECT .* FROM "members" WHERE "members"."member_id" = .*`).
			WithArgs(memberID).
			WillReturnRows(sqlmock.NewRows([]string{"member_id", "name"}).
				AddRow(memberID, "Member 1"))

		result, err := service.GetApplications(context.Background(), &memberID)

		assert.NoError(t, err)
		assert.Len(t, result, 1)
		assert.Equal(t, memberID, result[0].MemberID)

		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestApplicationService_CreateApplicationSubmission(t *testing.T) {
	t.Run("CreateApplicationSubmission_Success", func(t *testing.T) {
		db, mock, cleanup := dbtest.SetupMockDB(t)
		defer cleanup()

		pdpService := policy.NewClient("http://mock-pdp")
		mockIDP := &idptest.Mock{}
		service := NewService(db, pdpService, mockIDP)

		// Mock DB expectations
		// 1. Validate member
		mock.ExpectQuery(`SELECT .*`).
			WillReturnRows(sqlmock.NewRows([]string{"member_id", "name"}).AddRow("member-123", "Test Member"))

		// 2. Create submission
		mock.ExpectQuery(`INSERT INTO .*`).
			WillReturnRows(sqlmock.NewRows([]string{"submission_id"}).AddRow("sub_123"))

		desc := "Test Description"
		req := &CreateApplicationSubmissionRequest{
			ApplicationName:        "Test Submission",
			ApplicationDescription: &desc,
			SelectedFields: []policy.SelectedFieldRecord{
				{FieldName: "field1", SchemaID: "schema-123"},
			},
			MemberID: "member-123",
		}

		result, err := service.CreateApplicationSubmission(context.Background(), req)

		assert.NoError(t, err)
		assert.NotNil(t, result)
		if result != nil {
			assert.Equal(t, req.ApplicationName, result.ApplicationName)
			assert.Equal(t, string(kernel.StatusPending), result.Status)
		}

		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("CreateApplicationSubmission_MemberNotFound", func(t *testing.T) {
		db, mock, cleanup := dbtest.SetupMockDB(t)
		defer cleanup()

		pdpService := policy.NewClient("http://mock-pdp")
		mockIDP := &idptest.Mock{}
		service := NewService(db, pdpService, mockIDP)

		// Mock DB expectations
		mock.ExpectQuery(`SELECT .*`).
			WillReturnError(gorm.ErrRecordNotFound)

		desc := "Test Description"
		req := &CreateApplicationSubmissionRequest{
			ApplicationName:        "Test Submission",
			ApplicationDescription: &desc,
			SelectedFields: []policy.SelectedFieldRecord{
				{FieldName: "field1", SchemaID: "schema-123"},
			},
			MemberID: "non-existent-member",
		}

		result, err := service.CreateApplicationSubmission(context.Background(), req)

		assert.Error(t, err)
		assert.Nil(t, result)
		assert.ErrorIs(t, err, gorm.ErrRecordNotFound)

		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestApplicationService_UpdateApplicationSubmission(t *testing.T) {
	t.Run("UpdateApplicationSubmission_Success", func(t *testing.T) {
		db, mock, cleanup := dbtest.SetupMockDB(t)
		defer cleanup()

		pdpService := policy.NewClient("http://mock-pdp")
		mockIDP := &idptest.Mock{}
		service := NewService(db, pdpService, mockIDP)

		// Mock DB expectations
		// 1. Find submission
		mock.ExpectQuery(`SELECT .*`).
			WillReturnRows(sqlmock.NewRows([]string{"submission_id", "application_name", "member_id", "status"}).
				AddRow("sub_123", "Original", "member-123", string(kernel.StatusPending)))

		// 2. Save submission
		mock.ExpectExec(`UPDATE "application_submissions"`).
			WillReturnResult(sqlmock.NewResult(0, 1))

		newName := "Updated"
		req := &UpdateApplicationSubmissionRequest{
			ApplicationName: &newName,
		}

		result, err := service.UpdateApplicationSubmission(context.Background(), "sub_123", req)

		assert.NoError(t, err)
		assert.NotNil(t, result)
		if result != nil {
			assert.Equal(t, newName, result.ApplicationName)
		}

		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("UpdateApplicationSubmission_NotFound", func(t *testing.T) {
		db, mock, cleanup := dbtest.SetupMockDB(t)
		defer cleanup()

		pdpService := policy.NewClient("http://mock-pdp")
		mockIDP := &idptest.Mock{}
		service := NewService(db, pdpService, mockIDP)

		// Mock DB expectations
		mock.ExpectQuery(`SELECT .*`).
			WillReturnError(gorm.ErrRecordNotFound)

		updatedName := "Updated"
		req := &UpdateApplicationSubmissionRequest{ApplicationName: &updatedName}
		result, err := service.UpdateApplicationSubmission(context.Background(), "non-existent", req)

		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "application submission not found")

		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("UpdateApplicationSubmission_ApprovalWithApplicationCreationFailure", func(t *testing.T) {
		db, mock, cleanup := dbtest.SetupMockDB(t)
		defer cleanup()

		// Mock PDP failure
		mockTransport := &MockRoundTripper{
			RoundTripFunc: func(req *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusInternalServerError,
					Body:       io.NopCloser(bytes.NewBufferString(`{"error": "pdp error"}`)),
					Header:     make(http.Header),
				}, nil
			},
		}
		pdpService := policy.NewClient("http://mock-pdp")
		pdpService.HTTPClient = &http.Client{Transport: mockTransport}

		mockIDP := &idptest.Mock{}
		service := NewService(db, pdpService, mockIDP)

		// Mock DB expectations
		// 1. Find submission
		mock.ExpectQuery(`SELECT .*`).
			WillReturnRows(sqlmock.NewRows([]string{"submission_id", "application_name", "member_id", "status"}).
				AddRow("sub_123", "Original", "member-123", string(kernel.StatusPending)))

		// 2. Save submission (status update to Approved)
		mock.ExpectExec(`UPDATE "application_submissions"`).
			WillReturnResult(sqlmock.NewResult(0, 1))

		// 3. Create Application (will fail on PDP)
		expectMemberOuLookup(mock, "ou-123")
		mock.ExpectQuery(`INSERT INTO "applications"`).
			WillReturnRows(sqlmock.NewRows([]string{"application_id"}).AddRow("app_new"))

		// 4. Compensation: Delete application (from CreateApplication compensation)
		mock.ExpectExec(`DELETE FROM "applications"`).
			WillReturnResult(sqlmock.NewResult(0, 1))

		// 5. Compensation: Update submission status back to Pending
		mock.ExpectExec(`UPDATE "application_submissions"`).
			WillReturnResult(sqlmock.NewResult(0, 1))

		status := string(kernel.StatusApproved)
		req := &UpdateApplicationSubmissionRequest{
			Status: &status,
		}

		result, err := service.UpdateApplicationSubmission(context.Background(), "sub_123", req)

		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "failed to create application")

		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestApplicationService_GetApplicationSubmission(t *testing.T) {
	t.Run("GetApplicationSubmission_Success", func(t *testing.T) {
		db, mock, cleanup := dbtest.SetupMockDB(t)
		defer cleanup()

		pdpService := policy.NewClient("http://mock-pdp")
		mockIDP := &idptest.Mock{}
		service := NewService(db, pdpService, mockIDP)

		// Mock DB expectations
		mock.ExpectQuery(`SELECT .*`).
			WillReturnRows(sqlmock.NewRows([]string{"submission_id", "application_name", "member_id", "status"}).
				AddRow("sub_123", "Test Submission", "member-123", string(kernel.StatusPending)))

		// Preload Member
		mock.ExpectQuery(`SELECT .* FROM "members" WHERE "members"."member_id" = .*`).
			WithArgs("member-123").
			WillReturnRows(sqlmock.NewRows([]string{"member_id", "name"}).AddRow("member-123", "Test Member"))

		// Preload PreviousApplication (none)
		// Note: GORM might not execute this query if PreviousApplicationID is null in the struct returned above
		// But if it does, we should expect it. Let's see.
		// If PreviousApplicationID is null, GORM usually skips.

		result, err := service.GetApplicationSubmission(context.Background(), "sub_123")

		assert.NoError(t, err)
		assert.NotNil(t, result)
		if result != nil {
			assert.Equal(t, "sub_123", result.SubmissionID)
			assert.Equal(t, "Test Submission", result.ApplicationName)
		}

		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("GetApplicationSubmission_NotFound", func(t *testing.T) {
		db, mock, cleanup := dbtest.SetupMockDB(t)
		defer cleanup()

		pdpService := policy.NewClient("http://mock-pdp")
		mockIDP := &idptest.Mock{}
		service := NewService(db, pdpService, mockIDP)

		// Mock DB expectations
		mock.ExpectQuery(`SELECT .*`).
			WillReturnError(gorm.ErrRecordNotFound)

		result, err := service.GetApplicationSubmission(context.Background(), "non-existent")

		assert.Error(t, err)
		assert.Nil(t, result)
		assert.ErrorIs(t, err, gorm.ErrRecordNotFound)

		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestApplicationService_GetApplicationSubmissions(t *testing.T) {
	t.Run("GetApplicationSubmissions_NoFilter", func(t *testing.T) {
		db, mock, cleanup := dbtest.SetupMockDB(t)
		defer cleanup()

		pdpService := policy.NewClient("http://mock-pdp")
		mockIDP := &idptest.Mock{}
		service := NewService(db, pdpService, mockIDP)

		// Mock DB expectations
		mock.ExpectQuery(`SELECT .*`).
			WillReturnRows(sqlmock.NewRows([]string{"submission_id", "application_name", "member_id", "status"}).
				AddRow("sub_1", "Sub 1", "member-1", string(kernel.StatusPending)).
				AddRow("sub_2", "Sub 2", "member-1", string(kernel.StatusPending)))

		// Preload Member
		mock.ExpectQuery(`SELECT .*`).
			WillReturnRows(sqlmock.NewRows([]string{"member_id", "name"}).AddRow("member-1", "Test Member"))

		// Preload PreviousApplication (none)

		result, err := service.GetApplicationSubmissions(context.Background(), nil, nil)

		assert.NoError(t, err)
		assert.Len(t, result, 2)

		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("GetApplicationSubmissions_WithMemberIDFilter", func(t *testing.T) {
		db, mock, cleanup := dbtest.SetupMockDB(t)
		defer cleanup()

		pdpService := policy.NewClient("http://mock-pdp")
		mockIDP := &idptest.Mock{}
		service := NewService(db, pdpService, mockIDP)

		memberID := "member-123"

		// Mock DB expectations
		mock.ExpectQuery(`SELECT .* FROM "application_submissions" WHERE member_id = .* ORDER BY created_at DESC`).
			WithArgs(memberID).
			WillReturnRows(sqlmock.NewRows([]string{"submission_id", "application_name", "member_id", "status"}).
				AddRow("sub_1", "Sub 1", memberID, string(kernel.StatusPending)))

		// Preload Member
		mock.ExpectQuery(`SELECT .* FROM "members" WHERE "members"."member_id" = .*`).
			WithArgs(memberID).
			WillReturnRows(sqlmock.NewRows([]string{"member_id", "name"}).AddRow(memberID, "Test Member"))

		result, err := service.GetApplicationSubmissions(context.Background(), &memberID, nil)

		assert.NoError(t, err)
		assert.Len(t, result, 1)
		assert.Equal(t, memberID, result[0].MemberID)

		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("GetApplicationSubmissions_WithStatusFilter", func(t *testing.T) {
		db, mock, cleanup := dbtest.SetupMockDB(t)
		defer cleanup()

		pdpService := policy.NewClient("http://mock-pdp")
		mockIDP := &idptest.Mock{}
		service := NewService(db, pdpService, mockIDP)

		statusFilter := []string{string(kernel.StatusApproved)}

		// Mock DB expectations
		mock.ExpectQuery(`SELECT .* FROM "application_submissions" WHERE status IN .* ORDER BY created_at DESC`).
			WithArgs(statusFilter[0]).
			WillReturnRows(sqlmock.NewRows([]string{"submission_id", "application_name", "member_id", "status"}).
				AddRow("sub_2", "Sub 2", "member-123", string(kernel.StatusApproved)))

		// Preload Member
		mock.ExpectQuery(`SELECT .* FROM "members" WHERE "members"."member_id" = .*`).
			WithArgs("member-123").
			WillReturnRows(sqlmock.NewRows([]string{"member_id", "name"}).AddRow("member-123", "Test Member"))

		result, err := service.GetApplicationSubmissions(context.Background(), nil, &statusFilter)

		assert.NoError(t, err)
		if len(result) > 0 {
			assert.Equal(t, string(kernel.StatusApproved), result[0].Status)
		}

		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestApplicationService_CreateApplication_EdgeCases(t *testing.T) {
	t.Run("CreateApplication_EmptySelectedFields", func(t *testing.T) {
		db, mock, cleanup := dbtest.SetupMockDB(t)
		defer cleanup()

		pdpService := policy.NewClient("http://mock-pdp")
		mockIDP := &idptest.Mock{}
		service := NewService(db, pdpService, mockIDP)

		req := &CreateApplicationRequest{
			ApplicationName: "Test Application",
			SelectedFields:  []policy.SelectedFieldRecord{},
			MemberID:        "member-123",
		}

		// Mock DB expectations
		// 1. Create application
		expectMemberOuLookup(mock, "ou-123")
		mock.ExpectQuery(`INSERT INTO "applications"`).
			WillReturnRows(sqlmock.NewRows([]string{"application_id"}).AddRow("app_123"))

		// 2. Compensation: Delete application (because empty fields might cause PDP error or logic error)
		// Wait, empty selected fields might be valid for DB but PDP might reject?
		// The original test expected error.
		// If PDP service is mocked to return error (or if logic checks for empty fields), then compensation happens.
		// Let's assume PDP returns error for empty fields if we mock it that way.
		// Or if the logic itself checks.
		// The original test said "Will fail on PDP call but tests the request structure".
		// So we should mock PDP failure.

		mockTransport := &MockRoundTripper{
			RoundTripFunc: func(req *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusBadRequest,
					Body:       io.NopCloser(bytes.NewBufferString(`{"error": "empty fields"}`)),
					Header:     make(http.Header),
				}, nil
			},
		}
		pdpService.HTTPClient = &http.Client{Transport: mockTransport}

		mock.ExpectExec(`DELETE FROM "applications"`).
			WillReturnResult(sqlmock.NewResult(0, 1))

		_, err := service.CreateApplication(context.Background(), req)
		assert.Error(t, err)

		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestApplicationService_UpdateApplication_EdgeCases(t *testing.T) {
	t.Run("UpdateApplication_PartialUpdate", func(t *testing.T) {
		db, mock, cleanup := dbtest.SetupMockDB(t)
		defer cleanup()

		pdpService := policy.NewClient("http://mock-pdp")
		mockIDP := &idptest.Mock{}
		service := NewService(db, pdpService, mockIDP)

		// Mock DB expectations
		// 1. Find application
		mock.ExpectQuery(`SELECT .*`).
			WillReturnRows(sqlmock.NewRows([]string{"application_id", "application_name", "application_description", "member_id", "version"}).
				AddRow("app_123", "Original Name", "Original Description", "member-123", "v1"))

		// 2. Save application
		mock.ExpectExec(`UPDATE "applications"`).
			WillReturnResult(sqlmock.NewResult(0, 1))

		newName := "Updated Name Only"
		req := &UpdateApplicationRequest{
			ApplicationName: &newName,
		}

		result, err := service.UpdateApplication(context.Background(), "app_123", req)

		assert.NoError(t, err)
		assert.NotNil(t, result)
		if result != nil {
			assert.Equal(t, newName, result.ApplicationName)
			// Original description should remain
			// Note: The mock returned "Original Description", so result should have it if logic preserves it
			if result.ApplicationDescription != nil {
				assert.Equal(t, "Original Description", *result.ApplicationDescription)
			}
		}

		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestApplicationService_CreateApplicationSubmission_EdgeCases(t *testing.T) {
	t.Run("CreateApplicationSubmission_WithPreviousApplicationID", func(t *testing.T) {
		db, mock, cleanup := dbtest.SetupMockDB(t)
		defer cleanup()

		pdpService := policy.NewClient("http://mock-pdp")
		mockIDP := &idptest.Mock{}
		service := NewService(db, pdpService, mockIDP)

		// Mock DB expectations
		// 1. Validate previous application
		mock.ExpectQuery(`SELECT .*`).
			WillReturnRows(sqlmock.NewRows([]string{"application_id"}).AddRow("app_prev"))

		// 2. Validate member
		mock.ExpectQuery(`SELECT .*`).
			WillReturnRows(sqlmock.NewRows([]string{"member_id"}).AddRow("member-123"))

		// 3. Create submission
		mock.ExpectQuery(`INSERT INTO .*`).
			WillReturnRows(sqlmock.NewRows([]string{"submission_id"}).AddRow("sub_123"))

		prevAppID := "app_prev"
		desc := "Test Description"
		req := &CreateApplicationSubmissionRequest{
			ApplicationName:        "Test Submission",
			ApplicationDescription: &desc,
			SelectedFields: []policy.SelectedFieldRecord{
				{FieldName: "field1", SchemaID: "schema-123"},
			},
			MemberID:              "member-123",
			PreviousApplicationID: &prevAppID,
		}

		result, err := service.CreateApplicationSubmission(context.Background(), req)

		assert.NoError(t, err)
		assert.NotNil(t, result)
		if result != nil && result.PreviousApplicationID != nil {
			assert.Equal(t, prevAppID, *result.PreviousApplicationID)
		}

		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("CreateApplicationSubmission_InvalidPreviousApplicationID", func(t *testing.T) {
		db, mock, cleanup := dbtest.SetupMockDB(t)
		defer cleanup()

		pdpService := policy.NewClient("http://mock-pdp")
		mockIDP := &idptest.Mock{}
		service := NewService(db, pdpService, mockIDP)

		// Mock DB expectations
		// 1. Validate previous application
		mock.ExpectQuery(`SELECT .*`).
			WillReturnError(gorm.ErrRecordNotFound)

		invalidAppID := "non-existent-app"
		req := &CreateApplicationSubmissionRequest{
			ApplicationName:       "New Submission",
			SelectedFields:        []policy.SelectedFieldRecord{{FieldName: "field1", SchemaID: "schema-123"}},
			MemberID:              "member-123",
			PreviousApplicationID: &invalidAppID,
		}

		result, err := service.CreateApplicationSubmission(context.Background(), req)

		assert.Error(t, err)
		assert.Nil(t, result)
		assert.ErrorIs(t, err, gorm.ErrRecordNotFound)

		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestApplicationService_ApplicationIDsByClientID(t *testing.T) {
	db := setupSQLiteTestDB(t)
	memberID := createTestMember(t, db, "client-lookup@example.com")
	policeApp := createTestApplicationWithClientID(t, db, memberID, "police-client")
	taxApp := createTestApplicationWithClientID(t, db, memberID, "tax-client")
	service := NewService(db, policy.NewClient("http://mock-pdp"), &idptest.Mock{})
	ctx := context.Background()

	t.Run("one client", func(t *testing.T) {
		applicationID, err := service.ApplicationIDByClientID(ctx, "police-client")
		assert.NoError(t, err)
		assert.Equal(t, policeApp, applicationID)
	})

	t.Run("unknown client is empty, not an error", func(t *testing.T) {
		applicationID, err := service.ApplicationIDByClientID(ctx, "unknown-client")
		assert.NoError(t, err)
		assert.Equal(t, "", applicationID)
	})

	t.Run("many clients leave out unknown ones", func(t *testing.T) {
		byClientID, err := service.ApplicationIDsByClientIDs(ctx, []string{"police-client", "tax-client", "unknown-client"})
		assert.NoError(t, err)
		assert.Equal(t, map[string]string{"police-client": policeApp, "tax-client": taxApp}, byClientID)
	})

	t.Run("no clients", func(t *testing.T) {
		byClientID, err := service.ApplicationIDsByClientIDs(ctx, nil)
		assert.NoError(t, err)
		assert.Empty(t, byClientID)
	})
}
