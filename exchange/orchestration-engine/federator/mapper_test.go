package federator

import (
	"testing"

	"github.com/OpenNDX/openndx-core/exchange/orchestration-engine/pkg/graphql"
	"github.com/graphql-go/graphql/language/ast"
	"github.com/graphql-go/graphql/language/kinds"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildSchemaInfoMap_WithArrayAndSimpleFields(t *testing.T) {
	schemaSDL := `
		directive @sourceInfo(providerKey: String!, providerField: String!, schemaId: String) on FIELD_DEFINITION

		type Query {
			personInfo: PersonInfo
		}

		type PersonInfo {
			fullName: String @sourceInfo(providerKey: "drp", providerField: "person.fullName")
			ownedVehicles: [VehicleInfo] @sourceInfo(providerKey: "dmt", providerField: "vehicle.getVehicleInfos.data")
		}

		type VehicleInfo {
			regNo: String @sourceInfo(providerKey: "dmt", providerField: "vehicle.getVehicleInfos.data.registrationNumber")
			make: String @sourceInfo(providerKey: "dmt", providerField: "vehicle.getVehicleInfos.data.make")
		}
	`

	querySDL := `
				query {
			personInfo {
						fullName
						ownedVehicles {
							regNo
							make
				}
			}
		}
	`

	schema := ParseSchemaDoc(t, schemaSDL)
	query := ParseQueryDoc(t, querySDL)

	schemaInfoMap, err := BuildSchemaInfoMap(schema, query)
	require.NoError(t, err)

	require.Contains(t, schemaInfoMap, "personInfo.fullName")
	fullNameInfo := schemaInfoMap["personInfo.fullName"]
	assert.Equal(t, "drp", fullNameInfo.ProviderKey)
	assert.Equal(t, "person.fullName", fullNameInfo.ProviderField)
	assert.False(t, fullNameInfo.IsArray)

	require.Contains(t, schemaInfoMap, "personInfo.ownedVehicles")
	vehiclesInfo := schemaInfoMap["personInfo.ownedVehicles"]
	assert.Equal(t, "dmt", vehiclesInfo.ProviderKey)
	assert.True(t, vehiclesInfo.IsArray)
	assert.Equal(t, "vehicle.getVehicleInfos.data", vehiclesInfo.ProviderArrayFieldPath)

	require.Contains(t, vehiclesInfo.SubFieldSchemaInfos, "regNo")
	regInfo := vehiclesInfo.SubFieldSchemaInfos["regNo"]
	assert.Equal(t, "dmt", regInfo.ProviderKey)
	assert.Equal(t, "registrationNumber", regInfo.ProviderField)

	require.Contains(t, vehiclesInfo.SubFieldSchemaInfos, "make")
	makeInfo := vehiclesInfo.SubFieldSchemaInfos["make"]
	assert.Equal(t, "make", makeInfo.ProviderField)
}

func TestProcessNestedFieldsForArray(t *testing.T) {
	schemaSDL := `
		directive @sourceInfo(providerKey: String!, providerField: String!) on FIELD_DEFINITION

		type VehicleInfo {
			regNo: String @sourceInfo(providerKey: "dmt", providerField: "vehicle.getVehicleInfos.data.registrationNumber")
			make: String @sourceInfo(providerKey: "dmt", providerField: "vehicle.getVehicleInfos.data.make")
		}
	`

	querySDL := `
		query {
			personInfo {
				ownedVehicles {
					regNo
					make
				}
			}
		}
	`

	schema := ParseSchemaDoc(t, schemaSDL)
	query := ParseQueryDoc(t, querySDL)

	selectionSet := query.Definitions[0].(*ast.OperationDefinition).
		SelectionSet.Selections[0].(*ast.Field).
		SelectionSet.Selections[0].(*ast.Field).
		SelectionSet

	vehicleInfo := findTopLevelObjectDefinitionInSchema("VehicleInfo", schema)
	require.NotNil(t, vehicleInfo)

	subFields := make(map[string]*SourceSchemaInfo)
	processNestedFieldsForArray(selectionSet, schema, vehicleInfo, subFields)

	require.Len(t, subFields, 2)
	assert.Equal(t, "registrationNumber", subFields["regNo"].ProviderField)
	assert.Equal(t, "make", subFields["make"].ProviderField)
}

func TestPushVariablesFromVariableDefinition(t *testing.T) {
	arg := &ast.Argument{
		Name: &ast.Name{Value: "nic"},
		Value: &ast.Variable{
			Kind: kinds.Variable,
			Name: &ast.Name{Value: "nicVar"},
		},
	}

	argSource := &ArgSource{
		ArgMapping: &graphql.ArgMapping{},
		Argument:   arg,
	}

	request := graphql.Request{
		Variables: map[string]interface{}{
			"nicVar":  "123456789V",
			"flagVar": true,
		},
	}

	varDefs := []*ast.VariableDefinition{
		{
			Variable: &ast.Variable{Name: &ast.Name{Value: "nicVar"}},
		},
	}

	PushVariablesFromVariableDefinition(request, []*ArgSource{argSource}, varDefs)

	val, ok := argSource.Argument.Value.(*ast.StringValue)
	require.True(t, ok, "Argument value should be converted to StringValue")
	assert.Equal(t, "123456789V", val.Value)
}

func TestPushArgumentValue(t *testing.T) {
	tests := []struct {
		name     string
		value    interface{}
		expected ast.Value
	}{
		{
			name:  "String",
			value: "hello",
			expected: &ast.StringValue{
				Kind:  kinds.StringValue,
				Value: "hello",
			},
		},
		{
			name:  "Int",
			value: 65,
			expected: &ast.IntValue{
				Kind:  kinds.IntValue,
				Value: string(rune(65)),
			},
		},
		{
			name:  "Float",
			value: 123.45,
			expected: &ast.FloatValue{
				Kind:  kinds.FloatValue,
				Value: "123.45",
			},
		},
		{
			name:  "Bool",
			value: true,
			expected: &ast.BooleanValue{
				Kind:  kinds.BooleanValue,
				Value: true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			arg := &ast.Argument{Name: &ast.Name{Value: "test"}}
			PushArgumentValue(arg, tt.value)
			assert.Equal(t, tt.expected, arg.Value)
		})
	}
}

func TestQueryBuilder(t *testing.T) {
	tests := []struct {
		name          string
		fieldsMap     *[]ProviderLevelFieldRecord
		args          []*ArgSource
		expectedCount int
		expectedKeys  []string
		expectError   bool
		description   string
	}{
		{
			name: "Single Provider Query",
			fieldsMap: &[]ProviderLevelFieldRecord{
				{
					ServiceKey: "drp",
					SchemaId:   "drp-schema",
					FieldPath:  "person.fullName",
				},
				{
					ServiceKey: "drp",
					SchemaId:   "drp-schema",
					FieldPath:  "person.address",
				},
			},
			args: []*ArgSource{
				{
					ArgMapping: &graphql.ArgMapping{
						ProviderKey:   "drp",
						TargetArgName: "nic",
						SourceArgPath: "personInfo-nic",
						TargetArgPath: "drp.person",
					},
					Argument: &ast.Argument{
						Name:  &ast.Name{Value: "nic"},
						Value: &ast.StringValue{Value: "123456789V"},
					},
				},
			},
			expectedCount: 1,
			expectedKeys:  []string{"drp"},
			expectError:   false,
			description:   "Should build single provider query with arguments",
		},
		{
			name: "Multiple Provider Queries",
			fieldsMap: &[]ProviderLevelFieldRecord{
				{
					ServiceKey: "drp",
					SchemaId:   "drp-schema",
					FieldPath:  "person.fullName",
				},
				{
					ServiceKey: "rgd",
					SchemaId:   "rgd-schema",
					FieldPath:  "getPersonInfo.name",
				},
				{
					ServiceKey: "dmt",
					SchemaId:   "dmt-schema",
					FieldPath:  "vehicle.data",
				},
			},
			args: []*ArgSource{
				{
					ArgMapping: &graphql.ArgMapping{
						ProviderKey:   "drp",
						TargetArgName: "nic",
						SourceArgPath: "personInfo-nic",
						TargetArgPath: "drp.person",
					},
					Argument: &ast.Argument{
						Name:  &ast.Name{Value: "nic"},
						Value: &ast.StringValue{Value: "123456789V"},
					},
				},
				{
					ArgMapping: &graphql.ArgMapping{
						ProviderKey:   "rgd",
						TargetArgName: "nic",
						SourceArgPath: "personInfo-nic",
						TargetArgPath: "rgd.getPersonInfo",
					},
					Argument: &ast.Argument{
						Name:  &ast.Name{Value: "nic"},
						Value: &ast.StringValue{Value: "123456789V"},
					},
				},
			},
			expectedCount: 3,
			expectedKeys:  []string{"drp", "rgd", "dmt"},
			expectError:   false,
			description:   "Should build multiple provider queries",
		},
		{
			name:          "Empty Fields Map",
			fieldsMap:     &[]ProviderLevelFieldRecord{},
			args:          []*ArgSource{},
			expectedCount: 0,
			expectedKeys:  []string{},
			expectError:   false,
			description:   "Should handle empty fields map",
		},
		{
			name: "Query with Array Arguments",
			fieldsMap: &[]ProviderLevelFieldRecord{
				{
					ServiceKey: "dmt",
					SchemaId:   "dmt-schema",
					FieldPath:  "vehicle.getVehicleInfos.data",
				},
			},
			args: []*ArgSource{
				{
					ArgMapping: &graphql.ArgMapping{
						ProviderKey:   "dmt",
						TargetArgName: "regNos",
						SourceArgPath: "vehicles-regNos",
						TargetArgPath: "dmt.vehicle.getVehicleInfos",
					},
					Argument: &ast.Argument{
						Name: &ast.Name{Value: "regNos"},
						Value: &ast.ListValue{
							Values: []ast.Value{
								&ast.StringValue{Value: "ABC123"},
								&ast.StringValue{Value: "XYZ789"},
							},
						},
					},
				},
			},
			expectedCount: 1,
			expectedKeys:  []string{"dmt"},
			expectError:   false,
			description:   "Should handle array arguments for bulk queries",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			requests, err := QueryBuilder(tt.fieldsMap, tt.args)

			if tt.expectError {
				assert.Error(t, err, tt.description)
			} else {
				assert.NoError(t, err, tt.description)
				assert.Len(t, requests, tt.expectedCount, "Should create correct number of requests")

				// Verify service keys
				actualKeys := make([]string, len(requests))
				for i, request := range requests {
					actualKeys[i] = request.ServiceKey
				}
				assert.ElementsMatch(t, tt.expectedKeys, actualKeys, "Should have correct service keys")

				// Verify each request has a valid GraphQL query
				for _, request := range requests {
					assert.NotEmpty(t, request.GraphQLRequest.Query, "Should have non-empty query")
					assert.NotNil(t, request.GraphQLRequest, "Should have valid GraphQL request")
				}
			}
		})
	}
}

