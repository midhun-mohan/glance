package github

import (
	"fmt"
	"time"
)

type PRStatus string

const (
	PRStatusOpen   PRStatus = "Open"
	PRStatusDraft  PRStatus = "Draft"
	PRStatusMerged PRStatus = "Merged"
	PRStatusClosed PRStatus = "Closed"
)

type ReviewStatus string

const (
	ReviewApproved        ReviewStatus = "Approved"
	ReviewChangesReq      ReviewStatus = "Changes Requested"
	ReviewPending         ReviewStatus = "Pending"
	ReviewRequired        ReviewStatus = "Review Required"
)

type PullRequest struct {
	Title              string
	Repository         string
	Author             string
	Status             PRStatus
	ReviewStatus       ReviewStatus
	ChecksState        string // SUCCESS, FAILURE, PENDING, or "" (no checks)
	URL                string
	CreatedAt          time.Time
	UpdatedAt          time.Time
	Labels             []string
	Number             int
	IsDraft            bool
	Assignees          []string
	RequestedReviewers []string
}

func (pr PullRequest) Age() string {
	d := time.Since(pr.CreatedAt)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	case d < 7*24*time.Hour:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	default:
		weeks := int(d.Hours() / 24 / 7)
		if weeks < 5 {
			return fmt.Sprintf("%dw ago", weeks)
		}
		return fmt.Sprintf("%dmo ago", int(d.Hours()/24/30))
	}
}

func (pr PullRequest) IsReadyToMerge() bool {
	return pr.ChecksState == "SUCCESS" && pr.ReviewStatus == ReviewApproved
}

type Section int

const (
	SectionCreated Section = iota
	SectionReviewRequested
	SectionAssigned
	SectionMentions
	SectionBrowse
)

func (s Section) String() string {
	switch s {
	case SectionCreated:
		return "Created by me"
	case SectionReviewRequested:
		return "Review requested"
	case SectionAssigned:
		return "Assigned to me"
	case SectionMentions:
		return "Mentions"
	case SectionBrowse:
		return "Browse"
	default:
		return "Unknown"
	}
}

// PRFromDetail converts a PRDetail into a PullRequest summary.
func PRFromDetail(d *PRDetail) PullRequest {
	status := PRStatusOpen
	if d.IsDraft {
		status = PRStatusDraft
	}
	switch d.State {
	case "MERGED":
		status = PRStatusMerged
	case "CLOSED":
		status = PRStatusClosed
	}

	var reviewStatus ReviewStatus
	switch d.ReviewDecision {
	case "APPROVED":
		reviewStatus = ReviewApproved
	case "CHANGES_REQUESTED":
		reviewStatus = ReviewChangesReq
	case "REVIEW_REQUIRED":
		reviewStatus = ReviewRequired
	default:
		reviewStatus = ReviewPending
	}

	checksState := ""
	if len(d.Checks) > 0 {
		allSuccess := true
		anyFailure := false
		for _, ch := range d.Checks {
			if ch.Status != CheckSuccess && ch.Status != CheckSkipped && ch.Status != CheckNeutral {
				allSuccess = false
			}
			if ch.Status == CheckFailure {
				anyFailure = true
			}
		}
		switch {
		case anyFailure:
			checksState = "FAILURE"
		case allSuccess:
			checksState = "SUCCESS"
		default:
			checksState = "PENDING"
		}
	}

	return PullRequest{
		Title:        d.Title,
		Repository:   d.Repository,
		Author:       d.Author,
		Status:       status,
		ReviewStatus: reviewStatus,
		ChecksState:  checksState,
		URL:          d.URL,
		CreatedAt:    d.CreatedAt,
		UpdatedAt:    d.UpdatedAt,
		Labels:       d.Labels,
		Number:       d.Number,
		IsDraft:      d.IsDraft,
		Assignees:    d.Assignees,
	}
}

type PRsBySection map[Section][]PullRequest

// --- Rich detail types (fetched on-demand for a single PR) ---

type CheckStatus string

const (
	CheckSuccess    CheckStatus = "SUCCESS"
	CheckFailure    CheckStatus = "FAILURE"
	CheckPending    CheckStatus = "PENDING"
	CheckInProgress CheckStatus = "IN_PROGRESS"
	CheckSkipped    CheckStatus = "SKIPPED"
	CheckNeutral    CheckStatus = "NEUTRAL"
)

type CheckRun struct {
	Name   string
	Status CheckStatus
	URL    string
}

type ReviewEntry struct {
	Author string
	State  string // APPROVED, CHANGES_REQUESTED, COMMENTED, DISMISSED
}

type PRComment struct {
	Author    string
	Body      string
	CreatedAt time.Time
}

type ReviewComment struct {
	Author    string
	Body      string
	Path      string
	Line      int
	Position  int    // 1-based position in the diff (0 if unavailable)
	Side      string // "LEFT" or "RIGHT"
	CreatedAt time.Time
}

type ChangedFile struct {
	Filename         string
	Status           string // added, removed, modified, renamed, copied
	Additions        int
	Deletions        int
	Patch            string // unified diff content
	PreviousFilename string // only for renamed files
}

type PRDetail struct {
	Title          string
	Body           string
	URL            string
	Repository     string
	Author         string
	Number         int
	Additions      int
	Deletions      int
	ChangedFiles   int
	CommentsCount  int
	State          string // OPEN, CLOSED, MERGED
	Mergeable      string // MERGEABLE, CONFLICTING, UNKNOWN
	ReviewDecision string
	BaseRefName    string
	HeadRefName    string
	HeadCommitSHA  string
	IsDraft        bool
	CreatedAt      time.Time
	UpdatedAt      time.Time
	MergedAt       *time.Time
	MergedBy       string
	Assignees      []string
	Labels         []string
	Reviews        []ReviewEntry
	Checks         []CheckRun
	Files          []ChangedFile
	ReviewComments []ReviewComment
	Comments       []PRComment
}
