package grant

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newBypassService returns a service with one field for each combination of
// the policy flags in schema s1, named after the flags it sets.
func newBypassService(t *testing.T) *Service {
	t.Helper()
	db := setupTestDB(t)
	insertField(t, db, "s1", "consent", false, true)
	insertField(t, db, "s1", "allowlist_consent", true, true)
	insertField(t, db, "s1", "allowlist", true, false)
	insertField(t, db, "s1", "open", false, false)
	return NewService(db)
}

func bypassRequest(app string, fields ...string) *AccessGrantCreateRequest {
	records := make([]AccessGrantCreateRequestRecord, 0, len(fields))
	for _, field := range fields {
		records = append(records, AccessGrantCreateRequestRecord{SchemaID: "s1", FieldName: field})
	}
	return &AccessGrantCreateRequest{
		ApplicationID: app,
		Records:       records,
		GrantDuration: GrantDurationTypeOneMonth,
		ConsentBypass: true,
		LegalBasis:    "Police Act s.12",
	}
}

func grantBypass(t *testing.T, service *Service, app string, fields ...string) []AccessGrantResponse {
	t.Helper()
	resp, err := service.CreateAccessGrants(bypassRequest(app, fields...))
	require.NoError(t, err)
	return resp.Records
}

func grantPlain(t *testing.T, service *Service, app string, duration GrantDurationType, field string) AccessGrantResponse {
	t.Helper()
	resp, err := service.CreateAccessGrants(plainRequest(app, duration, AccessGrantCreateRequestRecord{SchemaID: "s1", FieldName: field}))
	require.NoError(t, err)
	return resp.Records[0]
}

func TestConsentBypass_CreatesBypassGrants(t *testing.T) {
	service := newBypassService(t)

	grants := grantBypass(t, service, "police", "consent", "allowlist_consent")
	require.Len(t, grants, 2)
	for _, grant := range grants {
		assert.Equal(t, GrantStatusActive, grant.Status)
		assert.True(t, grant.ConsentBypass)
		assert.Equal(t, "Police Act s.12", *grant.LegalBasis)
		expiresAt, err := time.Parse(time.RFC3339, grant.ExpiresAt)
		require.NoError(t, err)
		assert.WithinDuration(t, time.Now().AddDate(0, 1, 0), expiresAt, 5*time.Second)
	}

	got, err := service.GetAccessGrant(grants[0].ID)
	require.NoError(t, err)
	assert.True(t, got.ConsentBypass)

	revoked, err := service.RevokeAccessGrant(grants[0].ID)
	require.NoError(t, err)
	assert.Equal(t, GrantStatusRevoked, revoked.Status)
	assert.True(t, revoked.ConsentBypass)
}

func TestConsentBypass_Validation(t *testing.T) {
	service := newBypassService(t)

	tests := map[string]struct {
		change func(*AccessGrantCreateRequest)
		want   error
	}{
		"no legal basis":        {func(r *AccessGrantCreateRequest) { r.LegalBasis = " " }, ErrInvalidGrantRequest},
		"legal basis for plain": {func(r *AccessGrantCreateRequest) { r.ConsentBypass = false }, ErrInvalidGrantRequest},
		"no application":        {func(r *AccessGrantCreateRequest) { r.ApplicationID = "" }, ErrInvalidGrantRequest},
		"bad duration":          {func(r *AccessGrantCreateRequest) { r.GrantDuration = "7d" }, ErrInvalidGrantRequest},
		"unknown field":         {func(r *AccessGrantCreateRequest) { r.Records[0].FieldName = "nope" }, ErrFieldNotFound},
		"allowlist field":       {func(r *AccessGrantCreateRequest) { r.Records[0].FieldName = "allowlist" }, ErrInvalidGrantRequest},
		"open field":            {func(r *AccessGrantCreateRequest) { r.Records[0].FieldName = "open" }, ErrInvalidGrantRequest},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			req := bypassRequest("police", "consent")
			tt.change(req)
			_, err := service.CreateAccessGrants(req)
			assert.ErrorIs(t, err, tt.want)
		})
	}

	// A rejected field fails the whole request: nothing is written.
	_, err := service.CreateAccessGrants(bypassRequest("police", "consent", "allowlist"))
	assert.ErrorIs(t, err, ErrInvalidGrantRequest)
	all, err := service.ListAccessGrants(AccessGrantFilter{})
	require.NoError(t, err)
	assert.Empty(t, all.Records)
}

func TestConsentBypass_PlainGrantsLeaveBypassGrantsAlone(t *testing.T) {
	service := newBypassService(t)
	grant := grantBypass(t, service, "police", "allowlist_consent")[0]

	// Renewing or creating plain access must not touch the bypass grant's expiry.
	plain := grantPlain(t, service, "police", GrantDurationTypeOneYear, "allowlist_consent")
	assert.NotEqual(t, grant.ID, plain.ID)
	assert.False(t, plain.ConsentBypass)

	bypass, err := service.GetAccessGrant(grant.ID)
	require.NoError(t, err)
	assert.Equal(t, grant.ExpiresAt, bypass.ExpiresAt)

	plainOnly := false
	listed, err := service.ListAccessGrants(AccessGrantFilter{ApplicationID: "police", ConsentBypass: &plainOnly})
	require.NoError(t, err)
	assert.Len(t, listed.Records, 1)
}

func TestConsentBypass_RenewalUpdatesTheGrant(t *testing.T) {
	service := newBypassService(t)
	first := grantBypass(t, service, "police", "consent")[0]

	req := bypassRequest("police", "consent")
	req.GrantDuration = GrantDurationTypeOneYear
	req.LegalBasis = "Police Act s.14"
	resp, err := service.CreateAccessGrants(req)
	require.NoError(t, err)
	renewed := resp.Records[0]

	assert.Equal(t, first.ID, renewed.ID, "the active bypass grant is renewed, not duplicated")
	assert.Equal(t, "Police Act s.14", *renewed.LegalBasis)
	expiresAt, err := time.Parse(time.RFC3339, renewed.ExpiresAt)
	require.NoError(t, err)
	assert.WithinDuration(t, time.Now().AddDate(1, 0, 0), expiresAt, 5*time.Second)
}
