package git

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestListRemoteBranches(t *testing.T) {
	var arguments []string
	client := &Client{output: func(_ context.Context, args ...string) ([]byte, error) {
		arguments = append([]string(nil), args...)
		return []byte("feature/new\x00\nHEAD\x00refs/remotes/origin/main\nmain\x00\n"), nil
	}}

	branches, err := client.ListRemoteBranches(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	wantArguments := []string{
		"for-each-ref",
		"--sort=-committerdate",
		"--format=" + remoteBranchFormat,
		"refs/remotes/origin/",
	}
	if !reflect.DeepEqual(arguments, wantArguments) {
		t.Fatalf("git arguments = %q, want %q", arguments, wantArguments)
	}
	wantBranches := []RemoteBranch{{Name: "feature/new"}, {Name: "main"}}
	if !reflect.DeepEqual(branches, wantBranches) {
		t.Fatalf("branches = %+v, want %+v", branches, wantBranches)
	}
}

func TestListRemoteBranchesFailsWithoutPartialResult(t *testing.T) {
	tests := []struct {
		name   string
		output string
		err    error
		want   string
	}{
		{
			name:   "git fails after output",
			output: "main\x00\n",
			err:    errors.New("cannot read refs"),
			want:   "cannot read refs",
		},
		{
			name:   "later line is invalid",
			output: "main\x00\ninvalid\n",
			want:   "line 2",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client := &Client{output: func(context.Context, ...string) ([]byte, error) {
				return []byte(test.output), test.err
			}}

			branches, err := client.ListRemoteBranches(context.Background())
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want error containing %q", err, test.want)
			}
			if branches != nil {
				t.Fatalf("branches = %+v, want nil", branches)
			}
		})
	}
}
