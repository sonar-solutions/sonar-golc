//go:build resultsall
// +build resultsall

package main

import (
	"archive/zip"
	"bufio"
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/SonarSource-Demos/sonar-golc/pkg/utils"
)

//go:embed all:dist
var distFS embed.FS

const defaultPort = 8090

// getPort returns the server port from GOLC_RESULTS_PORT or PORT env, or defaultPort.
func getPort() int {
	for _, key := range []string{"GOLC_RESULTS_PORT", "PORT"} {
		if s := os.Getenv(key); s != "" {
			if p, err := strconv.Atoi(s); err == nil && p > 0 && p < 65536 {
				return p
			}
		}
	}
	return defaultPort
}

// HTTP header constants
const (
	contentTypeHeader   = "Content-Type"
	applicationJSONType = "application/json"
	applicationZipType  = "application/zip"
)

// Path constants for report directories
const (
	byFileReportDir       = "Results/byfile-report"
	byLanguageReportDir   = "Results/bylanguage-report"
	configResultsDir      = "Results/config"
	globalReportFile      = "Results/GlobalReport.json"
	codeLinesLanguageFile = "Results/code_lines_by_language.json"
)

// sanitizePathComponent sanitizes a path component to prevent path traversal attacks
func sanitizePathComponent(component string) string {
	// Remove any path traversal sequences
	component = strings.ReplaceAll(component, "..", "")
	component = strings.ReplaceAll(component, "/", "")
	component = strings.ReplaceAll(component, "\\", "")
	// Remove any null bytes
	component = strings.ReplaceAll(component, "\x00", "")
	// Trim whitespace
	component = strings.TrimSpace(component)
	return component
}

// buildSecurePath safely constructs a file path with validation
func buildSecurePath(basePath string, components ...string) string {
	sanitizedComponents := make([]string, len(components))
	for i, component := range components {
		sanitizedComponents[i] = sanitizePathComponent(component)
	}
	return filepath.Join(basePath, filepath.Join(sanitizedComponents...))
}

type Globalinfo struct {
	Organization           string `json:"Organization"`
	TotalLinesOfCode       string `json:"TotalLinesOfCode"`
	LargestRepository      string `json:"LargestRepository"`
	LinesOfCodeLargestRepo string `json:"LinesOfCodeLargestRepo"`
	DevOpsPlatform         string `json:"DevOpsPlatform"`
	NumberRepos            int    `json:"NumberRepos"`
}

type LanguageData struct {
	Language    string  `json:"Language"`
	CodeLines   int     `json:"CodeLines"`
	Percentage  float64 `json:"Percentage"`
	CodeLinesF  string  `json:"CodeLinesF"`
	RelativePct float64 `json:"-"`
}

type RepositoryData struct {
	Number      int    `json:"Number"`
	Repository  string `json:"Repository"`
	Org         string `json:"Org"`
	Branch      string `json:"Branch"`
	Lines       int    `json:"Lines"`
	BlankLines  int    `json:"BlankLines"`
	Comments    int    `json:"Comments"`
	CodeLines   int    `json:"CodeLines"`
	LinesF      string `json:"LinesF"`
	BlankLinesF string `json:"BlankLinesF"`
	CommentsF   string `json:"CommentsF"`
	CodeLinesF  string `json:"CodeLinesF"`
}

type ProjectBranch struct {
	Org         string `json:"Org"`
	ProjectKey  string `json:"ProjectKey"`
	RepoSlug    string `json:"RepoSlug"`
	MainBranch  string `json:"MainBranch"`
	LargestSize int64  `json:"LargestSize"`
}

type AnalysisResult struct {
	NumRepositories int             `json:"NumRepositories"`
	ProjectBranches []ProjectBranch `json:"ProjectBranches"`
}

type AnalysisResult_ProjectBranch = ProjectBranch

type RepositoryLanguageData struct {
	Language    string `json:"Language"`
	Files       int    `json:"Files"`
	Lines       int    `json:"Lines"`
	BlankLines  int    `json:"BlankLines"`
	Comments    int    `json:"Comments"`
	CodeLines   int    `json:"CodeLines"`
	FilesF      string `json:"FilesF"`
	LinesF      string `json:"LinesF"`
	BlankLinesF string `json:"BlankLinesF"`
	CommentsF   string `json:"CommentsF"`
	CodeLinesF  string `json:"CodeLinesF"`
}

type BranchData struct {
	Branch      string `json:"Branch"`
	Lines       int    `json:"Lines"`
	BlankLines  int    `json:"BlankLines"`
	Comments    int    `json:"Comments"`
	CodeLines   int    `json:"CodeLines"`
	LinesF      string `json:"LinesF"`
	BlankLinesF string `json:"BlankLinesF"`
	CommentsF   string `json:"CommentsF"`
	CodeLinesF  string `json:"CodeLinesF"`
}

type FileDetail struct {
	File        string `json:"File"`
	Lines       int    `json:"Lines"`
	BlankLines  int    `json:"BlankLines"`
	Comments    int    `json:"Comments"`
	CodeLines   int    `json:"CodeLines"`
	LinesF      string `json:"LinesF"`
	BlankLinesF string `json:"BlankLinesF"`
	CommentsF   string `json:"CommentsF"`
	CodeLinesF  string `json:"CodeLinesF"`
}

type RepositoryDetailData struct {
	Repository       string                   `json:"Repository"`
	MainBranch       string                   `json:"MainBranch"`
	Organization     string                   `json:"Organization"`
	TotalLines       int                      `json:"TotalLines"`
	TotalBlankLines  int                      `json:"TotalBlankLines"`
	TotalComments    int                      `json:"TotalComments"`
	TotalCodeLines   int                      `json:"TotalCodeLines"`
	TotalLinesF      string                   `json:"TotalLinesF"`
	TotalBlankLinesF string                   `json:"TotalBlankLinesF"`
	TotalCommentsF   string                   `json:"TotalCommentsF"`
	TotalCodeLinesF  string                   `json:"TotalCodeLinesF"`
	Languages        []RepositoryLanguageData `json:"Languages"`
	Files            []FileDetail             `json:"Files"`
	OtherBranches    []BranchData             `json:"OtherBranches"`
	GlobalReport     Globalinfo               `json:"GlobalReport"`
	Platform         string                   `json:"Platform"`
	PlatformIcon     string                   `json:"PlatformIcon"`
	RepositoryURL    string                   `json:"RepositoryURL"`
	NoteLOCExcluded  string                   `json:"NoteLOCExcluded"`
}

type PageData struct {
	Languages        []LanguageData
	GlobalReport     Globalinfo
	Repositories     []RepositoryData
	NoteLOCExcluded  string // Note that JSON is excluded from total (SonarQube behavior)
	Platform         string
}

var globalInfo Globalinfo       // Variable pour stocker les infos globales
var languageData []LanguageData // Variable pour stocker les données des langages

func getGlobalInfo() Globalinfo {
	return globalInfo
}

func getLanguageData() []LanguageData {
	return languageData
}

// commonPathPrefix returns the longest common slash-separated directory prefix
// shared by all paths (already normalised to forward slashes).
func commonPathPrefix(paths []string) string {
	if len(paths) == 0 {
		return ""
	}
	// Split each path into directory segments (drop the filename)
	dirOf := func(p string) string {
		idx := strings.LastIndex(p, "/")
		if idx < 0 {
			return ""
		}
		return p[:idx+1] // include trailing slash
	}
	prefix := dirOf(paths[0])
	for _, p := range paths[1:] {
		d := dirOf(p)
		for !strings.HasPrefix(d, prefix) {
			prefix = dirOf(strings.TrimRight(prefix, "/"))
			if prefix == "" {
				return ""
			}
		}
	}
	return prefix
}

// isMainBranch checks if a branch name is a main/default branch
func isMainBranch(branchName string) bool {
	mainBranches := []string{"main", "master", "develop", "development", "default"}
	for _, main := range mainBranches {
		if branchName == main {
			return true
		}
	}
	return false
}

