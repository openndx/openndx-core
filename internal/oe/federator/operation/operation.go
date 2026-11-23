// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Lanka Software Foundation

// Package operation normalizes a validated consumer query into the fields the
// federator plans against: fragments merged, @skip and @include applied, fields
// grouped by response key, and each field bound to its schema definition and
// @sourceInfo.
package operation

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/openndx/openndx-core/internal/oe/federator/schema"
	"github.com/openndx/openndx-core/internal/oe/pkg/graphql"
	"github.com/vektah/gqlparser/v2/ast"
	"github.com/vektah/gqlparser/v2/validator"
)

var (
	// ErrUnsupportedOperation is returned for mutations and subscriptions.
	ErrUnsupportedOperation = errors.New("only query operations are supported")
	// ErrMissingIdentifier is returned when no field has a value for the data-owner ID argument.
	ErrMissingIdentifier = errors.New("data owner ID argument is missing or empty")
	// ErrAmbiguousIdentifier is returned when fields have different values for the data-owner ID argument.
	ErrAmbiguousIdentifier = errors.New("conflicting data owner ID values in query")
)

// Operation is a normalized consumer query.
type Operation struct {
	// Fields are the root fields, in query order.
	Fields []*Field
	// Variables are the request's variables, coerced, with defaults applied.
	Variables map[string]any
}

// Field is one response key of a normalized selection set.
type Field struct {
	// ResponseKey is the field's alias, or its name.
	ResponseKey string
	Name        string
	// Path is the response keys from the root to this field.
	Path []string
	// SchemaPath is the field names from the root to this field, the path that
	// an argument mapping's sourceArgPath names.
	SchemaPath []string
	// Arguments are the coerced argument values, with schema defaults applied.
	// An argument the query leaves out and that has no default is absent.
	Arguments  map[string]any
	Definition *ast.FieldDefinition
	// SourceInfo is the field's @sourceInfo, or nil if it has none.
	SourceInfo *schema.SourceInfo
	// Selections are the field's normalized sub-fields, or nil for a leaf.
	Selections []*Field
}

// ArgumentBinding is an argument mapping bound to one occurrence of its
// consumer argument.
type ArgumentBinding struct {
	Mapping *graphql.ArgMapping
	// Field is the consumer field that carries the argument.
	Field *Field
	Value any
}

// Build normalizes the operation operationName in doc, which must already be
// validated against compiled.Schema. An empty operationName selects the only
// operation. Variable errors are returned as *gqlerror.Error.
func Build(compiled *schema.CompiledSchema, doc *ast.QueryDocument, operationName string, variables map[string]any) (*Operation, error) {
	op := doc.Operations.ForName(operationName)
	if op == nil {
		if operationName == "" {
			return nil, errors.New("operationName is required when the query has more than one operation")
		}
		return nil, fmt.Errorf("operation %q not found", operationName)
	}
	if op.Operation != ast.Query {
		return nil, ErrUnsupportedOperation
	}
	if compiled.Schema.Query == nil {
		return nil, errors.New("schema has no Query type")
	}

	vars, err := validator.VariableValues(compiled.Schema, op, variables)
	if err != nil {
		return nil, err
	}

	b := builder{compiled: compiled, fragments: doc.Fragments, vars: vars}
	fields, err := b.fields(compiled.Schema.Query, []ast.SelectionSet{op.SelectionSet}, nil, nil)
	if err != nil {
		return nil, err
	}
	return &Operation{Fields: fields, Variables: vars}, nil
}

// BindArguments binds each mapping to every occurrence of its consumer
// argument. A sourceArgPath such as "personInfo-nic" names the field path
// (personInfo) and the argument (nic); an argument with the same name on
// another field doesn't match. Bindings are in mapping order, then query order.
func (op *Operation) BindArguments(mappings []*graphql.ArgMapping) []ArgumentBinding {
	var bindings []ArgumentBinding
	for _, mapping := range mappings {
		if mapping == nil {
			continue
		}
		segments := strings.Split(mapping.SourceArgPath, "-")
		fieldPath, argName := segments[:len(segments)-1], segments[len(segments)-1]
		op.walk(func(field *Field) {
			if !slices.Equal(field.SchemaPath, fieldPath) {
				return
			}
			if value, ok := field.Arguments[argName]; ok {
				bindings = append(bindings, ArgumentBinding{Mapping: mapping, Field: field, Value: value})
			}
		})
	}
	return bindings
}

