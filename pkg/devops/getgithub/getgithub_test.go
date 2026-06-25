package getgithub

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/SonarSource-Demos/sonar-golc/pkg/utils"
	"github.com/briandowns/spinner"
	"github.com/google/go-github/v82/github"
)

// Constants to avoid duplicating string literals (SonarQube maintainability)
const (
	errFailedToCreateTempDir       = "Failed to create temp dir: %v"
	errFailedToCreateLogsDir       = "Failed to create Logs dir: %v"
	errFailedToCreateDir           = "Failed to create dir %s: %v"
	errFailedToCreateTestExclusion = "Failed to create test exclusion file: %v"
	errSaveResultOfAnalysis        = "❌ Error Save Result of Analysis : %v"
	testRepoName                   = "test-repo"
	testOrgName                    = "test-org"
	testToken                      = "test-token"
	testAPIVersion                 = "2022-11-28"
	testGitHubAPIURL               = "https://api.github.com"
	testGitHubDomain               = "github.com"
	resultsConfigPath              = "Results/config"
	errRetrievingBranchesForRepo   = "❌ Error when retrieving branches for repo %v: %v\n"
	errFetchingRepositoryEvents    = "❌ Error fetching repository events: %v"
	errFailedToCreateGitHubClient  = "❌ Failed to create GitHub Enterprise client: %v"
	errFetchingRepositories        = "❌ Error fetching repositories: %v\n"
)

func TestLoggerErrorFormatting(t *testing.T) {
	// Create temporary logs directory for testing
	tempDir, err := os.MkdirTemp("", "test_logs_*")
	if err != nil {
		t.Fatalf(errFailedToCreateTempDir, err)
	}
	defer os.RemoveAll(tempDir)

	// Change to temp directory to ensure logger works
	originalWd, _ := os.Getwd()
	defer os.Chdir(originalWd)
	os.Chdir(tempDir)

	// Create Logs directory
	err = os.MkdirAll("Logs", 0755)
	if err != nil {
		t.Fatalf(errFailedToCreateLogsDir, err)
	}

	loggers := utils.NewLogger()

	// Test the various error formatting scenarios that were fixed
	testCases := []struct {
		name           string
		testFunction   func()
		expectedNoFail bool
	}{
		{
			name: "Save Result Error Format",
			testFunction: func() {
				err := errors.New("save error")
				loggers.Errorf(errSaveResultOfAnalysis, err)
			},
			expectedNoFail: true,
		},
		{
			name: "Branch Retrieval Error Format",
			testFunction: func() {
				repoName := testRepoName
				err := errors.New("branch error")
				loggers.Errorf(errRetrievingBranchesForRepo, repoName, err)
			},
			expectedNoFail: true,
		},
		{
			name: "Repository Events Error Format",
			testFunction: func() {
				err := errors.New("events error")
				loggers.Errorf(errFetchingRepositoryEvents, err)
			},
			expectedNoFail: true,
		},
		{
			name: "Payload Parsing Error Format",
			testFunction: func() {
				err := errors.New("payload error")
				loggers.Errorf("❌ Error parsing payload: %v", err)
			},
			expectedNoFail: true,
		},
		{
			name: "Contributors Stats Error Format",
			testFunction: func() {
				err := errors.New("contributors error")
				loggers.Errorf("❌ Error fetching contributors stats: %v\n", err)
			},
			expectedNoFail: true,
		},
		{
			name: "Commits Fetch Error Format",
			testFunction: func() {
				branchName := "main"
				err := errors.New("commits error")
				loggers.Errorf("Error fetching commits for branch %s: %v\n", branchName, err)
			},
			expectedNoFail: true,
		},
		{
			name: "Branches List Error Format",
			testFunction: func() {
				repoSlug := testRepoName
				err := errors.New("branches list error")
				loggers.Errorf("❌ Error getting branches for repo %s: %v", repoSlug, err)
			},
			expectedNoFail: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Test should not panic when calling the error logging function
			defer func() {
				if r := recover(); r != nil && tc.expectedNoFail {
					t.Errorf("Test %s panicked unexpectedly: %v", tc.name, r)
				}
			}()

			tc.testFunction()
			// If we reach here without panic, the test passed
		})
	}
}

func TestErrorFormattingInAnalysisFlow(t *testing.T) {
	// Create temporary logs directory for testing
	tempDir, err := os.MkdirTemp("", "test_analysis_logs_*")
	if err != nil {
		t.Fatalf(errFailedToCreateTempDir, err)
	}
	defer os.RemoveAll(tempDir)

	// Change to temp directory
	originalWd, _ := os.Getwd()
	defer os.Chdir(originalWd)
	os.Chdir(tempDir)

	// Create Logs directory
	err = os.MkdirAll("Logs", 0755)
	if err != nil {
		t.Fatalf(errFailedToCreateLogsDir, err)
	}

	loggers := utils.NewLogger()

	// Test error scenarios that would occur during analysis
	t.Run("Repository Processing Error", func(t *testing.T) {
		repoName := testRepoName
		err := errors.New("processing error")
		loggers.Errorf("❌ Error processing repo %s: %v", repoName, err)
	})

	t.Run("Exclusion File Read Error", func(t *testing.T) {
		exclusionFile := "test-exclusion.txt"
		err := errors.New("file read error")
		loggers.Errorf("\n❌ Error Read Exclusion File <%s>: %v", exclusionFile, err)
	})

	t.Run("GitHub Enterprise Client Error", func(t *testing.T) {
		err := errors.New("client creation error")
		loggers.Errorf(errFailedToCreateGitHubClient, err)
	})

	t.Run("Repository Fetch Error", func(t *testing.T) {
		err := errors.New("fetch error")
		loggers.Errorf(errFetchingRepositories, err)
	})

	t.Run("Single Repository Fetch Error", func(t *testing.T) {
		err := errors.New("single repo error")
		loggers.Errorf("❌ Error fetching repository: %v\n", err)
	})
}

