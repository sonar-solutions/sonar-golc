//go:build golc
// +build golc

package main

import (
	"archive/zip"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SonarSource-Demos/sonar-golc/pkg/devops/getazure"
	getbibucket "github.com/SonarSource-Demos/sonar-golc/pkg/devops/getbitbucket/v2"
	getbibucketdc "github.com/SonarSource-Demos/sonar-golc/pkg/devops/getbitbucketdc"
	"github.com/SonarSource-Demos/sonar-golc/pkg/devops/getgithub"
	"github.com/SonarSource-Demos/sonar-golc/pkg/devops/getgitlab"
)

// Constants to avoid duplicating string literals (SonarQube maintainability)
const (
	errFailedToCreateLogsDir = "Failed to create Logs dir: %v"
	testConfigJSON           = "test_config.json"
	testResultsDir           = "Results"
	testLogsDir              = "Logs"
	testExclusionFile        = "test_exclusion.txt"
	sampleExclusionContent   = "repo1\nrepo2\n"
	validConfigContent       = `{"platforms": {"test": {}}, "logging": {"level": "info"}, "release": {"version": "2.0"}}`
	invalidConfigContent     = `{"invalid": "json"`
	testBackupSource         = "test_backup_source"
	testBackupTarget         = "test_backup.zip"
	testRepoName             = "test-repo"
	testOrgName              = "test-org"
	testUserName             = "test-user"
	testAccessToken          = "test-token"
	testDevOpsType           = "github"
)

// TestUtilityFunctions tests basic utility functions
func TestUtilityFunctions(t *testing.T) {
	t.Run("getFileNameIfExists function", func(t *testing.T) {
		// Test with non-existent file
		result := getFileNameIfExists("non-existent-file.txt")
		if result != "0" {
			t.Errorf("getFileNameIfExists should return '0' for non-existent file, got: %s", result)
		}

		// Test with existing file
		tempFile, err := os.CreateTemp("", "test_exists_*.txt")
		if err != nil {
			t.Fatalf("Failed to create temp file: %v", err)
		}
		defer os.Remove(tempFile.Name())
		tempFile.Close()

		result = getFileNameIfExists(tempFile.Name())
		if result != tempFile.Name() {
			t.Errorf("getFileNameIfExists should return filename for existing file, got: %s", result)
		}
	})

	t.Run("convertToSliceString function", func(t *testing.T) {
		input := []interface{}{"string1", "string2", "string3"}
		result := convertToSliceString(input)

		if len(result) != 3 {
			t.Errorf("convertToSliceString should return slice of length 3, got: %d", len(result))
		}

		expected := []string{"string1", "string2", "string3"}
		for i, v := range result {
			if v != expected[i] {
				t.Errorf("convertToSliceString[%d] = %s, want %s", i, v, expected[i])
			}
		}

		// Test with empty slice
		emptyInput := []interface{}{}
		emptyResult := convertToSliceString(emptyInput)
		if len(emptyResult) != 0 {
			t.Error("convertToSliceString should return empty slice for empty input")
		}
	})

	t.Run("extractDomain function", func(t *testing.T) {
		testCases := []struct {
			input    string
			expected string
		}{
			{"https://github.com/owner/repo", "github.com"},
			{"http://gitlab.com/group/project", "gitlab.com"},
			{"https://api.bitbucket.org/2.0/", "api.bitbucket.org"},
			{"github.com", "github.com"},
			{"localhost:8080/path", "localhost:8080"},
		}

		for _, tc := range testCases {
			result := extractDomain(tc.input)
			if result != tc.expected {
				t.Errorf("extractDomain(%s) = %s, want %s", tc.input, result, tc.expected)
			}
		}
	})

	t.Run("getExcludePaths function", func(t *testing.T) {
		// Test with nil
		result := getExcludePaths(nil)
		if len(result) != 0 {
			t.Error("getExcludePaths should return empty slice for nil input")
		}

		// Test with valid slice
		input := []interface{}{"path1", "path2", "path3"}
		result = getExcludePaths(input)
		if len(result) != 3 {
			t.Errorf("getExcludePaths should return slice of length 3, got: %d", len(result))
		}

		// Test with invalid type
		invalidInput := "not a slice"
		result = getExcludePaths(invalidInput)
		if len(result) != 0 {
			t.Error("getExcludePaths should return empty slice for invalid input")
		}
	})
}

