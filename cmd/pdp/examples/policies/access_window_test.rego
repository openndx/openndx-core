package openndx.classes.access_window_test

import data.openndx.pdp

now := "2026-10-31T12:00:00Z"

window := {"notBefore": "2026-10-01T00:00:00Z", "notAfter": "2026-11-30T23:59:59Z"}

field := {"schemaId": "census", "fieldName": "household.size", "policyClass": "access_window", "params": window}

grant := {"expiresAt": "2027-01-01T00:00:00Z"}

test_allows_inside_window if {
	pdp.decision == {"outcome": "allow"} with input as {"now": now, "field": field, "grant": grant}
}

test_window_bounds_are_inclusive if {
	pdp.decision == {"outcome": "allow"} with input as {"now": window.notBefore, "field": field, "grant": grant}
	pdp.decision == {"outcome": "allow"} with input as {"now": window.notAfter, "field": field, "grant": grant}
}

test_denies_before_window if {
	pdp.decision == {"outcome": "deny", "reason": "outside_access_window"} with input as {
		"now": "2026-09-30T23:59:59Z",
		"field": field,
		"grant": grant,
	}
}

test_denies_after_window if {
	pdp.decision == {"outcome": "deny", "reason": "outside_access_window"} with input as {
		"now": "2026-12-01T00:00:00Z",
		"field": field,
		"grant": grant,
	}
}

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

test_denies_without_params if {
	pdp.decision == {"outcome": "deny", "reason": "invalid_params"} with input as {
		"now": now,
		"field": object.remove(field, ["params"]),
		"grant": grant,
	}
}

test_denies_reversed_window if {
	pdp.decision == {"outcome": "deny", "reason": "invalid_params"} with input as {
		"now": now,
		"field": object.union(field, {"params": {"notBefore": window.notAfter, "notAfter": window.notBefore}}),
		"grant": grant,
	}
}
