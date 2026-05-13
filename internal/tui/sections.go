package tui

import (
	"github.com/charmbracelet/lipgloss"
	"github.com/midhun-mohan/glance/internal/github"
)

var sectionOrder = []github.Section{
	github.SectionCreated,
	github.SectionReviewRequested,
	github.SectionAssigned,
	github.SectionMentions,
	github.SectionBrowse,
}

var sectionLabels = map[github.Section]string{
	github.SectionCreated:         "[1] Created by me",
	github.SectionReviewRequested: "[2] Review requested",
	github.SectionAssigned:        "[3] Assigned",
	github.SectionMentions:        "[4] Mentions",
	github.SectionBrowse:          "[5] Browse",
}

// sectionShortLabel returns the section name without the leading shortcut prefix,
// for use in the footer where the shortcut number would be redundant.
var sectionShortLabels = map[github.Section]string{
	github.SectionCreated:         "Created by me",
	github.SectionReviewRequested: "Review requested",
	github.SectionAssigned:        "Assigned",
	github.SectionMentions:        "Mentions",
	github.SectionBrowse:          "Browse",
}

func sectionShortLabel(s github.Section) string {
	if v, ok := sectionShortLabels[s]; ok {
		return v
	}
	return ""
}

func renderTabs(active github.Section, counts, unseenCounts map[github.Section]int, width int) string {
	_ = counts // total counts are surfaced in the footer, not the tab labels
	var tabParts []string
	for _, s := range sectionOrder {
		tabText := sectionLabels[s]
		if unseen := unseenCounts[s]; unseen > 0 {
			tabText += " " + unseenDotStyle.Render(itoa(unseen) + " new")
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
