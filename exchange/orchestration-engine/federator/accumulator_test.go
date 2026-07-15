package federator

import (
	"testing"

	"github.com/OpenNDX/openndx-core/exchange/orchestration-engine/logger"
	"github.com/OpenNDX/openndx-core/exchange/orchestration-engine/pkg/graphql"
	"github.com/stretchr/testify/assert"
)

func init() {
	logger.Init()
}

func TestAccumulateResponse_SingleObject(t *testing.T) {
	// Test query with @sourceInfo directives
	query := `
		query {
			personInfo(nic: "123456789V") {
				fullName @sourceInfo(providerKey: "drp", providerField: "person.fullName")
				name @sourceInfo(providerKey: "rgd", providerField: "getPersonInfo.name")
				address @sourceInfo(providerKey: "drp", providerField: "person.permanentAddress")
			}
		}
	`

	queryDoc := ParseTestQuery(t, query)

	// Mock federated response
	federatedResponse := &FederationResponse{
		Responses: []*ProviderResponse{
			{
				ServiceKey: "drp",
				Response: graphql.Response{
					Data: map[string]interface{}{
						"person": map[string]interface{}{
							"fullName":         "John Doe",
							"permanentAddress": "123 Main St",
						},
					},
				},
			},
			{
				ServiceKey: "rgd",
				Response: graphql.Response{
					Data: map[string]interface{}{
						"getPersonInfo": map[string]interface{}{
							"name": "John",
						},
					},
				},
			},
		},
	}

	// Mock schema with source info directives
	schema := CreateTestSchema(t)

	// Accumulate response with schema
	response := AccumulateResponseWithSchema(queryDoc, federatedResponse, schema)

	// Verify response structure
	assert.NotNil(t, response.Data)
	assert.Contains(t, response.Data, "personInfo")

	personInfo := response.Data["personInfo"].(map[string]interface{})
	assert.Equal(t, "John Doe", personInfo["fullName"])
	assert.Equal(t, "John", personInfo["name"])
	assert.Equal(t, "123 Main St", personInfo["address"])
}

func TestAccumulateResponse_ArrayField(t *testing.T) {
	// Test query with array field and @sourceInfo directives
	query := `
		query {
			personInfo(nic: "123456789V") {
				fullName @sourceInfo(providerKey: "drp", providerField: "person.fullName")
				ownedVehicles @sourceInfo(providerKey: "dmt", providerField: "vehicle.getVehicleInfos.data") {
					regNo @sourceInfo(providerKey: "dmt", providerField: "vehicle.getVehicleInfos.data.registrationNumber")
					make @sourceInfo(providerKey: "dmt", providerField: "vehicle.getVehicleInfos.data.make")
					model @sourceInfo(providerKey: "dmt", providerField: "vehicle.getVehicleInfos.data.model")
				}
			}
		}
	`

	queryDoc := ParseTestQuery(t, query)

	// Mock federated response with array data
	federatedResponse := &FederationResponse{
		Responses: []*ProviderResponse{
			{
				ServiceKey: "drp",
				Response: graphql.Response{
					Data: map[string]interface{}{
						"person": map[string]interface{}{
							"fullName": "John Doe",
						},
					},
				},
			},
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
										"model":              "Camry",
									},
									map[string]interface{}{
										"registrationNumber": "XYZ789",
										"make":               "Honda",
										"model":              "Civic",
									},
								},
							},
						},
					},
				},
			},
		},
	}

	// Mock schema with source info directives
	schema := CreateTestSchema(t)

	// Accumulate response with schema
	response := AccumulateResponseWithSchema(queryDoc, federatedResponse, schema)

	// Verify response structure
	assert.NotNil(t, response.Data)
	assert.Contains(t, response.Data, "personInfo")

	personInfo := response.Data["personInfo"].(map[string]interface{})
	assert.Equal(t, "John Doe", personInfo["fullName"])

	// Verify array field
	ownedVehicles := personInfo["ownedVehicles"].([]map[string]interface{})
	assert.Len(t, ownedVehicles, 2)

	// Verify first vehicle
	vehicle1 := ownedVehicles[0]
	assert.Equal(t, "ABC123", vehicle1["regNo"])
	assert.Equal(t, "Toyota", vehicle1["make"])
	assert.Equal(t, "Camry", vehicle1["model"])

	// Verify second vehicle
	vehicle2 := ownedVehicles[1]
	assert.Equal(t, "XYZ789", vehicle2["regNo"])
	assert.Equal(t, "Honda", vehicle2["make"])
	assert.Equal(t, "Civic", vehicle2["model"])
}

