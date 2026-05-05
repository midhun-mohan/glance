package config

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
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

func TestDefaultConfigYAMLMatchesDefaultConfig(t *testing.T) {
	var fromYAML Config
	if err := yaml.Unmarshal(defaultConfigYAML, &fromYAML); err != nil {
		t.Fatalf("embedded default_config.yaml is invalid YAML: %v", err)
	}
	expected := DefaultConfig()
	if !reflect.DeepEqual(fromYAML, expected) {
		t.Errorf("embedded default_config.yaml does not match DefaultConfig()\ngot:  %+v\nwant: %+v", fromYAML, expected)
	}
}

func TestLoadCreatesDefaultConfig(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	cfg, created, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	expectedPath := filepath.Join(tmpDir, "glance", "config.yaml")
	if created != expectedPath {
		t.Errorf("Load() created = %q, want %q", created, expectedPath)
	}

	// Verify file was written
	data, err := os.ReadFile(expectedPath)
	if err != nil {
		t.Fatalf("reading created config: %v", err)
	}
	if string(data) != string(defaultConfigYAML) {
		t.Error("created config file does not match embedded template")
	}

	// Verify returned config matches defaults
	expected := DefaultConfig()
	if !reflect.DeepEqual(cfg, expected) {
		t.Errorf("Load() config does not match DefaultConfig()\ngot:  %+v\nwant: %+v", cfg, expected)
	}

	// Second call should not report creation
	_, created2, err := Load()
	if err != nil {
		t.Fatalf("second Load() error: %v", err)
	}
	if created2 != "" {
		t.Errorf("second Load() created = %q, want empty string", created2)
	}
}
