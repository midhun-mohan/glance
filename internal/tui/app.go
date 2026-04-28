package tui

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/atotto/clipboard"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/midhun-mohan/glance/internal/config"
	"github.com/midhun-mohan/glance/internal/filter"
	"github.com/midhun-mohan/glance/internal/github"
	"github.com/midhun-mohan/glance/internal/notify"
)

const (
	minPageSize     = 3
	defaultPageSize = 15
	// Column header (# Title Author Age R + separator) shown once at top
	linesColumnHeader     = 2
	// Each expanded repo group: just the repo name line
	linesPerRepoHeader    = 1
	linesPerRepoSeparator = 1 // blank line between groups
)

type Model struct {
	cfg           config.Config
	client        *github.Client
	notifier      *notify.Notifier
	presets       *filter.PresetManager

	// Data
	prs           github.PRsBySection
	orgs          []string
	unseenPRs     map[string]bool // PR URLs that appeared since last view

	// UI state
	activeSection github.Section
	cursor        int
	page          int
	width         int
	height        int

	// Filter / search
	filterExpr    string
	filterSet     filter.FilterSet
	searchMode    bool
	searchQuery   string

	// Help
	showHelp      bool

	// Preset picker
	showPresets   bool
	presetCursor  int

	// Collapsible repo groups
	collapsedRepos map[string]bool

	// Detail view (split-screen: left=files, right=diff/info)
	showDetail     bool
	detailLoading  bool
	detailData     *github.PRDetail
	detailError    error
	detailFocus    int // 0=left (files), 1=right (diff/info)
	detailRightTab int // 0=diff, 1=info
	fileCursor     int // selected file in left panel
	fileListScroll int // scroll offset for file list
	diffScroll     int // scroll offset for diff
	infoScroll     int // scroll offset for info panel
	checkCursor    int // selected check in info panel checks section
	checkNoURL     bool // briefly show "No URL available" message
	diffCursor     int  // line-level cursor in diff panel
	commentMode    bool // typing a review comment
	commentInput   string
	commentLoading bool

	// Loading / refresh countdown
	loading        bool
	firstLoad      bool
	lastRefresh    time.Time
	refreshInterval time.Duration
	hourglassFrame int
	err            error

	// Confirmation dialog (approve/reject/merge)
	confirmMode    string          // "", "approve", "reject", "merge"
	confirmInput   string          // message being typed
	confirmResult  string          // success/error message after action
	confirmLoading bool
	confirmPR      *confirmContext // PR being acted upon
}

// Messages
type prsLoadedMsg struct {
	prs github.PRsBySection
}

type orgsLoadedMsg struct {
	orgs []string
}

type errMsg struct {
	err error
}

type prDetailLoadedMsg struct {
	detail github.PRDetail
}

type prDetailErrorMsg struct {
	err error
}

type prActionResultMsg struct {
	success bool
	message string
}

type confirmContext struct {
	owner  string
	repo   string
	number int
	title  string
}

type clearCheckNoURLMsg struct{}

type commentResultMsg struct {
	success bool
	message string
}

type tickMsg time.Time
type countdownTickMsg time.Time

var hourglassFrames = []string{"⏳", "⌛"}

