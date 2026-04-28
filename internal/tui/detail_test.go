package tui

import (
	"testing"
	"time"

	"github.com/midhun-mohan/glance/internal/github"
)

func TestParseHunkHeader(t *testing.T) {
	tests := []struct {
		line     string
		wantOld  int
		wantNew  int
	}{
		{"@@ -1,3 +1,4 @@", 1, 1},
		{"@@ -10,6 +12,8 @@ func main() {", 10, 12},
		{"@@ -0,0 +1,25 @@", 0, 1},
		{"@@ -100 +200,5 @@", 100, 200},
		{"@@ -1 +1 @@", 1, 1},
	}

	for _, tt := range tests {
		t.Run(tt.line, func(t *testing.T) {
			old, new := parseHunkHeader(tt.line)
			if old != tt.wantOld || new != tt.wantNew {
				t.Errorf("parseHunkHeader(%q) = (%d, %d), want (%d, %d)", tt.line, old, new, tt.wantOld, tt.wantNew)
			}
		})
	}
}

func TestParseDiffLinesMeta(t *testing.T) {
	patch := "@@ -1,3 +1,4 @@\n context\n-removed\n+added\n+added2\n context2"

	meta := parseDiffLinesMeta(patch)

	if len(meta) != 6 {
		t.Fatalf("got %d lines, want 6", len(meta))
	}

	// @@ line
	if meta[0].commentable {
		t.Error("hunk header should not be commentable")
	}

	// context line
	if !meta[1].commentable || meta[1].side != "RIGHT" || meta[1].line != 1 {
		t.Errorf("context line: commentable=%v, side=%q, line=%d", meta[1].commentable, meta[1].side, meta[1].line)
	}

	// removed line
	if !meta[2].commentable || meta[2].side != "LEFT" || meta[2].line != 2 {
		t.Errorf("removed line: commentable=%v, side=%q, line=%d", meta[2].commentable, meta[2].side, meta[2].line)
	}

	// added line
	if !meta[3].commentable || meta[3].side != "RIGHT" || meta[3].line != 2 {
		t.Errorf("added line: commentable=%v, side=%q, line=%d", meta[3].commentable, meta[3].side, meta[3].line)
	}

	// added2 line
	if !meta[4].commentable || meta[4].side != "RIGHT" || meta[4].line != 3 {
		t.Errorf("added2 line: commentable=%v, side=%q, line=%d", meta[4].commentable, meta[4].side, meta[4].line)
	}

	// context2 line
	if !meta[5].commentable || meta[5].side != "RIGHT" || meta[5].line != 4 {
		t.Errorf("context2 line: commentable=%v, side=%q, line=%d", meta[5].commentable, meta[5].side, meta[5].line)
	}
}

func TestParseDiffLinesMetaMultiHunk(t *testing.T) {
	patch := "@@ -1,2 +1,2 @@\n-old\n+new\n@@ -10,2 +10,2 @@\n-old2\n+new2"
	meta := parseDiffLinesMeta(patch)

	if len(meta) != 6 {
		t.Fatalf("got %d lines, want 6", len(meta))
	}

	// Second hunk header should not be commentable
	if meta[3].commentable {
		t.Error("second hunk header should not be commentable")
	}

	// Lines after second hunk should use the second hunk's line numbers
	if meta[4].line != 10 || meta[4].side != "LEFT" {
		t.Errorf("second hunk removed: line=%d side=%q, want line=10 side=LEFT", meta[4].line, meta[4].side)
	}
	if meta[5].line != 10 || meta[5].side != "RIGHT" {
		t.Errorf("second hunk added: line=%d side=%q, want line=10 side=RIGHT", meta[5].line, meta[5].side)
	}
}

