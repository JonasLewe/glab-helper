package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
			if code := run(test.args, &stdout, &stderr); code != test.code {
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
	glabStub := `#!/bin/sh
printf '%s\n' "$*" >>"$COMMAND_LOG"
case "$*" in
  "repo view --output json")
    printf '%s\n' '{"id":42,"path_with_namespace":"group/project"}'
    ;;
  "api projects/group%2Fproject/variables/YOUTRACK_URL")
    if [ "${YOUTRACK_CONFIG_UNAVAILABLE:-}" = true ]; then exit 1; fi
    printf '%s\n' '{"value":"https://youtrack.example.com"}'
    ;;
  "api projects/group%2Fproject/variables/YOUTRACK_QUERY")
    printf '%s\n' '{"value":"project: APP tag: gitlab-sync"}'
    ;;
  "api projects/group%2Fproject/variables/YOUTRACK_TOKEN")
    printf '%s\n' '{"value":"secret-token"}'
    ;;
  "api projects/group%2Fproject/variables/YOUTRACK_TARGET_PROJECT")
    printf '%s\n' '{"value":"group/project"}'
    ;;
  "api --paginate projects/42/issues?state=all&per_page=100")
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
	t.Setenv("COMMAND_LOG", commandLog)
	t.Setenv("PATH", temporaryDirectory+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("GLAB_HELPER_YOUTRACK_PROJECT_PATH", "")

	const projectReadCommands = "api --paginate projects/42/issues?state=all&per_page=100\napi --paginate projects/42/milestones?per_page=100\napi --paginate projects/42/labels?per_page=100\nfor-each-ref --sort=-committerdate --format=%(refname:strip=3)%00%(symref) refs/remotes/origin/\n"
	tests := []struct {
		name                string
		args                []string
		youTrackUnavailable bool
		code                int
		wantOutput          string
		wantCommandSuffix   string
	}{
		{
			name:              "YouTrack config available",
			code:              2,
			wantOutput:        "YouTrack configuration is available",
			wantCommandSuffix: "api projects/group%2Fproject/variables/YOUTRACK_URL\napi projects/group%2Fproject/variables/YOUTRACK_QUERY\napi projects/group%2Fproject/variables/YOUTRACK_TOKEN\napi projects/group%2Fproject/variables/YOUTRACK_TARGET_PROJECT\n" + projectReadCommands,
		},
		{
			name:                "YouTrack config optional",
			youTrackUnavailable: true,
			code:                2,
			wantOutput:          "YouTrack configuration is unavailable",
			wantCommandSuffix:   "api projects/group%2Fproject/variables/YOUTRACK_URL\n" + projectReadCommands,
		},
		{
			name:                "maintenance requires YouTrack config",
			args:                []string{"--maintenance"},
			youTrackUnavailable: true,
			code:                1,
			wantOutput:          "Maintenance mode requires",
			wantCommandSuffix:   "api projects/group%2Fproject/variables/YOUTRACK_URL\n",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := os.Remove(commandLog); err != nil && !os.IsNotExist(err) {
				t.Fatal(err)
			}
			if test.youTrackUnavailable {
				t.Setenv("YOUTRACK_CONFIG_UNAVAILABLE", "true")
			} else {
				t.Setenv("YOUTRACK_CONFIG_UNAVAILABLE", "")
			}

			var stdout, stderr bytes.Buffer
			if code := run(test.args, &stdout, &stderr); code != test.code {
				t.Fatalf("exit code = %d, want %d; stderr: %s", code, test.code, stderr.String())
			}
			if output := stderr.String(); !strings.Contains(output, test.wantOutput) {
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
		})
	}
}
