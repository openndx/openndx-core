package services

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/openndx/openndx-core/internal/ce/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// consentRecordsDDL mirrors the consent_records table. AutoMigrate cannot be used on SQLite
// because the model's Postgres-only gen_random_uuid() default is not valid SQLite syntax.
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

func setupTestService(t *testing.T) (*ConsentService, *gorm.DB) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	// Each connection to :memory: is a separate database, so pin the pool to one connection
	// to let concurrent tests share the table
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.Exec(consentRecordsDDL).Error)

	svc, err := NewConsentService(db, "http://consent.example.com")
	require.NoError(t, err)
	return svc, db
}

// insertConsent stores a record for ownerID/appID with the given status, created `age` ago
func insertConsent(t *testing.T, db *gorm.DB, ownerID, appID string, status models.ConsentStatus, age time.Duration) *models.ConsentRecord {
	t.Helper()
	now := time.Now().UTC()
	createdAt := now.Add(-age)
	record := &models.ConsentRecord{
		ConsentID:        uuid.New(),
		OwnerID:          ownerID,
		AppID:            appID,
		Status:           string(status),
		Type:             string(models.TypeRealtime),
		CreatedAt:        createdAt,
		UpdatedAt:        createdAt,
		GrantDuration:    string(models.DurationOneHour),
		Fields:           []models.ConsentField{{FieldName: "person.fullName", SchemaID: "schema-1", Owner: models.OwnerCitizen}},
		ConsentPortalURL: "http://consent.example.com",
	}
	switch status {
	case models.StatusPending:
		expiresAt := createdAt.Add(time.Hour)
		record.PendingExpiresAt = &expiresAt
	case models.StatusApproved:
		expiresAt := createdAt.Add(time.Hour)
		record.GrantExpiresAt = &expiresAt
	}
	require.NoError(t, db.Create(record).Error)
	return record
}

func statusOf(t *testing.T, db *gorm.DB, consentID uuid.UUID) string {
	t.Helper()
	var record models.ConsentRecord
	require.NoError(t, db.Where("consent_id = ?", consentID).First(&record).Error)
	return record.Status
}

func TestListConsentsByOwner_OnlyReturnsOwnersConsentsNewestFirst(t *testing.T) {
	svc, db := setupTestService(t)
	older := insertConsent(t, db, "owner-1", "app-a", models.StatusRejected, 30*time.Minute)
	newer := insertConsent(t, db, "owner-1", "app-b", models.StatusPending, 5*time.Minute)
	insertConsent(t, db, "owner-2", "app-a", models.StatusPending, time.Minute)

	result, err := svc.ListConsentsByOwner(context.Background(), "owner-1", nil, 20, 0)
	require.NoError(t, err)

	// owner-2's newer consent must not appear
	assert.Equal(t, int64(2), result.Total)
	require.Len(t, result.Consents, 2)
	assert.Equal(t, newer.ConsentID.String(), result.Consents[0].ConsentID)
	assert.Equal(t, older.ConsentID.String(), result.Consents[1].ConsentID)
}

func TestListConsentsByOwner_FiltersByStatus(t *testing.T) {
	svc, db := setupTestService(t)
	pending := insertConsent(t, db, "owner-1", "app-a", models.StatusPending, 5*time.Minute)
	approved := insertConsent(t, db, "owner-1", "app-b", models.StatusApproved, 10*time.Minute)
	insertConsent(t, db, "owner-1", "app-c", models.StatusRejected, 15*time.Minute)

	result, err := svc.ListConsentsByOwner(context.Background(), "owner-1", []models.ConsentStatus{models.StatusPending}, 20, 0)
	require.NoError(t, err)
	assert.Equal(t, int64(1), result.Total)
	require.Len(t, result.Consents, 1)
	assert.Equal(t, pending.ConsentID.String(), result.Consents[0].ConsentID)

	result, err = svc.ListConsentsByOwner(context.Background(), "owner-1", []models.ConsentStatus{models.StatusPending, models.StatusApproved}, 20, 0)
	require.NoError(t, err)
	assert.Equal(t, int64(2), result.Total)
	assert.Equal(t, approved.ConsentID.String(), result.Consents[1].ConsentID)
}

func TestListConsentsByOwner_Paginates(t *testing.T) {
	svc, db := setupTestService(t)
	for i := 0; i < 5; i++ {
		insertConsent(t, db, "owner-1", uuid.NewString(), models.StatusRejected, time.Duration(i+1)*time.Minute)
	}

	result, err := svc.ListConsentsByOwner(context.Background(), "owner-1", nil, 2, 4)
	require.NoError(t, err)
	assert.Equal(t, int64(5), result.Total)
	assert.Len(t, result.Consents, 1)
	assert.Equal(t, 2, result.Limit)
	assert.Equal(t, 4, result.Offset)
}

