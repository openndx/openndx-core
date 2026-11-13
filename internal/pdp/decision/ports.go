package decision

import "context"

// FieldKey names one field of one schema.
type FieldKey struct {
	SchemaID  string
	FieldName string
}

// FieldPolicy is what a decision needs to know about one field: its access
// policy, and the details reported back for fields the application can't read
// outright.
type FieldPolicy struct {
	FieldKey
	Policy      Field
	DisplayName *string
	Description *string
	Owner       *string
}

// FieldGrant is an application's active grant for one field.
type FieldGrant struct {
	FieldKey
	Grant Grant
}

// FieldReader reads the policies of the fields a decision is asked about.
// *fieldpolicy.Service implements it.
type FieldReader interface {
	// FieldPolicies returns the policies of the named fields, in no particular
	// order. A key with no policy is left out of the result.
	FieldPolicies(ctx context.Context, keys []FieldKey) ([]FieldPolicy, error)
}

// GrantReader reads an application's active grants for the fields a decision
// is asked about. *grant.Service implements it.
type GrantReader interface {
	// ActiveGrants returns the application's active grants for the named
	// fields, in no particular order, expired ones included. A field can have
	// both a plain and a consent bypass grant.
	ActiveGrants(ctx context.Context, applicationID string, keys []FieldKey) ([]FieldGrant, error)
}