func getRepositoryData() ([]RepositoryData, error) {
	var repositories []RepositoryData

	// Detect platform and read analysis results
	platform, analysisFile, err := detectPlatformAndReadAnalysis()
	if err != nil {
		fmt.Printf("❌ Error reading analysis result file: %v\n", err)
		return nil, err
	}

	var analysisResult AnalysisResult
	err = json.Unmarshal(analysisFile, &analysisResult)
	if err != nil {
		fmt.Printf("❌ Error decoding JSON analysis result file for platform %s: %v\n", platform, err)
		return nil, err
	}

	// Group by repository to avoid duplicate entries (needed for --all-branches mode)
	repoMap := make(map[string]AnalysisResult_ProjectBranch)

	// First pass: Group by repository and prefer main/master/default branches
	for _, branch := range analysisResult.ProjectBranches {
		repoKey := branch.RepoSlug

		// If we haven't seen this repo, or if this is a main branch, use it
		if existing, exists := repoMap[repoKey]; !exists || isMainBranch(branch.MainBranch) {
			// Only override if current is main branch, or existing is not main branch
			if !exists || isMainBranch(branch.MainBranch) || !isMainBranch(existing.MainBranch) {
				repoMap[repoKey] = branch
			}
		}
	}

	// Process each unique repository (now showing only one branch per repository)
	i := 0
	for _, branch := range repoMap {
		i++
		// Construct filename for byfile report using platform-specific logic
		var fileName, byLanguagePath string
		if platform == "file" {
			// File mode uses simpler naming: Result_{slug}_byfile.json
			fileName = buildSecurePath(byFileReportDir,
				fmt.Sprintf("Result_%s_byfile.json", sanitizePathComponent(branch.RepoSlug)))
			byLanguagePath = buildSecurePath(byLanguageReportDir,
				fmt.Sprintf("Result_%s.json", sanitizePathComponent(branch.RepoSlug)))
		} else {
			firstPart := getFirstPartForPlatform(platform, branch, branch.RepoSlug)
			fileName = buildSecurePath(byFileReportDir,
				fmt.Sprintf("Result_%s_%s_%s_byfile.json",
					sanitizePathComponent(firstPart),
					sanitizePathComponent(branch.RepoSlug),
					sanitizePathComponent(branch.MainBranch)))
			byLanguagePath = buildSecurePath(byLanguageReportDir,
				fmt.Sprintf("Result_%s_%s_%s.json",
					sanitizePathComponent(firstPart),
					sanitizePathComponent(branch.RepoSlug),
					sanitizePathComponent(branch.MainBranch)))
		}

		// Read the byfile report
		fileData, err := os.ReadFile(fileName)
		if err != nil {
			fmt.Printf("❌ Error reading byfile report %s: %v\n", fileName, err)
			continue // Skip this repository if file doesn't exist
		}

		// Parse the JSON structure
		var reportData struct {
			TotalLines      int `json:"TotalLines"`
			TotalBlankLines int `json:"TotalBlankLines"`
			TotalComments   int `json:"TotalComments"`
			TotalCodeLines  int `json:"TotalCodeLines"`
		}

		err = json.Unmarshal(fileData, &reportData)
		if err != nil {
			fmt.Printf("❌ Error decoding JSON byfile report %s: %v\n", fileName, err)
			continue
		}

		// Code lines for report total: exclude JSON to match SonarQube behavior
		codeLinesForReport := reportData.TotalCodeLines
		if langData, err := os.ReadFile(byLanguagePath); err == nil {
			var byLang struct {
				Results []struct {
					Language  string `json:"Language"`
					CodeLines int    `json:"CodeLines"`
				} `json:"Results"`
			}
			if json.Unmarshal(langData, &byLang) == nil {
				for _, r := range byLang.Results {
					if strings.TrimSpace(r.Language) == utils.LanguageExcludedFromTotalLOC {
						codeLinesForReport = reportData.TotalCodeLines - r.CodeLines
						break
					}
				}
			}
		}

		// Create repository data entry (CodeLines excludes JSON for report total)
		repo := RepositoryData{
			Number:      i,
			Repository:  branch.RepoSlug,
			Org:         branch.Org,
			Branch:      branch.MainBranch,
			Lines:       reportData.TotalLines,
			BlankLines:  reportData.TotalBlankLines,
			Comments:    reportData.TotalComments,
			CodeLines:   codeLinesForReport,
			LinesF:      utils.FormatCodeLines(float64(reportData.TotalLines)),
			BlankLinesF: utils.FormatCodeLines(float64(reportData.TotalBlankLines)),
			CommentsF:   utils.FormatCodeLines(float64(reportData.TotalComments)),
			CodeLinesF:  utils.FormatCodeLines(float64(codeLinesForReport)),
		}

		repositories = append(repositories, repo)
	}

	// Sort repositories by Code Lines (descending) by default
	sort.Slice(repositories, func(i, j int) bool {
		return repositories[i].CodeLines > repositories[j].CodeLines
	})

	// Update numbers after sorting
	for i := range repositories {
		repositories[i].Number = i + 1
	}

	return repositories, nil
}

func detectPlatformAndReadAnalysis() (string, []byte, error) {
	// Try to detect platform from existing analysis result files
	// Supporting all platforms from config_sample.json
	platforms := []string{"github", "gitlab", "bitbucket", "bitbucket_dc", "azure", "file"}

	for _, platform := range platforms {
		filePath := fmt.Sprintf("%s/analysis_result_%s.json", configResultsDir, platform)
		if data, err := os.ReadFile(filePath); err == nil {
			return platform, data, nil
		}
	}

	// Default fallback to github if no specific file found
	data, err := os.ReadFile(fmt.Sprintf("%s/analysis_result_github.json", configResultsDir))
	if err != nil {
		return "", nil, fmt.Errorf("no analysis result file found")
	}
	return "github", data, nil
}

// Helper function to determine the correct first part of filename based on platform
func getFirstPartForPlatform(platform string, branch AnalysisResult_ProjectBranch, repoName string) string {
	switch platform {
	case "azure":
		// Azure uses ProjectKey for filenames
		if branch.ProjectKey != "" {
			return branch.ProjectKey
		}
		// Fallback to repoName if ProjectKey is not available
		return repoName
	case "bitbucket", "bitbucket_dc":
		// Bitbucket uses ProjectKey for filenames
		if branch.ProjectKey != "" {
			return branch.ProjectKey
		}
		// Fallback to Org if ProjectKey is not available
		return branch.Org
	case "github", "gitlab", "file":
		// GitHub, GitLab, and file use Org
		return branch.Org
	default:
		// Default fallback to Org
		return branch.Org
	}
}

// Helper function for cases where we only have orgName and repoName
func getFirstPartForFilename(platform, orgName, repoName string) string {
	switch platform {
	case "azure":
		// Azure uses ProjectKey (equals repoName) for filenames
		return repoName
	case "bitbucket", "bitbucket_dc":
		// Bitbucket uses ProjectKey for filenames - need to look it up from analysis results
		_, analysisFile, err := detectPlatformAndReadAnalysis()
		if err == nil {
			var analysisResult AnalysisResult
			err = json.Unmarshal(analysisFile, &analysisResult)
			if err == nil {
				// Find the repository and get its ProjectKey
				for _, branch := range analysisResult.ProjectBranches {
					if branch.RepoSlug == repoName {
						if branch.ProjectKey != "" {
							return branch.ProjectKey
						}
						break
					}
				}
			}
		}
		// Fallback to orgName if ProjectKey lookup fails
		return orgName
	case "github", "gitlab", "file":
		// GitHub, GitLab, and file use Org
		return orgName
	default:
		// Default fallback to Org
		return orgName
	}
}

func getOtherBranchesData(orgName, repoName, currentBranch string) []BranchData {
	var branches []BranchData

	// Detect platform to know which naming pattern to use
	platform, _, err := detectPlatformAndReadAnalysis()
	if err != nil {
		fmt.Printf("Warning: Could not detect platform: %v\n", err)
		return branches
	}

	// Get the correct first part for filename based on platform
	firstPart := getFirstPartForFilename(platform, orgName, repoName)

	// Look for all byfile reports for this repository (different branches)
	pattern := buildSecurePath(byFileReportDir,
		fmt.Sprintf("Result_%s_%s_*_byfile.json",
			sanitizePathComponent(firstPart),
			sanitizePathComponent(repoName)))
	matches, err := filepath.Glob(pattern)
	if err != nil {
		fmt.Printf("Warning: Could not search for branch files: %v\n", err)
		return branches
	}

	for _, filePath := range matches {
		// Extract branch name from filename
		filename := filepath.Base(filePath)
		// Format: Result_ORG_REPO_BRANCH_byfile.json
		parts := strings.Split(filename, "_")
		if len(parts) < 4 {
			continue
		}

		// Find the branch part (everything between REPO and "byfile.json")
		branchPart := strings.TrimSuffix(parts[len(parts)-2], ".json")
		if branchPart == "byfile" && len(parts) >= 5 {
			// Handle case where branch name is the second-to-last part
			branchPart = parts[len(parts)-3]
		}

		// Extract actual branch name - more robust parsing
		// Remove the prefix and suffix to get the branch name
		// Use firstPart (which is ProjectKey for Bitbucket, Org for others) instead of orgName
		prefix := fmt.Sprintf("Result_%s_%s_", sanitizePathComponent(firstPart), sanitizePathComponent(repoName))
		suffix := "_byfile.json"
		branchName := strings.TrimSuffix(strings.TrimPrefix(filename, prefix), suffix)

		// Skip the current branch (it's already shown in the main stats)
		if branchName == currentBranch {
			continue
		}

		// Read the byfile report for this branch
		data, err := os.ReadFile(filePath)
		if err != nil {
			fmt.Printf("Warning: Could not read branch file %s: %v\n", filePath, err)
			continue
		}

		var branchReport struct {
			TotalLines      int `json:"TotalLines"`
			TotalBlankLines int `json:"TotalBlankLines"`
			TotalComments   int `json:"TotalComments"`
			TotalCodeLines  int `json:"TotalCodeLines"`
		}

		err = json.Unmarshal(data, &branchReport)
		if err != nil {
			fmt.Printf("Warning: Could not parse branch file %s: %v\n", filePath, err)
			continue
		}

		// Create formatted branch data
		branchData := BranchData{
			Branch:      branchName,
			Lines:       branchReport.TotalLines,
			BlankLines:  branchReport.TotalBlankLines,
			Comments:    branchReport.TotalComments,
			CodeLines:   branchReport.TotalCodeLines,
			LinesF:      utils.FormatCodeLines(float64(branchReport.TotalLines)),
			BlankLinesF: utils.FormatCodeLines(float64(branchReport.TotalBlankLines)),
			CommentsF:   utils.FormatCodeLines(float64(branchReport.TotalComments)),
			CodeLinesF:  utils.FormatCodeLines(float64(branchReport.TotalCodeLines)),
		}

		branches = append(branches, branchData)
	}

	return branches
}

