package getbibucket

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/SonarSource-Demos/sonar-golc/pkg/utils"
)

func makeExclusionList(projects, repos []string) *utils.ExclusionList {
	el := &utils.ExclusionList{
		Projects: make(map[string]bool),
		Repos:    make(map[string]bool),
	}
	for _, p := range projects {
		el.Projects[p] = true
	}
	for _, r := range repos {
		el.Repos[r] = true
	}
	return el
}

func TestIsRepoExcluded(t *testing.T) {
	el := makeExclusionList(nil, []string{"PROJ/my-repo", "OTHER/other-repo"})

	tests := []struct {
		projectKey string
		repoKey    string
		want       bool
	}{
		{"PROJ", "my-repo", true},
		{"OTHER", "other-repo", true},
		{"PROJ", "other-repo", false},
		{"NONE", "my-repo", false},
	}
	for _, tc := range tests {
		got := isRepoExcluded(el, tc.projectKey, tc.repoKey)
		if got != tc.want {
			t.Errorf("isRepoExcluded(%q, %q) = %v, want %v", tc.projectKey, tc.repoKey, got, tc.want)
		}
	}
}

func TestIsProjectExcluded(t *testing.T) {
	el := makeExclusionList([]string{"excl-proj"}, nil)

	if !isProjectExcluded(el, "excl-proj") {
		t.Error("expected excl-proj to be excluded")
	}
	if isProjectExcluded(el, "other") {
		t.Error("expected other to not be excluded")
	}
}

func TestFindLargestRepository(t *testing.T) {
	branches := []ProjectBranch{
		{RepoSlug: "repo-a", MainBranch: "main", LargestSize: 100},
		{RepoSlug: "repo-b", MainBranch: "develop", LargestSize: 500},
		{RepoSlug: "repo-c", MainBranch: "feature", LargestSize: 200},
	}

	var totalSize int
	branch, slug := findLargestRepository(branches, &totalSize)

	if slug != "repo-b" {
		t.Errorf("expected repo-b, got %q", slug)
	}
	if branch != "develop" {
		t.Errorf("expected develop, got %q", branch)
	}
	if totalSize != 800 {
		t.Errorf("expected totalSize=800, got %d", totalSize)
	}
}

func TestFindLargestRepository_Empty(t *testing.T) {
	var totalSize int
	branch, slug := findLargestRepository(nil, &totalSize)
	if slug != "" || branch != "" {
		t.Errorf("expected empty strings for nil input, got %q %q", slug, branch)
	}
	if totalSize != 0 {
		t.Errorf("expected totalSize=0, got %d", totalSize)
	}
}

func TestGetAuthHeader_Bearer(t *testing.T) {
	tests := []struct {
		users string
	}{
		{""},
		{"XXXXX"},
	}
	for _, tc := range tests {
		got := getAuthHeader(tc.users, "mytoken")
		if got != "Bearer mytoken" {
			t.Errorf("getAuthHeader(%q, ...) = %q, want Bearer token", tc.users, got)
		}
	}
}

func TestGetAuthHeader_Basic(t *testing.T) {
	got := getAuthHeader("alice", "secret")
	expected := "Basic " + base64.StdEncoding.EncodeToString([]byte("alice:secret"))
	if got != expected {
		t.Errorf("expected Basic auth header, got %q", got)
	}
}

func TestLoadExclusionFileOrCreateNew_Zero(t *testing.T) {
	el, err := loadExclusionFileOrCreateNew("0")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(el.Projects) != 0 || len(el.Repos) != 0 {
		t.Error("expected empty exclusion list for '0'")
	}
}

func TestLoadExclusionFileOrCreateNew_MissingFile(t *testing.T) {
	_, err := loadExclusionFileOrCreateNew("/nonexistent/path/excl.txt")
	if err == nil {
		t.Error("expected error for missing file")
	}
}

func TestGetBitbucketUsername(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/user" {
			http.NotFound(w, r)
			return
		}
		json.NewEncoder(w).Encode(map[string]string{"username": "alice"})
	}))
	defer ts.Close()

	got := GetBitbucketUsername("", "token", ts.URL+"/")
	if got != "alice" {
		t.Errorf("expected alice, got %q", got)
	}
}

func TestGetBitbucketUsername_Unauthorized(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	}))
	defer ts.Close()

	got := GetBitbucketUsername("", "bad-token", ts.URL+"/")
	if got != "" {
		t.Errorf("expected empty username for 401, got %q", got)
	}
}

