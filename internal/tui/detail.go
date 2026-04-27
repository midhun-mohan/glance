package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/midhun-mohan/glance/internal/github"
)

// diffLineInfo holds metadata for a single diff line to enable commenting.
type diffLineInfo struct {
	line        int    // file line number (0 if non-commentable)
	side        string // "LEFT", "RIGHT", or ""
	commentable bool
}

// parseDiffLinesMeta parses a unified diff patch and computes line number metadata.
func parseDiffLinesMeta(patch string) []diffLineInfo {
	lines := strings.Split(patch, "\n")
	result := make([]diffLineInfo, len(lines))

	var oldLine, newLine int

	for i, line := range lines {
		if strings.HasPrefix(line, "@@") {
			old, new := parseHunkHeader(line)
			oldLine = old
			newLine = new
			result[i] = diffLineInfo{commentable: false}
		} else if strings.HasPrefix(line, "+") {
			result[i] = diffLineInfo{line: newLine, side: "RIGHT", commentable: true}
			newLine++
		} else if strings.HasPrefix(line, "-") {
			result[i] = diffLineInfo{line: oldLine, side: "LEFT", commentable: true}
			oldLine++
		} else if line == "" && i == len(lines)-1 {
			// trailing empty line
			result[i] = diffLineInfo{commentable: false}
		} else {
			// context line
			result[i] = diffLineInfo{line: newLine, side: "RIGHT", commentable: true}
			oldLine++
			newLine++
		}
	}

	return result
}

// parseHunkHeader extracts old and new starting line numbers from a hunk header.
// Format: @@ -oldStart[,oldCount] +newStart[,newCount] @@
func parseHunkHeader(line string) (oldStart, newStart int) {
	// Find the range specs between @@ markers
	line = strings.TrimPrefix(line, "@@")
	idx := strings.Index(line, "@@")
	if idx >= 0 {
		line = line[:idx]
	}
	line = strings.TrimSpace(line)

	parts := strings.Fields(line)
	for _, p := range parts {
		if strings.HasPrefix(p, "-") {
			fmt.Sscanf(p, "-%d", &oldStart)
		} else if strings.HasPrefix(p, "+") {
			fmt.Sscanf(p, "+%d", &newStart)
		}
	}
	return
}

// commentKey groups review comments by their location.
type commentKey struct {
	path string
	line int
	side string
}

// buildCommentMap groups review comments by (path, line, side) for fast lookup.
func buildCommentMap(comments []github.ReviewComment) map[commentKey][]github.ReviewComment {
	m := make(map[commentKey][]github.ReviewComment)
	for _, c := range comments {
		k := commentKey{path: c.Path, line: c.Line, side: c.Side}
		m[k] = append(m[k], c)
	}
	return m
}

// renderDetailView renders the split-screen PR detail view.
func (m Model) renderDetailView() string {
	if m.detailLoading {
		return m.renderDetailLoading()
	}
	if m.detailError != nil {
		return m.renderDetailError()
	}
	if m.detailData == nil {
		return ""
	}
	return m.renderSplitScreen()
}

// --- Loading / Error ---

func (m Model) renderDetailLoading() string {
	content := spinnerStyle.Render("⟳ Loading PR details...")
	overlay := detailOverlayStyle.Render(content)
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, overlay)
}

func (m Model) renderDetailError() string {
	var lines []string
	lines = append(lines, lipgloss.NewStyle().Bold(true).Foreground(dangerColor).Render("Error loading PR details"))
	lines = append(lines, "")
	lines = append(lines, detailBodyStyle.Render(m.detailError.Error()))
	lines = append(lines, "")
	lines = append(lines, helpDescStyle.Render("Esc close  r retry"))
	content := strings.Join(lines, "\n")
	overlay := detailOverlayStyle.Render(content)
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, overlay)
}

// --- Split Screen ---

