# Any authenticated application reads the field with the owner's consent. The
# allow-list is not checked, so an application without a grant, or with an
# expired one, still gets to ask the owner.
package openndx.classes.consent

import data.openndx.lib

decision := lib.deny("no_application") if {
	not lib.has_app
} else := lib.consent_required
