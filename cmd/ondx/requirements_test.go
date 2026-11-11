package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseRequirements(t *testing.T) {
	tests := []struct {
		in                 string
		allowlist, consent bool
	}{
		{"allowlist", true, false},
		{"consent", false, true},
		{"allowlist+consent", true, true},
		{"consent+allowlist", true, true},
		{"none", false, false},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			allowlist, consent, err := parseRequirements(tt.in)
			assert.NoError(t, err)
			assert.Equal(t, tt.allowlist, allowlist, "allowlist")
			assert.Equal(t, tt.consent, consent, "consent")
		})
	}

	// Removed policy class names and malformed lists are rejected.
	for _, in := range []string{"", "open", "allowlist_consent", "allowlist+", "consent+consent", "none+consent"} {
		_, _, err := parseRequirements(in)
		assert.Error(t, err, in)
	}
}

func TestFormatRequirements(t *testing.T) {
	for _, in := range []string{"allowlist", "consent", "allowlist+consent", "none"} {
		allowlist, consent, err := parseRequirements(in)
		assert.NoError(t, err)
		assert.Equal(t, in, formatRequirements(allowlist, consent))
	}
}
