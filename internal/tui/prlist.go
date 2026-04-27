package tui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/midhun-mohan/glance/internal/github"
)

// Fixed column widths
const (
	colStatus = 2  // ● ◐ ○
	colNumber = 7  // #12345
	colAuthor = 16
	colAge    = 9  // "3mo ago"
	colReview = 3  // ✓ ✗ ⏳
	colChecks = 3  // CI: ✓ ✗ ●
	colGap    = 3  // gaps between columns
)

var (
	repoHeaderStyle = lipgloss.NewStyle().
		Foreground(secondaryColor).
		Bold(true)

	colHeaderStyle = lipgloss.NewStyle().
		Foreground(mutedColor).
		Bold(true)

	separatorStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("#374151"))

	collapsedSelectedStyle = lipgloss.NewStyle().
		Background(selectedBg).
		Foreground(secondaryColor).
		Bold(true)
)

// displayItem represents a navigable item in the PR list: either a PR row or a collapsed group header.
type displayItem struct {
	isPR     bool
	pr       github.PullRequest // valid when isPR
	repoName string             // valid when !isPR (collapsed header)
	count    int                // valid when !isPR
}

func titleColWidth(totalWidth int) int {
	fixed := colStatus + colNumber + colAuthor + colAge + colReview + colChecks + colGap
	w := totalWidth - fixed - 7 // 7 = leading indent (4) + trailing pad (2) + extra gap
	if w < 15 {
		w = 15
	}
	return w
}

func renderPRList(items []displayItem, cursor int, width int, section github.Section, unseenPRs map[string]bool) string {
	if len(items) == 0 {
		return emptyStyle.Render("No pull requests in this section")
	}

	tw := titleColWidth(width)

	// Column header shown once at top
	var rows []string
	rows = append(rows, renderColumnHeader(tw, section))

	lastRepo := ""
	groupIndex := 0

	for i, item := range items {
		if item.isPR {
			// New expanded group header when repo changes
			if item.pr.Repository != lastRepo {
				if groupIndex > 0 {
					rows = append(rows, "")
				}
				rows = append(rows, repoHeaderStyle.Render(" ▾ "+item.pr.Repository))
				lastRepo = item.pr.Repository
				groupIndex++
			}
			isUnseen := unseenPRs[item.pr.URL]
			row := renderPRRow(item.pr, tw, section, isUnseen)
			if i == cursor {
				rows = append(rows, padAndHighlight(row, width, selectedPRStyle))
			} else {
				rows = append(rows, normalPRStyle.Render(row))
			}
		} else {
			// Collapsed group header
			if groupIndex > 0 {
				rows = append(rows, "")
			}
			header := fmt.Sprintf(" ▸ %s (%d)", item.repoName, item.count)
			if i == cursor {
				rows = append(rows, padAndHighlight(header, width, collapsedSelectedStyle))
			} else {
				rows = append(rows, repoHeaderStyle.Render(header))
			}
			lastRepo = item.repoName
			groupIndex++
		}
	}

	return strings.Join(rows, "\n")
}

// padAndHighlight strips inner ANSI codes from the row so the style's background
// is not cancelled by inner resets, then pads to full width and applies the style.
// This ensures the background color covers the entire row without gaps.
func padAndHighlight(row string, width int, style lipgloss.Style) string {
	plain := stripANSI(row)
	pad := width - lipgloss.Width(plain)
	if pad > 0 {
		plain += strings.Repeat(" ", pad)
	}
	return style.Render(plain)
}

// stripANSI removes all ANSI CSI SGR escape sequences from a string.
func stripANSI(s string) string {
	var out []byte
	i := 0
	for i < len(s) {
		if s[i] == '\x1b' && i+1 < len(s) && s[i+1] == '[' {
			// Skip past the 'm' that terminates the SGR sequence
			j := i + 2
			for j < len(s) && s[j] != 'm' {
				j++
			}
			if j < len(s) {
				i = j + 1
				continue
			}
		}
		out = append(out, s[i])
		i++
	}
	return string(out)
}

// countDisplayItemOverhead counts extra visual lines (group headers, blank separators)
// that a slice of display items would produce beyond the items themselves.
func countDisplayItemOverhead(items []displayItem) int {
	if len(items) == 0 {
		return 0
	}
	overhead := 0
	groupCount := 0
	lastRepo := ""

	for _, item := range items {
		repo := item.repoName
		if item.isPR {
			repo = item.pr.Repository
		}
		if repo != lastRepo {
			if groupCount > 0 {
				overhead++ // blank line between groups
			}
			if item.isPR {
				overhead++ // ▾ expanded group header line
			}
			// collapsed items don't add overhead — they ARE the display item
			lastRepo = repo
			groupCount++
		}
	}
	return overhead
}

