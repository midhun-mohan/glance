package notify

import (
	"os"
	"os/exec"
)

type macOSBackend struct{}

func (b *macOSBackend) Send(title, message string) error {
	app := terminalAppName()
	script := `tell application "` + escapeOsascript(app) + `" to display notification "` + escapeOsascript(message) + `" with title "` + escapeOsascript(title) + `"`
	return exec.Command("osascript", "-e", script).Run()
}

// terminalAppName returns the macOS app name of the running terminal so that
// clicking a notification brings the terminal to the front instead of opening
// Script Editor.
func terminalAppName() string {
	switch os.Getenv("TERM_PROGRAM") {
	case "ghostty":
		return "Ghostty"
	case "iTerm.app":
		return "iTerm"
	case "WezTerm":
		return "WezTerm"
	case "Hyper":
		return "Hyper"
	default:
		return "Terminal"
	}
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
