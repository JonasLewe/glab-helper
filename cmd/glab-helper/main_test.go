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
    printf '%s\n' '{"id":42,"path_with_namespace":"group/project","default_branch":"main"}'
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
  "api --paginate projects/42/boards?per_page=100")
    printf '%s\n' '[{"id":3,"name":"Development","lists":[]}]'
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
  "fetch --prune origin --quiet")
    ;;
  "for-each-ref --sort=-committerdate --format=%(refname)%00%(symref) refs/heads/ refs/remotes/origin/")
    printf 'refs/remotes/origin/feature/new\0\nrefs/heads/main\0\nrefs/remotes/origin/main\0\nrefs/remotes/origin/HEAD\0refs/remotes/origin/main\n'
    ;;
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

	const gitLabSnapshotCommands = "api --paginate projects/42/issues?state=all&issue_type=issue&per_page=100\napi graphql tasks\napi --paginate projects/42/milestones?per_page=100\napi --paginate projects/42/labels?per_page=100\napi --paginate projects/42/boards?per_page=100\n"
	const workItemReadCommands = "api --paginate projects/42/issues?state=all&issue_type=issue&per_page=100\napi graphql tasks\nfetch --prune origin --quiet\nfor-each-ref --sort=-committerdate --format=%(refname)%00%(symref) refs/heads/ refs/remotes/origin/\n"
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
			name:              "developer milestone preview",
			args:              []string{"--dev", "--dry-run"},
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
			name:                "dry-run diagnoses unavailable YouTrack config",
			args:                []string{"--dry-run"},
			youTrackUnavailable: true,
			code:                1,
			wantOutput:          "cannot read required YouTrack configuration variable YOUTRACK_URL",
			wantCommandSuffix:   "api projects/group%2Fproject/variables/YOUTRACK_URL\n",
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