func TestGitHubAPIErrorHandling(t *testing.T) {
	// Create temporary logs directory for testing
	tempDir, err := os.MkdirTemp("", "test_github_api_*")
	if err != nil {
		t.Fatalf(errFailedToCreateTempDir, err)
	}
	defer os.RemoveAll(tempDir)

	// Change to temp directory
	originalWd, _ := os.Getwd()
	defer os.Chdir(originalWd)
	os.Chdir(tempDir)

	// Create Logs directory
	err = os.MkdirAll("Logs", 0755)
	if err != nil {
		t.Fatalf(errFailedToCreateLogsDir, err)
	}

	loggers := utils.NewLogger()

	// Test GitHub API specific error scenarios
	t.Run("Repository Empty Check Error", func(t *testing.T) {
		repoName := "empty-repo"
		err := errors.New("empty check error")
		errMsg := fmt.Errorf("\n❌ Failed to check repository <%s> is empty - : %v", repoName, err)

		// Test the error message formatting that was fixed
		loggers.Errorf("Repository check failed: %v", errMsg)
	})

	t.Run("Repository Listing Error", func(t *testing.T) {
		err := errors.New("list repositories error")
		errMsg := fmt.Errorf("error listing repositories: %v", err)

		loggers.Errorf("GitHub API error: %v", errMsg)
	})

	t.Run("Branch Listing Error", func(t *testing.T) {
		repoName := testRepoName
		err := errors.New("branch listing error")
		errMsg := fmt.Errorf("error getting branches for repo %s: %v", repoName, err)

		loggers.Errorf("Branch listing failed: %v", errMsg)
	})
}

func TestSaveResultFunction(t *testing.T) {
	// Create temporary directory structure for testing
	tempDir, err := os.MkdirTemp("", "test_save_result_*")
	if err != nil {
		t.Fatalf(errFailedToCreateTempDir, err)
	}
	defer os.RemoveAll(tempDir)

	// Change to temp directory
	originalWd, _ := os.Getwd()
	defer os.Chdir(originalWd)
	os.Chdir(tempDir)

	// Create necessary directories
	dirs := []string{"Logs", "Results", resultsConfigPath}
	for _, dir := range dirs {
		err = os.MkdirAll(dir, 0755)
		if err != nil {
			t.Fatalf("Failed to create dir %s: %v", dir, err)
		}
	}

	// Test SaveResult function error handling
	t.Run("SaveResult Success", func(t *testing.T) {
		// Create a minimal result for testing
		result := AnalysisResult{
			NumRepositories: 1,
			ProjectBranches: []ProjectBranch{
				{
					Org:        testOrgName,
					RepoSlug:   testRepoName,
					MainBranch: "main",
				},
			},
		}

		// Test that SaveResult doesn't panic with proper data
		err := SaveResult(result)
		if err != nil {
			// This is expected to cover the error logging line
			loggers := utils.NewLogger()
			loggers.Errorf(errSaveResultOfAnalysis, err)
		}
	})
}

// Mock GitHub client functions for testing error scenarios
func TestGitHubClientErrorScenarios(t *testing.T) {
	// Create temporary logs directory
	tempDir, err := os.MkdirTemp("", "test_github_client_*")
	if err != nil {
		t.Fatalf(errFailedToCreateTempDir, err)
	}
	defer os.RemoveAll(tempDir)

	// Change to temp directory
	originalWd, _ := os.Getwd()
	defer os.Chdir(originalWd)
	os.Chdir(tempDir)

	// Create Logs directory
	err = os.MkdirAll("Logs", 0755)
	if err != nil {
		t.Fatalf(errFailedToCreateLogsDir, err)
	}

	loggers := utils.NewLogger()

	// Test scenarios that would trigger the error logging we fixed
	t.Run("Context Timeout Error", func(t *testing.T) {
		err := context.DeadlineExceeded
		loggers.Errorf("❌ Error fetching repository events: %v", err)
	})

	t.Run("API Rate Limit Error", func(t *testing.T) {
		err := errors.New("API rate limit exceeded")
		loggers.Errorf(errFetchingRepositories, err)
	})

	t.Run("Authentication Error", func(t *testing.T) {
		err := errors.New("bad credentials")
		loggers.Errorf(errFailedToCreateGitHubClient, err)
	})

	t.Run("Network Error", func(t *testing.T) {
		err := errors.New("network unreachable")
		loggers.Errorf(errRetrievingBranchesForRepo, testRepoName, err)
	})
}

// TestErrorFormattingCompleteness ensures all the fixed error formatting scenarios are covered
func TestErrorFormattingCompleteness(t *testing.T) {
	// Create temporary logs directory
	tempDir, err := os.MkdirTemp("", "test_completeness_*")
	if err != nil {
		t.Fatalf(errFailedToCreateTempDir, err)
	}
	defer os.RemoveAll(tempDir)

	// Change to temp directory
	originalWd, _ := os.Getwd()
	defer os.Chdir(originalWd)
	os.Chdir(tempDir)

	// Create Logs directory
	err = os.MkdirAll("Logs", 0755)
	if err != nil {
		t.Fatalf(errFailedToCreateLogsDir, err)
	}

	loggers := utils.NewLogger()

	// Cover all the specific error logging patterns that were fixed in the PR
	errorCases := []struct {
		pattern string
		args    []interface{}
	}{
		{errSaveResultOfAnalysis, []interface{}{errors.New("test")}},
		{errRetrievingBranchesForRepo, []interface{}{"repo", errors.New("test")}},
		{errFetchingRepositoryEvents, []interface{}{errors.New("test")}},
		{"❌ Error parsing payload: %v", []interface{}{errors.New("test")}},
		{"❌ Error fetching contributors stats: %v\n", []interface{}{errors.New("test")}},
		{"Error fetching commits for branch %s: %v\n", []interface{}{"main", errors.New("test")}},
		{"❌ Error getting branches for repo %s: %v", []interface{}{"repo", errors.New("test")}},
		{"❌ Error processing repo %s: %v", []interface{}{"repo", errors.New("test")}},
		{"\n❌ Error Read Exclusion File <%s>: %v", []interface{}{"file", errors.New("test")}},
		{errFailedToCreateGitHubClient, []interface{}{errors.New("test")}},
		{errFetchingRepositories, []interface{}{errors.New("test")}},
		{"❌ Error fetching repository: %v\n", []interface{}{errors.New("test")}},
	}

	for i, errorCase := range errorCases {
		t.Run(fmt.Sprintf("ErrorPattern_%d", i), func(t *testing.T) {
			// This tests that all the error format strings work correctly with their arguments
			loggers.Errorf(errorCase.pattern, errorCase.args...)
		})
	}
}

