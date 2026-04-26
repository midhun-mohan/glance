package notify

import (
	"os/exec"
)

type linuxBackend struct{}

func (b *linuxBackend) Send(title, message string) error {
	return exec.Command("notify-send", title, message).Run()
}
