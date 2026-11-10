package openndx.classes.consent_test

import data.openndx.pdp

now := "2026-10-31T12:00:00Z"

app := {"id": "app-1"}

field := {"schemaId": "drp", "fieldName": "person.fullName", "policyClass": "consent"}

test_requires_consent_without_grant if {
	pdp.decision == {"outcome": "consent_required"} with input as {"now": now, "app": app, "field": field}
}

test_requires_consent_with_grant if {
	pdp.decision == {"outcome": "consent_required"} with input as {
		"now": now,
		"app": app,
		"field": field,
		"grant": {"expiresAt": "2027-01-01T00:00:00Z"},
	}
}

test_ignores_expired_grant if {
	pdp.decision == {"outcome": "consent_required"} with input as {
		"now": now,
		"app": app,
		"field": field,
		"grant": {"expiresAt": "2026-10-31T11:59:59Z"},
	}
}

bypass := {"expiresAt": "2027-01-01T00:00:00Z", "consentBypass": true}

bypassed := {"outcome": "allow", "reason": "consent_bypassed", "obligations": ["audit.consent_bypassed"]}

test_bypass_skips_consent if {
	pdp.decision == bypassed with input as {"now": now, "app": app, "field": field, "grant": bypass}
}

test_expired_bypass_requires_consent if {
	pdp.decision == {"outcome": "consent_required"} with input as {
		"now": now,
		"app": app,
		"field": field,
		"grant": object.union(bypass, {"expiresAt": "2026-10-31T11:59:59Z"}),
	}
}

test_bypass_without_application_denies if {
	pdp.decision == {"outcome": "deny", "reason": "no_application"} with input as {"now": now, "field": field, "grant": bypass}
}

test_denies_without_application if {
	pdp.decision == {"outcome": "deny", "reason": "no_application"} with input as {"now": now, "field": field}
}

test_denies_empty_application if {
	pdp.decision == {"outcome": "deny", "reason": "no_application"} with input as {"now": now, "app": {"id": ""}, "field": field}
}
