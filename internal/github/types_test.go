package github

import (
	"testing"
	"time"
)

func TestPullRequestAge(t *testing.T) {
	tests := []struct {
		name string
		age  time.Duration
		want string
	}{
		{"just now", 10 * time.Second, "just now"},
		{"minutes", 15 * time.Minute, "15m ago"},
		{"hours", 5 * time.Hour, "5h ago"},
		{"days", 3 * 24 * time.Hour, "3d ago"},
		{"weeks", 2 * 7 * 24 * time.Hour, "2w ago"},
		{"months", 60 * 24 * time.Hour, "2mo ago"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pr := PullRequest{CreatedAt: time.Now().Add(-tt.age)}
			got := pr.Age()
			if got != tt.want {
				t.Errorf("Age() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestIsReadyToMerge(t *testing.T) {
	tests := []struct {
		name   string
		checks string
		review ReviewStatus
		want   bool
	}{
		{"ready", "SUCCESS", ReviewApproved, true},
		{"checks failed", "FAILURE", ReviewApproved, false},
		{"not approved", "SUCCESS", ReviewPending, false},
		{"no checks", "", ReviewApproved, false},
		{"both bad", "FAILURE", ReviewChangesReq, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pr := PullRequest{ChecksState: tt.checks, ReviewStatus: tt.review}
			if got := pr.IsReadyToMerge(); got != tt.want {
				t.Errorf("IsReadyToMerge() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSectionString(t *testing.T) {
	tests := []struct {
		section Section
		want    string
	}{
		{SectionCreated, "Created by me"},
		{SectionReviewRequested, "Review requested"},
		{SectionAssigned, "Assigned to me"},
		{SectionMentions, "Mentions"},
		{Section(99), "Unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.section.String(); got != tt.want {
				t.Errorf("String() = %q, want %q", got, tt.want)
			}
		})
	}
}
