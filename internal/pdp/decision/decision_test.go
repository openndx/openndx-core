package decision

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestDecide(t *testing.T) {
	now := time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC)
	future := &Grant{ExpiresAt: now.Add(24 * time.Hour)}
	past := &Grant{ExpiresAt: now.Add(-time.Nanosecond)}
	atExpiry := &Grant{ExpiresAt: now}
	bypass := &Grant{ExpiresAt: now.Add(24 * time.Hour), ConsentBypass: true}
	expiredBypass := &Grant{ExpiresAt: now.Add(-time.Nanosecond), ConsentBypass: true}

	allowlist := Field{RequiresAllowlist: true}
	allowlistConsent := Field{RequiresAllowlist: true, RequiresConsent: true}
	consent := Field{RequiresConsent: true}
	public := Field{}

	allow := Decision{Outcome: OutcomeAllow}
	notAllowlisted := Decision{Outcome: OutcomeDeny, Reason: ReasonNotAllowlisted}
	expired := Decision{Outcome: OutcomeExpired, Reason: ReasonGrantExpired}
	consentRequired := Decision{Outcome: OutcomeConsentRequired}
	bypassed := Decision{
		Outcome:     OutcomeAllow,
		Reason:      ReasonConsentBypassed,
		Obligations: []string{ObligationAuditConsentBypassed},
	}
	noApp := Decision{Outcome: OutcomeDeny, Reason: ReasonNoApplication}

	tests := []struct {
		name  string
		field Field
		appID string
		grant *Grant
		want  Decision
	}{
		{"allowlist, no grant", allowlist, "app-1", nil, notAllowlisted},
		{"allowlist, expired grant", allowlist, "app-1", past, expired},
		{"allowlist, grant at its expiry", allowlist, "app-1", atExpiry, allow},
		{"allowlist, grant", allowlist, "app-1", future, allow},
		{"allowlist, bypass grant is plain access", allowlist, "app-1", bypass, allow},
		{"allowlist, expired bypass grant", allowlist, "app-1", expiredBypass, expired},
		{"allowlist, no application", allowlist, "", future, noApp},

		{"allowlist and consent, no grant", allowlistConsent, "app-1", nil, notAllowlisted},
		{"allowlist and consent, expired grant", allowlistConsent, "app-1", past, expired},
		{"allowlist and consent, grant at its expiry", allowlistConsent, "app-1", atExpiry, consentRequired},
		{"allowlist and consent, grant", allowlistConsent, "app-1", future, consentRequired},
		{"allowlist and consent, bypass grant", allowlistConsent, "app-1", bypass, bypassed},
		{"allowlist and consent, expired bypass grant", allowlistConsent, "app-1", expiredBypass, expired},
		{"allowlist and consent, no application", allowlistConsent, "", bypass, noApp},

		{"consent, no grant", consent, "app-1", nil, consentRequired},
		{"consent, expired grant", consent, "app-1", past, consentRequired},
		{"consent, grant at its expiry", consent, "app-1", atExpiry, consentRequired},
		{"consent, grant", consent, "app-1", future, consentRequired},
		{"consent, bypass grant", consent, "app-1", bypass, bypassed},
		{"consent, expired bypass grant", consent, "app-1", expiredBypass, consentRequired},
		{"consent, no application", consent, "", bypass, noApp},

		{"public, no grant", public, "app-1", nil, allow},
		{"public, expired grant", public, "app-1", past, allow},
		{"public, grant at its expiry", public, "app-1", atExpiry, allow},
		{"public, grant", public, "app-1", future, allow},
		{"public, bypass grant is plain access", public, "app-1", bypass, allow},
		{"public, expired bypass grant", public, "app-1", expiredBypass, allow},
		{"public, no application", public, "", nil, noApp},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, Decide(tt.field, tt.appID, tt.grant, now))
		})
	}
}