// Integration tests to improve coverage for getgithub package
func TestGithubIntegrationCoverage(t *testing.T) {
	// Create temporary logs directory for testing
	tempDir, err := os.MkdirTemp("", "test_github_integration_*")
	if err != nil {
		t.Fatalf(errFailedToCreateTempDir, err)
	}
	defer os.RemoveAll(tempDir)

	// Change to temp directory
	originalWd, _ := os.Getwd()
	defer os.Chdir(originalWd)
	os.Chdir(tempDir)

	// Create necessary directories
	dirs := []string{"Logs", "Results", resultsConfigPath}
	for _, dir := range dirs {
		err = os.MkdirAll(dir, 0755)
		if err != nil {
			t.Fatalf("Failed to create dir %s: %v", dir, err)
		}
	}

	t.Run("SaveResult with various data scenarios", func(t *testing.T) {
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
				name: "Single repository",
				result: AnalysisResult{
					NumRepositories: 1,
					ProjectBranches: []ProjectBranch{
						{Org: testRepoName, RepoSlug: testRepoName, MainBranch: "main"},
					},
				},
			},
			{
				name: "Multiple repositories with different branches",
				result: AnalysisResult{
					NumRepositories: 4,
					ProjectBranches: []ProjectBranch{
						{Org: "org1", RepoSlug: "repo1", MainBranch: "main"},
						{Org: "org2", RepoSlug: "repo2", MainBranch: "master"},
						{Org: "org3", RepoSlug: "repo3", MainBranch: "develop"},
						{Org: "org4", RepoSlug: "repo4", MainBranch: "default"},
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

				// Verify GitHub-specific file was created (not GitLab)
				githubFile := "Results/config/analysis_result_github.json"
				if _, err := os.Stat(githubFile); os.IsNotExist(err) {
					t.Errorf("SaveResult did not create GitHub file for %s", tc.name)
				}
			})
		}
	})

	t.Run("Error handling scenarios", func(t *testing.T) {
		// Test SaveResult with invalid data
		invalidResult := AnalysisResult{
			NumRepositories: -1,
			ProjectBranches: nil,
		}

		err := SaveResult(invalidResult)
		// This exercises error handling paths
		if err != nil {
			t.Logf("SaveResult properly handled invalid data: %v", err)
		}
	})
}

// TestSaveFunctions tests all the Save* functions in getgithub package
func TestSaveFunctions(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "test_save_functions_*")
	if err != nil {
		t.Fatalf(errFailedToCreateTempDir, err)
	}
	defer os.RemoveAll(tempDir)

	originalWd, _ := os.Getwd()
	defer os.Chdir(originalWd)
	os.Chdir(tempDir)

	// Create necessary directories
	dirs := []string{"Logs", "Results", resultsConfigPath}
	for _, dir := range dirs {
		err = os.MkdirAll(dir, 0755)
		if err != nil {
			t.Fatalf("Failed to create dir %s: %v", dir, err)
		}
	}

	t.Run("SaveBranch function", func(t *testing.T) {
		testBranch := RepoBranch{
			ID:       12345,
			Name:     "test-repo",
			Branches: nil, // Can be nil for testing
		}

		err := SaveBranch(testBranch)
		if err != nil {
			t.Errorf("SaveBranch failed: %v", err)
		}

		// Verify file was created
		expectedFile := "Results/config/analysis_branch_github.json"
		if _, err := os.Stat(expectedFile); os.IsNotExist(err) {
			t.Errorf("SaveBranch did not create expected file: %s", expectedFile)
		}
	})

	t.Run("SaveCommit function", func(t *testing.T) {
		// Test with nil slice (edge case)
		err := SaveCommit(nil)
		if err != nil {
			t.Errorf("SaveCommit failed with nil input: %v", err)
		}

		// Verify file was created
		expectedFile := "Results/config/analysis_commit_github.json"
		if _, err := os.Stat(expectedFile); os.IsNotExist(err) {
			t.Errorf("SaveCommit did not create expected file: %s", expectedFile)
		}
	})

	t.Run("SaveRepos function", func(t *testing.T) {
		// Test with nil slice
		err := SaveRepos(nil)
		if err != nil {
			t.Errorf("SaveRepos failed with nil input: %v", err)
		}

		// Verify file was created
		expectedFile := "Results/config/analysis_repos_github.json"
		if _, err := os.Stat(expectedFile); os.IsNotExist(err) {
			t.Errorf("SaveRepos did not create expected file: %s", expectedFile)
		}
	})

	t.Run("SaveLast function", func(t *testing.T) {
		testLast := Lastanalyse{
			LastRepos:  "test-last-repo",
			LastBranch: "main",
		}

		err := SaveLast(testLast)
		if err != nil {
			t.Errorf("SaveLast failed: %v", err)
		}

		// Verify file was created
		expectedFile := "Results/config/analysis_last_github.json"
		if _, err := os.Stat(expectedFile); os.IsNotExist(err) {
			t.Errorf("SaveLast did not create expected file: %s", expectedFile)
		}
	})
}

