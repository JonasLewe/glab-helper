package gitlab

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestIssueEditWritesUseFocusedFields(t *testing.T) {
	var commands [][]string
	client := &Client{output: func(_ context.Context, args ...string) ([]byte, error) {
		commands = append(commands, append([]string(nil), args...))
		return []byte(`{}`), nil
	}}

	memberID := int64(5)
	milestoneID := int64(8)
	operations := []func() error{
		func() error { return client.UpdateIssueDescription(context.Background(), 42, 7, "New details") },
		func() error {
			return client.SetIssueLabels(context.Background(), 42, 7, []string{"backend", "status::Open"})
		},
		func() error { return client.SetIssueAssignee(context.Background(), 42, 7, &memberID) },
		func() error { return client.SetIssueAssignee(context.Background(), 42, 7, nil) },
		func() error { return client.SetIssueMilestone(context.Background(), 42, 7, &milestoneID) },
		func() error { return client.SetIssueMilestone(context.Background(), 42, 7, nil) },
		func() error { return client.CloseIssue(context.Background(), 42, 7) },
	}
	for _, operation := range operations {
		if err := operation(); err != nil {
			t.Fatal(err)
		}
	}

	want := [][]string{
		{"api", "projects/42/issues/7", "-X", "PUT", "-f", "description=New details"},
		{"api", "projects/42/issues/7", "-X", "PUT", "-f", "labels=backend,status::Open"},
		{"api", "projects/42/issues/7", "-X", "PUT", "-f", "assignee_id=5"},
		{"api", "projects/42/issues/7", "-X", "PUT", "-f", "assignee_id=0"},
		{"api", "projects/42/issues/7", "-X", "PUT", "-f", "milestone_id=8"},
		{"api", "projects/42/issues/7", "-X", "PUT", "-f", "milestone_id=0"},
		{"api", "projects/42/issues/7", "-X", "PUT", "-f", "state_event=close"},
	}
	if !reflect.DeepEqual(commands, want) {
		t.Fatalf("commands = %#v, want %#v", commands, want)
	}
}

func TestIssueEditWriteFailureHasContext(t *testing.T) {
	client := &Client{output: func(context.Context, ...string) ([]byte, error) {
		return nil, errors.New("forbidden")
	}}
	err := client.CloseIssue(context.Background(), 42, 7)
	if err == nil || !strings.Contains(err.Error(), "issue #7 state") || !strings.Contains(err.Error(), "forbidden") {
		t.Fatalf("error = %v", err)
	}
}
