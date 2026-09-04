package utils

import (
	"testing"
	"time"
)

func TestParseExpiryTime(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expected    time.Duration
		expectError bool
	}{
		{name: "ISO period days", input: "P30D", expected: 30 * 24 * time.Hour, expectError: false},
		{name: "ISO period time hours", input: "PT1H", expected: 1 * time.Hour, expectError: false},
		{name: "ISO lowercase minutes", input: "pt15m", expected: 15 * time.Minute, expectError: false},
		{name: "ISO unsupported months", input: "P1M", expected: 0, expectError: true},
		{name: "valid days", input: "30d", expected: 30 * 24 * time.Hour, expectError: false},
		{name: "valid hours", input: "12h", expected: 12 * time.Hour, expectError: false},
		{name: "zero duration", input: "0s", expected: 0, expectError: false},
		{name: "whitespace padded", input: " 30d ", expected: 30 * 24 * time.Hour, expectError: false},
		{name: "empty string", input: "", expected: 0, expectError: true},
		{name: "whitespace only", input: " ", expected: 0, expectError: true},
		{name: "prefix only P", input: "P", expected: 0, expectError: true},
		{name: "prefix only PT", input: "PT", expected: 0, expectError: true},
		{name: "single unit character", input: "s", expected: 0, expectError: true},
		{name: "malformed input with letters in number", input: "1xh", expected: 0, expectError: true},
		{name: "missing numeric value", input: "d", expected: 0, expectError: true},
		{name: "unsupported time unit", input: "30y", expected: 0, expectError: true},
		{name: "negative value", input: "-5d", expected: 0, expectError: true},
		{name: "max valid days boundary", input: "106751d", expected: 106751 * 24 * time.Hour, expectError: false},
		{name: "min invalid days boundary overflow", input: "106752d", expected: 0, expectError: true},
		{name: "extreme int64 overflow", input: "99999999999999999999999d", expected: 0, expectError: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			duration, err := ParseExpiryTime(tt.input)
			if tt.expectError && err == nil {
				t.Errorf("expected error for input %q, got nil", tt.input)
			}
			if !tt.expectError && err != nil {
				t.Errorf("unexpected error for input %q: %v", tt.input, err)
			}
			if !tt.expectError && duration != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, duration)
			}
		})
	}
}