func NewModel(cfg config.Config, client *github.Client, startSection github.Section, filterExpr string, username string) Model {
	notifier := notify.New(
		cfg.Notifications.Enabled,
		notify.EventConfig{
			NewAssignment:   cfg.Notifications.Events.NewAssignment,
			ReviewRequested: cfg.Notifications.Events.ReviewRequested,
			StatusChange:    cfg.Notifications.Events.StatusChange,
			Mentions:        cfg.Notifications.Events.Mentions,
			IncludeTeam:     cfg.Notifications.Events.IncludeTeam,
		},
		username,
	)

	m := Model{
		cfg:             cfg,
		client:          client,
		notifier:        notifier,
		presets:         filter.NewPresetManager(cfg.Presets),
		activeSection:   startSection,
		refreshInterval: cfg.Refresh.IntervalDuration(),
		collapsedRepos:  make(map[string]bool),
		unseenPRs:       make(map[string]bool),
		prs: github.PRsBySection{
			github.SectionCreated:         {},
			github.SectionReviewRequested: {},
			github.SectionAssigned:        {},
			github.SectionMentions:        {},
		},
		loading:   true,
		firstLoad: true,
	}

	if filterExpr != "" {
		m.filterExpr = filterExpr
		m.filterSet = filter.Parse(filterExpr)
	}

	return m
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(
		m.fetchOrgs(),
		m.autoRefreshTick(),
		m.countdownTick(),
	)
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return m.handleKey(msg)

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case orgsLoadedMsg:
		m.orgs = msg.orgs
		return m, m.fetchPRs()

	case prsLoadedMsg:
		oldPRs := m.prs
		m.prs = msg.prs
		m.loading = false
		m.lastRefresh = time.Now()
		m.err = nil
		// Only send notifications and mark unseen for PRs that appear after the initial load
		if m.firstLoad {
			m.firstLoad = false
		} else {
			go m.notifier.Diff(oldPRs, msg.prs)
			// Mark new PRs as unseen
			for _, section := range sectionOrder {
				oldSet := make(map[string]bool)
				for _, pr := range oldPRs[section] {
					oldSet[pr.URL] = true
				}
				for _, pr := range msg.prs[section] {
					if !oldSet[pr.URL] {
						m.unseenPRs[pr.URL] = true
					}
				}
			}
		}
		return m, nil

	case prDetailLoadedMsg:
		m.detailLoading = false
		m.detailData = &msg.detail
		return m, nil

	case prDetailErrorMsg:
		m.detailLoading = false
		m.detailError = msg.err
		return m, nil

	case errMsg:
		m.err = msg.err
		m.loading = false
		return m, nil

	case tickMsg:
		m.loading = true
		return m, tea.Batch(m.fetchPRs(), m.autoRefreshTick())

	case clearCheckNoURLMsg:
		m.checkNoURL = false
		return m, nil

	case commentResultMsg:
		m.commentLoading = false
		m.commentMode = false
		m.commentInput = ""
		m.confirmResult = msg.message
		if msg.success && m.showDetail && m.detailData != nil {
			// Re-fetch detail to show the new comment
			pr := github.PullRequest{
				Repository: m.detailData.Repository,
				Number:     m.detailData.Number,
			}
			return m, m.fetchPRDetail(pr)
		}
		return m, nil

	case countdownTickMsg:
		m.hourglassFrame = (m.hourglassFrame + 1) % len(hourglassFrames)
		return m, m.countdownTick()

	case prActionResultMsg:
		m.confirmLoading = false
		m.confirmMode = ""
		m.confirmInput = ""
		m.confirmResult = msg.message
		var cmds []tea.Cmd
		if msg.success && m.showDetail && m.confirmPR != nil {
			pr := github.PullRequest{
				Repository: m.confirmPR.owner + "/" + m.confirmPR.repo,
				Number:     m.confirmPR.number,
			}
			cmds = append(cmds, m.fetchPRDetail(pr))
		}
		if msg.success {
			cmds = append(cmds, m.fetchPRs())
		}
		m.confirmPR = nil
		if len(cmds) > 0 {
			return m, tea.Batch(cmds...)
		}
		return m, nil
	}

	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Global keys that always work
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	}

	// Help overlay intercepts all other keys
	if m.showHelp {
		m.showHelp = false
		return m, nil
	}

	// Clear result banner on any key press
	m.confirmResult = ""

	// Confirmation dialog intercepts all keys
	if m.confirmMode != "" {
		return m.handleConfirmKey(msg)
	}

	// Preset picker
	if m.showPresets {
		return m.handlePresetKey(msg)
	}

	// Detail view
	if m.showDetail {
		return m.handleDetailKey(msg)
	}

	// Search mode
	if m.searchMode {
		return m.handleSearchKey(msg)
	}

	switch msg.String() {
	case "q":
		return m, tea.Quit
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		} else if m.page > 0 {
			m.page--
			ps := m.currentPageSize()
			m.cursor = ps - 1
		}
		return m, nil
	case "down", "j":
		ps := m.currentPageSize()
		items, total := m.pagedDisplayItems(ps)
		if m.cursor < len(items)-1 {
			m.cursor++
		} else if (m.page+1)*ps < total {
			m.page++
			m.cursor = 0
		}
		return m, nil
	case "left", "h":
		if m.page > 0 {
			m.page--
			m.cursor = 0
		}
		return m, nil
	case "right", "l":
		ps := m.currentPageSize()
		total := len(m.allDisplayItems())
		if (m.page+1)*ps < total {
			m.page++
			m.cursor = 0
		}
		return m, nil
	case "tab":
		m.activeSection = nextSection(m.activeSection)
		m.cursor = 0
		m.page = 0
		return m, nil
	case "shift+tab":
		m.activeSection = prevSection(m.activeSection)
		m.cursor = 0
		m.page = 0
		return m, nil
	case "1", "2", "3", "4":
		n := int(msg.String()[0] - '0')
		if s, ok := sectionByNumber(n); ok {
			m.activeSection = s
			m.cursor = 0
			m.page = 0
		}
		return m, nil
	case "enter":
		ps := m.currentPageSize()
		items, _ := m.pagedDisplayItems(ps)
		if m.cursor >= 0 && m.cursor < len(items) && items[m.cursor].isPR {
			delete(m.unseenPRs, items[m.cursor].pr.URL)
			m.showDetail = true
			m.detailLoading = true
			m.detailData = nil
			m.detailError = nil
			m.detailFocus = 0
			m.detailRightTab = 0
			m.fileCursor = 0
			m.fileListScroll = 0
			m.diffScroll = 0
			m.infoScroll = 0
			m.checkCursor = 0
			m.checkNoURL = false
			m.diffCursor = 0
			m.commentMode = false
			m.commentInput = ""
			return m, m.fetchPRDetail(items[m.cursor].pr)
		}
		return m, nil
	case "o":
		ps := m.currentPageSize()
		items, _ := m.pagedDisplayItems(ps)
		if m.cursor >= 0 && m.cursor < len(items) && items[m.cursor].isPR {
			openBrowser(items[m.cursor].pr.URL)
		}
		return m, nil
	case "y":
		ps := m.currentPageSize()
		items, _ := m.pagedDisplayItems(ps)
		if m.cursor >= 0 && m.cursor < len(items) && items[m.cursor].isPR {
			_ = clipboard.WriteAll(items[m.cursor].pr.URL)
		}
		return m, nil
	case "/":
		m.searchMode = true
		m.searchQuery = ""
		return m, nil
	case "esc":
		m.filterExpr = ""
		m.filterSet = filter.FilterSet{}
		m.searchQuery = ""
		m.cursor = 0
		m.page = 0
		return m, nil
	case "r":
		m.loading = true
		return m, m.fetchPRs()
	case "c":
		ps := m.currentPageSize()
		items, _ := m.pagedDisplayItems(ps)
		if m.cursor >= 0 && m.cursor < len(items) {
			item := items[m.cursor]
			var repo string
			if item.isPR {
				repo = item.pr.Repository
			} else {
				repo = item.repoName
			}
			if m.collapsedRepos[repo] {
				delete(m.collapsedRepos, repo)
			} else {
				m.collapsedRepos[repo] = true
			}
		}
		return m, nil
	case "E":
		// Expand all collapsed groups
		m.collapsedRepos = make(map[string]bool)
		m.cursor = 0
		m.page = 0
		return m, nil
	case "?":
		m.showHelp = !m.showHelp
		return m, nil
	case "p":
		if len(m.presets.List()) > 0 {
			m.showPresets = true
			m.presetCursor = 0
		}
		return m, nil
	case "M":
		if m.activeSection == github.SectionCreated {
			ps := m.currentPageSize()
			items, _ := m.pagedDisplayItems(ps)
			if m.cursor >= 0 && m.cursor < len(items) && items[m.cursor].isPR && items[m.cursor].pr.IsReadyToMerge() {
				owner, repo := github.SplitOwnerRepo(items[m.cursor].pr.Repository)
				m.confirmMode = "merge"
				m.confirmInput = ""
				m.confirmPR = &confirmContext{
					owner:  owner,
					repo:   repo,
					number: items[m.cursor].pr.Number,
					title:  items[m.cursor].pr.Title,
				}
			}
		}
		return m, nil
	}

	return m, nil
}

