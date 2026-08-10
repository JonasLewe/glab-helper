package syncplan

import (
	"reflect"
	"strings"
	"testing"

	"github.com/JonasLewe/glab-helper/internal/gitlab"
	"github.com/JonasLewe/glab-helper/internal/projectconfig"
	"github.com/JonasLewe/glab-helper/internal/source"
)

func fullHierarchyConfig() projectconfig.Config {
	return projectconfig.Config{
		Version: 1,
		YouTrack: projectconfig.YouTrack{
			Query:  "project: APP",
			Fields: projectconfig.YouTrackFields{Kind: "Type", Status: "State", Priority: "Priority"},
			Hierarchy: []projectconfig.HierarchyLevel{
				{Role: "epic", Types: []string{"Epic"}},
				{Role: "feature", Types: []string{"Feature"}},
				{Role: "story", Types: []string{"User Story"}},
			},
		},
		GitLab: projectconfig.GitLab{Targets: map[string]string{"epic": "milestone", "feature": "issue", "story": "task"}},
	}
}

func fullHierarchySnapshot() source.Snapshot {
	return source.Snapshot{WorkItems: []source.WorkItem{
		{ID: "APP-3", Title: "Implement", Description: "Story details", Kind: "User Story", Role: "story", Status: "Open", Priority: "Minor", Tags: []string{"backend"}, ParentID: "APP-2"},
		{ID: "APP-1", Title: "Platform", Description: "Epic details", Kind: "Epic", Role: "epic"},
		{ID: "APP-2", Title: "API", Description: "Feature details", Kind: "Feature", Role: "feature", Status: "In Progress", Priority: "Major", Tags: []string{"backend"}, ParentID: "APP-1"},
	}}
}

func TestBuildCreatesDeterministicCombinedPlanAndAdoptsLegacyItems(t *testing.T) {
	current := Current{
		Milestones: []gitlab.Milestone{{ID: 5, Title: "Platform", Description: "Epic details", State: "active"}},
		Issues: []gitlab.Issue{{
			IID: 7, Title: "[APP-2] Old API", Description: "Old", Labels: []string{"manual", "prio::Old", "status::Open"}, State: "opened",
		}},
		Labels: []gitlab.Label{{Name: "backend"}, {Name: "manual"}, {Name: "status::Open"}},
	}

	plan, err := Build(fullHierarchySnapshot(), fullHierarchyConfig(), current)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Ignored != 0 || plan.Unchanged != 0 {
		t.Fatalf("summary = ignored %d, unchanged %d", plan.Ignored, plan.Unchanged)
	}
	wantTargets := []Target{Label, Label, Label, Milestone, Issue, Task}
	wantOperations := []Operation{Create, Create, Create, Update, Update, Create}
	if len(plan.Actions) != len(wantTargets) {
		t.Fatalf("actions = %#v, want %d", plan.Actions, len(wantTargets))
	}
	for index, action := range plan.Actions {
		if action.Desired.Target != wantTargets[index] || action.Operation != wantOperations[index] {
			t.Fatalf("action %d = %s %s, want %s %s", index, action.Operation, action.Desired.Target, wantOperations[index], wantTargets[index])
		}
	}
	if labels := []string{plan.Actions[0].Desired.Title, plan.Actions[1].Desired.Title, plan.Actions[2].Desired.Title}; !reflect.DeepEqual(labels, []string{"prio::Major", "prio::Minor", "status::In Progress"}) {
		t.Fatalf("label actions = %q", labels)
	}
	milestone := plan.Actions[3]
	if milestone.CurrentID != "5" || len(milestone.Changes) != 1 || milestone.Changes[0].Field != "description" {
		t.Fatalf("milestone action = %#v", milestone)
	}
	issue := plan.Actions[4]
	wantIssueFields := []string{"title", "description", "labels", "milestone"}
	var issueFields []string
	for _, change := range issue.Changes {
		issueFields = append(issueFields, change.Field)
	}
	if !reflect.DeepEqual(issueFields, wantIssueFields) {
		t.Fatalf("issue changes = %q, want %q", issueFields, wantIssueFields)
	}
	if !reflect.DeepEqual(issue.Desired.Labels, []string{"backend", "manual", "prio::Major", "status::In Progress"}) {
		t.Fatalf("desired issue labels = %q", issue.Desired.Labels)
	}
	task := plan.Actions[5]
	if task.Desired.SourceID != "APP-3" || task.Desired.ParentSourceID != "APP-2" || !strings.Contains(task.Desired.Description, "glab-helper:youtrack:APP-3") {
		t.Fatalf("task action = %#v", task)
	}
}

