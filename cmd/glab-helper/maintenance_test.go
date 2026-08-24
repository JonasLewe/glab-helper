package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/JonasLewe/glab-helper/internal/gitlab"
	"github.com/JonasLewe/glab-helper/internal/projectconfig"
	"github.com/JonasLewe/glab-helper/internal/source"
	"github.com/JonasLewe/glab-helper/internal/youtrack"
)

func TestSameResetPlanIgnoresOrderingButDetectsChanges(t *testing.T) {
	left := resetPlan{
		Issues: []gitlab.Issue{
			{IID: 12, Title: "Second", Description: "Details", Labels: []string{"b", "a"}, State: "closed", Assignees: []string{"sam", "alex"}},
			{IID: 11, Title: "First", Labels: []string{}, State: "opened", Assignees: []string{}},
		},
		Milestones: []gitlab.Milestone{{ID: 22, Title: "Second", State: "active"}, {ID: 21, Title: "First", State: "closed"}},
	}
	right := resetPlan{
		Issues: []gitlab.Issue{
			{IID: 11, Title: "First", State: "opened"},
			{IID: 12, Title: "Second", Description: "Details", Labels: []string{"a", "b"}, State: "closed", Assignees: []string{"alex", "sam"}},
		},
		Milestones: []gitlab.Milestone{{ID: 21, Title: "First", State: "closed"}, {ID: 22, Title: "Second", State: "active"}},
	}
	if !sameResetPlan(left, right) {
		t.Fatal("semantically identical reset plans were treated as changed")
	}
	right.Issues[1].Description = "Changed"
	if sameResetPlan(left, right) {
		t.Fatal("changed reset plan was accepted")
	}
}

func TestMaintenanceResetSafetyWorkflow(t *testing.T) {
	tests := []struct {
		name                 string
		args                 []string
		input                string
		scenario             string
		code                 int
		wantOutput           []string
		wantSnapshot         bool
		wantIssueDeletes     bool
		wantMilestoneDeletes bool
	}{
		{
			name:       "dry run",
			args:       []string{"--maintenance", "--dry-run"},
			wantOutput: []string{"3 issues will be permanently deleted", "2 milestones will be permanently deleted", "no GitLab changes or local snapshots were written"},
		},
		{
			name:       "wrong confirmation",
			args:       []string{"--maintenance"},
			input:      "wrong project\n",
			wantOutput: []string{"Reset cancelled. Nothing was deleted"},
		},
		{
			name:                 "successful reset",
			args:                 []string{"--maintenance"},
			input:                "RESET ALL group/project\n",
			wantOutput:           []string{"Mandatory pre-reset snapshot written", "Reset complete: 3 issues and 2 milestones deleted"},
			wantSnapshot:         true,
			wantIssueDeletes:     true,
			wantMilestoneDeletes: true,
		},
		{
			name:         "plan changes after confirmation",
			args:         []string{"--maintenance"},
			input:        "RESET ALL group/project\n",
			scenario:     "plan-change",
			code:         1,
			wantOutput:   []string{"deletion plan changed after confirmation"},
			wantSnapshot: true,
		},
		{
			name:             "issue deletion failure skips milestones",
			args:             []string{"--maintenance"},
			input:            "RESET ALL group/project\n",
			scenario:         "issue-failure",
			code:             1,
			wantOutput:       []string{"Failed to delete issue #11", "Milestone deletion skipped"},
			wantSnapshot:     true,
			wantIssueDeletes: true,
		},
		{
			name:                 "milestone deletion failure is reported",
			args:                 []string{"--maintenance"},
			input:                "RESET ALL group/project\n",
			scenario:             "milestone-failure",
			code:                 1,
			wantOutput:           []string{"Failed to delete milestone 21", "Reset incomplete"},
			wantSnapshot:         true,
			wantIssueDeletes:     true,
			wantMilestoneDeletes: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			directory, commandLog := prepareMaintenanceTest(t)
			t.Setenv("RESET_SCENARIO", test.scenario)
			var sourceReads int
			previousRead := readYouTrackSnapshot
			readYouTrackSnapshot = func(context.Context, youtrack.Config, projectconfig.Config) (source.Snapshot, error) {
				sourceReads++
				return source.Snapshot{}, errors.New("maintenance must not read YouTrack issues")
			}
			defer func() { readYouTrackSnapshot = previousRead }()

			var stdout, stderr bytes.Buffer
			if code := run(test.args, strings.NewReader(test.input), &stdout, &stderr); code != test.code {
				t.Fatalf("exit code = %d, want %d; stdout: %s; stderr: %s", code, test.code, stdout.String(), stderr.String())
			}
			output := stdout.String() + stderr.String()
			for _, want := range test.wantOutput {
				if !strings.Contains(output, want) {
					t.Fatalf("output %q does not contain %q", output, want)
				}
			}
			if sourceReads != 0 {
				t.Fatalf("YouTrack source reads = %d", sourceReads)
			}
			_, snapshotErr := os.Stat(filepath.Join(directory, ".glab-helper-snapshots"))
			if test.wantSnapshot && snapshotErr != nil {
				t.Fatalf("snapshot was not created: %v", snapshotErr)
			}
			if !test.wantSnapshot && !os.IsNotExist(snapshotErr) {
				t.Fatalf("unexpected snapshot state: %v", snapshotErr)
			}
			commands, err := os.ReadFile(commandLog)
			if err != nil {
				t.Fatal(err)
			}
			hasIssueDeletes := strings.Contains(string(commands), "issues/11 -X DELETE") && strings.Contains(string(commands), "issues/13 -X DELETE")
			hasMilestoneDeletes := strings.Contains(string(commands), "milestones/21 -X DELETE") && strings.Contains(string(commands), "milestones/22 -X DELETE")
			if hasIssueDeletes != test.wantIssueDeletes {
				t.Fatalf("issue delete presence = %t, want %t; commands: %s", hasIssueDeletes, test.wantIssueDeletes, commands)
			}
			if hasMilestoneDeletes != test.wantMilestoneDeletes {
				t.Fatalf("milestone delete presence = %t, want %t; commands: %s", hasMilestoneDeletes, test.wantMilestoneDeletes, commands)
			}
		})
	}
}

