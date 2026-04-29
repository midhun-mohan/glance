package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

type helpEntry struct {
	key  string
	desc string
}

var helpEntries = []helpEntry{
	{"↑/k", "Move up"},
	{"↓/j", "Move down"},
	{"←/h", "Previous page"},
	{"→/l", "Next page"},
	{"Tab", "Next section"},
	{"Shift+Tab", "Previous section"},
	{"1-5", "Jump to section"},
	{"Enter", "PR details"},
	{"o", "Open PR / check in browser"},
	{"y", "Copy PR URL"},
	{"c", "Comment (line or general)"},
	{"A", "Approve PR"},
	{"X", "Request changes"},
	{"M", "Merge PR (squash)"},
	{"c", "Collapse/expand group"},
	{"E", "Expand all groups"},
	{"B", "Browse any PR"},
	{"/", "Search / filter"},
	{"Esc", "Clear filter"},
	{"r", "Refresh"},
	{"p", "Filter presets"},
	{"?", "Toggle help"},
	{"q", "Quit"},
}

func renderHelp(width, height int) string {
	title := lipgloss.NewStyle().
		Bold(true).
		Foreground(primaryColor).
		Render("Keybindings")

	var lines []string
	lines = append(lines, title)
	lines = append(lines, "")

	for _, entry := range helpEntries {
		key := helpKeyStyle.Width(12).Render(entry.key)
		desc := helpDescStyle.Render(entry.desc)
		lines = append(lines, "  "+key+" "+desc)
	}

	content := strings.Join(lines, "\n")

	maxWidth := 40
	if width-4 < maxWidth {
		maxWidth = width - 4
	}
	overlay := helpOverlayStyle.
		Width(maxWidth).
		Render(content)

	// Center the overlay
	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, overlay)
}

func renderHelpBar(width int) string {
	hints := []helpEntry{
		{"↑↓", "navigate"},
		{"←→", "page"},
		{"Tab", "section"},
		{"Enter", "details"},
		{"o", "open"},
		{"y", "copy"},
		{"c", "collapse"},
		{"B", "browse"},
		{"/", "search"},
		{"r", "refresh"},
		{"?", "help"},
		{"q", "quit"},
	}

	var parts []string
	for _, h := range hints {
		key := helpKeyStyle.Render(h.key)
		desc := helpDescStyle.Render(h.desc)
		parts = append(parts, key+" "+desc)
	}

	return strings.Join(parts, "  ")
}