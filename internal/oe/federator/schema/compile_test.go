// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Lanka Software Foundation

package schema

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const personSchema = `
type Query {
	personInfo(nic: String!): PersonInfo
}

type PersonInfo {
	fullName: String @sourceInfo(schemaId: "identity-registry-v1", providerField: "person.fullName")
	birthDate: String @sourceInfo(schemaId: "civil-registry-v1", providerField: "getPersonInfo.birthDate")
}
`

func TestCompile(t *testing.T) {
	t.Run("indexes @sourceInfo and adds its declaration when missing", func(t *testing.T) {
		compiled, err := Compile(personSchema)
		require.NoError(t, err)

		info, ok := compiled.SourceInfo("PersonInfo", "fullName")
		require.True(t, ok)
		assert.Equal(t, SourceInfo{SchemaID: "identity-registry-v1", ProviderField: "person.fullName"}, info)

		info, ok = compiled.SourceInfo("PersonInfo", "birthDate")
		require.True(t, ok)
		assert.Equal(t, SourceInfo{SchemaID: "civil-registry-v1", ProviderField: "getPersonInfo.birthDate"}, info)

		_, ok = compiled.SourceInfo("Query", "personInfo")
		assert.False(t, ok, "fields without @sourceInfo are not indexed")
	})

	t.Run("accepts a declaration with the arguments in any order and nullability", func(t *testing.T) {
		_, err := Compile(`directive @sourceInfo(providerField: String, schemaId: String!) on FIELD_DEFINITION` + personSchema)
		require.NoError(t, err)
	})

	for name, sdl := range map[string]string{
		"syntax error":                      `type Query {`,
		"unknown type":                      `type Query { person: Person }`,
		"unknown directive argument":        `type Query { a: String @sourceInfo(schemaId: "p", providerField: "a", other: "x") }`,
		"providerKey argument":              `type Query { a: String @sourceInfo(providerKey: "p", schemaId: "p", providerField: "a") }`,
		"missing schemaId":                  `type Query { a: String @sourceInfo(providerField: "a") }`,
		"missing providerField":             `type Query { a: String @sourceInfo(schemaId: "p") }`,
		"declared on the wrong location":    `directive @sourceInfo(schemaId: String!, providerField: String!) on OBJECT type Query { a: String }`,
		"declared without providerField":    `directive @sourceInfo(schemaId: String!) on FIELD_DEFINITION type Query { a: String }`,
		"declared with providerKey":         `directive @sourceInfo(providerKey: String!, schemaId: String!, providerField: String!) on FIELD_DEFINITION type Query { a: String }`,
		"declared with a non-String type":   `directive @sourceInfo(schemaId: Int!, providerField: String!) on FIELD_DEFINITION type Query { a: String }`,
		"empty schemaId when nullable":      `directive @sourceInfo(schemaId: String, providerField: String) on FIELD_DEFINITION type Query { a: String @sourceInfo(providerField: "a") }`,
		"empty providerField when nullable": `directive @sourceInfo(schemaId: String, providerField: String) on FIELD_DEFINITION type Query { a: String @sourceInfo(schemaId: "p", providerField: "") }`,
	} {
		t.Run("rejects "+name, func(t *testing.T) {
			_, err := Compile(sdl)
			assert.Error(t, err)
		})
	}
}

// TestCompile_RepositorySchemas checks that the schemas shipped in the repository
// compile, whether or not they declare @sourceInfo.
func TestCompile_RepositorySchemas(t *testing.T) {
	for _, path := range []string{
		"../../../../cmd/oe/schema.graphql",
		"../../../../tests/integration/schema.graphql",
	} {
		t.Run(path, func(t *testing.T) {
			sdl, err := os.ReadFile(path)
			require.NoError(t, err)
			_, err = Compile(string(sdl))
			require.NoError(t, err)
		})
	}
}
