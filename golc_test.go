//go:build golc
// +build golc

package main

import (
	"archive/zip"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/SonarSource-Demos/sonar-golc/pkg/devops/getazure"
	getbibucket "github.com/SonarSource-Demos/sonar-golc/pkg/devops/getbitbucket"
	getbibucketdc "github.com/SonarSource-Demos/sonar-golc/pkg/devops/getbitbucketdc"
	"github.com/SonarSource-Demos/sonar-golc/pkg/devops/getgithub"
	"github.com/SonarSource-Demos/sonar-golc/pkg/devops/getgitlab"
	"github.com/SonarSource-Demos/sonar-golc/pkg/utils"

	"github.com/sirupsen/logrus"
)

// TestMain initializes the package-level logger that production code paths
// reference (it is normally set up inside runGolcInProcess, which the unit
// tests don't call). Without this, functions like parseJSONFile that log on
// error would nil-panic. See issue #81 coverage-pipeline repair.
func TestMain(m *testing.M) {
	logger = logrus.New()
	os.Exit(m.Run())
}

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

	t.Run("getCloneTimeout function", func(t *testing.T) {
		// Absent key -> default.
		if got := getCloneTimeout(map[string]interface{}{}); got != time.Duration(defaultCloneTimeoutMinutes*float64(time.Minute)) {
			t.Errorf("absent CloneTimeout: got %v, want default %v minutes", got, defaultCloneTimeoutMinutes)
		}
		// Explicit value (minutes) -> that duration.
		if got := getCloneTimeout(map[string]interface{}{"CloneTimeout": float64(5)}); got != 5*time.Minute {
			t.Errorf("CloneTimeout=5: got %v, want 5m", got)
		}
		// Zero -> disabled (0 duration, no deadline).
		if got := getCloneTimeout(map[string]interface{}{"CloneTimeout": float64(0)}); got != 0 {
			t.Errorf("CloneTimeout=0: got %v, want 0 (disabled)", got)
		}
		// Negative -> disabled.
		if got := getCloneTimeout(map[string]interface{}{"CloneTimeout": float64(-3)}); got != 0 {
			t.Errorf("CloneTimeout<0: got %v, want 0 (disabled)", got)
		}
	})

	t.Run("skipRecorder is concurrency-safe", func(t *testing.T) {
		r := &skipRecorder{}
		r.reset()
		var wg sync.WaitGroup
		for i := 0; i < 50; i++ {
			wg.Add(1)
			go func(n int) {
				defer wg.Done()
				r.add(skippedRepo{RepoSlug: "repo", Branch: "main", Reason: "clone timed out"})
			}(i)
		}
		wg.Wait()
		if got := len(r.snapshot()); got != 50 {
			t.Errorf("skipRecorder recorded %d entries, want 50", got)
		}
		// reset clears the slate for the next run.
		r.reset()
		if got := len(r.snapshot()); got != 0 {
			t.Errorf("after reset, got %d entries, want 0", got)
		}
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
			AnalyseReposListFile(emptyDirs, emptyExclusions, emptyExtensions, []string{}, []string{}, false, false, "Results")
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
		// Note: This test may require user interaction if Results directory exists
		// In a real scenario, you might want to mock os.Stat or use a temporary directory
		result := setupResultsDirectory("test-platform")

		// Should return a valid directory path
		if result == "" {
			t.Error("setupResultsDirectory should return non-empty directory path")
		}
	})
}

// TestConfigVersionCompatible pins the compatibility rule, because it decides whether an
// existing user's config.json still loads. Getting it wrong means either a startup failure
// with no underlying problem, or accepting a config whose schema really did change.
func TestConfigVersionCompatible(t *testing.T) {
	cases := []struct {
		name     string
		config   string
		expected string
		want     bool
	}{
		{"exact match", "2.1", "2.1", true},
		// The reason this rule exists: a 2.0 config must keep working on a 2.1 build.
		{"older minor accepted", "2.0", "2.1", true},
		{"newer minor accepted", "2.5", "2.1", true},
		{"patch level accepted", "2.0.6", "2.1", true},
		{"tag prefixes tolerated", "ver2.0", "2.1", true},
		{"v prefix tolerated", "v2.0", "2.1", true},
		{"whitespace tolerated", " 2.0 ", "2.1", true},
		// A major bump is the signal that the schema genuinely changed.
		{"older major rejected", "1.9", "2.1", false},
		{"newer major rejected", "3.0", "2.1", false},
		// Malformed rather than merely old.
		{"empty rejected", "", "2.1", false},
		{"non-numeric rejected", "abc", "2.1", false},
		{"prefix only rejected", "v", "2.1", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := configVersionCompatible(tc.config, tc.expected); got != tc.want {
				t.Errorf("configVersionCompatible(%q, %q) = %v, want %v", tc.config, tc.expected, got, tc.want)
			}
		})
	}
}

