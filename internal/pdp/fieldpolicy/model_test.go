package fieldpolicy

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func TestPolicy_TableName(t *testing.T) {
	p := Policy{}
	assert.Equal(t, "policy_metadata", p.TableName())
}

func TestPolicy_ToResponse(t *testing.T) {
	now := time.Now()
	displayName := "Test Display Name"
	description := "Test Description"
	owner := OwnerCitizen

	p := Policy{
		ID:                uuid.New(),
		SchemaID:          "schema-123",
		FieldName:         "person.fullName",
		DisplayName:       &displayName,
		Description:       &description,
		Source:            SourcePrimary,
		RequiresAllowlist: true,
		RequiresConsent:   true,
		Owner:             &owner,
		CreatedAt:         now,
		UpdatedAt:         now,
	}

	response := p.ToResponse()

	assert.Equal(t, p.ID.String(), response.ID)
	assert.Equal(t, p.SchemaID, response.SchemaID)
	assert.Equal(t, p.FieldName, response.FieldName)
	assert.Equal(t, p.DisplayName, response.DisplayName)
	assert.Equal(t, p.Description, response.Description)
	assert.Equal(t, p.Source, response.Source)
	assert.True(t, response.RequiresAllowlist)
	assert.True(t, response.RequiresConsent)
	assert.Equal(t, p.Owner, response.Owner)
	assert.Equal(t, p.CreatedAt.Format(time.RFC3339), response.CreatedAt)
	assert.Equal(t, p.UpdatedAt.Format(time.RFC3339), response.UpdatedAt)
}

func TestSource_Scan(t *testing.T) {
	tests := []struct {
		name    string
		value   any
		want    Source
		wantErr bool
	}{
		{
			name:    "nil value",
			value:   nil,
			want:    SourceFallback,
			wantErr: false,
		},
		{
			name:    "valid string - primary",
			value:   "primary",
			want:    SourcePrimary,
			wantErr: false,
		},
		{
			name:    "valid string - fallback",
			value:   "fallback",
			want:    SourceFallback,
			wantErr: false,
		},
		{
			name:    "invalid type",
			value:   123,
			want:    "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var s Source
			err := s.Scan(tt.value)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, s)
			}
		})
	}
}

func TestSource_Value(t *testing.T) {
	s := SourcePrimary
	value, err := s.Value()
	assert.NoError(t, err)
	assert.Equal(t, "primary", value)
}

func TestOwner_Scan(t *testing.T) {
	tests := []struct {
		name    string
		value   any
		want    Owner
		wantErr bool
	}{
		{
			name:    "nil value",
			value:   nil,
			want:    OwnerCitizen,
			wantErr: false,
		},
		{
			name:    "valid string - citizen",
			value:   "citizen",
			want:    OwnerCitizen,
			wantErr: false,
		},
		{
			name:    "invalid type",
			value:   123,
			want:    "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var o Owner
			err := o.Scan(tt.value)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, o)
			}
		})
	}
}

func TestOwner_Value(t *testing.T) {
	o := OwnerCitizen
	value, err := o.Value()
	assert.NoError(t, err)
	assert.Equal(t, "citizen", value)
}
