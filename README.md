# glab-helper

> [!WARNING]
> **The `v2` branch is work in progress.** Use `main` for the stable Zsh
> version; the Go migration is intentionally incomplete and not ready for daily
> use.

Minimal terminal workflow for syncing Jira work into GitLab, picking up issues,
and managing their branches from a focused fzf-driven TUI.

## Features

- **Sync Jira** — reconcile Jira Epics and User Stories into GitLab milestones and issues (one-way)
- **Work on existing issues** — browse open issues, see which already have branches
- **Branch management** — prune deleted remote branches, generate issue branch names, select a current base, and check out
- **Forward-only status sync** — map Jira progress and review states to labels and close GitLab issues when Jira reaches Done
- **Label & milestone creation** — create new labels and milestones inline, or auto-sync from Jira
- **Jira markup conversion** — bold, italic, headings, ordered/unordered lists (nested), strikethrough, links, code blocks, monospace, and macro stripping converted to Markdown
- **Developer tools** — granular Epic/Story sync, manual issue creation, and snapshot export behind `--dev`

## Install

**Requirements:** macOS (Homebrew) or Arch Linux (pacman)

```bash
git clone git@github.com:JonasLewe/glab-helper.git ~/.local/share/glab-helper
cd ~/.local/share/glab-helper
./install.sh
```

This installs runtime dependencies (`zsh`, `glab`, `fzf 0.35+`, `jq`, `curl`) and symlinks `glab-helper` to `~/.local/bin/`.

### Go migration

The production command remains the Zsh implementation. The first migration
step provides only the existing offline help and version commands:

```bash
go run ./cmd/glab-helper --help
go run ./cmd/glab-helper --version
```

Without an offline flag, the development binary validates the current GitLab
project, reads and validates every GitLab issue, milestone, and label without
mutations, reads the locally available `origin` remote-tracking branches
without fetching or pruning, and then stops. Interactive issue, branch, Jira,
and mutation workflows still use the Zsh implementation.

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

## Use an existing Jira setup

Jira sync is the primary workflow of `glab-helper`. Jira is technically not
required to work on an existing GitLab issue or branch, but without it the
normal menu has no sync action and maintenance mode is unavailable.

If your team already configured Jira, **do not create the CI/CD variables again
and do not copy the Jira token to your machine**. Ask the project maintainer for:

- the target GitLab repository;
- the GitLab project that stores the four Jira CI/CD variables;
- access to read those variables and to work with issues in the target project.

If the variables are stored in the target project, no local Jira setting is
needed. The helper uses the current GitLab project by default:

```bash
cd /path/to/target-project
glab auth status
glab-helper
```

If the variables are stored in a separate configuration project, derive its
exact URL-encoded path from a clone of that project:

```bash
cd /path/to/config-project
export GLAB_HELPER_JIRA_PROJECT_PATH="$(glab repo view --output json | jq -r '.path_with_namespace | @uri')"
printf '%s\n' "$GLAB_HELPER_JIRA_PROJECT_PATH"
```

To keep that value across Zsh sessions, run this once before leaving the
configuration project:

```bash
setting="export GLAB_HELPER_JIRA_PROJECT_PATH='$(glab repo view --output json | jq -r '.path_with_namespace | @uri')'"; rc_file="${ZDOTDIR:-$HOME}/.zshrc"; grep -Fqx "$setting" "$rc_file" 2>/dev/null || printf '\n%s\n' "$setting" >> "$rc_file"; source "$rc_file"
```

Then run the helper from the target project:

```bash
cd /path/to/target-project
glab-helper
```

The local setting identifies only where the variables live. Their values,
including `JIRA_TOKEN`, are retrieved through `glab` and are not added to your
shell configuration.

A working setup prints `Jira integration available` and shows `Sync Jira` as
the first action. For Bash, put the same resolved export in `~/.bashrc`. If Jira
is not detected, verify the encoded project path and ask a maintainer whether
your GitLab account may retrieve the project's CI/CD variable values through
the API. Do not use `--dev` to bypass this setup.

## Usage

Run from any cloned GitLab repo:

```bash
glab-helper [--dev | --maintenance] [--dry-run] [--version]
```

The `--dev` flag exposes advanced developer actions and skips the Jira
target-project check. It does not expose reset or other bulk-delete commands.

The separate `--maintenance` flag exposes destructive project cleanup. It
cannot be combined with `--dev` and never skips the configured target-project
check.

The `--dry-run` flag makes the complete process read-only. The normal menu
contains one combined `Preview Jira` action; `--dev --dry-run` exposes separate
Epic and Story previews. Neither mode performs GitLab mutations, creates or
checks out Git branches, or writes snapshots. With `--maintenance`, it previews
the complete reset plan without deleting or writing a snapshot.

`--help` and `--version` work offline. Unknown arguments are rejected instead
of opening the normal menu.

With Jira configured, the normal menu deliberately contains only:

```text
Sync Jira
Work on existing issue
Exit
```

`Sync Jira` is first and therefore selected by default. `Exit` is always the
last action, including in developer and dry-run menus.

### Sync Jira

This is the everyday one-way reconciliation from Jira into GitLab. It:

