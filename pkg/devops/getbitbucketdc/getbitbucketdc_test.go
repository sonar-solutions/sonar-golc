package getbibucketdc

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/SonarSource-Demos/sonar-golc/pkg/utils"
)

// --- Pure logic tests ---

func TestIsProjectAndRepoExcluded(t *testing.T) {
	el := utils.ExclusionList{
		Projects: make(map[string]bool),
		Repos:    map[string]bool{"PROJ/my-repo": true},
	}

	if !isProjectAndRepoExcluded("PROJ/my-repo", el) {
		t.Error("expected PROJ/my-repo to be excluded")
	}
	if isProjectAndRepoExcluded("PROJ/other-repo", el) {
		t.Error("expected PROJ/other-repo to not be excluded")
	}
}

func TestIsProjectAndRepoExcluded_FalseValue(t *testing.T) {
	// An entry with value false should NOT be considered excluded.
	el := utils.ExclusionList{
		Projects: make(map[string]bool),
		Repos:    map[string]bool{"PROJ/repo": false},
	}
	if isProjectAndRepoExcluded("PROJ/repo", el) {
		t.Error("entry with value=false should not be excluded")
	}
}

func TestIsProjectExcluded1(t *testing.T) {
	el := utils.ExclusionList{
		Projects: map[string]bool{"excl-proj": true},
		Repos:    make(map[string]bool),
	}

	if !isProjectExcluded1("excl-proj", el) {
		t.Error("expected excl-proj to be excluded")
	}
	if isProjectExcluded1("other", el) {
		t.Error("expected other to not be excluded")
	}
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

func TestIsRepoExcluded(t *testing.T) {
	// getbitbucketdc's isRepoExcluded takes the full "PROJECT/REPO" composite key.
	el := utils.NewExclusionList(nil, []string{"PROJ/my-repo"})

	if !isRepoExcluded(el, "PROJ/my-repo") {
		t.Error("expected PROJ/my-repo to be excluded")
	}
	if isRepoExcluded(el, "PROJ/other") {
		t.Error("expected PROJ/other to not be excluded")
	}
}

func TestSummarizeAnalysisResults_ReturnsSameSlice(t *testing.T) {
	branches := []ProjectBranch{
		{Org: "org", ProjectKey: "PROJ", RepoSlug: "repo-a", MainBranch: "main", LargestSize: 100},
		{Org: "org", ProjectKey: "PROJ", RepoSlug: "repo-b", MainBranch: "develop", LargestSize: 500},
	}

	result := summarizeAnalysisResults(branches, 2)

	if len(result) != 2 {
		t.Errorf("expected 2 branches returned, got %d", len(result))
	}
	if result[0].RepoSlug != "repo-a" || result[1].RepoSlug != "repo-b" {
		t.Errorf("expected same slice returned, got %v", result)
	}
}

func TestSummarizeAnalysisResults_Empty(t *testing.T) {
	result := summarizeAnalysisResults(nil, 0)
	if result != nil {
		t.Errorf("expected nil for empty input, got %v", result)
	}
}

func TestLoadOrCreateExclusionList_Zero(t *testing.T) {
	el, err := loadOrCreateExclusionList("0")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(el.Projects) != 0 || len(el.Repos) != 0 {
		t.Error("expected empty exclusion list for '0'")
	}
}

func TestLoadOrCreateExclusionList_MissingFile(t *testing.T) {
	_, err := loadOrCreateExclusionList("/nonexistent/path/excl.txt")
	if err == nil {
		t.Error("expected error for missing file")
	}
}

// --- HTTP-dependent tests ---

func TestFetchBranches(t *testing.T) {
	branches := []Branch{
		{ID: "refs/heads/main", Name: "main", IsDefault: true},
		{ID: "refs/heads/develop", Name: "develop", IsDefault: false},
	}
	response := BranchResponse{
		Size:       2,
		Limit:      100,
		IsLastPage: true,
		Values:     branches,
	}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(response)
	}))
	defer ts.Close()

	result, err := fetchBranches(ts.URL+"/branches", "test-token")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Values) != 2 {
		t.Errorf("expected 2 branches, got %d", len(result.Values))
	}
	if !result.IsLastPage {
		t.Error("expected IsLastPage=true")
	}
}

