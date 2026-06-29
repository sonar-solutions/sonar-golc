//go:build webui || golc
// +build webui golc

package main

import (
	"archive/zip"
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/SonarSource-Demos/sonar-golc/assets"
	"github.com/SonarSource-Demos/sonar-golc/pkg/goloc"
	"github.com/briandowns/spinner"

	"github.com/SonarSource-Demos/sonar-golc/pkg/devops/getazure"
	getbibucket "github.com/SonarSource-Demos/sonar-golc/pkg/devops/getbitbucket"
	getbibucketdc "github.com/SonarSource-Demos/sonar-golc/pkg/devops/getbitbucketdc"
	"github.com/SonarSource-Demos/sonar-golc/pkg/devops/getgithub"
	"github.com/SonarSource-Demos/sonar-golc/pkg/devops/getgitlab"
	"github.com/SonarSource-Demos/sonar-golc/pkg/utils"
)

type OrganizationData struct {
	Organization           string `json:"Organization"`
	TotalLinesOfCode       string `json:"TotalLinesOfCode"`
	LargestRepository      string `json:"LargestRepository"`
	LinesOfCodeLargestRepo string `json:"LinesOfCodeLargestRepo"`
	DevOpsPlatform         string `json:"DevOpsPlatform"`
	NumberRepos            int    `json:"NumberRepos"`
}

type Repository struct {
	ID            int    `json:"id"`
	Name          string `json:"name"`
	DefaultBranch string `json:"default_branch"`
	Path          string `json:"path"`
}

type Project struct {
	KEY    string `json:"key"`
	ID     int    `json:"id"`
	Name   string `json:"name"`
	Public bool   `json:"public"`
	Type   string `json:"type"`
	Links  Links  `json:"links"`
}

type Links struct {
	Self []SelfLink `json:"self"`
}

type SelfLink struct {
	Href string `json:"href"`
}

type Config struct {
	Platforms map[string]interface{} `json:"platforms"`
	Release   ReleaseConfig          `json:"release"`
}

type ReleaseConfig struct {
	Version string `json:"version"`
}

type Report struct {
	TotalFiles      int `json:",omitempty"`
	TotalLines      int
	TotalBlankLines int
	TotalComments   int
	TotalCodeLines  int
	Results         interface{}
}

type Result struct {
	TotalFiles      int           `json:"TotalFiles"`
	TotalLines      int           `json:"TotalLines"`
	TotalBlankLines int           `json:"TotalBlankLines"`
	TotalComments   int           `json:"TotalComments"`
	TotalCodeLines  int           `json:"TotalCodeLines"`
	Results         []LanguageRes `json:"Results"`
}

type LanguageRes struct {
	Language   string `json:"Language"`
	Files      int    `json:"Files"`
	Lines      int    `json:"Lines"`
	BlankLines int    `json:"BlankLines"`
	Comments   int    `json:"Comments"`
	CodeLines  int    `json:"CodeLines"`
}

type RepoParams struct {
	ProjectKey   string
	Namespace    string
	RepoSlug     string
	MainBranch   string
	PathToScan   string
	WorkDir      string
	CloneTimeout time.Duration
}

type logWriter struct {
	stdout  *os.File
	logFile *os.File
}

const errorMessageRepo = "❌ Error Analyse Repositories: "
const errorMessageDi = "\r❌ Error deleting Repository Directory: %v\n"
const errorMessageAnalyse = "\r❌ No Analysis performed...\n"
const errorMessageRepos = "Error Get Info Repositories in organization '%s' : '%s'"
const directoryconf = "/config"

var logFile *os.File
var AppConfig Config
var logger *logrus.Logger
var version1 = "2.0"

var directoriesToCreate = []string{
	directoryconf,
	"/byfile-report",
	"/bylanguage-report",
	"/byfile-report/csv-report",
	"/byfile-report/pdf-report",
	"/bylanguage-report/csv-report",
	"/bylanguage-report/pdf-report",
}

// Check Exclusion File Exist
func getFileNameIfExists(filePath string) string {
	_, err := os.Stat(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			//The file does not exist
			return "0"
		} else {
			// Check file
			//fmt.Printf("❌ Error check file exclusion: %v\n", err)
			logger.Errorf("❌ Error check file exclusion: %v\n", err)
			return "0"
		}
	} else {
		return filePath
	}
}

// Load Config File
func LoadConfig(filename string) (Config, error) {
	var config Config

	// Lire le contenu du fichier de configuration
	data, err := os.ReadFile(filename)
	if err != nil {
		return config, fmt.Errorf("❌ failed to read config file: %v", err)
	}

	if err := json.Unmarshal(data, &config); err != nil {
		return config, fmt.Errorf("❌ failed to parse config JSON: %v", err)
	}

	return config, nil
}

// Parse Result Files in JSON Format
func parseJSONFile(filePath, reponame string) int {
	file, err := os.ReadFile(filePath)
	if err != nil {
		//fmt.Println("❌ Error reading file:", err)
		logger.Errorf("❌ Error reading file: %v", err)
	}

	var report Report
	err = json.Unmarshal(file, &report)
	if err != nil {
		//fmt.Println("❌ Error parsing JSON:", err)
		logger.Errorf("❌ Error parsing JSON: %v", err)
	}

	return report.TotalCodeLines
}

// convert To Slice String
func convertToSliceString(in []interface{}) []string {
	out := make([]string, len(in))
	for i, v := range in {
		out[i] = v.(string)
	}
	return out
}

// Extract url domain
func extractDomain(url string) string {
	// Remove the "http://" or "https://" prefix
	url = strings.TrimPrefix(url, "https://")
	url = strings.TrimPrefix(url, "http://")

	// Find the index of the first "/"
	index := strings.Index(url, "/")

	// If "/" is found, return the part before "/"
	if index != -1 {
		return url[:index]
	}

	// Otherwise, return the entire url (in case there is no "/")
	return url
}

// Create a Bakup File for Result directory
func createBackup(sourceDir, pwd string) error {
	backupDir := filepath.Join(pwd, "Saves")
	backupFilePath := generateBackupFilePath(sourceDir, backupDir)

	if err := createBackupDirectory(backupDir); err != nil {
		return err
	}

	if err := ZipDirectory(sourceDir, backupFilePath); err != nil {
		return err
	}

	logger.Infof("✅ Backup created successfully:%s", backupFilePath)
	return nil
}

func ZipDirectory(source string, target string) error {
	// Création du fichier zip
	zipFile, err := os.Create(target)
	if err != nil {
		return err
	}
	defer zipFile.Close()

	// Création d'un nouvel archive zip
	zipWriter := zip.NewWriter(zipFile)
	defer zipWriter.Close()

	// Parcours du répertoire source
	return filepath.Walk(source, func(file string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// On construit le chemin relatif pour le zip
		relativePath, err := filepath.Rel(filepath.Dir(source), file)
		if err != nil {
			return err
		}

		if fi.IsDir() {
			// Ajouter le répertoire au zip
			_, err := zipWriter.Create(relativePath + "/")
			return err
		}

		// Ouvrir le fichier à zipper
		fileToZip, err := os.Open(file)
		if err != nil {
			return err
		}
		defer fileToZip.Close()

		// Créer une entrée dans le zip
		writer, err := zipWriter.Create(relativePath)
		if err != nil {
			return err
		}

		// Copier le contenu du fichier dans l'entrée zip
		_, err = io.Copy(writer, fileToZip)
		return err
	})
}