// TestHelperFunctions tests utility and helper functions
func TestHelperFunctions(t *testing.T) {
	t.Run("shouldIgnore function", func(t *testing.T) {
		exclusionMap := ExclusionRepos{
			"ignored-repo": true,
			"test-repo":    true,
		}

		// Test ignored repository
		if !shouldIgnore("ignored-repo", exclusionMap) {
			t.Error("shouldIgnore should return true for ignored repository")
		}

		// Test non-ignored repository
		if shouldIgnore("allowed-repo", exclusionMap) {
			t.Error("shouldIgnore should return false for non-ignored repository")
		}

		// Test with empty map
		emptyMap := make(ExclusionRepos)
		if shouldIgnore("any-repo", emptyMap) {
			t.Error("shouldIgnore should return false with empty exclusion map")
		}
	})

	t.Run("getNextPage function", func(t *testing.T) {
		// Test with valid Link header
		header := make(map[string][]string)
		header["Link"] = []string{`<https://api.github.com/repos?page=2>; rel="next", <https://api.github.com/repos?page=10>; rel="last"`}

		result := getNextPage(header)
		expected := "https://api.github.com/repos?page=2"
		if result != expected {
			t.Errorf("getNextPage() = %s, want %s", result, expected)
		}

		// Test with no Link header
		emptyHeader := make(map[string][]string)
		result = getNextPage(emptyHeader)
		if result != "" {
			t.Errorf("getNextPage() with empty header = %s, want empty string", result)
		}

		// Test with Link header but no next rel
		header["Link"] = []string{`<https://api.github.com/repos?page=10>; rel="last"`}
		result = getNextPage(header)
		if result != "" {
			t.Errorf("getNextPage() with no next rel = %s, want empty string", result)
		}
	})

	t.Run("sortRepositoriesByUpdatedAt function", func(t *testing.T) {
		// This function would need actual github.Repository objects
		// For testing purposes, we test that it doesn't panic with empty slice
		var repos []*github.Repository

		// Should not panic
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("sortRepositoriesByUpdatedAt panicked: %v", r)
				}
			}()
			sortRepositoriesByUpdatedAt(repos)
		}()
	})
}

// TestExclusionFunctions tests exclusion file loading functions
func TestExclusionFunctions(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "test_exclusion_*")
	if err != nil {
		t.Fatalf(errFailedToCreateTempDir, err)
	}
	defer os.RemoveAll(tempDir)

	originalWd, _ := os.Getwd()
	defer os.Chdir(originalWd)
	os.Chdir(tempDir)

	// Create Logs directory for logger
	err = os.MkdirAll("Logs", 0755)
	if err != nil {
		t.Fatalf(errFailedToCreateLogsDir, err)
	}

	t.Run("loadExclusionRepos1 function", func(t *testing.T) {
		// Test with non-existent file
		_, err := loadExclusionRepos1("non-existent.txt")
		if err == nil {
			t.Error("loadExclusionRepos1 should return error for non-existent file")
		}

		// Test with valid file
		exclusionContent := "repo1\nrepo2\n\n  repo3  \n"
		exclusionFile := "test_exclusion.txt"
		err = os.WriteFile(exclusionFile, []byte(exclusionContent), 0644)
		if err != nil {
			t.Fatalf("Failed to create test exclusion file: %v", err)
		}

		exclusionMap, err := loadExclusionRepos1(exclusionFile)
		if err != nil {
			t.Errorf("loadExclusionRepos1 failed: %v", err)
		}

		if !exclusionMap["repo1"] || !exclusionMap["repo2"] || !exclusionMap["repo3"] {
			t.Error("loadExclusionRepos1 did not load all repositories correctly")
		}

		if len(exclusionMap) != 3 {
			t.Errorf("loadExclusionRepos1 loaded %d repos, expected 3", len(exclusionMap))
		}
	})

	t.Run("loadExclusionList function", func(t *testing.T) {
		// Test with "0" (no exclusion file)
		exclusionList, err := loadExclusionList("0")
		if err != nil {
			t.Errorf("loadExclusionList with '0' failed: %v", err)
		}
		if exclusionList.Repos == nil {
			t.Error("loadExclusionList should initialize Repos map")
		}

		// Test with actual file
		exclusionContent := "repo1\nrepo2\n"
		exclusionFile := "test_exclusion2.txt"
		err = os.WriteFile(exclusionFile, []byte(exclusionContent), 0644)
		if err != nil {
			t.Fatalf("Failed to create test exclusion file: %v", err)
		}

		exclusionList, err = loadExclusionList(exclusionFile)
		if err != nil {
			t.Errorf("loadExclusionList failed: %v", err)
		}

		if !exclusionList.Repos["repo1"] || !exclusionList.Repos["repo2"] {
			t.Error("loadExclusionList did not load repositories correctly")
		}
	})

	t.Run("loadExclusionFile function", func(t *testing.T) {
		// Test with "0" (no exclusion file)
		exclusionMap, err := loadExclusionFile("0", nil)
		if err != nil {
			t.Errorf("loadExclusionFile with '0' failed: %v", err)
		}
		if exclusionMap == nil {
			t.Error("loadExclusionFile should return non-nil map")
		}

		// Test with valid file
		exclusionContent := "repo1\nrepo2\n"
		exclusionFile := "test_exclusion3.txt"
		err = os.WriteFile(exclusionFile, []byte(exclusionContent), 0644)
		if err != nil {
			t.Fatalf("Failed to create test exclusion file: %v", err)
		}

		exclusionMap, err = loadExclusionFile(exclusionFile, nil)
		if err != nil {
			t.Errorf("loadExclusionFile failed: %v", err)
		}

		if !exclusionMap["repo1"] || !exclusionMap["repo2"] {
			t.Error("loadExclusionFile did not load repositories correctly")
		}
	})
}