func (m Model) renderSplitScreen() string {
	d := m.detailData
	totalW := m.width - 2 // outer border
	totalH := m.height - 2

	if totalW < 40 {
		totalW = 40
	}
	if totalH < 10 {
		totalH = 10
	}

	// --- Header (2 lines) ---
	header := m.renderDetailHeader(totalW)
	headerH := lipgloss.Height(header)

	// --- Footer (1 line) ---
	footer := m.renderDetailFooter(totalW)
	footerH := lipgloss.Height(footer)

	// --- Panels ---
	panelH := totalH - headerH - footerH - 2 // 2 for separator lines
	if panelH < 5 {
		panelH = 5
	}

	leftW := totalW * 30 / 100
	if leftW < 20 {
		leftW = 20
	}
	if leftW > 40 {
		leftW = 40
	}
	rightW := totalW - leftW - 1 // 1 for vertical separator

	// Build left panel content (file list)
	leftContent := m.renderFileListPanel(leftW, panelH, d)

	// Build right panel content (diff or info)
	var rightContent string
	if m.detailRightTab == 1 {
		rightContent = m.renderInfoPanel(rightW, panelH, d)
	} else {
		rightContent = m.renderDiffPanel(rightW, panelH, d)
	}

	// Style panels with focus indicator
	leftStyle := panelInactiveBorder
	rightStyle := panelInactiveBorder
	if m.detailFocus == 0 {
		leftStyle = panelActiveBorder
	} else {
		rightStyle = panelActiveBorder
	}

	leftPanel := leftStyle.Width(leftW - 2).MaxWidth(leftW).Height(panelH).Render(leftContent)
	rightPanel := rightStyle.Width(rightW - 2).MaxWidth(rightW).Height(panelH).Render(rightContent)

	split := lipgloss.JoinHorizontal(lipgloss.Top, leftPanel, rightPanel)

	// Assemble
	view := strings.Join([]string{header, split, footer}, "\n")

	box := screenBoxStyle.Width(totalW).Height(totalH).Render(view)
	return box
}

func (m Model) renderDetailHeader(width int) string {
	d := m.detailData

	// Line 1: #number  title
	num := helpKeyStyle.Render(fmt.Sprintf("#%d", d.Number))
	titleText := d.Title
	maxTitleW := width - 10
	if len(titleText) > maxTitleW {
		titleText = titleText[:maxTitleW-1] + "…"
	}
	title := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#F9FAFB")).Render("  " + titleText)
	line1 := num + title

	// Line 2: repo • branch info • status summary
	repoStr := repoStyle.Render(d.Repository)
	branchStr := detailBodyStyle.Render(d.BaseRefName + " ← " + d.HeadRefName)
	statusLine := buildStatusLine(d)
	line2 := repoStr + "  " + branchStr + "  " + statusLine

	// Right-panel tabs indicator
	var diffTab, infoTab string
	if m.detailRightTab == 0 {
		diffTab = detailTabActive.Render("[Diff]")
		infoTab = detailTabInactive.Render(" Info ")
	} else {
		diffTab = detailTabInactive.Render(" Diff ")
		infoTab = detailTabActive.Render("[Info]")
	}
	tabs := diffTab + " " + infoTab
	tabW := lipgloss.Width(tabs)
	line1W := lipgloss.Width(line1)
	gap := width - line1W - tabW - 2
	if gap < 1 {
		gap = 1
	}
	line1 = line1 + strings.Repeat(" ", gap) + tabs

	sep := panelHeaderSep.Render(strings.Repeat("─", width))

	// Result banner from PR actions
	if m.confirmResult != "" {
		bannerStyle := lipgloss.NewStyle().Foreground(successColor).Bold(true)
		if strings.HasPrefix(m.confirmResult, "✗") {
			bannerStyle = lipgloss.NewStyle().Foreground(dangerColor).Bold(true)
		}
		banner := bannerStyle.Padding(0, 1).Render(m.confirmResult)
		return line1 + "\n" + line2 + "\n" + banner + "\n" + sep
	}

	return line1 + "\n" + line2 + "\n" + sep
}