func TestFetchAllBranches_SinglePage(t *testing.T) {
	response := BranchResponse{
		IsLastPage: true,
		Values: []Branch{
			{Name: "main", IsDefault: true},
			{Name: "develop", IsDefault: false},
		},
	}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(response)
	}))
	defer ts.Close()

	result, err := fetchAllBranches(ts.URL+"/branches", "token")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 2 {
		t.Errorf("expected 2 branches, got %d", len(result))
	}
}

func TestFetchAllBranches_Pagination(t *testing.T) {
	var tsURL string

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := r.URL.Query().Get("start")
		if start == "1" {
			json.NewEncoder(w).Encode(BranchResponse{
				IsLastPage: true,
				Values:     []Branch{{Name: "develop"}},
			})
		} else {
			json.NewEncoder(w).Encode(BranchResponse{
				IsLastPage:    false,
				NextPageStart: 1,
				Values:        []Branch{{Name: "main"}},
			})
		}
		_ = tsURL
	}))
	tsURL = ts.URL
	defer ts.Close()

	result, err := fetchAllBranches(ts.URL+"/branches", "token")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 2 {
		t.Errorf("expected 2 branches across 2 pages, got %d", len(result))
	}
}

func TestGetDefaultBranch(t *testing.T) {
	response := BranchesResponse{
		IsLastPage: true,
		Values: []Branch{
			{Name: "develop", IsDefault: false},
			{Name: "main", IsDefault: true},
		},
	}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(response)
	}))
	defer ts.Close()

	// url1 must contain a query parameter so that &start=N can be appended.
	branch, err := getDefaultBranch(ts.URL+"/branches?limit=100", "token")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if branch == nil {
		t.Fatal("expected non-nil branch")
	}
	if branch.Name != "main" {
		t.Errorf("expected main, got %q", branch.Name)
	}
}

func TestGetDefaultBranch_NotFound(t *testing.T) {
	response := BranchesResponse{
		IsLastPage: true,
		Values:     []Branch{{Name: "develop", IsDefault: false}},
	}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(response)
	}))
	defer ts.Close()

	_, err := getDefaultBranch(ts.URL+"/branches?limit=100", "token")
	if err == nil {
		t.Error("expected error when no default branch found")
	}
}

func TestGetDefaultBranch_Pagination(t *testing.T) {
	var tsURL string

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := r.URL.Query().Get("start")
		if start == "1" {
			json.NewEncoder(w).Encode(BranchesResponse{
				IsLastPage: true,
				Values:     []Branch{{Name: "main", IsDefault: true}},
			})
		} else {
			json.NewEncoder(w).Encode(BranchesResponse{
				IsLastPage:    false,
				NextPageStart: 1,
				Values:        []Branch{{Name: "develop", IsDefault: false}},
			})
		}
		_ = tsURL
	}))
	tsURL = ts.URL
	defer ts.Close()

	branch, err := getDefaultBranch(fmt.Sprintf("%s/branches?limit=100", ts.URL), "token")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if branch.Name != "main" {
		t.Errorf("expected main, got %q", branch.Name)
	}
}

func TestIfExistBranches(t *testing.T) {
	response := BranchResponse{
		IsLastPage: true,
		Values:     []Branch{{Name: "feature-x"}},
	}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(response)
	}))
	defer ts.Close()

	result, err := ifExistBranches(ts.URL+"/branches", "token")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 || result[0].Name != "feature-x" {
		t.Errorf("expected [feature-x], got %v", result)
	}
}

func TestIfExistBranches_APIError(t *testing.T) {
	errorResp := map[string]interface{}{
		"type": "error",
		"error": map[string]string{
			"message": "branch not found",
		},
	}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(errorResp)
	}))
	defer ts.Close()

	_, err := ifExistBranches(ts.URL+"/branches", "token")
	if err == nil {
		t.Error("expected error for API error response")
	}
}
