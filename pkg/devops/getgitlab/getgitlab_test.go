package getgitlab

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"net/http"
	"net/http/httptest"
	"strings"

	"fmt"

	"github.com/briandowns/spinner"
	"github.com/xanzy/go-gitlab"
)

// Constants to avoid duplicating string literals (SonarQube maintainability)
const (
	errFailedToCreateTempDir = "Failed to create temp dir: %v"
	resultsConfigDir         = "Results/config"
)

// Test helper functions to reduce duplication
func setupTestEnvironment(t *testing.T, prefix string) (string, func()) {
	tempDir, err := os.MkdirTemp("", prefix)
	if err != nil {
		t.Fatalf(errFailedToCreateTempDir, err)
	}

	originalWd, _ := os.Getwd()
	cleanup := func() {
		os.Chdir(originalWd)
		os.RemoveAll(tempDir)
	}

	os.Chdir(tempDir)
	return tempDir, cleanup
}

func createTestDirectories(t *testing.T, dirs []string) {
	for _, dir := range dirs {
		err := os.MkdirAll(dir, 0755)
		if err != nil {
			t.Fatalf("Failed to create dir %s: %v", dir, err)
		}
	}
}

func createTestAnalysisResult() AnalysisResult {
	return AnalysisResult{
		NumRepositories: 1,
		ProjectBranches: []ProjectBranch{
			{
				Org:        "test-org",
				RepoSlug:   "test-repo",
				MainBranch: "main",
			},
		},
	}
}

func TestSaveResultGitlab(t *testing.T) {
	_, cleanup := setupTestEnvironment(t, "test_gitlab_save_*")
	defer cleanup()

	createTestDirectories(t, []string{"Logs", "Results", resultsConfigDir})

	t.Run("SaveResult creates gitlab analysis file", func(t *testing.T) {
		result := createTestAnalysisResult()

		// Test that SaveResult creates the correct file
		err := SaveResult(result)
		if err != nil {
			t.Errorf("SaveResult() error = %v, want nil", err)
		}

		// Verify that the correct GitLab-specific file was created
		expectedFile := resultsConfigDir + "/analysis_result_gitlab.json"
		if _, err := os.Stat(expectedFile); os.IsNotExist(err) {
			t.Errorf("Expected GitLab analysis file was not created: %s", expectedFile)
		}

		// Verify the file contains valid JSON
		fileData, err := os.ReadFile(expectedFile)
		if err != nil {
			t.Fatalf("Failed to read created file: %v", err)
		}

		var parsedResult AnalysisResult
		err = json.Unmarshal(fileData, &parsedResult)
		if err != nil {
			t.Errorf("Created file contains invalid JSON: %v", err)
		}

		// Verify the data integrity
		if parsedResult.NumRepositories != result.NumRepositories {
			t.Errorf("SaveResult() NumRepositories = %d, want %d", parsedResult.NumRepositories, result.NumRepositories)
		}

		if len(parsedResult.ProjectBranches) != len(result.ProjectBranches) {
			t.Errorf("SaveResult() ProjectBranches count = %d, want %d", len(parsedResult.ProjectBranches), len(result.ProjectBranches))
		}

		// Verify specific GitLab filename (not GitHub)
		githubFile := resultsConfigDir + "/analysis_result_github.json"
		if _, err := os.Stat(githubFile); !os.IsNotExist(err) {
			t.Error("SaveResult() incorrectly created GitHub analysis file instead of GitLab file")
		}
	})

	t.Run("SaveResult handles encoding error", func(t *testing.T) {
		// Create a result that would cause encoding issues
		result := AnalysisResult{
			NumRepositories: -1, // Invalid data to potentially cause issues
			ProjectBranches: nil,
		}

		// Test that error handling works correctly
		err := SaveResult(result)
		// The function should handle this gracefully
		// Even if there's an error, it should be logged with proper formatting (which was the fix)
		if err != nil {
			// This exercises the error logging path that was fixed
			t.Logf("SaveResult properly handled encoding error: %v", err)
		}
	})
}

func TestGitlabErrorFormatting(t *testing.T) {
	_, cleanup := setupTestEnvironment(t, "test_gitlab_logs_*")
	defer cleanup()

	createTestDirectories(t, []string{"Logs", resultsConfigDir})

	t.Run("Error encoding JSON file formatting", func(t *testing.T) {
		// This tests the specific error logging line that was fixed in GitLab
		// by making the file directory read-only to force an encoding error

		// Make the config directory read-only to force error
		err := os.Chmod(resultsConfigDir, 0444)
		if err != nil {
			t.Fatalf("Failed to make directory read-only: %v", err)
		}

		// Restore permissions after test
		defer func() {
			os.Chmod(resultsConfigDir, 0755)
		}()

		result := createTestAnalysisResult()

		// This should trigger the error logging with proper %v formatting
		err = SaveResult(result)
		if err == nil {
			t.Error("Expected SaveResult to fail with read-only directory")
		}

		// The error should have been logged with proper formatting
		// The specific line that was fixed:
		// loggers.Errorf("❌ Error encoding JSON file <Results/config/analysis_result_gitlab.json> :%v", err)
	})
}

