package models

import (
	"time"
)

// GrantDurationType represents the grant type enum
type GrantDurationType string

const (
	GrantDurationTypeOneMonth GrantDurationType = "30d"
	GrantDurationTypeOneYear  GrantDurationType = "365d"
)

// ExpiresAt returns when a grant of this duration starting at from expires,
// or false for an unknown duration.
func (d GrantDurationType) ExpiresAt(from time.Time) (time.Time, bool) {
	switch d {
	case GrantDurationTypeOneMonth:
		return from.AddDate(0, 1, 0), true
	case GrantDurationTypeOneYear:
		return from.AddDate(1, 0, 0), true
	}
	return time.Time{}, false
}