func (m Model) handleSearchKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.searchMode = false
		m.searchQuery = ""
		m.filterExpr = ""
		m.filterSet = filter.FilterSet{}
		m.cursor = 0
		m.page = 0
		return m, nil
	case "enter":
		m.searchMode = false
		// Check if input looks like a filter expression (contains ":")
		if strings.Contains(m.searchQuery, ":") {
			m.filterExpr = m.searchQuery
			m.filterSet = filter.Parse(m.searchQuery)
			m.searchQuery = ""
		}
		m.cursor = 0
		m.page = 0
		return m, nil
	case "backspace":
		if len(m.searchQuery) > 0 {
			m.searchQuery = m.searchQuery[:len(m.searchQuery)-1]
		}
		m.cursor = 0
		m.page = 0
		return m, nil
	default:
		if len(msg.String()) == 1 {
			m.searchQuery += msg.String()
			m.cursor = 0
		}
		return m, nil
	}
}

func (m Model) handlePresetKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	presetNames := m.presetNames()
	switch msg.String() {
	case "esc", "p":
		m.showPresets = false
		return m, nil
	case "up", "k":
		m.presetCursor--
		if m.presetCursor < 0 {
			m.presetCursor = 0
		}
		return m, nil
	case "down", "j":
		m.presetCursor++
		if m.presetCursor >= len(presetNames) {
			m.presetCursor = len(presetNames) - 1
		}
		return m, nil
	case "enter":
		if m.presetCursor >= 0 && m.presetCursor < len(presetNames) {
			name := presetNames[m.presetCursor]
			if fs, ok := m.presets.Get(name); ok {
				m.filterSet = fs
				m.filterExpr = m.presets.List()[name]
			}
		}
		m.showPresets = false
		m.cursor = 0
		m.page = 0
		return m, nil
	}
	return m, nil
}