// TestConfigFunctions tests configuration-related functions
func TestConfigFunctions(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "test_config_*")
	if err != nil {
		t.Fatalf(errFailedToCreateTempDir, err)
	}
	defer os.RemoveAll(tempDir)

	originalWd, _ := os.Getwd()
	defer os.Chdir(originalWd)
	os.Chdir(tempDir)

	t.Run("LoadConfig function", func(t *testing.T) {
		// Test with non-existent file
		_, err := LoadConfig("non-existent.json")
		if err == nil {
			t.Error("LoadConfig should return error for non-existent file")
		}

		// Test with valid config
		err = os.WriteFile(testConfigJSON, []byte(validConfigContent), 0644)
		if err != nil {
			t.Fatalf("Failed to create test config file: %v", err)
		}

		config, err := LoadConfig(testConfigJSON)
		if err != nil {
			t.Errorf("LoadConfig failed with valid config: %v", err)
		}

		if config.Release.Version != "2.0" {
			t.Errorf("LoadConfig version = %s, want 2.0", config.Release.Version)
		}

		// Test with invalid JSON
		invalidConfigFile := "invalid_config.json"
		err = os.WriteFile(invalidConfigFile, []byte(invalidConfigContent), 0644)
		if err != nil {
			t.Fatalf("Failed to create invalid config file: %v", err)
		}

		_, err = LoadConfig(invalidConfigFile)
		if err == nil {
			t.Error("LoadConfig should return error for invalid JSON")
		}
	})

	t.Run("parseJSONFile function", func(t *testing.T) {
		// Create test JSON file
		testJSON := `{"TotalCodeLines": 1500, "TotalLines": 2000}`
		testJSONFile := "test_result.json"
		err := os.WriteFile(testJSONFile, []byte(testJSON), 0644)
		if err != nil {
			t.Fatalf("Failed to create test JSON file: %v", err)
		}

		result := parseJSONFile(testJSONFile, testRepoName)
		if result != 1500 {
			t.Errorf("parseJSONFile should return 1500, got: %d", result)
		}

		// Test with non-existent file
		result = parseJSONFile("non-existent.json", testRepoName)
		if result != 0 {
			t.Errorf("parseJSONFile should return 0 for non-existent file, got: %d", result)
		}

		// Test with invalid JSON
		invalidJSONFile := "invalid.json"
		err = os.WriteFile(invalidJSONFile, []byte("invalid json"), 0644)
		if err != nil {
			t.Fatalf("Failed to create invalid JSON file: %v", err)
		}

		result = parseJSONFile(invalidJSONFile, testRepoName)
		if result != 0 {
			t.Errorf("parseJSONFile should return 0 for invalid JSON, got: %d", result)
		}
	})
}

