package federator

import (
	"fmt"
	"strings"

	"github.com/OpenNDX/openndx-core/exchange/orchestration-engine/logger"
	"github.com/OpenNDX/openndx-core/exchange/orchestration-engine/pkg/federator"
	"github.com/OpenNDX/openndx-core/exchange/orchestration-engine/pkg/graphql"
	"github.com/graphql-go/graphql/language/ast"
	"github.com/graphql-go/graphql/language/visitor"
)

func AccumulateResponse(queryAST *ast.Document, federatedResponse *FederationResponse) graphql.Response {
	// Use the simple accumulator for backward compatibility
	// For array-aware processing, use AccumulateResponseWithSchemaInfo instead
	return accumulateResponseSimple(queryAST, federatedResponse)
}

// AccumulateResponseWithSchema uses schema for directive resolution
func AccumulateResponseWithSchema(queryAST *ast.Document, federatedResponse *FederationResponse, schema *ast.Document) graphql.Response {
	responseData := make(map[string]interface{})
	path := make([]string, 0)
	isTopLevel := true

	visitor.Visit(queryAST, &visitor.VisitorOptions{
		Enter: func(p visitor.VisitFuncParams) (string, interface{}) {
			if node, ok := p.Node.(*ast.Field); ok {
				fieldName := node.Name.Value

				// Handle top-level query fields
				if isTopLevel {
					responseData[fieldName] = make(map[string]interface{})
					path = append(path, fieldName)
					isTopLevel = false
					return visitor.ActionNoChange, p.Node
				}

				// Handle nested fields
				if len(path) > 0 {
					// Only skip processing if this is truly a nested field of an array that's already been processed
					// Don't skip array fields themselves or their direct children
					isNested := isNestedFieldOfArray(path, queryAST)
					isArray := isArrayField(node)
					if isNested && !isArray {
						return visitor.ActionNoChange, p.Node
					}

					// Add current field to path for nested processing
					currentPath := append(path, fieldName)

					// Use schema to extract source info
					fieldPath := strings.Join(currentPath, ".")
					var providerInfo *federator.SourceInfo
					if schema != nil {
						providerInfo = federator.ExtractSourceInfoFromSchema(schema, fieldPath)
						if providerInfo == nil {
							// Try to find the field in the correct type context for nested array fields
							// For nested array fields like className within class array, look in VehicleClass type
							if len(currentPath) > 1 && currentPath[len(currentPath)-2] == "class" {
								// We're processing a field within the class array, so look in VehicleClass type
								nestedFieldPath := "VehicleClass." + fieldName
								providerInfo = federator.ExtractSourceInfoFromSchema(schema, nestedFieldPath)
							}
						}
					}

					// Fallback to old method if schema method fails
					if providerInfo == nil {
						providerInfo = federator.ExtractSourceInfoFromDirective(node)
					}

					if providerInfo != nil {
						response := federatedResponse.GetProviderResponse(providerInfo.ProviderKey)
						if response != nil {
							value, err := GetValueAtPath(response.Response.Data, providerInfo.ProviderField)
							if err == nil {
								// Check if this is an array field by looking at the data type and schema
								if isArrayFieldValue(fieldName, value) {
									logger.Log.Debug("Processing as array field", "fieldName", fieldName)
									// This is an array field with nested selections
									if node.SelectionSet != nil && len(node.SelectionSet.Selections) > 0 {
										processArrayFieldSimple(responseData, currentPath, fieldName, value, node.SelectionSet, federatedResponse, schema)
									} else {
										// Array field without nested selections - just add the array
										fullPath := strings.Join(currentPath, ".")
										_, err = PushValue(responseData, fullPath, value)
										if err != nil {
											logger.Log.Error("Error pushing array value", "path", fullPath, "error", err)
										}
									}
									// Update path for array fields
									path = append(path, fieldName)
								} else {
									logger.Log.Debug("Processing as simple/object field", "fieldName", fieldName)
									// Simple field or object field
									fullPath := strings.Join(currentPath, ".")
									_, err = PushValue(responseData, fullPath, value)
									if err != nil {
										logger.Log.Error("Error pushing value", "path", fullPath, "error", err)
									}
									// Update path for simple fields
									path = append(path, fieldName)
								}
							} else {
								logger.Log.Error("Error getting value", "path", providerInfo.ProviderField, "error", err)
							}
						}
					} else {
						logger.Log.Warn("No @sourceInfo directive found for field", "fieldName", fieldName, "path", strings.Join(currentPath, "."))
					}
				}
			}
			return visitor.ActionNoChange, p.Node
		},
		Leave: func(p visitor.VisitFuncParams) (string, interface{}) {
			if node, ok := p.Node.(*ast.Field); ok {
				// Remove the current field from the path when leaving
				if len(path) > 0 && path[len(path)-1] == node.Name.Value {
					path = path[:len(path)-1]
				}
				// Reset to top level if we're back at the root
				if len(path) == 0 {
					isTopLevel = true
				}
			}
			return visitor.ActionNoChange, p.Node
		},
	}, nil)

	return graphql.Response{Data: responseData}
}

