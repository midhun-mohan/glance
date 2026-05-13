package github

import "strings"

// sanitizeForTerminal replaces every C0 control byte (0x00-0x1F) except newline
// with a single space. Applied at the API boundary so PR titles, bodies,
// labels, branch names, author logins, and comment text from a malicious or
// compromised repo cannot inject ANSI escape sequences into the terminal
// renderer (lipgloss does not strip ANSI from its input content).
func sanitizeForTerminal(s string) string {
	return strings.Map(func(r rune) rune {
		if r < 0x20 && r != '\n' {
			return ' '
		}
		return r
	}, s)
}

// sanitizeSliceInPlace sanitizes every element of ss in place and returns ss
// for chaining at field-assignment sites.
func sanitizeSliceInPlace(ss []string) []string {
	for i, s := range ss {
		ss[i] = sanitizeForTerminal(s)
	}
	return ss
}

// sanitizeErrorBody truncates b to at most 512 bytes and sanitizes control
// bytes so a malicious or proxy-injected response body cannot inject escape
// sequences or unbounded text into error strings shown in the TUI.
func sanitizeErrorBody(b []byte) string {
	const cap = 512
	if len(b) > cap {
		b = b[:cap]
	}
	return sanitizeForTerminal(string(b))
}
