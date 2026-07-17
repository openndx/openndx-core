package models

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestPolicyMetadata_TableName(t *testing.T) {
	pm := PolicyMetadata{}
	assert.Equal(t, "policy_metadata", pm.TableName())
}

func TestPolicyMetadata_BeforeCreate(t *testing.T) {
	tests := []struct {
		name    string
		pm      PolicyMetadata
		wantErr bool
		errMsg  string
	}{
		{
			name: "Valid - isOwner true, owner nil",
			pm: PolicyMetadata{
				IsOwner: true,
				Owner:   nil,
			},
			wantErr: false,
		},
		{
			name: "Valid - isOwner false, owner set",
			pm: PolicyMetadata{
				IsOwner: false,
				Owner:   ownerPtr(OwnerCitizen),
			},
			wantErr: false,
		},
		{
			name: "Invalid - isOwner false, owner nil",
			pm: PolicyMetadata{
				IsOwner: false,
				Owner:   nil,
			},
			wantErr: true,
			errMsg:  "owner must be specified when isOwner is false",
		},
		{
			name: "Invalid - isOwner true, owner set",
			pm: PolicyMetadata{
				IsOwner: true,
				Owner:   ownerPtr(OwnerCitizen),
			},
			wantErr: true,
			errMsg:  "must be null when isOwner is true",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
			if err != nil {
				t.Fatalf("Failed to connect to test database: %v", err)
			}
			err = tt.pm.BeforeCreate(db)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestPolicyMetadata_BeforeUpdate(t *testing.T) {
	tests := []struct {
		name    string
		pm      PolicyMetadata
		wantErr bool
		errMsg  string
	}{
		{
			name: "Valid - isOwner true, owner nil",
			pm: PolicyMetadata{
				IsOwner: true,
				Owner:   nil,
			},
			wantErr: false,
		},
		{
			name: "Invalid - isOwner false, owner nil",
			pm: PolicyMetadata{
				IsOwner: false,
				Owner:   nil,
			},
			wantErr: true,
			errMsg:  "owner must be specified when isOwner is false",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
			assert.NoError(t, err)
			err = tt.pm.BeforeUpdate(db)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestPolicyMetadata_ToResponse(t *testing.T) {
	now := time.Now()
	displayName := "Test Display Name"
	description := "Test Description"
	owner := OwnerCitizen

	pm := PolicyMetadata{
		ID:                uuid.New(),
		SchemaID:          "schema-123",
		FieldName:         "person.fullName",
		DisplayName:       &displayName,
		Description:       &description,
		Source:            SourcePrimary,
		IsOwner:           true,
		AccessControlType: AccessControlTypePublic,
		AllowList:         make(AllowList),
		Owner:             &owner,
		CreatedAt:         now,
		UpdatedAt:         now,
	}

	response := pm.ToResponse()

	assert.Equal(t, pm.ID.String(), response.ID)
	assert.Equal(t, pm.SchemaID, response.SchemaID)
	assert.Equal(t, pm.FieldName, response.FieldName)
	assert.Equal(t, pm.DisplayName, response.DisplayName)
	assert.Equal(t, pm.Description, response.Description)
	assert.Equal(t, pm.Source, response.Source)
	assert.Equal(t, pm.IsOwner, response.IsOwner)
	assert.Equal(t, pm.AccessControlType, response.AccessControlType)
	assert.Equal(t, pm.AllowList, response.AllowList)
	assert.Equal(t, pm.Owner, response.Owner)
	assert.Equal(t, pm.CreatedAt.Format(time.RFC3339), response.CreatedAt)
	assert.Equal(t, pm.UpdatedAt.Format(time.RFC3339), response.UpdatedAt)
}

func ownerPtr(o Owner) *Owner {
	return &o
}
