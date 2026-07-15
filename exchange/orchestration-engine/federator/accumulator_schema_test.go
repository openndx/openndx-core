package federator

import (
	"testing"

	"github.com/OpenNDX/openndx-core/exchange/orchestration-engine/pkg/graphql"
	"github.com/stretchr/testify/assert"
)

// TestAccumulateResponseWithSchema_ArrayField tests array field processing with schema
func TestAccumulateResponseWithSchema_ArrayField(t *testing.T) {
	query := `
		query {
			personInfo(nic: "123456789V") {
				ownedVehicles {
					regNo
					make
				}
			}
		}
	`

	queryDoc := ParseTestQuery(t, query)
	schema := CreateTestSchema(t)

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
									map[string]interface{}{
										"registrationNumber": "XYZ789",
										"make":               "Honda",
									},
								},
							},
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
	assert.Contains(t, personInfo, "ownedVehicles")

	ownedVehicles := personInfo["ownedVehicles"]
	assert.NotNil(t, ownedVehicles)

	// The array might be []interface{} or []map[string]interface{}
	ownedVehiclesArray, ok := ownedVehicles.([]interface{})
	if !ok {
		// Try as []map[string]interface{}
		ownedVehiclesMapArray, ok2 := ownedVehicles.([]map[string]interface{})
		if ok2 {
			assert.Len(t, ownedVehiclesMapArray, 2)
			assert.Equal(t, "ABC123", ownedVehiclesMapArray[0]["regNo"])
			assert.Equal(t, "Toyota", ownedVehiclesMapArray[0]["make"])
			assert.Equal(t, "XYZ789", ownedVehiclesMapArray[1]["regNo"])
			assert.Equal(t, "Honda", ownedVehiclesMapArray[1]["make"])
			return
		}
		t.Fatalf("Unexpected type for ownedVehicles: %T", ownedVehicles)
	}

	assert.Len(t, ownedVehiclesArray, 2)

	vehicle1 := ownedVehiclesArray[0].(map[string]interface{})
	assert.Equal(t, "ABC123", vehicle1["regNo"])
	assert.Equal(t, "Toyota", vehicle1["make"])

	vehicle2 := ownedVehiclesArray[1].(map[string]interface{})
	assert.Equal(t, "XYZ789", vehicle2["regNo"])
	assert.Equal(t, "Honda", vehicle2["make"])
}

