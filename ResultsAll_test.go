//go:build resultsall
// +build resultsall

package main

import (
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Constants to avoid duplicating string literals (SonarQube maintainability)
const (
	testResultsDirRA            = "Results"
	testConfigDir               = "Results/config"
	testByFileReportDir         = "Results/byfile-report"
	testByLanguageReportDir     = "Results/bylanguage-report"
	testGlobalReportFile        = "Results/GlobalReport.json"
	testCodeLinesByLanguageFile = "Results/code_lines_by_language.json"
	testAnalysisResultFile      = "Results/config/analysis_result_github.json"
	testOrgNameRA               = "test-org"
	testRepoNameRA              = "test-repo"
	testBranchName              = "main"
	contentTypeJSON             = "application/json"
	contentTypeZip              = "application/zip"
)

// TestUtilityFunctions tests basic utility functions
func TestUtilityFunctions(t *testing.T) {
	t.Run("getGlobalInfo function", func(t *testing.T) {
		// Test getting global info (should not panic)
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("getGlobalInfo panicked: %v", r)
				}
			}()
			result := getGlobalInfo()
			// Result can be empty initially, just ensure it doesn't crash
			_ = result
		}()
	})

	t.Run("getLanguageData function", func(t *testing.T) {
		// Test getting language data (should not panic)
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("getLanguageData panicked: %v", r)
				}
			}()
			result := getLanguageData()
			// Result can be empty initially, just ensure it doesn't crash
			_ = result
		}()
	})

	t.Run("isMainBranch function", func(t *testing.T) {
		testCases := []struct {
			branchName string
			expected   bool
		}{
			{"main", true},
			{"master", true},
			{"develop", true},
			{"development", true},
			{"default", true},
			{"feature-branch", false},
			{"bugfix-123", false},
			{"", false},
		}

		for _, tc := range testCases {
			result := isMainBranch(tc.branchName)
			if result != tc.expected {
				t.Errorf("isMainBranch(%s) = %v, want %v", tc.branchName, result, tc.expected)
			}
		}
	})

	t.Run("isPortOpen function", func(t *testing.T) {
		// Test with a port that should be closed
		result := isPortOpen(99999) // Very high port number, likely closed
		if result {
			t.Log("Port 99999 is unexpectedly open")
		}

		// Test with a specific port by opening a listener
		listener, err := net.Listen("tcp", ":0") // Let OS choose available port
		if err != nil {
			t.Fatalf("Failed to create test listener: %v", err)
		}
		defer listener.Close()

		addr := listener.Addr().(*net.TCPAddr)
		port := addr.Port

		// Now test if our open port is detected as open
		result = isPortOpen(port)
		if !result {
			t.Errorf("isPortOpen should detect open port %d", port)
		}
	})
}

