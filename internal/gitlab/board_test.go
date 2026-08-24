package gitlab

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestListBoardsReadsLabelAndNonLabelLists(t *testing.T) {
	var arguments []string
	client := &Client{output: func(_ context.Context, args ...string) ([]byte, error) {
		arguments = append([]string(nil), args...)
		return []byte(`[{"id":3,"name":"Development","lists":[{"id":8,"label":{"name":"status::Open"}},{"id":9,"assignee":{"id":4},"label":null}]}]`), nil
	}}

	boards, err := client.ListBoards(context.Background(), 42)
	if err != nil {
		t.Fatal(err)
	}
	wantArguments := []string{"api", "--paginate", "projects/42/boards?per_page=100"}
	if !reflect.DeepEqual(arguments, wantArguments) {
		t.Fatalf("glab arguments = %q, want %q", arguments, wantArguments)
	}
	want := []Board{{ID: 3, Name: "Development", Lists: []BoardList{{ID: 8, LabelName: "status::Open"}, {ID: 9}}}}
	if !reflect.DeepEqual(boards, want) {
		t.Fatalf("boards = %#v, want %#v", boards, want)
	}
}

func TestListBoardsFailsWithoutPartialResult(t *testing.T) {
	client := &Client{output: func(context.Context, ...string) ([]byte, error) {
		return []byte(`[{"id":3,"name":"Development","lists":[]}]`), errors.New("later page failed")
	}}

	boards, err := client.ListBoards(context.Background(), 42)
	if err == nil || !strings.Contains(err.Error(), "later page failed") {
		t.Fatalf("error = %v, want pagination failure", err)
	}
	if boards != nil {
		t.Fatalf("boards = %#v, want nil", boards)
	}
}

func TestParseBoardRejectsIncompleteSchema(t *testing.T) {
	for _, data := range []string{
		`{"name":"Development","lists":[]}`,
		`{"id":3,"lists":[]}`,
		`{"id":3,"name":"Development"}`,
		`{"id":3,"name":"Development","lists":[{"id":8,"label":{"name":""}}]}`,
	} {
		if _, err := parseBoard([]byte(data)); err == nil {
			t.Fatalf("incomplete board %s was accepted", data)
		}
	}
}
