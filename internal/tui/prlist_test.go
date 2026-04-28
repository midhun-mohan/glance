package tui

import (
	"testing"

	"github.com/midhun-mohan/glance/internal/github"
)

func TestTruncate(t *testing.T) {
	tests := []struct {
		name string
		s    string
		max  int
		want string
	}{
		{"short", "hello", 10, "hello"},
		{"exact", "hello", 5, "hello"},
		{"truncated", "hello world", 8, "hello w…"},
		{"empty", "", 5, ""},
		{"one char max", "hello", 2, "h…"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := truncate(tt.s, tt.max)
			if got != tt.want {
				t.Errorf("truncate(%q, %d) = %q, want %q", tt.s, tt.max, got, tt.want)
			}
		})
	}
}

func TestStripANSI(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"no ansi", "hello", "hello"},
		{"color code", "\x1b[31mred\x1b[0m", "red"},
		{"bold", "\x1b[1mbold\x1b[0m", "bold"},
		{"multiple", "\x1b[32mgreen\x1b[0m and \x1b[34mblue\x1b[0m", "green and blue"},
		{"empty", "", ""},
		{"unterminated escape dropped", "hello\x1b[31m", "hello"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stripANSI(tt.input)
			if got != tt.want {
				t.Errorf("stripANSI(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestCountDisplayItemOverhead(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		got := countDisplayItemOverhead(nil)
		if got != 0 {
			t.Errorf("got %d, want 0", got)
		}
	})

	t.Run("single repo group", func(t *testing.T) {
		items := []displayItem{
			{isPR: true, pr: github.PullRequest{Repository: "acme/web"}},
			{isPR: true, pr: github.PullRequest{Repository: "acme/web"}},
		}
		got := countDisplayItemOverhead(items)
		// 1 expanded group header, no separator before first
		if got != 1 {
			t.Errorf("got %d, want 1", got)
		}
	})

	t.Run("two repo groups", func(t *testing.T) {
		items := []displayItem{
			{isPR: true, pr: github.PullRequest{Repository: "acme/web"}},
			{isPR: true, pr: github.PullRequest{Repository: "acme/api"}},
		}
		got := countDisplayItemOverhead(items)
		// 2 group headers + 1 blank separator between them = 3
		if got != 3 {
			t.Errorf("got %d, want 3", got)
		}
	})

	t.Run("collapsed group no extra overhead", func(t *testing.T) {
		items := []displayItem{
			{repoName: "acme/web", count: 5},
		}
		got := countDisplayItemOverhead(items)
		// collapsed item IS the display item, no overhead
		if got != 0 {
			t.Errorf("got %d, want 0", got)
		}
	})
}

func TestSortByRepo(t *testing.T) {
	prs := []github.PullRequest{
		{Repository: "zoo/app", Title: "z1"},
		{Repository: "acme/web", Title: "a1"},
		{Repository: "acme/web", Title: "a2"},
		{Repository: "mid/api", Title: "m1"},
	}
	sortByRepo(prs)

	want := []string{"acme/web", "acme/web", "mid/api", "zoo/app"}
	for i, pr := range prs {
		if pr.Repository != want[i] {
			t.Errorf("prs[%d].Repository = %q, want %q", i, pr.Repository, want[i])
		}
	}

	// Verify stable sort: acme/web PRs keep original order
	if prs[0].Title != "a1" || prs[1].Title != "a2" {
		t.Error("stable sort violated: same-repo PRs reordered")
	}
}