// DataOwnerID returns the value of the argument argName, read from every field
// in the operation, aliases included. Only non-empty strings count. It returns
// ErrMissingIdentifier when there is no value and ErrAmbiguousIdentifier when
// the values differ.
func (op *Operation) DataOwnerID(argName string) (string, error) {
	var id string
	var ambiguous bool
	op.walk(func(field *Field) {
		value, ok := field.Arguments[argName].(string)
		if !ok || value == "" {
			return
		}
		if id == "" {
			id = value
		} else if value != id {
			ambiguous = true
		}
	})
	switch {
	case ambiguous:
		return "", ErrAmbiguousIdentifier
	case id == "":
		return "", ErrMissingIdentifier
	}
	return id, nil
}

// walk calls visit for every field, depth first in query order.
func (op *Operation) walk(visit func(*Field)) {
	var walk func([]*Field)
	walk = func(fields []*Field) {
		for _, field := range fields {
			visit(field)
			walk(field.Selections)
		}
	}
	walk(op.Fields)
}

type builder struct {
	compiled  *schema.CompiledSchema
	fragments ast.FragmentDefinitionList
	vars      map[string]any
}

// fields normalizes selectionSets, which all select on objectType. More than one
// selection set is given when fields with the same response key are merged.
func (b *builder) fields(objectType *ast.Definition, selectionSets []ast.SelectionSet, path, schemaPath []string) ([]*Field, error) {
	groups := &fieldGroups{byKey: map[string][]*ast.Field{}}
	for _, selectionSet := range selectionSets {
		if err := b.collectFields(objectType, selectionSet, map[string]bool{}, groups); err != nil {
			return nil, err
		}
	}

	fields := make([]*Field, 0, len(groups.keys))
	for _, key := range groups.keys {
		field, err := b.field(objectType, key, groups.byKey[key], path, schemaPath)
		if err != nil {
			return nil, err
		}
		fields = append(fields, field)
	}
	return fields, nil
}

// field normalizes the fields grouped under responseKey. Validation guarantees
// they have the same name and arguments, so their sub-selections are merged.
func (b *builder) field(objectType *ast.Definition, responseKey string, group []*ast.Field, path, schemaPath []string) (*Field, error) {
	first := group[0]
	definition := first.Definition
	if definition == nil {
		return nil, fmt.Errorf("field %s.%s has no definition; the query must be validated first", objectType.Name, first.Name)
	}

	args, err := b.arguments(definition, first.Arguments)
	if err != nil {
		return nil, fmt.Errorf("arguments of %s.%s: %w", objectType.Name, first.Name, err)
	}

	field := &Field{
		ResponseKey: responseKey,
		Name:        first.Name,
		Path:        appendPath(path, responseKey),
		SchemaPath:  appendPath(schemaPath, first.Name),
		Arguments:   args,
		Definition:  definition,
	}
	if info, ok := b.compiled.SourceInfo(objectType.Name, first.Name); ok {
		field.SourceInfo = &info
	}

	var selectionSets []ast.SelectionSet
	for _, f := range group {
		if len(f.SelectionSet) > 0 {
			selectionSets = append(selectionSets, f.SelectionSet)
		}
	}
	if len(selectionSets) == 0 {
		return field, nil
	}

	fieldType := b.compiled.Schema.Types[definition.Type.Name()]
	if fieldType == nil || fieldType.Kind != ast.Object {
		// The OE can't tell which concrete type a provider returned, so it has
		// no object type to collect the sub-fields against.
		return nil, fmt.Errorf("field %s.%s has type %s: fields of interface or union type are not supported",
			objectType.Name, first.Name, definition.Type.Name())
	}
	field.Selections, err = b.fields(fieldType, selectionSets, field.Path, field.SchemaPath)
	if err != nil {
		return nil, err
	}
	return field, nil
}

