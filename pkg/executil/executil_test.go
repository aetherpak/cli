package executil

import (
	"bytes"
	"strings"
	"testing"
)

func TestStreamWithPrefix(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		prefix   string
		expected string
	}{
		{
			name:     "regular lines",
			input:    "line1\nline2\n",
			prefix:   "pref |",
			expected: "pref | line1\npref | line2\n",
		},
		{
			name:     "carriage returns",
			input:    "line1\r\nline2\r\n",
			prefix:   "pref |",
			expected: "pref | line1\npref | line2\n",
		},
		{
			name:     "no trailing newline",
			input:    "line1\nline2",
			prefix:   "pref |",
			expected: "pref | line1\npref | line2\n",
		},
		{
			name:     "empty input",
			input:    "",
			prefix:   "pref |",
			expected: "",
		},
		{
			name:     "empty prefix",
			input:    "line1\nline2\n",
			prefix:   "",
			expected: "line1\nline2\n",
		},
		{
			name:     "long line",
			input:    strings.Repeat("a", 10000) + "\n",
			prefix:   "pref |",
			expected: "pref | " + strings.Repeat("a", 10000) + "\n",
		},
		{
			name:     "long line no newline",
			input:    strings.Repeat("b", 10000),
			prefix:   "pref |",
			expected: "pref | " + strings.Repeat("b", 10000) + "\n",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			StreamWithPrefix(strings.NewReader(tc.input), &buf, tc.prefix)
			actual := buf.String()
			if actual != tc.expected {
				t.Errorf("expected:\n%q\ngot:\n%q", tc.expected, actual)
			}
		})
	}
}