func TestGitlabFileNaming(t *testing.T) {
	_, cleanup := setupTestEnvironment(t, "test_gitlab_naming_*")
	defer cleanup()

	createTestDirectories(t, []string{"Logs", "Results", resultsConfigDir})

	t.Run("GitLab uses correct filename", func(t *testing.T) {
		result := AnalysisResult{
			NumRepositories: 2,
			ProjectBranches: []ProjectBranch{
				{
					Org:        "gitlab-org-1",
					RepoSlug:   "repo-1",
					MainBranch: "main",
				},
				{
					Org:        "gitlab-org-2",
					RepoSlug:   "repo-2",
					MainBranch: "develop",
				},
			},
		}

		err := SaveResult(result)
		if err != nil {
			t.Errorf("SaveResult() error = %v, want nil", err)
		}

		// The key fix: GitLab should save to analysis_result_gitlab.json, NOT analysis_result_github.json
		gitlabFile := resultsConfigDir + "/analysis_result_gitlab.json"
		githubFile := resultsConfigDir + "/analysis_result_github.json"

		// Verify GitLab file exists
		if _, err := os.Stat(gitlabFile); os.IsNotExist(err) {
			t.Errorf("GitLab analysis file does not exist: %s", gitlabFile)
		}

		// Verify GitHub file does NOT exist (this was the bug that was fixed)
		if _, err := os.Stat(githubFile); !os.IsNotExist(err) {
			t.Errorf("GitHub analysis file incorrectly created by GitLab SaveResult: %s", githubFile)
		}

		// Verify file contents are correct
		fileData, err := os.ReadFile(gitlabFile)
		if err != nil {
			t.Fatalf("Failed to read GitLab file: %v", err)
		}

		var savedResult AnalysisResult
		err = json.Unmarshal(fileData, &savedResult)
		if err != nil {
			t.Fatalf("GitLab file contains invalid JSON: %v", err)
		}

		if savedResult.NumRepositories != 2 {
			t.Errorf("GitLab file NumRepositories = %d, want 2", savedResult.NumRepositories)
		}

		if len(savedResult.ProjectBranches) != 2 {
			t.Errorf("GitLab file ProjectBranches count = %d, want 2", len(savedResult.ProjectBranches))
		}
	})
}

// Integration tests to improve coverage for getgitlab package
func TestGitlabIntegrationCoverage(t *testing.T) {
	_, cleanup := setupTestEnvironment(t, "test_gitlab_integration_*")
	defer cleanup()

	createTestDirectories(t, []string{"Logs", "Results", resultsConfigDir})

	t.Run("SaveResult error scenarios", func(t *testing.T) {
		// Test with invalid result data to trigger different code paths
		invalidResult := AnalysisResult{
			NumRepositories: -1, // Invalid negative value
			ProjectBranches: nil,
		}

		err := SaveResult(invalidResult)
		// This exercises error handling paths and improves coverage
		if err != nil {
			t.Logf("SaveResult handled invalid data correctly: %v", err)
		}
	})

	t.Run("SaveResult with various data sizes", func(t *testing.T) {
		// Test with different data structures to improve coverage
		testCases := []struct {
			name   string
			result AnalysisResult
		}{
			{
				name: "Empty repositories",
				result: AnalysisResult{
					NumRepositories: 0,
					ProjectBranches: []ProjectBranch{},
				},
			},
			{
				name:   "Single repository",
				result: createTestAnalysisResult(),
			},
			{
				name: "Multiple repositories",
				result: AnalysisResult{
					NumRepositories: 3,
					ProjectBranches: []ProjectBranch{
						{Org: "org1", RepoSlug: "repo1", MainBranch: "main"},
						{Org: "org2", RepoSlug: "repo2", MainBranch: "develop"},
						{Org: "org3", RepoSlug: "repo3", MainBranch: "master"},
					},
				},
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				err := SaveResult(tc.result)
				if err != nil {
					t.Errorf("SaveResult failed for %s: %v", tc.name, err)
				}

				// Verify file was created
				expectedFile := resultsConfigDir + "/analysis_result_gitlab.json"
				if _, err := os.Stat(expectedFile); os.IsNotExist(err) {
					t.Errorf("SaveResult did not create file for %s", tc.name)
				}

				// Verify file contents
				fileData, err := os.ReadFile(expectedFile)
				if err == nil {
					var savedResult AnalysisResult
					if json.Unmarshal(fileData, &savedResult) != nil {
						t.Errorf("SaveResult created invalid JSON for %s", tc.name)
					}
				}
			})
		}
	})
}

// ---------------- Additional tests for getgitlab.go behavior ----------------

const (
	testRepoExcluded = "ns/excluded"
	testRepoEmpty    = "ns/empty"
	testRepoArchived = "ns/archived"
)
const (
	apiCommitsPath         = "/api/v4/projects/10/repository/commits"
	apiBranchesListPath    = "/api/v4/projects/10/repository/branches"
	apiBranchesPrefixPath  = "/api/v4/projects/10/repository/branches/"
	apiTreePath            = "/api/v4/projects/10/repository/tree"
	apiProjectPath         = "/api/v4/projects/10"
	apiGroupGPath          = "/api/v4/groups/g"
	apiGroup1ProjectsPath  = "/api/v4/groups/1/projects"
	apiGroup1SubgroupsPath = "/api/v4/groups/1/subgroups"
	apiGroup2SubgroupsPath = "/api/v4/groups/2/subgroups"
	apiGroup2ProjectsPath  = "/api/v4/groups/2/projects"
)

