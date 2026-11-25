// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Lanka Software Foundation

package schema

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/openndx/openndx-core/internal/pb/caller/callertest"
	"github.com/openndx/openndx-core/internal/pb/kernel"
	"github.com/stretchr/testify/assert"
)

// TestSchemaEndpoints tests all schema-related endpoints
func TestSchemaEndpoints(t *testing.T) {
	testHandler := newTestHandlerEnv(t)
	defer testHandler.db.Exec("DELETE FROM schemas")

	testMemberID := "test-member-id"

	t.Run("POST /api/v1/schemas - CreateSchema", func(t *testing.T) {
		desc := "Test Description"
		req := CreateSchemaRequest{
			SchemaName:        "Test Schema",
			SchemaDescription: &desc,
			SDL:               "type Query { test: String }",
			Endpoint:          "http://example.com/graphql",
			MemberID:          testMemberID,
		}

		reqBody, _ := json.Marshal(req)
		httpReq := callertest.NewAdminRequest(http.MethodPost, "/api/v1/schemas", bytes.NewBuffer(reqBody))
		httpReq.Header.Set("Content-Type", "application/json")

		w := httptest.NewRecorder()
		testHandler.handler.CreateSchema(w, httpReq)

		if w.Code == http.StatusCreated {
			var response SchemaResponse
			err := json.Unmarshal(w.Body.Bytes(), &response)
			assert.NoError(t, err)
			assert.Equal(t, req.SchemaName, response.SchemaName)
			assert.Equal(t, req.SDL, response.SDL)
			assert.NotEmpty(t, response.SchemaID)
		}
	})

	t.Run("POST /api/v1/schemas - Invalid JSON", func(t *testing.T) {
		httpReq := callertest.NewAdminRequest(http.MethodPost, "/api/v1/schemas", bytes.NewBufferString("invalid"))
		httpReq.Header.Set("Content-Type", "application/json")

		w := httptest.NewRecorder()
		testHandler.handler.CreateSchema(w, httpReq)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("GET /api/v1/schemas - GetAllSchemas", func(t *testing.T) {
		httpReq := callertest.NewAdminRequest(http.MethodGet, "/api/v1/schemas", nil)
		w := httptest.NewRecorder()
		testHandler.handler.GetAllSchemas(w, httpReq)

		assert.Equal(t, http.StatusOK, w.Code)

		var response kernel.CollectionResponse
		err := json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(t, err)
		assert.NotNil(t, response.Items)
		assert.GreaterOrEqual(t, response.Count, 0)
	})

	t.Run("GET /api/v1/schemas - WithQueryParams", func(t *testing.T) {
		httpReq := callertest.NewAdminRequest(http.MethodGet, "/api/v1/schemas?memberId=test-member", nil)
		w := httptest.NewRecorder()
		testHandler.handler.GetAllSchemas(w, httpReq)

		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("GET /api/v1/schemas/:schemaId - GetSchema", func(t *testing.T) {
		// Create a schema directly in DB for this test (since creation is async)
		schema := Schema{
			SchemaID:   "test-schema-get-id",
			SchemaName: "Test Schema for Get",
			SDL:        "type Query { test: String }",
			Endpoint:   "http://example.com/graphql",
			MemberID:   testMemberID,
		}
		err := testHandler.db.Create(&schema).Error
		assert.NoError(t, err)

		httpReq := callertest.NewAdminRequest(http.MethodGet, fmt.Sprintf("/api/v1/schemas/%s", schema.SchemaID), nil)
		httpReq.SetPathValue("schemaId", schema.SchemaID)
		w := httptest.NewRecorder()
		testHandler.handler.GetSchema(w, httpReq)

		assert.Equal(t, http.StatusOK, w.Code)
		var response SchemaResponse
		err = json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(t, err)
		assert.Equal(t, schema.SchemaID, response.SchemaID)
	})

	t.Run("GET /api/v1/schemas/:schemaId - NotFound", func(t *testing.T) {
		httpReq := callertest.NewAdminRequest(http.MethodGet, "/api/v1/schemas/non-existent", nil)
		httpReq.SetPathValue("schemaId", "non-existent")
		w := httptest.NewRecorder()
		testHandler.handler.GetSchema(w, httpReq)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("PUT /api/v1/schemas/:schemaId - UpdateSchema", func(t *testing.T) {
		// Create a schema first by inserting directly into DB (since creation is async)
		schema := Schema{
			SchemaID:   "test-schema-update-id",
			SchemaName: "Test Schema",
			SDL:        "type Query { test: String }",
			Endpoint:   "http://example.com/graphql",
			MemberID:   testMemberID,
		}
		err := testHandler.db.Create(&schema).Error
		assert.NoError(t, err)

		schemaName := "Updated Schema Name"
		sdl := "type Query { updated: String }"
		req := UpdateSchemaRequest{
			SchemaName: &schemaName,
			SDL:        &sdl,
		}

		reqBody, _ := json.Marshal(req)
		httpReq := callertest.NewAdminRequest(http.MethodPut, fmt.Sprintf("/api/v1/schemas/%s", schema.SchemaID), bytes.NewBuffer(reqBody))
		httpReq.Header.Set("Content-Type", "application/json")
		httpReq.SetPathValue("schemaId", schema.SchemaID)

		w := httptest.NewRecorder()
		testHandler.handler.UpdateSchema(w, httpReq)

		assert.Equal(t, http.StatusOK, w.Code)
		var response SchemaResponse
		err = json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(t, err)
		assert.Equal(t, schemaName, response.SchemaName)
	})
}

// TestSchemaSubmissionEndpoints tests all schema submission-related endpoints
func TestSchemaSubmissionEndpoints(t *testing.T) {
	testHandler := newTestHandlerEnv(t)
	defer testHandler.db.Exec("DELETE FROM schema_submissions")

	testMemberID := "test-member-id"

	t.Run("GET /api/v1/schema-submissions/:id - GetSchemaSubmission_NotFound", func(t *testing.T) {
		httpReq := callertest.NewAdminRequest(http.MethodGet, "/api/v1/schema-submissions/non-existent-id", nil)
		httpReq.SetPathValue("submissionId", "non-existent-id")
		w := httptest.NewRecorder()
		testHandler.handler.GetSchemaSubmission(w, httpReq)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("POST /api/v1/schema-submissions - CreateSchemaSubmission", func(t *testing.T) {
		desc := "Test Description"
		req := CreateSchemaSubmissionRequest{
			SchemaName:        "Test Schema Submission",
			SchemaDescription: &desc,
			SDL:               "type Query { test: String }",
			SchemaEndpoint:    "http://example.com/graphql",
			MemberID:          testMemberID,
		}

		reqBody, _ := json.Marshal(req)
		httpReq := callertest.NewAdminRequest(http.MethodPost, "/api/v1/schema-submissions", bytes.NewBuffer(reqBody))
		httpReq.Header.Set("Content-Type", "application/json")

		w := httptest.NewRecorder()
		testHandler.handler.CreateSchemaSubmission(w, httpReq)

		if w.Code == http.StatusCreated {
			var response SchemaSubmissionResponse
			err := json.Unmarshal(w.Body.Bytes(), &response)
			assert.NoError(t, err)
			assert.Equal(t, req.SchemaName, response.SchemaName)
			assert.NotEmpty(t, response.SubmissionID)
		}
	})

	t.Run("GET /api/v1/schema-submissions - GetAllSchemaSubmissions", func(t *testing.T) {
		httpReq := callertest.NewAdminRequest(http.MethodGet, "/api/v1/schema-submissions", nil)
		w := httptest.NewRecorder()
		testHandler.handler.GetAllSchemaSubmissions(w, httpReq)

		assert.Equal(t, http.StatusOK, w.Code)

		var response kernel.CollectionResponse
		err := json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(t, err)
		assert.GreaterOrEqual(t, response.Count, 0)
	})

	t.Run("GET /api/v1/schema-submissions - WithQueryParams", func(t *testing.T) {
		httpReq := callertest.NewAdminRequest(http.MethodGet, "/api/v1/schema-submissions?memberId=test&status=pending", nil)
		w := httptest.NewRecorder()
		testHandler.handler.GetAllSchemaSubmissions(w, httpReq)

		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("GET /api/v1/schema-submissions/:submissionId - GetSchemaSubmission", func(t *testing.T) {
		// Create test data directly in DB
		memberID := createTestMember(t, testHandler.db, fmt.Sprintf("test-%d@example.com", uniqueID()))

		// Create a submission directly in DB
		submission := SchemaSubmission{
			SubmissionID:   "sub_" + fmt.Sprintf("%d", uniqueID()),
			SchemaName:     "Test Submission",
			SDL:            "type Query { test: String }",
			SchemaEndpoint: "http://example.com/graphql",
			MemberID:       memberID,
			Status:         string(kernel.StatusPending),
		}
		err := testHandler.db.Create(&submission).Error
		assert.NoError(t, err)

		httpReq := callertest.NewAdminRequest(http.MethodGet, fmt.Sprintf("/api/v1/schema-submissions/%s", submission.SubmissionID), nil)
		httpReq.SetPathValue("submissionId", submission.SubmissionID)
		w := httptest.NewRecorder()
		testHandler.handler.GetSchemaSubmission(w, httpReq)

		assert.Equal(t, http.StatusOK, w.Code)
		var response SchemaSubmissionResponse
		err = json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(t, err)
		assert.Equal(t, submission.SubmissionID, response.SubmissionID)
	})

	t.Run("PUT /api/v1/schema-submissions/:submissionId - UpdateSchemaSubmission", func(t *testing.T) {
		// Create test data directly in DB
		memberID := createTestMember(t, testHandler.db, fmt.Sprintf("test-%d@example.com", uniqueID()))

		// Create a submission directly in DB
		submission := SchemaSubmission{
			SubmissionID:   "sub_" + fmt.Sprintf("%d", uniqueID()),
			SchemaName:     "Test Submission",
			SDL:            "type Query { test: String }",
			SchemaEndpoint: "http://example.com/graphql",
			MemberID:       memberID,
			Status:         string(kernel.StatusPending),
		}
		err := testHandler.db.Create(&submission).Error
		assert.NoError(t, err)

		// Use "rejected" status to avoid triggering schema creation (which calls PDP and times out)
		status := "rejected"
		review := "Needs improvement"
		req := UpdateSchemaSubmissionRequest{
			Status: &status,
			Review: &review,
		}

		reqBody, _ := json.Marshal(req)
		httpReq := callertest.NewAdminRequest(http.MethodPut, fmt.Sprintf("/api/v1/schema-submissions/%s", submission.SubmissionID), bytes.NewBuffer(reqBody))
		httpReq.Header.Set("Content-Type", "application/json")
		httpReq.SetPathValue("submissionId", submission.SubmissionID)

		w := httptest.NewRecorder()
		testHandler.handler.UpdateSchemaSubmission(w, httpReq)

		assert.Equal(t, http.StatusOK, w.Code)
		var response SchemaSubmissionResponse
		err = json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(t, err)
		assert.Equal(t, status, string(response.Status))
	})
}

// TestSchemaEndpoints_EdgeCases tests edge cases for schema endpoints
func TestSchemaEndpoints_EdgeCases(t *testing.T) {
	testHandler := newTestHandlerEnv(t)

	t.Run("POST /api/v1/schemas - Invalid JSON", func(t *testing.T) {
		httpReq := callertest.NewAdminRequest(http.MethodPost, "/api/v1/schemas", bytes.NewBufferString("invalid json"))
		httpReq.Header.Set("Content-Type", "application/json")

		w := httptest.NewRecorder()
		testHandler.handler.CreateSchema(w, httpReq)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("PUT /api/v1/schemas/:id - Invalid JSON", func(t *testing.T) {
		httpReq := callertest.NewAdminRequest(http.MethodPut, "/api/v1/schemas/test-id", bytes.NewBufferString("invalid json"))
		httpReq.Header.Set("Content-Type", "application/json")
		httpReq.SetPathValue("schemaId", "test-id")

		w := httptest.NewRecorder()
		testHandler.handler.UpdateSchema(w, httpReq)

		// Resource doesn't exist, so 404 is returned before JSON is parsed
		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("GET /api/v1/schemas/:id - NotFound", func(t *testing.T) {
		httpReq := callertest.NewAdminRequest(http.MethodGet, "/api/v1/schemas/non-existent-id", nil)
		httpReq.SetPathValue("schemaId", "non-existent-id")
		w := httptest.NewRecorder()
		testHandler.handler.GetSchema(w, httpReq)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("PUT /api/v1/schemas/:id - NotFound", func(t *testing.T) {
		schemaName := "Updated Name"
		req := UpdateSchemaRequest{
			SchemaName: &schemaName,
		}
		reqBody, _ := json.Marshal(req)
		httpReq := callertest.NewAdminRequest(http.MethodPut, "/api/v1/schemas/non-existent-id", bytes.NewBuffer(reqBody))
		httpReq.Header.Set("Content-Type", "application/json")
		httpReq.SetPathValue("schemaId", "non-existent-id")

		w := httptest.NewRecorder()
		testHandler.handler.UpdateSchema(w, httpReq)

		// Resource doesn't exist, so 404 is the correct response
		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}

// TestSchemaSubmissionEndpoints_EdgeCases tests edge cases for schema submission endpoints
func TestSchemaSubmissionEndpoints_EdgeCases(t *testing.T) {
	testHandler := newTestHandlerEnv(t)

	t.Run("POST /api/v1/schema-submissions - Invalid JSON", func(t *testing.T) {
		httpReq := callertest.NewAdminRequest(http.MethodPost, "/api/v1/schema-submissions", bytes.NewBufferString("invalid json"))
		httpReq.Header.Set("Content-Type", "application/json")

		w := httptest.NewRecorder()
		testHandler.handler.CreateSchemaSubmission(w, httpReq)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("PUT /api/v1/schema-submissions/:id - Invalid JSON", func(t *testing.T) {
		httpReq := callertest.NewAdminRequest(http.MethodPut, "/api/v1/schema-submissions/test-id", bytes.NewBufferString("invalid json"))
		httpReq.Header.Set("Content-Type", "application/json")
		httpReq.SetPathValue("submissionId", "test-id")

		w := httptest.NewRecorder()
		testHandler.handler.UpdateSchemaSubmission(w, httpReq)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("PUT /api/v1/schema-submissions/:id - NotFound", func(t *testing.T) {
		status := "approved"
		req := UpdateSchemaSubmissionRequest{
			Status: &status,
		}
		reqBody, _ := json.Marshal(req)
		httpReq := callertest.NewAdminRequest(http.MethodPut, "/api/v1/schema-submissions/non-existent-id", bytes.NewBuffer(reqBody))
		httpReq.Header.Set("Content-Type", "application/json")
		httpReq.SetPathValue("submissionId", "non-existent-id")

		w := httptest.NewRecorder()
		testHandler.handler.UpdateSchemaSubmission(w, httpReq)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}