// accumulateResponseSimple is the fallback simple accumulator
func accumulateResponseSimple(queryAST *ast.Document, federatedResponse *FederationResponse) graphql.Response {
	responseData := make(map[string]interface{})
	path := make([]string, 0)
	isTopLevel := true

	visitor.Visit(queryAST, &visitor.VisitorOptions{
		Enter: func(p visitor.VisitFuncParams) (string, interface{}) {
			if node, ok := p.Node.(*ast.Field); ok {
				fieldName := node.Name.Value

				// Handle top-level query fields
				if isTopLevel {
					responseData[fieldName] = make(map[string]interface{})
					path = append(path, fieldName)
					isTopLevel = false
					return visitor.ActionNoChange, p.Node
				}

				// Handle nested fields
				if len(path) > 0 {
					// Only skip processing if this is truly a nested field of an array that's already been processed
					// Don't skip array fields themselves or their direct children
					if isNestedFieldOfArray(path, queryAST) && !isArrayField(node) {
						return visitor.ActionNoChange, p.Node
					}

					providerInfo := federator.ExtractSourceInfoFromDirective(node)
					if providerInfo != nil {
						response := federatedResponse.GetProviderResponse(providerInfo.ProviderKey)
						if response != nil {
							value, err := GetValueAtPath(response.Response.Data, providerInfo.ProviderField)
							if err == nil {
								logger.Log.Debug("Processing field", "fieldName", fieldName, "path", path, "valueType", fmt.Sprintf("%T", value), "hasSelectionSet", node.SelectionSet != nil && len(node.SelectionSet.Selections) > 0)
								// Check if this is an array field by looking at the selection set and data type
								if node.SelectionSet != nil && len(node.SelectionSet.Selections) > 0 && isArrayFieldValue(fieldName, value) {
									logger.Log.Debug("Processing as array field", "fieldName", fieldName)
									// This is an array field with nested selections
									processArrayFieldSimple(responseData, append(path, fieldName), fieldName, value, node.SelectionSet, federatedResponse, nil)
								} else {
									logger.Log.Debug("Processing as simple/object field", "fieldName", fieldName)
									// Simple field or object field
									fullPath := strings.Join(append(path, fieldName), ".")
									_, err = PushValue(responseData, fullPath, value)
									if err != nil {
										logger.Log.Error("Error pushing value", "path", fullPath, "error", err)
									}
								}
							} else {
								logger.Log.Error("Error getting value", "path", providerInfo.ProviderField, "error", err)
							}
						}
					} else {
						logger.Log.Warn("No @sourceInfo directive found for field", "fieldName", fieldName, "path", strings.Join(append(path, fieldName), "."))
					}
				}
				path = append(path, fieldName)
			}
			return visitor.ActionNoChange, p.Node
		},
		Leave: func(p visitor.VisitFuncParams) (string, interface{}) {
			if node, ok := p.Node.(*ast.Field); ok {
				fieldName := node.Name.Value
				if len(path) > 0 && path[len(path)-1] == fieldName {
					path = path[:len(path)-1]
				}
				if len(path) == 0 {
					isTopLevel = true
				}
			}
			return visitor.ActionNoChange, p.Node
		},
	}, nil)

	return graphql.Response{Data: responseData}
}