func getPlatformInfoAndURL(platform, org, repo string) (string, string) {
	switch platform {
	case "github":
		return "fab fa-github", fmt.Sprintf("https://github.com/%s/%s", org, repo)
	case "gitlab":
		return "fab fa-gitlab", fmt.Sprintf("https://gitlab.com/%s/%s", org, repo)
	case "bitbucket":
		return "fab fa-bitbucket", fmt.Sprintf("https://bitbucket.org/%s/%s", org, repo)
	case "azure":
		return "fab fa-microsoft", fmt.Sprintf("https://dev.azure.com/%s/_git/%s", org, repo)
	default:
		return "fab fa-github", fmt.Sprintf("https://github.com/%s/%s", org, repo)
	}
}

func getRepositoryDetailData(repoName, branchName string) (*RepositoryDetailData, error) {
	// Detect platform and read analysis results
	platform, analysisFile, err := detectPlatformAndReadAnalysis()
	if err != nil {
		return nil, fmt.Errorf("error reading analysis file: %v", err)
	}

	var analysisResult AnalysisResult
	err = json.Unmarshal(analysisFile, &analysisResult)
	if err != nil {
		return nil, fmt.Errorf("error decoding analysis result file: %v", err)
	}

	// Find the repository in analysis results
	var orgName string
	var foundBranch *ProjectBranch
	for _, branch := range analysisResult.ProjectBranches {
		if branch.RepoSlug == repoName {
			orgName = branch.Org
			foundBranch = &branch
			break
		}
	}

	if foundBranch == nil {
		return nil, fmt.Errorf("repository %s not found in analysis results", repoName)
	}

	// Read the byfile report for totals
	// Use getFirstPartForPlatform with the branch object to get correct ProjectKey for Bitbucket
	var firstPart string
	if foundBranch != nil {
		firstPart = getFirstPartForPlatform(platform, *foundBranch, repoName)
	} else {
		firstPart = getFirstPartForFilename(platform, orgName, repoName)
	}
	var byFileReportPath string
	if platform == "file" {
		byFileReportPath = buildSecurePath(byFileReportDir,
			fmt.Sprintf("Result_%s_byfile.json", sanitizePathComponent(repoName)))
	} else {
		byFileReportPath = buildSecurePath(byFileReportDir,
			fmt.Sprintf("Result_%s_%s_%s_byfile.json",
				sanitizePathComponent(firstPart),
				sanitizePathComponent(repoName),
				sanitizePathComponent(branchName)))
	}

	byFileData, err := os.ReadFile(byFileReportPath)
	if err != nil {
		return nil, fmt.Errorf("error reading byfile report %s: %v", byFileReportPath, err)
	}

	var byFileReport struct {
		TotalLines      int `json:"TotalLines"`
		TotalBlankLines int `json:"TotalBlankLines"`
		TotalComments   int `json:"TotalComments"`
		TotalCodeLines  int `json:"TotalCodeLines"`
		Results         []struct {
			File       string `json:"File"`
			Lines      int    `json:"Lines"`
			BlankLines int    `json:"BlankLines"`
			Comments   int    `json:"Comments"`
			CodeLines  int    `json:"CodeLines"`
		} `json:"Results"`
	}

	err = json.Unmarshal(byFileData, &byFileReport)
	if err != nil {
		return nil, fmt.Errorf("error decoding byfile report: %v", err)
	}

	// Read the bylanguage report for language breakdown
	var byLanguageReportPath string
	if platform == "file" {
		byLanguageReportPath = buildSecurePath(byLanguageReportDir,
			fmt.Sprintf("Result_%s.json", sanitizePathComponent(repoName)))
	} else {
		byLanguageReportPath = buildSecurePath(byLanguageReportDir,
			fmt.Sprintf("Result_%s_%s_%s.json",
				sanitizePathComponent(firstPart),
				sanitizePathComponent(repoName),
				sanitizePathComponent(branchName)))
	}

	languageData, err := os.ReadFile(byLanguageReportPath)
	if err != nil {
		return nil, fmt.Errorf("error reading bylanguage report %s: %v", byLanguageReportPath, err)
	}

	var languageReport struct {
		TotalFiles      int                      `json:"TotalFiles"`
		TotalLines      int                      `json:"TotalLines"`
		TotalBlankLines int                      `json:"TotalBlankLines"`
		TotalComments   int                      `json:"TotalComments"`
		TotalCodeLines  int                      `json:"TotalCodeLines"`
		Results         []RepositoryLanguageData `json:"Results"`
	}

	err = json.Unmarshal(languageData, &languageReport)
	if err != nil {
		return nil, fmt.Errorf("error decoding bylanguage report: %v", err)
	}

	// Read global report
	globalData, err := os.ReadFile(globalReportFile)
	if err != nil {
		return nil, fmt.Errorf("error reading GlobalReport.json: %v", err)
	}

	var globalInfo Globalinfo
	err = json.Unmarshal(globalData, &globalInfo)
	if err != nil {
		return nil, fmt.Errorf("error decoding GlobalReport.json: %v", err)
	}

	// Process language data to add formatted fields
	var formattedLanguages []RepositoryLanguageData
	for _, lang := range languageReport.Results {
		formattedLang := RepositoryLanguageData{
			Language:    lang.Language,
			Files:       lang.Files,
			Lines:       lang.Lines,
			BlankLines:  lang.BlankLines,
			Comments:    lang.Comments,
			CodeLines:   lang.CodeLines,
			FilesF:      utils.FormatCodeLines(float64(lang.Files)),
			LinesF:      utils.FormatCodeLines(float64(lang.Lines)),
			BlankLinesF: utils.FormatCodeLines(float64(lang.BlankLines)),
			CommentsF:   utils.FormatCodeLines(float64(lang.Comments)),
			CodeLinesF:  utils.FormatCodeLines(float64(lang.CodeLines)),
		}
		formattedLanguages = append(formattedLanguages, formattedLang)
	}

	// Build per-file list (populated when byfile report has file-level Results).
	// Strip the common directory prefix so paths are relative to the repo root.
	// This is needed on Windows where goloc stores full temp clone paths.
	var rawPaths []string
	for _, f := range byFileReport.Results {
		rawPaths = append(rawPaths, filepath.ToSlash(f.File))
	}
	prefix := commonPathPrefix(rawPaths)
	var formattedFiles []FileDetail
	for _, f := range byFileReport.Results {
		rel := strings.TrimPrefix(filepath.ToSlash(f.File), prefix)
		rel = strings.TrimPrefix(rel, "/")
		if rel == "" {
			rel = filepath.Base(f.File)
		}
		formattedFiles = append(formattedFiles, FileDetail{
			File:        rel,
			Lines:       f.Lines,
			BlankLines:  f.BlankLines,
			Comments:    f.Comments,
			CodeLines:   f.CodeLines,
			LinesF:      utils.FormatCodeLines(float64(f.Lines)),
			BlankLinesF: utils.FormatCodeLines(float64(f.BlankLines)),
			CommentsF:   utils.FormatCodeLines(float64(f.Comments)),
			CodeLinesF:  utils.FormatCodeLines(float64(f.CodeLines)),
		})
	}

	// Get platform info and repository URL
	platformIcon, repositoryURL := getPlatformInfoAndURL(platform, orgName, repoName)

	// Code lines for report total: exclude JSON to match SonarQube behavior
	totalCodeLinesForReport := byFileReport.TotalCodeLines
	for _, lang := range languageReport.Results {
		if strings.TrimSpace(lang.Language) == utils.LanguageExcludedFromTotalLOC {
			totalCodeLinesForReport = byFileReport.TotalCodeLines - lang.CodeLines
			break
		}
	}

	// Get other branches by finding all byfile reports for this repository
	otherBranches := getOtherBranchesData(orgName, repoName, branchName)

	repoDetail := &RepositoryDetailData{
		Repository:       repoName,
		MainBranch:       branchName,
		Organization:     orgName,
		TotalLines:       byFileReport.TotalLines,
		TotalBlankLines:  byFileReport.TotalBlankLines,
		TotalComments:    byFileReport.TotalComments,
		TotalCodeLines:   totalCodeLinesForReport,
		TotalLinesF:      utils.FormatCodeLines(float64(byFileReport.TotalLines)),
		TotalBlankLinesF: utils.FormatCodeLines(float64(byFileReport.TotalBlankLines)),
		TotalCommentsF:   utils.FormatCodeLines(float64(byFileReport.TotalComments)),
		TotalCodeLinesF:  utils.FormatCodeLines(float64(totalCodeLinesForReport)),
		Languages:        formattedLanguages,
		Files:            formattedFiles,
		OtherBranches:    otherBranches,
		GlobalReport:     globalInfo,
		Platform:         platform,
		PlatformIcon:     platformIcon,
		RepositoryURL:    repositoryURL,
		NoteLOCExcluded:  utils.NoteExcludedFromTotal,
	}

	return repoDetail, nil
}

func startServer(port int) {
	fmt.Printf("✅ Server started on http://localhost:%d\n", port)
	fmt.Println("✅ Please type < Ctrl+C > to stop the server")
	http.ListenAndServe(fmt.Sprintf(":%d", port), nil)
}

func isPortOpen(port int) bool {
	address := fmt.Sprintf("localhost:%d", port)
	conn, err := net.Dial("tcp", address)
	if err != nil {
		return false
	}
	defer conn.Close()
	return true
}

