package gitlab

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestListLabels(t *testing.T) {
	var arguments []string
	client := &Client{output: func(_ context.Context, args ...string) ([]byte, error) {
		arguments = append([]string(nil), args...)
		return []byte(`[{"id":10,"name":"bug","color":"#d9534f"}]` + "\n" +
			`[{"id":11,"name":"team-a","color":"#5843ad"}]`), nil
	}}

	labels, err := client.ListLabels(context.Background(), 42)
	if err != nil {
		t.Fatal(err)
	}
	wantArguments := []string{"api", "--paginate", "projects/42/labels?per_page=100"}
	if !reflect.DeepEqual(arguments, wantArguments) {
		t.Fatalf("glab arguments = %q, want %q", arguments, wantArguments)
	}
	wantLabels := []Label{{ID: 10, Name: "bug"}, {ID: 11, Name: "team-a"}}
	if !reflect.DeepEqual(labels, wantLabels) {
		t.Fatalf("labels = %+v, want %+v", labels, wantLabels)
	}
}

func TestListLabelsFailsWithoutPartialResult(t *testing.T) {
	tests := []struct {
		name   string
		output string
		err    error
		want   string
	}{
		{
			name:   "request fails after output",
			output: `[{"id":10,"name":"bug"}]`,
			err:    errors.New("later page failed"),
			want:   "later page failed",
		},
		{
			name:   "later page has invalid schema",
			output: `[{"id":10,"name":"bug"}]` + "\n" + `[{"id":11,"name":false}]`,
			want:   "page 2 label 1",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client := &Client{output: func(context.Context, ...string) ([]byte, error) {
				return []byte(test.output), test.err
			}}

			labels, err := client.ListLabels(context.Background(), 42)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want error containing %q", err, test.want)
			}
			if labels != nil {
				t.Fatalf("labels = %+v, want nil", labels)
			}
		})
	}
}
