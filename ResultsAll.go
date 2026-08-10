//go:build resultsall
// +build resultsall

package main

import (
	"archive/zip"
	"bufio"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
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
	"sync"

	"github.com/SonarSource-Demos/sonar-golc/assets"
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
	resultsBaseDir        = "Results"
	byFileReportDir       = "Results/byfile-report"
	byLanguageReportDir   = "Results/bylanguage-report"
	configResultsDir      = "Results/config"
	globalReportFile      = "Results/GlobalReport.json"
	codeLinesLanguageFile = "Results/code_lines_by_language.json"
)

// sanitizePathComponent sanitizes a path component to prevent path traversal attacks.
//
// Delegates to the shared implementation: the deselection key is built from the same
// normalization, and a second copy of these rules here is exactly how the page and the
// generated reports came to key the same repository differently.
func sanitizePathComponent(component string) string {
	return utils.SanitizeResultComponent(component)
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

// The repository row, its inventory and the platform naming rules all live in pkg/utils
// now. Aliases rather than copies: two structurally identical definitions were what let
// this page and the generated reports drift apart in the first place.
type (
	RepositoryData = utils.RepositoryData
	ProjectBranch  = utils.ProjectBranch
	AnalysisResult = utils.AnalysisResult
)

// AnalysisResult_ProjectBranch is the older spelling used through this file.
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
	Languages       []LanguageData
	RawLanguages    []LanguageData      // unsummarized per-language rows, as served by /api/languages
	GlobalReport    Globalinfo          // adjusted for deselected repos; see utils.AdjustGlobalInfo
	Repositories    []RepositoryData    // repositories counted in the totals above
	SkippedRepos    []utils.SkippedRepo // repos the analysis phase could not complete (clone timeout/failure, analysis error)
	ScanSummary     *ScanSummaryView    // per-run repository breakdown; nil on older result sets
	NoteLOCExcluded string              // Note that JSON is excluded from total (SonarQube behavior)
	Platform        string

	// Deselection: repositories analyzed but removed from every total by the user.
	// Deselected is empty and RawTotalLinesOfCode equals GlobalReport.TotalLinesOfCode
	// on an untouched selection, so the page renders exactly as before.
	// TableRows is every repository in ranked order, with Deselected set on the excluded
	// ones. The table renders from this single list so a deselected repository keeps its
	// position instead of jumping to the bottom — its size relative to the others is
	// usually why it was being looked at, and losing that ordering makes the effect of the
	// change hard to judge and the row hard to find again.
	TableRows           []RepositoryData
	Deselected          []RepositoryData
	DeselectedKeys      []string // same set as Deselected, for the page's JavaScript
	DeselectedCount     int
	DeselectedCodeLines string // formatted LOC removed from the totals
	RawTotalLinesOfCode string // total across every analyzed repository, for comparison
	ScannedRepositories int    // counted + deselected, i.e. every repository with results
	TopLanguagesShown   int    // how many languages the Top Languages column lists
}

// ScanSummaryView is the template-facing view of utils.ScanSummary. Analyzed is
// adjusted for repos that failed during the analysis phase, and Skipped is that
// failure count, so the row reconciles: Scanned = Analyzed + Archived + Empty + Excluded + Skipped.
//
// Deselected is reported separately and is NOT subtracted from Analyzed: those
// repositories were analyzed, and Excluded already means "filtered out before
// analysis". Folding them together would break the reconciliation above.
type ScanSummaryView struct {
	Scanned    int
	Analyzed   int
	Archived   int
	Empty      int
	Excluded   int
	Skipped    int
	Deselected int
}

var globalInfo Globalinfo       // Variable pour stocker les infos globales
var languageData []LanguageData // Variable pour stocker les données des langages

// dataMu guards globalInfo, languageData and currentPage. The deselection endpoint
// rebuilds all three while other requests are being served, so unlike the original
// load-once-at-startup design they can no longer be read unsynchronized.
var dataMu sync.RWMutex

// currentPage is the view every handler renders from. Held here rather than captured
// by the handler closures so a rebuild is visible to requests already registered.
var currentPage PageData

func getGlobalInfo() Globalinfo {
	dataMu.RLock()
	defer dataMu.RUnlock()
	return globalInfo
}

func getLanguageData() []LanguageData {
	dataMu.RLock()
	defer dataMu.RUnlock()
	return languageData
}

// snapshot returns the current view. Callers get a copy of the struct, so the slices
// inside must be treated as read-only — a rebuild replaces them rather than mutating
// them in place, which keeps concurrent readers consistent without deep copying.
func snapshot() PageData {
	dataMu.RLock()
	defer dataMu.RUnlock()
	return currentPage
}

