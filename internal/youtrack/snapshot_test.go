package youtrack

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/JonasLewe/glab-helper/internal/projectconfig"
	"github.com/JonasLewe/glab-helper/internal/source"
)

type httpDoerFunc func(*http.Request) (*http.Response, error)

func (function httpDoerFunc) Do(request *http.Request) (*http.Response, error) {
	return function(request)
}

func testHTTPResponse(statusCode int, body string) *http.Response {
	return &http.Response{
		StatusCode: statusCode,
		Status:     fmt.Sprintf("%d %s", statusCode, http.StatusText(statusCode)),
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     make(http.Header),
	}
}

func testProjectConfig() projectconfig.Config {
	return projectconfig.Config{
		Version: 1,
		YouTrack: projectconfig.YouTrack{
			Query:  "project: APP tag: gitlab-sync",
			Fields: projectconfig.YouTrackFields{Kind: "Type", Status: "State", Priority: "Priority"},
			Hierarchy: []projectconfig.HierarchyLevel{
				{Role: "epic", Types: []string{"Epic"}},
				{Role: "feature", Types: []string{"Feature"}},
				{Role: "story", Types: []string{"User Story"}},
			},
		},
		GitLab: projectconfig.GitLab{Targets: map[string]string{"epic": "milestone", "feature": "issue", "story": "task"}},
	}
}

func TestReadSnapshotPaginatesIntoProviderNeutralWorkItems(t *testing.T) {
	const token = "secret-token"
	var skips []string
	client := httpDoerFunc(func(request *http.Request) (*http.Response, error) {
		if request.Method != http.MethodGet || request.URL.Path != "/api/issues" {
			t.Errorf("request = %s %s, want GET /api/issues", request.Method, request.URL.Path)
		}
		if authorization := request.Header.Get("Authorization"); authorization != "Bearer "+token {
			t.Errorf("Authorization = %q", authorization)
		}
		if accept := request.Header.Get("Accept"); accept != "application/json" {
			t.Errorf("Accept = %q", accept)
		}
		if request.Body != nil {
			t.Error("read-only issue request unexpectedly contains a body")
		}
		query := request.URL.Query()
		if query.Get("query") != "project: APP tag: gitlab-sync" || query.Get("fields") != issueFields || query.Get("$top") != "2" {
			t.Errorf("unexpected query: %q", query)
		}
		wantFields := []string{"Type", "State", "Priority"}
		if fields := query["customFields"]; !reflect.DeepEqual(fields, wantFields) {
			t.Errorf("customFields = %q, want %q", fields, wantFields)
		}
		skips = append(skips, query.Get("$skip"))

		switch query.Get("$skip") {
		case "0":
			return testHTTPResponse(http.StatusOK, `[
  {"id":"2-1","idReadable":"APP-1","summary":"Platform","description":"Epic details","resolved":null,"tags":[{"name":"sync"}],"customFields":[{"name":"Type","value":{"name":"Epic"}},{"name":"State","value":{"name":"Open"}},{"name":"Priority","value":{"name":"Major"}}],"parent":null},
  {"id":"2-2","idReadable":"APP-2","summary":"API","description":null,"resolved":null,"tags":[{"name":"backend"},{"name":"sync"}],"customFields":[{"name":"Type","value":{"name":"Feature"}},{"name":"State","value":{"name":"In Progress"}},{"name":"Priority","value":[{"name":"Minor"}]}],"parent":{"issues":[{"idReadable":"APP-1"}]}}
]`), nil
		case "2":
			return testHTTPResponse(http.StatusOK, `[{"id":"2-3","idReadable":"APP-3","summary":"Finished","description":"Done","resolved":1720000000000,"tags":[],"customFields":[{"name":"Type","value":{"name":"User Story"}}],"parent":{"issues":[{"idReadable":"APP-2"}]}}]`), nil
		default:
			return testHTTPResponse(http.StatusBadRequest, "unexpected page"), nil
		}
	})

	connection := Config{URL: "https://youtrack.example.com", token: token}
	snapshot, err := readSnapshotWithClient(context.Background(), client, connection, testProjectConfig(), 2)
	if err != nil {
		t.Fatal(err)
	}
	want := source.Snapshot{WorkItems: []source.WorkItem{
		{ID: "APP-1", Title: "Platform", Description: "Epic details", Kind: "Epic", Role: "epic", Status: "Open", Priority: "Major", Tags: []string{"sync"}},
		{ID: "APP-2", Title: "API", Kind: "Feature", Role: "feature", Status: "In Progress", Priority: "Minor", Tags: []string{"backend", "sync"}, ParentID: "APP-1"},
		{ID: "APP-3", Title: "Finished", Description: "Done", Kind: "User Story", Role: "story", Tags: []string{}, ParentID: "APP-2", Resolved: true},
	}}
	if !reflect.DeepEqual(snapshot, want) {
		t.Fatalf("snapshot = %#v, want %#v", snapshot, want)
	}
	if !reflect.DeepEqual(skips, []string{"0", "2"}) {
		t.Fatalf("pagination skips = %q, want [0 2]", skips)
	}
}