// Generate Backup File Name
func generateBackupFilePath(sourceDir, backupDir string) string {
	backupFileName := fmt.Sprintf("%s_%s.zip", filepath.Base(sourceDir), time.Now().Format("2006-01-02_15-04-05"))
	return filepath.Join(backupDir, backupFileName)
}

// Create a backup Directory
func createBackupDirectory(backupDir string) error {
	if _, err := os.Stat(backupDir); os.IsNotExist(err) {
		if err := os.MkdirAll(backupDir, 0755); err != nil {
			return fmt.Errorf("error creating backup directory: %s", err)
		}
	}
	return nil
}

// Add Files in backup
func addFilesToBackup(sourceDir string, zipWriter *zip.Writer) error {
	return filepath.Walk(sourceDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if path == sourceDir {
			return nil
		}
		relPath, err := filepath.Rel(sourceDir, path)
		if err != nil {
			return err
		}
		if err := addFileToZip(path, relPath, info, zipWriter); err != nil {
			return err
		}
		return nil
	})
}

func addFileToZip(filePath, relPath string, fileInfo os.FileInfo, zipWriter *zip.Writer) error {
	zipFile, err := zipWriter.Create(relPath)
	if err != nil {
		return err
	}
	if !fileInfo.IsDir() {
		file, err := os.Open(filePath)
		if err != nil {
			return err
		}
		defer file.Close()
		_, err = io.Copy(zipFile, file)
		if err != nil {
			return err
		}
	}
	return nil
}

// Generic function to analyze repositories
func AnalyseReposList(DestinationResult string, platformConfig map[string]interface{}, repolist interface{}, analyseRepoFunc func(project interface{}, DestinationResult string, platformConfig map[string]interface{}, spin *spinner.Spinner, results chan int, count *int)) (cpt int) {
	//fmt.Print("\n🔎 Analysis of Repos ...\n")
	logger.Infof("🔎 Analysis of Repos ...\n")

	spin := spinner.New(spinner.CharSets[35], 100*time.Millisecond)
	spin.Color("green", "bold")
	spin.FinalMSG = ""

	repos := repolist.([]interface{})
	total := len(repos)

	// Reset the per-run skip recorder so a re-run does not inherit stale entries.
	skipped.reset()

	// Effective concurrency:
	//   - multithreading off        -> 1 (strictly sequential)
	//   - list <= NumberWorkerRepos -> run them all at once (preserves prior behavior)
	//   - otherwise                 -> Workers
	concurrency := 1
	if platformConfig["Multithreading"].(bool) {
		if total <= int(platformConfig["NumberWorkerRepos"].(float64)) {
			concurrency = total
		} else {
			concurrency = int(platformConfig["Workers"].(float64))
		}
	}
	if concurrency < 1 {
		concurrency = 1
	}

	// Rolling worker pool: a semaphore caps concurrency while every finished repo
	// frees its slot for the next one immediately. This replaces the old fixed-batch
	// barrier, where a single slow or hung repository stalled its entire batch (up to
	// Workers-1 idle workers) and froze overall progress. Combined with the per-repo
	// clone timeout in gogit.Getrepos, no single repository can hang the whole scan.
	results := make(chan int, total) // buffered so worker sends never block
	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup
	count := 1

	for _, project := range repos {
		wg.Add(1)
		sem <- struct{}{} // acquire a slot (blocks once concurrency is reached)
		go func(p interface{}) {
			defer wg.Done()
			defer func() { <-sem }() // release the slot
			analyseRepoFunc(p, DestinationResult, platformConfig, spin, results, &count)
		}(project)
	}

	wg.Wait()
	close(results)

	// Persist the skipped repositories so the ResultsAll web page and the PDF report
	// can surface them. DestinationResult is the base Results directory at this point.
	skippedList := skipped.snapshot()
	if err := utils.SaveSkippedRepos(DestinationResult, skippedList); err != nil {
		logger.Errorf("❌ Error saving skipped repositories: %v", err)
	}
	if len(skippedList) > 0 {
		logger.Warnf("⚠️  %d repository(ies) were skipped during analysis (see report for details)", len(skippedList))
	}

	return total
}

func getExcludePaths(configValue interface{}) []string {
	if configValue == nil {
		return []string{}
	}
	if excludePaths, ok := configValue.([]interface{}); ok {
		return convertToSliceString(excludePaths)
	}
	return []string{}
}

func getStringSliceConfig(platformConfig map[string]interface{}, key string) []string {
	return getExcludePaths(platformConfig[key])
}

// getWorkDir returns the optional per-platform "WorkDir" setting (base directory for
// temporary clones). Absent or non-string => "", which makes goloc fall back to the
// GOLC_WORKDIR env var and then os.TempDir() (historical default).
func getWorkDir(platformConfig map[string]interface{}) string {
	if v, ok := platformConfig["WorkDir"]; ok && v != nil {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// defaultCloneTimeoutMinutes bounds a single repository clone by default so that
// one stalled clone can no longer hang an entire org-wide scan. Operators can raise
// it for very large repos, or set CloneTimeout to 0 to disable the deadline.
const defaultCloneTimeoutMinutes = 15.0

// getCloneTimeout reads the optional per-platform "CloneTimeout" setting (in minutes)
// and returns it as a duration. Absent/invalid => default; an explicit 0 (or negative)
// disables the deadline.
func getCloneTimeout(platformConfig map[string]interface{}) time.Duration {
	minutes := defaultCloneTimeoutMinutes
	if v, ok := platformConfig["CloneTimeout"]; ok && v != nil {
		if f, ok := v.(float64); ok {
			if f <= 0 {
				return 0
			}
			minutes = f
		}
	}
	return time.Duration(minutes * float64(time.Minute))
}

// skippedRepo describes a repository/branch that the analysis phase could not
// complete (clone timeout, clone failure, or counting error). It maps directly to
// utils.SkippedRepo for persistence.
type skippedRepo struct {
	ProjectKey string
	RepoSlug   string
	Branch     string
	Reason     string
}

// skipRecorder accumulates skipped repositories across the concurrent analysis
// workers. A single golc run analyzes one platform, so a package-level recorder
// (reset at the start of each AnalyseReposList) is sufficient; the mutex makes it
// safe for the worker goroutines to record concurrently.
type skipRecorder struct {
	mu    sync.Mutex
	items []skippedRepo
}

func (r *skipRecorder) reset() {
	r.mu.Lock()
	r.items = nil
	r.mu.Unlock()
}

func (r *skipRecorder) add(s skippedRepo) {
	r.mu.Lock()
	r.items = append(r.items, s)
	r.mu.Unlock()
}

func (r *skipRecorder) snapshot() []utils.SkippedRepo {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]utils.SkippedRepo, len(r.items))
	for i, s := range r.items {
		out[i] = utils.SkippedRepo{
			ProjectKey: s.ProjectKey,
			RepoSlug:   s.RepoSlug,
			Branch:     s.Branch,
			Reason:     s.Reason,
		}
	}
	return out
}

