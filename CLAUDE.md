# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project

mygit is a terminal dashboard for monitoring GitHub pull requests across organizations. It uses Bubble Tea for the TUI, Cobra for CLI parsing, and GitHub's GraphQL API (authenticated via `gh` CLI) to fetch PRs in four sections: Created, Review Requested, Assigned, and Mentions.

## Build & Development Commands

```bash
make build          # Compile to bin/mygit (injects version via ldflags)
make run            # Build and execute
make install        # Install globally
make test           # Run all tests
make lint           # Run golangci-lint
make clean          # Remove build artifacts
```

The version string is injected into `internal/config.Version` at build time.

## Architecture

**Entry point**: `cmd/mygit/main.go` — Cobra CLI setup, validates `gh` auth, loads config, initializes Bubble Tea program.

**Internal packages** (all under `internal/`):

- **config** — XDG-compliant YAML config loading. Primary path: `~/.config/mygit/config.yaml`, fallback: `~/.mygit.yaml`. Handles org lists, refresh intervals, notification toggles, filter presets, and UI theme settings.

- **github** — GitHub API layer. Auth delegates to `gh auth token`. `client.go` makes GraphQL requests. `types.go` defines core domain types: `PullRequest`, `Section` (enum 0-3), `PRStatus`, `ReviewStatus`, and `PRsBySection` map.

- **tui** — Bubble Tea UI layer. `app.go` is the root Model with all state and the Update/View loop. `prlist.go` renders PRs grouped by repository with dynamic column widths. `sections.go` handles tab switching. `search.go` integrates fuzzy search (sahilm/fuzzy). `statusbar.go` shows refresh countdown. `styles.go` defines the color palette (purple primary, cyan secondary, green/amber/red for status). `help.go` defines the keybinding overlay.

- **filter** — Filter expression system. `parser.go` parses expressions like `repo:acme/* status:open label:urgent created:<7d`. `engine.go` applies filters with AND logic, supporting wildcards and relative date durations. `presets.go` manages named filter presets from config.

- **notify** — Desktop notification system. `notify.go` diffs PR sets between refreshes to detect new assignments/reviews/mentions. Platform backends: `macos.go` (osascript), `linux.go` (notify-send).

**Data flow**: CLI flags → config loaded → gh auth validated → token obtained → Bubble Tea starts → initial PR fetch via GraphQL search queries (one per section, scoped by org) → PRs rendered in grouped list → auto-refresh on timer → notifier diffs old/new PR sets for desktop alerts.

**Concurrency model**: PR fetching runs as Bubble Tea commands (goroutines). Auto-refresh and countdown are Bubble Tea tick commands. Notification dispatch is fire-and-forget.

## Key Conventions

- Errors are wrapped with context: `fmt.Errorf("context: %w", err)`
- No global mutable state; styles in `styles.go` are package-level but immutable
- Pagination accounts for repo group header overhead (3 lines per group) when calculating page size
- GitHub search queries are constructed per-section with org scoping (e.g., `author:{user} org:{org1} org:{org2}`)
