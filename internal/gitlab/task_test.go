package gitlab

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestListTasksPaginatesGraphQLWithoutPartialResult(t *testing.T) {
	var commands [][]string
	client := &Client{output: func(_ context.Context, args ...string) ([]byte, error) {
		commands = append(commands, append([]string(nil), args...))
		if len(commands) == 1 {
			return []byte(`{"data":{"namespace":{"workItems":{"nodes":[{"id":"gid://gitlab/WorkItem/10","iid":"7","title":"Task","description":"Details","state":"OPEN","workItemType":{"name":"Task"},"widgets":[{"__typename":"WorkItemWidgetHierarchy","parent":{"id":"gid://gitlab/WorkItem/3","iid":"4"}},{"__typename":"WorkItemWidgetLabels","labels":{"nodes":[{"title":"team-a"}],"pageInfo":{"hasNextPage":false}}}]}],"pageInfo":{"hasNextPage":true,"endCursor":"cursor-1"}}}}}`), nil
		}
		return []byte(`{"data":{"namespace":{"workItems":{"nodes":[{"id":"gid://gitlab/WorkItem/11","iid":8,"title":"Done","description":null,"state":"CLOSED","workItemType":{"name":"Task"},"widgets":[{"__typename":"WorkItemWidgetHierarchy","parent":null},{"__typename":"WorkItemWidgetLabels","labels":{"nodes":[],"pageInfo":{"hasNextPage":false}}}]}],"pageInfo":{"hasNextPage":false,"endCursor":null}}}}}`), nil
	}}

	tasks, err := client.ListTasks(context.Background(), "group/project")
	if err != nil {
		t.Fatal(err)
	}
	want := []Task{
		{ID: "gid://gitlab/WorkItem/10", IID: 7, Title: "Task", Description: "Details", State: "OPEN", Labels: []string{"team-a"}, ParentIID: 4},
		{ID: "gid://gitlab/WorkItem/11", IID: 8, Title: "Done", State: "CLOSED", Labels: []string{}},
	}
	if !reflect.DeepEqual(tasks, want) {
		t.Fatalf("tasks = %#v, want %#v", tasks, want)
	}
	if len(commands) != 2 || !reflect.DeepEqual(commands[1][len(commands[1])-2:], []string{"-f", "after=cursor-1"}) {
		t.Fatalf("commands = %#v, want second request with cursor", commands)
	}
	for _, command := range commands {
		joined := strings.Join(command, " ")
		if !strings.Contains(joined, "api graphql") || !strings.Contains(joined, "fullPath=group/project") || !strings.Contains(joined, "workItems(types: [TASK]") {
			t.Fatalf("unexpected GraphQL command: %q", joined)
		}
	}
}

func TestListTasksRejectsGraphQLErrors(t *testing.T) {
	client := &Client{output: func(context.Context, ...string) ([]byte, error) {
		return []byte(`{"data":{"namespace":null},"errors":[{"message":"Tasks unavailable"}]}`), nil
	}}

	tasks, err := client.ListTasks(context.Background(), "group/project")
	if err == nil || !strings.Contains(err.Error(), "Tasks unavailable") {
		t.Fatalf("error = %v, want GraphQL error", err)
	}
	if tasks != nil {
		t.Fatalf("tasks = %#v, want no partial result", tasks)
	}
}

func TestParseTaskRejectsTruncatedLabels(t *testing.T) {
	data := []byte(`{"data":{"namespace":{"workItems":{"nodes":[{"id":"gid://gitlab/WorkItem/10","iid":"7","title":"Task","description":null,"state":"OPEN","workItemType":{"name":"Task"},"widgets":[{"__typename":"WorkItemWidgetHierarchy","parent":null},{"__typename":"WorkItemWidgetLabels","labels":{"nodes":[],"pageInfo":{"hasNextPage":true}}}]}],"pageInfo":{"hasNextPage":false,"endCursor":null}}}}}`)

	_, _, err := parseTaskPage(data)
	if err == nil || !strings.Contains(err.Error(), "more than 100 labels") {
		t.Fatalf("error = %v, want truncated-label rejection", err)
	}
}

func TestListTasksDropsEarlierPagesOnFailure(t *testing.T) {
	requests := 0
	client := &Client{output: func(context.Context, ...string) ([]byte, error) {
		requests++
		if requests == 1 {
			return []byte(`{"data":{"namespace":{"workItems":{"nodes":[],"pageInfo":{"hasNextPage":true,"endCursor":"cursor-1"}}}}}`), nil
		}
		return nil, errors.New("later page failed")
	}}

	tasks, err := client.ListTasks(context.Background(), "group/project")
	if err == nil || !strings.Contains(err.Error(), "later page failed") {
		t.Fatalf("error = %v, want later page failure", err)
	}
	if tasks != nil {
		t.Fatalf("tasks = %#v, want no partial result", tasks)
	}
}
