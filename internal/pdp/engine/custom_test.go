package engine

import (
	"maps"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// examplePoliciesDir holds the example custom class the docs point to.
const examplePoliciesDir = "../../../cmd/pdp/examples/policies"

// officeHours is a custom class that reads its parameters and uses the shared
// helpers and a helper package of its own.
const officeHours = `package openndx.classes.office_hours

import data.acme.clock
import data.openndx.lib

decision := lib.deny("not_allowlisted") if {
	not lib.has_grant
} else := lib.deny("outside_office_hours") if {
	not clock.between(input.field.params.from, input.field.params.to)
} else := lib.allow
`

const acmeClock = `package acme.clock

between(from, to) if {
	hour := time.clock(time.parse_rfc3339_ns(input.now))[0]
	hour >= from
	hour < to
}
`

// writeModules writes Rego files, by path relative to a new directory, and
// returns the directory.
func writeModules(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, src := range files {
		path := filepath.Join(dir, filepath.FromSlash(name))
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o750))
		require.NoError(t, os.WriteFile(path, []byte(src), 0o600))
	}
	return dir
}

func TestNew_CustomClass(t *testing.T) {
	dir := writeModules(t, map[string]string{
		"office_hours.rego": officeHours,
		"lib/clock.rego":    acmeClock,
		// Test files are not loaded: this one would not compile.
		"office_hours_test.rego": "package openndx.classes.office_hours_test\n\ntest_x if undefined_function(1)\n",
		// Hidden directories are skipped. A Kubernetes ConfigMap mount keeps
		// a copy of every file in one, and this copy would fail to load.
		"..data/office_hours.rego": "package openndx.pdp\n\nextra := true\n",
		".hidden.rego":             "not rego",
		"README.md":                "not rego",
	})

	e, err := New(t.Context(), WithModulesDir(dir))
	require.NoError(t, err)

	assert.Equal(t, []string{ClassAllowlist, ClassAllowlistConsent, ClassConsent, "office_hours", ClassOpen}, e.Classes())
	assert.True(t, e.HasClass("office_hours"))
	assert.False(t, e.HasClass("office_hours_test"))

	noon := time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC)
	grant := &Grant{ExpiresAt: noon.Add(24 * time.Hour)}
	tests := []struct {
		name  string
		now   time.Time
		grant *Grant
		want  Decision
	}{
		{"inside hours", noon, grant, Decision{Outcome: OutcomeAllow}},
		{"outside hours", noon.Add(8 * time.Hour), grant, Decision{Outcome: OutcomeDeny, Reason: "outside_office_hours"}},
		{"without grant", noon, nil, Decision{Outcome: OutcomeDeny, Reason: "not_allowlisted"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := e.Decide(t.Context(), Input{
				App: App{ID: "app-1"},
				Field: Field{
					SchemaID: "drp", FieldName: "person.fullName", PolicyClass: "office_hours",
					Params: map[string]any{"from": 9, "to": 17},
				},
				Grant: tt.grant,
				Now:   tt.now,
			})
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}

	// The built-in classes still decide their fields.
	got, err := e.Decide(t.Context(), Input{App: App{ID: "app-1"}, Field: Field{PolicyClass: ClassOpen}, Now: noon})
	require.NoError(t, err)
	assert.Equal(t, Decision{Outcome: OutcomeAllow}, got)
}

func TestNew_ModulesDirWithoutModules(t *testing.T) {
	for name, dir := range map[string]string{"not configured": "", "empty directory": t.TempDir()} {
		t.Run(name, func(t *testing.T) {
			e, err := New(t.Context(), WithModulesDir(dir))
			require.NoError(t, err)
			assert.Equal(t, []string{ClassAllowlist, ClassAllowlistConsent, ClassConsent, ClassOpen}, e.Classes())
		})
	}
}

// TestNew_RejectsCustomModules checks that the PDP would refuse to start.
func TestNew_RejectsCustomModules(t *testing.T) {
	const allow = "\n\ndecision := {\"outcome\": \"allow\"}\n"
	tests := []struct {
		name    string
		files   map[string]string
		wantErr string
	}{
		{
			"redefines a built-in class",
			map[string]string{"allowlist.rego": "package openndx.classes.allowlist" + allow},
			`policy class "allowlist" is built in and cannot be redefined`,
		},
		{
			"adds a rule to a built-in class",
			map[string]string{"open.rego": "package openndx.classes.open\n\nextra := true\n"},
			`policy class "open" is built in and cannot be redefined`,
		},
		{
			"redefines the router",
			map[string]string{"pdp.rego": "package openndx.pdp" + allow},
			"package openndx.pdp is reserved",
		},
		{
			"redefines the shared helpers",
			map[string]string{"lib.rego": "package openndx.lib\n\nhas_grant := true\n"},
			"package openndx.lib is reserved",
		},
		{
			"defines classes as rules of the classes package",
			map[string]string{"classes.rego": "package openndx.classes\n\nsneaky := {\"decision\": {\"outcome\": \"allow\"}}\n"},
			"package openndx.classes is reserved",
		},
		{
			"nests a package under a class",
			map[string]string{"nested.rego": "package openndx.classes.window.helpers\n\nx := 1\n"},
			"package openndx.classes.window.helpers is reserved",
		},
		{
			"class without a decision rule",
			map[string]string{"window.rego": "package openndx.classes.window\n\nallowed := true\n"},
			`policy class "window" has no decision rule`,
		},
		{
			"syntax error",
			map[string]string{"window.rego": "package openndx.classes.window\n\ndecision := {\n"},
			"parse policy",
		},
		{
			"compile error",
			map[string]string{"window.rego": "package openndx.classes.window\n\ndecision := no_such_helper(input)\n"},
			"compile policies",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := New(t.Context(), WithModulesDir(writeModules(t, tt.files)))
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}

	t.Run("missing directory", func(t *testing.T) {
		_, err := New(t.Context(), WithModulesDir(filepath.Join(t.TempDir(), "missing")))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "read policy modules directory")
	})

	t.Run("not a directory", func(t *testing.T) {
		file := filepath.Join(t.TempDir(), "window.rego")
		require.NoError(t, os.WriteFile(file, []byte("package openndx.classes.window"+allow), 0o600))
		_, err := New(t.Context(), WithModulesDir(file))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "is not a directory")
	})
}

