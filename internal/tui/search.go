package tui

import (
	"strings"

	"github.com/midhunmohan/mygit/internal/github"
	"github.com/sahilm/fuzzy"
)

type searchablePRs struct {
	prs []github.PullRequest
}

func (s searchablePRs) String(i int) string {
	pr := s.prs[i]
	return pr.Title + " " + pr.Repository + " " + pr.Author
}

func (s searchablePRs) Len() int {
	return len(s.prs)
}

func fuzzyFilter(query string, prs []github.PullRequest) []github.PullRequest {
	if query == "" {
		return prs
	}

	source := searchablePRs{prs: prs}
	matches := fuzzy.FindFrom(query, source)

	result := make([]github.PullRequest, len(matches))
	for i, m := range matches {
		result[i] = prs[m.Index]
	}
	return result
}

func renderFilterBar(filterExpr string, searchMode bool, searchQuery string, width int) string {
	if searchMode {
		prompt := filterChipStyle.Render("/")
		return filterBarStyle.Width(width).Render(prompt + " " + searchQuery + "█")
	}

	if filterExpr != "" {
		parts := strings.Fields(filterExpr)
		var chips []string
		for _, p := range parts {
			chips = append(chips, filterChipStyle.Render(p))
		}
		return filterBarStyle.Width(width).Render("Filter: " + strings.Join(chips, " "))
	}

	// Show active fuzzy search after exiting search input mode
	if searchQuery != "" {
		return filterBarStyle.Width(width).Render("Search: " + filterChipStyle.Render(searchQuery) + "  " + helpDescStyle.Render("Esc to clear"))
	}

	return ""
}
