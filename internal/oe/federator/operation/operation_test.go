// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Lanka Software Foundation

package operation

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strings"
	"testing"

	"github.com/openndx/openndx-core/internal/oe/federator/schema"
	"github.com/openndx/openndx-core/internal/oe/pkg/graphql"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vektah/gqlparser/v2/ast"
	"github.com/vektah/gqlparser/v2/gqlerror"
	"github.com/vektah/gqlparser/v2/parser"
	"github.com/vektah/gqlparser/v2/validator"
)

const testSchema = `
type Query {
	personInfo(nic: String!, lang: String = "en"): PersonInfo
	vehicles(ownerNic: String): [Vehicle]
	search(nic: String!): Searchable
	ownerOf(nic: String!): PersonInfo
}

type Mutation {
	noop: String
}

interface Node {
	id: ID
}

union Searchable = PersonInfo | Vehicle

type PersonInfo implements Node {
	id: ID
	fullName: String @sourceInfo(schemaId: "identity-registry-v1", providerField: "person.fullName")
	address: String @sourceInfo(schemaId: "identity-registry-v1", providerField: "person.permanentAddress")
	ownedVehicles(year: Int): [Vehicle] @sourceInfo(schemaId: "vehicle-registry-v1", providerField: "vehicle.getVehicleInfos.data")
	birthInfo: BirthInfo
}

type BirthInfo {
	date: String @sourceInfo(schemaId: "civil-registry-v1", providerField: "getPersonInfo.birthDate")
}

type Vehicle implements Node {
	id: ID
	regNo: String @sourceInfo(schemaId: "vehicle-registry-v1", providerField: "vehicle.getVehicleInfos.data.registrationNumber")
	make: String @sourceInfo(schemaId: "vehicle-registry-v1", providerField: "vehicle.getVehicleInfos.data.make")
}
`

func compile(t *testing.T) *schema.CompiledSchema {
	t.Helper()
	compiled, err := schema.Compile(testSchema)
	require.NoError(t, err)
	return compiled
}

// build parses and validates query, as FederateQuery does, then normalizes it.
func build(t *testing.T, query string, variables map[string]any) (*Operation, error) {
	t.Helper()
	compiled := compile(t)
	doc, err := parser.ParseQuery(&ast.Source{Name: "Query", Input: query})
	require.NoError(t, err)
	require.Empty(t, validator.ValidateWithRules(compiled.Schema, doc, nil), "the test query must be valid")
	return Build(compiled, doc, "", variables)
}

func mustBuild(t *testing.T, query string, variables map[string]any) *Operation {
	t.Helper()
	op, err := build(t, query, variables)
	require.NoError(t, err)
	return op
}

// keys returns the response keys of fields.
func keys(fields []*Field) []string {
	keys := make([]string, 0, len(fields))
	for _, f := range fields {
		keys = append(keys, f.ResponseKey)
	}
	return keys
}