// TestPlatformFunctions tests platform detection and processing functions
func TestPlatformFunctions(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "test_platform_*")
	if err != nil {
		t.Fatalf(errFailedToCreateTempDir, err)
	}
	defer os.RemoveAll(tempDir)

	originalWd, _ := os.Getwd()
	defer os.Chdir(originalWd)
	os.Chdir(tempDir)

	// Create necessary directory structure
	dirs := []string{testResultsDirRA, testConfigDir}
	for _, dir := range dirs {
		err = os.MkdirAll(dir, 0755)
		if err != nil {
			t.Fatalf("Failed to create dir %s: %v", dir, err)
		}
	}

	t.Run("detectPlatformAndReadAnalysis function", func(t *testing.T) {
		// Test with no analysis files (should return error)
		_, _, err := detectPlatformAndReadAnalysis()
		if err == nil {
			t.Error("detectPlatformAndReadAnalysis should return error when no analysis files exist")
		}

		// Test with GitHub analysis file
		analysisData := `{
			"NumRepositories": 1,
			"ProjectBranches": [
				{
					"Org": "test-org",
					"RepoSlug": "test-repo",
					"MainBranch": "main"
				}
			]
		}`

		err = os.WriteFile(testAnalysisResultFile, []byte(analysisData), 0644)
		if err != nil {
			t.Fatalf("Failed to create analysis file: %v", err)
		}

		platform, data, err := detectPlatformAndReadAnalysis()
		if err != nil {
			t.Errorf("detectPlatformAndReadAnalysis failed: %v", err)
		}

		if platform != "github" {
			t.Errorf("detectPlatformAndReadAnalysis platform = %s, want github", platform)
		}

		if len(data) == 0 {
			t.Error("detectPlatformAndReadAnalysis should return non-empty data")
		}
	})

	t.Run("getFirstPartForPlatform function", func(t *testing.T) {
		testCases := []struct {
			platform string
			branch   AnalysisResult_ProjectBranch
			repoName string
			expected string
		}{
			{
				"azure",
				AnalysisResult_ProjectBranch{ProjectKey: "project-key", Org: testOrgNameRA},
				testRepoNameRA,
				"project-key",
			},
			{
				"github",
				AnalysisResult_ProjectBranch{Org: testOrgNameRA},
				testRepoNameRA,
				testOrgNameRA,
			},
			{
				"gitlab",
				AnalysisResult_ProjectBranch{Org: testOrgNameRA},
				testRepoNameRA,
				testOrgNameRA,
			},
			{
				"bitbucket",
				AnalysisResult_ProjectBranch{Org: testOrgNameRA},
				testRepoNameRA,
				testOrgNameRA,
			},
		}

		for _, tc := range testCases {
			result := getFirstPartForPlatform(tc.platform, tc.branch, tc.repoName)
			if result != tc.expected {
				t.Errorf("getFirstPartForPlatform(%s) = %s, want %s", tc.platform, result, tc.expected)
			}
		}
	})

	t.Run("getFirstPartForFilename function", func(t *testing.T) {
		testCases := []struct {
			platform string
			orgName  string
			repoName string
			expected string
		}{
			{"azure", testOrgNameRA, testRepoNameRA, testRepoNameRA},
			{"github", testOrgNameRA, testRepoNameRA, testOrgNameRA},
			{"gitlab", testOrgNameRA, testRepoNameRA, testOrgNameRA},
			{"bitbucket", testOrgNameRA, testRepoNameRA, testOrgNameRA},
			{"unknown", testOrgNameRA, testRepoNameRA, testOrgNameRA},
		}

		for _, tc := range testCases {
			result := getFirstPartForFilename(tc.platform, tc.orgName, tc.repoName)
			if result != tc.expected {
				t.Errorf("getFirstPartForFilename(%s) = %s, want %s", tc.platform, result, tc.expected)
			}
		}
	})

	t.Run("getPlatformInfoAndURL function", func(t *testing.T) {
		testCases := []struct {
			platform     string
			org          string
			repo         string
			expectedIcon string
			expectedURL  string
		}{
			{"github", testOrgNameRA, testRepoNameRA, "fab fa-github", "https://github.com/test-org/test-repo"},
			{"gitlab", testOrgNameRA, testRepoNameRA, "fab fa-gitlab", "https://gitlab.com/test-org/test-repo"},
			{"bitbucket", testOrgNameRA, testRepoNameRA, "fab fa-bitbucket", "https://bitbucket.org/test-org/test-repo"},
			{"azure", testOrgNameRA, testRepoNameRA, "fab fa-microsoft", "https://dev.azure.com/test-org/_git/test-repo"},
			{"unknown", testOrgNameRA, testRepoNameRA, "fab fa-github", "https://github.com/test-org/test-repo"},
		}

		for _, tc := range testCases {
			icon, url := getPlatformInfoAndURL(tc.platform, tc.org, tc.repo)
			if icon != tc.expectedIcon {
				t.Errorf("getPlatformInfoAndURL(%s) icon = %s, want %s", tc.platform, icon, tc.expectedIcon)
			}
			if url != tc.expectedURL {
				t.Errorf("getPlatformInfoAndURL(%s) url = %s, want %s", tc.platform, url, tc.expectedURL)
			}
		}
	})
}

