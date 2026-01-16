package llm

import (
	"testing"
)

func TestCountToken(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected int
	}{
		{
			name:     "empty string",
			input:    "",
			expected: 0,
		},
		{
			name:     "simple word",
			input:    "hello",
			expected: 1,
		},
		{
			name:     "two words",
			input:    "hello world",
			expected: 2,
		},
		{
			name:     "sentence",
			input:    "The quick brown fox jumps over the lazy dog",
			expected: 9,
		},
		{
			name:     "code snippet",
			input:    "package main\n\nfunc main() {\n\tfmt.Println(\"Hello\")\n}",
			expected: 13,
		},
		{
			name:     "JSON",
			input:    `{"name": "test", "value": 123}`,
			expected: 12,
		},
		{
			name:     "unicode Chinese",
			input:    "你好世界",
			expected: 5,
		},
		{
			name:     "emoji",
			input:    "👋🌍🚀",
			expected: 9,
		},
		{
			name:     "mixed content",
			input:    "Hello, 世界! 🌍",
			expected: 10,
		},
		{
			name:     "newlines",
			input:    "line1\nline2\nline3",
			expected: 8,
		},
		{
			name:     "tabs",
			input:    "a\tb\tc",
			expected: 3,
		},
		{
			name:     "spaces only",
			input:    "   ",
			expected: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CountToken(tt.input)
			if result != tt.expected {
				t.Errorf("CountToken(%q) = %d, want %d", tt.input, result, tt.expected)
			}
		})
	}
}

func TestCountToken_EmptyAndWhitespace(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"empty string", ""},
		{"single space", " "},
		{"multiple spaces", "   "},
		{"single tab", "\t"},
		{"multiple tabs", "\t\t\t"},
		{"single newline", "\n"},
		{"multiple newlines", "\n\n\n"},
		{"mixed whitespace", " \t\n \t\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CountToken(tt.input)
			if tt.input == "" && result != 0 {
				t.Errorf("CountToken(%q) = %d, want 0", tt.input, result)
			}
			if tt.input != "" && result < 1 {
				t.Errorf("CountToken(%q) = %d, want >= 1", tt.input, result)
			}
		})
	}
}

func TestCountToken_Punctuation(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"period", "."},
		{"comma", ","},
		{"exclamation", "!"},
		{"question", "?"},
		{"colon", ":"},
		{"semicolon", ";"},
		{"quotes", `"quote"`},
		{"parentheses", "(text)"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CountToken(tt.input)
			if result < 1 {
				t.Errorf("CountToken(%q) = %d, want >= 1", tt.input, result)
			}
		})
	}
}

func TestCountToken_Consistency(t *testing.T) {
	input := "This is a test sentence for token counting consistency."

	// Run multiple times to ensure consistent results
	var firstResult int
	for i := 0; i < 10; i++ {
		result := CountToken(input)
		if i == 0 {
			firstResult = result
		}
		if result != firstResult {
			t.Errorf("CountToken returned inconsistent results: first=%d, iteration %d=%d",
				firstResult, i, result)
		}
	}
}

func TestCountToken_English(t *testing.T) {
	tests := []struct {
		name  string
		input string
		min   int
		max   int
	}{
		{"single character", "a", 1, 1},
		{"two characters", "ab", 1, 1},
		{"short word hello", "hello", 1, 1},
		{"hello world", "hello world", 2, 2},
		{"sentence", "The quick brown fox jumps over the lazy dog", 9, 9},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CountToken(tt.input)
			if result < tt.min || result > tt.max {
				t.Errorf("CountToken(%q) = %d, want between %d and %d",
					tt.input, result, tt.min, tt.max)
			}
		})
	}
}

func TestCountToken_SpecialCharacters(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"at symbol", "@"},
		{"hashtag", "#"},
		{"dollar", "$"},
		{"percent", "%"},
		{"ampersand", "&"},
		{"asterisk", "*"},
		{"plus", "+"},
		{"equals", "="},
		{"URL", "https://example.com/path?query=value"},
		{"email", "user@example.com"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CountToken(tt.input)
			if result < 1 {
				t.Errorf("CountToken(%q) = %d, want >= 1", tt.input, result)
			}
		})
	}
}
