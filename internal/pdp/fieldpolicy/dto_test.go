package fieldpolicy

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPolicyMetadataPatchRequest_NullFlag(t *testing.T) {
	var req PolicyMetadataPatchRequest
	assert.Empty(t, req.NullFlag())

	req.RequiresConsent = OptionalPatchField[bool]{Set: true}
	assert.Equal(t, "requiresConsent", req.NullFlag())

	req.RequiresAllowlist = OptionalPatchField[bool]{Set: true}
	assert.Equal(t, "requiresAllowlist", req.NullFlag())

	set := false
	req = PolicyMetadataPatchRequest{RequiresAllowlist: OptionalPatchField[bool]{Set: true, Value: &set}}
	assert.Empty(t, req.NullFlag())
}
