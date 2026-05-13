package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

// renderShortcutsBar renders the top row of the footer: the shortcut keys
// centered across the full width.
func renderShortcutsBar(width int) string {
	bar := renderHelpBar(width)
	// statusBarStyle has Padding(0, 1) which adds 2 columns to the rendered
	// width, so center within width-2 to keep the final output exactly `width`
	// columns. Otherwise the footer overflows the screen box and pushes the
	// right border off the terminal.
	inner := width - 2
	if inner < 0 {
		inner = 0
	}
	centered := lipgloss.PlaceHorizontal(inner, lipgloss.Center, bar)
	return statusBarStyle.Render(centered)
}

// renderFooterSeparator renders a thin horizontal rule between the shortcuts
// row and the info row.
func renderFooterSeparator(width int) string {
	if width <= 0 {
		return ""
	}
	return lipgloss.NewStyle().Foreground(mutedColor).Render(strings.Repeat("─", width))
}

// renderInfoBar renders the bottom row of the footer:
// glance | section: N | page X/Y | last refreshed Nm ago | ⟳ refresh in 4m30s
// All segments separated by " | ".
func renderInfoBar(
	appName string,
	sectionLabel string,
	sectionCount int,
	page int,
	pages int,
	lastRefresh time.Time,
	loading bool,
	firstLoad bool,
	refreshInterval time.Duration,
	hourglassFrame int,
	width int,
) string {
	sep := ageStyle.Render(" | ")

	var segs []string
	segs = append(segs, helpKeyStyle.Render(appName))

	if sectionLabel != "" {
		segs = append(segs, ageStyle.Render(fmt.Sprintf("%s: %d", sectionLabel, sectionCount)))
	}

	if pages > 1 {
		segs = append(segs, ageStyle.Render(fmt.Sprintf("page %d/%d", page+1, pages)))
	}

	if !lastRefresh.IsZero() {
		segs = append(segs, ageStyle.Render("last refreshed "+lastRefresh.Format("15:04:05")))
	}

	// Loading/countdown segment
	var right string
	if loading && firstLoad {
		hg := hourglassFrames[hourglassFrame%len(hourglassFrames)]
		right = spinnerStyle.Render(hg + " loading...")
	} else if loading {
		right = spinnerStyle.Render("⟳ refreshing...")
	} else if !lastRefresh.IsZero() {
		remaining := refreshInterval - time.Since(lastRefresh)
		if remaining < 0 {
			remaining = 0
		}
		hg := hourglassFrames[hourglassFrame%len(hourglassFrames)]
		right = ageStyle.Render(fmt.Sprintf("%s %s", hg, formatCountdown(remaining)))
	}
	if right != "" {
		segs = append(segs, right)
	}

	line := strings.Join(segs, sep)
	inner := width - 2
	if inner < 0 {
		inner = 0
	}
	centered := lipgloss.PlaceHorizontal(inner, lipgloss.Center, line)
	return statusBarStyle.Render(centered)
}

func formatCountdown(d time.Duration) string {
	if d <= 0 {
		return "refreshing..."
	}
	secs := int(d.Seconds())
	if secs < 60 {
		return fmt.Sprintf("refresh in %ds", secs)
	}
	mins := secs / 60
	secs = secs % 60
	return fmt.Sprintf("refresh in %dm%02ds", mins, secs)
}