func TestRecursivelyExtractSourceSchemaInfo(t *testing.T) {
	schema := CreateTestSchema(t)

	tests := []struct {
		name           string
		query          string
		expectedFields []string
		expectedArgs   int
		description    string
	}{
		{
			name: "Simple Query with Multiple Fields",
			query: `
				query {
					personInfo(nic: "123456789V") {
						fullName
						name
						address
					}
				}
			`,
			expectedFields: []string{
				"drp.person.fullName",
				"rgd.getPersonInfo.name",
				"drp.person.permanentAddress",
			},
			expectedArgs: 1,
			description:  "Should extract source info from multiple simple fields",
		},
		{
			name: "Array Field Query",
			query: `
				query {
					personInfo(nic: "123456789V") {
						fullName
						ownedVehicles {
							regNo
							make
							model
						}
					}
				}
			`,
			expectedFields: []string{
				"drp.person.fullName",
				"dmt.vehicle.getVehicleInfos.data",
				"dmt.vehicle.getVehicleInfos.data.registrationNumber",
				"dmt.vehicle.getVehicleInfos.data.make",
				"dmt.vehicle.getVehicleInfos.data.model",
			},
			expectedArgs: 1,
			description:  "Should extract source info from array fields",
		},
		{
			name: "Complex Query with Multiple Arrays",
			query: `
				query {
					personInfo(nic: "123456789V") {
						fullName
						address
						ownedVehicles {
							regNo
							make
						}
						class {
							className
						}
					}
				}
			`,
			expectedFields: []string{
				"drp.person.fullName",
				"drp.person.permanentAddress",
				"dmt.vehicle.getVehicleInfos.data",
				"dmt.vehicle.getVehicleInfos.data.registrationNumber",
				"dmt.vehicle.getVehicleInfos.data.make",
				"dmt.vehicle.getVehicleInfos.classes",
				"dmt.vehicle.getVehicleInfos.classes.className",
			},
			expectedArgs: 1,
			description:  "Should extract source info from complex nested structure with multiple arrays",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			queryDoc := ParseTestQuery(t, tt.query)
			operationDef := queryDoc.Definitions[0].(*ast.OperationDefinition)
			selectionSet := operationDef.SelectionSet

			// Get query object definition from schema
			queryObjectDef := GetQueryObjectDefinition(schema)
			assert.NotNil(t, queryObjectDef, "Should find query object definition")

			// Extract source schema info
			directives, arguments := RecursivelyExtractSourceSchemaInfo(
				selectionSet, schema, queryObjectDef, nil, nil)

			// Convert directives to field map
			fieldMapPtr := ProviderFieldMap(directives)
			fieldMap := *fieldMapPtr

			// Build a map of extracted fields for easier assertion
			extractedFields := make(map[string]bool)
			for _, record := range fieldMap {
				// Build key as: providerKey.fieldPath
				key := record.ServiceKey + "." + record.FieldPath
				extractedFields[key] = true
			}

			assert.Len(t, fieldMap, len(tt.expectedFields), "Should extract correct number of fields")
			assert.Len(t, arguments, tt.expectedArgs, "Should extract correct number of arguments")

			// Verify extracted fields
			for _, expectedField := range tt.expectedFields {
				assert.True(t, extractedFields[expectedField], "Should contain field: %s", expectedField)
			}
		})
	}
}

