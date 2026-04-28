package github

import "testing"

func TestSplitOwnerRepo(t *testing.T) {
	tests := []struct {
		input     string
		wantOwner string
		wantRepo  string
	}{
		{"acme/web", "acme", "web"},
		{"org/my-repo", "org", "my-repo"},
		{"noslash", "noslash", ""},
		{"a/b/c", "a", "b/c"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			owner, repo := SplitOwnerRepo(tt.input)
			if owner != tt.wantOwner || repo != tt.wantRepo {
				t.Errorf("SplitOwnerRepo(%q) = (%q, %q), want (%q, %q)", tt.input, owner, repo, tt.wantOwner, tt.wantRepo)
			}
		})
	}
}

func TestMapCheckRunStatus(t *testing.T) {
	tests := []struct {
		status     string
		conclusion string
		want       CheckStatus
	}{
		{"COMPLETED", "SUCCESS", CheckSuccess},
		{"COMPLETED", "FAILURE", CheckFailure},
		{"COMPLETED", "TIMED_OUT", CheckFailure},
		{"COMPLETED", "ACTION_REQUIRED", CheckFailure},
		{"COMPLETED", "SKIPPED", CheckSkipped},
		{"COMPLETED", "NEUTRAL", CheckNeutral},
		{"COMPLETED", "UNKNOWN", CheckFailure},
		{"IN_PROGRESS", "", CheckInProgress},
		{"QUEUED", "", CheckPending},
		{"", "", CheckPending},
		{"completed", "success", CheckSuccess},
	}

	for _, tt := range tests {
		t.Run(tt.status+"_"+tt.conclusion, func(t *testing.T) {
			got := mapCheckRunStatus(tt.status, tt.conclusion)
			if got != tt.want {
				t.Errorf("mapCheckRunStatus(%q, %q) = %q, want %q", tt.status, tt.conclusion, got, tt.want)
			}
		})
	}
}

func TestMapStatusContextState(t *testing.T) {
	tests := []struct {
		state string
		want  CheckStatus
	}{
		{"SUCCESS", CheckSuccess},
		{"FAILURE", CheckFailure},
		{"ERROR", CheckFailure},
		{"PENDING", CheckPending},
		{"", CheckPending},
		{"success", CheckSuccess},
	}

	for _, tt := range tests {
		t.Run(tt.state, func(t *testing.T) {
			got := mapStatusContextState(tt.state)
			if got != tt.want {
				t.Errorf("mapStatusContextState(%q) = %q, want %q", tt.state, got, tt.want)
			}
		})
	}
}

func TestMapPRStatus(t *testing.T) {
	tests := []struct {
		state   string
		isDraft bool
		want    PRStatus
	}{
		{"OPEN", false, PRStatusOpen},
		{"OPEN", true, PRStatusDraft},
		{"MERGED", false, PRStatusMerged},
		{"CLOSED", false, PRStatusClosed},
		{"UNKNOWN", false, PRStatusOpen},
	}

	for _, tt := range tests {
		t.Run(tt.state, func(t *testing.T) {
			got := mapPRStatus(tt.state, tt.isDraft)
			if got != tt.want {
				t.Errorf("mapPRStatus(%q, %v) = %q, want %q", tt.state, tt.isDraft, got, tt.want)
			}
		})
	}
}

func TestMapReviewStatus(t *testing.T) {
	tests := []struct {
		decision string
		want     ReviewStatus
	}{
		{"APPROVED", ReviewApproved},
		{"CHANGES_REQUESTED", ReviewChangesReq},
		{"REVIEW_REQUIRED", ReviewRequired},
		{"", ReviewPending},
		{"UNKNOWN", ReviewPending},
	}

	for _, tt := range tests {
		t.Run(tt.decision, func(t *testing.T) {
			got := mapReviewStatus(tt.decision)
			if got != tt.want {
				t.Errorf("mapReviewStatus(%q) = %q, want %q", tt.decision, got, tt.want)
			}
		})
	}
}