// collectFields is the spec's CollectFields: it adds the fields selectionSet
// selects on objectType to groups, following fragments and applying @skip and
// @include.
func (b *builder) collectFields(objectType *ast.Definition, selectionSet ast.SelectionSet, visitedFragments map[string]bool, groups *fieldGroups) error {
	for _, selection := range selectionSet {
		var directives ast.DirectiveList
		switch s := selection.(type) {
		case *ast.Field:
			directives = s.Directives
		case *ast.FragmentSpread:
			directives = s.Directives
		case *ast.InlineFragment:
			directives = s.Directives
		}
		include, err := b.included(directives)
		if err != nil {
			return err
		}
		if !include {
			continue
		}

		switch s := selection.(type) {
		case *ast.Field:
			key := s.Alias
			if key == "" {
				key = s.Name
			}
			groups.add(key, s)
		case *ast.FragmentSpread:
			if visitedFragments[s.Name] {
				continue
			}
			visitedFragments[s.Name] = true
			fragment := b.fragments.ForName(s.Name)
			if fragment == nil {
				return fmt.Errorf("fragment %s is not defined", s.Name)
			}
			if !b.fragmentApplies(objectType, fragment.TypeCondition) {
				continue
			}
			if err := b.collectFields(objectType, fragment.SelectionSet, visitedFragments, groups); err != nil {
				return err
			}
		case *ast.InlineFragment:
			if s.TypeCondition != "" && !b.fragmentApplies(objectType, s.TypeCondition) {
				continue
			}
			if err := b.collectFields(objectType, s.SelectionSet, visitedFragments, groups); err != nil {
				return err
			}
		}
	}
	return nil
}

// included applies @skip and @include.
func (b *builder) included(directives ast.DirectiveList) (bool, error) {
	if skip := directives.ForName("skip"); skip != nil {
		value, err := b.condition(skip)
		if err != nil || value {
			return false, err
		}
	}
	if include := directives.ForName("include"); include != nil {
		return b.condition(include)
	}
	return true, nil
}

// condition returns the value of a @skip or @include directive's if argument.
func (b *builder) condition(directive *ast.Directive) (bool, error) {
	arg := directive.Arguments.ForName("if")
	if arg == nil {
		return false, fmt.Errorf("@%s requires the if argument", directive.Name)
	}
	value, err := arg.Value.Value(b.vars)
	if err != nil {
		return false, fmt.Errorf("@%s(if:): %w", directive.Name, err)
	}
	condition, ok := value.(bool)
	if !ok {
		return false, fmt.Errorf("@%s(if:) must be a Boolean, got %T", directive.Name, value)
	}
	return condition, nil
}

// fragmentApplies is the spec's DoesFragmentTypeApply: typeCondition is
// objectType itself, an interface it implements, or a union that contains it.
func (b *builder) fragmentApplies(objectType *ast.Definition, typeCondition string) bool {
	return slices.ContainsFunc(b.compiled.Schema.PossibleTypes[typeCondition], func(def *ast.Definition) bool {
		return def.Name == objectType.Name
	})
}

// arguments is the spec's CoerceArgumentValues for a validated query: provided
// values with variables substituted, and schema defaults for arguments that are
// left out or set to a variable the request doesn't provide.
func (b *builder) arguments(definition *ast.FieldDefinition, provided ast.ArgumentList) (map[string]any, error) {
	args := map[string]any{}
	for _, argDef := range definition.Arguments {
		if arg := provided.ForName(argDef.Name); arg != nil && b.hasValue(arg.Value) {
			value, err := arg.Value.Value(b.vars)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", argDef.Name, err)
			}
			args[argDef.Name] = value
			continue
		}
		if argDef.DefaultValue != nil {
			value, err := argDef.DefaultValue.Value(nil)
			if err != nil {
				return nil, fmt.Errorf("default value of %s: %w", argDef.Name, err)
			}
			args[argDef.Name] = value
		}
	}
	return args, nil
}

// hasValue reports whether value is a literal or a variable the request
// provides (after variable defaults are applied).
func (b *builder) hasValue(value *ast.Value) bool {
	if value.Kind != ast.Variable {
		return true
	}
	_, ok := b.vars[value.Raw]
	return ok
}

// fieldGroups groups fields by response key, keeping the order in which keys
// are first seen.
type fieldGroups struct {
	keys  []string
	byKey map[string][]*ast.Field
}

func (g *fieldGroups) add(key string, field *ast.Field) {
	if _, ok := g.byKey[key]; !ok {
		g.keys = append(g.keys, key)
	}
	g.byKey[key] = append(g.byKey[key], field)
}

// appendPath returns a new slice, so sibling fields don't share a backing array.
func appendPath(path []string, segment string) []string {
	return append(slices.Clip(path), segment)
}
