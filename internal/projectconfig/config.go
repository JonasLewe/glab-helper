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
		if !validRole(level.Role) {
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

	if len(config.GitLab.Targets) != len(seenRoles) {
		return fmt.Errorf("gitlab.targets must contain exactly one target for every hierarchy role")
	}
	targetSequence := make([]string, 0, len(config.YouTrack.Hierarchy))
	seenTargets := make(map[string]struct{})
	for _, level := range config.YouTrack.Hierarchy {
		target, exists := config.GitLab.Targets[level.Role]
		if !exists {
			return fmt.Errorf("gitlab.targets is missing role %q", level.Role)
		}
		if target != "milestone" && target != "issue" && target != "task" {
			return fmt.Errorf("gitlab.targets role %q has unsupported Community Edition target %q", level.Role, target)
		}
		if _, exists := seenTargets[target]; exists {
			return fmt.Errorf("GitLab target %q is assigned more than once", target)
		}
		seenTargets[target] = struct{}{}
		targetSequence = append(targetSequence, target)
	}
	for role := range config.GitLab.Targets {
		if _, exists := seenRoles[role]; !exists {
			return fmt.Errorf("gitlab.targets contains unknown role %q", role)
		}
	}
	if err := validateCommunityEditionHierarchy(targetSequence); err != nil {
		return err
	}

	return nil
}

func (config Config) RoleForKind(kind string) (string, int, bool) {
	for index, level := range config.YouTrack.Hierarchy {
		for _, issueType := range level.Types {
			if strings.EqualFold(kind, issueType) {
				return level.Role, index, true
			}
		}
	}
	return "", 0, false
}

func validRole(role string) bool {
	if role == "" || role[0] < 'a' || role[0] > 'z' {
		return false
	}
	for _, character := range role[1:] {
		if (character < 'a' || character > 'z') && (character < '0' || character > '9') && character != '_' && character != '-' {
			return false
		}
	}
	return true
}

func validateCommunityEditionHierarchy(targets []string) error {
	for index, target := range targets {
		switch target {
		case "milestone":
			if index != 0 {
				return fmt.Errorf("GitLab milestone can only be the root hierarchy target")
			}
		case "issue":
			if index > 0 && targets[index-1] != "milestone" {
				return fmt.Errorf("GitLab issue can only be a root or follow a milestone in Community Edition")
			}
		case "task":
			if index == 0 || targets[index-1] != "issue" || index != len(targets)-1 {
				return fmt.Errorf("GitLab task must be the final level directly below an issue in Community Edition")
			}
		}
	}
	return nil
}
