package member

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/openndx/openndx-core/internal/pb/caller/callertest"
	"github.com/openndx/openndx-core/internal/pb/database/dbtest"
	"github.com/openndx/openndx-core/internal/pb/kernel"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// newTestHandler creates a member Handler backed by an in-memory SQLite database.
func newTestHandler(t *testing.T) (*Handler, *gorm.DB) {
	db := dbtest.SetupSQLiteDB(t, &Member{})
	return NewHandler(NewService(db)), db
}

// createTestMember stores a member linked to the given organization unit and returns its ID
func createTestMember(t *testing.T, db *gorm.DB, ouID string) string {
	m := Member{
		MemberID:    "mem_" + fmt.Sprintf("%d", uniqueID()),
		Name:        "Test Member",
		Email:       fmt.Sprintf("org-%d@example.com", uniqueID()),
		PhoneNumber: "1234567890",
		OuID:        ouID,
	}
	require.NoError(t, db.Create(&m).Error)
	return m.MemberID
}

// TestMemberEndpoints tests all member-related endpoints
func TestMemberEndpoints(t *testing.T) {
	handler, _ := newTestHandler(t)

	t.Run("POST /api/v1/members - CreateMember", func(t *testing.T) {
		req := CreateMemberRequest{
			Name:        "Test Member",
			Email:       fmt.Sprintf("test-%d@example.com", uniqueID()),
			PhoneNumber: "1234567890",
			OuID:        fmt.Sprintf("ou-%d", uniqueID()),
		}

		reqBody, _ := json.Marshal(req)
		httpReq := callertest.NewAdminRequest(http.MethodPost, "/api/v1/members", bytes.NewBuffer(reqBody))
		httpReq.Header.Set("Content-Type", "application/json")

		w := httptest.NewRecorder()
		handler.CreateMember(w, httpReq)

		assert.Equal(t, http.StatusCreated, w.Code)
		var response MemberResponse
		err := json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(t, err)
		assert.Equal(t, req.Name, response.Name)
		assert.Equal(t, req.Email, response.Email)
		assert.Equal(t, req.PhoneNumber, response.PhoneNumber)
		assert.Equal(t, req.OuID, response.OuID)
		assert.NotEmpty(t, response.MemberID)
	})

	t.Run("POST /api/v1/members - DuplicateOuID", func(t *testing.T) {
		ouID := fmt.Sprintf("ou-%d", uniqueID())
		for i, wantCode := range []int{http.StatusCreated, http.StatusBadRequest} {
			reqBody, _ := json.Marshal(CreateMemberRequest{Name: fmt.Sprintf("Org %d", i), OuID: ouID})
			httpReq := callertest.NewAdminRequest(http.MethodPost, "/api/v1/members", bytes.NewBuffer(reqBody))
			w := httptest.NewRecorder()
			handler.CreateMember(w, httpReq)

			assert.Equal(t, wantCode, w.Code)
			if wantCode == http.StatusBadRequest {
				// The fixed public message, not the database driver error
				assert.Contains(t, w.Body.String(), ErrMemberAlreadyExists.Error())
				assert.NotContains(t, w.Body.String(), "UNIQUE")
			}
		}
	})

	t.Run("POST /api/v1/members - MissingOuID", func(t *testing.T) {
		req := CreateMemberRequest{
			Name:        "Test Member",
			Email:       fmt.Sprintf("test-%d@example.com", uniqueID()),
			PhoneNumber: "1234567890",
		}

		reqBody, _ := json.Marshal(req)
		httpReq := callertest.NewAdminRequest(http.MethodPost, "/api/v1/members", bytes.NewBuffer(reqBody))
		httpReq.Header.Set("Content-Type", "application/json")

		w := httptest.NewRecorder()
		handler.CreateMember(w, httpReq)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("POST /api/v1/members - Invalid JSON", func(t *testing.T) {
		httpReq := callertest.NewAdminRequest(http.MethodPost, "/api/v1/members", bytes.NewBufferString("invalid json"))
		httpReq.Header.Set("Content-Type", "application/json")

		w := httptest.NewRecorder()
		handler.CreateMember(w, httpReq)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("PUT /api/v1/members/:id - UpdateMember_InvalidJSON", func(t *testing.T) {
		httpReq := callertest.NewAdminRequest(http.MethodPut, "/api/v1/members/test-id", bytes.NewBufferString("invalid json"))
		httpReq.Header.Set("Content-Type", "application/json")
		httpReq.SetPathValue("memberId", "test-id")
		w := httptest.NewRecorder()
		handler.UpdateMember(w, httpReq)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("PUT /api/v1/members/:id - UpdateMember_NotFound", func(t *testing.T) {
		name := "Updated Name"
		req := UpdateMemberRequest{
			Name: &name,
		}
		reqBody, _ := json.Marshal(req)
		httpReq := callertest.NewAdminRequest(http.MethodPut, "/api/v1/members/non-existent-id", bytes.NewBuffer(reqBody))
		httpReq.Header.Set("Content-Type", "application/json")
		httpReq.SetPathValue("memberId", "non-existent-id")
		w := httptest.NewRecorder()
		handler.UpdateMember(w, httpReq)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("GET /api/v1/members - GetAllMembers", func(t *testing.T) {
		httpReq := callertest.NewAdminRequest(http.MethodGet, "/api/v1/members", nil)
		w := httptest.NewRecorder()
		handler.GetAllMembers(w, httpReq)

		assert.Equal(t, http.StatusOK, w.Code)

		var response kernel.CollectionResponse
		err := json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(t, err)
		assert.NotNil(t, response.Items)
		assert.GreaterOrEqual(t, response.Count, 0)
	})

	t.Run("GET /api/v1/members/:memberId - NotFound", func(t *testing.T) {
		httpReq := callertest.NewAdminRequest(http.MethodGet, "/api/v1/members/non-existent-id", nil)
		httpReq.SetPathValue("memberId", "non-existent-id")
		w := httptest.NewRecorder()
		handler.GetMember(w, httpReq)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}

// TestMemberEndpoints_OrganizationUnitOwnership tests that non-admin users can
// only access the member linked to their own organization unit
func TestMemberEndpoints_OrganizationUnitOwnership(t *testing.T) {
	handler, db := newTestHandler(t)
	ownMemberID := createTestMember(t, db, callertest.MemberUser.OuID)
	otherMemberID := createTestMember(t, db, "other-ou")

	// A member-scoped caller whose organization unit belongs to ownMemberID
	memberUser := callertest.MemberUser.WithMember(ownMemberID)
	newMemberRequest := func(method, url string, body io.Reader) *http.Request {
		return callertest.NewAuthenticatedRequest(method, url, body, memberUser)
	}

	t.Run("GetMember - own organization unit", func(t *testing.T) {
		httpReq := newMemberRequest(http.MethodGet, "/api/v1/members/"+ownMemberID, nil)
		httpReq.SetPathValue("memberId", ownMemberID)
		w := httptest.NewRecorder()
		handler.GetMember(w, httpReq)

		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("GetMember - other organization unit", func(t *testing.T) {
		httpReq := newMemberRequest(http.MethodGet, "/api/v1/members/"+otherMemberID, nil)
		httpReq.SetPathValue("memberId", otherMemberID)
		w := httptest.NewRecorder()
		handler.GetMember(w, httpReq)

		assert.Equal(t, http.StatusForbidden, w.Code)
	})

	t.Run("UpdateMember - own organization unit", func(t *testing.T) {
		name := "Renamed Org"
		reqBody, _ := json.Marshal(UpdateMemberRequest{Name: &name})
		httpReq := newMemberRequest(http.MethodPut, "/api/v1/members/"+ownMemberID, bytes.NewBuffer(reqBody))
		httpReq.SetPathValue("memberId", ownMemberID)
		w := httptest.NewRecorder()
		handler.UpdateMember(w, httpReq)

		assert.Equal(t, http.StatusOK, w.Code)
		var response MemberResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
		assert.Equal(t, "Renamed Org", response.Name)
	})

	t.Run("UpdateMember - other organization unit", func(t *testing.T) {
		name := "Hijacked Org"
		reqBody, _ := json.Marshal(UpdateMemberRequest{Name: &name})
		httpReq := newMemberRequest(http.MethodPut, "/api/v1/members/"+otherMemberID, bytes.NewBuffer(reqBody))
		httpReq.SetPathValue("memberId", otherMemberID)
		w := httptest.NewRecorder()
		handler.UpdateMember(w, httpReq)

		assert.Equal(t, http.StatusForbidden, w.Code)
	})

	t.Run("GetAllMembers - member sees only own organization unit", func(t *testing.T) {
		// The ouId query param is ignored for non-admin users
		httpReq := newMemberRequest(http.MethodGet, "/api/v1/members?ouId=other-ou", nil)
		w := httptest.NewRecorder()
		handler.GetAllMembers(w, httpReq)

		assert.Equal(t, http.StatusOK, w.Code)
		var response struct {
			Items []MemberResponse `json:"items"`
			Count int              `json:"count"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
		require.Len(t, response.Items, 1)
		assert.Equal(t, ownMemberID, response.Items[0].MemberID)
	})

	t.Run("GetAllMembers - admin filters by unknown ouId", func(t *testing.T) {
		httpReq := callertest.NewAdminRequest(http.MethodGet, "/api/v1/members?ouId=unknown-ou", nil)
		w := httptest.NewRecorder()
		handler.GetAllMembers(w, httpReq)

		assert.Equal(t, http.StatusOK, w.Code)
		var response kernel.CollectionResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
		assert.Equal(t, 0, response.Count)
		assert.Equal(t, []any{}, response.Items)
	})

	t.Run("GetAllMembers - admin filters by ouId", func(t *testing.T) {
		httpReq := callertest.NewAdminRequest(http.MethodGet, "/api/v1/members?ouId=other-ou", nil)
		w := httptest.NewRecorder()
		handler.GetAllMembers(w, httpReq)

		assert.Equal(t, http.StatusOK, w.Code)
		var response struct {
			Items []MemberResponse `json:"items"`
			Count int              `json:"count"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
		require.Len(t, response.Items, 1)
		assert.Equal(t, otherMemberID, response.Items[0].MemberID)
	})
}

// testIDs backs uniqueID.
var testIDs atomic.Int64

// uniqueID returns a number no other call in this test binary returns, for
// building unique test IDs and emails. uniqueID() isn't unique:
// on Windows, consecutive calls can return the same value.
func uniqueID() int64 {
	return testIDs.Add(1)
}
