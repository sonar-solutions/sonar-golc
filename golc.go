//go:build webui || golc
// +build webui golc

package main

import (
	"archive/zip"
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
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

// version1 is the config-file schema version this build expects. Only its major
// component is enforced — see configVersionCompatible.
//
// This is the config compatibility version, not the build identity; the release tag is
// stamped into assets.Version at link time.
var version1 = "2.1"

var directoriesToCreate = []string{
	directoryconf,
	"/byfile-report",
	"/bylanguage-report",
	"/byfile-report/csv-report",
	"/byfile-report/pdf-report",
	"/bylanguage-report/csv-report",
	"/bylanguage-report/pdf-report",
}

// configVersionCompatible reports whether a config file's declared version can be used by
// a build expecting `expected`.
//
// Only the major component has to match. A minor release adds optional fields with
// defaults rather than changing the schema, so rejecting a 2.0 config from a 2.1 build
// would force every existing user to hand-edit a file that works perfectly — a startup
// failure with no underlying problem. A major bump remains the signal that the schema
// really changed and the file must be regenerated.
//
// An empty or unparseable version is rejected: those indicate a malformed config rather
// than an older one.
func configVersionCompatible(configVersion, expected string) bool {
	major := majorVersion(configVersion)
	return major != "" && major == majorVersion(expected)
}

// majorVersion extracts the leading numeric component of a version string, tolerating the
// `v`/`ver` prefixes used by release tags. Returns "" when there is no leading number.
func majorVersion(v string) string {
	v = strings.TrimSpace(v)
	v = strings.TrimPrefix(v, "ver")
	v = strings.TrimPrefix(v, "v")
	end := strings.IndexFunc(v, func(r rune) bool { return r < '0' || r > '9' })
	if end == 0 {
		return ""
	}
	if end > 0 {
		return v[:end]
	}
	return v
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
// azureCloneHost returns the authority and any virtual-directory path from a configured
// Azure URL, with the scheme and surrounding slashes removed.
//
// It is not extractDomain: that stops at the first "/", which is right for GitLab but
// wrong here. Azure DevOps Server is routinely installed under a virtual directory - IIS
// defaults to https://server/tfs/ - and dropping that segment makes every clone fail.
// The hosted service has no path, so it resolves to the bare host exactly as before.
func azureCloneHost(rawURL string) string {
	trimmed := strings.TrimPrefix(strings.TrimPrefix(rawURL, "https://"), "http://")

	return strings.Trim(trimmed, "/")
}

// azureCloneURL builds the authenticated clone URL for an Azure repository. It exists as a
// function so the exact string can be asserted: testing azureCloneHost alone is not enough,
// because a caller can simply not use it.
//
// A configured username produces user:token credentials instead of a bare token, and turns
// on the NTLM negotiator. That is what an Azure DevOps Server behind Windows authentication
// needs: go-git sends the pair as Basic, and the negotiator upgrades it to NTLM only if the
// server answers with an NTLM challenge. Without a username nothing changes.
func azureCloneURL(platformConfig map[string]interface{}, projectKey, repoSlug string) string {
	credentials := platformConfig["AccessToken"].(string)
	if username, _ := platformConfig["Users"].(string); username != "" {
		getazure.EnableNTLM()
		credentials = url.UserPassword(username, credentials).String()
	}

	return fmt.Sprintf("%s://%s@%s/%s/%s/%s/%s",
		platformConfig["Protocol"].(string),
		credentials,
		azureCloneHost(platformConfig["Url"].(string)),
		platformConfig["Organization"].(string),
		projectKey, "_git", repoSlug)
}

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
func AnalyseReposList(DestinationResult string, platformConfig map[string]interface{}, repolist interface{}, analyseRepoFunc func(project interface{}, DestinationResult string, platformConfig map[string]interface{}, spin *spinner.Spinner, results chan int, count *atomic.Int64)) (cpt int) {
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
	// Progress counter shared across the worker goroutines. It is read and bumped
	// concurrently by performRepoAnalysis, so it must be atomic, not a plain int.
	var count atomic.Int64

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

	// Every worker reports exactly once, on success and on each error path alike, so a
	// missing report means one neither finished nor failed - it disappeared. Counting
	// them is what turns that into a visible number: returning `total` here would report
	// repositories *found* under a heading that says *analyzed*, so a repository that
	// vanished would be counted in the summary and its absence never mentioned. Silent
	// omission is the worst failure mode for a tool used to size a licence.
	analysed := 0
	for range results {
		analysed++
	}

	// Persist the skipped repositories so the ResultsAll web page and the PDF report
	// can surface them. DestinationResult is the base Results directory at this point.
	skippedList := skipped.snapshot()
	if err := utils.SaveSkippedRepos(DestinationResult, skippedList); err != nil {
		logger.Errorf("❌ Error saving skipped repositories: %v", err)
	}
	if len(skippedList) > 0 {
		logger.Warnf("⚠️  %d repository(ies) were skipped during analysis (see report for details)", len(skippedList))
	}
	if analysed < total {
		logger.Errorf("❌ %d of %d repositories did not complete analysis and are missing from the "+
			"totals, with no error of their own. The reported lines of code are therefore an "+
			"under-count - re-run before relying on them.", total-analysed, total)
	}

	return analysed
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

// platformConfigDefaults is every key the analysis reads back out of a platform's
// configuration block with an unchecked type assertion, paired with the value to
// fall back on. The types are the ones encoding/json produces — numbers decode to
// float64 and arrays to []interface{} — because the readers assert those directly.
//
// Keys that already have a defensive accessor (WorkDir, CloneTimeout) are absent on
// purpose: those readers handle a missing or mistyped value themselves, so there is
// nothing here to protect and no reason to alter their behaviour.
var platformConfigDefaults = map[string]interface{}{
	"AccessToken":       "",
	"Apiver":            "",
	"Baseapi":           "",
	"Branch":            "",
	"DevOps":            "",
	"Directory":         "",
	"FileExclusion":     "",
	"FileLoad":          "",
	"Organization":      "",
	"Project":           "",
	"Protocol":          "",
	"Repos":             "",
	"Url":               "",
	"Users":             "",
	"Workspace":         "",
	"DefaultBranch":     true,
	"Multithreading":    true,
	"Org":               true,
	"ResultAll":         true,
	"ResultByFile":      true,
	"ScanSubDirs":       true,
	"ExcludeTests":      false,
	"ExcludeVendor":     false,
	"Stats":             false,
	"Factor":            float64(33),
	"NumberWorkerRepos": float64(10),
	"Period":            float64(-1),
	"Workers":           float64(10),
	"ExcludePaths":      []interface{}{},
	"ExtExclusion":      []interface{}{},
	"FileNamePatterns":  []interface{}{},
	"FolderKeywords":    []interface{}{},
}

// resultAggregate is the cross-repository tally computed from the per-repository
// result files written by the analysis phase.
type resultAggregate struct {
	TotalCodeLines int    // sum across repositories, excluding the language left out of totals
	MaxCodeLines   int    // code lines in the largest repository
	MaxProject     string // project/organisation owning the largest repository
	MaxRepo        string // name of the largest repository
	Repositories   int    // result files parsed: one per analysed repository or directory
}

// aggregateResultFiles reads the per-repository result files in dir and tallies them.
//
// Repositories is counted once per successfully parsed file, deliberately separate
// from the largest-repository comparison further down: that branch only runs when a
// new maximum is found, so incrementing there counts how many times the running
// maximum changed rather than how many repositories were analysed.
//
// Only canonically-named result files are accepted. A file whose name cannot be
// parsed, or whose contents cannot be read or decoded, is skipped and contributes
// nothing to the totals, the repository count, or largest-repository candidacy.
func aggregateResultFiles(dir string, isFilePlatform bool) (resultAggregate, error) {
	var aggregate resultAggregate

	files, err := os.ReadDir(dir)
	if err != nil {
		return aggregate, err
	}

	for _, file := range files {
		if file.IsDir() || !strings.HasSuffix(file.Name(), ".json") {
			continue
		}

		var parsedOrg, parsedRepo string
		if isFilePlatform {
			// The file platform writes single-component Result_<Repo>.json names.
			parsedRepo = strings.TrimSuffix(strings.TrimPrefix(file.Name(), "Result_"), ".json")
		} else {
			org, repo, _, ok := utils.ParseResultFileName(file.Name())
			if !ok {
				logger.Warnf("⚠️  Skipping unparsable result file: %s", file.Name())
				continue
			}
			parsedOrg, parsedRepo = org, repo
		}

		jsonData, err := os.ReadFile(filepath.Join(dir, file.Name()))
		if err != nil {
			logger.Errorf("❌ Error reading file %s: %v\n", file.Name(), err)
			continue
		}

		var result Result
		if err := json.Unmarshal(jsonData, &result); err != nil {
			logger.Errorf("❌ Error parsing JSON contents of file %s: %v\n", file.Name(), err)
			continue
		}

		aggregate.Repositories++

		// Exclude JSON LOC from the total to match SonarQube's behaviour.
		jsonLOC := 0
		for _, r := range result.Results {
			if strings.TrimSpace(r.Language) == utils.LanguageExcludedFromTotalLOC {
				jsonLOC += r.CodeLines
				break
			}
		}
		codeLinesForTotal := result.TotalCodeLines - jsonLOC
		aggregate.TotalCodeLines += codeLinesForTotal

		// Update the (max, project, repo) triple together — the name was validated
		// above, so a new maximum always carries a usable repository identity.
		if codeLinesForTotal > aggregate.MaxCodeLines {
			aggregate.MaxCodeLines = codeLinesForTotal
			aggregate.MaxProject = parsedOrg
			aggregate.MaxRepo = parsedRepo
		}
	}

	return aggregate, nil
}

// normalizePlatformConfig substitutes a usable default for every key in
// platformConfigDefaults that the configuration omits, sets to null, or gives a type
// the readers do not expect.
//
// Without it those values reach unchecked assertions such as
// platformConfig["FileLoad"].(string), and a config file that simply omits an
// optional key crashes the run with "interface conversion: interface {} is nil, not
// string" instead of reporting anything actionable. Normalising once here keeps every
// downstream reader — including the ones in pkg/devops — safe.
//
// A missing key is expected and silent. A key present with the wrong type is a
// genuine mistake in the config file, so those names are returned for the caller to
// report.
func normalizePlatformConfig(platformConfig map[string]interface{}) []string {
	if platformConfig == nil {
		return nil
	}

	var mistyped []string
	for key, fallback := range platformConfigDefaults {
		value, present := platformConfig[key]
		if !present || value == nil {
			platformConfig[key] = fallback
			continue
		}

		var matches bool
		switch fallback.(type) {
		case string:
			_, matches = value.(string)
		case bool:
			_, matches = value.(bool)
		case float64:
			_, matches = value.(float64)
		case []interface{}:
			_, matches = value.([]interface{})
		}
		if !matches {
			mistyped = append(mistyped, key)
			platformConfig[key] = fallback
		}
	}

	sort.Strings(mistyped) // map iteration order is random; keep the message stable
	return mistyped
}

// requiredPlatformKeys returns the settings a run of this platform cannot proceed
// without, given the rest of its configuration.
//
// Defaulting an absent key (see normalizePlatformConfig) removes the panic, but on
// its own it would let a genuinely incomplete config run to a confusing end: an empty
// Organization reaching Azure DevOps surfaces only "The resource cannot be found."
// with nothing pointing at the config file. These keys are therefore reported
// explicitly instead.
//
// The list is deliberately narrow — only keys with no fallback anywhere else:
//   - GitHub resolves an empty Organization from the authenticated user when
//     analysing a personal account, so it is required only for an organisation.
//   - GitLab takes its groups from the group field and runs with Organization empty.
//   - The file platform already reports a missing directory itself, with a message
//     that also covers the FileLoad alternative.
func requiredPlatformKeys(platformConfig map[string]interface{}) []string {
	devops, _ := platformConfig["DevOps"].(string)

	switch devops {
	case "github":
		keys := []string{"Url", "AccessToken"}
		if isOrg, _ := platformConfig["Org"].(bool); isOrg {
			keys = append(keys, "Organization")
		}
		return keys
	case "gitlab":
		return []string{"Url", "AccessToken"}
	case "azure":
		return []string{"Url", "AccessToken", "Organization"}
	case "bitbucket":
		return []string{"Url", "AccessToken", "Workspace"}
	case "bitbucket_dc":
		return []string{"Url", "AccessToken"}
	default:
		return nil
	}
}

// missingRequiredKeys names the required settings that are absent or blank. Run it
// after normalizePlatformConfig, which guarantees these keys hold a string.
func missingRequiredKeys(platformConfig map[string]interface{}) []string {
	if platformConfig == nil {
		return nil
	}

	var missing []string
	for _, key := range requiredPlatformKeys(platformConfig) {
		value, _ := platformConfig[key].(string)
		if strings.TrimSpace(value) == "" {
			missing = append(missing, key)
		}
	}
	return missing
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
func analyseBitCRepo(project interface{}, DestinationResult string, platformConfig map[string]interface{}, spin *spinner.Spinner, results chan int, count *atomic.Int64) {
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
		ProjectKey:   p.ProjectKey,
		Namespace:    "",
		RepoSlug:     p.RepoSlug,
		MainBranch:   p.MainBranch,
		PathToScan:   pathToScan,
		WorkDir:      getWorkDir(platformConfig),
		CloneTimeout: getCloneTimeout(platformConfig),
	}
	performRepoAnalysis(params, DestinationResult, spin, results, count, analysisOptions{
		ExcludeExtensions: excludeExtensions,
		ExcludePaths:      excludePath,
		FolderKeywords:    folderKeywords,
		FileNamePatterns:  fileNamePatterns,
		ResultByFile:      platformConfig["ResultByFile"].(bool),
		ResultAll:         platformConfig["ResultAll"].(bool),
	})
}

// Analysis functions for Bitbucket DC
func analyseBitSRVRepo(project interface{}, DestinationResult string, platformConfig map[string]interface{}, trimmedURL string, spin *spinner.Spinner, results chan int, count *atomic.Int64) {
	p := project.(getbibucketdc.ProjectBranch)
	var excludeExtensions []string

	excludeExtensions = convertToSliceString(platformConfig["ExtExclusion"].([]interface{}))
	excludePath := getExcludePaths(platformConfig["ExcludePaths"])
	folderKeywords := getStringSliceConfig(platformConfig, "FolderKeywords")
	fileNamePatterns := getStringSliceConfig(platformConfig, "FileNamePatterns")

	params := RepoParams{
		ProjectKey:   p.ProjectKey,
		Namespace:    "",
		RepoSlug:     p.RepoSlug,
		MainBranch:   p.MainBranch,
		PathToScan:   fmt.Sprintf("%s://%s:%s@%sscm/%s/%s.git", platformConfig["Protocol"].(string), platformConfig["Users"].(string), platformConfig["AccessToken"].(string), trimmedURL, p.ProjectKey, p.RepoSlug),
		WorkDir:      getWorkDir(platformConfig),
		CloneTimeout: getCloneTimeout(platformConfig),
	}
	performRepoAnalysis(params, DestinationResult, spin, results, count, analysisOptions{
		ExcludeExtensions: excludeExtensions,
		ExcludePaths:      excludePath,
		FolderKeywords:    folderKeywords,
		FileNamePatterns:  fileNamePatterns,
		ResultByFile:      platformConfig["ResultByFile"].(bool),
		ResultAll:         platformConfig["ResultAll"].(bool),
	})
}

// Analysis functions for GitHub
func analyseGithubRepo(project interface{}, DestinationResult string, platformConfig map[string]interface{}, spin *spinner.Spinner, results chan int, count *atomic.Int64) {
	p := project.(getgithub.ProjectBranch)
	var excludeExtensions []string

	excludeExtensions = convertToSliceString(platformConfig["ExtExclusion"].([]interface{}))
	excludePath := getExcludePaths(platformConfig["ExcludePaths"])
	folderKeywords := getStringSliceConfig(platformConfig, "FolderKeywords")
	fileNamePatterns := getStringSliceConfig(platformConfig, "FileNamePatterns")

	baseapi := extractDomain(platformConfig["Baseapi"].(string))
	params := RepoParams{
		ProjectKey:   p.Org,
		Namespace:    "",
		RepoSlug:     p.RepoSlug,
		MainBranch:   p.MainBranch,
		PathToScan:   fmt.Sprintf("%s://%s:x-oauth-basic@%s/%s/%s.git", platformConfig["Protocol"].(string), platformConfig["AccessToken"].(string), baseapi, p.Org, p.RepoSlug),
		WorkDir:      getWorkDir(platformConfig),
		CloneTimeout: getCloneTimeout(platformConfig),
	}
	performRepoAnalysis(params, DestinationResult, spin, results, count, analysisOptions{
		ExcludeExtensions: excludeExtensions,
		ExcludePaths:      excludePath,
		FolderKeywords:    folderKeywords,
		FileNamePatterns:  fileNamePatterns,
		ResultByFile:      platformConfig["ResultByFile"].(bool),
		ResultAll:         platformConfig["ResultAll"].(bool),
	})
}

// Analysis functions for GitLab
func analyseGitlabRepo(project interface{}, DestinationResult string, platformConfig map[string]interface{}, spin *spinner.Spinner, results chan int, count *atomic.Int64) {
	p := project.(getgitlab.ProjectBranch)
	var excludeExtensions []string

	excludeExtensions = convertToSliceString(platformConfig["ExtExclusion"].([]interface{}))
	excludePath := getExcludePaths(platformConfig["ExcludePaths"])
	folderKeywords := getStringSliceConfig(platformConfig, "FolderKeywords")
	fileNamePatterns := getStringSliceConfig(platformConfig, "FileNamePatterns")

	domain := extractDomain(platformConfig["Url"].(string))

	params := RepoParams{
		ProjectKey:   p.Org,
		Namespace:    p.Namespace,
		RepoSlug:     p.RepoSlug,
		MainBranch:   p.MainBranch,
		PathToScan:   fmt.Sprintf("%s://gitlab-ci-token:%s@%s/%s.git", platformConfig["Protocol"].(string), platformConfig["AccessToken"].(string), domain, p.Namespace),
		WorkDir:      getWorkDir(platformConfig),
		CloneTimeout: getCloneTimeout(platformConfig),
	}
	performRepoAnalysis(params, DestinationResult, spin, results, count, analysisOptions{
		ExcludeExtensions: excludeExtensions,
		ExcludePaths:      excludePath,
		FolderKeywords:    folderKeywords,
		FileNamePatterns:  fileNamePatterns,
		ResultByFile:      platformConfig["ResultByFile"].(bool),
		ResultAll:         platformConfig["ResultAll"].(bool),
	})
}

func analyseAzurebRepo(project interface{}, DestinationResult string, platformConfig map[string]interface{}, spin *spinner.Spinner, results chan int, count *atomic.Int64) {
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
		// The host comes from the configured Url rather than a literal, so the same code
		// path serves Azure DevOps Server. For the hosted service Url is
		// https://dev.azure.com/ and this resolves to exactly what it did before.
		PathToScan:   azureCloneURL(platformConfig, p.ProjectKey, p.RepoSlug),
		WorkDir:      getWorkDir(platformConfig),
		CloneTimeout: getCloneTimeout(platformConfig),
	}
	performRepoAnalysis(params, DestinationResult, spin, results, count, analysisOptions{
		ExcludeExtensions: excludeExtensions,
		ExcludePaths:      excludePath,
		FolderKeywords:    folderKeywords,
		FileNamePatterns:  fileNamePatterns,
		ResultByFile:      platformConfig["ResultByFile"].(bool),
		ResultAll:         platformConfig["ResultAll"].(bool),
	})
}

// analysisOptions bundles the per-repo analysis knobs (exclusions, keyword/name
// filters, and the report-mode flags) so the worker functions can pass them as a
// single value instead of a long parameter list.
type analysisOptions struct {
	ExcludeExtensions []string
	ExcludePaths      []string
	FolderKeywords    []string
	FileNamePatterns  []string
	ResultByFile      bool
	ResultAll         bool
}

// Perform repository analysis (common logic)
func performRepoAnalysis(params RepoParams, DestinationResult string, spin *spinner.Spinner, results chan int, count *atomic.Int64, opts analysisOptions) {
	// Always use a consistent filename pattern so downstream parsing works across platforms.
	// Format: Result_<OrgOrProjectKey>__<RepoSlug>__<Branch>
	// The double-underscore field separator keeps `_` free to appear inside any component
	// (GitLab group names, repo slugs, branch names like feat_xyz), so the parser in
	// pkg/reporter/pdf can recover all three fields unambiguously.
	outputFileName := fmt.Sprintf("Result_%s__%s__%s", params.ProjectKey, params.RepoSlug, params.MainBranch)
	golocParams := goloc.Params{
		Path:              params.PathToScan,
		ByFile:            opts.ResultByFile,
		ByAll:             opts.ResultAll,
		ExcludePaths:      opts.ExcludePaths,
		ExcludeExtensions: opts.ExcludeExtensions,
		IncludeExtensions: []string{},
		FolderKeywords:    opts.FolderKeywords,
		FileNamePatterns:  opts.FileNamePatterns,
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
	if opts.ResultAll {
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
		count.Add(1)
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
		//count.Add(1)

		if opts.ResultAll {

			if err := gc.Run(); err != nil {
				fmt.Print("\n")
				logger.Errorf("❌ Error during analysis with ByAll = true: %v", err)
				recordSkip(fmt.Sprintf("analysis error: %v", err))
				count.Add(1)
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
				count.Add(1)
				results <- 1
				return
			}

			if err := gc.Run(); err != nil {
				fmt.Print("\n")
				logger.Errorf("❌ Error during analysis with ByFile = false: %v", err)
				recordSkip(fmt.Sprintf("analysis error: %v", err))
				count.Add(1)
				results <- 1
				return
			}
		} else {
			// If ByAll = false, just run normally
			if err := gc.Run(); err != nil {
				fmt.Print("\n")
				logger.Errorf("❌ Error during analysis: %v", err)
				recordSkip(fmt.Sprintf("analysis error: %v", err))
				count.Add(1)
				results <- 1
				return
			}
		}

		// Temp-clone cleanup is handled by the deferred RemoveAll registered above,
		// so it runs on success and on every early-return error path alike.
		golocParams.Cloned = false
		spin.Stop()
		logger.Infof("\r\t\t\t\t✅ %d The repository <%s> has been analyzed\n", count.Add(1), params.RepoSlug)
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
	return AnalyseReposList(DestinationResult, platformConfig, repoInterfaces, func(project interface{}, DestinationResult string, platformConfig map[string]interface{}, spin *spinner.Spinner, results chan int, count *atomic.Int64) {
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
	NumRepositories int                 `json:"NumRepositories"`
	ProjectBranches []fileProjectBranch `json:"ProjectBranches"`
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
// opts carries the exclusions/keyword filters and report-mode flags (ExcludePaths
// holds the file-exclusion list for the File platform).
func analyseDirectory(dir string, opts analysisOptions, destDir string, count *atomic.Int64) {
	params := goloc.Params{
		Path:              dir,
		ByFile:            opts.ResultByFile,
		ByAll:             opts.ResultAll,
		ExcludePaths:      opts.ExcludePaths,
		ExcludeExtensions: opts.ExcludeExtensions,
		IncludeExtensions: []string{},
		FolderKeywords:    opts.FolderKeywords,
		FileNamePatterns:  opts.FileNamePatterns,
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
	if opts.ResultAll {
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

	if err := runGlocPasses(gc, params, opts.ResultAll); err != nil {
		return
	}

	logger.Infof("\t✅ %d The directory <%s> has been analyzed\n", count.Add(1), dir)
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
	// Shared across the per-directory goroutines below; atomic to avoid a data race.
	var count atomic.Int64

	opts := analysisOptions{
		ExcludeExtensions: extexclusion,
		ExcludePaths:      fileexclusionEX,
		FolderKeywords:    folderKeywords,
		FileNamePatterns:  fileNamePatterns,
		ResultByFile:      ResultByFile,
		ResultAll:         ResultAll,
	}
	for _, Listdirectories := range Listdirectorie {
		go func(dir string) {
			defer wg.Done()
			analyseDirectory(dir, opts, destDir, &count)
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
	if !configVersionCompatible(AppConfig.Release.Version, version1) {
		fmt.Fprintf(os.Stderr, "\n❌ Incompatible config file: this build needs a %s.x config but found %q - Use the correct config.json file!\n",
			majorVersion(version1), AppConfig.Release.Version)
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

	// Fill in anything the config omits before the readers below assert on it, so a
	// missing optional key reports a problem instead of panicking mid-run.
	if mistyped := normalizePlatformConfig(platformConfig); len(mistyped) > 0 {
		logger.Warnf("⚠️  Config key(s) with an unexpected type, using defaults instead: %s",
			strings.Join(mistyped, ", "))
	}

	// Defaulting absent keys must not turn an incomplete config into a run that fails
	// somewhere far away with an unrelated-looking message, so say so here instead.
	if missing := missingRequiredKeys(platformConfig); len(missing) > 0 {
		logger.Errorf("❌ Configuration for platform '%s' is missing required setting(s): %s",
			platform, strings.Join(missing, ", "))
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

		startTime = time.Now()

		// GoLC analyses one branch per repository: the default branch, a branch named
		// explicitly in the configuration, or — when neither applies — the most active
		// branch. Selection happens inside the platform getter.
		repositories, err := getgithub.GetRepoGithubList(platformConfig, fileexclusionEX, false)
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

	isFilePlatform := platformConfig["DevOps"].(string) == "file"
	aggregate, err := aggregateResultFiles(DestinationResult, isFilePlatform)
	if err != nil {
		logger.Errorf("❌ Error listing files:%v", err)
		exitGolc(1)
	}

	totalCodeLinesSum := aggregate.TotalCodeLines
	maxTotalCodeLines = aggregate.MaxCodeLines
	maxProject = aggregate.MaxProject
	maxRepo = aggregate.MaxRepo
	// Every other platform already knows its repository count from the analysis
	// phase; the file platform only learns it here, from the result files.
	if isFilePlatform {
		NumberRepos = aggregate.Repositories
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

	// A repository selection made on the results page belongs to the run it was made
	// against. This run rediscovered the repositories from scratch, so carrying the
	// old selection over would silently drop repositories from the new totals — and
	// possibly name repositories this scan never saw. Clear it before the reports are
	// built so a fresh analysis always reports the whole scan.
	if err := utils.ClearDeselectedRepos(baseResultsDir); err != nil {
		logger.Errorf("❌ Error clearing repository selection: %v", err)
	}

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
