// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Lanka Software Foundation

package federator

import (
	"bytes"
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/openndx/openndx-core/internal/oe/federator/operation"
	"github.com/openndx/openndx-core/internal/oe/pkg/graphql"
	"github.com/openndx/openndx-core/internal/oe/policy"
	"github.com/vektah/gqlparser/v2/ast"
	"github.com/vektah/gqlparser/v2/formatter"
	"github.com/vektah/gqlparser/v2/parser"
)

// plan is the set of provider operations that answer a normalized consumer
// operation, and how each consumer field maps back to them.
type plan struct {
	// fetches run in parallel: every fetch depends only on the consumer's arguments.
	fetches []*fetch
	// sources maps each consumer field that reads provider data directly (a leaf,
	// or a list whose items are read relative to it) to the fetch that serves it.
	sources map[*operation.Field]*fetch
}

// fetch is one provider operation.
type fetch struct {
	schemaID string
	// query is the printed provider operation. It declares one variable per
	// mapped argument, so its text doesn't depend on the argument values.
	query string
	// variables are the values of the operation's variables.
	variables map[string]any
	// fields are the provider field paths the operation reads, in query order.
	fields []string
	// consumerFields are the consumer fields served directly by this fetch, used
	// to place provider errors in the consumer response.
	consumerFields []*operation.Field

	key  string
	root *selectionNode
	args []providerArgument
}

// providerArgument is a mapped argument attached to the provider field at path,
// sent as a variable of type typ.
type providerArgument struct {
	path  string
	name  string
	typ   *ast.Type
	value any
}

// elementScope is the provider list whose items the fields inside a consumer list
// field are read from.
type elementScope struct {
	fetch *fetch
	// declared is the list's providerField: its items' fields are declared
	// relative to it, or by their last path segment.
	declared string
	// requested is the provider path the list is requested at.
	requested string
}

// buildPlan plans the provider operations for op. Each argument mapping is
// attached only at its targetArgPath, with the value bound from the consumer
// field that carries it or one of its ancestors. targetTypes holds the parsed
// targetArgType of the mappings that set one.
func buildPlan(op *operation.Operation, mappings []*graphql.ArgMapping, targetTypes map[*graphql.ArgMapping]*ast.Type) (*plan, error) {
	p := &planner{
		targetTypes: targetTypes,
		bindings:    op.BindArguments(mappings),
		plan:        &plan{sources: map[*operation.Field]*fetch{}},
		byKey:       map[string]*fetch{},
	}
	if err := p.fields(op.Fields, nil); err != nil {
		return nil, err
	}
	for _, f := range p.plan.fetches {
		f.query, f.variables = f.print()
	}
	return p.plan, nil
}

// targetArgTypes parses the targetArgType of each mapping that sets one.
func targetArgTypes(mappings []*graphql.ArgMapping) (map[*graphql.ArgMapping]*ast.Type, error) {
	types := map[*graphql.ArgMapping]*ast.Type{}
	for i, mapping := range mappings {
		if mapping == nil || mapping.TargetArgType == "" {
			continue
		}
		typ, err := parseType(mapping.TargetArgType)
		if err != nil {
			return nil, fmt.Errorf("argMappings[%d].targetArgType: %w", i, err)
		}
		types[mapping] = typ
	}
	return types, nil
}

// parseType parses a GraphQL type reference such as "[String!]!".
func parseType(s string) (*ast.Type, error) {
	doc, err := parser.ParseQuery(&ast.Source{Input: "query($v: " + s + ") { __typename }"})
	if err == nil && len(doc.Operations) == 1 && len(doc.Operations[0].VariableDefinitions) == 1 {
		// The round trip rejects anything after the type, such as a default value.
		if typ := doc.Operations[0].VariableDefinitions[0].Type; typ.String() == strings.Join(strings.Fields(s), "") {
			return typ, nil
		}
	}
	return nil, fmt.Errorf("%q is not a GraphQL type", s)
}

// requiredFields returns the provider fields the PDP must authorize: every leaf
// the plan reads, once.
func (p *plan) requiredFields() []policy.RequiredField {
	seen := map[policy.RequiredField]bool{}
	var fields []policy.RequiredField
	for _, f := range p.fetches {
		for _, path := range f.fields {
			field := policy.RequiredField{SchemaID: f.schemaID, FieldName: path}
			if !seen[field] {
				seen[field] = true
				fields = append(fields, field)
			}
		}
	}
	return fields
}

