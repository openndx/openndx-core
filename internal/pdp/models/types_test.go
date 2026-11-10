package models

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestGrantDurationType_ExpiresAt(t *testing.T) {
	from := time.Date(2026, 1, 31, 12, 0, 0, 0, time.UTC)

	got, ok := GrantDurationTypeOneMonth.ExpiresAt(from)
	assert.True(t, ok)
	assert.Equal(t, from.AddDate(0, 1, 0), got)

	got, ok = GrantDurationTypeOneYear.ExpiresAt(from)
	assert.True(t, ok)
	assert.Equal(t, from.AddDate(1, 0, 0), got)

	_, ok = GrantDurationType("7d").ExpiresAt(from)
	assert.False(t, ok)
}

func TestGrantStatus_IsValid(t *testing.T) {
	assert.True(t, GrantStatusActive.IsValid())
	assert.True(t, GrantStatusRevoked.IsValid())
	assert.False(t, GrantStatus("pending").IsValid())
	assert.False(t, GrantStatus("").IsValid())
}

func TestPolicyParams_Scan(t *testing.T) {
	tests := []struct {
		name    string
		value   interface{}
		want    PolicyParams
		wantErr bool
	}{
		{
			name:  "nil value",
			value: nil,
			want:  PolicyParams{},
		},
		{
			name:  "empty bytes",
			value: []byte{},
			want:  PolicyParams{},
		},
		{
			name:  "bytes",
			value: []byte(`{"purpose":"tax"}`),
			want:  PolicyParams{"purpose": "tax"},
		},
		{
			name:  "string",
			value: `{"maxAgeDays":30}`,
			want:  PolicyParams{"maxAgeDays": float64(30)},
		},
		{
			name:    "invalid JSON",
			value:   []byte(`{`),
			wantErr: true,
		},
		{
			name:    "invalid type",
			value:   123,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var pp PolicyParams
			err := pp.Scan(tt.value)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, pp)
			}
		})
	}
}

func TestPolicyParams_Value(t *testing.T) {
	value, err := PolicyParams(nil).Value()
	assert.NoError(t, err)
	assert.Equal(t, []byte("{}"), value)

	value, err = PolicyParams{"purpose": "tax"}.Value()
	assert.NoError(t, err)
	assert.JSONEq(t, `{"purpose":"tax"}`, string(value.([]byte)))
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