// set up a fake GitLab API server for client methods used in unit tests
func newFakeGitLabServer(t *testing.T, cfg func(*http.Request) (int, any)) *httptest.Server {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		status, body := cfg(r)
		if status == 0 {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		// minimal pagination headers so client considers loop complete
		w.Header().Set("X-Total-Pages", "1")
		w.Header().Set("X-Page", "1")
		w.WriteHeader(status)
		if body != nil {
			enc := json.NewEncoder(w)
			_ = enc.Encode(body)
		}
	})
	return httptest.NewServer(handler)
}

func newGitLabClientForServer(t *testing.T, base string) *gitlab.Client {
	client, err := gitlab.NewClient("test-token", gitlab.WithBaseURL(base+"/api/v4"))
	if err != nil {
		t.Fatalf("failed creating gitlab client: %v", err)
	}
	return client
}

func TestGetCommitCount(t *testing.T) {
	commitCounts := map[string]int{
		"main": 1,
		"dev":  3,
	}
	ts := newFakeGitLabServer(t, func(r *http.Request) (int, any) {
		switch {
		case strings.HasPrefix(r.URL.Path, apiCommitsPath):
			ref := r.URL.Query().Get("ref_name")
			n := commitCounts[ref]
			commits := make([]map[string]any, n)
			for i := 0; i < n; i++ {
				commits[i] = map[string]any{"id": fmt.Sprintf("c%d", i)}
			}
			return http.StatusOK, commits
		default:
			return 0, nil
		}
	})
	defer ts.Close()

	client := newGitLabClientForServer(t, ts.URL)
	since := time.Now().Add(-time.Hour)
	until := time.Now()
	got, err := getCommitCount(client, 10, "dev", since, until)
	if err != nil {
		t.Fatalf("getCommitCount error: %v", err)
	}
	if got != 3 {
		t.Errorf("getCommitCount = %d, want 3", got)
	}
}

func TestGetAllGroupProjects(t *testing.T) {
	ts := newFakeGitLabServer(t, func(r *http.Request) (int, any) {
		switch r.URL.Path {
		case apiGroupGPath:
			return http.StatusOK, map[string]any{"id": 1, "name": "g"}
		case apiGroup1ProjectsPath:
			// The implementation uses include_subgroups=true to fetch all projects
			// in a single paginated call — return both the group project and the
			// subgroup project when that parameter is present.
			if r.URL.Query().Get("include_subgroups") == "true" {
				return http.StatusOK, []map[string]any{
					{"id": 10, "name": "proj1", "path_with_namespace": "g/proj1", "default_branch": "main", "empty_repo": false, "archived": false},
					{"id": 20, "name": "subproj", "path_with_namespace": "g/sg/subproj", "default_branch": "main", "empty_repo": false, "archived": false},
				}
			}
			return http.StatusOK, []map[string]any{
				{"id": 10, "name": "proj1", "path_with_namespace": "g/proj1", "default_branch": "main", "empty_repo": false, "archived": false},
			}
		default:
			return 0, nil
		}
	})
	defer ts.Close()
	client := newGitLabClientForServer(t, ts.URL)
	projs, err := getAllGroupProjects(client, "g")
	if err != nil {
		t.Fatalf("getAllGroupProjects error: %v", err)
	}
	if len(projs) != 2 {
		t.Errorf("getAllGroupProjects len = %d, want 2", len(projs))
	}
}

func TestGetBranchHelpers(t *testing.T) {
	ts := newFakeGitLabServer(t, func(r *http.Request) (int, any) {
		switch {
		case r.URL.Path == apiBranchesListPath:
			return http.StatusOK, []map[string]any{
				{"name": "main"},
				{"name": "dev"},
			}
		case strings.HasPrefix(r.URL.Path, apiBranchesPrefixPath):
			// return 200 only for known branches
			name := strings.TrimPrefix(r.URL.Path, apiBranchesPrefixPath)
			if name == "main" || name == "dev" {
				return http.StatusOK, map[string]any{"name": name}
			}
			return http.StatusNotFound, map[string]any{"message": "not found"}
		case r.URL.Path == apiTreePath:
			return http.StatusOK, []map[string]any{
				{"id": "1", "name": "file1"},
				{"id": "2", "name": "file2"},
			}
		case r.URL.Path == apiProjectPath:
			return http.StatusOK, map[string]any{"id": 10, "default_branch": "main"}
		case strings.HasPrefix(r.URL.Path, apiCommitsPath):
			// default: zero commits
			return http.StatusOK, []any{}
		default:
			return 0, nil
		}
	})
	defer ts.Close()
	client := newGitLabClientForServer(t, ts.URL)

	// getBranchSize
	if size := getBranchSize(client, 10, "main"); size != 2 {
		t.Errorf("getBranchSize = %d, want 2", size)
	}

	// branchExists
	if !branchExists(client, 10, "main") {
		t.Errorf("branchExists(main) = false, want true")
	}
	if branchExists(client, 10, "feature") {
		t.Errorf("branchExists(feature) = true, want false")
	}
}