// TestDataFunctions tests data processing and loading functions
func TestDataFunctions(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "test_data_*")
	if err != nil {
		t.Fatalf(errFailedToCreateTempDir, err)
	}
	defer os.RemoveAll(tempDir)

	originalWd, _ := os.Getwd()
	defer os.Chdir(originalWd)
	os.Chdir(tempDir)

	// Create necessary directory structure
	dirs := []string{testResultsDirRA, testConfigDir, testByFileReportDir, testByLanguageReportDir}
	for _, dir := range dirs {
		err = os.MkdirAll(dir, 0755)
		if err != nil {
			t.Fatalf("Failed to create dir %s: %v", dir, err)
		}
	}

	t.Run("loadApplicationData function", func(t *testing.T) {
		// Create required test files
		globalReportData := `{
			"Organization": "test-org",
			"TotalLinesOfCode": "10000",
			"LargestRepository": "test-repo",
			"LinesOfCodeLargestRepo": "5000",
			"DevOpsPlatform": "github",
			"NumberRepos": 1
		}`

		codeLinesByLanguageData := `[
			{
				"Language": "Go",
				"CodeLines": 5000,
				"Percentage": 50.0,
				"CodeLinesF": "5,000"
			},
			{
				"Language": "JavaScript",
				"CodeLines": 3000,
				"Percentage": 30.0,
				"CodeLinesF": "3,000"
			}
		]`

		analysisResultData := `{
			"NumRepositories": 1,
			"ProjectBranches": [
				{
					"Org": "test-org",
					"RepoSlug": "test-repo",
					"MainBranch": "main"
				}
			]
		}`

		byFileReportData := `{
			"TotalLines": 10000,
			"TotalBlankLines": 1000,
			"TotalComments": 2000,
			"TotalCodeLines": 7000,
			"Results": []
		}`

		// Write test files
		err = os.WriteFile(testGlobalReportFile, []byte(globalReportData), 0644)
		if err != nil {
			t.Fatalf("Failed to create global report file: %v", err)
		}

		err = os.WriteFile(testCodeLinesByLanguageFile, []byte(codeLinesByLanguageData), 0644)
		if err != nil {
			t.Fatalf("Failed to create code lines by language file: %v", err)
		}

		err = os.WriteFile(testAnalysisResultFile, []byte(analysisResultData), 0644)
		if err != nil {
			t.Fatalf("Failed to create analysis result file: %v", err)
		}

		// Create byfile report
		byFileReportPath := filepath.Join(testByFileReportDir, "Result_test-org__test-repo__main_byfile.json")
		err = os.WriteFile(byFileReportPath, []byte(byFileReportData), 0644)
		if err != nil {
			t.Fatalf("Failed to create byfile report: %v", err)
		}

		// Test loadApplicationData
		pageData, err := loadApplicationData()
		if err != nil {
			t.Errorf("loadApplicationData failed: %v", err)
		}

		// Verify data structure
		if pageData.GlobalReport.Organization != testOrgNameRA {
			t.Errorf("loadApplicationData GlobalReport.Organization = %s, want %s", pageData.GlobalReport.Organization, testOrgNameRA)
		}

		if len(pageData.Languages) == 0 {
			t.Error("loadApplicationData should return non-empty Languages")
		}

		if len(pageData.Repositories) == 0 {
			t.Error("loadApplicationData should return non-empty Repositories")
		}
	})

	t.Run("getRepositoryData function", func(t *testing.T) {
		// Create required test files for getRepositoryData
		analysisResultData := `{
			"NumRepositories": 2,
			"ProjectBranches": [
				{
					"Org": "test-org",
					"RepoSlug": "repo1",
					"MainBranch": "main"
				},
				{
					"Org": "test-org",
					"RepoSlug": "repo2",
					"MainBranch": "develop"
				}
			]
		}`

		byFileReport1Data := `{
			"TotalLines": 5000,
			"TotalBlankLines": 500,
			"TotalComments": 1000,
			"TotalCodeLines": 3500
		}`

		byFileReport2Data := `{
			"TotalLines": 8000,
			"TotalBlankLines": 800,
			"TotalComments": 1600,
			"TotalCodeLines": 5600
		}`

		// Write test files
		err = os.WriteFile(testAnalysisResultFile, []byte(analysisResultData), 0644)
		if err != nil {
			t.Fatalf("Failed to create analysis result file: %v", err)
		}

		repo1Path := filepath.Join(testByFileReportDir, "Result_test-org__repo1__main_byfile.json")
		err = os.WriteFile(repo1Path, []byte(byFileReport1Data), 0644)
		if err != nil {
			t.Fatalf("Failed to create repo1 byfile report: %v", err)
		}

		repo2Path := filepath.Join(testByFileReportDir, "Result_test-org__repo2__develop_byfile.json")
		err = os.WriteFile(repo2Path, []byte(byFileReport2Data), 0644)
		if err != nil {
			t.Fatalf("Failed to create repo2 byfile report: %v", err)
		}

		// Test getRepositoryData
		repositories, err := getRepositoryData()
		if err != nil {
			t.Errorf("getRepositoryData failed: %v", err)
		}

		if len(repositories) != 2 {
			t.Errorf("getRepositoryData should return 2 repositories, got: %d", len(repositories))
		}

		// Verify sorting (should be sorted by CodeLines descending)
		if repositories[0].CodeLines < repositories[1].CodeLines {
			t.Error("getRepositoryData should sort repositories by CodeLines descending")
		}
	})

	t.Run("getOtherBranchesData function", func(t *testing.T) {
		// Create analysis result file
		analysisResultData := `{
			"NumRepositories": 1,
			"ProjectBranches": [
				{
					"Org": "test-org",
					"RepoSlug": "test-repo",
					"MainBranch": "main"
				}
			]
		}`

		err = os.WriteFile(testAnalysisResultFile, []byte(analysisResultData), 0644)
		if err != nil {
			t.Fatalf("Failed to create analysis result file: %v", err)
		}

		// Create multiple branch reports
		mainBranchData := `{"TotalLines": 5000, "TotalBlankLines": 500, "TotalComments": 1000, "TotalCodeLines": 3500}`
		developBranchData := `{"TotalLines": 4500, "TotalBlankLines": 450, "TotalComments": 900, "TotalCodeLines": 3150}`

		mainBranchPath := filepath.Join(testByFileReportDir, "Result_test-org__test-repo__main_byfile.json")
		developBranchPath := filepath.Join(testByFileReportDir, "Result_test-org__test-repo__develop_byfile.json")

		err = os.WriteFile(mainBranchPath, []byte(mainBranchData), 0644)
		if err != nil {
			t.Fatalf("Failed to create main branch report: %v", err)
		}

		err = os.WriteFile(developBranchPath, []byte(developBranchData), 0644)
		if err != nil {
			t.Fatalf("Failed to create develop branch report: %v", err)
		}

		// Test getOtherBranchesData
		branches := getOtherBranchesData(testOrgNameRA, testRepoNameRA, testBranchName)

		// Should find the develop branch (excluding main)
		if len(branches) == 0 {
			t.Error("getOtherBranchesData should find other branches")
		}

		// Verify that main branch is excluded
		for _, branch := range branches {
			if branch.Branch == testBranchName {
				t.Error("getOtherBranchesData should exclude the current branch")
			}
		}
	})
}

