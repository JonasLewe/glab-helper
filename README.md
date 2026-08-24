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

The production command remains the Jira-based Zsh implementation. Go v2 is
also replacing the team's retiring Jira source with YouTrack and is not yet a
drop-in replacement. Offline help and version commands are available:

```bash
go run ./cmd/glab-helper --help
go run ./cmd/glab-helper --version
go run ./cmd/glab-helper --dry-run
go run ./cmd/glab-helper
```

The Go sync path validates the current GitLab project and reads every GitLab
issue, task, milestone, label, and issue board before planning any write.
Issues are filtered explicitly to GitLab's `issue` type; tasks and their parent
issues are read separately through the paginated work-item GraphQL API. The
binary reads every issue selected by the configured YouTrack query through paginated `GET`
requests into an in-memory, provider-neutral snapshot. YouTrack is never
written. Missing YouTrack access remains optional except in maintenance and
synchronization-preview modes. The normal Go menu can synchronize YouTrack,
select an existing GitLab issue or task, manage its branch, edit an issue, or
exit. The remaining developer and maintenance actions still use the
Jira-based Zsh implementation.

`--dry-run` now builds and prints the combined YouTrack synchronization plan.
The preview includes labels, milestones, issues, tasks, their configured
relationships, field updates, forward-only closing, unchanged items, and
ignored source levels. It returns before reading or changing branches and does
not send any GitLab or YouTrack write request.

Without `--dry-run`, the same plan is printed and must be confirmed explicitly
with `y` or `yes`; an empty response and every other answer cancel without a
write. A confirmed plan creates or updates GitLab labels, milestones, issues,
and tasks in dependency order. Tasks are created through the work-item API
with their issue parent already assigned. Updates never reopen closed targets.
If one action fails, execution stops immediately, reports how many preceding
actions completed, and asks for a fresh `--dry-run` before retrying. The plan is
idempotent, so successfully completed actions are recognized on that retry.

The `Work on existing issue or task` action first runs
`git fetch --prune origin`, then lists all open GitLab issues and tasks in one
`fzf` selector.
Tasks show their parent issue, and an item is annotated when a current local or
remote branch starts with its IID followed by `-`. The branch action can check
out that branch or create a local branch from any current local or `origin`
branch. New names default to `<iid>-<title-slug>` and are validated by Git;
the repository's default branch is placed first in the base selector. Checkout
remains optional. This workflow changes only local Git refs and the working
tree; it never writes GitLab or YouTrack and does not push a created branch.
`--dry-run` returns before fetching or reading branches.

For a selected GitLab issue, the Go action menu can also edit the description,
toggle project labels, assign one project member or unassign everyone, set or
remove a milestone, and close the issue after an explicit confirmation. The
menu returns after each edit so several fields can be changed in one session.
Descriptions open in `VISUAL`, then `EDITOR`, with `nvim`, `vim`, or `vi` as
fallbacks. A selected GitLab task exposes only branch management until the
task-editing API is migrated separately.

The Go `--dev` menu additionally provides:

- a milestone-only YouTrack sync and the complete work-item sync;
- creation of one selected, not-yet-synchronized YouTrack issue with only its
  required labels, board list, and parent milestone;
- manual GitLab issue creation with existing or newly planned labels, an
  optional assignee, and an existing or newly planned milestone;
- a local export of all project issues, milestones, and labels.

`--dev --dry-run` exposes the granular milestone-only and complete previews but
no write actions. The milestone-only mode still reads and validates the full
YouTrack hierarchy before projecting non-milestone targets to `ignore` for that
single plan. Creating from YouTrack also uses the normal synchronization planner
and confirmation instead of a separate write path. Manual creation delays new
labels and milestones until the final confirmation and offers the regular
branch workflow after the issue exists.

Snapshot exports are written as private JSON files below
`.glab-helper-snapshots/` in the target repository. The directory contains
`issues.json`, `milestones.json`, `labels.json`, and `metadata.json` and is the
same backup format used by the upcoming maintenance migration.

