package youtrack

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/JonasLewe/glab-helper/internal/projectconfig"
	"github.com/JonasLewe/glab-helper/internal/source"
)

const (
	issuePageSize = 100
	issueFields   = "idReadable,summary,description,resolved,tags(name),customFields(name,value(name)),parent(issues(idReadable))"
)

var snapshotHTTPClient = &http.Client{Timeout: 30 * time.Second}

type httpDoer interface {
	Do(*http.Request) (*http.Response, error)
}

type issueJSON struct {
	IDReadable   *string            `json:"idReadable"`
	Summary      *string            `json:"summary"`
	Description  json.RawMessage    `json:"description"`
	Resolved     json.RawMessage    `json:"resolved"`
	Tags         *[]namedJSON       `json:"tags"`
	CustomFields *[]customFieldJSON `json:"customFields"`
	Parent       json.RawMessage    `json:"parent"`
}

type namedJSON struct {
	Name *string `json:"name"`
}

type customFieldJSON struct {
	Name  *string         `json:"name"`
	Value json.RawMessage `json:"value"`
}

type parentJSON struct {
	Issues *[]struct {
		IDReadable *string `json:"idReadable"`
	} `json:"issues"`
}

func ReadSnapshot(ctx context.Context, connection Config, project projectconfig.Config) (source.Snapshot, error) {
	return readSnapshotWithClient(ctx, snapshotHTTPClient, connection, project, issuePageSize)
}

func readSnapshotWithClient(ctx context.Context, client httpDoer, connection Config, project projectconfig.Config, pageSize int) (source.Snapshot, error) {
	snapshot, err := readSnapshot(ctx, client, connection, project, pageSize)
	if err == nil {
		return snapshot, nil
	}

	message := err.Error()
	if connection.token != "" {
		message = strings.ReplaceAll(message, connection.token, "<redacted>")
	}
	return source.Snapshot{}, fmt.Errorf("read YouTrack source snapshot: %s", message)
}

func readSnapshot(ctx context.Context, client httpDoer, connection Config, project projectconfig.Config, pageSize int) (source.Snapshot, error) {
	if strings.TrimSpace(connection.URL) == "" || strings.TrimSpace(connection.token) == "" {
		return source.Snapshot{}, fmt.Errorf("incomplete YouTrack configuration")
	}
	if pageSize < 1 {
		return source.Snapshot{}, fmt.Errorf("page size must be positive")
	}

	endpoint, err := url.Parse(connection.URL + "/api/issues")
	if err != nil {
		return source.Snapshot{}, fmt.Errorf("build YouTrack issues endpoint: %w", err)
	}

	items := make([]source.WorkItem, 0)
	seenIDs := make(map[string]struct{})
	for skip := 0; ; skip += pageSize {
		page, err := readIssuePage(ctx, client, endpoint, connection, project, pageSize, skip)
		if err != nil {
			return source.Snapshot{}, fmt.Errorf("page starting at %d: %w", skip, err)
		}
		if len(page) > pageSize {
			return source.Snapshot{}, fmt.Errorf("page starting at %d returned %d issues, limit is %d", skip, len(page), pageSize)
		}

		for index, rawIssue := range page {
			item, err := parseIssue(rawIssue, project)
			if err != nil {
				return source.Snapshot{}, fmt.Errorf("page starting at %d issue %d: %w", skip, index+1, err)
			}
			if _, exists := seenIDs[item.ID]; exists {
				return source.Snapshot{}, fmt.Errorf("duplicate YouTrack issue %q across pages", item.ID)
			}
			seenIDs[item.ID] = struct{}{}
			items = append(items, item)
		}

		if len(page) < pageSize {
			if err := validateSnapshotHierarchy(items, project); err != nil {
				return source.Snapshot{}, err
			}
			return source.Snapshot{WorkItems: items}, nil
		}
	}
}