func TestFindFieldDefinitionFromFieldName(t *testing.T) {
	schema := CreateTestSchema(t)

	tests := []struct {
		name           string
		fieldName      string
		parentObject   string
		expectedExists bool
		description    string
	}{
		{
			name:           "Existing Field in PersonInfo",
			fieldName:      "fullName",
			parentObject:   "PersonInfo",
			expectedExists: true,
			description:    "Should find existing field in PersonInfo",
		},
		{
			name:           "Existing Field in VehicleInfo",
			fieldName:      "regNo",
			parentObject:   "VehicleInfo",
			expectedExists: true,
			description:    "Should find existing field in VehicleInfo",
		},
		{
			name:           "Non-existent Field",
			fieldName:      "nonExistentField",
			parentObject:   "PersonInfo",
			expectedExists: false,
			description:    "Should return nil for non-existent field",
		},
		{
			name:           "Non-existent Parent Object",
			fieldName:      "fullName",
			parentObject:   "NonExistentType",
			expectedExists: false,
			description:    "Should return nil for non-existent parent object",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fieldDef := FindFieldDefinitionFromFieldName(tt.fieldName, schema, tt.parentObject)

			if tt.expectedExists {
				assert.NotNil(t, fieldDef, tt.description)
				assert.Equal(t, tt.fieldName, fieldDef.Name.Value, "Should have correct field name")
			} else {
				assert.Nil(t, fieldDef, tt.description)
			}
		})
	}
}

func TestGetQueryObjectDefinition(t *testing.T) {
	schema := CreateTestSchema(t)

	queryDef := GetQueryObjectDefinition(schema)
	assert.NotNil(t, queryDef, "Should find query object definition")
	assert.Equal(t, "Query", queryDef.Name.Value, "Should have correct name")
	assert.Greater(t, len(queryDef.Fields), 0, "Should have fields")
}

// Helper functions
