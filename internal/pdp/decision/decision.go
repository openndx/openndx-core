// Package decision decides access to one field from two flags on the field,
// requiresAllowlist and requiresConsent, and the requesting application's
// grant for it.
package decision

import "time"

// Outcome is the decision for one field.
type Outcome string

const (
	OutcomeAllow           Outcome = "allow"
	OutcomeDeny            Outcome = "deny"
	OutcomeExpired         Outcome = "expired"
	OutcomeConsentRequired Outcome = "consent_required"
)

// Reasons explain an outcome.
const (
	ReasonNoApplication   = "no_application"
	ReasonNotAllowlisted  = "not_allowlisted"
	ReasonGrantExpired    = "grant_expired"
	ReasonConsentBypassed = "consent_bypassed"
)

// ObligationAuditConsentBypassed tells the Orchestration Engine to record that
// the field was read without the owner's consent through a bypass grant.
const ObligationAuditConsentBypassed = "audit.consent_bypassed"

// Field is the access policy of one field.
type Field struct {
	// RequiresAllowlist means the application needs an unexpired grant for the field.
	RequiresAllowlist bool
	// RequiresConsent means the owner must consent, unless the application
	// holds an unexpired consent bypass grant.
	RequiresConsent bool
}

// Grant is the requesting application's active grant for the field.
type Grant struct {
	ExpiresAt time.Time
	// ConsentBypass marks a grant that lets the application skip the owner's
	// consent while it is unexpired.
	ConsentBypass bool
}

// Expired reports whether the grant has expired at now. A grant is valid up to
// and including its expiry.
func (g Grant) Expired(now time.Time) bool {
	return now.After(g.ExpiresAt)
}

// Decision is the result for one field. Obligations name extra actions the
// Orchestration Engine must take, such as recording that consent was bypassed.
type Decision struct {
	Outcome     Outcome
	Reason      string
	Obligations []string
}

// Decide returns the decision for one field. appID is the requesting
// application's ID and grant its active grant for the field, or nil. Checks
// apply in order:
//
//  1. No application: deny.
//  2. The field requires the allow-list and the application has no grant: deny.
//  3. The field requires the allow-list and the grant has expired: expired.
//  4. The field requires consent and the grant is an unexpired consent bypass
//     grant: allow, with the audit.consent_bypassed obligation.
//  5. The field requires consent: consent required.
//  6. Otherwise: allow.
//
// A bypass grant also counts as the application's allow-list entry.
func Decide(field Field, appID string, grant *Grant, now time.Time) Decision {
	if appID == "" {
		return Decision{Outcome: OutcomeDeny, Reason: ReasonNoApplication}
	}
	if field.RequiresAllowlist {
		if grant == nil {
			return Decision{Outcome: OutcomeDeny, Reason: ReasonNotAllowlisted}
		}
		if grant.Expired(now) {
			return Decision{Outcome: OutcomeExpired, Reason: ReasonGrantExpired}
		}
	}
	if field.RequiresConsent {
		if grant != nil && grant.ConsentBypass && !grant.Expired(now) {
			return Decision{
				Outcome:     OutcomeAllow,
				Reason:      ReasonConsentBypassed,
				Obligations: []string{ObligationAuditConsentBypassed},
			}
		}
		return Decision{Outcome: OutcomeConsentRequired}
	}
	return Decision{Outcome: OutcomeAllow}
}