// TestAccumulateResponseWithSchema_NestedArrayField tests nested array field processing
func TestAccumulateResponseWithSchema_NestedArrayField(t *testing.T) {
	query := `
		query {
			personInfo(nic: "123456789V") {
				class {
					className
				}
			}
		}
	`

	queryDoc := ParseTestQuery(t, query)
	schema := CreateTestSchema(t)

	federatedResponse := &FederationResponse{
		Responses: []*ProviderResponse{
			{
				ServiceKey: "dmt",
				Response: graphql.Response{
					Data: map[string]interface{}{
						"vehicle": map[string]interface{}{
							"getVehicleInfos": map[string]interface{}{
								"classes": []interface{}{
									map[string]interface{}{
										"className": "Sedan",
									},
									map[string]interface{}{
										"className": "SUV",
									},
								},
							},
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
	assert.Contains(t, personInfo, "class")

	classes := personInfo["class"]
	assert.NotNil(t, classes)

	// Handle both []interface{} and []map[string]interface{}
	classesArray, ok := classes.([]interface{})
	if !ok {
		classesMapArray, ok2 := classes.([]map[string]interface{})
		if ok2 {
			assert.Len(t, classesMapArray, 2)
			assert.Equal(t, "Sedan", classesMapArray[0]["className"])
			assert.Equal(t, "SUV", classesMapArray[1]["className"])
			return
		}
		t.Fatalf("Unexpected type for classes: %T", classes)
	}

	assert.Len(t, classesArray, 2)
	class1 := classesArray[0].(map[string]interface{})
	assert.Equal(t, "Sedan", class1["className"])
	class2 := classesArray[1].(map[string]interface{})
	assert.Equal(t, "SUV", class2["className"])
}

// TestAccumulateResponseWithSchema_ArrayFieldWithoutSelectionSet tests array field without nested selections
func TestAccumulateResponseWithSchema_ArrayFieldWithoutSelectionSet(t *testing.T) {
	query := `
		query {
			personInfo(nic: "123456789V") {
				ownedVehicles
			}
		}
	`

	queryDoc := ParseTestQuery(t, query)
	schema := CreateTestSchema(t)

	federatedResponse := &FederationResponse{
		Responses: []*ProviderResponse{
			{
				ServiceKey: "dmt",
				Response: graphql.Response{
					Data: map[string]interface{}{
						"vehicle": map[string]interface{}{
							"getVehicleInfos": map[string]interface{}{
								"data": []interface{}{
									map[string]interface{}{"regNo": "ABC123"},
									map[string]interface{}{"regNo": "XYZ789"},
								},
							},
						},
					},
				},
			},
		},
	}

	response := AccumulateResponseWithSchema(queryDoc, federatedResponse, schema)

	assert.NotNil(t, response.Data)
	assert.Contains(t, response.Data, "personInfo")
}

// TestAccumulateResponseWithSchema_ProviderNotFound tests when provider response is not found
func TestAccumulateResponseWithSchema_ProviderNotFound(t *testing.T) {
	query := `
		query {
			personInfo(nic: "123456789V") {
				fullName @sourceInfo(providerKey: "nonexistent", providerField: "person.fullName")
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

	assert.NotNil(t, response.Data)
	// Should not crash even when provider is not found
}

// TestAccumulateResponseWithSchema_ValueNotFound tests when value path doesn't exist in provider response
func TestAccumulateResponseWithSchema_ValueNotFound(t *testing.T) {
	query := `
		query {
			personInfo(nic: "123456789V") {
				fullName @sourceInfo(providerKey: "drp", providerField: "person.nonexistent")
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

	assert.NotNil(t, response.Data)
	// Should not crash even when value path doesn't exist
}

// TestAccumulateResponseWithSchema_NilSchema tests behavior with nil schema
func TestAccumulateResponseWithSchema_NilSchema(t *testing.T) {
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

	response := AccumulateResponseWithSchema(queryDoc, federatedResponse, nil)

	assert.NotNil(t, response.Data)
	// Should fallback to directive-based extraction when schema is nil
	assert.Contains(t, response.Data, "personInfo")
}

// TestAccumulateResponseWithSchema_MixedSimpleAndArrayFields tests mixing simple and array fields
func TestAccumulateResponseWithSchema_MixedSimpleAndArrayFields(t *testing.T) {
	query := `
		query {
			personInfo(nic: "123456789V") {
				fullName
				ownedVehicles {
					regNo
				}
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
				ServiceKey: "dmt",
				Response: graphql.Response{
					Data: map[string]interface{}{
						"vehicle": map[string]interface{}{
							"getVehicleInfos": map[string]interface{}{
								"data": []interface{}{
									map[string]interface{}{
										"registrationNumber": "ABC123",
									},
								},
							},
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
	assert.Contains(t, personInfo, "fullName")
	assert.Contains(t, personInfo, "ownedVehicles")
}

// TestAccumulateResponseWithSchema_EmptyArray tests empty array handling
func TestAccumulateResponseWithSchema_EmptyArray(t *testing.T) {
	query := `
		query {
			personInfo(nic: "123456789V") {
				ownedVehicles {
					regNo
				}
			}
		}
	`

	queryDoc := ParseTestQuery(t, query)
	schema := CreateTestSchema(t)

	federatedResponse := &FederationResponse{
		Responses: []*ProviderResponse{
			{
				ServiceKey: "dmt",
				Response: graphql.Response{
					Data: map[string]interface{}{
						"vehicle": map[string]interface{}{
							"getVehicleInfos": map[string]interface{}{
								"data": []interface{}{},
							},
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
	assert.Contains(t, personInfo, "ownedVehicles")

	ownedVehicles := personInfo["ownedVehicles"]
	assert.NotNil(t, ownedVehicles)

	ownedVehiclesArray, ok := ownedVehicles.([]interface{})
	if !ok {
		ownedVehiclesMapArray, ok2 := ownedVehicles.([]map[string]interface{})
		if ok2 {
			assert.Len(t, ownedVehiclesMapArray, 0)
			return
		}
		t.Fatalf("Unexpected type for ownedVehicles: %T", ownedVehicles)
	}

	assert.Len(t, ownedVehiclesArray, 0)
}
