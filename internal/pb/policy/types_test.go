package policy

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

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
			name:    "invalid type",
			value:   "not bytes",
			want:    nil,
			wantErr: true,
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
	al := AllowList{
		"app1": {
			ExpiresAt: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			UpdatedAt: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		},
	}

	value, err := al.Value()
	assert.NoError(t, err)
	assert.NotNil(t, value)

	// Verify it's valid JSON
	var result AllowList
	err = json.Unmarshal(value.([]byte), &result)
	assert.NoError(t, err)
	assert.Equal(t, al, result)
}
