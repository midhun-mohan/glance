package filter

import (
	"testing"
	"time"
)

func TestParse(t *testing.T) {
	tests := []struct {
		name     string
		expr     string
		wantLen  int
		wantType []FilterType
	}{
		{"empty", "", 0, nil},
		{"single repo", "repo:acme/web", 1, []FilterType{FilterRepo}},
		{"multiple filters", "repo:acme/* status:open label:bug", 3, []FilterType{FilterRepo, FilterStatus, FilterLabel}},
		{"unknown key ignored", "foo:bar repo:x/y", 1, []FilterType{FilterRepo}},
		{"no colon ignored", "justtext", 0, nil},
		{"author filter", "author:alice", 1, []FilterType{FilterAuthor}},
		{"created filter", "created:<7d", 1, []FilterType{FilterCreated}},
		{"updated filter", "updated:>3d", 1, []FilterType{FilterUpdated}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fs := Parse(tt.expr)
			if len(fs.Filters) != tt.wantLen {
				t.Fatalf("Parse(%q) got %d filters, want %d", tt.expr, len(fs.Filters), tt.wantLen)
			}
			for i, wantType := range tt.wantType {
				if fs.Filters[i].Type != wantType {
					t.Errorf("filter[%d].Type = %d, want %d", i, fs.Filters[i].Type, wantType)
				}
			}
		})
	}
}

func TestParseToken(t *testing.T) {
	tests := []struct {
		token    string
		wantOk   bool
		wantType FilterType
		wantVal  string
		wantOp   Operator
	}{
		{"repo:acme/web", true, FilterRepo, "acme/web", OpEquals},
		{"status:open", true, FilterStatus, "open", OpEquals},
		{"status:DRAFT", true, FilterStatus, "draft", OpEquals},
		{"label:urgent", true, FilterLabel, "urgent", OpEquals},
		{"author:alice", true, FilterAuthor, "alice", OpEquals},
		{"created:<7d", true, FilterCreated, "7d", OpLessThan},
		{"created:>2w", true, FilterCreated, "2w", OpGreaterThan},
		{"updated:<1d", true, FilterUpdated, "1d", OpLessThan},
		{"nocolon", false, 0, "", 0},
		{"unknown:val", false, 0, "", 0},
	}

	for _, tt := range tests {
		t.Run(tt.token, func(t *testing.T) {
			f, ok := parseToken(tt.token)
			if ok != tt.wantOk {
				t.Fatalf("parseToken(%q) ok = %v, want %v", tt.token, ok, tt.wantOk)
			}
			if !ok {
				return
			}
			if f.Type != tt.wantType {
				t.Errorf("Type = %d, want %d", f.Type, tt.wantType)
			}
			if f.Value != tt.wantVal {
				t.Errorf("Value = %q, want %q", f.Value, tt.wantVal)
			}
			if f.Operator != tt.wantOp {
				t.Errorf("Operator = %d, want %d", f.Operator, tt.wantOp)
			}
		})
	}
}

func TestParseOperatorValue(t *testing.T) {
	tests := []struct {
		input   string
		wantOp  Operator
		wantVal string
	}{
		{">5d", OpGreaterThan, "5d"},
		{"<3h", OpLessThan, "3h"},
		{"7d", OpEquals, "7d"},
		{"", OpEquals, ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			op, val := parseOperatorValue(tt.input)
			if op != tt.wantOp {
				t.Errorf("op = %d, want %d", op, tt.wantOp)
			}
			if val != tt.wantVal {
				t.Errorf("val = %q, want %q", val, tt.wantVal)
			}
		})
	}
}

func TestParseDuration(t *testing.T) {
	tests := []struct {
		input string
		want  time.Duration
		ok    bool
	}{
		{"7d", 7 * 24 * time.Hour, true},
		{"3h", 3 * time.Hour, true},
		{"30m", 30 * time.Minute, true},
		{"2w", 14 * 24 * time.Hour, true},
		{"1d", 24 * time.Hour, true},
		{"0d", 0, true},
		{"", 0, false},
		{"d", 0, false},
		{"abc", 0, false},
		{"7x", 0, false},
		{"7", 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, ok := ParseDuration(tt.input)
			if ok != tt.ok {
				t.Fatalf("ParseDuration(%q) ok = %v, want %v", tt.input, ok, tt.ok)
			}
			if got != tt.want {
				t.Errorf("ParseDuration(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestParseDate(t *testing.T) {
	t.Run("relative duration", func(t *testing.T) {
		got, ok := ParseDate("7d")
		if !ok {
			t.Fatal("expected ok")
		}
		expected := time.Now().Add(-7 * 24 * time.Hour)
		diff := got.Sub(expected)
		if diff < -time.Second || diff > time.Second {
			t.Errorf("ParseDate(7d) off by %v", diff)
		}
	})

	t.Run("absolute date", func(t *testing.T) {
		got, ok := ParseDate("2024-06-15")
		if !ok {
			t.Fatal("expected ok")
		}
		expected := time.Date(2024, 6, 15, 0, 0, 0, 0, time.UTC)
		if !got.Equal(expected) {
			t.Errorf("got %v, want %v", got, expected)
		}
	})

	t.Run("invalid", func(t *testing.T) {
		_, ok := ParseDate("notadate")
		if ok {
			t.Error("expected not ok")
		}
	})
}
