package federator

import (
	"strconv"
	"strings"

	"github.com/OpenNDX/openndx-core/exchange/orchestration-engine/pkg/graphql"
	"github.com/graphql-go/graphql/language/ast"
	"github.com/graphql-go/graphql/language/kinds"
	"github.com/graphql-go/graphql/language/printer"
)

// SchemaCollectionResponse holds the traditional response structure for backward compatibility
type SchemaCollectionResponse struct {
	ProviderFieldMap    *[]ProviderLevelFieldRecord
	Arguments           []*ast.Argument
	VariableDefinitions []*ast.VariableDefinition
}

// SourceSchemaInfo holds information about a field's source mapping and array properties
type SourceSchemaInfo struct {
	ProviderKey            string                       // The provider service key
	ProviderField          string                       // The field path in the provider response
	IsArray                bool                         // Flag to identify array fields
	ProviderArrayFieldPath string                       // Path to the source array in the provider's response (e.g., "vehicle.getVehicleInfos.data")
	SubFieldSchemaInfos    map[string]*SourceSchemaInfo // Schema info for fields inside array elements
}

func QueryBuilder(maps *[]ProviderLevelFieldRecord, args []*ArgSource) ([]*federationServiceRequest, error) {
	// initialize return variable
	requests := make([]*federationServiceRequest, 0)

	queries := BuildProviderLevelQuery(maps)

	// convert the queries into federationServiceRequest
	for _, q := range queries {
		// find the arguments to the specific provider
		providerArgs := make([]*ArgSource, 0)

		for _, arg := range args {
			if arg == nil {
				continue
			}

			if arg.ArgMapping.ProviderKey == q.ServiceKey && arg.ArgMapping.SchemaID == q.SchemaID {
				providerArgs = append(providerArgs, arg)
			}
		}

		PushArgumentsToProviderQueryAst(providerArgs, q)

		query := printer.Print(q.QueryAst).(string)
		println(printer.Print(q.QueryAst).(string))

		requests = append(requests, &federationServiceRequest{
			ServiceKey: q.ServiceKey,
			SchemaID:   q.SchemaID,
			GraphQLRequest: graphql.Request{
				Query:     query,
				Variables: nil,
			},
		})
	}

	return requests, nil
}

type ProviderLevelFieldRecord struct {
	ServiceKey string
	SchemaId   string
	FieldPath  string
}

// ProviderFieldMap A function to convert the directives into a map of service key to a list of fields.
func ProviderFieldMap(directives []*ast.Directive) *[]ProviderLevelFieldRecord {
	fieldMap := make([]ProviderLevelFieldRecord, 0)

	for _, dir := range directives {
		if dir.Name.Value == "sourceInfo" {
			record := ProviderLevelFieldRecord{}
			for _, arg := range dir.Arguments {
				if arg.Name.Value == "providerKey" {
					if val, ok := arg.Value.(*ast.StringValue); ok {
						record.ServiceKey = val.Value
					}
				}
				if arg.Name.Value == "schemaId" {
					if val, ok := arg.Value.(*ast.StringValue); ok {
						record.SchemaId = val.Value
					}
				}
				if arg.Name.Value == "providerField" {
					if val, ok := arg.Value.(*ast.StringValue); ok {
						record.FieldPath = val.Value
					}
				}
			}
			fieldMap = append(fieldMap, record)
		}
	}
	return &fieldMap
}

func ProviderSchemaCollector(schema *ast.Document, query *ast.Document) (*SchemaCollectionResponse, error) {
	// map of service key to list of fields

	// only query is supported not mutations or subscriptions
	if len(query.Definitions) != 1 || query.Definitions[0].(*ast.OperationDefinition).Operation != "query" {
		return nil, &graphql.JSONError{
			Message: "Only query operation is supported",
		}
	}

	// iterate through the query fields
	selections := query.Definitions[0].(*ast.OperationDefinition).SelectionSet
	// get the query object definition from the schema
	queryObjectDef := GetQueryObjectDefinition(schema)

	if queryObjectDef == nil {
		return nil, &graphql.JSONError{
			Message: "Query object definition not found in schema",
		}
	}
	providerDirectives, arguments := RecursivelyExtractSourceSchemaInfo(selections, schema, queryObjectDef, nil, nil)

	providerFieldMap := ProviderFieldMap(providerDirectives)

	// get variable definitions from the query
	variableDefinitions := query.Definitions[0].(*ast.OperationDefinition).VariableDefinitions

	return &SchemaCollectionResponse{
		ProviderFieldMap:    providerFieldMap,
		Arguments:           arguments,
		VariableDefinitions: variableDefinitions,
	}, nil
}

