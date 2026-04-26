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
	Title        string
	Repository   string
	Author       string
	Status       PRStatus
	ReviewStatus ReviewStatus
	ChecksState  string // SUCCESS, FAILURE, PENDING, or "" (no checks)
	URL          string
	CreatedAt    time.Time
	UpdatedAt    time.Time
	Labels       []string
	Number       int
	IsDraft      bool
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
	default:
		return "Unknown"
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
}

type ReviewEntry struct {
	Author string
	State  string // APPROVED, CHANGES_REQUESTED, COMMENTED, DISMISSED
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
}
