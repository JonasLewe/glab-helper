package gitlab

import "testing"

func TestParseProject(t *testing.T) {
	project, err := parseProject([]byte(`{"id":42,"path_with_namespace":"group/project","default_branch":"main"}`))
	if err != nil {
		t.Fatal(err)
	}
	if project.ID != 42 || project.Path != "group/project" || project.DefaultBranch != "main" {
		t.Fatalf("unexpected project: %+v", project)
	}

	if _, err := parseProject([]byte(`{"id":42}`)); err == nil {
		t.Fatal("missing project path was accepted")
	}
}
