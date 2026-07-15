package services

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/OpenNDX/openndx-core/portal-backend/idp"
	"github.com/OpenNDX/openndx-core/portal-backend/v1/models"
	"github.com/stretchr/testify/assert"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// MockIDP is a fake identity provider for testing
type MockIDP struct {
	CreateUserFunc                  func(ctx context.Context, user *idp.User) (*idp.UserInfo, error)
	UpdateUserFunc                  func(ctx context.Context, userID string, user *idp.User) (*idp.UserInfo, error)
	DeleteUserFunc                  func(ctx context.Context, userID string) error
	AddMemberToGroupByGroupNameFunc func(ctx context.Context, groupName string, member *idp.GroupMember) (*string, error)
	RemoveMemberFromGroupFunc       func(ctx context.Context, groupID string, userID string) error
	// Missing methods from interface
	GetUserFunc            func(ctx context.Context, userID string) (*idp.UserInfo, error)
	GetGroupFunc           func(ctx context.Context, groupID string) (*idp.GroupInfo, error)
	GetGroupByNameFunc     func(ctx context.Context, groupName string) (*string, error)
	CreateGroupFunc        func(ctx context.Context, group *idp.Group) (*idp.GroupInfo, error)
	UpdateGroupFunc        func(ctx context.Context, groupID string, group *idp.Group) (*idp.GroupInfo, error)
	AddMemberToGroupFunc   func(ctx context.Context, groupID string, memberInfo *idp.GroupMember) error
	CreateApplicationFunc  func(ctx context.Context, app *idp.Application) (*string, error)
	DeleteApplicationFunc  func(ctx context.Context, applicationID string) error
	DeleteGroupFunc        func(ctx context.Context, groupID string) error
	GetApplicationInfoFunc func(ctx context.Context, applicationID string) (*idp.ApplicationInfo, error)
	GetApplicationOIDCFunc func(ctx context.Context, applicationID string) (*idp.ApplicationOIDCInfo, error)
}

func (m *MockIDP) CreateUser(ctx context.Context, user *idp.User) (*idp.UserInfo, error) {
	if m.CreateUserFunc != nil {
		return m.CreateUserFunc(ctx, user)
	}
	return &idp.UserInfo{Id: "idp_123", Email: user.Email}, nil
}

func (m *MockIDP) UpdateUser(ctx context.Context, userID string, user *idp.User) (*idp.UserInfo, error) {
	if m.UpdateUserFunc != nil {
		return m.UpdateUserFunc(ctx, userID, user)
	}
	return &idp.UserInfo{Id: userID, Email: user.Email}, nil
}

func (m *MockIDP) DeleteUser(ctx context.Context, userID string) error {
	if m.DeleteUserFunc != nil {
		return m.DeleteUserFunc(ctx, userID)
	}
	return nil
}

func (m *MockIDP) AddMemberToGroupByGroupName(ctx context.Context, groupName string, member *idp.GroupMember) (*string, error) {
	if m.AddMemberToGroupByGroupNameFunc != nil {
		return m.AddMemberToGroupByGroupNameFunc(ctx, groupName, member)
	}
	groupID := "group_123"
	return &groupID, nil
}

func (m *MockIDP) RemoveMemberFromGroup(ctx context.Context, groupID string, userID string) error {
	if m.RemoveMemberFromGroupFunc != nil {
		return m.RemoveMemberFromGroupFunc(ctx, groupID, userID)
	}
	return nil
}

// Implement other interface methods with stubs
func (m *MockIDP) GetUser(ctx context.Context, userID string) (*idp.UserInfo, error) { return nil, nil }

func (m *MockIDP) GetGroup(ctx context.Context, groupID string) (*idp.GroupInfo, error) {
	return nil, nil
}

func (m *MockIDP) GetGroupByName(ctx context.Context, groupName string) (*string, error) {
	return nil, nil
}

func (m *MockIDP) CreateGroup(ctx context.Context, group *idp.Group) (*idp.GroupInfo, error) {
	return nil, nil
}

func (m *MockIDP) UpdateGroup(ctx context.Context, groupID string, group *idp.Group) (*idp.GroupInfo, error) {
	return nil, nil
}

func (m *MockIDP) AddMemberToGroup(ctx context.Context, groupID string, memberInfo *idp.GroupMember) error {
	return nil
}

func (m *MockIDP) CreateApplication(ctx context.Context, app *idp.Application) (*string, error) {
	if m.CreateApplicationFunc != nil {
		return m.CreateApplicationFunc(ctx, app)
	}
	appID := "mock-idp-app-id"
	return &appID, nil
}