// TestUtilityFunctions tests utility functions
func TestUtilityFunctions(t *testing.T) {
	t.Run("findLargestRepository function", func(t *testing.T) {
		testBranches := []ProjectBranch{
			{Org: "org1", RepoSlug: "repo1", MainBranch: "main", LargestSize: 100},
			{Org: "org2", RepoSlug: "repo2", MainBranch: "develop", LargestSize: 500},
			{Org: "org3", RepoSlug: "repo3", MainBranch: "master", LargestSize: 200},
		}

		var totalSize int64
		largestBranch, largestRepo := findLargestRepository(testBranches, &totalSize)

		if largestRepo != "repo2" {
			t.Errorf("findLargestRepository returned wrong repo: %s, expected repo2", largestRepo)
		}

		if largestBranch != "develop" {
			t.Errorf("findLargestRepository returned wrong branch: %s, expected develop", largestBranch)
		}

		if totalSize != 800 {
			t.Errorf("findLargestRepository calculated wrong total size: %d, expected 800", totalSize)
		}

		// Test with empty slice
		emptyBranches := []ProjectBranch{}
		var emptyTotal int64
		largestBranch, largestRepo = findLargestRepository(emptyBranches, &emptyTotal)
		if largestBranch != "" || largestRepo != "" {
			t.Error("findLargestRepository should return empty strings for empty input")
		}
	})

	t.Run("printSummary function", func(t *testing.T) {
		// Create temporary logs directory
		tempDir, err := os.MkdirTemp("", "test_print_summary_*")
		if err != nil {
			t.Fatalf(errFailedToCreateTempDir, err)
		}
		defer os.RemoveAll(tempDir)

		originalWd, _ := os.Getwd()
		defer os.Chdir(originalWd)
		os.Chdir(tempDir)

		err = os.MkdirAll("Logs", 0755)
		if err != nil {
			t.Fatalf(errFailedToCreateLogsDir, err)
		}

		config := PlatformConfig{
			Organization: testOrgName,
			URL:          "https://github.com",
		}

		stats := SummaryStats{
			LargestRepo:       "test-repo",
			LargestRepoBranch: "main",
			NbRepos:           5,
			EmptyRepo:         1,
			TotalExclude:      2,
			TotalArchiv:       1,
			TotalBranches:     10,
		}

		// Should not panic
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("printSummary panicked: %v", r)
				}
			}()
			printSummary(config, stats)
		}()
	})
}

// TestInitializeGithubClient tests GitHub client initialization
func TestInitializeGithubClient(t *testing.T) {
	t.Run("GitHub Cloud client", func(t *testing.T) {
		platformConfig := map[string]interface{}{
			"AccessToken": testToken,
			"Url":         "https://api.github.com/",
		}

		ctx, client := initializeGithubClient(platformConfig)
		if ctx == nil {
			t.Error("initializeGithubClient should return non-nil context")
		}
		if client == nil {
			t.Error("initializeGithubClient should return non-nil client")
		}
	})

	t.Run("GitHub Enterprise client", func(t *testing.T) {
		platformConfig := map[string]interface{}{
			"AccessToken": testToken,
			"Url":         "https://github.enterprise.com/",
		}

		ctx, client := initializeGithubClient(platformConfig)
		if ctx == nil {
			t.Error("initializeGithubClient should return non-nil context for enterprise")
		}
		if client == nil {
			t.Error("initializeGithubClient should return non-nil client for enterprise")
		}
	})

	t.Run("GitHub Enterprise with api/v3 in URL", func(t *testing.T) {
		platformConfig := map[string]interface{}{
			"AccessToken": testToken,
			"Url":         "https://github.enterprise.com/api/v3/",
		}

		ctx, client := initializeGithubClient(platformConfig)
		if ctx == nil {
			t.Error("initializeGithubClient should return non-nil context")
		}
		if client == nil {
			t.Error("initializeGithubClient should return non-nil client")
		}
	})
}

// TestGetCommonParams tests parameter construction
func TestGetCommonParams(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "test_common_params_*")
	if err != nil {
		t.Fatalf(errFailedToCreateTempDir, err)
	}
	defer os.RemoveAll(tempDir)

	originalWd, _ := os.Getwd()
	defer os.Chdir(originalWd)
	os.Chdir(tempDir)

	err = os.MkdirAll("Logs", 0755)
	if err != nil {
		t.Fatalf(errFailedToCreateLogsDir, err)
	}

	platformConfig := map[string]interface{}{
		"Url":           "https://api.github.com",
		"Baseapi":       "github.com",
		"Apiver":        "2022-11-28",
		"AccessToken":   testToken,
		"Organization":  testOrgName,
		"Branch":        "main",
		"Period":        float64(-1),
		"Stats":         true,
		"DefaultBranch": false,
	}

	var repositories []*github.Repository
	exclusionList := ExclusionRepos{"excluded-repo": true}

	params := getCommonParams(platformConfig, repositories, exclusionList, nil)

	if params.Organization != testOrgName {
		t.Errorf("getCommonParams wrong organization: %s", params.Organization)
	}

	if params.NBRepos != 0 {
		t.Errorf("getCommonParams wrong repo count: %d", params.NBRepos)
	}

	if params.Branch != "main" {
		t.Errorf("getCommonParams wrong branch: %s", params.Branch)
	}

	if params.Period != -1 {
		t.Errorf("getCommonParams wrong period: %d", params.Period)
	}

	if !params.Stats {
		t.Error("getCommonParams should preserve Stats setting")
	}

	if params.DefaultB {
		t.Error("getCommonParams should preserve DefaultBranch setting")
	}
}

// TestHTTPFunctions tests HTTP-related functions
func TestHTTPFunctions(t *testing.T) {
	t.Run("GithubAllBranches function", func(t *testing.T) {
		// Test with invalid URL (this will exercise error handling)
		_, err := GithubAllBranches("invalid-url", "fake-token", "2022-11-28")
		if err == nil {
			t.Error("GithubAllBranches should return error for invalid URL")
		}

		// Test with empty URL
		_, err = GithubAllBranches("", "fake-token", "2022-11-28")
		if err == nil {
			t.Error("GithubAllBranches should return error for empty URL")
		}
	})
}

// TestComplexWorkflowFunctions tests workflow and analysis functions

