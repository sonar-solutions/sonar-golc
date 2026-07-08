package getazure

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/SonarSource-Demos/sonar-golc/pkg/utils"
	"github.com/google/uuid"
	"github.com/microsoft/azure-devops-go-api/azuredevops/git"
)

// newDisabledReposParams builds the minimal params fetchDisabledRepoIDs needs,
// pointing ApiURL at the given test server.
func newDisabledReposParams(apiURL string) ParamsProjectAzure {
	return ParamsProjectAzure{
		Context:     context.Background(),
		ApiURL:      apiURL,
		AccessToken: "token",
	}
}

func TestFetchDisabledRepoIDs_ParsesDisabledFlag(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"value":[
			{"id":"AAAAAAAA-1111-2222-3333-444444444444","isDisabled":true},
			{"id":"BBBBBBBB-1111-2222-3333-444444444444","isDisabled":false},
			{"id":"CCCCCCCC-1111-2222-3333-444444444444"}
		]}`))
	}))
	defer ts.Close()

	disabled := fetchDisabledRepoIDs(newDisabledReposParams(ts.URL), "PROJ")

	if len(disabled) != 1 {
		t.Fatalf("expected exactly 1 disabled repo, got %d: %v", len(disabled), disabled)
	}
	// IDs are keyed lowercase so lookups from git.GitRepository UUIDs match.
	if !disabled["aaaaaaaa-1111-2222-3333-444444444444"] {
		t.Errorf("disabled repo id not found (case-insensitive): %v", disabled)
	}
}

func TestFetchDisabledRepoIDs_BadEndpointIsEmpty(t *testing.T) {
	// A control character in the URL makes request construction fail, exercising
	// the early-return warning path without any network call.
	if disabled := fetchDisabledRepoIDs(newDisabledReposParams("http://\x7f-bad"), "PROJ"); len(disabled) != 0 {
		t.Errorf("expected empty set when the request cannot be built, got %v", disabled)
	}
}

func TestFetchDisabledRepoIDs_Non200IsEmpty(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer ts.Close()

	if disabled := fetchDisabledRepoIDs(newDisabledReposParams(ts.URL), "PROJ"); len(disabled) != 0 {
		t.Errorf("expected empty set on non-200 response, got %v", disabled)
	}
}

func TestFetchDisabledRepoIDs_BadJSONIsEmpty(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("{not json"))
	}))
	defer ts.Close()

	if disabled := fetchDisabledRepoIDs(newDisabledReposParams(ts.URL), "PROJ"); len(disabled) != 0 {
		t.Errorf("expected empty set on malformed JSON, got %v", disabled)
	}
}

func TestAzureDiscoverySkipped(t *testing.T) {
	// 20 discovered, 2 empty, 3 excluded, 1 archived, 10 analyzed -> 4 dropped mid-loop.
	if got := azureDiscoverySkipped(20, 2, 3, 1, 10); got != 4 {
		t.Errorf("azureDiscoverySkipped = %d, want 4", got)
	}
	// Never negative, even if the counts somehow over-subtract.
	if got := azureDiscoverySkipped(5, 2, 3, 1, 10); got != 0 {
		t.Errorf("azureDiscoverySkipped should floor at 0, got %d", got)
	}
}

// TestListReposForProject_CountsDisabled is the core #74 assertion: a disabled
// (archived-equivalent) repo is counted as archived and dropped from the analyzed
// set, while a normal repo is returned.
func TestListReposForProject_CountsDisabled(t *testing.T) {
	disabledID := uuid.New()
	enabledID := uuid.New()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintf(w, `{"value":[{"id":"%s","isDisabled":true},{"id":"%s","isDisabled":false}]}`, disabledID, enabledID)
	}))
	defer ts.Close()

	disabledName, enabledName := "disabled-repo", "live-repo"
	fc := &fakeGitClient{
		repos: &[]git.GitRepository{
			{Id: &disabledID, Name: &disabledName},
			{Id: &enabledID, Name: &enabledName},
		},
		items:    &[]git.GitItem{{}},        // non-empty content
		branches: &[]git.GitBranchStats{{}}, // at least one branch
	}

	parms := ParamsProjectAzure{
		Context:       context.Background(),
		ApiURL:        ts.URL,
		AccessToken:   "token",
		Exclusionlist: &utils.ExclusionList{Projects: map[string]bool{}, Repos: map[string]bool{}},
	}

	archived, empty, excluded, repos, err := listReposForProject(parms, "PROJ", fc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if archived != 1 {
		t.Errorf("archived = %d, want 1 (the disabled repo)", archived)
	}
	if empty != 0 || excluded != 0 {
		t.Errorf("empty=%d excluded=%d, want 0 and 0", empty, excluded)
	}
	if len(repos) != 1 || *repos[0].Name != enabledName {
		t.Errorf("expected only the live repo returned, got %d repos", len(repos))
	}
}

// TestListReposForProject_ExcludedAndEmpty covers the exclusion and empty-repo
// filter branches alongside the archived one.
func TestListReposForProject_ExcludedAndEmpty(t *testing.T) {
	excludedID := uuid.New()
	emptyID := uuid.New()

	// No repos are disabled here; the REST endpoint returns an empty set.
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"value":[]}`))
	}))
	defer ts.Close()

	excludedName, emptyName := "excluded-repo", "empty-repo"
	fc := &fakeGitClient{
		repos: &[]git.GitRepository{
			{Id: &excludedID, Name: &excludedName},
			{Id: &emptyID, Name: &emptyName},
		},
		items:    &[]git.GitItem{},        // empty content -> repo counts as empty
		branches: &[]git.GitBranchStats{}, // no branches
	}

	parms := ParamsProjectAzure{
		Context:     context.Background(),
		ApiURL:      ts.URL,
		AccessToken: "token",
		Exclusionlist: &utils.ExclusionList{
			Projects: map[string]bool{},
			Repos:    map[string]bool{"PROJ/" + excludedName: true},
		},
	}

	archived, empty, excluded, repos, err := listReposForProject(parms, "PROJ", fc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if archived != 0 || excluded != 1 || empty != 1 {
		t.Errorf("archived=%d excluded=%d empty=%d, want 0/1/1", archived, excluded, empty)
	}
	if len(repos) != 0 {
		t.Errorf("expected no analyzable repos, got %d", len(repos))
	}
}