// publish installs a freshly loaded view.
func publish(pd PageData) {
	dataMu.Lock()
	defer dataMu.Unlock()
	currentPage = pd
	globalInfo = pd.GlobalReport
	languageData = pd.RawLanguages
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

// getRepositoryData collects every analyzed repository. The layout logic — inventory
// discovery, per-platform naming and the deselection key — lives in pkg/utils, so this
// page and the generated reports read through exactly the same rules. They used to carry
// separate copies, and the drift between them produced repositories that were deselected
// here yet still counted in the PDF.
func getRepositoryData() ([]RepositoryData, error) {
	return utils.ReadRepositoryData(resultsBaseDir)
}

func detectPlatformAndReadAnalysis() (string, []byte, error) {
	spec, data, err := utils.DetectPlatform(resultsBaseDir)
	return spec.Name, data, err
}

// getFirstPartForPlatform returns the leading component of a result file name — the
// organization for GitHub and GitLab, the project key for Azure and Bitbucket.
func getFirstPartForPlatform(platform string, branch AnalysisResult_ProjectBranch, repoName string) string {
	spec, ok := utils.PlatformSpecFor(platform)
	if !ok {
		return branch.Org
	}
	return spec.FirstPart(branch)
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

	// Look for all byfile reports for this repository (different branches).
	// Producer writes Result_<Org>__<Repo>__<Branch>_byfile.json (see golc.go and
	// the byfile path constructed above) — `__` between fields, `_byfile` tail.
	pattern := buildSecurePath(byFileReportDir,
		fmt.Sprintf("Result_%s__%s__*_byfile.json",
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
		// Format: Result_<Org>__<Repo>__<Branch>_byfile.json
		// Extract the branch by trimming the known prefix and suffix; with the
		// `__` field separator the result is unambiguous regardless of any `_`
		// inside Org / Repo / Branch.
		prefix := fmt.Sprintf("Result_%s__%s__", sanitizePathComponent(firstPart), sanitizePathComponent(repoName))
		suffix := "_byfile.json"
		if !strings.HasPrefix(filename, prefix) || !strings.HasSuffix(filename, suffix) {
			continue
		}
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
			fmt.Sprintf("Result_%s__%s__%s_byfile.json",
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
			fmt.Sprintf("Result_%s__%s__%s.json",
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

	// Bring every offered report up to date first. Reports are generated on demand, so
	// without this the archive could contain artifacts built from an earlier selection —
	// and unlike a single download, nothing about a ZIP tells the recipient that.
	// Results/customized is inside the tree, so both variants are picked up.
	//
	// A generation failure is logged rather than fatal: the archive still carries the
	// per-repository results, which are what a user most needs from it.
	regenerateMu.Lock()
	if err := syncReportVariantsLocked(); err != nil {
		fmt.Println("⚠️  report generation failed while building the archive:", err)
	}
	regenerateMu.Unlock()

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

	// The persisted selection, applied to everything below.
	deselectedSet := utils.LoadDeselectionSet(resultsBaseDir)

	// Language totals are computed here from the per-repository result files rather than
	// read from the generated code_lines_by_language.json. Reports are written on demand,
	// so that file is not necessarily current — reading it could show a language chart
	// describing a different repository set than this page's own table.
	//
	// Locals, not the package vars: publish is the only writer of those, so a failed load
	// cannot leave the served view half-updated.
	totals, _, err := utils.CollectResultTotals(resultsBaseDir, deselectedSet)
	if err != nil {
		return pageData, fmt.Errorf("error reading per-repository result files: %v", err)
	}
	rawLanguages := make([]LanguageData, 0, len(totals))
	for language, codeLines := range totals {
		rawLanguages = append(rawLanguages, LanguageData{Language: language, CodeLines: codeLines})
	}
	// Sorted so the page is stable across reloads (Go map order is randomized).
	sort.Slice(rawLanguages, func(i, j int) bool {
		if rawLanguages[i].CodeLines != rawLanguages[j].CodeLines {
			return rawLanguages[i].CodeLines > rawLanguages[j].CodeLines
		}
		return rawLanguages[i].Language < rawLanguages[j].Language
	})

	// Fall back to the aggregate file when the per-repository files yield nothing — an
	// older or partial result set may have only the aggregate. The fallback is skipped
	// when a selection is active, because that file describes a repository set the
	// selection has not been applied to and would contradict the table.
	if len(rawLanguages) == 0 && len(deselectedSet) == 0 {
		if aggregate, readErr := os.ReadFile(codeLinesLanguageFile); readErr == nil {
			if json.Unmarshal(aggregate, &rawLanguages) != nil {
				rawLanguages = nil
			}
		}
	}

	languages := buildLanguageSummary(rawLanguages)

	data0, err := os.ReadFile(globalReportFile)
	if err != nil {
		return pageData, fmt.Errorf("error reading GlobalReport.json file: %v", err)
	}

	var info Globalinfo
	err = json.Unmarshal(data0, &info)
	if err != nil {
		return pageData, fmt.Errorf("error decoding JSON GlobalReport.json file: %v", err)
	}
	rawTotalLOC := info.TotalLinesOfCode

	repositoryData, err := getRepositoryData()
	if err != nil {
		fmt.Println("❌ Error loading repository data:", err)
		repositoryData = []RepositoryData{}
	}

	// The table view keeps every repository in its ranked position, flagging the excluded
	// ones. Built before partitioning, which renumbers each group independently.
	tableRows := buildTableRows(repositoryData, deselectedSet)

	// Split off the repositories the user removed from the totals. With no
	// deselection this returns everything in Repositories and nothing in deselected.
	repositoryData, deselected := partitionDeselected(repositoryData, deselectedSet)

	// GlobalReport.json holds the figures as scanned and is never rewritten, so the
	// headline numbers are re-derived here for the deselected set.
	deselectedCodeLines := 0
	deselectedKeys := make([]string, 0, len(deselected))
	for _, repo := range deselected {
		deselectedCodeLines += repo.CodeLines
		deselectedKeys = append(deselectedKeys, repo.Key)
	}
	info = adjustGlobalInfo(info, languages, repositoryData, len(deselected))

	detectedPlatform, _, _ := detectPlatformAndReadAnalysis()

	// Repositories that could not be analyzed (clone timeout/failure or counting
	// error). Missing file (older result sets) => empty list, so the page still renders.
	skippedRepos := utils.LoadSkippedRepos("Results")

	// Per-run repository breakdown for the Scan Summary card. Missing file (older
	// result sets) => nil, so the card is omitted and the page still renders.
	scanSummary := buildScanSummaryView(utils.LoadScanSummary("Results"), len(skippedRepos), len(deselected))

	pageData = PageData{
		Languages:       languages,
		RawLanguages:    rawLanguages,
		GlobalReport:    info,
		Repositories:    repositoryData,
		SkippedRepos:    skippedRepos,
		ScanSummary:     scanSummary,
		NoteLOCExcluded: utils.NoteExcludedFromTotal,
		Platform:        detectedPlatform,

		TableRows:           tableRows,
		Deselected:          deselected,
		DeselectedKeys:      deselectedKeys,
		DeselectedCount:     len(deselected),
		ScannedRepositories: len(repositoryData) + len(deselected),
		TopLanguagesShown:   utils.TopLanguagesShown,
		DeselectedCodeLines: utils.FormatCodeLines(float64(deselectedCodeLines)),
		RawTotalLinesOfCode: rawTotalLOC,
	}

	return pageData, nil
}

// buildTableRows returns every repository in its original ranked order, flagging the
// deselected ones and numbering only those still counted. Keeping a deselected row in
// place preserves the size ordering the user is reading the table for; the numbering
// skips it, which is what the "—" in its row column represents.
func buildTableRows(repositories []RepositoryData, deselected utils.DeselectionSet) []RepositoryData {
	rows := make([]RepositoryData, 0, len(repositories))
	counted := 0
	for _, repo := range repositories {
		if deselected.Contains(repo.Key) {
			repo.Deselected = true
			repo.Number = 0
		} else {
			counted++
			repo.Number = counted
		}
		rows = append(rows, repo)
	}
	return rows
}

// partitionDeselected splits repositories into those still counted and those the
// user removed from the totals, renumbering each group from 1.
func partitionDeselected(repositories []RepositoryData, deselected utils.DeselectionSet) (kept, removed []RepositoryData) {
	for _, repo := range repositories {
		if deselected.Contains(repo.Key) {
			removed = append(removed, repo)
		} else {
			kept = append(kept, repo)
		}
	}
	for i := range kept {
		kept[i].Number = i + 1
	}
	for i := range removed {
		removed[i].Number = i + 1
	}
	return kept, removed
}

// adjustGlobalInfo mirrors utils.AdjustGlobalInfo for this page's own types: it
// re-derives the headline figures from the repositories that survived a deselection,
// and returns ginfo untouched when the selection is untouched so an unfiltered page
// shows exactly the numbers the scan produced.
func adjustGlobalInfo(ginfo Globalinfo, languages []LanguageData, kept []RepositoryData, deselectedCount int) Globalinfo {
	if deselectedCount == 0 {
		return ginfo
	}

	total := 0
	for _, lang := range languages {
		if strings.TrimSpace(lang.Language) != utils.LanguageExcludedFromTotalLOC {
			total += lang.CodeLines
		}
	}
	ginfo.TotalLinesOfCode = utils.FormatCodeLines(float64(total))

	maxLOC, largest := 0, ""
	for _, repo := range kept {
		if repo.CodeLines > maxLOC {
			maxLOC = repo.CodeLines
			largest = repo.Repository
		}
	}
	ginfo.LargestRepository = largest
	ginfo.LinesOfCodeLargestRepo = utils.FormatCodeLines(float64(maxLOC))

	ginfo.NumberRepos -= deselectedCount
	if ginfo.NumberRepos < 0 {
		ginfo.NumberRepos = 0
	}
	return ginfo
}

// buildScanSummaryView adapts a persisted utils.ScanSummary into the template view.
// Analysis-phase failures (skippedCount, from analysis_skipped.json) are subtracted
// from the analyzed count and folded together with the summary's discovery-phase
// Skipped total, so the displayed row reconciles:
// Scanned = Analyzed + Archived + Empty + Excluded + Skipped. Returns nil when no
// summary was persisted.
func buildScanSummaryView(summary *utils.ScanSummary, skippedCount, deselectedCount int) *ScanSummaryView {
	if summary == nil {
		return nil
	}
	analyzed := summary.Analyzed - skippedCount
	if analyzed < 0 {
		analyzed = 0
	}
	return &ScanSummaryView{
		Scanned:    summary.Scanned,
		Analyzed:   analyzed,
		Archived:   summary.Archived,
		Empty:      summary.Empty,
		Excluded:   summary.Excluded,
		Skipped:    summary.Skipped + skippedCount,
		Deselected: deselectedCount,
	}
}

// regenerateMu serializes every read, write and removal of the generated report files.
// Generating overwrites shared files, so two concurrent requests could interleave writes
// and leave the PDF, the CSV and the page describing different repository sets.
//
// It also owns the existence of the customized report directory: creating and deleting it
// must both hold this lock, or a reset can remove the directory while an in-flight
// rebuild is still writing it and the generator simply re-creates it afterwards.
//
// selectionMu separately serializes changes to the persisted selection. They are two
// locks because a download that has to generate a report should not block on someone
// changing the selection, nor the reverse.
//
// Lock order is selectionMu then regenerateMu — applyDeselection takes both. Nothing
// acquires them the other way round; keep it that way.
var (
	regenerateMu sync.Mutex
	selectionMu  sync.Mutex
)

// customizedReportsDir holds the reports that reflect the user's current selection.
// They live in their own directory rather than beside the originals with a suffix, so
// that browsing or zipping Results/ cannot mix the two sets up.
var customizedReportsDir = utils.CustomizedReportsDir(resultsBaseDir)

// reportVariant is one of the two report sets that can be served.
type reportVariant struct {
	// name identifies the variant in the state file.
	name string
	// customized reports whether the persisted selection applies. The full-scan variant
	// ignores it, which is what keeps the original always available.
	customized bool
	// dir is the base directory this variant's artifacts are written under.
	dir string
}

var (
	fullScanVariant   = reportVariant{name: "full-scan", customized: false, dir: resultsBaseDir}
	customizedVariant = reportVariant{name: "customized", customized: true, dir: customizedReportsDir}
)

// globalPDFPath and summary paths for a variant.
func (v reportVariant) globalPDFPath() string { return filepath.Join(v.dir, "GlobalReport.pdf") }
func (v reportVariant) languageTotalsPath() string {
	return filepath.Join(v.dir, "code_lines_by_language.json")
}
func (v reportVariant) summaryPDFPath() string {
	return filepath.Join(v.dir, "byfile-report", "pdf-report", "repository_summary.pdf")
}
func (v reportVariant) summaryCSVPath() string {
	return filepath.Join(v.dir, "byfile-report", "csv-report", "repository_summary.csv")
}

// reportsState records which selection each variant's artifacts were built from, so a
// download can tell whether they are current without regenerating every time.
type reportsState struct {
	// Stamps maps a variant name to the stamp its artifacts were built from.
	Stamps map[string]string `json:"Stamps"`
}

func reportsStatePath() string {
	return filepath.Join(configResultsDir, "reports_state.json")
}

// reportStamp fingerprints everything that would change a variant's content: the
// selection it covers, and the identity of the scan itself. GlobalReport.json is
// rewritten by every analysis run, so its modification time changes when a new scan
// lands and invalidates artifacts built from the previous one.
func reportStamp(v reportVariant, deselected []utils.DeselectedRepo) string {
	keys := make([]string, 0, len(deselected))
	if v.customized {
		for _, repo := range deselected {
			keys = append(keys, repo.Key)
		}
		sort.Strings(keys)
	}

	scanID := "unknown"
	if info, err := os.Stat(globalReportFile); err == nil {
		scanID = fmt.Sprintf("%d-%d", info.ModTime().UnixNano(), info.Size())
	}

	sum := sha256.Sum256([]byte(v.name + "\x00" + scanID + "\x00" + strings.Join(keys, "\x00")))
	return hex.EncodeToString(sum[:])
}

func loadReportsState() reportsState {
	state := reportsState{Stamps: map[string]string{}}
	data, err := os.ReadFile(reportsStatePath())
	if err != nil {
		return state
	}
	if err := json.Unmarshal(data, &state); err != nil || state.Stamps == nil {
		return reportsState{Stamps: map[string]string{}}
	}
	return state
}

func saveReportsState(state reportsState) error {
	if err := os.MkdirAll(configResultsDir, 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(state, "", "    ")
	if err != nil {
		return err
	}
	return os.WriteFile(reportsStatePath(), data, 0644)
}

// ensureReports generates a variant's artifacts if they are missing or were built from a
// different selection, and does nothing when they are already current.
//
// Callers must hold regenerateMu: the check and the generation have to be atomic, or two
// concurrent downloads both see "stale" and write over each other.
func ensureReports(v reportVariant) error {
	deselected := utils.LoadDeselectedRepos(resultsBaseDir)
	if v.customized && len(deselected) == 0 {
		// Nothing is deselected, so the customized variant would duplicate the full
		// scan. Callers should not offer it; treat a request for it as the full scan.
		v = fullScanVariant
	}

	want := reportStamp(v, deselected)
	state := loadReportsState()

	// A stamp match is only trustworthy if the files are actually still there.
	if state.Stamps[v.name] == want && filesExist(
		v.globalPDFPath(), v.summaryPDFPath(), v.summaryCSVPath(),
	) {
		return nil
	}

	if err := generateReports(v, deselected); err != nil {
		return err
	}

	state.Stamps[v.name] = want
	if err := saveReportsState(state); err != nil {
		// The reports are on disk and correct; only the freshness record failed, which
		// costs a needless regeneration next time rather than a wrong report.
		fmt.Println("⚠️  could not record report freshness:", err)
	}
	return nil
}

func filesExist(paths ...string) bool {
	for _, path := range paths {
		if info, err := os.Stat(path); err != nil || info.Size() == 0 {
			return false
		}
	}
	return true
}

// generateReports writes one variant's artifacts. The full-scan variant passes an empty
// selection, which is what makes the original always reproducible: the per-repository
// result files are never modified, so it can be rebuilt at any time.
func generateReports(v reportVariant, deselected []utils.DeselectedRepo) error {
	applied := deselected
	if !v.customized {
		applied = nil
	}

	if err := utils.CreateGlobalReportWith(resultsBaseDir, utils.GlobalReportOptions{
		Deselected:         applied,
		PDFPath:            v.globalPDFPath(),
		LanguageTotalsPath: v.languageTotalsPath(),
	}); err != nil {
		return fmt.Errorf("cannot generate global report: %w", err)
	}

	if err := utils.GenerateRepositorySummaryReportsWith(resultsBaseDir, utils.SummaryReportOptions{
		Deselected: utils.DeselectionKeys(applied),
		OutputDir:  v.dir,
	}); err != nil {
		return fmt.Errorf("cannot generate repository summary reports: %w", err)
	}
	return nil
}

// DeselectionRequest is the payload of POST /api/deselected: the keys of the
// repositories to leave out of every total. An empty list restores the full scan.
type DeselectionRequest struct {
	Keys []string `json:"Keys"`
}

// errAllDeselected rejects a request that would leave nothing counted. It is a distinct
// error so the handler can answer 422 rather than 500: the request is well-formed and the
// server is fine, the instruction just cannot be carried out. A 500 would tell a
// programmatic caller to retry something that will never succeed.
var errAllDeselected = errors.New("cannot deselect every repository — at least one must remain counted")

// DeselectionResponse reports the state after the change so the caller does not have
// to re-fetch it.
type DeselectionResponse struct {
	DeselectedCount     int    `json:"DeselectedCount"`
	CountedRepositories int    `json:"CountedRepositories"`
	TotalLinesOfCode    string `json:"TotalLinesOfCode"`
	RawTotalLinesOfCode string `json:"RawTotalLinesOfCode"`
	Ignored             int    `json:"Ignored"` // submitted keys that match no analyzed repository
}

// handleDeselected persists a new selection, regenerates every report from it, and
// republishes the page data.
//
// GET returns the repositories currently deselected. POST replaces the selection
// wholesale — the client always sends the complete list, so there is no partial
// state to reconcile and a reset is just an empty list.
func handleDeselected(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		w.Header().Set(contentTypeHeader, applicationJSONType)
		deselected := snapshot().Deselected
		if deselected == nil {
			deselected = []RepositoryData{}
		}
		json.NewEncoder(w).Encode(deselected)

	case http.MethodPost:
		var req DeselectionRequest
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}

		selectionMu.Lock()
		defer selectionMu.Unlock()

		resp, err := applyDeselection(req.Keys)
		if errors.Is(err, errAllDeselected) {
			http.Error(w, err.Error(), http.StatusUnprocessableEntity)
			return
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Reports are rebuilt in the background rather than before responding. A download
		// generates on demand anyway (see handleReport), so this exists only so that
		// anyone reading Results/ directly — a script, a CI job, someone opening the
		// folder — finds current files without having clicked a link first.
		//
		// Spawned here rather than inside applyDeselection so that the apply logic stays
		// synchronous and testable.
		go rebuildReportsInBackground()

		w.Header().Set(contentTypeHeader, applicationJSONType)
		json.NewEncoder(w).Encode(resp)

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// applyDeselection validates the requested keys, persists them, regenerates the
// reports and republishes the page data. Callers must hold regenerateMu.
func applyDeselection(keys []string) (*DeselectionResponse, error) {
	// Every repository known to this result set, keyed as the checkboxes key it.
	// Reloading rather than trusting the snapshot means a key that was deselected a
	// moment ago is still recognised.
	all, err := getRepositoryData()
	if err != nil {
		return nil, fmt.Errorf("cannot read repository data: %w", err)
	}
	byKey := make(map[string]RepositoryData, len(all))
	for _, repo := range all {
		byKey[repo.Key] = repo
	}

	// Keys arrive from the browser, so only those matching an analyzed repository are
	// persisted. Dropping unknown keys keeps a stale tab or a hand-edited request from
	// writing entries that silently match nothing.
	records := make([]utils.DeselectedRepo, 0, len(keys))
	seen := make(map[string]bool, len(keys))
	ignored := 0
	for _, key := range keys {
		repo, ok := byKey[key]
		if !ok || seen[key] {
			if !ok {
				ignored++
			}
			continue
		}
		seen[key] = true
		records = append(records, utils.DeselectedRepo{
			Key:    repo.Key,
			Org:    repo.Org,
			Repo:   repo.Repository,
			Branch: repo.Branch,
		})
	}

	// Refuse to deselect everything: the aggregation would produce a zero-LOC report
	// that looks like a failed scan, and there is no way back from the page once the
	// table it is driven from is empty.
	if len(all) > 0 && len(records) == len(all) {
		return nil, errAllDeselected
	}

	if err := utils.SaveDeselectedRepos(resultsBaseDir, records); err != nil {
		return nil, fmt.Errorf("cannot save selection: %w", err)
	}

	// With no selection left, the customized reports describe nothing. They are
	// filtered, understated reports under ordinary file names, and the ZIP archives the
	// whole tree — so leaving them behind means a reset user can still hand over an
	// understated report believing it is current. Delete them rather than rely on the
	// download links no longer being offered.
	//
	// Under regenerateMu, because a rebuild spawned by an earlier request may still be
	// writing that directory: removing it without the lock lets the generator re-create
	// it immediately afterwards, restoring the very files this is deleting.
	if len(records) == 0 {
		regenerateMu.Lock()
		err := utils.ClearCustomizedReports(resultsBaseDir)
		regenerateMu.Unlock()
		if err != nil {
			return nil, fmt.Errorf("cannot remove stale customized reports: %w", err)
		}
	}

	// The page is rebuilt from the result files in memory, so it is correct as soon as
	// the selection is saved — no PDF work is needed to answer this request.
	pd, err := loadApplicationData()
	if err != nil {
		return nil, fmt.Errorf("cannot reload results: %w", err)
	}
	publish(pd)

	return &DeselectionResponse{
		DeselectedCount:     pd.DeselectedCount,
		CountedRepositories: len(pd.Repositories),
		TotalLinesOfCode:    pd.GlobalReport.TotalLinesOfCode,
		RawTotalLinesOfCode: pd.RawTotalLinesOfCode,
		Ignored:             ignored,
	}, nil
}

// requestedReport maps a URL name to the artifact it serves and the download file name
// offered for it.
type requestedReport struct {
	variant  reportVariant
	path     func(reportVariant) string
	filename string
}

// reportRoutes is the set of downloadable reports. The full-scan entries keep their
// original URLs so existing links and bookmarks still work, and they always describe
// the whole scan regardless of any selection.
//
// Download file names are explicit and self-describing because these files get detached
// from the dashboard and emailed: two PDFs called GlobalReport.pdf that disagree about
// the total would be genuinely dangerous.
var reportRoutes = map[string]requestedReport{
	"global-report.pdf": {fullScanVariant, reportVariant.globalPDFPath, "GlobalReport_full-scan.pdf"},
	"repository-summary.pdf": {fullScanVariant, reportVariant.summaryPDFPath,
		"RepositorySummary_full-scan.pdf"},
	"repository-summary.csv": {fullScanVariant, reportVariant.summaryCSVPath,
		"RepositorySummary_full-scan.csv"},

	"global-report-customized.pdf": {customizedVariant, reportVariant.globalPDFPath,
		"GlobalReport_selection.pdf"},
	"repository-summary-customized.pdf": {customizedVariant, reportVariant.summaryPDFPath,
		"RepositorySummary_selection.pdf"},
	"repository-summary-customized.csv": {customizedVariant, reportVariant.summaryCSVPath,
		"RepositorySummary_selection.csv"},
}

// handleReport serves a report, generating it first if it is missing or was built from a
// different selection. Generating on access rather than when the selection changes means
// the user never waits for PDF work they might not need, and a report is never served
// stale.
func handleReport(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/reports/")
	route, ok := reportRoutes[name]
	if !ok {
		http.NotFound(w, r)
		return
	}

	variant := route.variant
	filename := route.filename
	if variant.customized && len(utils.LoadDeselectedRepos(resultsBaseDir)) == 0 {
		// Nothing is deselected, so there is no distinct customized report to serve.
		// Fall back to the full scan rather than 404 on a link left over from a
		// selection that has since been reset.
		variant = fullScanVariant
		filename = reportRoutes[strings.Replace(name, "-customized", "", 1)].filename
	}

	regenerateMu.Lock()
	err := ensureReports(variant)
	regenerateMu.Unlock()
	if err != nil {
		http.Error(w, fmt.Sprintf("Could not generate the report: %v", err), http.StatusInternalServerError)
		return
	}

	filePath := route.path(variant)
	if _, statErr := os.Stat(filePath); statErr != nil {
		http.Error(w, "Report not available", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	http.ServeFile(w, r, filePath)
}

// rebuildReportsInBackground brings every offered report up to date without blocking a
// request. Failures are logged, not surfaced: the next download regenerates on demand and
// reports the error to whoever is actually waiting for the file.
func rebuildReportsInBackground() {
	regenerateMu.Lock()
	defer regenerateMu.Unlock()
	if err := syncReportVariantsLocked(); err != nil {
		fmt.Println("⚠️  background report generation failed:", err)
	}
}

// syncReportVariantsLocked makes the report files on disk match the current selection:
// the variants that should exist are generated, and the customized directory is removed
// when no selection applies to it.
//
// The removal is repeated here rather than trusted to have happened at reset time so the
// state converges from any starting point — a crash between saving an empty selection and
// deleting the directory would otherwise leave understated reports in the tree for good.
//
// Callers must hold regenerateMu.
func syncReportVariantsLocked() error {
	variants := variantsToBuild()

	customizedApplies := false
	for _, v := range variants {
		if v.customized {
			customizedApplies = true
		}
	}
	if !customizedApplies {
		if err := utils.ClearCustomizedReports(resultsBaseDir); err != nil {
			return fmt.Errorf("cannot remove stale customized reports: %w", err)
		}
	}

	for _, v := range variants {
		if err := ensureReports(v); err != nil {
			return err
		}
	}
	return nil
}

// variantsToBuild returns the report variants worth having on disk right now: the full
// scan always, plus the customized set only when a selection actually exists.
func variantsToBuild() []reportVariant {
	if len(utils.LoadDeselectedRepos(resultsBaseDir)) == 0 {
		return []reportVariant{fullScanVariant}
	}
	return []reportVariant{fullScanVariant, customizedVariant}
}

// setupHTTPHandlers configures all HTTP route handlers. pageData seeds the view;
// handlers then read it through snapshot() so a deselection rebuild is picked up.
func setupHTTPHandlers(pageData PageData) {
	publish(pageData)

	// Load HTML template. The "add" helper renders 1-based row numbers in the
	// skipped-repositories table (Go templates have no built-in increment).
	tmpl := template.Must(template.New("index").Funcs(template.FuncMap{
		"add": func(a, b int) int { return a + b },
	}).Parse(htmlTemplate))

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		err := tmpl.Execute(w, snapshot())
		if err != nil {
			http.Error(w, "❌ Error executing HTML template", http.StatusInternalServerError)
			return
		}
	})

	http.HandleFunc("/api/deselected", handleDeselected)

	http.HandleFunc("/download", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			zipResults(w, r)
			return
		}
		http.Error(w, "❌ Method not allowed", http.StatusMethodNotAllowed)
	})

	http.HandleFunc("/reports/", handleReport)

	// API Endpoint for Language Data
	http.HandleFunc("/api/languages", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set(contentTypeHeader, applicationJSONType)
		json.NewEncoder(w).Encode(snapshot().RawLanguages)
	})

	// API Endpoint for Global Info
	http.HandleFunc("/api/global-info", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set(contentTypeHeader, applicationJSONType)
		json.NewEncoder(w).Encode(snapshot().GlobalReport)
	})

	// API Endpoint for Repository Data. Returns the repositories counted in the
	// totals; deselected ones are served by /api/deselected.
	http.HandleFunc("/api/repositories", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set(contentTypeHeader, applicationJSONType)
		json.NewEncoder(w).Encode(snapshot().Repositories)
	})

	// API Endpoint for repositories that could not be analyzed (clone timeout/failure,
	// analysis error). Always returns a JSON array (empty when nothing was skipped).
	http.HandleFunc("/api/skipped-repositories", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set(contentTypeHeader, applicationJSONType)
		skipped := snapshot().SkippedRepos
		if skipped == nil {
			skipped = []utils.SkippedRepo{}
		}
		json.NewEncoder(w).Encode(skipped)
	})

	// API Endpoint for the per-run repository breakdown (scanned/analyzed/archived/
	// empty/excluded/skipped). Returns null when no summary was persisted.
	http.HandleFunc("/api/scan-summary", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set(contentTypeHeader, applicationJSONType)
		json.NewEncoder(w).Encode(snapshot().ScanSummary)
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

	// Report the build and exit. This binary is shipped and run separately from golc-launcher, so
	// it needs its own way to answer "which release is this?".
	if len(os.Args) > 1 && (os.Args[1] == "--version" || os.Args[1] == "-v") {
		fmt.Printf("GoLC ResultsAll %s\n", assets.Version)
		os.Exit(0)
	}

	// No report generation at startup: reports are built when they are first requested,
	// and a stale one cannot be served because the freshness stamp covers both the
	// selection and the scan identity.
	if deselected := utils.LoadDeselectedRepos(resultsBaseDir); len(deselected) > 0 {
		fmt.Printf("ℹ️  %d repositories are deselected — totals exclude them\n", len(deselected))
	}

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
                  {{/* Reports are generated when first requested, so a link may take a
                       moment on its first click — the JS below shows a spinner. The
                       full-scan entries always cover every analysed repository, whatever
                       is currently selected. */}}
                  {{if .DeselectedCount}}<li><h6 class="dropdown-header">Full scan &mdash; all {{.ScannedRepositories}} repositories</h6></li>{{end}}
                  <li><a class="dropdown-item report-link" href="/reports/global-report.pdf" download><i class="fas fa-file-pdf text-primary me-2"></i>Global Report PDF</a></li>
                  <li><a class="dropdown-item report-link" href="/reports/repository-summary.pdf" download><i class="fas fa-file-pdf text-success me-2"></i>Repository Summary PDF</a></li>
                  <li><a class="dropdown-item report-link" href="/reports/repository-summary.csv" download><i class="fas fa-file-csv me-2" style="color:#e67e22;"></i>Repository Summary CSV</a></li>
                  {{if .DeselectedCount}}
                  <li><hr class="dropdown-divider"></li>
                  <li><h6 class="dropdown-header">Current selection &mdash; {{.DeselectedCount}} excluded</h6></li>
                  <li><a class="dropdown-item report-link" href="/reports/global-report-customized.pdf" download><i class="fas fa-file-pdf text-primary me-2"></i>Global Report PDF <span class="badge bg-secondary ms-1" style="font-size:0.65em;">customized</span></a></li>
                  <li><a class="dropdown-item report-link" href="/reports/repository-summary-customized.pdf" download><i class="fas fa-file-pdf text-success me-2"></i>Repository Summary PDF <span class="badge bg-secondary ms-1" style="font-size:0.65em;">customized</span></a></li>
                  <li><a class="dropdown-item report-link" href="/reports/repository-summary-customized.csv" download><i class="fas fa-file-csv me-2" style="color:#e67e22;"></i>Repository Summary CSV <span class="badge bg-secondary ms-1" style="font-size:0.65em;">customized</span></a></li>
                  {{end}}
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
                  <i class="fas fa-code-branch"></i> Lines of Code by Repository ({{len .Repositories}} repositories counted{{if .DeselectedCount}}, {{.DeselectedCount}} deselected{{end}})
                </h5>
                <div class="card-body">

                  <!-- Deselection controls: uncheck a repository to remove it from every
                       total on this page and from the generated reports. -->
                  <div id="selectionBar" class="d-flex flex-wrap align-items-center gap-2 mb-3 p-2 rounded" style="background-color:#eef2f7;">
                    <span class="fw-bold" style="color:#333;"><i class="fas fa-filter"></i> Selection</span>
                    <span id="selectionSummary" class="text-muted small"></span>
                    <div class="ms-auto d-flex flex-wrap gap-2">
                      <button type="button" id="btnSelectAll" class="btn btn-sm btn-outline-secondary">Select all</button>
                      <button type="button" id="btnApplySelection" class="btn btn-sm btn-primary" disabled>
                        <i class="fas fa-check"></i> Apply selection
                      </button>
                      <button type="button" id="btnResetSelection" class="btn btn-sm btn-outline-danger"{{if not .DeselectedCount}} disabled{{end}}>
                        Reset to full scan
                      </button>
                    </div>
                  </div>
                  <div id="selectionStatus" class="alert d-none py-2 small" role="status"></div>
                  {{if .DeselectedCount}}
                  <div class="alert alert-secondary py-2 small" role="note">
                    <i class="fas fa-info-circle"></i>
                    <strong>{{.DeselectedCount}}</strong> repositories ({{.DeselectedCodeLines}} code lines) are excluded from every
                    total on this page and in the PDF/CSV reports. Total across all analyzed
                    repositories was <strong>{{.RawTotalLinesOfCode}}</strong>.
                  </div>
                  {{end}}

                  <div class="table-responsive">
                    <table class="table table-striped table-hover">
                      <thead class="table-dark">
                        <tr>
                          <th scope="col" style="width:2.5rem;">
                            <input type="checkbox" id="selectAllCheckbox" class="form-check-input" checked
                                   title="Select or deselect every repository" aria-label="Select all repositories">
                          </th>
                          <th scope="col">#</th>
                          <th scope="col" class="sortable" data-column="repository">
                            Repository <i class="fas fa-sort sort-icon"></i>
                          </th>
                          <th scope="col" class="sortable" data-column="branch">
                            Branch <i class="fas fa-sort sort-icon"></i>
                          </th>
                          <th scope="col" class="sortable" data-column="language"
                              title="The {{.TopLanguagesShown}} largest languages by code lines. JSON is excluded, matching the Code Lines column.">
                            Top Languages <i class="fas fa-sort sort-icon"></i>
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
                        {{/* One loop over TableRows, not counted-then-deselected: a
                             deselected repository keeps its ranked position so its size
                             relative to the others stays visible and the row stays where
                             the user left it. */}}
                        {{range .TableRows}}
                        <tr {{if .Deselected}}class="deselected-row" style="opacity:0.55;" {{end}}data-key="{{.Key}}" data-repository="{{if eq $platform "gitlab"}}{{.Org}}/{{end}}{{.Repository}}" data-branch="{{.Branch}}" data-language="{{.PrimaryLanguage}}" data-lines="{{.Lines}}" data-blanklines="{{.BlankLines}}" data-comments="{{.Comments}}" data-codelines="{{.CodeLines}}">
                          <td><input type="checkbox" class="form-check-input repo-select" {{if not .Deselected}}checked {{end}}value="{{.Key}}" aria-label="Count {{.Repository}} in the totals"></td>
                          <td class="row-num">{{if .Deselected}}&mdash;{{else}}{{.Number}}{{end}}</td>
                          <td>
                            <a href="/repository/{{.Repository}}/{{.Branch}}" class="repo-link">{{if and (eq $platform "gitlab") .Org}}<span class="text-muted" style="font-size:0.85em;">{{.Org}}&thinsp;/&thinsp;</span>{{end}}{{.Repository}}</a>
                            {{if .Deselected}}<span class="badge bg-secondary ms-1" style="font-size:0.65em;">deselected</span>{{end}}
                          </td>
                          <td>{{.Branch}}</td>
                          <td class="top-languages">{{template "topLanguages" .TopLanguages}}</td>
                          <td>{{.LinesF}}</td>
                          <td>{{.BlankLinesF}}</td>
                          <td>{{.CommentsF}}</td>
                          <td><strong>{{.CodeLinesF}}</strong></td>
                        </tr>
                        {{end}}
                      </tbody>
                      <tfoot class="table-secondary">
                        <tr id="totalsRow">
                          <td></td>
                          <td><strong>Total</strong></td>
                          <td colspan="3"><strong id="totalRepoCount">{{len .Repositories}} repositories</strong></td>
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

      {{if .ScanSummary}}
      <!-- Scan Summary Section -->
      <section id="scan-summary-section" style="background-color: #f8f9fa; padding: 2rem 0 1rem 0;">
        <div class="container">
          <div class="row">
            <div class="col-12">
              <div class="card shadow-lg">
                <h5 class="card-header text-white" style="background-color:#0073ba;">
                  <i class="fas fa-clipboard-list"></i> Scan Summary
                </h5>
                <div class="card-body">
                  <p class="text-muted mb-3" style="font-size:0.9rem;">
                    Repository breakdown for this run. <strong>Scanned</strong> is the total discovered; it splits into <strong>Analyzed</strong> plus the repositories filtered out (<strong>Archived</strong>/disabled, <strong>Empty</strong>, <strong>Excluded</strong>) and those that could not be completed (<strong>Skipped</strong>).{{if .ScanSummary.Deselected}} <strong>Deselected</strong> counts repositories that were analyzed but removed from the totals by selection — they are part of <strong>Analyzed</strong>, not a separate slice of <strong>Scanned</strong>.{{end}}
                  </p>
                  <div class="row text-center">
                    <div class="col">
                      <div class="h3 mb-0">{{.ScanSummary.Scanned}}</div>
                      <div class="text-muted small">Scanned</div>
                    </div>
                    <div class="col">
                      <div class="h3 mb-0 text-success">{{.ScanSummary.Analyzed}}</div>
                      <div class="text-muted small">Analyzed</div>
                    </div>
                    <div class="col">
                      <div class="h3 mb-0">{{.ScanSummary.Archived}}</div>
                      <div class="text-muted small">Archived</div>
                    </div>
                    <div class="col">
                      <div class="h3 mb-0">{{.ScanSummary.Empty}}</div>
                      <div class="text-muted small">Empty</div>
                    </div>
                    <div class="col">
                      <div class="h3 mb-0">{{.ScanSummary.Excluded}}</div>
                      <div class="text-muted small">Excluded</div>
                    </div>
                    <div class="col">
                      <div class="h3 mb-0" style="color:#d68910;">{{.ScanSummary.Skipped}}</div>
                      <div class="text-muted small">Skipped</div>
                    </div>
                    {{if .ScanSummary.Deselected}}
                    <div class="col">
                      <div class="h3 mb-0" style="color:#485468;">{{.ScanSummary.Deselected}}</div>
                      <div class="text-muted small">Deselected</div>
                    </div>
                    {{end}}
                  </div>
                </div>
              </div>
            </div>
          </div>
        </div>
      </section>
      {{end}}

      {{if .SkippedRepos}}
      <!-- Skipped Repositories Section -->
      <section id="skipped-section" style="background-color: #f8f9fa; padding: 0 0 3rem 0;">
        <div class="container">
          <div class="row">
            <div class="col-12">
              <div class="card shadow-lg">
                <h5 class="card-header text-white" style="background-color:#d68910;">
                  <i class="fas fa-exclamation-triangle"></i> Skipped Repositories ({{len .SkippedRepos}})
                </h5>
                <div class="card-body">
                  <p class="text-muted mb-3" style="font-size:0.9rem;">
                    These repositories were targeted but could not be analyzed (clone timeout, clone failure, or analysis error) and are <strong>not</strong> included in the totals above. Review the reason, then retry, raise the clone timeout, or add them to the exclusion list.
                  </p>
                  <div class="table-responsive">
                    <table class="table table-striped table-hover">
                      <thead class="table-dark">
                        <tr>
                          <th scope="col">#</th>
                          <th scope="col">Repository</th>
                          <th scope="col">Branch</th>
                          <th scope="col">Reason</th>
                        </tr>
                      </thead>
                      <tbody>
                        {{range $i, $r := .SkippedRepos}}
                        <tr>
                          <td>{{add $i 1}}</td>
                          <td>{{if $r.ProjectKey}}<span class="text-muted" style="font-size:0.85em;">{{$r.ProjectKey}}&thinsp;/&thinsp;</span>{{end}}{{$r.RepoSlug}}</td>
                          <td>{{$r.Branch}}</td>
                          <td><code style="color:#b9770e;">{{$r.Reason}}</code></td>
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

        function formatNumber(num) {
            return num.toLocaleString();
        }

        // Totals are summed from the checked rows rather than from server-rendered
        // constants, so unchecking a repository updates them immediately — before
        // Apply rebuilds the reports server-side.
        function calculateRepositoryTotals() {
            let totalLines = 0, totalBlankLines = 0, totalComments = 0, totalCodeLines = 0, counted = 0;

            document.querySelectorAll('#repositoryTableBody tr').forEach(row => {
                const box = row.querySelector('.repo-select');
                if (!box || !box.checked) return;
                counted++;
                totalLines      += parseInt(row.dataset.lines) || 0;
                totalBlankLines += parseInt(row.dataset.blanklines) || 0;
                totalComments   += parseInt(row.dataset.comments) || 0;
                totalCodeLines  += parseInt(row.dataset.codelines) || 0;
            });

            document.getElementById('totalLines').innerHTML = '<strong>' + formatNumber(totalLines) + '</strong>';
            document.getElementById('totalBlankLines').innerHTML = '<strong>' + formatNumber(totalBlankLines) + '</strong>';
            document.getElementById('totalComments').innerHTML = '<strong>' + formatNumber(totalComments) + '</strong>';
            document.getElementById('totalCodeLines').innerHTML = '<strong>' + formatNumber(totalCodeLines) + '</strong>';
            const repoCount = document.getElementById('totalRepoCount');
            if (repoCount) repoCount.textContent = counted + ' repositories';
        }

        // ─── Repository deselection ──────────────────────────────────────────
        // The set persisted server-side when the page was rendered. Comparing against
        // it tells us whether the current checkboxes are a real change, so Apply is
        // only enabled when there is something to apply.
        const persistedDeselected = new Set({{.DeselectedKeys}});

        function currentDeselectedKeys() {
            const keys = [];
            document.querySelectorAll('#repositoryTableBody .repo-select').forEach(box => {
                if (!box.checked) keys.push(box.value);
            });
            return keys;
        }

        function sameAsPersisted(keys) {
            if (keys.length !== persistedDeselected.size) return false;
            return keys.every(k => persistedDeselected.has(k));
        }

        function showSelectionStatus(message, variant) {
            const el = document.getElementById('selectionStatus');
            el.className = 'alert alert-' + variant + ' py-2 small';
            el.innerHTML = message;
        }

        function refreshSelectionUI() {
            const keys = currentDeselectedKeys();
            const total = document.querySelectorAll('#repositoryTableBody .repo-select').length;
            const counted = total - keys.length;

            document.getElementById('selectionSummary').textContent =
                counted + ' of ' + total + ' repositories counted' +
                (keys.length ? ' · ' + keys.length + ' deselected' : '');

            // Deselecting everything would leave a zero-LOC report with no way back
            // from this page, so it is blocked here as well as server-side.
            const apply = document.getElementById('btnApplySelection');
            apply.disabled = sameAsPersisted(keys) || counted === 0;
            apply.title = counted === 0 ? 'At least one repository must remain counted' : '';

            const box = document.getElementById('selectAllCheckbox');
            box.checked = keys.length === 0;
            box.indeterminate = keys.length > 0 && counted > 0;

            calculateRepositoryTotals();
        }

        async function submitSelection(keys) {
            const apply = document.getElementById('btnApplySelection');
            const reset = document.getElementById('btnResetSelection');
            apply.disabled = true;
            reset.disabled = true;
            showSelectionStatus('<i class="fas fa-spinner fa-spin"></i> Applying selection…', 'info');

            try {
                const res = await fetch('/api/deselected', {
                    method: 'POST',
                    headers: {'Content-Type': 'application/json'},
                    body: JSON.stringify({Keys: keys})
                });
                if (!res.ok) {
                    showSelectionStatus('<i class="fas fa-exclamation-triangle"></i> ' +
                        (await res.text() || 'Could not apply the selection.'), 'danger');
                    refreshSelectionUI();
                    reset.disabled = false;
                    return;
                }
                // Reload so the page, the chart, the language breakdown and the
                // download links all come from the freshly regenerated reports.
                window.location.reload();
            } catch (err) {
                showSelectionStatus('<i class="fas fa-exclamation-triangle"></i> ' + err, 'danger');
                refreshSelectionUI();
                reset.disabled = false;
            }
        }

        document.querySelectorAll('#repositoryTableBody .repo-select').forEach(box => {
            box.addEventListener('change', refreshSelectionUI);
        });

        document.getElementById('selectAllCheckbox').addEventListener('change', function() {
            const checked = this.checked;
            document.querySelectorAll('#repositoryTableBody .repo-select').forEach(box => { box.checked = checked; });
            refreshSelectionUI();
        });

        document.getElementById('btnSelectAll').addEventListener('click', function() {
            document.querySelectorAll('#repositoryTableBody .repo-select').forEach(box => { box.checked = true; });
            refreshSelectionUI();
        });

        document.getElementById('btnApplySelection').addEventListener('click', function() {
            submitSelection(currentDeselectedKeys());
        });

        document.getElementById('btnResetSelection').addEventListener('click', function() {
            submitSelection([]);
        });

        refreshSelectionUI();

        // ─── Report downloads ────────────────────────────────────────────────
        // Reports are generated when first requested, which can take a moment on a large
        // org. A plain download link would just appear to do nothing, so the click is
        // intercepted to show progress and the file is handed to the browser afterwards.
        document.querySelectorAll('.report-link').forEach(link => {
            link.addEventListener('click', async function(event) {
                event.preventDefault();
                if (link.dataset.busy === '1') return;

                const original = link.innerHTML;
                link.dataset.busy = '1';
                link.innerHTML = '<i class="fas fa-spinner fa-spin me-2"></i>Preparing…';

                try {
                    const res = await fetch(link.href);
                    if (!res.ok) throw new Error((await res.text()) || res.statusText);

                    // Honour the file name the server chose, so a customized report is
                    // never saved under a name suggesting it covers the whole scan.
                    const disposition = res.headers.get('Content-Disposition') || '';
                    const match = /filename="?([^"]+)"?/.exec(disposition);
                    const blob = await res.blob();
                    const url = URL.createObjectURL(blob);
                    const a = document.createElement('a');
                    a.href = url;
                    a.download = match ? match[1] : 'report';
                    document.body.appendChild(a);
                    a.click();
                    a.remove();
                    URL.revokeObjectURL(url);
                } catch (err) {
                    showSelectionStatus('<i class="fas fa-exclamation-triangle"></i> Could not prepare the report: ' + err, 'danger');
                } finally {
                    link.innerHTML = original;
                    delete link.dataset.busy;
                }
            });
        });

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
                
                if (column === 'repository' || column === 'branch' || column === 'language') {
                    // Sorts on the primary language; repositories with no language data
                    // carry an empty value and sort together.
                    aVal = (a.dataset[column] || '').toLowerCase();
                    bVal = (b.dataset[column] || '').toLowerCase();
                    return currentSort.direction === 'asc' ?
                        aVal.localeCompare(bVal) : bVal.localeCompare(aVal);
                } else {
                    aVal = parseInt(a.dataset[column]);
                    bVal = parseInt(b.dataset[column]);
                    return currentSort.direction === 'asc' ? aVal - bVal : bVal - aVal;
                }
            });
            
            // Re-append rows and renumber the counted ones. Deselected rows keep their
            // dash: they are not part of the numbered sequence.
            let counted = 0;
            rows.forEach(row => {
                const numCell = row.querySelector('.row-num');
                const box = row.querySelector('.repo-select');
                if (numCell) {
                    numCell.textContent = (box && !box.checked) ? '—' : ++counted;
                }
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

{{/* Renders a repository's largest languages as "Go 12.3K · Java 4.1K · XML 900".
     An em dash when the by-language result file was missing, so "unknown" is visibly
     unknown rather than an empty-looking cell. */}}
{{define "topLanguages"}}{{if .}}{{range $i, $lang := .}}{{if $i}} <span class="text-muted">·</span> {{end}}<span style="font-weight:500;">{{$lang.Language}}</span>&nbsp;<span class="text-muted" style="font-size:0.85em;">{{$lang.CodeLinesF}}</span>{{end}}{{else}}<span class="text-muted">&mdash;</span>{{end}}{{end}}
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
