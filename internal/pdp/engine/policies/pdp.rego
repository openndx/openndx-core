# Router: sends each requested field to the policy class named in its metadata.
#
# Input (one field per evaluation):
#   app.id             requesting application's OIDC client_id
#   field.schemaId     provider schema of the field
#   field.fieldName    field path, for example person.fullName
#   field.policyClass  name of a package under data.openndx.classes
#   grant.expiresAt    RFC 3339 expiry of the app's grant; absent when the app has none
#   now                RFC 3339 evaluation time
#
# Output: {"outcome": "allow" | "deny" | "expired" | "consent_required", "reason": string}
package openndx.pdp

default decision := {"outcome": "deny", "reason": "unknown_policy_class"}

decision := data.openndx.classes[input.field.policyClass].decision