func TestMaintenanceSnapshotFailureBlocksDeletes(t *testing.T) {
	_, commandLog := prepareMaintenanceTest(t)
	previousWrite := writeProjectBackup
	writeProjectBackup = func(string, time.Time, int64, string, string, []gitlab.Issue, []gitlab.Milestone, []gitlab.Label) (string, error) {
		return "", errors.New("disk full")
	}
	defer func() { writeProjectBackup = previousWrite }()

	var stdout, stderr bytes.Buffer
	code := run([]string{"--maintenance"}, strings.NewReader("RESET ALL group/project\n"), &stdout, &stderr)
	if code != 1 || !strings.Contains(stderr.String(), "mandatory pre-reset snapshot failed") {
		t.Fatalf("exit code = %d; stdout: %s; stderr: %s", code, stdout.String(), stderr.String())
	}
	commands, err := os.ReadFile(commandLog)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(commands), "-X DELETE") {
		t.Fatalf("snapshot failure allowed deletes: %s", commands)
	}
}

func prepareMaintenanceTest(t *testing.T) (string, string) {
	t.Helper()
	directory := t.TempDir()
	t.Chdir(directory)
	commandLog := filepath.Join(directory, "commands")
	glabPath := filepath.Join(directory, "glab")
	fzfPath := filepath.Join(directory, "fzf")
	configPath := filepath.Join(directory, ".glab-helper.json")
	config := `{
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
	if err := os.WriteFile(configPath, []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	glabStub := `#!/bin/sh
printf '%s\n' "$*" >>"$COMMAND_LOG"
issues='[
  {"iid":11,"title":"First issue","description":"Details","labels":["team-a"],"milestone":{"title":"Release"},"state":"opened","assignees":[{"username":"alex"}]},
  {"iid":12,"title":"Closed issue","description":"","labels":[],"milestone":null,"state":"closed","assignees":[]},
  {"iid":13,"title":"Manual issue","description":"Keep me","labels":[],"milestone":null,"state":"opened","assignees":[]}
]'
issues_changed='[
  {"iid":11,"title":"Changed after confirmation","description":"Details","labels":["team-a"],"milestone":{"title":"Release"},"state":"opened","assignees":[{"username":"alex"}]},
  {"iid":12,"title":"Closed issue","description":"","labels":[],"milestone":null,"state":"closed","assignees":[]},
  {"iid":13,"title":"Manual issue","description":"Keep me","labels":[],"milestone":null,"state":"opened","assignees":[]}
]'
milestones='[
  {"id":21,"title":"Release","description":"Plan","state":"active"},
  {"id":22,"title":"Manual milestone","description":null,"state":"closed"}
]'
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
  "api --paginate projects/42/issues?state=all&issue_type=issue&per_page=100")
    count=$(grep -cF 'api --paginate projects/42/issues?state=all&issue_type=issue&per_page=100' "$COMMAND_LOG")
    if [ "${RESET_SCENARIO:-}" = plan-change ] && [ "$count" -gt 1 ]; then
      printf '%s\n' "$issues_changed"
    else
      printf '%s\n' "$issues"
    fi
    ;;
  "api --paginate projects/42/milestones?per_page=100")
    printf '%s\n' "$milestones"
    ;;
  "api --paginate projects/42/labels?per_page=100")
    printf '%s\n' '[{"id":9,"name":"team-a"}]'
    ;;
  "api projects/42/issues/11 -X DELETE")
    if [ "${RESET_SCENARIO:-}" = issue-failure ]; then
      printf '%s\n' 'HTTP 403' >&2
      exit 1
    fi
    ;;
  "api projects/42/issues/12 -X DELETE"|"api projects/42/issues/13 -X DELETE")
    ;;
  "api projects/42/milestones/21 -X DELETE")
    if [ "${RESET_SCENARIO:-}" = milestone-failure ]; then
      printf '%s\n' 'HTTP 403' >&2
      exit 1
    fi
    ;;
  "api projects/42/milestones/22 -X DELETE")
    ;;
  *)
    printf '%s\n' "unexpected glab invocation: $*" >&2
    exit 99
    ;;
esac
`
	fzfStub := `#!/bin/sh
IFS= read -r selected
printf '%s\n' "$selected"
`
	for path, content := range map[string]string{glabPath: glabStub, fzfPath: fzfStub} {
		if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("COMMAND_LOG", commandLog)
	t.Setenv("PATH", directory+string(os.PathListSeparator)+os.Getenv("PATH"))
	return directory, commandLog
}
