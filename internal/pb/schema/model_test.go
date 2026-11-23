// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Lanka Software Foundation

package schema

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTableName_Schema(t *testing.T) {
	s := Schema{}
	assert.Equal(t, "schemas", s.TableName())
}

func TestTableName_SchemaSubmission(t *testing.T) {
	ss := SchemaSubmission{}
	assert.Equal(t, "schema_submissions", ss.TableName())
}
