package policy

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vektah/gqlparser/v2/ast"
)

func TestSDLParser_buildFieldPath(t *testing.T) {
	parser := newSDLParser()

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
			result := parser.buildFieldPath(tt.typeName, tt.fieldName)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestSDLParser_getDirectiveValue(t *testing.T) {
	parser := newSDLParser()

	// Create a directive list with test data
	directives := ast.DirectiveList{
		{
			Name: "displayName",
			Arguments: ast.ArgumentList{
				{
					Name: "value",
					Value: &ast.Value{
						Kind: ast.StringValue,
						Raw:  `"Full name"`,
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
		result := parser.getDirectiveValue(directives, "displayName", "value")
		// The value should have quotes stripped
		assert.Contains(t, result, "Full name")
	})

	t.Run("GetNonExistentDirective", func(t *testing.T) {
		result := parser.getDirectiveValue(directives, "nonExistent", "type")
		assert.Empty(t, result)
	})

	t.Run("GetNonExistentArgument", func(t *testing.T) {
		result := parser.getDirectiveValue(directives, "displayName", "nonExistent")
		assert.Empty(t, result)
	})
}

func TestSDLParser_getBaseTypeName(t *testing.T) {
	parser := newSDLParser()

	t.Run("SimpleType", func(t *testing.T) {
		fieldType := &ast.Type{
			NamedType: "User",
		}
		result := parser.getBaseTypeName(fieldType)
		assert.Equal(t, "User", result)
	})

	t.Run("ListType", func(t *testing.T) {
		fieldType := &ast.Type{
			Elem: &ast.Type{
				NamedType: "User",
			},
		}
		result := parser.getBaseTypeName(fieldType)
		assert.Equal(t, "User", result)
	})

	t.Run("NonNullListType", func(t *testing.T) {
		fieldType := &ast.Type{
			NonNull: true,
			Elem: &ast.Type{
				NamedType: "User",
			},
		}
		result := parser.getBaseTypeName(fieldType)
		assert.Equal(t, "User", result)
	})
}

func TestSDLParser_isBuiltInType(t *testing.T) {
	parser := newSDLParser()

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
			result := parser.isBuiltInType(tt.typeName)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestSDLParser_isRootType(t *testing.T) {
	parser := newSDLParser()

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
			result := parser.isRootType(tt.typeName)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestSDLParser_ParseSDLToPolicyRequest_InvalidSDL(t *testing.T) {
	parser := newSDLParser()

	invalidSDL := `type User { id: ID!` // Missing closing brace

	_, err := parser.ParseSDLToPolicyRequest("test-schema", invalidSDL)
	assert.Error(t, err)
	if err != nil {
		assert.Contains(t, err.Error(), "failed to parse GraphQL schema")
	}
}

func TestSDLParser_ParseSDLToPolicyRequest_WithNestedFields(t *testing.T) {
	parser := newSDLParser()

	sdl := `
	directive @accessControl(allowlist: Boolean!, consent: Boolean!) on FIELD_DEFINITION
	directive @source(value: String) on FIELD_DEFINITION

	type BirthInfo {
	  birthCertificateID: ID! @accessControl(allowlist: true, consent: false) @source(value: "fallback")
	  birthPlace: String! @accessControl(allowlist: true, consent: false) @source(value: "fallback")
	}

	type User {
	  id: ID! @accessControl(allowlist: true, consent: false) @source(value: "fallback")
	  birthInfo: BirthInfo @accessControl(allowlist: true, consent: false) @source(value: "fallback")
	}

	type Query {
	  getUser(id: ID!): User
	}
	`

	request, err := parser.ParseSDLToPolicyRequest("test-schema", sdl)
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

func TestSDLParser_ParseSDLToPolicyRequest_WithDirectives(t *testing.T) {
	parser := newSDLParser()

	sdl := `
	directive @accessControl(allowlist: Boolean!, consent: Boolean!) on FIELD_DEFINITION
	directive @source(value: String) on FIELD_DEFINITION
	directive @displayName(value: String) on FIELD_DEFINITION
	directive @description(value: String) on FIELD_DEFINITION
	directive @owner(value: String) on FIELD_DEFINITION

	type User {
	  id: ID! @accessControl(allowlist: true, consent: true) @source(value: "drc") @displayName(value: "User ID") @description(value: "Unique identifier") @owner(value: "member")
	}

	type Query {
	  getUser(id: ID!): User
	}
	`

	request, err := parser.ParseSDLToPolicyRequest("test-schema", sdl)
	assert.NoError(t, err)
	assert.NotNil(t, request)

	// Find the user.id record
	var userIDRecord *PolicyMetadataCreateRequestRecord
	for i := range request.Records {
		if request.Records[i].FieldName == "user.id" {
			userIDRecord = &request.Records[i]
			break
		}
	}

	assert.NotNil(t, userIDRecord)
	assertFlags(t, *userIDRecord, true, true)
	assert.Equal(t, Source("drc"), userIDRecord.Source)
	assert.NotNil(t, userIDRecord.DisplayName)
	assert.Equal(t, "User ID", *userIDRecord.DisplayName)
	assert.NotNil(t, userIDRecord.Description)
	assert.Equal(t, "Unique identifier", *userIDRecord.Description)
	assert.NotNil(t, userIDRecord.Owner)
	assert.Equal(t, Owner("member"), *userIDRecord.Owner)
}

func TestSDLParser_ParseSDLToPolicyRequest_DefaultsToAllowlist(t *testing.T) {
	sdl := `
	directive @source(value: String) on FIELD_DEFINITION

	type User {
	  id: ID! @source(value: "primary")
	}
	`

	request, err := newSDLParser().ParseSDLToPolicyRequest("test-schema", sdl)
	assert.NoError(t, err)
	assert.Len(t, request.Records, 1)
	assertFlags(t, request.Records[0], true, false)
	assert.Nil(t, request.Records[0].Owner)
}

func assertFlags(t *testing.T, record PolicyMetadataCreateRequestRecord, allowlist, consent bool) {
	t.Helper()
	require.NotNil(t, record.RequiresAllowlist, record.FieldName)
	require.NotNil(t, record.RequiresConsent, record.FieldName)
	assert.Equal(t, allowlist, *record.RequiresAllowlist, "%s requiresAllowlist", record.FieldName)
	assert.Equal(t, consent, *record.RequiresConsent, "%s requiresConsent", record.FieldName)
}

func TestSDLParser_ParseSDLToPolicyRequest_AccessControlFlags(t *testing.T) {
	sdl := `
	directive @accessControl(allowlist: Boolean!, consent: Boolean!) on FIELD_DEFINITION
	directive @source(value: String) on FIELD_DEFINITION
	directive @owner(value: String) on FIELD_DEFINITION

	type Person {
	  nic: String @accessControl(allowlist: true, consent: true) @source(value: "primary") @owner(value: "citizen")
	  address: String @accessControl(allowlist: true, consent: false) @source(value: "primary")
	  birthDate: String @accessControl(allowlist: false, consent: true) @source(value: "primary") @owner(value: "citizen")
	  fullName: String @accessControl(consent: false, allowlist: false) @source(value: "primary")
	}
	`

	request, err := newSDLParser().ParseSDLToPolicyRequest("test-schema", sdl)
	require.NoError(t, err)
	records := make(map[string]PolicyMetadataCreateRequestRecord, len(request.Records))
	for _, record := range request.Records {
		records[record.FieldName] = record
	}
	require.Len(t, records, 4)
	assertFlags(t, records["person.nic"], true, true)
	assertFlags(t, records["person.address"], true, false)
	assertFlags(t, records["person.birthDate"], false, true)
	assertFlags(t, records["person.fullName"], false, false)
	require.NotNil(t, records["person.birthDate"].Owner)
	assert.Equal(t, OwnerCitizen, *records["person.birthDate"].Owner)
	assert.Nil(t, records["person.fullName"].Owner)
}

func TestSDLParser_ParseSDLToPolicyRequest_RejectsInvalidAccessControl(t *testing.T) {
	const directives = `
	directive @accessControl(type: String, policy: String, allowlist: Boolean, consent: Boolean) on FIELD_DEFINITION
	directive @source(value: String) on FIELD_DEFINITION
	directive @isOwner(value: Boolean) on FIELD_DEFINITION
	`

	tests := []struct {
		name    string
		field   string
		wantErr string
	}{
		{
			name:    "accessControl type",
			field:   `nic: String @accessControl(type: "restricted") @source(value: "primary")`,
			wantErr: "field person.nic: @accessControl(type: ...) was removed",
		},
		{
			name:    "accessControl policy",
			field:   `nic: String @accessControl(policy: "allowlist_consent") @source(value: "primary")`,
			wantErr: "field person.nic: @accessControl(policy: ...) was removed",
		},
		{
			name:    "isOwner",
			field:   `nic: String @accessControl(allowlist: true, consent: false) @isOwner(value: true)`,
			wantErr: "field person.nic: @isOwner was removed",
		},
		{
			name:    "accessControl without flags",
			field:   `nic: String @accessControl @source(value: "primary")`,
			wantErr: "field person.nic: @accessControl needs both allowlist and consent",
		},
		{
			name:    "accessControl missing consent",
			field:   `nic: String @accessControl(allowlist: false) @source(value: "primary")`,
			wantErr: "field person.nic: @accessControl needs both allowlist and consent",
		},
		{
			name:    "flag that is not a boolean",
			field:   `nic: String @accessControl(allowlist: "yes", consent: false) @source(value: "primary")`,
			wantErr: `field person.nic: @accessControl(allowlist: ...) must be true or false, not "yes"`,
		},
		{
			name:    "removed directive on a nested field",
			field:   "address: Address\n}\ntype Address {\n  city: String @accessControl(type: \"public\")",
			wantErr: "@accessControl(type: ...) was removed",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sdl := directives + "type Person {\n  " + tt.field + "\n}\n"

			_, err := newSDLParser().ParseSDLToPolicyRequest("test-schema", sdl)
			assert.ErrorContains(t, err, tt.wantErr)
		})
	}
}

func TestSDLParser_ParseSDLToPolicyRequest_NoDirectives(t *testing.T) {
	parser := newSDLParser()

	sdl := `
	type User {
	  id: ID!
	  name: String!
	}

	type Query {
	  getUser(id: ID!): User
	}
	`

	request, err := parser.ParseSDLToPolicyRequest("test-schema", sdl)
	assert.NoError(t, err)
	assert.NotNil(t, request)
	// Should have no records since no directives are present
	assert.Equal(t, 0, len(request.Records))
}