func TestGetAllProjectsWithAuth(t *testing.T) {
	response := map[string]interface{}{
		"values":  []Projectc{{Key: "P1", Name: "Project 1"}, {Key: "P2", Name: "Project 2"}},
		"next":    "",
		"pagelen": 100,
	}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(response)
	}))
	defer ts.Close()

	el := makeExclusionList(nil, nil)
	result, excluded, err := getAllProjectsWithAuth("ws", "token", "", ts.URL+"/", el)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 2 {
		t.Errorf("expected 2 projects, got %d", len(result))
	}
	if excluded != 0 {
		t.Errorf("expected 0 excluded, got %d", excluded)
	}
}

func TestGetAllProjectsWithAuth_ExcludesProjects(t *testing.T) {
	response := map[string]interface{}{
		"values": []Projectc{{Key: "KEEP", Name: "Keep"}, {Key: "EXCL", Name: "Exclude"}},
		"next":   "",
	}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(response)
	}))
	defer ts.Close()

	el := makeExclusionList([]string{"EXCL"}, nil)
	result, excluded, err := getAllProjectsWithAuth("ws", "token", "", ts.URL+"/", el)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 || result[0].Key != "KEEP" {
		t.Errorf("expected [KEEP], got %v", result)
	}
	if excluded != 1 {
		t.Errorf("expected 1 excluded, got %d", excluded)
	}
}

func TestGetAllProjectsWithAuth_HTTPError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "forbidden", http.StatusForbidden)
	}))
	defer ts.Close()

	el := makeExclusionList(nil, nil)
	_, _, err := getAllProjectsWithAuth("ws", "token", "", ts.URL+"/", el)
	if err == nil {
		t.Error("expected error for HTTP 403")
	}
}

func TestGetAllProjectsWithAuth_Pagination(t *testing.T) {
	var tsURL string

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/page2" {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"values":  []Projectc{{Key: "P2", Name: "Project 2"}},
				"next":    "",
				"pagelen": 1,
			})
		} else {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"values":  []Projectc{{Key: "P1", Name: "Project 1"}},
				"next":    fmt.Sprintf("%s/page2", tsURL),
				"pagelen": 1,
			})
		}
	}))
	tsURL = ts.URL
	defer ts.Close()

	el := makeExclusionList(nil, nil)
	result, _, err := getAllProjectsWithAuth("ws", "token", "", ts.URL+"/", el)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 2 {
		t.Errorf("expected 2 projects across 2 pages, got %d", len(result))
	}
}

func TestGetSpecificProjectsWithAuth(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/workspaces/ws/projects/P1" {
			json.NewEncoder(w).Encode(Projectc{Key: "P1", Name: "Project 1"})
		} else {
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	el := makeExclusionList(nil, nil)
	result, excluded, err := getSepecificProjectsWithAuth("ws", "P1", "token", "", ts.URL+"/", el)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 || result[0].Key != "P1" {
		t.Errorf("expected [P1], got %v", result)
	}
	if excluded != 0 {
		t.Errorf("expected 0 excluded, got %d", excluded)
	}
}

func TestGetSpecificProjectsWithAuth_MultipleKeys(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/workspaces/ws/projects/P1":
			json.NewEncoder(w).Encode(Projectc{Key: "P1", Name: "Project 1"})
		case "/workspaces/ws/projects/P2":
			json.NewEncoder(w).Encode(Projectc{Key: "P2", Name: "Project 2"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	el := makeExclusionList(nil, nil)
	result, _, err := getSepecificProjectsWithAuth("ws", "P1, P2", "token", "", ts.URL+"/", el)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 2 {
		t.Errorf("expected 2 projects, got %d", len(result))
	}
}

func TestGetSpecificProjectsWithAuth_HTTPError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer ts.Close()

	el := makeExclusionList(nil, nil)
	_, _, err := getSepecificProjectsWithAuth("ws", "P1", "token", "", ts.URL+"/", el)
	if err == nil {
		t.Error("expected error for HTTP 404")
	}
}

func TestGetSpecificProjectsWithAuth_ExcludedProject(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(Projectc{Key: "EXCL", Name: "Excluded"})
	}))
	defer ts.Close()

	el := makeExclusionList([]string{"EXCL"}, nil)
	result, excluded, err := getSepecificProjectsWithAuth("ws", "EXCL", "token", "", ts.URL+"/", el)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("expected no projects, got %v", result)
	}
	if excluded != 1 {
		t.Errorf("expected 1 excluded, got %d", excluded)
	}
}
