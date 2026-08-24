package gitlab

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestListMembersReadsEveryPage(t *testing.T) {
	var command []string
	client := &Client{output: func(_ context.Context, args ...string) ([]byte, error) {
		command = append([]string(nil), args...)
		return []byte("[{\"id\":5,\"username\":\"alex\",\"name\":\"Alex Example\"}]\n[{\"id\":6,\"username\":\"sam\",\"name\":\"Sam Example\"}]\n"), nil
	}}

	members, err := client.ListMembers(context.Background(), 42)
	if err != nil {
		t.Fatal(err)
	}
	want := []Member{{ID: 5, Username: "alex", Name: "Alex Example"}, {ID: 6, Username: "sam", Name: "Sam Example"}}
	if !reflect.DeepEqual(members, want) {
		t.Fatalf("members = %#v, want %#v", members, want)
	}
	wantCommand := []string{"api", "--paginate", "projects/42/members/all?per_page=100"}
	if !reflect.DeepEqual(command, wantCommand) {
		t.Fatalf("command = %#v, want %#v", command, wantCommand)
	}
}

func TestListMembersRejectsIncompleteAndFailedResponses(t *testing.T) {
	tests := []struct {
		name   string
		output []byte
		err    error
		want   string
	}{
		{name: "missing username", output: []byte(`[{"id":5,"name":"Alex"}]`), want: `field "username"`},
		{name: "request failure", err: errors.New("forbidden"), want: "forbidden"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client := &Client{output: func(context.Context, ...string) ([]byte, error) {
				return test.output, test.err
			}}
			_, err := client.ListMembers(context.Background(), 42)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want containing %q", err, test.want)
			}
		})
	}
}
