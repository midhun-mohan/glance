package notify

import "testing"

func TestEscapeOsascript(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"plain", "hello world", "hello world"},
		{"double quotes", `say "hello"`, `say \"hello\"`},
		{"backslash", `path\to\file`, `path\\to\\file`},
		{"both", `"path\to"`, `\"path\\to\"`},
		{"empty", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := escapeOsascript(tt.input)
			if got != tt.want {
				t.Errorf("escapeOsascript(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestEscapePowerShell(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"plain", "hello world", "hello world"},
		{"single quotes", "it's done", "it''s done"},
		{"backtick literal", "use `this`", "use `this`"},
		{"dollar literal", "$var", "$var"},
		{"combined", "it's $100 `ok`", "it''s $100 `ok`"},
		{"empty", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := escapePowerShell(tt.input)
			if got != tt.want {
				t.Errorf("escapePowerShell(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