// TestHTTPHandlers tests HTTP handler functions
func TestHTTPHandlers(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "test_http_*")
	if err != nil {
		t.Fatalf(errFailedToCreateTempDir, err)
	}
	defer os.RemoveAll(tempDir)

	originalWd, _ := os.Getwd()
	defer os.Chdir(originalWd)
	os.Chdir(tempDir)

	// Create test Results directory with a test file
	err = os.MkdirAll(testResultsDirRA, 0755)
	if err != nil {
		t.Fatalf("Failed to create dir %s: %v", testResultsDirRA, err)
	}

	testFile := filepath.Join(testResultsDirRA, "test.txt")
	err = os.WriteFile(testFile, []byte("test content"), 0644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	t.Run("zipResults handler", func(t *testing.T) {
		// Create test request
		req, err := http.NewRequest("GET", "/download", nil)
		if err != nil {
			t.Fatalf("Failed to create request: %v", err)
		}

		// Create test response recorder
		rr := httptest.NewRecorder()

		// Call the handler
		zipResults(rr, req)

		// Check response status
		if status := rr.Code; status != http.StatusOK {
			t.Errorf("zipResults returned wrong status code: got %v want %v", status, http.StatusOK)
		}

		// Check content type
		contentType := rr.Header().Get("Content-Type")
		if contentType != contentTypeZip {
			t.Errorf("zipResults returned wrong content type: got %v want %v", contentType, contentTypeZip)
		}

		// Check content disposition
		contentDisposition := rr.Header().Get("Content-Disposition")
		if !strings.Contains(contentDisposition, "Results.zip") {
			t.Errorf("zipResults should set correct Content-Disposition header: got %v", contentDisposition)
		}
	})

	t.Run("setupHTTPHandlers function", func(t *testing.T) {
		// Create minimal page data
		pageData := PageData{
			Languages:    []LanguageData{},
			GlobalReport: Globalinfo{},
			Repositories: []RepositoryData{},
		}

		// Should not panic
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("setupHTTPHandlers panicked: %v", r)
				}
			}()
			setupHTTPHandlers(pageData)
		}()
	})
}