func TestBuildFileComments(t *testing.T) {
	comments := []github.ReviewComment{
		{Author: "alice", Body: "fix this", Path: "main.go", Line: 10, Side: "RIGHT"},
		{Author: "bob", Body: "looks good", Path: "main.go", Line: 10, Side: "RIGHT"},
		{Author: "carol", Body: "nit", Path: "util.go", Line: 5, Side: "LEFT"},
		{Author: "dave", Body: "position comment", Path: "main.go", Position: 3},
	}

	fc := buildFileComments(comments, "main.go")

	// Line/side-based lookup
	k := commentKey{path: "main.go", line: 10, side: "RIGHT"}
	if len(fc.byLineSide[k]) != 2 {
		t.Errorf("main.go:10:RIGHT got %d comments, want 2", len(fc.byLineSide[k]))
	}

	// Position-based lookup
	if len(fc.byPosition[3]) != 1 {
		t.Errorf("position 3 got %d comments, want 1", len(fc.byPosition[3]))
	}

	// Different file filtered out
	fc2 := buildFileComments(comments, "util.go")
	k2 := commentKey{path: "util.go", line: 5, side: "LEFT"}
	if len(fc2.byLineSide[k2]) != 1 {
		t.Errorf("util.go:5:LEFT got %d comments, want 1", len(fc2.byLineSide[k2]))
	}
	if len(fc2.byPosition) != 0 {
		t.Errorf("util.go should have no position-based comments")
	}
}

func TestCommentsForDiffLine(t *testing.T) {
	comments := []github.ReviewComment{
		{Author: "alice", Body: "by position", Path: "main.go", Position: 2},
		{Author: "bob", Body: "by line", Path: "main.go", Line: 5, Side: "RIGHT"},
	}
	fc := buildFileComments(comments, "main.go")

	t.Run("match by position", func(t *testing.T) {
		// diffIndex=1 -> position=2
		got := fc.commentsForDiffLine(1, diffLineInfo{line: 99, side: "LEFT", commentable: true}, "main.go")
		if len(got) != 1 || got[0].Author != "alice" {
			t.Errorf("expected alice's position comment, got %v", got)
		}
	})

	t.Run("match by line/side fallback", func(t *testing.T) {
		got := fc.commentsForDiffLine(99, diffLineInfo{line: 5, side: "RIGHT", commentable: true}, "main.go")
		if len(got) != 1 || got[0].Author != "bob" {
			t.Errorf("expected bob's line comment, got %v", got)
		}
	})

	t.Run("no match", func(t *testing.T) {
		got := fc.commentsForDiffLine(50, diffLineInfo{line: 999, side: "RIGHT", commentable: true}, "main.go")
		if len(got) != 0 {
			t.Errorf("expected no match, got %v", got)
		}
	})
}

func TestWrapText(t *testing.T) {
	tests := []struct {
		name  string
		text  string
		width int
		want  int // number of lines
	}{
		{"short", "hello", 80, 1},
		{"exact", "hello world", 11, 1},
		{"wrap", "hello world foo bar", 10, 3},
		{"empty", "", 80, 1},
		{"newlines", "line1\nline2\nline3", 80, 3},
		{"zero width", "hello", 0, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lines := wrapText(tt.text, tt.width)
			if len(lines) != tt.want {
				t.Errorf("wrapText(%q, %d) = %d lines, want %d: %v", tt.text, tt.width, len(lines), tt.want, lines)
			}
		})
	}
}

func TestSanitizeDiffLine(t *testing.T) {
	t.Run("replaces tabs", func(t *testing.T) {
		got := sanitizeDiffLine("\tfoo", 80)
		if got != "    foo" {
			t.Errorf("got %q, want %q", got, "    foo")
		}
	})

	t.Run("truncates long lines", func(t *testing.T) {
		long := "+" + string(make([]byte, 200))
		got := sanitizeDiffLine(long, 50)
		if len(got) > 60 { // some overhead from truncation char
			t.Errorf("line not truncated: len=%d", len(got))
		}
	})
}

func TestFormatCommentAge(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name string
		t    time.Time
		want string
	}{
		{"minutes", now.Add(-30 * time.Minute), "30m ago"},
		{"hours", now.Add(-5 * time.Hour), "5h ago"},
		{"days", now.Add(-3 * 24 * time.Hour), "3d ago"},
		{"old", now.Add(-30 * 24 * time.Hour), now.Add(-30 * 24 * time.Hour).Format("Jan 2")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatCommentAge(tt.t)
			if got != tt.want {
				t.Errorf("formatCommentAge() = %q, want %q", got, tt.want)
			}
		})
	}
}
