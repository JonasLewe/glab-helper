package syncplan

import (
	"bytes"
	"strings"
	"testing"
)

func TestWritePreviewIsReadOnlyAndShowsRelationships(t *testing.T) {
	plan := Plan{
		Actions: []Action{
			{Operation: Create, Desired: DesiredItem{Target: Label, Title: "team-a"}},
			{Operation: Create, Desired: DesiredItem{Target: Task, SourceID: "APP-3", Title: "[APP-3] Implement", ParentSourceID: "APP-2", Labels: []string{"team-a"}}},
			{Operation: Update, Desired: DesiredItem{Target: Issue, SourceID: "APP-2", Title: "[APP-2] API"}, CurrentIID: 7, Changes: []Change{{Field: "description", From: "old", To: "new"}, {Field: "state", From: "opened", To: "closed"}}},
		},
		Unchanged: 1,
		Ignored:   2,
	}
	var output bytes.Buffer
	WritePreview(&output, "group/project", plan)

	for _, want := range []string{
		"YouTrack synchronization preview for group/project (read-only)",
		`CREATE label "team-a"`,
		`CREATE task APP-3 from YouTrack APP-3: "[APP-3] Implement" under APP-2 [labels: team-a]`,
		`UPDATE issue #7 from YouTrack APP-2: "[APP-2] API"`,
		"description: changed",
		`state: "opened" -> "closed"`,
		"Summary: 2 to create, 1 to update, 1 unchanged, 2 ignored source items.",
		"No GitLab or YouTrack changes were applied.",
	} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("output %q does not contain %q", output.String(), want)
		}
	}
}