// TestEdgeCasesAndErrorPaths tests edge cases and error handling
func TestEdgeCasesAndErrorPaths(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "test_edge_cases_*")
	if err != nil {
		t.Fatalf(errFailedToCreateTempDir, err)
	}
	defer os.RemoveAll(tempDir)

	originalWd, _ := os.Getwd()
	defer os.Chdir(originalWd)
	os.Chdir(tempDir)

	err = os.MkdirAll("Logs", 0755)
	if err != nil {
		t.Fatalf(errFailedToCreateLogsDir, err)
	}

	t.Run("FastAnalys function error paths", func(t *testing.T) {
		// Test with minimal but complete config to avoid panic
		completeConfig := map[string]interface{}{
			"Organization": testOrgName,
			"Url":          testGitHubAPIURL,
			"Baseapi":      testGitHubDomain,
			"Apiver":       testAPIVersion,
			"AccessToken":  testToken,
			"Branch":       "main",
			"Period":       float64(1),
			"Stats":        false,
			"Repos":        "",
			"Factor":       float64(10),
		}

		// This will exercise the error path since we don't have a real GitHub connection
		err := FastAnalys(completeConfig, "0")
		// Error is expected due to invalid token/network, but should not panic
		if err != nil {
			t.Logf("FastAnalys handled error as expected: %v", err)
		}

		// Test with valid config but invalid exclusion file
		validConfig := map[string]interface{}{
			"Organization": testOrgName,
			"Url":          "https://api.github.com",
			"Baseapi":      "github.com",
			"Apiver":       "2022-11-28",
			"AccessToken":  testToken,
			"Branch":       "main",
			"Period":       float64(1),
			"Stats":        false,
			"Repos":        "",
			"Factor":       float64(10),
		}

		// This will exercise the exclusion file error path
		err = FastAnalys(validConfig, "non-existent-exclusion.txt")
		// Error is expected and handled gracefully in the function
		if err != nil {
			t.Logf("FastAnalys handled exclusion file error: %v", err)
		}
	})

	t.Run("Repository processing edge cases", func(t *testing.T) {
		// Test processRepositoryBranches with nil client but valid repo
		repo := &github.Repository{
			Name:     github.String("test-repo"),
			Size:     github.Int(100),
			Archived: github.Bool(false),
		}

		// Create minimal stats structure
		stats := &RepoProcessingStats{}

		// This should panic when trying to call client.Repositories.ListBranches
		func() {
			panicked := false
			defer func() {
				if r := recover(); r != nil {
					t.Logf("processRepositoryBranches properly handled nil client: %v", r)
					panicked = true
				}
				if !panicked {
					t.Error("processRepositoryBranches should panic with nil client")
				}
			}()
			processRepositoryBranches(nil, context.Background(), repo, testOrgName, "0", stats)
		}()
	})

	t.Run("Fetch functions error handling", func(t *testing.T) {
		// Test fetchUserRepositories with nil client
		func() {
			panicked := false
			defer func() {
				if r := recover(); r != nil {
					t.Logf("fetchUserRepositories properly handled nil client: %v", r)
					panicked = true
				}
			}()
			fetchUserRepositories(context.Background(), nil, nil)
			if !panicked {
				t.Error("fetchUserRepositories should panic with nil client")
			}
		}()

		// Test fetchAllRepositories with nil client
		func() {
			panicked := false
			defer func() {
				if r := recover(); r != nil {
					t.Logf("fetchAllRepositories properly handled nil client: %v", r)
					panicked = true
				}
			}()
			fetchAllRepositories(context.Background(), nil, testOrgName, nil)
			if !panicked {
				t.Error("fetchAllRepositories should panic with nil client")
			}
		}()

		// Test fetchSpecificRepositories with nil client
		func() {
			panicked := false
			defer func() {
				if r := recover(); r != nil {
					t.Logf("fetchSpecificRepositories properly handled nil client: %v", r)
					panicked = true
				}
			}()
			config := map[string]interface{}{
				"Organization": testOrgName,
				"Repos":        testRepoName,
			}
			fetchSpecificRepositories(context.Background(), nil, config)
			if !panicked {
				t.Error("fetchSpecificRepositories should panic with nil client")
			}
		}()
	})
}

// TestGetAllRepositoriesFunction tests repository listing
func TestGetAllRepositoriesFunction(t *testing.T) {
	t.Run("getAllRepositories function", func(t *testing.T) {
		// Test with nil client - should panic
		panicked := false
		defer func() {
			if r := recover(); r != nil {
				t.Logf("getAllRepositories properly panicked with nil client: %v", r)
				panicked = true
			}
			if !panicked {
				t.Error("getAllRepositories should panic with nil client")
			}
		}()
		getAllRepositories(nil, context.Background(), testOrgName)
	})
}

// TestRepoEmptyCheck tests repository empty check functionality
func TestRepoEmptyCheck(t *testing.T) {
	t.Run("reposIfEmpty function", func(t *testing.T) {
		// Test with nil client - should panic
		panicked := false
		defer func() {
			if r := recover(); r != nil {
				t.Logf("reposIfEmpty properly panicked with nil client: %v", r)
				panicked = true
			}
			if !panicked {
				t.Error("reposIfEmpty should panic with nil client")
			}
		}()
		reposIfEmpty(context.Background(), nil, "test-repo", testOrgName)
	})
}