func (m Model) handleDetailKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Comment input mode intercepts all keys
	if m.commentMode {
		return m.handleCommentKey(msg)
	}

	// Global keys (any focus)
	switch msg.String() {
	case "esc":
		m.showDetail = false
		m.detailData = nil
		m.detailLoading = false
		m.detailError = nil
		return m, nil
	case "o":
		if m.detailData != nil {
			// Context-aware: open check URL when info panel is focused with checks
			if m.detailFocus == 1 && m.detailRightTab == 1 && len(m.detailData.Checks) > 0 {
				ch := m.detailData.Checks[m.checkCursor]
				if ch.URL != "" {
					openBrowser(ch.URL)
				} else {
					m.checkNoURL = true
					return m, tea.Tick(2*time.Second, func(t time.Time) tea.Msg {
						return clearCheckNoURLMsg{}
					})
				}
			} else {
				openBrowser(m.detailData.URL)
			}
		}
		return m, nil
	case "y":
		if m.detailData != nil {
			_ = clipboard.WriteAll(m.detailData.URL)
		}
		return m, nil
	case "i":
		if m.detailRightTab == 0 {
			m.detailRightTab = 1
			m.infoScroll = 0
		} else {
			m.detailRightTab = 0
			m.diffScroll = 0
		}
		return m, nil
	case "tab":
		m.detailFocus = 1 - m.detailFocus
		return m, nil
	case "r":
		ps := m.currentPageSize()
		items, _ := m.pagedDisplayItems(ps)
		if m.cursor >= 0 && m.cursor < len(items) && items[m.cursor].isPR {
			m.detailLoading = true
			m.detailData = nil
			m.detailError = nil
			return m, m.fetchPRDetail(items[m.cursor].pr)
		}
		return m, nil
	case "A":
		if m.detailData != nil {
			owner, repo := github.SplitOwnerRepo(m.detailData.Repository)
			m.confirmMode = "approve"
			m.confirmInput = ""
			m.confirmPR = &confirmContext{
				owner:  owner,
				repo:   repo,
				number: m.detailData.Number,
				title:  m.detailData.Title,
			}
		}
		return m, nil
	case "X":
		if m.detailData != nil {
			owner, repo := github.SplitOwnerRepo(m.detailData.Repository)
			m.confirmMode = "reject"
			m.confirmInput = ""
			m.confirmPR = &confirmContext{
				owner:  owner,
				repo:   repo,
				number: m.detailData.Number,
				title:  m.detailData.Title,
			}
		}
		return m, nil
	case "M":
		if m.activeSection == github.SectionCreated && m.detailData != nil {
			owner, repo := github.SplitOwnerRepo(m.detailData.Repository)
			m.confirmMode = "merge"
			m.confirmInput = ""
			m.confirmPR = &confirmContext{
				owner:  owner,
				repo:   repo,
				number: m.detailData.Number,
				title:  m.detailData.Title,
			}
		}
		return m, nil
	case "c":
		if m.detailData != nil {
			// If diff panel focused on a commentable line, do inline review comment
			if m.detailFocus == 1 && m.detailRightTab == 0 && m.fileCursor < len(m.detailData.Files) {
				f := m.detailData.Files[m.fileCursor]
				if f.Patch != "" {
					meta := parseDiffLinesMeta(f.Patch)
					if m.diffCursor < len(meta) && meta[m.diffCursor].commentable {
						m.commentMode = true
						m.commentInput = ""
						return m, nil
					}
				}
			}
			// Otherwise, open general PR comment dialog
			owner, repo := github.SplitOwnerRepo(m.detailData.Repository)
			m.confirmMode = "comment"
			m.confirmInput = ""
			m.confirmPR = &confirmContext{
				owner:  owner,
				repo:   repo,
				number: m.detailData.Number,
				title:  m.detailData.Title,
			}
		}
		return m, nil
	}

	// Focus-specific keys
	if m.detailFocus == 0 {
		// Left panel: file list navigation
		switch msg.String() {
		case "up", "k":
			if m.fileCursor > 0 {
				m.fileCursor--
				m.diffScroll = 0
				m.diffCursor = 0
			}
			return m, nil
		case "down", "j":
			if m.detailData != nil && m.fileCursor < len(m.detailData.Files)-1 {
				m.fileCursor++
				m.diffScroll = 0
				m.diffCursor = 0
			}
			return m, nil
		case "enter":
			m.detailFocus = 1
			m.detailRightTab = 0
			m.diffScroll = 0
			return m, nil
		}
	} else {
		// Right panel: diff cursor or info navigation
		switch msg.String() {
		case "up", "k":
			if m.detailRightTab == 0 {
				if m.diffCursor > 0 {
					m.diffCursor--
				}
			} else if m.detailData != nil && len(m.detailData.Checks) > 0 {
				if m.checkCursor > 0 {
					m.checkCursor--
				}
			} else {
				if m.infoScroll > 0 {
					m.infoScroll--
				}
			}
			return m, nil
		case "down", "j":
			if m.detailRightTab == 0 {
				maxLine := m.diffLineCount() - 1
				if maxLine < 0 {
					maxLine = 0
				}
				if m.diffCursor < maxLine {
					m.diffCursor++
				}
			} else if m.detailData != nil && len(m.detailData.Checks) > 0 {
				if m.checkCursor < len(m.detailData.Checks)-1 {
					m.checkCursor++
				}
			} else {
				m.infoScroll++
			}
			return m, nil
		case "enter":
			m.detailFocus = 0
			return m, nil
		}
	}
	return m, nil
}

