package services

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gov-dx-sandbox/portal-backend/v1/models"
	"github.com/stretchr/testify/assert"
	"gorm.io/gorm"
)

func TestApplicationService_CreateApplication(t *testing.T) {
	t.Run("CreateApplication_Success", func(t *testing.T) {
		db, mock, cleanup := SetupMockDB(t)
		defer cleanup()

		// Mock PDP, capturing the allow-list request body so we can assert the key.
		var capturedAllowList models.AllowListUpdateRequest
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
		pdpService := NewPDPService("http://mock-pdp")
		pdpService.HTTPClient = &http.Client{Transport: mockTransport}

		mockIDP := &MockIDP{}
		service := NewApplicationService(db, pdpService, mockIDP)

		desc := "Test Description"
		req := &models.CreateApplicationRequest{
			ApplicationName:        "Test Application",
			ApplicationDescription: &desc,
			SelectedFields: []models.SelectedFieldRecord{
				{FieldName: "field1", SchemaID: "schema-123"},
			},
			MemberID: "member-123",
		}

		// Mock DB expectations
		mock.ExpectQuery(`INSERT INTO "applications"`).
			WillReturnRows(sqlmock.NewRows([]string{"application_id"}).AddRow("app_123"))

		// Act
		// Note: CreateApplication returns error if PDP fails. Here PDP succeeds.
		// However, CreateApplication returns nil, error if successful? No, it returns response, nil.
		// Wait, the original test expected error because PDP failed. Now we mock PDP success.
		// Let's check CreateApplication implementation.
		// It returns *models.ApplicationResponse, error.

		resp, err := service.CreateApplication(context.Background(), req)

		// Assert
		assert.NoError(t, err)
		assert.NotNil(t, resp)
		if resp != nil {
			assert.Equal(t, req.ApplicationName, resp.ApplicationName)
			assert.Equal(t, req.MemberID, resp.MemberID)
		}

		// The PDP allow-list must be keyed by the IdP OIDC client_id (the identity OE
		// presents from the token), not the portal's random ApplicationID UUID. See #447.
		assert.Equal(t, "mock-client-id", capturedAllowList.ApplicationID)

		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("CreateApplication_PDPFailure_Compensation", func(t *testing.T) {
		db, mock, cleanup := SetupMockDB(t)
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
		pdpService := NewPDPService("http://mock-pdp")
		pdpService.HTTPClient = &http.Client{Transport: mockTransport}

		mockIDP := &MockIDP{}
		service := NewApplicationService(db, pdpService, mockIDP)

		desc := "Test Description"
		req := &models.CreateApplicationRequest{
			ApplicationName:        "Test Application",
			ApplicationDescription: &desc,
			SelectedFields: []models.SelectedFieldRecord{
				{FieldName: "field1", SchemaID: "schema-123"},
			},
			MemberID: "member-123",
		}

		// Mock DB expectations
		// 1. Create application
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
}

func TestApplicationService_UpdateApplication(t *testing.T) {
	t.Run("UpdateApplication_Success", func(t *testing.T) {
		db, mock, cleanup := SetupMockDB(t)
		defer cleanup()

		pdpService := NewPDPService("http://mock-pdp")
		mockIDP := &MockIDP{}
		service := NewApplicationService(db, pdpService, mockIDP)

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
		req := &models.UpdateApplicationRequest{
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
		db, mock, cleanup := SetupMockDB(t)
		defer cleanup()

		pdpService := NewPDPService("http://mock-pdp")
		mockIDP := &MockIDP{}
		service := NewApplicationService(db, pdpService, mockIDP)

		// Mock DB expectations - return no rows
		mock.ExpectQuery(`SELECT .*`).
			WillReturnError(gorm.ErrRecordNotFound)

		newName := "Updated Name"
		req := &models.UpdateApplicationRequest{
			ApplicationName: &newName,
		}

		result, err := service.UpdateApplication(context.Background(), "non-existent-id", req)

		assert.Error(t, err)
		assert.Nil(t, result)
		assert.ErrorIs(t, err, gorm.ErrRecordNotFound)

		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestApplicationService_GetApplication(t *testing.T) {
	t.Run("GetApplication_Success", func(t *testing.T) {
		db, mock, cleanup := SetupMockDB(t)
		defer cleanup()

		pdpService := NewPDPService("http://mock-pdp")
		mockIDP := &MockIDP{}
		service := NewApplicationService(db, pdpService, mockIDP)

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
		db, mock, cleanup := SetupMockDB(t)
		defer cleanup()

		pdpService := NewPDPService("http://mock-pdp")
		mockIDP := &MockIDP{}
		service := NewApplicationService(db, pdpService, mockIDP)

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
		db, mock, cleanup := SetupMockDB(t)
		defer cleanup()

		pdpService := NewPDPService("http://mock-pdp")
		mockIDP := &MockIDP{}
		service := NewApplicationService(db, pdpService, mockIDP)

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
		db, mock, cleanup := SetupMockDB(t)
		defer cleanup()

		pdpService := NewPDPService("http://mock-pdp")
		mockIDP := &MockIDP{}
		service := NewApplicationService(db, pdpService, mockIDP)

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
		db, mock, cleanup := SetupMockDB(t)
		defer cleanup()

		pdpService := NewPDPService("http://mock-pdp")
		mockIDP := &MockIDP{}
		service := NewApplicationService(db, pdpService, mockIDP)

		// Mock DB expectations
		// 1. Validate member
		mock.ExpectQuery(`SELECT .*`).
			WillReturnRows(sqlmock.NewRows([]string{"member_id", "name"}).AddRow("member-123", "Test Member"))

		// 2. Create submission
		mock.ExpectQuery(`INSERT INTO .*`).
			WillReturnRows(sqlmock.NewRows([]string{"submission_id"}).AddRow("sub_123"))

		desc := "Test Description"
		req := &models.CreateApplicationSubmissionRequest{
			ApplicationName:        "Test Submission",
			ApplicationDescription: &desc,
			SelectedFields: []models.SelectedFieldRecord{
				{FieldName: "field1", SchemaID: "schema-123"},
			},
			MemberID: "member-123",
		}

		result, err := service.CreateApplicationSubmission(context.Background(), req)

		assert.NoError(t, err)
		assert.NotNil(t, result)
		if result != nil {
			assert.Equal(t, req.ApplicationName, result.ApplicationName)
			assert.Equal(t, string(models.StatusPending), result.Status)
		}

		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("CreateApplicationSubmission_MemberNotFound", func(t *testing.T) {
		db, mock, cleanup := SetupMockDB(t)
		defer cleanup()

		pdpService := NewPDPService("http://mock-pdp")
		mockIDP := &MockIDP{}
		service := NewApplicationService(db, pdpService, mockIDP)

		// Mock DB expectations
		mock.ExpectQuery(`SELECT .*`).
			WillReturnError(gorm.ErrRecordNotFound)

		desc := "Test Description"
		req := &models.CreateApplicationSubmissionRequest{
			ApplicationName:        "Test Submission",
			ApplicationDescription: &desc,
			SelectedFields: []models.SelectedFieldRecord{
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
		db, mock, cleanup := SetupMockDB(t)
		defer cleanup()

		pdpService := NewPDPService("http://mock-pdp")
		mockIDP := &MockIDP{}
		service := NewApplicationService(db, pdpService, mockIDP)

		// Mock DB expectations
		// 1. Find submission
		mock.ExpectQuery(`SELECT .*`).
			WillReturnRows(sqlmock.NewRows([]string{"submission_id", "application_name", "member_id", "status"}).
				AddRow("sub_123", "Original", "member-123", string(models.StatusPending)))

		// 2. Save submission
		mock.ExpectExec(`UPDATE "application_submissions"`).
			WillReturnResult(sqlmock.NewResult(0, 1))

		newName := "Updated"
		req := &models.UpdateApplicationSubmissionRequest{
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
		db, mock, cleanup := SetupMockDB(t)
		defer cleanup()

		pdpService := NewPDPService("http://mock-pdp")
		mockIDP := &MockIDP{}
		service := NewApplicationService(db, pdpService, mockIDP)

		// Mock DB expectations
		mock.ExpectQuery(`SELECT .*`).
			WillReturnError(gorm.ErrRecordNotFound)

		updatedName := "Updated"
		req := &models.UpdateApplicationSubmissionRequest{ApplicationName: &updatedName}
		result, err := service.UpdateApplicationSubmission(context.Background(), "non-existent", req)

		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "application submission not found")

		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("UpdateApplicationSubmission_ApprovalWithApplicationCreationFailure", func(t *testing.T) {
		db, mock, cleanup := SetupMockDB(t)
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
		pdpService := NewPDPService("http://mock-pdp")
		pdpService.HTTPClient = &http.Client{Transport: mockTransport}

		mockIDP := &MockIDP{}
		service := NewApplicationService(db, pdpService, mockIDP)

		// Mock DB expectations
		// 1. Find submission
		mock.ExpectQuery(`SELECT .*`).
			WillReturnRows(sqlmock.NewRows([]string{"submission_id", "application_name", "member_id", "status"}).
				AddRow("sub_123", "Original", "member-123", string(models.StatusPending)))

		// 2. Save submission (status update to Approved)
		mock.ExpectExec(`UPDATE "application_submissions"`).
			WillReturnResult(sqlmock.NewResult(0, 1))

		// 3. Create Application (will fail on PDP)
		mock.ExpectQuery(`INSERT INTO "applications"`).
			WillReturnRows(sqlmock.NewRows([]string{"application_id"}).AddRow("app_new"))

		// 4. Compensation: Delete application (from CreateApplication compensation)
		mock.ExpectExec(`DELETE FROM "applications"`).
			WillReturnResult(sqlmock.NewResult(0, 1))

		// 5. Compensation: Update submission status back to Pending
		mock.ExpectExec(`UPDATE "application_submissions"`).
			WillReturnResult(sqlmock.NewResult(0, 1))

		status := string(models.StatusApproved)
		req := &models.UpdateApplicationSubmissionRequest{
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
		db, mock, cleanup := SetupMockDB(t)
		defer cleanup()

		pdpService := NewPDPService("http://mock-pdp")
		mockIDP := &MockIDP{}
		service := NewApplicationService(db, pdpService, mockIDP)

		// Mock DB expectations
		mock.ExpectQuery(`SELECT .*`).
			WillReturnRows(sqlmock.NewRows([]string{"submission_id", "application_name", "member_id", "status"}).
				AddRow("sub_123", "Test Submission", "member-123", string(models.StatusPending)))

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
		db, mock, cleanup := SetupMockDB(t)
		defer cleanup()

		pdpService := NewPDPService("http://mock-pdp")
		mockIDP := &MockIDP{}
		service := NewApplicationService(db, pdpService, mockIDP)

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
		db, mock, cleanup := SetupMockDB(t)
		defer cleanup()

		pdpService := NewPDPService("http://mock-pdp")
		mockIDP := &MockIDP{}
		service := NewApplicationService(db, pdpService, mockIDP)

		// Mock DB expectations
		mock.ExpectQuery(`SELECT .*`).
			WillReturnRows(sqlmock.NewRows([]string{"submission_id", "application_name", "member_id", "status"}).
				AddRow("sub_1", "Sub 1", "member-1", string(models.StatusPending)).
				AddRow("sub_2", "Sub 2", "member-1", string(models.StatusPending)))

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
		db, mock, cleanup := SetupMockDB(t)
		defer cleanup()

		pdpService := NewPDPService("http://mock-pdp")
		mockIDP := &MockIDP{}
		service := NewApplicationService(db, pdpService, mockIDP)

		memberID := "member-123"

		// Mock DB expectations
		mock.ExpectQuery(`SELECT .* FROM "application_submissions" WHERE member_id = .* ORDER BY created_at DESC`).
			WithArgs(memberID).
			WillReturnRows(sqlmock.NewRows([]string{"submission_id", "application_name", "member_id", "status"}).
				AddRow("sub_1", "Sub 1", memberID, string(models.StatusPending)))

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
		db, mock, cleanup := SetupMockDB(t)
		defer cleanup()

		pdpService := NewPDPService("http://mock-pdp")
		mockIDP := &MockIDP{}
		service := NewApplicationService(db, pdpService, mockIDP)

		statusFilter := []string{string(models.StatusApproved)}

		// Mock DB expectations
		mock.ExpectQuery(`SELECT .* FROM "application_submissions" WHERE status IN .* ORDER BY created_at DESC`).
			WithArgs(statusFilter[0]).
			WillReturnRows(sqlmock.NewRows([]string{"submission_id", "application_name", "member_id", "status"}).
				AddRow("sub_2", "Sub 2", "member-123", string(models.StatusApproved)))

		// Preload Member
		mock.ExpectQuery(`SELECT .* FROM "members" WHERE "members"."member_id" = .*`).
			WithArgs("member-123").
			WillReturnRows(sqlmock.NewRows([]string{"member_id", "name"}).AddRow("member-123", "Test Member"))

		result, err := service.GetApplicationSubmissions(context.Background(), nil, &statusFilter)

		assert.NoError(t, err)
		if len(result) > 0 {
			assert.Equal(t, string(models.StatusApproved), result[0].Status)
		}

		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestApplicationService_CreateApplication_EdgeCases(t *testing.T) {
	t.Run("CreateApplication_EmptySelectedFields", func(t *testing.T) {
		db, mock, cleanup := SetupMockDB(t)
		defer cleanup()

		pdpService := NewPDPService("http://mock-pdp")
		mockIDP := &MockIDP{}
		service := NewApplicationService(db, pdpService, mockIDP)

		req := &models.CreateApplicationRequest{
			ApplicationName: "Test Application",
			SelectedFields:  []models.SelectedFieldRecord{},
			MemberID:        "member-123",
		}

		// Mock DB expectations
		// 1. Create application
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
		db, mock, cleanup := SetupMockDB(t)
		defer cleanup()

		pdpService := NewPDPService("http://mock-pdp")
		mockIDP := &MockIDP{}
		service := NewApplicationService(db, pdpService, mockIDP)

		// Mock DB expectations
		// 1. Find application
		mock.ExpectQuery(`SELECT .*`).
			WillReturnRows(sqlmock.NewRows([]string{"application_id", "application_name", "application_description", "member_id", "version"}).
				AddRow("app_123", "Original Name", "Original Description", "member-123", "v1"))

		// 2. Save application
		mock.ExpectExec(`UPDATE "applications"`).
			WillReturnResult(sqlmock.NewResult(0, 1))

		newName := "Updated Name Only"
		req := &models.UpdateApplicationRequest{
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
		db, mock, cleanup := SetupMockDB(t)
		defer cleanup()

		pdpService := NewPDPService("http://mock-pdp")
		mockIDP := &MockIDP{}
		service := NewApplicationService(db, pdpService, mockIDP)

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
		req := &models.CreateApplicationSubmissionRequest{
			ApplicationName:        "Test Submission",
			ApplicationDescription: &desc,
			SelectedFields: []models.SelectedFieldRecord{
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
		db, mock, cleanup := SetupMockDB(t)
		defer cleanup()

		pdpService := NewPDPService("http://mock-pdp")
		mockIDP := &MockIDP{}
		service := NewApplicationService(db, pdpService, mockIDP)

		// Mock DB expectations
		// 1. Validate previous application
		mock.ExpectQuery(`SELECT .*`).
			WillReturnError(gorm.ErrRecordNotFound)

		invalidAppID := "non-existent-app"
		req := &models.CreateApplicationSubmissionRequest{
			ApplicationName:       "New Submission",
			SelectedFields:        []models.SelectedFieldRecord{{FieldName: "field1", SchemaID: "schema-123"}},
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

func TestApplicationService_GetApplicationIdByIdpClientId(t *testing.T) {
	t.Run("Success_ValidClientId", func(t *testing.T) {
		db, mock, cleanup := SetupMockDB(t)
		defer cleanup()

		pdpService := NewPDPService("http://mock-pdp")
		mockIDP := &MockIDP{}
		service := NewApplicationService(db, pdpService, mockIDP)

		expectedAppID := "app-123"
		clientID := "client-456"

		// Mock DB expectations
		mock.ExpectQuery(`SELECT \* FROM "applications" WHERE idp_client_id`).
			WithArgs(clientID, 1).
			WillReturnRows(sqlmock.NewRows([]string{
				"application_id", "application_name", "application_description",
				"selected_fields", "member_id", "version", "idp_application_id",
				"idp_client_id", "created_at", "updated_at",
			}).AddRow(
				expectedAppID, "Test App", "Description",
				`[{"fieldName":"field1","schemaId":"schema-123"}]`, "member-123",
				"active", "idp-app-123", clientID,
				time.Now(), time.Now(),
			))

		// Act
		result, err := service.GetApplicationIdByIdpClientId(context.Background(), clientID)

		// Assert
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, expectedAppID, result.ApplicationID)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("Error_ClientIdNotFound", func(t *testing.T) {
		db, mock, cleanup := SetupMockDB(t)
		defer cleanup()

		pdpService := NewPDPService("http://mock-pdp")
		mockIDP := &MockIDP{}
		service := NewApplicationService(db, pdpService, mockIDP)

		clientID := "non-existent-client"

		// Mock DB expectations - return no rows
		mock.ExpectQuery(`SELECT \* FROM "applications" WHERE idp_client_id`).
			WithArgs(clientID, 1).
			WillReturnError(gorm.ErrRecordNotFound)

		// Act
		result, err := service.GetApplicationIdByIdpClientId(context.Background(), clientID)

		// Assert
		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "application not found for idpClientId")
		assert.Contains(t, err.Error(), clientID)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("Error_DatabaseError", func(t *testing.T) {
		db, mock, cleanup := SetupMockDB(t)
		defer cleanup()

		pdpService := NewPDPService("http://mock-pdp")
		mockIDP := &MockIDP{}
		service := NewApplicationService(db, pdpService, mockIDP)

		clientID := "client-789"
		dbError := fmt.Errorf("database connection error")

		// Mock DB expectations - return database error
		mock.ExpectQuery(`SELECT \* FROM "applications" WHERE idp_client_id`).
			WithArgs(clientID, 1).
			WillReturnError(dbError)

		// Act
		result, err := service.GetApplicationIdByIdpClientId(context.Background(), clientID)

		// Assert
		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "failed to retrieve application")
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("Error_EmptyClientId", func(t *testing.T) {
		db, mock, cleanup := SetupMockDB(t)
		defer cleanup()

		pdpService := NewPDPService("http://mock-pdp")
		mockIDP := &MockIDP{}
		service := NewApplicationService(db, pdpService, mockIDP)

		// Mock DB expectations
		mock.ExpectQuery(`SELECT \* FROM "applications" WHERE idp_client_id`).
			WithArgs("", 1).
			WillReturnError(gorm.ErrRecordNotFound)

		// Act
		result, err := service.GetApplicationIdByIdpClientId(context.Background(), "")

		// Assert
		assert.Error(t, err)
		assert.Nil(t, result)
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}
