// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Lanka Software Foundation

package federator

import (
	"fmt"
	"slices"
	"strings"

	"github.com/openndx/openndx-core/internal/oe/federator/operation"
	"github.com/openndx/openndx-core/internal/oe/internals/errors"
	"github.com/openndx/openndx-core/internal/oe/pkg/graphql"
)

// merge builds the consumer response for op from the results of p's fetches.
// It depends only on its arguments, so the results may come from any executor.
//
// Values are placed by response key. A field served by a failed fetch is null
// with an error at its path, and a null in a non-null field propagates to the
// nearest nullable ancestor, as the spec's value completion does. Errors in
// provider responses are passed on.
func merge(op *operation.Operation, p *plan, results map[*fetch]fetchResult) graphql.Response {
	m := &merger{plan: p, results: results}
	data, failed := m.object(op.Fields, "Query", nil, nil)
	if failed {
		data = nil
	}
	m.providerErrors()
	return graphql.Response{Data: data, Errors: m.errors}
}

type merger struct {
	plan    *plan
	results map[*fetch]fetchResult
	errors  []interface{}
}

// itemScope is the provider list item that fields inside a consumer list field
// are read from.
type itemScope struct {
	item any
	// declared is the list's providerField.
	declared string
}

// object completes fields on typeName. failed reports that a non-null field is
// null, so the object itself must be null.
func (m *merger) object(fields []*operation.Field, typeName string, scope *itemScope, path []any) (map[string]interface{}, bool) {
	out := make(map[string]interface{}, len(fields))
	for _, field := range fields {
		value, failed := m.value(field, typeName, scope, path)
		if failed {
			return nil, true
		}
		out[field.ResponseKey] = value
	}
	return out, false
}

// value resolves field on parentType. failed reports a null that must propagate
// to the parent; its error has already been recorded.
func (m *merger) value(field *operation.Field, parentType string, scope *itemScope, parentPath []any) (any, bool) {
	path := append(slices.Clip(parentPath), field.ResponseKey)
	if field.Name == "__typename" {
		return parentType, false
	}

	info := field.SourceInfo
	isList := field.Definition.Type.Elem != nil
	hasSelections := len(field.Selections) > 0
	switch {
	case info != nil && (isList || !hasSelections):
		var raw any
		if scope != nil {
			raw = valueAt(scope.item, relativePath(info.ProviderField, scope.declared))
		} else {
			f := m.plan.sources[field]
			result := m.results[f]
			if result.err != nil {
				m.fieldError(fmt.Sprintf("Failed to fetch data from provider for schema %s", f.schemaID), errors.CodeProviderError, path)
				return m.complete(field, parentType, nil, true, path)
			}
			raw = valueAt(map[string]interface{}(result.response.Data), info.ProviderField)
		}
		if isList && hasSelections {
			return m.list(field, parentType, raw, info.ProviderField, path)
		}
		return m.complete(field, parentType, raw, false, path)

	case hasSelections:
		object, failed := m.object(field.Selections, field.Definition.Type.Name(), scope, path)
		if failed {
			return m.complete(field, parentType, nil, true, path)
		}
		return m.complete(field, parentType, object, false, path)

	default:
		// A leaf without @sourceInfo has no source.
		return m.complete(field, parentType, nil, false, path)
	}
}