func readIssuePage(ctx context.Context, client httpDoer, endpoint *url.URL, connection Config, project projectconfig.Config, pageSize, skip int) ([]json.RawMessage, error) {
	requestURL := *endpoint
	query := requestURL.Query()
	query.Set("fields", issueFields)
	query.Set("query", project.YouTrack.Query)
	query.Set("$top", strconv.Itoa(pageSize))
	query.Set("$skip", strconv.Itoa(skip))
	for _, field := range []string{project.YouTrack.Fields.Kind, project.YouTrack.Fields.Status, project.YouTrack.Fields.Priority} {
		query.Add("customFields", field)
	}
	requestURL.RawQuery = query.Encode()

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("build issue request: %w", err)
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Authorization", "Bearer "+connection.token)

	response, err := client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("send issue request: %w", err)
	}
	defer response.Body.Close()

	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("issue request returned %s", response.Status)
	}

	decoder := json.NewDecoder(response.Body)
	var page []json.RawMessage
	if err := decoder.Decode(&page); err != nil {
		return nil, fmt.Errorf("decode issue response: %w", err)
	}
	if page == nil {
		return nil, fmt.Errorf("decode issue response: expected an array")
	}
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("decode issue response: unexpected trailing JSON value")
		}
		return nil, fmt.Errorf("decode issue response: %w", err)
	}
	return page, nil
}

func parseIssue(data []byte, project projectconfig.Config) (source.WorkItem, error) {
	var value issueJSON
	if err := json.Unmarshal(data, &value); err != nil {
		return source.WorkItem{}, err
	}
	if value.IDReadable == nil || strings.TrimSpace(*value.IDReadable) == "" {
		return source.WorkItem{}, fmt.Errorf("missing or invalid field %q", "idReadable")
	}
	if value.Summary == nil || strings.TrimSpace(*value.Summary) == "" {
		return source.WorkItem{}, fmt.Errorf("missing or invalid field %q", "summary")
	}
	if value.Description == nil {
		return source.WorkItem{}, fmt.Errorf("missing field %q", "description")
	}
	if value.Resolved == nil {
		return source.WorkItem{}, fmt.Errorf("missing field %q", "resolved")
	}
	if value.Tags == nil {
		return source.WorkItem{}, fmt.Errorf("field %q must be an array", "tags")
	}
	if value.CustomFields == nil {
		return source.WorkItem{}, fmt.Errorf("field %q must be an array", "customFields")
	}
	if value.Parent == nil {
		return source.WorkItem{}, fmt.Errorf("missing field %q", "parent")
	}

	item := source.WorkItem{
		ID:    *value.IDReadable,
		Title: *value.Summary,
		Tags:  make([]string, 0, len(*value.Tags)),
	}
	if !bytes.Equal(value.Description, []byte("null")) {
		if err := json.Unmarshal(value.Description, &item.Description); err != nil {
			return source.WorkItem{}, fmt.Errorf("field %q: %w", "description", err)
		}
	}
	if !bytes.Equal(value.Resolved, []byte("null")) {
		var resolvedAt int64
		if err := json.Unmarshal(value.Resolved, &resolvedAt); err != nil || resolvedAt < 1 {
			return source.WorkItem{}, fmt.Errorf("field %q must be null or a positive timestamp", "resolved")
		}
		item.Resolved = true
	}

	for index, tag := range *value.Tags {
		if tag.Name == nil || strings.TrimSpace(*tag.Name) == "" {
			return source.WorkItem{}, fmt.Errorf("field %q item %d has no valid name", "tags", index+1)
		}
		item.Tags = append(item.Tags, *tag.Name)
	}
	sort.Strings(item.Tags)

	for index, field := range *value.CustomFields {
		if field.Name == nil || strings.TrimSpace(*field.Name) == "" {
			return source.WorkItem{}, fmt.Errorf("field %q item %d has no valid name", "customFields", index+1)
		}
		fieldRole := ""
		switch {
		case strings.EqualFold(*field.Name, project.YouTrack.Fields.Kind):
			fieldRole = "kind"
		case strings.EqualFold(*field.Name, project.YouTrack.Fields.Status):
			fieldRole = "status"
		case strings.EqualFold(*field.Name, project.YouTrack.Fields.Priority):
			fieldRole = "priority"
		default:
			continue
		}
		fieldValue, err := parseNamedFieldValue(field.Value)
		if err != nil {
			return source.WorkItem{}, fmt.Errorf("custom field %q: %w", *field.Name, err)
		}
		switch fieldRole {
		case "kind":
			item.Kind = fieldValue
		case "status":
			item.Status = fieldValue
		case "priority":
			item.Priority = fieldValue
		}
	}
	role, found := project.RoleForKind(item.Kind)
	if !found {
		return source.WorkItem{}, fmt.Errorf("YouTrack issue %q custom field %q value %q is not assigned to a configured hierarchy role", item.ID, project.YouTrack.Fields.Kind, item.Kind)
	}
	item.Role = role

	if !bytes.Equal(value.Parent, []byte("null")) {
		var parent parentJSON
		if err := json.Unmarshal(value.Parent, &parent); err != nil {
			return source.WorkItem{}, fmt.Errorf("YouTrack issue %q field %q: %w", item.ID, "parent", err)
		}
		if parent.Issues == nil {
			return source.WorkItem{}, fmt.Errorf("YouTrack issue %q (%s) field %q must contain exactly one valid issue; the issues collection is missing", item.ID, item.Kind, "parent")
		}
		if len(*parent.Issues) != 1 {
			parentIDs := make([]string, 0, len(*parent.Issues))
			for _, parentIssue := range *parent.Issues {
				if parentIssue.IDReadable == nil || strings.TrimSpace(*parentIssue.IDReadable) == "" {
					parentIDs = append(parentIDs, "<invalid>")
					continue
				}
				parentIDs = append(parentIDs, *parentIssue.IDReadable)
			}
			return source.WorkItem{}, fmt.Errorf("YouTrack issue %q (%s) field %q must contain exactly one valid issue; got %d: %s", item.ID, item.Kind, "parent", len(*parent.Issues), strings.Join(parentIDs, ", "))
		}
		if (*parent.Issues)[0].IDReadable == nil || strings.TrimSpace(*(*parent.Issues)[0].IDReadable) == "" {
			return source.WorkItem{}, fmt.Errorf("YouTrack issue %q (%s) field %q contains an invalid issue ID", item.ID, item.Kind, "parent")
		}
		item.ParentID = *(*parent.Issues)[0].IDReadable
	}

	return item, nil
}

