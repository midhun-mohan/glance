package tui

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/midhun-mohan/glance/internal/github"
)

// diffLineInfo holds metadata for a single diff line to enable commenting.
type diffLineInfo struct {
	line        int    // file line number (0 if non-commentable)
	oldLine     int    // old-side file line number (0 if N/A)
	newLine     int    // new-side file line number (0 if N/A)
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
			result[i] = diffLineInfo{line: newLine, newLine: newLine, side: "RIGHT", commentable: true}
			newLine++
		} else if strings.HasPrefix(line, "-") {
			result[i] = diffLineInfo{line: oldLine, oldLine: oldLine, side: "LEFT", commentable: true}
			oldLine++
		} else if line == "" && i == len(lines)-1 {
			// trailing empty line
			result[i] = diffLineInfo{commentable: false}
		} else {
			// context line
			result[i] = diffLineInfo{line: newLine, oldLine: oldLine, newLine: newLine, side: "RIGHT", commentable: true}
			oldLine++
			newLine++
		}
	}

	return result
}

// diffLineNumWidth returns the column width needed to render every new-side
// line number in the patch right-aligned. Minimum of 2 to keep narrow patches
// from looking cramped.
func diffLineNumWidth(meta []diffLineInfo) int {
	maxLine := 0
	for _, m := range meta {
		if m.newLine > maxLine {
			maxLine = m.newLine
		}
	}
	w := len(fmt.Sprintf("%d", maxLine))
	if w < 2 {
		w = 2
	}
	return w
}

