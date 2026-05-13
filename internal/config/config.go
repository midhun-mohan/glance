package config

import (
	"bytes"
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

//go:embed default_config.yaml
var defaultConfigYAML []byte

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
	IncludeTeam     bool `yaml:"include_team"` // notify for team-based assignments/reviews (default: false)
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
		return filepath.Join(xdg, "glance"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "glance"), nil
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
	fallback := filepath.Join(home, ".glance.yaml")
	if _, err := os.Stat(fallback); err == nil {
		return fallback, nil
	}
	return primary, nil
}

// checkConfigSymlink refuses to load the config if `path` is a symlink whose
// resolved target escapes the user's home directory. Same-filesystem symlinks
// inside $HOME (e.g. a dotfiles repo at ~/dotfiles/glance/config.yaml) are
// allowed. Returns nil if the path is a regular file or does not exist.
func checkConfigSymlink(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return nil
	}
	if info.Mode()&os.ModeSymlink == 0 {
		return nil
	}
	target, err := filepath.EvalSymlinks(path)
	if err != nil {
		return fmt.Errorf("resolving config symlink: %w", err)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	resolvedHome, err := filepath.EvalSymlinks(home)
	if err != nil {
		resolvedHome = home
	}
	rel, err := filepath.Rel(resolvedHome, target)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return fmt.Errorf("refusing to read config: symlink target %q is outside $HOME", target)
	}
	return nil
}

// writeDefaultConfig writes the embedded default config template to the
// primary config path. Returns the path written, or empty string on failure.
func writeDefaultConfig() string {
	dir, err := ConfigDir()
	if err != nil {
		return ""
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return ""
	}
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, defaultConfigYAML, 0o600); err != nil {
		return ""
	}
	return path
}

// Load reads the config file, or returns defaults if none exists.
// The second return value is the path of a newly created default config file
// (empty string if no file was created).
func Load() (Config, string, error) {
	cfg := DefaultConfig()
	path, err := ConfigPath()
	if err != nil {
		return cfg, "", nil
	}
	if err := checkConfigSymlink(path); err != nil {
		return cfg, "", err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			created := writeDefaultConfig()
			return cfg, created, nil
		}
		return cfg, "", fmt.Errorf("reading config: %w", err)
	}
	data = bytes.ReplaceAll(data, []byte("\t"), []byte("  "))
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return cfg, "", fmt.Errorf("parsing config: %w", err)
	}
	return cfg, "", nil
}

func Save(cfg Config) error {
	dir, err := ConfigDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	path := filepath.Join(dir, "config.yaml")
	return os.WriteFile(path, data, 0o600)
}
