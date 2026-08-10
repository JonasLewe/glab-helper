package projectconfig

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const validConfiguration = `{
  "version": 1,
  "youtrack": {
    "query": "project: MLOPS tag: gitlab-sync",
    "fields": {"kind": "Type", "status": "State", "priority": "Priority"},
    "hierarchy": [
      {"role": "epic", "types": ["Epic"]},
      {"role": "feature", "types": ["Feature"]},
      {"role": "story", "types": ["User Story", "Story"]}
    ]
  },
  "gitlab": {
    "targets": {"epic": "milestone", "feature": "issue", "story": "task"}
  }
}`

func TestLoad(t *testing.T) {
	path := filepath.Join(t.TempDir(), "project.json")
	if err := os.WriteFile(path, []byte(validConfiguration), 0o600); err != nil {
		t.Fatal(err)
	}

	config, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	role, level, found := config.RoleForKind("user story")
	if !found || role != "story" || level != 2 {
		t.Fatalf("RoleForKind = %q, %d, %t; want story, 2, true", role, level, found)
	}
}

func TestLoadReportsMissingConfiguration(t *testing.T) {
	_, err := Load(filepath.Join(t.TempDir(), "missing.json"))
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("error = %v, want ErrNotFound", err)
	}
}

func TestLoadRejectsUnknownFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "project.json")
	configuration := strings.Replace(validConfiguration, `"version": 1`, `"version": 1, "unexpected": true`, 1)
	if err := os.WriteFile(path, []byte(configuration), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := Load(path); err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("error = %v, want unknown field", err)
	}
}

func TestValidateRejectsUnsupportedCommunityEditionHierarchy(t *testing.T) {
	config := Config{
		Version: 1,
		YouTrack: YouTrack{
			Query:  "project: APP",
			Fields: YouTrackFields{Kind: "Type", Status: "State", Priority: "Priority"},
			Hierarchy: []HierarchyLevel{
				{Role: "feature", Types: []string{"Feature"}},
				{Role: "story", Types: []string{"User Story"}},
			},
		},
		GitLab: GitLab{Targets: map[string]string{"feature": "issue", "story": "issue"}},
	}

	if err := config.Validate(); err == nil || !strings.Contains(err.Error(), "assigned more than once") {
		t.Fatalf("error = %v, want duplicate target rejection", err)
	}
}
