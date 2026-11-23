// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Lanka Software Foundation

package schema

import (
	"fmt"
	"sort"

	"github.com/vektah/gqlparser/v2"
	"github.com/vektah/gqlparser/v2/ast"
	"github.com/vektah/gqlparser/v2/parser"
)

// sourceInfoDirective is the directive that maps a consumer schema field to the
// provider field that serves it.
const sourceInfoDirective = "sourceInfo"

// sourceInfoDeclaration is added to schemas that use @sourceInfo without declaring it.
const sourceInfoDeclaration = `directive @sourceInfo(schemaId: String!, providerField: String!) on FIELD_DEFINITION`

// sourceInfoArguments are the arguments a @sourceInfo declaration must have,
// each of base type String.
var sourceInfoArguments = []string{"providerField", "schemaId"}

// SourceInfo is a field's @sourceInfo: the provider field that serves it.
type SourceInfo struct {
	SchemaID      string
	ProviderField string
}

// CompiledSchema is a consumer schema validated against the GraphQL spec.
type CompiledSchema struct {
	// Schema is the spec AST that consumer queries are validated against.
	Schema *ast.Schema

	sourceInfo map[fieldKey]SourceInfo
}

type fieldKey struct{ typeName, fieldName string }

// SourceInfo returns the @sourceInfo of typeName.fieldName, if the field has one.
func (c *CompiledSchema) SourceInfo(typeName, fieldName string) (SourceInfo, bool) {
	info, ok := c.sourceInfo[fieldKey{typeName, fieldName}]
	return info, ok
}

// Compile validates sdl against the GraphQL spec and indexes its @sourceInfo
// directives. A schema that uses @sourceInfo without declaring it gets the
// standard declaration; a schema that declares it differently is rejected.
func Compile(sdl string) (*CompiledSchema, error) {
	doc, err := parser.ParseSchema(&ast.Source{Name: "schema", Input: sdl})
	if err != nil {
		return nil, err
	}

	sources := []*ast.Source{{Name: "schema", Input: sdl}}
	if declaration := doc.Directives.ForName(sourceInfoDirective); declaration == nil {
		sources = append(sources, &ast.Source{Name: "sourceInfo", Input: sourceInfoDeclaration})
	} else if err := checkSourceInfoDeclaration(declaration); err != nil {
		return nil, err
	}

	schema, err := gqlparser.LoadSchema(sources...)
	if err != nil {
		return nil, err
	}

	index, err := indexSourceInfo(schema)
	if err != nil {
		return nil, err
	}

	return &CompiledSchema{Schema: schema, sourceInfo: index}, nil
}

// checkSourceInfoDeclaration rejects a @sourceInfo declaration that the
// federator can't read: it must apply to field definitions and take exactly
// schemaId and providerField, each a String.
func checkSourceInfoDeclaration(declaration *ast.DirectiveDefinition) error {
	if !containsLocation(declaration.Locations, ast.LocationFieldDefinition) {
		return fmt.Errorf("directive @%s must be declared on FIELD_DEFINITION", sourceInfoDirective)
	}
	names := make([]string, 0, len(declaration.Arguments))
	for _, arg := range declaration.Arguments {
		if arg.Type.Elem != nil || arg.Type.NamedType != "String" {
			return fmt.Errorf("argument %s of directive @%s must be a String", arg.Name, sourceInfoDirective)
		}
		names = append(names, arg.Name)
	}
	sort.Strings(names)
	if fmt.Sprint(names) != fmt.Sprint(sourceInfoArguments) {
		return fmt.Errorf("directive @%s must take schemaId and providerField, got %v", sourceInfoDirective, names)
	}
	return nil
}

func containsLocation(locations []ast.DirectiveLocation, want ast.DirectiveLocation) bool {
	for _, location := range locations {
		if location == want {
			return true
		}
	}
	return false
}

// indexSourceInfo returns the @sourceInfo of every field in schema that has one.
func indexSourceInfo(schema *ast.Schema) (map[fieldKey]SourceInfo, error) {
	index := make(map[fieldKey]SourceInfo)
	for _, def := range schema.Types {
		if def.BuiltIn {
			continue
		}
		for _, field := range def.Fields {
			directive := field.Directives.ForName(sourceInfoDirective)
			if directive == nil {
				continue
			}
			info := SourceInfo{
				SchemaID:      argumentString(directive, "schemaId"),
				ProviderField: argumentString(directive, "providerField"),
			}
			if info.SchemaID == "" || info.ProviderField == "" {
				return nil, fmt.Errorf("@%s on %s.%s must set schemaId and providerField", sourceInfoDirective, def.Name, field.Name)
			}
			index[fieldKey{def.Name, field.Name}] = info
		}
	}
	return index, nil
}

// argumentString returns the string value of directive's argument name, or "".
func argumentString(directive *ast.Directive, name string) string {
	arg := directive.Arguments.ForName(name)
	if arg == nil || arg.Value == nil || arg.Value.Kind != ast.StringValue {
		return ""
	}
	return arg.Value.Raw
}
