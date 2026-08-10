package gitlab

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestListMilestones(t *testing.T) {
	var arguments []string
	client := &Client{output: func(_ context.Context, args ...string) ([]byte, error) {
		arguments = append([]string(nil), args...)
		return []byte(`[{"id":10,"title":"First","description":null,"state":"active"}]` + "\n" +
			`[{"id":11,"title":"Second","description":"Details","state":"closed"}]`), nil
	}}

	milestones, err := client.ListMilestones(context.Background(), 42)
	if err != nil {
		t.Fatal(err)
	}
	wantArguments := []string{"api", "--paginate", "projects/42/milestones?per_page=100"}
	if !reflect.DeepEqual(arguments, wantArguments) {
		t.Fatalf("glab arguments = %q, want %q", arguments, wantArguments)
	}
	wantMilestones := []Milestone{
		{ID: 10, Title: "First", State: "active"},
		{ID: 11, Title: "Second", Description: "Details", State: "closed"},
	}
	if !reflect.DeepEqual(milestones, wantMilestones) {
		t.Fatalf("milestones = %+v, want %+v", milestones, wantMilestones)
	}
}

func TestListMilestonesFailsWithoutPartialResult(t *testing.T) {
	tests := []struct {
		name   string
		output string
		err    error
		want   string
	}{
		{
			name:   "request fails after output",
			output: `[{"id":10,"title":"First","description":"","state":"active"}]`,
			err:    errors.New("later page failed"),
			want:   "later page failed",
		},
		{
			name: "later page has invalid schema",
			output: `[{"id":10,"title":"First","description":"","state":"active"}]` + "\n" +
				`[{"id":11,"title":false,"description":"","state":"closed"}]`,
			want: "page 2 milestone 1",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client := &Client{output: func(context.Context, ...string) ([]byte, error) {
				return []byte(test.output), test.err
			}}

			milestones, err := client.ListMilestones(context.Background(), 42)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want error containing %q", err, test.want)
			}
			if milestones != nil {
				t.Fatalf("milestones = %+v, want nil", milestones)
			}
		})
	}
}
