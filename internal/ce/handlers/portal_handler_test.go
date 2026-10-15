package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/openndx/openndx-core/internal/ce/middleware"
	"github.com/openndx/openndx-core/internal/ce/models"
	"github.com/openndx/openndx-core/internal/ce/services"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// setOwnerSubjectInContext is a test helper to set the owner subject (UID) in context
// Uses the same context key as middleware.auth.go (ownerSubjectKey = "ownerSubject")
func setOwnerSubjectInContext(ctx context.Context, subject string) context.Context {
	type contextKey string
	const ownerSubjectKey contextKey = "ownerSubject"
	return context.WithValue(ctx, ownerSubjectKey, subject)
}

func TestPortalHandler_HealthCheck(t *testing.T) {
	handler := &PortalHandler{consentService: nil}

	req := httptest.NewRequest("GET", "/api/v1/health", nil)
	w := httptest.NewRecorder()

	handler.HealthCheck(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var response map[string]string
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, "healthy", response["status"])
}

func TestPortalHandler_GetConsent_MissingConsentId(t *testing.T) {
	handler := &PortalHandler{consentService: nil}

	req := httptest.NewRequest("GET", "/api/v1/consents/", nil)
	w := httptest.NewRecorder()

	handler.GetConsent(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestPortalHandler_GetConsent_MethodNotAllowed(t *testing.T) {
	handler := &PortalHandler{consentService: nil}

	req := httptest.NewRequest("POST", "/api/v1/consents/test-id", nil)
	w := httptest.NewRecorder()

	handler.GetConsent(w, req)

	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
}

func TestPortalHandler_GetConsent_InvalidUUID(t *testing.T) {
	handler := &PortalHandler{consentService: nil}

	req := httptest.NewRequest("GET", "/api/v1/consents/invalid-uuid", nil)
	req = req.WithContext(setOwnerSubjectInContext(req.Context(), "user-123"))
	w := httptest.NewRecorder()

	handler.GetConsent(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestPortalHandler_UpdateConsent_InvalidUUID(t *testing.T) {
	handler := &PortalHandler{consentService: nil}

	req := httptest.NewRequest("PUT", "/api/v1/consents/invalid-uuid", nil)
	req = req.WithContext(setOwnerSubjectInContext(req.Context(), "user-123"))
	w := httptest.NewRecorder()

	handler.UpdateConsent(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestPortalHandler_UpdateConsent_InvalidAction(t *testing.T) {
	handler := &PortalHandler{consentService: nil}

	consentID := uuid.New().String()
	reqBody := map[string]string{"action": "invalid"}
	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest("PUT", "/api/v1/consents/"+consentID, bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(setOwnerSubjectInContext(req.Context(), "user-123"))
	w := httptest.NewRecorder()

	handler.UpdateConsent(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestPortalHandler_UpdateConsent_MethodNotAllowed(t *testing.T) {
	handler := &PortalHandler{consentService: nil}

	consentID := uuid.New().String()
	req := httptest.NewRequest("GET", "/api/v1/consents/"+consentID, nil)
	req = req.WithContext(setOwnerSubjectInContext(req.Context(), "user-123"))
	w := httptest.NewRecorder()

	handler.UpdateConsent(w, req)

	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
}

// Note: GetConsent and UpdateConsent success paths require PathValue which needs a registered route.
// These are tested in integration tests. Here we focus on validation and error handling that doesn't require PathValue.

func TestPortalHandler_UpdateConsent_RejectAction(t *testing.T) {
	handler := &PortalHandler{consentService: nil}

	consentID := uuid.New().String()
	reqBody := map[string]string{"action": "reject"}
	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest("PUT", "/api/v1/consents/"+consentID, bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(setOwnerSubjectInContext(req.Context(), "user-123"))
	w := httptest.NewRecorder()

	// PathValue requires registered route, so this tests validation up to that point
	handler.UpdateConsent(w, req)

	// PathValue returns empty without route, so we get 400
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestPortalHandler_UpdateConsent_InvalidBody(t *testing.T) {
	handler := &PortalHandler{consentService: nil}

	consentID := uuid.New().String()
	req := httptest.NewRequest("PUT", "/api/v1/consents/"+consentID, bytes.NewBufferString("invalid json"))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(setOwnerSubjectInContext(req.Context(), "user-123"))
	w := httptest.NewRecorder()

	handler.UpdateConsent(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestPortalHandler_UpdateConsent_MissingConsentId(t *testing.T) {
	handler := &PortalHandler{consentService: nil}

	req := httptest.NewRequest("PUT", "/api/v1/consents/", nil)
	req = req.WithContext(setOwnerSubjectInContext(req.Context(), "user-123"))
	w := httptest.NewRecorder()

	handler.UpdateConsent(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestPortalHandler_NewPortalHandler(t *testing.T) {
	handler := NewPortalHandler(nil)
	assert.NotNil(t, handler)
	assert.Nil(t, handler.consentService)
}

func TestPortalHandler_HealthCheck_MethodNotAllowed(t *testing.T) {
	handler := &PortalHandler{consentService: nil}

	req := httptest.NewRequest("POST", "/api/v1/health", nil)
	w := httptest.NewRecorder()

	handler.HealthCheck(w, req)

	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
}

func TestPortalHandler_ListConsents_MethodNotAllowed(t *testing.T) {
	handler := &PortalHandler{consentService: nil}

	req := httptest.NewRequest("POST", "/api/v1/consents", nil)
	req = req.WithContext(middleware.WithOwnerSubject(req.Context(), "user-123"))
	w := httptest.NewRecorder()

	handler.ListConsents(w, req)

	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
}

func TestPortalHandler_ListConsents_MissingSubject(t *testing.T) {
	handler := &PortalHandler{consentService: nil}

	req := httptest.NewRequest("GET", "/api/v1/consents", nil)
	w := httptest.NewRecorder()

	handler.ListConsents(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestPortalHandler_ListConsents_InvalidQuery(t *testing.T) {
	tests := map[string]string{
		"unknown status":    "status=pending,granted",
		"non-numeric limit": "limit=abc",
		"zero limit":        "limit=0",
		"limit above max":   "limit=101",
		"negative offset":   "offset=-1",
	}
	for name, rawQuery := range tests {
		t.Run(name, func(t *testing.T) {
			handler := &PortalHandler{consentService: nil}

			req := httptest.NewRequest("GET", "/api/v1/consents?"+rawQuery, nil)
			req = req.WithContext(middleware.WithOwnerSubject(req.Context(), "user-123"))
			w := httptest.NewRecorder()

			handler.ListConsents(w, req)

			assert.Equal(t, http.StatusBadRequest, w.Code)
		})
	}
}

func TestParseStatusFilter(t *testing.T) {
	statuses, err := parseStatusFilter([]string{"pending, approved", "expired", ""})
	assert.NoError(t, err)
	assert.Equal(t, []models.ConsentStatus{models.StatusPending, models.StatusApproved, models.StatusExpired}, statuses)

	statuses, err = parseStatusFilter(nil)
	assert.NoError(t, err)
	assert.Empty(t, statuses)
}

// consentRecordsDDL mirrors the consent_records table; see services/consent_service_test.go for why
// AutoMigrate cannot be used on SQLite.
const consentRecordsDDL = `CREATE TABLE consent_records (
	consent_id TEXT PRIMARY KEY,
	owner_id TEXT NOT NULL,
	app_id TEXT NOT NULL,
	app_name TEXT,
	status TEXT NOT NULL,
	type TEXT NOT NULL,
	created_at DATETIME NOT NULL,
	updated_at DATETIME NOT NULL,
	pending_expires_at DATETIME,
	grant_expires_at DATETIME,
	grant_duration TEXT NOT NULL,
	fields TEXT NOT NULL,
	session_id TEXT,
	consent_portal_url TEXT NOT NULL,
	updated_by TEXT
)`

// newDBPortalHandler returns a handler backed by a real ConsentService on an in-memory database
func newDBPortalHandler(t *testing.T) (*PortalHandler, *gorm.DB) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.Exec(consentRecordsDDL).Error)

	svc, err := services.NewConsentService(db, "http://consent.example.com")
	require.NoError(t, err)
	return NewPortalHandler(svc), db
}

// insertConsentRecord stores a fresh consent for ownerID with the given status
func insertConsentRecord(t *testing.T, db *gorm.DB, ownerID string, status models.ConsentStatus) *models.ConsentRecord {
	t.Helper()
	now := time.Now().UTC()
	expiresAt := now.Add(time.Hour)
	record := &models.ConsentRecord{
		ConsentID:        uuid.New(),
		OwnerID:          ownerID,
		AppID:            "app-a",
		Status:           string(status),
		Type:             string(models.TypeRealtime),
		CreatedAt:        now,
		UpdatedAt:        now,
		GrantDuration:    string(models.DurationOneHour),
		Fields:           []models.ConsentField{{FieldName: "person.fullName", SchemaID: "schema-1", Owner: models.OwnerCitizen}},
		ConsentPortalURL: "http://consent.example.com",
	}
	switch status {
	case models.StatusPending:
		record.PendingExpiresAt = &expiresAt
	case models.StatusApproved:
		record.GrantExpiresAt = &expiresAt
	}
	require.NoError(t, db.Create(record).Error)
	return record
}

// putConsentAction sends PUT /api/v1/consents/{consentId} as subject with the given action
func putConsentAction(handler *PortalHandler, consentID, subject, action string) *httptest.ResponseRecorder {
	body, _ := json.Marshal(map[string]string{"action": action})
	req := httptest.NewRequest("PUT", "/api/v1/consents/"+consentID, bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("consentId", consentID)
	req = req.WithContext(middleware.WithOwnerSubject(req.Context(), subject))
	w := httptest.NewRecorder()
	handler.UpdateConsent(w, req)
	return w
}

func TestPortalHandler_UpdateConsent_Revoke(t *testing.T) {
	t.Run("owner revokes approved consent", func(t *testing.T) {
		handler, db := newDBPortalHandler(t)
		record := insertConsentRecord(t, db, "owner-1", models.StatusApproved)

		w := putConsentAction(handler, record.ConsentID.String(), "owner-1", string(models.ActionRevoke))

		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		var response map[string]string
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
		assert.Equal(t, string(models.StatusRevoked), response["status"])

		var stored models.ConsentRecord
		require.NoError(t, db.Where("consent_id = ?", record.ConsentID).First(&stored).Error)
		assert.Equal(t, string(models.StatusRevoked), stored.Status)
		require.NotNil(t, stored.UpdatedBy)
		assert.Equal(t, "owner-1", *stored.UpdatedBy)
	})

	t.Run("other owner is forbidden", func(t *testing.T) {
		handler, db := newDBPortalHandler(t)
		record := insertConsentRecord(t, db, "owner-1", models.StatusApproved)

		w := putConsentAction(handler, record.ConsentID.String(), "owner-2", string(models.ActionRevoke))

		assert.Equal(t, http.StatusForbidden, w.Code)
		assert.Contains(t, w.Body.String(), string(models.ErrorCodeForbidden))
	})

	t.Run("non-approved consent conflicts", func(t *testing.T) {
		handler, db := newDBPortalHandler(t)
		record := insertConsentRecord(t, db, "owner-1", models.StatusPending)

		w := putConsentAction(handler, record.ConsentID.String(), "owner-1", string(models.ActionRevoke))

		assert.Equal(t, http.StatusConflict, w.Code)
		assert.Contains(t, w.Body.String(), string(models.ErrorCodeConsentNotRevocable))
	})

	t.Run("unknown consent is not found", func(t *testing.T) {
		handler, _ := newDBPortalHandler(t)

		w := putConsentAction(handler, uuid.NewString(), "owner-1", string(models.ActionRevoke))

		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}
