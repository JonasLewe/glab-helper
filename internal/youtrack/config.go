package youtrack

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/JonasLewe/glab-helper/internal/gitlab"
)

const (
	variableURL           = "YOUTRACK_URL"
	variableToken         = "YOUTRACK_TOKEN"
	variableQuery         = "YOUTRACK_QUERY"
	variableTargetProject = "YOUTRACK_TARGET_PROJECT"
)

type Config struct {
	URL                 string
	Query               string
	TargetProject       string
	VariableProjectPath string
	token               string
}

type projectVariableReader func(context.Context, string, string) (string, error)

func LoadConfig(ctx context.Context, client *gitlab.Client, currentProjectPath, variableProjectPath string, skipTargetCheck bool) (Config, error) {
	return loadConfig(ctx, client.ProjectVariable, currentProjectPath, variableProjectPath, skipTargetCheck)
}

func loadConfig(ctx context.Context, readVariable projectVariableReader, currentProjectPath, variableProjectPath string, skipTargetCheck bool) (Config, error) {
	if currentProjectPath == "" {
		return Config{}, fmt.Errorf("current GitLab project path must not be empty")
	}
	if variableProjectPath == "" {
		variableProjectPath = url.PathEscape(currentProjectPath)
	}
	if strings.TrimSpace(variableProjectPath) == "" {
		return Config{}, fmt.Errorf("YouTrack variable project path must not be empty")
	}

	readRequired := func(key string) (string, error) {
		value, err := readVariable(ctx, variableProjectPath, key)
		if err != nil || strings.TrimSpace(value) == "" {
			return "", fmt.Errorf("cannot read required YouTrack configuration variable %s", key)
		}
		return value, nil
	}

	youTrackURL, err := readRequired(variableURL)
	if err != nil {
		return Config{}, err
	}
	youTrackURL, err = normalizeBaseURL(youTrackURL)
	if err != nil {
		return Config{}, fmt.Errorf("invalid YouTrack base URL")
	}
	query, err := readRequired(variableQuery)
	if err != nil {
		return Config{}, err
	}
	token, err := readRequired(variableToken)
	if err != nil {
		return Config{}, err
	}
	targetProject, err := readRequired(variableTargetProject)
	if err != nil {
		return Config{}, err
	}

	if !skipTargetCheck && currentProjectPath != targetProject {
		return Config{}, fmt.Errorf("YouTrack target project does not match the current GitLab project")
	}

	return Config{
		URL:                 youTrackURL,
		Query:               query,
		TargetProject:       targetProject,
		VariableProjectPath: variableProjectPath,
		token:               token,
	}, nil
}

func normalizeBaseURL(value string) (string, error) {
	value = strings.TrimSpace(value)
	parsed, err := url.Parse(value)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return "", fmt.Errorf("expected an absolute HTTP(S) URL")
	}
	if parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", fmt.Errorf("URL must not contain credentials, query parameters, or a fragment")
	}
	return strings.TrimRight(value, "/"), nil
}
