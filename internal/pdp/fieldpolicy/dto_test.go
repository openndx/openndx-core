package fieldpolicy

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPolicyPatchRequest_NullFlag(t *testing.T) {
	var req PolicyPatchRequest
	assert.Empty(t, req.NullFlag())

	req.RequiresConsent = OptionalPatchField[bool]{Set: true}
	assert.Equal(t, "requiresConsent", req.NullFlag())

	req.RequiresAllowlist = OptionalPatchField[bool]{Set: true}
	assert.Equal(t, "requiresAllowlist", req.NullFlag())

	set := false
	req = PolicyPatchRequest{RequiresAllowlist: OptionalPatchField[bool]{Set: true, Value: &set}}
	assert.Empty(t, req.NullFlag())
}