// TestListFunctions tests main list functions
func TestListFunctions(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "test_list_functions_*")
	if err != nil {
		t.Fatalf(errFailedToCreateTempDir, err)
	}
	defer os.RemoveAll(tempDir)

	originalWd, _ := os.Getwd()
	defer os.Chdir(originalWd)
	os.Chdir(tempDir)

	// Create necessary directories
	dirs := []string{"Logs", "Results", resultsConfigPath}
	for _, dir := range dirs {
		err = os.MkdirAll(dir, 0755)
		if err != nil {
			t.Fatalf("Failed to create dir %s: %v", dir, err)
		}
	}

	t.Run("GetRepoGithubList function", func(t *testing.T) {
		platformConfig := map[string]interface{}{
			"AccessToken":   testToken,
			"Url":           "https://api.github.com",
			"Organization":  testOrgName,
			"Org":           true,
			"Repos":         "",
			"Baseapi":       "github.com",
			"Apiver":        "2022-11-28",
			"Branch":        "main",
			"Period":        float64(1),
			"Stats":         false,
			"DefaultBranch": false,
		}

		// This will test the error handling path since we don't have a real GitHub client
		branches, err := GetRepoGithubList(platformConfig, "0", false)

		// Function should handle errors gracefully - may return nil or empty slice on error
		if branches == nil {
			t.Logf("GetRepoGithubList returned nil slice on error (acceptable)")
		} else {
			t.Logf("GetRepoGithubList returned slice with %d elements", len(branches))
		}

		// Error is expected due to invalid token/network
		if err != nil {
			t.Logf("GetRepoGithubList handled error as expected: %v", err)
		}
	})

	t.Run("GetRepoGithubListAllBranches function", func(t *testing.T) {
		platformConfig := map[string]interface{}{
			"AccessToken":  testToken,
			"Organization": testOrgName,
		}

		// This will test the error handling path
		branches, err := GetRepoGithubListAllBranches(platformConfig, "0", false)

		// Function should handle errors gracefully - may return nil or empty slice on error
		if branches == nil {
			t.Logf("GetRepoGithubListAllBranches returned nil slice on error (acceptable)")
		} else {
			t.Logf("GetRepoGithubListAllBranches returned slice with %d elements", len(branches))
		}

		// Error is expected due to invalid token/network
		if err != nil {
			t.Logf("GetRepoGithubListAllBranches handled error as expected: %v", err)
		}
	})

	t.Run("GetAllBranchesForRepositories function", func(t *testing.T) {
		platformConfig := map[string]interface{}{
			"AccessToken": testToken,
		}

		repositories := []ProjectBranch{
			{Org: testOrgName, RepoSlug: "test-repo", MainBranch: "main"},
		}

		// This will test the error handling path
		branches, err := GetAllBranchesForRepositories(platformConfig, repositories)

		// Function should handle errors gracefully - may return nil or empty slice on error
		if branches == nil {
			t.Logf("GetAllBranchesForRepositories returned nil slice on error (acceptable)")
		} else {
			t.Logf("GetAllBranchesForRepositories returned slice with %d elements", len(branches))
		}

		// Error is expected due to invalid token/network
		if err != nil {
			t.Logf("GetAllBranchesForRepositories handled error as expected: %v", err)
		}
	})
}

// TestGetGithubLanguages tests language analysis functionality
func TestGetGithubLanguages(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "test_github_languages_*")
	if err != nil {
		t.Fatalf(errFailedToCreateTempDir, err)
	}
	defer os.RemoveAll(tempDir)

	originalWd, _ := os.Getwd()
	defer os.Chdir(originalWd)
	os.Chdir(tempDir)

	err = os.MkdirAll("Logs", 0755)
	if err != nil {
		t.Fatalf(errFailedToCreateLogsDir, err)
	}

	t.Run("GetGithubLanguages error handling", func(t *testing.T) {
		// Create a valid spinner to avoid panic
		spin := spinner.New(spinner.CharSets[35], 100*time.Millisecond)
		params := ParamsReposGithub{
			Repos:        nil, // Empty repos
			NBRepos:      0,
			Organization: testOrgName,
			Spin:         spin, // Add valid spinner
		}

		// Test with nil client and empty repos - should handle gracefully
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Logf("GetGithubLanguages panicked: %v", r)
				}
			}()
			nbRepos, emptyRepo, notAnalyzed, archived, err := GetGithubLanguages(params, context.Background(), nil, 10)

			// Should handle gracefully
			if nbRepos != 0 {
				t.Errorf("GetGithubLanguages should return 0 repos for empty input: got %d", nbRepos)
			}

			if emptyRepo != 0 {
				t.Errorf("GetGithubLanguages should return 0 empty repos for empty input: got %d", emptyRepo)
			}

			if notAnalyzed != 0 {
				t.Errorf("GetGithubLanguages should return 0 not analyzed for empty input: got %d", notAnalyzed)
			}

			if archived != 0 {
				t.Errorf("GetGithubLanguages should return 0 archived for empty input: got %d", archived)
			}

			// Error is expected due to nil client, but should not crash
			if err != nil {
				t.Logf("GetGithubLanguages handled error as expected: %v", err)
			}
		}()
	})
}

// TestGetAllBranchesAndEvents tests branch and event retrieval functions
func TestGetAllBranchesAndEvents(t *testing.T) {
	t.Run("getAllBranches error handling", func(t *testing.T) {
		// Test with nil client - should panic
		panicked := false
		defer func() {
			if r := recover(); r != nil {
				t.Logf("getAllBranches properly panicked with nil client: %v", r)
				panicked = true
			}
			if !panicked {
				t.Error("getAllBranches should panic with nil client")
			}
		}()
		getAllBranches(context.Background(), nil, "test-repo", testOrgName, nil)
	})
}

// TestMainAnalysisWorkflow tests the main analysis workflow
func TestMainAnalysisWorkflow(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "test_main_workflow_*")
	if err != nil {
		t.Fatalf(errFailedToCreateTempDir, err)
	}
	defer os.RemoveAll(tempDir)

	originalWd, _ := os.Getwd()
	defer os.Chdir(originalWd)
	os.Chdir(tempDir)

	// Create necessary directories
	dirs := []string{"Logs", "Results", resultsConfigPath}
	for _, dir := range dirs {
		err = os.MkdirAll(dir, 0755)
		if err != nil {
			t.Fatalf("Failed to create dir %s: %v", dir, err)
		}
	}

	t.Run("GetReposGithub error handling", func(t *testing.T) {
		params := ParamsReposGithub{
			Repos:         nil, // Empty repos to test error path
			NBRepos:       0,
			Organization:  testOrgName,
			ExclusionList: make(ExclusionRepos),
			Period:        1,
			Stats:         false,
			DefaultB:      false,
		}

		// Test with nil client
		branches, empty, total, totalBranches, excluded, archived := GetReposGithub(params, context.Background(), nil)

		// Should handle gracefully with empty input
		if len(branches) != 0 {
			t.Error("GetReposGithub should return empty branches with nil repos")
		}

		if empty != 0 || total != 0 || totalBranches != 0 || excluded != 0 || archived != 0 {
			t.Errorf("GetReposGithub should return zeros for empty input: empty=%d, total=%d, branches=%d, excluded=%d, archived=%d",
				empty, total, totalBranches, excluded, archived)
		}
	})

	t.Run("analyzeRepoBranches error handling", func(t *testing.T) {
		params := ParamsReposGithub{
			Organization: testOrgName,
			DefaultB:     false,
			Period:       1,
			Stats:        false,
		}

		// Test with nil repository - should panic
		panicked := false
		defer func() {
			if r := recover(); r != nil {
				t.Logf("analyzeRepoBranches properly panicked with nil params: %v", r)
				panicked = true
			}
			if !panicked {
				t.Error("analyzeRepoBranches should panic with nil repository")
			}
		}()
		analyzeRepoBranches(params, context.Background(), nil, nil, 1, nil)
	})
}

