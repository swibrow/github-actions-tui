# GitHub Actions TUI

A terminal UI for monitoring GitHub Actions workflows, built with Go and [Bubble Tea](https://github.com/charmbracelet/bubbletea).

## Features

- Browse workflow runs with a sidebar for filtering by workflow
- Drill into jobs and view step-level details with dependency-based tier grouping
- View job logs directly in the terminal with search and timestamp toggle
- Filter runs by branch, actor, status, and event
- Switch between run attempts for re-run workflows
- Auto-refresh when active runs are detected (faster polling for in-progress jobs)
- Vim-style navigation (`j`/`k`, `h`/`l`, `gg`/`G`)
- Mouse support: click to drill in, right-click to go back, scroll content
- Text selection in log view (mouse capture disabled automatically)
- Open runs, jobs, PRs, or branches in the browser
- Switch repositories without restarting
- SQLite caching for workflows, jobs, logs, and YAML to reduce API calls

## Authentication

The TUI needs a GitHub token to access the API. It resolves credentials in this order:

1. **GitHub CLI** — reads the token from `gh auth login` (stored in `~/.config/gh/hosts.yml`)
2. **`GH_TOKEN`** — environment variable used by the GitHub CLI
3. **`GITHUB_TOKEN`** — environment variable commonly set in CI

The easiest way to get started:

```sh
gh auth login
```

## Prerequisites

- Go 1.25+
- A GitHub token (see [Authentication](#authentication))

## Install

### Homebrew

```sh
brew install swibrow/tap/gha
```

### Go

```sh
go install github.com/swibrow/github-actions-tui@latest
```

## Usage

Run from inside a Git repository with a GitHub remote:

```sh
gha
```

## Keybindings

### Navigation

| Key | Action |
|---|---|
| `j`/`k` or `↑`/`↓` | Move cursor up/down |
| `Tab` | Switch panes (sidebar / main) |
| `Enter` | Select / drill in |
| `Esc` or `q` | Go back / quit |
| `gg` / `G` | Jump to top / bottom |

### Tree Sidebar

| Key | Action |
|---|---|
| `l` or `→` | Expand workflow / view jobs |
| `h` or `←` | Collapse / go to parent |
| `Enter` | Expand/collapse or drill in |

### Actions

| Key | Action |
|---|---|
| `/` | Filter runs / search logs |
| `r` | Refresh data |
| `b` | Toggle sidebar |
| `o` | Open selected item in browser |
| `p` | Open PR or branch in browser |
| `O` | Open actions page in browser |
| `S` | Switch repository |
| `?` | Toggle help overlay |

### Jobs View

| Key | Action |
|---|---|
| `[` / `]` | Previous / next run attempt |

### Logs View

| Key | Action |
|---|---|
| `/` | Search logs |
| `n` / `N` | Next / previous search match |
| `t` | Toggle timestamps |

### Mouse

| Action | Effect |
|---|---|
| Left click | Select item and drill in |
| Right click | Go back (same as Esc) |
| Scroll | Scroll content |

Mouse capture is automatically disabled in the log content view so you can select and copy text normally.

## License

MIT
