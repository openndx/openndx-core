// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Lanka Software Foundation

package member

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTableName_Member(t *testing.T) {
	m := Member{}
	assert.Equal(t, "members", m.TableName())
}