// accumulateResponseWithSchema uses schema info to handle arrays properly
func accumulateResponseWithSchema(queryAST *ast.Document, federatedResponse *FederationResponse, schemaInfoMap map[string]*SourceSchemaInfo) graphql.Response {
	responseData := make(map[string]interface{})
	path := make([]string, 0)
	isTopLevel := true

	visitor.Visit(queryAST, &visitor.VisitorOptions{
		Enter: func(p visitor.VisitFuncParams) (string, interface{}) {
			if node, ok := p.Node.(*ast.Field); ok {
				fieldName := node.Name.Value

				// Handle top-level query fields
				if isTopLevel {
					responseData[fieldName] = make(map[string]interface{})
					path = append(path, fieldName)
					isTopLevel = false
					return visitor.ActionNoChange, p.Node
				}

				// Handle nested fields
				if len(path) > 0 {
					fullPath := strings.Join(append(path, fieldName), ".")
					schemaInfo, exists := schemaInfoMap[fullPath]

					if exists {
						if schemaInfo.IsArray {
							// Handle array field
							processArrayFieldWithSchema(responseData, path, fieldName, schemaInfo, federatedResponse)
						} else {
							// Handle simple field
							processSimpleField(responseData, path, fieldName, schemaInfo, federatedResponse)
						}
					} else {
						logger.Log.Warn("No schema info found for field", "fieldName", fieldName, "path", fullPath)
					}
				}
				path = append(path, fieldName)
			}
			return visitor.ActionNoChange, p.Node
		},
		Leave: func(p visitor.VisitFuncParams) (string, interface{}) {
			if node, ok := p.Node.(*ast.Field); ok {
				fieldName := node.Name.Value
				if len(path) > 0 && path[len(path)-1] == fieldName {
					path = path[:len(path)-1]
				}
				if len(path) == 0 {
					isTopLevel = true
				}
			}
			return visitor.ActionNoChange, p.Node
		},
	}, nil)

	return graphql.Response{Data: responseData}
}

// isArrayField checks if the current field is an array field by looking at its selection set
func isArrayField(field *ast.Field) bool {
	return field.SelectionSet != nil && len(field.SelectionSet.Selections) > 0
}

// isNestedFieldOfArray checks if the current field is a nested field of an array
// by examining the query AST to determine which fields are arrays
func isNestedFieldOfArray(path []string, queryAST *ast.Document) bool {
	if len(path) < 2 {
		return false
	}

	// Get the parent field (the array field) from the path
	arrayFieldName := path[len(path)-2]

	// Find the array field in the query AST
	arrayField := findFieldInQuery(queryAST, arrayFieldName)
	if arrayField == nil {
		return false
	}

	// Check if this field has a selection set (indicating it's an array/object field)
	// and if we're currently processing a nested field of it
	return arrayField.SelectionSet != nil && len(arrayField.SelectionSet.Selections) > 0
}

// findFieldInQuery recursively searches for a field with the given name in the query AST
func findFieldInQuery(queryAST *ast.Document, fieldName string) *ast.Field {
	for _, definition := range queryAST.Definitions {
		if operation, ok := definition.(*ast.OperationDefinition); ok {
			if operation.SelectionSet != nil {
				for _, selection := range operation.SelectionSet.Selections {
					if field, ok := selection.(*ast.Field); ok {
						if field.Name != nil && field.Name.Value == fieldName {
							return field
						}
						// Recursively search in nested selections
						if found := findFieldInSelectionSet(field.SelectionSet, fieldName); found != nil {
							return found
						}
					}
				}
			}
		}
	}
	return nil
}

// findFieldInSelectionSet recursively searches for a field in a selection set
func findFieldInSelectionSet(selectionSet *ast.SelectionSet, fieldName string) *ast.Field {
	if selectionSet == nil {
		return nil
	}

	for _, selection := range selectionSet.Selections {
		if field, ok := selection.(*ast.Field); ok {
			if field.Name != nil && field.Name.Value == fieldName {
				return field
			}
			// Recursively search in nested selections
			if found := findFieldInSelectionSet(field.SelectionSet, fieldName); found != nil {
				return found
			}
		}
	}
	return nil
}