var skipped = &skipRecorder{}

// exitGolc terminates the process after sweeping any temp clones that deferred
// cleanup would otherwise miss (os.Exit does not run deferred funcs) — issue #81.
// Use this instead of os.Exit anywhere a clone may already exist on disk.
func exitGolc(code int) {
	utils.CleanupTempClones()
	os.Exit(code)
}

// Analysis functions for different repository types

// Analysis functions for Bitbucket Cloud
func analyseBitCRepo(project interface{}, DestinationResult string, platformConfig map[string]interface{}, spin *spinner.Spinner, results chan int, count *int) {
	p := project.(getbibucket.ProjectBranch)
	var excludeExtensions []string

	excludeExtensions = convertToSliceString(platformConfig["ExtExclusion"].([]interface{}))
	excludePath := getExcludePaths(platformConfig["ExcludePaths"])
	folderKeywords := getStringSliceConfig(platformConfig, "FolderKeywords")
	fileNamePatterns := getStringSliceConfig(platformConfig, "FileNamePatterns")

	// Determine git clone URL format
	// For git operations with API tokens, use x-bitbucket-api-token-auth (static username for API tokens)
	// For API calls, we use email:token (Basic Auth)
	// For git clone, we use x-bitbucket-api-token-auth:token format for API tokens
	workspace := platformConfig["Workspace"].(string)
	users := ""
	if usersVal, ok := platformConfig["Users"]; ok && usersVal != nil {
		users = usersVal.(string)
	}

	var pathToScan string
	if users != "" && users != "XXXXX" {
		// For git operations with API tokens, use x-bitbucket-api-token-auth as the username
		// This is a static username that Bitbucket provides for API token authentication
		// The actual Bitbucket username field in the API is the workspace ID, not suitable for git
		pathToScan = fmt.Sprintf("%s://x-bitbucket-api-token-auth:%s@%s/%s/%s.git", platformConfig["Protocol"].(string), platformConfig["AccessToken"].(string), platformConfig["Baseapi"].(string), workspace, p.RepoSlug)
	} else {
		// Use x-token-auth format for App Passwords (legacy)
		pathToScan = fmt.Sprintf("%s://x-token-auth:%s@%s/%s/%s.git", platformConfig["Protocol"].(string), platformConfig["AccessToken"].(string), platformConfig["Baseapi"].(string), workspace, p.RepoSlug)
	}

	params := RepoParams{
		ProjectKey: p.ProjectKey,
		Namespace:  "",
		RepoSlug:   p.RepoSlug,
		MainBranch: p.MainBranch,
		PathToScan: pathToScan,
		WorkDir:    getWorkDir(platformConfig),
		CloneTimeout: getCloneTimeout(platformConfig),
	}
	performRepoAnalysis(params, DestinationResult, spin, results, count, excludeExtensions, excludePath, folderKeywords, fileNamePatterns, platformConfig["ResultByFile"].(bool), platformConfig["ResultAll"].(bool))
}

// Analysis functions for Bitbucket DC
func analyseBitSRVRepo(project interface{}, DestinationResult string, platformConfig map[string]interface{}, trimmedURL string, spin *spinner.Spinner, results chan int, count *int) {
	p := project.(getbibucketdc.ProjectBranch)
	var excludeExtensions []string

	excludeExtensions = convertToSliceString(platformConfig["ExtExclusion"].([]interface{}))
	excludePath := getExcludePaths(platformConfig["ExcludePaths"])
	folderKeywords := getStringSliceConfig(platformConfig, "FolderKeywords")
	fileNamePatterns := getStringSliceConfig(platformConfig, "FileNamePatterns")

	params := RepoParams{
		ProjectKey: p.ProjectKey,
		Namespace:  "",
		RepoSlug:   p.RepoSlug,
		MainBranch: p.MainBranch,
		PathToScan: fmt.Sprintf("%s://%s:%s@%sscm/%s/%s.git", platformConfig["Protocol"].(string), platformConfig["Users"].(string), platformConfig["AccessToken"].(string), trimmedURL, p.ProjectKey, p.RepoSlug),
		WorkDir:    getWorkDir(platformConfig),
		CloneTimeout: getCloneTimeout(platformConfig),
	}
	performRepoAnalysis(params, DestinationResult, spin, results, count, excludeExtensions, excludePath, folderKeywords, fileNamePatterns, platformConfig["ResultByFile"].(bool), platformConfig["ResultAll"].(bool))
}

// Analysis functions for GitHub
func analyseGithubRepo(project interface{}, DestinationResult string, platformConfig map[string]interface{}, spin *spinner.Spinner, results chan int, count *int) {
	p := project.(getgithub.ProjectBranch)
	var excludeExtensions []string

	excludeExtensions = convertToSliceString(platformConfig["ExtExclusion"].([]interface{}))
	excludePath := getExcludePaths(platformConfig["ExcludePaths"])
	folderKeywords := getStringSliceConfig(platformConfig, "FolderKeywords")
	fileNamePatterns := getStringSliceConfig(platformConfig, "FileNamePatterns")

	baseapi := extractDomain(platformConfig["Baseapi"].(string))
	params := RepoParams{
		ProjectKey: p.Org,
		Namespace:  "",
		RepoSlug:   p.RepoSlug,
		MainBranch: p.MainBranch,
		PathToScan: fmt.Sprintf("%s://%s:x-oauth-basic@%s/%s/%s.git", platformConfig["Protocol"].(string), platformConfig["AccessToken"].(string), baseapi, p.Org, p.RepoSlug),
		WorkDir:    getWorkDir(platformConfig),
		CloneTimeout: getCloneTimeout(platformConfig),
	}
	performRepoAnalysis(params, DestinationResult, spin, results, count, excludeExtensions, excludePath, folderKeywords, fileNamePatterns, platformConfig["ResultByFile"].(bool), platformConfig["ResultAll"].(bool))
}

// Analysis functions for GitLab
func analyseGitlabRepo(project interface{}, DestinationResult string, platformConfig map[string]interface{}, spin *spinner.Spinner, results chan int, count *int) {
	p := project.(getgitlab.ProjectBranch)
	var excludeExtensions []string

	excludeExtensions = convertToSliceString(platformConfig["ExtExclusion"].([]interface{}))
	excludePath := getExcludePaths(platformConfig["ExcludePaths"])
	folderKeywords := getStringSliceConfig(platformConfig, "FolderKeywords")
	fileNamePatterns := getStringSliceConfig(platformConfig, "FileNamePatterns")

	domain := extractDomain(platformConfig["Url"].(string))

	params := RepoParams{
		ProjectKey: p.Org,
		Namespace:  p.Namespace,
		RepoSlug:   p.RepoSlug,
		MainBranch: p.MainBranch,
		PathToScan: fmt.Sprintf("%s://gitlab-ci-token:%s@%s/%s.git", platformConfig["Protocol"].(string), platformConfig["AccessToken"].(string), domain, p.Namespace),
		WorkDir:    getWorkDir(platformConfig),
		CloneTimeout: getCloneTimeout(platformConfig),
	}
	performRepoAnalysis(params, DestinationResult, spin, results, count, excludeExtensions, excludePath, folderKeywords, fileNamePatterns, platformConfig["ResultByFile"].(bool), platformConfig["ResultAll"].(bool))
}