func TestWorkItemBranchCreationWorkflow(t *testing.T) {
	temporaryDirectory := t.TempDir()
	t.Chdir(temporaryDirectory)
	commandLog := filepath.Join(temporaryDirectory, "git-commands")
	glabPath := filepath.Join(temporaryDirectory, "glab")
	gitPath := filepath.Join(temporaryDirectory, "git")
	fzfPath := filepath.Join(temporaryDirectory, "fzf")

	glabStub := `#!/bin/sh
if [ "$1 $2" = "api graphql" ]; then
  printf '%s\n' '{"data":{"namespace":{"workItems":{"nodes":[],"pageInfo":{"hasNextPage":false,"endCursor":null}}}}}'
  exit 0
fi
case "$*" in
  "repo view --output json")
    printf '%s\n' '{"id":42,"path_with_namespace":"group/project","default_branch":"main"}'
    ;;
  "api --paginate projects/42/issues?state=all&issue_type=issue&per_page=100")
    printf '%s\n' '[{"iid":7,"title":"Implement parser","description":"","labels":[],"milestone":null,"state":"opened","assignees":[]}]'
    ;;
  *)
    exit 99
    ;;
esac
`
	gitStub := `#!/bin/sh
printf '%s\n' "$*" >>"$COMMAND_LOG"
case "$*" in
  "fetch --prune origin --quiet")
    ;;
  "for-each-ref --sort=-committerdate --format=%(refname)%00%(symref) refs/heads/ refs/remotes/origin/")
    printf 'refs/heads/main\0\nrefs/remotes/origin/main\0\nrefs/remotes/origin/HEAD\0refs/remotes/origin/main\n'
    ;;
  "check-ref-format --branch 7-implement-parser")
    printf '%s\n' '7-implement-parser'
    ;;
  "branch 7-implement-parser origin/main")
    ;;
  "checkout 7-implement-parser")
    ;;
  *)
    exit 99
    ;;
esac
`
	fzfStub := `#!/bin/sh
case "$*" in
  *"Action >"*)
    printf '%s\n' 'Work on existing issue or task'
    ;;
  *"Issue or task >"*)
    IFS= read -r selected
    printf '%s\n' "$selected"
    ;;
  *"Work item action >"*)
    printf '%s\n' 'Branch (checkout / create)'
    ;;
  *"Base branch >"*)
    printf '%s\n' 'main (default)'
    ;;
  *)
    exit 99
    ;;
esac
`
	for path, content := range map[string]string{glabPath: glabStub, gitPath: gitStub, fzfPath: fzfStub} {
		if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("COMMAND_LOG", commandLog)
	t.Setenv("PATH", temporaryDirectory+string(os.PathListSeparator)+os.Getenv("PATH"))

	var stdout, stderr bytes.Buffer
	if code := run(nil, strings.NewReader("\ny\n"), &stdout, &stderr); code != 0 {
		t.Fatalf("exit code = %d; stderr: %s", code, stderr.String())
	}
	output := stdout.String()
	if !strings.Contains(output, "Created branch 7-implement-parser from main") || !strings.Contains(output, "Checked out branch 7-implement-parser") {
		t.Fatalf("output = %q", output)
	}
	commands, err := os.ReadFile(commandLog)
	if err != nil {
		t.Fatal(err)
	}
	wantCommands := "fetch --prune origin --quiet\n" +
		"for-each-ref --sort=-committerdate --format=%(refname)%00%(symref) refs/heads/ refs/remotes/origin/\n" +
		"check-ref-format --branch 7-implement-parser\n" +
		"branch 7-implement-parser origin/main\n" +
		"checkout 7-implement-parser\n"
	if string(commands) != wantCommands {
		t.Fatalf("git commands = %q, want %q", commands, wantCommands)
	}
}

func TestIssueEditWorkflowLoopsThroughActions(t *testing.T) {
	temporaryDirectory := t.TempDir()
	t.Chdir(temporaryDirectory)
	commandLog := filepath.Join(temporaryDirectory, "commands")
	fzfState := filepath.Join(temporaryDirectory, "fzf-state")
	glabPath := filepath.Join(temporaryDirectory, "glab")
	gitPath := filepath.Join(temporaryDirectory, "git")
	fzfPath := filepath.Join(temporaryDirectory, "fzf")
	editorPath := filepath.Join(temporaryDirectory, "editor")

	glabStub := `#!/bin/sh
printf '%s\n' "$*" >>"$COMMAND_LOG"
if [ "$1 $2" = "api graphql" ]; then
  printf '%s\n' '{"data":{"namespace":{"workItems":{"nodes":[],"pageInfo":{"hasNextPage":false,"endCursor":null}}}}}'
  exit 0
fi
case "$*" in
  "repo view --output json")
    printf '%s\n' '{"id":42,"path_with_namespace":"group/project","default_branch":"main"}'
    ;;
  "api --paginate projects/42/issues?state=all&issue_type=issue&per_page=100")
    printf '%s\n' '[{"iid":7,"title":"Implement parser","description":"Old details","labels":["backend"],"milestone":{"title":"Release"},"state":"opened","assignees":[{"username":"alex"}]}]'
    ;;
  "api --paginate projects/42/labels?per_page=100")
    printf '%s\n' '[{"id":3,"name":"backend"},{"id":4,"name":"team-a"}]'
    ;;
  "api --paginate projects/42/members/all?per_page=100")
    printf '%s\n' '[{"id":5,"username":"alex","name":"Alex Example"},{"id":6,"username":"sam","name":"Sam Example"}]'
    ;;
  "api --paginate projects/42/milestones?per_page=100")
    printf '%s\n' '[{"id":8,"title":"Release","description":null,"state":"active"},{"id":9,"title":"Next","description":null,"state":"active"}]'
    ;;
  "api projects/42/issues/7 -X PUT"*)
    printf '%s\n' '{}'
    ;;
  *)
    exit 99
    ;;
esac
`
	gitStub := `#!/bin/sh
case "$*" in
  "fetch --prune origin --quiet")
    ;;
  "for-each-ref --sort=-committerdate --format=%(refname)%00%(symref) refs/heads/ refs/remotes/origin/")
    printf 'refs/heads/main\0\nrefs/remotes/origin/main\0\nrefs/remotes/origin/HEAD\0refs/remotes/origin/main\n'
    ;;
  *)
    exit 99
    ;;
esac
`
	fzfStub := `#!/bin/sh
case "$*" in
  *"Action >"*)
    printf '%s\n' 'Work on existing issue or task'
    ;;
  *"Issue or task >"*)
    IFS= read -r selected
    printf '%s\n' "$selected"
    ;;
  *"Work item action >"*)
    step=0
    if [ -f "$FZF_STATE" ]; then step=$(cat "$FZF_STATE"); fi
    step=$((step + 1))
    printf '%s\n' "$step" >"$FZF_STATE"
    case "$step" in
      1) printf '%s\n' 'Edit description' ;;
      2) printf '%s\n' 'Edit labels' ;;
      3) printf '%s\n' 'Edit assignee' ;;
      4) printf '%s\n' 'Edit milestone' ;;
      5) printf '%s\n' 'Close issue' ;;
      *) exit 99 ;;
    esac
    ;;
  *"Label >"*)
    printf '%s\n' 'Add label: team-a'
    ;;
  *"Assignee >"*)
    printf '%s\n' 'sam (Sam Example)'
    ;;
  *"Milestone >"*)
    printf '%s\n' 'Next [milestone 9]'
    ;;
  *)
    exit 99
    ;;
esac
`
	editorStub := "#!/bin/sh\nprintf '%s' 'New details' >\"$1\"\n"
	for path, content := range map[string]string{glabPath: glabStub, gitPath: gitStub, fzfPath: fzfStub, editorPath: editorStub} {
		if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("COMMAND_LOG", commandLog)
	t.Setenv("FZF_STATE", fzfState)
	t.Setenv("VISUAL", editorPath)
	t.Setenv("EDITOR", "")
	t.Setenv("PATH", temporaryDirectory+string(os.PathListSeparator)+os.Getenv("PATH"))

	var stdout, stderr bytes.Buffer
	if code := run(nil, strings.NewReader("y\n"), &stdout, &stderr); code != 0 {
		t.Fatalf("exit code = %d; stdout: %s; stderr: %s", code, stdout.String(), stderr.String())
	}
	output := stdout.String()
	for _, want := range []string{
		"Updated description for issue #7",
		"Updated labels for issue #7: backend, team-a",
		"Assigned issue #7 to sam",
		"Set issue #7 milestone to Next",
		"Closed issue #7",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output %q does not contain %q", output, want)
		}
	}
	commands, err := os.ReadFile(commandLog)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"api projects/42/issues/7 -X PUT -f description=New details",
		"api projects/42/issues/7 -X PUT -f labels=backend,team-a",
		"api projects/42/issues/7 -X PUT -f assignee_id=6",
		"api projects/42/issues/7 -X PUT -f milestone_id=9",
		"api projects/42/issues/7 -X PUT -f state_event=close",
	} {
		if !strings.Contains(string(commands), want+"\n") {
			t.Fatalf("commands %q do not contain %q", commands, want)
		}
	}
}

