// Package fieldpolicy owns the field policy catalogue: the policy metadata of
// every field a schema exposes, stored in policy_metadata. Each field carries
// two flags, requiresAllowlist and requiresConsent, plus its display name,
// description, source and owner.
//
// Handler serves /api/v1/policy/metadata. Service.FieldPolicies implements
// decision.FieldReader, the port policy decisions read fields through.
package fieldpolicy
