package models

import (
	"encoding/json"
	"testing"
	"time"

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

func TestTableName_Member(t *testing.T) {
	m := Member{}
	assert.Equal(t, "members", m.TableName())
}

func TestTableName_Schema(t *testing.T) {
	s := Schema{}
	assert.Equal(t, "schemas", s.TableName())
}

func TestTableName_SchemaSubmission(t *testing.T) {
	ss := SchemaSubmission{}
	assert.Equal(t, "schema_submissions", ss.TableName())
}

func TestTableName_Application(t *testing.T) {
	a := Application{}
	assert.Equal(t, "applications", a.TableName())
}

func TestTableName_ApplicationSubmission(t *testing.T) {
	as := ApplicationSubmission{}
	assert.Equal(t, "application_submissions", as.TableName())
}

func TestFlexibleStringSlice_UnmarshalJSON(t *testing.T) {
	tests := []struct {
		name    string
		json    string
		want    FlexibleStringSlice
		wantErr bool
	}{
		{
			name:    "single string",
			json:    `"test"`,
			want:    FlexibleStringSlice{"test"},
			wantErr: false,
		},
		{
			name:    "string array",
			json:    `["test1", "test2"]`,
			want:    FlexibleStringSlice{"test1", "test2"},
			wantErr: false,
		},
		{
			name:    "empty string",
			json:    `""`,
			want:    nil,
			wantErr: true,
		},
		{
			name:    "empty array",
			json:    `[]`,
			want:    FlexibleStringSlice{},
			wantErr: false,
		},
		{
			name:    "invalid type",
			json:    `123`,
			want:    nil,
			wantErr: true,
		},
		{
			name:    "invalid string in array",
			json:    `["test", ""]`,
			want:    nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var f FlexibleStringSlice
			err := json.Unmarshal([]byte(tt.json), &f)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, f)
			}
		})
	}
}

func TestFlexibleStringSlice_ToStringSlice(t *testing.T) {
	f := FlexibleStringSlice{"a", "b"}
	assert.Equal(t, []string{"a", "b"}, f.ToStringSlice())
}
