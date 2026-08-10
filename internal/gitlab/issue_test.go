package gitlab

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestListIssues(t *testing.T) {
	var arguments []string
	client := &Client{output: func(_ context.Context, args ...string) ([]byte, error) {
		arguments = append([]string(nil), args...)
		return []byte(`[{"iid":1,"title":"First","description":null,"labels":[],"milestone":null,"state":"opened","assignees":[]}]` + "\n" +
			`[{"iid":2,"title":"Second","description":"Details","labels":["team-a"],"milestone":{"title":"Release"},"state":"closed","assignees":[{"username":"alex"}]}]`), nil
	}}

	issues, err := client.ListIssues(context.Background(), 42)
	if err != nil {
		t.Fatal(err)
	}
	wantArguments := []string{"api", "--paginate", "projects/42/issues?state=all&issue_type=issue&per_page=100"}
	if !reflect.DeepEqual(arguments, wantArguments) {
		t.Fatalf("glab arguments = %q, want %q", arguments, wantArguments)
	}
	if len(issues) != 2 {
		t.Fatalf("issue count = %d, want 2", len(issues))
	}
	if issues[0].Description != "" || issues[1].Milestone == nil || issues[1].Milestone.Title != "Release" {
		t.Fatalf("unexpected issues: %+v", issues)
	}
	if !reflect.DeepEqual(issues[1].Assignees, []string{"alex"}) {
		t.Fatalf("assignees = %q, want [alex]", issues[1].Assignees)
	}
}

func TestListIssuesFailsWithoutPartialResult(t *testing.T) {
	tests := []struct {
		name   string
		output string
		err    error
		want   string
	}{
		{
			name:   "request fails after output",
			output: `[{"iid":1,"title":"First","description":"","labels":[],"milestone":null,"state":"opened","assignees":[]}]`,
			err:    errors.New("later page failed"),
			want:   "later page failed",
		},
		{
			name: "later page has invalid schema",
			output: `[{"iid":1,"title":"First","description":"","labels":[],"milestone":null,"state":"opened","assignees":[]}]` + "\n" +
				`[{"iid":2,"title":false,"description":"","labels":[],"milestone":null,"state":"opened","assignees":[]}]`,
			want: "page 2 issue 1",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client := &Client{output: func(context.Context, ...string) ([]byte, error) {
				return []byte(test.output), test.err
			}}

			issues, err := client.ListIssues(context.Background(), 42)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want error containing %q", err, test.want)
			}
			if issues != nil {
				t.Fatalf("issues = %+v, want nil", issues)
			}
		})
	}
}
