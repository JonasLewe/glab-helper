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
glab-helper [--dev] [--dry-run]
```

The `--dev` flag enables developer mode — advanced commands and skips the Jira target project check, allowing you to use Jira sync features against any repo.
The `--dry-run` flag exposes the explicit preview-only story sync action, intended for use together with `--dev`.

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

### Branch creation

When creating a branch, you can:

- Edit the auto-generated name (format: `<issue-nr>-<slugified-title>`)
- Pick any remote branch as base (sorted by most recent commit, default branch marked)
- Optionally check out the new branch immediately

## Jira Integration (optional)

Jira integration is deliberately one-way:

```text
Jira (read-only)  ──>  GitLab (read/write)
```

`glab-helper` only sends GET requests to Jira. It never creates, edits,
transitions, comments on, or deletes Jira issues. Use a dedicated Jira account
or token with read-only access. Normal syncs can create or update GitLab issues,
labels, and milestones, but always show a preview and ask for confirmation
first.

### Prerequisites

Before enabling the integration, verify that:

- the Jira instance is Jira Data Center with REST API v2 and Bearer-token
  authentication;
- the token can read every Jira issue selected by the configured labels;
- relevant issue types are named exactly `Story` and `Epic`;
- stories and epics share a unique Jira label for this GitLab target;
- the Story-to-Epic link is available as the string field
  `customfield_10000` if milestone mapping is required;
- `glab auth status` succeeds for the GitLab instance;
- the GitLab identity can read the configuration project's CI/CD variables and
  can create or update issues, labels, and milestones in the target project.

### Setup

1. From the cloned target repository, determine its exact GitLab path:

   ```bash
   glab repo view --output json | jq -r '.path_with_namespace'
   ```

   Use the complete result, including any subgroup, as
   `JIRA_TARGET_PROJECT`.

2. Choose a GitLab project to hold the Jira configuration. This can be the
   target project itself or a dedicated configuration project. No repository
   file is required. Tell `glab-helper` where the CI/CD variables live by using
   the URL-encoded project path:

   ```bash
   export GLAB_HELPER_JIRA_PROJECT_PATH='example-group%2Fconfig-project'
   ```

   Encode every `/` as `%2F`. For example,
   `example-group/service-api` becomes
   `example-group%2Fservice-api`.

   To add the setting to Zsh permanently without creating duplicate lines,
   replace the example path and run this once:

   ```bash
   setting="export GLAB_HELPER_JIRA_PROJECT_PATH='example-group%2Fconfig-project'"; rc_file="${ZDOTDIR:-$HOME}/.zshrc"; grep -Fqx "$setting" "$rc_file" 2>/dev/null || printf '\n%s\n' "$setting" >> "$rc_file"; source "$rc_file"
   ```

   If Bash is your login shell, use `~/.bashrc` instead of `~/.zshrc`.

3. In that project's **Settings > CI/CD > Variables**, create:

   | Variable | Example | Masked? | Purpose |
   |---|---|---:|---|
   | `JIRA_URL` | `https://jira.company.com` | No | Base URL without a trailing slash |
   | `JIRA_BOARD_LABELS` | `release-v1` | No | Jira label identifying the items to synchronize |
   | `JIRA_TOKEN` | Personal Access Token | **Yes** | Read-only Jira Data Center Bearer token |
   | `JIRA_TARGET_PROJECT` | `example-group/service-api` | No | Exact GitLab `path_with_namespace` allowed to sync |

   The Jira query uses `labels in (...)`: an issue matching any configured
   label is selected. Use labels unique to this integration because the current
   query does not also restrict a Jira project key.

4. Validate discovery from the target repository:

   ```bash
   glab-helper
   ```

   A successful setup prints `Jira integration available` and shows
   `Sync epics from Jira` and `Sync stories from Jira`. Press ESC to exit the
   menu without making changes.

Do not use `--dev` as a permanent setup shortcut. On the current Zsh `main`
branch it skips the target-project guard and exposes destructive development
commands. It is not required when `JIRA_TARGET_PROJECT` is configured correctly.

The variable project acts as a secret store, not as a CI runtime; no pipeline,
webhook, Jira application, or `.gitlab-ci.yml` is required. Users running the
tool must be allowed to retrieve these variables through the GitLab API.
Masking protects the token in logs but does not replace least-privilege access
and token rotation.

### What gets synced

| Jira | GitLab |
|------|--------|
| User Story | Issue (title prefixed with `[PROJ-123]`) |
| Epic | Milestone (with description) |
| Subtasks | Checkboxes in issue description |
| Labels | Labels (auto-created if missing) |
| Priority | Label (e.g. `prio::medium`) |

### Current limitations

- One configuration set supports exactly one `JIRA_TARGET_PROJECT` at a time.
  Changing it moves Jira availability from the old target to the new target.
- Multiple targets require separate configuration projects (or variables in
  each target project) and the matching `GLAB_HELPER_JIRA_PROJECT_PATH` value.
  Automatic multi-project profiles are not implemented in the Zsh version.
- One configuration set also represents one Jira URL, token, and label filter.
- Selection is label-based across every Jira project visible to the token; no
  `JIRA_PROJECT_KEY` constraint exists. Prefer a unique label and a
  least-privilege Jira account.
- Issue type names `Story` and `Epic`, plus the Epic Link field
  `customfield_10000`, are currently hard-coded. Other Jira schemes require a
  code change. Without that field, stories can lack milestone association.
- Labels requiring quoting or custom JQL expressions are not configurable.
- Jira Cloud and API v3 are not currently supported or tested.
- Jira always remains read-only. The sync has no Jira write-back or
  bidirectional conflict resolution.

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

## License

MIT