// TestServerFunctions tests server startup and management functions
func TestServerFunctions(t *testing.T) {
	t.Run("handleServerStartup function", func(t *testing.T) {
		// Should not panic during setup (though it won't actually start server in test)
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("handleServerStartup panicked: %v", r)
				}
			}()
			// Note: This will try to start a server, but we'll let it run briefly
			go handleServerStartup()

			// Give it a moment to initialize
			time.Sleep(100 * time.Millisecond)
		}()
	})

	t.Run("startServer function", func(t *testing.T) {
		// Test starting server on a specific port (briefly)
		go func() {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("startServer panicked: %v", r)
				}
			}()
			// Use a high port number to avoid conflicts
			startServer(58080)
		}()

		// Give it a moment to start
		time.Sleep(100 * time.Millisecond)
	})
}

// TestZipFunction tests the ZipDirectory function
func TestZipFunction(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "test_zip_*")
	if err != nil {
		t.Fatalf(errFailedToCreateTempDir, err)
	}
	defer os.RemoveAll(tempDir)

	originalWd, _ := os.Getwd()
	defer os.Chdir(originalWd)
	os.Chdir(tempDir)

	t.Run("ZipDirectory function", func(t *testing.T) {
		// Create source directory with test files
		sourceDir := "test_source"
		err := os.MkdirAll(sourceDir, 0755)
		if err != nil {
			t.Fatalf("Failed to create source directory: %v", err)
		}

		// Create test files
		testFile1 := filepath.Join(sourceDir, "file1.txt")
		testFile2 := filepath.Join(sourceDir, "file2.txt")
		err = os.WriteFile(testFile1, []byte("content 1"), 0644)
		if err != nil {
			t.Fatalf("Failed to create test file 1: %v", err)
		}
		err = os.WriteFile(testFile2, []byte("content 2"), 0644)
		if err != nil {
			t.Fatalf("Failed to create test file 2: %v", err)
		}

		// Create subdirectory with file
		subDir := filepath.Join(sourceDir, "subdir")
		err = os.MkdirAll(subDir, 0755)
		if err != nil {
			t.Fatalf("Failed to create subdirectory: %v", err)
		}

		subFile := filepath.Join(subDir, "subfile.txt")
		err = os.WriteFile(subFile, []byte("sub content"), 0644)
		if err != nil {
			t.Fatalf("Failed to create sub file: %v", err)
		}

		// Test ZipDirectory
		targetZip := "test.zip"
		err = ZipDirectory(sourceDir, targetZip)
		if err != nil {
			t.Errorf("ZipDirectory failed: %v", err)
		}

		// Verify zip file was created
		if _, err := os.Stat(targetZip); os.IsNotExist(err) {
			t.Error("ZipDirectory should create zip file")
		}

		// Test with non-existent source directory
		err = ZipDirectory("non-existent", "test2.zip")
		if err == nil {
			t.Error("ZipDirectory should return error for non-existent source")
		}
	})
}