func analyseAzurebRepo(project interface{}, DestinationResult string, platformConfig map[string]interface{}, spin *spinner.Spinner, results chan int, count *int) {
	p := project.(getazure.ProjectBranch)
	var excludeExtensions []string

	excludeExtensions = convertToSliceString(platformConfig["ExtExclusion"].([]interface{}))
	excludePath := getExcludePaths(platformConfig["ExcludePaths"])
	folderKeywords := getStringSliceConfig(platformConfig, "FolderKeywords")
	fileNamePatterns := getStringSliceConfig(platformConfig, "FileNamePatterns")

	params := RepoParams{
		ProjectKey: p.ProjectKey,
		Namespace:  "",
		RepoSlug:   p.RepoSlug,
		MainBranch: p.MainBranch,
		PathToScan: fmt.Sprintf("%s://%s@%s/%s/%s/%s/%s", platformConfig["Protocol"].(string), platformConfig["AccessToken"].(string), "dev.azure.com", platformConfig["Organization"].(string), p.ProjectKey, "_git", p.RepoSlug),
		WorkDir:    getWorkDir(platformConfig),
		CloneTimeout: getCloneTimeout(platformConfig),
	}
	performRepoAnalysis(params, DestinationResult, spin, results, count, excludeExtensions, excludePath, folderKeywords, fileNamePatterns, platformConfig["ResultByFile"].(bool), platformConfig["ResultAll"].(bool))
}

// Perform repository analysis (common logic)
func performRepoAnalysis(params RepoParams, DestinationResult string, spin *spinner.Spinner, results chan int, count *int, excludeExtension []string, excludePaths []string, folderKeywords []string, fileNamePatterns []string, ResultByFile bool, ResultAll bool) {
	// Always use a consistent filename pattern so downstream parsing works across platforms.
	// Format: Result_<OrgOrProjectKey>__<RepoSlug>__<Branch>
	// The double-underscore field separator keeps `_` free to appear inside any component
	// (GitLab group names, repo slugs, branch names like feat_xyz), so the parser in
	// pkg/reporter/pdf can recover all three fields unambiguously.
	outputFileName := fmt.Sprintf("Result_%s__%s__%s", params.ProjectKey, params.RepoSlug, params.MainBranch)
	golocParams := goloc.Params{
		Path:             params.PathToScan,
		ByFile:           ResultByFile,
		ByAll:            ResultAll,
		ExcludePaths:     excludePaths,
		ExcludeExtensions: excludeExtension,
		IncludeExtensions: []string{},
		FolderKeywords:   folderKeywords,
		FileNamePatterns: fileNamePatterns,
		OrderByLang:       false,
		OrderByFile:       false,
		OrderByCode:       false,
		OrderByLine:       false,
		OrderByBlank:      false,
		OrderByComment:    false,
		Order:             "DESC",
		OutputName:        outputFileName,
		OutputPath:        DestinationResult,
		ReportFormats:     []string{"json"},
		Branch:            params.MainBranch,
		Cloned:            false,
		Repopath:          "",
		WorkDir:           params.WorkDir,
		CloneTimeout:      params.CloneTimeout,
	}
	if ResultAll {
		golocParams.ByFile = true
	}

	// recordSkip notes that this repository/branch could not be analyzed so it can be
	// surfaced (with a reason) in the web page and PDF report instead of silently
	// vanishing from the totals.
	recordSkip := func(reason string) {
		skipped.add(skippedRepo{
			ProjectKey: params.ProjectKey,
			RepoSlug:   params.RepoSlug,
			Branch:     params.MainBranch,
			Reason:     reason,
		})
	}

	MessB := fmt.Sprintf("   Extracting files from repo : %s ", params.RepoSlug)
	spin.Suffix = MessB
	spin.Start()

	gc, err := goloc.NewGCloc(golocParams, assets.Languages)
	if err != nil {
		spin.Stop()
		logger.Errorf(errorMessageRepo+"%v", err)
		recordSkip(err.Error())
		*count++
		results <- 1
		return
	} else {

		// Guarantee cleanup of the temp clone on every exit path (including the
		// gc.Run() / NewGCloc error returns below). Both analysis passes share the
		// same clone directory, so capture it once here; without this, a failed
		// analysis leaked its clone and slowly filled the work dir (issue #81).
		repoPath, repoDisposable := gc.Repopath, gc.RepopathDisposable
		defer func() {
			if repoDisposable && repoPath != "" {
				if err1 := os.RemoveAll(repoPath); err1 != nil {
					logger.Errorf(errorMessageDi, err1)
				}
				utils.UnregisterTempClone(repoPath)
			}
		}()

		//gc.Run()
		//*count++

		if ResultAll {

			if err := gc.Run(); err != nil {
				fmt.Print("\n")
				logger.Errorf("❌ Error during analysis with ByAll = true: %v", err)
				recordSkip(fmt.Sprintf("analysis error: %v", err))
				*count++
				results <- 1
				return
			}

			// Second call to Run with ByFile = false
			golocParams.ByFile = false
			golocParams.Cloned = true
			golocParams.Repopath = gc.Repopath
			golocParams.RepopathDisposable = gc.RepopathDisposable

			gc, err = goloc.NewGCloc(golocParams, assets.Languages)
			if err != nil {
				fmt.Print("\n")
				logger.Errorf("❌ Error initializing GCloc for ByFile = false: %v", err)
				recordSkip(fmt.Sprintf("analysis error: %v", err))
				*count++
				results <- 1
				return
			}

			if err := gc.Run(); err != nil {
				fmt.Print("\n")
				logger.Errorf("❌ Error during analysis with ByFile = false: %v", err)
				recordSkip(fmt.Sprintf("analysis error: %v", err))
				*count++
				results <- 1
				return
			}
		} else {
			// If ByAll = false, just run normally
			if err := gc.Run(); err != nil {
				fmt.Print("\n")
				logger.Errorf("❌ Error during analysis: %v", err)
				recordSkip(fmt.Sprintf("analysis error: %v", err))
				*count++
				results <- 1
				return
			}
		}

		// Temp-clone cleanup is handled by the deferred RemoveAll registered above,
		// so it runs on success and on every early-return error path alike.
		golocParams.Cloned = false
		spin.Stop()
		logger.Infof("\r\t\t\t\t✅ %d The repository <%s> has been analyzed\n", *count, params.RepoSlug)
		// Send result through channel
		results <- 1
	}
}

// Specific analysis functions calling the generic one

// Analysis function call for BitBucket Cloud
func AnalyseReposListBitC(DestinationResult string, platformConfig map[string]interface{}, repolist []getbibucket.ProjectBranch) (cpt int) {
	repoInterfaces := make([]interface{}, len(repolist))
	for i, v := range repolist {
		repoInterfaces[i] = v
	}
	return AnalyseReposList(DestinationResult, platformConfig, repoInterfaces, analyseBitCRepo)
}