func TestBuild(t *testing.T) {
	t.Run("binds fields to their definitions and @sourceInfo", func(t *testing.T) {
		op := mustBuild(t, `query { personInfo(nic: "1") { fullName birthInfo { date } } }`, nil)

		require.Len(t, op.Fields, 1)
		person := op.Fields[0]
		assert.Equal(t, "personInfo", person.ResponseKey)
		assert.Equal(t, "personInfo", person.Definition.Name)
		assert.Nil(t, person.SourceInfo)
		assert.Equal(t, []string{"fullName", "birthInfo"}, keys(person.Selections))

		fullName := person.Selections[0]
		assert.Equal(t, []string{"personInfo", "fullName"}, fullName.Path)
		assert.Equal(t, &schema.SourceInfo{SchemaID: "identity-registry-v1", ProviderField: "person.fullName"}, fullName.SourceInfo)
		assert.Nil(t, fullName.Selections)

		date := person.Selections[1].Selections[0]
		assert.Equal(t, []string{"personInfo", "birthInfo", "date"}, date.Path)
		assert.Equal(t, "getPersonInfo.birthDate", date.SourceInfo.ProviderField)
	})

	t.Run("binds list fields and their element fields", func(t *testing.T) {
		op := mustBuild(t, `query { personInfo(nic: "1") { ownedVehicles { regNo make } } }`, nil)

		vehicles := op.Fields[0].Selections[0]
		assert.Equal(t, "vehicle.getVehicleInfos.data", vehicles.SourceInfo.ProviderField)
		assert.NotNil(t, vehicles.Definition.Type.Elem, "the field is a list")
		require.Equal(t, []string{"regNo", "make"}, keys(vehicles.Selections))
		assert.Equal(t, "vehicle.getVehicleInfos.data.registrationNumber", vehicles.Selections[0].SourceInfo.ProviderField)
	})

	t.Run("keeps aliased fields apart", func(t *testing.T) {
		op := mustBuild(t, `query { a: personInfo(nic: "111") { fullName } b: personInfo(nic: "222") { name: address } }`, nil)

		require.Equal(t, []string{"a", "b"}, keys(op.Fields))
		a, b := op.Fields[0], op.Fields[1]
		assert.Equal(t, "personInfo", a.Name)
		assert.Equal(t, []string{"a"}, a.Path)
		assert.Equal(t, []string{"personInfo"}, a.SchemaPath)
		assert.Equal(t, "111", a.Arguments["nic"])
		assert.Equal(t, "222", b.Arguments["nic"])

		name := b.Selections[0]
		assert.Equal(t, "name", name.ResponseKey)
		assert.Equal(t, "address", name.Name)
		assert.Equal(t, []string{"b", "name"}, name.Path)
		assert.Equal(t, []string{"personInfo", "address"}, name.SchemaPath)
		assert.Equal(t, "person.permanentAddress", name.SourceInfo.ProviderField)
	})

	t.Run("merges fields with the same response key", func(t *testing.T) {
		op := mustBuild(t, `query { personInfo(nic: "1") { fullName } personInfo(nic: "1") { address fullName } }`, nil)

		require.Len(t, op.Fields, 1)
		assert.Equal(t, []string{"fullName", "address"}, keys(op.Fields[0].Selections))
	})

	t.Run("merges named and inline fragments", func(t *testing.T) {
		op := mustBuild(t, `
			query {
				personInfo(nic: "1") {
					...names
					... { address }
					... on PersonInfo { birthInfo { date } }
					...names
				}
			}
			fragment names on PersonInfo { fullName }`, nil)

		assert.Equal(t, []string{"fullName", "address", "birthInfo"}, keys(op.Fields[0].Selections))
	})

	t.Run("applies fragments on an interface or union only when they contain the type", func(t *testing.T) {
		op := mustBuild(t, `
			query {
				personInfo(nic: "1") {
					... on Node { id }
					... on Searchable {
						... on PersonInfo { fullName }
						... on Vehicle { make }
					}
					...vehicleFields
				}
			}
			fragment vehicleFields on Searchable { ... on Vehicle { regNo } }`, nil)

		assert.Equal(t, []string{"id", "fullName"}, keys(op.Fields[0].Selections))
	})

	t.Run("applies @skip and @include", func(t *testing.T) {
		op := mustBuild(t, `
			query ($withAddress: Boolean!, $skipName: Boolean = true) {
				personInfo(nic: "1") {
					fullName @skip(if: $skipName)
					address @include(if: $withAddress)
					id @skip(if: false) @include(if: true)
					birthInfo @include(if: false) { date }
					... @skip(if: true) { ownedVehicles { make } }
				}
			}`, map[string]any{"withAddress": true})

		assert.Equal(t, []string{"address", "id"}, keys(op.Fields[0].Selections))
	})

	t.Run("coerces arguments with variables and defaults", func(t *testing.T) {
		op := mustBuild(t, `
			query ($nic: String!, $lang: String, $year: Int = 2020) {
				personInfo(nic: $nic, lang: $lang) { ownedVehicles(year: $year) { make } }
				vehicles { make }
			}`, map[string]any{"nic": "FL-2954817360"})

		person := op.Fields[0]
		assert.Equal(t, map[string]any{"nic": "FL-2954817360", "lang": "en"}, person.Arguments,
			"a variable the request leaves out falls back to the argument's default")
		assert.Equal(t, map[string]any{"year": int64(2020)}, person.Selections[0].Arguments,
			"a variable's default applies")
		assert.Empty(t, op.Fields[1].Arguments, "an optional argument without a default is absent")
		assert.Equal(t, map[string]any{"nic": "FL-2954817360", "year": int64(2020)}, op.Variables)
	})

	t.Run("coerces literal arguments", func(t *testing.T) {
		op := mustBuild(t, `query { personInfo(nic: "1", lang: null) { ownedVehicles(year: 2024) { make } } }`, nil)

		assert.Equal(t, map[string]any{"nic": "1", "lang": nil}, op.Fields[0].Arguments, "an explicit null is kept")
		assert.Equal(t, map[string]any{"year": int64(2024)}, op.Fields[0].Selections[0].Arguments)
	})

	t.Run("selects the named operation", func(t *testing.T) {
		compiled := compile(t)
		doc, err := parser.ParseQuery(&ast.Source{Input: `
			query A { personInfo(nic: "1") { fullName } }
			query B { vehicles { make } }`})
		require.NoError(t, err)
		require.Empty(t, validator.ValidateWithRules(compiled.Schema, doc, nil))

		op, err := Build(compiled, doc, "B", nil)
		require.NoError(t, err)
		assert.Equal(t, []string{"vehicles"}, keys(op.Fields))

		_, err = Build(compiled, doc, "", nil)
		assert.ErrorContains(t, err, "operationName is required")

		_, err = Build(compiled, doc, "C", nil)
		assert.ErrorContains(t, err, `operation "C" not found`)
	})

	t.Run("rejects mutations", func(t *testing.T) {
		_, err := build(t, `mutation { noop }`, nil)
		assert.ErrorIs(t, err, ErrUnsupportedOperation)
	})

	t.Run("rejects a missing required variable", func(t *testing.T) {
		_, err := build(t, `query ($nic: String!) { personInfo(nic: $nic) { fullName } }`, nil)
		var gqlErr *gqlerror.Error
		require.ErrorAs(t, err, &gqlErr)
		assert.Contains(t, gqlErr.Message, "must be defined")
	})

	t.Run("rejects selections on a field of union or interface type", func(t *testing.T) {
		_, err := build(t, `query { search(nic: "1") { ... on PersonInfo { fullName } } }`, nil)
		assert.ErrorContains(t, err, "Query.search has type Searchable: fields of interface or union type are not supported")
	})
}

