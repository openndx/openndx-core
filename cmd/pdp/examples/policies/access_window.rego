# Example custom policy class. Allow-listed applications read the field only
# inside a time window set per field, for example while a census is running.
#
# Parameters, set as the field's policyParams:
#   notBefore  RFC 3339 start of the window, inclusive
#   notAfter   RFC 3339 end of the window, inclusive
#
# See ../../CUSTOM_POLICY_CLASSES.md.
package openndx.classes.access_window

import data.openndx.lib

decision := lib.deny("invalid_params") if {
	not valid_params
} else := lib.deny("not_allowlisted") if {
	not lib.has_grant
} else := lib.expired if {
	lib.grant_expired
} else := lib.deny("outside_access_window") if {
	not in_window
} else := lib.allow

# Missing parameters leave this undefined, so the field is denied.
valid_params if {
	time.parse_rfc3339_ns(input.field.params.notBefore) <= time.parse_rfc3339_ns(input.field.params.notAfter)
}

in_window if {
	now := time.parse_rfc3339_ns(input.now)
	now >= time.parse_rfc3339_ns(input.field.params.notBefore)
	now <= time.parse_rfc3339_ns(input.field.params.notAfter)
}
