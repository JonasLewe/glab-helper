package projectconfig

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

const DefaultPath = ".glab-helper.json"

var ErrNotFound = errors.New("glab-helper project configuration not found")

type Config struct {
	Version  int      `json:"version"`
	YouTrack YouTrack `json:"youtrack"`
	GitLab   GitLab   `json:"gitlab"`
}

type YouTrack struct {
	Query     string           `json:"query"`
	Fields    YouTrackFields   `json:"fields"`
	Hierarchy []HierarchyLevel `json:"hierarchy"`
}

type YouTrackFields struct {
	Kind     string `json:"kind"`
	Status   string `json:"status"`
	Priority string `json:"priority"`
}

type HierarchyLevel struct {
	Role  string   `json:"role"`
	Types []string `json:"types"`
}

type GitLab struct {
	Targets map[string]string `json:"targets"`
}

func Load(path string) (Config, error) {
	if path == "" {
		path = DefaultPath
	}

	file, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Config{}, fmt.Errorf("%w: %s", ErrNotFound, path)
		}
		return Config{}, fmt.Errorf("open glab-helper project configuration: %w", err)
	}
	defer file.Close()

	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	var config Config
	if err := decoder.Decode(&config); err != nil {
		return Config{}, fmt.Errorf("decode glab-helper project configuration: %w", err)
	}
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return Config{}, fmt.Errorf("decode glab-helper project configuration: unexpected trailing JSON value")
		}
		return Config{}, fmt.Errorf("decode glab-helper project configuration: %w", err)
	}
	if err := config.Validate(); err != nil {
		return Config{}, fmt.Errorf("validate glab-helper project configuration: %w", err)
	}
	return config, nil
}

func (config Config) Validate() error {
	if config.Version != 1 {
		return fmt.Errorf("version must be 1")
	}
	if strings.TrimSpace(config.YouTrack.Query) == "" {
		return fmt.Errorf("youtrack.query must not be empty")
	}

	fieldNames := []struct {
		key   string
		value string
	}{
		{key: "kind", value: config.YouTrack.Fields.Kind},
		{key: "status", value: config.YouTrack.Fields.Status},
		{key: "priority", value: config.YouTrack.Fields.Priority},
	}
	seenFieldNames := make(map[string]string, len(fieldNames))
	for _, field := range fieldNames {
		if strings.TrimSpace(field.value) == "" {
			return fmt.Errorf("youtrack.fields.%s must not be empty", field.key)
		}
		if field.value != strings.TrimSpace(field.value) {
			return fmt.Errorf("youtrack.fields.%s must not contain surrounding whitespace", field.key)
		}
		normalized := strings.ToLower(field.value)
		if previous, exists := seenFieldNames[normalized]; exists {
			return fmt.Errorf("youtrack fields %s and %s must use different names", previous, field.key)
		}
		seenFieldNames[normalized] = field.key
	}

	if len(config.YouTrack.Hierarchy) == 0 {
		return fmt.Errorf("youtrack.hierarchy must contain at least one level")
	}
	seenRoles := make(map[string]struct{}, len(config.YouTrack.Hierarchy))
	seenTypes := make(map[string]string)
	for index, level := range config.YouTrack.Hierarchy {
		if strings.TrimSpace(level.Role) == "" || level.Role != strings.TrimSpace(level.Role) {
			return fmt.Errorf("youtrack.hierarchy level %d has invalid role %q", index+1, level.Role)
		}
		if _, exists := seenRoles[level.Role]; exists {
			return fmt.Errorf("youtrack.hierarchy contains duplicate role %q", level.Role)
		}
		seenRoles[level.Role] = struct{}{}
		if len(level.Types) == 0 {
			return fmt.Errorf("youtrack.hierarchy role %q must contain at least one type", level.Role)
		}
		for _, issueType := range level.Types {
			if strings.TrimSpace(issueType) == "" {
				return fmt.Errorf("youtrack.hierarchy role %q contains an empty type", level.Role)
			}
			if issueType != strings.TrimSpace(issueType) {
				return fmt.Errorf("YouTrack type %q must not contain surrounding whitespace", issueType)
			}
			normalized := strings.ToLower(issueType)
			if previous, exists := seenTypes[normalized]; exists {
				return fmt.Errorf("YouTrack type %q is assigned to both %q and %q", issueType, previous, level.Role)
			}
			seenTypes[normalized] = level.Role
		}
	}

	targetSequence := make([]string, 0, len(config.YouTrack.Hierarchy))
	for _, level := range config.YouTrack.Hierarchy {
		target, exists := config.GitLab.Targets[level.Role]
		if !exists {
			return fmt.Errorf("gitlab.targets is missing role %q", level.Role)
		}
		if target != "ignore" && target != "milestone" && target != "issue" && target != "task" {
			return fmt.Errorf("gitlab.targets role %q has unsupported Community Edition target %q", level.Role, target)
		}
		if target == "ignore" {
			continue
		}
		targetSequence = append(targetSequence, target)
	}
	for role := range config.GitLab.Targets {
		if _, exists := seenRoles[role]; !exists {
			return fmt.Errorf("gitlab.targets contains unknown role %q", role)
		}
	}
	if len(targetSequence) == 0 {
		return fmt.Errorf("gitlab.targets must synchronize at least one hierarchy role")
	}
	if err := validateCommunityEditionHierarchy(targetSequence); err != nil {
		return err
	}

	return nil
}

func (config Config) RoleForKind(kind string) (string, bool) {
	for _, level := range config.YouTrack.Hierarchy {
		for _, issueType := range level.Types {
			if strings.EqualFold(kind, issueType) {
				return level.Role, true
			}
		}
	}
	return "", false
}

func (config Config) TargetForRole(role string) (string, bool) {
	target, found := config.GitLab.Targets[role]
	return target, found
}

func validateCommunityEditionHierarchy(targets []string) error {
	switch strings.Join(targets, "/") {
	case "milestone", "issue", "milestone/issue", "issue/task", "milestone/issue/task":
		return nil
	default:
		return fmt.Errorf("unsupported GitLab Community Edition hierarchy %q", strings.Join(targets, " -> "))
	}
}