// BuildSchemaInfoMap creates a map of field paths to SourceSchemaInfo for array-aware processing
func BuildSchemaInfoMap(schema *ast.Document, query *ast.Document) (map[string]*SourceSchemaInfo, error) {
	// only query is supported not mutations or subscriptions
	if len(query.Definitions) != 1 || query.Definitions[0].(*ast.OperationDefinition).Operation != "query" {
		return nil, &graphql.JSONError{
			Message: "Only query operation is supported",
		}
	}

	// iterate through the query fields
	selections := query.Definitions[0].(*ast.OperationDefinition).SelectionSet
	// get the query object definition from the schema
	queryObjectDef := GetQueryObjectDefinition(schema)

	if queryObjectDef == nil {
		return nil, &graphql.JSONError{
			Message: "Query object definition not found in schema",
		}
	}

	schemaInfoMap := make(map[string]*SourceSchemaInfo)
	buildSchemaInfoMapRecursive(selections, schema, queryObjectDef, "", schemaInfoMap)

	return schemaInfoMap, nil
}

// buildSchemaInfoMapRecursive builds a map of field paths to SourceSchemaInfo with array awareness
func buildSchemaInfoMapRecursive(
	selectionSet *ast.SelectionSet,
	schema *ast.Document,
	objectDefinition *ast.ObjectDefinition,
	parentPath string,
	schemaInfoMap map[string]*SourceSchemaInfo,
) {
	if selectionSet == nil {
		return
	}

	for _, selection := range selectionSet.Selections {
		if field, ok := selection.(*ast.Field); ok {
			fieldName := field.Name.Value
			currentPath := fieldName
			if parentPath != "" {
				currentPath = parentPath + "." + fieldName
			}

			// Find the field definition in the schema
			fieldDef := FindFieldDefinitionFromFieldName(fieldName, schema, objectDefinition.Name.Value)

			if fieldDef != nil && len(fieldDef.Directives) > 0 {
				// Check for @sourceInfo directive
				for _, dir := range fieldDef.Directives {
					if dir.Name.Value == "sourceInfo" {
						// Extract provider key and field from directive
						var providerKey, providerField string
						for _, arg := range dir.Arguments {
							if arg.Name.Value == "providerKey" {
								if val, ok := arg.Value.(*ast.StringValue); ok {
									providerKey = val.Value
								}
							}
							if arg.Name.Value == "providerField" {
								if val, ok := arg.Value.(*ast.StringValue); ok {
									providerField = val.Value
								}
							}
						}

						// Check if this is an array field
						isArray := false
						providerArrayFieldPath := ""
						if fieldDef.Type != nil && fieldDef.Type.GetKind() == "List" {
							isArray = true
							providerArrayFieldPath = providerField
						}

						// Create SourceSchemaInfo
						schemaInfo := &SourceSchemaInfo{
							ProviderKey:            providerKey,
							ProviderField:          providerField,
							IsArray:                isArray,
							ProviderArrayFieldPath: providerArrayFieldPath,
							SubFieldSchemaInfos:    make(map[string]*SourceSchemaInfo),
						}

						// If this is an array field, process nested fields
						if isArray && selection.GetSelectionSet() != nil && len(selection.GetSelectionSet().Selections) > 0 {
							// Get the type of the array elements
							var nestedObjectDef *ast.ObjectDefinition
							if listType, ok := fieldDef.Type.(*ast.List); ok {
								if namedType, ok := listType.Type.(*ast.Named); ok {
									nestedObjectDef = findTopLevelObjectDefinitionInSchema(namedType.Name.Value, schema)
								}
							}

							if nestedObjectDef != nil {
								// Process nested fields for array elements
								processNestedFieldsForArray(selection.GetSelectionSet(), schema, nestedObjectDef, schemaInfo.SubFieldSchemaInfos)
							}
						}

						schemaInfoMap[currentPath] = schemaInfo
						break
					}
				}
			}

			// Process nested fields for non-array fields
			if fieldDef != nil && fieldDef.Type != nil && fieldDef.Type.GetKind() != "List" && selection.GetSelectionSet() != nil && len(selection.GetSelectionSet().Selections) > 0 {
				var nestedObjectDef *ast.ObjectDefinition
				if fieldDef.Type.GetKind() == "Named" {
					nestedObjectDef = findTopLevelObjectDefinitionInSchema(fieldDef.Type.(*ast.Named).Name.Value, schema)
				}

				if nestedObjectDef != nil {
					buildSchemaInfoMapRecursive(selection.GetSelectionSet(), schema, nestedObjectDef, currentPath, schemaInfoMap)
				}
			}
		}
	}
}

