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
	role, found := config.RoleForKind("user story")
	if !found || role != "story" {
		t.Fatalf("RoleForKind = %q, %t; want story, true", role, found)
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

	if err := config.Validate(); err == nil {
		t.Fatal("duplicate GitLab target was accepted")
	}
}

func TestValidateAllowsIgnoredSourceLevels(t *testing.T) {
	config := Config{
		Version: 1,
		YouTrack: YouTrack{
			Query:  "project: APP",
			Fields: YouTrackFields{Kind: "Type", Status: "State", Priority: "Priority"},
			Hierarchy: []HierarchyLevel{
				{Role: "epic", Types: []string{"Epic"}},
				{Role: "feature", Types: []string{"Feature"}},
				{Role: "story", Types: []string{"User Story"}},
			},
		},
		GitLab: GitLab{Targets: map[string]string{"epic": "ignore", "feature": "milestone", "story": "issue"}},
	}

	if err := config.Validate(); err != nil {
		t.Fatal(err)
	}
	if target, found := config.TargetForRole("epic"); !found || target != "ignore" {
		t.Fatalf("TargetForRole = %q, %t; want ignore, true", target, found)
	}
}

func TestValidateRejectsAllIgnoredSourceLevels(t *testing.T) {
	config := Config{
		Version: 1,
		YouTrack: YouTrack{
			Query:     "project: APP",
			Fields:    YouTrackFields{Kind: "Type", Status: "State", Priority: "Priority"},
			Hierarchy: []HierarchyLevel{{Role: "epic", Types: []string{"Epic"}}},
		},
		GitLab: GitLab{Targets: map[string]string{"epic": "ignore"}},
	}

	if err := config.Validate(); err == nil || !strings.Contains(err.Error(), "at least one") {
		t.Fatalf("error = %v, want all-ignored rejection", err)
	}
}
