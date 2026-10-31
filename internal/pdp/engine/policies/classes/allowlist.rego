# Allow-listed applications read the field without owner consent.
package openndx.classes.allowlist

import data.openndx.lib

decision := lib.deny("not_allowlisted") if {
	not lib.has_grant
} else := lib.expired if {
	lib.grant_expired
} else := lib.allow
