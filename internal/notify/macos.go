package notify

import (
	"os/exec"
)

type macOSBackend struct{}

func (b *macOSBackend) Send(title, message string) error {
	script := `display notification "` + escapeOsascript(message) + `" with title "` + escapeOsascript(title) + `"`
	return exec.Command("osascript", "-e", script).Run()
}

func escapeOsascript(s string) string {
	result := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '"':
			result = append(result, '\\', '"')
		case '\\':
			result = append(result, '\\', '\\')
		default:
			result = append(result, s[i])
		}
	}
	return string(result)
}
