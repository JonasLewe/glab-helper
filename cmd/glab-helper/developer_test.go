package main

import (
	"reflect"
	"testing"

	"github.com/JonasLewe/glab-helper/internal/projectconfig"
	"github.com/JonasLewe/glab-helper/internal/syncplan"
)

func TestProjectConfigForMilestoneScopeIgnoresOtherTargets(t *testing.T) {
	config := projectconfig.Config{
		Version: 1,
		YouTrack: projectconfig.YouTrack{
			Query:  "project: APP",
			Fields: projectconfig.YouTrackFields{Kind: "Type", Status: "State", Priority: "Priority"},
			Hierarchy: []projectconfig.HierarchyLevel{
				{Role: "epic", Types: []string{"Epic"}},
				{Role: "story", Types: []string{"User Story"}},
			},
		},
		GitLab: projectconfig.GitLab{Targets: map[string]string{"epic": "milestone", "story": "issue"}},
	}

	got, err := projectConfigForScope(config, syncMilestones)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"epic": "milestone", "story": "ignore"}
	if !reflect.DeepEqual(got.GitLab.Targets, want) {
		t.Fatalf("targets = %#v, want %#v", got.GitLab.Targets, want)
	}
	if config.GitLab.Targets["story"] != "issue" {
		t.Fatal("the original project configuration was mutated")
	}
}

func TestPlanForSingleIssueKeepsOnlyItsDependencies(t *testing.T) {
	plan := syncplan.Plan{
		References: map[string]syncplan.Reference{"existing": {Target: syncplan.Milestone, ID: "8"}},
		Actions: []syncplan.Action{
			{Operation: syncplan.Create, Desired: syncplan.DesiredItem{Target: syncplan.Label, Title: "backend"}},
			{Operation: syncplan.Create, Desired: syncplan.DesiredItem{Target: syncplan.Label, Title: "frontend"}},
			{Operation: syncplan.Create, Desired: syncplan.DesiredItem{Target: syncplan.BoardList, Title: "backend", BoardID: 3, BoardName: "Development"}},
			{Operation: syncplan.Create, Desired: syncplan.DesiredItem{Target: syncplan.BoardList, Title: "frontend", BoardID: 3, BoardName: "Development"}},
			{Operation: syncplan.Create, Desired: syncplan.DesiredItem{Target: syncplan.Milestone, SourceID: "APP-1", Title: "Release"}},
			{Operation: syncplan.Create, Desired: syncplan.DesiredItem{Target: syncplan.Milestone, SourceID: "APP-9", Title: "Other"}},
			{Operation: syncplan.Create, Desired: syncplan.DesiredItem{Target: syncplan.Issue, SourceID: "APP-2", ParentSourceID: "APP-1", Title: "Selected", Labels: []string{"backend"}}},
			{Operation: syncplan.Create, Desired: syncplan.DesiredItem{Target: syncplan.Issue, SourceID: "APP-10", ParentSourceID: "APP-9", Title: "Other", Labels: []string{"frontend"}}},
		},
	}

	got, err := planForSingleIssue(plan, "APP-2")
	if err != nil {
		t.Fatal(err)
	}
	var identities []string
	for _, action := range got.Actions {
		identities = append(identities, string(action.Desired.Target)+":"+action.Desired.Title)
	}
	want := []string{"label:backend", "board list:backend", "milestone:Release", "issue:Selected"}
	if !reflect.DeepEqual(identities, want) {
		t.Fatalf("actions = %#v, want %#v", identities, want)
	}
	if !reflect.DeepEqual(got.References, plan.References) {
		t.Fatalf("references = %#v", got.References)
	}
}

func TestValidHexColor(t *testing.T) {
	for _, value := range []string{"#000000", "#E44D2E", "#abcdef"} {
		if !validHexColor(value) {
			t.Fatalf("valid color %q was rejected", value)
		}
	}
	for _, value := range []string{"", "E44D2E", "#abcd", "#GG0000"} {
		if validHexColor(value) {
			t.Fatalf("invalid color %q was accepted", value)
		}
	}
}