func ZipDirectory(source string, target string) error {
	zipFile, err := os.Create(target)
	if err != nil {
		return err
	}
	defer zipFile.Close()

	zipWriter := zip.NewWriter(zipFile)
	defer zipWriter.Close()

	return filepath.Walk(source, func(file string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		relativePath, err := filepath.Rel(filepath.Dir(source), file)
		if err != nil {
			return err
		}

		if fi.IsDir() {
			_, err := zipWriter.Create(relativePath + "/")
			return err
		}

		fileToZip, err := os.Open(file)
		if err != nil {
			return err
		}
		defer fileToZip.Close()

		writer, err := zipWriter.Create(relativePath)
		if err != nil {
			return err
		}

		_, err = io.Copy(writer, fileToZip)
		return err
	})
}

func zipResults(w http.ResponseWriter, r *http.Request) {
	resultsDir := "./Results"
	target := "Results.zip"

	err := ZipDirectory(resultsDir, target)
	if err != nil {
		http.Error(w, "Error creating zip file", http.StatusInternalServerError)
		return
	}

	w.Header().Set(contentTypeHeader, applicationZipType)
	w.Header().Set("Content-Disposition", "attachment; filename=Results.zip")

	http.ServeFile(w, r, "Results.zip")
}

// buildLanguageSummary converts raw language data into a sorted, percentage-annotated slice.
func buildLanguageSummary(rawData []LanguageData) []LanguageData {
	totals := make(map[string]int)
	for _, r := range rawData {
		totals[r.Language] += r.CodeLines
	}

	totalExcl := 0
	var languages []LanguageData
	for lang, total := range totals {
		if strings.TrimSpace(lang) != utils.LanguageExcludedFromTotalLOC {
			totalExcl += total
		}
		languages = append(languages, LanguageData{
			Language:   lang,
			CodeLines:  total,
			CodeLinesF: utils.FormatCodeLines(float64(total)),
		})
	}

	applyLanguagePercentages(languages, totalExcl)

	sort.Slice(languages, func(i, j int) bool {
		return languages[i].CodeLines > languages[j].CodeLines
	})
	if len(languages) > 0 && languages[0].CodeLines > 0 {
		maxLOC := float64(languages[0].CodeLines)
		for i := range languages {
			languages[i].RelativePct = float64(languages[i].CodeLines) / maxLOC * 100
		}
	}
	return languages
}

// applyLanguagePercentages sets the Percentage field for each language entry.
func applyLanguagePercentages(languages []LanguageData, totalExcludingJSON int) {
	for i := range languages {
		if strings.TrimSpace(languages[i].Language) == utils.LanguageExcludedFromTotalLOC || totalExcludingJSON == 0 {
			languages[i].Percentage = 0
		} else {
			languages[i].Percentage = float64(languages[i].CodeLines) / float64(totalExcludingJSON) * 100
		}
	}
}

// loadApplicationData loads and processes all required data files
func loadApplicationData() (PageData, error) {
	var pageData PageData

	inputFileData, err := os.ReadFile(codeLinesLanguageFile)
	if err != nil {
		return pageData, fmt.Errorf("error reading code_lines_by_language.json file: %v", err)
	}

	err = json.Unmarshal(inputFileData, &languageData)
	if err != nil {
		return pageData, fmt.Errorf("error decoding JSON code_lines_by_language.json file: %v", err)
	}

	languages := buildLanguageSummary(languageData)

	data0, err := os.ReadFile(globalReportFile)
	if err != nil {
		return pageData, fmt.Errorf("error reading GlobalReport.json file: %v", err)
	}

	err = json.Unmarshal(data0, &globalInfo)
	if err != nil {
		return pageData, fmt.Errorf("error decoding JSON GlobalReport.json file: %v", err)
	}

	repositoryData, err := getRepositoryData()
	if err != nil {
		fmt.Println("❌ Error loading repository data:", err)
		repositoryData = []RepositoryData{}
	}

	detectedPlatform, _, _ := detectPlatformAndReadAnalysis()

	pageData = PageData{
		Languages:       languages,
		GlobalReport:    globalInfo,
		Repositories:    repositoryData,
		NoteLOCExcluded: utils.NoteExcludedFromTotal,
		Platform:        detectedPlatform,
	}

	return pageData, nil
}

// setupHTTPHandlers configures all HTTP route handlers
func setupHTTPHandlers(pageData PageData) {
	// Load HTML template
	tmpl := template.Must(template.New("index").Parse(htmlTemplate))

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		err := tmpl.Execute(w, pageData)
		if err != nil {
			http.Error(w, "❌ Error executing HTML template", http.StatusInternalServerError)
			return
		}
	})

	http.HandleFunc("/download", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			zipResults(w, r)
			return
		}
		http.Error(w, "❌ Method not allowed", http.StatusMethodNotAllowed)
	})

	http.HandleFunc("/reports/", func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(r.URL.Path, "/reports/")
		var filePath string
		switch name {
		case "global-report.pdf":
			filePath = "Results/GlobalReport.pdf"
		case "repository-summary.pdf":
			filePath = "Results/byfile-report/pdf-report/repository_summary.pdf"
		case "repository-summary.csv":
			filePath = "Results/byfile-report/csv-report/repository_summary.csv"
		default:
			http.NotFound(w, r)
			return
		}
		if _, err := os.Stat(filePath); os.IsNotExist(err) {
			http.Error(w, "Report not yet available", http.StatusNotFound)
			return
		}
		http.ServeFile(w, r, filePath)
	})

	// API Endpoint for Language Data
	http.HandleFunc("/api/languages", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set(contentTypeHeader, applicationJSONType)
		json.NewEncoder(w).Encode(languageData)
	})

	// API Endpoint for Global Info
	http.HandleFunc("/api/global-info", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set(contentTypeHeader, applicationJSONType)
		json.NewEncoder(w).Encode(globalInfo)
	})

	// API Endpoint for Repository Data
	http.HandleFunc("/api/repositories", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set(contentTypeHeader, applicationJSONType)
		json.NewEncoder(w).Encode(pageData.Repositories)
	})

	// Repository Detail Page Handler
	http.HandleFunc("/repository/", func(w http.ResponseWriter, r *http.Request) {
		// Parse URL path to extract repository name and branch.
		// Branch names may contain "/" (e.g. feature/my-branch), so split on
		// the first "/" only: everything after is the branch name.
		rest := strings.TrimPrefix(r.URL.Path, "/repository/")
		slashIdx := strings.Index(rest, "/")
		if slashIdx < 0 {
			http.Error(w, "Invalid repository URL", http.StatusBadRequest)
			return
		}

		repoName := rest[:slashIdx]
		branchName := rest[slashIdx+1:]
		if repoName == "" || branchName == "" {
			http.Error(w, "Invalid repository URL", http.StatusBadRequest)
			return
		}

		// Get repository detail data
		repoData, err := getRepositoryDetailData(repoName, branchName)
		if err != nil {
			http.Error(w, fmt.Sprintf("Error loading repository data: %v", err), http.StatusInternalServerError)
			return
		}

		// Execute repository detail template
		tmplRepo := template.Must(template.New("repository").Parse(repositoryDetailTemplate))
		err = tmplRepo.Execute(w, repoData)
		if err != nil {
			http.Error(w, "Error executing repository template", http.StatusInternalServerError)
			return
		}
	})
}

// handleServerStartup manages port checking and server startup
func handleServerStartup() {
	port := getPort()
	fmt.Println("✅ Launching web visualization...")
	sub, err := fs.Sub(distFS, "dist")
	if err != nil {
		panic("embedded dist/ not found: " + err.Error())
	}
	http.Handle("/dist/", http.StripPrefix("/dist/", http.FileServer(http.FS(sub))))

	if isPortOpen(port) {
		handlePortConflict(port)
	} else {
		startServer(port)
	}
}

// isStdinTTY returns true if stdin is a terminal (interactive).
func isStdinTTY() bool {
	fi, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}

// handlePortConflict handles the case when the chosen port is in use
func handlePortConflict(port int) {
	fmt.Printf("❗️ Port %d is already in use.\n", port)
	if !isStdinTTY() {
		fmt.Println("❌ Not running in a terminal. Set PORT environment variable or free the port and try again.")
		os.Exit(1)
	}
	reader := bufio.NewReader(os.Stdin)

	fmt.Print("✅ Please enter the port you wish to use : ")
	portStr, _ := reader.ReadString('\n')
	portStr = strings.TrimSpace(portStr)
	newPort, err := strconv.Atoi(portStr)
	if err != nil {
		fmt.Println("❌ Invalid port...")
		os.Exit(1)
	}
	if isPortOpen(newPort) {
		fmt.Printf("❌ Port %d is already in use...\n", newPort)
		os.Exit(1)
	} else {
		startServer(newPort)
	}
}

func main() {
	utils.ChdirToBinaryDir()

	pageData, err := loadApplicationData()
	if err != nil {
		fmt.Println("❌", err)
		return
	}

	setupHTTPHandlers(pageData)
	handleServerStartup()
}

