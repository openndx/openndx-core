// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Lanka Software Foundation

package member

import (
	"context"
	"testing"

	"github.com/openndx/openndx-core/internal/pb/database/dbtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMemberIDByOuID(t *testing.T) {
	db := dbtest.SetupSQLiteDB(t, &Member{})
	service := NewService(db)
	ctx := context.Background()

	require.NoError(t, db.Create(&Member{
		MemberID:    "mem_1",
		Name:        "Test Member",
		Email:       "org@example.com",
		PhoneNumber: "1234567890",
		OuID:        "test-ou-id",
	}).Error)

	t.Run("organization unit with a member", func(t *testing.T) {
		id, err := service.MemberIDByOuID(ctx, "test-ou-id")
		assert.NoError(t, err)
		assert.Equal(t, "mem_1", id)
	})

	t.Run("organization unit without a member", func(t *testing.T) {
		id, err := service.MemberIDByOuID(ctx, "other-ou-id")
		assert.NoError(t, err)
		assert.Empty(t, id)
	})

	t.Run("no organization unit", func(t *testing.T) {
		id, err := service.MemberIDByOuID(ctx, "")
		assert.NoError(t, err)
		assert.Empty(t, id)
	})
}
