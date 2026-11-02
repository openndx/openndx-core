package policy

import (
	"fmt"
	"strings"

	"github.com/vektah/gqlparser/v2"
	"github.com/vektah/gqlparser/v2/ast"
)

// sdlParser handles GraphQL SDL parsing and conversion to policy metadata requests
type sdlParser struct{}

// newSDLParser creates a new sdlParser instance
func newSDLParser() *sdlParser {
	return &sdlParser{}
}

// ParseSDLToPolicyRequest parses GraphQL SDL and creates a PolicyMetadataCreateRequest
func (h *sdlParser) ParseSDLToPolicyRequest(schemaID, sdl string) (*PolicyMetadataCreateRequest, error) {
	schema, err := gqlparser.LoadSchema(&ast.Source{Input: sdl})
	if err != nil {
		return nil, fmt.Errorf("failed to parse GraphQL schema: %w", err)
	}

	var records []PolicyMetadataCreateRequestRecord

	// Process all types (excluding built-ins and Query/Mutation/Subscription)
	for typeName, typeDefinition := range schema.Types {
		if h.isBuiltInType(typeName) || h.isRootType(typeName) {
			continue
		}

		// Process fields for each type
		for _, field := range typeDefinition.Fields {
			fieldPath := h.buildFieldPath(typeName, field.Name)

			// Extract directives from field
			record, err := h.createRecordFromField(fieldPath, field)
			if err != nil {
				return nil, err
			}
			if record != nil {
				records = append(records, *record)
			}

			// Handle nested object types
			nestedRecords, err := h.processNestedFields(schema, fieldPath, field.Type, field)
			if err != nil {
				return nil, err
			}
			records = append(records, nestedRecords...)
		}
	}

	return &PolicyMetadataCreateRequest{
		SchemaID: schemaID,
		Records:  records,
	}, nil
}

// createRecordFromField creates a PolicyMetadataCreateRequestRecord from a GraphQL field
func (h *sdlParser) createRecordFromField(fieldPath string, field *ast.FieldDefinition) (*PolicyMetadataCreateRequestRecord, error) {
	if err := h.rejectRemovedDirectives(fieldPath, field.Directives); err != nil {
		return nil, err
	}

	// Extract directives
	accessControl := field.Directives.ForName("accessControl")
	policyClass := h.getDirectiveValue(field.Directives, "accessControl", "policy")
	sourceValue := h.getDirectiveValue(field.Directives, "source", "value")
	displayName := h.getDirectiveValue(field.Directives, "displayName", "value")
	description := h.getDirectiveValue(field.Directives, "description", "value")
	ownerValue := h.getDirectiveValue(field.Directives, "owner", "value")

	if accessControl != nil && policyClass == "" {
		return nil, fmt.Errorf("field %s: @accessControl needs a policy, e.g. @accessControl(policy: %q)",
			fieldPath, PolicyClassAllowlistConsent)
	}

	// Skip if no relevant directives found
	if accessControl == nil && sourceValue == "" {
		return nil, nil
	}

	record := PolicyMetadataCreateRequestRecord{
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
		record.Source = Source(sourceValue)
	} else {
		record.Source = SourceFallback // default
	}

	// Set policy class (default to "allowlist" if not specified)
	if policyClass != "" {
		record.PolicyClass = policyClass
	} else {
		record.PolicyClass = PolicyClassAllowlist // default
	}

	// Set owner if specified (can be null)
	if ownerValue != "" {
		owner := Owner(ownerValue)
		record.Owner = &owner
	}

	return &record, nil
}

// rejectRemovedDirectives fails on the access-control directives replaced by
// policy classes. Ignoring them would quietly drop a consent requirement.
func (h *sdlParser) rejectRemovedDirectives(fieldPath string, directives ast.DirectiveList) error {
	if directives.ForName("isOwner") != nil {
		return fmt.Errorf("field %s: @isOwner was removed; drop it and set the field's policy with @accessControl(policy: ...)", fieldPath)
	}
	if accessControl := directives.ForName("accessControl"); accessControl != nil && accessControl.Arguments.ForName("type") != nil {
		return fmt.Errorf("field %s: @accessControl(type: ...) was removed; use @accessControl(policy: %q) or @accessControl(policy: %q)",
			fieldPath, PolicyClassAllowlist, PolicyClassAllowlistConsent)
	}
	return nil
}

// processNestedFields recursively processes nested object fields
func (h *sdlParser) processNestedFields(schema *ast.Schema, basePath string, fieldType *ast.Type, parentField *ast.FieldDefinition) ([]PolicyMetadataCreateRequestRecord, error) {
	var records []PolicyMetadataCreateRequestRecord

	// Get the actual type name (handle lists and non-nulls)
	typeName := h.getBaseTypeName(fieldType)

	// Check if this is a custom type (not a scalar)
	if typeDefinition, exists := schema.Types[typeName]; exists && !h.isBuiltInType(typeName) {
		for _, nestedField := range typeDefinition.Fields {
			nestedPath := basePath + "." + nestedField.Name

			// Create record for nested field
			record, err := h.createRecordFromField(nestedPath, nestedField)
			if err != nil {
				return nil, err
			}
			if record != nil {
				records = append(records, *record)
			}

			// Recursively process further nested fields
			furtherNested, err := h.processNestedFields(schema, nestedPath, nestedField.Type, nestedField)
			if err != nil {
				return nil, err
			}
			records = append(records, furtherNested...)
		}
	}

	return records, nil
}

// buildFieldPath creates the dot-notation field path (e.g., "user.birthInfo")
func (h *sdlParser) buildFieldPath(typeName, fieldName string) string {
	return strings.ToLower(typeName) + "." + fieldName
}

// getDirectiveValue extracts a value from a directive
func (h *sdlParser) getDirectiveValue(directives ast.DirectiveList, directiveName, argName string) string {
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
func (h *sdlParser) getBaseTypeName(fieldType *ast.Type) string {
	if fieldType.Elem != nil {
		return h.getBaseTypeName(fieldType.Elem)
	}
	return fieldType.NamedType
}

// isBuiltInType checks if a type is a GraphQL built-in type
func (h *sdlParser) isBuiltInType(typeName string) bool {
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
func (h *sdlParser) isRootType(typeName string) bool {
	rootTypes := map[string]bool{
		"Query":        true,
		"Mutation":     true,
		"Subscription": true,
	}
	return rootTypes[typeName]
}
