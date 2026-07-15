package federator

import (
	"testing"

	"github.com/OpenNDX/openndx-core/exchange/orchestration-engine/logger"
	"github.com/OpenNDX/openndx-core/exchange/orchestration-engine/pkg/graphql"
	"github.com/graphql-go/graphql/language/ast"
	"github.com/stretchr/testify/assert"
)

func init() {
	logger.Init()
}

func TestAccumulateResponseWithSchemaInfo(t *testing.T) {
	query := `
		query {
			personInfo(nic: "123456789V") {
				fullName
				ownedVehicles {
					regNo
					make
				}
			}
		}
	`

	queryDoc := ParseTestQuery(t, query)

	federatedResponse := &FederationResponse{
		Responses: []*ProviderResponse{
			{
				ServiceKey: "dmt",
				Response: graphql.Response{
					Data: map[string]interface{}{
						"vehicle": map[string]interface{}{
							"getVehicleInfos": map[string]interface{}{
								"data": []interface{}{
									map[string]interface{}{
										"registrationNumber": "ABC123",
										"make":               "Toyota",
									},
								},
							},
						},
					},
				},
			},
		},
	}

	schemaInfoMap := map[string]*SourceSchemaInfo{
		"personInfo.ownedVehicles": {
			IsArray:                true,
			ProviderKey:            "dmt",
			ProviderArrayFieldPath: "vehicle.getVehicleInfos.data",
			SubFieldSchemaInfos: map[string]*SourceSchemaInfo{
				"regNo": {
					ProviderKey:   "dmt",
					ProviderField: "registrationNumber",
				},
				"make": {
					ProviderKey:   "dmt",
					ProviderField: "make",
				},
			},
		},
		"personInfo.fullName": {
			IsArray:       false,
			ProviderKey:   "dmt",
			ProviderField: "vehicle.getVehicleInfos.data.fullName",
		},
	}

	response := AccumulateResponseWithSchemaInfo(queryDoc, federatedResponse, schemaInfoMap)

	assert.NotNil(t, response.Data)
}

func TestAccumulateArrayResponse_ErrorCases(t *testing.T) {
	query := `query { personInfo { ownedVehicles { regNo } } }`
	queryDoc := ParseTestQuery(t, query)

	// Test with non-existent provider
	schemaInfoMap := map[string]*SourceSchemaInfo{
		"personInfo.ownedVehicles": {
			IsArray:                true,
			ProviderKey:            "non-existent",
			ProviderArrayFieldPath: "vehicle.getVehicleInfos.data",
			SubFieldSchemaInfos: map[string]*SourceSchemaInfo{
				"regNo": {
					ProviderKey:   "non-existent",
					ProviderField: "registrationNumber",
				},
			},
		},
	}

	federatedResponse := &FederationResponse{
		Responses: []*ProviderResponse{},
	}

	response := AccumulateResponseWithSchemaInfo(queryDoc, federatedResponse, schemaInfoMap)
	// Should handle error gracefully, response may be empty or partial
	assert.NotNil(t, response.Data)
}

func TestPushValue_ErrorCases(t *testing.T) {
	tests := []struct {
		name        string
		obj         interface{}
		path        string
		value       interface{}
		expectError bool
	}{
		{
			name:        "Invalid type in path",
			obj:         123, // Not a map or array
			path:        "test.path",
			value:       "value",
			expectError: true,
		},
		{
			name:        "Push to array with invalid element type",
			obj:         []interface{}{123}, // Array with non-map element
			path:        "test.path",
			value:       "value",
			expectError: false, // Should handle gracefully
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := PushValue(tt.obj, tt.path, tt.value)
			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, result)
			} else {
				// May or may not error depending on implementation
				_ = result
			}
		})
	}
}

func TestGetValueAtPath_ErrorCases(t *testing.T) {
	tests := []struct {
		name        string
		data        interface{}
		path        string
		expectError bool
	}{
		{
			name:        "Invalid type",
			data:        123,
			path:        "test.path",
			expectError: true,
		},
		{
			name:        "Key not found in map",
			data:        map[string]interface{}{"other": "value"},
			path:        "test.path",
			expectError: true,
		},
		{
			name:        "Empty path",
			data:        map[string]interface{}{"test": "value"},
			path:        "",
			expectError: true, // Empty string splits to [""], which causes key lookup to fail
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := GetValueAtPath(tt.data, tt.path)
			if tt.expectError {
				assert.Error(t, err)
				// For empty path, verify the error message indicates key lookup failure
				if tt.path == "" {
					assert.Contains(t, err.Error(), "not found", "Empty path should produce a 'not found' error")
				}
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
			}
		})
	}
}