// TestBackupFunctions tests backup-related functions
func TestBackupFunctions(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "test_backup_*")
	if err != nil {
		t.Fatalf(errFailedToCreateTempDir, err)
	}
	defer os.RemoveAll(tempDir)

	originalWd, _ := os.Getwd()
	defer os.Chdir(originalWd)
	os.Chdir(tempDir)

	err = os.MkdirAll(testLogsDir, 0755)
	if err != nil {
		t.Fatalf(errFailedToCreateLogsDir, err)
	}

	t.Run("generateBackupFilePath function", func(t *testing.T) {
		sourceDir := testBackupSource
		backupDir := "backup"

		result := generateBackupFilePath(sourceDir, backupDir)

		if !strings.Contains(result, sourceDir) {
			t.Errorf("generateBackupFilePath should contain source dir name: %s", result)
		}

		if !strings.HasSuffix(result, ".zip") {
			t.Errorf("generateBackupFilePath should end with .zip: %s", result)
		}

		if !strings.Contains(result, backupDir) {
			t.Errorf("generateBackupFilePath should be in backup directory: %s", result)
		}
	})

	t.Run("createBackupDirectory function", func(t *testing.T) {
		backupDir := "test_backup_dir"

		err := createBackupDirectory(backupDir)
		if err != nil {
			t.Errorf("createBackupDirectory failed: %v", err)
		}

		// Verify directory was created
		if _, err := os.Stat(backupDir); os.IsNotExist(err) {
			t.Error("createBackupDirectory should create the directory")
		}

		// Test with existing directory (should not fail)
		err = createBackupDirectory(backupDir)
		if err != nil {
			t.Errorf("createBackupDirectory should handle existing directory: %v", err)
		}
	})

	t.Run("ZipDirectory function", func(t *testing.T) {
		// Create source directory with files
		sourceDir := testBackupSource
		err := os.MkdirAll(sourceDir, 0755)
		if err != nil {
			t.Fatalf("Failed to create source directory: %v", err)
		}

		// Create test files
		testFile1 := filepath.Join(sourceDir, "test1.txt")
		testFile2 := filepath.Join(sourceDir, "test2.txt")
		err = os.WriteFile(testFile1, []byte("test content 1"), 0644)
		if err != nil {
			t.Fatalf("Failed to create test file 1: %v", err)
		}
		err = os.WriteFile(testFile2, []byte("test content 2"), 0644)
		if err != nil {
			t.Fatalf("Failed to create test file 2: %v", err)
		}

		targetZip := testBackupTarget
		err = ZipDirectory(sourceDir, targetZip)
		if err != nil {
			t.Errorf("ZipDirectory failed: %v", err)
		}

		// Verify zip file was created
		if _, err := os.Stat(targetZip); os.IsNotExist(err) {
			t.Error("ZipDirectory should create zip file")
		}

		// Verify zip file contents
		zipReader, err := zip.OpenReader(targetZip)
		if err != nil {
			t.Errorf("Failed to open zip file: %v", err)
		} else {
			defer zipReader.Close()

			if len(zipReader.File) < 2 {
				t.Errorf("Zip should contain at least 2 files, got: %d", len(zipReader.File))
			}
		}
	})

	t.Run("createBackup function", func(t *testing.T) {
		// Create source directory
		sourceDir := "backup_test_source"
		err := os.MkdirAll(sourceDir, 0755)
		if err != nil {
			t.Fatalf("Failed to create source directory: %v", err)
		}

		// Create test file
		testFile := filepath.Join(sourceDir, "test.txt")
		err = os.WriteFile(testFile, []byte("test content"), 0644)
		if err != nil {
			t.Fatalf("Failed to create test file: %v", err)
		}

		pwd, _ := os.Getwd()
		err = createBackup(sourceDir, pwd)
		if err != nil {
			t.Errorf("createBackup failed: %v", err)
		}

		// Verify backup directory and file were created
		savesDir := filepath.Join(pwd, "Saves")
		if _, err := os.Stat(savesDir); os.IsNotExist(err) {
			t.Error("createBackup should create Saves directory")
		}
	})
}