func (m Model) renderDetailFooter(width int) string {
	sep := panelHeaderSep.Render(strings.Repeat("─", width))

	var nav string
	if m.commentMode {
		nav = "Enter submit  Esc cancel"
	} else if m.detailFocus == 0 {
		nav = "↑↓ files  Tab switch"
	} else if m.detailRightTab == 0 {
		nav = "↑↓ navigate  c comment  Tab switch"
	} else if m.detailRightTab == 1 && m.detailData != nil && len(m.detailData.Checks) > 0 {
		nav = "↑↓ checks  o open check  Tab switch"
	} else {
		nav = "↑↓ scroll  Tab switch"
	}

	actions := "c comment  A approve  X reject"
	if m.activeSection == github.SectionCreated {
		actions += "  M merge"
	}

	hints := helpDescStyle.Render(nav + "  " + actions + "  o open  y copy  r refresh  Esc close")

	return sep + "\n" + hints
}

// --- Left Panel: File List ---

func (m Model) renderFileListPanel(width, height int, d *github.PRDetail) string {
	if len(d.Files) == 0 {
		return emptyStyle.Render("No files changed")
	}

	contentW := width - 4 // border + padding

	var lines []string
	for i, f := range d.Files {
		icon := fileStatusIcon(f.Status)
		stats := helpDescStyle.Render(fmt.Sprintf("+%d -%d", f.Additions, f.Deletions))

		name := f.Filename
		nameW := contentW - 12
		if nameW < 10 {
			nameW = 10
		}
		// Show just the filename, truncate path from left if needed
		if len(name) > nameW {
			name = "…" + name[len(name)-nameW+1:]
		}

		row := fmt.Sprintf("%s %-*s %s", icon, nameW, name, stats)

		if i == m.fileCursor {
			row = padAndHighlight(row, contentW, selectedPRStyle)
		}
		lines = append(lines, row)
	}

	// Auto-scroll to keep cursor visible
	visH := height - 2 // panel border
	if visH < 3 {
		visH = 3
	}
	scroll := m.fileListScroll
	if m.fileCursor < scroll {
		scroll = m.fileCursor
	}
	if m.fileCursor >= scroll+visH {
		scroll = m.fileCursor - visH + 1
	}
	if scroll < 0 {
		scroll = 0
	}

	end := scroll + visH
	if end > len(lines) {
		end = len(lines)
	}
	if scroll >= len(lines) {
		scroll = 0
		end = len(lines)
		if end > visH {
			end = visH
		}
	}
	visible := lines[scroll:end]

	return strings.Join(visible, "\n")
}

// --- Right Panel: Diff ---

func (m Model) renderDiffPanel(width, height int, d *github.PRDetail) string {
	if len(d.Files) == 0 {
		return emptyStyle.Render("No files to show")
	}
	if m.fileCursor >= len(d.Files) {
		return ""
	}

	f := d.Files[m.fileCursor]
	contentW := width - 4

	var lines []string

	// File header
	icon := fileStatusIcon(f.Status)
	stats := helpDescStyle.Render(fmt.Sprintf("+%d -%d", f.Additions, f.Deletions))
	lines = append(lines, detailSectionStyle.Render(icon+"  "+f.Filename)+"  "+stats)
	if f.PreviousFilename != "" {
		lines = append(lines, detailBodyStyle.Render("renamed from "+f.PreviousFilename))
	}
	lines = append(lines, panelHeaderSep.Render(strings.Repeat("─", contentW)))

	// Diff content with cursor, inline comments, and comment input
	if f.Patch == "" {
		lines = append(lines, emptyStyle.Render("Binary file or too large"))
	} else {
		patchLines := strings.Split(f.Patch, "\n")
		meta := parseDiffLinesMeta(f.Patch)
		commentMap := buildCommentMap(d.ReviewComments)
		isFocused := m.detailFocus == 1 && m.detailRightTab == 0

		// Track which rendered line the cursor maps to (for auto-scroll)
		cursorRenderLine := -1

		for i, dl := range patchLines {
			dl = sanitizeDiffLine(dl, contentW)
			styled := styleDiffLine(dl)

			// Show cursor highlight when diff panel is focused
			if isFocused && i == m.diffCursor {
				cursorRenderLine = len(lines)
				styled = padAndHighlight(dl, contentW, diffCursorStyle)
			}

			lines = append(lines, styled)

			// Show comment input below cursor line
			if m.commentMode && i == m.diffCursor {
				lines = append(lines, renderCommentInput(m.commentInput, m.commentLoading, contentW)...)
			}

			// Show existing review comments below matching diff lines
			if i < len(meta) && meta[i].commentable {
				k := commentKey{path: f.Filename, line: meta[i].line, side: meta[i].side}
				if comments, ok := commentMap[k]; ok {
					for _, c := range comments {
						lines = append(lines, renderInlineComment(c, contentW)...)
					}
				}
			}
		}

		// Auto-scroll to keep cursor visible
		if cursorRenderLine >= 0 {
			visH := height - 2
			if visH < 3 {
				visH = 3
			}
			if cursorRenderLine < m.diffScroll {
				m.diffScroll = cursorRenderLine
			}
			if cursorRenderLine >= m.diffScroll+visH {
				m.diffScroll = cursorRenderLine - visH + 1
			}
		}
	}

	// Scrolling
	visH := height - 2
	if visH < 3 {
		visH = 3
	}
	scroll := m.diffScroll
	maxScroll := len(lines) - visH
	if maxScroll < 0 {
		maxScroll = 0
	}
	if scroll > maxScroll {
		scroll = maxScroll
	}
	if scroll < 0 {
		scroll = 0
	}

	end := scroll + visH
	if end > len(lines) {
		end = len(lines)
	}
	visible := lines[scroll:end]

	return strings.Join(visible, "\n")
}