- creates and updates GitLab milestones from Jira Epics;
- creates missing GitLab issues from Jira User Stories;
- updates synced titles, descriptions, Jira labels, priorities, milestones, and
  forward-only workflow states;
- closes a GitLab issue when its Jira Story reaches the Done category.

It shows the complete plan and asks for confirmation before applying changes.
Use `glab-helper --dry-run` for the same combined plan without writes.

### Work on existing issue

Browse all open issues with fzf. Issues that already have a branch are marked. After selecting an issue, an action menu lets you:

- **Branch** — check out an existing branch or create a new one
- **Edit description** — opens the current description in your editor
- **Edit labels** — re-select labels via fzf
- **Edit assignee** — pick a new assignee or unassign
- **Edit milestone** — change or remove the milestone
- **Close issue** — close the issue on GitLab

The menu loops after each action so you can make multiple changes in one session. Press ESC to exit.

### Developer menu

Run `glab-helper --dev` only when a granular or administrative workflow is
needed. It contains:

- **Sync epics from Jira** — reconcile only Jira Epics and GitLab milestones;
- **Sync stories from Jira** — run the full Story sync explicitly;
- **Create issue** — create a GitLab issue from Jira with review or create one
  manually;
- **Work on existing issue** — the same daily issue workflow;
- **Export GitLab snapshot** — manually export issues, labels, and milestones.

The normal `Sync Jira` action may still offer an optional pre-sync snapshot.
Only the standalone export action is hidden from the normal menu.

### Maintenance reset

Use maintenance mode to remove all GitLab issues and milestones before a clean
resync. Always inspect the read-only plan first:

```bash
glab-helper --maintenance --dry-run
```

If the plan is correct, run:

```bash
glab-helper --maintenance
```

The reset permanently deletes:

- every open and closed GitLab issue in the configured target project;
- every active and closed project milestone.

It preserves branches, labels, and merge requests. Before the first deletion it
requires typing `RESET ALL <full-project-path>`, creates a complete local
snapshot, and revalidates that the displayed plan has not changed. If an issue
deletion fails, milestone deletion is skipped. Issue deletion is permanent and
includes its discussions; the snapshot is an audit backup, not an automatic
restore mechanism. The GitLab identity must have permission to delete both
project issues and milestones.

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
- Unscoped project reset is not available in the normal or `--dev` TUI. The
  separate maintenance reset requires a matching configured target project,
  exact full-reset confirmation, mandatory snapshot, and plan revalidation.

### Branch creation

When creating a branch, you can:

- Edit the auto-generated name (format: `<issue-nr>-<slugified-title>`)
- Pick any current remote branch as base (deleted remote branches are pruned
  first; remaining branches are sorted by most recent commit and the default
  is marked)
- Optionally check out the new branch immediately

## Jira administration

Most users do not need this section. It is for the maintainer who connects a
GitLab target project to Jira for the first time.

Jira integration is deliberately one-way:

```text
Jira (read-only)  ──>  GitLab (read/write)
```

`glab-helper` only sends GET requests to Jira. It never creates, edits,
transitions, comments on, or deletes Jira issues. Use a dedicated Jira account
or token with read-only access. Normal syncs can create or update GitLab issues,
labels, and milestones, but always show a preview and ask for confirmation
first.

### Prerequisites for first-time setup

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

### First-time setup

1. From the cloned target repository, determine its exact GitLab path:

   ```bash
   glab repo view --output json | jq -r '.path_with_namespace'
   ```

   Use the complete result, including any subgroup, as
   `JIRA_TARGET_PROJECT`.

2. Choose a GitLab project to hold the Jira configuration. This can be the
   target project itself or a dedicated configuration project. No repository
   file is required. Record its URL-encoded path for every user:

   ```bash
   export GLAB_HELPER_JIRA_PROJECT_PATH='example-group%2Fconfig-project'
   ```

   Encode every `/` as `%2F`. For example,
   `example-group/service-api` becomes
   `example-group%2Fservice-api`.

   The helper uses the current target project by default. It cannot discover a
   different configuration project automatically. In that case, give intended
   users sufficient access to retrieve its CI/CD variable values, then share
   the encoded path with them. The preceding user section explains their only
   required local setting.

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

   A successful setup prints `Jira integration available` and shows `Sync Jira`
   as the first menu action. Press ESC to exit without making changes.

Do not use `--dev` as a permanent setup shortcut. On the current Zsh `main`
branch it skips the target-project guard and exposes advanced GitLab write
actions; it does not configure Jira. It is not required when
`JIRA_TARGET_PROJECT` is configured correctly.

The variable project acts as a secret store, not as a CI runtime; no pipeline,
webhook, Jira application, or `.gitlab-ci.yml` is required. Users running the
tool must be allowed to retrieve these variables through the GitLab API.
GitLab generally reserves project CI/CD variable management for Maintainers;
instance permissions and custom roles can further restrict API access. Masking
protects the token in logs but does not replace least-privilege access and token
rotation.

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

The suite includes API contract and failure-path checks for pagination,
read failures, dry-run writes, confirmation ordering, and partial-failure exit
codes. ShellCheck is run automatically when installed.

## License

MIT
