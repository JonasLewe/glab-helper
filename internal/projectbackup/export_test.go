package projectbackup

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/JonasLewe/glab-helper/internal/gitlab"
)

func TestWriteCreatesPrivateCompleteSnapshot(t *testing.T) {
	root := filepath.Join(t.TempDir(), "snapshots")
	createdAt := time.Date(2026, 8, 24, 12, 34, 56, 0, time.FixedZone("CEST", 2*60*60))
	directory, err := Write(
		root,
		createdAt,
		42,
		"group/project",
		"Manual Export",
		[]gitlab.Issue{{IID: 7, Title: "Issue", Description: "Details", Labels: []string{"team-a"}, State: "opened", Assignees: []string{"alex"}}},
		[]gitlab.Milestone{{ID: 8, Title: "Release", Description: "Plan", State: "active"}},
		[]gitlab.Label{{ID: 9, Name: "team-a"}},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(filepath.Base(directory), "20260824-123456-manual-export-") {
		t.Fatalf("snapshot directory = %q", directory)
	}
	for _, name := range []string{"issues.json", "milestones.json", "labels.json", "metadata.json"} {
		info, err := os.Stat(filepath.Join(directory, name))
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o600 {
			t.Fatalf("%s mode = %o", name, info.Mode().Perm())
		}
	}

	metadataData, err := os.ReadFile(filepath.Join(directory, "metadata.json"))
	if err != nil {
		t.Fatal(err)
	}
	var got metadata
	if err := json.Unmarshal(metadataData, &got); err != nil {
		t.Fatal(err)
	}
	if got.CreatedAt != "2026-08-24T10:34:56Z" || got.RepoName != "group/project" || got.ProjectID != 42 || got.Label != "manual-export" {
		t.Fatalf("metadata = %#v", got)
	}
	issuesData, err := os.ReadFile(filepath.Join(directory, "issues.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(issuesData), `"iid": 7`) || !strings.Contains(string(issuesData), `"assignees":`) {
		t.Fatalf("issues snapshot = %s", issuesData)
	}
}

func TestWriteRejectsInvalidDestination(t *testing.T) {
	if _, err := Write("", time.Now(), 42, "group/project", "manual", nil, nil, nil); err == nil {
		t.Fatal("empty snapshot root was accepted")
	}
}
