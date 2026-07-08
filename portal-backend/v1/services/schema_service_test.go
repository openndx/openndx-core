package services

import (
	"bytes"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gov-dx-sandbox/portal-backend/v1/models"
	"github.com/stretchr/testify/assert"

	"gorm.io/gorm"
)

func TestSchemaService_UpdateSchema(t *testing.T) {
	t.Run("UpdateSchema_Success", func(t *testing.T) {
		db, mock, cleanup := SetupMockDB(t)
		defer cleanup()

		pdpService := NewPDPService("http://localhost:9999")
		service := NewSchemaService(db, pdpService)

		schemaID := "sch_123"
		originalDesc := "Original Description"
		newName := "Updated Name"
		newSDL := "type Query { updated: String }"

		// Mock: Find schema
		mock.ExpectQuery(`SELECT .* FROM "schemas"`).
			WithArgs(schemaID, 1).
			WillReturnRows(sqlmock.NewRows([]string{"schema_id", "schema_name", "schema_description", "sdl", "endpoint", "member_id", "version", "created_at", "updated_at"}).
				AddRow(schemaID, "Original Name", originalDesc, "type Query { original: String }", "http://original.com", "member-123", string(models.ActiveVersion), time.Now(), time.Now()))

		// Mock: Update schema
		mock.ExpectExec(`UPDATE "schemas"`).
			WillReturnResult(sqlmock.NewResult(0, 1))

		req := &models.UpdateSchemaRequest{
			SchemaName: &newName,
			SDL:        &newSDL,
		}

		result, err := service.UpdateSchema(schemaID, req)

		assert.NoError(t, err)
		assert.NotNil(t, result)
		if result != nil {
			assert.Equal(t, newName, result.SchemaName)
			assert.Equal(t, newSDL, result.SDL)
		}

		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("UpdateSchema_NotFound", func(t *testing.T) {
		db, mock, cleanup := SetupMockDB(t)
		defer cleanup()

		pdpService := NewPDPService("http://localhost:9999")
		service := NewSchemaService(db, pdpService)

		// Mock: Find schema - not found
		mock.ExpectQuery(`SELECT .* FROM "schemas"`).
			WithArgs("non-existent-id", 1).
			WillReturnError(gorm.ErrRecordNotFound)

		newName := "Updated Name"
		req := &models.UpdateSchemaRequest{
			SchemaName: &newName,
		}

		result, err := service.UpdateSchema("non-existent-id", req)

		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "schema not found")

		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestSchemaService_GetSchema(t *testing.T) {
	t.Run("GetSchema_Success", func(t *testing.T) {
		db, mock, cleanup := SetupMockDB(t)
		defer cleanup()

		pdpService := NewPDPService("http://localhost:9999")
		service := NewSchemaService(db, pdpService)

		schemaID := "sch_123"
		desc := "Test Description"

		// Mock: Find schema (GORM passes schemaID and LIMIT as parameters)
		mock.ExpectQuery(`SELECT .* FROM "schemas"`).
			WithArgs(schemaID, 1).
			WillReturnRows(sqlmock.NewRows([]string{"schema_id", "schema_name", "schema_description", "sdl", "endpoint", "member_id", "version", "created_at", "updated_at"}).
				AddRow(schemaID, "Test Schema", desc, "type Query { test: String }", "http://example.com", "member-123", string(models.ActiveVersion), time.Now(), time.Now()))

		result, err := service.GetSchema(schemaID)

		assert.NoError(t, err)
		assert.NotNil(t, result)
		if result != nil {
			assert.Equal(t, schemaID, result.SchemaID)
			assert.Equal(t, "Test Schema", result.SchemaName)
		}

		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("GetSchema_NotFound", func(t *testing.T) {
		db, mock, cleanup := SetupMockDB(t)
		defer cleanup()

		pdpService := NewPDPService("http://localhost:9999")
		service := NewSchemaService(db, pdpService)

		// Mock: Find schema - not found
		mock.ExpectQuery(`SELECT .* FROM "schemas"`).
			WithArgs("non-existent-id", 1).
			WillReturnError(gorm.ErrRecordNotFound)

		result, err := service.GetSchema("non-existent-id")

		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "schema not found")

		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestSchemaService_GetSchemas(t *testing.T) {
	t.Run("GetSchemas_NoFilter", func(t *testing.T) {
		db, mock, cleanup := SetupMockDB(t)
		defer cleanup()

		pdpService := NewPDPService("http://localhost:9999")
		service := NewSchemaService(db, pdpService)

		// Mock: Find all schemas
		mock.ExpectQuery(`SELECT .* FROM "schemas" ORDER BY created_at DESC`).
			WillReturnRows(sqlmock.NewRows([]string{"schema_id", "schema_name", "sdl", "endpoint", "member_id", "version", "created_at", "updated_at"}).
				AddRow("sch_1", "Schema 1", "type Query { test1: String }", "http://example.com", "member-1", string(models.ActiveVersion), time.Now(), time.Now()).
				AddRow("sch_2", "Schema 2", "type Query { test2: String }", "http://example.com", "member-2", string(models.ActiveVersion), time.Now(), time.Now()))

		result, err := service.GetSchemas(nil)

		assert.NoError(t, err)
		assert.Len(t, result, 2)

		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("GetSchemas_WithMemberIDFilter", func(t *testing.T) {
		db, mock, cleanup := SetupMockDB(t)
		defer cleanup()

		pdpService := NewPDPService("http://localhost:9999")
		service := NewSchemaService(db, pdpService)

		memberID := "member-123"

		// Mock: Find schemas filtered by member_id
		mock.ExpectQuery(`SELECT .* FROM "schemas" WHERE member_id = .* ORDER BY created_at DESC`).
			WithArgs(memberID).
			WillReturnRows(sqlmock.NewRows([]string{"schema_id", "schema_name", "sdl", "endpoint", "member_id", "version", "created_at", "updated_at"}).
				AddRow("sch_1", "Schema 1", "type Query { test1: String }", "http://example.com", memberID, string(models.ActiveVersion), time.Now(), time.Now()))

		result, err := service.GetSchemas(&memberID)

		assert.NoError(t, err)
		assert.Len(t, result, 1)
		if len(result) > 0 {
			assert.Equal(t, memberID, result[0].MemberID)
		}

		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestSchemaService_CreateSchemaSubmission(t *testing.T) {
	t.Run("CreateSchemaSubmission_Success", func(t *testing.T) {
		db, mock, cleanup := SetupMockDB(t)
		defer cleanup()

		pdpService := NewPDPService("http://localhost:9999")
		service := NewSchemaService(db, pdpService)

		memberID := "member-123"
		desc := "Test Description"

		// Mock: Check if member exists (GORM passes memberID and LIMIT as parameters)
		mock.ExpectQuery(`SELECT .* FROM "members"`).
			WithArgs(memberID, 1).
			WillReturnRows(sqlmock.NewRows([]string{"member_id", "name", "email", "phone_number"}).
				AddRow(memberID, "Test Member", "test@example.com", "1234567890"))

		// Mock: Create submission
		mock.ExpectQuery(`INSERT INTO "schema_submissions"`).
			WillReturnRows(sqlmock.NewRows([]string{"submission_id"}).AddRow("sub_123"))

		req := &models.CreateSchemaSubmissionRequest{
			SchemaName:        "Test Submission",
			SchemaDescription: &desc,
			SDL:               "type Query { test: String }",
			SchemaEndpoint:    "http://example.com",
			MemberID:          memberID,
		}

		result, err := service.CreateSchemaSubmission(req)

		assert.NoError(t, err)
		assert.NotNil(t, result)
		if result != nil {
			assert.Equal(t, req.SchemaName, result.SchemaName)
			assert.NotEmpty(t, result.SubmissionID)
			assert.Equal(t, string(models.StatusPending), result.Status)
		}

		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("CreateSchemaSubmission_MemberNotFound", func(t *testing.T) {
		db, mock, cleanup := SetupMockDB(t)
		defer cleanup()

		pdpService := NewPDPService("http://localhost:9999")
		service := NewSchemaService(db, pdpService)

		// Mock: Check if member exists - not found
		mock.ExpectQuery(`SELECT .* FROM "members"`).
			WithArgs("non-existent-member", 1).
			WillReturnError(gorm.ErrRecordNotFound)

		desc := "Test Description"
		req := &models.CreateSchemaSubmissionRequest{
			SchemaName:        "Test Submission",
			SchemaDescription: &desc,
			SDL:               "type Query { test: String }",
			SchemaEndpoint:    "http://example.com",
			MemberID:          "non-existent-member",
		}

		result, err := service.CreateSchemaSubmission(req)

		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "member not found")

		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestSchemaService_UpdateSchemaSubmission(t *testing.T) {
	t.Run("UpdateSchemaSubmission_Success", func(t *testing.T) {
		db, mock, cleanup := SetupMockDB(t)
		defer cleanup()

		pdpService := NewPDPService("http://localhost:9999")
		service := NewSchemaService(db, pdpService)

		submissionID := "sub_123"
		newName := "Updated"
		newSDL := "type Query { updated: String }"

		// Mock: Find submission
		mock.ExpectQuery(`SELECT .* FROM "schema_submissions"`).
			WithArgs(submissionID, 1).
			WillReturnRows(sqlmock.NewRows([]string{"submission_id", "schema_name", "sdl", "schema_endpoint", "member_id", "status", "created_at", "updated_at"}).
				AddRow(submissionID, "Original", "type Query { original: String }", "http://original.com", "member-123", string(models.StatusPending), time.Now(), time.Now()))

		// Mock: Update submission
		mock.ExpectExec(`UPDATE "schema_submissions"`).
			WillReturnResult(sqlmock.NewResult(0, 1))

		req := &models.UpdateSchemaSubmissionRequest{
			SchemaName: &newName,
			SDL:        &newSDL,
		}

		result, err := service.UpdateSchemaSubmission(submissionID, req)

		assert.NoError(t, err)
		assert.NotNil(t, result)
		if result != nil {
			assert.Equal(t, newName, result.SchemaName)
			assert.Equal(t, newSDL, result.SDL)
		}

		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("UpdateSchemaSubmission_NotFound", func(t *testing.T) {
		db, mock, cleanup := SetupMockDB(t)
		defer cleanup()

		pdpService := NewPDPService("http://localhost:9999")
		service := NewSchemaService(db, pdpService)

		// Mock: Find submission - not found
		mock.ExpectQuery(`SELECT .* FROM "schema_submissions"`).
			WithArgs("non-existent", 1).
			WillReturnError(gorm.ErrRecordNotFound)

		updatedName := "Updated"
		req := &models.UpdateSchemaSubmissionRequest{SchemaName: &updatedName}
		result, err := service.UpdateSchemaSubmission("non-existent", req)

		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "schema submission not found")

		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("UpdateSchemaSubmission_EmptySDL", func(t *testing.T) {
		db, mock, cleanup := SetupMockDB(t)
		defer cleanup()

		pdpService := NewPDPService("http://localhost:9999")
		service := NewSchemaService(db, pdpService)

		submissionID := "sub_123"

		// Mock: Find submission
		mock.ExpectQuery(`SELECT .* FROM "schema_submissions"`).
			WithArgs(submissionID, 1).
			WillReturnRows(sqlmock.NewRows([]string{"submission_id", "schema_name", "sdl", "schema_endpoint", "member_id", "status", "created_at", "updated_at"}).
				AddRow(submissionID, "Test", "type Query { test: String }", "http://example.com", "member-123", string(models.StatusPending), time.Now(), time.Now()))

		emptySDL := ""
		req := &models.UpdateSchemaSubmissionRequest{SDL: &emptySDL}
		result, err := service.UpdateSchemaSubmission(submissionID, req)

		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "SDL field cannot be empty")

		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestSchemaService_GetSchemaSubmission(t *testing.T) {
	t.Run("GetSchemaSubmission_Success", func(t *testing.T) {
		db, mock, cleanup := SetupMockDB(t)
		defer cleanup()

		pdpService := NewPDPService("http://localhost:9999")
		service := NewSchemaService(db, pdpService)

		submissionID := "sub_123"

		// Mock: Find submission
		mock.ExpectQuery(`SELECT .* FROM "schema_submissions"`).
			WithArgs(submissionID, 1).
			WillReturnRows(sqlmock.NewRows([]string{"submission_id", "schema_name", "sdl", "schema_endpoint", "member_id", "status", "created_at", "updated_at"}).
				AddRow(submissionID, "Test Submission", "type Query { test: String }", "http://example.com", "member-123", string(models.StatusPending), time.Now(), time.Now()))

		result, err := service.GetSchemaSubmission(submissionID)

		assert.NoError(t, err)
		assert.NotNil(t, result)
		if result != nil {
			assert.Equal(t, submissionID, result.SubmissionID)
			assert.Equal(t, "Test Submission", result.SchemaName)
		}

		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("GetSchemaSubmission_NotFound", func(t *testing.T) {
		db, mock, cleanup := SetupMockDB(t)
		defer cleanup()

		pdpService := NewPDPService("http://localhost:9999")
		service := NewSchemaService(db, pdpService)

		// Mock: Find submission - not found
		mock.ExpectQuery(`SELECT .* FROM "schema_submissions"`).
			WithArgs("non-existent", 1).
			WillReturnError(gorm.ErrRecordNotFound)

		result, err := service.GetSchemaSubmission("non-existent")

		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "schema submission not found")

		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestSchemaService_GetSchemaSubmissions(t *testing.T) {
	t.Run("GetSchemaSubmissions_NoFilter", func(t *testing.T) {
		db, mock, cleanup := SetupMockDB(t)
		defer cleanup()

		pdpService := NewPDPService("http://localhost:9999")
		service := NewSchemaService(db, pdpService)

		// Mock: Find all submissions (with Preload)
		mock.ExpectQuery(`SELECT .* FROM "schema_submissions"`).
			WillReturnRows(sqlmock.NewRows([]string{"submission_id", "schema_name", "sdl", "schema_endpoint", "member_id", "status", "created_at", "updated_at"}).
				AddRow("sub_1", "Sub 1", "type Query { test1: String }", "http://example.com", "member-123", string(models.StatusPending), time.Now(), time.Now()).
				AddRow("sub_2", "Sub 2", "type Query { test2: String }", "http://example.com", "member-123", string(models.StatusPending), time.Now(), time.Now()))

		// Preload query for Member (GORM only preloads if foreign key is not nil)
		// Since PreviousSchemaID is nil in test data, schema preload is skipped
		mock.ExpectQuery(`SELECT .* FROM "members"`).WillReturnRows(sqlmock.NewRows([]string{"member_id", "name", "email"}))

		result, err := service.GetSchemaSubmissions(nil, nil)

		assert.NoError(t, err)
		assert.Len(t, result, 2)

		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("GetSchemaSubmissions_WithMemberIDFilter", func(t *testing.T) {
		db, mock, cleanup := SetupMockDB(t)
		defer cleanup()

		pdpService := NewPDPService("http://localhost:9999")
		service := NewSchemaService(db, pdpService)

		memberID := "member-123"

		// Mock: Find submissions filtered by member_id
		mock.ExpectQuery(`SELECT .* FROM "schema_submissions"`).
			WithArgs(memberID).
			WillReturnRows(sqlmock.NewRows([]string{"submission_id", "schema_name", "sdl", "schema_endpoint", "member_id", "status", "created_at", "updated_at"}).
				AddRow("sub_1", "Sub 1", "type Query { test1: String }", "http://example.com", memberID, string(models.StatusPending), time.Now(), time.Now()))

		// Preload query for Member (GORM only preloads if foreign key is not nil)
		// Since PreviousSchemaID is nil in test data, schema preload is skipped
		mock.ExpectQuery(`SELECT .* FROM "members"`).WillReturnRows(sqlmock.NewRows([]string{"member_id", "name", "email"}))

		result, err := service.GetSchemaSubmissions(&memberID, nil)

		assert.NoError(t, err)
		assert.Len(t, result, 1)
		assert.Equal(t, memberID, result[0].MemberID)

		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("GetSchemaSubmissions_WithStatusFilter", func(t *testing.T) {
		db, mock, cleanup := SetupMockDB(t)
		defer cleanup()

		pdpService := NewPDPService("http://localhost:9999")
		service := NewSchemaService(db, pdpService)

		statusFilter := []string{string(models.StatusApproved)}

		// Mock: Find submissions filtered by status
		mock.ExpectQuery(`SELECT .* FROM "schema_submissions"`).
			WithArgs(string(models.StatusApproved)).
			WillReturnRows(sqlmock.NewRows([]string{"submission_id", "schema_name", "sdl", "schema_endpoint", "member_id", "status", "created_at", "updated_at"}).
				AddRow("sub_2", "Sub 2", "type Query { test2: String }", "http://example.com", "member-123", string(models.StatusApproved), time.Now(), time.Now()))

		// Preload query for Member (GORM only preloads if foreign key is not nil)
		// Since PreviousSchemaID is nil in test data, schema preload is skipped
		mock.ExpectQuery(`SELECT .* FROM "members"`).WillReturnRows(sqlmock.NewRows([]string{"member_id", "name", "email"}))

		result, err := service.GetSchemaSubmissions(nil, &statusFilter)

		assert.NoError(t, err)
		assert.Len(t, result, 1)
		if len(result) > 0 {
			assert.Equal(t, string(models.StatusApproved), result[0].Status)
		}

		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestSchemaService_CreateSchema_EdgeCases(t *testing.T) {
	t.Run("CreateSchema_EmptySDL", func(t *testing.T) {
		db, mock, cleanup := SetupMockDB(t)
		defer cleanup()

		// Mock PDP failure (empty SDL will fail validation or PDP call)
		mockTransport := &MockRoundTripper{
			RoundTripFunc: func(req *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusBadRequest,
					Body:       io.NopCloser(bytes.NewBufferString(`{"error": "invalid SDL"}`)),
					Header:     make(http.Header),
				}, nil
			},
		}
		pdpService := NewPDPService("http://mock-pdp")
		pdpService.HTTPClient = &http.Client{Transport: mockTransport}

		service := NewSchemaService(db, pdpService)

		// Mock: Create schema (will succeed, then PDP fails)
		mock.ExpectQuery(`INSERT INTO "schemas"`).
			WillReturnRows(sqlmock.NewRows([]string{"schema_id"}).AddRow("sch_123"))

		// Mock: Compensation - delete schema
		mock.ExpectExec(`DELETE FROM "schemas"`).
			WillReturnResult(sqlmock.NewResult(0, 1))

		req := &models.CreateSchemaRequest{
			SchemaName: "Test Schema",
			SDL:        "",
		}

		_, err := service.CreateSchema(req)

		// Should fail validation or PDP call
		assert.Error(t, err)

		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("CreateSchema_CompensationFailure", func(t *testing.T) {
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

		service := NewSchemaService(db, pdpService)

		// Mock: Create schema
		mock.ExpectQuery(`INSERT INTO "schemas"`).
			WillReturnRows(sqlmock.NewRows([]string{"schema_id"}).AddRow("sch_123"))

		// Mock: Compensation - delete schema fails
		mock.ExpectExec(`DELETE FROM "schemas"`).
			WillReturnError(gorm.ErrRecordNotFound)

		req := &models.CreateSchemaRequest{
			SchemaName: "Test Schema",
			SDL:        "type Query { test: String }",
			Endpoint:   "http://example.com/graphql",
			MemberID:   "member-123",
		}

		// This tests the compensation path when PDP fails
		_, err := service.CreateSchema(req)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to compensate")

		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestSchemaService_UpdateSchema_EdgeCases(t *testing.T) {
	t.Run("UpdateSchema_PartialUpdate", func(t *testing.T) {
		db, mock, cleanup := SetupMockDB(t)
		defer cleanup()

		pdpService := NewPDPService("http://localhost:9999")
		service := NewSchemaService(db, pdpService)

		schemaID := "sch_123"
		originalDesc := "Original Description"
		newName := "Updated Name Only"

		// Mock: Find schema
		mock.ExpectQuery(`SELECT .* FROM "schemas"`).
			WithArgs(schemaID, 1).
			WillReturnRows(sqlmock.NewRows([]string{"schema_id", "schema_name", "schema_description", "sdl", "endpoint", "member_id", "version", "created_at", "updated_at"}).
				AddRow(schemaID, "Original Name", originalDesc, "type Query { original: String }", "http://original.com", "member-123", string(models.ActiveVersion), time.Now(), time.Now()))

		// Mock: Update schema
		mock.ExpectExec(`UPDATE "schemas"`).
			WillReturnResult(sqlmock.NewResult(0, 1))

		// Only update name, leave other fields unchanged
		req := &models.UpdateSchemaRequest{
			SchemaName: &newName,
		}

		result, err := service.UpdateSchema(schemaID, req)

		assert.NoError(t, err)
		assert.NotNil(t, result)
		if result != nil {
			assert.Equal(t, newName, result.SchemaName)
			// Original description should remain
			if result.SchemaDescription != nil {
				assert.Equal(t, originalDesc, *result.SchemaDescription)
			}
		}

		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("UpdateSchema_AllFields", func(t *testing.T) {
		db, mock, cleanup := SetupMockDB(t)
		defer cleanup()

		pdpService := NewPDPService("http://localhost:9999")
		service := NewSchemaService(db, pdpService)

		schemaID := "sch_123"
		newName := "Updated"
		newSDL := "type Query { updated: String }"
		newEndpoint := "http://updated.com"
		newVersion := "v2.0"

		// Mock: Find schema
		mock.ExpectQuery(`SELECT .* FROM "schemas"`).
			WithArgs(schemaID, 1).
			WillReturnRows(sqlmock.NewRows([]string{"schema_id", "schema_name", "sdl", "endpoint", "member_id", "version", "created_at", "updated_at"}).
				AddRow(schemaID, "Original", "type Query { original: String }", "http://original.com", "member-123", string(models.ActiveVersion), time.Now(), time.Now()))

		// Mock: Update schema
		mock.ExpectExec(`UPDATE "schemas"`).
			WillReturnResult(sqlmock.NewResult(0, 1))

		req := &models.UpdateSchemaRequest{
			SchemaName: &newName,
			SDL:        &newSDL,
			Endpoint:   &newEndpoint,
			Version:    &newVersion,
		}

		result, err := service.UpdateSchema(schemaID, req)

		assert.NoError(t, err)
		assert.NotNil(t, result)
		if result != nil {
			assert.Equal(t, newName, result.SchemaName)
			assert.Equal(t, newSDL, result.SDL)
			assert.Equal(t, newEndpoint, result.Endpoint)
			assert.Equal(t, newVersion, result.Version)
		}

		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestSchemaService_CreateSchemaSubmission_EdgeCases(t *testing.T) {
	t.Run("CreateSchemaSubmission_WithPreviousSchemaID", func(t *testing.T) {
		db, mock, cleanup := SetupMockDB(t)
		defer cleanup()

		pdpService := NewPDPService("http://localhost:9999")
		service := NewSchemaService(db, pdpService)

		memberID := "member-123"
		previousSchemaID := "sch_prev"

		// Mock: Check if member exists (GORM passes memberID and LIMIT as parameters)
		mock.ExpectQuery(`SELECT .* FROM "members"`).
			WithArgs(memberID, 1).
			WillReturnRows(sqlmock.NewRows([]string{"member_id", "name", "email", "phone_number"}).
				AddRow(memberID, "Test", "test@example.com", "123"))

		// Mock: Check if previous schema exists (GORM passes schemaID and LIMIT as parameters)
		mock.ExpectQuery(`SELECT .* FROM "schemas"`).
			WithArgs(previousSchemaID, 1).
			WillReturnRows(sqlmock.NewRows([]string{"schema_id", "schema_name", "sdl", "endpoint", "member_id", "version"}).
				AddRow(previousSchemaID, "Previous Schema", "type Query { prev: String }", "http://prev.com", memberID, string(models.ActiveVersion)))

		// Mock: Create submission
		mock.ExpectQuery(`INSERT INTO "schema_submissions"`).
			WillReturnRows(sqlmock.NewRows([]string{"submission_id"}).AddRow("sub_123"))

		req := &models.CreateSchemaSubmissionRequest{
			SchemaName:       "New Submission",
			SDL:              "type Query { new: String }",
			SchemaEndpoint:   "http://new.com",
			MemberID:         memberID,
			PreviousSchemaID: &previousSchemaID,
		}

		result, err := service.CreateSchemaSubmission(req)

		assert.NoError(t, err)
		assert.NotNil(t, result)
		if result != nil {
			assert.Equal(t, previousSchemaID, *result.PreviousSchemaID)
		}

		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("CreateSchemaSubmission_InvalidPreviousSchemaID", func(t *testing.T) {
		db, mock, cleanup := SetupMockDB(t)
		defer cleanup()

		pdpService := NewPDPService("http://localhost:9999")
		service := NewSchemaService(db, pdpService)

		memberID := "member-123"
		invalidSchemaID := "non-existent-schema"

		// Mock: Check if member exists (GORM passes memberID and LIMIT as parameters)
		mock.ExpectQuery(`SELECT .* FROM "members"`).
			WithArgs(memberID, 1).
			WillReturnRows(sqlmock.NewRows([]string{"member_id", "name", "email", "phone_number"}).
				AddRow(memberID, "Test", "test@example.com", "123"))

		// Mock: Check if previous schema exists - not found (GORM passes schemaID and LIMIT as parameters)
		mock.ExpectQuery(`SELECT .* FROM "schemas"`).
			WithArgs(invalidSchemaID, 1).
			WillReturnError(gorm.ErrRecordNotFound)

		req := &models.CreateSchemaSubmissionRequest{
			SchemaName:       "New Submission",
			SDL:              "type Query { new: String }",
			SchemaEndpoint:   "http://new.com",
			MemberID:         memberID,
			PreviousSchemaID: &invalidSchemaID,
		}

		result, err := service.CreateSchemaSubmission(req)

		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "previous schema not found")

		assert.NoError(t, mock.ExpectationsWereMet())
	})
}
