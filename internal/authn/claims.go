// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Lanka Software Foundation

package authn

import "strings"

// Claims is a read-only view of a validated token's claims. Its zero value is
// empty and safe to use. Names are exact, top-level keys, with no path expansion.
// Standard claim names are canonicalized on input (e.g. EMAIL becomes email);
// custom names remain case-sensitive. Conflicting standard names are rejected.
//
// Null and blank values remain present: use Lookup to distinguish them from
// missing claims and to inspect their JSON types. Returned collections are
// detached copies, including nested maps and slices.
type Claims struct {
	values map[string]any
}

// String returns a string-valued claim, or "" for a missing/null/wrong-type
// claim. It never coerces other JSON types to strings.
func (c Claims) String(name string) string {
	value, _ := c.values[name].(string)
	return value
}

// Strings returns a copy of an array of strings, or nil for a missing, null or
// wrong-type claim. Only the OAuth2 scope claim also accepts a whitespace-
// delimited string. Free text in any other claim is never split into grants.
// An empty array (or a blank/null scope) returns an empty, non-nil slice.
func (c Claims) Strings(name string) []string {
	if name == "scope" {
		if value, present := c.values[name]; present {
			switch value := value.(type) {
			case string:
				return strings.Fields(value)
			case nil:
				// Match spaceDelimitedScope's historical JSON-null behavior.
				return []string{}
			}
		}
	}
	value, ok := c.values[name].([]any)
	if !ok {
		return nil
	}
	result := make([]string, len(value))
	for i, item := range value {
		str, ok := item.(string)
		if !ok {
			return nil
		}
		result[i] = str
	}
	return result
}

// Lookup returns a detached JSON value and whether the exact name is present.
// A JSON null returns (nil, true); a missing claim returns (nil, false).
// Values have JSON-decoded Go types: string, float64, bool, []any,
// map[string]any, or nil.
func (c Claims) Lookup(name string) (any, bool) {
	value, ok := c.values[name]
	return cloneClaimValue(value), ok
}

func cloneClaimValue(value any) any {
	switch value := value.(type) {
	case map[string]any:
		result := make(map[string]any, len(value))
		for name, item := range value {
			result[name] = cloneClaimValue(item)
		}
		return result
	case []any:
		result := make([]any, len(value))
		for i, item := range value {
			result[i] = cloneClaimValue(item)
		}
		return result
	default:
		return value
	}
}
