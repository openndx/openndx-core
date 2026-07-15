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

func TestAccumulateResponse_SimpleBackwardCompatibility(t *testing.T) {
	// Test AccumulateResponse (simple version without schema)
	query := `
		query {
			personInfo(nic: "123456789V") {
				fullName @sourceInfo(providerKey: "drp", providerField: "person.fullName")
			}
		}
	`

	queryDoc := ParseTestQuery(t, query)

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
		},
	}

	response := AccumulateResponse(queryDoc, federatedResponse)

	assert.NotNil(t, response.Data)
	assert.Contains(t, response.Data, "personInfo")
}

func TestPushArrayValue(t *testing.T) {
	tests := []struct {
		name        string
		obj         interface{}
		path        string
		value       interface{}
		expectError bool
	}{
		{
			name:        "Push array to empty object",
			obj:         map[string]interface{}{},
			path:        "personInfo.vehicles",
			value:       []interface{}{map[string]interface{}{"regNo": "ABC123"}},
			expectError: false,
		},
		{
			name:        "Push array to existing path",
			obj:         map[string]interface{}{"personInfo": map[string]interface{}{}},
			path:        "personInfo.vehicles",
			value:       []interface{}{map[string]interface{}{"regNo": "XYZ789"}},
			expectError: false,
		},
		{
			name:        "Push to array elements",
			obj:         map[string]interface{}{"vehicles": []interface{}{map[string]interface{}{}}},
			path:        "vehicles.regNo",
			value:       "ABC123",
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := PushArrayValue(tt.obj, tt.path, tt.value)
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
			}
		})
	}
}

func TestPushValue_WithNil(t *testing.T) {
	// Test PushValue with nil object
	result, err := PushValue(nil, "test.path", "value")
	assert.NoError(t, err)
	assert.NotNil(t, result)

	resultMap := result.(map[string]interface{})
	assert.Contains(t, resultMap, "test")
}

func TestPushValue_DeepNesting(t *testing.T) {
	obj := map[string]interface{}{}

	result, err := PushValue(obj, "level1.level2.level3.value", "deep")
	assert.NoError(t, err)

	resultMap := result.(map[string]interface{})
	level1 := resultMap["level1"].(map[string]interface{})
	level2 := level1["level2"].(map[string]interface{})
	level3 := level2["level3"].(map[string]interface{})
	assert.Equal(t, "deep", level3["value"])
}

func TestGetValueAtPath_DeepNesting(t *testing.T) {
	data := map[string]interface{}{
		"level1": map[string]interface{}{
			"level2": map[string]interface{}{
				"level3": map[string]interface{}{
					"value": "deep",
				},
			},
		},
	}

	result, err := GetValueAtPath(data, "level1.level2.level3.value")
	assert.NoError(t, err)
	assert.Equal(t, "deep", result)
}

func TestGetValueAtPath_ArrayWithNestedPath(t *testing.T) {
	data := map[string]interface{}{
		"vehicles": []interface{}{
			map[string]interface{}{
				"info": map[string]interface{}{
					"regNo": "ABC123",
				},
			},
			map[string]interface{}{
				"info": map[string]interface{}{
					"regNo": "XYZ789",
				},
			},
		},
	}

	result, err := GetValueAtPath(data, "vehicles.info.regNo")
	assert.NoError(t, err)

	regNos := result.([]interface{})
	assert.Len(t, regNos, 2)
	assert.Contains(t, regNos, "ABC123")
	assert.Contains(t, regNos, "XYZ789")
}

func TestAccumulateResponseWithSchema_NoSourceInfo(t *testing.T) {
	// Test with query that has no @sourceInfo directives
	query := `
		query {
			personInfo(nic: "123456789V") {
				fullName
			}
		}
	`

	queryDoc := ParseTestQuery(t, query)
	schema := CreateTestSchema(t)

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
		},
	}

	response := AccumulateResponseWithSchema(queryDoc, federatedResponse, schema)

	// Should still return response, but may not have data if no sourceInfo
	assert.NotNil(t, response.Data)
}

func TestAccumulateResponseWithSchema_EmptyResponse(t *testing.T) {
	query := `
		query {
			personInfo(nic: "123456789V") {
				fullName @sourceInfo(providerKey: "drp", providerField: "person.fullName")
			}
		}
	`

	queryDoc := ParseTestQuery(t, query)
	schema := CreateTestSchema(t)

	federatedResponse := &FederationResponse{
		Responses: []*ProviderResponse{},
	}

	response := AccumulateResponseWithSchema(queryDoc, federatedResponse, schema)
	assert.NotNil(t, response.Data)
}

func TestAccumulateResponseWithSchema_MultipleProviders(t *testing.T) {
	query := `
		query {
			personInfo(nic: "123456789V") {
				fullName @sourceInfo(providerKey: "drp", providerField: "person.fullName")
				name @sourceInfo(providerKey: "rgd", providerField: "getPersonInfo.name")
			}
		}
	`

	queryDoc := ParseTestQuery(t, query)
	schema := CreateTestSchema(t)

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

	response := AccumulateResponseWithSchema(queryDoc, federatedResponse, schema)

	assert.NotNil(t, response.Data)
	assert.Contains(t, response.Data, "personInfo")

	personInfo := response.Data["personInfo"].(map[string]interface{})
	assert.Equal(t, "John Doe", personInfo["fullName"])
	assert.Equal(t, "John", personInfo["name"])
}

func TestPushValue_OverwriteExisting(t *testing.T) {
	obj := map[string]interface{}{
		"personInfo": map[string]interface{}{
			"fullName": "Old Name",
		},
	}

	result, err := PushValue(obj, "personInfo.fullName", "New Name")
	assert.NoError(t, err)

	resultMap := result.(map[string]interface{})
	personInfo := resultMap["personInfo"].(map[string]interface{})
	assert.Equal(t, "New Name", personInfo["fullName"])
}

func TestPushValue_ArrayElement(t *testing.T) {
	obj := map[string]interface{}{
		"vehicles": []interface{}{
			map[string]interface{}{"regNo": "ABC123"},
			map[string]interface{}{"regNo": "XYZ789"},
		},
	}

	// Push to all array elements
	result, err := PushValue(obj, "vehicles.make", "Toyota")
	assert.NoError(t, err)

	resultMap := result.(map[string]interface{})
	vehicles := resultMap["vehicles"].([]interface{})
	assert.Len(t, vehicles, 2)

	for _, v := range vehicles {
		vehicle := v.(map[string]interface{})
		assert.Equal(t, "Toyota", vehicle["make"])
	}
}
