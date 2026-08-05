package yunhu

import (
	"reflect"
	"testing"
)

func TestParseGroupIDs(t *testing.T) {
	tests := []struct {
		input    string
		expected []string
	}{
		{"12345678", []string{"12345678"}},
		{"12345678,87654321", []string{"12345678", "87654321"}},
		{"12345678， 87654321", []string{"12345678", "87654321"}},
		{"", nil},
	}

	for _, tt := range tests {
		result := parseGroupIDs(tt.input)
		if !reflect.DeepEqual(result, tt.expected) {
			t.Errorf("parseGroupIDs(%q) = %v, expected %v", tt.input, result, tt.expected)
		}
	}
}
