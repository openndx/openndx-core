// Package engine evaluates field access decisions with OPA, using the Rego
// policies embedded under policies/ and, when a modules directory is
// configured, the custom policy classes in it.
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
	"os"
	"path/filepath"
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

// Option configures New.
type Option func(*options)

type options struct {
	modulesDir string
}

// WithModulesDir also compiles the custom policy classes in dir: every .rego
// file under it, except _test.rego files and hidden files and directories.
// An empty dir loads none.
func WithModulesDir(dir string) Option {
	return func(o *options) { o.modulesDir = dir }
}

// New compiles the embedded policies and any custom policy classes. It fails
// if a module does not compile, if a custom module redefines a built-in class
// or sits in another package under openndx, or if a class has no decision rule.
func New(ctx context.Context, opts ...Option) (*Engine, error) {
	var o options
	for _, opt := range opts {
		opt(&o)
	}

	sources, err := readModules(policyFS, "policies", false)
	if err != nil {
		return nil, fmt.Errorf("read embedded policies: %w", err)
	}
	modules, err := parseModules(sources)
	if err != nil {
		return nil, err
	}
	// hasDecision records, for each class, whether a module defines its
	// decision rule.
	hasDecision := make(map[string]bool)
	for _, module := range modules {
		if class, ok := className(module); ok {
			hasDecision[class] = hasDecision[class] || definesDecision(module)
		}
	}

	if o.modulesDir != "" {
		builtIn := maps.Clone(hasDecision)
		sources, err := readModulesDir(o.modulesDir, false)
		if err != nil {
			return nil, err
		}
		custom, err := parseModules(sources)
		if err != nil {
			return nil, err
		}
		for _, name := range slices.Sorted(maps.Keys(custom)) {
			module := custom[name]
			class, err := customClassName(module, builtIn)
			if err != nil {
				return nil, fmt.Errorf("custom policy %s: %w", name, err)
			}
			if class != "" {
				hasDecision[class] = hasDecision[class] || definesDecision(module)
			}
			modules[name] = module
		}
	}

	classes := make(map[string]struct{}, len(hasDecision))
	for _, class := range slices.Sorted(maps.Keys(hasDecision)) {
		if !hasDecision[class] {
			return nil, fmt.Errorf("policy class %q has no decision rule", class)
		}
		classes[class] = struct{}{}
	}

	regoOpts := []func(*rego.Rego){rego.Query(decisionQuery), rego.StrictBuiltinErrors(true)}
	for _, module := range modules {
		regoOpts = append(regoOpts, rego.ParsedModule(module))
	}
	query, err := rego.New(regoOpts...).PrepareForEval(ctx)
	if err != nil {
		return nil, fmt.Errorf("compile policies: %w", err)
	}
	return &Engine{query: query, classes: classes}, nil
}

// parseModules parses Rego sources, keyed by path.
func parseModules(sources map[string]string) (map[string]*ast.Module, error) {
	modules := make(map[string]*ast.Module, len(sources))
	for _, name := range slices.Sorted(maps.Keys(sources)) {
		module, err := ast.ParseModule(name, sources[name])
		if err != nil {
			return nil, fmt.Errorf("parse policy %s: %w", name, err)
		}
		modules[name] = module
	}
	return modules, nil
}

// customClassName checks the package of a custom module and returns the class
// it defines, or "" for a helper package outside openndx. Everything under
// openndx except a new class is reserved: a custom module could otherwise add
// rules to the router, the shared helpers or a built-in class.
func customClassName(module *ast.Module, builtIn map[string]bool) (string, error) {
	path := module.Package.Path
	if !path.HasPrefix(openndxRef) {
		return "", nil
	}
	class, ok := className(module)
	if !ok || class == "" {
		return "", fmt.Errorf("package %s is reserved: a custom module is a policy class in package openndx.classes.<name>, or a helper in a package outside openndx",
			strings.TrimPrefix(path.String(), "data."))
	}
	if _, ok := builtIn[class]; ok {
		return "", fmt.Errorf("policy class %q is built in and cannot be redefined", class)
	}
	return class, nil
}

// definesDecision reports whether a module has a rule named decision.
func definesDecision(module *ast.Module) bool {
	for _, rule := range module.Rules {
		if ref := rule.Head.Ref(); len(ref) > 0 && ref[0].Value.Compare(ast.Var("decision")) == 0 {
			return true
		}
	}
	return false
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

var (
	openndxRef = ast.MustParseRef("data.openndx")
	classesRef = ast.MustParseRef("data.openndx.classes")
)

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

// readModulesDir returns the Rego sources in a directory on disk, keyed by
// file path, with or without the _test.rego files.
func readModulesDir(dir string, withTests bool) (map[string]string, error) {
	info, err := os.Stat(dir)
	if err != nil {
		return nil, fmt.Errorf("read policy modules directory: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("read policy modules directory: %s is not a directory", dir)
	}
	// Absolute paths name the file in errors wherever the PDP was started, and
	// can't collide with the embedded modules' relative paths.
	if abs, err := filepath.Abs(dir); err == nil {
		dir = abs
	}
	sources, err := readModules(os.DirFS(dir), ".", withTests)
	if err != nil {
		return nil, fmt.Errorf("read policy modules directory %s: %w", dir, err)
	}
	modules := make(map[string]string, len(sources))
	for path, src := range sources {
		modules[filepath.Join(dir, filepath.FromSlash(path))] = src
	}
	return modules, nil
}

// readModules returns the Rego sources under root by path, with or without
// the _test.rego files. Hidden files and directories are skipped: a
// Kubernetes ConfigMap mount keeps a second copy of every file in one.
func readModules(fsys fs.FS, root string, withTests bool) (map[string]string, error) {
	modules := make(map[string]string)
	err := fs.WalkDir(fsys, root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path != root && strings.HasPrefix(d.Name(), ".") {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if d.IsDir() || !strings.HasSuffix(path, ".rego") {
			return nil
		}
		if !withTests && strings.HasSuffix(path, "_test.rego") {
			return nil
		}
		src, err := fs.ReadFile(fsys, path)
		if err != nil {
			return err
		}
		modules[path] = string(src)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return modules, nil
}
