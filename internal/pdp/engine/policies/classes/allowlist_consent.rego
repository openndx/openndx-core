# Allow-listed applications read the field only with the owner's consent.
package openndx.classes.allowlist_consent

import data.openndx.lib

decision := lib.deny("not_allowlisted") if {
	not lib.has_grant
} else := lib.expired if {
	lib.grant_expired
} else := lib.consent_required
