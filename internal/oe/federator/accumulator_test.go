package federator

import (
	"testing"

	"github.com/openndx/openndx-core/internal/oe/logger"
	"github.com/stretchr/testify/assert"
)

func init() {
	logger.Init()
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
