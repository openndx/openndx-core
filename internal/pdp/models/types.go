package models

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
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

// PolicyParams holds a policy class's parameters for one field, stored as JSONB.
type PolicyParams map[string]any

// Scan implements the sql.Scanner interface for PolicyParams
func (pp *PolicyParams) Scan(value interface{}) error {
	var bytes []byte
	switch v := value.(type) {
	case nil:
	case []byte:
		bytes = v
	case string:
		bytes = []byte(v)
	default:
		return fmt.Errorf("cannot scan %T into PolicyParams", value)
	}

	*pp = make(PolicyParams)
	if len(bytes) == 0 {
		return nil
	}
	return json.Unmarshal(bytes, pp)
}

// Value implements the driver.Valuer interface for PolicyParams
func (pp PolicyParams) Value() (driver.Value, error) {
	if pp == nil {
		return json.Marshal(map[string]any{})
	}
	return json.Marshal(pp)
}

// Source represents the source enum
type Source string

const (
	SourcePrimary  Source = "primary"
	SourceFallback Source = "fallback"
)

// Scan implements the sql.Scanner interface for Source
func (s *Source) Scan(value interface{}) error {
	if value == nil {
		*s = SourceFallback
		return nil
	}
	if str, ok := value.(string); ok {
		*s = Source(str)
		return nil
	}
	return fmt.Errorf("cannot scan %T into Source", value)
}

// Value implements the driver.Valuer interface for Source
func (s Source) Value() (driver.Value, error) {
	return string(s), nil
}

// Owner represents the owner enum
type Owner string

const (
	OwnerCitizen Owner = "citizen"
)

// Scan implements the sql.Scanner interface for Owner
func (o *Owner) Scan(value interface{}) error {
	if value == nil {
		*o = OwnerCitizen
		return nil
	}
	if str, ok := value.(string); ok {
		*o = Owner(str)
		return nil
	}
	return fmt.Errorf("cannot scan %T into Owner", value)
}

// Value implements the driver.Valuer interface for Owner
func (o Owner) Value() (driver.Value, error) {
	return string(o), nil
}