// TestDetailDataFunction tests repository detail data retrieval
func TestDetailDataFunction(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "test_detail_*")
	if err != nil {
		t.Fatalf(errFailedToCreateTempDir, err)
	}
	defer os.RemoveAll(tempDir)

	originalWd, _ := os.Getwd()
	defer os.Chdir(originalWd)
	os.Chdir(tempDir)

	// Create necessary directory structure
	dirs := []string{testResultsDirRA, testConfigDir, testByFileReportDir, testByLanguageReportDir}
	for _, dir := range dirs {
		err = os.MkdirAll(dir, 0755)
		if err != nil {
			t.Fatalf("Failed to create dir %s: %v", dir, err)
		}
	}

	t.Run("getRepositoryDetailData function", func(t *testing.T) {
		// Create required test files
		analysisResultData := `{
			"NumRepositories": 1,
			"ProjectBranches": [
				{
					"Org": "test-org",
					"RepoSlug": "test-repo",
					"MainBranch": "main"
				}
			]
		}`

		globalReportData := `{
			"Organization": "test-org",
			"TotalLinesOfCode": "10000",
			"LargestRepository": "test-repo",
			"LinesOfCodeLargestRepo": "5000",
			"DevOpsPlatform": "github",
			"NumberRepos": 1
		}`

		byFileReportData := `{
			"TotalLines": 5000,
			"TotalBlankLines": 500,
			"TotalComments": 1000,
			"TotalCodeLines": 3500,
			"Results": []
		}`

		byLanguageReportData := `{
			"TotalFiles": 10,
			"TotalLines": 5000,
			"TotalBlankLines": 500,
			"TotalComments": 1000,
			"TotalCodeLines": 3500,
			"Results": [
				{
					"Language": "Go",
					"Files": 5,
					"Lines": 3000,
					"BlankLines": 300,
					"Comments": 600,
					"CodeLines": 2100
				}
			]
		}`

		// Write test files
		err = os.WriteFile(testAnalysisResultFile, []byte(analysisResultData), 0644)
		if err != nil {
			t.Fatalf("Failed to create analysis result file: %v", err)
		}

		err = os.WriteFile(testGlobalReportFile, []byte(globalReportData), 0644)
		if err != nil {
			t.Fatalf("Failed to create global report file: %v", err)
		}

		byFileReportPath := filepath.Join(testByFileReportDir, "Result_test-org__test-repo__main_byfile.json")
		err = os.WriteFile(byFileReportPath, []byte(byFileReportData), 0644)
		if err != nil {
			t.Fatalf("Failed to create byfile report: %v", err)
		}

		byLanguageReportPath := filepath.Join(testByLanguageReportDir, "Result_test-org__test-repo__main.json")
		err = os.WriteFile(byLanguageReportPath, []byte(byLanguageReportData), 0644)
		if err != nil {
			t.Fatalf("Failed to create bylanguage report: %v", err)
		}

		// Test getRepositoryDetailData
		repoData, err := getRepositoryDetailData(testRepoNameRA, testBranchName)
		if err != nil {
			t.Errorf("getRepositoryDetailData failed: %v", err)
		}

		if repoData == nil {
			t.Fatal("getRepositoryDetailData should return non-nil data")
		}

		if repoData.Repository != testRepoNameRA {
			t.Errorf("getRepositoryDetailData Repository = %s, want %s", repoData.Repository, testRepoNameRA)
		}

		if repoData.MainBranch != testBranchName {
			t.Errorf("getRepositoryDetailData MainBranch = %s, want %s", repoData.MainBranch, testBranchName)
		}

		if repoData.Organization != testOrgNameRA {
			t.Errorf("getRepositoryDetailData Organization = %s, want %s", repoData.Organization, testOrgNameRA)
		}

		if len(repoData.Languages) == 0 {
			t.Error("getRepositoryDetailData should return non-empty Languages")
		}

		// Test with non-existent repository
		_, err = getRepositoryDetailData("non-existent-repo", testBranchName)
		if err == nil {
			t.Error("getRepositoryDetailData should return error for non-existent repository")
		}
	})
}