func TestPushValue_ArrayHandling(t *testing.T) {
	tests := []struct {
		name        string
		obj         interface{}
		path        string
		value       interface{}
		expected    interface{}
		expectError bool
		description string
	}{
		{
			name:        "Push to empty object",
			obj:         map[string]interface{}{},
			path:        "personInfo.fullName",
			value:       "John Doe",
			expected:    map[string]interface{}{"personInfo": map[string]interface{}{"fullName": "John Doe"}},
			expectError: false,
			description: "Should push value to empty object",
		},
		{
			name:        "Push to existing object",
			obj:         map[string]interface{}{"personInfo": map[string]interface{}{"name": "John"}},
			path:        "personInfo.fullName",
			value:       "John Doe",
			expected:    map[string]interface{}{"personInfo": map[string]interface{}{"name": "John", "fullName": "John Doe"}},
			expectError: false,
			description: "Should push value to existing object",
		},
		{
			name:        "Push to array field",
			obj:         map[string]interface{}{},
			path:        "personInfo.ownedVehicles",
			value:       []interface{}{map[string]interface{}{"regNo": "ABC123"}},
			expected:    map[string]interface{}{"personInfo": map[string]interface{}{"ownedVehicles": []interface{}{map[string]interface{}{"regNo": "ABC123"}}}},
			expectError: false,
			description: "Should push array value to object",
		},
		{
			name:        "Push to nested array",
			obj:         map[string]interface{}{"personInfo": map[string]interface{}{"ownedVehicles": []interface{}{}}},
			path:        "personInfo.ownedVehicles.regNo",
			value:       "ABC123",
			expected:    map[string]interface{}{"personInfo": map[string]interface{}{"ownedVehicles": []interface{}{}}},
			expectError: false,
			description: "Should push value to nested array (applies to all elements)",
		},
		{
			name:        "Push to bulk array",
			obj:         map[string]interface{}{},
			path:        "personInfos",
			value:       []interface{}{map[string]interface{}{"fullName": "John"}, map[string]interface{}{"fullName": "Jane"}},
			expected:    map[string]interface{}{"personInfos": []interface{}{map[string]interface{}{"fullName": "John"}, map[string]interface{}{"fullName": "Jane"}}},
			expectError: false,
			description: "Should push bulk array to object",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := PushValue(tt.obj, tt.path, tt.value)

			if tt.expectError {
				assert.Error(t, err, tt.description)
			} else {
				assert.NoError(t, err, tt.description)
				assert.Equal(t, tt.expected, result, tt.description)
			}
		})
	}
}

func TestGetValueAtPath_ArrayHandling(t *testing.T) {
	tests := []struct {
		name        string
		data        interface{}
		path        string
		expected    interface{}
		expectError bool
		description string
	}{
		{
			name: "Get value from object",
			data: map[string]interface{}{
				"person": map[string]interface{}{
					"fullName": "John Doe",
				},
			},
			path:        "person.fullName",
			expected:    "John Doe",
			expectError: false,
			description: "Should get value from object path",
		},
		{
			name: "Get value from array",
			data: map[string]interface{}{
				"vehicle": map[string]interface{}{
					"getVehicleInfos": map[string]interface{}{
						"data": []interface{}{
							map[string]interface{}{
								"registrationNumber": "ABC123",
								"make":               "Toyota",
							},
							map[string]interface{}{
								"registrationNumber": "XYZ789",
								"make":               "Honda",
							},
						},
					},
				},
			},
			path:        "vehicle.getVehicleInfos.data",
			expected:    []interface{}{map[string]interface{}{"registrationNumber": "ABC123", "make": "Toyota"}, map[string]interface{}{"registrationNumber": "XYZ789", "make": "Honda"}},
			expectError: false,
			description: "Should get array value from path",
		},
		{
			name: "Get value from array elements",
			data: map[string]interface{}{
				"persons": []interface{}{
					map[string]interface{}{
						"fullName": "John Doe",
					},
					map[string]interface{}{
						"fullName": "Jane Smith",
					},
				},
			},
			path:        "persons.fullName",
			expected:    []interface{}{"John Doe", "Jane Smith"},
			expectError: false,
			description: "Should get values from all array elements",
		},
		{
			name: "Get value from nested array elements",
			data: map[string]interface{}{
				"personInfo": map[string]interface{}{
					"ownedVehicles": []interface{}{
						map[string]interface{}{
							"regNo": "ABC123",
							"make":  "Toyota",
						},
					},
				},
			},
			path:        "personInfo.ownedVehicles.regNo",
			expected:    []interface{}{"ABC123"},
			expectError: false,
			description: "Should get values from all nested array elements",
		},
		{
			name: "Get non-existent key",
			data: map[string]interface{}{
				"person": map[string]interface{}{
					"name": "John",
				},
			},
			path:        "person.fullName",
			expected:    nil,
			expectError: true,
			description: "Should return error for non-existent key",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := GetValueAtPath(tt.data, tt.path)

			if tt.expectError {
				assert.Error(t, err, tt.description)
			} else {
				assert.NoError(t, err, tt.description)
				assert.Equal(t, tt.expected, result, tt.description)
			}
		})
	}
}