type planner struct {
	targetTypes map[*graphql.ArgMapping]*ast.Type
	bindings    []operation.ArgumentBinding
	plan        *plan
	byKey       map[string]*fetch
}

// fields plans fields. scope is nil outside a provider list.
func (p *planner) fields(fields []*operation.Field, scope *elementScope) error {
	for _, field := range fields {
		if err := p.field(field, scope); err != nil {
			return err
		}
	}
	return nil
}

func (p *planner) field(field *operation.Field, scope *elementScope) error {
	if field.Name == "__typename" {
		return nil
	}
	if strings.HasPrefix(field.Name, "__") {
		return fmt.Errorf("introspection field %s is not supported", field.Name)
	}

	info := field.SourceInfo
	isList := field.Definition.Type.Elem != nil
	switch {
	case scope != nil && info != nil && (isList || len(field.Selections) == 0):
		requested := scope.requested + "." + relativePath(info.ProviderField, scope.declared)
		if isList && len(field.Selections) > 0 {
			return p.fields(field.Selections, &elementScope{fetch: scope.fetch, declared: info.ProviderField, requested: requested})
		}
		scope.fetch.addField(requested)
		return nil

	case scope == nil && info != nil && (isList || len(field.Selections) == 0):
		f, err := p.fetchFor(field)
		if err != nil {
			return err
		}
		p.plan.sources[field] = f
		f.consumerFields = append(f.consumerFields, field)
		if isList && len(field.Selections) > 0 {
			return p.fields(field.Selections, &elementScope{fetch: f, declared: info.ProviderField, requested: info.ProviderField})
		}
		f.addField(info.ProviderField)
		return nil

	case isList && len(field.Selections) > 0:
		// Without @sourceInfo there is no provider list to read the items from.
		return fmt.Errorf("list field %s has no @sourceInfo, so its items can't be read from a provider",
			strings.Join(field.SchemaPath, "."))

	default:
		// An object field without @sourceInfo, or one whose @sourceInfo only
		// documents its provider object: its fields carry their own sources.
		return p.fields(field.Selections, scope)
	}
}

// fetchFor returns the fetch that serves field: the one for its schema with the
// same mapped arguments, created on first use.
func (p *planner) fetchFor(field *operation.Field) (*fetch, error) {
	info := field.SourceInfo
	args, err := p.arguments(field)
	if err != nil {
		return nil, err
	}

	var key strings.Builder
	key.WriteString(info.SchemaID)
	for _, arg := range args {
		value, err := json.Marshal(arg.value)
		if err != nil {
			return nil, fmt.Errorf("argument %s of %s: %w", arg.name, arg.path, err)
		}
		key.WriteString("\x00" + arg.path + "\x00" + arg.name + "\x00" + arg.typ.String() + "\x00" + string(value))
	}
	if f, ok := p.byKey[key.String()]; ok {
		return f, nil
	}

	f := &fetch{
		schemaID: info.SchemaID,
		key:      key.String(),
		root:     &selectionNode{},
		args:     args,
	}
	p.byKey[f.key] = f
	p.plan.fetches = append(p.plan.fetches, f)
	return f, nil
}

// arguments returns the mapped arguments for the provider field that serves
// field: the bindings for its schema, carried by field or an
// ancestor, whose targetArgPath leads to the provider field. The first binding
// for each target path and argument wins; the result is sorted.
func (p *planner) arguments(field *operation.Field) ([]providerArgument, error) {
	info := field.SourceInfo
	seen := map[[2]string]bool{}
	var args []providerArgument
	for _, binding := range p.bindings {
		mapping := binding.Mapping
		if mapping.SchemaID != info.SchemaID ||
			!isPathPrefix(info.ProviderField, mapping.TargetArgPath) ||
			!hasPrefix(field.Path, binding.Field.Path) {
			continue
		}
		id := [2]string{mapping.TargetArgPath, mapping.TargetArgName}
		if seen[id] {
			continue
		}
		seen[id] = true

		typ := p.targetTypes[mapping]
		if typ == nil {
			var err error
			if typ, err = consumerArgType(binding); err != nil {
				return nil, err
			}
		}
		args = append(args, providerArgument{path: mapping.TargetArgPath, name: mapping.TargetArgName, typ: typ, value: binding.Value})
	}
	sort.Slice(args, func(i, j int) bool {
		if args[i].path != args[j].path {
			return args[i].path < args[j].path
		}
		return args[i].name < args[j].name
	})
	return args, nil
}

