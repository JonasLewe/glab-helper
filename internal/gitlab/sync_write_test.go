package gitlab

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestSyncRESTWritesUseExplicitFieldsAndForwardOnlyClose(t *testing.T) {
	var commands [][]string
	client := &Client{output: func(_ context.Context, args ...string) ([]byte, error) {
		commands = append(commands, append([]string(nil), args...))
		switch {
		case reflect.DeepEqual(args[:4], []string{"api", "projects/42/labels", "-X", "POST"}):
			return []byte(`{"id":4,"name":"status::Open"}`), nil
		case reflect.DeepEqual(args[:4], []string{"api", "projects/42/boards/3/lists", "-X", "POST"}):
			return []byte(`{"id":6,"label":{"name":"status::Open"}}`), nil
		case reflect.DeepEqual(args[:4], []string{"api", "projects/42/milestones", "-X", "POST"}):
			return []byte(`{"id":5,"title":"Release","description":"Details","state":"active"}`), nil
		case reflect.DeepEqual(args[:4], []string{"api", "projects/42/issues", "-X", "POST"}):
			return []byte(`{"iid":7}`), nil
		default:
			return []byte(`{}`), nil
		}
	}}

	label, err := client.CreateLabel(context.Background(), 42, "status::Open")
	if err != nil {
		t.Fatal(err)
	}
	if label.ID != 4 || label.Name != "status::Open" {
		t.Fatalf("label = %#v", label)
	}
	if err := client.CreateBoardList(context.Background(), 42, 3, label.ID, label.Name); err != nil {
		t.Fatal(err)
	}
	milestone, err := client.CreateMilestone(context.Background(), 42, "Release", "Details", true)
	if err != nil {
		t.Fatal(err)
	}
	if milestone.ID != 5 || milestone.State != "closed" {
		t.Fatalf("milestone = %#v", milestone)
	}
	milestoneID := int64(5)
	issue, err := client.CreateIssue(context.Background(), 42, "[APP-2] API", "Description", []string{"backend", "status::Open"}, &milestoneID, true)
	if err != nil {
		t.Fatal(err)
	}
	if issue.IID != 7 {
		t.Fatalf("issue = %#v", issue)
	}

	want := [][]string{
		{"api", "projects/42/labels", "-X", "POST", "-f", "name=status::Open", "-f", "color=#428BCA"},
		{"api", "projects/42/boards/3/lists", "-X", "POST", "-f", "label_id=4"},
		{"api", "projects/42/milestones", "-X", "POST", "-f", "title=Release", "-f", "description=Details"},
		{"api", "projects/42/milestones/5", "-X", "PUT", "-f", "title=Release", "-f", "description=Details", "-f", "state_event=close"},
		{"api", "projects/42/issues", "-X", "POST", "-f", "title=[APP-2] API", "-f", "description=Description", "-f", "labels=backend,status::Open", "-f", "issue_type=issue", "-f", "milestone_id=5"},
		{"api", "projects/42/issues/7", "-X", "PUT", "-f", "title=[APP-2] API", "-f", "description=Description", "-f", "labels=backend,status::Open", "-f", "milestone_id=5", "-f", "state_event=close"},
	}
	if !reflect.DeepEqual(commands, want) {
		t.Fatalf("commands = %#v, want %#v", commands, want)
	}
}

func TestSyncRESTWriteFailureHasContext(t *testing.T) {
	client := &Client{output: func(context.Context, ...string) ([]byte, error) {
		return nil, errors.New("forbidden")
	}}
	if _, err := client.CreateLabel(context.Background(), 42, "team-a"); err == nil || !strings.Contains(err.Error(), `label "team-a"`) || !strings.Contains(err.Error(), "forbidden") {
		t.Fatalf("error = %v", err)
	}
}

func TestWorkItemQueriesAndMutationsValidateResponses(t *testing.T) {
	var commands [][]string
	client := &Client{output: func(_ context.Context, args ...string) ([]byte, error) {
		commands = append(commands, append([]string(nil), args...))
		joined := strings.Join(args, " ")
		switch {
		case strings.Contains(joined, "query ProjectWorkItem"):
			return []byte(`{"data":{"namespace":{"workItem":{"id":"gid://gitlab/WorkItem/20","iid":"7","workItemType":{"name":"Issue"}}}}}`), nil
		case strings.Contains(joined, "query ProjectTaskType"):
			return []byte(`{"data":{"namespace":{"workItemTypes":{"nodes":[{"id":"gid://gitlab/WorkItems::Type/5","name":"Task"}]}}}}`), nil
		case strings.Contains(joined, "mutation CreateProjectTask"):
			return []byte(`{"data":{"workItemCreate":{"workItem":{"id":"gid://gitlab/WorkItem/30","iid":"9","workItemType":{"name":"Task"}},"errors":[]}}}`), nil
		case strings.Contains(joined, "mutation SetWorkItemParent"):
			return []byte(`{"data":{"workItemUpdate":{"workItem":{"id":"gid://gitlab/WorkItem/30"},"errors":[]}}}`), nil
		default:
			return nil, errors.New("unexpected command")
		}
	}}

	issueID, err := client.WorkItemID(context.Background(), "group/project", 7, "Issue")
	if err != nil || issueID != "gid://gitlab/WorkItem/20" {
		t.Fatalf("WorkItemID = %q, %v", issueID, err)
	}
	taskTypeID, err := client.TaskTypeID(context.Background(), "group/project")
	if err != nil || taskTypeID != "gid://gitlab/WorkItems::Type/5" {
		t.Fatalf("TaskTypeID = %q, %v", taskTypeID, err)
	}
	task, err := client.CreateTask(context.Background(), "group/project", taskTypeID, issueID, "[APP-3] Task", "Details")
	if err != nil || task.ID != "gid://gitlab/WorkItem/30" || task.IID != 9 {
		t.Fatalf("CreateTask = %#v, %v", task, err)
	}
	if err := client.SetWorkItemParent(context.Background(), task.ID, issueID); err != nil {
		t.Fatal(err)
	}
	if len(commands) != 4 {
		t.Fatalf("commands = %#v", commands)
	}
	createCommand := strings.Join(commands[2], " ")
	for _, want := range []string{`"namespacePath":"group/project"`, `"workItemTypeId":"gid://gitlab/WorkItems::Type/5"`, `"parentId":"gid://gitlab/WorkItem/20"`, `"description":"Details"`} {
		if !strings.Contains(createCommand, want) {
			t.Fatalf("create command %q does not contain %q", createCommand, want)
		}
	}
}

func TestWorkItemMutationRejectsPayloadErrors(t *testing.T) {
	client := &Client{output: func(context.Context, ...string) ([]byte, error) {
		return []byte(`{"data":{"workItemUpdate":{"workItem":null,"errors":["parent is invalid"]}}}`), nil
	}}

	err := client.SetWorkItemParent(context.Background(), "gid://gitlab/WorkItem/30", "gid://gitlab/WorkItem/20")
	if err == nil || !strings.Contains(err.Error(), "parent is invalid") {
		t.Fatalf("error = %v", err)
	}
}