// processNestedFieldsForArray processes nested fields specifically for array elements
func processNestedFieldsForArray(
	selectionSet *ast.SelectionSet,
	schema *ast.Document,
	objectDefinition *ast.ObjectDefinition,
	subFieldSchemaInfos map[string]*SourceSchemaInfo,
) {
	if selectionSet == nil {
		return
	}

	for _, selection := range selectionSet.Selections {
		if field, ok := selection.(*ast.Field); ok {
			fieldName := field.Name.Value

			// Find the field definition in the schema
			fieldDef := FindFieldDefinitionFromFieldName(fieldName, schema, objectDefinition.Name.Value)

			if fieldDef != nil && len(fieldDef.Directives) > 0 {
				// Check for @sourceInfo directive
				for _, dir := range fieldDef.Directives {
					if dir.Name.Value == "sourceInfo" {
						// Extract provider key and field from directive
						var providerKey, providerField string
						for _, arg := range dir.Arguments {
							if arg.Name.Value == "providerKey" {
								if val, ok := arg.Value.(*ast.StringValue); ok {
									providerKey = val.Value
								}
							}
							if arg.Name.Value == "providerField" {
								if val, ok := arg.Value.(*ast.StringValue); ok {
									providerField = val.Value
								}
							}
						}

						// Create SourceSchemaInfo for sub-field
						// For array sub-fields, the provider field should be relative to the array element
						// Extract just the field name from the full path
						relativeFieldPath := providerField
						if strings.Contains(providerField, ".") {
							// Extract the last part of the path (e.g., "registrationNumber" from "vehicle.getVehicleInfos.data.registrationNumber")
							parts := strings.Split(providerField, ".")
							relativeFieldPath = parts[len(parts)-1]
						}

						subFieldSchemaInfos[fieldName] = &SourceSchemaInfo{
							ProviderKey:   providerKey,
							ProviderField: relativeFieldPath,
							IsArray:       false,
						}
						break
					}
				}
			}
		}
	}
}