func (m *MockIDP) DeleteApplication(ctx context.Context, applicationID string) error {
	if m.DeleteApplicationFunc != nil {
		return m.DeleteApplicationFunc(ctx, applicationID)
	}
	return nil
}

func (m *MockIDP) DeleteGroup(ctx context.Context, groupID string) error { return nil }

func (m *MockIDP) GetApplicationInfo(ctx context.Context, applicationID string) (*idp.ApplicationInfo, error) {
	if m.GetApplicationInfoFunc != nil {
		return m.GetApplicationInfoFunc(ctx, applicationID)
	}
	return nil, nil
}

func (m *MockIDP) GetApplicationOIDC(ctx context.Context, applicationID string) (*idp.ApplicationOIDCInfo, error) {
	if m.GetApplicationOIDCFunc != nil {
		return m.GetApplicationOIDCFunc(ctx, applicationID)
	}
	return &idp.ApplicationOIDCInfo{
		ClientId:     "mock-client-id",
		ClientSecret: "mock-client-secret",
	}, nil
}

// setupMemberMockDB creates a mock database for testing
func setupMemberMockDB(t *testing.T) (*gorm.DB, sqlmock.Sqlmock, func()) {
	var db *sql.DB
	var mock sqlmock.Sqlmock
	var err error

	db, mock, err = sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}

	dialector := postgres.New(postgres.Config{
		Conn:       db,
		DriverName: "postgres",
	})

	gormDB, err := gorm.Open(dialector, &gorm.Config{
		SkipDefaultTransaction: true,
	})
	if err != nil {
		t.Fatalf("failed to open gorm db: %v", err)
	}

	cleanup := func() {
		db.Close()
	}

	return gormDB, mock, cleanup
}

func TestCreateMember_Success(t *testing.T) {
	// Arrange
	db, mock, cleanup := setupMemberMockDB(t)
	defer cleanup()

	mockIDP := &MockIDP{
		CreateUserFunc: func(ctx context.Context, user *idp.User) (*idp.UserInfo, error) {
			return &idp.UserInfo{
				Id:          "idp_123",
				Email:       user.Email,
				FirstName:   user.FirstName,
				PhoneNumber: user.PhoneNumber,
			}, nil
		},
		AddMemberToGroupByGroupNameFunc: func(ctx context.Context, groupName string, member *idp.GroupMember) (*string, error) {
			groupID := "group_123"
			return &groupID, nil
		},
	}

	service := NewMemberService(db, mockIDP)
	ctx := context.Background()

	req := &models.CreateMemberRequest{
		Name:        "John Doe",
		Email:       "john@example.com",
		PhoneNumber: "+1234567890",
	}

	now := time.Now()

	// Mock database INSERT
	// With SkipDefaultTransaction: true, GORM won't start a transaction for single create
	mock.ExpectQuery(`INSERT INTO "members"`).
		WillReturnRows(sqlmock.NewRows([]string{"member_id", "created_at", "updated_at"}).
			AddRow("mem_test", now, now))

	// Act
	result, err := service.CreateMember(ctx, req)

	// Assert
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, "John Doe", result.Name)
	assert.Equal(t, "john@example.com", result.Email)
	assert.Equal(t, "idp_123", result.IdpUserID)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestCreateMember_IDPCreateUserError(t *testing.T) {
	// Arrange
	db, _, cleanup := setupMemberMockDB(t)
	defer cleanup()

	mockIDP := &MockIDP{
		CreateUserFunc: func(ctx context.Context, user *idp.User) (*idp.UserInfo, error) {
			return nil, errors.New("IDP service unavailable")
		},
	}

	service := NewMemberService(db, mockIDP)
	ctx := context.Background()

	req := &models.CreateMemberRequest{
		Name:        "John Doe",
		Email:       "john@example.com",
		PhoneNumber: "+1234567890",
	}

	// Act
	result, err := service.CreateMember(ctx, req)

	// Assert
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "failed to create user in IDP")
}

func TestCreateMember_EmailMismatch_WithRollback(t *testing.T) {
	// Arrange
	db, _, cleanup := setupMemberMockDB(t)
	defer cleanup()

	mockIDP := &MockIDP{
		CreateUserFunc: func(ctx context.Context, user *idp.User) (*idp.UserInfo, error) {
			// Return user with different email (simulating mismatch)
			return &idp.UserInfo{
				Id:          "idp_123",
				Email:       "wrong@example.com",
				FirstName:   user.FirstName,
				PhoneNumber: user.PhoneNumber,
			}, nil
		},
		DeleteUserFunc: func(ctx context.Context, userID string) error {
			return nil
		},
	}

	service := NewMemberService(db, mockIDP)
	ctx := context.Background()

	req := &models.CreateMemberRequest{
		Name:        "John Doe",
		Email:       "john@example.com",
		PhoneNumber: "+1234567890",
	}

	// Act
	result, err := service.CreateMember(ctx, req)

	// Assert
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "IDP user email mismatch")
}

