package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JonasLewe/glab-helper/internal/projectconfig"
	"github.com/JonasLewe/glab-helper/internal/source"
	"github.com/JonasLewe/glab-helper/internal/youtrack"
)

func TestOfflineCLI(t *testing.T) {
	tests := []struct {
		name string
		args []string
		code int
		want string
	}{
		{name: "help", args: []string{"--dev", "--dry-run", "--help"}, want: "Usage: glab-helper"},
		{name: "version", args: []string{"--dry-run", "--version"}, want: "glab-helper 0.1.0"},
		{name: "conflicting modes", args: []string{"--dev", "--maintenance", "--help"}, code: 2, want: "cannot be combined"},
		{name: "unknown argument", args: []string{"--version", "--unknown"}, code: 2, want: "Unknown argument"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			if code := run(test.args, strings.NewReader(""), &stdout, &stderr); code != test.code {
				t.Fatalf("exit code = %d, want %d", code, test.code)
			}
			if output := stdout.String() + stderr.String(); !strings.Contains(output, test.want) {
				t.Fatalf("output %q does not contain %q", output, test.want)
			}
		})
	}
}

func TestOnlineCLIReadsProjectData(t *testing.T) {
	temporaryDirectory := t.TempDir()
	commandLog := filepath.Join(temporaryDirectory, "commands")
	glabPath := filepath.Join(temporaryDirectory, "glab")
	gitPath := filepath.Join(temporaryDirectory, "git")
	fzfPath := filepath.Join(temporaryDirectory, "fzf")
	projectConfigPath := filepath.Join(temporaryDirectory, "project-config.json")
	projectConfiguration := `{
  "version": 1,
  "youtrack": {
    "query": "project: APP tag: gitlab-sync",
    "fields": {"kind": "Type", "status": "State", "priority": "Priority"},
    "hierarchy": [
      {"role": "epic", "types": ["Epic"]},
      {"role": "feature", "types": ["Feature"]},
      {"role": "story", "types": ["User Story"]}
    ]
  },
  "gitlab": {"targets": {"epic": "milestone", "feature": "issue", "story": "task"}}
}`
	if err := os.WriteFile(projectConfigPath, []byte(projectConfiguration), 0o600); err != nil {
		t.Fatal(err)
	}
	glabStub := `#!/bin/sh
if [ "$1 $2" = "api graphql" ]; then
  printf '%s\n' 'api graphql tasks' >>"$COMMAND_LOG"
  printf '%s\n' '{"data":{"namespace":{"workItems":{"nodes":[],"pageInfo":{"hasNextPage":false,"endCursor":null}}}}}'
  exit 0
fi
case "$*" in
  "api projects/42/milestones -X POST"*)
    printf '%s\n' 'api create milestone' >>"$COMMAND_LOG"
    printf '%s\n' '{"id":10,"title":"New Platform","description":"Epic details","state":"active"}'
    exit 0
    ;;
esac
printf '%s\n' "$*" >>"$COMMAND_LOG"
case "$*" in
  "repo view --output json")
    printf '%s\n' '{"id":42,"path_with_namespace":"group/project"}'
    ;;
  "api projects/group%2Fproject/variables/YOUTRACK_URL")
    if [ "${YOUTRACK_CONFIG_UNAVAILABLE:-}" = true ]; then exit 1; fi
    printf '{"value":"%s"}\n' "$YOUTRACK_URL"
    ;;
  "api projects/group%2Fproject/variables/YOUTRACK_TOKEN")
    printf '%s\n' '{"value":"secret-token"}'
    ;;
  "api projects/group%2Fproject/variables/YOUTRACK_TARGET_PROJECT")
    printf '%s\n' '{"value":"group/project"}'
    ;;
  "api --paginate projects/42/issues?state=all&issue_type=issue&per_page=100")
    printf '%s\n' '[{"iid":7,"title":"Issue","description":"","labels":[],"milestone":null,"state":"opened","assignees":[]}]'
    ;;
  "api --paginate projects/42/milestones?per_page=100")
    printf '%s\n' '[{"id":8,"title":"Milestone","description":null,"state":"active"}]'
    ;;
  "api --paginate projects/42/labels?per_page=100")
    printf '%s\n' '[{"id":9,"name":"team-a","color":"#5843ad"}]'
    ;;
  *)
    exit 99
    ;;
esac
`
	if err := os.WriteFile(glabPath, []byte(glabStub), 0o755); err != nil {
		t.Fatal(err)
	}
	gitStub := `#!/bin/sh
printf '%s\n' "$*" >>"$COMMAND_LOG"
case "$*" in
  "for-each-ref --sort=-committerdate --format=%(refname:strip=3)%00%(symref) refs/remotes/origin/")
    printf 'feature/new\0\nHEAD\0refs/remotes/origin/main\nmain\0\n'
    ;;
  *)
    exit 99
    ;;
esac
`
	if err := os.WriteFile(gitPath, []byte(gitStub), 0o755); err != nil {
		t.Fatal(err)
	}
	fzfStub := `#!/bin/sh
IFS= read -r selected
printf '%s\n' "$selected"
`
	if err := os.WriteFile(fzfPath, []byte(fzfStub), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("COMMAND_LOG", commandLog)
	t.Setenv("PATH", temporaryDirectory+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("GLAB_HELPER_YOUTRACK_PROJECT_PATH", "")

	const gitLabSnapshotCommands = "api --paginate projects/42/issues?state=all&issue_type=issue&per_page=100\napi graphql tasks\napi --paginate projects/42/milestones?per_page=100\napi --paginate projects/42/labels?per_page=100\n"
	const workItemReadCommands = "api --paginate projects/42/issues?state=all&issue_type=issue&per_page=100\napi graphql tasks\nfor-each-ref --sort=-committerdate --format=%(refname:strip=3)%00%(symref) refs/remotes/origin/\n"
	tests := []struct {
		name                 string
		args                 []string
		youTrackUnavailable  bool
		projectConfigMissing bool
		youTrackReadFails    bool
		snapshot             source.Snapshot
		input                string
		code                 int
		wantOutput           string
		wantCommandSuffix    string
	}{
		{
			name:              "YouTrack config available",
			code:              0,
			wantOutput:        "No GitLab changes are needed",
			wantCommandSuffix: "api projects/group%2Fproject/variables/YOUTRACK_URL\napi projects/group%2Fproject/variables/YOUTRACK_TOKEN\napi projects/group%2Fproject/variables/YOUTRACK_TARGET_PROJECT\n" + gitLabSnapshotCommands,
		},
		{
			name:              "read-only combined preview",
			args:              []string{"--dry-run"},
			code:              0,
			wantOutput:        "no GitLab or YouTrack changes have been applied",
			wantCommandSuffix: "api projects/group%2Fproject/variables/YOUTRACK_URL\napi projects/group%2Fproject/variables/YOUTRACK_TOKEN\napi projects/group%2Fproject/variables/YOUTRACK_TARGET_PROJECT\n" + gitLabSnapshotCommands,
		},
		{
			name:              "apply requires explicit confirmation",
			snapshot:          source.Snapshot{WorkItems: []source.WorkItem{{ID: "APP-1", Title: "New Platform", Description: "Epic details", Kind: "Epic", Role: "epic"}}},
			code:              0,
			wantOutput:        "Synchronization cancelled",
			wantCommandSuffix: "api projects/group%2Fproject/variables/YOUTRACK_URL\napi projects/group%2Fproject/variables/YOUTRACK_TOKEN\napi projects/group%2Fproject/variables/YOUTRACK_TARGET_PROJECT\n" + gitLabSnapshotCommands,
		},
		{
			name:              "confirmed plan is applied",
			snapshot:          source.Snapshot{WorkItems: []source.WorkItem{{ID: "APP-1", Title: "New Platform", Description: "Epic details", Kind: "Epic", Role: "epic"}}},
			input:             "yes\n",
			code:              0,
			wantOutput:        "Applied 1 GitLab synchronization actions",
			wantCommandSuffix: "api projects/group%2Fproject/variables/YOUTRACK_URL\napi projects/group%2Fproject/variables/YOUTRACK_TOKEN\napi projects/group%2Fproject/variables/YOUTRACK_TARGET_PROJECT\n" + gitLabSnapshotCommands + "api create milestone\n",
		},
		{
			name:                "YouTrack config optional",
			youTrackUnavailable: true,
			code:                0,
			wantOutput:          "Selected issue #7",
			wantCommandSuffix:   "api projects/group%2Fproject/variables/YOUTRACK_URL\n" + workItemReadCommands,
		},
		{
			name:                "maintenance requires YouTrack config",
			args:                []string{"--maintenance"},
			youTrackUnavailable: true,
			code:                1,
			wantOutput:          "Maintenance mode requires",
			wantCommandSuffix:   "api projects/group%2Fproject/variables/YOUTRACK_URL\n",
		},
		{
			name:                 "explicit missing project config fails",
			projectConfigMissing: true,
			code:                 1,
			wantOutput:           "Cannot load the project configuration",
			wantCommandSuffix:    "",
		},
		{
			name:              "YouTrack read failure stops before GitLab reads",
			args:              []string{"--dry-run"},
			youTrackReadFails: true,
			code:              1,
			wantOutput:        "Cannot read the complete YouTrack source snapshot",
			wantCommandSuffix: "api projects/group%2Fproject/variables/YOUTRACK_URL\napi projects/group%2Fproject/variables/YOUTRACK_TOKEN\napi projects/group%2Fproject/variables/YOUTRACK_TARGET_PROJECT\n",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			youTrackRequests := 0
			previousReadYouTrackSnapshot := readYouTrackSnapshot
			readYouTrackSnapshot = func(_ context.Context, _ youtrack.Config, _ projectconfig.Config) (source.Snapshot, error) {
				youTrackRequests++
				if test.youTrackReadFails {
					return source.Snapshot{}, errors.New("later page failed")
				}
				if test.snapshot.WorkItems != nil {
					return test.snapshot, nil
				}
				return source.Snapshot{WorkItems: []source.WorkItem{}}, nil
			}
			defer func() { readYouTrackSnapshot = previousReadYouTrackSnapshot }()
			t.Setenv("YOUTRACK_URL", "https://youtrack.example.com")
			if test.projectConfigMissing {
				t.Setenv("GLAB_HELPER_CONFIG", filepath.Join(temporaryDirectory, "missing.json"))
			} else {
				t.Setenv("GLAB_HELPER_CONFIG", projectConfigPath)
			}

			if err := os.Remove(commandLog); err != nil && !os.IsNotExist(err) {
				t.Fatal(err)
			}
			if test.youTrackUnavailable || test.projectConfigMissing {
				t.Setenv("YOUTRACK_CONFIG_UNAVAILABLE", "true")
			} else {
				t.Setenv("YOUTRACK_CONFIG_UNAVAILABLE", "")
			}

			var stdout, stderr bytes.Buffer
			if code := run(test.args, strings.NewReader(test.input), &stdout, &stderr); code != test.code {
				t.Fatalf("exit code = %d, want %d; stderr: %s", code, test.code, stderr.String())
			}
			if output := stdout.String() + stderr.String(); !strings.Contains(output, test.wantOutput) {
				t.Fatalf("output %q does not contain %q", output, test.wantOutput)
			}

			commands, err := os.ReadFile(commandLog)
			if err != nil {
				t.Fatal(err)
			}
			wantCommands := "repo view --output json\n" + test.wantCommandSuffix
			if string(commands) != wantCommands {
				t.Fatalf("commands = %q, want %q", commands, wantCommands)
			}
			wantYouTrackRequests := 1
			if test.youTrackUnavailable || test.projectConfigMissing {
				wantYouTrackRequests = 0
			}
			if youTrackRequests != wantYouTrackRequests {
				t.Fatalf("YouTrack request count = %d, want %d", youTrackRequests, wantYouTrackRequests)
			}
		})
	}
}
