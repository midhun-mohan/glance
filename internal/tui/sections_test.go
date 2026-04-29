package tui

import (
	"testing"

	"github.com/midhun-mohan/glance/internal/github"
)

func TestNextSection(t *testing.T) {
	tests := []struct {
		current github.Section
		want    github.Section
	}{
		{github.SectionCreated, github.SectionReviewRequested},
		{github.SectionReviewRequested, github.SectionAssigned},
		{github.SectionAssigned, github.SectionMentions},
		{github.SectionMentions, github.SectionBrowse},
		{github.SectionBrowse, github.SectionCreated}, // wraps around
	}

	for _, tt := range tests {
		got := nextSection(tt.current)
		if got != tt.want {
			t.Errorf("nextSection(%d) = %d, want %d", tt.current, got, tt.want)
		}
	}
}

func TestPrevSection(t *testing.T) {
	tests := []struct {
		current github.Section
		want    github.Section
	}{
		{github.SectionCreated, github.SectionBrowse}, // wraps around
		{github.SectionReviewRequested, github.SectionCreated},
		{github.SectionAssigned, github.SectionReviewRequested},
		{github.SectionMentions, github.SectionAssigned},
		{github.SectionBrowse, github.SectionMentions},
	}

	for _, tt := range tests {
		got := prevSection(tt.current)
		if got != tt.want {
			t.Errorf("prevSection(%d) = %d, want %d", tt.current, got, tt.want)
		}
	}
}

func TestSectionByNumber(t *testing.T) {
	tests := []struct {
		n    int
		want github.Section
		ok   bool
	}{
		{1, github.SectionCreated, true},
		{2, github.SectionReviewRequested, true},
		{3, github.SectionAssigned, true},
		{4, github.SectionMentions, true},
		{5, github.SectionBrowse, true},
		{0, 0, false},
		{6, 0, false},
		{-1, 0, false},
	}

	for _, tt := range tests {
		got, ok := sectionByNumber(tt.n)
		if ok != tt.ok {
			t.Errorf("sectionByNumber(%d) ok = %v, want %v", tt.n, ok, tt.ok)
		}
		if ok && got != tt.want {
			t.Errorf("sectionByNumber(%d) = %d, want %d", tt.n, got, tt.want)
		}
	}
}

func TestItoa(t *testing.T) {
	tests := []struct {
		n    int
		want string
	}{
		{0, "0"},
		{1, "1"},
		{42, "42"},
		{100, "100"},
		{999, "999"},
	}

	for _, tt := range tests {
		got := itoa(tt.n)
		if got != tt.want {
			t.Errorf("itoa(%d) = %q, want %q", tt.n, got, tt.want)
		}
	}
}
