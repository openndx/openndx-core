package utils

import (
	"fmt"
	"strings"

	"github.com/OpenNDX/openndx-core/portal-backend/v1/models"
	"github.com/vektah/gqlparser/v2"
	"github.com/vektah/gqlparser/v2/ast"
)

// GraphQLHandler handles GraphQL SDL parsing and conversion to policy metadata requests
type GraphQLHandler struct{}

// NewGraphQLHandler creates a new GraphQLHandler instance
func NewGraphQLHandler() *GraphQLHandler {
	return &GraphQLHandler{}
}

// ParseSDLToPolicyRequest parses GraphQL SDL and creates a PolicyMetadataCreateRequest
func (h *GraphQLHandler) ParseSDLToPolicyRequest(schemaID, sdl string) (*models.PolicyMetadataCreateRequest, error) {
	schema, err := gqlparser.LoadSchema(&ast.Source{Input: sdl})
	if err != nil {
		return nil, fmt.Errorf("failed to parse GraphQL schema: %w", err)
	}

	var records []models.PolicyMetadataCreateRequestRecord

	// Process all types (excluding built-ins and Query/Mutation/Subscription)
	for typeName, typeDefinition := range schema.Types {
		if h.isBuiltInType(typeName) || h.isRootType(typeName) {
			continue
		}

		// Process fields for each type
		for _, field := range typeDefinition.Fields {
			fieldPath := h.buildFieldPath(typeName, field.Name)

			// Extract directives from field
			record := h.createRecordFromField(fieldPath, field)
			if record != nil {
				records = append(records, *record)
			}

			// Handle nested object types
			nestedRecords := h.processNestedFields(schema, fieldPath, field.Type, field)
			records = append(records, nestedRecords...)
		}
	}

	return &models.PolicyMetadataCreateRequest{
		SchemaID: schemaID,
		Records:  records,
	}, nil
}

// createRecordFromField creates a PolicyMetadataCreateRequestRecord from a GraphQL field
func (h *GraphQLHandler) createRecordFromField(fieldPath string, field *ast.FieldDefinition) *models.PolicyMetadataCreateRequestRecord {
	// Extract directives
	accessControlType := h.getDirectiveValue(field.Directives, "accessControl", "type")
	sourceValue := h.getDirectiveValue(field.Directives, "source", "value")
	displayName := h.getDirectiveValue(field.Directives, "displayName", "value")
	description := h.getDirectiveValue(field.Directives, "description", "value")
	isOwnerValue := h.getDirectiveValue(field.Directives, "isOwner", "value")
	ownerValue := h.getDirectiveValue(field.Directives, "owner", "value")

	// Skip if no relevant directives found
	if accessControlType == "" && sourceValue == "" {
		return nil
	}

	record := models.PolicyMetadataCreateRequestRecord{
		FieldName: fieldPath,
	}

	// Set display name (can be null)
	if displayName != "" {
		record.DisplayName = &displayName
	}

	// Set description (can be null)
	if description != "" {
		record.Description = &description
	}

	// Set source (default to "fallback" if not specified)
	if sourceValue != "" {
		record.Source = models.Source(sourceValue)
	} else {
		record.Source = models.SourceFallback // default
	}

	// Set access control type (default to "public" if not specified)
	if accessControlType != "" {
		record.AccessControlType = models.AccessControlType(accessControlType)
	} else {
		record.AccessControlType = models.AccessControlTypePublic // default
	}

	// Set isOwner (default to false)
	record.IsOwner = false
	if isOwnerValue == "true" {
		record.IsOwner = true
	}

	// Set owner if specified (can be null)
	if ownerValue != "" {
		owner := models.Owner(ownerValue)
		record.Owner = &owner
	}

	return &record
}

// processNestedFields recursively processes nested object fields
func (h *GraphQLHandler) processNestedFields(schema *ast.Schema, basePath string, fieldType *ast.Type, parentField *ast.FieldDefinition) []models.PolicyMetadataCreateRequestRecord {
	var records []models.PolicyMetadataCreateRequestRecord

	// Get the actual type name (handle lists and non-nulls)
	typeName := h.getBaseTypeName(fieldType)

	// Check if this is a custom type (not a scalar)
	if typeDefinition, exists := schema.Types[typeName]; exists && !h.isBuiltInType(typeName) {
		for _, nestedField := range typeDefinition.Fields {
			nestedPath := basePath + "." + nestedField.Name

			// Create record for nested field
			record := h.createRecordFromField(nestedPath, nestedField)
			if record != nil {
				records = append(records, *record)
			}

			// Recursively process further nested fields
			furtherNested := h.processNestedFields(schema, nestedPath, nestedField.Type, nestedField)
			records = append(records, furtherNested...)
		}
	}

	return records
}

// buildFieldPath creates the dot-notation field path (e.g., "user.birthInfo")
func (h *GraphQLHandler) buildFieldPath(typeName, fieldName string) string {
	return strings.ToLower(typeName) + "." + fieldName
}

// getDirectiveValue extracts a value from a directive
func (h *GraphQLHandler) getDirectiveValue(directives ast.DirectiveList, directiveName, argName string) string {
	for _, directive := range directives {
		if directive.Name == directiveName {
			for _, arg := range directive.Arguments {
				if arg.Name == argName {
					// Remove quotes from string values
					value := arg.Value.String()
					if strings.HasPrefix(value, `"`) && strings.HasSuffix(value, `"`) {
						return strings.Trim(value, `"`)
					}
					return value
				}
			}
		}
	}
	return ""
}

// getBaseTypeName extracts the base type name from a potentially wrapped type
func (h *GraphQLHandler) getBaseTypeName(fieldType *ast.Type) string {
	if fieldType.Elem != nil {
		return h.getBaseTypeName(fieldType.Elem)
	}
	return fieldType.NamedType
}

// isBuiltInType checks if a type is a GraphQL built-in type
func (h *GraphQLHandler) isBuiltInType(typeName string) bool {
	builtIns := map[string]bool{
		"String":              true,
		"Int":                 true,
		"Float":               true,
		"Boolean":             true,
		"ID":                  true,
		"__Schema":            true,
		"__Type":              true,
		"__Field":             true,
		"__InputValue":        true,
		"__EnumValue":         true,
		"__Directive":         true,
		"__TypeKind":          true,
		"__DirectiveLocation": true,
	}
	return builtIns[typeName]
}

// isRootType checks if a type is a root operation type
func (h *GraphQLHandler) isRootType(typeName string) bool {
	rootTypes := map[string]bool{
		"Query":        true,
		"Mutation":     true,
		"Subscription": true,
	}
	return rootTypes[typeName]
}
