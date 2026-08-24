# glab-helper

`glab-helper` synchronizes a read-only YouTrack selection into GitLab and
provides focused GitLab issue and branch workflows in the terminal.

The normal workflow is intentionally small: synchronize YouTrack or exit.
Advanced issue, branch, export, and maintenance actions use explicit modes.

## What it does

- maps YouTrack hierarchy levels to GitLab milestones, issues, or tasks;
- creates and updates labels, milestones, issues, tasks, and issue-board lists;
- preserves unrelated GitLab labels and never reopens closed work items;
- closes GitLab work items when their YouTrack source is resolved;
- previews every synchronization and requires confirmation before writing;
- keeps YouTrack strictly read-only;
- supports issue editing and branch creation in developer mode;
- protects maintenance reset with an exact confirmation, a mandatory snapshot,
  and plan revalidation.

## Install or update

Supported automatic package managers are Homebrew on macOS and pacman on Arch
Linux. The installer needs Go 1.26 or newer to build the binary. At runtime,
only `glab` and `fzf` are required.

```bash
git clone git@github.com:JonasLewe/glab-helper.git ~/.local/share/glab-helper
cd ~/.local/share/glab-helper
./install.sh
```

For an existing clone:

```bash
cd ~/.local/share/glab-helper
git pull --ff-only
./install.sh
```

The installer builds and verifies a temporary binary before atomically
replacing `~/.local/bin/glab-helper`. A failed build leaves the installed
version untouched. If `~/.local/bin` is not already in `PATH`, add this to your
shell configuration:

```bash
export PATH="$HOME/.local/bin:$PATH"
```

Authenticate the GitLab CLI once and verify the connection:

```bash
glab auth login
glab auth status
```

The GitLab token used by `glab` needs API access to the target project.

## YouTrack setup

### 1. Create the YouTrack tag

Create the release tag used to select work items, for example `mind-v1`, and
assign it to every Epic and User Story that belongs to the synchronization.
The tag does not technically need to be shared; make it shared when the team
should be able to find and assign it.

Every selected non-root item must have exactly one selected parent on the
preceding configured hierarchy level. With Epic -> User Story, each selected
User Story therefore needs one selected Epic parent. Epics themselves have no
parent in this configuration.

### 2. Create a read-only YouTrack token

In the YouTrack account security settings, create a permanent token for an
identity that can read the selected issues, their fields, tags, and hierarchy.
The helper never writes to YouTrack.

### 3. Add GitLab CI/CD variables

In the target GitLab project, open **Settings -> CI/CD -> Variables** and add:

| Variable | Example | Masked | Hidden |
|---|---|---:|---:|
| `YOUTRACK_URL` | `https://youtrack.apps.olympus.mars.de` | No | No |
| `YOUTRACK_TOKEN` | permanent YouTrack token | Yes | **No** |
| `YOUTRACK_TARGET_PROJECT` | `mind/mind-dev` | No | No |

Use only the YouTrack base URL, without `/dashboard`. The target project is the
exact GitLab `path_with_namespace`, not its full browser URL.

`YOUTRACK_TOKEN` must not be hidden: GitLab does not expose the value of a
hidden variable through the API used by the local helper. Masking still
protects it from accidental log output.

By default, the variables are read from the current GitLab project. If a
separate project stores them, export its URL-encoded path before starting:

```bash
export GLAB_HELPER_YOUTRACK_PROJECT_PATH='group%2Fconfiguration-project'
```

### 4. Add the project configuration

From the GitLab repository that will receive the synchronized work items:

```bash
cp ~/.local/share/glab-helper/.glab-helper.json.example .glab-helper.json
```

The example is ready for the team's Epic -> User Story hierarchy. Replace only
`YOUR_VERSION_TAG` with the release tag at first, for example:

```json
"query": "project: ARC-CS-MIND tag: mind-v1 Type: Epic, {User Story}"
```

`query` is one string because it is one YouTrack search expression sent
unchanged to the YouTrack API. Its parts mean:

- `project: ARC-CS-MIND` selects the YouTrack project;
- `tag: mind-v1` selects only the current release;
- `Type: Epic, {User Story}` accepts both configured issue types. Braces keep
  the type name containing a space together.

The complete default configuration is:

