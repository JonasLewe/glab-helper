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

	"github.com/JonasLewe/glab-helper/internal/source"
)

const (
	issuePageSize = 100
	issueFields   = "id,idReadable,summary,description,resolved,tags(name),customFields(name,value(name)),parent(issues(idReadable))"
)

var snapshotCustomFields = []string{"Type", "State", "Priority"}

var snapshotHTTPClient = &http.Client{Timeout: 30 * time.Second}

type httpDoer interface {
	Do(*http.Request) (*http.Response, error)
}

type issueJSON struct {
	ID           *string            `json:"id"`
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

func ReadSnapshot(ctx context.Context, config Config) (source.Snapshot, error) {
	return readSnapshotWithClient(ctx, snapshotHTTPClient, config, issuePageSize)
}

func readSnapshotWithClient(ctx context.Context, client httpDoer, config Config, pageSize int) (source.Snapshot, error) {
	snapshot, err := readSnapshot(ctx, client, config, pageSize)
	if err == nil {
		return snapshot, nil
	}

	message := err.Error()
	if config.token != "" {
		message = strings.ReplaceAll(message, config.token, "<redacted>")
	}
	return source.Snapshot{}, fmt.Errorf("read YouTrack source snapshot: %s", message)
}

func readSnapshot(ctx context.Context, client httpDoer, config Config, pageSize int) (source.Snapshot, error) {
	if strings.TrimSpace(config.URL) == "" || strings.TrimSpace(config.Query) == "" || strings.TrimSpace(config.token) == "" {
		return source.Snapshot{}, fmt.Errorf("incomplete YouTrack configuration")
	}
	if pageSize < 1 {
		return source.Snapshot{}, fmt.Errorf("page size must be positive")
	}

	endpoint, err := url.Parse(config.URL + "/api/issues")
	if err != nil {
		return source.Snapshot{}, fmt.Errorf("build YouTrack issues endpoint: %w", err)
	}

	items := make([]source.WorkItem, 0)
	seenIDs := make(map[string]struct{})
	for skip := 0; ; skip += pageSize {
		page, err := readIssuePage(ctx, client, endpoint, config, pageSize, skip)
		if err != nil {
			return source.Snapshot{}, fmt.Errorf("page starting at %d: %w", skip, err)
		}
		if len(page) > pageSize {
			return source.Snapshot{}, fmt.Errorf("page starting at %d returned %d issues, limit is %d", skip, len(page), pageSize)
		}

		for index, rawIssue := range page {
			item, err := parseIssue(rawIssue)
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
			return source.Snapshot{WorkItems: items}, nil
		}
	}
}

func readIssuePage(ctx context.Context, client httpDoer, endpoint *url.URL, config Config, pageSize, skip int) ([]json.RawMessage, error) {
	requestURL := *endpoint
	query := requestURL.Query()
	query.Set("fields", issueFields)
	query.Set("query", config.Query)
	query.Set("$top", strconv.Itoa(pageSize))
	query.Set("$skip", strconv.Itoa(skip))
	for _, field := range snapshotCustomFields {
		query.Add("customFields", field)
	}
	requestURL.RawQuery = query.Encode()

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("build issue request: %w", err)
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Authorization", "Bearer "+config.token)

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

func parseIssue(data []byte) (source.WorkItem, error) {
	var value issueJSON
	if err := json.Unmarshal(data, &value); err != nil {
		return source.WorkItem{}, err
	}
	if value.ID == nil || strings.TrimSpace(*value.ID) == "" {
		return source.WorkItem{}, fmt.Errorf("missing or invalid field %q", "id")
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

	seenTags := make(map[string]struct{}, len(*value.Tags))
	for index, tag := range *value.Tags {
		if tag.Name == nil || strings.TrimSpace(*tag.Name) == "" {
			return source.WorkItem{}, fmt.Errorf("field %q item %d has no valid name", "tags", index+1)
		}
		if _, exists := seenTags[*tag.Name]; exists {
			return source.WorkItem{}, fmt.Errorf("field %q contains duplicate %q", "tags", *tag.Name)
		}
		seenTags[*tag.Name] = struct{}{}
		item.Tags = append(item.Tags, *tag.Name)
	}
	sort.Strings(item.Tags)

	seenFields := make(map[string]struct{})
	for index, field := range *value.CustomFields {
		if field.Name == nil || strings.TrimSpace(*field.Name) == "" {
			return source.WorkItem{}, fmt.Errorf("field %q item %d has no valid name", "customFields", index+1)
		}
		name := strings.ToLower(*field.Name)
		if name != "type" && name != "state" && name != "priority" {
			continue
		}
		if _, exists := seenFields[name]; exists {
			return source.WorkItem{}, fmt.Errorf("field %q contains duplicate %q", "customFields", *field.Name)
		}
		seenFields[name] = struct{}{}
		fieldValue, err := parseNamedFieldValue(field.Value)
		if err != nil {
			return source.WorkItem{}, fmt.Errorf("custom field %q: %w", *field.Name, err)
		}
		switch name {
		case "type":
			item.Kind = fieldValue
		case "state":
			item.Status = fieldValue
		case "priority":
			item.Priority = fieldValue
		}
	}

	if !bytes.Equal(value.Parent, []byte("null")) {
		var parent parentJSON
		if err := json.Unmarshal(value.Parent, &parent); err != nil {
			return source.WorkItem{}, fmt.Errorf("field %q: %w", "parent", err)
		}
		if parent.Issues == nil || len(*parent.Issues) != 1 || (*parent.Issues)[0].IDReadable == nil || strings.TrimSpace(*(*parent.Issues)[0].IDReadable) == "" {
			return source.WorkItem{}, fmt.Errorf("field %q must contain exactly one valid issue", "parent")
		}
		item.ParentID = *(*parent.Issues)[0].IDReadable
	}

	return item, nil
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
