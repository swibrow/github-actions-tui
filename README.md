# GitHub Actions TUI

A terminal UI for monitoring GitHub Actions workflows, built with Go and [Bubble Tea](https://github.com/charmbracelet/bubbletea).

## Features

- Browse workflow runs with a sidebar for filtering by workflow
- Drill into jobs and view step-level details
- View job logs directly in the terminal
- Filter runs by branch, actor, status, and event
- Auto-refresh when active runs are detected
- Vim-style navigation (`j`/`k`, `h`/`l`, `gg`/`G`)
- Mouse support (click to focus panes, scroll content)

## Prerequisites

- Go 1.25+
- GitHub CLI (`gh`) authenticated, or `GH_TOKEN`/`GITHUB_TOKEN` set

## Install

```sh
go install github.com/swibrow/github-actions-tui@latest
```

## Usage

Run from inside a Git repository with a GitHub remote:

```sh
github-actions-tui
```

## Keybindings

| Key | Action |
|---|---|
| `j`/`k` or `↑`/`↓` | Move cursor |
| `h`/`l` or `←`/`→` | Switch panes |
| `Enter` | Select / drill in |
| `Esc` | Go back |
| `gg` / `G` | Top / bottom |
| `/` | Open filter bar |
| `r` | Refresh |
| `?` | Toggle help |
| `q` | Quit |

## License

MIT