// This function recursively traverses the selection set to extract @sourceInfo directives.
func RecursivelyExtractSourceSchemaInfo(
	selectionSet *ast.SelectionSet,
	schema *ast.Document,
	objectDefinition *ast.ObjectDefinition,
	directives []*ast.Directive,
	arguments []*ast.Argument,
) ([]*ast.Directive, []*ast.Argument) {
	// base case
	if selectionSet == nil {
		return directives, arguments
	}

	// if directives is nil, initialize it
	if directives == nil {
		directives = make([]*ast.Directive, 0)
		arguments = make([]*ast.Argument, 0)
	}

	for _, selection := range selectionSet.Selections {
		if field, ok := selection.(*ast.Field); ok {
			// Find the field definition in the schema
			fieldDef := FindFieldDefinitionFromFieldName(field.Name.Value, schema, objectDefinition.Name.Value)

			// Check for @sourceInfo directive
			if fieldDef != nil && len(fieldDef.Directives) > 0 {
				for _, dir := range fieldDef.Directives {
					if dir.Name.Value == "sourceInfo" {
						directives = append(directives, dir)

						// push the directive to the query ast
						if field.Directives == nil {
							field.Directives = make([]*ast.Directive, 0)
						}
						field.Directives = append(field.Directives, dir)
					}
				}
			}

			if len(field.Arguments) > 0 {
				arguments = append(arguments, field.Arguments...)
			}

			if selection.GetSelectionSet() != nil && len(selection.GetSelectionSet().Selections) > 0 {
				// Recursively process nested selection sets
				var nestedObjectDef *ast.ObjectDefinition
				isArrayField := false

				if fieldDef != nil && fieldDef.Type != nil {
					if fieldDef.Type.GetKind() == "Named" {
						nestedObjectDef = findTopLevelObjectDefinitionInSchema(fieldDef.Type.(*ast.Named).Name.Value, schema)
					} else if fieldDef.Type.GetKind() == "List" {
						isArrayField = true
						// For array fields, get the type of the list elements
						if listType, ok := fieldDef.Type.(*ast.List); ok {
							if namedType, ok := listType.Type.(*ast.Named); ok {
								nestedObjectDef = findTopLevelObjectDefinitionInSchema(namedType.Name.Value, schema)
							}
						}
					}
				}

				if nestedObjectDef != nil {
					selectionSet := field.GetSelectionSet()
					// For backward compatibility, use the old function for non-array fields
					if !isArrayField {
						directives, arguments = RecursivelyExtractSourceSchemaInfo(selectionSet, schema, nestedObjectDef, directives, arguments)
					} else {
						// For array fields, use the new function
						directives, arguments = RecursivelyExtractSourceSchemaInfoWithArrayInfo(selectionSet, schema, nestedObjectDef, directives, arguments, isArrayField)
					}
				}
			}
		}
	}
	return directives, arguments
}

// RecursivelyExtractSourceSchemaInfoWithArrayInfo is similar to RecursivelyExtractSourceSchemaInfo but includes array field information
func RecursivelyExtractSourceSchemaInfoWithArrayInfo(
	selectionSet *ast.SelectionSet,
	schema *ast.Document,
	objectDefinition *ast.ObjectDefinition,
	directives []*ast.Directive,
	arguments []*ast.Argument,
	isArrayField bool,
) ([]*ast.Directive, []*ast.Argument) {
	// base case
	if selectionSet == nil {
		return directives, arguments
	}

	// if directives is nil, initialize it
	if directives == nil {
		directives = make([]*ast.Directive, 0)
		arguments = make([]*ast.Argument, 0)
	}

	for _, selection := range selectionSet.Selections {
		if field, ok := selection.(*ast.Field); ok {
			// Find the field definition in the schema
			fieldDef := FindFieldDefinitionFromFieldName(field.Name.Value, schema, objectDefinition.Name.Value)

			// Check for @sourceInfo directive
			if fieldDef != nil && len(fieldDef.Directives) > 0 {
				for _, dir := range fieldDef.Directives {
					if dir.Name.Value == "sourceInfo" {
						// For array fields, we might need to handle the directive differently
						directives = append(directives, dir)

						// push the directive to the query ast
						if field.Directives == nil {
							field.Directives = make([]*ast.Directive, 0)
						}
						field.Directives = append(field.Directives, dir)
					}
				}
			}

			if len(field.Arguments) > 0 {
				arguments = append(arguments, field.Arguments...)
			}

			if selection.GetSelectionSet() != nil && len(selection.GetSelectionSet().Selections) > 0 {
				// Recursively process nested selection sets
				var nestedObjectDef *ast.ObjectDefinition
				nestedIsArrayField := false

				if fieldDef != nil && fieldDef.Type != nil {
					if fieldDef.Type.GetKind() == "Named" {
						nestedObjectDef = findTopLevelObjectDefinitionInSchema(fieldDef.Type.(*ast.Named).Name.Value, schema)
					} else if fieldDef.Type.GetKind() == "List" {
						nestedIsArrayField = true
						// For array fields, get the type of the list elements
						if listType, ok := fieldDef.Type.(*ast.List); ok {
							if namedType, ok := listType.Type.(*ast.Named); ok {
								nestedObjectDef = findTopLevelObjectDefinitionInSchema(namedType.Name.Value, schema)
							}
						}
					}
				}

				if nestedObjectDef != nil {
					selectionSet := field.GetSelectionSet()
					directives, arguments = RecursivelyExtractSourceSchemaInfoWithArrayInfo(selectionSet, schema, nestedObjectDef, directives, arguments, nestedIsArrayField)
				}
			}
		}
	}
	return directives, arguments
}

