// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Lanka Software Foundation

package graphql

import "fmt"

type Response struct {
	Data   map[string]interface{} `json:"data"`
	Errors []interface{}          `json:"errors,omitempty"`
}

type JSONError struct {
	Message    string                 `json:"message"`
	Extensions map[string]interface{} `json:"extensions"`
}

// Error implements the error interface (required)
func (e *JSONError) Error() string {
	// Default string for logging
	return fmt.Sprintf("message=%s", e.Message)
}
