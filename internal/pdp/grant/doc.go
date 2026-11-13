// Package grant owns application access grants, stored in access_grants: an
// application's plain or consent bypass grant for one field, with an expiry.
// A field can hold both kinds of grant for the same application.
//
// Handler serves /api/v1/policy/grants. Service.ActiveGrants implements
// decision.GrantReader, the port policy decisions read grants through. To
// check the fields it grants, the package reads policy_metadata through its
// own minimal model, so it does not depend on fieldpolicy.
package grant