func (m Model) handleCommentKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.commentLoading {
		return m, nil
	}

	switch msg.String() {
	case "esc":
		m.commentMode = false
		m.commentInput = ""
		return m, nil
	case "enter":
		if strings.TrimSpace(m.commentInput) == "" {
			return m, nil
		}
		m.commentLoading = true
		return m, m.submitComment()
	case "backspace":
		if len(m.commentInput) > 0 {
			m.commentInput = m.commentInput[:len(m.commentInput)-1]
		}
		return m, nil
	default:
		if len(msg.String()) == 1 {
			m.commentInput += msg.String()
		}
		return m, nil
	}
}

func (m Model) submitComment() tea.Cmd {
	d := m.detailData
	fileCursor := m.fileCursor
	diffCursor := m.diffCursor
	body := m.commentInput
	client := m.client

	return func() tea.Msg {
		if d == nil || fileCursor >= len(d.Files) {
			return commentResultMsg{success: false, message: "✗ No file selected"}
		}
		f := d.Files[fileCursor]
		meta := parseDiffLinesMeta(f.Patch)
		if diffCursor >= len(meta) || !meta[diffCursor].commentable {
			return commentResultMsg{success: false, message: "✗ Cannot comment on this line"}
		}
		owner, repo := github.SplitOwnerRepo(d.Repository)
		position := diffCursor + 1 // 1-based position in the diff
		err := client.CreateReviewComment(owner, repo, d.Number, d.HeadCommitSHA, f.Filename, body, position)
		if err != nil {
			return commentResultMsg{success: false, message: fmt.Sprintf("✗ Failed: %v", err)}
		}
		return commentResultMsg{success: true, message: "✓ Comment added"}
	}
}

// diffLineCount returns the number of diff lines for the current file.
func (m Model) diffLineCount() int {
	if m.detailData == nil || m.fileCursor >= len(m.detailData.Files) {
		return 0
	}
	f := m.detailData.Files[m.fileCursor]
	if f.Patch == "" {
		return 0
	}
	return len(strings.Split(f.Patch, "\n"))
}

func (m Model) handleConfirmKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.confirmLoading {
		return m, nil
	}

	switch msg.String() {
	case "esc":
		m.confirmMode = ""
		m.confirmInput = ""
		m.confirmPR = nil
		return m, nil
	case "enter":
		if (m.confirmMode == "reject" || m.confirmMode == "comment") && strings.TrimSpace(m.confirmInput) == "" {
			return m, nil
		}
		m.confirmLoading = true
		return m, m.submitPRAction()
	case "backspace":
		if m.confirmMode != "merge" && len(m.confirmInput) > 0 {
			m.confirmInput = m.confirmInput[:len(m.confirmInput)-1]
		}
		return m, nil
	default:
		if m.confirmMode != "merge" && len(msg.String()) == 1 {
			m.confirmInput += msg.String()
		}
		return m, nil
	}
}

func (m Model) submitPRAction() tea.Cmd {
	pr := m.confirmPR
	mode := m.confirmMode
	input := m.confirmInput
	client := m.client

	return func() tea.Msg {
		switch mode {
		case "approve":
			err := client.ApprovePR(pr.owner, pr.repo, pr.number, input)
			if err != nil {
				return prActionResultMsg{success: false, message: fmt.Sprintf("✗ Failed: %v", err)}
			}
			return prActionResultMsg{success: true, message: "✓ PR approved"}
		case "reject":
			err := client.RequestChangesPR(pr.owner, pr.repo, pr.number, input)
			if err != nil {
				return prActionResultMsg{success: false, message: fmt.Sprintf("✗ Failed: %v", err)}
			}
			return prActionResultMsg{success: true, message: "✓ Changes requested"}
		case "merge":
			commitTitle := fmt.Sprintf("%s (#%d)", pr.title, pr.number)
			err := client.MergePR(pr.owner, pr.repo, pr.number, commitTitle)
			if err != nil {
				return prActionResultMsg{success: false, message: fmt.Sprintf("✗ Failed: %v", err)}
			}
			return prActionResultMsg{success: true, message: "✓ PR merged"}
		case "comment":
			err := client.CreatePRComment(pr.owner, pr.repo, pr.number, input)
			if err != nil {
				return prActionResultMsg{success: false, message: fmt.Sprintf("✗ Failed: %v", err)}
			}
			return prActionResultMsg{success: true, message: "✓ Comment added"}
		}
		return prActionResultMsg{success: false, message: "✗ Unknown action"}
	}
}

func (m Model) presetNames() []string {
	presets := m.presets.List()
	names := make([]string, 0, len(presets))
	for name := range presets {
		names = append(names, name)
	}
	return names
}

