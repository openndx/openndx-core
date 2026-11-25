// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Lanka Software Foundation

package graphql

type ArgMapping struct {
	TargetArgName string `json:"targetArgName" yaml:"targetArgName"`
	SchemaID      string `json:"schemaId" yaml:"schemaId"`
	SourceArgPath string `json:"sourceArgPath" yaml:"sourceArgPath"`
	TargetArgPath string `json:"targetArgPath" yaml:"targetArgPath"`
	// TargetArgType is the provider argument's GraphQL type, such as "ID!". When
	// empty, the consumer argument's type is used.
	TargetArgType string `json:"targetArgType,omitempty" yaml:"targetArgType,omitempty"`
}