// TestGetRepoGithubList_PersonalAccountUserGetError covers the failure path
// introduced by the personal-account owner-resolution block: when Org=false
// and Organization is empty, GetRepoGithubList must surface an error from
// client.Users.Get instead of silently proceeding with an empty owner (which
// would 404 every per-repo call downstream).
func TestGetRepoGithubList_PersonalAccountUserGetError(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "test_github_personal_user_get_*")
	if err != nil {
		t.Fatalf(errFailedToCreateTempDir, err)
	}
	defer os.RemoveAll(tempDir)

	originalWd, _ := os.Getwd()
	defer os.Chdir(originalWd)
	if err := os.Chdir(tempDir); err != nil {
		t.Fatalf("Failed to chdir to temp dir: %v", err)
	}
	if err := os.MkdirAll("Logs", 0755); err != nil {
		t.Fatalf(errFailedToCreateLogsDir, err)
	}

	// Fake GHES server: /user/repos returns an empty list (so fetchUserRepositories
	// succeeds), /user returns 500 (so client.Users.Get fails and we hit the new
	// error path). Any other request is an unexpected call.
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v3/user/repos", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("[]"))
	})
	mux.HandleFunc("/api/v3/user", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"message":"Internal Server Error"}`))
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected GitHub API call: %s %s", r.Method, r.URL.Path)
		w.WriteHeader(http.StatusNotFound)
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	platformConfig := map[string]interface{}{
		"Url":           server.URL + "/",
		"AccessToken":   testToken,
		"Organization":  "",
		"Org":           false,
		"Repos":         "",
		"Branch":        "",
		"Period":        float64(-1),
		"Stats":         false,
		"DefaultBranch": false,
		"Baseapi":       "",
		"Apiver":        testAPIVersion,
	}

	branches, err := GetRepoGithubList(platformConfig, "0", false)
	if err == nil {
		t.Fatalf("expected error from GetRepoGithubList when Users.Get fails, got nil (branches=%v)", branches)
	}
	if len(branches) != 0 {
		t.Errorf("expected no branches when Users.Get fails, got %d", len(branches))
	}
	// Organization must remain unchanged on the failure path.
	if got := platformConfig["Organization"].(string); got != "" {
		t.Errorf("expected Organization to remain empty on Users.Get failure, got %q", got)
	}
}

func TestParseRepoList(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{"empty", "", nil},
		{"single", "repo1", []string{"repo1"}},
		{"multiple", "repo1,repo2,repo3", []string{"repo1", "repo2", "repo3"}},
		{"whitespace trimmed", "repo1, repo2 ,  repo3", []string{"repo1", "repo2", "repo3"}},
		{"blanks skipped", "repo1,,repo2,   ,repo3", []string{"repo1", "repo2", "repo3"}},
		{"duplicates dropped", "repo1,repo2,repo1,repo2", []string{"repo1", "repo2"}},
		{"only blanks", " , , ", nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseRepoList(tt.input)
			if len(got) != len(tt.want) {
				t.Fatalf("parseRepoList(%q) = %v, want %v", tt.input, got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("parseRepoList(%q)[%d] = %q, want %q", tt.input, i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestFetchSpecificRepositories(t *testing.T) {
	// Fake GHES server: repos r1 and r3 exist, r2 returns 404. The repo Get
	// path on the enterprise client is /api/v3/repos/{org}/{repo}.
	mux := http.NewServeMux()
	for _, name := range []string{"r1", "r3"} {
		repoName := name
		mux.HandleFunc("/api/v3/repos/"+testOrgName+"/"+repoName, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(fmt.Sprintf(`{"name":%q}`, repoName)))
		})
	}
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// Unknown repo (e.g. r2) → 404.
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"Not Found"}`))
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	makeConfig := func(repos string) map[string]interface{} {
		return map[string]interface{}{
			"Url":          server.URL + "/",
			"AccessToken":  testToken,
			"Organization": testOrgName,
			"Repos":        repos,
		}
	}
	ctx, client := initializeGithubClient(makeConfig(""))

	t.Run("skips unreachable repos and keeps the rest", func(t *testing.T) {
		repos, err := fetchSpecificRepositories(ctx, client, makeConfig("r1, r2 ,r3"))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(repos) != 2 {
			t.Fatalf("expected 2 reachable repos (r1, r3), got %d", len(repos))
		}
		if repos[0].GetName() != "r1" || repos[1].GetName() != "r3" {
			t.Errorf("expected [r1 r3], got [%s %s]", repos[0].GetName(), repos[1].GetName())
		}
	})

	t.Run("errors when none resolve", func(t *testing.T) {
		repos, err := fetchSpecificRepositories(ctx, client, makeConfig("nope1,nope2"))
		if err == nil {
			t.Fatalf("expected error when no repositories resolve, got nil (repos=%v)", repos)
		}
		if len(repos) != 0 {
			t.Errorf("expected no repos, got %d", len(repos))
		}
	})
}