func TestReadSnapshotFailsWithoutPartialResultOrTokenLeak(t *testing.T) {
	const token = "secret-token"
	client := httpDoerFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.Query().Get("$skip") == "0" {
			return testHTTPResponse(http.StatusOK, `[{"id":"2-1","idReadable":"APP-1","summary":"First","description":null,"resolved":null,"tags":[],"customFields":[{"name":"Type","value":{"name":"Epic"}}],"parent":null}]`), nil
		}
		return nil, errors.New("transport repeated " + token)
	})

	connection := Config{URL: "https://youtrack.example.com", token: token}
	snapshot, err := readSnapshotWithClient(context.Background(), client, connection, testProjectConfig(), 1)
	if err == nil {
		t.Fatal("later page failure was accepted")
	}
	if snapshot.WorkItems != nil {
		t.Fatalf("snapshot = %#v, want no partial result", snapshot)
	}
	if strings.Contains(err.Error(), token) {
		t.Fatalf("error leaked YouTrack token: %v", err)
	}
}

func TestParseIssueRejectsIncompleteResponse(t *testing.T) {
	_, err := parseIssue([]byte(`{"id":"2-1","idReadable":"APP-1","summary":"Incomplete"}`), testProjectConfig())
	if err == nil || !strings.Contains(err.Error(), "description") {
		t.Fatalf("error = %v, want missing description", err)
	}
}

func TestParseIssueAcceptsEmptyParentIssueCollectionForRoot(t *testing.T) {
	data := []byte(`{"idReadable":"APP-1","summary":"Root","description":null,"resolved":null,"tags":[],"customFields":[{"name":"Type","value":{"name":"Epic"}}],"parent":{"issues":[]}}`)

	item, err := parseIssue(data, testProjectConfig())
	if err != nil {
		t.Fatal(err)
	}
	if item.ID != "APP-1" || item.Role != "epic" || item.ParentID != "" {
		t.Fatalf("item = %#v, want root Epic without parent", item)
	}
}

func TestParseIssueHierarchyErrorsIdentifyTheSourceIssue(t *testing.T) {
	project := testProjectConfig()
	project.YouTrack.Hierarchy = project.YouTrack.Hierarchy[:2]

	tests := []struct {
		name string
		data string
		want string
	}{
		{
			name: "unknown type",
			data: `{"idReadable":"APP-9","summary":"Unexpected","description":null,"resolved":null,"tags":[],"customFields":[{"name":"Type","value":{"name":"User Story"}}],"parent":null}`,
			want: `YouTrack issue "APP-9" custom field "Type" value "User Story"`,
		},
		{
			name: "multiple parents",
			data: `{"idReadable":"APP-10","summary":"Ambiguous","description":null,"resolved":null,"tags":[],"customFields":[{"name":"Type","value":{"name":"Feature"}}],"parent":{"issues":[{"idReadable":"APP-1"},{"idReadable":"APP-2"}]}}`,
			want: `YouTrack issue "APP-10" (Feature) field "parent" must contain exactly one valid issue; got 2: APP-1, APP-2`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := parseIssue([]byte(test.data), project)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestReadSnapshotRejectsBrokenConfiguredHierarchy(t *testing.T) {
	client := httpDoerFunc(func(*http.Request) (*http.Response, error) {
		return testHTTPResponse(http.StatusOK, `[{"id":"2-3","idReadable":"APP-3","summary":"Orphan","description":null,"resolved":null,"tags":[],"customFields":[{"name":"Type","value":{"name":"User Story"}}],"parent":{"issues":[]}}]`), nil
	})
	connection := Config{URL: "https://youtrack.example.com", token: "secret-token"}

	snapshot, err := readSnapshotWithClient(context.Background(), client, connection, testProjectConfig(), 100)
	if err == nil || !strings.Contains(err.Error(), "has no parent") {
		t.Fatalf("error = %v, want missing parent", err)
	}
	if snapshot.WorkItems != nil {
		t.Fatalf("snapshot = %#v, want no partial result", snapshot)
	}
}

func TestParseIssueUsesConfiguredFieldNameAndTypeAlias(t *testing.T) {
	project := testProjectConfig()
	project.YouTrack.Fields.Kind = "Work Item Type"
	project.YouTrack.Hierarchy[0].Types = []string{"Initiative"}
	data := []byte(`{"id":"2-1","idReadable":"APP-1","summary":"Root","description":null,"resolved":null,"tags":[],"customFields":[{"name":"Work Item Type","value":{"name":"Initiative"}}],"parent":null}`)

	item, err := parseIssue(data, project)
	if err != nil {
		t.Fatal(err)
	}
	if item.Kind != "Initiative" || item.Role != "epic" {
		t.Fatalf("item kind/role = %q/%q, want Initiative/epic", item.Kind, item.Role)
	}
}