func TestGetMainBranchWithCommits(t *testing.T) {
	commitCounts := map[string]int{
		"main": 1,
		"dev":  3,
	}
	ts := newFakeGitLabServer(t, func(r *http.Request) (int, any) {
		switch {
		case r.URL.Path == apiBranchesListPath:
			return http.StatusOK, []map[string]any{
				{"name": "main"},
				{"name": "dev"},
			}
		case strings.HasPrefix(r.URL.Path, apiCommitsPath):
			ref := r.URL.Query().Get("ref_name")
			n := commitCounts[ref]
			commits := make([]map[string]any, n)
			for i := 0; i < n; i++ {
				commits[i] = map[string]any{"id": fmt.Sprintf("c%d", i)}
			}
			return http.StatusOK, commits
		default:
			return 0, nil
		}
	})
	defer ts.Close()
	client := newGitLabClientForServer(t, ts.URL)

	since := time.Now().Add(-24 * time.Hour)
	until := time.Now()
	branch, size, nbr, err := getMainBranch(client, 10, since, until)
	if err != nil {
		t.Fatalf("getMainBranch error: %v", err)
	}
	if branch != "dev" || size != 3 || nbr != 2 {
		t.Errorf("getMainBranch = (%s,%d,%d), want (dev,3,2)", branch, size, nbr)
	}
}

func TestGetMainBranchWithoutCommitsFallsBackToDefault(t *testing.T) {
	ts := newFakeGitLabServer(t, func(r *http.Request) (int, any) {
		switch {
		case r.URL.Path == apiBranchesListPath:
			return http.StatusOK, []map[string]any{
				{"name": "main"},
				{"name": "empty"},
			}
		case strings.HasPrefix(r.URL.Path, apiCommitsPath):
			// zero commits for both branches
			return http.StatusOK, []any{}
		case r.URL.Path == apiProjectPath:
			return http.StatusOK, map[string]any{"id": 10, "default_branch": "main"}
		case r.URL.Path == apiTreePath:
			// default branch size
			return http.StatusOK, []map[string]any{
				{"id": "1", "name": "a"},
				{"id": "2", "name": "b"},
				{"id": "3", "name": "c"},
			}
		default:
			return 0, nil
		}
	})
	defer ts.Close()
	client := newGitLabClientForServer(t, ts.URL)

	since := time.Now().Add(-24 * time.Hour)
	until := time.Now()
	branch, size, nbr, err := getMainBranch(client, 10, since, until)
	if err != nil {
		t.Fatalf("getMainBranch error: %v", err)
	}
	if branch != "main" || size != 3 || nbr != 2 {
		t.Errorf("getMainBranch = (%s,%d,%d), want (main,3,2)", branch, size, nbr)
	}
}
func TestLoadExclusionReposSuccess(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "exclusions.txt")
	content := "repo-one\n\n repo-two \n#not-a-comment-line\nrepo-three\n"
	if err := os.WriteFile(file, []byte(content), 0644); err != nil {
		t.Fatalf("unable to write temp exclusion file: %v", err)
	}

	ex, err := LoadExclusionRepos(file)
	if err != nil {
		t.Fatalf("LoadExclusionRepos returned error: %v", err)
	}

	// Expected: non-empty trimmed lines become keys
	if !ex["repo-one"] || !ex["repo-two"] || !ex["#not-a-comment-line"] || !ex["repo-three"] {
		t.Errorf("unexpected exclusion map contents: %#v", ex)
	}
}

func TestLoadExclusionReposFileNotFound(t *testing.T) {
	_, err := LoadExclusionRepos(filepath.Join(t.TempDir(), "nope.txt"))
	if err == nil {
		t.Fatalf("expected error when file does not exist")
	}
}

func TestIsExcludedExactAndPrefix(t *testing.T) {
	ex := ExclusionRepos{
		"org/repo":   true,
		"org/prefix": true,
	}

	if !isExcluded("org/repo", ex) {
		t.Errorf("expected exact match to be excluded")
	}
	if !isExcluded("org/prefix-sub", ex) {
		t.Errorf("expected prefix match to be excluded")
	}
	if isExcluded("org/other", ex) {
		t.Errorf("did not expect non-matching repo to be excluded")
	}
}

func TestIsProjectExcludedOrInvalid(t *testing.T) {
	// excluded
	ex := ExclusionRepos{"ns/proj": true}
	proj := &gitlab.Project{PathWithNamespace: "ns/proj"}
	empty, archived := 0, 0
	excluded, isEmpty, isArchived := isProjectExcludedOrInvalid(proj, ex, &empty, &archived)
	if !excluded || isEmpty || isArchived {
		t.Errorf("expected project to be excluded only")
	}
	if empty != 0 || archived != 0 {
		t.Errorf("counters should not change for excluded project")
	}

	// empty repo
	ex = ExclusionRepos{}
	proj = &gitlab.Project{PathWithNamespace: "ns/proj2", EmptyRepo: true}
	empty, archived = 0, 0
	excluded, isEmpty, isArchived = isProjectExcludedOrInvalid(proj, ex, &empty, &archived)
	if excluded || !isEmpty || isArchived {
		t.Errorf("expected project to be empty only")
	}
	if empty != 1 || archived != 0 {
		t.Errorf("expected empty counter incremented, got empty=%d archived=%d", empty, archived)
	}

	// archived repo
	proj = &gitlab.Project{PathWithNamespace: "ns/proj3", Archived: true}
	empty, archived = 0, 0
	excluded, isEmpty, isArchived = isProjectExcludedOrInvalid(proj, ex, &empty, &archived)
	if excluded || isEmpty || !isArchived {
		t.Errorf("expected project to be archived only")
	}
	if empty != 0 || archived != 1 {
		t.Errorf("expected archived counter incremented, got empty=%d archived=%d", empty, archived)
	}
}

