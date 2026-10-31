package engine

import (
	"testing"
	"time"

	"github.com/open-policy-agent/opa/v1/ast"
	"github.com/open-policy-agent/opa/v1/tester"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRegoPolicies runs the _test.rego files next to the embedded policies.
func TestRegoPolicies(t *testing.T) {
	sources, err := readModules(true)
	require.NoError(t, err)

	modules := make(map[string]*ast.Module, len(sources))
	for path, src := range sources {
		module, err := ast.ParseModule(path, src)
		require.NoError(t, err, path)
		modules[path] = module
	}

	results, err := tester.NewRunner().SetModules(modules).RunTests(t.Context(), nil)
	require.NoError(t, err)

	count := 0
	for result := range results {
		count++
		if !result.Pass() {
			t.Errorf("%s.%s: %s", result.Package, result.Name, result)
			if result.Error != nil {
				t.Errorf("  error: %v", result.Error)
			}
		}
	}
	assert.Positive(t, count, "no Rego tests found")
}

func TestEngine_Decide(t *testing.T) {
	e, err := New(t.Context())
	require.NoError(t, err)

	now := time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC)
	future := &Grant{ExpiresAt: now.Add(24 * time.Hour)}
	past := &Grant{ExpiresAt: now.Add(-time.Nanosecond)}

	tests := []struct {
		name  string
		class string
		grant *Grant
		want  Decision
	}{
		{"allowlist without grant", ClassAllowlist, nil, Decision{OutcomeDeny, "not_allowlisted"}},
		{"allowlist with expired grant", ClassAllowlist, past, Decision{OutcomeExpired, "grant_expired"}},
		{"allowlist with grant", ClassAllowlist, future, Decision{Outcome: OutcomeAllow}},
		{"allowlist_consent without grant", ClassAllowlistConsent, nil, Decision{OutcomeDeny, "not_allowlisted"}},
		{"allowlist_consent with expired grant", ClassAllowlistConsent, past, Decision{OutcomeExpired, "grant_expired"}},
		{"allowlist_consent with grant", ClassAllowlistConsent, future, Decision{Outcome: OutcomeConsentRequired}},
		{"unknown class", "no_such_class", future, Decision{OutcomeDeny, "unknown_policy_class"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := e.Decide(t.Context(), Input{
				App:   App{ID: "app-1"},
				Field: Field{SchemaID: "drp", FieldName: "person.fullName", PolicyClass: tt.class},
				Grant: tt.grant,
				Now:   now,
			})
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestToDecision_RejectsUnknownOutcome(t *testing.T) {
	_, err := toDecision(map[string]any{"outcome": "maybe"})
	assert.Error(t, err)

	_, err = toDecision("allow")
	assert.Error(t, err)
}