// HTML template
const htmlTemplate = `
<!DOCTYPE html>
<html lang="en-US" dir="ltr">
  <head>
    <meta charset="utf-8">
    <meta http-equiv="X-UA-Compatible" content="IE=edge">
    <meta name="viewport" content="width=device-width, initial-scale=1">
    <title>Result Go LOC</title>
    <link href="https://fonts.googleapis.com/css2?family=Manrope:wght@200;300;400;500;600;700&amp;display=swap" rel="stylesheet">
    <link href="/dist/css/theme.min.css" rel="stylesheet" type="text/css" />
    <link href="/dist/vendors/fontawesome/css/all.min.css" rel="stylesheet" type="text/css" />
    <style>
        .chart-container {
            flex: 1;
        }
        .percentage-container {
            flex: 1;
            padding-left: 20px;
        }
            .modal {
            display: none; 
            position: fixed; 
            z-index: 1; 
            left: 0;
            top: 0;
            width: 100%; 
            height: 100%; 
            overflow: auto; 
            background-color: rgb(0,0,0);
            background-color: rgba(0,0,0,0.4);
            padding-top: 60px;
        }
        .modal-content {
            background-color: #fefefe;
            margin: 5% auto; 
            padding: 20px;
            border: 1px solid #888;
            width: 80%; 
        }
            .close {
            color: #aaa;
            float: right;
            font-size: 28px;
            font-weight: bold;
        }
        .close:hover,
        .close:focus {
            color: black;
            text-decoration: none;
            cursor: pointer;
        }


      .css-xvw69q {
        background-color: rgb(255, 255, 255);
        border: 1px solid rgb(225, 230, 243);
        padding: 1.5rem;
        border-radius: 0.25rem;
      }
      
      html {
        scroll-behavior: smooth;
      }
      
      .navbar {
        background: rgba(253, 106, 133, 0.15) !important;
        backdrop-filter: blur(10px);
        box-shadow: 0 1px 3px rgba(0,0,0,0.1);
        border-bottom: 1px solid rgba(253, 106, 133, 0.2);
        padding: 0.25rem 0 !important;
        min-height: 3rem !important;
      }
      
      .navbar-brand {
        padding: 0.25rem 0 !important;
      }
      
      .navbar-brand img {
        height: 2rem !important;
        filter: brightness(1.1);
      }
      
      .navbar-nav {
        padding: 0.25rem 0 !important;
      }
      
      .navbar-nav .nav-link {
        font-weight: 500;
        color: rgba(255,255,255,0.9) !important;
        transition: all 0.3s ease;
        padding: 0.25rem 1rem !important;
        font-size: 0.9rem;
      }
      
      .navbar-nav .nav-link:hover {
        color: #fd6a85 !important;
        background-color: rgba(253, 106, 133, 0.1);
        border-radius: 4px;
      }
      
      .repo-link {
        color: #007bff;
        text-decoration: none;
        font-weight: 500;
        transition: all 0.3s ease;
      }
      
      .repo-link:hover {
        color: #fd6a85;
        text-decoration: underline;
      }
       .sw-flex {
        display: flex !important;
      }
      .sw-items-baseline {
       align-items: baseline !important;
      }
      .sw-mt-4 {
        margin-top: 1rem !important;
      }
        .rule-desc, .markdown {
          line-height: 1.5;
      }
      
      /* Sortable table styles */
      .sortable {
          cursor: pointer;
          user-select: none;
          transition: background-color 0.2s;
      }
      
      .sortable:hover {
          background-color: rgba(52, 144, 220, 0.3) !important;
      }
      
      .sort-icon {
          float: right;
          margin-left: 0.5rem;
          opacity: 0.7;
          transition: opacity 0.2s;
      }
      
      .sortable:hover .sort-icon {
          opacity: 1;
      }
      
      .sortable.sorted .sort-icon {
          opacity: 1;
          color: #ffd700;
      }
      
      /* Make repository table full width */
      .repository-table-container .card-body {
          padding: 0;
      }

      .repository-table-container .table-responsive {
          margin: 0;
      }

      .repository-table-container .table {
          margin-top: 0;
          margin-bottom: 0;
      }
      /* Language bar chart */
      .lang-bar-row { margin-bottom: 0.6rem; }
      .lang-bar-header { display:flex; justify-content:space-between; align-items:baseline; margin-bottom:3px; gap:4px; }
      .lang-bar-name { font-weight:600; font-size:0.85rem; max-width:56%; overflow:hidden; text-overflow:ellipsis; white-space:nowrap; }
      .lang-bar-meta { font-size:0.75rem; opacity:0.85; white-space:nowrap; text-align:right; }
      .lang-bar-track { background:rgba(255,255,255,.18); border-radius:3px; height:5px; overflow:hidden; }
      .lang-bar-fill { background:rgba(255,255,255,.85); border-radius:3px; height:5px; transition:width .4s ease; }
      /* Reports dropdown — sharp rectangular corners */
      .dropdown-menu { border-radius: 4px !important; }
    </style>
    <script src="/dist/vendors/chartjs/chart.js"></script>
    <script src="/dist/vendors/bootstrap/js/bootstrap.bundle.min.js"></script>
  </head>
  <body>
    <main class="main" id="top">
      <nav class="navbar navbar-expand-lg fixed-top navbar-dark" data-navbar-on-scroll="data-navbar-on-scroll">
       <div class="container"><a class="navbar-brand" href="index.html"><img src="dist/img/Logo.png" alt="" /></a>
          <button class="navbar-toggler" type="button" data-bs-toggle="collapse" data-bs-target="#navbarSupportedContent" aria-controls="navbarSupportedContent" aria-expanded="false" aria-label="Toggle navigation"><i class="fa-solid fa-bars text-white fs-3"></i></button>
          <div class="collapse navbar-collapse" id="navbarSupportedContent">
            <ul class="navbar-nav ms-auto mt-2 mt-lg-0">
              <li class="nav-item dropdown">
                <a class="nav-link dropdown-toggle" href="#" id="reportsDropdown" role="button" data-bs-toggle="dropdown" aria-expanded="false">
                  <i class="fas fa-file-pdf me-1"></i>Reports
                </a>
                <ul class="dropdown-menu dropdown-menu-end" aria-labelledby="reportsDropdown">
                  <li><a class="dropdown-item" href="/reports/global-report.pdf" download><i class="fas fa-file-pdf text-primary me-2"></i>Global Report PDF</a></li>
                  <li><a class="dropdown-item" href="/reports/repository-summary.pdf" download><i class="fas fa-file-pdf text-success me-2"></i>Repository Summary PDF</a></li>
                  <li><a class="dropdown-item" href="/reports/repository-summary.csv" download><i class="fas fa-file-csv me-2" style="color:#e67e22;"></i>Repository Summary CSV</a></li>
                  <li><hr class="dropdown-divider"></li>
                  <li><a class="dropdown-item" href="/download"><i class="fas fa-file-archive me-2"></i>Download All ZIP</a></li>
                </ul>
              </li>
              <li class="nav-item"><a class="nav-link" aria-current="page" title="API REF" href="#" id="apiButton">API</a></li>
            </ul>
          </div>
        </div>
      </nav>
      <div class="bg-dark"><img class="img-fluid position-absolute end-0" src="dist/img/bg.png" alt="" />
      <section>
        <div class="container">
          <div class="row align-items-center py-lg-8 py-6" style="margin-top: -5%">
            <div class="col-lg-6 text-center text-lg-start">
              <h1 class="text-white fs-5 fs-xl-6">Results</h1>
                <div class="card text-white bg-primary mb-4" style="max-width: 24rem;">
                  <h5 class="card-header text-white" style="padding: 1rem 1rem;"> <i class="fas fa-chart-line"></i> Organization: {{.GlobalReport.Organization}}
                    {{if eq .GlobalReport.DevOpsPlatform "bitbucket_dc"}}
                        <i class="fab fa-bitbucket"></i>
                    {{else if eq .GlobalReport.DevOpsPlatform "bitbucket"}}
                        <i class="fab fa-bitbucket"></i>
                    {{else if eq .GlobalReport.DevOpsPlatform "github"}}
                        <i class="fab fa-github"></i>
                    {{else if eq .GlobalReport.DevOpsPlatform "gitlab"}}
                        <i class="fab fa-gitlab"></i>
                    {{else if eq .GlobalReport.DevOpsPlatform "azure"}}
                        <i class="fab fa-microsoft"></i>
                    {{else}}
                        <i class="fas fa-folder"></i>
                    {{end}}
                  </h5>
                  <div class="card-body" style="padding: 1rem 1rem;">
                    <p class="card-text"><i class="fas fa-code-branch"></i> Total lines of code : {{.GlobalReport.TotalLinesOfCode}}</p>
                    <p class="card-text"><i class="fas fa-folder"></i> Largest Repository : {{.GlobalReport.LargestRepository}}</p>
                    <p class="card-text"><i class="fas fa-code-branch"></i> Lines of code in largest Repository : {{.GlobalReport.LinesOfCodeLargestRepo}}</p>
                    <p class="card-text"><i class="fas fa-code-branch"></i> Number of Repositories analyzed : {{.GlobalReport.NumberRepos}}</p>
                    <p class="card-text small"><i class="fas fa-info-circle"></i> {{.NoteLOCExcluded}}</p>
                  </div>
                </div>
                <div class="chart-container">
                  <canvas id="camembertChart" width="400" height="400"></canvas>
                </div>
            </div>
            <div class="col-lg-6 mt-3 mt-lg-0">
                              <div class="card text-white bg-primary mb-4" style="max-width: 21rem;">
                <h5 class="card-header text-white" style="padding: 0.75rem 1rem;">
                  <i class="fas fa-code"></i> Languages
                  <small class="text-white-50" style="font-size:0.7rem;display:block;font-weight:400;margin-top:2px;">sorted by lines of code &darr;</small>
                </h5>
                <div class="card-body text-white" style="padding: 0.75rem 1rem; max-height:440px; overflow-y:auto;">
                    {{range .Languages}}
                    <div class="lang-bar-row">
                      <div class="lang-bar-header">
                        <span class="lang-bar-name" title="{{.Language}}">{{.Language}}{{if eq .Language "JSON"}}&nbsp;<span style="font-size:0.68rem;opacity:0.65;font-weight:400;">(excl.)</span>{{end}}</span>
                        <span class="lang-bar-meta">{{if ne .Language "JSON"}}{{printf "%.1f" .Percentage}}% · {{end}}{{.CodeLinesF}} LOC</span>
                      </div>
                      <div class="lang-bar-track">
                        <div class="lang-bar-fill" style="width:{{printf "%.1f" .RelativePct}}%;"></div>
                      </div>
                    </div>
                    {{end}}
                </div>
              </div>
              <div class="text-center mt-3">
                <a href="#repository-section" class="btn btn-outline-light btn-lg">
                  <i class="fas fa-table"></i> View Repository Details
                </a>
              </div>
            </div>
          </div>
        </div>
      </section>
      
      <!-- Repository Details Table Section -->
      <section id="repository-section" style="background-color: #f8f9fa; padding: 3rem 0; margin-top: 2rem;">
        <div class="container">
          <div class="row">
            <div class="col-12">
              <h2 class="text-center mb-4" style="color: #333;">
                <i class="fas fa-table"></i> Repository Analysis Details
              </h2>
              <div class="card shadow-lg repository-table-container">
                <h5 class="card-header bg-primary text-white">
                  <i class="fas fa-code-branch"></i> Lines of Code by Repository ({{len .Repositories}} repositories analyzed)
                </h5>
                <div class="card-body">
                  <div class="table-responsive">
                    <table class="table table-striped table-hover">
                      <thead class="table-dark">
                        <tr>
                          <th scope="col">#</th>
                          <th scope="col" class="sortable" data-column="repository">
                            Repository <i class="fas fa-sort sort-icon"></i>
                          </th>
                          <th scope="col" class="sortable" data-column="branch">
                            Branch <i class="fas fa-sort sort-icon"></i>
                          </th>
                          <th scope="col" class="sortable" data-column="lines">
                            Lines <i class="fas fa-sort sort-icon"></i>
                          </th>
                          <th scope="col" class="sortable" data-column="blanklines">
                            Blank Lines <i class="fas fa-sort sort-icon"></i>
                          </th>
                          <th scope="col" class="sortable" data-column="comments">
                            Comments <i class="fas fa-sort sort-icon"></i>
                          </th>
                          <th scope="col" class="sortable" data-column="codelines">
                            Code Lines <i class="fas fa-sort-down sort-icon"></i>
                          </th>
                        </tr>
                      </thead>
                      <tbody id="repositoryTableBody">
                        {{$platform := .Platform}}
                        {{range .Repositories}}
                        <tr data-repository="{{if eq $platform "gitlab"}}{{.Org}}/{{end}}{{.Repository}}" data-branch="{{.Branch}}" data-lines="{{.Lines}}" data-blanklines="{{.BlankLines}}" data-comments="{{.Comments}}" data-codelines="{{.CodeLines}}">
                          <td>{{.Number}}</td>
                          <td><a href="/repository/{{.Repository}}/{{.Branch}}" class="repo-link">{{if and (eq $platform "gitlab") .Org}}<span class="text-muted" style="font-size:0.85em;">{{.Org}}&thinsp;/&thinsp;</span>{{end}}{{.Repository}}</a></td>
                          <td>{{.Branch}}</td>
                          <td>{{.LinesF}}</td>
                          <td>{{.BlankLinesF}}</td>
                          <td>{{.CommentsF}}</td>
                          <td><strong>{{.CodeLinesF}}</strong></td>
                        </tr>
                        {{end}}
                      </tbody>
                      <tfoot class="table-secondary">
                        <tr id="totalsRow">
                          <td><strong>Total</strong></td>
                          <td colspan="2"><strong>{{len .Repositories}} repositories</strong></td>
                          <td id="totalLines"><strong>-</strong></td>
                          <td id="totalBlankLines"><strong>-</strong></td>
                          <td id="totalComments"><strong>-</strong></td>
                          <td id="totalCodeLines"><strong>-</strong></td>
                        </tr>
                      </tfoot>
                    </table>
                  </div>
                </div>
              </div>
            </div>
          </div>
        </div>
      </section>
    </main>

     <!-- Modal -->

      <!-- Modal -->
    <div id="apiModal" class="modal modal-lg" >
     <div class="modal-dialog modal-dialog-centered modal-lg">
      <div class="modal-content">
        <span class="close"><i class="fa fa-times-circle"></i></span>
           <div class="css-xvw69q e1wpxmm14">
                 <header class="sw-flex sw-items-baseline">
                    <h3><i class="fa fa-info-circle"></i> API Information</h3>
                 </header>
                 <div class="sw-mt-4 markdown"><i class="fa fa-link"></i> <strong>GET</strong> /api/languages</div>
                 <div class="sw-mt-4 markdown">return a list of language with number of line of code</div>
                 <div class="accordion" id="accordion1">
                    <div class="accordion-item">
                      <h2 class="accordion-header" id="headingOne">
                        <button class="accordion-button" type="button" data-bs-toggle="collapse" data-bs-target="#collapseOne" aria-expanded="false" aria-controls="collapseOne">
                          <strong>Response Example<strong>
                        </button>
                      </h2>
                    <div id="collapseOne" class="accordion-collapse collapse" aria-labelledby="headingOne" data-bs-parent="#accordion1">
                        <div class="accordion-body">
                        <pre><code>
                          {  
                            "Language":"C#",
                            "CodeLines":17826,
                            "Percentage":0,
                            "CodeLinesF":""
                          }
                        </code></pre>
                        </div>
                    </div>
                  </div>
                   <div class="sw-mt-4 markdown"><i class="fa fa-link"></i> <strong>GET</strong> /api/global-info</div>
                   <div class="sw-mt-4 markdown">Returns the global information for the analysis.</div>
                   
                   <div class="sw-mt-4 markdown"><i class="fa fa-link"></i> <strong>GET</strong> /api/repositories</div>
                   <div class="sw-mt-4 markdown">Returns detailed repository metrics including lines of code per repository.</div>

                    <div class="accordion" id="accordion2">
                    <div class="accordion-item">
                      <h2 class="accordion-header" id="headingOne2">
                        <button class="accordion-button" type="button" data-bs-toggle="collapse" data-bs-target="#collapseTwo" aria-expanded="false" aria-controls="collapseTwo">
                          <strong>Response Example<strong>
                        </button>
                      </h2>
                    <div id="collapseTwo" class="accordion-collapse collapse" aria-labelledby="headingOne" data-bs-parent="#accordion2">
                        <div class="accordion-body">
                        <pre><code>
                          {  
                            "Organization":	"SonarSource-Demos"
                            "TotalLinesOfCode":	"7.13M"
                            "LargestRepository":	"opencv"
                            "LinesOfCodeLargestRepo":	"2.34M"
                            "DevOpsPlatform":	"github"
                            "NumberRepos":	4
                          }
                        </code></pre>
                        </div>
                    </div>
                  </div>

                   
              </div>
            </div>
      </div>
      </div>
    </div>


   
    <script src="/dist/vendors/chartjs/chart.js"></script>
    <script>
        var ctx = document.getElementById('camembertChart').getContext('2d');
        var camembertChart = new Chart(ctx, {
            type: 'doughnut',
            data: {
                labels: [{{range .Languages}}"{{.Language}}",{{end}}],
                datasets: [{
                    label: 'LOC ',
                    data: [{{range .Languages}}{{.CodeLines}},{{end}}],
                    backgroundColor: [
                        'rgba(255, 99, 132, 0.5)',
                        'rgba(54, 162, 235, 0.5)',
                        'rgba(255, 206, 86, 0.5)',
                        'rgba(75, 192, 192, 0.5)',
                        'rgba(153, 102, 255, 0.5)',
                        'rgba(255, 159, 64, 0.5)'
                    ],
                    borderColor: [
                        'rgba(255, 99, 132, 1)',
                        'rgba(54, 162, 235, 1)',
                        'rgba(255, 206, 86, 1)',
                        'rgba(75, 192, 192, 1)',
                        'rgba(153, 102, 255, 1)',
                        'rgba(255, 159, 64, 1)'
                    ],
                    borderWidth: 1
                }]
            },
            options: {
                responsive: false,
                legend: {
                    display: false
                },
                plugins: {
                    legend: {
                        labels: {
                            color: 'white' 
                        }
                    }, 
                    tooltip: {
                        callbacks: {
                            label: function(context) {
                                return context.label + ': ' + context.raw.toLocaleString() + ' LOC';
                            }
                        }
                    }
                }
            }
        });
        var modal = document.getElementById("apiModal");
        var btn = document.getElementById("apiButton");
        var span = document.getElementsByClassName("close")[0];

        btn.onclick = function() {
            modal.style.display = "block";
        }

        span.onclick = function() {
            modal.style.display = "none";
        }

        window.onclick = function(event) {
            if (event.target == modal) {
                modal.style.display = "none";
            }
        }

        // Calculate and display totals for repository table
        function calculateRepositoryTotals() {
            let totalLines = 0;
            let totalBlankLines = 0;
            let totalComments = 0;
            let totalCodeLines = 0;

            {{range .Repositories}}
            totalLines += {{.Lines}};
            totalBlankLines += {{.BlankLines}};
            totalComments += {{.Comments}};
            totalCodeLines += {{.CodeLines}};
            {{end}}

            // Format numbers with commas
            function formatNumber(num) {
                return num.toLocaleString();
            }

            // Update totals in the table
            document.getElementById('totalLines').innerHTML = '<strong>' + formatNumber(totalLines) + '</strong>';
            document.getElementById('totalBlankLines').innerHTML = '<strong>' + formatNumber(totalBlankLines) + '</strong>';
            document.getElementById('totalComments').innerHTML = '<strong>' + formatNumber(totalComments) + '</strong>';
            document.getElementById('totalCodeLines').innerHTML = '<strong>' + formatNumber(totalCodeLines) + '</strong>';
        }

        // Calculate totals when page loads
        calculateRepositoryTotals();
        
        // Repository table sorting functionality
        let currentSort = { column: 'codelines', direction: 'desc' };
        
        // Function to update sorting icons
        function updateSortingIcons(activeColumn, direction) {
            // Reset all icons
            document.querySelectorAll('.sortable .sort-icon').forEach(icon => {
                icon.className = 'fas fa-sort sort-icon';
                icon.parentElement.classList.remove('sorted');
            });
            
            // Set active column icon
            const activeHeader = document.querySelector('[data-column="' + activeColumn + '"]');
            if (activeHeader) {
                const icon = activeHeader.querySelector('.sort-icon');
                icon.className = 'fas fa-sort-' + (direction === 'asc' ? 'up' : 'down') + ' sort-icon';
                activeHeader.classList.add('sorted');
            }
        }
        
        // Initialize sorting state on page load
        document.addEventListener('DOMContentLoaded', function() {
            updateSortingIcons('codelines', 'desc');
        });
        
        function sortTable(column) {
            const tbody = document.getElementById('repositoryTableBody');
            const rows = Array.from(tbody.querySelectorAll('tr'));
            
            // Determine sort direction
            if (currentSort.column === column) {
                currentSort.direction = currentSort.direction === 'asc' ? 'desc' : 'asc';
            } else {
                currentSort.direction = 'desc'; // Default to descending for new column
                currentSort.column = column;
            }
            
            // Sort rows
            rows.sort((a, b) => {
                let aVal, bVal;
                
                if (column === 'repository' || column === 'branch') {
                    aVal = a.dataset[column].toLowerCase();
                    bVal = b.dataset[column].toLowerCase();
                    return currentSort.direction === 'asc' ? 
                        aVal.localeCompare(bVal) : bVal.localeCompare(aVal);
                } else {
                    aVal = parseInt(a.dataset[column]);
                    bVal = parseInt(b.dataset[column]);
                    return currentSort.direction === 'asc' ? aVal - bVal : bVal - aVal;
                }
            });
            
            // Update row numbers and re-append rows
            rows.forEach((row, index) => {
                row.querySelector('td:first-child').textContent = index + 1;
                tbody.appendChild(row);
            });
            
            // Update sort icons
            updateSortingIcons(column, currentSort.direction);
        }
        
        // Add click handlers to sortable columns
        document.querySelectorAll('.sortable').forEach(header => {
            header.addEventListener('click', () => {
                sortTable(header.dataset.column);
            });
        });

    </script>
  </body>
</html>
`

