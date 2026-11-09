# Router: sends each requested field to the policy class named in its metadata.
#
# Input (one field per evaluation):
#   app.id             requesting application's OIDC client_id
#   field.schemaId     provider schema of the field
#   field.fieldName    field path, for example person.fullName
#   field.policyClass  name of a package under data.openndx.classes
#   field.params       the class's parameters for this field; absent when there are none
#   grant.expiresAt    RFC 3339 expiry of the app's grant; absent when the app has none
#   now                RFC 3339 evaluation time
#
# Output: {"outcome": "allow" | "deny" | "expired" | "consent_required",
#          "reason": string, "obligations": [string]}
#
# The outcome set is fixed: an unknown outcome denies the field. reason and
# obligations are optional. Obligations name extra actions the Orchestration
# Engine must take, such as "audit.consent_bypassed"; it records them in the
# audit trail with the field's outcome.
package openndx.pdp

default decision := {"outcome": "deny", "reason": "unknown_policy_class"}

decision := data.openndx.classes[input.field.policyClass].decision
