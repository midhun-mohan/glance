package tui

import (
	"fmt"
	"net/url"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
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
	minPageSize    = 3
	maxPRsPerRepo  = 10
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
	pendingKey    string // for multi-key sequences like gg

	// Preset picker
	showPresets   bool
	presetCursor  int

	// Collapsible repo groups
	collapsedRepos     map[string]bool
	expandedRepoLimit  map[string]int // per-repo PR display limit (default maxPRsPerRepo)

	// Detail view (split-screen: left=files, right=diff/info)
	showDetail     bool
	detailLoading  bool
	detailData     *github.PRDetail
	detailError    error
	// detailPreview is true when detailData represents a not-yet-created PR
	// (a branch-compare preview). The detail view then offers "create PR"
	// instead of approve/merge/close.
	detailPreview       bool
	detailPreviewBranch github.Branch
	detailFocus    int // 0=left (files), 1=right (diff/info)
	detailRightTab int // 0=diff, 1=info
	fileCursor     int // selected file in left panel
	fileListScroll int // scroll offset for file list
	diffScroll     int // scroll offset for diff
	infoScroll     int // scroll offset for info panel (fallback when no checks)
	checkCursor    int // selected check in info panel checks section
	checkNoURL     bool // briefly show "No URL available" message
	checkUnsafeURL bool // briefly show "Unsafe URL — refused to open" message
	diffCursor     int  // line-level cursor in diff panel
	diffWrap       bool // wrap long diff lines instead of truncating ('w' toggle)
	leftPanelPct   int  // detail-view left panel width as a percent of totalW; 0 = default
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

	// Self-upgrade notice (populated once at startup from GitHub releases)
	latestVersion string

	// Favorite repos (always pinned to top); mirrored to cfg.Favorites on toggle.
	favorites     map[string]bool
	showFavorites bool

	// Browse PR dialog
	browseMode     bool
	browseInput    string
	browseError    string
	browsePending  bool // a browse-initiated detail fetch is in progress

	// Repo-picker state inside the Browse dialog. Suggestions populate as the
	// user types a repo name (no `#`), debounced via a monotonic sequence
	// number so stale responses are dropped.
	browseRepoSuggestions []github.Repository
	browseRepoCursor      int
	browseRepoLoading     bool
	browseRepoError       string
	browseSearchSeq       int

	// Branch-picker step inside the Browse dialog. Activated after the user
	// picks a repo: lists the viewer's branches in that repo so they can pick
	// one and open a new PR.
	browseBranchStep     bool
	browseBranchOwner    string
	browseBranchRepo     string
	browseBranchDefault  string // repo default branch (PR base default)
	browseBranches       []github.Branch
	browseBranchCursor   int
	browseBranchLoading  bool
	browseBranchError    string
	// True when the branch picker was opened directly from the list view
	// (cursor row's repo). Used so Esc on the branch picker closes the dialog
	// entirely, rather than dropping into an empty repo-search step.
	browseBranchFromList bool
	// Snapshot of the repo-search step before entering the branch picker
	// (only set when entered from the in-dialog P key). Restored on Esc so
	// the user returns to their prior search input and suggestions.
	browsePrevInput       string
	browsePrevSuggestions []github.Repository
	browsePrevCursor      int

	// Create-PR dialog (opens from the diff preview)
	createPRMode       bool
	createPROwner      string
	createPRRepo       string
	createPRHead       string
	createPRTitle      string
	createPRBody       string
	createPRBase       string
	createPRFocus      int // 0=title, 1=body, 2=base
	createPRSubmitting bool
	createPRError      string

	username string

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
	nodeID string
}

type clearCheckNoURLMsg struct{}

type clearCheckUnsafeURLMsg struct{}

type commentResultMsg struct {
	success bool
	message string
}

type tickMsg time.Time
type countdownTickMsg time.Time

type latestReleaseMsg struct {
	tag string
}

type repoSearchResultMsg struct {
	seq   int
	repos []github.Repository
	err   error
}

type repoPRsLoadedMsg struct {
	prs []github.PullRequest
	err error
}

type branchesLoadedMsg struct {
	owner         string
	repo          string
	defaultBranch string
	branches      []github.Branch
	err           error
}

type compareLoadedMsg struct {
	owner   string
	repo    string
	branch  github.Branch
	base    string
	files   []github.ChangedFile
	err     error
}

