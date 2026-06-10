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
		{
			name:     "extremely long line triggers flush at 64KB",
			input:    strings.Repeat("c", 70000),
			prefix:   "pref |",
			expected: "pref | " + strings.Repeat("c", 65536) + "\npref | " + strings.Repeat("c", 4464) + "\n",
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

func TestStreamToTargets(t *testing.T) {
	input := "hello\nworld\r\nfinal line"
	var buf1, buf2 bytes.Buffer

	targets := []StreamTarget{
		{Writer: &buf1, Prefix: "W1 |"},
		{Writer: &buf2, Prefix: "W2 |"},
	}

	StreamToTargets(strings.NewReader(input), targets...)

	expected1 := "W1 | hello\nW1 | world\nW1 | final line\n"
	expected2 := "W2 | hello\nW2 | world\nW2 | final line\n"

	if actual1 := buf1.String(); actual1 != expected1 {
		t.Errorf("buf1 mismatch:\nexpected: %q\ngot: %q", expected1, actual1)
	}
	if actual2 := buf2.String(); actual2 != expected2 {
		t.Errorf("buf2 mismatch:\nexpected: %q\ngot: %q", expected2, actual2)
	}
}