func validateSnapshotHierarchy(items []source.WorkItem, project projectconfig.Config) error {
	itemsByID := make(map[string]source.WorkItem, len(items))
	roleLevels := make(map[string]int, len(project.YouTrack.Hierarchy))
	for index, level := range project.YouTrack.Hierarchy {
		roleLevels[level.Role] = index
	}
	for _, item := range items {
		itemsByID[item.ID] = item
	}
	for _, item := range items {
		level := roleLevels[item.Role]
		if level == 0 {
			if item.ParentID != "" {
				return fmt.Errorf("YouTrack issue %q with root role %q unexpectedly has parent %q", item.ID, item.Role, item.ParentID)
			}
			continue
		}
		if item.ParentID == "" {
			return fmt.Errorf("YouTrack issue %q with role %q has no parent", item.ID, item.Role)
		}
		parent, exists := itemsByID[item.ParentID]
		if !exists {
			return fmt.Errorf("YouTrack issue %q references parent %q outside the configured query", item.ID, item.ParentID)
		}
		expectedRole := project.YouTrack.Hierarchy[level-1].Role
		if parent.Role != expectedRole {
			return fmt.Errorf("YouTrack issue %q with role %q requires parent role %q, got %q", item.ID, item.Role, expectedRole, parent.Role)
		}
	}
	return nil
}

func parseNamedFieldValue(data []byte) (string, error) {
	data = bytes.TrimSpace(data)
	if len(data) == 0 {
		return "", fmt.Errorf("missing value")
	}
	if bytes.Equal(data, []byte("null")) {
		return "", nil
	}
	if data[0] == '[' {
		var values []namedJSON
		if err := json.Unmarshal(data, &values); err != nil {
			return "", fmt.Errorf("expected named values: %w", err)
		}
		if len(values) == 0 {
			return "", nil
		}
		if len(values) > 1 {
			return "", fmt.Errorf("expected at most one named value")
		}
		if values[0].Name == nil || strings.TrimSpace(*values[0].Name) == "" {
			return "", fmt.Errorf("expected a non-empty name")
		}
		return *values[0].Name, nil
	}
	var value namedJSON
	if err := json.Unmarshal(data, &value); err != nil {
		return "", fmt.Errorf("expected a single named value: %w", err)
	}
	if value.Name == nil || strings.TrimSpace(*value.Name) == "" {
		return "", fmt.Errorf("expected a non-empty name")
	}
	return *value.Name, nil
}