func TestCreateMember_GroupAssignmentError_WithRollback(t *testing.T) {
	// Arrange
	db, _, cleanup := setupMemberMockDB(t)
	defer cleanup()

	mockIDP := &MockIDP{
		CreateUserFunc: func(ctx context.Context, user *idp.User) (*idp.UserInfo, error) {
			return &idp.UserInfo{
				Id:          "idp_123",
				Email:       user.Email,
				FirstName:   user.FirstName,
				PhoneNumber: user.PhoneNumber,
			}, nil
		},
		AddMemberToGroupByGroupNameFunc: func(ctx context.Context, groupName string, member *idp.GroupMember) (*string, error) {
			return nil, errors.New("group assignment failed")
		},
		DeleteUserFunc: func(ctx context.Context, userID string) error {
			return nil
		},
	}

	service := NewMemberService(db, mockIDP)
	ctx := context.Background()

	req := &models.CreateMemberRequest{
		Name:        "John Doe",
		Email:       "john@example.com",
		PhoneNumber: "+1234567890",
	}

	// Act
	result, err := service.CreateMember(ctx, req)

	// Assert
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "failed to add user to group")
}

func TestCreateMember_DatabaseError_WithRollback(t *testing.T) {
	// Arrange
	db, mock, cleanup := setupMemberMockDB(t)
	defer cleanup()

	mockIDP := &MockIDP{
		CreateUserFunc: func(ctx context.Context, user *idp.User) (*idp.UserInfo, error) {
			return &idp.UserInfo{
				Id:          "idp_123",
				Email:       user.Email,
				FirstName:   user.FirstName,
				PhoneNumber: user.PhoneNumber,
			}, nil
		},
		AddMemberToGroupByGroupNameFunc: func(ctx context.Context, groupName string, member *idp.GroupMember) (*string, error) {
			groupID := "group_123"
			return &groupID, nil
		},
		RemoveMemberFromGroupFunc: func(ctx context.Context, groupID string, userID string) error {
			return nil
		},
		DeleteUserFunc: func(ctx context.Context, userID string) error {
			return nil
		},
	}

	service := NewMemberService(db, mockIDP)
	ctx := context.Background()

	req := &models.CreateMemberRequest{
		Name:        "John Doe",
		Email:       "john@example.com",
		PhoneNumber: "+1234567890",
	}

	// Mock database to return error
	mock.ExpectQuery(`INSERT INTO "members"`).
		WillReturnError(errors.New("database constraint violation"))

	// Act
	result, err := service.CreateMember(ctx, req)

	// Assert
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "failed to create member in database")
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestUpdateMember_Success(t *testing.T) {
	// Arrange
	db, mock, cleanup := setupMemberMockDB(t)
	defer cleanup()

	mockIDP := &MockIDP{
		UpdateUserFunc: func(ctx context.Context, userID string, user *idp.User) (*idp.UserInfo, error) {
			return &idp.UserInfo{
				Id:          userID,
				Email:       user.Email,
				FirstName:   user.FirstName,
				LastName:    user.LastName,
				PhoneNumber: user.PhoneNumber,
			}, nil
		},
	}

	service := NewMemberService(db, mockIDP)
	ctx := context.Background()

	memberID := "mem_123"
	newName := "Jane Doe"
	req := &models.UpdateMemberRequest{
		Name: &newName,
	}

	now := time.Now()

	// Mock SELECT to find member
	// GORM might generate: SELECT * FROM "members" WHERE member_id = $1 ORDER BY "members"."member_id" LIMIT 1
	mock.ExpectQuery(`SELECT .* FROM "members"`).
		WithArgs(memberID, 1).
		WillReturnRows(sqlmock.NewRows([]string{
			"member_id", "idp_user_id", "name", "email", "phone_number", "created_at", "updated_at",
		}).AddRow("mem_123", "idp_123", "John Doe", "john@example.com", "+1234567890", now, now))

	// Mock UPDATE
	mock.ExpectExec(`UPDATE "members"`).
		WillReturnResult(sqlmock.NewResult(1, 1))

	// Act
	result, err := service.UpdateMember(ctx, memberID, req)

	// Assert
	assert.NoError(t, err)
	if err != nil {
		return
	}
	assert.NotNil(t, result)
	if result == nil {
		return
	}
	assert.Equal(t, "Jane Doe", result.Name)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestUpdateMember_NotFound(t *testing.T) {
	// Arrange
	db, mock, cleanup := setupMemberMockDB(t)
	defer cleanup()

	service := NewMemberService(db, &MockIDP{})
	ctx := context.Background()

	memberID := "mem_nonexistent"
	newName := "Jane Doe"
	req := &models.UpdateMemberRequest{
		Name: &newName,
	}

	// Mock SELECT returning no rows
	mock.ExpectQuery(`SELECT .* FROM "members"`).
		WithArgs(memberID, 1).
		WillReturnError(gorm.ErrRecordNotFound)

	// Act
	result, err := service.UpdateMember(ctx, memberID, req)

	// Assert
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "member not found")
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestUpdateMember_IDPUpdateError_NoRollback(t *testing.T) {
	// Arrange
	db, mock, cleanup := setupMemberMockDB(t)
	defer cleanup()

	mockIDP := &MockIDP{
		UpdateUserFunc: func(ctx context.Context, userID string, user *idp.User) (*idp.UserInfo, error) {
			return nil, errors.New("IDP update failed")
		},
	}

	service := NewMemberService(db, mockIDP)
	ctx := context.Background()

	memberID := "mem_123"
	newName := "Jane Doe"
	req := &models.UpdateMemberRequest{
		Name: &newName,
	}

	now := time.Now()

	// Mock SELECT
	mock.ExpectQuery(`SELECT .* FROM "members"`).
		WithArgs(memberID, 1).
		WillReturnRows(sqlmock.NewRows([]string{
			"member_id", "idp_user_id", "name", "email", "phone_number", "created_at", "updated_at",
		}).AddRow("mem_123", "idp_123", "John Doe", "john@example.com", "+1234567890", now, now))

	// Act
	result, err := service.UpdateMember(ctx, memberID, req)

	// Assert
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "failed to update user in IDP")
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetMember_Success(t *testing.T) {
	// Arrange
	db, mock, cleanup := setupMemberMockDB(t)
	defer cleanup()

	service := NewMemberService(db, &MockIDP{})
	ctx := context.Background()

	memberID := "mem_123"
	now := time.Now()

	// Mock SELECT
	mock.ExpectQuery(`SELECT .* FROM "members"`).
		WithArgs(memberID, 1).
		WillReturnRows(sqlmock.NewRows([]string{
			"member_id", "idp_user_id", "name", "email", "phone_number", "created_at", "updated_at",
		}).AddRow("mem_123", "idp_123", "John Doe", "john@example.com", "+1234567890", now, now))

	// Act
	result, err := service.GetMember(ctx, memberID)

	// Assert
	assert.NoError(t, err)
	if err != nil {
		return
	}
	assert.NotNil(t, result)
	if result == nil {
		return
	}
	assert.Equal(t, "mem_123", result.MemberID)
	assert.Equal(t, "John Doe", result.Name)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetAllMembers_NoFilter(t *testing.T) {
	// Arrange
	db, mock, cleanup := setupMemberMockDB(t)
	defer cleanup()

	service := NewMemberService(db, &MockIDP{})
	ctx := context.Background()

	now := time.Now()

	// Mock SELECT all
	mock.ExpectQuery(`SELECT .* FROM "members"`).
		WillReturnRows(sqlmock.NewRows([]string{
			"member_id", "idp_user_id", "name", "email", "phone_number", "created_at", "updated_at",
		}).
			AddRow("mem_1", "idp_1", "John Doe", "john@example.com", "+1111111111", now, now).
			AddRow("mem_2", "idp_2", "Jane Doe", "jane@example.com", "+2222222222", now, now))

	// Act
	result, err := service.GetAllMembers(ctx, nil, nil)

	// Assert
	assert.NoError(t, err)
	assert.Len(t, result, 2)
	assert.Equal(t, "John Doe", result[0].Name)
	assert.Equal(t, "Jane Doe", result[1].Name)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetAllMembers_WithEmailFilter(t *testing.T) {
	// Arrange
	db, mock, cleanup := setupMemberMockDB(t)
	defer cleanup()

	service := NewMemberService(db, &MockIDP{})
	ctx := context.Background()

	email := "john@example.com"
	now := time.Now()

	// Mock filtered SELECT
	mock.ExpectQuery(`SELECT .* FROM "members"`).
		WillReturnRows(sqlmock.NewRows([]string{
			"member_id", "idp_user_id", "name", "email", "phone_number", "created_at", "updated_at",
		}).AddRow("mem_1", "idp_1", "John Doe", "john@example.com", "+1111111111", now, now))

	// Act
	result, err := service.GetAllMembers(ctx, nil, &email)

	// Assert
	assert.NoError(t, err)
	assert.Len(t, result, 1)
	assert.Equal(t, "john@example.com", result[0].Email)
	assert.NoError(t, mock.ExpectationsWereMet())
}
