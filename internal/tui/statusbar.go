package tui

import (
	"fmt"
	"time"

	"github.com/charmbracelet/lipgloss"
)

func renderStatusBar(lastRefresh time.Time, loading bool, firstLoad bool, refreshInterval time.Duration, hourglassFrame int, width int) string {
	left := renderHelpBar(width)

	var right string
	if loading && firstLoad {
		hourglass := hourglassFrames[hourglassFrame%len(hourglassFrames)]
		right = spinnerStyle.Render(hourglass + " Loading...")
	} else if loading {
		right = spinnerStyle.Render("⟳ Refreshing...")
	} else if !lastRefresh.IsZero() {
		remaining := refreshInterval - time.Since(lastRefresh)
		if remaining < 0 {
			remaining = 0
		}
		hourglass := hourglassFrames[hourglassFrame%len(hourglassFrames)]
		right = ageStyle.Render(fmt.Sprintf("%s %s", hourglass, formatCountdown(remaining)))
	}

	rightWidth := lipgloss.Width(right)
	leftWidth := width - rightWidth - 2
	if leftWidth < 0 {
		leftWidth = 0
	}

	leftRendered := lipgloss.NewStyle().Width(leftWidth).Render(left)
	return statusBarStyle.Width(width).Render(
		lipgloss.JoinHorizontal(lipgloss.Top, leftRendered, right),
	)
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
