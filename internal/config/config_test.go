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

func TestCheckConfigSymlink(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	regular := filepath.Join(tmpHome, "regular.yaml")
	if err := os.WriteFile(regular, []byte("a: 1\n"), 0o600); err != nil {
		t.Fatalf("writing regular file: %v", err)
	}

	insideTarget := filepath.Join(tmpHome, "dotfiles", "config.yaml")
	if err := os.MkdirAll(filepath.Dir(insideTarget), 0o700); err != nil {
		t.Fatalf("mkdir inside: %v", err)
	}
	if err := os.WriteFile(insideTarget, []byte("a: 1\n"), 0o600); err != nil {
		t.Fatalf("writing inside target: %v", err)
	}
	insideLink := filepath.Join(tmpHome, "inside.yaml")
	if err := os.Symlink(insideTarget, insideLink); err != nil {
		t.Fatalf("symlink inside: %v", err)
	}

	outsideDir := t.TempDir()
	outsideTarget := filepath.Join(outsideDir, "config.yaml")
	if err := os.WriteFile(outsideTarget, []byte("a: 1\n"), 0o600); err != nil {
		t.Fatalf("writing outside target: %v", err)
	}
	outsideLink := filepath.Join(tmpHome, "outside.yaml")
	if err := os.Symlink(outsideTarget, outsideLink); err != nil {
		t.Fatalf("symlink outside: %v", err)
	}

	tests := []struct {
		name    string
		path    string
		wantErr bool
	}{
		{"regular file", regular, false},
		{"missing file", filepath.Join(tmpHome, "nope.yaml"), false},
		{"symlink inside $HOME", insideLink, false},
		{"symlink escaping $HOME", outsideLink, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := checkConfigSymlink(tt.path)
			if (err != nil) != tt.wantErr {
				t.Errorf("checkConfigSymlink(%q) err = %v, wantErr = %v", tt.path, err, tt.wantErr)
			}
		})
	}
}
