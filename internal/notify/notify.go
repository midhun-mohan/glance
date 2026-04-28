package notify

import (
	"fmt"
	"runtime"
	"strings"

	"github.com/midhun-mohan/glance/internal/github"
)

type Notifier struct {
	enabled  bool
	events   EventConfig
	backend  Backend
	username string
}

type EventConfig struct {
	NewAssignment   bool
	ReviewRequested bool
	StatusChange    bool
	Mentions        bool
	IncludeTeam     bool
}

type Backend interface {
	Send(title, message string) error
}

func New(enabled bool, events EventConfig, username string) *Notifier {
	var backend Backend
	switch runtime.GOOS {
	case "darwin":
		backend = &macOSBackend{}
	case "linux":
		backend = &linuxBackend{}
	case "windows":
		backend = &windowsBackend{}
	default:
		backend = &noopBackend{}
	}

	return &Notifier{
		enabled:  enabled,
		events:   events,
		backend:  backend,
		username: username,
	}
}

func (n *Notifier) Diff(oldPRs, newPRs github.PRsBySection) {
	if !n.enabled {
		return
	}

	if n.events.ReviewRequested {
		n.diffSection(oldPRs, newPRs, github.SectionReviewRequested,
			"Review requested", "Review requested: %s in %s",
			n.directReviewFilter())
	}
	if n.events.NewAssignment {
		n.diffSection(oldPRs, newPRs, github.SectionAssigned,
			"New PR assigned", "New PR assigned: %s in %s",
			n.directAssignFilter())
	}
	if n.events.Mentions {
		n.diffSection(oldPRs, newPRs, github.SectionMentions,
			"New mention", "You were mentioned in %s (%s)", nil)
	}
}

// directReviewFilter returns a filter that only passes PRs where the user
// is directly in the requested reviewers list. Returns nil if team reviews
// are included (no filtering).
func (n *Notifier) directReviewFilter() func(github.PullRequest) bool {
	if n.events.IncludeTeam || n.username == "" {
		return nil
	}
	return func(pr github.PullRequest) bool {
		for _, r := range pr.RequestedReviewers {
			if strings.EqualFold(r, n.username) {
				return true
			}
		}
		return false
	}
}

// directAssignFilter returns a filter that only passes PRs where the user
// is directly in the assignees list. Returns nil if team assignments are included.
func (n *Notifier) directAssignFilter() func(github.PullRequest) bool {
	if n.events.IncludeTeam || n.username == "" {
		return nil
	}
	return func(pr github.PullRequest) bool {
		for _, a := range pr.Assignees {
			if strings.EqualFold(a, n.username) {
				return true
			}
		}
		return false
	}
}

func (n *Notifier) diffSection(oldPRs, newPRs github.PRsBySection, section github.Section, title, msgFmt string, filter func(github.PullRequest) bool) {
	oldSet := make(map[string]bool)
	for _, pr := range oldPRs[section] {
		oldSet[pr.URL] = true
	}

	for _, pr := range newPRs[section] {
		if !oldSet[pr.URL] {
			if filter != nil && !filter(pr) {
				continue
			}
			msg := fmt.Sprintf(msgFmt, pr.Title, pr.Repository)
			_ = n.backend.Send(title, msg)
		}
	}
}

type noopBackend struct{}

func (b *noopBackend) Send(_, _ string) error {
	return nil
}
