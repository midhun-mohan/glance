package filter

import (
	"testing"
	"time"

	"github.com/midhun-mohan/glance/internal/github"
)

func TestFilterSetIsEmpty(t *testing.T) {
	if !(FilterSet{}).IsEmpty() {
		t.Error("empty FilterSet should be empty")
	}
	if (FilterSet{Filters: []Filter{{}}}).IsEmpty() {
		t.Error("FilterSet with filters should not be empty")
	}
}

func TestMatchWildcard(t *testing.T) {
	tests := []struct {
		pattern string
		value   string
		want    bool
	}{
		{"acme/web", "acme/web", true},
		{"acme/web", "acme/api", false},
		{"acme/*", "acme/web", true},
		{"acme/*", "acme/api", true},
		{"acme/*", "other/web", false},
		{"ACME/*", "acme/web", true},
		{"acme/*", "ACME/WEB", true},
		{"*-api", "acme-api", true},
		{"*-api", "acme-web", false},
		{"pre*fix", "prefix", true},
		{"pre*fix", "pre-something-fix", true},
		{"pre*fix", "pre-something-other", false},
		{"exact", "exact", true},
		{"exact", "other", false},
	}

	for _, tt := range tests {
		t.Run(tt.pattern+"_"+tt.value, func(t *testing.T) {
			got := matchWildcard(tt.pattern, tt.value)
			if got != tt.want {
				t.Errorf("matchWildcard(%q, %q) = %v, want %v", tt.pattern, tt.value, got, tt.want)
			}
		})
	}
}

func TestMatchStatus(t *testing.T) {
	tests := []struct {
		status string
		prStat github.PRStatus
		want   bool
	}{
		{"open", github.PRStatusOpen, true},
		{"Open", github.PRStatusOpen, true},
		{"open", github.PRStatusDraft, false},
		{"draft", github.PRStatusDraft, true},
		{"merged", github.PRStatusMerged, true},
		{"closed", github.PRStatusClosed, true},
		{"unknown", github.PRStatusOpen, false},
	}

	for _, tt := range tests {
		t.Run(tt.status+"_"+string(tt.prStat), func(t *testing.T) {
			pr := github.PullRequest{Status: tt.prStat}
			got := matchStatus(tt.status, pr)
			if got != tt.want {
				t.Errorf("matchStatus(%q, %v) = %v, want %v", tt.status, tt.prStat, got, tt.want)
			}
		})
	}
}

func TestMatchLabel(t *testing.T) {
	pr := github.PullRequest{Labels: []string{"bug", "Urgent", "P1"}}

	tests := []struct {
		label string
		want  bool
	}{
		{"bug", true},
		{"BUG", true},
		{"urgent", true},
		{"p1", true},
		{"feature", false},
	}

	for _, tt := range tests {
		t.Run(tt.label, func(t *testing.T) {
			got := matchLabel(tt.label, pr)
			if got != tt.want {
				t.Errorf("matchLabel(%q) = %v, want %v", tt.label, got, tt.want)
			}
		})
	}

	t.Run("no labels", func(t *testing.T) {
		empty := github.PullRequest{}
		if matchLabel("anything", empty) {
			t.Error("expected no match on empty labels")
		}
	})
}

func TestMatchDate(t *testing.T) {
	now := time.Now()

	// ParseDate("7d") returns (now - 7d). OpLessThan checks t.Before(target).
	// So created:<7d matches PRs whose CreatedAt is BEFORE (now - 7d), i.e. OLDER than 7 days.

	t.Run("less than 7d (old PR matches)", func(t *testing.T) {
		f := Filter{Type: FilterCreated, Operator: OpLessThan, Value: "7d"}
		old := now.Add(-30 * 24 * time.Hour)
		if !matchDate(f, old) {
			t.Error("30-day-old PR should match created:<7d (before the cutoff)")
		}
	})

	t.Run("less than 7d (recent PR doesn't match)", func(t *testing.T) {
		f := Filter{Type: FilterCreated, Operator: OpLessThan, Value: "7d"}
		recent := now.Add(-2 * 24 * time.Hour)
		if matchDate(f, recent) {
			t.Error("2-day-old PR should not match created:<7d (after the cutoff)")
		}
	})

	t.Run("greater than 7d (recent PR matches)", func(t *testing.T) {
		f := Filter{Type: FilterCreated, Operator: OpGreaterThan, Value: "7d"}
		recent := now.Add(-2 * 24 * time.Hour)
		if !matchDate(f, recent) {
			t.Error("2-day-old PR should match created:>7d (after the cutoff date)")
		}
	})

	t.Run("invalid date value passes", func(t *testing.T) {
		f := Filter{Type: FilterCreated, Operator: OpLessThan, Value: "notadate"}
		if !matchDate(f, now) {
			t.Error("invalid date should pass (return true)")
		}
	})
}

func TestFilterSetMatch(t *testing.T) {
	pr := github.PullRequest{
		Repository: "acme/web",
		Status:     github.PRStatusOpen,
		Author:     "alice",
		Labels:     []string{"bug"},
		CreatedAt:  time.Now().Add(-2 * 24 * time.Hour),
	}

	t.Run("all match", func(t *testing.T) {
		fs := Parse("repo:acme/* status:open author:alice")
		if !fs.Match(pr) {
			t.Error("expected match")
		}
	})

	t.Run("one fails", func(t *testing.T) {
		fs := Parse("repo:acme/* status:merged")
		if fs.Match(pr) {
			t.Error("expected no match")
		}
	})

	t.Run("empty filter matches all", func(t *testing.T) {
		fs := Parse("")
		if !fs.Match(pr) {
			t.Error("empty filter should match everything")
		}
	})
}
