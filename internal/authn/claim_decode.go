// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Lanka Software Foundation

package authn

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

// UnmarshalJSON retains the entire payload while keeping the existing typed
// decoding rules for standard and registered claims. No view escapes until
// signature verification and all claim validation have succeeded.
func (c *tokenClaims) UnmarshalJSON(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	start, err := decoder.Token()
	if err != nil {
		return err
	}
	if start != json.Delim('{') {
		return fmt.Errorf("jwt claims must be an object")
	}
	payload := make(map[string]any)
	for decoder.More() {
		key, err := decoder.Token()
		if err != nil {
			return err
		}
		name := canonicalClaimName(key.(string))
		if _, exists := payload[name]; exists {
			return fmt.Errorf("jwt has duplicate claim %q", name)
		}
		var value any
		if err := decoder.Decode(&value); err != nil {
			return err
		}
		payload[name] = value
	}
	if _, err := decoder.Token(); err != nil {
		return err
	}
	// A distinct type avoids recursion into UnmarshalJSON. These typed values
	// are used only for validation, never retained as mutable identity copies.
	type typedClaims tokenClaims
	var typed typedClaims
	if err := json.Unmarshal(data, &typed); err != nil {
		return err
	}
	*c = tokenClaims(typed)
	c.payload = payload
	return nil
}

// canonicalClaimName preserves the historical case-insensitive binding of
// standard claims. Custom names are exact and whitespace is significant.
// EqualFold matches encoding/json's struct-tag case handling.
func canonicalClaimName(name string) string {
	for _, canonical := range standardClaimNames {
		if strings.EqualFold(name, canonical) {
			return canonical
		}
	}
	return name
}

// validatedClaims is called only after typed decoding, JWT validation, and
// roles validation. Preserve registered/required identifier values exactly as
// validated; normalize identity and custom strings as the previous API did.
func validatedClaims(c *tokenClaims) Claims {
	for name, value := range c.payload {
		switch name {
		case "sub", "client_id", "iss", "aud", "exp", "nbf", "iat", "jti":
			// Validation used these exact values; do not rewrite identifiers.
		case "scope":
			if _, ok := value.([]any); ok {
				// Preserve the existing scope array decoder's behavior, including
				// its treatment of null elements as empty strings.
				items := make([]any, len(c.Scopes))
				for i, scope := range c.Scopes {
					items[i] = scope
				}
				c.payload[name] = items
			}
		default:
			c.payload[name] = normalizeClaimValue(value)
		}
	}
	return Claims{values: c.payload}
}

// normalizeClaimValue trims the whitespace around every JSON string a claim
// value directly carries. Any other shape is returned unchanged, and nested
// objects are never rewritten.
func normalizeClaimValue(value any) any {
	switch v := value.(type) {
	case string:
		return strings.TrimSpace(v)
	case []any:
		// Keep the decoded JSON array shape for Lookup and Strings.
		trimmed := make([]any, len(v))
		for i, item := range v {
			s, ok := item.(string)
			if !ok {
				return value // not an array of strings: leave exactly as decoded
			}
			trimmed[i] = strings.TrimSpace(s)
		}
		return trimmed
	default:
		return value
	}
}
