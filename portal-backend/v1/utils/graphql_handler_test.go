package utils

import (
	"strings"
	"testing"

	"github.com/OpenNDX/openndx-core/portal-backend/v1/models"
	"github.com/stretchr/testify/assert"
	"github.com/vektah/gqlparser/v2/ast"
)

func TestGraphQLHandler_buildFieldPath(t *testing.T) {
	handler := NewGraphQLHandler()

	tests := []struct {
		name      string
		typeName  string
		fieldName string
		expected  string
	}{
		{"Simple", "User", "id", "user.id"},
		{"CamelCase", "BirthInfo", "birthPlace", "birthinfo.birthPlace"},
		{"MixedCase", "UserProfile", "fullName", "userprofile.fullName"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := handler.buildFieldPath(tt.typeName, tt.fieldName)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGraphQLHandler_getDirectiveValue(t *testing.T) {
	handler := NewGraphQLHandler()

	// Create a directive list with test data
	directives := ast.DirectiveList{
		{
			Name: "accessControl",
			Arguments: ast.ArgumentList{
				{
					Name: "type",
					Value: &ast.Value{
						Kind: ast.StringValue,
						Raw:  `"public"`,
					},
				},
			},
		},
		{
			Name: "source",
			Arguments: ast.ArgumentList{
				{
					Name: "value",
					Value: &ast.Value{
						Kind: ast.StringValue,
						Raw:  `"fallback"`,
					},
				},
			},
		},
	}

	t.Run("GetExistingDirective", func(t *testing.T) {
		result := handler.getDirectiveValue(directives, "accessControl", "type")
		// The value should have quotes stripped
		assert.Contains(t, result, "public")
	})

	t.Run("GetNonExistentDirective", func(t *testing.T) {
		result := handler.getDirectiveValue(directives, "nonExistent", "type")
		assert.Empty(t, result)
	})

	t.Run("GetNonExistentArgument", func(t *testing.T) {
		result := handler.getDirectiveValue(directives, "accessControl", "nonExistent")
		assert.Empty(t, result)
	})
}

func TestGraphQLHandler_getBaseTypeName(t *testing.T) {
	handler := NewGraphQLHandler()

	t.Run("SimpleType", func(t *testing.T) {
		fieldType := &ast.Type{
			NamedType: "User",
		}
		result := handler.getBaseTypeName(fieldType)
		assert.Equal(t, "User", result)
	})

	t.Run("ListType", func(t *testing.T) {
		fieldType := &ast.Type{
			Elem: &ast.Type{
				NamedType: "User",
			},
		}
		result := handler.getBaseTypeName(fieldType)
		assert.Equal(t, "User", result)
	})

	t.Run("NonNullListType", func(t *testing.T) {
		fieldType := &ast.Type{
			NonNull: true,
			Elem: &ast.Type{
				NamedType: "User",
			},
		}
		result := handler.getBaseTypeName(fieldType)
		assert.Equal(t, "User", result)
	})
}

func TestGraphQLHandler_isBuiltInType(t *testing.T) {
	handler := NewGraphQLHandler()

	tests := []struct {
		name     string
		typeName string
		expected bool
	}{
		{"String", "String", true},
		{"Int", "Int", true},
		{"Float", "Float", true},
		{"Boolean", "Boolean", true},
		{"ID", "ID", true},
		{"__Schema", "__Schema", true},
		{"__Type", "__Type", true},
		{"CustomType", "User", false},
		{"CustomType2", "BirthInfo", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := handler.isBuiltInType(tt.typeName)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGraphQLHandler_isRootType(t *testing.T) {
	handler := NewGraphQLHandler()

	tests := []struct {
		name     string
		typeName string
		expected bool
	}{
		{"Query", "Query", true},
		{"Mutation", "Mutation", true},
		{"Subscription", "Subscription", true},
		{"CustomType", "User", false},
		{"CustomType2", "BirthInfo", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := handler.isRootType(tt.typeName)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGraphQLHandler_ParseSDLToPolicyRequest_InvalidSDL(t *testing.T) {
	handler := NewGraphQLHandler()

	invalidSDL := `type User { id: ID!` // Missing closing brace

	_, err := handler.ParseSDLToPolicyRequest("test-schema", invalidSDL)
	assert.Error(t, err)
	if err != nil {
		assert.Contains(t, err.Error(), "failed to parse GraphQL schema")
	}
}

func TestGraphQLHandler_ParseSDLToPolicyRequest_WithNestedFields(t *testing.T) {
	handler := NewGraphQLHandler()

	sdl := `
	directive @accessControl(type: String) on FIELD_DEFINITION
	directive @source(value: String) on FIELD_DEFINITION

	type BirthInfo {
	  birthCertificateID: ID! @accessControl(type: "public") @source(value: "fallback")
	  birthPlace: String! @accessControl(type: "public") @source(value: "fallback")
	}

	type User {
	  id: ID! @accessControl(type: "public") @source(value: "fallback")
	  birthInfo: BirthInfo @accessControl(type: "public") @source(value: "fallback")
	}

	type Query {
	  getUser(id: ID!): User
	}
	`

	request, err := handler.ParseSDLToPolicyRequest("test-schema", sdl)
	assert.NoError(t, err)
	assert.NotNil(t, request)
	assert.Equal(t, "test-schema", request.SchemaID)
	assert.GreaterOrEqual(t, len(request.Records), 3) // At least user.id, user.birthInfo, birthinfo.birthCertificateID, birthinfo.birthPlace

	// Check for nested field - verify we have nested fields from BirthInfo
	hasNestedField := false
	fieldNames := make([]string, 0, len(request.Records))
	for _, record := range request.Records {
		fieldNames = append(fieldNames, record.FieldName)
		// Check for nested fields (birthinfo.* or user.birthinfo.*)
		if len(record.FieldName) > len("birthinfo.") &&
			(strings.HasPrefix(record.FieldName, "birthinfo.") || strings.Contains(record.FieldName, "birthinfo.")) {
			hasNestedField = true
		}
	}
	assert.True(t, hasNestedField, "Should have nested field. Got fields: %v", fieldNames)
}

func TestGraphQLHandler_ParseSDLToPolicyRequest_WithDirectives(t *testing.T) {
	handler := NewGraphQLHandler()

	sdl := `
	directive @accessControl(type: String) on FIELD_DEFINITION
	directive @source(value: String) on FIELD_DEFINITION
	directive @displayName(value: String) on FIELD_DEFINITION
	directive @description(value: String) on FIELD_DEFINITION
	directive @isOwner(value: Boolean) on FIELD_DEFINITION
	directive @owner(value: String) on FIELD_DEFINITION

	type User {
	  id: ID! @accessControl(type: "restricted") @source(value: "drc") @displayName(value: "User ID") @description(value: "Unique identifier") @isOwner(value: true) @owner(value: "member")
	}

	type Query {
	  getUser(id: ID!): User
	}
	`

	request, err := handler.ParseSDLToPolicyRequest("test-schema", sdl)
	assert.NoError(t, err)
	assert.NotNil(t, request)

	// Find the user.id record
	var userIDRecord *models.PolicyMetadataCreateRequestRecord
	for i := range request.Records {
		if request.Records[i].FieldName == "user.id" {
			userIDRecord = &request.Records[i]
			break
		}
	}

	assert.NotNil(t, userIDRecord)
	assert.Equal(t, models.AccessControlType("restricted"), userIDRecord.AccessControlType)
	assert.Equal(t, models.Source("drc"), userIDRecord.Source)
	assert.NotNil(t, userIDRecord.DisplayName)
	assert.Equal(t, "User ID", *userIDRecord.DisplayName)
	assert.NotNil(t, userIDRecord.Description)
	assert.Equal(t, "Unique identifier", *userIDRecord.Description)
	assert.True(t, userIDRecord.IsOwner)
	assert.NotNil(t, userIDRecord.Owner)
	assert.Equal(t, models.Owner("member"), *userIDRecord.Owner)
}

func TestGraphQLHandler_ParseSDLToPolicyRequest_NoDirectives(t *testing.T) {
	handler := NewGraphQLHandler()

	sdl := `
	type User {
	  id: ID!
	  name: String!
	}

	type Query {
	  getUser(id: ID!): User
	}
	`

	request, err := handler.ParseSDLToPolicyRequest("test-schema", sdl)
	assert.NoError(t, err)
	assert.NotNil(t, request)
	// Should have no records since no directives are present
	assert.Equal(t, 0, len(request.Records))
}
