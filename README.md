# glab-helper

Interactive GitLab workflow helper for the terminal. Create issues, pick up existing ones, and manage branches — all from a single fzf-driven TUI.

## Features

- **Create new issues** — title, labels, assignee, milestone, description (opens your editor)
- **Work on existing issues** — browse open issues, see which already have branches
- **Branch management** — auto-generate branch names from issues, pick any base branch, checkout
- **Label & milestone creation** — create new labels and milestones inline
- **fzf everywhere** — fuzzy search for issues, labels, members, milestones, branches

## Install

**Requirements:** macOS (Homebrew) or Arch Linux (pacman)

```bash
git clone git@github.com:JonasLewe/glab-helper.git ~/.local/share/glab-helper
cd ~/.local/share/glab-helper
./install.sh
```

This installs dependencies (`glab`, `fzf 0.35+`, `jq`) and symlinks `glab-helper` to `~/.local/bin/`.

Make sure `~/.local/bin` is in your PATH:

```bash
export PATH="$HOME/.local/bin:$PATH"
```

### Authenticate glab

```bash
glab auth login
```

Choose your GitLab instance, authenticate with a personal access token (scopes: `api`, `read_repository`, `write_repository`), and verify with `glab auth status`.

## Usage

Run from any cloned GitLab repo:

```bash
glab-helper
```

You'll be presented with two options:

### Create new issue

Walk through an interactive flow: title, labels (multi-select), assignee, milestone, description (opens nvim/vim). After creation, optionally create a branch linked to the issue.

### Work on existing issue

Browse all open issues with fzf. Issues that already have a branch are marked. Select one to create a new branch or check out an existing one.

### Branch creation

When creating a branch, you can:

- Edit the auto-generated name (format: `<issue-nr>-<slugified-title>`)
- Pick any remote branch as base (sorted by most recent commit, default branch marked)
- Optionally check out the new branch immediately

## Dependencies

| Tool | Min version | Purpose |
|------|-------------|---------|
| [glab](https://gitlab.com/gitlab-org/cli) | — | GitLab CLI |
| [fzf](https://github.com/junegunn/fzf) | 0.35+ | Fuzzy finder |
| [jq](https://jqlang.github.io/jq/) | — | JSON processing |
| nvim or vim | — | Description editor (optional, falls back to `$VISUAL`/`$EDITOR`) |

## License

MIT