func TestBindArguments(t *testing.T) {
	identityRegistry := &graphql.ArgMapping{SchemaID: "identity-registry-v1", SourceArgPath: "personInfo-nic", TargetArgPath: "person", TargetArgName: "nic"}
	vehicleRegistry := &graphql.ArgMapping{SchemaID: "vehicle-registry-v1", SourceArgPath: "personInfo-ownedVehicles-year", TargetArgPath: "vehicle.getVehicleInfos", TargetArgName: "year"}

	t.Run("binds each occurrence by the full path", func(t *testing.T) {
		op := mustBuild(t, `
			query {
				a: personInfo(nic: "111") { ownedVehicles(year: 2024) { make } }
				b: personInfo(nic: "222") { fullName }
				ownerOf(nic: "333") { fullName }
			}`, nil)

		bindings := op.BindArguments([]*graphql.ArgMapping{identityRegistry, vehicleRegistry, nil})

		require.Len(t, bindings, 3)
		assert.Same(t, identityRegistry, bindings[0].Mapping)
		assert.Equal(t, "111", bindings[0].Value)
		assert.Equal(t, []string{"a"}, bindings[0].Field.Path)
		assert.Equal(t, "222", bindings[1].Value)
		assert.Equal(t, []string{"b"}, bindings[1].Field.Path)
		assert.Same(t, vehicleRegistry, bindings[2].Mapping)
		assert.Equal(t, int64(2024), bindings[2].Value)
		assert.Equal(t, []string{"a", "ownedVehicles"}, bindings[2].Field.Path)
	})

	t.Run("binds a default value and skips an absent argument", func(t *testing.T) {
		op := mustBuild(t, `query { personInfo(nic: "1") { ownedVehicles { make } } }`, nil)
		lang := &graphql.ArgMapping{SourceArgPath: "personInfo-lang"}

		bindings := op.BindArguments([]*graphql.ArgMapping{lang, vehicleRegistry})

		require.Len(t, bindings, 1)
		assert.Same(t, lang, bindings[0].Mapping)
		assert.Equal(t, "en", bindings[0].Value)
	})

	t.Run("doesn't bind an argument by name alone", func(t *testing.T) {
		op := mustBuild(t, `query { ownerOf(nic: "1") { fullName } }`, nil)

		bindings := op.BindArguments([]*graphql.ArgMapping{{SourceArgPath: "nic"}, {SourceArgPath: "personInfo-nic"}})

		assert.Empty(t, bindings)
	})
}