// Analysis function call for BitBucket DC
func AnalyseReposListBitSRV(DestinationResult string, platformConfig map[string]interface{}, repolist []getbibucketdc.ProjectBranch) (cpt int) {
	URLcut := platformConfig["Protocol"].(string) + "://"
	trimmedURL := strings.TrimPrefix(platformConfig["Url"].(string), URLcut)
	repoInterfaces := make([]interface{}, len(repolist))
	for i, v := range repolist {
		repoInterfaces[i] = v
	}
	return AnalyseReposList(DestinationResult, platformConfig, repoInterfaces, func(project interface{}, DestinationResult string, platformConfig map[string]interface{}, spin *spinner.Spinner, results chan int, count *int) {
		analyseBitSRVRepo(project, DestinationResult, platformConfig, trimmedURL, spin, results, count)
	})
}

// Analysis function call for GitHub
func AnalyseReposListGithub(DestinationResult string, platformConfig map[string]interface{}, repolist []getgithub.ProjectBranch) (cpt int) {
	repoInterfaces := make([]interface{}, len(repolist))
	for i, v := range repolist {
		repoInterfaces[i] = v
	}
	return AnalyseReposList(DestinationResult, platformConfig, repoInterfaces, analyseGithubRepo)
}

// Analysis function call for Gitlab
func AnalyseReposListGitlab(DestinationResult string, platformConfig map[string]interface{}, repolist []getgitlab.ProjectBranch) (cpt int) {
	repoInterfaces := make([]interface{}, len(repolist))
	for i, v := range repolist {
		repoInterfaces[i] = v
	}
	return AnalyseReposList(DestinationResult, platformConfig, repoInterfaces, analyseGitlabRepo)
}

// Analysis function call for Gitlab
func AnalyseReposListAzure(DestinationResult string, platformConfig map[string]interface{}, repolist []getazure.ProjectBranch) (cpt int) {
	repoInterfaces := make([]interface{}, len(repolist))
	for i, v := range repolist {
		repoInterfaces[i] = v
	}
	return AnalyseReposList(DestinationResult, platformConfig, repoInterfaces, analyseAzurebRepo)
}

/* ---------------- File analysis result serialisation ---------------- */

type fileProjectBranch struct {
	Org         string `json:"Org"`
	ProjectKey  string `json:"ProjectKey"`
	RepoSlug    string `json:"RepoSlug"`
	MainBranch  string `json:"MainBranch"`
	LargestSize int64  `json:"LargestSize"`
}

type fileAnalysisResult struct {
	NumRepositories int                  `json:"NumRepositories"`
	ProjectBranches []fileProjectBranch  `json:"ProjectBranches"`
}

func saveFileAnalysisResult(destDir, org string, dirs []string) error {
	result := fileAnalysisResult{}
	for _, d := range dirs {
		result.ProjectBranches = append(result.ProjectBranches, fileProjectBranch{
			Org:        org,
			RepoSlug:   filepath.Base(d),
			MainBranch: "file",
		})
	}
	result.NumRepositories = len(result.ProjectBranches)

	configDir := filepath.Join(destDir, "config")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return err
	}
	f, err := os.Create(filepath.Join(configDir, "analysis_result_file.json"))
	if err != nil {
		return err
	}
	defer f.Close()
	return json.NewEncoder(f).Encode(result)
}

/* ---------------- Analyse Directory ---------------- */

// analyseDirectory runs the goloc analysis for a single directory entry.
func analyseDirectory(dir string, ResultByFile, ResultAll bool, fileexclusionEX, extexclusion, folderKeywords, fileNamePatterns []string, destDir string, count *int) {
	params := goloc.Params{
		Path:              dir,
		ByFile:            ResultByFile,
		ByAll:             ResultAll,
		ExcludePaths:      fileexclusionEX,
		ExcludeExtensions: extexclusion,
		IncludeExtensions: []string{},
		FolderKeywords:    folderKeywords,
		FileNamePatterns:  fileNamePatterns,
		OrderByLang:       false,
		OrderByFile:       false,
		OrderByCode:       false,
		OrderByLine:       false,
		OrderByBlank:      false,
		OrderByComment:    false,
		Order:             "DESC",
		OutputName:        "Result_",
		OutputPath:        destDir,
		ReportFormats:     []string{"json"},
		Branch:            "",
		Token:             "",
		Cloned:            false,
		Repopath:          "",
	}
	if ResultAll {
		params.ByFile = true
	}

	gc, err := goloc.NewGCloc(params, assets.Languages)
	if err != nil {
		logger.Errorf(errorMessageRepo+"%v", err)
		return
	}

	// Directory analysis normally points at the user's local path (not disposable),
	// but if goloc had to extract to a temp dir this guarantees it is removed on
	// every exit path, consistent with the repository analysis path (issue #81).
	if gc.RepopathDisposable && gc.Repopath != "" {
		defer func(p string) {
			if err1 := os.RemoveAll(p); err1 != nil {
				logger.Errorf(errorMessageDi, err1)
			}
			utils.UnregisterTempClone(p)
		}(gc.Repopath)
	}

	if err := runGlocPasses(gc, params, ResultAll); err != nil {
		return
	}

	logger.Infof("\t✅ %d The directory <%s> has been analyzed\n", *count, dir)
	*count++
}

// runGlocPasses executes either a dual-pass (ResultAll) or single-pass analysis.
func runGlocPasses(gc *goloc.GCloc, params goloc.Params, ResultAll bool) error {
	if !ResultAll {
		if err := gc.Run(); err != nil {
			fmt.Print("\n")
			logger.Errorf("❌ Error during analysis: %v", err)
			return err
		}
		return nil
	}

	// First run: byfile report
	if err := gc.Run(); err != nil {
		fmt.Print("\n")
		logger.Errorf("❌ Error during analysis (byfile pass): %v", err)
		return err
	}

	// Second run: bylanguage report
	params.ByFile = false
	params.Cloned = true
	params.Repopath = gc.Repopath
	params.RepopathDisposable = gc.RepopathDisposable

	gc2, err := goloc.NewGCloc(params, assets.Languages)
	if err != nil {
		fmt.Print("\n")
		logger.Errorf("❌ Error initializing GCloc for bylanguage pass: %v", err)
		return err
	}
	if err := gc2.Run(); err != nil {
		fmt.Print("\n")
		logger.Errorf("❌ Error during analysis (bylanguage pass): %v", err)
		return err
	}
	return nil
}

func AnalyseReposListFile(Listdirectorie, fileexclusionEX []string, extexclusion, folderKeywords, fileNamePatterns []string, ResultByFile bool, ResultAll bool, destDir string) {
	logger.Infof("🔎 Analysis of Directories ...\n")

	var wg sync.WaitGroup
	wg.Add(len(Listdirectorie))
	count := 1

	for _, Listdirectories := range Listdirectorie {
		go func(dir string) {
			defer wg.Done()
			analyseDirectory(dir, ResultByFile, ResultAll, fileexclusionEX, extexclusion, folderKeywords, fileNamePatterns, destDir, &count)
		}(Listdirectories)
	}

	wg.Wait()
}

/* ---------------- End Analyse Directory ---------------- */

func AnalyseRun(params goloc.Params, reponame string) {
	gc, err := goloc.NewGCloc(params, assets.Languages)
	if err != nil {
		fmt.Println(errorMessageRepo, err)
		exitGolc(1)
	}

	gc.Run()
}

