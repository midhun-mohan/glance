package github

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// LatestReleaseTag fetches the tag name of the latest published release of
// midhun-mohan/glance on GitHub. The request uses a short timeout and is
// intended for non-blocking startup checks; callers should treat any error as
// a silent failure (e.g. offline, rate-limited, no releases yet).
func LatestReleaseTag() (string, error) {
	req, err := http.NewRequest("GET", "https://api.github.com/repos/midhun-mohan/glance/releases/latest", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("github api: %s", resp.Status)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", err
	}

	var payload struct {
		TagName string `json:"tag_name"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return "", err
	}
	return payload.TagName, nil
}