func TestBuildCompressesIgnoredSourceHierarchy(t *testing.T) {
	config := fullHierarchyConfig()
	config.GitLab.Targets = map[string]string{"epic": "ignore", "feature": "milestone", "story": "issue"}

	plan, err := Build(fullHierarchySnapshot(), config, Current{})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Ignored != 1 {
		t.Fatalf("ignored = %d, want 1", plan.Ignored)
	}
	var itemActions []Action
	for _, action := range plan.Actions {
		if action.Desired.Target != Label {
			itemActions = append(itemActions, action)
		}
	}
	if len(itemActions) != 2 || itemActions[0].Desired.Target != Milestone || itemActions[1].Desired.Target != Issue {
		t.Fatalf("item actions = %#v", itemActions)
	}
	if itemActions[0].Desired.SourceID != "APP-2" || itemActions[0].Desired.ParentSourceID != "" {
		t.Fatalf("milestone = %#v", itemActions[0].Desired)
	}
	if itemActions[1].Desired.ParentSourceID != "APP-2" || itemActions[1].Desired.ParentTitle != "API" {
		t.Fatalf("issue = %#v", itemActions[1].Desired)
	}
}

func TestBuildRecognizesUnchangedTaskParent(t *testing.T) {
	snapshot := fullHierarchySnapshot()
	current := Current{
		Milestones: []gitlab.Milestone{{ID: 5, Title: "Platform", Description: withSourceMarker("Epic details", "APP-1"), State: "active"}},
		Issues: []gitlab.Issue{{
			IID: 7, Title: "[APP-2] API", Description: withSourceMarker("Feature details", "APP-2"), Labels: []string{"backend", "prio::Major", "status::In Progress"}, Milestone: &gitlab.IssueMilestone{Title: "Platform"}, State: "opened",
		}},
		Tasks: []gitlab.Task{{
			ID: "gid://gitlab/WorkItem/9", IID: 9, Title: "[APP-3] Implement", Description: withSourceMarker("Story details", "APP-3"), Labels: []string{"backend", "prio::Minor", "status::Open"}, ParentIID: 7, State: "OPEN",
		}},
		Labels: []gitlab.Label{{Name: "backend"}, {Name: "prio::Major"}, {Name: "prio::Minor"}, {Name: "status::In Progress"}, {Name: "status::Open"}},
	}

	plan, err := Build(snapshot, fullHierarchyConfig(), current)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Actions) != 0 || plan.Unchanged != 3 {
		t.Fatalf("plan = %#v, want three unchanged items", plan)
	}
}

func TestBuildRejectsLegacyIdentityAtWrongConfiguredTarget(t *testing.T) {
	current := Current{Issues: []gitlab.Issue{{IID: 4, Title: "[APP-3] Implement", Description: "Old Jira issue", Labels: []string{}, State: "opened"}}}

	plan, err := Build(fullHierarchySnapshot(), fullHierarchyConfig(), current)
	if err == nil || !strings.Contains(err.Error(), "legacy identity") || !strings.Contains(err.Error(), "targets task") {
		t.Fatalf("error = %v, want wrong-target legacy identity", err)
	}
	if len(plan.Actions) != 0 {
		t.Fatalf("plan = %#v, want no partial plan", plan)
	}
}
