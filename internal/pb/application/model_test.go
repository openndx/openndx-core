// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Lanka Software Foundation

package application

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSelectedFieldRecords_Scan(t *testing.T) {
	tests := []struct {
		name    string
		value   interface{}
		want    SelectedFieldRecords
		wantErr bool
	}{
		{
			name:    "nil value",
			value:   nil,
			want:    SelectedFieldRecords{},
			wantErr: false,
		},
		{
			name:  "valid JSON bytes",
			value: []byte(`[{"fieldName":"field1","schemaId":"sch1"}]`),
			want: SelectedFieldRecords{
				{FieldName: "field1", SchemaID: "sch1"},
			},
			wantErr: false,
		},
		{
			name:  "valid JSON string",
			value: `[{"fieldName":"field2","schemaId":"sch2"}]`,
			want: SelectedFieldRecords{
				{FieldName: "field2", SchemaID: "sch2"},
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
			name:    "invalid JSON",
			value:   []byte(`invalid json`),
			want:    nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var sfr SelectedFieldRecords
			err := sfr.Scan(tt.value)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, sfr)
			}
		})
	}
}

func TestSelectedFieldRecords_Value(t *testing.T) {
	sfr := SelectedFieldRecords{
		{FieldName: "field1", SchemaID: "sch1"},
		{FieldName: "field2", SchemaID: "sch2"},
	}

	value, err := sfr.Value()
	assert.NoError(t, err)
	assert.NotNil(t, value)

	// Verify it's valid JSON
	var result SelectedFieldRecords
	err = json.Unmarshal(value.([]byte), &result)
	assert.NoError(t, err)
	assert.Equal(t, sfr, result)
}

func TestSelectedFieldRecords_GormDataType(t *testing.T) {
	var sfr SelectedFieldRecords
	assert.Equal(t, "jsonb", sfr.GormDataType())
}

func TestTableName_Application(t *testing.T) {
	a := Application{}
	assert.Equal(t, "applications", a.TableName())
}

func TestTableName_ApplicationSubmission(t *testing.T) {
	as := ApplicationSubmission{}
	assert.Equal(t, "application_submissions", as.TableName())
}