func (m Model) filteredPRs() []github.PullRequest {
	prs := m.prs[m.activeSection]

	// Apply filter set
	if !m.filterSet.IsEmpty() {
		var filtered []github.PullRequest
		for _, pr := range prs {
			if m.filterSet.Match(pr) {
				filtered = append(filtered, pr)
			}
		}
		prs = filtered
	}

	// Apply fuzzy search (persists after exiting search input mode)
	if m.searchQuery != "" && !strings.Contains(m.searchQuery, ":") {
		prs = fuzzyFilter(m.searchQuery, prs)
	}

	// Sort by repository for grouping
	sortByRepo(prs)

	return prs
}

// allDisplayItems returns a flat list of navigable items: PR rows for expanded
// groups and collapsed-header items for collapsed groups, in sorted repo order.
func (m Model) allDisplayItems() []displayItem {
	allFiltered := m.filteredPRs()

	// Group by repo (already sorted)
	type repoGroup struct {
		name string
		prs  []github.PullRequest
	}
	var groups []repoGroup
	currentRepo := ""
	for _, pr := range allFiltered {
		if pr.Repository != currentRepo {
			currentRepo = pr.Repository
			groups = append(groups, repoGroup{name: currentRepo})
		}
		groups[len(groups)-1].prs = append(groups[len(groups)-1].prs, pr)
	}

	var items []displayItem
	for _, g := range groups {
		if m.collapsedRepos[g.name] {
			items = append(items, displayItem{
				repoName: g.name,
				count:    len(g.prs),
			})
		} else {
			for _, pr := range g.prs {
				items = append(items, displayItem{isPR: true, pr: pr})
			}
		}
	}
	return items
}

func (m Model) pagedDisplayItems(ps int) (page []displayItem, total int) {
	all := m.allDisplayItems()
	total = len(all)
	start := m.page * ps
	if start >= total {
		return nil, total
	}
	end := start + ps
	if end > total {
		end = total
	}
	return all[start:end], total
}

// calcDisplayPageSize determines how many display items fit in the given available height.
// It iteratively accounts for the group headers and blank separators.
func calcDisplayPageSize(availableLines int, allItems []displayItem, startIndex int) int {
	if availableLines <= 0 || len(allItems) == 0 {
		return minPageSize
	}

	candidate := availableLines
	if candidate > len(allItems)-startIndex {
		candidate = len(allItems) - startIndex
	}

	for i := 0; i < 5; i++ {
		end := startIndex + candidate
		if end > len(allItems) {
			end = len(allItems)
		}
		slice := allItems[startIndex:end]
		overhead := countDisplayItemOverhead(slice)
		newCandidate := availableLines - overhead
		if newCandidate < minPageSize {
			newCandidate = minPageSize
		}
		if newCandidate > len(allItems)-startIndex {
			newCandidate = len(allItems) - startIndex
		}
		if newCandidate == candidate {
			break
		}
		candidate = newCandidate
	}

	if candidate < minPageSize {
		candidate = minPageSize
	}
	return candidate
}

func (m Model) effectivePageSize(availableLines int) int {
	all := m.allDisplayItems()
	roughPS := availableLines - 2*linesPerRepoHeader
	if roughPS < minPageSize {
		roughPS = minPageSize
	}
	startIndex := m.page * roughPS
	if startIndex >= len(all) {
		startIndex = 0
	}
	return calcDisplayPageSize(availableLines, all, startIndex)
}

// currentPageSize computes the dynamic page size based on the current terminal dimensions.
func (m Model) currentPageSize() int {
	if m.height == 0 || m.width == 0 {
		return defaultPageSize
	}
	innerHeight := m.height - 2
	chromeLines := 5
	if m.filterExpr != "" || m.searchMode {
		chromeLines++
	}
	if m.err != nil {
		chromeLines++
	}
	available := innerHeight - chromeLines

	// Column header shown once at top of PR list
	available -= linesColumnHeader

	if available < minPageSize {
		available = minPageSize
	}
	return m.effectivePageSize(available)
}

func totalPages(count, ps int) int {
	if count == 0 {
		return 1
	}
	return (count-1)/ps + 1
}