```json
{
  "version": 1,
  "youtrack": {
    "query": "project: ARC-CS-MIND tag: mind-v1 Type: Epic, {User Story}",
    "fields": {
      "kind": "Type",
      "status": "State",
      "priority": "Priority"
    },
    "hierarchy": [
      {"role": "epic", "types": ["Epic"]},
      {"role": "story", "types": ["User Story"]}
    ]
  },
  "gitlab": {
    "targets": {
      "epic": "milestone",
      "story": "issue"
    }
  }
}
```

The role names are local configuration keys; the values in `types` and
`fields` must match YouTrack exactly. To introduce or remove a Feature level,
edit the query, hierarchy, and targets together. A level may map to
`milestone`, `issue`, `task`, or `ignore`. GitLab Community Edition supports
these synchronized sequences:

- `milestone`
- `issue`
- `milestone -> issue`
- `issue -> task`
- `milestone -> issue -> task`

Add the local files to the target repository's `.gitignore`:

```gitignore
/.glab-helper.json
/.glab-helper-snapshots/
```

Set `GLAB_HELPER_CONFIG` only when the JSON file lives at a different path.

### 5. Validate without writes

Run this from the configured target repository:

```bash
glab-helper --dry-run
```

The command reads the complete YouTrack and GitLab state, prints the planned
changes, and exits without modifying GitLab, YouTrack, local branches, or
snapshots.

## Daily use

```bash
cd /path/to/target-repository
glab-helper
```

The normal menu contains `Sync YouTrack` and `Exit`. Synchronization creates or
updates the required GitLab labels, milestones, issues, tasks, and missing
label-based lists on the `Development` issue board. If that board does not
exist, the helper uses the project board with the lowest ID. GitLab's built-in
Open and Closed lists remain responsible for open and resolved endpoints.

Descriptions receive a stable YouTrack identity marker. Existing targets may
also be adopted by title or a recognized legacy marker. Ambiguous identities
stop the run instead of creating duplicates. Applying a plan stops at the
first failed action; run `--dry-run` again before retrying.

## Modes

```text
glab-helper [--dev | --maintenance] [--dry-run] [--version]
```

- `--dry-run` previews the complete synchronization without writes.
- `--dev` exposes granular synchronization, manual issue creation, work on an
  existing issue or task, branch handling, issue editing, and snapshot export.
- `--maintenance` exposes only the guarded project reset.
- `--help` and `--version` work without GitLab or YouTrack access.

`--dev` and `--maintenance` cannot be combined. Work on an existing issue is
developer-only because it is not part of the normal team workflow.

### Branch and issue workflow

In `--dev`, selecting an issue or task fetches and prunes `origin`, then offers
an existing matching branch or a new `<iid>-<title-slug>` branch from a chosen
local or remote base. A new branch is local until the user pushes it.

GitLab issues can also be edited: description, labels, assignee, milestone,
and closing are supported. Descriptions use `VISUAL`, then `EDITOR`, then
`nvim`, `vim`, or `vi`. Tasks currently expose branch handling only.

### Snapshots and maintenance

Manual exports and mandatory pre-reset backups create private timestamped
directories below `.glab-helper-snapshots/` automatically; users never need to
create that directory themselves.

Always preview maintenance first:

```bash
glab-helper --maintenance --dry-run
glab-helper --maintenance
```

The real reset requires `RESET ALL <full-project-path>`, writes a snapshot,
revalidates the plan, and then deletes issues before milestones. Branches,
labels, merge requests, and YouTrack are preserved. A snapshot is an audit
backup, not an automatic restore mechanism.

## Troubleshooting

| Message | Meaning and fix |
|---|---|
| `requires a valid project configuration and YouTrack access` | Check `.glab-helper.json`, the three GitLab variables, `glab auth status`, and project permissions. |
| variable shows `value present: false` | Disable **Hidden** for that variable. Keep the token **Masked**. |
| `field "parent" must contain exactly one valid issue` | A selected non-root item has zero or multiple parents for the configured hierarchy, or a root item has the wrong YouTrack type. |
| `references parent ... outside the configured query` | The parent is not selected by the query; check its project, release tag, and type. |
| type is not assigned to a hierarchy role | Add that exact YouTrack type to `hierarchy`, or correct the issue type/query. |

Use `NO_COLOR=1`, `CLICOLOR=0`, or `TERM=dumb` for plain terminal output.

## Development

```bash
go vet -all ./...
go test -race -count=1 ./...
go test -shuffle=on -count=10 ./...
./tests/install.sh
go build -trimpath -o /tmp/glab-helper ./cmd/glab-helper
```

The Go implementation uses only the standard library. The last stable Zsh
implementation is preserved by the `v0.1.0-zsh-final` tag.
