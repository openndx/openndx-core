package federator

import (
	"strings"

	"github.com/graphql-go/graphql/language/ast"
)

// FindFieldDefinitionInSchema finds a field definition in the schema by type name and field name
func FindFieldDefinitionInSchema(schema *ast.Document, typeName, fieldName string) *ast.FieldDefinition {
	// Check if typeName is empty to avoid panic
	if len(typeName) == 0 {
		return nil
	}

	for _, def := range schema.Definitions {
		if objType, ok := def.(*ast.ObjectDefinition); ok {
			// Convert to PascalCase for type matching (vehicleInfo -> VehicleInfo)
			var pascalTypeName string
			if len(typeName) > 0 {
				pascalTypeName = strings.ToUpper(typeName[:1]) + typeName[1:]
			} else {
				pascalTypeName = typeName
			}
			if objType.Name.Value == pascalTypeName {
				for _, field := range objType.Fields {
					if field.Name.Value == fieldName {
						return field
					}
				}
			}
		}
	}
	return nil
}

// namedTypeName unwraps NonNull and List wrappers to the underlying named type.
func namedTypeName(t ast.Type) string {
	for t != nil {
		switch typ := t.(type) {
		case *ast.Named:
			if typ.Name == nil {
				return ""
			}
			return typ.Name.Value
		case *ast.NonNull:
			t = typ.Type
		case *ast.List:
			t = typ.Type
		default:
			return ""
		}
	}
	return ""
}

// ExtractSourceInfoFromSchema extracts @sourceInfo directive from schema using field path.
// Paths are type.field or type.intermediate....leaf. Intermediate fields may be objects or
// lists (including NonNull wrappers such as [VehicleClass!]!); each step is unwrapped to the
// underlying named type before continuing to the leaf.
func ExtractSourceInfoFromSchema(schema *ast.Document, fieldPath string) *SourceInfo {
	parts := strings.Split(fieldPath, ".")
	if len(parts) < 2 {
		return nil
	}

	typeName := parts[0]
	for _, fieldName := range parts[1 : len(parts)-1] {
		fieldDef := FindFieldDefinitionInSchema(schema, typeName, fieldName)
		if fieldDef == nil {
			return nil
		}
		typeName = namedTypeName(fieldDef.Type)
		if typeName == "" {
			return nil
		}
	}

	return findAndExtractSourceInfo(schema, typeName, parts[len(parts)-1])
}

// findAndExtractSourceInfo is a helper function to find a field and extract its source info
func findAndExtractSourceInfo(schema *ast.Document, typeName, fieldName string) *SourceInfo {
	fieldDef := FindFieldDefinitionInSchema(schema, typeName, fieldName)
	if fieldDef == nil {
		return nil
	}
	return ExtractSourceInfoFromSchemaField(fieldDef)
}