// TestFileFunctions tests file reading and processing functions
func TestFileFunctions(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "test_files_*")
	if err != nil {
		t.Fatalf(errFailedToCreateTempDir, err)
	}
	defer os.RemoveAll(tempDir)

	originalWd, _ := os.Getwd()
	defer os.Chdir(originalWd)
	os.Chdir(tempDir)

	t.Run("ReadLines function", func(t *testing.T) {
		// Test with non-existent file
		_, err := ReadLines("non-existent.txt")
		if err == nil {
			t.Error("ReadLines should return error for non-existent file")
		}

		// Test with valid file
		testContent := "line1\nline2\nline3\n"
		testFile := "test_lines.txt"
		err = os.WriteFile(testFile, []byte(testContent), 0644)
		if err != nil {
			t.Fatalf("Failed to create test file: %v", err)
		}

		lines, err := ReadLines(testFile)
		if err != nil {
			t.Errorf("ReadLines failed: %v", err)
		}

		if len(lines) != 3 {
			t.Errorf("ReadLines should return 3 lines, got: %d", len(lines))
		}

		expected := []string{"line1", "line2", "line3"}
		for i, line := range lines {
			if line != expected[i] {
				t.Errorf("ReadLines[%d] = %s, want %s", i, line, expected[i])
			}
		}

		// Test with empty file
		emptyFile := "empty.txt"
		err = os.WriteFile(emptyFile, []byte(""), 0644)
		if err != nil {
			t.Fatalf("Failed to create empty file: %v", err)
		}

		lines, err = ReadLines(emptyFile)
		if err != nil {
			t.Errorf("ReadLines should handle empty file: %v", err)
		}

		if len(lines) != 0 {
			t.Errorf("ReadLines should return empty slice for empty file, got: %d lines", len(lines))
		}
	})
}

// TestDirectoryFunctions tests directory creation and management
func TestDirectoryFunctions(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "test_directories_*")
	if err != nil {
		t.Fatalf(errFailedToCreateTempDir, err)
	}
	defer os.RemoveAll(tempDir)

	originalWd, _ := os.Getwd()
	defer os.Chdir(originalWd)
	os.Chdir(tempDir)

	t.Run("createDirectories function", func(t *testing.T) {
		basePath := "test_base"
		paths := []string{"/dir1", "/dir2", "/dir3/subdir"}

		// Should not panic and should create directories
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("createDirectories panicked: %v", r)
				}
			}()
			createDirectories(basePath, paths)
		}()

		// Verify directories were created
		for _, path := range paths {
			fullPath := basePath + path
			if _, err := os.Stat(fullPath); os.IsNotExist(err) {
				t.Errorf("createDirectories should create directory: %s", fullPath)
			}
		}
	})

	t.Run("displayLanguages function", func(t *testing.T) {
		// Should not panic
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("displayLanguages panicked: %v", r)
				}
			}()
			displayLanguages()
		}()
	})
}

// TestAnalysisFunctions tests repository analysis functions
func TestAnalysisFunctions(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "test_analysis_*")
	if err != nil {
		t.Fatalf(errFailedToCreateTempDir, err)
	}
	defer os.RemoveAll(tempDir)

	originalWd, _ := os.Getwd()
	defer os.Chdir(originalWd)
	os.Chdir(tempDir)

	err = os.MkdirAll(testLogsDir, 0755)
	if err != nil {
		t.Fatalf(errFailedToCreateLogsDir, err)
	}

	t.Run("AnalyseRepo function", func(t *testing.T) {
		// Test with basic parameters
		count := AnalyseRepo(testResultsDir, testUserName, testAccessToken, testDevOpsType, testOrgName, testRepoName)

		// Should return 1 (indicating attempt was made)
		if count != 1 {
			t.Errorf("AnalyseRepo should return 1, got: %d", count)
		}
	})

	t.Run("waitForWorkers function", func(t *testing.T) {
		// Create a test channel
		results := make(chan int, 3)

		// Send some results
		results <- 1
		results <- 1
		results <- 1

		// Should not hang or panic
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("waitForWorkers panicked: %v", r)
				}
			}()
			waitForWorkers(3, results)
		}()
	})
}

