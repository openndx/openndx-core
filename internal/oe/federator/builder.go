package federator

import (
	"strings"

	"github.com/graphql-go/graphql/language/ast"
	"github.com/graphql-go/graphql/language/kinds"
)

func BuildProviderLevelQuery(fieldsMap *[]ProviderLevelFieldRecord) []*FederationServiceAST {
	var queries []*FederationServiceAST
	var addedServiceKeys []string

	for _, field := range *fieldsMap {

		serviceKey := field.ServiceKey

		if !contains(addedServiceKeys, serviceKey) {
			addedServiceKeys = append(addedServiceKeys, serviceKey)
			queries = append(queries, &FederationServiceAST{
				ServiceKey: serviceKey,
				SchemaID:   field.SchemaId,
				QueryAst: &ast.Document{
					Kind: kinds.Document,
					Definitions: []ast.Node{
						&ast.OperationDefinition{
							Kind:      kinds.OperationDefinition,
							Operation: "query",
							Name: &ast.Name{
								Kind:  kinds.Name,
								Value: "Query" + serviceKey,
							},
							SelectionSet: &ast.SelectionSet{
								Kind: kinds.SelectionSet,
							},
						},
					},
				},
			})
		}
		// find the query with the service key
		for _, q := range queries {
			if q.ServiceKey == serviceKey && q.SchemaID == field.SchemaId {
				args := strings.Split(field.FieldPath, ".")
				pushFieldToAst(args, q.QueryAst.Definitions[0].(*ast.OperationDefinition).SelectionSet)
				break
			}
		}
	}
	return queries
}

func pushFieldToAst(field []string, parentField *ast.SelectionSet) {
	// loop through selectionSets to match the field path.
	current := field[0]

	// Cases:
	// 1. If there are no fields, add the field to the selection set.
	// 2. If there are fields, check if the field already exists.
	//    a. If it exists, go deeper if there are more fields nested.
	//    b. If it does not exist, add it to the selection set and go deeper if there are more fields nested.

	// if there are no fields, add the field to the selection set.
	if len(parentField.Selections) == 0 {
		parentField.Selections = append(parentField.Selections, &ast.Field{
			Kind: kinds.Field,
			Name: &ast.Name{
				Kind:  kinds.Name,
				Value: current,
			},
		})
		// if there are more fields nested, go deeper.
		if len(field) > 1 {
			parentField.Selections[len(parentField.Selections)-1].(*ast.Field).SelectionSet = &ast.SelectionSet{
				Kind: kinds.SelectionSet,
			}
			pushFieldToAst(field[1:], parentField.Selections[0].(*ast.Field).GetSelectionSet())
		}
		return
	} else {
		// if there are fields, check if the field already exists.
		var found bool
		for _, f := range parentField.Selections {
			if current == f.(*ast.Field).Name.Value {
				found = true
				// if there are more fields nested, go deeper.
				if len(field) > 1 {
					if f.GetSelectionSet() == nil {
						f.(*ast.Field).SelectionSet = &ast.SelectionSet{
							Kind: kinds.SelectionSet,
						}
					}
					pushFieldToAst(field[1:], f.GetSelectionSet())
				}
				break
			}
		}
		if !found {
			newField := &ast.Field{
				Kind: kinds.Field,
				Name: &ast.Name{
					Kind:  kinds.Name,
					Value: current,
				},
			}
			// if the field does not exist, add it to the selection set.
			parentField.Selections = append(parentField.Selections, newField)

			// if there are more fields nested, go deeper.
			if len(field) > 1 {
				newField.SelectionSet = &ast.SelectionSet{
					Kind: kinds.SelectionSet,
				}
				pushFieldToAst(field[1:], parentField.Selections[len(parentField.Selections)-1].(*ast.Field).GetSelectionSet())
			}
			return
		}
	}
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
