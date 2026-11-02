package openndx.pdp_test

import data.openndx.pdp

now := "2026-10-31T12:00:00Z"

test_unknown_class_denies if {
	pdp.decision == {"outcome": "deny", "reason": "unknown_policy_class"} with input as {
		"now": now,
		"field": {"policyClass": "no_such_class"},
		"grant": {"expiresAt": "2027-01-01T00:00:00Z"},
	}
}

test_missing_class_denies if {
	pdp.decision == {"outcome": "deny", "reason": "unknown_policy_class"} with input as {"now": now, "field": {}}
}
