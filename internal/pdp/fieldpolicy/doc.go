// Package fieldpolicy owns the field policy catalogue: the policy of every
// field a schema exposes, stored in field_policies. Each field carries
// two flags, requiresAllowlist and requiresConsent, plus its display name,
// description, source and owner.
//
// Handler serves /api/v1/policy/fields. Service.FieldPolicies implements
// decision.FieldReader, the port policy decisions read fields through.
package fieldpolicy
