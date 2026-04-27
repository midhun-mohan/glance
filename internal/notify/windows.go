package notify

import (
	"fmt"
	"os/exec"
	"strings"
)

type windowsBackend struct{}

func (b *windowsBackend) Send(title, message string) error {
	// Use PowerShell to show a Windows toast notification (built-in on Windows 10+)
	script := fmt.Sprintf(
		`[Windows.UI.Notifications.ToastNotificationManager, Windows.UI.Notifications, ContentType = WindowsRuntime] > $null; `+
			`$template = [Windows.UI.Notifications.ToastNotificationManager]::GetTemplateContent([Windows.UI.Notifications.ToastTemplateType]::ToastText02); `+
			`$textNodes = $template.GetElementsByTagName('text'); `+
			`$textNodes.Item(0).AppendChild($template.CreateTextNode('%s')) > $null; `+
			`$textNodes.Item(1).AppendChild($template.CreateTextNode('%s')) > $null; `+
			`$toast = [Windows.UI.Notifications.ToastNotification]::new($template); `+
			`[Windows.UI.Notifications.ToastNotificationManager]::CreateToastNotifier('glance').Show($toast)`,
		escapePowerShell(title),
		escapePowerShell(message),
	)
	return exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", script).Run()
}

func escapePowerShell(s string) string {
	s = strings.ReplaceAll(s, "'", "''")
	s = strings.ReplaceAll(s, "`", "``")
	s = strings.ReplaceAll(s, "$", "`$")
	return s
}