func AnalyseRepo(DestinationResult string, Users string, AccessToken string, DevOps string, Organization string, reponame string) (cpt int) {

	//pathToScan := fmt.Sprintf("git::https://%s@%s.com/%s/%s", AccessToken, DevOps, Organization, reponame)
	pathToScan := fmt.Sprintf("https://%s:%s@%s.com/%s/%s", Users, AccessToken, DevOps, Organization, reponame)

	outputFileName := fmt.Sprintf("Result_%s", reponame)
	params := goloc.Params{
		Path:              pathToScan,
		ByFile:            false,
		ByAll:             false,
		ExcludePaths:      []string{},
		ExcludeExtensions: []string{},
		IncludeExtensions: []string{},
		OrderByLang:       false,
		OrderByFile:       false,
		OrderByCode:       false,
		OrderByLine:       false,
		OrderByBlank:      false,
		OrderByComment:    false,
		Order:             "DESC",
		OutputName:        outputFileName,
		OutputPath:        DestinationResult,
		ReportFormats:     []string{"json"},
		Branch:            "",
		Token:             "",
		Cloned:            true,
		Repopath:          "",
	}
	gc, err := goloc.NewGCloc(params, assets.Languages)
	if err != nil {
		fmt.Println(errorMessageRepo, err)
		exitGolc(1)
	}

	gc.Run()
	cpt++

	// Remove repository directory only if it is a temp clone, not the user's source directory
	if gc.RepopathDisposable && gc.Repopath != "" {
		err1 := os.RemoveAll(gc.Repopath)
		if err1 != nil {
			fmt.Printf(errorMessageDi, err1)
			return
		}
		utils.UnregisterTempClone(gc.Repopath)
	}

	return cpt
}

// Function Read LoadFile for list of directories
func ReadLines(filename string) ([]string, error) {

	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)

	var lines []string

	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return lines, nil
}

func displayLanguages() {
	fmt.Printf("%-18s | %-78s | %-15s | %s\n", "Language", "Extensions", "Single Comments", "Multi Line Comments")
	fmt.Println("-------------------+--------------------------------------------------------------------------------+-----------------+--------------------")

	for lang, config := range assets.Languages {
		extensions := strings.Join(config.Extensions, ", ") // Concatenate extensions with comma separator

		singleComments := strings.Join(config.LineComments, ", ") // Concatenate single comments with comma separator

		multiLineComments := ""
		for _, comments := range config.MultiLineComments {
			for _, comment := range comments {
				multiLineComments += comment + " "
			}
		}

		fmt.Printf("%-18s | %-78s | %-15s | %s\n", lang, extensions, singleComments, multiLineComments)
	}
}

func createDirectories(basePath string, paths []string) {
	for _, path := range paths {
		fullPath := basePath + path
		if err := os.MkdirAll(fullPath, os.ModePerm); err != nil {
			panic(err)
		}
	}
}

// setupResultsDirectory creates the Results directory tree for a given platform.
func setupResultsDirectory(platform string) string {
	pwd, err := os.Getwd()
	if err != nil {
		fmt.Println("Error:", err)
	}
	DestinationResult := pwd + "/Results"
	logger.Infof("✅ Using configuration for DevOps platform '%s'\n", platform)
	_ = os.RemoveAll(DestinationResult)
	if err := os.MkdirAll(DestinationResult, os.ModePerm); err != nil {
		panic(err)
	}
	createDirectories(DestinationResult, directoriesToCreate)
	return DestinationResult
}