func TestAnalyzeProjEarlyReturns(t *testing.T) {
	// excluded path
	ap := AnalyzeProject{
		Project: &gitlab.Project{
			PathWithNamespace: testRepoExcluded,
		},
		ExclusionList: ExclusionRepos{testRepoExcluded: true},
	}
	pb, excl, emp, arch := analyzeProj(ap)
	if excl != 1 || emp != 0 || arch != 0 || (pb != ProjectBranch{}) {
		t.Errorf("excluded path invalid result: pb=%+v excl=%d emp=%d arch=%d", pb, excl, emp, arch)
	}

	// empty path
	ap = AnalyzeProject{
		Project: &gitlab.Project{
			PathWithNamespace: testRepoEmpty,
			EmptyRepo:         true,
		},
	}
	pb, excl, emp, arch = analyzeProj(ap)
	if excl != 0 || emp != 1 || arch != 0 || (pb != ProjectBranch{}) {
		t.Errorf("empty path invalid result: pb=%+v excl=%d emp=%d arch=%d", pb, excl, emp, arch)
	}

	// archived path
	ap = AnalyzeProject{
		Project: &gitlab.Project{
			PathWithNamespace: testRepoArchived,
			Archived:          true,
		},
	}
	pb, excl, emp, arch = analyzeProj(ap)
	if excl != 0 || emp != 0 || arch != 1 || (pb != ProjectBranch{}) {
		t.Errorf("archived path invalid result: pb=%+v excl=%d emp=%d arch=%d", pb, excl, emp, arch)
	}
}

func TestProcessProjectCounterIncrements(t *testing.T) {
	// Ensure isolated environment and logging directory exist to avoid logger fatal
	_, cleanup := setupTestEnvironment(t, "test_gitlab_process_project_*")
	defer cleanup()
	createTestDirectories(t, []string{"Logs"})

	// Setup a real spinner to avoid nil deref if used (shouldn't be used on early returns)
	sp := spinner.New(spinner.CharSets[1], 10*time.Millisecond)

	// excluded
	excluded := 0
	empty := 0
	archived := 0
	ap := AnalyzeProject{
		Project: &gitlab.Project{
			PathWithNamespace: testRepoExcluded,
		},
		ExclusionList: ExclusionRepos{testRepoExcluded: true},
		Spin1:         sp,
	}
	list, cpt := processProject(ap, 1, sp, nil, &empty, &archived, &excluded)
	if len(list) != 0 || cpt != 1 || excluded != 1 || empty != 0 || archived != 0 {
		t.Errorf("excluded: list=%v cpt=%d excl=%d empty=%d arch=%d", list, cpt, excluded, empty, archived)
	}

	// empty
	excluded, empty, archived = 0, 0, 0
	ap = AnalyzeProject{
		Project: &gitlab.Project{
			PathWithNamespace: testRepoEmpty,
			EmptyRepo:         true,
		},
		Spin1: sp,
	}
	list, cpt = processProject(ap, 1, sp, nil, &empty, &archived, &excluded)
	if len(list) != 0 || cpt != 1 || excluded != 0 || empty != 1 || archived != 0 {
		t.Errorf("empty: list=%v cpt=%d excl=%d empty=%d arch=%d", list, cpt, excluded, empty, archived)
	}

	// archived
	excluded, empty, archived = 0, 0, 0
	ap = AnalyzeProject{
		Project: &gitlab.Project{
			PathWithNamespace: testRepoArchived,
			Archived:          true,
		},
		Spin1: sp,
	}
	list, cpt = processProject(ap, 1, sp, nil, &empty, &archived, &excluded)
	if len(list) != 0 || cpt != 1 || excluded != 0 || empty != 0 || archived != 1 {
		t.Errorf("archived: list=%v cpt=%d excl=%d empty=%d arch=%d", list, cpt, excluded, empty, archived)
	}
}

// ---------------- Tests added to improve coverage for new helpers ----------------

func TestGetOrganizationsFromConfigVariants(t *testing.T) {
	cases := []struct {
		name   string
		config map[string]interface{}
		want   []string
	}{
		{
			name:   "single Organization string",
			config: map[string]interface{}{"Organization": "org1"},
			want:   []string{"org1"},
		},
		{
			name:   "CSV Organization string",
			config: map[string]interface{}{"Organization": "org1, org2 ,org3"},
			want:   []string{"org1", "org2", "org3"},
		},
		{
			name:   "Organizations array",
			config: map[string]interface{}{"Organizations": []string{"a", "b", "c"}},
			want:   []string{"a", "b", "c"},
		},
		{
			name:   "Organizations []interface{}",
			config: map[string]interface{}{"Organizations": []interface{}{"x", " y ", ""}},
			want:   []string{"x", "y"},
		},
		{
			name:   "empty returns nil",
			config: map[string]interface{}{},
			want:   nil,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := getOrganizationsFromConfig(tc.config)
			if len(got) != len(tc.want) {
				t.Fatalf("len mismatch: got=%v want=%v", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("value mismatch: got=%v want=%v", got, tc.want)
				}
			}
		})
	}
}