// renderInlineComment renders an existing review comment as an inline block.
func renderInlineComment(c github.ReviewComment, maxWidth int) []string {
	var lines []string
	age := formatCommentAge(c.CreatedAt)
	header := commentHeaderStyle.Render(fmt.Sprintf("  💬 @%s (%s)", c.Author, age))
	lines = append(lines, header)
	for _, wl := range wrapText(c.Body, maxWidth-4) {
		lines = append(lines, commentBodyStyle.Render("  "+wl))
	}
	return lines
}

// renderCommentInput renders the inline comment text input as separate lines
// with a bordered box containing the input, separator, and hints.
func renderCommentInput(input string, loading bool, maxWidth int) []string {
	if loading {
		return []string{commentHeaderStyle.Render("  ⟳ Submitting comment...")}
	}
	var lines []string
	inputW := maxWidth - 8 // account for indent + border + padding
	if inputW < 10 {
		inputW = 10
	}
	text := input
	if len(text) > inputW {
		text = text[len(text)-inputW:]
	}
	enterKey := helpKeyStyle.Render("Enter")
	escKey := helpKeyStyle.Render("Esc")
	hints := enterKey + " submit  •  " + escKey + " cancel"
	content := text + "█\n" + strings.Repeat("─", inputW) + "\n" + hints
	box := confirmInputStyle.Width(inputW).Render(content)
	// Split bordered box into separate lines for proper scroll accounting
	for _, bl := range strings.Split(box, "\n") {
		lines = append(lines, "  "+bl)
	}
	return lines
}

func formatCommentAge(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	case d < 7*24*time.Hour:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	default:
		return t.Format("Jan 2")
	}
}

// --- Right Panel: Info (PR details, reviews, checks) ---

