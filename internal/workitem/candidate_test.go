package workitem

import (
	"reflect"
	"strings"
	"testing"

	gitrepo "github.com/JonasLewe/glab-helper/internal/git"
	"github.com/JonasLewe/glab-helper/internal/gitlab"
)

func TestOpenCandidatesCombinesIssuesTasksAndBranches(t *testing.T) {
	issues := []gitlab.Issue{
		{IID: 7, Title: "Open issue", State: "opened"},
		{IID: 9, Title: "Closed issue", State: "closed"},
	}
	tasks := []gitlab.Task{
		{IID: 8, Title: "Open task", State: "OPEN", ParentIID: 7},
		{IID: 6, Title: "Closed task", State: "CLOSED", ParentIID: 7},
	}
	branches := []gitrepo.Branch{{Name: "8-task-branch", Remote: true}, {Name: "7-issue-branch", Local: true}, {Name: "7-older-branch", Remote: true}, {Name: "007-not-issue", Remote: true}, {Name: "main", Local: true, Remote: true}}

	got := OpenCandidates(issues, tasks, branches)
	want := []Candidate{
		{Kind: Task, IID: 8, Title: "Open task", ParentIID: 7, Branch: gitrepo.Branch{Name: "8-task-branch", Remote: true}},
		{Kind: Issue, IID: 7, Title: "Open issue", Branch: gitrepo.Branch{Name: "7-issue-branch", Local: true}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("candidates = %#v, want %#v", got, want)
	}
	if display := got[0].Display(); display != "Task   #8  Open task  [parent: #7]  [branch: 8-task-branch]" {
		t.Fatalf("task display = %q", display)
	}
	if name := BranchName(42, "Über-long API / parser title"); name != "42-ber-long-api-parser-title" {
		t.Fatalf("branch name = %q", name)
	}
	if name := BranchName(7, "!!!"); name != "7-issue" {
		t.Fatalf("fallback branch name = %q", name)
	}
	if name := BranchName(7, strings.Repeat("a", 100)); len(name) != 60 || !strings.HasPrefix(name, "7-") {
		t.Fatalf("long branch name = %q", name)
	}
}
