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
	"github.com/stretchr/testify/require"
)

// TestSchemaEndpoints_MemberOwnership checks that a caller without the -all
// scopes only reaches its own member's schemas and schema submissions.
func TestSchemaEndpoints_MemberOwnership(t *testing.T) {
	env := newTestHandlerEnv(t)
	ownMemberID := createTestMember(t, env.db, fmt.Sprintf("own-%d@example.com", uniqueID()))
	otherMemberID := createTestMember(t, env.db, fmt.Sprintf("other-%d@example.com", uniqueID()))
	ownSchemaID := createTestSchema(t, env.db, ownMemberID)
	otherSchemaID := createTestSchema(t, env.db, otherMemberID)
	otherSubmission := SchemaSubmission{
		SubmissionID:   "sub_other_" + fmt.Sprintf("%d", uniqueID()),
		SchemaName:     "Other Submission",
		SDL:            "type Query { test: String }",
		SchemaEndpoint: "http://example.com/graphql",
		MemberID:       otherMemberID,
		Status:         string(kernel.StatusPending),
	}
	require.NoError(t, env.db.Create(&otherSubmission).Error)

	owner := callertest.MemberUser.WithMember(ownMemberID)

	t.Run("GetSchema - own schema", func(t *testing.T) {
		req := callertest.NewAuthenticatedRequest(http.MethodGet, "/api/v1/schemas/"+ownSchemaID, nil, owner)
		req.SetPathValue("schemaId", ownSchemaID)
		w := httptest.NewRecorder()
		env.handler.GetSchema(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("GetSchema - another member's schema", func(t *testing.T) {
		req := callertest.NewAuthenticatedRequest(http.MethodGet, "/api/v1/schemas/"+otherSchemaID, nil, owner)
		req.SetPathValue("schemaId", otherSchemaID)
		w := httptest.NewRecorder()
		env.handler.GetSchema(w, req)
		assert.Equal(t, http.StatusForbidden, w.Code)
	})

	t.Run("UpdateSchema - another member's schema", func(t *testing.T) {
		body, _ := json.Marshal(UpdateSchemaRequest{})
		req := callertest.NewAuthenticatedRequest(http.MethodPut, "/api/v1/schemas/"+otherSchemaID, bytes.NewBuffer(body), owner)
		req.SetPathValue("schemaId", otherSchemaID)
		w := httptest.NewRecorder()
		env.handler.UpdateSchema(w, req)
		assert.Equal(t, http.StatusForbidden, w.Code)
	})

	t.Run("GetAllSchemas - memberId filter is ignored", func(t *testing.T) {
		req := callertest.NewAuthenticatedRequest(http.MethodGet, "/api/v1/schemas?memberId="+otherMemberID, nil, owner)
		w := httptest.NewRecorder()
		env.handler.GetAllSchemas(w, req)

		require.Equal(t, http.StatusOK, w.Code)
		var response struct {
			Items []Schema `json:"items"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
		require.Len(t, response.Items, 1)
		assert.Equal(t, ownSchemaID, response.Items[0].SchemaID)
	})

	t.Run("GetSchemaSubmission - another member's submission", func(t *testing.T) {
		req := callertest.NewAuthenticatedRequest(http.MethodGet, "/api/v1/schema-submissions/"+otherSubmission.SubmissionID, nil, owner)
		req.SetPathValue("submissionId", otherSubmission.SubmissionID)
		w := httptest.NewRecorder()
		env.handler.GetSchemaSubmission(w, req)
		assert.Equal(t, http.StatusForbidden, w.Code)
	})

	t.Run("GetAllSchemaSubmissions - only own submissions", func(t *testing.T) {
		req := callertest.NewAuthenticatedRequest(http.MethodGet, "/api/v1/schema-submissions?memberId="+otherMemberID, nil, owner)
		w := httptest.NewRecorder()
		env.handler.GetAllSchemaSubmissions(w, req)

		require.Equal(t, http.StatusOK, w.Code)
		var response kernel.CollectionResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
		assert.Equal(t, 0, response.Count)
	})

	t.Run("Caller without a member is denied", func(t *testing.T) {
		req := callertest.NewMemberRequest(http.MethodGet, "/api/v1/schemas", nil)
		w := httptest.NewRecorder()
		env.handler.GetAllSchemas(w, req)
		assert.Equal(t, http.StatusForbidden, w.Code)
	})
}