func (m Model) renderInfoPanel(width, height int, d *github.PRDetail) string {
	contentW := width - 4

	var lines []string

	// Metadata
	addField := func(label, value string) {
		l := helpKeyStyle.Width(12).Render(label)
		v := lipgloss.NewStyle().Foreground(lipgloss.Color("#D1D5DB")).Render(value)
		lines = append(lines, l+" "+v)
	}

	addField("Author", d.Author)
	if len(d.Assignees) > 0 {
		addField("Assignees", strings.Join(d.Assignees, ", "))
	}
	if len(d.Labels) > 0 {
		addField("Labels", strings.Join(d.Labels, ", "))
	}
	addField("Created", d.CreatedAt.Format("2006-01-02"))
	addField("Updated", d.UpdatedAt.Format("2006-01-02"))
	if d.MergedAt != nil {
		addField("Merged", d.MergedAt.Format("2006-01-02")+" by "+d.MergedBy)
	}
	if d.Mergeable != "" && d.State != "MERGED" {
		addField("Mergeable", d.Mergeable)
	}
	if d.CommentsCount > 0 {
		addField("Comments", fmt.Sprintf("%d", d.CommentsCount))
	}

	// Description
	lines = append(lines, "")
	lines = append(lines, detailSectionStyle.Render("Description"))
	if d.Body == "" {
		lines = append(lines, detailBodyStyle.Render("No description provided."))
	} else {
		for _, wl := range wrapText(d.Body, contentW) {
			lines = append(lines, detailBodyStyle.Render(wl))
		}
	}

	// Reviews
	if len(d.Reviews) > 0 {
		lines = append(lines, "")
		lines = append(lines, detailSectionStyle.Render("Reviews"))
		for _, r := range d.Reviews {
			icon := reviewStateIcon(r.State)
			name := lipgloss.NewStyle().Width(16).Render(r.Author)
			state := detailBodyStyle.Render(r.State)
			lines = append(lines, icon+" "+name+" "+state)
		}
	}

	// Checks
	selectedCheckLine := -1
	if len(d.Checks) > 0 {
		passed := 0
		for _, ch := range d.Checks {
			if ch.Status == github.CheckSuccess {
				passed++
			}
		}
		lines = append(lines, "")
		checksHeader := fmt.Sprintf("Checks (%d/%d passed)", passed, len(d.Checks))
		if m.detailFocus == 1 && m.detailRightTab == 1 {
			checksHeader += "  ↑↓ navigate  o open"
		}
		lines = append(lines, detailSectionStyle.Render(checksHeader))
		for i, ch := range d.Checks {
			icon := checkIcon(ch.Status)
			nameW := contentW - 20
			if nameW < 10 {
				nameW = 10
			}
			name := ch.Name
			if len(name) > nameW {
				name = name[:nameW-1] + "…"
			}
			nameStr := lipgloss.NewStyle().Width(nameW).Render(name)
			status := detailBodyStyle.Render(string(ch.Status))
			row := icon + " " + nameStr + " " + status
			if i == m.checkCursor && m.detailFocus == 1 && m.detailRightTab == 1 {
				selectedCheckLine = len(lines)
				if m.checkNoURL {
					row = padAndHighlight(row, contentW, lipgloss.NewStyle().Background(lipgloss.Color("#7F1D1D")))
					row += "\n" + lipgloss.NewStyle().Foreground(dangerColor).Render("  No URL available")
				} else {
					row = padAndHighlight(row, contentW, selectedPRStyle)
				}
			}
			lines = append(lines, row)
		}
	}

	// Scrolling — auto-scroll to keep selected check visible
	visH := height - 2
	if visH < 3 {
		visH = 3
	}

	scroll := m.infoScroll

	if selectedCheckLine >= 0 {
		if selectedCheckLine < scroll {
			scroll = selectedCheckLine
		}
		if selectedCheckLine >= scroll+visH {
			scroll = selectedCheckLine - visH + 1
		}
	}

	maxScroll := len(lines) - visH
	if maxScroll < 0 {
		maxScroll = 0
	}
	if scroll > maxScroll {
		scroll = maxScroll
	}
	if scroll < 0 {
		scroll = 0
	}

	end := scroll + visH
	if end > len(lines) {
		end = len(lines)
	}
	visible := lines[scroll:end]

	return strings.Join(visible, "\n")
}

// --- Shared helpers ---