// runGolcInProcess runs the GoLC analysis for the given platform key (e.g. "Github").
// It is invoked by the webui binary when started with --internal-run <platform>.
func runGolcInProcess(platform string) {
	// Sweep any temp clones still registered by failed/partial analyses on the
	// normal completion path too. Successful repos self-clean via their own
	// deferred RemoveAll; this catches clones whose NewGCloc/clone failed before
	// that defer was installed (os.Exit paths are covered by exitGolc) — issue #81.
	defer utils.CleanupTempClones()

	// Load config
	configPath := os.Getenv("GOLC_CONFIG_FILE")
	if configPath == "" {
		configPath = "config.json"
	}
	var err error
	AppConfig, err = LoadConfig(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "\n❌ Failed to load config: %s\n", err)
		exitGolc(1)
	}
	if AppConfig.Release.Version != version1 {
		fmt.Fprintf(os.Stderr, "\n❌ Version mismatch: expected %s but got %s - Use the correct config.json file!\n", version1, AppConfig.Release.Version)
		exitGolc(1)
	}

	// Setup logging
	logDir := "Logs"
	if _, statErr := os.Stat(logDir); os.IsNotExist(statErr) {
		if mkErr := os.MkdirAll(logDir, 0755); mkErr != nil {
			fmt.Fprintf(os.Stderr, "❌ Failed to create log directory: %v\n", mkErr)
			exitGolc(1)
		}
	}
	_ = os.Remove("Logs/Logs.log")
	_ = os.Remove("Logs/debug.log")
	utils.ResetSharedLogger()
	logger = utils.SharedLogger()
	logger.Info("✅ Configuration loaded successfully and version matched!")

	// Resolve platform config
	platformConfig, ok := AppConfig.Platforms[platform].(map[string]interface{})
	if !ok {
		fmt.Fprintf(os.Stderr, "\n❌ Configuration for DevOps platform '%s' not found\n", platform)
		exitGolc(1)
	}

	var maxTotalCodeLines int
	var maxProject, maxRepo string
	var NumberRepos int
	var startTime time.Time
	var ListDirectory []string
	var ListExclusion []string
	var message0, message1, message2, message3, message4, message5 string

	// Setup results directory
	DestinationResult := setupResultsDirectory(platform)
	fmt.Printf("\n")

	// Create Global Report File

	GlobalReport := DestinationResult + "/GlobalReport.txt"
	file, err := os.Create(GlobalReport)
	if err != nil {
		logger.Errorf("❌ Error creating file:%v", err)
		return
	}
	defer file.Close()

	/*---------------------------------- Select type of DevOps Platform ----------------------------------------------------*/

	switch devops := platformConfig["DevOps"].(string); devops {

	case "azure":
		var fileexclusion = ".cloc_azure_ignore"
		fileexclusionEX := getFileNameIfExists(fileexclusion)

		startTime = time.Now()

		gitproject, err := getazure.GetRepoAzureList(platformConfig, fileexclusionEX)
		if err != nil {
			//fmt.Printf(errorMessageRepos, platformConfig["Organization"].(string), err)
			logger.Errorf(errorMessageRepos, platformConfig["Organization"].(string), err)
			return
		}

		if len(gitproject) == 0 {
			logger.Error(errorMessageAnalyse)
			exitGolc(1)

		} else {

			NumberRepos = AnalyseReposListAzure(DestinationResult, platformConfig, gitproject)

		}

	case "github":

		var fileexclusion = ".cloc_github_ignore"
		fileexclusionEX := getFileNameIfExists(fileexclusion)
		var fast bool

		startTime = time.Now()

		if false {
			// fast mode (not available via web UI)
		} else {
			fast = false

			if false {
				fmt.Println("🌿 All-branches mode enabled for Github")
				logger.Infof("🌿 All-branches mode enabled - analyzing ALL branches for each repository")

				// Get the main repositories list (one per repo)
				repositories, err := getgithub.GetRepoGithubList(platformConfig, fileexclusionEX, fast)
				if err != nil {
					logger.Errorf(errorMessageRepos, platformConfig["Organization"].(string), err)
					return
				}

				if len(repositories) == 0 {
					logger.Error(errorMessageAnalyse)
					exitGolc(1)
				} else {
					// Get all branches for each repository and analyze them
					allBranches, err := getgithub.GetAllBranchesForRepositories(platformConfig, repositories)
					if err != nil {
						logger.Errorf("❌ Error getting all branches: %v", err)
						return
					}

					NumberRepos = AnalyseReposListGithub(DestinationResult, platformConfig, allBranches)
				}
			} else {
				repositories, err := getgithub.GetRepoGithubList(platformConfig, fileexclusionEX, fast)
				if err != nil {
					logger.Errorf(errorMessageRepos, platformConfig["Organization"].(string), err)
					return
				}

				if len(repositories) == 0 {
					logger.Error(errorMessageAnalyse)
					exitGolc(1)

				} else {

					NumberRepos = AnalyseReposListGithub(DestinationResult, platformConfig, repositories)

				}
			}
		}

	case "gitlab":

		var fileexclusion = ".cloc_gitlab_ignore"
		fileexclusionEX := getFileNameIfExists(fileexclusion)

		startTime = time.Now()

		gitproject, err := getgitlab.GetRepoGitLabList(platformConfig, fileexclusionEX)
		if err != nil {
			logger.Errorf(errorMessageRepos, platformConfig["Organization"].(string), err)
			return
		}

		if len(gitproject) == 0 {
			logger.Error(errorMessageAnalyse)
			exitGolc(1)

		} else {
			//exitGolc(1)
			NumberRepos = AnalyseReposListGitlab(DestinationResult, platformConfig, gitproject)

		}

	case "bitbucket_dc":

		var fileexclusion = platformConfig["FileExclusion"].(string)
		fileexclusionEX := getFileNameIfExists(fileexclusion)

		startTime = time.Now()
		projects, err := getbibucketdc.GetProjectBitbucketList(platformConfig, fileexclusionEX)
		if err != nil {
			logger.Errorf("❌ Error Get Info Projects in Bitbucket server '%s' : ", err)
			exitGolc(1)
		}

		if len(projects) == 0 {
			logger.Error(errorMessageAnalyse)
			exitGolc(1)

		} else {

			// Run scanning repositories
			NumberRepos = AnalyseReposListBitSRV(DestinationResult, platformConfig, projects)
		}

	case "bitbucket":
		var fileexclusion = platformConfig["FileExclusion"].(string)
		fileexclusionEX := getFileNameIfExists(fileexclusion)

		startTime = time.Now()

		projects1, err := getbibucket.GetProjectBitbucketListCloud(platformConfig, fileexclusionEX)

		if err != nil {
			logger.Errorf("❌ Error Get Info Project(s) in Bitbucket cloud '%v' ", err)
			return
		}
		if len(projects1) == 0 {
			logger.Errorf(errorMessageAnalyse)
			exitGolc(1)

		} else {
			// Run scanning repositories
			NumberRepos = AnalyseReposListBitC(DestinationResult, platformConfig, projects1)
		}

	case "file":

		fileexclusionEX := getFileNameIfExists(platformConfig["FileExclusion"].(string))
		fileload := getFileNameIfExists(platformConfig["FileLoad"].(string))
		var excludeExtensions []string
		excludeExtensions = convertToSliceString(platformConfig["ExtExclusion"].([]interface{}))

		if fileexclusionEX != "0" {
			ListExclusion, err = ReadLines(fileexclusionEX)
			if err != nil {
				logger.Errorf("❌ Error reading file <.cloc_file_ignore>:%v", err)
				exitGolc(1)
			}
		} else {
			ListExclusion = make([]string, 0)

		}

		if fileload != "0" {
			ListDirectory, err = ReadLines(fileload)
			if err != nil {
				logger.Errorf("❌ Error reading file <.cloc_file_load>:%v", err)
				exitGolc(1)
			}
			if len(ListDirectory) == 0 {
				ListDirectory = append(ListDirectory, platformConfig["Directory"].(string))
			}
		} else {
			dirField := platformConfig["Directory"].(string)
			if len(dirField) == 0 {
				logger.Error("❌ No analysis possible, no directory, specified file or specified loading file")
				exitGolc(1)
			} else {
				for _, d := range strings.Split(dirField, "\n") {
					if d = strings.TrimSpace(d); d != "" {
						ListDirectory = append(ListDirectory, d)
					}
				}
			}
		}
		logger.Debugf("→ file mode: %d exclusion(s) loaded", len(ListExclusion))
		logger.Debugf("→ file mode: %d director(ies) to scan", len(ListDirectory))

		// Expand to immediate subdirectories when ScanSubDirs is set
		scanSubDirs, _ := platformConfig["ScanSubDirs"].(bool)
		if scanSubDirs {
			var expanded []string
			for _, d := range ListDirectory {
				entries, err := os.ReadDir(d)
				if err != nil {
					logger.Errorf("❌ Cannot read directory <%s>: %v", d, err)
					continue
				}
				found := false
				for _, e := range entries {
					if e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
						expanded = append(expanded, filepath.Join(d, e.Name()))
						found = true
					}
				}
				if !found {
					// No subdirectories — analyze the directory itself
					expanded = append(expanded, d)
				}
			}
			if len(expanded) > 0 {
				ListDirectory = expanded
				logger.Debugf("→ file mode: ScanSubDirs expanded to %d director(ies)", len(ListDirectory))
			}
		}
		for _, d := range ListDirectory {
			logger.Debugf("→ directory %s: analyzing", d)
		}
		startTime = time.Now()
		AnalyseReposListFile(ListDirectory, ListExclusion, excludeExtensions, getStringSliceConfig(platformConfig, "FolderKeywords"), getStringSliceConfig(platformConfig, "FileNamePatterns"), platformConfig["ResultByFile"].(bool), platformConfig["ResultAll"].(bool), DestinationResult)

		// Write analysis_result_file.json so ResultsAll can list the repos
		if err := saveFileAnalysisResult(DestinationResult, platformConfig["Organization"].(string), ListDirectory); err != nil {
			logger.Errorf("❌ Failed to write analysis_result_file.json: %v", err)
		}
	}

	/*---------------------------------- End Select type of DevOps Platform ----------------------------------------------------*/

	// Begin of report file analysis
	//fmt.Print("\n🔎 Analyse Report ...\n")

	fmt.Printf("\n")
	logger.Infof("🔎 Analyse Report ...\n")
	spin := spinner.New(spinner.CharSets[35], 100*time.Millisecond)
	spin.Suffix = " Analyse Report..."
	spin.Color("green", "bold")
	spin.Start()

	if platformConfig["ResultAll"].(bool) {

		DestinationResult = DestinationResult + "/bylanguage-report/"
	} else if platformConfig["ResultByFile"].(bool) {

		DestinationResult = DestinationResult + "/byfile-report/"
	} else {

		DestinationResult = DestinationResult + "/bylanguage-report/"
	}

	// List files in the directory
	files, err := os.ReadDir(DestinationResult)
	if err != nil {
		logger.Errorf("❌ Error listing files:%v", err)
		exitGolc(1)
	}

	// Initialize the sum of TotalCodeLines (excluding JSON to match SonarQube behavior)
	totalCodeLinesSum := 0

	// Analyse All file
	for _, file := range files {
		// Check if the file is a JSON file
		if !file.IsDir() && strings.HasSuffix(file.Name(), ".json") {
			// Hard-cutover: only accept canonically-named result files. Legacy
			// single-`_` names and other malformed names contribute neither LOC
			// nor LargestRepository candidacy — their identity is not
			// authoritative and they will be regenerated on the next analysis
			// run. Parsing happens before the JSON body is read so an unparsable
			// file has zero side-effects on totals or largest-tracking.
			isFilePlatform := platformConfig["DevOps"].(string) == "file"
			var parsedOrg, parsedRepo string
			if isFilePlatform {
				// File platform writes single-component Result_<Repo>.json.
				parsedRepo = strings.TrimSuffix(strings.TrimPrefix(file.Name(), "Result_"), ".json")
			} else {
				org, repo, _, ok := utils.ParseResultFileName(file.Name())
				if !ok {
					logger.Warnf("⚠️  Skipping unparsable result file: %s", file.Name())
					continue
				}
				parsedOrg = org
				parsedRepo = repo
			}

			// Read contents of JSON file
			filePath := filepath.Join(DestinationResult, file.Name())
			jsonData, err := os.ReadFile(filePath)
			if err != nil {
				logger.Errorf("❌ Error reading file %s: %v\n", file.Name(), err)
				continue
			}

			// Parse JSON content into a Result structure
			var result Result
			err = json.Unmarshal(jsonData, &result)
			if err != nil {
				logger.Errorf("❌ Error parsing JSON contents of file %s: %v\n", file.Name(), err)
				continue
			}

			// Exclude JSON LOC from total to match SonarQube standard behavior
			jsonLOC := 0
			for _, r := range result.Results {
				if strings.TrimSpace(r.Language) == utils.LanguageExcludedFromTotalLOC {
					jsonLOC += r.CodeLines
					break
				}
			}
			codeLinesForTotal := result.TotalCodeLines - jsonLOC

			totalCodeLinesSum += codeLinesForTotal

			// Update the (max, project, repo) triple atomically — the name was
			// already validated at the top of this iteration, so a new maximum
			// always carries a usable repo identity.
			if codeLinesForTotal > maxTotalCodeLines {
				maxTotalCodeLines = codeLinesForTotal
				maxProject = parsedOrg
				maxRepo = parsedRepo
				if isFilePlatform {
					NumberRepos++
				}
			}
		}

	}
	maxTotalCodeLines1 := utils.FormatCodeLines(float64(maxTotalCodeLines))
	totalCodeLinesSum1 := utils.FormatCodeLines(float64(totalCodeLinesSum))

	if totalCodeLinesSum1 == "0" {
		spin.Stop()
		fmt.Println("\n --------------------------------------------------------------------")
		logger.Errorf("  ❌ Analysis produced 0 lines of code. Possible causes:")
		logger.Errorf("     • The directory/repository path does not exist or is empty")
		logger.Errorf("     • All files were excluded by the exclusion rules")
		logger.Errorf("     • The token lacks read access to the repositories")
		logger.Errorf("     • No source files with recognised extensions were found")
		logger.Errorf("     • Check the logs above for per-repo errors")
		fmt.Println("\n --------------------------------------------------------------------")
		exitGolc(1)
	}

	// Global Result file
	data := OrganizationData{
		Organization:           platformConfig["Organization"].(string),
		TotalLinesOfCode:       totalCodeLinesSum1,
		LargestRepository:      maxRepo,
		LinesOfCodeLargestRepo: maxTotalCodeLines1,
		DevOpsPlatform:         platformConfig["DevOps"].(string),
		NumberRepos:            NumberRepos,
	}

	jsonData, err := json.MarshalIndent(data, "", "    ")
	if err != nil {
		logger.Errorf("❌ Error during JSON encoding in Gobal Report:%v", err)
		return
	}
	// Created Global Result json file
	file1, err := os.Create("Results/GlobalReport.json")
	if err != nil {
		logger.Errorf("❌ Error during file creation Gobal Report:%v", err)
		return
	}
	defer file.Close()

	_, err = file1.Write(jsonData)
	if err != nil {
		logger.Errorf("❌ Error writing to file:%v", err)
		return
	}
	spin.Stop()

	// Determine base Results directory (parent of current DestinationResult)
	baseResultsDir := filepath.Dir(filepath.Clean(DestinationResult))

	// Generated Global Report (walks the directory for Result_* files)
	// Pass the base Results directory for consistency across platforms.
	err = utils.CreateGlobalReport(baseResultsDir)
	if err != nil {
		logger.Errorf("❌ Error creating global report: %v", err)
		exitGolc(1)
	}

	err = utils.GenerateRepositorySummaryReports(baseResultsDir)
	if err != nil {
		logger.Errorf("❌ Error creating repository summary reports: %v", err)
	}

	fmt.Printf("\n")

	endTime := time.Now()
	duration := endTime.Sub(startTime)

	hours := int(duration.Hours())
	minutes := int(duration.Minutes()) % 60
	seconds := int(duration.Seconds()) % 60

	if platformConfig["DevOps"].(string) != "file" {
		message0 = fmt.Sprintf("✅ Number of Repository analyzed in Organization <%s> is %d ", platformConfig["Organization"].(string), NumberRepos)
		message1 = fmt.Sprintf("✅ The repository with the largest line of code is in project <%s> the repo name is <%s> with <%s> lines of code", maxProject, maxRepo, maxTotalCodeLines1)
		message2 = fmt.Sprintf("✅ The total sum of lines of code in Organization <%s> is : %s Lines of Code\n", platformConfig["Organization"].(string), totalCodeLinesSum1)
		message4 = fmt.Sprintf("✅ Time elapsed : %02d:%02d:%02d\n", hours, minutes, seconds)
		message3 = message0 + message1 + message2
		message5 = message3 + message4

	} else {
		message0 = fmt.Sprintf("✅ Number of Directory analyzed in Organization <%s> is %d ", platformConfig["Organization"].(string), NumberRepos)
		message2 = fmt.Sprintf("✅ The total sum of lines of code in Organization <%s> is : %s Lines of Code\n", platformConfig["Organization"].(string), totalCodeLinesSum1)
		message4 = fmt.Sprintf("✅ Time elapsed : %02d:%02d:%02d\n", hours, minutes, seconds)
		message3 = message0 + message2
		message5 = message3 + message4

	}

	// Old logger infos
	/*fmt.Println(message3)
	fmt.Println("\n✅ Reports are located in the <'Results'> directory")
	fmt.Println(message4)*/

	logger.Info(message0)
	logger.Info(message2)
	logger.Infof("✅ Reports are located in the <'Results'> directory")
	logger.Info(message4)

	// Write message in Gobal Report File
	_, err = file.WriteString(message5)
	if err != nil {
		logger.Errorf("❌ Error writing to file:%v", err)
		return
	}

}