func renderColumnHeader(tw int, section github.Section) string {
	st := colHeaderStyle.Width(colStatus).Render("")
	num := colHeaderStyle.Width(colNumber).Render("#")
	title := colHeaderStyle.Width(tw).Render("Title")
	author := colHeaderStyle.Width(colAuthor).Render("Author")
	age := colHeaderStyle.Width(colAge).Render("Age")
	review := colHeaderStyle.Width(colReview).Render("R")
	checksLabel := "CI"
	if section == github.SectionCreated {
		checksLabel = "Rdy"
	}
	checks := colHeaderStyle.Width(colChecks).Render(checksLabel)

	header := fmt.Sprintf("   %s %s %s %s %s %s %s",
		st, num, title, author, age, review, checks)

	sep := separatorStyle.Render("   " + strings.Repeat("─", colStatus+colNumber+tw+colAuthor+colAge+colReview+colChecks+6))

	return header + "\n" + sep
}

func renderPRRow(pr github.PullRequest, tw int, section github.Section, isUnseen bool) string {
	// Leading indicator: red bar for unseen, space otherwise
	var lead string
	if isUnseen {
		lead = unseenDotStyle.Render("▎") + "  "
	} else {
		lead = "   "
	}

	st := getStatusIcon(pr.Status)
	// Pad status icon cell to fixed width
	stCell := lipgloss.NewStyle().Width(colStatus).Render(st)

	num := mutedColor
	numStr := fmt.Sprintf("#%d", pr.Number)
	if len(numStr) > colNumber {
		numStr = numStr[:colNumber]
	}
	numCell := lipgloss.NewStyle().Width(colNumber).Foreground(num).Render(numStr)

	title := truncate(pr.Title, tw)
	titleCell := titleStyle.Width(tw).Render(title)

	author := truncate(pr.Author, colAuthor)
	authorCell := authorStyle.Width(colAuthor).Render(author)

	age := pr.Age()
	ageCell := ageStyle.Width(colAge).Render(age)

	rev := getReviewIcon(pr.ReviewStatus)
	revCell := lipgloss.NewStyle().Width(colReview).Render(rev)

	var lastColIcon string
	if section == github.SectionCreated {
		lastColIcon = getReadyIcon(pr)
	} else {
		lastColIcon = getChecksIcon(pr.ChecksState)
	}
	lastColCell := lipgloss.NewStyle().Width(colChecks).Render(lastColIcon)

	return fmt.Sprintf("%s%s %s %s %s %s %s %s",
		lead, stCell, numCell, titleCell, authorCell, ageCell, revCell, lastColCell)
}

func truncate(s string, max int) string {
	if lipgloss.Width(s) <= max {
		return s
	}
	// Trim rune-by-rune to handle wide characters
	for i := range s {
		if lipgloss.Width(s[:i]) >= max-1 {
			return s[:i] + "…"
		}
	}
	return s
}

func getStatusIcon(status github.PRStatus) string {
	switch status {
	case github.PRStatusOpen:
		return statusOpenStyle.Render("●")
	case github.PRStatusDraft:
		return statusDraftStyle.Render("●")
	case github.PRStatusMerged:
		return statusMergedStyle.Render("●")
	case github.PRStatusClosed:
		return statusClosedStyle.Render("○")
	default:
		return "○"
	}
}

func getReviewIcon(status github.ReviewStatus) string {
	switch status {
	case github.ReviewApproved:
		return reviewApprovedStyle.Render("✓")
	case github.ReviewChangesReq:
		return reviewChangesStyle.Render("✗")
	case github.ReviewPending:
		return reviewPendingStyle.Render("⏳")
	case github.ReviewRequired:
		return reviewPendingStyle.Render("⏳")
	default:
		return reviewPendingStyle.Render("⏳")
	}
}

func getChecksIcon(state string) string {
	switch strings.ToUpper(state) {
	case "SUCCESS":
		return lipgloss.NewStyle().Foreground(successColor).Render("✓")
	case "FAILURE", "ERROR":
		return lipgloss.NewStyle().Foreground(dangerColor).Render("✗")
	case "PENDING", "EXPECTED":
		return lipgloss.NewStyle().Foreground(warningColor).Render("●")
	default:
		return lipgloss.NewStyle().Foreground(mutedColor).Render("·")
	}
}

func getReadyIcon(pr github.PullRequest) string {
	if pr.IsReadyToMerge() {
		return lipgloss.NewStyle().Foreground(successColor).Render("✓")
	}
	return lipgloss.NewStyle().Foreground(dangerColor).Render("✗")
}

func sortByRepo(prs []github.PullRequest) {
	sort.SliceStable(prs, func(i, j int) bool {
		return prs[i].Repository < prs[j].Repository
	})
}
