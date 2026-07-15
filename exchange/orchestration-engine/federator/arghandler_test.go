package federator

import (
	"testing"

	"github.com/OpenNDX/openndx-core/exchange/orchestration-engine/pkg/graphql"
	"github.com/graphql-go/graphql/language/ast"
	"github.com/stretchr/testify/assert"
)

func TestFindRequiredArguments(t *testing.T) {
	tests := []struct {
		name           string
		flattenedPaths *[]ProviderLevelFieldRecord
		argMappings    []*graphql.ArgMapping
		expectedCount  int
		expectedKeys   []string
		description    string
	}{
		{
			name: "Single Argument Mapping",
			flattenedPaths: &[]ProviderLevelFieldRecord{
				{
					ServiceKey: "drp",
					SchemaId:   "v1",
					FieldPath:  "person.fullName",
				},
				{
					ServiceKey: "drp",
					SchemaId:   "v1",
					FieldPath:  "person.address",
				},
			},
			argMappings: []*graphql.ArgMapping{
				{
					ProviderKey:   "drp",
					SchemaID:      "v1",
					TargetArgName: "nic",
					SourceArgPath: "personInfo-nic",
					TargetArgPath: "person",
				},
			},
			expectedCount: 1,
			expectedKeys:  []string{"person"},
			description:   "Should find single required argument",
		},
		{
			name: "Multiple Argument Mappings",
			flattenedPaths: &[]ProviderLevelFieldRecord{
				{
					ServiceKey: "drp",
					SchemaId:   "drp-v1",
					FieldPath:  "person.fullName",
				},
				{
					ServiceKey: "rgd",
					SchemaId:   "rgd-v2",
					FieldPath:  "getPersonInfo.name",
				},
				{
					ServiceKey: "dmt",
					SchemaId:   "rgd-v1",
					FieldPath:  "vehicle.getVehicleInfos.data",
				},
			},
			argMappings: []*graphql.ArgMapping{
				{
					ProviderKey:   "drp",
					SchemaID:      "drp-v1",
					TargetArgName: "nic",
					SourceArgPath: "personInfo-nic",
					TargetArgPath: "person",
				},
				{
					ProviderKey:   "rgd",
					SchemaID:      "rgd-v2",
					TargetArgName: "nic",
					SourceArgPath: "personInfo-nic",
					TargetArgPath: "getPersonInfo",
				},
				{
					ProviderKey:   "dmt",
					TargetArgName: "regNos",
					SchemaID:      "rgd-v1",
					SourceArgPath: "vehicles-regNos",
					TargetArgPath: "vehicle.getVehicleInfos",
				},
			},
			expectedCount: 3,
			expectedKeys:  []string{"person", "getPersonInfo", "vehicle.getVehicleInfos"},
			description:   "Should find multiple required arguments",
		},
		{
			name:           "Empty Argument Mappings",
			flattenedPaths: &[]ProviderLevelFieldRecord{},
			argMappings:    []*graphql.ArgMapping{},
			expectedCount:  0,
			expectedKeys:   []string{},
			description:    "Should handle empty argument mappings",
		},
		{
			name: "Duplicate Source Paths",
			flattenedPaths: &[]ProviderLevelFieldRecord{
				{
					ServiceKey: "drp",
					SchemaId:   "11",
					FieldPath:  "person.fullName",
				},
				{
					ServiceKey: "rgd",
					SchemaId:   "12",
					FieldPath:  "getPersonInfo.name",
				},
			},
			argMappings: []*graphql.ArgMapping{
				{
					ProviderKey:   "drp",
					SchemaID:      "11",
					TargetArgName: "nic",
					SourceArgPath: "personInfo-nic",
					TargetArgPath: "person",
				},
				{
					ProviderKey:   "rgd",
					TargetArgName: "nic",
					SchemaID:      "12",
					SourceArgPath: "personInfo-nic",
					TargetArgPath: "getPersonInfo",
				},
			},
			expectedCount: 2,
			expectedKeys:  []string{"person", "getPersonInfo"},
			description:   "Should find both argument mappings",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			requiredArgs := FindRequiredArguments(tt.flattenedPaths, tt.argMappings)

			assert.Len(t, requiredArgs, tt.expectedCount, tt.description)

			// Verify target paths
			actualKeys := make([]string, len(requiredArgs))
			for i, arg := range requiredArgs {
				actualKeys[i] = arg.TargetArgPath
			}
			assert.ElementsMatch(t, tt.expectedKeys, actualKeys, "Should have correct target paths")
		})
	}
}

