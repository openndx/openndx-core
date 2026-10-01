package federator

import (
	"testing"

	"github.com/graphql-go/graphql/language/parser"
	"github.com/graphql-go/graphql/language/source"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testSchemaSDL = `
directive @sourceInfo(providerKey: String!, providerField: String!, schemaId: String) on FIELD_DEFINITION

type PersonInfo {
	fullName: String @sourceInfo(providerKey: "drp", schemaId: "drp-schema-v1", providerField: "person.fullName")
	class: [VehicleClass!]! @sourceInfo(providerKey: "dmt", schemaId: "dmt-schema-v1", providerField: "vehicle.classes")
	profile: Profile! @sourceInfo(providerKey: "drp", schemaId: "drp-schema-v1", providerField: "person.profile")
}

type Profile {
	contact: Contact @sourceInfo(providerKey: "drp", schemaId: "drp-schema-v1", providerField: "person.contact")
}

type Contact {
	email: String @sourceInfo(providerKey: "drp", schemaId: "drp-schema-v1", providerField: "person.email")
}

type VehicleClass {
	className: String @sourceInfo(providerKey: "dmt", schemaId: "dmt-schema-v1", providerField: "vehicle.classes.className")
}
`

func TestExtractSourceInfoFromSchema(t *testing.T) {
	doc, err := parser.Parse(parser.ParseParams{Source: source.NewSource(&source.Source{Body: []byte(testSchemaSDL)})})
	require.NoError(t, err)

	t.Run("direct type.field", func(t *testing.T) {
		info := ExtractSourceInfoFromSchema(doc, "PersonInfo.fullName")
		require.NotNil(t, info)
		assert.Equal(t, "drp", info.ProviderKey)
		assert.Equal(t, "person.fullName", info.ProviderField)
	})

	t.Run("nonnull wrapped list intermediate", func(t *testing.T) {
		// class is [VehicleClass!]! — previously failed because only bare *ast.List was handled
		info := ExtractSourceInfoFromSchema(doc, "personInfo.class.className")
		require.NotNil(t, info)
		assert.Equal(t, "dmt", info.ProviderKey)
		assert.Equal(t, "vehicle.classes.className", info.ProviderField)
	})

	t.Run("object intermediate longer than three parts", func(t *testing.T) {
		info := ExtractSourceInfoFromSchema(doc, "personInfo.profile.contact.email")
		require.NotNil(t, info)
		assert.Equal(t, "drp", info.ProviderKey)
		assert.Equal(t, "person.email", info.ProviderField)
	})

	t.Run("missing field returns nil", func(t *testing.T) {
		assert.Nil(t, ExtractSourceInfoFromSchema(doc, "personInfo.nope"))
		assert.Nil(t, ExtractSourceInfoFromSchema(doc, "personInfo.class.missing"))
		assert.Nil(t, ExtractSourceInfoFromSchema(doc, "alone"))
	})
}
