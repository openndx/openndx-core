package application

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

// TestApplicationEndpoints_MemberOwnership checks that a caller without the
// -all scopes only reaches its own member's applications and application
// submissions.
func TestApplicationEndpoints_MemberOwnership(t *testing.T) {
	th := NewTestHandler(t)
	ownMemberID := createTestMember(t, th.db, fmt.Sprintf("own-%d@example.com", uniqueID()))
	otherMemberID := createTestMember(t, th.db, fmt.Sprintf("other-%d@example.com", uniqueID()))
	ownApplicationID := createTestApplication(t, th.db, ownMemberID)
	otherApplicationID := createTestApplication(t, th.db, otherMemberID)
	otherSubmission := ApplicationSubmission{
		SubmissionID:    "sub_other_" + fmt.Sprintf("%d", uniqueID()),
		ApplicationName: "Other Submission",
		SelectedFields:  SelectedFieldRecords{{FieldName: "field1", SchemaID: "schema-123"}},
		MemberID:        otherMemberID,
		Status:          string(kernel.StatusPending),
	}
	require.NoError(t, th.db.Create(&otherSubmission).Error)

	owner := callertest.MemberUser.WithMember(ownMemberID)

	t.Run("GetApplication - own application", func(t *testing.T) {
		req := callertest.NewAuthenticatedRequest(http.MethodGet, "/api/v1/applications/"+ownApplicationID, nil, owner)
		req.SetPathValue("applicationId", ownApplicationID)
		w := httptest.NewRecorder()
		th.handler.GetApplication(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("GetApplication - another member's application", func(t *testing.T) {
		req := callertest.NewAuthenticatedRequest(http.MethodGet, "/api/v1/applications/"+otherApplicationID, nil, owner)
		req.SetPathValue("applicationId", otherApplicationID)
		w := httptest.NewRecorder()
		th.handler.GetApplication(w, req)
		assert.Equal(t, http.StatusForbidden, w.Code)
	})

	t.Run("UpdateApplication - another member's application", func(t *testing.T) {
		body, _ := json.Marshal(UpdateApplicationRequest{})
		req := callertest.NewAuthenticatedRequest(http.MethodPut, "/api/v1/applications/"+otherApplicationID, bytes.NewBuffer(body), owner)
		req.SetPathValue("applicationId", otherApplicationID)
		w := httptest.NewRecorder()
		th.handler.UpdateApplication(w, req)
		assert.Equal(t, http.StatusForbidden, w.Code)
	})

	t.Run("GetAllApplications - memberId filter is ignored", func(t *testing.T) {
		req := callertest.NewAuthenticatedRequest(http.MethodGet, "/api/v1/applications?memberId="+otherMemberID, nil, owner)
		w := httptest.NewRecorder()
		th.handler.GetAllApplications(w, req)

		require.Equal(t, http.StatusOK, w.Code)
		var response struct {
			Items []Application `json:"items"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
		require.Len(t, response.Items, 1)
		assert.Equal(t, ownApplicationID, response.Items[0].ApplicationID)
	})

	t.Run("GetApplicationSubmission - another member's submission", func(t *testing.T) {
		req := callertest.NewAuthenticatedRequest(http.MethodGet, "/api/v1/application-submissions/"+otherSubmission.SubmissionID, nil, owner)
		req.SetPathValue("submissionId", otherSubmission.SubmissionID)
		w := httptest.NewRecorder()
		th.handler.GetApplicationSubmission(w, req)
		assert.Equal(t, http.StatusForbidden, w.Code)
	})

	t.Run("UpdateApplicationSubmission - another member's submission", func(t *testing.T) {
		body, _ := json.Marshal(UpdateApplicationSubmissionRequest{})
		req := callertest.NewAuthenticatedRequest(http.MethodPut, "/api/v1/application-submissions/"+otherSubmission.SubmissionID, bytes.NewBuffer(body), owner)
		req.SetPathValue("submissionId", otherSubmission.SubmissionID)
		w := httptest.NewRecorder()
		th.handler.UpdateApplicationSubmission(w, req)
		assert.Equal(t, http.StatusForbidden, w.Code)
	})

	t.Run("CreateApplicationSubmission - for another member", func(t *testing.T) {
		body, _ := json.Marshal(CreateApplicationSubmissionRequest{MemberID: otherMemberID})
		req := callertest.NewAuthenticatedRequest(http.MethodPost, "/api/v1/application-submissions", bytes.NewBuffer(body), owner)
		w := httptest.NewRecorder()
		th.handler.CreateApplicationSubmission(w, req)
		assert.Equal(t, http.StatusForbidden, w.Code)
	})

	t.Run("Caller without a member is denied", func(t *testing.T) {
		req := callertest.NewMemberRequest(http.MethodGet, "/api/v1/applications", nil)
		w := httptest.NewRecorder()
		th.handler.GetAllApplications(w, req)
		assert.Equal(t, http.StatusForbidden, w.Code)
	})
}
