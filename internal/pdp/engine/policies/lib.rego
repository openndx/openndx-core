# Helpers shared by the policy classes.
package openndx.lib

has_grant if input.grant

grant_expired if time.parse_rfc3339_ns(input.now) > time.parse_rfc3339_ns(input.grant.expiresAt)

allow := {"outcome": "allow"}

consent_required := {"outcome": "consent_required"}

expired := {"outcome": "expired", "reason": "grant_expired"}

deny(reason) := {"outcome": "deny", "reason": reason}