// processArrayFieldSimple handles array fields with nested selections
func processArrayFieldSimple(responseData map[string]interface{}, path []string, fieldName string, sourceArray interface{}, selectionSet *ast.SelectionSet, federatedResponse *FederationResponse, schema *ast.Document) {
	// Convert source array to []interface{}
	var arrayData []interface{}
	if arr, ok := sourceArray.([]interface{}); ok {
		arrayData = arr
	} else {
		logger.Log.Warn("Expected array at field", "fieldName", fieldName, "gotType", fmt.Sprintf("%T", sourceArray))
		return
	}

	// Create destination array
	destinationArray := make([]map[string]interface{}, 0, len(arrayData))

	// Process each item in the source array
	for _, sourceItem := range arrayData {
		if sourceItemMap, ok := sourceItem.(map[string]interface{}); ok {
			// Create destination object
			destinationObject := make(map[string]interface{})

			// Process each nested field in the selection set
			for _, selection := range selectionSet.Selections {
				if nestedField, ok := selection.(*ast.Field); ok {
					nestedFieldName := nestedField.Name.Value

					// Get the @sourceInfo directive for the nested field from schema
					// For nested fields within arrays, look in the correct type context
					var nestedProviderInfo *federator.SourceInfo
					if schema != nil {
						// Try to find the field in the correct type context for nested array fields
						// First, try to determine the array element type from the schema
						// We need to find the parent type that contains this array field
						arrayElementTypeName := ""

						// Try to find the array field in the schema to get its element type
						for _, def := range schema.Definitions {
							if objType, ok := def.(*ast.ObjectDefinition); ok {
								for _, field := range objType.Fields {
									if field.Name.Value == fieldName {
										// Check if this is an array type (List type)
										if listType, ok := field.Type.(*ast.List); ok {
											// Get the element type from the list
											if namedType, ok := listType.Type.(*ast.Named); ok {
												arrayElementTypeName = namedType.Name.Value
												break
											}
										}
									}
								}
								if arrayElementTypeName != "" {
									break
								}
							}
						}

						if arrayElementTypeName != "" {
							nestedFieldPath := arrayElementTypeName + "." + nestedFieldName
							nestedProviderInfo = federator.ExtractSourceInfoFromSchema(schema, nestedFieldPath)
						}
					}

					// Fallback to old method if schema method fails
					if nestedProviderInfo == nil {
						nestedProviderInfo = federator.ExtractSourceInfoFromDirective(nestedField)
					}

					if nestedProviderInfo != nil {
						// Extract the relative field path from the full provider field path
						relativeFieldPath := extractRelativeFieldPath(nestedProviderInfo.ProviderField)

						// Get value from source item using relative field path
						value, err := GetValueAtPath(sourceItemMap, relativeFieldPath)
						if err == nil {
							destinationObject[nestedFieldName] = value
						} else {
							logger.Log.Error("Error getting sub-field", "path", relativeFieldPath, "error", err)
						}
					} else {
						logger.Log.Warn("No @sourceInfo directive found for nested field", "fieldName", nestedFieldName)
					}
				}
			}

			destinationArray = append(destinationArray, destinationObject)
		}
	}

	// Add the completed array to the response
	fullPath := strings.Join(path, ".")
	_, err := PushValue(responseData, fullPath, destinationArray)
	if err != nil {
		logger.Log.Error("Error pushing array", "path", fullPath, "error", err)
	}
}

// extractRelativeFieldPath extracts the relative field path from a full provider field path
func extractRelativeFieldPath(providerField string) string {
	// For paths like "vehicle.getVehicleInfos.data.registrationNumber",
	// extract just "registrationNumber"
	parts := strings.Split(providerField, ".")
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}
	return providerField
}

// processSimpleField handles simple (non-array) fields
func processSimpleField(responseData map[string]interface{}, path []string, fieldName string, schemaInfo *SourceSchemaInfo, federatedResponse *FederationResponse) {
	response := federatedResponse.GetProviderResponse(schemaInfo.ProviderKey)
	if response != nil {
		value, err := GetValueAtPath(response.Response.Data, schemaInfo.ProviderField)
		if err == nil {
			fullPath := strings.Join(append(path, fieldName), ".")
			_, err = PushValue(responseData, fullPath, value)
			if err != nil {
				logger.Log.Error("Error pushing value", "path", fullPath, "error", err)
			}
		} else {
			logger.Log.Error("Error getting value", "path", schemaInfo.ProviderField, "error", err)
		}
	}
}

// processArrayFieldWithSchema handles array fields using schema information
func processArrayFieldWithSchema(responseData map[string]interface{}, path []string, fieldName string, schemaInfo *SourceSchemaInfo, federatedResponse *FederationResponse) {
	response := federatedResponse.GetProviderResponse(schemaInfo.ProviderKey)
	if response == nil {
		logger.Log.Warn("No response found for provider", "providerKey", schemaInfo.ProviderKey)
		return
	}

	// Get the source array from the provider response
	sourceArray, err := GetValueAtPath(response.Response.Data, schemaInfo.ProviderArrayFieldPath)
	if err != nil {
		logger.Log.Error("Error getting array", "path", schemaInfo.ProviderArrayFieldPath, "error", err)
		return
	}

	// Convert to array if it's not already
	var arrayData []interface{}
	if arr, ok := sourceArray.([]interface{}); ok {
		arrayData = arr
	} else {
		logger.Log.Warn("Expected array at path", "path", schemaInfo.ProviderArrayFieldPath, "gotType", fmt.Sprintf("%T", sourceArray))
		return
	}

	// Create destination array
	destinationArray := make([]map[string]interface{}, 0, len(arrayData))

	// Process each item in the source array
	for _, sourceItem := range arrayData {
		if sourceItemMap, ok := sourceItem.(map[string]interface{}); ok {
			// Create destination object
			destinationObject := make(map[string]interface{})

			// Process each sub-field
			for subFieldName, subFieldSchemaInfo := range schemaInfo.SubFieldSchemaInfos {
				// Get value from source item using relative field path
				value, err := GetValueAtPath(sourceItemMap, subFieldSchemaInfo.ProviderField)
				if err == nil {
					destinationObject[subFieldName] = value
				}
			}

			destinationArray = append(destinationArray, destinationObject)
		}
	}

	// Add the completed array to the response
	fullPath := strings.Join(append(path, fieldName), ".")
	_, err = PushValue(responseData, fullPath, destinationArray)
	if err != nil {
		logger.Log.Error("Error pushing array", "path", fullPath, "error", err)
	}
}

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