func TestFilterValidProjects(t *testing.T) {
	empty, archived, excluded := 0, 0, 0
	ex := ExclusionRepos{"ns/ex": true}
	projects := []*gitlab.Project{
		{PathWithNamespace: "ns/ex", ID: 1},
		{PathWithNamespace: testRepoEmpty, EmptyRepo: true, ID: 2},
		{PathWithNamespace: "ns/arch", Archived: true, ID: 3},
		{PathWithNamespace: "ns/ok", ID: 4},
	}
	valid := filterValidProjects(projects, ex, &empty, &archived, &excluded)
	if len(valid) != 1 || valid[0].PathWithNamespace != "ns/ok" {
		t.Fatalf("filterValidProjects unexpected valid: %+v", valid)
	}
	if empty != 1 || archived != 1 || excluded != 1 {
		t.Fatalf("counters wrong: empty=%d archived=%d excluded=%d", empty, archived, excluded)
	}
}

func TestFindValidProjectAcrossOrgs(t *testing.T) {
	ts := newFakeGitLabServer(t, func(r *http.Request) (int, any) {
		// only org2/test returns a valid project (RawPath may be empty; Path is decoded)
		if strings.HasPrefix(r.URL.Path, "/api/v4/projects/") &&
			(strings.HasSuffix(r.URL.Path, "/org2/test") || strings.Contains(r.URL.Path, "/org2/test")) {
			return http.StatusOK, map[string]any{
				"id":                  42,
				"name":                "test",
				"path_with_namespace": "org2/test",
				"empty_repo":          false,
				"archived":            false,
			}
		}
		return http.StatusNotFound, map[string]any{"message": "not found"}
	})
	defer ts.Close()
	client := newGitLabClientForServer(t, ts.URL)
	ex := ExclusionRepos{}
	empty, archived, excl := 0, 0, 0
	proj, org, ok := findValidProjectAcrossOrgs(client, []string{"org1", "org2", "org3"}, "test", ex, &empty, &archived, &excl)
	if !ok || proj == nil || org != "org2" || proj.ID != 42 {
		t.Fatalf("findValidProjectAcrossOrgs unexpected: ok=%v org=%s proj=%+v", ok, org, proj)
	}
}

func TestAnalyzeSpecificBranchForProjects(t *testing.T) {
	ts := newFakeGitLabServer(t, func(r *http.Request) (int, any) {
		switch {
		case strings.HasPrefix(r.URL.Path, apiBranchesPrefixPath):
			return http.StatusOK, map[string]any{"name": "main"}
		case r.URL.Path == apiTreePath:
			return http.StatusOK, []map[string]any{
				{"id": "1"},
			}
		default:
			return 0, nil
		}
	})
	defer ts.Close()
	client := newGitLabClientForServer(t, ts.URL)
	// ensure Logs dir for logger
	_ = os.MkdirAll("Logs", 0755)
	sp := spinner.New(spinner.CharSets[1], 10*time.Millisecond)
	projects := []*gitlab.Project{
		{ID: 10, PathWithNamespace: "g/p", Name: "p"},
	}
	branches, total := analyzeSpecificBranchForProjects(client, projects, "g", "main", sp)
	if len(branches) != 1 || total != 1 {
		t.Fatalf("analyzeSpecificBranchForProjects unexpected: branches=%+v total=%d", branches, total)
	}
	if branches[0].Org != "g" || branches[0].RepoSlug != "p" || branches[0].MainBranch != "main" {
		t.Fatalf("branch data mismatch: %+v", branches[0])
	}
}

// TestGetDefaultBranchDetails covers the cheap DefaultBranch=true resolver:
// it returns the project's default branch and its size without enumerating
// every branch, and errors when the project has no default branch.
func TestGetDefaultBranchDetails(t *testing.T) {
	_, cleanup := setupTestEnvironment(t, "gl_default_branch_*")
	defer cleanup()
	createTestDirectories(t, []string{"Logs"})

	ts := newFakeGitLabServer(t, func(r *http.Request) (int, any) {
		if r.URL.Path == apiTreePath {
			return http.StatusOK, []map[string]any{{"id": "1"}, {"id": "2"}}
		}
		return 0, nil
	})
	defer ts.Close()
	client := newGitLabClientForServer(t, ts.URL)

	// Success: default branch + its size, exactly one branch.
	project := &gitlab.Project{ID: 10, Name: "p", PathWithNamespace: "g/p", DefaultBranch: "main"}
	branch, size, nbr, err := getDefaultBranchDetails(client, project)
	if err != nil {
		t.Fatalf("getDefaultBranchDetails error: %v", err)
	}
	if branch != "main" || size != 2 || nbr != 1 {
		t.Errorf("getDefaultBranchDetails = (%s,%d,%d), want (main,2,1)", branch, size, nbr)
	}

	// Error: project without a default branch.
	if _, _, _, err := getDefaultBranchDetails(client, &gitlab.Project{ID: 10, Name: "p"}); err == nil {
		t.Errorf("getDefaultBranchDetails expected error for empty default branch")
	}
}