func buildStatusLine(d *github.PRDetail) string {
	var parts []string

	switch {
	case d.IsDraft:
		parts = append(parts, statusDraftStyle.Render("◐ Draft"))
	case d.State == "MERGED":
		parts = append(parts, statusMergedStyle.Render("● Merged"))
	case d.State == "CLOSED":
		parts = append(parts, statusClosedStyle.Render("○ Closed"))
	default:
		parts = append(parts, statusOpenStyle.Render("● Open"))
	}

	switch d.ReviewDecision {
	case "APPROVED":
		parts = append(parts, reviewApprovedStyle.Render("✓ Approved"))
	case "CHANGES_REQUESTED":
		parts = append(parts, reviewChangesStyle.Render("✗ Changes Requested"))
	case "REVIEW_REQUIRED":
		parts = append(parts, reviewPendingStyle.Render("⏳ Review Required"))
	}

	add := lipgloss.NewStyle().Foreground(successColor).Render(fmt.Sprintf("+%d", d.Additions))
	del := lipgloss.NewStyle().Foreground(dangerColor).Render(fmt.Sprintf("-%d", d.Deletions))
	parts = append(parts, add+" "+del)
	parts = append(parts, detailBodyStyle.Render(fmt.Sprintf("%d files", d.ChangedFiles)))

	sep := detailBodyStyle.Render(" • ")
	return strings.Join(parts, sep)
}

func reviewStateIcon(state string) string {
	switch state {
	case "APPROVED":
		return reviewApprovedStyle.Render("✓")
	case "CHANGES_REQUESTED":
		return reviewChangesStyle.Render("✗")
	case "COMMENTED":
		return helpDescStyle.Render("💬")
	case "DISMISSED":
		return helpDescStyle.Render("⊘")
	default:
		return reviewPendingStyle.Render("⏳")
	}
}

func checkIcon(status github.CheckStatus) string {
	switch status {
	case github.CheckSuccess:
		return lipgloss.NewStyle().Foreground(successColor).Render("✓")
	case github.CheckFailure:
		return lipgloss.NewStyle().Foreground(dangerColor).Render("✗")
	case github.CheckInProgress:
		return lipgloss.NewStyle().Foreground(warningColor).Render("●")
	case github.CheckSkipped, github.CheckNeutral:
		return lipgloss.NewStyle().Foreground(mutedColor).Render("−")
	default:
		return lipgloss.NewStyle().Foreground(mutedColor).Render("○")
	}
}

func fileStatusIcon(status string) string {
	switch status {
	case "added":
		return fileAddedStyle.Render("A")
	case "removed":
		return fileDeletedStyle.Render("D")
	case "renamed":
		return fileRenamedStyle.Render("R")
	case "copied":
		return fileRenamedStyle.Render("C")
	default:
		return fileModifiedStyle.Render("M")
	}
}

// sanitizeDiffLine replaces tabs with spaces and truncates to maxWidth
// using visible-width-aware logic (not byte slicing).
func sanitizeDiffLine(line string, maxWidth int) string {
	// Replace tabs with 4 spaces
	line = strings.ReplaceAll(line, "\t", "    ")
	// Replace other control chars that break terminal layout
	line = strings.Map(func(r rune) rune {
		if r < 32 && r != '\n' {
			return ' '
		}
		return r
	}, line)
	// Width-aware truncation
	if lipgloss.Width(line) > maxWidth {
		line = truncate(line, maxWidth)
	}
	return line
}

func styleDiffLine(line string) string {
	switch {
	case strings.HasPrefix(line, "@@"):
		return diffHunkStyle.Render(line)
	case strings.HasPrefix(line, "+"):
		return diffAddStyle.Render(line)
	case strings.HasPrefix(line, "-"):
		return diffDelStyle.Render(line)
	default:
		return diffContextStyle.Render(line)
	}
}

func wrapText(text string, maxWidth int) []string {
	if maxWidth <= 0 {
		maxWidth = 80
	}
	// Sanitize tabs and control characters
	text = strings.ReplaceAll(text, "\t", "    ")
	text = strings.ReplaceAll(text, "\r", "")
	var result []string
	for _, paragraph := range strings.Split(text, "\n") {
		if paragraph == "" {
			result = append(result, "")
			continue
		}
		words := strings.Fields(paragraph)
		if len(words) == 0 {
			result = append(result, "")
			continue
		}
		line := words[0]
		for _, w := range words[1:] {
			if len(line)+1+len(w) > maxWidth {
				result = append(result, line)
				line = w
			} else {
				line += " " + w
			}
		}
		result = append(result, line)
	}
	return result
}
