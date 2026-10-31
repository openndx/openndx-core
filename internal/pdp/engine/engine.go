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
	"strings"
	"time"

	"github.com/open-policy-agent/opa/v1/rego"
)

// Built-in policy classes.
const (
	ClassAllowlist        = "allowlist"
	ClassAllowlistConsent = "allowlist_consent"
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

// Field identifies the requested field and the policy class that governs it.
type Field struct {
	SchemaID    string `json:"schemaId"`
	FieldName   string `json:"fieldName"`
	PolicyClass string `json:"policyClass"`
}

// Grant is the requesting application's access to the field.
type Grant struct {
	ExpiresAt time.Time `json:"expiresAt"`
}

// Decision is the policy result for one field.
type Decision struct {
	Outcome Outcome
	Reason  string
}

// Engine holds the compiled policies.
type Engine struct {
	query rego.PreparedEvalQuery
}

// New compiles the embedded policies. It fails if any of them does not compile.
func New(ctx context.Context) (*Engine, error) {
	modules, err := readModules(false)
	if err != nil {
		return nil, err
	}

	opts := []func(*rego.Rego){rego.Query(decisionQuery), rego.StrictBuiltinErrors(true)}
	for name, src := range modules {
		opts = append(opts, rego.Module(name, src))
	}
	query, err := rego.New(opts...).PrepareForEval(ctx)
	if err != nil {
		return nil, fmt.Errorf("compile policies: %w", err)
	}
	return &Engine{query: query}, nil
}

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
	return Decision{Outcome: Outcome(outcome), Reason: reason}, nil
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