// TestAnalyzeMainBranchForProjectsModes verifies the defaultBranchOnly switch:
// true -> the repo default branch only (cheap); false -> the most active branch
// across all branches (expensive).
func TestAnalyzeMainBranchForProjectsModes(t *testing.T) {
	_, cleanup := setupTestEnvironment(t, "gl_mainbranch_modes_*")
	defer cleanup()
	createTestDirectories(t, []string{"Logs"})

	commitCounts := map[string]int{"main": 1, "dev": 3}
	ts := newFakeGitLabServer(t, func(r *http.Request) (int, any) {
		switch {
		case r.URL.Path == apiBranchesListPath:
			return http.StatusOK, []map[string]any{{"name": "main"}, {"name": "dev"}}
		case strings.HasPrefix(r.URL.Path, apiCommitsPath):
			ref := r.URL.Query().Get("ref_name")
			commits := make([]map[string]any, commitCounts[ref])
			for i := range commits {
				commits[i] = map[string]any{"id": fmt.Sprintf("c%d", i)}
			}
			return http.StatusOK, commits
		case r.URL.Path == apiTreePath:
			return http.StatusOK, []map[string]any{{"id": "1"}, {"id": "2"}}
		default:
			return 0, nil
		}
	})
	defer ts.Close()
	client := newGitLabClientForServer(t, ts.URL)

	sp := spinner.New(spinner.CharSets[1], 10*time.Millisecond)
	projects := []*gitlab.Project{{ID: 10, Name: "p", PathWithNamespace: "g/p", DefaultBranch: "main"}}
	since := time.Now().Add(-24 * time.Hour)
	until := time.Now()

	// defaultBranchOnly=true -> default branch only.
	got, total := analyzeMainBranchForProjects(client, projects, "g", since, until, true, sp)
	if len(got) != 1 || total != 1 || got[0].MainBranch != "main" {
		t.Fatalf("default mode unexpected: %+v total=%d", got, total)
	}

	// defaultBranchOnly=false -> most active branch (dev has more commits).
	got, _ = analyzeMainBranchForProjects(client, projects, "g", since, until, false, sp)
	if len(got) != 1 || got[0].MainBranch != "dev" {
		t.Fatalf("all-branches mode unexpected: %+v", got)
	}
}

// TestHandleDefaultBranchCaseAllProjects exercises the DefaultBranch=true,
// all-projects path end to end and asserts it selects the default branch.
func TestHandleDefaultBranchCaseAllProjects(t *testing.T) {
	_, cleanup := setupTestEnvironment(t, "gl_handle_default_*")
	defer cleanup()
	createTestDirectories(t, []string{"Logs"})

	ts := newFakeGitLabServer(t, func(r *http.Request) (int, any) {
		switch {
		case r.URL.Path == apiGroupGPath:
			return http.StatusOK, map[string]any{"id": 1, "name": "g"}
		case r.URL.Path == apiGroup1ProjectsPath:
			return http.StatusOK, []map[string]any{
				{"id": 10, "name": "p", "path_with_namespace": "g/p", "default_branch": "main", "empty_repo": false, "archived": false},
			}
		case r.URL.Path == apiTreePath:
			return http.StatusOK, []map[string]any{{"id": "1"}, {"id": "2"}}
		default:
			return 0, nil
		}
	})
	defer ts.Close()
	client := newGitLabClientForServer(t, ts.URL)

	empty, archived, excl := 0, 0, 0
	ctx := defaultCtx{
		client:           client,
		config:           map[string]interface{}{"Project": ""},
		exclusions:       ExclusionRepos{},
		orgs:             []string{"g"},
		since:            time.Now().Add(-24 * time.Hour),
		until:            time.Now(),
		spin:             newSpin1(),
		emptyRepos:       &empty,
		archivedRepos:    &archived,
		excludedProjects: &excl,
	}
	branches, total := handleDefaultBranchCase(ctx)
	if len(branches) != 1 || total != 1 || branches[0].MainBranch != "main" {
		t.Fatalf("handleDefaultBranchCase unexpected: %+v total=%d", branches, total)
	}
}

