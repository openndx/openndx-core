package models

import (
	"database/sql/driver"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestAccessControlType_Scan(t *testing.T) {
	tests := []struct {
		name    string
		value   interface{}
		want    AccessControlType
		wantErr bool
	}{
		{
			name:    "nil value",
			value:   nil,
			want:    AccessControlTypeRestricted,
			wantErr: false,
		},
		{
			name:    "valid string - public",
			value:   "public",
			want:    AccessControlTypePublic,
			wantErr: false,
		},
		{
			name:    "valid string - restricted",
			value:   "restricted",
			want:    AccessControlTypeRestricted,
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
			var act AccessControlType
			err := act.Scan(tt.value)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, act)
			}
		})
	}
}

func TestAccessControlType_Value(t *testing.T) {
	act := AccessControlTypePublic
	value, err := act.Value()
	assert.NoError(t, err)
	assert.Equal(t, "public", value)
}

func TestSource_Scan(t *testing.T) {
	tests := []struct {
		name    string
		value   interface{}
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
		value   interface{}
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

func TestAllowList_Scan(t *testing.T) {
	tests := []struct {
		name    string
		value   interface{}
		want    AllowList
		wantErr bool
	}{
		{
			name:    "nil value",
			value:   nil,
			want:    make(AllowList),
			wantErr: false,
		},
		{
			name:  "valid JSON bytes",
			value: []byte(`{"app1":{"expires_at":"2024-01-01T00:00:00Z","updated_at":"2024-01-01T00:00:00Z"}}`),
			want: AllowList{
				"app1": {
					ExpiresAt: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
					UpdatedAt: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
				},
			},
			wantErr: false,
		},
		{
			name:  "valid JSON string",
			value: `{"app1":{"expires_at":"2024-01-01T00:00:00Z","updated_at":"2024-01-01T00:00:00Z"}}`,
			want: AllowList{
				"app1": {
					ExpiresAt: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
					UpdatedAt: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
				},
			},
			wantErr: false,
		},
		{
			name:    "invalid type",
			value:   123,
			want:    nil,
			wantErr: true,
		},
		{
			name:    "empty bytes",
			value:   []byte{},
			want:    make(AllowList),
			wantErr: false,
		},
		{
			name:    "invalid JSON",
			value:   []byte(`invalid json`),
			want:    nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var al AllowList
			err := al.Scan(tt.value)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, al)
			}
		})
	}
}

func TestAllowList_Value(t *testing.T) {
	tests := []struct {
		name  string
		al    AllowList
		check func(t *testing.T, value driver.Value)
	}{
		{
			name: "empty allow list",
			al:   make(AllowList),
			check: func(t *testing.T, value driver.Value) {
				assert.NotNil(t, value)
				bytes, ok := value.([]byte)
				assert.True(t, ok)
				var result map[string]AllowListEntry
				err := json.Unmarshal(bytes, &result)
				assert.NoError(t, err)
				assert.Equal(t, 0, len(result))
			},
		},
		{
			name: "non-empty allow list",
			al: AllowList{
				"app1": {
					ExpiresAt: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
					UpdatedAt: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
				},
			},
			check: func(t *testing.T, value driver.Value) {
				assert.NotNil(t, value)
				bytes, ok := value.([]byte)
				assert.True(t, ok)
				var result AllowList
				err := json.Unmarshal(bytes, &result)
				assert.NoError(t, err)
				assert.Equal(t, 1, len(result))
				assert.NotNil(t, result["app1"])
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			value, err := tt.al.Value()
			assert.NoError(t, err)
			tt.check(t, value)
		})
	}
}
