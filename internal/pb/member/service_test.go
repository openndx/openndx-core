// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Lanka Software Foundation

package member

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/openndx/openndx-core/internal/pb/database/dbtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// newSQLiteService creates a member Service backed by an in-memory SQLite database
func newSQLiteService(t *testing.T) (*Service, *gorm.DB) {
	db := dbtest.SetupSQLiteDB(t, &Member{})
	return NewService(db), db
}

func TestCreateMember_Success(t *testing.T) {
	service, db := newSQLiteService(t)

	req := &CreateMemberRequest{
		Name:        "Acme Org",
		Email:       "contact@acme.example",
		PhoneNumber: "+1234567890",
		OuID:        "ou-acme",
	}

	result, err := service.CreateMember(context.Background(), req)

	require.NoError(t, err)
	_, err = uuid.Parse(result.MemberID)
	assert.NoError(t, err, "member ID should be a plain UUID")
	assert.Equal(t, "Acme Org", result.Name)
	assert.Equal(t, "contact@acme.example", result.Email)
	assert.Equal(t, "+1234567890", result.PhoneNumber)
	assert.Equal(t, "ou-acme", result.OuID)

	var stored Member
	require.NoError(t, db.First(&stored, "member_id = ?", result.MemberID).Error)
	assert.Equal(t, "ou-acme", stored.OuID)
}

func TestCreateMember_WithoutContactDetails(t *testing.T) {
	service, _ := newSQLiteService(t)

	result, err := service.CreateMember(context.Background(), &CreateMemberRequest{
		Name: "Acme Org",
		OuID: "ou-acme",
	})

	require.NoError(t, err)
	assert.Empty(t, result.Email)
	assert.Empty(t, result.PhoneNumber)
	assert.Equal(t, "ou-acme", result.OuID)
}

func TestCreateMember_SharedContactEmail(t *testing.T) {
	service, _ := newSQLiteService(t)
	ctx := context.Background()

	// Two departments can share one contact mailbox
	for _, ouID := range []string{"ou-dept-a", "ou-dept-b"} {
		_, err := service.CreateMember(ctx, &CreateMemberRequest{
			Name:  "Department " + ouID,
			Email: "it@ministry.example",
			OuID:  ouID,
		})
		require.NoError(t, err)
	}
}

func TestCreateMember_EmptyOuID(t *testing.T) {
	service, db := newSQLiteService(t)

	req := &CreateMemberRequest{
		Name:        "Acme Org",
		Email:       "contact@acme.example",
		PhoneNumber: "+1234567890",
	}

	result, err := service.CreateMember(context.Background(), req)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "ouId must not be empty")

	var count int64
	require.NoError(t, db.Model(&Member{}).Count(&count).Error)
	assert.Zero(t, count)
}

func TestCreateMember_DuplicateOuID(t *testing.T) {
	service, _ := newSQLiteService(t)
	ctx := context.Background()

	_, err := service.CreateMember(ctx, &CreateMemberRequest{
		Name:        "Acme Org",
		Email:       "contact@acme.example",
		PhoneNumber: "+1234567890",
		OuID:        "ou-acme",
	})
	require.NoError(t, err)

	result, err := service.CreateMember(ctx, &CreateMemberRequest{
		Name:        "Other Org",
		Email:       "contact@other.example",
		PhoneNumber: "+1987654321",
		OuID:        "ou-acme",
	})

	assert.ErrorIs(t, err, ErrMemberAlreadyExists)
	assert.Nil(t, result)
}

func TestCreateMember_DatabaseError(t *testing.T) {
	db, mock, cleanup := dbtest.SetupMockDB(t)
	defer cleanup()

	service := NewService(db)

	mock.ExpectQuery(`INSERT INTO "members"`).
		WillReturnError(errors.New("database constraint violation"))

	result, err := service.CreateMember(context.Background(), &CreateMemberRequest{
		Name:        "Acme Org",
		Email:       "contact@acme.example",
		PhoneNumber: "+1234567890",
		OuID:        "ou-acme",
	})

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "failed to create member in database")
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestUpdateMember_Success(t *testing.T) {
	service, _ := newSQLiteService(t)
	ctx := context.Background()

	created, err := service.CreateMember(ctx, &CreateMemberRequest{
		Name:        "Acme Org",
		Email:       "contact@acme.example",
		PhoneNumber: "+1234567890",
		OuID:        "ou-acme",
	})
	require.NoError(t, err)

	newName := "Acme Corporation"
	newEmail := "data-exchange@acme.example"
	newPhone := "+1111111111"
	result, err := service.UpdateMember(ctx, created.MemberID, &UpdateMemberRequest{
		Name:        &newName,
		Email:       &newEmail,
		PhoneNumber: &newPhone,
	})

	require.NoError(t, err)
	assert.Equal(t, "Acme Corporation", result.Name)
	assert.Equal(t, "data-exchange@acme.example", result.Email)
	assert.Equal(t, "+1111111111", result.PhoneNumber)
	// The OU link is not updatable
	assert.Equal(t, "ou-acme", result.OuID)
}

func TestUpdateMember_NotFound(t *testing.T) {
	service, _ := newSQLiteService(t)

	newName := "Jane Doe"
	result, err := service.UpdateMember(context.Background(), "mem_nonexistent", &UpdateMemberRequest{
		Name: &newName,
	})

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "member not found")
}

func TestGetMember_Success(t *testing.T) {
	service, _ := newSQLiteService(t)
	ctx := context.Background()

	created, err := service.CreateMember(ctx, &CreateMemberRequest{
		Name:        "Acme Org",
		Email:       "contact@acme.example",
		PhoneNumber: "+1234567890",
		OuID:        "ou-acme",
	})
	require.NoError(t, err)

	result, err := service.GetMember(ctx, created.MemberID)

	require.NoError(t, err)
	assert.Equal(t, created.MemberID, result.MemberID)
	assert.Equal(t, "Acme Org", result.Name)
	assert.Equal(t, "ou-acme", result.OuID)
}

func TestGetAllMembers(t *testing.T) {
	service, _ := newSQLiteService(t)
	ctx := context.Background()

	for _, req := range []*CreateMemberRequest{
		{Name: "Acme Org", Email: "contact@acme.example", PhoneNumber: "+1111111111", OuID: "ou-acme"},
		{Name: "Other Org", Email: "contact@other.example", PhoneNumber: "+2222222222", OuID: "ou-other"},
	} {
		_, err := service.CreateMember(ctx, req)
		require.NoError(t, err)
	}

	t.Run("NoFilter", func(t *testing.T) {
		result, err := service.GetAllMembers(ctx, nil)
		require.NoError(t, err)
		assert.Len(t, result, 2)
	})

	t.Run("EmptyFilter", func(t *testing.T) {
		empty := ""
		result, err := service.GetAllMembers(ctx, &empty)
		require.NoError(t, err)
		assert.Len(t, result, 2)
	})

	t.Run("ByOuID", func(t *testing.T) {
		ouID := "ou-other"
		result, err := service.GetAllMembers(ctx, &ouID)
		require.NoError(t, err)
		require.Len(t, result, 1)
		assert.Equal(t, "Other Org", result[0].Name)
	})

	t.Run("UnknownOuID", func(t *testing.T) {
		ouID := "ou-unknown"
		result, err := service.GetAllMembers(ctx, &ouID)
		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.Empty(t, result)
	})
}
