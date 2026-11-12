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
