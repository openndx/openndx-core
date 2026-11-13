package decision_test

import (
	"testing"
	"time"

	"github.com/openndx/openndx-core/internal/pdp/decision"
	"github.com/openndx/openndx-core/internal/pdp/fieldpolicy"
	"github.com/openndx/openndx-core/internal/pdp/grant"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// newBypassService returns a service with one field for each combination of
// the policy flags in schema s1, named after the flags it sets, and its
// database.
func newBypassService(t *testing.T) (testService, *gorm.DB) {
	t.Helper()
	db := setupTestDB(t)
	field := func(name string, allowlist, consent bool) fieldpolicy.PolicyMetadataCreateRequestRecord {
		return fieldpolicy.PolicyMetadataCreateRequestRecord{
			FieldName: name, Source: fieldpolicy.SourcePrimary,
			RequiresAllowlist: new(allowlist), RequiresConsent: new(consent),
		}
	}
	_, err := fieldpolicy.NewService(db).CreatePolicyMetadata(&fieldpolicy.PolicyMetadataCreateRequest{
		SchemaID: "s1",
		Records: []fieldpolicy.PolicyMetadataCreateRequestRecord{
			field("consent", false, true),
			field("allowlist_consent", true, true),
			field("allowlist", true, false),
			field("open", false, false),
		},
	})
	require.NoError(t, err)
	return newTestService(db), db
}

func grantBypass(t *testing.T, service testService, app string, fields ...string) []grant.AccessGrantResponse {
	t.Helper()
	records := make([]grant.AccessGrantCreateRequestRecord, 0, len(fields))
	for _, field := range fields {
		records = append(records, grant.AccessGrantCreateRequestRecord{SchemaID: "s1", FieldName: field})
	}
	resp, err := service.CreateAccessGrants(&grant.AccessGrantCreateRequest{
		ApplicationID: app,
		Records:       records,
		GrantDuration: grant.GrantDurationTypeOneMonth,
		ConsentBypass: true,
		LegalBasis:    "Police Act s.12",
	})
	require.NoError(t, err)
	return resp.Records
}

func grantPlain(t *testing.T, service testService, app string, duration grant.GrantDurationType, field string) grant.AccessGrantResponse {
	t.Helper()
	resp, err := service.CreateAccessGrants(&grant.AccessGrantCreateRequest{
		ApplicationID: app,
		GrantDuration: duration,
		Records:       []grant.AccessGrantCreateRequestRecord{{SchemaID: "s1", FieldName: field}},
	})
	require.NoError(t, err)
	return resp.Records[0]
}

func decideField(t *testing.T, service testService, app, field string) decision.PolicyDecisionFieldOutcome {
	t.Helper()
	resp, err := service.GetPolicyDecision(t.Context(), &decision.PolicyDecisionRequest{
		ApplicationID:  app,
		RequiredFields: []decision.PolicyDecisionRequestRecord{{SchemaID: "s1", FieldName: field}},
	})
	require.NoError(t, err)
	require.Len(t, resp.Fields, 1)
	return resp.Fields[0]
}

var bypassed = decision.PolicyDecisionFieldOutcome{
	Outcome: "allow", Reason: "consent_bypassed", Obligations: []string{"audit.consent_bypassed"},
}

func withField(outcome decision.PolicyDecisionFieldOutcome, field string) decision.PolicyDecisionFieldOutcome {
	outcome.SchemaID, outcome.FieldName = "s1", field
	return outcome
}

func TestConsentBypass_GrantSkipsConsent(t *testing.T) {
	service, _ := newBypassService(t)
	grantBypass(t, service, "police", "consent", "allowlist_consent")

	// Both fields that require consent are bypassed for the granted application only.
	assert.Equal(t, withField(bypassed, "consent"), decideField(t, service, "police", "consent"))
	assert.Equal(t, withField(bypassed, "allowlist_consent"), decideField(t, service, "police", "allowlist_consent"))
	assert.Equal(t, "consent_required", decideField(t, service, "other-app", "consent").Outcome)
	assert.Equal(t, "deny", decideField(t, service, "other-app", "allowlist_consent").Outcome)
}

func TestConsentBypass_Revoke(t *testing.T) {
	service, _ := newBypassService(t)

	// The application is also allow-listed for the allowlist_consent field.
	grantPlain(t, service, "police", grant.GrantDurationTypeOneMonth, "allowlist_consent")
	bypass := grantBypass(t, service, "police", "allowlist_consent")[0]
	assert.Equal(t, withField(bypassed, "allowlist_consent"), decideField(t, service, "police", "allowlist_consent"))

	_, err := service.RevokeAccessGrant(bypass.ID)
	require.NoError(t, err)

	// Back to the plain grant: consent is asked again.
	assert.Equal(t, "consent_required", decideField(t, service, "police", "allowlist_consent").Outcome)
}

func TestConsentBypass_Expiry(t *testing.T) {
	service, db := newBypassService(t)
	grantBypass(t, service, "police", "consent", "allowlist_consent")
	require.NoError(t, db.Model(&grant.AccessGrant{}).
		Where("consent_bypass = ?", true).
		Update("expires_at", time.Now().Add(-time.Minute)).Error)

	assert.Equal(t, "consent_required", decideField(t, service, "police", "consent").Outcome)
	assert.Equal(t, "expired", decideField(t, service, "police", "allowlist_consent").Outcome)

	// With a plain grant too, the plain grant applies once the bypass expires.
	grantPlain(t, service, "police", grant.GrantDurationTypeOneMonth, "allowlist_consent")
	assert.Equal(t, "consent_required", decideField(t, service, "police", "allowlist_consent").Outcome)
}

func TestConsentBypass_BypassGrantWinsOverPlainGrant(t *testing.T) {
	service, _ := newBypassService(t)
	grantBypass(t, service, "police", "allowlist_consent")
	grantPlain(t, service, "police", grant.GrantDurationTypeOneYear, "allowlist_consent")

	// The bypass grant still wins while it is unexpired.
	assert.Equal(t, withField(bypassed, "allowlist_consent"), decideField(t, service, "police", "allowlist_consent"))
}
