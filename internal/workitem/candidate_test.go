package workitem

import (
	"reflect"
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
	branches := []gitrepo.RemoteBranch{{Name: "8-task-branch"}, {Name: "7-issue-branch"}, {Name: "7-older-branch"}, {Name: "main"}}

	got := OpenCandidates(issues, tasks, branches)
	want := []Candidate{
		{Kind: Task, IID: 8, Title: "Open task", ParentIID: 7, Branch: "8-task-branch"},
		{Kind: Issue, IID: 7, Title: "Open issue", Branch: "7-issue-branch"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("candidates = %#v, want %#v", got, want)
	}
	if display := got[0].Display(); display != "Task   #8  Open task  [parent: #7]  [branch: 8-task-branch]" {
		t.Fatalf("task display = %q", display)
	}
}