// FindFieldDefinitionFromFieldName Helper function to find a field definition in the schema by field name and parent object name
func FindFieldDefinitionFromFieldName(fieldName string, schema *ast.Document, parentObjectName string) *ast.FieldDefinition {
	// Find the parent object definition in the schema
	parentObjectDef := findTopLevelObjectDefinitionInSchema(parentObjectName, schema)
	if parentObjectDef == nil {
		return nil
	}

	// Find the field definition within the parent object
	fieldDef := findFieldDefinitionInObject(parentObjectDef, fieldName)
	return fieldDef
}

func PushArgumentValue(arg *ast.Argument, val interface{}) {
	switch v := val.(type) {
	case string:
		arg.Value = &ast.StringValue{
			Kind:  kinds.StringValue,
			Value: v,
		}
	case int:
		arg.Value = &ast.IntValue{
			Kind:  kinds.IntValue,
			Value: string(rune(v)),
		}
	case float64:
		// inside your case float64:
		arg.Value = &ast.FloatValue{
			Kind:  kinds.FloatValue,
			Value: strconv.FormatFloat(v, 'f', -1, 64),
		}
	case bool:
		arg.Value = &ast.BooleanValue{
			Kind:  kinds.BooleanValue,
			Value: v,
		}
	}
}

// PushVariablesFromVariableDefinition replaces variable references in arguments with actual values from the request.
func PushVariablesFromVariableDefinition(request graphql.Request, extractedArgs []*ArgSource, variableDefinitions []*ast.VariableDefinition) {
	for _, arg := range extractedArgs {
		if arg == nil || arg.Argument == nil || arg.Argument.Value == nil {
			continue
		}

		if arg.Argument.Value.GetKind() == "Variable" {
			if variable, ok := arg.Argument.Value.(*ast.Variable); ok && variable.Name != nil {
				varName := variable.Name.Value
				if val, exists := request.Variables[varName]; exists {
					// find the corresponding variable definition
					for _, v := range variableDefinitions {
						if v != nil && v.Variable != nil && v.Variable.Name != nil && v.Variable.Name.Value == varName {
							// replace the argument value with the variable value
							PushArgumentValue(arg.Argument, val)
							break
						}
					}
				}
			}
		}
	}
}

// Helper function to find a top level object field in the schema by name
func findTopLevelObjectDefinitionInSchema(objectName string, schema *ast.Document) *ast.ObjectDefinition {
	for _, def := range schema.Definitions {
		if objDef, ok := def.(*ast.ObjectDefinition); ok {
			if objDef.Name.Value == objectName {
				return objDef
			}
		}
	}
	return nil
}

// Helper function to find a field definition in an object definition by name
func findFieldDefinitionInObject(objectDef *ast.ObjectDefinition, fieldName string) *ast.FieldDefinition {
	for _, fieldDef := range objectDef.Fields {
		if fieldDef.Name.Value == fieldName {
			return fieldDef
		}
	}
	return nil
}

func GetQueryObjectDefinition(schema *ast.Document) *ast.ObjectDefinition {
	for _, def := range schema.Definitions {
		if objDef, ok := def.(*ast.ObjectDefinition); ok {
			if objDef.Name.Value == "Query" {
				return objDef
			}
		}
	}
	return nil
}