func TestAccumulateResponse_MixedObjectAndArray(t *testing.T) {
	// Test query with both object and array fields
	query := `
		query {
			personInfo(nic: "123456789V") {
				fullName
				address
				ownedVehicles {
					regNo
					make
					model
				}
			}
		}
	`

	queryDoc := ParseTestQuery(t, query)

	// Mock federated response with both object and array data
	federatedResponse := &FederationResponse{
		Responses: []*ProviderResponse{
			{
				ServiceKey: "drp",
				Response: graphql.Response{
					Data: map[string]interface{}{
						"person": map[string]interface{}{
							"fullName":         "John Doe",
							"permanentAddress": "123 Main St",
						},
					},
				},
			},
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
										"model":              "Camry",
									},
									map[string]interface{}{
										"registrationNumber": "XYZ789",
										"make":               "Honda",
										"model":              "Civic",
									},
								},
							},
						},
					},
				},
			},
		},
	}

	// Mock schema with source info directives
	schema := CreateTestSchema(t)

	// Accumulate response with schema
	response := AccumulateResponseWithSchema(queryDoc, federatedResponse, schema)

	// Verify response structure
	assert.NotNil(t, response.Data)

	// Verify personInfo object
	assert.Contains(t, response.Data, "personInfo")
	personInfo := response.Data["personInfo"].(map[string]interface{})
	assert.Equal(t, "John Doe", personInfo["fullName"])
	assert.Equal(t, "123 Main St", personInfo["address"])

	// Verify ownedVehicles array exists and has correct length
	assert.Contains(t, personInfo, "ownedVehicles")
	ownedVehicles, ok := personInfo["ownedVehicles"].([]interface{})
	if !ok {
		// Try alternate type that might be returned
		ownedVehiclesAlt, ok := personInfo["ownedVehicles"].([]map[string]interface{})
		assert.True(t, ok, "ownedVehicles should be an array")
		assert.Len(t, ownedVehiclesAlt, 2)

		// Verify vehicle details
		assert.Equal(t, "ABC123", ownedVehiclesAlt[0]["regNo"])
		assert.Equal(t, "Toyota", ownedVehiclesAlt[0]["make"])
		assert.Equal(t, "Camry", ownedVehiclesAlt[0]["model"])

		assert.Equal(t, "XYZ789", ownedVehiclesAlt[1]["regNo"])
		assert.Equal(t, "Honda", ownedVehiclesAlt[1]["make"])
		assert.Equal(t, "Civic", ownedVehiclesAlt[1]["model"])
		return
	}

	assert.Len(t, ownedVehicles, 2)

	// Verify vehicle details
	vehicle1 := ownedVehicles[0].(map[string]interface{})
	assert.Equal(t, "ABC123", vehicle1["regNo"])
	assert.Equal(t, "Toyota", vehicle1["make"])
	assert.Equal(t, "Camry", vehicle1["model"])

	vehicle2 := ownedVehicles[1].(map[string]interface{})
	assert.Equal(t, "XYZ789", vehicle2["regNo"])
	assert.Equal(t, "Honda", vehicle2["make"])
	assert.Equal(t, "Civic", vehicle2["model"])
}
