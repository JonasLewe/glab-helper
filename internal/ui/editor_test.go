package ui

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEditTextUsesConfiguredEditor(t *testing.T) {
	directory := t.TempDir()
	editorPath := filepath.Join(directory, "editor")
	editor := "#!/bin/sh\nprintf '%s' 'updated description' >\"$1\"\n"
	if err := os.WriteFile(editorPath, []byte(editor), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("VISUAL", editorPath)
	t.Setenv("EDITOR", "")

	var stdout, stderr bytes.Buffer
	updated, err := EditText(context.Background(), "initial", strings.NewReader(""), &stdout, &stderr)
	if err != nil {
		t.Fatal(err)
	}
	if updated != "updated description" {
		t.Fatalf("updated description = %q", updated)
	}
}

func TestEditTextReportsEditorFailure(t *testing.T) {
	directory := t.TempDir()
	editorPath := filepath.Join(directory, "editor")
	if err := os.WriteFile(editorPath, []byte("#!/bin/sh\nexit 9\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("VISUAL", editorPath)
	t.Setenv("EDITOR", "")

	_, err := EditText(context.Background(), "initial", strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "exit status 9") {
		t.Fatalf("error = %v", err)
	}
}