The Go version reads these CI/CD variables from the current GitLab project, or
from the URL-encoded project selected by
`GLAB_HELPER_YOUTRACK_PROJECT_PATH`:

| Variable | Purpose |
|---|---|
| `YOUTRACK_URL` | Base URL of the YouTrack instance |
| `YOUTRACK_TOKEN` | Masked (not hidden), read-only permanent token |
| `YOUTRACK_TARGET_PROJECT` | Exact target GitLab `path_with_namespace` |

Non-secret, project-specific behavior lives in a local `.glab-helper.json` in
the target repository. Start with the versioned example:

```bash
cp ~/.local/share/glab-helper/.glab-helper.json.example .glab-helper.json
```

The glab-helper repository ignores its own local configuration and snapshots.
In another target repository, add these entries to that repository's
`.gitignore` as well:

```gitignore
/.glab-helper.json
/.glab-helper-snapshots/
```

The example intentionally uses `YOUR_VERSION_TAG`; replace it with the
YouTrack release tag for the target, for example `mind-v1`.

Set `GLAB_HELPER_CONFIG` to use a different path. An explicitly selected path
must exist; only the absent default `.glab-helper.json` is treated as an
optional integration. The configuration defines the
YouTrack query, the names of the type/status/priority fields, accepted type
aliases, the ordered hierarchy, and its GitLab targets. The checked-in example
maps the current two-level hierarchy as follows:

```text
YouTrack Epic      -> GitLab milestone
  YouTrack Story   -> GitLab issue
```

This target model works with GitLab Community Edition 19.2.1. Every configured
non-root YouTrack item must reference an item from the preceding hierarchy
level that is also selected by the query. The complete read fails on unknown
types, missing parents, or invalid target mappings.

Set a role's GitLab target to `ignore` to validate its YouTrack items and
relationships without synchronizing that level. Target validation removes all
ignored roles before checking the remaining Community Edition hierarchy. For
example, this keeps the complete Epic -> Feature -> Story source tree while
syncing only features as milestones and stories as issues:

```json
"gitlab": {
  "targets": {
    "epic": "ignore",
    "feature": "milestone",
    "story": "issue"
  }
}
```

At least one role must be synchronized. Non-ignored targets must form one of
the supported sequences: `milestone`, `issue`, `milestone -> issue`,
`issue -> task`, or `milestone -> issue -> task`.

The resulting snapshot contains the readable issue ID, title, description,
resolved state, tags, parent ID, raw YouTrack type, and normalized hierarchy
role. It is never written to disk. The preview copies YouTrack tags to GitLab
labels and derives the scoped labels `prio::<YouTrack value>` and
`status::<YouTrack value>`. Existing non-priority and non-status labels are
preserved; a resolved source item may close its GitLab target, but an open
source item never reopens a closed target.

During synchronization, the Go helper also reads the project's issue boards.
When an open synchronized issue or task uses an intermediate
`status::<value>` label, the helper creates a missing label-based list on the
`Development` board. If no board has that name, it uses the project board with
the lowest ID. Existing lists are never removed or reordered. `status::Open`
continues to use GitLab's built-in Open list, while terminal labels such as
`status::Done` or `status::Closed` use the built-in Closed list. Board-list
creation is included in `--dry-run` and requires the same explicit
confirmation as every other GitLab write.

Synced descriptions receive a stable
`<!-- glab-helper:youtrack:PROJECT-123 -->` identity marker. The first preview
can also adopt existing milestones by title, legacy `<!-- jira:... -->`
markers, and issues or tasks whose title starts with `[PROJECT-123]`. Ambiguous
identities, duplicate markers, and an existing legacy identity whose GitLab
type conflicts with the configured target stop the preview instead of creating
a duplicate. A missing default project configuration or missing YouTrack
access remains optional outside maintenance and preview modes; a present but
invalid configuration is always rejected.

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
