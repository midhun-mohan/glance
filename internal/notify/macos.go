package notify

import (
	"os"
	"os/exec"
)

type macOSBackend struct{}

func (b *macOSBackend) Send(title, message string) error {
	// Plain osascript. We deliberately do NOT wrap this in `tell application "X" to ...`
	// AppleScript's `display notification` is always handled by the scripting
	// host regardless of the surrounding tell block, so the wrapper does not
	// route the notification through the target app and can make macOS surface
	// "Script Editor" when the user clicks the banner.
	script := `display notification "` + escapeOsascript(message) + `" with title "` + escapeOsascript(title) + `"`
	return exec.Command("osascript", "-e", script).Run()
}

// terminalAppName returns the macOS app name of the running terminal. Kept for
// potential future use; not currently invoked because the `tell application`
// wrapper does not actually route `display notification` through the target.
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
		c := s[i]
		switch {
		case c == '"':
			result = append(result, '\\', '"')
		case c == '\\':
			result = append(result, '\\', '\\')
		case c < 0x20:
			result = append(result, ' ')
		default:
			result = append(result, c)
		}
	}
	return string(result)
}