// list completes a list field whose items are read relative to declared.
func (m *merger) list(field *operation.Field, parentType string, raw any, declared string, path []any) (any, bool) {
	if raw == nil {
		return m.complete(field, parentType, nil, false, path)
	}
	items, ok := raw.([]interface{})
	if !ok {
		m.fieldError(fmt.Sprintf("Expected a list from the provider, got %T", raw), errors.CodeProviderError, path)
		return m.complete(field, parentType, nil, true, path)
	}

	elemType := field.Definition.Type.Elem
	out := make([]interface{}, 0, len(items))
	for i, item := range items {
		itemPath := append(slices.Clip(path), i)
		var value any
		if item != nil {
			object, failed := m.object(field.Selections, elemType.Name(), &itemScope{item: item, declared: declared}, itemPath)
			if !failed {
				value = object
			} else if elemType.NonNull {
				return m.complete(field, parentType, nil, true, path)
			}
		} else if elemType.NonNull {
			m.fieldError(fmt.Sprintf("Cannot return null for non-nullable list item of %s.%s.", parentType, field.Name), "", itemPath)
			return m.complete(field, parentType, nil, true, path)
		}
		out = append(out, value)
	}
	return m.complete(field, parentType, out, false, path)
}

// complete returns value for field, propagating a null in a non-null field.
// reported is true when the null's error has already been recorded.
func (m *merger) complete(field *operation.Field, parentType string, value any, reported bool, path []any) (any, bool) {
	if value != nil {
		return value, false
	}
	if !field.Definition.Type.NonNull {
		return nil, false
	}
	if !reported {
		m.fieldError(fmt.Sprintf("Cannot return null for non-nullable field %s.%s.", parentType, field.Name), "", path)
	}
	return nil, true
}

// providerErrors passes on the errors in provider responses. Each is placed at
// the first consumer field the fetch serves whose provider path overlaps the
// error's path; an error without a matching field has no path.
func (m *merger) providerErrors() {
	for _, f := range m.plan.fetches {
		result, ok := m.results[f]
		if !ok || result.err != nil {
			continue
		}
		for _, raw := range result.response.Errors {
			message, providerPath := providerError(raw)
			var path []any
			for _, field := range f.consumerFields {
				if pathsOverlap(providerPath, strings.Split(field.SourceInfo.ProviderField, ".")) {
					path = slices.Clone(stringsToAny(field.Path))
					break
				}
			}
			m.fieldError(fmt.Sprintf("Provider for schema %s: %s", f.schemaID, message), errors.CodeProviderError, path)
		}
	}
}

func (m *merger) fieldError(message, code string, path []any) {
	reported := map[string]interface{}{"message": message}
	if len(path) > 0 {
		reported["path"] = slices.Clone(path)
	}
	if code != "" {
		reported["extensions"] = map[string]interface{}{"code": code}
	}
	m.errors = append(m.errors, reported)
}

// providerError reads the message and the leading field names of the path of a
// GraphQL error decoded from a provider response.
func providerError(raw interface{}) (string, []string) {
	reported, ok := raw.(map[string]interface{})
	if !ok {
		return fmt.Sprint(raw), nil
	}
	message, _ := reported["message"].(string)
	rawPath, _ := reported["path"].([]interface{})
	var path []string
	for _, segment := range rawPath {
		name, ok := segment.(string)
		if !ok {
			break
		}
		path = append(path, name)
	}
	return message, path
}

// pathsOverlap reports whether one path is the start of the other. An empty
// path overlaps nothing.
func pathsOverlap(a, b []string) bool {
	if len(a) == 0 || len(b) == 0 {
		return false
	}
	n := min(len(a), len(b))
	return slices.Equal(a[:n], b[:n])
}

func stringsToAny(path []string) []any {
	out := make([]any, len(path))
	for i, s := range path {
		out[i] = s
	}
	return out
}

// valueAt returns the value at the dot-separated path in data, or nil when a
// segment is missing. A list along the path yields the value for each item.
func valueAt(data any, path string) any {
	return valueAtSegments(data, strings.Split(path, "."))
}

func valueAtSegments(data any, segments []string) any {
	if len(segments) == 0 {
		return data
	}
	switch current := data.(type) {
	case map[string]interface{}:
		return valueAtSegments(current[segments[0]], segments[1:])
	case []interface{}:
		values := make([]interface{}, 0, len(current))
		for _, item := range current {
			values = append(values, valueAtSegments(item, segments))
		}
		return values
	default:
		return nil
	}
}