// TestNew_ClassSplitAcrossFiles checks that one class may span several files,
// as long as one of them defines its decision.
func TestNew_ClassSplitAcrossFiles(t *testing.T) {
	dir := writeModules(t, map[string]string{
		"window.rego":         "package openndx.classes.window\n\ndecision := {\"outcome\": \"allow\"} if open\n",
		"window_helpers.rego": "package openndx.classes.window\n\nopen := true\n",
	})

	e, err := New(t.Context(), WithModulesDir(dir))
	require.NoError(t, err)

	got, err := e.Decide(t.Context(), Input{Field: Field{PolicyClass: "window"}, Now: time.Now()})
	require.NoError(t, err)
	assert.Equal(t, Decision{Outcome: OutcomeAllow}, got)
}

// TestCustomClass_UndefinedDecisionDenies checks that a custom class with no
// answer for an input fails closed.
func TestCustomClass_UndefinedDecisionDenies(t *testing.T) {
	dir := writeModules(t, map[string]string{
		"window.rego": "package openndx.classes.window\n\ndecision := {\"outcome\": \"allow\"} if input.field.params.open == true\n",
	})
	e, err := New(t.Context(), WithModulesDir(dir))
	require.NoError(t, err)

	got, err := e.Decide(t.Context(), Input{Field: Field{PolicyClass: "window"}, Now: time.Now()})
	require.NoError(t, err)
	assert.Equal(t, Decision{Outcome: OutcomeDeny, Reason: "unknown_policy_class"}, got)
}

// TestExamplePolicies loads the example custom class the way the PDP does and
// runs its Rego tests, so the documented example keeps working.
func TestExamplePolicies(t *testing.T) {
	e, err := New(t.Context(), WithModulesDir(examplePoliciesDir))
	require.NoError(t, err)
	assert.True(t, e.HasClass("access_window"))

	now := time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC)
	field := Field{
		SchemaID: "census", FieldName: "household.size", PolicyClass: "access_window",
		Params: map[string]any{"notBefore": "2026-10-01T00:00:00Z", "notAfter": "2026-11-30T23:59:59Z"},
	}
	grant := &Grant{ExpiresAt: now.AddDate(1, 0, 0)}

	got, err := e.Decide(t.Context(), Input{App: App{ID: "app-1"}, Field: field, Grant: grant, Now: now})
	require.NoError(t, err)
	assert.Equal(t, Decision{Outcome: OutcomeAllow}, got)

	got, err = e.Decide(t.Context(), Input{App: App{ID: "app-1"}, Field: field, Grant: grant, Now: now.AddDate(0, 2, 0)})
	require.NoError(t, err)
	assert.Equal(t, Decision{Outcome: OutcomeDeny, Reason: "outside_access_window"}, got)

	// A parameter that is not a timestamp is a built-in error, which the PDP
	// treats as a failed evaluation: the field is denied.
	field.Params = map[string]any{"notBefore": "tomorrow", "notAfter": "2026-11-30T23:59:59Z"}
	got, err = e.Decide(t.Context(), Input{App: App{ID: "app-1"}, Field: field, Grant: grant, Now: now})
	require.Error(t, err)
	assert.Equal(t, Decision{Outcome: OutcomeDeny, Reason: "evaluation_error"}, got)

	// The example's own tests, with the built-in modules they import.
	sources, err := readModules(policyFS, "policies", false)
	require.NoError(t, err)
	example, err := readModulesDir(examplePoliciesDir, true)
	require.NoError(t, err)
	maps.Copy(sources, example)
	assert.Positive(t, runRegoTests(t, sources), "no example Rego tests found")
}
