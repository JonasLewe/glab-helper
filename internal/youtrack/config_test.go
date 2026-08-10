package youtrack

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestLoadConfig(t *testing.T) {
	values := map[string]string{
		variableURL:           "https://youtrack.example.com/",
		variableQuery:         "project: APP tag: gitlab-sync",
		variableToken:         "secret-token",
		variableTargetProject: "group/project",
	}
	var requests []string
	readVariable := func(_ context.Context, projectPath, key string) (string, error) {
		requests = append(requests, projectPath+":"+key)
		return values[key], nil
	}

	config, err := loadConfig(context.Background(), readVariable, "group/project", "", false)
	if err != nil {
		t.Fatal(err)
	}
	if config.URL != "https://youtrack.example.com" ||
		config.Query != "project: APP tag: gitlab-sync" ||
		config.TargetProject != "group/project" ||
		config.VariableProjectPath != "group%2Fproject" ||
		config.token != "secret-token" {
		t.Fatal("loaded YouTrack configuration does not match the expected values")
	}
	wantRequests := []string{
		"group%2Fproject:YOUTRACK_URL",
		"group%2Fproject:YOUTRACK_QUERY",
		"group%2Fproject:YOUTRACK_TOKEN",
		"group%2Fproject:YOUTRACK_TARGET_PROJECT",
	}
	if !reflect.DeepEqual(requests, wantRequests) {
		t.Fatalf("requests = %q, want %q", requests, wantRequests)
	}
}

func TestLoadConfigTargetGuard(t *testing.T) {
	values := map[string]string{
		variableURL:           "https://youtrack.example.com",
		variableQuery:         "project: APP",
		variableToken:         "secret-token",
		variableTargetProject: "other/project",
	}
	readVariable := func(_ context.Context, _, key string) (string, error) {
		return values[key], nil
	}

	if _, err := loadConfig(context.Background(), readVariable, "group/project", "group%2Fconfig", false); err == nil {
		t.Fatal("mismatched target project was accepted")
	}
	config, err := loadConfig(context.Background(), readVariable, "group/project", "group%2Fconfig", true)
	if err != nil {
		t.Fatal(err)
	}
	if config.VariableProjectPath != "group%2Fconfig" || config.TargetProject != "other/project" {
		t.Fatal("development mode did not preserve the explicit configuration project")
	}
}

func TestLoadConfigRejectsInvalidURLBeforeReadingToken(t *testing.T) {
	var requests []string
	readVariable := func(_ context.Context, _ string, key string) (string, error) {
		requests = append(requests, key)
		return "not-a-url", nil
	}

	_, err := loadConfig(context.Background(), readVariable, "group/project", "", false)
	if err == nil {
		t.Fatal("invalid YouTrack URL was accepted")
	}
	if !reflect.DeepEqual(requests, []string{variableURL}) {
		t.Fatalf("requests = %q, want only YOUTRACK_URL", requests)
	}
}

func TestLoadConfigDoesNotLeakTokenThroughErrors(t *testing.T) {
	const token = "secret-token"
	readVariable := func(_ context.Context, _ string, key string) (string, error) {
		switch key {
		case variableURL:
			return "https://youtrack.example.com", nil
		case variableQuery:
			return "project: APP", nil
		case variableToken:
			return token, nil
		default:
			return "", errors.New("server repeated " + token)
		}
	}

	_, err := loadConfig(context.Background(), readVariable, "group/project", "", false)
	if err == nil {
		t.Fatal("variable read failure was accepted")
	}
	if strings.Contains(err.Error(), token) {
		t.Fatalf("error leaked YouTrack token: %v", err)
	}
}
