// Package decision owns policy decisions. Decide decides access to one field
// from two flags on the field, requiresAllowlist and requiresConsent, and the
// requesting application's grant for it.
//
// Service decides every field of a request and derives the legacy outcome
// lists, and Handler serves /api/v1/policy/decide. The service reads fields
// and grants through the ports it declares, FieldReader and GrantReader, which
// fieldpolicy and grant implement. This package imports neither of them.
package decision
