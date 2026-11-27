// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Lanka Software Foundation

package federator

import (
	"strconv"
	"strings"
	"testing"

	"github.com/openndx/openndx-core/internal/oe/federator/operation"
	"github.com/openndx/openndx-core/internal/oe/federator/schema"
	"github.com/openndx/openndx-core/internal/oe/pkg/graphql"
	"github.com/openndx/openndx-core/internal/oe/policy"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vektah/gqlparser/v2/ast"
	"github.com/vektah/gqlparser/v2/parser"
	"github.com/vektah/gqlparser/v2/validator"
)

const planTestSchema = `
enum Lang { EN SI }

type Query {
	personInfo(nic: String!, lang: Lang, year: Int, filter: Filter): PersonInfo
	required(nic: String!): Required!
}

input Filter {
	makes: [String!]
	since: Int
}

type PersonInfo {
	fullName: String @sourceInfo(schemaId: "identity-registry-v1", providerField: "person.fullName")
	tags: [String] @sourceInfo(schemaId: "identity-registry-v1", providerField: "person.tags")
	ownedVehicles: [Vehicle!] @sourceInfo(schemaId: "vehicle-registry-v1", providerField: "vehicle.list.data")
	untracked: String
	contacts: [Contact]
}

type Contact {
	phone: String @sourceInfo(schemaId: "identity-registry-v1", providerField: "person.phone")
}

type Vehicle {
	regNo: String! @sourceInfo(schemaId: "vehicle-registry-v1", providerField: "vehicle.list.data.registrationNumber")
	make: String @sourceInfo(schemaId: "vehicle-registry-v1", providerField: "make")
	owner: Owner
}

type Owner {
	name: String @sourceInfo(schemaId: "vehicle-registry-v1", providerField: "vehicle.list.data.owner.name")
}

type Required {
	value: String! @sourceInfo(schemaId: "identity-registry-v1", providerField: "required.value")
	optional: String @sourceInfo(schemaId: "identity-registry-v1", providerField: "required.optional")
}
`

var planTestMappings = []*graphql.ArgMapping{
	{SchemaID: "identity-registry-v1", SourceArgPath: "personInfo-nic", TargetArgPath: "person", TargetArgName: "nic"},
	{SchemaID: "identity-registry-v1", SourceArgPath: "personInfo-lang", TargetArgPath: "person", TargetArgName: "lang"},
	{SchemaID: "identity-registry-v1", SourceArgPath: "personInfo-year", TargetArgPath: "person", TargetArgName: "year"},
	{SchemaID: "vehicle-registry-v1", SourceArgPath: "personInfo-nic", TargetArgPath: "vehicle.list", TargetArgName: "ownerNic"},
	{SchemaID: "vehicle-registry-v1", SourceArgPath: "personInfo-filter", TargetArgPath: "vehicle.list", TargetArgName: "filter", TargetArgType: "VehicleFilter"},
	{SchemaID: "identity-registry-v1", SourceArgPath: "required-nic", TargetArgPath: "required", TargetArgName: "nic"},
}

// planFor normalizes and plans query against planTestSchema.
func planFor(t *testing.T, query string, variables map[string]any) (*operation.Operation, *plan) {
	t.Helper()
	return planWith(t, query, variables, planTestMappings)
}

// planWith is planFor with mappings in place of planTestMappings.
func planWith(t *testing.T, query string, variables map[string]any, mappings []*graphql.ArgMapping) (*operation.Operation, *plan) {
	t.Helper()
	compiled, err := schema.Compile(planTestSchema)
	require.NoError(t, err)
	doc, err := parser.ParseQuery(&ast.Source{Input: query})
	require.NoError(t, err)
	require.Empty(t, validator.ValidateWithRules(compiled.Schema, doc, nil), "the test query must be valid")
	op, err := operation.Build(compiled, doc, "", variables)
	require.NoError(t, err)
	targetTypes, err := targetArgTypes(mappings)
	require.NoError(t, err)
	p, err := buildPlan(op, mappings, targetTypes)
	require.NoError(t, err)
	return op, p
}

// compact joins a printed query onto one line.
func compact(query string) string {
	return strings.Join(strings.Fields(query), " ")
}

