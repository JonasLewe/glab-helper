# glab-helper

Interactive GitLab workflow helper for the terminal. Create issues, pick up existing ones, and manage branches — all from a single fzf-driven TUI.

## Features

- **Create issues from Jira** — sync User Stories from Jira Data Center to GitLab issues (one-way)
- **Create issues manually** — title, labels, assignee, milestone, description (opens your editor)
- **Bulk sync epics** — import all Jira Epics as GitLab Milestones in one go
- **Bulk sync stories** — import all unsynced Jira User Stories as GitLab Issues (labels, priority, subtasks, milestone — no manual interaction)
- **Work on existing issues** — browse open issues, see which already have branches
- **Branch management** — auto-generate branch names from issues, pick any base branch, checkout
- **Label & milestone creation** — create new labels and milestones inline, or auto-sync from Jira
- **Jira markup conversion** — bold, italic, headings, ordered/unordered lists (nested), strikethrough, links, code blocks, monospace, and macro stripping converted to Markdown
- **fzf everywhere** — fuzzy search for issues, labels, members, milestones, branches

## Install

**Requirements:** macOS (Homebrew) or Arch Linux (pacman)

```bash
git clone git@github.com:JonasLewe/glab-helper.git ~/.local/share/glab-helper
cd ~/.local/share/glab-helper
./install.sh
```

This installs runtime dependencies (`zsh`, `glab`, `fzf 0.35+`, `jq`, `curl`) and symlinks `glab-helper` to `~/.local/bin/`.

Make sure `~/.local/bin` is in your PATH:

```bash
export PATH="$HOME/.local/bin:$PATH"
```

### Authenticate glab

```bash
glab auth login
```

You'll be prompted for:

1. **GitLab instance URL** — e.g. `https://gitlab.company.com`
2. **Authentication method** — choose `Token`
3. **Access token** — create one in GitLab under Settings > Access Tokens with scopes:
   - `api` (full API access)
   - `read_repository`
   - `write_repository`
4. **Git protocol** — `HTTPS` (or `SSH` if you have keys configured)

Verify with:

```bash
glab auth status
```

This should show your GitLab instance and username. The token is stored locally by glab and used by glab-helper automatically.

## Usage

Run from any cloned GitLab repo:

```bash
glab-helper [--dev] [--dry-run] [--version]
```

The `--dev` flag skips the Jira target-project check. It does not enable
destructive maintenance commands.

The `--dry-run` flag makes the complete process read-only. Its menu contains
only Jira epic and story previews; it never performs GitLab mutations, creates
or checks out Git branches, or writes snapshots. It can be combined with
`--dev`, but does not require it.

`--help` and `--version` work offline. Unknown arguments are rejected instead
of opening the normal menu.

You'll be presented with the following options (Jira options only appear when integration is configured):

### Create issue

If Jira integration is configured, you can choose between:

- **From Jira** — select an unsynced Jira User Story. Title, description (converted to Markdown), labels, priority, milestone (from Epic), and subtasks (as checkboxes) are pre-filled. Review the description in your editor before creating.
- **Manual** — walk through an interactive flow: title, labels (multi-select), assignee, milestone, description (opens nvim/vim).

After creation, optionally create a branch linked to the issue.

### Work on existing issue

Browse all open issues with fzf. Issues that already have a branch are marked. After selecting an issue, an action menu lets you:

- **Branch** — check out an existing branch or create a new one
- **Edit description** — opens the current description in your editor
- **Edit labels** — re-select labels via fzf
- **Edit assignee** — pick a new assignee or unassign
- **Edit milestone** — change or remove the milestone
- **Close issue** — close the issue on GitLab

The menu loops after each action so you can make multiple changes in one session. Press ESC to exit.

### Sync epics from Jira

Bulk-imports all Jira Epics (filtered by board labels) as GitLab Milestones. Epic descriptions are converted to Markdown and added as milestone descriptions. Existing milestones are updated with the current Jira description (Jira is source of truth). Shows a dry-run preview with separate create/update counts before proceeding.

### Sync stories from Jira

Bulk-imports all unsynced Jira User Stories as GitLab Issues. Each issue gets the converted description, labels, priority label, subtasks as checkboxes, and milestone (from Epic). No assignee is set and no editor review — fully automatic. Missing milestones and labels are created on the fly, existing milestones are updated with epic descriptions. Shows a dry-run preview with confirmation.

## Sync safety

- GitLab and Jira list reads validate their JSON schema and abort on request,
  pagination, or schema errors. Partial result sets are never treated as empty
  successful responses.
- Jira pagination advances by the number of issues actually returned, so
  server-side page limits do not skip results.
- Story sync aborts if epic data is incomplete, preserving all existing
  milestone assignments.
- Labels and milestones are only created after the final confirmation. If a
  required dependency fails or cannot be verified, its issue is not created.
- POST requests are not blindly retried after an ambiguous failure. Safe reads
  and idempotent updates use bounded exponential backoff.
- Partial failures and failed snapshot exports return a non-zero exit status.
- Project reset is not available in the product TUI, including in `--dev`
  mode.

### Branch creation

When creating a branch, you can:

- Edit the auto-generated name (format: `<issue-nr>-<slugified-title>`)
- Pick any remote branch as base (sorted by most recent commit, default branch marked)
- Optionally check out the new branch immediately

## Jira Integration (optional)

Enables one-way sync of Jira User Stories to GitLab issues. Stories are filtered by board labels and only unsynced stories are shown.

### Setup

Set these as **Project Variables** in the glab-helper GitLab project (Settings > CI/CD > Variables):

| Variable | Example | Masked? |
|----------|---------|---------|
| `JIRA_URL` | `https://jira.company.com` | No |
| `JIRA_BOARD_LABELS` | `team-a,project-x` | No |
| `JIRA_TOKEN` | Personal Access Token | **Yes** |
| `JIRA_TARGET_PROJECT` | `group/project-name` | No |

- `JIRA_BOARD_LABELS`: comma-separated Jira labels used to filter stories
- `JIRA_TARGET_PROJECT`: the GitLab project path where issues should be created (Jira option only appears in this project)
- `JIRA_TOKEN`: a PAT with read access to Jira (Data Center: Bearer token)

Team members need access to the glab-helper project to read the variables.

### What gets synced

| Jira | GitLab |
|------|--------|
| User Story | Issue (title prefixed with `[PROJ-123]`) |
| Epic | Milestone (with description) |
| Subtasks | Checkboxes in issue description |
| Labels | Labels (auto-created if missing) |
| Priority | Label (e.g. `prio::medium`) |

## Dependencies

| Tool | Min version | Purpose |
|------|-------------|---------|
| zsh | 5+ | Runtime shell for `glab-helper` |
| [glab](https://gitlab.com/gitlab-org/cli) | — | GitLab CLI |
| [fzf](https://github.com/junegunn/fzf) | 0.35+ | Fuzzy finder |
| [jq](https://jqlang.github.io/jq/) | — | JSON processing |
| [curl](https://curl.se/) | — | Jira API requests |
| nvim or vim | — | Description editor (optional, falls back to `$VISUAL`/`$EDITOR`) |

## Development

Run the local smoke and syntax checks with:

```bash
./tests/run.sh
```

The suite includes API contract and failure-path checks for pagination,
read failures, dry-run writes, confirmation ordering, and partial-failure exit
codes. ShellCheck is run automatically when installed.

## License

MIT
