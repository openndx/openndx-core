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
		{name: "days", input: "30d", expected: 30 * 24 * time.Hour},
		{name: "hours", input: "1h", expected: time.Hour},
		{name: "minutes", input: "15m", expected: 15 * time.Minute},
		{name: "seconds", input: "45s", expected: 45 * time.Second},
		{name: "rejects non-numeric prefix", input: "1xh", expectError: true},
		{name: "rejects overflow days", input: "9223372037d", expectError: true},
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
