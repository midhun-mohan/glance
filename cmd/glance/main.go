package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"

	"github.com/midhun-mohan/glance/internal/config"
	"github.com/midhun-mohan/glance/internal/github"
	"github.com/midhun-mohan/glance/internal/tui"
)

var (
	flagSection         string
	flagOrg             string
	flagRepo            string
	flagFilter          string
	flagPreset          string
	flagNoNotifications bool
	flagRefresh         string
	flagConfig          bool
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "glance",
		Short: "Your GitHub pull requests, at a glance",
		Long:  "glance is an interactive terminal UI tool that gives developers a unified dashboard of their GitHub pull requests across all organizations they belong to.",
		RunE:  run,
	}

	rootCmd.Flags().StringVarP(&flagSection, "section", "s", "", "Start on a specific section (created|reviews|assigned|mentions)")
	rootCmd.Flags().StringVarP(&flagOrg, "org", "o", "", "Limit to specific org(s), comma-separated")
	rootCmd.Flags().StringVarP(&flagRepo, "repo", "r", "", "Limit to specific repo(s), comma-separated")
	rootCmd.Flags().StringVarP(&flagFilter, "filter", "f", "", "Apply a filter expression on startup")
	rootCmd.Flags().StringVarP(&flagPreset, "preset", "p", "", "Apply a saved filter preset on startup")
	rootCmd.Flags().BoolVar(&flagNoNotifications, "no-notifications", false, "Disable desktop notifications for this session")
	rootCmd.Flags().StringVar(&flagRefresh, "refresh", "", "Override auto-refresh interval")
	rootCmd.Flags().BoolVar(&flagConfig, "config", false, "Open config file in $EDITOR")

	rootCmd.Version = config.Version

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func run(cmd *cobra.Command, args []string) error {
	// Handle --config flag
	if flagConfig {
		return openConfigInEditor()
	}

	// Check gh CLI
	if err := github.CheckGHInstalled(); err != nil {
		return err
	}
	if err := github.CheckAuth(); err != nil {
		return err
	}

	// Get token
	token, err := github.GetToken()
	if err != nil {
		return err
	}

	// Load config
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: %v, using defaults\n", err)
		cfg = config.DefaultConfig()
	}

	// Apply CLI overrides
	if flagOrg != "" {
		cfg.Orgs.AutoDetect = false
		cfg.Orgs.Include = strings.Split(flagOrg, ",")
	}
	if flagRepo != "" {
		repos := strings.Split(flagRepo, ",")
		parts := make([]string, len(repos))
		for i, r := range repos {
			parts[i] = "repo:" + strings.TrimSpace(r)
		}
		if flagFilter != "" {
			flagFilter += " " + strings.Join(parts, " ")
		} else {
			flagFilter = strings.Join(parts, " ")
		}
	}
	if flagNoNotifications {
		cfg.Notifications.Enabled = false
	}
	if flagRefresh != "" {
		cfg.Refresh.Interval = flagRefresh
	}

	// Resolve preset
	if flagPreset != "" {
		if preset, ok := cfg.Presets[flagPreset]; ok {
			if flagFilter != "" {
				flagFilter += " " + preset
			} else {
				flagFilter = preset
			}
		} else {
			return fmt.Errorf("unknown preset: %s", flagPreset)
		}
	}

	// Determine starting section
	startSection := github.SectionCreated
	if flagSection != "" {
		switch strings.ToLower(flagSection) {
		case "created":
			startSection = github.SectionCreated
		case "reviews", "review":
			startSection = github.SectionReviewRequested
		case "assigned":
			startSection = github.SectionAssigned
		case "mentions":
			startSection = github.SectionMentions
		default:
			return fmt.Errorf("unknown section: %s (use: created, reviews, assigned, mentions)", flagSection)
		}
	}

	// Create client and model
	client := github.NewClient(token)
	model := tui.NewModel(cfg, client, startSection, flagFilter)

	// Run TUI
	p := tea.NewProgram(model, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		return fmt.Errorf("running TUI: %w", err)
	}

	return nil
}

func openConfigInEditor() error {
	path, err := config.ConfigPath()
	if err != nil {
		return err
	}

	// Ensure config exists
	if _, err := os.Stat(path); os.IsNotExist(err) {
		cfg := config.DefaultConfig()
		if err := config.Save(cfg); err != nil {
			return fmt.Errorf("creating default config: %w", err)
		}
	}

	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "vi"
	}

	editorCmd := exec.Command(editor, path)
	editorCmd.Stdin = os.Stdin
	editorCmd.Stdout = os.Stdout
	editorCmd.Stderr = os.Stderr
	return editorCmd.Run()
}
