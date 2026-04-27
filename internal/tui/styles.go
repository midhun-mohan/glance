package tui

import "github.com/charmbracelet/lipgloss"

var (
	// Colors
	primaryColor   = lipgloss.Color("#7C3AED")
	secondaryColor = lipgloss.Color("#06B6D4")
	successColor   = lipgloss.Color("#10B981")
	warningColor   = lipgloss.Color("#F59E0B")
	dangerColor    = lipgloss.Color("#EF4444")
	mutedColor     = lipgloss.Color("#6B7280")
	bgColor        = lipgloss.Color("#1F2937")
	headerBgColor  = lipgloss.Color("#111827")
	selectedBg     = lipgloss.Color("#374151")

	// Header
	headerStyle = lipgloss.NewStyle().
			Background(headerBgColor).
			Foreground(lipgloss.Color("#F9FAFB")).
			Bold(true).
			Padding(0, 1)

	// Tabs
	activeTabStyle = lipgloss.NewStyle().
			Foreground(primaryColor).
			Bold(true).
			Padding(0, 1).
			Border(lipgloss.NormalBorder(), false, false, true, false).
			BorderForeground(primaryColor)

	inactiveTabStyle = lipgloss.NewStyle().
				Foreground(mutedColor).
				Padding(0, 1)

	// PR list
	selectedPRStyle = lipgloss.NewStyle().
			Background(selectedBg).
			Foreground(lipgloss.Color("#F9FAFB"))

	normalPRStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#D1D5DB"))

	// Status indicators
	statusOpenStyle   = lipgloss.NewStyle().Foreground(successColor).Bold(true)
	statusDraftStyle  = lipgloss.NewStyle().Foreground(mutedColor)
	statusMergedStyle = lipgloss.NewStyle().Foreground(primaryColor)
	statusClosedStyle = lipgloss.NewStyle().Foreground(dangerColor)

	// Review status
	reviewApprovedStyle  = lipgloss.NewStyle().Foreground(successColor)
	reviewChangesStyle   = lipgloss.NewStyle().Foreground(dangerColor)
	reviewPendingStyle   = lipgloss.NewStyle().Foreground(warningColor)

	// Filter bar
	filterBarStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#D1D5DB")).
			Padding(0, 1)

	filterChipStyle = lipgloss.NewStyle().
			Foreground(secondaryColor).
			Bold(true)

	// Status bar
	statusBarStyle = lipgloss.NewStyle().
			Foreground(mutedColor).
			Padding(0, 1)

	// Help
	helpKeyStyle = lipgloss.NewStyle().
			Foreground(secondaryColor).
			Bold(true)

	helpDescStyle = lipgloss.NewStyle().
			Foreground(mutedColor)

	// Repo name
	repoStyle = lipgloss.NewStyle().
			Foreground(secondaryColor)

	// Age
	ageStyle = lipgloss.NewStyle().
			Foreground(mutedColor)

	// Author
	authorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#A78BFA"))

	// Title
	titleStyle = lipgloss.NewStyle().
			Bold(true)

	// Spinner / loading
	spinnerStyle = lipgloss.NewStyle().
			Foreground(primaryColor)

	// Help overlay
	helpOverlayStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(primaryColor).
				Padding(1, 2)

	// Empty state
	emptyStyle = lipgloss.NewStyle().
			Foreground(mutedColor).
			Italic(true).
			Padding(2, 4)

	// Screen box
	screenBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(primaryColor)

	// Detail view
	detailOverlayStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(primaryColor).
				Padding(1, 2)

	detailSectionStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("#F9FAFB"))

	detailBodyStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#9CA3AF"))

	// Diff styles
	diffAddStyle = lipgloss.NewStyle().Foreground(successColor)
	diffDelStyle = lipgloss.NewStyle().Foreground(dangerColor)
	diffHunkStyle = lipgloss.NewStyle().Foreground(secondaryColor).Bold(true)
	diffContextStyle = lipgloss.NewStyle().Foreground(mutedColor)

	// File status styles
	fileAddedStyle    = lipgloss.NewStyle().Foreground(successColor).Bold(true)
	fileDeletedStyle  = lipgloss.NewStyle().Foreground(dangerColor).Bold(true)
	fileModifiedStyle = lipgloss.NewStyle().Foreground(warningColor).Bold(true)
	fileRenamedStyle  = lipgloss.NewStyle().Foreground(secondaryColor).Bold(true)

	// Split-screen panel styles
	panelActiveBorder = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder(), true).
				BorderForeground(primaryColor)

	panelInactiveBorder = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder(), true).
				BorderForeground(lipgloss.Color("#374151"))

	panelHeaderSep = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#374151"))

	detailTabActive = lipgloss.NewStyle().
			Foreground(primaryColor).
			Bold(true)

	detailTabInactive = lipgloss.NewStyle().
				Foreground(mutedColor)

	// Confirmation dialog input
	confirmInputStyle = lipgloss.NewStyle().
				Border(lipgloss.NormalBorder()).
				BorderForeground(mutedColor).
				Padding(0, 1)

	// Unseen PR indicator
	unseenDotStyle = lipgloss.NewStyle().
			Foreground(dangerColor).
			Bold(true)

	// Diff cursor highlight
	diffCursorStyle = lipgloss.NewStyle().
			Background(lipgloss.Color("#374151")).
			Foreground(lipgloss.Color("#F9FAFB"))

	// Inline review comment styles
	commentHeaderStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#A78BFA")).
				Bold(true)

	commentBodyStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#9CA3AF"))
)
