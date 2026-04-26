package github

import (
	"fmt"
	"os/exec"
	"strings"
)

func CheckAuth() error {
	cmd := exec.Command("gh", "auth", "status")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("gh auth check failed: %s\nPlease run: gh auth login", strings.TrimSpace(string(output)))
	}
	return nil
}

func GetToken() (string, error) {
	cmd := exec.Command("gh", "auth", "token")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get gh token: %w\nPlease run: gh auth login", err)
	}
	return strings.TrimSpace(string(output)), nil
}

func CheckGHInstalled() error {
	_, err := exec.LookPath("gh")
	if err != nil {
		return fmt.Errorf("gh CLI not found. Please install it: https://cli.github.com/")
	}
	return nil
}
