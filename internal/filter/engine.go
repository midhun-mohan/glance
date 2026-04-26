package filter

import (
	"strings"
	"time"

	"github.com/midhunmohan/mygit/internal/github"
)

func (fs FilterSet) IsEmpty() bool {
	return len(fs.Filters) == 0
}

func (fs FilterSet) Match(pr github.PullRequest) bool {
	for _, f := range fs.Filters {
		if !matchFilter(f, pr) {
			return false
		}
	}
	return true
}

func matchFilter(f Filter, pr github.PullRequest) bool {
	switch f.Type {
	case FilterRepo:
		return matchWildcard(f.Value, pr.Repository)
	case FilterStatus:
		return matchStatus(f.Value, pr)
	case FilterLabel:
		return matchLabel(f.Value, pr)
	case FilterAuthor:
		return strings.EqualFold(f.Value, pr.Author)
	case FilterCreated:
		return matchDate(f, pr.CreatedAt)
	case FilterUpdated:
		return matchDate(f, pr.UpdatedAt)
	default:
		return true
	}
}

func matchWildcard(pattern, value string) bool {
	pattern = strings.ToLower(pattern)
	value = strings.ToLower(value)

	if strings.HasSuffix(pattern, "/*") {
		prefix := strings.TrimSuffix(pattern, "/*")
		return strings.HasPrefix(value, prefix+"/")
	}
	if strings.Contains(pattern, "*") {
		parts := strings.Split(pattern, "*")
		if len(parts) == 2 {
			return strings.HasPrefix(value, parts[0]) && strings.HasSuffix(value, parts[1])
		}
	}
	return pattern == value
}

func matchStatus(status string, pr github.PullRequest) bool {
	switch strings.ToLower(status) {
	case "open":
		return pr.Status == github.PRStatusOpen
	case "draft":
		return pr.Status == github.PRStatusDraft
	case "merged":
		return pr.Status == github.PRStatusMerged
	case "closed":
		return pr.Status == github.PRStatusClosed
	default:
		return false
	}
}

func matchLabel(label string, pr github.PullRequest) bool {
	for _, l := range pr.Labels {
		if strings.EqualFold(l, label) {
			return true
		}
	}
	return false
}

func matchDate(f Filter, t time.Time) bool {
	target, ok := ParseDate(f.Value)
	if !ok {
		return true
	}
	switch f.Operator {
	case OpGreaterThan:
		return t.After(target)
	case OpLessThan:
		return t.Before(target)
	default:
		return t.After(target)
	}
}
