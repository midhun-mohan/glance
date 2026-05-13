package github

import (
	"strings"
	"testing"
)

func TestSanitizeForTerminal(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"plain ascii unchanged", "hello world", "hello world"},
		{"emoji unchanged", "ship it 🚀", "ship it 🚀"},
		{"newline preserved", "line1\nline2", "line1\nline2"},
		{"ESC (0x1b) replaced", "before\x1bafter", "before after"},
		{"clear-screen sequence neutralized", "\x1b[2J\x1b[Hattack", " [2J [Hattack"},
		{"OSC 52 clipboard sequence neutralized", "\x1b]52;c;ZXZpbA==\x07tail", " ]52;c;ZXZpbA== tail"},
		{"OSC 8 hyperlink neutralized", "\x1b]8;;evil\x07text\x1b]8;;\x07", " ]8;;evil text ]8;; "},
		{"null byte replaced", "user\x00name", "user name"},
		{"carriage return replaced", "line1\rline2", "line1 line2"},
		{"tab replaced", "col1\tcol2", "col1 col2"},
		{"bell replaced", "ding\x07dong", "ding dong"},
		{"backspace replaced", "abc\x08def", "abc def"},
		{"multiple controls collapsed individually", "\x1b\x1b\x1b", "   "},
		{"unicode high codepoints unchanged", "Ω π λ", "Ω π λ"},
		{"empty string", "", ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := sanitizeForTerminal(tc.in); got != tc.want {
				t.Errorf("sanitizeForTerminal(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestSanitizeSliceInPlace(t *testing.T) {
	in := []string{"clean", "with\x1bescape", "another\x00null"}
	out := sanitizeSliceInPlace(in)

	want := []string{"clean", "with escape", "another null"}
	if len(out) != len(want) {
		t.Fatalf("len = %d, want %d", len(out), len(want))
	}
	for i := range want {
		if out[i] != want[i] {
			t.Errorf("[%d] = %q, want %q", i, out[i], want[i])
		}
	}
	// Confirm in-place mutation: caller's slice is also updated.
	if &in[0] != &out[0] {
		t.Error("expected in-place mutation, got new backing array")
	}
}

func TestSanitizeErrorBody(t *testing.T) {
	t.Run("short body unchanged", func(t *testing.T) {
		got := sanitizeErrorBody([]byte(`{"message":"Not Found"}`))
		if got != `{"message":"Not Found"}` {
			t.Errorf("got %q", got)
		}
	})
	t.Run("escape bytes neutralized", func(t *testing.T) {
		got := sanitizeErrorBody([]byte("oops\x1b[2J\x07"))
		if got != "oops [2J " {
			t.Errorf("got %q", got)
		}
	})
	t.Run("truncated at 512 bytes", func(t *testing.T) {
		got := sanitizeErrorBody([]byte(strings.Repeat("a", 1000)))
		if len(got) != 512 {
			t.Errorf("len = %d, want 512", len(got))
		}
	})
}
