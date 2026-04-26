package filter

import (
	"strings"
	"time"
)

type FilterType int

const (
	FilterRepo FilterType = iota
	FilterStatus
	FilterLabel
	FilterAuthor
	FilterCreated
	FilterUpdated
)

type Operator int

const (
	OpEquals Operator = iota
	OpGreaterThan
	OpLessThan
)

type Filter struct {
	Type     FilterType
	Value    string
	Operator Operator
}

type FilterSet struct {
	Filters []Filter
}

func Parse(expr string) FilterSet {
	fs := FilterSet{}
	if expr == "" {
		return fs
	}

	tokens := strings.Fields(expr)
	for _, token := range tokens {
		if f, ok := parseToken(token); ok {
			fs.Filters = append(fs.Filters, f)
		}
	}
	return fs
}

func parseToken(token string) (Filter, bool) {
	parts := strings.SplitN(token, ":", 2)
	if len(parts) != 2 {
		return Filter{}, false
	}

	key := strings.ToLower(parts[0])
	value := parts[1]

	switch key {
	case "repo":
		return Filter{Type: FilterRepo, Value: value, Operator: OpEquals}, true
	case "status":
		return Filter{Type: FilterStatus, Value: strings.ToLower(value), Operator: OpEquals}, true
	case "label":
		return Filter{Type: FilterLabel, Value: value, Operator: OpEquals}, true
	case "author":
		return Filter{Type: FilterAuthor, Value: value, Operator: OpEquals}, true
	case "created", "updated":
		ft := FilterCreated
		if key == "updated" {
			ft = FilterUpdated
		}
		op, val := parseOperatorValue(value)
		return Filter{Type: ft, Value: val, Operator: op}, true
	default:
		return Filter{}, false
	}
}

func parseOperatorValue(value string) (Operator, string) {
	if strings.HasPrefix(value, ">") {
		return OpGreaterThan, value[1:]
	}
	if strings.HasPrefix(value, "<") {
		return OpLessThan, value[1:]
	}
	return OpEquals, value
}

// ParseDuration parses relative date strings like "7d", "3h", "2w"
func ParseDuration(value string) (time.Duration, bool) {
	if len(value) < 2 {
		return 0, false
	}
	suffix := value[len(value)-1]
	numStr := value[:len(value)-1]

	var num int
	for _, ch := range numStr {
		if ch < '0' || ch > '9' {
			return 0, false
		}
		num = num*10 + int(ch-'0')
	}

	switch suffix {
	case 'm':
		return time.Duration(num) * time.Minute, true
	case 'h':
		return time.Duration(num) * time.Hour, true
	case 'd':
		return time.Duration(num) * 24 * time.Hour, true
	case 'w':
		return time.Duration(num) * 7 * 24 * time.Hour, true
	default:
		return 0, false
	}
}

// ParseDate parses either a relative duration or an absolute date
func ParseDate(value string) (time.Time, bool) {
	// Try relative duration first
	if d, ok := ParseDuration(value); ok {
		return time.Now().Add(-d), true
	}
	// Try absolute date
	t, err := time.Parse("2006-01-02", value)
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}