func (m Model) View() string {
	if m.width == 0 {
		return "Loading..."
	}

	// Help overlay (full screen, no box)
	if m.showHelp {
		return renderHelp(m.width, m.height)
	}

	// Confirmation dialog overlay
	if m.confirmMode != "" {
		return m.renderConfirmDialog()
	}

	// Detail view overlay
	if m.showDetail {
		return m.renderDetailView()
	}

	// Preset picker overlay (full screen, no box)
	if m.showPresets {
		return m.renderPresetPicker()
	}

	// Inner content width/height accounting for the surrounding box border
	innerWidth := m.width - 2   // left + right border
	innerHeight := m.height - 2 // top + bottom border
	if innerWidth < 20 {
		innerWidth = 20
	}
	if innerHeight < 5 {
		innerHeight = 5
	}

	// --- Build chrome sections first to measure their height ---
	var chrome []string

	// Header
	header := renderHeader(m.orgs, innerWidth)
	chrome = append(chrome, header)

	// Tabs
	counts := make(map[github.Section]int)
	unseenCounts := make(map[github.Section]int)
	for _, s := range sectionOrder {
		counts[s] = len(m.prs[s])
		for _, pr := range m.prs[s] {
			if m.unseenPRs[pr.URL] {
				unseenCounts[s]++
			}
		}
	}
	tabs := renderTabs(m.activeSection, counts, unseenCounts, innerWidth)
	chrome = append(chrome, tabs)

	// Filter bar
	filterBar := renderFilterBar(m.filterExpr, m.searchMode, m.searchQuery, innerWidth)
	if filterBar != "" {
		chrome = append(chrome, filterBar)
	}

	// Error display
	if m.err != nil {
		errDisplay := lipgloss.NewStyle().
			Foreground(dangerColor).
			Padding(0, 1).
			Render(fmt.Sprintf("Error: %v", m.err))
		chrome = append(chrome, errDisplay)
	}

	// Result banner (from PR actions)
	if m.confirmResult != "" {
		bannerColor := successColor
		if strings.HasPrefix(m.confirmResult, "✗") {
			bannerColor = dangerColor
		}
		banner := lipgloss.NewStyle().
			Foreground(bannerColor).
			Bold(true).
			Padding(0, 1).
			Render(m.confirmResult)
		chrome = append(chrome, banner)
	}

	// Measure chrome height
	chromeHeight := 0
	for _, s := range chrome {
		chromeHeight += lipgloss.Height(s)
	}
	// Reserve: 1 for status bar, 1 for page info line
	reservedLines := 2
	availableListHeight := innerHeight - chromeHeight - reservedLines
	if availableListHeight < minPageSize {
		availableListHeight = minPageSize
	}

	// --- Dynamic paging based on available height ---
	ps := m.effectivePageSize(availableListHeight)
	items, total := m.pagedDisplayItems(ps)
	pages := totalPages(total, ps)

	// Clamp page/cursor in case window shrank
	if m.page >= pages {
		m.page = pages - 1
	}
	if m.cursor >= len(items) {
		m.cursor = len(items) - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}

	// --- Assemble final view ---
	var sections []string
	sections = append(sections, chrome...)

	if pages > 1 {
		pageInfo := ageStyle.Render(fmt.Sprintf("  Page %d/%d (%d items)", m.page+1, pages, total))
		sections = append(sections, pageInfo)
	}

	prList := renderPRList(items, m.cursor, innerWidth, m.activeSection, m.unseenPRs)
	prListView := lipgloss.NewStyle().Height(availableListHeight).MaxHeight(availableListHeight).Render(prList)
	sections = append(sections, prListView)

	// Status bar
	statusBar := renderStatusBar(m.lastRefresh, m.loading, m.firstLoad, m.refreshInterval, m.hourglassFrame, innerWidth)
	sections = append(sections, statusBar)

	content := strings.Join(sections, "\n")

	// Wrap everything in a box
	box := screenBoxStyle.
		Width(innerWidth).
		Height(innerHeight).
		Render(content)

	return box
}

func renderHeader(orgs []string, width int) string {
	title := headerStyle.Render("glance")

	orgInfo := ""
	if len(orgs) > 0 {
		orgInfo = ageStyle.Render(fmt.Sprintf("orgs: %d", len(orgs)))
	}

	titleWidth := lipgloss.Width(title)
	orgWidth := lipgloss.Width(orgInfo)
	gap := width - titleWidth - orgWidth - 2
	if gap < 0 {
		gap = 0
	}

	return title + strings.Repeat(" ", gap) + orgInfo
}

func (m Model) renderPresetPicker() string {
	names := m.presetNames()
	presets := m.presets.List()

	var lines []string
	title := lipgloss.NewStyle().Bold(true).Foreground(primaryColor).Render("Filter Presets")
	lines = append(lines, title, "")

	for i, name := range names {
		expr := presets[name]
		prefix := "  "
		if i == m.presetCursor {
			prefix = "▸ "
			lines = append(lines, selectedPRStyle.Render(prefix+helpKeyStyle.Render(name)+" "+helpDescStyle.Render(expr)))
		} else {
			lines = append(lines, prefix+helpKeyStyle.Render(name)+" "+helpDescStyle.Render(expr))
		}
	}

	content := strings.Join(lines, "\n")
	overlay := helpOverlayStyle.Render(content)
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, overlay)
}