// isArrayFieldValue checks if a field is an array field based on the value type
func isArrayFieldValue(fieldName string, value interface{}) bool {
	// Check if the value is an array
	if _, ok := value.([]interface{}); ok {
		logger.Log.Debug("isArrayFieldValue: value is array", "fieldName", fieldName, "valueType", fmt.Sprintf("%T", value))
		return true
	}

	logger.Log.Debug("isArrayFieldValue: value is not array", "fieldName", fieldName, "valueType", fmt.Sprintf("%T", value))
	return false
}

// isArrayFieldInSchema checks if a field is an array field based on the schema definition
func isArrayFieldInSchema(schema *ast.Document, parentTypeName, fieldName string) bool {
	if schema == nil || parentTypeName == "" || fieldName == "" {
		return false
	}

	// Find the parent type definition in the schema
	for _, def := range schema.Definitions {
		if objType, ok := def.(*ast.ObjectDefinition); ok {
			// Convert to PascalCase for type matching (vehicleInfo -> VehicleInfo)
			pascalTypeName := strings.ToUpper(parentTypeName[:1]) + parentTypeName[1:]
			if objType.Name.Value == pascalTypeName {
				// Find the field in the parent type
				for _, field := range objType.Fields {
					if field.Name.Value == fieldName {
						// Check if this is an array type (List type)
						if _, ok := field.Type.(*ast.List); ok {
							return true
						}
					}
				}
			}
		}
	}
	return false
}

// getArrayElementTypeNameFromSchema dynamically derives array element type names from the schema
func getArrayElementTypeNameFromSchema(schema *ast.Document, parentTypeName, arrayFieldName string) string {
	if schema == nil || parentTypeName == "" {
		return ""
	}

	// Find the parent type definition in the schema
	for _, def := range schema.Definitions {
		if objType, ok := def.(*ast.ObjectDefinition); ok {
			// Convert to PascalCase for type matching (vehicleInfo -> VehicleInfo)
			pascalTypeName := strings.ToUpper(parentTypeName[:1]) + parentTypeName[1:]
			if objType.Name.Value == pascalTypeName {
				// Find the array field in the parent type
				for _, field := range objType.Fields {
					if field.Name.Value == arrayFieldName {
						// Check if this is an array type (List type)
						if listType, ok := field.Type.(*ast.List); ok {
							// Get the element type from the list
							if namedType, ok := listType.Type.(*ast.Named); ok {
								return namedType.Name.Value
							}
						}
					}
				}
			}
		}
	}
	return ""
}

// PushArrayValue is similar to PushValue but with enhanced array handling
func PushArrayValue(obj interface{}, path string, value interface{}) (interface{}, error) {
	keys := strings.Split(path, ".")
	return pushArrayRecursive(obj, keys, value)
}

func pushArrayRecursive(obj interface{}, keys []string, value interface{}) (interface{}, error) {
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

		newChild, err := pushArrayRecursive(child, keys[1:], value)
		if err != nil {
			return nil, err
		}
		curr[key] = newChild
		return curr, nil

	case []interface{}:
		// For arrays: apply pushArrayRecursive to all elements
		newArr := make([]interface{}, len(curr))
		for i, elem := range curr {
			newChild, err := pushArrayRecursive(elem, keys, value)
			if err != nil {
				return nil, err
			}
			newArr[i] = newChild
		}
		return newArr, nil

	case nil:
		// Initialize a map if nil
		child := map[string]interface{}{}
		newChild, err := pushArrayRecursive(child, keys, value)
		if err != nil {
			return nil, err
		}
		return newChild, nil

	default:
		return nil, fmt.Errorf("unexpected type %T at key %q", obj, key)
	}
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
