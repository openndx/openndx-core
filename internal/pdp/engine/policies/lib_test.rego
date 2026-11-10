package openndx.lib_test

import data.openndx.lib

now := "2026-10-31T12:00:00Z"

bypass := {"expiresAt": "2027-01-01T00:00:00Z", "consentBypass": true}

test_bypass_while_unexpired if {
	lib.consent_bypassed with input as {"now": now, "grant": bypass}
}

test_no_bypass_after_expiry if {
	not lib.consent_bypassed with input as {"now": now, "grant": object.union(bypass, {"expiresAt": "2026-10-31T11:59:59Z"})}
}

test_no_bypass_for_plain_grant if {
	not lib.consent_bypassed with input as {"now": now, "grant": object.union(bypass, {"consentBypass": false})}
	not lib.consent_bypassed with input as {"now": now, "grant": {"expiresAt": "2027-01-01T00:00:00Z"}}
}

test_no_bypass_without_grant if {
	not lib.consent_bypassed with input as {"now": now}
}
