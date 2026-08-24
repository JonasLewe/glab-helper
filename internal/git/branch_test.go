package git

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestManageBranchesUsesFreshValidatedRefs(t *testing.T) {
	var commands [][]string
	client := &Client{output: func(_ context.Context, args ...string) ([]byte, error) {
		commands = append(commands, append([]string(nil), args...))
		if args[0] == "for-each-ref" {
			return []byte("refs/remotes/origin/8-task\x00\nrefs/heads/main\x00\nrefs/remotes/origin/main\x00\nrefs/remotes/origin/HEAD\x00refs/remotes/origin/main\n"), nil
		}
		return nil, nil
	}}

	if err := client.PruneRemoteBranches(context.Background()); err != nil {
		t.Fatal(err)
	}
	branches, err := client.ListBranches(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	wantBranches := []Branch{
		{Name: "8-task", Remote: true},
		{Name: "main", Local: true, Remote: true},
	}
	if !reflect.DeepEqual(branches, wantBranches) {
		t.Fatalf("branches = %#v, want %#v", branches, wantBranches)
	}
	if err := client.CreateBranch(context.Background(), "9-new-task", branches[1]); err != nil {
		t.Fatal(err)
	}
	if err := client.CheckoutBranch(context.Background(), Branch{Name: "9-new-task", Local: true}); err != nil {
		t.Fatal(err)
	}
	if err := client.CheckoutBranch(context.Background(), branches[0]); err != nil {
		t.Fatal(err)
	}
	wantCommands := [][]string{
		{"fetch", "--prune", "origin", "--quiet"},
		{"for-each-ref", "--sort=-committerdate", "--format=" + branchFormat, "refs/heads/", "refs/remotes/origin/"},
		{"check-ref-format", "--branch", "9-new-task"},
		{"branch", "9-new-task", "origin/main"},
		{"checkout", "9-new-task"},
		{"checkout", "-b", "8-task", "origin/8-task"},
	}
	if !reflect.DeepEqual(commands, wantCommands) {
		t.Fatalf("git commands = %#v, want %#v", commands, wantCommands)
	}
}

func TestBranchMutationStopsOnValidationOrRefreshFailure(t *testing.T) {
	requests := 0
	client := &Client{output: func(_ context.Context, args ...string) ([]byte, error) {
		requests++
		return nil, errors.New("rejected " + args[0])
	}}

	if err := client.PruneRemoteBranches(context.Background()); err == nil || !strings.Contains(err.Error(), "refresh and prune") {
		t.Fatalf("prune error = %v", err)
	}
	if err := client.CreateBranch(context.Background(), "-invalid", Branch{Name: "main", Remote: true}); err == nil || !strings.Contains(err.Error(), "validate branch name") {
		t.Fatalf("create error = %v", err)
	}
	if requests != 2 {
		t.Fatalf("requests = %d, want 2 with no create after failed validation", requests)
	}
}
