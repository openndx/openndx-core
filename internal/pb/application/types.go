// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Lanka Software Foundation

package application

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"fmt"

	"github.com/openndx/openndx-core/internal/pb/policy"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// SelectedFieldRecords represents an array of policy.SelectedFieldRecord with custom scanning
type SelectedFieldRecords []policy.SelectedFieldRecord

// Scan implements the sql.Scanner interface for SelectedFieldRecords
func (sfr *SelectedFieldRecords) Scan(value interface{}) error {
	if value == nil {
		*sfr = SelectedFieldRecords{}
		return nil
	}

	var bytes []byte
	switch v := value.(type) {
	case []byte:
		bytes = v
	case string:
		bytes = []byte(v)
	default:
		return fmt.Errorf("cannot scan %T into SelectedFieldRecords", value)
	}

	return json.Unmarshal(bytes, sfr)
}

// Value implements the driver.Valuer interface for SelectedFieldRecords
func (sfr *SelectedFieldRecords) Value() (driver.Value, error) {
	return json.Marshal(*sfr)
}

// GormDataType gorm common data type
func (SelectedFieldRecords) GormDataType() string {
	return "jsonb"
}

// GormValue implements the GormValuerInterface
func (sfr SelectedFieldRecords) GormValue(ctx context.Context, db *gorm.DB) clause.Expr {
	data, err := json.Marshal(sfr)
	if err != nil {
		// Panic on marshaling error to prevent silent data loss
		// JSON marshaling of SelectedFieldRecords should never fail under normal circumstances
		panic(fmt.Sprintf("Failed to marshal SelectedFieldRecords to JSON: %v", err))
	}

	// Use dialect-appropriate syntax
	// PostgreSQL uses ::jsonb cast, SQLite uses JSON() function or plain ?
	dialector := db.Name() // the Dialector's name: "postgres" or "sqlite"
	var sql string
	if dialector == "postgres" {
		sql = "?::jsonb"
	} else {
		// SQLite and other databases - use plain parameter
		// GORM will handle JSON encoding automatically
		sql = "?"
	}

	return clause.Expr{
		SQL:  sql,
		Vars: []interface{}{string(data)},
	}
}