// Repository Detail HTML template
const repositoryDetailTemplate = `
<!DOCTYPE html>
<html lang="en-US" dir="ltr">
  <head>
    <meta charset="utf-8">
    <meta http-equiv="X-UA-Compatible" content="IE=edge">
    <meta name="viewport" content="width=device-width, initial-scale=1">
    <title>{{.Repository}} - Repository Details</title>
    <link href="https://fonts.googleapis.com/css2?family=Manrope:wght@200;300;400;500;600;700&amp;display=swap" rel="stylesheet">
    <link href="/dist/css/theme.min.css" rel="stylesheet" type="text/css" />
    <link href="/dist/vendors/fontawesome/css/all.min.css" rel="stylesheet" type="text/css" />
    <style>
      .navbar {
        background: rgba(253, 106, 133, 0.15) !important;
        backdrop-filter: blur(10px);
        box-shadow: 0 1px 3px rgba(0,0,0,0.1);
        border-bottom: 1px solid rgba(253, 106, 133, 0.2);
        padding: 0.25rem 0 !important;
        min-height: 3rem !important;
      }
      
      .navbar-brand {
        padding: 0.25rem 0 !important;
      }
      
      .navbar-brand img {
        height: 2rem !important;
        filter: brightness(1.1);
      }
      
      .navbar-nav {
        padding: 0.25rem 0 !important;
      }
      
      .navbar-nav .nav-link {
        font-weight: 500;
        color: rgba(255,255,255,0.9) !important;
        transition: all 0.3s ease;
        padding: 0.25rem 1rem !important;
        font-size: 0.9rem;
      }
      
      .navbar-nav .nav-link:hover {
        color: #fd6a85 !important;
        background-color: rgba(253, 106, 133, 0.1);
        border-radius: 4px;
      }
      
      .back-btn {
        color: #007bff;
        text-decoration: none;
        font-weight: 500;
      }
      
      .back-btn:hover {
        color: #fd6a85;
      }
      
      .stat-card {
        background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
        color: white;
        border-radius: 10px;
        padding: 1.5rem;
        margin-bottom: 1rem;
      }
      
      .lang-table th {
        background-color: #343a40;
        color: white;
      }
      
      .repo-external-link {
        color: #fff;
        text-decoration: none;
        transition: all 0.3s ease;
        display: inline-flex;
        align-items: center;
        gap: 0.5rem;
      }
      
      .repo-external-link:hover {
        color: #00d4aa;
        text-decoration: none;
      }
      
      .repo-external-link i {
        font-size: 1.1em;
      }
    </style>
  </head>
  <body>
    <main class="main" id="top">
      <nav class="navbar navbar-expand-lg fixed-top navbar-dark" data-navbar-on-scroll="data-navbar-on-scroll">
       <div class="container"><a class="navbar-brand" href="/"><img src="/dist/img/Logo.png" alt="" /></a>
          <button class="navbar-toggler" type="button" data-bs-toggle="collapse" data-bs-target="#navbarSupportedContent" aria-controls="navbarSupportedContent" aria-expanded="false" aria-label="Toggle navigation"><i class="fa-solid fa-bars text-white fs-3"></i></button>
          <div class="collapse navbar-collapse" id="navbarSupportedContent">
            <ul class="navbar-nav ms-auto mt-2 mt-lg-0">
              <li class="nav-item"><a class="nav-link" href="/">Dashboard</a></li>
              <li class="nav-item"><a class="nav-link" href="#" id="apiButton">API</a></li>
            </ul>
          </div>
        </div>
      </nav>
      
      <div class="bg-dark" style="padding-top: 5rem; padding-bottom: 2rem;">
        <div class="container">
          <div class="row">
            <div class="col-12">
              <div class="mb-3">
                <a href="/" class="back-btn">
                  <i class="fas fa-arrow-left"></i> Back to Dashboard
                </a>
              </div>
              <h1 class="text-white fs-3 mb-4">
                <i class="fab fa-git-alt"></i> {{.Repository}}
              </h1>
              
              <div class="row">
                <div class="col-md-4">
                  <div class="stat-card">
                    <h5><i class="fas fa-info-circle"></i> Repository Info</h5>
                    <p><strong>Organization:</strong> {{.Organization}}</p>
                    <p><strong>Main Branch:</strong> {{.MainBranch}}</p>
                    <p><strong>Repository:</strong> 
                      <a href="{{.RepositoryURL}}" target="_blank" class="repo-external-link">
                        <i class="{{.PlatformIcon}}"></i>
                        {{.Repository}}
                        <i class="fas fa-external-link-alt" style="font-size: 0.8em; margin-left: 0.3rem;"></i>
                      </a>
                    </p>
                  </div>
                </div>
                
                <div class="col-md-4">
                  <div class="stat-card">
                    <h5><i class="fas fa-chart-line"></i> Summary Stats</h5>
                    <p><strong>Total Lines:</strong> {{.TotalLinesF}}</p>
                    <p><strong>Code Lines:</strong> {{.TotalCodeLinesF}}</p>
                    <p><strong>Languages:</strong> {{len .Languages}}</p>
                    <p class="small" style="margin-top: 0.5rem; opacity: 0.95;"><i class="fas fa-info-circle"></i> {{.NoteLOCExcluded}}</p>
                  </div>
                </div>
                
                <div class="col-md-4">
                  <div class="stat-card">
                    <h5><i class="fas fa-code"></i> Code Details</h5>
                    <p><strong>Blank Lines:</strong> {{.TotalBlankLinesF}}</p>
                    <p><strong>Comments:</strong> {{.TotalCommentsF}}</p>
                    <p><strong>Files:</strong> Multiple</p>
                  </div>
                </div>
              </div>
            </div>
          </div>
        </div>
      </div>
      
      <!-- Language Breakdown Section -->
      <section style="background-color: #f8f9fa; padding: 3rem 0;">
        <div class="container">
          <div class="row">
            <div class="col-12">
              <h2 class="text-center mb-4">
                <i class="fas fa-code"></i> Language Breakdown for {{.Repository}}
              </h2>
              <div class="card shadow">
                <div class="card-body">
                  <div class="table-responsive">
                    <table class="table table-striped lang-table">
                      <thead>
                        <tr>
                          <th>Language</th>
                          <th>Files</th>
                          <th>Total Lines</th>
                          <th>Blank Lines</th>
                          <th>Comments</th>
                          <th>Code Lines</th>
                        </tr>
                      </thead>
                      <tbody>
                        {{range .Languages}}
                        <tr>
                          <td><strong>{{.Language}}</strong></td>
                          <td>{{.FilesF}}</td>
                          <td>{{.LinesF}}</td>
                          <td>{{.BlankLinesF}}</td>
                          <td>{{.CommentsF}}</td>
                          <td><strong>{{.CodeLinesF}}</strong></td>
                        </tr>
                        {{end}}
                      </tbody>
                    </table>
                  </div>
                </div>
              </div>
            </div>
          </div>
        </div>
      </section>
      
      {{if .OtherBranches}}
      <!-- Other Branches Section -->
      <section style="padding: 3rem 0;">
        <div class="container">
          <div class="row">
            <div class="col-12">
              <h2 class="text-center mb-4">
                <i class="fas fa-code-branch"></i> Other Branches
              </h2>
              <div class="card shadow">
                <div class="card-body">
                  <div class="table-responsive">
                    <table class="table table-striped">
                      <thead class="table-dark">
                        <tr>
                          <th>Branch</th>
                          <th>Total Lines</th>
                          <th>Blank Lines</th>
                          <th>Comments</th>
                          <th>Code Lines</th>
                        </tr>
                      </thead>
                      <tbody>
                        {{range .OtherBranches}}
                        <tr>
                          <td><strong>{{.Branch}}</strong></td>
                          <td>{{.LinesF}}</td>
                          <td>{{.BlankLinesF}}</td>
                          <td>{{.CommentsF}}</td>
                          <td><strong>{{.CodeLinesF}}</strong></td>
                        </tr>
                        {{end}}
                      </tbody>
                    </table>
                  </div>
                </div>
              </div>
            </div>
          </div>
        </div>
      </section>
      {{end}}

      {{if .Files}}
      <!-- File Tree Section -->
      <section style="background-color:#f8f9fa;padding:3rem 0;">
        <div class="container">
          <div class="row">
            <div class="col-12">
              <h2 class="text-center mb-4">
                <i class="fas fa-sitemap"></i> File Tree <small class="text-muted fs-6">({{len .Files}} files)</small>
              </h2>
              <div class="card shadow">
                <div class="card-body p-0">
                  <div class="px-3 pt-3 pb-2 d-flex gap-2 align-items-center border-bottom">
                    <input type="text" id="treeSearch" class="form-control form-control-sm" placeholder="Search files and folders..." style="max-width:360px;">
                    <button class="btn btn-sm btn-outline-secondary" onclick="expandAll()">Expand all</button>
                    <button class="btn btn-sm btn-outline-secondary" onclick="collapseAll()">Collapse all</button>
                    <span id="treeMatchCount" class="text-muted small ms-auto"></span>
                  </div>
                  <div style="overflow-y:auto;max-height:650px;">
                    <table class="table table-sm mb-0" id="treeTable" style="border-collapse:collapse;">
                      <thead style="position:sticky;top:0;background:#343a40;color:#fff;z-index:2;">
                        <tr>
                          <th style="padding:.5rem 1rem;font-weight:600;">Name</th>
                          <th style="padding:.5rem 1rem;text-align:right;white-space:nowrap;font-weight:600;">Code Lines</th>
                        </tr>
                      </thead>
                      <tbody id="treeBody"></tbody>
                    </table>
                  </div>
                </div>
              </div>
            </div>
          </div>
        </div>
      </section>
      <script>
      (function(){
        var RAW = [
          {{range .Files}}{"p":{{.File | printf "%q"}},"c":{{.CodeLines}}},
          {{end}}
        ];

        // ── Build tree ──────────────────────────────────────────────
        function buildTree(files) {
          var root = {name:'', children:{}, fileList:[], loc:0, isDir:true};
          files.forEach(function(f){
            var parts = f.p.replace(/\\/g,'/').split('/');
            var node = root;
            for (var i=0;i<parts.length-1;i++){
              var seg=parts[i];
              if(!node.children[seg]) node.children[seg]={name:seg,children:{},fileList:[],loc:0,isDir:true};
              node=node.children[seg];
            }
            node.fileList.push({name:parts[parts.length-1],loc:f.c});
          });
          function sumLOC(n){
            var t=0;
            n.fileList.forEach(function(f){t+=f.loc;});
            Object.values(n.children).forEach(function(c){t+=sumLOC(c);c.loc=c._sum;});
            n._sum=t; return t;
          }
          sumLOC(root); root.loc=root._sum;
          function fix(n){n.loc=n._sum; Object.values(n.children).forEach(fix);}
          fix(root);
          return root;
        }

        // ── Flatten tree to rows ─────────────────────────────────────
        var allRows=[];
        function flatten(node, depth, parentId){
          var sorted = Object.keys(node.children).sort();
          sorted.forEach(function(k){
            var child=node.children[k];
            var id='n'+(allRows.length);
            allRows.push({id:id,parentId:parentId,depth:depth,name:child.name,loc:child.loc,isDir:true,open:false,visible:true,matched:true});
            flatten(child,depth+1,id);
          });
          var sortedFiles=node.fileList.slice().sort(function(a,b){return b.loc-a.loc;});
          sortedFiles.forEach(function(f){
            allRows.push({id:'n'+(allRows.length),parentId:parentId,depth:depth,name:f.name,loc:f.loc,isDir:false,visible:true,matched:true});
          });
        }

        var tree=buildTree(RAW);
        flatten(tree,0,null);

        // ── Render ───────────────────────────────────────────────────
        var tbody=document.getElementById('treeBody');

        function fmtLOC(n){
          if(n===0) return '<span style="color:#aaa;">—</span>';
          return n.toLocaleString();
        }

        function render(){
          var html='';
          allRows.forEach(function(r,i){
            if(!r.visible) return;
            var indent=r.depth*20;
            var icon, toggle='';
            if(r.isDir){
              icon=r.open?'<i class="fas fa-folder-open" style="color:#f5a623;margin-right:6px;"></i>'
                         :'<i class="fas fa-folder" style="color:#f5a623;margin-right:6px;"></i>';
              toggle='<i class="fas fa-chevron-'+(r.open?'down':'right')+'" style="color:#aaa;font-size:.7rem;margin-right:6px;"></i>';
            } else {
              var ext=r.name.split('.').pop().toLowerCase();
              icon='<i class="fas fa-file-code" style="color:#6c757d;margin-right:6px;"></i>';
            }
            var bg = r.matched && document.getElementById('treeSearch').value ? 'background:#fffde7;' : '';
            var cursor = r.isDir ? 'cursor:pointer;' : '';
            var nameHtml = r.isDir
              ? '<span style="font-weight:600;">'+escHtml(r.name)+'</span>'
              : '<span style="font-family:monospace;font-size:.85rem;">'+escHtml(r.name)+'</span>';
            html += '<tr data-i="'+i+'" style="border-bottom:1px solid #e9ecef;'+cursor+bg+'" '
              +(r.isDir?'onclick="toggleDir('+i+')"':'')+'>'
              +'<td style="padding:.4rem 1rem .4rem '+(16+indent)+'px;">'
              +toggle+icon+nameHtml+'</td>'
              +'<td style="padding:.4rem 1rem;text-align:right;white-space:nowrap;font-size:.9rem;">'+fmtLOC(r.loc)+'</td>'
              +'</tr>';
          });
          tbody.innerHTML=html;
          updateMatchCount();
        }

        function escHtml(s){return s.replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;');}

        function getDescendantIds(i){
          var depth=allRows[i].depth, ids=[];
          for(var j=i+1;j<allRows.length;j++){
            if(allRows[j].depth<=depth) break;
            ids.push(j);
          }
          return ids;
        }

        window.toggleDir=function(i){
          var r=allRows[i];
          r.open=!r.open;
          var desc=getDescendantIds(i);
          if(r.open){
            // show only direct children
            desc.forEach(function(j){
              if(allRows[j].depth===r.depth+1) allRows[j].visible=true;
            });
          } else {
            // hide all descendants
            desc.forEach(function(j){ allRows[j].visible=false; allRows[j].open=false; });
          }
          render();
        };

        window.expandAll=function(){
          allRows.forEach(function(r){r.visible=true; if(r.isDir) r.open=true;});
          render();
        };
        window.collapseAll=function(){
          allRows.forEach(function(r){
            if(r.depth===0) r.visible=true; else r.visible=false;
            r.open=false;
          });
          render();
        };

        // ── Search ───────────────────────────────────────────────────
        function updateMatchCount(){
          var q=document.getElementById('treeSearch').value;
          if(!q){document.getElementById('treeMatchCount').textContent='';return;}
          var n=allRows.filter(function(r){return r.visible&&r.matched;}).length;
          document.getElementById('treeMatchCount').textContent=n+' match'+(n===1?'':'es');
        }

        document.getElementById('treeSearch').addEventListener('input',function(){
          var q=this.value.toLowerCase();
          if(!q){
            allRows.forEach(function(r){r.matched=true;});
            collapseAll(); return;
          }
          // Mark matched rows and expose their ancestors
          var matchedIdxs=new Set();
          allRows.forEach(function(r,i){ r.matched=r.name.toLowerCase().includes(q); if(r.matched) matchedIdxs.add(i); });
          // For each match, make all ancestors visible+open
          matchedIdxs.forEach(function(i){
            for(var j=i-1;j>=0;j--){
              if(allRows[j].depth<allRows[i].depth&&allRows[j].isDir){
                allRows[j].open=true; allRows[j].visible=true;
                if(allRows[j].depth===0) break;
              }
            }
            allRows[i].visible=true;
          });
          // Hide rows that are neither matched nor ancestors
          allRows.forEach(function(r,i){
            if(!matchedIdxs.has(i) && !(r.open&&r.isDir)) {
              // show if parent is open and this row is matched or on the path to a match
              var parentOpen=r.depth===0||true;
              if(!r.matched) r.visible=r.open;
            }
          });
          render();
        });

        // Initial render (root level only)
        collapseAll();
      })();
      </script>
      {{end}}

    </main>

    <script src="/dist/vendors/bootstrap/js/bootstrap.bundle.min.js"></script>
  </body>
</html>
`
