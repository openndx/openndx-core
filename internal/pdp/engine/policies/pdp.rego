# Router: sends each requested field to the policy class named in its metadata.
#
# Input (one field per evaluation):
#   app.id             requesting application's OIDC client_id
#   field.schemaId     provider schema of the field
#   field.fieldName    field path, for example person.fullName
#   field.policyClass  name of a package under data.openndx.classes
#   field.params       the class's parameters for this field; absent when there are none
#   grant              the app's grant for the field; absent when the app has none
#     .expiresAt       RFC 3339 expiry
#     .consentBypass   true for a consent bypass grant
#   now                RFC 3339 evaluation time
#
# When the app holds both a plain and a consent bypass grant for the field, the
# PDP passes the bypass grant while it is unexpired, otherwise the plain one.
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
