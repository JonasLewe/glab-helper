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

func TestOnlineCLIReadsAllIssues(t *testing.T) {
	temporaryDirectory := t.TempDir()
	commandLog := filepath.Join(temporaryDirectory, "commands")
	glabPath := filepath.Join(temporaryDirectory, "glab")
	glabStub := `#!/bin/sh
printf '%s\n' "$*" >>"$COMMAND_LOG"
case "$*" in
  "repo view --output json")
    printf '%s\n' '{"id":42,"path_with_namespace":"group/project"}'
    ;;
  "api --paginate projects/42/issues?state=all&per_page=100")
    printf '%s\n' '[{"iid":7,"title":"Issue","description":"","labels":[],"milestone":null,"state":"opened","assignees":[]}]'
    ;;
  *)
    exit 99
    ;;
esac
`
	if err := os.WriteFile(glabPath, []byte(glabStub), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("COMMAND_LOG", commandLog)
	t.Setenv("PATH", temporaryDirectory+string(os.PathListSeparator)+os.Getenv("PATH"))

	var stdout, stderr bytes.Buffer
	if code := run(nil, &stdout, &stderr); code != 2 {
		t.Fatalf("exit code = %d, want 2; stderr: %s", code, stderr.String())
	}
	if output := stderr.String(); !strings.Contains(output, "read 1 GitLab issues without changes") {
		t.Fatalf("output %q does not report the complete issue read", output)
	}

	commands, err := os.ReadFile(commandLog)
	if err != nil {
		t.Fatal(err)
	}
	wantCommands := "repo view --output json\napi --paginate projects/42/issues?state=all&per_page=100\n"
	if string(commands) != wantCommands {
		t.Fatalf("commands = %q, want %q", commands, wantCommands)
	}
}