type createPRResultMsg struct {
	success bool
	message string
	pr      *github.CreatedPR
	owner   string
	repo    string
}

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

	favorites := make(map[string]bool, len(cfg.Favorites))
	for _, repo := range cfg.Favorites {
		repo = strings.TrimSpace(repo)
		if repo != "" {
			favorites[repo] = true
		}
	}

	m := Model{
		cfg:             cfg,
		client:          client,
		notifier:        notifier,
		presets:         filter.NewPresetManager(cfg.Presets),
		activeSection:   startSection,
		refreshInterval: cfg.Refresh.IntervalDuration(),
		username:        username,
		collapsedRepos:    make(map[string]bool),
		expandedRepoLimit: make(map[string]int),
		unseenPRs:        make(map[string]bool),
		favorites:        favorites,
		prs: github.PRsBySection{
			github.SectionCreated:         {},
			github.SectionReviewRequested: {},
			github.SectionAssigned:        {},
			github.SectionMentions:        {},
			github.SectionBrowse:          {},
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
		checkLatestRelease(),
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
		browsePRs := m.prs[github.SectionBrowse] // preserve across refresh
		oldPRs := m.prs
		m.prs = msg.prs
		m.prs[github.SectionBrowse] = browsePRs
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
		// Sort files by directory path so the grouped view has no duplicates.
		sort.SliceStable(m.detailData.Files, func(i, j int) bool {
			di := filepath.Dir(m.detailData.Files[i].Filename)
			dj := filepath.Dir(m.detailData.Files[j].Filename)
			return di < dj
		})
		if m.browsePending {
			m.browsePending = false
			pr := github.PRFromDetail(&msg.detail)
			// Add to browse section if not already present
			found := false
			for _, existing := range m.prs[github.SectionBrowse] {
				if existing.URL == pr.URL {
					found = true
					break
				}
			}
			if !found {
				m.prs[github.SectionBrowse] = append(m.prs[github.SectionBrowse], pr)
			}
		}
		return m, nil

	case prDetailErrorMsg:
		m.detailLoading = false
		m.detailError = msg.err
		return m, nil

	case errMsg:
		// On background refresh, suppress transient errors if we already have data.
		if !m.firstLoad && m.hasPRData() {
			m.loading = false
			return m, nil
		}
		m.err = msg.err
		m.loading = false
		return m, nil

	case tickMsg:
		m.loading = true
		return m, tea.Batch(m.fetchPRs(), m.autoRefreshTick())

	case clearCheckNoURLMsg:
		m.checkNoURL = false
		return m, nil

	case clearCheckUnsafeURLMsg:
		m.checkUnsafeURL = false
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

	case latestReleaseMsg:
		if isNewerVersion(config.Version, msg.tag) {
			m.latestVersion = msg.tag
		}
		return m, nil

	case repoSearchResultMsg:
		// Drop stale responses: only the most recent query matches the seq.
		if msg.seq != m.browseSearchSeq {
			return m, nil
		}
		m.browseRepoLoading = false
		if msg.err != nil {
			m.browseRepoError = msg.err.Error()
			m.browseRepoSuggestions = nil
			m.browseRepoCursor = 0
			return m, nil
		}
		m.browseRepoError = ""
		m.browseRepoSuggestions = msg.repos
		m.browseRepoCursor = 0
		return m, nil

	case repoPRsLoadedMsg:
		m.loading = false
		if msg.err != nil {
			m.confirmResult = "✗ " + msg.err.Error()
			return m, nil
		}
		existing := make(map[string]bool, len(m.prs[github.SectionBrowse]))
		for _, pr := range m.prs[github.SectionBrowse] {
			existing[pr.URL] = true
		}
		for _, pr := range msg.prs {
			if !existing[pr.URL] {
				m.prs[github.SectionBrowse] = append(m.prs[github.SectionBrowse], pr)
				existing[pr.URL] = true
			}
		}
		return m, nil

	case branchesLoadedMsg:
		// Only react if we're still in the branch step waiting for this repo.
		if !m.browseMode || !m.browseBranchStep ||
			m.browseBranchOwner != msg.owner || m.browseBranchRepo != msg.repo {
			return m, nil
		}
		m.browseBranchLoading = false
		if msg.err != nil {
			m.browseBranchError = msg.err.Error()
			m.browseBranches = nil
			m.browseBranchCursor = 0
			return m, nil
		}
		m.browseBranchError = ""
		m.browseBranchDefault = msg.defaultBranch
		m.browseBranches = msg.branches
		m.browseBranchCursor = 0
		return m, nil

	case compareLoadedMsg:
		// Build a synthetic PRDetail for the diff viewer.
		m.detailLoading = false
		if msg.err != nil {
			m.detailError = msg.err
			return m, nil
		}
		files := msg.files
		var add, del int
		for _, f := range files {
			add += f.Additions
			del += f.Deletions
		}
		title := msg.branch.LastCommitSubject
		if title == "" {
			title = msg.branch.Name
		}
		m.detailData = &github.PRDetail{
			Title:         "[Preview] " + title,
			Body:          msg.branch.LastCommitBody,
			Repository:    msg.owner + "/" + msg.repo,
			Author:        m.username,
			State:         "OPEN",
			BaseRefName:   msg.base,
			HeadRefName:   msg.branch.Name,
			HeadCommitSHA: msg.branch.HeadSHA,
			Additions:     add,
			Deletions:     del,
			ChangedFiles:  len(files),
			Files:         files,
		}
		sort.SliceStable(m.detailData.Files, func(i, j int) bool {
			di := filepath.Dir(m.detailData.Files[i].Filename)
			dj := filepath.Dir(m.detailData.Files[j].Filename)
			if di != dj {
				return di < dj
			}
			return m.detailData.Files[i].Filename < m.detailData.Files[j].Filename
		})
		m.detailError = nil
		return m, nil

	case createPRResultMsg:
		m.createPRSubmitting = false
		if !msg.success {
			m.createPRError = msg.message
			return m, nil
		}
		// Success: clear dialog state, drop the preview, and load the new PR.
		m.createPRMode = false
		m.createPRError = ""
		m.confirmResult = msg.message
		newPR := github.PullRequest{
			Repository: msg.owner + "/" + msg.repo,
		}
		if msg.pr != nil {
			newPR.Number = msg.pr.Number
			newPR.URL = msg.pr.URL
		}
		m.detailPreview = false
		m.detailPreviewBranch = github.Branch{}
		m.detailLoading = true
		m.detailData = nil
		m.detailError = nil
		(&m).resetBrowseDialog()
		return m, tea.Batch(m.fetchPRDetail(newPR), m.fetchPRs())

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

	// Favorites overlay intercepts all other keys
	if m.showFavorites {
		m.showFavorites = false
		return m, nil
	}

	// Clear result banner on any key press
	m.confirmResult = ""

	// Create-PR dialog intercepts all keys (sits above the diff preview).
	if m.createPRMode {
		return m.handleCreatePRKey(msg)
	}

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

	// Browse PR mode
	if m.browseMode {
		return m.handleBrowseKey(msg)
	}

	// Search mode
	if m.searchMode {
		return m.handleSearchKey(msg)
	}

	// Handle pending key sequences (gg)
	if m.pendingKey == "g" {
		m.pendingKey = ""
		if msg.String() == "g" {
			// gg — jump to top
			m.page = 0
			m.cursor = 0
			return m, nil
		}
		// Not a recognized sequence, fall through to normal handling
	}

	switch msg.String() {
	case "q":
		return m, tea.Quit
	case "g":
		m.pendingKey = "g"
		return m, nil
	case "G":
		// Jump to bottom
		all := m.allDisplayItems()
		if len(all) > 0 {
			ps := m.currentPageSize()
			lastIdx := len(all) - 1
			m.page = lastIdx / ps
			m.cursor = lastIdx % ps
		}
		return m, nil
	case "ctrl+d":
		// Half-page down
		ps := m.currentPageSize()
		half := ps / 2
		_, total := m.pagedDisplayItems(ps)
		globalIdx := m.page*ps + m.cursor + half
		if globalIdx >= total {
			globalIdx = total - 1
		}
		if globalIdx >= 0 {
			m.page = globalIdx / ps
			m.cursor = globalIdx % ps
		}
		return m, nil
	case "ctrl+u":
		// Half-page up
		ps := m.currentPageSize()
		half := ps / 2
		globalIdx := m.page*ps + m.cursor - half
		if globalIdx < 0 {
			globalIdx = 0
		}
		m.page = globalIdx / ps
		m.cursor = globalIdx % ps
		return m, nil
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
	case "J", "shift+down":
		// Jump to next repo group
		m.jumpToNextRepoGroup()
		return m, nil
	case "K", "shift+up":
		// Jump to previous repo group
		m.jumpToPrevRepoGroup()
		return m, nil
	case "left", "h":
		ps := m.currentPageSize()
		items, _ := m.pagedDisplayItems(ps)
		// If cursor is on a "more" row and the repo is expanded, collapse by one batch
		if m.cursor >= 0 && m.cursor < len(items) && items[m.cursor].isMore {
			repo := items[m.cursor].repoName
			current := m.repoDisplayLimit(repo)
			if current > maxPRsPerRepo {
				newLimit := current - maxPRsPerRepo
				if newLimit <= maxPRsPerRepo {
					delete(m.expandedRepoLimit, repo)
				} else {
					m.expandedRepoLimit[repo] = newLimit
				}
				m.setCursorToRepoTail(repo)
				return m, nil
			}
		}
		// Otherwise, page backward
		if m.page > 0 {
			m.page--
			m.cursor = 0
		}
		return m, nil
	case "right", "l":
		ps := m.currentPageSize()
		items, _ := m.pagedDisplayItems(ps)
		// If cursor is on a "more" row, expand to show the next batch
		if m.cursor >= 0 && m.cursor < len(items) && items[m.cursor].isMore {
			repo := items[m.cursor].repoName
			m.expandedRepoLimit[repo] = m.repoDisplayLimit(repo) + maxPRsPerRepo
			m.setCursorToRepoTail(repo)
			return m, nil
		}
		// Otherwise, page forward
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
	case "1", "2", "3", "4", "5":
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
		if m.cursor >= 0 && m.cursor < len(items) {
			item := items[m.cursor]
			if item.isPR {
				delete(m.unseenPRs, item.pr.URL)
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
				m.checkUnsafeURL = false
				m.diffCursor = 0
				m.commentMode = false
				m.commentInput = ""
				return m, m.fetchPRDetail(item.pr)
			} else if item.isMore {
				repo := item.repoName
				m.expandedRepoLimit[repo] = m.repoDisplayLimit(repo) + maxPRsPerRepo
				m.setCursorToRepoTail(repo)
				return m, nil
			}
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
			delete(m.expandedRepoLimit, repo) // reset to default limit
			// Move cursor to the repo's header/first item in the new layout
			m.moveCursorToRepo(repo)
		}
		return m, nil
	case "C":
		// Toggle all groups: collapse all if any expanded, expand all if all collapsed
		repos := m.currentRepoNames()
		allCollapsed := true
		for _, name := range repos {
			if !m.collapsedRepos[name] {
				allCollapsed = false
				break
			}
		}
		if allCollapsed {
			m.collapsedRepos = make(map[string]bool)
		} else {
			for _, name := range repos {
				m.collapsedRepos[name] = true
			}
		}
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
	case "B":
		m.browseMode = true
		m.browseInput = ""
		m.browseError = ""
		return m, nil
	case "f":
		ps := m.currentPageSize()
		items, _ := m.pagedDisplayItems(ps)
		if m.cursor >= 0 && m.cursor < len(items) {
			repo := itemRepo(items[m.cursor])
			if repo != "" {
				m.toggleFavorite(repo)
				m.moveCursorToRepo(repo)
			}
		}
		return m, nil
	case "F":
		m.showFavorites = true
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
	case "P":
		// Open the branch picker for the repo under the cursor — works
		// whether the cursor is on a PR row or a repo group header.
		ps := m.currentPageSize()
		items, _ := m.pagedDisplayItems(ps)
		if m.cursor >= 0 && m.cursor < len(items) {
			repoFull := itemRepo(items[m.cursor])
			owner, repo := github.SplitOwnerRepo(repoFull)
			if owner != "" && repo != "" {
				return (&m).enterBranchStep(owner, repo, true)
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

func (m Model) handleBrowseKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Branch-picker step has its own key handling.
	if m.browseBranchStep {
		return m.handleBrowseBranchKey(msg)
	}

	// Handle paste: use raw runes (msg.String() wraps pastes in [...])
	if msg.Paste {
		m.browseInput += string(msg.Runes)
		m.browseError = ""
		cmd := (&m).startRepoSearchIfNeeded()
		return m, cmd
	}

	switch msg.String() {
	case "esc":
		m.resetBrowseDialog()
		return m, nil
	case "up":
		if len(m.browseRepoSuggestions) > 0 && m.browseRepoCursor > 0 {
			m.browseRepoCursor--
		}
		return m, nil
	case "down":
		if len(m.browseRepoSuggestions) > 0 && m.browseRepoCursor < len(m.browseRepoSuggestions)-1 {
			m.browseRepoCursor++
		}
		return m, nil
	case "P":
		// Open the branch picker for the highlighted repo suggestion.
		if len(m.browseRepoSuggestions) > 0 && m.browseRepoCursor >= 0 && m.browseRepoCursor < len(m.browseRepoSuggestions) {
			sel := m.browseRepoSuggestions[m.browseRepoCursor]
			owner, repo := github.SplitOwnerRepo(sel.NameWithOwner)
			if owner != "" && repo != "" {
				return (&m).enterBranchStep(owner, repo, false)
			}
		}
		return m, nil
	case "enter":
		// Enter on a repo suggestion loads its open PRs (original behavior).
		if len(m.browseRepoSuggestions) > 0 && m.browseRepoCursor >= 0 && m.browseRepoCursor < len(m.browseRepoSuggestions) {
			sel := m.browseRepoSuggestions[m.browseRepoCursor]
			owner, repo := github.SplitOwnerRepo(sel.NameWithOwner)
			if owner != "" && repo != "" {
				m.resetBrowseDialog()
				m.activeSection = github.SectionBrowse
				m.cursor = 0
				m.page = 0
				m.loading = true
				return m, m.fetchRepoOpenPRs(owner, repo)
			}
		}
		// Otherwise fall back to the PR-ref parser (owner/repo#N or URL).
		owner, repo, number, err := parsePRReference(m.browseInput)
		if err != nil {
			m.browseError = err.Error()
			return m, nil
		}
		m.resetBrowseDialog()
		m.browsePending = true
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
		pr := github.PullRequest{
			Repository: owner + "/" + repo,
			Number:     number,
		}
		return m, m.fetchPRDetail(pr)
	case "backspace":
		if len(m.browseInput) > 0 {
			m.browseInput = m.browseInput[:len(m.browseInput)-1]
		}
		m.browseError = ""
		cmd := (&m).startRepoSearchIfNeeded()
		return m, cmd
	case "left":
		// no-op: cursor movement not supported, but don't insert the char
		return m, nil
	case "right":
		return m, nil
	default:
		if len(msg.String()) == 1 {
			m.browseInput += msg.String()
			m.browseError = ""
			cmd := (&m).startRepoSearchIfNeeded()
			return m, cmd
		}
		return m, nil
	}
}

// resetBrowseDialog clears every piece of dialog/picker state so the dialog
// reopens fresh next time.
func (m *Model) resetBrowseDialog() {
	m.browseMode = false
	m.browseInput = ""
	m.browseError = ""
	m.browseRepoSuggestions = nil
	m.browseRepoCursor = 0
	m.browseRepoLoading = false
	m.browseRepoError = ""
	m.browseBranchStep = false
	m.browseBranchFromList = false
	m.browseBranchOwner = ""
	m.browseBranchRepo = ""
	m.browseBranchDefault = ""
	m.browseBranches = nil
	m.browseBranchCursor = 0
	m.browseBranchLoading = false
	m.browseBranchError = ""
	m.browsePrevInput = ""
	m.browsePrevSuggestions = nil
	m.browsePrevCursor = 0
}

// enterBranchStep transitions the Browse dialog directly into the
// branch-picker step for a specific repo and kicks off the branch fetch.
// fromList records whether the caller is the list-view P key (true) or the
// in-dialog repo-search P key (false); this drives Esc back-nav.
// Caller is expected to be operating on a pointer receiver.
func (m *Model) enterBranchStep(owner, repo string, fromList bool) (tea.Model, tea.Cmd) {
	// Snapshot the repo-search state when entering from the dialog, so Esc
	// can restore the user's prior search input and suggestion list.
	if !fromList {
		m.browsePrevInput = m.browseInput
		m.browsePrevSuggestions = m.browseRepoSuggestions
		m.browsePrevCursor = m.browseRepoCursor
	} else {
		m.browsePrevInput = ""
		m.browsePrevSuggestions = nil
		m.browsePrevCursor = 0
	}
	m.browseMode = true
	m.browseBranchStep = true
	m.browseBranchFromList = fromList
	m.browseBranchOwner = owner
	m.browseBranchRepo = repo
	m.browseBranchDefault = ""
	m.browseBranches = nil
	m.browseBranchCursor = 0
	m.browseBranchLoading = true
	m.browseBranchError = ""
	m.browseInput = ""
	m.browseError = ""
	m.browseRepoSuggestions = nil
	m.browseRepoCursor = 0
	m.browseRepoLoading = false
	m.browseRepoError = ""
	return *m, m.fetchUserBranches(owner, repo)
}

// handleBrowseBranchKey runs once the user has picked a repo and the dialog
// is showing the viewer's branches in that repo.
func (m Model) handleBrowseBranchKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.browseBranchLoading {
		// Allow esc to back out even while loading.
		if msg.String() == "esc" {
			(&m).leaveBranchStep()
		}
		return m, nil
	}

	switch msg.String() {
	case "esc":
		(&m).leaveBranchStep()
		return m, nil
	case "up", "k":
		if m.browseBranchCursor > 0 {
			m.browseBranchCursor--
		}
		return m, nil
	case "down", "j":
		if m.browseBranchCursor < len(m.browseBranches)-1 {
			m.browseBranchCursor++
		}
		return m, nil
	case "enter":
		if m.browseBranchCursor < 0 || m.browseBranchCursor >= len(m.browseBranches) {
			return m, nil
		}
		branch := m.browseBranches[m.browseBranchCursor]
		base := m.browseBranchDefault
		owner := m.browseBranchOwner
		repo := m.browseBranchRepo
		// Hide the dialog (so the preview shows through) but PRESERVE branch
		// state so Esc on the preview can restore the picker without re-fetch.
		m.browseMode = false
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
		m.diffCursor = 0
		m.detailPreview = true
		m.detailPreviewBranch = branch
		return m, m.fetchCompareDiff(owner, repo, base, branch)
	}
	return m, nil
}

// leaveBranchStep handles Esc from the branch-picker step. If the picker was
// opened directly from the list view (P key), Esc closes the whole dialog.
// Otherwise, Esc backs out to the repo-search step and restores the prior
// search input and suggestion list captured when we entered.
func (m *Model) leaveBranchStep() {
	if m.browseBranchFromList {
		m.resetBrowseDialog()
		return
	}
	m.browseBranchStep = false
	m.browseBranchFromList = false
	m.browseBranchOwner = ""
	m.browseBranchRepo = ""
	m.browseBranchDefault = ""
	m.browseBranches = nil
	m.browseBranchCursor = 0
	m.browseBranchLoading = false
	m.browseBranchError = ""
	// Restore the repo-search state captured on entry.
	m.browseMode = true
	m.browseInput = m.browsePrevInput
	m.browseError = ""
	m.browseRepoSuggestions = m.browsePrevSuggestions
	m.browseRepoCursor = m.browsePrevCursor
	m.browseRepoLoading = false
	m.browseRepoError = ""
	m.browsePrevInput = ""
	m.browsePrevSuggestions = nil
	m.browsePrevCursor = 0
}

// startRepoSearchIfNeeded decides whether the current input looks like a repo
// search (vs a PR reference) and, if so, kicks off a debounced GitHub search.
// Returns nil when the input is a PR ref, too short, or otherwise not searchable.
func (m *Model) startRepoSearchIfNeeded() tea.Cmd {
	input := strings.TrimSpace(m.browseInput)
	// Looks like a PR ref → no repo suggestions.
	if strings.Contains(input, "#") || strings.Contains(input, "github.com/") {
		m.browseRepoSuggestions = nil
		m.browseRepoCursor = 0
		m.browseRepoLoading = false
		m.browseRepoError = ""
		return nil
	}
	if len(input) < 2 {
		m.browseRepoSuggestions = nil
		m.browseRepoCursor = 0
		m.browseRepoLoading = false
		m.browseRepoError = ""
		return nil
	}
	m.browseSearchSeq++
	seq := m.browseSearchSeq
	m.browseRepoLoading = true
	m.browseRepoError = ""
	client := m.client
	orgs := m.orgs
	return tea.Tick(300*time.Millisecond, func(_ time.Time) tea.Msg {
		repos, err := client.SearchRepositories(input, orgs)
		return repoSearchResultMsg{seq: seq, repos: repos, err: err}
	})
}

// parsePRReference parses a PR reference in the form "owner/repo#123" or a
// GitHub URL like "https://github.com/owner/repo/pull/123".
func parsePRReference(input string) (owner, repo string, number int, err error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return "", "", 0, fmt.Errorf("empty input")
	}

	// Try URL: https://github.com/owner/repo/pull/123
	urlRe := regexp.MustCompile(`github\.com/([^/]+)/([^/]+)/pull/(\d+)`)
	if m := urlRe.FindStringSubmatch(input); m != nil {
		n, _ := strconv.Atoi(m[3])
		return m[1], m[2], n, nil
	}

	// Try short form: owner/repo#123
	shortRe := regexp.MustCompile(`^([^/]+)/([^#]+)#(\d+)$`)
	if m := shortRe.FindStringSubmatch(input); m != nil {
		n, _ := strconv.Atoi(m[3])
		return m[1], m[2], n, nil
	}

	return "", "", 0, fmt.Errorf("use owner/repo#123 or GitHub URL")
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
		// In a branch-compare preview, Esc backs to the branch picker rather
		// than dropping straight to the underlying list.
		if m.detailPreview {
			m.showDetail = false
			m.detailData = nil
			m.detailLoading = false
			m.detailError = nil
			m.detailPreview = false
			m.detailPreviewBranch = github.Branch{}
			// Branch-step state is still alive — just re-show the dialog.
			m.browseMode = true
			return m, nil
		}
		m.showDetail = false
		m.detailData = nil
		m.detailLoading = false
		m.detailError = nil
		return m, nil
	case "P":
		// Create-PR from a branch-compare preview.
		if m.detailPreview && m.detailData != nil {
			owner, repo := github.SplitOwnerRepo(m.detailData.Repository)
			m.createPRMode = true
			m.createPROwner = owner
			m.createPRRepo = repo
			m.createPRHead = m.detailData.HeadRefName
			m.createPRTitle = strings.TrimSpace(m.detailPreviewBranch.LastCommitSubject)
			if m.createPRTitle == "" {
				m.createPRTitle = m.detailPreviewBranch.Name
			}
			m.createPRBody = m.detailPreviewBranch.LastCommitBody
			m.createPRBase = m.detailData.BaseRefName
			m.createPRFocus = 0
			m.createPRSubmitting = false
			m.createPRError = ""
		}
		return m, nil
	case "o":
		if m.detailData != nil {
			// Preview has no real URL yet; ignore browser-open.
			if m.detailPreview {
				return m, nil
			}
			// Context-aware: open check URL when info panel is focused with checks
			if m.detailFocus == 1 && m.detailRightTab == 1 && len(m.detailData.Checks) > 0 {
				ch := m.detailData.Checks[m.checkCursor]
				if ch.URL == "" {
					m.checkNoURL = true
					return m, tea.Tick(2*time.Second, func(t time.Time) tea.Msg {
						return clearCheckNoURLMsg{}
					})
				}
				if !openBrowser(ch.URL) {
					m.checkUnsafeURL = true
					return m, tea.Tick(2*time.Second, func(t time.Time) tea.Msg {
						return clearCheckUnsafeURLMsg{}
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
			m.checkCursor = 0
		} else {
			m.detailRightTab = 0
			m.diffScroll = 0
		}
		return m, nil
	case "tab":
		m.detailFocus = 1 - m.detailFocus
		return m, nil
	case "r":
		if m.detailPreview {
			return m, nil
		}
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
		if m.detailData != nil && !m.detailPreview {
			owner, repo := github.SplitOwnerRepo(m.detailData.Repository)
			m.confirmMode = "approve"
			m.confirmInput = ""
			m.confirmPR = &confirmContext{
				owner:  owner,
				repo:   repo,
				number: m.detailData.Number,
				title:  m.detailData.Title,
				nodeID: m.detailData.NodeID,
			}
		}
		return m, nil
	case "X":
		if m.detailData != nil && !m.detailPreview {
			owner, repo := github.SplitOwnerRepo(m.detailData.Repository)
			m.confirmMode = "reject"
			m.confirmInput = ""
			m.confirmPR = &confirmContext{
				owner:  owner,
				repo:   repo,
				number: m.detailData.Number,
				title:  m.detailData.Title,
				nodeID: m.detailData.NodeID,
			}
		}
		return m, nil
	case "M":
		if !m.detailPreview && m.activeSection == github.SectionCreated && m.detailData != nil {
			owner, repo := github.SplitOwnerRepo(m.detailData.Repository)
			m.confirmMode = "merge"
			m.confirmInput = ""
			m.confirmPR = &confirmContext{
				owner:  owner,
				repo:   repo,
				number: m.detailData.Number,
				title:  m.detailData.Title,
				nodeID: m.detailData.NodeID,
			}
		}
		return m, nil
	case "E":
		// Close or reopen PR (E = end the PR's life / reopen it)
		if !m.detailPreview && m.detailData != nil && m.detailData.State != "MERGED" {
			owner, repo := github.SplitOwnerRepo(m.detailData.Repository)
			if m.detailData.State == "CLOSED" {
				m.confirmMode = "reopen"
			} else {
				m.confirmMode = "close"
			}
			m.confirmInput = ""
			m.confirmPR = &confirmContext{
				owner:  owner,
				repo:   repo,
				number: m.detailData.Number,
				title:  m.detailData.Title,
				nodeID: m.detailData.NodeID,
			}
		}
		return m, nil
	case "w":
		// Toggle diff line wrapping (long lines wrap to multiple visual rows
		// instead of being truncated with …).
		if m.detailRightTab == 0 {
			m.diffWrap = !m.diffWrap
			m.diffScroll = 0
		}
		return m, nil
	case "<", ",":
		// Shrink the left (file list) panel.
		m.leftPanelPct = clampLeftPanelPct(m.currentLeftPanelPct() - 5)
		return m, nil
	case ">", ".":
		// Grow the left (file list) panel.
		m.leftPanelPct = clampLeftPanelPct(m.currentLeftPanelPct() + 5)
		return m, nil
	case "D":
		// Toggle draft status
		if !m.detailPreview && m.detailData != nil && m.detailData.State == "OPEN" {
			owner, repo := github.SplitOwnerRepo(m.detailData.Repository)
			if m.detailData.IsDraft {
				m.confirmMode = "ready"
			} else {
				m.confirmMode = "draft"
			}
			m.confirmInput = ""
			m.confirmPR = &confirmContext{
				owner:  owner,
				repo:   repo,
				number: m.detailData.Number,
				title:  m.detailData.Title,
				nodeID: m.detailData.NodeID,
			}
		}
		return m, nil
	case "c":
		if m.detailPreview {
			return m, nil
		}
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
				nodeID: m.detailData.NodeID,
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
				m.checkNoURL = false
				m.checkUnsafeURL = false
			} else {
				max := m.infoMaxScroll()
				if m.infoScroll > max {
					m.infoScroll = max
				}
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
				m.checkNoURL = false
				m.checkUnsafeURL = false
			} else {
				if m.infoScroll < m.infoMaxScroll() {
					m.infoScroll++
				}
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

func (m Model) handleCreatePRKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.createPRSubmitting {
		return m, nil
	}

	// Paste: append into the focused field as raw runes.
	if msg.Paste {
		m.createPRError = ""
		m.appendCreatePRField(string(msg.Runes))
		return m, nil
	}

	switch msg.String() {
	case "esc":
		m.createPRMode = false
		m.createPRError = ""
		m.createPRSubmitting = false
		return m, nil
	case "tab":
		m.createPRFocus = (m.createPRFocus + 1) % 3
		return m, nil
	case "shift+tab":
		m.createPRFocus = (m.createPRFocus + 2) % 3
		return m, nil
	case "ctrl+s":
		// Submit
		if strings.TrimSpace(m.createPRTitle) == "" {
			m.createPRError = "Title is required"
			return m, nil
		}
		if strings.TrimSpace(m.createPRBase) == "" {
			m.createPRError = "Base branch is required"
			return m, nil
		}
		m.createPRSubmitting = true
		m.createPRError = ""
		return m, m.submitCreatePR()
	case "enter":
		// Enter submits from title/base; inserts a newline in the body field.
		if m.createPRFocus == 1 {
			m.createPRBody += "\n"
			return m, nil
		}
		if strings.TrimSpace(m.createPRTitle) == "" {
			m.createPRError = "Title is required"
			return m, nil
		}
		if strings.TrimSpace(m.createPRBase) == "" {
			m.createPRError = "Base branch is required"
			return m, nil
		}
		m.createPRSubmitting = true
		m.createPRError = ""
		return m, m.submitCreatePR()
	case "backspace":
		m.createPRError = ""
		switch m.createPRFocus {
		case 0:
			if len(m.createPRTitle) > 0 {
				m.createPRTitle = m.createPRTitle[:len(m.createPRTitle)-1]
			}
		case 1:
			if len(m.createPRBody) > 0 {
				m.createPRBody = m.createPRBody[:len(m.createPRBody)-1]
			}
		case 2:
			if len(m.createPRBase) > 0 {
				m.createPRBase = m.createPRBase[:len(m.createPRBase)-1]
			}
		}
		return m, nil
	default:
		if len(msg.String()) == 1 {
			m.createPRError = ""
			m.appendCreatePRField(msg.String())
		}
		return m, nil
	}
}

func (m *Model) appendCreatePRField(s string) {
	switch m.createPRFocus {
	case 0:
		m.createPRTitle += s
	case 1:
		m.createPRBody += s
	case 2:
		m.createPRBase += s
	}
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
		noInput := m.confirmMode == "merge" || m.confirmMode == "close" || m.confirmMode == "reopen" || m.confirmMode == "draft" || m.confirmMode == "ready"
		if !noInput && len(m.confirmInput) > 0 {
			m.confirmInput = m.confirmInput[:len(m.confirmInput)-1]
		}
		return m, nil
	default:
		noInput := m.confirmMode == "merge" || m.confirmMode == "close" || m.confirmMode == "reopen" || m.confirmMode == "draft" || m.confirmMode == "ready"
		if !noInput && len(msg.String()) == 1 {
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
		case "close":
			err := client.ClosePR(pr.owner, pr.repo, pr.number)
			if err != nil {
				return prActionResultMsg{success: false, message: fmt.Sprintf("✗ Failed: %v", err)}
			}
			return prActionResultMsg{success: true, message: "✓ PR closed"}
		case "reopen":
			err := client.ReopenPR(pr.owner, pr.repo, pr.number)
			if err != nil {
				return prActionResultMsg{success: false, message: fmt.Sprintf("✗ Failed: %v", err)}
			}
			return prActionResultMsg{success: true, message: "✓ PR reopened"}
		case "draft":
			err := client.ConvertToDraft(pr.nodeID)
			if err != nil {
				return prActionResultMsg{success: false, message: fmt.Sprintf("✗ Failed: %v", err)}
			}
			return prActionResultMsg{success: true, message: "✓ PR converted to draft"}
		case "ready":
			err := client.MarkReadyForReview(pr.nodeID)
			if err != nil {
				return prActionResultMsg{success: false, message: fmt.Sprintf("✗ Failed: %v", err)}
			}
			return prActionResultMsg{success: true, message: "✓ PR marked ready for review"}
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

// hasPRData returns true if any section has PR data from a previous fetch.
func (m Model) hasPRData() bool {
	for _, prs := range m.prs {
		if len(prs) > 0 {
			return true
		}
	}
	return false
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

	// Sort by repository for grouping (favorites pinned to top)
	sortByRepoFavorites(prs, m.favorites)

	return prs
}

// repoDisplayLimit returns how many PRs to show for a repo (default maxPRsPerRepo).
func (m Model) repoDisplayLimit(repo string) int {
	if limit, ok := m.expandedRepoLimit[repo]; ok {
		return limit
	}
	return maxPRsPerRepo
}

// setCursorToRepoTail positions the cursor on the "… and N more" row for the
// given repo, or on the last PR of that repo if all PRs are shown.
// It also adjusts m.page so the target item is on the visible page.
func (m *Model) setCursorToRepoTail(repo string) {
	all := m.allDisplayItems()
	targetIdx := -1
	for i, item := range all {
		if item.isMore && item.repoName == repo {
			targetIdx = i
			break
		}
		if item.isPR && item.pr.Repository == repo {
			targetIdx = i // last PR of this repo as fallback
		}
	}
	if targetIdx < 0 {
		return
	}
	ps := m.currentPageSize()
	m.page = targetIdx / ps
	m.cursor = targetIdx % ps
}

// moveCursorToRepo positions the cursor on the first display item for the given
// repo (collapsed header or first PR), adjusting m.page accordingly.
func (m *Model) moveCursorToRepo(repo string) {
	all := m.allDisplayItems()
	for i, item := range all {
		if (item.isPR && item.pr.Repository == repo) || (!item.isPR && item.repoName == repo) {
			ps := m.currentPageSize()
			m.page = i / ps
			m.cursor = i % ps
			return
		}
	}
}

// jumpToNextRepoGroup moves the cursor to the first item of the next repo group.
func (m *Model) jumpToNextRepoGroup() {
	all := m.allDisplayItems()
	ps := m.currentPageSize()
	globalIdx := m.page*ps + m.cursor

	// Find current repo
	if globalIdx < 0 || globalIdx >= len(all) {
		return
	}
	currentRepo := itemRepo(all[globalIdx])

	// Scan forward past current repo, then land on the first item of the next
	for i := globalIdx + 1; i < len(all); i++ {
		if itemRepo(all[i]) != currentRepo {
			m.page = i / ps
			m.cursor = i % ps
			return
		}
	}
}

// jumpToPrevRepoGroup moves the cursor to the first item of the previous repo group.
func (m *Model) jumpToPrevRepoGroup() {
	all := m.allDisplayItems()
	ps := m.currentPageSize()
	globalIdx := m.page*ps + m.cursor

	if globalIdx <= 0 || globalIdx >= len(all) {
		return
	}
	currentRepo := itemRepo(all[globalIdx])

	// If already on the first item of current group, jump to the previous group
	// Otherwise, jump to the first item of the current group
	firstOfCurrent := globalIdx
	for firstOfCurrent > 0 && itemRepo(all[firstOfCurrent-1]) == currentRepo {
		firstOfCurrent--
	}

	target := firstOfCurrent
	if globalIdx == firstOfCurrent && firstOfCurrent > 0 {
		// Already at group start — find start of previous group
		prevRepo := itemRepo(all[firstOfCurrent-1])
		target = firstOfCurrent - 1
		for target > 0 && itemRepo(all[target-1]) == prevRepo {
			target--
		}
	}

	m.page = target / ps
	m.cursor = target % ps
}

// toggleFavorite flips the favorite bit for repo, syncs cfg.Favorites
// (sorted, deduped) and persists the config. On save failure the result
// banner shows an error; the in-memory toggle is preserved either way.
func (m *Model) toggleFavorite(repo string) {
	if m.favorites == nil {
		m.favorites = map[string]bool{}
	}
	if m.favorites[repo] {
		delete(m.favorites, repo)
	} else {
		m.favorites[repo] = true
	}

	list := make([]string, 0, len(m.favorites))
	for r := range m.favorites {
		list = append(list, r)
	}
	sort.Strings(list)
	m.cfg.Favorites = list

	if err := config.Save(m.cfg); err != nil {
		m.confirmResult = fmt.Sprintf("✗ Saved favorite in-memory but failed to persist: %v", err)
	}
}

func itemRepo(item displayItem) string {
	if item.isPR {
		return item.pr.Repository
	}
	return item.repoName
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
			displayLimit := m.repoDisplayLimit(g.name)
			limit := len(g.prs)
			if limit > displayLimit {
				limit = displayLimit
			}
			for _, pr := range g.prs[:limit] {
				items = append(items, displayItem{isPR: true, pr: pr})
			}
			if len(g.prs) > limit {
				items = append(items, displayItem{
					isMore:   true,
					repoName: g.name,
					count:    len(g.prs) - limit,
				})
			}
		}
	}
	return items
}

// currentRepoNames returns the unique repo names from the current section's filtered PRs.
func (m Model) currentRepoNames() []string {
	var names []string
	seen := map[string]bool{}
	for _, pr := range m.filteredPRs() {
		if !seen[pr.Repository] {
			seen[pr.Repository] = true
			names = append(names, pr.Repository)
		}
	}
	return names
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


// currentPageSize computes how many display items fit in the available terminal height.
func (m Model) currentPageSize() int {
	if m.height == 0 || m.width == 0 {
		return 20
	}
	innerHeight := m.height - 2
	chromeLines := 5
	if m.filterExpr != "" || m.searchMode {
		chromeLines++
	}
	if m.err != nil {
		chromeLines++
	}
	available := innerHeight - chromeLines - linesColumnHeader - 3 // 3 = shortcuts + separator + info row
	if available < minPageSize {
		available = minPageSize
	}

	// Walk forward from start, counting visual lines consumed,
	// to determine how many items fit in the available space.
	all := m.allDisplayItems()
	if len(all) == 0 {
		return minPageSize
	}

	linesUsed := 0
	count := 0
	lastRepo := ""
	groupIndex := 0

	for i := 0; i < len(all); i++ {
		item := all[i]
		repo := item.repoName
		if item.isPR {
			repo = item.pr.Repository
		}

		extraLines := 0
		if repo != lastRepo {
			if groupIndex > 0 {
				extraLines++ // blank separator between groups
			}
			if item.isPR {
				extraLines++ // repo header line
			}
			lastRepo = repo
			groupIndex++
		}

		if linesUsed+extraLines+1 > available && count > 0 {
			break
		}

		linesUsed += extraLines + 1
		count++
	}

	if count < minPageSize {
		count = minPageSize
	}
	return count
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

	// Full-screen centered loading on first fetch
	if m.loading && m.firstLoad {
		hourglass := hourglassFrames[m.hourglassFrame%len(hourglassFrames)]
		loading := spinnerStyle.Render(hourglass + " Fetching pull requests...")
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, loading)
	}

	// Help overlay (full screen, no box)
	if m.showHelp {
		return renderHelp(m.width, m.height)
	}

	// Favorites overlay
	if m.showFavorites {
		return m.renderFavoritesDialog()
	}

	// Create-PR dialog overlay (top-most when active)
	if m.createPRMode {
		return m.renderCreatePRDialog()
	}

	// Confirmation dialog overlay
	if m.confirmMode != "" {
		return m.renderConfirmDialog()
	}

	// Browse PR dialog overlay (must come before detail view so the branch
	// picker remains reachable while a preview is loading in the background).
	if m.browseMode {
		return m.renderBrowseDialog()
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
	if header != "" {
		chrome = append(chrome, header)
	}

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
	// Reserve: shortcuts row + separator + info row = 3 lines
	reservedLines := 3
	availableListHeight := innerHeight - chromeHeight - reservedLines
	if availableListHeight < minPageSize {
		availableListHeight = minPageSize
	}

	// --- Paging ---
	ps := m.currentPageSize()
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

	// Compute per-repo PR counts from filtered PRs.
	repoCounts := map[string]int{}
	for _, pr := range m.filteredPRs() {
		repoCounts[pr.Repository]++
	}

	prList := renderPRList(items, m.cursor, innerWidth, m.unseenPRs, repoCounts, m.favorites)
	prListView := lipgloss.NewStyle().Height(availableListHeight).MaxHeight(availableListHeight).Render(prList)
	sections = append(sections, prListView)

	// Footer: shortcuts row, separator, info row
	sectionTotal := len(m.prs[m.activeSection])
	sections = append(sections, renderShortcutsBar(innerWidth))
	sections = append(sections, renderFooterSeparator(innerWidth))
	sections = append(sections, renderInfoBar(
		"glance",
		sectionShortLabel(m.activeSection),
		sectionTotal,
		m.page,
		pages,
		m.lastRefresh,
		m.loading,
		m.firstLoad,
		m.refreshInterval,
		m.hourglassFrame,
		m.latestVersion,
		innerWidth,
	))

	content := strings.Join(sections, "\n")

	// Wrap everything in a box
	box := screenBoxStyle.
		Width(innerWidth).
		Height(innerHeight).
		Render(content)

	return box
}

func renderHeader(orgs []string, width int) string {
	if len(orgs) == 0 {
		return ""
	}

	// Available width for the org list — leave a 2-col gutter on the right.
	avail := width - 2 - len("orgs: ")
	if avail < 4 {
		avail = 4
	}
	list := strings.Join(orgs, ", ")
	if lipgloss.Width(list) > avail {
		list = truncate(list, avail)
	}
	orgInfo := ageStyle.Render("orgs: " + list)

	orgWidth := lipgloss.Width(orgInfo)
	gap := width - orgWidth - 2
	if gap < 0 {
		gap = 0
	}

	return strings.Repeat(" ", gap) + orgInfo
}

func (m Model) renderFavoritesDialog() string {
	title := lipgloss.NewStyle().Bold(true).Foreground(primaryColor).Render("★ Favorite Repos")

	var lines []string
	lines = append(lines, title, "")

	if len(m.favorites) == 0 {
		lines = append(lines, emptyStyle.Render("No favorites yet — press f on a repo to pin it."))
	} else {
		repos := make([]string, 0, len(m.favorites))
		for r := range m.favorites {
			repos = append(repos, r)
		}
		sort.Strings(repos)
		for _, r := range repos {
			lines = append(lines, "  "+helpKeyStyle.Render("★")+" "+helpDescStyle.Render(r))
		}
	}

	lines = append(lines, "")
	lines = append(lines, helpDescStyle.Render("Press any key to close"))

	content := strings.Join(lines, "\n")
	overlay := helpOverlayStyle.Render(content)
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, overlay)
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

func (m Model) renderBrowseDialog() string {
	boxW := m.width * 2 / 3
	if boxW < 54 {
		boxW = 54
	}
	if boxW > 100 {
		boxW = 100
	}
	if boxW > m.width-4 {
		boxW = m.width - 4
	}
	inputW := boxW - 10
	if inputW < 30 {
		inputW = 30
	}

	if m.browseBranchStep {
		return m.renderBrowseBranchStep(boxW)
	}

	var lines []string
	title := lipgloss.NewStyle().Bold(true).Foreground(secondaryColor).Render("Browse PR")
	lines = append(lines, title)
	lines = append(lines, "")
	lines = append(lines, helpDescStyle.Render("Type owner/repo#N or a URL — or search a repo by name:"))
	inputBox := confirmInputStyle.Width(inputW).Render(m.browseInput + "█")
	lines = append(lines, inputBox)

	if m.browseError != "" {
		lines = append(lines, lipgloss.NewStyle().Foreground(dangerColor).Render(m.browseError))
	}

	// Repo suggestions area (only when the input is repo-name-shaped).
	if m.browseRepoLoading || m.browseRepoError != "" || len(m.browseRepoSuggestions) > 0 {
		lines = append(lines, "")
		switch {
		case m.browseRepoError != "":
			lines = append(lines, lipgloss.NewStyle().Foreground(dangerColor).Render("Search error: "+m.browseRepoError))
		case m.browseRepoLoading && len(m.browseRepoSuggestions) == 0:
			lines = append(lines, spinnerStyle.Render("⟳ Searching repos..."))
		case len(m.browseRepoSuggestions) == 0:
			lines = append(lines, emptyStyle.Render("No matching repos"))
		default:
			for i, r := range m.browseRepoSuggestions {
				name := r.NameWithOwner
				desc := r.Description
				row := helpKeyStyle.Render(name)
				if desc != "" {
					row += " " + helpDescStyle.Render(truncate(desc, 36))
				}
				if i == m.browseRepoCursor {
					lines = append(lines, selectedPRStyle.Render("▸ "+row))
				} else {
					lines = append(lines, "  "+row)
				}
			}
		}
	}

	lines = append(lines, "")
	enterKey := helpKeyStyle.Render("Enter")
	escKey := helpKeyStyle.Render("Esc")
	if len(m.browseRepoSuggestions) > 0 {
		upDown := helpKeyStyle.Render("↑↓")
		newKey := helpKeyStyle.Render("P")
		lines = append(lines, upDown+" pick  •  "+enterKey+" view PRs  •  "+newKey+" new PR  •  "+escKey+" cancel")
	} else {
		lines = append(lines, enterKey+" open  •  "+escKey+" cancel")
	}

	content := strings.Join(lines, "\n")
	overlay := helpOverlayStyle.Width(boxW).Render(content)
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, overlay)
}

func (m Model) renderBrowseBranchStep(boxW int) string {
	var lines []string
	repoLabel := m.browseBranchOwner + "/" + m.browseBranchRepo
	title := lipgloss.NewStyle().Bold(true).Foreground(secondaryColor).Render("Pick branch — " + repoLabel)
	lines = append(lines, title)
	lines = append(lines, "")
	lines = append(lines, helpDescStyle.Render("Your branches in this repo (Enter to preview diff & create PR):"))
	lines = append(lines, "")

	switch {
	case m.browseBranchLoading:
		lines = append(lines, spinnerStyle.Render("⟳ Loading branches..."))
	case m.browseBranchError != "":
		lines = append(lines, lipgloss.NewStyle().Foreground(dangerColor).Render(m.browseBranchError))
	case len(m.browseBranches) == 0:
		lines = append(lines, emptyStyle.Render("No branches found where you authored any of the last few commits."))
	default:
		maxRows := 12
		start := 0
		if m.browseBranchCursor >= maxRows {
			start = m.browseBranchCursor - maxRows + 1
		}
		end := start + maxRows
		if end > len(m.browseBranches) {
			end = len(m.browseBranches)
		}

		// Content width inside the overlay: subtract border (2) + padding (4).
		contentW := boxW - 6
		if contentW < 30 {
			contentW = 30
		}

		// Compute column widths from the visible slice. Branch and author
		// columns are sized to the longest value in view, capped so very long
		// names don't squeeze out the subject column.
		const (
			branchCap = 32
			authorCap = 20
			gutter    = 2
			prefixW   = 2 // "▸ " or "  "
		)
		branchW := 0
		authorW := 0
		for i := start; i < end; i++ {
			b := m.browseBranches[i]
			if w := lipgloss.Width(b.Name); w > branchW {
				branchW = w
			}
			a := b.LastCommitAuthor
			if a != "" {
				if w := lipgloss.Width("@" + a); w > authorW {
					authorW = w
				}
			}
		}
		if branchW > branchCap {
			branchW = branchCap
		}
		if authorW > authorCap {
			authorW = authorCap
		}
		subjectW := contentW - prefixW - branchW - gutter - authorW - gutter
		if subjectW < 10 {
			subjectW = 10
		}

		padRight := func(s string, w int) string {
			gap := w - lipgloss.Width(s)
			if gap <= 0 {
				return s
			}
			return s + strings.Repeat(" ", gap)
		}
		padLeft := func(s string, w int) string {
			gap := w - lipgloss.Width(s)
			if gap <= 0 {
				return s
			}
			return strings.Repeat(" ", gap) + s
		}

		for i := start; i < end; i++ {
			b := m.browseBranches[i]
			subject := b.LastCommitSubject
			if subject == "" {
				subject = "(no commit subject)"
			}
			authorRaw := ""
			if b.LastCommitAuthor != "" {
				authorRaw = "@" + b.LastCommitAuthor
			}

			nameCell := padRight(truncate(b.Name, branchW), branchW)
			subjCell := padRight(truncate(subject, subjectW), subjectW)
			authorCell := padLeft(truncate(authorRaw, authorW), authorW)

			row := helpKeyStyle.Render(nameCell) +
				strings.Repeat(" ", gutter) +
				helpDescStyle.Render(subjCell) +
				strings.Repeat(" ", gutter) +
				ageStyle.Render(authorCell)

			if i == m.browseBranchCursor {
				lines = append(lines, selectedPRStyle.Render("▸ "+row))
			} else {
				lines = append(lines, "  "+row)
			}
		}
		if end < len(m.browseBranches) {
			lines = append(lines, helpDescStyle.Render(fmt.Sprintf("  …%d more", len(m.browseBranches)-end)))
		}
	}

	lines = append(lines, "")
	enterKey := helpKeyStyle.Render("Enter")
	escKey := helpKeyStyle.Render("Esc")
	upDown := helpKeyStyle.Render("↑↓")
	lines = append(lines, upDown+" pick  •  "+enterKey+" preview diff  •  "+escKey+" cancel")

	content := strings.Join(lines, "\n")
	overlay := helpOverlayStyle.Width(boxW).Render(content)
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, overlay)
}

func (m Model) renderCreatePRDialog() string {
	var lines []string

	boxW := m.width * 2 / 3
	if boxW < 60 {
		boxW = 60
	}
	if boxW > 110 {
		boxW = 110
	}
	if boxW > m.width-4 {
		boxW = m.width - 4
	}
	inputW := boxW - 10
	if inputW < 40 {
		inputW = 40
	}

	title := lipgloss.NewStyle().Bold(true).Foreground(primaryColor).Render(
		fmt.Sprintf("Create PR — %s/%s", m.createPROwner, m.createPRRepo))
	lines = append(lines, title)
	lines = append(lines, "")
	lines = append(lines, helpDescStyle.Render(fmt.Sprintf("from %s into %s", m.createPRHead, m.createPRBase)))
	lines = append(lines, "")

	field := func(label, value string, focused bool, height int) {
		labelStr := helpDescStyle.Render(label)
		cursor := ""
		if focused {
			cursor = "█"
		}
		box := confirmInputStyle.Width(inputW)
		if height > 1 {
			box = box.Height(height)
		}
		content := value + cursor
		lines = append(lines, labelStr)
		lines = append(lines, box.Render(content))
	}

	field("Title (required):", m.createPRTitle, m.createPRFocus == 0, 1)
	lines = append(lines, "")
	field("Body:", m.createPRBody, m.createPRFocus == 1, 4)
	lines = append(lines, "")
	field("Base branch:", m.createPRBase, m.createPRFocus == 2, 1)

	if m.createPRError != "" {
		lines = append(lines, "")
		lines = append(lines, lipgloss.NewStyle().Foreground(dangerColor).Render(m.createPRError))
	}

	lines = append(lines, "")
	if m.createPRSubmitting {
		lines = append(lines, spinnerStyle.Render("⟳ Creating PR..."))
	} else {
		tabKey := helpKeyStyle.Render("Tab")
		enterKey := helpKeyStyle.Render("Enter")
		submitKey := helpKeyStyle.Render("Ctrl+S")
		escKey := helpKeyStyle.Render("Esc")
		lines = append(lines, tabKey+" next field  •  "+enterKey+" submit ("+helpKeyStyle.Render("\\n")+" in body)  •  "+submitKey+" submit  •  "+escKey+" cancel")
	}

	content := strings.Join(lines, "\n")
	overlay := helpOverlayStyle.Width(boxW).Render(content)
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

	case "close":
		title := lipgloss.NewStyle().Bold(true).Foreground(dangerColor).Render(
			fmt.Sprintf("Close PR #%d?", pr.number))
		lines = append(lines, title)
		lines = append(lines, "")
		lines = append(lines, detailBodyStyle.Render(pr.title))

	case "reopen":
		title := lipgloss.NewStyle().Bold(true).Foreground(successColor).Render(
			fmt.Sprintf("Reopen PR #%d?", pr.number))
		lines = append(lines, title)
		lines = append(lines, "")
		lines = append(lines, detailBodyStyle.Render(pr.title))

	case "draft":
		title := lipgloss.NewStyle().Bold(true).Foreground(warningColor).Render(
			fmt.Sprintf("Convert PR #%d to draft?", pr.number))
		lines = append(lines, title)
		lines = append(lines, "")
		lines = append(lines, detailBodyStyle.Render(pr.title))

	case "ready":
		title := lipgloss.NewStyle().Bold(true).Foreground(successColor).Render(
			fmt.Sprintf("Mark PR #%d ready for review?", pr.number))
		lines = append(lines, title)
		lines = append(lines, "")
		lines = append(lines, detailBodyStyle.Render(pr.title))
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

func (m Model) fetchRepoOpenPRs(owner, repo string) tea.Cmd {
	client := m.client
	return func() tea.Msg {
		prs, err := client.FetchOpenPRsInRepo(owner, repo)
		return repoPRsLoadedMsg{prs: prs, err: err}
	}
}

func (m Model) fetchUserBranches(owner, repo string) tea.Cmd {
	client := m.client
	username := m.username
	return func() tea.Msg {
		info, err := client.ListUserBranches(owner, repo, username)
		return branchesLoadedMsg{
			owner:         owner,
			repo:          repo,
			defaultBranch: info.DefaultBranch,
			branches:      info.Branches,
			err:           err,
		}
	}
}

func (m Model) fetchCompareDiff(owner, repo, base string, branch github.Branch) tea.Cmd {
	client := m.client
	return func() tea.Msg {
		files, err := client.CompareBranches(owner, repo, base, branch.Name)
		return compareLoadedMsg{
			owner:  owner,
			repo:   repo,
			base:   base,
			branch: branch,
			files:  files,
			err:    err,
		}
	}
}

func (m Model) submitCreatePR() tea.Cmd {
	client := m.client
	owner := m.createPROwner
	repo := m.createPRRepo
	head := m.createPRHead
	title := strings.TrimSpace(m.createPRTitle)
	body := m.createPRBody
	base := strings.TrimSpace(m.createPRBase)
	return func() tea.Msg {
		pr, err := client.CreatePullRequest(owner, repo, title, body, head, base, false)
		if err != nil {
			return createPRResultMsg{success: false, message: fmt.Sprintf("✗ Failed to create PR: %v", err), owner: owner, repo: repo}
		}
		return createPRResultMsg{success: true, message: fmt.Sprintf("✓ PR #%d created", pr.Number), pr: pr, owner: owner, repo: repo}
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

// checkLatestRelease asks GitHub for the latest published release tag once at
// startup. Any error is silent — the footer hint only appears on success.
func checkLatestRelease() tea.Cmd {
	return func() tea.Msg {
		tag, err := github.LatestReleaseTag()
		if err != nil {
			return latestReleaseMsg{tag: ""}
		}
		return latestReleaseMsg{tag: tag}
	}
}

// isNewerVersion returns true when remote is strictly newer than local. Both
// may be prefixed with "v" and may carry a pre-release suffix ("-rc1"), which
// is ignored for the comparison. Returns false if local is "dev" or either
// version is unparseable, so unknown states never surface as a false alarm.
func isNewerVersion(local, remote string) bool {
	if remote == "" {
		return false
	}
	if local == "" || local == "dev" {
		return false
	}
	lp, ok := parseSemver(local)
	if !ok {
		return false
	}
	rp, ok := parseSemver(remote)
	if !ok {
		return false
	}
	for i := 0; i < 3; i++ {
		if rp[i] != lp[i] {
			return rp[i] > lp[i]
		}
	}
	return false
}

func parseSemver(v string) ([3]int, bool) {
	var out [3]int
	s := strings.TrimPrefix(strings.TrimSpace(v), "v")
	// Drop pre-release / build metadata
	if i := strings.IndexAny(s, "-+"); i >= 0 {
		s = s[:i]
	}
	parts := strings.SplitN(s, ".", 4)
	if len(parts) < 1 {
		return out, false
	}
	for i := 0; i < 3 && i < len(parts); i++ {
		n, err := strconv.Atoi(parts[i])
		if err != nil {
			return out, false
		}
		out[i] = n
	}
	return out, true
}

// openBrowser opens rawURL in the user's default browser. Returns false if the
// URL was refused by isSafeBrowserURL (caller may want to surface a banner).
func openBrowser(rawURL string) bool {
	if !isSafeBrowserURL(rawURL) {
		return false
	}
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", rawURL)
	case "linux":
		cmd = exec.Command("xdg-open", rawURL)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", rawURL)
	default:
		return false
	}
	_ = cmd.Start()
	return true
}

// isSafeBrowserURL allows only http(s) URLs. Status check URLs come from CI
// integrations and are not trusted: a leading "-" would let open/xdg-open
// parse the URL as a flag, and schemes like file:// / javascript: / smb:
// can be turned into local file disclosure or credential leaks.
func isSafeBrowserURL(rawURL string) bool {
	if rawURL == "" || strings.HasPrefix(rawURL, "-") {
		return false
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	switch u.Scheme {
	case "http", "https":
		return u.Host != ""
	}
	return false
}