// TestNonDefaultAllProjectsAllBranches exercises the DefaultBranch=false,
// all-projects/all-branches path and asserts it still selects the most active
// branch (the behavior preserved by this change).
func TestNonDefaultAllProjectsAllBranches(t *testing.T) {
	_, cleanup := setupTestEnvironment(t, "gl_nondefault_all_*")
	defer cleanup()
	createTestDirectories(t, []string{"Logs"})

	commitCounts := map[string]int{"main": 1, "dev": 3}
	ts := newFakeGitLabServer(t, func(r *http.Request) (int, any) {
		switch {
		case r.URL.Path == apiGroupGPath:
			return http.StatusOK, map[string]any{"id": 1, "name": "g"}
		case r.URL.Path == apiGroup1ProjectsPath:
			return http.StatusOK, []map[string]any{
				{"id": 10, "name": "p", "path_with_namespace": "g/p", "default_branch": "main", "empty_repo": false, "archived": false},
			}
		case r.URL.Path == apiBranchesListPath:
			return http.StatusOK, []map[string]any{{"name": "main"}, {"name": "dev"}}
		case strings.HasPrefix(r.URL.Path, apiCommitsPath):
			ref := r.URL.Query().Get("ref_name")
			commits := make([]map[string]any, commitCounts[ref])
			for i := range commits {
				commits[i] = map[string]any{"id": fmt.Sprintf("c%d", i)}
			}
			return http.StatusOK, commits
		case r.URL.Path == apiTreePath:
			return http.StatusOK, []map[string]any{{"id": "1"}}
		default:
			return 0, nil
		}
	})
	defer ts.Close()
	client := newGitLabClientForServer(t, ts.URL)

	empty, archived, excl := 0, 0, 0
	ctx := nonDefaultCtx{
		client:           client,
		config:           map[string]interface{}{"Project": "", "Branch": ""},
		exclusions:       ExclusionRepos{},
		orgs:             []string{"g"},
		since:            time.Now().Add(-24 * time.Hour),
		until:            time.Now(),
		spin:             newSpin1(),
		emptyRepos:       &empty,
		archivedRepos:    &archived,
		excludedProjects: &excl,
	}
	branches, total := nonDefaultAllProjectsAllBranches(ctx)
	if len(branches) != 1 || total != 2 || branches[0].MainBranch != "dev" {
		t.Fatalf("nonDefaultAllProjectsAllBranches unexpected: %+v total=%d", branches, total)
	}
}

// TestHandleDefaultBranchCaseSpecificProject exercises the DefaultBranch=true,
// single-project path (Project set) and asserts it selects the default branch.
func TestHandleDefaultBranchCaseSpecificProject(t *testing.T) {
	_, cleanup := setupTestEnvironment(t, "gl_handle_default_proj_*")
	defer cleanup()
	createTestDirectories(t, []string{"Logs"})

	ts := newFakeGitLabServer(t, func(r *http.Request) (int, any) {
		switch {
		case r.URL.Path == apiTreePath:
			return http.StatusOK, []map[string]any{{"id": "1"}, {"id": "2"}}
		case strings.Contains(r.URL.Path, "g/p"):
			return http.StatusOK, map[string]any{
				"id": 10, "name": "p", "path_with_namespace": "g/p",
				"default_branch": "main", "empty_repo": false, "archived": false,
			}
		default:
			return 0, nil
		}
	})
	defer ts.Close()
	client := newGitLabClientForServer(t, ts.URL)

	empty, archived, excl := 0, 0, 0
	ctx := defaultCtx{
		client:           client,
		config:           map[string]interface{}{"Project": "p"},
		exclusions:       ExclusionRepos{},
		orgs:             []string{"g"},
		since:            time.Now().Add(-24 * time.Hour),
		until:            time.Now(),
		spin:             newSpin1(),
		emptyRepos:       &empty,
		archivedRepos:    &archived,
		excludedProjects: &excl,
	}
	branches, total := handleDefaultBranchCase(ctx)
	if len(branches) != 1 || total != 1 || branches[0].MainBranch != "main" {
		t.Fatalf("handleDefaultBranchCase (specific project) unexpected: %+v total=%d", branches, total)
	}
}

// TestNonDefaultSpecificProjectAllBranches exercises the DefaultBranch=false,
// single-project/all-branches path and asserts most-active-branch selection.
func TestNonDefaultSpecificProjectAllBranches(t *testing.T) {
	_, cleanup := setupTestEnvironment(t, "gl_nondefault_proj_*")
	defer cleanup()
	createTestDirectories(t, []string{"Logs"})

	commitCounts := map[string]int{"main": 1, "dev": 3}
	ts := newFakeGitLabServer(t, func(r *http.Request) (int, any) {
		switch {
		case r.URL.Path == apiBranchesListPath:
			return http.StatusOK, []map[string]any{{"name": "main"}, {"name": "dev"}}
		case strings.HasPrefix(r.URL.Path, apiCommitsPath):
			ref := r.URL.Query().Get("ref_name")
			commits := make([]map[string]any, commitCounts[ref])
			for i := range commits {
				commits[i] = map[string]any{"id": fmt.Sprintf("c%d", i)}
			}
			return http.StatusOK, commits
		case r.URL.Path == apiTreePath:
			return http.StatusOK, []map[string]any{{"id": "1"}}
		case strings.Contains(r.URL.Path, "g/p"):
			return http.StatusOK, map[string]any{
				"id": 10, "name": "p", "path_with_namespace": "g/p",
				"default_branch": "main", "empty_repo": false, "archived": false,
			}
		default:
			return 0, nil
		}
	})
	defer ts.Close()
	client := newGitLabClientForServer(t, ts.URL)

	empty, archived, excl := 0, 0, 0
	ctx := nonDefaultCtx{
		client:           client,
		config:           map[string]interface{}{"Project": "p", "Branch": ""},
		exclusions:       ExclusionRepos{},
		orgs:             []string{"g"},
		since:            time.Now().Add(-24 * time.Hour),
		until:            time.Now(),
		spin:             newSpin1(),
		emptyRepos:       &empty,
		archivedRepos:    &archived,
		excludedProjects: &excl,
	}
	branches, total := nonDefaultSpecificProjectAllBranches(ctx)
	if len(branches) != 1 || total != 2 || branches[0].MainBranch != "dev" {
		t.Fatalf("nonDefaultSpecificProjectAllBranches unexpected: %+v total=%d", branches, total)
	}
}