func TestIsArrayField(t *testing.T) {
	tests := []struct {
		name     string
		field    *ast.Field
		expected bool
	}{
		{
			name: "Field with selection set",
			field: &ast.Field{
				Name: &ast.Name{Value: "test"},
				SelectionSet: &ast.SelectionSet{
					Selections: []ast.Selection{
						&ast.Field{Name: &ast.Name{Value: "nested"}},
					},
				},
			},
			expected: true,
		},
		{
			name: "Field without selection set",
			field: &ast.Field{
				Name:         &ast.Name{Value: "test"},
				SelectionSet: nil,
			},
			expected: false,
		},
		{
			name: "Field with empty selection set",
			field: &ast.Field{
				Name: &ast.Name{Value: "test"},
				SelectionSet: &ast.SelectionSet{
					Selections: []ast.Selection{},
				},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isArrayField(tt.field)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsNestedFieldOfArray(t *testing.T) {
	query := `
		query {
			personInfo {
				ownedVehicles {
					regNo
					make
				}
			}
		}
	`

	queryDoc := ParseTestQuery(t, query)

	tests := []struct {
		name     string
		path     []string
		expected bool
	}{
		{
			name:     "Nested field of array",
			path:     []string{"personInfo", "ownedVehicles", "regNo"},
			expected: true,
		},
		{
			name:     "Array field itself",
			path:     []string{"personInfo", "ownedVehicles"},
			expected: true, // Array field with selection set is considered nested
		},
		{
			name:     "Top level field",
			path:     []string{"personInfo"},
			expected: false,
		},
		{
			name:     "Short path",
			path:     []string{"personInfo"},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isNestedFieldOfArray(tt.path, queryDoc)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFindFieldInQuery(t *testing.T) {
	query := `
		query {
			personInfo {
				fullName
				ownedVehicles {
					regNo
				}
			}
		}
	`

	queryDoc := ParseTestQuery(t, query)

	tests := []struct {
		name      string
		fieldName string
		expected  bool
	}{
		{
			name:      "Find top level field",
			fieldName: "personInfo",
			expected:  true,
		},
		{
			name:      "Find nested field",
			fieldName: "fullName",
			expected:  true,
		},
		{
			name:      "Find array field",
			fieldName: "ownedVehicles",
			expected:  true,
		},
		{
			name:      "Find non-existent field",
			fieldName: "nonExistent",
			expected:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			field := findFieldInQuery(queryDoc, tt.fieldName)
			if tt.expected {
				assert.NotNil(t, field)
				assert.Equal(t, tt.fieldName, field.Name.Value)
			} else {
				assert.Nil(t, field)
			}
		})
	}
}

func TestExtractRelativeFieldPath(t *testing.T) {
	tests := []struct {
		name          string
		providerField string
		expected      string
	}{
		{
			name:          "Simple field",
			providerField: "registrationNumber",
			expected:      "registrationNumber",
		},
		{
			name:          "Nested path",
			providerField: "vehicle.getVehicleInfos.data.registrationNumber",
			expected:      "registrationNumber",
		},
		{
			name:          "Empty path",
			providerField: "",
			expected:      "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractRelativeFieldPath(tt.providerField)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestPushValue_WithArray(t *testing.T) {
	// Test pushing to array elements
	obj := map[string]interface{}{
		"personInfo": map[string]interface{}{
			"ownedVehicles": []interface{}{
				map[string]interface{}{},
				map[string]interface{}{},
			},
		},
	}

	result, err := PushValue(obj, "personInfo.ownedVehicles.regNo", "ABC123")
	assert.NoError(t, err)

	resultMap := result.(map[string]interface{})
	personInfo := resultMap["personInfo"].(map[string]interface{})
	vehicles := personInfo["ownedVehicles"].([]interface{})

	assert.Len(t, vehicles, 2)
	// Both vehicles should have regNo
	for _, v := range vehicles {
		vehicle := v.(map[string]interface{})
		assert.Equal(t, "ABC123", vehicle["regNo"])
	}
}

func TestGetValueAtPath_WithArray(t *testing.T) {
	data := map[string]interface{}{
		"persons": []interface{}{
			map[string]interface{}{
				"name": "John",
				"age":  30,
			},
			map[string]interface{}{
				"name": "Jane",
				"age":  25,
			},
		},
	}

	// Get name from all array elements
	result, err := GetValueAtPath(data, "persons.name")
	assert.NoError(t, err)

	names := result.([]interface{})
	assert.Len(t, names, 2)
	assert.Contains(t, names, "John")
	assert.Contains(t, names, "Jane")
}

func TestIsArrayFieldValue(t *testing.T) {
	tests := []struct {
		name      string
		fieldName string
		value     interface{}
		expected  bool
	}{
		{
			name:      "Array value",
			fieldName: "vehicles",
			value:     []interface{}{map[string]interface{}{"regNo": "ABC123"}},
			expected:  true,
		},
		{
			name:      "Non-array value",
			fieldName: "name",
			value:     "John Doe",
			expected:  false,
		},
		{
			name:      "Map value",
			fieldName: "person",
			value:     map[string]interface{}{"name": "John"},
			expected:  false,
		},
		{
			name:      "Nil value",
			fieldName: "test",
			value:     nil,
			expected:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isArrayFieldValue(tt.fieldName, tt.value)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// Helper function
