package federator

import (
	"fmt"
	"strings"

	"github.com/graphql-go/graphql/language/ast"
	"github.com/openndx/openndx-core/internal/oe/logger"
	"github.com/openndx/openndx-core/internal/oe/pkg/graphql"
)

// AccumulateResponseWithSchemaInfo uses schema information for array-aware processing
func AccumulateResponseWithSchemaInfo(queryAST *ast.Document, federatedResponse *FederationResponse, schemaInfoMap map[string]*SourceSchemaInfo) graphql.Response {
	responseData := make(map[string]interface{})

	// Process each field in the schema info map
	for fieldPath, schemaInfo := range schemaInfoMap {
		if schemaInfo.IsArray {
			// Handle array fields with object-by-object processing
			err := accumulateArrayResponse(responseData, fieldPath, schemaInfo, federatedResponse)
			if err != nil {
				logger.Log.Error("Error processing array field", "path", fieldPath, "error", err)
			}
		} else {
			// Handle regular fields
			response := federatedResponse.GetProviderResponse(schemaInfo.ProviderKey)
			if response != nil {
				value, err := GetValueAtPath(response.Response.Data, schemaInfo.ProviderField)
				if err == nil {
					_, err = PushValue(responseData, fieldPath, value)
				} else {
					logger.Log.Error("Error getting value", "path", schemaInfo.ProviderField, "error", err)
				}
			}
		}
	}

	return graphql.Response{
		Data: responseData,
	}
}

// accumulateArrayResponse handles the logic for building an array of objects from a provider response
func accumulateArrayResponse(
	destination map[string]interface{},
	fieldPath string, // e.g., "personInfo.ownedVehicles"
	fieldSchemaInfo *SourceSchemaInfo, // The schema info for the 'ownedVehicles' field
	federatedResponse *FederationResponse,
) error {
	// 1. Get the provider response
	response := federatedResponse.GetProviderResponse(fieldSchemaInfo.ProviderKey)
	if response == nil {
		return fmt.Errorf("no response found for provider %s", fieldSchemaInfo.ProviderKey)
	}

	// 2. Extract the entire source array from the provider response
	// Uses the ProviderArrayFieldPath from the schema info.
	sourceArrayInterface, err := GetValueAtPath(response.Response.Data, fieldSchemaInfo.ProviderArrayFieldPath)
	if err != nil {
		// Handle cases where the path doesn't exist gracefully
		return fmt.Errorf("source array path not found: %s", fieldSchemaInfo.ProviderArrayFieldPath)
	}

	sourceArray, ok := sourceArrayInterface.([]interface{})
	if !ok {
		// The data at the path was not an array, which is an error
		return fmt.Errorf("expected an array at path %s but got %T", fieldSchemaInfo.ProviderArrayFieldPath, sourceArrayInterface)
	}

	// 3. Create the destination array that we will populate
	destinationArray := make([]map[string]interface{}, 0, len(sourceArray))

	// 4. Iterate over each item in the source array
	for _, sourceItemInterface := range sourceArray {
		sourceItem, _ := sourceItemInterface.(map[string]interface{})

		// 5. Create a new destination object for each source item
		destinationObject := make(map[string]interface{})

		// 6. Populate the destination object using the sub-field mappings
		for consumerFieldName, subFieldInfo := range fieldSchemaInfo.SubFieldSchemaInfos {
			// The provider field path (e.g., "registrationNumber") is relative to the source item
			value, err := GetValueAtPath(sourceItem, subFieldInfo.ProviderField)
			if err == nil {
				// Use the final part of the consumer field name as the key (e.g., "regNo")
				keyParts := strings.Split(consumerFieldName, ".")
				key := keyParts[len(keyParts)-1]
				destinationObject[key] = value
			} else {
				// Field not found in source item, skip it silently
			}
		}
		destinationArray = append(destinationArray, destinationObject)
	}

	// 7. Push the completed destination array into the final response structure
	_, err = PushValue(destination, fieldPath, destinationArray)
	return err
}

// PushValue pushes a value into a JSON-like structure (map[string]interface{} / []interface{})
// using a dot-notation path. If a segment already points to an array, the value is appended to all items.
func PushValue(obj interface{}, path string, value interface{}) (interface{}, error) {
	keys := strings.Split(path, ".")
	return pushRecursive(obj, keys, value)
}

func pushRecursive(obj interface{}, keys []string, value interface{}) (interface{}, error) {
	if len(keys) == 0 {
		return value, nil
	}

	key := keys[0]

	switch curr := obj.(type) {
	case map[string]interface{}:
		child, ok := curr[key]
		if !ok {
			// If more keys → create map, else assign value
			if len(keys) > 1 {
				child = map[string]interface{}{}
			} else {
				curr[key] = value
				return curr, nil
			}
		}

		newChild, err := pushRecursive(child, keys[1:], value)
		if err != nil {
			return nil, err
		}
		curr[key] = newChild
		return curr, nil

	case []interface{}:
		// For arrays: apply pushRecursive to all elements
		newArr := make([]interface{}, len(curr))
		for i, elem := range curr {
			newChild, err := pushRecursive(elem, keys, value)
			if err != nil {
				return nil, err
			}
			newArr[i] = newChild
		}
		return newArr, nil

	case nil:
		// Initialize a map if nil
		child := map[string]interface{}{}
		newChild, err := pushRecursive(child, keys, value)
		if err != nil {
			return nil, err
		}
		return newChild, nil

	default:
		return nil, fmt.Errorf("unexpected type %T at key %q", obj, key)
	}
}

func GetValueAtPath(data interface{}, path string) (interface{}, error) {
	keys := strings.Split(path, ".")
	return getValueRecursive(data, keys)
}

func getValueRecursive(data interface{}, keys []string) (interface{}, error) {
	if len(keys) == 0 {
		return data, nil
	}

	key := keys[0]

	switch curr := data.(type) {
	case map[string]interface{}:
		child, ok := curr[key]
		if !ok {
			return nil, fmt.Errorf("key %q not found", key)
		}
		return getValueRecursive(child, keys[1:])

	case []interface{}:
		// For arrays: apply getValueRecursive to all elements
		var results []interface{}
		for _, elem := range curr {
			childValue, err := getValueRecursive(elem, keys)
			if err != nil {
				return nil, err
			}
			results = append(results, childValue)
		}
		return results, nil

	default:
		return nil, fmt.Errorf("unexpected type %T at key %q", data, key)
	}
}
