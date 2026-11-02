// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Lanka Software Foundation

package authn

import (
	"encoding/json"
	"fmt"
	"strings"
)

// rolesClaim is the claim authn reads roles from. It must stay in sync with the
// json tag on tokenClaims.Roles.
const rolesClaim = "roles"

// standardClaimNames lists the canonical names recognized by typed validation.
// Input matching is case-insensitive, preserving encoding/json's struct binding;
// the unified Claims view uses these exact names for lookup.
var standardClaimNames = []string{
	"sub", "client_id", "scope", rolesClaim,
	"email", "ouId", "ouHandle",
	"iss", "aud", "exp", "nbf", "iat", "jti",
}

// rolesValue captures the raw "roles" claim without imposing a shape at JSON
// decode time, so parseRolesClaim can judge it: binding the field as []string
// would reject a wrong-shaped claim with an opaque encoding/json error, and
// would leave role strings untrimmed.
type rolesValue struct{ value any }

func (r *rolesValue) UnmarshalJSON(data []byte) error {
	return json.Unmarshal(data, &r.value)
}

// parseRolesClaim converts an already-decoded roles claim value into the roles
// slice, enforcing exactly the shape a []string field would: absent or JSON
// null yields no roles; an array of strings yields those roles (an empty array
// yields an empty, non-nil slice); anything else rejects the token. Role
// strings are whitespace-trimmed, since a role of " Trader " can never match a
// check for "Trader".
//
// Only the OAuth2 "scope" claim accepts a space-delimited string. Doing
// the same for roles would turn
// `"roles": "Customs Officer"` into two roles and let a check for "Customs"
// pass for a principal that was never granted it.
//
// Rejecting rather than ignoring a wrong-shaped value is also deliberate: roles
// feed authz.Principal, and a silently empty roles slice reads as "granted
// nothing" — which fails closed for a require-role check, but fails OPEN for
// any check phrased as "deny if the principal has role X".
func parseRolesClaim(value any) ([]string, error) {
	switch v := value.(type) {
	case nil:
		return nil, nil
	case []any:
		roles := make([]string, 0, len(v))
		for _, item := range v {
			role, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("jwt claim %q must be an array of strings", rolesClaim)
			}
			roles = append(roles, strings.TrimSpace(role))
		}
		return roles, nil
	default:
		return nil, fmt.Errorf("jwt claim %q must be an array of strings", rolesClaim)
	}
}