func TestMajorVersion(t *testing.T) {
	cases := map[string]string{
		"2.1": "2", "2.0.6": "2", "ver2.0": "2", "v3": "3", "10.4": "10",
		"2": "2", "": "", "abc": "", "v": "",
	}
	for in, want := range cases {
		if got := majorVersion(in); got != want {
			t.Errorf("majorVersion(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestVersionsAreInLockstep guards the pairing that actually breaks users: the UI writes
// version1 into every config it creates, so a literal that drifted from the scanner's
// expectation would make the UI generate configs its own scanner refuses.
func TestShippedSampleConfigIsCompatible(t *testing.T) {
	data, err := os.ReadFile("config_sample.json")
	if err != nil {
		t.Skipf("config_sample.json not readable: %v", err)
	}
	var sample struct {
		Release struct{ Version string } `json:"Release"`
	}
	if err := json.Unmarshal(data, &sample); err != nil {
		t.Fatalf("config_sample.json is not valid JSON: %v", err)
	}
	if !configVersionCompatible(sample.Release.Version, version1) {
		t.Errorf("config_sample.json declares %q, which this build (%q) would reject",
			sample.Release.Version, version1)
	}
}

// --- Regression tests for the config panic and the file-platform directory count ---

// TestNormalizePlatformConfigFillsMissingKeys covers the crash this function exists to
// prevent: a config that simply omits an optional key used to reach an unchecked
// assertion such as platformConfig["FileLoad"].(string) and panic with
// "interface conversion: interface {} is nil, not string".
func TestNormalizePlatformConfigFillsMissingKeys(t *testing.T) {
	cfg := map[string]interface{}{"DevOps": "file", "Directory": "/tmp"}

	if mistyped := normalizePlatformConfig(cfg); len(mistyped) != 0 {
		t.Fatalf("absent keys must be filled silently, got mistyped=%v", mistyped)
	}

	// The assertions the analysis actually performs must now all succeed.
	for _, key := range []string{"FileLoad", "FileExclusion", "Organization", "Protocol"} {
		if _, ok := cfg[key].(string); !ok {
			t.Errorf("cfg[%q] = %#v, want a string", key, cfg[key])
		}
	}
	for _, key := range []string{"ResultAll", "ResultByFile", "ScanSubDirs"} {
		if _, ok := cfg[key].(bool); !ok {
			t.Errorf("cfg[%q] = %#v, want a bool", key, cfg[key])
		}
	}
	for _, key := range []string{"Workers", "NumberWorkerRepos"} {
		if _, ok := cfg[key].(float64); !ok {
			t.Errorf("cfg[%q] = %#v, want a float64", key, cfg[key])
		}
	}
	if _, ok := cfg["ExtExclusion"].([]interface{}); !ok {
		t.Errorf("cfg[\"ExtExclusion\"] = %#v, want []interface{}", cfg["ExtExclusion"])
	}
}

func TestNormalizePlatformConfigPreservesSuppliedValues(t *testing.T) {
	cfg := map[string]interface{}{
		"DevOps":       "github",
		"Organization": "acme",
		"ResultAll":    false,
		"Workers":      float64(4),
		"ExtExclusion": []interface{}{".css"},
	}

	normalizePlatformConfig(cfg)

	if cfg["Organization"] != "acme" {
		t.Errorf("Organization = %#v, want \"acme\"", cfg["Organization"])
	}
	if cfg["ResultAll"] != false {
		t.Errorf("ResultAll = %#v, want false (a supplied false must not be overwritten)", cfg["ResultAll"])
	}
	if cfg["Workers"] != float64(4) {
		t.Errorf("Workers = %#v, want 4", cfg["Workers"])
	}
	if got := cfg["ExtExclusion"].([]interface{}); len(got) != 1 || got[0] != ".css" {
		t.Errorf("ExtExclusion = %#v, want [.css]", got)
	}
}

func TestNormalizePlatformConfigReportsMistypedKeys(t *testing.T) {
	cfg := map[string]interface{}{
		"DevOps":       "file",
		"Workers":      "ten",  // string where a number belongs
		"ResultAll":    "yes",  // string where a bool belongs
		"ExtExclusion": ".css", // string where an array belongs
		"Organization": nil,    // explicit null: absent, not mistyped
	}

	mistyped := normalizePlatformConfig(cfg)

	want := []string{"ExtExclusion", "ResultAll", "Workers"} // sorted, and no "Organization"
	if len(mistyped) != len(want) {
		t.Fatalf("mistyped = %v, want %v", mistyped, want)
	}
	for i := range want {
		if mistyped[i] != want[i] {
			t.Fatalf("mistyped = %v, want %v", mistyped, want)
		}
	}
	if _, ok := cfg["Workers"].(float64); !ok {
		t.Errorf("a mistyped key must be replaced by its default, got %#v", cfg["Workers"])
	}
	if _, ok := cfg["Organization"].(string); !ok {
		t.Errorf("an explicit null must be replaced by its default, got %#v", cfg["Organization"])
	}
}

func TestNormalizePlatformConfigNilMap(t *testing.T) {
	if got := normalizePlatformConfig(nil); got != nil {
		t.Errorf("normalizePlatformConfig(nil) = %v, want nil", got)
	}
}

// writeResultFile writes one per-repository result file of the shape the analysis
// phase produces.
func writeResultFile(t *testing.T, dir, name string, totalCodeLines, jsonCodeLines int) {
	t.Helper()
	payload := map[string]interface{}{
		"TotalCodeLines": totalCodeLines,
		"Results": []map[string]interface{}{
			{"Language": "Java", "CodeLines": totalCodeLines - jsonCodeLines},
			{"Language": utils.LanguageExcludedFromTotalLOC, "CodeLines": jsonCodeLines},
		},
	}
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), data, 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

// TestAggregateResultFilesCountsEveryRepository is the regression test for the
// directory count. Repository counting used to live inside the largest-repository
// branch, so it reported how many times the running maximum changed. These sizes
// ascend, which would have hidden the bug, then descend, which exposes it: a
// 5-directory scan reported 2.
func TestAggregateResultFilesCountsEveryRepository(t *testing.T) {
	dir := t.TempDir()
	writeResultFile(t, dir, "Result_alpha.json", 100, 0)
	writeResultFile(t, dir, "Result_bravo.json", 900, 0) // new maximum
	writeResultFile(t, dir, "Result_charlie.json", 50, 0)
	writeResultFile(t, dir, "Result_delta.json", 20, 0)
	writeResultFile(t, dir, "Result_echo.json", 10, 0)

	aggregate, err := aggregateResultFiles(dir, true)
	if err != nil {
		t.Fatalf("aggregateResultFiles: %v", err)
	}

	if aggregate.Repositories != 5 {
		t.Errorf("Repositories = %d, want 5 (one per result file, not per new maximum)",
			aggregate.Repositories)
	}
	if aggregate.TotalCodeLines != 1080 {
		t.Errorf("TotalCodeLines = %d, want 1080", aggregate.TotalCodeLines)
	}
	if aggregate.MaxRepo != "bravo" || aggregate.MaxCodeLines != 900 {
		t.Errorf("largest = %q/%d, want bravo/900", aggregate.MaxRepo, aggregate.MaxCodeLines)
	}
}

func TestAggregateResultFilesExcludesJSONFromTotals(t *testing.T) {
	dir := t.TempDir()
	writeResultFile(t, dir, "Result_alpha.json", 1000, 400)

	aggregate, err := aggregateResultFiles(dir, true)
	if err != nil {
		t.Fatalf("aggregateResultFiles: %v", err)
	}
	if aggregate.TotalCodeLines != 600 {
		t.Errorf("TotalCodeLines = %d, want 600 (1000 less the 400 excluded)", aggregate.TotalCodeLines)
	}
}

func TestAggregateResultFilesSkipsUnreadableAndNonResultFiles(t *testing.T) {
	dir := t.TempDir()
	writeResultFile(t, dir, "Result_alpha.json", 100, 0)
	if err := os.WriteFile(filepath.Join(dir, "Result_broken.json"), []byte("{not json"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("ignore me"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := os.Mkdir(filepath.Join(dir, "csv-report"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	aggregate, err := aggregateResultFiles(dir, true)
	if err != nil {
		t.Fatalf("aggregateResultFiles: %v", err)
	}
	if aggregate.Repositories != 1 {
		t.Errorf("Repositories = %d, want 1 (undecodable, non-JSON and directories all skipped)",
			aggregate.Repositories)
	}
}

func TestAggregateResultFilesMissingDirectory(t *testing.T) {
	if _, err := aggregateResultFiles(filepath.Join(t.TempDir(), "absent"), true); err == nil {
		t.Error("expected an error for a missing directory")
	}
}

// TestMissingRequiredKeysReportsIncompleteConfig covers the other half of
// normalizePlatformConfig: defaulting an absent key removes the panic, but a config
// that is genuinely incomplete must still say so rather than failing later with an
// unrelated-looking message ("The resource cannot be found.").
func TestMissingRequiredKeysReportsIncompleteConfig(t *testing.T) {
	cfg := map[string]interface{}{"DevOps": "azure"}
	normalizePlatformConfig(cfg)

	missing := missingRequiredKeys(cfg)
	want := map[string]bool{"Url": true, "AccessToken": true, "Organization": true}
	if len(missing) != len(want) {
		t.Fatalf("missing = %v, want %v", missing, want)
	}
	for _, key := range missing {
		if !want[key] {
			t.Errorf("unexpected key reported as required: %q", key)
		}
	}
}

func TestMissingRequiredKeysSatisfiedConfig(t *testing.T) {
	cfg := map[string]interface{}{
		"DevOps":       "azure",
		"Url":          "https://dev.azure.com/",
		"AccessToken":  "token",
		"Organization": "acme",
	}
	normalizePlatformConfig(cfg)

	if missing := missingRequiredKeys(cfg); len(missing) != 0 {
		t.Errorf("missing = %v, want none", missing)
	}
}

// TestMissingRequiredKeysBlankIsMissing guards against a whitespace-only value
// passing as supplied.
func TestMissingRequiredKeysBlankIsMissing(t *testing.T) {
	cfg := map[string]interface{}{
		"DevOps":       "azure",
		"Url":          "https://dev.azure.com/",
		"AccessToken":  "   ",
		"Organization": "acme",
	}
	normalizePlatformConfig(cfg)

	missing := missingRequiredKeys(cfg)
	if len(missing) != 1 || missing[0] != "AccessToken" {
		t.Errorf("missing = %v, want [AccessToken]", missing)
	}
}

// TestMissingRequiredKeysGitHubPersonalAccount pins the exemption that makes this
// list safe to enforce: getgithub resolves an empty Organization from the
// authenticated user for a personal account, so requiring it unconditionally would
// reject a configuration that works today.
func TestMissingRequiredKeysGitHubPersonalAccount(t *testing.T) {
	personal := map[string]interface{}{
		"DevOps": "github", "Url": "https://api.github.com/", "AccessToken": "token",
		"Org": false, "Organization": "",
	}
	normalizePlatformConfig(personal)
	if missing := missingRequiredKeys(personal); len(missing) != 0 {
		t.Errorf("personal account: missing = %v, want none (Organization is derived)", missing)
	}

	org := map[string]interface{}{
		"DevOps": "github", "Url": "https://api.github.com/", "AccessToken": "token",
		"Org": true, "Organization": "",
	}
	normalizePlatformConfig(org)
	if missing := missingRequiredKeys(org); len(missing) != 1 || missing[0] != "Organization" {
		t.Errorf("organisation: missing = %v, want [Organization]", missing)
	}
}

// TestMissingRequiredKeysGitLabRunsWithoutOrganization pins the second exemption: a
// real working GitLab config leaves Organization empty and takes its groups from the
// group field.
func TestMissingRequiredKeysGitLabRunsWithoutOrganization(t *testing.T) {
	cfg := map[string]interface{}{
		"DevOps": "gitlab", "Url": "https://gitlab.com/", "AccessToken": "token",
		"Organization": "",
	}
	normalizePlatformConfig(cfg)
	if missing := missingRequiredKeys(cfg); len(missing) != 0 {
		t.Errorf("missing = %v, want none (GitLab runs with Organization empty)", missing)
	}
}

// TestMissingRequiredKeysFilePlatform confirms the file platform is left to its own
// check, which already reports the Directory/FileLoad alternatives.
func TestMissingRequiredKeysFilePlatform(t *testing.T) {
	cfg := map[string]interface{}{"DevOps": "file"}
	normalizePlatformConfig(cfg)
	if missing := missingRequiredKeys(cfg); len(missing) != 0 {
		t.Errorf("missing = %v, want none", missing)
	}
}

// TestShippedSampleConfigSatisfiesRequiredKeys guards the enforcement against the
// configuration the project ships: every platform block in config_sample.json must
// pass, or the sample would be rejected by its own scanner.
func TestShippedSampleConfigSatisfiesRequiredKeys(t *testing.T) {
	data, err := os.ReadFile("config_sample.json")
	if err != nil {
		t.Skipf("config_sample.json not readable: %v", err)
	}
	var sample struct {
		Platforms map[string]interface{} `json:"platforms"`
	}
	if err := json.Unmarshal(data, &sample); err != nil {
		t.Fatalf("config_sample.json is not valid JSON: %v", err)
	}
	for name, raw := range sample.Platforms {
		cfg, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		normalizePlatformConfig(cfg)
		if missing := missingRequiredKeys(cfg); len(missing) != 0 {
			t.Errorf("config_sample.json platform %q would be rejected, missing %v", name, missing)
		}
	}
}