// diffLineNumPrefix builds the " NEW │ " prefix shown before each diff line.
// Only the new-side line number is rendered; removed and hunk-header rows get
// a blank-but-same-width column so the separator stays aligned.
// continuation=true emits a same-width blank prefix used for wrapped continuation
// rows.
func diffLineNumPrefix(info diffLineInfo, numWidth int, continuation bool) string {
	pad := strings.Repeat(" ", numWidth)
	if continuation {
		return " " + pad + " │ "
	}
	newStr := pad
	if info.newLine > 0 {
		newStr = fmt.Sprintf("%*d", numWidth, info.newLine)
	}
	return " " + newStr + " │ "
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

// fileComments holds both position-indexed and line/side-indexed comments for a file.
type fileComments struct {
	byPosition map[int][]github.ReviewComment    // position (1-based diff index) -> comments
	byLineSide map[commentKey][]github.ReviewComment // (path, line, side) -> comments
}

// buildFileComments builds lookup indexes for review comments on a specific file.
func buildFileComments(comments []github.ReviewComment, path string) fileComments {
	fc := fileComments{
		byPosition: make(map[int][]github.ReviewComment),
		byLineSide: make(map[commentKey][]github.ReviewComment),
	}
	for _, c := range comments {
		if c.Path != path {
			continue
		}
		if c.Position > 0 {
			fc.byPosition[c.Position] = append(fc.byPosition[c.Position], c)
		} else if c.Line > 0 {
			k := commentKey{path: c.Path, line: c.Line, side: c.Side}
			fc.byLineSide[k] = append(fc.byLineSide[k], c)
		}
	}
	return fc
}

// commentsForDiffLine returns review comments matching a diff line, checking
// position first (most reliable), then falling back to line/side.
func (fc fileComments) commentsForDiffLine(diffIndex int, meta diffLineInfo, path string) []github.ReviewComment {
	// Position is 1-based, diffIndex is 0-based
	if cs, ok := fc.byPosition[diffIndex+1]; ok {
		return cs
	}
	if meta.commentable && meta.line > 0 {
		k := commentKey{path: path, line: meta.line, side: meta.side}
		if cs, ok := fc.byLineSide[k]; ok {
			return cs
		}
	}
	return nil
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

// currentLeftPanelPct returns the active left-panel width percentage, falling
// back to the 30% default when the user hasn't resized.
func (m Model) currentLeftPanelPct() int {
	if m.leftPanelPct == 0 {
		return 30
	}
	return m.leftPanelPct
}

// clampLeftPanelPct keeps the user-adjustable split width inside a sane range.
func clampLeftPanelPct(pct int) int {
	if pct < 15 {
		return 15
	}
	if pct > 60 {
		return 60
	}
	return pct
}

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

	leftW := totalW * m.currentLeftPanelPct() / 100
	// Hard floor for the file-list panel. The row layout is
	// "  ICON NAME STATS" — with worst-case stats like "+9999 -9999" (11
	// cells) and a 3-cell minimum name, the row needs 19 visible cells, plus
	// 2 for the border = 21. Round up to 24 for a small safety margin so
	// shrinking the split with `<` can't produce a panel whose rows wrap
	// inside lipgloss and double the panel height.
	if leftW < 24 {
		leftW = 24
	}
	if leftW > totalW-25 {
		// Always leave at least 25 cols for the diff panel.
		leftW = totalW - 25
	}
	// Cap expansion at the width needed to render the longest file/folder
	// row, so growing the split with `>` doesn't add trailing whitespace
	// past the actual text content.
	if maxW := fileListMaxPanelWidth(d); maxW > 0 && leftW > maxW {
		leftW = maxW
		if leftW < 24 {
			leftW = 24
		}
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

	var actions string
	if m.detailPreview {
		actions = "P create PR  w wrap  <> resize"
	} else {
		actions = "c comment  A approve  X reject  E close/reopen  D draft"
		if m.activeSection == github.SectionCreated {
			actions += "  M merge"
		}
		actions += "  w wrap  <> resize"
	}

	var tail string
	if m.detailPreview {
		tail = "  i diff/info  Esc close"
	} else {
		tail = "  i diff/info  o open  y copy  r refresh  Esc close"
	}

	hints := helpDescStyle.Render(nav + "  " + actions + tail)

	return sep + "\n" + hints
}

// --- Left Panel: File List ---

// fileListMaxPanelWidth returns the panel width (including border/padding)
// needed to render every file and folder row without truncation. Used to
// cap the user-adjustable split so the file viewer can't expand past its
// actual content.
func fileListMaxPanelWidth(d *github.PRDetail) int {
	if d == nil || len(d.Files) == 0 {
		return 0
	}
	addW, delW := 2, 2
	for _, f := range d.Files {
		if w := len(fmt.Sprintf("+%d", f.Additions)); w > addW {
			addW = w
		}
		if w := len(fmt.Sprintf("-%d", f.Deletions)); w > delW {
			delW = w
		}
	}
	statsW := addW + 1 + delW // "+N -N"

	maxContentW := 0
	dirIconW := lipgloss.Width("📁 ")
	lastDir := ""
	for _, f := range d.Files {
		// File row: "  " + icon(1) + " " + name + " " + stats = 5 + name + stats
		nameW := lipgloss.Width(filepath.Base(f.Filename))
		if w := 5 + nameW + statsW; w > maxContentW {
			maxContentW = w
		}
		dir := filepath.Dir(f.Filename)
		if dir == "." {
			dir = "/"
		}
		if dir != lastDir {
			if w := dirIconW + lipgloss.Width(dir); w > maxContentW {
				maxContentW = w
			}
			lastDir = dir
		}
	}
	return maxContentW + 4 // border (2) + padding (2)
}

func (m Model) renderFileListPanel(width, height int, d *github.PRDetail) string {
	if len(d.Files) == 0 {
		return emptyStyle.Render("No files changed")
	}

	contentW := width - 4 // border + padding

	// Pre-compute the widest "+N" and "-N" cells across the file list so each
	// row can right-align its stats to the same column boundaries. Without this
	// the +/- numbers visibly jitter between rows of different magnitudes.
	addW, delW := 2, 2
	for _, f := range d.Files {
		if w := len(fmt.Sprintf("+%d", f.Additions)); w > addW {
			addW = w
		}
		if w := len(fmt.Sprintf("-%d", f.Deletions)); w > delW {
			delW = w
		}
	}

	// Render lines grouped by directory.
	var lines []string
	cursorLine := -1 // rendered line index of selected file
	lastDir := ""
	for i, f := range d.Files {
		dir := filepath.Dir(f.Filename)
		if dir == "." {
			dir = "/"
		}

		// Emit folder header when directory changes.
		if dir != lastDir {
			if lastDir != "" {
				lines = append(lines, "") // blank separator between groups
			}
			// Truncate the directory path (not the icon) so the folder icon is
			// always visible even when the user shrinks the panel — otherwise
			// truncateLeft eats the leading "📁 " prefix on long paths.
			iconPrefix := "📁 "
			dirMaxW := contentW - lipgloss.Width(iconPrefix)
			if dirMaxW < 3 {
				dirMaxW = 3
			}
			dirText := dir
			if lipgloss.Width(dirText) > dirMaxW {
				dirText = truncateLeft(dirText, dirMaxW)
			}
			lines = append(lines, helpDescStyle.Render(iconPrefix+dirText))
			lastDir = dir
		}

		icon := fileStatusIcon(f.Status)
		addStat := lipgloss.NewStyle().Foreground(successColor).Render(fmt.Sprintf("%*s", addW, fmt.Sprintf("+%d", f.Additions)))
		delStat := lipgloss.NewStyle().Foreground(dangerColor).Render(fmt.Sprintf("%*s", delW, fmt.Sprintf("-%d", f.Deletions)))
		stats := addStat + " " + delStat

		// Row layout: "  ICON NAME STATS" — overhead is 5 cells (2 lead + icon
		// + 2 spaces) plus the visible width of stats. nameW must be small
		// enough that the row fits within contentW even when the user has
		// shrunk the split panel; otherwise lipgloss wraps the row inside the
		// panel and the file-list height balloons past panelH.
		statsW := lipgloss.Width(stats)
		nameW := contentW - 5 - statsW
		if nameW < 3 {
			nameW = 3
		}
		name := filepath.Base(f.Filename)
		if len(name) > nameW {
			name = "…" + name[len(name)-nameW+1:]
		}

		row := fmt.Sprintf("  %s %-*s %s", icon, nameW, name, stats)

		if i == m.fileCursor {
			cursorLine = len(lines)
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
	if cursorLine >= 0 {
		if cursorLine < scroll {
			scroll = cursorLine
		}
		if cursorLine >= scroll+visH {
			scroll = cursorLine - visH + 1
		}
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

	// File header. Truncate the filename so the header stays within contentW —
	// otherwise lipgloss wraps it inside the panel and the split-screen layout
	// breaks for deeply-nested paths.
	icon := fileStatusIcon(f.Status)
	statsText := fmt.Sprintf("+%d -%d", f.Additions, f.Deletions)
	stats := helpDescStyle.Render(statsText)
	// header overhead: icon (1) + 2 spaces + 2 spaces + stats width.
	nameMax := contentW - 1 - 2 - 2 - lipgloss.Width(statsText)
	if nameMax < 10 {
		nameMax = 10
	}
	displayName := f.Filename
	if lipgloss.Width(displayName) > nameMax {
		displayName = truncateLeft(displayName, nameMax)
	}
	lines = append(lines, detailSectionStyle.Render(icon+"  "+displayName)+"  "+stats)
	if f.PreviousFilename != "" {
		prev := "renamed from " + f.PreviousFilename
		if lipgloss.Width(prev) > contentW {
			prev = truncateLeft(prev, contentW)
		}
		lines = append(lines, detailBodyStyle.Render(prev))
	}
	lines = append(lines, panelHeaderSep.Render(strings.Repeat("─", contentW)))

	// Diff content with cursor, inline comments, and comment input
	if f.Patch == "" {
		lines = append(lines, emptyStyle.Render("Binary file or too large"))
	} else {
		patchLines := strings.Split(f.Patch, "\n")
		meta := parseDiffLinesMeta(f.Patch)
		fc := buildFileComments(d.ReviewComments, f.Filename)
		isFocused := m.detailFocus == 1 && m.detailRightTab == 0

		// Reserve a fixed-width gutter for the new-side line number so the
		// column stays aligned across the whole patch regardless of hunks.
		numWidth := diffLineNumWidth(meta)
		prefixW := numWidth + 4 // " NEW │ "
		diffContentW := contentW - prefixW
		if diffContentW < 10 {
			diffContentW = 10
		}

		// Track which rendered line the cursor maps to (for auto-scroll)
		cursorRenderLine := -1

		for i, dl := range patchLines {
			var lineMeta diffLineInfo
			if i < len(meta) {
				lineMeta = meta[i]
			}
			// In wrap mode, one source diff line can become several visual
			// rows; in truncate mode there's always exactly one. Either way,
			// the cursor highlight is applied to every visual row for the
			// source line at m.diffCursor so the highlight covers the full
			// wrapped block.
			visualRows := diffLineVisualRows(dl, diffContentW, m.diffWrap)
			isCursor := isFocused && i == m.diffCursor
			if isCursor {
				cursorRenderLine = len(lines)
			}
			for j, vr := range visualRows {
				prefix := diffLineNumPrefix(lineMeta, numWidth, j > 0)
				if isCursor {
					lines = append(lines, padAndHighlight(prefix+vr, contentW, diffCursorStyle))
				} else {
					lines = append(lines, diffContextStyle.Render(prefix)+styleDiffLine(vr))
				}
			}

			// Show comment input below cursor line
			if m.commentMode && i == m.diffCursor {
				lines = append(lines, renderCommentInput(m.commentInput, m.commentLoading, contentW)...)
			}

			// Show existing review comments below matching diff lines
			if matched := fc.commentsForDiffLine(i, lineMeta, f.Filename); len(matched) > 0 {
				for _, c := range matched {
					lines = append(lines, renderInlineComment(c, contentW)...)
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

// renderInlineComment renders an existing review comment in a bordered box.
func renderInlineComment(c github.ReviewComment, maxWidth int) []string {
	boxW := maxWidth - 4 // indent from diff edge
	if boxW < 14 {
		boxW = 14
	}
	innerW := boxW - 4 // border (2) + padding (2)
	if innerW < 10 {
		innerW = 10
	}
	age := formatCommentAge(c.CreatedAt)
	header := commentHeaderStyle.Render(fmt.Sprintf("💬 @%s (%s)", c.Author, age))
	var bodyLines []string
	bodyLines = append(bodyLines, header)
	for _, wl := range wrapText(c.Body, innerW) {
		bodyLines = append(bodyLines, styleCommentLine(wl))
	}
	content := strings.Join(bodyLines, "\n")
	box := commentBoxStyle.Width(boxW).Render(content)
	var lines []string
	for _, bl := range strings.Split(box, "\n") {
		lines = append(lines, "  "+bl)
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
	boxW := maxWidth - 4 // indent from diff edge
	if boxW < 14 {
		boxW = 14
	}
	innerW := boxW - 4 // border (2) + padding (2)
	if innerW < 10 {
		innerW = 10
	}
	text := input
	if len(text) > innerW {
		text = text[len(text)-innerW:]
	}
	enterKey := helpKeyStyle.Render("Enter")
	escKey := helpKeyStyle.Render("Esc")
	hints := enterKey + " submit  •  " + escKey + " cancel"
	content := text + "█\n" + strings.Repeat("─", innerW) + "\n" + hints
	box := confirmInputStyle.Width(boxW).Render(content)
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
	lines, selectedCheckLine := m.buildInfoLines(width)

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

// buildInfoLines builds every rendered line for the Info panel (metadata,
// description, reviews, comments, checks) and returns the rendered index of
// the currently selected check, or -1 if none. The scroll/clip step is done
// separately so this function can be called from key handlers to clamp scroll.
func (m Model) buildInfoLines(width int) ([]string, int) {
	d := m.detailData
	if d == nil {
		return nil, -1
	}
	contentW := width - 4

	var lines []string

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

	lines = append(lines, "")
	lines = append(lines, detailSectionStyle.Render("Description"))
	if d.Body == "" {
		lines = append(lines, detailBodyStyle.Render("No description provided."))
	} else {
		for _, wl := range wrapText(d.Body, contentW) {
			lines = append(lines, detailBodyStyle.Render(wl))
		}
	}

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

	if len(d.Comments) > 0 {
		lines = append(lines, "")
		lines = append(lines, detailSectionStyle.Render(
			fmt.Sprintf("Comments (%d)", len(d.Comments))))
		for _, c := range d.Comments {
			age := formatCommentAge(c.CreatedAt)
			header := commentHeaderStyle.Render(
				fmt.Sprintf("  @%s (%s)", c.Author, age))
			lines = append(lines, header)
			for _, wl := range wrapText(c.Body, contentW-4) {
				lines = append(lines, "  "+styleCommentLine(wl))
			}
			lines = append(lines, "")
		}
	}

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
				switch {
				case m.checkNoURL:
					row = padAndHighlight(row, contentW, lipgloss.NewStyle().Background(lipgloss.Color("#7F1D1D")))
					row += "\n" + lipgloss.NewStyle().Foreground(dangerColor).Render("  No URL available")
				case m.checkUnsafeURL:
					row = padAndHighlight(row, contentW, lipgloss.NewStyle().Background(lipgloss.Color("#7F1D1D")))
					row += "\n" + lipgloss.NewStyle().Foreground(dangerColor).Render("  Unsafe URL — refused to open")
				default:
					row = padAndHighlight(row, contentW, selectedPRStyle)
				}
			}
			lines = append(lines, row)
		}
	}

	return lines, selectedCheckLine
}

// infoPanelSize returns the (width, height) of the right detail panel,
// matching the math in renderSplitScreen so callers can compute scroll bounds.
func (m Model) infoPanelSize() (int, int) {
	totalW := m.width - 2
	totalH := m.height - 2
	if totalW < 40 {
		totalW = 40
	}
	if totalH < 10 {
		totalH = 10
	}
	header := m.renderDetailHeader(totalW)
	headerH := lipgloss.Height(header)
	footer := m.renderDetailFooter(totalW)
	footerH := lipgloss.Height(footer)
	panelH := totalH - headerH - footerH - 2
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
	rightW := totalW - leftW - 1
	return rightW, panelH
}

// infoMaxScroll returns the largest valid value for m.infoScroll given the
// current panel size and content. Used by handlers to clamp scroll state.
func (m Model) infoMaxScroll() int {
	if m.detailData == nil {
		return 0
	}
	rw, rh := m.infoPanelSize()
	lines, _ := m.buildInfoLines(rw)
	visH := rh - 2
	if visH < 3 {
		visH = 3
	}
	max := len(lines) - visH
	if max < 0 {
		max = 0
	}
	return max
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

// diffLineVisualRows converts one raw patch line into the visual rows it
// should occupy on screen. When wrap=false it returns a single truncated row;
// when wrap=true the line is split into rune chunks of at most maxWidth visible
// cells, with the diff prefix (+, -, space) repeated on each continuation row
// so add/delete coloring still applies after wrapping.
func diffLineVisualRows(line string, maxWidth int, wrap bool) []string {
	// Normalize tabs and stray control chars first, matching sanitizeDiffLine.
	line = strings.ReplaceAll(line, "\t", "    ")
	line = strings.Map(func(r rune) rune {
		if r < 32 && r != '\n' {
			return ' '
		}
		return r
	}, line)
	if !wrap {
		if lipgloss.Width(line) > maxWidth {
			line = truncate(line, maxWidth)
		}
		return []string{line}
	}
	if lipgloss.Width(line) <= maxWidth {
		return []string{line}
	}
	// Continuation rows reuse the leading +/-/space so the diff color stays
	// consistent. Hunk headers (@@) and the first row keep their natural prefix.
	var contPrefix string
	if len(line) > 0 {
		switch line[0] {
		case '+', '-', ' ':
			contPrefix = string(line[0])
		}
	}
	var rows []string
	first, rest := splitByWidth(line, maxWidth)
	rows = append(rows, first)
	for lipgloss.Width(rest) > 0 {
		chunkMax := maxWidth - lipgloss.Width(contPrefix)
		if chunkMax < 1 {
			chunkMax = 1
		}
		chunk, more := splitByWidth(rest, chunkMax)
		rows = append(rows, contPrefix+chunk)
		rest = more
	}
	return rows
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

// styleCommentLine applies diff-style colors to comment body lines that look
// like diff content (e.g. lines starting with +, -, @@, diff, etc.).
func styleCommentLine(line string) string {
	switch {
	case strings.HasPrefix(line, "@@"):
		return diffHunkStyle.Render(line)
	case strings.HasPrefix(line, "diff "):
		return diffHunkStyle.Render(line)
	case strings.HasPrefix(line, "+++"), strings.HasPrefix(line, "---"):
		return diffHunkStyle.Render(line)
	case strings.HasPrefix(line, "+"):
		return diffAddStyle.Render(line)
	case strings.HasPrefix(line, "-"):
		return diffDelStyle.Render(line)
	default:
		return commentBodyStyle.Render(line)
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
		var line string
		for _, w := range words {
			// Hard-break words that are wider than maxWidth (long URLs,
			// unbroken identifiers) so they don't overflow the container.
			for lipgloss.Width(w) > maxWidth {
				prefix, rest := splitByWidth(w, maxWidth)
				if line != "" {
					result = append(result, line)
					line = ""
				}
				result = append(result, prefix)
				w = rest
			}
			if line == "" {
				line = w
			} else if lipgloss.Width(line)+1+lipgloss.Width(w) > maxWidth {
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

// splitByWidth returns the longest prefix of s with visible width <= maxWidth,
// and the remainder. Rune-aware.
func splitByWidth(s string, maxWidth int) (string, string) {
	w := 0
	for i, r := range s {
		rw := lipgloss.Width(string(r))
		if rw == 0 {
			rw = 1
		}
		if w+rw > maxWidth {
			return s[:i], s[i:]
		}
		w += rw
	}
	return s, ""
}
