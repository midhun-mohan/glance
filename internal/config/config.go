package config

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

var Version = "dev"

type Config struct {
	Orgs          OrgConfig          `yaml:"orgs"`
	Refresh       RefreshConfig      `yaml:"refresh"`
	Notifications NotificationConfig `yaml:"notifications"`
	Presets       map[string]string  `yaml:"presets"`
	UI            UIConfig           `yaml:"ui"`
}

type OrgConfig struct {
	AutoDetect bool     `yaml:"auto_detect"`
	Include    []string `yaml:"include"`
	Exclude    []string `yaml:"exclude"`
}

type RefreshConfig struct {
	Interval string `yaml:"interval"`
	OnFocus  bool   `yaml:"on_focus"`
}

func (r RefreshConfig) IntervalDuration() time.Duration {
	d, err := time.ParseDuration(r.Interval)
	if err != nil {
		return 5 * time.Minute
	}
	return d
}

type NotificationConfig struct {
	Enabled bool                `yaml:"enabled"`
	Events  NotificationEvents  `yaml:"events"`
}

type NotificationEvents struct {
	NewAssignment   bool `yaml:"new_assignment"`
	ReviewRequested bool `yaml:"review_requested"`
	StatusChange    bool `yaml:"status_change"`
	Mentions        bool `yaml:"mentions"`
}

type UIConfig struct {
	Theme       string `yaml:"theme"`
	Compact     bool   `yaml:"compact"`
	ShowAvatars bool   `yaml:"show_avatars"`
}

func DefaultConfig() Config {
	return Config{
		Orgs: OrgConfig{
			AutoDetect: true,
			Include:    []string{},
			Exclude:    []string{},
		},
		Refresh: RefreshConfig{
			Interval: "5m",
			OnFocus:  true,
		},
		Notifications: NotificationConfig{
			Enabled: true,
			Events: NotificationEvents{
				NewAssignment:   true,
				ReviewRequested: true,
				StatusChange:    true,
				Mentions:        true,
			},
		},
		Presets: map[string]string{},
		UI: UIConfig{
			Theme:       "auto",
			Compact:     false,
			ShowAvatars: false,
		},
	}
}

func ConfigDir() (string, error) {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "mygit"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "mygit"), nil
}

func ConfigPath() (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	primary := filepath.Join(dir, "config.yaml")
	if _, err := os.Stat(primary); err == nil {
		return primary, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return primary, nil
	}
	fallback := filepath.Join(home, ".mygit.yaml")
	if _, err := os.Stat(fallback); err == nil {
		return fallback, nil
	}
	return primary, nil
}

func Load() (Config, error) {
	cfg := DefaultConfig()
	path, err := ConfigPath()
	if err != nil {
		return cfg, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return cfg, fmt.Errorf("reading config: %w", err)
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("parsing config: %w", err)
	}
	return cfg, nil
}

func Save(cfg Config) error {
	dir, err := ConfigDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	path := filepath.Join(dir, "config.yaml")
	return os.WriteFile(path, data, 0o644)
}
