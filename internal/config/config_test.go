package config

import (
	"testing"
	"time"
)

func TestIntervalDuration(t *testing.T) {
	tests := []struct {
		name     string
		interval string
		want     time.Duration
	}{
		{"5 minutes", "5m", 5 * time.Minute},
		{"30 seconds", "30s", 30 * time.Second},
		{"1 hour", "1h", time.Hour},
		{"2m30s", "2m30s", 2*time.Minute + 30*time.Second},
		{"empty defaults to 5m", "", 5 * time.Minute},
		{"invalid defaults to 5m", "notaduration", 5 * time.Minute},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := RefreshConfig{Interval: tt.interval}
			got := r.IntervalDuration()
			if got != tt.want {
				t.Errorf("IntervalDuration() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if !cfg.Orgs.AutoDetect {
		t.Error("AutoDetect should be true by default")
	}
	if cfg.Refresh.Interval != "5m" {
		t.Errorf("Refresh.Interval = %q, want 5m", cfg.Refresh.Interval)
	}
	if !cfg.Notifications.Enabled {
		t.Error("Notifications should be enabled by default")
	}
	if !cfg.Notifications.Events.NewAssignment {
		t.Error("NewAssignment notifications should be enabled by default")
	}
	if !cfg.Notifications.Events.ReviewRequested {
		t.Error("ReviewRequested notifications should be enabled by default")
	}
	if !cfg.Notifications.Events.StatusChange {
		t.Error("StatusChange notifications should be enabled by default")
	}
	if !cfg.Notifications.Events.Mentions {
		t.Error("Mentions notifications should be enabled by default")
	}
	if cfg.Presets == nil {
		t.Error("Presets should not be nil")
	}
	if cfg.UI.Theme != "auto" {
		t.Errorf("UI.Theme = %q, want auto", cfg.UI.Theme)
	}
}
