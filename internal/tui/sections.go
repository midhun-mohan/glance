package tui

import (
	"github.com/charmbracelet/lipgloss"
	"github.com/midhunmohan/mygit/internal/github"
)

var sectionOrder = []github.Section{
	github.SectionCreated,
	github.SectionReviewRequested,
	github.SectionAssigned,
	github.SectionMentions,
}

var sectionLabels = map[github.Section]string{
	github.SectionCreated:         "Created by me",
	github.SectionReviewRequested: "Review requested",
	github.SectionAssigned:        "Assigned",
	github.SectionMentions:        "Mentions",
}

func renderTabs(active github.Section, counts map[github.Section]int, width int) string {
	var tabParts []string
	for _, s := range sectionOrder {
		label := sectionLabels[s]
		count := counts[s]
		tabText := label
		if count > 0 {
			tabText = label + " (" + itoa(count) + ")"
		}
		if s == active {
			tabParts = append(tabParts, activeTabStyle.Render(tabText))
		} else {
			tabParts = append(tabParts, inactiveTabStyle.Render(tabText))
		}
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, tabParts...)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	s := ""
	for n > 0 {
		s = string(rune('0'+n%10)) + s
		n /= 10
	}
	return s
}

func nextSection(current github.Section) github.Section {
	for i, s := range sectionOrder {
		if s == current {
			return sectionOrder[(i+1)%len(sectionOrder)]
		}
	}
	return sectionOrder[0]
}

func prevSection(current github.Section) github.Section {
	for i, s := range sectionOrder {
		if s == current {
			return sectionOrder[(i-1+len(sectionOrder))%len(sectionOrder)]
		}
	}
	return sectionOrder[0]
}

func sectionByNumber(n int) (github.Section, bool) {
	if n < 1 || n > len(sectionOrder) {
		return 0, false
	}
	return sectionOrder[n-1], true
}
