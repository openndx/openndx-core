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

// AllowListEntry represents an entry in the allow list
type AllowListEntry struct {
	ExpiresAt time.Time `json:"expires_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// AllowList represents the JSONB allow list as a HashMap with custom scanning
// Key: application_id, Value: AllowListEntry
type AllowList map[string]AllowListEntry

// Scan implements the sql.Scanner interface for AllowList
func (al *AllowList) Scan(value interface{}) error {
	if value == nil {
		*al = make(AllowList)
		return nil
	}

	var bytes []byte
	switch v := value.(type) {
	case []byte:
		bytes = v
	case string:
		bytes = []byte(v)
	default:
		return fmt.Errorf("cannot scan %T into AllowList", value)
	}

	if len(bytes) == 0 {
		*al = make(AllowList)
		return nil
	}

	return json.Unmarshal(bytes, al)
}

// Value implements the driver.Valuer interface for AllowList
func (al AllowList) Value() (driver.Value, error) {
	if len(al) == 0 {
		return json.Marshal(map[string]AllowListEntry{})
	}
	return json.Marshal(al)
}