func TestBuildPlan(t *testing.T) {
	t.Run("builds one operation per provider with arguments only at their target path", func(t *testing.T) {
		_, p := planFor(t, `{ personInfo(nic: "1", lang: SI) { fullName tags ownedVehicles { regNo make owner { name } } untracked __typename } }`, nil)

		require.Len(t, p.fetches, 2)
		identityRegistry, vehicleRegistry := p.fetches[0], p.fetches[1]
		assert.Equal(t, "identity-registry-v1", identityRegistry.schemaID)
		assert.Equal(t, `query Queryidentity_registry_v1 ($lang: Lang!, $nic: String!) { person(lang: $lang, nic: $nic) { fullName tags } }`, compact(identityRegistry.query))
		assert.Equal(t, map[string]any{"lang": "SI", "nic": "1"}, identityRegistry.variables)
		assert.Equal(t, []string{"person.fullName", "person.tags"}, identityRegistry.fields)

		assert.Equal(t, "vehicle-registry-v1", vehicleRegistry.schemaID)
		assert.Equal(t, `query Queryvehicle_registry_v1 ($ownerNic: String!) { vehicle { list(ownerNic: $ownerNic) { data { registrationNumber make owner { name } } } } }`, compact(vehicleRegistry.query),
			"a list item's field declared outside the list is read by its last segment")
		assert.Equal(t, map[string]any{"ownerNic": "1"}, vehicleRegistry.variables)
		assert.Equal(t, []string{"vehicle.list.data.registrationNumber", "vehicle.list.data.make", "vehicle.list.data.owner.name"}, vehicleRegistry.fields)

		assert.Equal(t, []policy.RequiredField{
			{SchemaID: "identity-registry-v1", FieldName: "person.fullName"},
			{SchemaID: "identity-registry-v1", FieldName: "person.tags"},
			{SchemaID: "vehicle-registry-v1", FieldName: "vehicle.list.data.registrationNumber"},
			{SchemaID: "vehicle-registry-v1", FieldName: "vehicle.list.data.make"},
			{SchemaID: "vehicle-registry-v1", FieldName: "vehicle.list.data.owner.name"},
		}, p.requiredFields(), "only leaves are authorized, not the list container")
	})

	t.Run("shares a fetch between aliases with the same arguments", func(t *testing.T) {
		_, p := planFor(t, `{ a: personInfo(nic: "1") { fullName } b: personInfo(nic: "1") { name: fullName tags } }`, nil)

		require.Len(t, p.fetches, 1)
		assert.Equal(t, `query Queryidentity_registry_v1 ($nic: String!) { person(nic: $nic) { fullName tags } }`, compact(p.fetches[0].query))
		assert.Equal(t, []policy.RequiredField{
			{SchemaID: "identity-registry-v1", FieldName: "person.fullName"},
			{SchemaID: "identity-registry-v1", FieldName: "person.tags"},
		}, p.requiredFields(), "a provider field read twice is authorized once")
	})

	t.Run("splits fetches when aliases map different argument values", func(t *testing.T) {
		_, p := planFor(t, `{ a: personInfo(nic: "1", lang: EN) { fullName } b: personInfo(nic: "1", lang: SI) { fullName } }`, nil)

		require.Len(t, p.fetches, 2)
		assert.Equal(t, p.fetches[0].query, p.fetches[1].query, "the operation text doesn't depend on the values")
		assert.Equal(t, map[string]any{"lang": "EN", "nic": "1"}, p.fetches[0].variables)
		assert.Equal(t, map[string]any{"lang": "SI", "nic": "1"}, p.fetches[1].variables)
	})

	t.Run("forwards variables and defaults with their consumer argument type", func(t *testing.T) {
		_, p := planFor(t, `query ($lang: Lang = SI, $year: Int) { personInfo(nic: "1", lang: $lang, year: $year) { fullName } }`,
			map[string]any{"year": float64(2024)})

		require.Len(t, p.fetches, 1)
		assert.Equal(t, `query Queryidentity_registry_v1 ($lang: Lang!, $nic: String!, $year: Int!) { person(lang: $lang, nic: $nic, year: $year) { fullName } }`,
			compact(p.fetches[0].query), "a nullable consumer type is declared non-null when the value isn't null")
		assert.Equal(t, map[string]any{"lang": "SI", "nic": "1", "year": float64(2024)}, p.fetches[0].variables,
			"a variable's default is forwarded")
	})

	t.Run("forwards a null value with the nullable consumer type", func(t *testing.T) {
		_, p := planFor(t, `{ personInfo(nic: "1", year: null) { fullName } }`, nil)

		require.Len(t, p.fetches, 1)
		assert.Equal(t, `query Queryidentity_registry_v1 ($nic: String!, $year: Int) { person(nic: $nic, year: $year) { fullName } }`, compact(p.fetches[0].query))
		assert.Equal(t, map[string]any{"nic": "1", "year": nil}, p.fetches[0].variables)
	})

	t.Run("forwards an object variable with its targetArgType", func(t *testing.T) {
		_, p := planFor(t, `query ($filter: Filter) { personInfo(nic: "1", filter: $filter) { ownedVehicles { regNo } } }`,
			map[string]any{"filter": map[string]any{"makes": []any{"Toyota", "Honda"}, "since": float64(2020)}})

		require.Len(t, p.fetches, 1)
		assert.Equal(t, `query Queryvehicle_registry_v1 ($filter: VehicleFilter, $ownerNic: String!) { vehicle { list(filter: $filter, ownerNic: $ownerNic) { data { registrationNumber } } } }`,
			compact(p.fetches[0].query), "targetArgType is declared as given")
		assert.Equal(t, map[string]any{
			"filter":   map[string]any{"makes": []any{"Toyota", "Honda"}, "since": float64(2020)},
			"ownerNic": "1",
		}, p.fetches[0].variables)
	})

	t.Run("splits fetches when aliases map different object values", func(t *testing.T) {
		_, p := planFor(t, `{ a: personInfo(nic: "1", filter: {makes: ["Toyota"]}) { ownedVehicles { regNo } } b: personInfo(nic: "1", filter: {makes: ["Honda"]}) { ownedVehicles { make } } }`, nil)

		require.Len(t, p.fetches, 2)
		assert.Equal(t, map[string]any{"makes": []any{"Toyota"}}, p.fetches[0].variables["filter"])
		assert.Equal(t, map[string]any{"makes": []any{"Honda"}}, p.fetches[1].variables["filter"])
	})

	t.Run("suffixes a variable whose name is already taken", func(t *testing.T) {
		_, p := planWith(t, `{ personInfo(nic: "1", year: 2024) { tags } }`, nil, []*graphql.ArgMapping{
			{SchemaID: "identity-registry-v1", SourceArgPath: "personInfo-nic", TargetArgPath: "person", TargetArgName: "id"},
			{SchemaID: "identity-registry-v1", SourceArgPath: "personInfo-year", TargetArgPath: "person.tags", TargetArgName: "id", TargetArgType: "ID!"},
		})

		require.Len(t, p.fetches, 1)
		assert.Equal(t, `query Queryidentity_registry_v1 ($id: String!, $id_2: ID!) { person(id: $id) { tags(id: $id_2) } }`, compact(p.fetches[0].query))
		assert.Equal(t, map[string]any{"id": "1", "id_2": int64(2024)}, p.fetches[0].variables)
	})

	t.Run("plans nothing for __typename and fields without @sourceInfo", func(t *testing.T) {
		_, p := planFor(t, `{ __typename personInfo(nic: "1") { untracked __typename } }`, nil)

		assert.Empty(t, p.fetches)
	})

	t.Run("rejects a list field with a selection but no @sourceInfo", func(t *testing.T) {
		compiled, err := schema.Compile(planTestSchema)
		require.NoError(t, err)
		doc, err := parser.ParseQuery(&ast.Source{Input: `{ personInfo(nic: "1") { contacts { phone } } }`})
		require.NoError(t, err)
		require.Empty(t, validator.ValidateWithRules(compiled.Schema, doc, nil))
		op, err := operation.Build(compiled, doc, "", nil)
		require.NoError(t, err)

		_, err = buildPlan(op, planTestMappings, nil)
		assert.EqualError(t, err, "list field personInfo.contacts has no @sourceInfo, so its items can't be read from a provider")
	})

	t.Run("rejects introspection fields", func(t *testing.T) {
		compiled, err := schema.Compile(planTestSchema)
		require.NoError(t, err)
		doc, err := parser.ParseQuery(&ast.Source{Input: `{ __schema { queryType { name } } }`})
		require.NoError(t, err)
		require.Empty(t, validator.ValidateWithRules(compiled.Schema, doc, nil))
		op, err := operation.Build(compiled, doc, "", nil)
		require.NoError(t, err)

		_, err = buildPlan(op, planTestMappings, nil)
		assert.EqualError(t, err, "introspection field __schema is not supported")
	})
}

func TestParseType(t *testing.T) {
	for _, s := range []string{"ID", "ID!", "[String!]!", " [ String ! ] "} {
		typ, err := parseType(s)
		require.NoError(t, err, s)
		assert.Equal(t, strings.Join(strings.Fields(s), ""), typ.String())
	}
	for _, s := range []string{"", "!", "[String", "String = 1", "String) { a } query q($b: Int", "String @deprecated"} {
		_, err := parseType(s)
		assert.EqualError(t, err, strconv.Quote(s)+" is not a GraphQL type")
	}
}