func (m Model) renderConfirmDialog() string {
	if m.confirmPR == nil {
		return ""
	}

	pr := m.confirmPR
	var lines []string

	switch m.confirmMode {
	case "approve":
		title := lipgloss.NewStyle().Bold(true).Foreground(successColor).Render(
			fmt.Sprintf("Approve PR #%d?", pr.number))
		lines = append(lines, title)
		lines = append(lines, "")
		lines = append(lines, helpDescStyle.Render("Message (optional):"))
		inputBox := confirmInputStyle.Width(35).Render(m.confirmInput + "█")
		lines = append(lines, inputBox)

	case "reject":
		title := lipgloss.NewStyle().Bold(true).Foreground(dangerColor).Render(
			fmt.Sprintf("Request changes on PR #%d?", pr.number))
		lines = append(lines, title)
		lines = append(lines, "")
		lines = append(lines, helpDescStyle.Render("Message (required):"))
		inputBox := confirmInputStyle.Width(35).Render(m.confirmInput + "█")
		lines = append(lines, inputBox)

	case "merge":
		title := lipgloss.NewStyle().Bold(true).Foreground(primaryColor).Render(
			fmt.Sprintf("Squash and merge PR #%d?", pr.number))
		commitMsg := fmt.Sprintf("%s (#%d)", pr.title, pr.number)
		lines = append(lines, title)
		lines = append(lines, "")
		lines = append(lines, helpDescStyle.Render("Commit: ")+detailBodyStyle.Render(fmt.Sprintf(`"%s"`, commitMsg)))

	case "comment":
		title := lipgloss.NewStyle().Bold(true).Foreground(secondaryColor).Render(
			fmt.Sprintf("Comment on PR #%d", pr.number))
		lines = append(lines, title)
		lines = append(lines, "")
		lines = append(lines, helpDescStyle.Render("Comment (required):"))
		inputBox := confirmInputStyle.Width(35).Render(m.confirmInput + "█")
		lines = append(lines, inputBox)
	}

	if m.confirmLoading {
		lines = append(lines, "")
		lines = append(lines, spinnerStyle.Render("⟳ Processing..."))
	} else {
		lines = append(lines, "")
		enterKey := helpKeyStyle.Render("Enter")
		escKey := helpKeyStyle.Render("Esc")
		lines = append(lines, enterKey+" confirm  •  "+escKey+" cancel")
	}

	content := strings.Join(lines, "\n")
	overlay := helpOverlayStyle.Width(42).Render(content)
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, overlay)
}

// Commands
func (m Model) fetchOrgs() tea.Cmd {
	return func() tea.Msg {
		// Use configured orgs if specified
		if len(m.cfg.Orgs.Include) > 0 {
			return orgsLoadedMsg{orgs: m.cfg.Orgs.Include}
		}

		if !m.cfg.Orgs.AutoDetect {
			return orgsLoadedMsg{orgs: []string{}}
		}

		orgs, err := m.client.GetOrganizations()
		if err != nil {
			return errMsg{err: fmt.Errorf("fetching orgs: %w", err)}
		}

		// Apply exclusions
		if len(m.cfg.Orgs.Exclude) > 0 {
			excludeSet := make(map[string]bool)
			for _, e := range m.cfg.Orgs.Exclude {
				excludeSet[e] = true
			}
			var filtered []string
			for _, org := range orgs {
				if !excludeSet[org] {
					filtered = append(filtered, org)
				}
			}
			orgs = filtered
		}

		return orgsLoadedMsg{orgs: orgs}
	}
}

func (m Model) fetchPRDetail(pr github.PullRequest) tea.Cmd {
	return func() tea.Msg {
		owner, repo := github.SplitOwnerRepo(pr.Repository)
		detail, err := m.client.FetchPRDetail(owner, repo, pr.Number)
		if err != nil {
			return prDetailErrorMsg{err: err}
		}
		// Also fetch file diffs via REST API
		files, err := m.client.FetchPRFiles(owner, repo, pr.Number)
		if err != nil {
			// Non-fatal: show detail without files
			detail.Files = nil
		} else {
			detail.Files = files
		}
		// Fetch inline review comments
		reviewComments, err := m.client.FetchPRReviewComments(owner, repo, pr.Number)
		if err != nil {
			detail.ReviewComments = nil
		} else {
			detail.ReviewComments = reviewComments
		}
		// Fetch general PR comments
		prComments, err := m.client.FetchPRComments(owner, repo, pr.Number)
		if err != nil {
			detail.Comments = nil
		} else {
			detail.Comments = prComments
		}
		return prDetailLoadedMsg{detail: *detail}
	}
}

func (m Model) fetchPRs() tea.Cmd {
	return func() tea.Msg {
		prs, err := m.client.FetchAllPRs(m.orgs)
		if err != nil {
			return errMsg{err: err}
		}
		return prsLoadedMsg{prs: prs}
	}
}

func (m Model) autoRefreshTick() tea.Cmd {
	return tea.Tick(m.refreshInterval, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func (m Model) countdownTick() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg {
		return countdownTickMsg(t)
	})
}

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "linux":
		cmd = exec.Command("xdg-open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		return
	}
	_ = cmd.Start()
}