// consumerArgType returns the type to declare for a mapping without a
// targetArgType: the consumer argument's type, made non-null when the value
// isn't null so that it also fits a non-null provider argument.
func consumerArgType(binding operation.ArgumentBinding) (*ast.Type, error) {
	segments := strings.Split(binding.Mapping.SourceArgPath, "-")
	argDef := binding.Field.Definition.Arguments.ForName(segments[len(segments)-1])
	if argDef == nil {
		return nil, fmt.Errorf("argument %s of %s has no definition", segments[len(segments)-1], strings.Join(binding.Field.SchemaPath, "."))
	}
	typ := argDef.Type
	if binding.Value != nil && !typ.NonNull {
		nonNull := *typ
		nonNull.NonNull = true
		typ = &nonNull
	}
	return typ, nil
}

// addField adds the provider field at path to the operation, once.
func (f *fetch) addField(path string) {
	if slices.Contains(f.fields, path) {
		return
	}
	f.fields = append(f.fields, path)
	node := f.root
	for _, name := range strings.Split(path, ".") {
		node = node.child(name)
	}
}

// print returns the provider operation, named "Query" + the schema ID, with
// characters that aren't valid in a GraphQL name replaced by "_", and the values
// of its variables. Each mapped argument is a variable named after the
// argument, with a numeric suffix when the name is already taken.
func (f *fetch) print() (string, map[string]any) {
	var definitions ast.VariableDefinitionList
	var variables map[string]any
	for _, arg := range f.args {
		name := arg.name
		for i := 2; definitions.ForName(name) != nil; i++ {
			name = fmt.Sprintf("%s_%d", arg.name, i)
		}
		if variables == nil {
			variables = map[string]any{}
		}
		variables[name] = arg.value
		definitions = append(definitions, &ast.VariableDefinition{Variable: name, Type: arg.typ})

		node := f.root
		for segment := range strings.SplitSeq(arg.path, ".") {
			node = node.child(segment)
		}
		node.arguments = append(node.arguments, &ast.Argument{Name: arg.name, Value: &ast.Value{Kind: ast.Variable, Raw: name}})
	}

	doc := &ast.QueryDocument{Operations: ast.OperationList{{
		Operation:           ast.Query,
		Name:                "Query" + operationName(f.schemaID),
		VariableDefinitions: definitions,
		SelectionSet:        f.root.selectionSet(),
	}}}
	var buf bytes.Buffer
	formatter.NewFormatter(&buf, formatter.WithIndent("  ")).FormatQueryDocument(doc)
	return strings.TrimSpace(buf.String()), variables
}

// operationName returns id with every character that isn't valid in a GraphQL
// name replaced by "_".
func operationName(id string) string {
	return strings.Map(func(r rune) rune {
		if r == '_' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' {
			return r
		}
		return '_'
	}, id)
}

// selectionNode is a provider field in a fetch's selection tree.
type selectionNode struct {
	name      string
	arguments ast.ArgumentList
	children  []*selectionNode
}

func (n *selectionNode) child(name string) *selectionNode {
	for _, c := range n.children {
		if c.name == name {
			return c
		}
	}
	c := &selectionNode{name: name}
	n.children = append(n.children, c)
	return c
}

func (n *selectionNode) selectionSet() ast.SelectionSet {
	if len(n.children) == 0 {
		return nil
	}
	set := make(ast.SelectionSet, 0, len(n.children))
	for _, c := range n.children {
		set = append(set, &ast.Field{Name: c.name, Arguments: c.arguments, SelectionSet: c.selectionSet()})
	}
	return set
}

// relativePath returns the path of a list item's field, given its declared
// providerField and the list's: the part after the list's path, or, for fields
// declared outside it, the last segment.
func relativePath(path, list string) string {
	if rest, ok := strings.CutPrefix(path, list+"."); ok {
		return rest
	}
	return path[strings.LastIndex(path, ".")+1:]
}

// isPathPrefix reports whether prefix is path or a dot-separated ancestor of it.
func isPathPrefix(path, prefix string) bool {
	return path == prefix || strings.HasPrefix(path, prefix+".")
}

// hasPrefix reports whether prefix is the start of path.
func hasPrefix(path, prefix []string) bool {
	return len(prefix) <= len(path) && slices.Equal(path[:len(prefix)], prefix)
}
