// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Lanka Software Foundation

package federator

import (
	stderrors "errors"

	"github.com/openndx/openndx-core/internal/oe/internals/errors"
	"github.com/openndx/openndx-core/internal/oe/pkg/graphql"
	"github.com/vektah/gqlparser/v2/ast"
	"github.com/vektah/gqlparser/v2/gqlerror"
	"github.com/vektah/gqlparser/v2/parser"
	"github.com/vektah/gqlparser/v2/validator"
)

// parseQuery parses a consumer query. On a syntax error it returns the error
// response to send instead.
func parseQuery(query string) (*ast.QueryDocument, *graphql.Response) {
	doc, err := parser.ParseQuery(&ast.Source{Name: "Query", Input: query})
	if err != nil {
		var gqlErr *gqlerror.Error
		if !stderrors.As(err, &gqlErr) {
			gqlErr = gqlerror.Wrap(err)
		}
		return nil, invalidQueryResponse(gqlerror.List{gqlErr})
	}
	return doc, nil
}

// validateQuery validates query against schema with the GraphQL spec's
// validation rules. It returns the error response to send when the query is invalid.
func validateQuery(schema *ast.Schema, query *ast.QueryDocument) *graphql.Response {
	if errs := validator.ValidateWithRules(schema, query, nil); len(errs) > 0 {
		return invalidQueryResponse(errs)
	}
	return nil
}

// invalidQueryResponse reports errs as GraphQL errors with their locations and
// the BAD_REQUEST code.
func invalidQueryResponse(errs gqlerror.List) *graphql.Response {
	reported := make([]interface{}, 0, len(errs))
	for _, err := range errs {
		reported = append(reported, queryError(err))
	}
	return &graphql.Response{Data: nil, Errors: reported}
}

func queryError(err *gqlerror.Error) map[string]interface{} {
	reported := map[string]interface{}{
		"message":    err.Message,
		"extensions": map[string]interface{}{"code": errors.CodeBadRequest},
	}
	if len(err.Locations) > 0 {
		locations := make([]map[string]int, 0, len(err.Locations))
		for _, location := range err.Locations {
			locations = append(locations, map[string]int{"line": location.Line, "column": location.Column})
		}
		reported["locations"] = locations
	}
	return reported
}
