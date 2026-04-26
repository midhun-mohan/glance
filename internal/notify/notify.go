package notify

import (
	"fmt"
	"runtime"

	"github.com/midhunmohan/mygit/internal/github"
)

type Notifier struct {
	enabled bool
	events  EventConfig
	backend Backend
}

type EventConfig struct {
	NewAssignment   bool
	ReviewRequested bool
	StatusChange    bool
	Mentions        bool
}

type Backend interface {
	Send(title, message string) error
}

func New(enabled bool, events EventConfig) *Notifier {
	var backend Backend
	switch runtime.GOOS {
	case "darwin":
		backend = &macOSBackend{}
	case "linux":
		backend = &linuxBackend{}
	default:
		backend = &noopBackend{}
	}

	return &Notifier{
		enabled: enabled,
		events:  events,
		backend: backend,
	}
}

func (n *Notifier) Diff(oldPRs, newPRs github.PRsBySection) {
	if !n.enabled {
		return
	}

	if n.events.ReviewRequested {
		n.diffSection(oldPRs, newPRs, github.SectionReviewRequested,
			"Review requested", "Review requested: %s in %s")
	}
	if n.events.NewAssignment {
		n.diffSection(oldPRs, newPRs, github.SectionAssigned,
			"New PR assigned", "New PR assigned: %s in %s")
	}
	if n.events.Mentions {
		n.diffSection(oldPRs, newPRs, github.SectionMentions,
			"New mention", "You were mentioned in %s (%s)")
	}
}

func (n *Notifier) diffSection(oldPRs, newPRs github.PRsBySection, section github.Section, title, msgFmt string) {
	oldSet := make(map[string]bool)
	for _, pr := range oldPRs[section] {
		oldSet[pr.URL] = true
	}

	for _, pr := range newPRs[section] {
		if !oldSet[pr.URL] {
			msg := fmt.Sprintf(msgFmt, pr.Title, pr.Repository)
			_ = n.backend.Send(title, msg)
		}
	}
}

type noopBackend struct{}

func (b *noopBackend) Send(_, _ string) error {
	return nil
}
