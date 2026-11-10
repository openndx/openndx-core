// Package engine evaluates field access decisions with OPA, using the Rego
// policies embedded under policies/.
//
// The router package openndx.pdp sends each field to the policy class named in
// its metadata (openndx.classes.<name>). Every class returns one of a fixed set
// of outcomes; anything else, an unknown class or an evaluation error denies
// the field.
package engine

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/open-policy-agent/opa/v1/ast"
	"github.com/open-policy-agent/opa/v1/rego"
)

// Built-in policy classes.
const (
	ClassAllowlist        = "allowlist"
	ClassAllowlistConsent = "allowlist_consent"
	ClassConsent          = "consent"
	ClassOpen             = "open"
)

// Outcome is the decision for one field.
type Outcome string

const (
	OutcomeAllow           Outcome = "allow"
	OutcomeDeny            Outcome = "deny"
	OutcomeExpired         Outcome = "expired"
	OutcomeConsentRequired Outcome = "consent_required"
)

const decisionQuery = "data.openndx.pdp.decision"

//go:embed policies
var policyFS embed.FS

// Input is the document a policy class evaluates for one requested field.
type Input struct {
	App   App       `json:"app"`
	Field Field     `json:"field"`
	Grant *Grant    `json:"grant,omitempty"`
	Now   time.Time `json:"now"`
}

// App identifies the requesting application.
type App struct {
	ID string `json:"id"`
}

// Field identifies the requested field, the policy class that governs it and
// the class's parameters for this field.
type Field struct {
	SchemaID    string         `json:"schemaId"`
	FieldName   string         `json:"fieldName"`
	PolicyClass string         `json:"policyClass"`
	Params      map[string]any `json:"params,omitempty"`
}

// Grant is the requesting application's access to the field.
type Grant struct {
	ExpiresAt time.Time `json:"expiresAt"`
	// ConsentBypass marks a grant that lets the application skip the owner's
	// consent while it is unexpired.
	ConsentBypass bool `json:"consentBypass"`
}

// Decision is the policy result for one field. Obligations name extra actions
// the policy asks the Orchestration Engine to take, such as recording that
// consent was bypassed.
type Decision struct {
	Outcome     Outcome
	Reason      string
	Obligations []string
}

// Engine holds the compiled policies.
type Engine struct {
	query   rego.PreparedEvalQuery
	classes map[string]struct{}
}

// New compiles the embedded policies. It fails if any of them does not compile.
func New(ctx context.Context) (*Engine, error) {
	modules, err := readModules(false)
	if err != nil {
		return nil, err
	}

	classes := make(map[string]struct{})
	opts := []func(*rego.Rego){rego.Query(decisionQuery), rego.StrictBuiltinErrors(true)}
	for name, src := range modules {
		module, err := ast.ParseModule(name, src)
		if err != nil {
			return nil, fmt.Errorf("parse policy %s: %w", name, err)
		}
		if class, ok := className(module); ok {
			classes[class] = struct{}{}
		}
		opts = append(opts, rego.ParsedModule(module))
	}
	query, err := rego.New(opts...).PrepareForEval(ctx)
	if err != nil {
		return nil, fmt.Errorf("compile policies: %w", err)
	}
	return &Engine{query: query, classes: classes}, nil
}

// HasClass reports whether a policy class with this name is loaded.
func (e *Engine) HasClass(name string) bool {
	_, ok := e.classes[name]
	return ok
}

// Classes returns the names of the loaded policy classes, sorted.
func (e *Engine) Classes() []string {
	return slices.Sorted(maps.Keys(e.classes))
}

// className returns the class name of a module in package
// openndx.classes.<name>. Other modules, such as the router, the shared
// helpers or a nested package, are not classes.
func className(module *ast.Module) (string, bool) {
	path := module.Package.Path
	if len(path) != 4 || !path[:3].Equal(classesRef) {
		return "", false
	}
	name, ok := path[3].Value.(ast.String)
	return string(name), ok
}

var classesRef = ast.MustParseRef("data.openndx.classes")

// Decide evaluates the policy for one field. It always returns a usable
// decision: when evaluation fails the decision is a deny and the error says why.
func (e *Engine) Decide(ctx context.Context, in Input) (Decision, error) {
	results, err := e.query.Eval(ctx, rego.EvalInput(in))
	if err != nil {
		return deny("evaluation_error"), fmt.Errorf("evaluate policy: %w", err)
	}
	if len(results) == 0 || len(results[0].Expressions) == 0 {
		return deny("no_decision"), errors.New("policy returned no decision")
	}
	decision, err := toDecision(results[0].Expressions[0].Value)
	if err != nil {
		return deny("invalid_decision"), err
	}
	return decision, nil
}

func toDecision(value any) (Decision, error) {
	doc, ok := value.(map[string]any)
	if !ok {
		return Decision{}, fmt.Errorf("policy decision is %T, not an object", value)
	}
	outcome, _ := doc["outcome"].(string)
	switch Outcome(outcome) {
	case OutcomeAllow, OutcomeDeny, OutcomeExpired, OutcomeConsentRequired:
	default:
		return Decision{}, fmt.Errorf("policy returned unknown outcome %q", outcome)
	}
	reason, _ := doc["reason"].(string)
	obligations, err := toObligations(doc["obligations"])
	if err != nil {
		return Decision{}, err
	}
	return Decision{Outcome: Outcome(outcome), Reason: reason, Obligations: obligations}, nil
}

// toObligations reads the optional obligations of a decision: a list (or Rego
// set) of non-empty strings. A malformed value fails the decision, because the
// Orchestration Engine could not honor obligations it never receives.
func toObligations(value any) ([]string, error) {
	if value == nil {
		return nil, nil
	}
	items, ok := value.([]any)
	if !ok {
		return nil, fmt.Errorf("policy obligations are %T, not a list", value)
	}
	obligations := make([]string, 0, len(items))
	for _, item := range items {
		obligation, ok := item.(string)
		if !ok || obligation == "" {
			return nil, fmt.Errorf("policy obligation %v is not a non-empty string", item)
		}
		obligations = append(obligations, obligation)
	}
	if len(obligations) == 0 {
		return nil, nil
	}
	return obligations, nil
}

func deny(reason string) Decision {
	return Decision{Outcome: OutcomeDeny, Reason: reason}
}

// readModules returns the embedded Rego sources by path, with or without the
// _test.rego files.
func readModules(withTests bool) (map[string]string, error) {
	modules := make(map[string]string)
	err := fs.WalkDir(policyFS, "policies", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".rego") {
			return err
		}
		if !withTests && strings.HasSuffix(path, "_test.rego") {
			return nil
		}
		src, err := policyFS.ReadFile(path)
		if err != nil {
			return err
		}
		modules[path] = string(src)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("read embedded policies: %w", err)
	}
	return modules, nil
}
