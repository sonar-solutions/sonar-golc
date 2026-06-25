package getbibucket

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/SonarSource-Demos/sonar-golc/pkg/utils"
	"github.com/ktrysmt/go-bitbucket"
)

func TestIsRepoExcluded(t *testing.T) {
	el := utils.NewExclusionList(nil, []string{"PROJ/my-repo", "OTHER/other-repo"})

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

func TestMatchesSingleRepos(t *testing.T) {
	tests := []struct {
		name        string
		singleRepos string
		repoSlug    string
		want        bool
	}{
		{"single match", "repo1", "repo1", true},
		{"single no match", "repo1", "repo2", false},
		{"multiple first", "repo1,repo2,repo3", "repo1", true},
		{"multiple middle", "repo1,repo2,repo3", "repo2", true},
		{"multiple last", "repo1,repo2,repo3", "repo3", true},
		{"multiple no match", "repo1,repo2,repo3", "repo4", false},
		{"whitespace tolerated", "repo1, repo2 ,  repo3", "repo2", true},
		{"empty filter", "", "repo1", false},
		{"no partial match", "repo10", "repo1", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := matchesSingleRepos(tc.singleRepos, tc.repoSlug); got != tc.want {
				t.Errorf("matchesSingleRepos(%q, %q) = %v, want %v", tc.singleRepos, tc.repoSlug, got, tc.want)
			}
		})
	}
}

func TestListRepos_SingleReposFilter(t *testing.T) {
	// Mock the "is repo empty" file listing so matched repos are treated as
	// non-empty (one file present).
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(Response1{
			Values:  []FileInfo{{Path: "README.md"}},
			Pagelen: 100,
			Page:    1,
		})
	}))
	defer ts.Close()

	reposRes := &bitbucket.RepositoriesRes{
		Items: []bitbucket.Repository{
			{Slug: "repo1", Mainbranch: bitbucket.RepositoryBranch{Name: "main"}},
			{Slug: "repo2", Mainbranch: bitbucket.RepositoryBranch{Name: "main"}},
			{Slug: "repo3", Mainbranch: bitbucket.RepositoryBranch{Name: "main"}},
		},
	}

	t.Run("collects all matching repos from a comma list", func(t *testing.T) {
		parms := ParamsProjectBitbucket{
			SingleRepos:      "repo1,repo3",
			Workspace:        "ws",
			AccessToken:      "token",
			BitbucketURLBase: ts.URL + "/",
			Exclusionlist:    utils.NewExclusionList(nil, nil),
		}
		empty, excluded, repos, err := listRepos(parms, "PROJ", reposRes)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if empty != 0 || excluded != 0 {
			t.Errorf("expected no empty/excluded, got empty=%d excluded=%d", empty, excluded)
		}
		if len(repos) != 2 {
			t.Fatalf("expected repo1 and repo3, got %d", len(repos))
		}
		if repos[0].Slug != "repo1" || repos[1].Slug != "repo3" {
			t.Errorf("expected [repo1 repo3], got [%s %s]", repos[0].Slug, repos[1].Slug)
		}
	})

	t.Run("excluded matched repo is skipped not analyzed", func(t *testing.T) {
		parms := ParamsProjectBitbucket{
			SingleRepos:      "repo1,repo2",
			Workspace:        "ws",
			AccessToken:      "token",
			BitbucketURLBase: ts.URL + "/",
			Exclusionlist:    utils.NewExclusionList(nil, []string{"PROJ/repo1"}),
		}
		_, excluded, repos, err := listRepos(parms, "PROJ", reposRes)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if excluded != 1 {
			t.Errorf("expected excluded=1, got %d", excluded)
		}
		if len(repos) != 1 || repos[0].Slug != "repo2" {
			t.Fatalf("expected only repo2, got %+v", repos)
		}
	})
}

func TestIsProjectExcluded(t *testing.T) {
	el := utils.NewExclusionList([]string{"excl-proj"}, nil)

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

	el := utils.NewExclusionList(nil, nil)
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

	el := utils.NewExclusionList([]string{"EXCL"}, nil)
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

	el := utils.NewExclusionList(nil, nil)
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

	el := utils.NewExclusionList(nil, nil)
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

	el := utils.NewExclusionList(nil, nil)
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

	el := utils.NewExclusionList(nil, nil)
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

	el := utils.NewExclusionList(nil, nil)
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

	el := utils.NewExclusionList([]string{"EXCL"}, nil)
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

func TestListReposForProject_ArchivedFiltered(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repositories/ws":
			json.NewEncoder(w).Encode(map[string]interface{}{
				"values": []map[string]interface{}{
					{
						"slug":        "repo-archived",
						"full_name":   "ws/repo-archived",
						"is_archived": true,
						"mainbranch":  map[string]string{"name": "main"},
						"project":     map[string]string{"key": "PROJ"},
					},
					{
						"slug":        "repo-active",
						"full_name":   "ws/repo-active",
						"is_archived": false,
						"mainbranch":  map[string]string{"name": "main"},
						"project":     map[string]string{"key": "PROJ"},
					},
				},
				"next": "",
			})
		case "/repositories/ws/repo-active/src/main/":
			json.NewEncoder(w).Encode(map[string]interface{}{
				"values":  []map[string]interface{}{{"path": "README.md", "type": "commit_file"}},
				"pagelen": 100,
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	parms := ParamsProjectBitbucket{
		Workspace:        "ws",
		AccessToken:      "token",
		BitbucketURLBase: ts.URL + "/",
		Exclusionlist:    utils.NewExclusionList(nil, nil),
	}

	archivedCount, emptyOrArchived, excluded, repos, err := listReposForProject(parms, "PROJ")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if archivedCount != 1 {
		t.Errorf("expected archivedCount=1, got %d", archivedCount)
	}
	if emptyOrArchived != 0 {
		t.Errorf("expected emptyOrArchived=0, got %d", emptyOrArchived)
	}
	if excluded != 0 {
		t.Errorf("expected excluded=0, got %d", excluded)
	}
	if len(repos) != 1 {
		t.Errorf("expected 1 repo, got %d", len(repos))
	}
}

func TestListReposForProject_SingleReposArchived(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/repositories/ws" {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"values": []map[string]interface{}{
					{
						"slug":        "repo-archived",
						"full_name":   "ws/repo-archived",
						"is_archived": true,
						"mainbranch":  map[string]string{"name": "main"},
						"project":     map[string]string{"key": "PROJ"},
					},
				},
				"next": "",
			})
		} else {
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	parms := ParamsProjectBitbucket{
		Workspace:        "ws",
		AccessToken:      "token",
		BitbucketURLBase: ts.URL + "/",
		Exclusionlist:    utils.NewExclusionList(nil, nil),
		SingleRepos:      "repo-archived",
	}

	archivedCount, emptyOrArchived, excluded, repos, err := listReposForProject(parms, "PROJ")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if archivedCount != 1 {
		t.Errorf("expected archivedCount=1, got %d", archivedCount)
	}
	if emptyOrArchived != 0 {
		t.Errorf("expected emptyOrArchived=0, got %d", emptyOrArchived)
	}
	if excluded != 0 {
		t.Errorf("expected excluded=0, got %d", excluded)
	}
	if len(repos) != 0 {
		t.Errorf("expected 0 repos, got %d", len(repos))
	}
}
