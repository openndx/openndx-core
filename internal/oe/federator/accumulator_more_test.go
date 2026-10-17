package federator

import (
	"testing"

	"github.com/openndx/openndx-core/internal/oe/logger"
	"github.com/stretchr/testify/assert"
)

func init() {
	logger.Init()
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