func TestDeveloperManualIssueCreation(t *testing.T) {
	temporaryDirectory := t.TempDir()
	t.Chdir(temporaryDirectory)
	commandLog := filepath.Join(temporaryDirectory, "commands")
	glabPath := filepath.Join(temporaryDirectory, "glab")
	fzfPath := filepath.Join(temporaryDirectory, "fzf")
	editorPath := filepath.Join(temporaryDirectory, "editor")

	glabStub := `#!/bin/sh
printf '%s\n' "$*" >>"$COMMAND_LOG"
case "$*" in
  "repo view --output json")
    printf '%s\n' '{"id":42,"path_with_namespace":"group/project","default_branch":"main"}'
    ;;
  "api --paginate projects/42/labels?per_page=100")
    printf '%s\n' '[{"id":3,"name":"backend"},{"id":4,"name":"team-a"}]'
    ;;
  "api --paginate projects/42/members/all?per_page=100")
    printf '%s\n' '[{"id":6,"username":"sam","name":"Sam Example"}]'
    ;;
  "api --paginate projects/42/milestones?per_page=100")
    printf '%s\n' '[{"id":9,"title":"Next","description":null,"state":"active"}]'
    ;;
  "api projects/42/issues -X POST"*)
    printf '%s\n' '{"iid":11}'
    ;;
  *)
    exit 99
    ;;
esac
`
	fzfStub := `#!/bin/sh
case "$*" in
  *"Action >"*)
    printf '%s\n' 'Create GitLab issue'
    ;;
  *"Labels >"*)
    printf '%s\n' 'Label: backend'
    printf '%s\n' 'Label: team-a'
    ;;
  *"Assignee >"*)
    printf '%s\n' 'sam (Sam Example)'
    ;;
  *"Milestone >"*)
    printf '%s\n' 'Next [milestone 9]'
    ;;
  *)
    exit 99
    ;;
esac
`
	editorStub := "#!/bin/sh\nprintf '%s' 'Manual details' >\"$1\"\n"
	for path, content := range map[string]string{glabPath: glabStub, fzfPath: fzfStub, editorPath: editorStub} {
		if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("COMMAND_LOG", commandLog)
	t.Setenv("VISUAL", editorPath)
	t.Setenv("EDITOR", "")
	t.Setenv("PATH", temporaryDirectory+string(os.PathListSeparator)+os.Getenv("PATH"))

	var stdout, stderr bytes.Buffer
	if code := run([]string{"--dev"}, strings.NewReader("Manual issue\ny\nn\n"), &stdout, &stderr); code != 0 {
		t.Fatalf("exit code = %d; stdout: %s; stderr: %s", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), "Created GitLab issue #11") {
		t.Fatalf("stdout = %q", stdout.String())
	}
	commands, err := os.ReadFile(commandLog)
	if err != nil {
		t.Fatal(err)
	}
	want := "api projects/42/issues -X POST -f title=Manual issue -f description=Manual details -f labels=backend,team-a -f issue_type=issue -f milestone_id=9 -f assignee_id=6\n"
	if !strings.Contains(string(commands), want) {
		t.Fatalf("commands %q do not contain %q", commands, want)
	}
}

func TestDeveloperManualIssueCreationPlansNewDependencies(t *testing.T) {
	temporaryDirectory := t.TempDir()
	t.Chdir(temporaryDirectory)
	commandLog := filepath.Join(temporaryDirectory, "commands")
	fzfState := filepath.Join(temporaryDirectory, "fzf-state")
	glabPath := filepath.Join(temporaryDirectory, "glab")
	fzfPath := filepath.Join(temporaryDirectory, "fzf")
	editorPath := filepath.Join(temporaryDirectory, "editor")

	glabStub := `#!/bin/sh
printf '%s\n' "$*" >>"$COMMAND_LOG"
case "$*" in
  "repo view --output json")
    printf '%s\n' '{"id":42,"path_with_namespace":"group/project","default_branch":"main"}'
    ;;
  "api --paginate projects/42/labels?per_page=100"|"api --paginate projects/42/members/all?per_page=100"|"api --paginate projects/42/milestones?per_page=100")
    printf '%s\n' '[]'
    ;;
  "api projects/42/labels -X POST"*)
    printf '%s\n' '{"id":4,"name":"frontend"}'
    ;;
  "api projects/42/milestones -X POST"*)
    printf '%s\n' '{"id":5,"title":"Release 2","description":null,"state":"active"}'
    ;;
  "api projects/42/issues -X POST"*)
    printf '%s\n' '{"iid":12}'
    ;;
  *)
    exit 99
    ;;
esac
`
	fzfStub := `#!/bin/sh
case "$*" in
  *"Action >"*)
    printf '%s\n' 'Create GitLab issue'
    ;;
  *"Labels >"*)
    step=0
    if [ -f "$FZF_STATE" ]; then step=$(cat "$FZF_STATE"); fi
    step=$((step + 1))
    printf '%s\n' "$step" >"$FZF_STATE"
    if [ "$step" = 1 ]; then
      printf '%s\n' 'Create new label...'
    else
      printf '%s\n' 'New label: frontend'
    fi
    ;;
  *"Assignee >"*)
    printf '%s\n' 'No assignee'
    ;;
  *"Milestone >"*)
    printf '%s\n' 'Create new milestone...'
    ;;
  *)
    exit 99
    ;;
esac
`
	editorStub := "#!/bin/sh\nprintf '%s' 'New dependency details' >\"$1\"\n"
	for path, content := range map[string]string{glabPath: glabStub, fzfPath: fzfStub, editorPath: editorStub} {
		if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("COMMAND_LOG", commandLog)
	t.Setenv("FZF_STATE", fzfState)
	t.Setenv("VISUAL", editorPath)
	t.Setenv("EDITOR", "")
	t.Setenv("PATH", temporaryDirectory+string(os.PathListSeparator)+os.Getenv("PATH"))

	input := "New capability\nfrontend\n#112233\nRelease 2\n2026-09-30\ny\nn\n"
	var stdout, stderr bytes.Buffer
	if code := run([]string{"--dev"}, strings.NewReader(input), &stdout, &stderr); code != 0 {
		t.Fatalf("exit code = %d; stdout: %s; stderr: %s", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), "Created GitLab issue #12") {
		t.Fatalf("stdout = %q", stdout.String())
	}
	commands, err := os.ReadFile(commandLog)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"api projects/42/labels -X POST -f name=frontend -f color=#112233",
		"api projects/42/milestones -X POST -f title=Release 2 -f due_date=2026-09-30",
		"api projects/42/issues -X POST -f title=New capability -f description=New dependency details -f labels=frontend -f issue_type=issue -f milestone_id=5",
	} {
		if !strings.Contains(string(commands), want+"\n") {
			t.Fatalf("commands %q do not contain %q", commands, want)
		}
	}
}

func TestDeveloperSnapshotExport(t *testing.T) {
	temporaryDirectory := t.TempDir()
	t.Chdir(temporaryDirectory)
	glabPath := filepath.Join(temporaryDirectory, "glab")
	fzfPath := filepath.Join(temporaryDirectory, "fzf")

	glabStub := `#!/bin/sh
case "$*" in
  "repo view --output json")
    printf '%s\n' '{"id":42,"path_with_namespace":"group/project","default_branch":"main"}'
    ;;
  "api --paginate projects/42/issues?state=all&issue_type=issue&per_page=100")
    printf '%s\n' '[{"iid":7,"title":"Issue","description":"Details","labels":[],"milestone":null,"state":"opened","assignees":[]}]'
    ;;
  "api --paginate projects/42/milestones?per_page=100")
    printf '%s\n' '[{"id":8,"title":"Release","description":null,"state":"active"}]'
    ;;
  "api --paginate projects/42/labels?per_page=100")
    printf '%s\n' '[{"id":9,"name":"team-a"}]'
    ;;
  *)
    exit 99
    ;;
esac
`
	fzfStub := `#!/bin/sh
printf '%s\n' 'Export GitLab snapshot'
`
	for path, content := range map[string]string{glabPath: glabStub, fzfPath: fzfStub} {
		if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", temporaryDirectory+string(os.PathListSeparator)+os.Getenv("PATH"))

	var stdout, stderr bytes.Buffer
	if code := run([]string{"--dev"}, strings.NewReader(""), &stdout, &stderr); code != 0 {
		t.Fatalf("exit code = %d; stdout: %s; stderr: %s", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), "Exported GitLab snapshot to .glab-helper-snapshots/") {
		t.Fatalf("stdout = %q", stdout.String())
	}
	entries, err := os.ReadDir(filepath.Join(temporaryDirectory, ".glab-helper-snapshots"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || !entries[0].IsDir() {
		t.Fatalf("snapshot entries = %#v", entries)
	}
	for _, name := range []string{"issues.json", "milestones.json", "labels.json", "metadata.json"} {
		if _, err := os.Stat(filepath.Join(temporaryDirectory, ".glab-helper-snapshots", entries[0].Name(), name)); err != nil {
			t.Fatal(err)
		}
	}
}

func TestDeveloperCreatesOneUnsynchronizedYouTrackIssue(t *testing.T) {
	temporaryDirectory := t.TempDir()
	t.Chdir(temporaryDirectory)
	commandLog := filepath.Join(temporaryDirectory, "commands")
	glabPath := filepath.Join(temporaryDirectory, "glab")
	fzfPath := filepath.Join(temporaryDirectory, "fzf")
	projectConfigPath := filepath.Join(temporaryDirectory, "project-config.json")
	projectConfiguration := `{
  "version": 1,
  "youtrack": {
    "query": "project: APP tag: release-v1",
    "fields": {"kind": "Type", "status": "State", "priority": "Priority"},
    "hierarchy": [
      {"role": "epic", "types": ["Epic"]},
      {"role": "story", "types": ["User Story"]}
    ]
  },
  "gitlab": {"targets": {"epic": "milestone", "story": "issue"}}
}`
	if err := os.WriteFile(projectConfigPath, []byte(projectConfiguration), 0o600); err != nil {
		t.Fatal(err)
	}

	glabStub := `#!/bin/sh
printf '%s\n' "$*" >>"$COMMAND_LOG"
if [ "$1 $2" = "api graphql" ]; then
  printf '%s\n' '{"data":{"namespace":{"workItems":{"nodes":[],"pageInfo":{"hasNextPage":false,"endCursor":null}}}}}'
  exit 0
fi
case "$*" in
  "repo view --output json")
    printf '%s\n' '{"id":42,"path_with_namespace":"group/project","default_branch":"main"}'
    ;;
  "api projects/group%2Fproject/variables/YOUTRACK_URL")
    printf '%s\n' '{"value":"https://youtrack.example.com"}'
    ;;
  "api projects/group%2Fproject/variables/YOUTRACK_TOKEN")
    printf '%s\n' '{"value":"secret-token"}'
    ;;
  "api projects/group%2Fproject/variables/YOUTRACK_TARGET_PROJECT")
    printf '%s\n' '{"value":"group/project"}'
    ;;
  "api --paginate projects/42/issues?state=all&issue_type=issue&per_page=100"|"api --paginate projects/42/milestones?per_page=100"|"api --paginate projects/42/labels?per_page=100")
    printf '%s\n' '[]'
    ;;
  "api --paginate projects/42/boards?per_page=100")
    printf '%s\n' '[{"id":3,"name":"Development","lists":[]}]'
    ;;
  "api projects/42/labels -X POST -f name=release-v1 -f color=#428BCA")
    printf '%s\n' '{"id":4,"name":"release-v1"}'
    ;;
  "api projects/42/labels -X POST -f name=prio::Major -f color=#428BCA")
    printf '%s\n' '{"id":5,"name":"prio::Major"}'
    ;;
  "api projects/42/labels -X POST -f name=status::Open -f color=#428BCA")
    printf '%s\n' '{"id":6,"name":"status::Open"}'
    ;;
  "api projects/42/milestones -X POST"*)
    printf '%s\n' '{"id":8,"title":"Release","description":"Epic details\n\n<!-- glab-helper:youtrack:APP-1 -->","state":"active"}'
    ;;
  "api projects/42/issues -X POST"*)
    printf '%s\n' '{"iid":7}'
    ;;
  *)
    exit 99
    ;;
esac
`
	fzfStub := `#!/bin/sh
case "$*" in
  *"Action >"*)
    printf '%s\n' 'Create GitLab issue'
    ;;
  *"Create issue >"*)
    printf '%s\n' 'From YouTrack'
    ;;
  *"YouTrack issue >"*)
    IFS= read -r selected
    printf '%s\n' "$selected"
    ;;
  *)
    exit 99
    ;;
esac
`
	for path, content := range map[string]string{glabPath: glabStub, fzfPath: fzfStub} {
		if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("COMMAND_LOG", commandLog)
	t.Setenv("GLAB_HELPER_CONFIG", projectConfigPath)
	t.Setenv("PATH", temporaryDirectory+string(os.PathListSeparator)+os.Getenv("PATH"))
	previousReadYouTrackSnapshot := readYouTrackSnapshot
	readYouTrackSnapshot = func(context.Context, youtrack.Config, projectconfig.Config) (source.Snapshot, error) {
		return source.Snapshot{WorkItems: []source.WorkItem{
			{ID: "APP-1", Title: "Release", Description: "Epic details", Kind: "Epic", Role: "epic", Status: "Open"},
			{ID: "APP-2", Title: "Parser", Description: "Story details", Kind: "User Story", Role: "story", Status: "Open", Priority: "Major", Tags: []string{"release-v1"}, ParentID: "APP-1"},
		}}, nil
	}
	defer func() { readYouTrackSnapshot = previousReadYouTrackSnapshot }()

	var stdout, stderr bytes.Buffer
	if code := run([]string{"--dev"}, strings.NewReader("y\n"), &stdout, &stderr); code != 0 {
		t.Fatalf("exit code = %d; stdout: %s; stderr: %s", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), "Applied 5 GitLab actions for the selected YouTrack issue") {
		t.Fatalf("stdout = %q", stdout.String())
	}
	commands, err := os.ReadFile(commandLog)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"api projects/42/milestones -X POST -f title=Release",
		"api projects/42/issues -X POST -f title=[APP-2] Parser",
	} {
		if !strings.Contains(string(commands), want) {
			t.Fatalf("commands %q do not contain %q", commands, want)
		}
	}
}
