package syncapply

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"

	"github.com/JonasLewe/glab-helper/internal/gitlab"
	"github.com/JonasLewe/glab-helper/internal/syncplan"
)

type fakeClient struct {
	calls  []string
	failOn string
}

func (client *fakeClient) record(call string) error {
	client.calls = append(client.calls, call)
	if call == client.failOn {
		return errors.New("write failed")
	}
	return nil
}

func (client *fakeClient) CreateLabel(_ context.Context, _ int64, name string) error {
	return client.record("label:" + name)
}

func (client *fakeClient) CreateMilestone(_ context.Context, _ int64, title, _ string, close bool) (gitlab.Milestone, error) {
	if err := client.record(fmt.Sprintf("milestone:%s:%t", title, close)); err != nil {
		return gitlab.Milestone{}, err
	}
	return gitlab.Milestone{ID: 11, Title: title}, nil
}

func (client *fakeClient) UpdateMilestone(_ context.Context, _, milestoneID int64, _, _ string, close bool) error {
	return client.record(fmt.Sprintf("update-milestone:%d:%t", milestoneID, close))
}

func (client *fakeClient) CreateIssue(_ context.Context, _ int64, title, _ string, _ []string, milestoneID *int64, close bool) (gitlab.CreatedIssue, error) {
	milestone := int64(0)
	if milestoneID != nil {
		milestone = *milestoneID
	}
	if err := client.record(fmt.Sprintf("issue:%s:%d:%t", title, milestone, close)); err != nil {
		return gitlab.CreatedIssue{}, err
	}
	return gitlab.CreatedIssue{IID: 22}, nil
}

func (client *fakeClient) UpdateIssue(_ context.Context, _, issueIID int64, _ string, _ string, _ []string, _ *int64, close bool) error {
	return client.record(fmt.Sprintf("update-issue:%d:%t", issueIID, close))
}

func (client *fakeClient) WorkItemID(_ context.Context, _ string, iid int64, expectedType string) (string, error) {
	call := fmt.Sprintf("work-item-id:%s:%d", expectedType, iid)
	if err := client.record(call); err != nil {
		return "", err
	}
	return fmt.Sprintf("gid://gitlab/WorkItem/%d", iid), nil
}

func (client *fakeClient) TaskTypeID(context.Context, string) (string, error) {
	if err := client.record("task-type"); err != nil {
		return "", err
	}
	return "gid://gitlab/WorkItems::Type/5", nil
}

func (client *fakeClient) CreateTask(_ context.Context, _, _, parentID, title, _ string) (gitlab.WorkItemReference, error) {
	if err := client.record("task:" + title + ":" + parentID); err != nil {
		return gitlab.WorkItemReference{}, err
	}
	return gitlab.WorkItemReference{ID: "gid://gitlab/WorkItem/33", IID: 33}, nil
}

func (client *fakeClient) SetWorkItemParent(_ context.Context, taskID, parentID string) error {
	return client.record("parent:" + taskID + ":" + parentID)
}

func TestApplyCreatesHierarchyInDependencyOrder(t *testing.T) {
	plan := syncplan.Plan{Actions: []syncplan.Action{
		{Operation: syncplan.Create, Desired: syncplan.DesiredItem{Target: syncplan.Label, Title: "team-a"}},
		{Operation: syncplan.Create, Desired: syncplan.DesiredItem{Target: syncplan.Milestone, SourceID: "APP-1", Title: "Platform", Description: "Epic"}},
		{Operation: syncplan.Create, Desired: syncplan.DesiredItem{Target: syncplan.Issue, SourceID: "APP-2", Title: "[APP-2] API", Description: "Feature", ParentSourceID: "APP-1"}},
		{Operation: syncplan.Create, Desired: syncplan.DesiredItem{Target: syncplan.Task, SourceID: "APP-3", Title: "[APP-3] Implement", Description: "Story", Labels: []string{"team-a"}, ParentSourceID: "APP-2", Resolved: true}},
	}}
	client := &fakeClient{}

	result, err := Apply(context.Background(), client, 42, "group/project", plan)
	if err != nil {
		t.Fatal(err)
	}
	if result.Applied != 4 || result.Total != 4 {
		t.Fatalf("result = %#v", result)
	}
	wantCalls := []string{
		"label:team-a",
		"milestone:Platform:false",
		"issue:[APP-2] API:11:false",
		"work-item-id:Issue:22",
		"task-type",
		"task:[APP-3] Implement:gid://gitlab/WorkItem/22",
		"update-issue:33:true",
	}
	if !reflect.DeepEqual(client.calls, wantCalls) {
		t.Fatalf("calls = %q, want %q", client.calls, wantCalls)
	}
}

func TestApplyUpdatesOnlyChangedTaskParent(t *testing.T) {
	plan := syncplan.Plan{
		References: map[string]syncplan.Reference{
			"APP-1": {Target: syncplan.Issue, ID: "1", IID: 1},
			"APP-2": {Target: syncplan.Issue, ID: "2", IID: 2},
			"APP-3": {Target: syncplan.Task, ID: "gid://gitlab/WorkItem/3", IID: 3},
		},
		Actions: []syncplan.Action{{
			Operation: syncplan.Update,
			Desired:   syncplan.DesiredItem{Target: syncplan.Task, SourceID: "APP-3", Title: "[APP-3] Task", ParentSourceID: "APP-2"},
			CurrentID: "gid://gitlab/WorkItem/3", CurrentIID: 3,
			Changes: []syncplan.Change{{Field: "parent", From: "#1", To: "APP-2"}},
		}},
	}
	client := &fakeClient{}

	if _, err := Apply(context.Background(), client, 42, "group/project", plan); err != nil {
		t.Fatal(err)
	}
	want := []string{"work-item-id:Issue:2", "parent:gid://gitlab/WorkItem/3:gid://gitlab/WorkItem/2"}
	if !reflect.DeepEqual(client.calls, want) {
		t.Fatalf("calls = %q, want %q", client.calls, want)
	}
}

func TestApplyReportsCompletedActionsAfterPartialFailure(t *testing.T) {
	plan := syncplan.Plan{Actions: []syncplan.Action{
		{Operation: syncplan.Create, Desired: syncplan.DesiredItem{Target: syncplan.Label, Title: "team-a"}},
		{Operation: syncplan.Create, Desired: syncplan.DesiredItem{Target: syncplan.Issue, SourceID: "APP-2", Title: "[APP-2] API"}},
	}}
	client := &fakeClient{failOn: "issue:[APP-2] API:0:false"}

	result, err := Apply(context.Background(), client, 42, "group/project", plan)
	if err == nil || result.Applied != 1 || result.Total != 2 {
		t.Fatalf("result/error = %#v, %v", result, err)
	}
}

func TestApplyValidatesEntirePlanBeforeWrites(t *testing.T) {
	plan := syncplan.Plan{Actions: []syncplan.Action{{
		Operation: syncplan.Create,
		Desired:   syncplan.DesiredItem{Target: syncplan.Task, SourceID: "APP-3", Title: "Task", ParentSourceID: "missing"},
	}}}
	client := &fakeClient{}

	if _, err := Apply(context.Background(), client, 42, "group/project", plan); err == nil {
		t.Fatal("invalid plan was accepted")
	}
	if len(client.calls) != 0 {
		t.Fatalf("writes occurred before validation: %q", client.calls)
	}
}
