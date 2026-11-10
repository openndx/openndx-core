package services

import (
	"testing"
	"time"

	"github.com/openndx/openndx-core/internal/pdp/engine"
	"github.com/openndx/openndx-core/internal/pdp/models"
	"github.com/openndx/openndx-core/internal/pdp/testhelpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newBypassService returns a service with one field of each built-in class in
// schema s1: consent, allowlist_consent, allowlist and open.
func newBypassService(t *testing.T) *PolicyMetadataService {
	t.Helper()
	service := NewPolicyMetadataService(setupTestDB(t), testhelpers.Engine(t))
	_, err := service.CreatePolicyMetadata(&models.PolicyMetadataCreateRequest{
		SchemaID: "s1",
		Records: []models.PolicyMetadataCreateRequestRecord{
			{FieldName: "consent", Source: models.SourcePrimary, PolicyClass: engine.ClassConsent},
			{FieldName: "allowlist_consent", Source: models.SourcePrimary, PolicyClass: engine.ClassAllowlistConsent},
			{FieldName: "allowlist", Source: models.SourcePrimary, PolicyClass: engine.ClassAllowlist},
			{FieldName: "open", Source: models.SourcePrimary, PolicyClass: engine.ClassOpen},
		},
	})
	require.NoError(t, err)
	return service
}

func bypassRequest(app string, fields ...string) *models.AccessGrantCreateRequest {
	records := make([]models.AccessGrantCreateRequestRecord, 0, len(fields))
	for _, field := range fields {
		records = append(records, models.AccessGrantCreateRequestRecord{SchemaID: "s1", FieldName: field})
	}
	return &models.AccessGrantCreateRequest{
		ApplicationID: app,
		Records:       records,
		GrantDuration: models.GrantDurationTypeOneMonth,
		ConsentBypass: true,
		LegalBasis:    "Police Act s.12",
	}
}

func grantBypass(t *testing.T, service *PolicyMetadataService, app string, fields ...string) []models.AccessGrantResponse {
	t.Helper()
	resp, err := service.CreateAccessGrants(bypassRequest(app, fields...))
	require.NoError(t, err)
	return resp.Records
}

func grantPlain(t *testing.T, service *PolicyMetadataService, app string, duration models.GrantDurationType, field string) models.AccessGrantResponse {
	t.Helper()
	resp, err := service.CreateAccessGrants(&models.AccessGrantCreateRequest{
		ApplicationID: app,
		GrantDuration: duration,
		Records:       []models.AccessGrantCreateRequestRecord{{SchemaID: "s1", FieldName: field}},
	})
	require.NoError(t, err)
	return resp.Records[0]
}

func decideField(t *testing.T, service *PolicyMetadataService, app, field string) models.PolicyDecisionFieldOutcome {
	t.Helper()
	resp, err := service.GetPolicyDecision(t.Context(), &models.PolicyDecisionRequest{
		ApplicationID:  app,
		RequiredFields: []models.PolicyDecisionRequestRecord{{SchemaID: "s1", FieldName: field}},
	})
	require.NoError(t, err)
	require.Len(t, resp.Fields, 1)
	return resp.Fields[0]
}

var bypassed = models.PolicyDecisionFieldOutcome{
	Outcome: "allow", Reason: "consent_bypassed", Obligations: []string{"audit.consent_bypassed"},
}

func withField(outcome models.PolicyDecisionFieldOutcome, field string) models.PolicyDecisionFieldOutcome {
	outcome.SchemaID, outcome.FieldName = "s1", field
	return outcome
}

func TestConsentBypass_GrantSkipsConsent(t *testing.T) {
	service := newBypassService(t)

	grants := grantBypass(t, service, "police", "consent", "allowlist_consent")
	require.Len(t, grants, 2)
	for _, grant := range grants {
		assert.Equal(t, models.GrantStatusActive, grant.Status)
		assert.True(t, grant.ConsentBypass)
		assert.Equal(t, "Police Act s.12", *grant.LegalBasis)
		expiresAt, err := time.Parse(time.RFC3339, grant.ExpiresAt)
		require.NoError(t, err)
		assert.WithinDuration(t, time.Now().AddDate(0, 1, 0), expiresAt, 5*time.Second)
	}

	// Both consent classes are bypassed for the granted application only.
	assert.Equal(t, withField(bypassed, "consent"), decideField(t, service, "police", "consent"))
	assert.Equal(t, withField(bypassed, "allowlist_consent"), decideField(t, service, "police", "allowlist_consent"))
	assert.Equal(t, "consent_required", decideField(t, service, "other-app", "consent").Outcome)
	assert.Equal(t, "deny", decideField(t, service, "other-app", "allowlist_consent").Outcome)

	got, err := service.GetAccessGrant(grants[0].ID)
	require.NoError(t, err)
	assert.True(t, got.ConsentBypass)
	_, err = service.GetAccessGrant("not-a-uuid")
	assert.ErrorIs(t, err, ErrAccessGrantNotFound)
}

func TestConsentBypass_Validation(t *testing.T) {
	service := newBypassService(t)

	tests := map[string]struct {
		change func(*models.AccessGrantCreateRequest)
		want   error
	}{
		"no legal basis":        {func(r *models.AccessGrantCreateRequest) { r.LegalBasis = " " }, ErrInvalidGrantRequest},
		"legal basis for plain": {func(r *models.AccessGrantCreateRequest) { r.ConsentBypass = false }, ErrInvalidGrantRequest},
		"no application":        {func(r *models.AccessGrantCreateRequest) { r.ApplicationID = "" }, ErrInvalidGrantRequest},
		"bad duration":          {func(r *models.AccessGrantCreateRequest) { r.GrantDuration = "7d" }, ErrInvalidGrantRequest},
		"unknown field":         {func(r *models.AccessGrantCreateRequest) { r.Records[0].FieldName = "nope" }, ErrPolicyMetadataNotFound},
		"allowlist field":       {func(r *models.AccessGrantCreateRequest) { r.Records[0].FieldName = "allowlist" }, ErrInvalidGrantRequest},
		"open field":            {func(r *models.AccessGrantCreateRequest) { r.Records[0].FieldName = "open" }, ErrInvalidGrantRequest},
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
	all, err := service.ListAccessGrants(models.AccessGrantFilter{})
	require.NoError(t, err)
	assert.Empty(t, all.Records)
}

func TestConsentBypass_Revoke(t *testing.T) {
	service := newBypassService(t)

	// The application is also allow-listed for the allowlist_consent field.
	grantPlain(t, service, "police", models.GrantDurationTypeOneMonth, "allowlist_consent")
	grant := grantBypass(t, service, "police", "allowlist_consent")[0]
	assert.Equal(t, withField(bypassed, "allowlist_consent"), decideField(t, service, "police", "allowlist_consent"))

	revoked, err := service.RevokeAccessGrant(grant.ID)
	require.NoError(t, err)
	assert.Equal(t, models.GrantStatusRevoked, revoked.Status)
	assert.True(t, revoked.ConsentBypass)

	// Back to the plain grant: consent is asked again.
	assert.Equal(t, "consent_required", decideField(t, service, "police", "allowlist_consent").Outcome)
}

func TestConsentBypass_Expiry(t *testing.T) {
	service := newBypassService(t)
	grantBypass(t, service, "police", "consent", "allowlist_consent")
	require.NoError(t, service.db.Model(&models.AccessGrant{}).
		Where("consent_bypass = ?", true).
		Update("expires_at", time.Now().Add(-time.Minute)).Error)

	assert.Equal(t, "consent_required", decideField(t, service, "police", "consent").Outcome)
	assert.Equal(t, "expired", decideField(t, service, "police", "allowlist_consent").Outcome)

	// With a plain grant too, the plain grant applies once the bypass expires.
	grantPlain(t, service, "police", models.GrantDurationTypeOneMonth, "allowlist_consent")
	assert.Equal(t, "consent_required", decideField(t, service, "police", "allowlist_consent").Outcome)
}

func TestConsentBypass_PlainGrantsLeaveBypassGrantsAlone(t *testing.T) {
	service := newBypassService(t)
	grant := grantBypass(t, service, "police", "allowlist_consent")[0]

	// Renewing or creating plain access must not touch the bypass grant's expiry.
	plain := grantPlain(t, service, "police", models.GrantDurationTypeOneYear, "allowlist_consent")
	assert.NotEqual(t, grant.ID, plain.ID)
	assert.False(t, plain.ConsentBypass)

	bypass, err := service.GetAccessGrant(grant.ID)
	require.NoError(t, err)
	assert.Equal(t, grant.ExpiresAt, bypass.ExpiresAt)

	plainOnly := false
	listed, err := service.ListAccessGrants(models.AccessGrantFilter{ApplicationID: "police", ConsentBypass: &plainOnly})
	require.NoError(t, err)
	assert.Len(t, listed.Records, 1)

	// The bypass grant still wins while it is unexpired.
	assert.Equal(t, withField(bypassed, "allowlist_consent"), decideField(t, service, "police", "allowlist_consent"))
}

func TestConsentBypass_RenewalUpdatesTheGrant(t *testing.T) {
	service := newBypassService(t)
	first := grantBypass(t, service, "police", "consent")[0]

	req := bypassRequest("police", "consent")
	req.GrantDuration = models.GrantDurationTypeOneYear
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

func TestPreferredGrant(t *testing.T) {
	now := time.Now()
	plain := &models.AccessGrant{ExpiresAt: now.Add(time.Hour)}
	bypass := &models.AccessGrant{ConsentBypass: true, ExpiresAt: now.Add(time.Hour)}
	expiredBypass := &models.AccessGrant{ConsentBypass: true, ExpiresAt: now.Add(-time.Hour)}

	assert.Same(t, plain, preferredGrant(nil, plain, now))
	assert.Same(t, bypass, preferredGrant(plain, bypass, now))
	assert.Same(t, bypass, preferredGrant(bypass, plain, now))
	assert.Same(t, plain, preferredGrant(expiredBypass, plain, now))
	assert.Same(t, plain, preferredGrant(plain, expiredBypass, now))
	assert.Same(t, expiredBypass, preferredGrant(nil, expiredBypass, now))
}
