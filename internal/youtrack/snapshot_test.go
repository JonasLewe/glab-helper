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
		if fields := query["customFields"]; !reflect.DeepEqual(fields, snapshotCustomFields) {
			t.Errorf("customFields = %q, want %q", fields, snapshotCustomFields)
		}
		skips = append(skips, query.Get("$skip"))

		switch query.Get("$skip") {
		case "0":
			return testHTTPResponse(http.StatusOK, `[
  {"id":"2-1","idReadable":"APP-1","summary":"Platform","description":"Epic details","resolved":null,"tags":[{"name":"sync"}],"customFields":[{"name":"Type","value":{"name":"Epic"}},{"name":"State","value":{"name":"Open"}},{"name":"Priority","value":{"name":"Major"}}],"parent":null},
  {"id":"2-2","idReadable":"APP-2","summary":"API","description":null,"resolved":null,"tags":[{"name":"backend"},{"name":"sync"}],"customFields":[{"name":"Type","value":{"name":"Task"}},{"name":"State","value":{"name":"In Progress"}},{"name":"Priority","value":[{"name":"Minor"}]}],"parent":{"issues":[{"idReadable":"APP-1"}]}}
]`), nil
		case "2":
			return testHTTPResponse(http.StatusOK, `[{"id":"2-3","idReadable":"APP-3","summary":"Finished","description":"Done","resolved":1720000000000,"tags":[],"customFields":[],"parent":null}]`), nil
		default:
			return testHTTPResponse(http.StatusBadRequest, "unexpected page"), nil
		}
	})

	config := Config{URL: "https://youtrack.example.com", Query: "project: APP tag: gitlab-sync", token: token}
	snapshot, err := readSnapshotWithClient(context.Background(), client, config, 2)
	if err != nil {
		t.Fatal(err)
	}
	want := source.Snapshot{WorkItems: []source.WorkItem{
		{ID: "APP-1", Title: "Platform", Description: "Epic details", Kind: "Epic", Status: "Open", Priority: "Major", Tags: []string{"sync"}},
		{ID: "APP-2", Title: "API", Kind: "Task", Status: "In Progress", Priority: "Minor", Tags: []string{"backend", "sync"}, ParentID: "APP-1"},
		{ID: "APP-3", Title: "Finished", Description: "Done", Tags: []string{}, Resolved: true},
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
			return testHTTPResponse(http.StatusOK, `[{"id":"2-1","idReadable":"APP-1","summary":"First","description":null,"resolved":null,"tags":[],"customFields":[],"parent":null}]`), nil
		}
		return nil, errors.New("transport repeated " + token)
	})

	config := Config{URL: "https://youtrack.example.com", Query: "project: APP", token: token}
	snapshot, err := readSnapshotWithClient(context.Background(), client, config, 1)
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
	_, err := parseIssue([]byte(`{"id":"2-1","idReadable":"APP-1","summary":"Incomplete"}`))
	if err == nil || !strings.Contains(err.Error(), "description") {
		t.Fatalf("error = %v, want missing description", err)
	}
}