func TestDataOwnerID(t *testing.T) {
	for name, tc := range map[string]struct {
		query string
		want  string
		err   error
	}{
		"one value":                    {query: `{ personInfo(nic: "1") { fullName } }`, want: "1"},
		"the same value on two fields": {query: `{ a: personInfo(nic: "1") { fullName } b: ownerOf(nic: "1") { address } }`, want: "1"},
		"no value":                     {query: `{ vehicles(ownerNic: "1") { make } }`, err: ErrMissingIdentifier},
		"an empty value":               {query: `{ personInfo(nic: "") { fullName } }`, err: ErrMissingIdentifier},
		"different values via aliases": {query: `{ a: personInfo(nic: "111") { fullName } b: personInfo(nic: "222") { address } }`, err: ErrAmbiguousIdentifier},
	} {
		t.Run(name, func(t *testing.T) {
			op := mustBuild(t, tc.query, nil)

			got, err := op.DataOwnerID("nic")

			if tc.err != nil {
				assert.ErrorIs(t, err, tc.err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}

	t.Run("reads a value from a variable", func(t *testing.T) {
		op := mustBuild(t, `query ($nic: String!) { personInfo(nic: $nic) { fullName } }`, map[string]any{"nic": "42"})

		got, err := op.DataOwnerID("nic")
		require.NoError(t, err)
		assert.Equal(t, "42", got)
	})
}

// TestBuild_SampleSchema normalizes a query against cmd/oe/schema.graphql that
// uses a named fragment, an inline fragment, aliases, @skip and @include with
// variables, and two data owners.
func TestBuild_SampleSchema(t *testing.T) {
	src, err := schema.Load("../../../../cmd/oe/schema.graphql")
	require.NoError(t, err)
	compiled, err := src.Active(context.Background())
	require.NoError(t, err)

	doc, err := parser.ParseQuery(&ast.Source{Name: "Query", Input: `
		query Person($nic: String!, $withBirth: Boolean!, $skipAddress: Boolean = true) {
			citizen: personInfo(nic: $nic) {
				...identity
				home: address @skip(if: $skipAddress)
				... on PersonInfo { profession }
				birthInfo @include(if: $withBirth) {
					place: birthPlace
					district
				}
			}
			other: personInfo(nic: "FL-7316048259") { fullName }
		}
		fragment identity on PersonInfo { fullName sex dateOfBirth }`})
	require.NoError(t, err)
	require.Empty(t, validator.ValidateWithRules(compiled.Schema, doc, nil))

	op, err := Build(compiled, doc, "Person", map[string]any{"nic": "FL-4829105736", "withBirth": true})
	require.NoError(t, err)

	assert.Equal(t, strings.TrimLeft(`
citizen (personInfo) nic=FL-4829105736
  fullName <- identity-registry-v1 person.fullName
  sex <- civil-registry-v1 getPersonInfo.sex
  dateOfBirth <- civil-registry-v1 getPersonInfo.birthDate
  profession <- identity-registry-v1 person.profession
  birthInfo
    place (birthPlace) <- civil-registry-v1 getPersonInfo.birthPlace
    district <- civil-registry-v1 getPersonInfo.district
other (personInfo) nic=FL-7316048259
  fullName <- identity-registry-v1 person.fullName
`, "\n"), render(op.Fields))

	_, err = op.DataOwnerID("nic")
	assert.ErrorIs(t, err, ErrAmbiguousIdentifier, "the query asks about two data owners")

	// The argument mappings in cmd/oe/config.example.yaml.
	identityRegistry := &graphql.ArgMapping{SchemaID: "identity-registry-v1", SourceArgPath: "personInfo-nic", TargetArgPath: "person", TargetArgName: "nic"}
	civilRegistry := &graphql.ArgMapping{SchemaID: "civil-registry-v1", SourceArgPath: "personInfo-nic", TargetArgPath: "getPersonInfo", TargetArgName: "nic"}
	var bound []string
	for _, binding := range op.BindArguments([]*graphql.ArgMapping{identityRegistry, civilRegistry}) {
		bound = append(bound, fmt.Sprintf("%s %s=%v", binding.Mapping.SchemaID, strings.Join(binding.Field.Path, "."), binding.Value))
	}
	assert.Equal(t, []string{"identity-registry-v1 citizen=FL-4829105736", "identity-registry-v1 other=FL-7316048259", "civil-registry-v1 citizen=FL-4829105736", "civil-registry-v1 other=FL-7316048259"}, bound)
}

// render prints fields one per line, indented by depth: the response key, the
// field name when aliased, the arguments, and the @sourceInfo.
func render(fields []*Field) string {
	var b strings.Builder
	var write func(fields []*Field, indent string)
	write = func(fields []*Field, indent string) {
		for _, field := range fields {
			b.WriteString(indent + field.ResponseKey)
			if field.Name != field.ResponseKey {
				b.WriteString(" (" + field.Name + ")")
			}
			for _, name := range slices.Sorted(maps.Keys(field.Arguments)) {
				fmt.Fprintf(&b, " %s=%v", name, field.Arguments[name])
			}
			if info := field.SourceInfo; info != nil {
				fmt.Fprintf(&b, " <- %s %s", info.SchemaID, info.ProviderField)
			}
			b.WriteString("\n")
			write(field.Selections, indent+"  ")
		}
	}
	write(fields, "")
	return b.String()
}
