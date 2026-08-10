package gitlab

import (
	"context"
	"reflect"
	"testing"
)

func TestProjectVariable(t *testing.T) {
	var arguments []string
	client := &Client{output: func(_ context.Context, args ...string) ([]byte, error) {
		arguments = append([]string(nil), args...)
		return []byte(`{"value":"configured"}`), nil
	}}

	value, err := client.ProjectVariable(context.Background(), "group%2Fconfig", "YOUTRACK_URL")
	if err != nil {
		t.Fatal(err)
	}
	if value != "configured" {
		t.Fatalf("value = %q, want configured", value)
	}
	wantArguments := []string{"api", "projects/group%2Fconfig/variables/YOUTRACK_URL"}
	if !reflect.DeepEqual(arguments, wantArguments) {
		t.Fatalf("glab arguments = %q, want %q", arguments, wantArguments)
	}
}

func TestProjectVariableRejectsMissingValue(t *testing.T) {
	client := &Client{output: func(context.Context, ...string) ([]byte, error) {
		return []byte(`{"value":""}`), nil
	}}

	if _, err := client.ProjectVariable(context.Background(), "group%2Fconfig", "YOUTRACK_TOKEN"); err == nil {
		t.Fatal("empty variable value was accepted")
	}
}
