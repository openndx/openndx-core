package openndx.classes.allowlist_consent_test

import data.openndx.pdp

now := "2026-10-31T12:00:00Z"

field := {"schemaId": "drp", "fieldName": "person.fullName", "policyClass": "allowlist_consent"}

test_denies_without_grant if {
	pdp.decision == {"outcome": "deny", "reason": "not_allowlisted"} with input as {"now": now, "field": field}
}

test_expired_grant if {
	pdp.decision == {"outcome": "expired", "reason": "grant_expired"} with input as {
		"now": now,
		"field": field,
		"grant": {"expiresAt": "2026-10-31T11:59:59Z"},
	}
}

test_requires_consent_with_grant if {
	pdp.decision == {"outcome": "consent_required"} with input as {
		"now": now,
		"field": field,
		"grant": {"expiresAt": "2027-01-01T00:00:00Z"},
	}
}
