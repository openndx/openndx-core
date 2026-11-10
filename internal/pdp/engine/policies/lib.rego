# Helpers shared by the policy classes.
package openndx.lib

has_app if input.app.id != ""

has_grant if input.grant

grant_expired if time.parse_rfc3339_ns(input.now) > time.parse_rfc3339_ns(input.grant.expiresAt)

# The application may skip the owner's consent: its grant is an unexpired
# consent bypass grant. The PDP passes active grants only. Classes that ask for
# consent use it to decide on bypass.
consent_bypassed if {
	input.grant.consentBypass == true
	not grant_expired
}

allow := {"outcome": "allow"}

# Allowed without the owner's consent through a bypass grant. The obligation
# tells the Orchestration Engine to record the bypass in the audit trail.
consent_bypass_allow := {
	"outcome": "allow",
	"reason": "consent_bypassed",
	"obligations": ["audit.consent_bypassed"],
}

consent_required := {"outcome": "consent_required"}

expired := {"outcome": "expired", "reason": "grant_expired"}

deny(reason) := {"outcome": "deny", "reason": reason}
