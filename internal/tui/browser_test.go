package tui

import "testing"

func TestIsSafeBrowserURL(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want bool
	}{
		{"https github", "https://github.com/owner/repo/pull/1", true},
		{"http localhost", "http://localhost:8080/path", true},
		{"https uppercase scheme normalizes", "HTTPS://example.com", true},
		{"empty string", "", false},
		{"leading dash flag-style", "-a /Applications/Calculator.app", false},
		{"leading double-dash", "--launch=evil", false},
		{"file scheme", "file:///etc/passwd", false},
		{"javascript scheme", "javascript:alert(1)", false},
		{"data scheme", "data:text/html,<script>alert(1)</script>", false},
		{"smb scheme", "smb://attacker.example.com/share", false},
		{"vbscript scheme", "vbscript:msgbox(1)", false},
		{"ftp scheme", "ftp://example.com/file", false},
		{"protocol-relative", "//example.com/path", false},
		{"no scheme bare path", "/etc/passwd", false},
		{"https without host", "https://", false},
		{"http without host", "http:///path", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := isSafeBrowserURL(tc.url); got != tc.want {
				t.Errorf("isSafeBrowserURL(%q) = %v, want %v", tc.url, got, tc.want)
			}
		})
	}
}