func TestExtractRequiredArguments(t *testing.T) {
	tests := []struct {
		name          string
		argMappings   []*graphql.ArgMapping
		arguments     []*ast.Argument
		expectedCount int
		expectError   bool
		description   string
	}{
		{
			name: "Single String Argument",
			argMappings: []*graphql.ArgMapping{
				{
					ProviderKey:   "drp",
					TargetArgName: "nic",
					SourceArgPath: "personInfo-nic",
					TargetArgPath: "drp.person",
				},
			},
			arguments: []*ast.Argument{
				{
					Name:  &ast.Name{Value: "nic"},
					Value: &ast.StringValue{Value: "123456789V"},
				},
			},
			expectedCount: 1,
			expectError:   false,
			description:   "Should extract single string argument",
		},
		{
			name: "Array Argument",
			argMappings: []*graphql.ArgMapping{
				{
					ProviderKey:   "dmt",
					TargetArgName: "regNos",
					SourceArgPath: "vehicles-regNos",
					TargetArgPath: "dmt.vehicle.getVehicleInfos",
				},
			},
			arguments: []*ast.Argument{
				{
					Name: &ast.Name{Value: "regNos"},
					Value: &ast.ListValue{
						Values: []ast.Value{
							&ast.StringValue{Value: "ABC123"},
							&ast.StringValue{Value: "XYZ789"},
						},
					},
				},
			},
			expectedCount: 1,
			expectError:   false,
			description:   "Should extract array argument",
		},
		{
			name: "Multiple Arguments",
			argMappings: []*graphql.ArgMapping{
				{
					ProviderKey:   "drp",
					TargetArgName: "nic",
					SourceArgPath: "personInfo-nic",
					TargetArgPath: "drp.person",
				},
				{
					ProviderKey:   "drp",
					TargetArgName: "includeVehicles",
					SourceArgPath: "personInfo-includeVehicles",
					TargetArgPath: "drp.person",
				},
			},
			arguments: []*ast.Argument{
				{
					Name:  &ast.Name{Value: "nic"},
					Value: &ast.StringValue{Value: "123456789V"},
				},
				{
					Name:  &ast.Name{Value: "includeVehicles"},
					Value: &ast.BooleanValue{Value: true},
				},
			},
			expectedCount: 2,
			expectError:   false,
			description:   "Should extract multiple arguments",
		},
		{
			name:          "Empty Arguments",
			argMappings:   []*graphql.ArgMapping{},
			arguments:     []*ast.Argument{},
			expectedCount: 0,
			expectError:   false,
			description:   "Should handle empty arguments",
		},
		{
			name: "No Matching Arguments",
			argMappings: []*graphql.ArgMapping{
				{
					ProviderKey:   "drp",
					TargetArgName: "nic",
					SourceArgPath: "personInfo-nic",
					TargetArgPath: "drp.person",
				},
			},
			arguments: []*ast.Argument{
				{
					Name:  &ast.Name{Value: "differentArg"},
					Value: &ast.StringValue{Value: "value"},
				},
			},
			expectedCount: 0,
			expectError:   false,
			description:   "Should handle no matching arguments",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			argSources := ExtractRequiredArguments(tt.argMappings, tt.arguments)

			assert.Len(t, argSources, tt.expectedCount, "Should extract correct number of arguments")

			// Verify argument values
			for _, argSource := range argSources {
				assert.NotNil(t, argSource.Argument, "Should have valid argument")
				assert.NotNil(t, argSource.ArgMapping, "Should have valid argument mapping")
			}
		})
	}
}

func TestFindArrayRequiredArguments(t *testing.T) {
	flattenedPaths := []string{
		"personInfo.ownedVehicles",
		"personInfo.ownedVehicles.regNo",
		"personInfo.ownedVehicles.make",
		"personInfo.allowList",
	}

	argMappings := []*graphql.ArgMapping{
		{
			ProviderKey:   "dmt",
			TargetArgName: "regNos",
			SourceArgPath: "vehicles-regNos",
			TargetArgPath: "personInfo.ownedVehicles",
		},
		{
			ProviderKey:   "drp",
			TargetArgName: "ownerIds",
			SourceArgPath: "allowList-ownerIds",
			TargetArgPath: "personInfo.allowList",
		},
	}

	required := FindArrayRequiredArguments(flattenedPaths, argMappings)
	assert.Len(t, required, 2)

	targets := []string{required[0].TargetArgPath, required[1].TargetArgPath}
	assert.ElementsMatch(t, []string{"personInfo.ownedVehicles", "personInfo.allowList"}, targets)
}

func TestExtractArrayRequiredArguments(t *testing.T) {
	argMappings := []*graphql.ArgMapping{
		{
			ProviderKey:   "dmt",
			TargetArgName: "regNos",
			SourceArgPath: "vehicles-regNos",
			TargetArgPath: "personInfo.ownedVehicles",
		},
		{
			ProviderKey:   "drp",
			TargetArgName: "ids",
			SourceArgPath: "owners-ids",
			TargetArgPath: "personInfo.allowList",
		},
	}

	arguments := []*ast.Argument{
		{
			Name: &ast.Name{Value: "regNos"},
			Value: &ast.ListValue{
				Values: []ast.Value{
					&ast.StringValue{Value: "ABC123"},
					&ast.StringValue{Value: "XYZ789"},
				},
			},
		},
		{
			Name: &ast.Name{Value: "ids"},
			Value: &ast.ListValue{
				Values: []ast.Value{
					&ast.StringValue{Value: "owner-1"},
					&ast.StringValue{Value: "owner-2"},
				},
			},
		},
	}

	argSources := ExtractArrayRequiredArguments(argMappings, arguments)
	assert.Len(t, argSources, 2)

	first := argSources[0]
	assert.Equal(t, "regNos", first.Argument.Name.Value)
	assert.Equal(t, "personInfo.ownedVehicles", first.ArgMapping.TargetArgPath)

	second := argSources[1]
	assert.Equal(t, "ids", second.Argument.Name.Value)
	assert.Equal(t, "personInfo.allowList", second.ArgMapping.TargetArgPath)
}
