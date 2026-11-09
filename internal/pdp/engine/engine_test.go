package engine

import (
	"testing"
	"time"

	"github.com/open-policy-agent/opa/v1/ast"
	"github.com/open-policy-agent/opa/v1/rego"
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
		{"allowlist without grant", ClassAllowlist, nil, Decision{Outcome: OutcomeDeny, Reason: "not_allowlisted"}},
		{"allowlist with expired grant", ClassAllowlist, past, Decision{Outcome: OutcomeExpired, Reason: "grant_expired"}},
		{"allowlist with grant", ClassAllowlist, future, Decision{Outcome: OutcomeAllow}},
		{"allowlist_consent without grant", ClassAllowlistConsent, nil, Decision{Outcome: OutcomeDeny, Reason: "not_allowlisted"}},
		{"allowlist_consent with expired grant", ClassAllowlistConsent, past, Decision{Outcome: OutcomeExpired, Reason: "grant_expired"}},
		{"allowlist_consent with grant", ClassAllowlistConsent, future, Decision{Outcome: OutcomeConsentRequired}},
		{"consent without grant", ClassConsent, nil, Decision{Outcome: OutcomeConsentRequired}},
		{"consent with expired grant", ClassConsent, past, Decision{Outcome: OutcomeConsentRequired}},
		{"consent with grant", ClassConsent, future, Decision{Outcome: OutcomeConsentRequired}},
		{"unknown class", "no_such_class", future, Decision{Outcome: OutcomeDeny, Reason: "unknown_policy_class"}},
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

func TestEngine_Classes(t *testing.T) {
	e, err := New(t.Context())
	require.NoError(t, err)

	assert.Equal(t, []string{ClassAllowlist, ClassAllowlistConsent, ClassConsent}, e.Classes())
	assert.True(t, e.HasClass(ClassAllowlist))
	assert.True(t, e.HasClass(ClassAllowlistConsent))
	assert.True(t, e.HasClass(ClassConsent))
	assert.False(t, e.HasClass("no_such_class"))
	assert.False(t, e.HasClass(""))
}

func TestToDecision_Obligations(t *testing.T) {
	tests := []struct {
		name    string
		value   any
		want    []string
		wantErr bool
	}{
		{"absent", nil, nil, false},
		{"empty list", []any{}, nil, false},
		{"list", []any{"audit.consent_bypassed", "notify.owner"}, []string{"audit.consent_bypassed", "notify.owner"}, false},
		{"not a list", "audit.consent_bypassed", nil, true},
		{"non-string item", []any{"audit.consent_bypassed", 1}, nil, true},
		{"empty string item", []any{""}, nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := map[string]any{"outcome": "allow"}
			if tt.value != nil {
				doc["obligations"] = tt.value
			}
			got, err := toDecision(doc)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, Decision{Outcome: OutcomeAllow, Obligations: tt.want}, got)
		})
	}
}

// TestToDecision_RegoSetObligations checks that obligations a policy builds as
// a Rego set reach the decision, in sorted order.
func TestToDecision_RegoSetObligations(t *testing.T) {
	results, err := rego.New(
		rego.Query("data.example.decision"),
		rego.Module("example.rego", `package example

decision := {"outcome": "allow", "obligations": {"notify.owner", "audit.consent_bypassed"}}`),
	).Eval(t.Context())
	require.NoError(t, err)
	require.Len(t, results, 1)

	got, err := toDecision(results[0].Expressions[0].Value)
	require.NoError(t, err)
	assert.Equal(t, []string{"audit.consent_bypassed", "notify.owner"}, got.Obligations)
}

func TestToDecision_RejectsUnknownOutcome(t *testing.T) {
	_, err := toDecision(map[string]any{"outcome": "maybe"})
	assert.Error(t, err)

	_, err = toDecision("allow")
	assert.Error(t, err)
}