func TestListConsentsByOwner_ExpiresStaleConsents(t *testing.T) {
	svc, db := setupTestService(t)
	stalePending := insertConsent(t, db, "owner-1", "app-a", models.StatusPending, 2*time.Hour)
	staleApproved := insertConsent(t, db, "owner-1", "app-b", models.StatusApproved, 2*time.Hour)
	freshPending := insertConsent(t, db, "owner-1", "app-c", models.StatusPending, time.Minute)
	otherOwnerStale := insertConsent(t, db, "owner-2", "app-a", models.StatusPending, 2*time.Hour)

	result, err := svc.ListConsentsByOwner(context.Background(), "owner-1", []models.ConsentStatus{models.StatusPending}, 20, 0)
	require.NoError(t, err)
	require.Len(t, result.Consents, 1)
	assert.Equal(t, freshPending.ConsentID.String(), result.Consents[0].ConsentID)

	assert.Equal(t, string(models.StatusExpired), statusOf(t, db, stalePending.ConsentID))
	assert.Equal(t, string(models.StatusExpired), statusOf(t, db, staleApproved.ConsentID))
	assert.Equal(t, string(models.StatusPending), statusOf(t, db, otherOwnerStale.ConsentID))
}

func TestListConsentsByOwner_EmptyResultIsNotNil(t *testing.T) {
	svc, _ := setupTestService(t)

	result, err := svc.ListConsentsByOwner(context.Background(), "owner-1", nil, 20, 0)
	require.NoError(t, err)
	assert.NotNil(t, result.Consents)
	assert.Equal(t, int64(0), result.Total)
}

func TestListConsentsByOwner_RequiresOwnerID(t *testing.T) {
	svc, _ := setupTestService(t)

	_, err := svc.ListConsentsByOwner(context.Background(), "", nil, 20, 0)
	assert.ErrorIs(t, err, models.ErrConsentGetFailed)
}

func TestGetConsentPortalView_ExpiresStalePending(t *testing.T) {
	svc, db := setupTestService(t)
	stale := insertConsent(t, db, "owner-1", "app-a", models.StatusPending, 2*time.Hour)

	view, err := svc.GetConsentPortalView(context.Background(), stale.ConsentID.String(), "owner-1")
	require.NoError(t, err)
	assert.Equal(t, models.StatusExpired, view.Status)
	assert.Equal(t, stale.ConsentID.String(), view.ConsentID)
	assert.Equal(t, string(models.StatusExpired), statusOf(t, db, stale.ConsentID))
}

func TestGetConsentPortalView_OtherOwnerCannotTriggerExpiry(t *testing.T) {
	svc, db := setupTestService(t)
	stale := insertConsent(t, db, "owner-1", "app-a", models.StatusPending, 2*time.Hour)

	_, err := svc.GetConsentPortalView(context.Background(), stale.ConsentID.String(), "owner-2")
	assert.ErrorIs(t, err, models.ErrConsentAccessDenied)
	assert.Equal(t, string(models.StatusPending), statusOf(t, db, stale.ConsentID))
}

func TestUpdateConsentStatusByPortalAction_OtherOwnerIsDenied(t *testing.T) {
	for name, age := range map[string]time.Duration{"fresh": time.Minute, "stale": 2 * time.Hour} {
		t.Run(name, func(t *testing.T) {
			svc, db := setupTestService(t)
			pending := insertConsent(t, db, "owner-1", "app-a", models.StatusPending, age)

			err := svc.UpdateConsentStatusByPortalAction(context.Background(), models.ConsentPortalActionRequest{
				ConsentID: pending.ConsentID.String(),
				OwnerID:   "owner-2",
				Action:    models.ActionApprove,
				UpdatedBy: "owner-2",
			})
			assert.ErrorIs(t, err, models.ErrConsentAccessDenied)
			assert.Equal(t, string(models.StatusPending), statusOf(t, db, pending.ConsentID))
		})
	}
}

