# Any authenticated application reads the field, with no allow-list entry and
# no owner consent. The application ID is still required, so every access is
# attributable in the audit trail.
package openndx.classes.open

import data.openndx.lib

decision := lib.deny("no_application") if {
	not lib.has_app
} else := lib.allow