// TestAnalysisListFunctions tests the various AnalyseReposList* functions
func TestAnalysisListFunctions(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "test_analysis_list_*")
	if err != nil {
		t.Fatalf(errFailedToCreateTempDir, err)
	}
	defer os.RemoveAll(tempDir)

	originalWd, _ := os.Getwd()
	defer os.Chdir(originalWd)
	os.Chdir(tempDir)

	err = os.MkdirAll(testLogsDir, 0755)
	if err != nil {
		t.Fatalf(errFailedToCreateLogsDir, err)
	}

	// Create basic platform config for testing
	platformConfig := map[string]interface{}{
		"Multithreading":    false,
		"NumberWorkerRepos": float64(5),
		"Workers":           float64(2),
		"ExtExclusion":      []interface{}{".git", ".svn"},
		"ExcludePaths":      []interface{}{},
		"ResultByFile":      false,
		"ResultAll":         false,
		"Protocol":          "https",
		"AccessToken":       testAccessToken,
		"Baseapi":           "github.com",
		"Organization":      testOrgName,
		"Workspace":         "workspace",
		"Users":             testUserName,
		"Url":               "https://github.com",
	}

	t.Run("AnalyseReposListBitC function", func(t *testing.T) {
		// Test with empty repository list
		emptyRepos := []getbibucket.ProjectBranch{}
		count := AnalyseReposListBitC(testResultsDir, platformConfig, emptyRepos)

		if count != 0 {
			t.Errorf("AnalyseReposListBitC should return 0 for empty repos, got: %d", count)
		}
	})

	t.Run("AnalyseReposListBitSRV function", func(t *testing.T) {
		// Test with empty repository list
		emptyRepos := []getbibucketdc.ProjectBranch{}
		count := AnalyseReposListBitSRV(testResultsDir, platformConfig, emptyRepos)

		if count != 0 {
			t.Errorf("AnalyseReposListBitSRV should return 0 for empty repos, got: %d", count)
		}
	})

	t.Run("AnalyseReposListGithub function", func(t *testing.T) {
		// Test with empty repository list
		emptyRepos := []getgithub.ProjectBranch{}
		count := AnalyseReposListGithub(testResultsDir, platformConfig, emptyRepos)

		if count != 0 {
			t.Errorf("AnalyseReposListGithub should return 0 for empty repos, got: %d", count)
		}
	})

	t.Run("AnalyseReposListGitlab function", func(t *testing.T) {
		// Test with empty repository list
		emptyRepos := []getgitlab.ProjectBranch{}
		count := AnalyseReposListGitlab(testResultsDir, platformConfig, emptyRepos)

		if count != 0 {
			t.Errorf("AnalyseReposListGitlab should return 0 for empty repos, got: %d", count)
		}
	})

	t.Run("AnalyseReposListAzure function", func(t *testing.T) {
		// Test with empty repository list
		emptyRepos := []getazure.ProjectBranch{}
		count := AnalyseReposListAzure(testResultsDir, platformConfig, emptyRepos)

		if count != 0 {
			t.Errorf("AnalyseReposListAzure should return 0 for empty repos, got: %d", count)
		}
	})

	t.Run("AnalyseReposListFile function", func(t *testing.T) {
		// Test with empty directory list
		emptyDirs := []string{}
		emptyExclusions := []string{}
		emptyExtensions := []string{}

		// Should not panic
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("AnalyseReposListFile panicked: %v", r)
				}
			}()
			AnalyseReposListFile(emptyDirs, emptyExclusions, emptyExtensions, false, false, "Results")
		}()
	})
}

// TestFlagsFunctions tests command line flag parsing and validation
func TestFlagsFunctions(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "test_flags_*")
	if err != nil {
		t.Fatalf(errFailedToCreateTempDir, err)
	}
	defer os.RemoveAll(tempDir)

	originalWd, _ := os.Getwd()
	defer os.Chdir(originalWd)
	os.Chdir(tempDir)

	err = os.MkdirAll(testLogsDir, 0755)
	if err != nil {
		t.Fatalf(errFailedToCreateLogsDir, err)
	}

	t.Run("setupResultsDirectory function", func(t *testing.T) {
		flags := ApplicationFlags{
			DevOps:      "test-platform",
			Fast:        false,
			AllBranches: false,
		}

		// Note: This test may require user interaction if Results directory exists
		// In a real scenario, you might want to mock os.Stat or use a temporary directory
		result := setupResultsDirectory(flags)

		// Should return a valid directory path
		if result == "" {
			t.Error("setupResultsDirectory should return non-empty directory path")
		}
	})
}