func TestUpdateConsentStatusByPortalAction_SecondDecisionIsRejected(t *testing.T) {
	svc, db := setupTestService(t)
	pending := insertConsent(t, db, "owner-1", "app-a", models.StatusPending, time.Minute)

	req := models.ConsentPortalActionRequest{
		ConsentID: pending.ConsentID.String(),
		OwnerID:   "owner-1",
		Action:    models.ActionApprove,
		UpdatedBy: "owner-1",
	}
	require.NoError(t, svc.UpdateConsentStatusByPortalAction(context.Background(), req))

	req.Action = models.ActionReject
	err := svc.UpdateConsentStatusByPortalAction(context.Background(), req)
	assert.ErrorIs(t, err, models.ErrConsentNotPending)
	assert.Equal(t, string(models.StatusApproved), statusOf(t, db, pending.ConsentID))
}

func TestUpdateConsentStatusByPortalAction_ConcurrentDecisionsHaveSingleWinner(t *testing.T) {
	svc, db := setupTestService(t)
	pending := insertConsent(t, db, "owner-1", "app-a", models.StatusPending, time.Minute)

	const attempts = 10
	var wg sync.WaitGroup
	errs := make([]error, attempts)
	for i := range attempts {
		action := models.ActionApprove
		if i%2 == 1 {
			action = models.ActionReject
		}
		wg.Go(func() {
			errs[i] = svc.UpdateConsentStatusByPortalAction(context.Background(), models.ConsentPortalActionRequest{
				ConsentID: pending.ConsentID.String(),
				OwnerID:   "owner-1",
				Action:    action,
				UpdatedBy: "owner-1",
			})
		})
	}
	wg.Wait()

	var winner *models.ConsentPortalAction
	for i, err := range errs {
		if err == nil {
			require.Nil(t, winner, "more than one decision succeeded")
			action := models.ActionApprove
			if i%2 == 1 {
				action = models.ActionReject
			}
			winner = &action
			continue
		}
		assert.ErrorIs(t, err, models.ErrConsentNotPending)
	}
	require.NotNil(t, winner, "no decision succeeded")

	expected := models.StatusApproved
	if *winner == models.ActionReject {
		expected = models.StatusRejected
	}
	assert.Equal(t, string(expected), statusOf(t, db, pending.ConsentID))
}

func TestUpdateConsentStatusByPortalAction_ApprovesPending(t *testing.T) {
	svc, db := setupTestService(t)
	pending := insertConsent(t, db, "owner-1", "app-a", models.StatusPending, time.Minute)

	err := svc.UpdateConsentStatusByPortalAction(context.Background(), models.ConsentPortalActionRequest{
		ConsentID: pending.ConsentID.String(),
		OwnerID:   "owner-1",
		Action:    models.ActionApprove,
		UpdatedBy: "owner-1",
	})
	require.NoError(t, err)

	var record models.ConsentRecord
	require.NoError(t, db.Where("consent_id = ?", pending.ConsentID).First(&record).Error)
	assert.Equal(t, string(models.StatusApproved), record.Status)
	assert.NotNil(t, record.GrantExpiresAt)
	assert.Nil(t, record.PendingExpiresAt)
}

func TestUpdateConsentStatusByPortalAction_RejectsNonPending(t *testing.T) {
	for _, status := range []models.ConsentStatus{models.StatusApproved, models.StatusRejected, models.StatusRevoked, models.StatusExpired} {
		t.Run(string(status), func(t *testing.T) {
			svc, db := setupTestService(t)
			record := insertConsent(t, db, "owner-1", "app-a", status, time.Minute)

			err := svc.UpdateConsentStatusByPortalAction(context.Background(), models.ConsentPortalActionRequest{
				ConsentID: record.ConsentID.String(),
				OwnerID:   "owner-1",
				Action:    models.ActionApprove,
				UpdatedBy: "owner-1",
			})
			assert.True(t, errors.Is(err, models.ErrConsentNotPending), "got %v", err)
			assert.Equal(t, string(status), statusOf(t, db, record.ConsentID))
		})
	}
}

func TestUpdateConsentStatusByPortalAction_ExpiredPendingCannotBeApproved(t *testing.T) {
	svc, db := setupTestService(t)
	stale := insertConsent(t, db, "owner-1", "app-a", models.StatusPending, 2*time.Hour)

	err := svc.UpdateConsentStatusByPortalAction(context.Background(), models.ConsentPortalActionRequest{
		ConsentID: stale.ConsentID.String(),
		OwnerID:   "owner-1",
		Action:    models.ActionApprove,
		UpdatedBy: "owner-1",
	})
	assert.ErrorIs(t, err, models.ErrConsentNotPending)
	assert.Equal(t, string(models.StatusExpired), statusOf(t, db, stale.ConsentID))
}
