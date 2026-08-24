package gitlab

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestDeletePlanningResourcesUsesExplicitIdentifiers(t *testing.T) {
	var commands [][]string
	client := &Client{output: func(_ context.Context, args ...string) ([]byte, error) {
		commands = append(commands, append([]string(nil), args...))
		return nil, nil
	}}

	if err := client.DeleteIssue(context.Background(), 42, 7); err != nil {
		t.Fatal(err)
	}
	if err := client.DeleteMilestone(context.Background(), 42, 8); err != nil {
		t.Fatal(err)
	}
	want := [][]string{
		{"api", "projects/42/issues/7", "-X", "DELETE"},
		{"api", "projects/42/milestones/8", "-X", "DELETE"},
	}
	if !reflect.DeepEqual(commands, want) {
		t.Fatalf("commands = %#v, want %#v", commands, want)
	}
}

func TestDeletePlanningResourceFailureHasContext(t *testing.T) {
	client := &Client{output: func(context.Context, ...string) ([]byte, error) {
		return nil, errors.New("forbidden")
	}}
	if err := client.DeleteIssue(context.Background(), 42, 7); err == nil || !strings.Contains(err.Error(), "issue #7") || !strings.Contains(err.Error(), "forbidden") {
		t.Fatalf("issue error = %v", err)
	}
	if err := client.DeleteMilestone(context.Background(), 42, 8); err == nil || !strings.Contains(err.Error(), "milestone 8") || !strings.Contains(err.Error(), "forbidden") {
		t.Fatalf("milestone error = %v", err)
	}
}
