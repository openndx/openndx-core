# Allow-listed applications read the field only with the owner's consent,
# unless their grant is a consent bypass grant.
package openndx.classes.allowlist_consent

import data.openndx.lib

decision := lib.deny("not_allowlisted") if {
	not lib.has_grant
} else := lib.expired if {
	lib.grant_expired
} else := lib.consent_bypass_allow if {
	lib.consent_bypassed
} else := lib.consent_required
