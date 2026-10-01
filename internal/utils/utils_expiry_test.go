package utils

import (
	"fmt"
	"math"
	"testing"
	"time"
)

func TestParseExpiryTime(t *testing.T) {
	maxDays := int64(math.MaxInt64) / int64(24*time.Hour)

	tests := []struct {
		name        string
		input       string
		expected    time.Duration
		expectError bool
	}{
		{name: "days", input: "30d", expected: 30 * 24 * time.Hour},
		{name: "hours", input: "1h", expected: time.Hour},
		{name: "minutes", input: "15m", expected: 15 * time.Minute},
		{name: "seconds", input: "45s", expected: 45 * time.Second},
		{name: "zero", input: "0h", expected: 0},
		{name: "max days", input: fmt.Sprintf("%dd", maxDays), expected: time.Duration(maxDays) * 24 * time.Hour},
		{name: "rejects non-numeric prefix", input: "1xh", expectError: true},
		{name: "rejects overflow days", input: "9223372037d", expectError: true},
		{name: "rejects days above max", input: fmt.Sprintf("%dd", maxDays+1), expectError: true},
		{name: "rejects empty", input: "", expectError: true},
		{name: "rejects unknown unit", input: "10w", expectError: true},
		{name: "rejects negative", input: "-1h", expectError: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseExpiryTime(tt.input)
			if tt.expectError {
				if err == nil {
					t.Fatalf("ParseExpiryTime(%q) = %v, want error", tt.input, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseExpiryTime(%q) unexpected error: %v", tt.input, err)
			}
			if got != tt.expected {
				t.Fatalf("ParseExpiryTime(%q) = %v, want %v", tt.input, got, tt.expected)
			}
		})
	}
}
