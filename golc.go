//go:build golc
// +build golc

package main

import (
	"archive/zip"
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/SonarSource-Demos/sonar-golc/assets"
	"github.com/SonarSource-Demos/sonar-golc/pkg/goloc"
	"github.com/briandowns/spinner"

	"github.com/SonarSource-Demos/sonar-golc/pkg/devops/getazure"
	getbibucket "github.com/SonarSource-Demos/sonar-golc/pkg/devops/getbitbucket/v2"
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
	Logging   LoggingConfig          `json:"logging"`
	Release   ReleaseConfig          `json:"release"`
}

type LoggingConfig struct {
	Level logrus.Level `json:"level"`
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
	ProjectKey string
	Namespace  string
	RepoSlug   string
	MainBranch string
	PathToScan string
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
var version1 = "1.0.9"

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
	messageF := ""
	spin.FinalMSG = messageF

	// Create a channel to receive results
	results := make(chan int)
	count := 1

	if platformConfig["Multithreading"].(bool) {
		if len(repolist.([]interface{})) > int(platformConfig["NumberWorkerRepos"].(float64)) {
			// Launch goroutines in batches of X
			X := int(platformConfig["Workers"].(float64))
			batches := len(repolist.([]interface{})) / X
			remainder := len(repolist.([]interface{})) % X
			for i := 0; i < batches; i++ {
				for j := i * X; j < (i+1)*X; j++ {
					go analyseRepoFunc(repolist.([]interface{})[j], DestinationResult, platformConfig, spin, results, &count)
				}
				waitForWorkers(X, results)
			}
			// Launch remaining goroutines
			for i := batches * X; i < batches*X+remainder; i++ {
				go analyseRepoFunc(repolist.([]interface{})[i], DestinationResult, platformConfig, spin, results, &count)
			}
			waitForWorkers(remainder, results)
		} else {
			// Launch goroutines for each repo
			for _, project := range repolist.([]interface{}) {
				go analyseRepoFunc(project, DestinationResult, platformConfig, spin, results, &count)
			}
			waitForWorkers(len(repolist.([]interface{})), results)
		}
	} else {
		// Without multithreading
		for _, project := range repolist.([]interface{}) {
			// Execute the analysis synchronously
			analyseRepoFunc(project, DestinationResult, platformConfig, spin, results, &count)
		}
	}

	return len(repolist.([]interface{}))
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

// Analysis functions for different repository types

// Analysis functions for Bitbucket Cloud
func analyseBitCRepo(project interface{}, DestinationResult string, platformConfig map[string]interface{}, spin *spinner.Spinner, results chan int, count *int) {
	p := project.(getbibucket.ProjectBranch)
	var excludeExtensions []string

	excludeExtensions = convertToSliceString(platformConfig["ExtExclusion"].([]interface{}))
	excludePath := getExcludePaths(platformConfig["ExcludePaths"])

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
	}
	performRepoAnalysis(params, DestinationResult, spin, results, count, excludeExtensions, excludePath, platformConfig["ResultByFile"].(bool), platformConfig["ResultAll"].(bool))
}

// Analysis functions for Bitbucket DC
func analyseBitSRVRepo(project interface{}, DestinationResult string, platformConfig map[string]interface{}, trimmedURL string, spin *spinner.Spinner, results chan int, count *int) {
	p := project.(getbibucketdc.ProjectBranch)
	var excludeExtensions []string

	excludeExtensions = convertToSliceString(platformConfig["ExtExclusion"].([]interface{}))
	excludePath := getExcludePaths(platformConfig["ExcludePaths"])

	params := RepoParams{
		ProjectKey: p.ProjectKey,
		Namespace:  "",
		RepoSlug:   p.RepoSlug,
		MainBranch: p.MainBranch,
		PathToScan: fmt.Sprintf("%s://%s:%s@%sscm/%s/%s.git", platformConfig["Protocol"].(string), platformConfig["Users"].(string), platformConfig["AccessToken"].(string), trimmedURL, p.ProjectKey, p.RepoSlug),
	}
	performRepoAnalysis(params, DestinationResult, spin, results, count, excludeExtensions, excludePath, platformConfig["ResultByFile"].(bool), platformConfig["ResultAll"].(bool))
}

// Analysis functions for GitHub
func analyseGithubRepo(project interface{}, DestinationResult string, platformConfig map[string]interface{}, spin *spinner.Spinner, results chan int, count *int) {
	p := project.(getgithub.ProjectBranch)
	var excludeExtensions []string

	excludeExtensions = convertToSliceString(platformConfig["ExtExclusion"].([]interface{}))
	excludePath := getExcludePaths(platformConfig["ExcludePaths"])

	params := RepoParams{
		ProjectKey: p.Org,
		Namespace:  "",
		RepoSlug:   p.RepoSlug,
		MainBranch: p.MainBranch,
		PathToScan: fmt.Sprintf("%s://%s:x-oauth-basic@%s/%s/%s.git", platformConfig["Protocol"].(string), platformConfig["AccessToken"].(string), platformConfig["Baseapi"].(string), p.Org, p.RepoSlug),
	}
	performRepoAnalysis(params, DestinationResult, spin, results, count, excludeExtensions, excludePath, platformConfig["ResultByFile"].(bool), platformConfig["ResultAll"].(bool))
}

// Analysis functions for GitLab
func analyseGitlabRepo(project interface{}, DestinationResult string, platformConfig map[string]interface{}, spin *spinner.Spinner, results chan int, count *int) {
	p := project.(getgitlab.ProjectBranch)
	var excludeExtensions []string

	excludeExtensions = convertToSliceString(platformConfig["ExtExclusion"].([]interface{}))
	excludePath := getExcludePaths(platformConfig["ExcludePaths"])

	domain := extractDomain(platformConfig["Url"].(string))

	params := RepoParams{
		ProjectKey: p.Org,
		Namespace:  p.Namespace,
		RepoSlug:   p.RepoSlug,
		MainBranch: p.MainBranch,
		PathToScan: fmt.Sprintf("%s://gitlab-ci-token:%s@%s/%s.git", platformConfig["Protocol"].(string), platformConfig["AccessToken"].(string), domain, p.Namespace),
	}
	performRepoAnalysis(params, DestinationResult, spin, results, count, excludeExtensions, excludePath, platformConfig["ResultByFile"].(bool), platformConfig["ResultAll"].(bool))
}

func analyseAzurebRepo(project interface{}, DestinationResult string, platformConfig map[string]interface{}, spin *spinner.Spinner, results chan int, count *int) {
	p := project.(getazure.ProjectBranch)
	var excludeExtensions []string

	excludeExtensions = convertToSliceString(platformConfig["ExtExclusion"].([]interface{}))
	excludePath := getExcludePaths(platformConfig["ExcludePaths"])

	params := RepoParams{
		ProjectKey: p.ProjectKey,
		Namespace:  "",
		RepoSlug:   p.RepoSlug,
		MainBranch: p.MainBranch,
		PathToScan: fmt.Sprintf("%s://%s@%s/%s/%s/%s/%s", platformConfig["Protocol"].(string), platformConfig["AccessToken"].(string), "dev.azure.com", platformConfig["Organization"].(string), p.ProjectKey, "_git", p.RepoSlug),
	}
	performRepoAnalysis(params, DestinationResult, spin, results, count, excludeExtensions, excludePath, platformConfig["ResultByFile"].(bool), platformConfig["ResultAll"].(bool))
}

// Perform repository analysis (common logic)
func performRepoAnalysis(params RepoParams, DestinationResult string, spin *spinner.Spinner, results chan int, count *int, excludeExtension []string, excludePaths []string, ResultByFile bool, ResultAll bool) {
	// Always use a consistent filename pattern so downstream parsing works across platforms
	// Format: Result_<OrgOrProjectKey>_<RepoSlug>_<Branch>
	outputFileName := fmt.Sprintf("Result_%s_%s_%s", params.ProjectKey, params.RepoSlug, params.MainBranch)
	golocParams := goloc.Params{
		Path:         params.PathToScan,
		ByFile:       ResultByFile,
		ByAll:        ResultAll,
		ExcludePaths: excludePaths,
		//ExcludePaths:      []string{},
		ExcludeExtensions: excludeExtension,
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
		Branch:            params.MainBranch,
		Cloned:            false,
		Repopath:          "",
	}
	if ResultAll {
		golocParams.ByFile = true
	}
	MessB := fmt.Sprintf("   Extracting files from repo : %s ", params.RepoSlug)
	spin.Suffix = MessB
	spin.Start()

	gc, err := goloc.NewGCloc(golocParams, assets.Languages)
	if err != nil {
		logger.Errorf(errorMessageRepo+"%v", err)
		*count++
		results <- 1
		return
	} else {

		//gc.Run()
		//*count++

		if ResultAll {

			if err := gc.Run(); err != nil {
				fmt.Print("\n")
				logger.Errorf("❌ Error during analysis with ByAll = true: %v", err)
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
				*count++
				results <- 1
				return
			}

			if err := gc.Run(); err != nil {
				fmt.Print("\n")
				logger.Errorf("❌ Error during analysis with ByFile = false: %v", err)
				*count++
				results <- 1
				return
			}
		} else {
			// If ByAll = false, just run normally
			if err := gc.Run(); err != nil {
				fmt.Print("\n")
				logger.Errorf("❌ Error during analysis: %v", err)
				*count++
				results <- 1
				return
			}
		}

		// Remove repository directory only if it is a temp clone, not the user's source directory
		if gc.RepopathDisposable && gc.Repopath != "" {
			err1 := os.RemoveAll(gc.Repopath)
			if err1 != nil {
				logger.Errorf(errorMessageDi, err1)
			}
		}
		golocParams.Cloned = false
		spin.Stop()
		logger.Infof("\r\t\t\t\t✅ %d The repository <%s> has been analyzed\n", *count, params.RepoSlug)
		// Send result through channel
		results <- 1
	}
}

// Wait for all goroutines to complete
func waitForWorkers(numWorkers int, results chan int) {
	for i := 0; i < numWorkers; i++ {
		fmt.Printf("\r Waiting for workers...\n")
		<-results
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

/* ---------------- Analyse Directory ---------------- */

func AnalyseReposListFile(Listdirectorie, fileexclusionEX []string, extexclusion []string, ResultByFile bool, ResultAll bool) {

	type Configuration struct {
		ExcludeExtensions []string
	}

	//fmt.Print("\n🔎 Analysis of Directories ...\n")
	logger.Infof("🔎 Analysis of Directories ...\n")

	var wg sync.WaitGroup
	wg.Add(len(Listdirectorie))
	count := 1

	for _, Listdirectories := range Listdirectorie {
		go func(dir string) {
			defer wg.Done()

			//fmt.Println("Rep:", Listdirectories)

			spin := spinner.New(spinner.CharSets[35], 100*time.Millisecond)
			spin.Color("green", "bold")
			messageF := ""
			spin.FinalMSG = messageF

			outputFileName := "Result_"

			params := goloc.Params{
				Path:              dir,
				ByFile:            ResultByFile,
				ByAll:             ResultAll,
				ExcludePaths:      fileexclusionEX,
				ExcludeExtensions: extexclusion,
				IncludeExtensions: []string{},
				OrderByLang:       false,
				OrderByFile:       false,
				OrderByCode:       false,
				OrderByLine:       false,
				OrderByBlank:      false,
				OrderByComment:    false,
				Order:             "DESC",
				OutputName:        outputFileName,
				OutputPath:        "Results",
				ReportFormats:     []string{"json"},
				Branch:            "",
				Token:             "",
				Cloned:            false,
				Repopath:          "",
			}

			gc, err := goloc.NewGCloc(params, assets.Languages)
			if err != nil {
				logger.Errorf(errorMessageRepo+"%v", err)
				return
			} else {

				if ResultAll {

					if err := gc.Run(); err != nil {
						fmt.Print("\n")
						logger.Errorf("❌ Error during analysis with ByAll = true: %v", err)

						return
					}

					// Second call to Run with ByFile = false
					params.ByFile = true

					params.Cloned = false
					params.Repopath = gc.Repopath
					params.RepopathDisposable = gc.RepopathDisposable

					gc, err = goloc.NewGCloc(params, assets.Languages)
					if err != nil {
						fmt.Print("\n")
						logger.Errorf("❌ Error initializing GCloc for ByFile = false: %v", err)
						return
					}

					if err := gc.Run(); err != nil {
						fmt.Print("\n")
						logger.Errorf("❌ Error during analysis with ByFile = false: %v", err)
						return
					}
				} else {
					// If ByAll = false, just run normally
					if err := gc.Run(); err != nil {
						fmt.Print("\n")
						logger.Errorf("❌ Error during analysis: %v", err)
						return
					}
				}

			}

			//gc.Run()
			spin.Stop()
			logger.Infof("\t✅ %d The directory <%s> has been analyzed\n", count, dir)
			count++
		}(Listdirectories)

	}

	wg.Wait()
}

/* ---------------- End Analyse Directory ---------------- */

func AnalyseRun(params goloc.Params, reponame string) {
	gc, err := goloc.NewGCloc(params, assets.Languages)
	if err != nil {
		fmt.Println(errorMessageRepo, err)
		os.Exit(1)
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
		os.Exit(1)
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

func init() {

	// Load Config file (path from GOLC_CONFIG_FILE env, default config.json)
	configPath := os.Getenv("GOLC_CONFIG_FILE")
	if configPath == "" {
		configPath = "config.json"
	}
	var err error
	AppConfig, err = LoadConfig(configPath)
	if err != nil {
		logrus.Fatalf("\n❌ Failed to load config: %s", err)
		os.Exit(1)
	}

	if AppConfig.Release.Version != version1 {
		logrus.Fatalf("\n❌ Version mismatch: expected %s but got %s - Use the correct config.json file !", version1, AppConfig.Release.Version)
		os.Exit(1)
	}

	logrus.Info("✅ Configuration loaded successfully and version matched!")

	// Create Logs Directory
	logDir := "Logs"
	if _, err := os.Stat(logDir); os.IsNotExist(err) {
		err = os.MkdirAll(logDir, 0755)
		if err != nil {
			logrus.Fatalf("❌ Failed to create log directory: %v", err)
		}
	}
	// Remove Log file
	if err := os.Remove("Logs/Logs.log"); err != nil && !os.IsNotExist(err) {
		logrus.Fatalf("❌ Failed to delete old log file: %v", err)
	}

	// Set Loggin
	// Create a new logger instance

	logger = utils.NewLogger()
	logger.SetLevel(AppConfig.Logging.Level)
}

// ApplicationFlags holds command line arguments
type ApplicationFlags struct {
	DevOps      string
	Fast        bool
	AllBranches bool
	Help        bool
	Languages   bool
	Version     bool
}

// parseAndValidateFlags processes command line arguments and validates them
func parseAndValidateFlags() (ApplicationFlags, map[string]interface{}) {
	// Define flags
	devopsFlag := flag.String("devops", "", "Specify the DevOps platform")
	fastFlag := flag.Bool("fast", false, "Enable fast mode (only for Github)")
	allBranchesFlag := flag.Bool("all-branches", false, "Analyze all branches for each repository (not just main branch)")
	helpFlag := flag.Bool("help", false, "Show help message")
	languagesFlag := flag.Bool("languages", false, "Show all supported languages")
	versionflag := flag.Bool("version", false, "Show version")

	flag.Parse()

	// Handle informational flags
	if *helpFlag {
		fmt.Println("Usage: golc -devops [OPTIONS]")
		fmt.Println("Options:  <BitBucketSRV>||<BitBucket>||<Github>||<Gitlab>||<Azure>||<File>")
		fmt.Println("")
		fmt.Println("Examples:")
		fmt.Println("  golc -devops Github                    # Analyze main branches only")
		fmt.Println("  golc -devops Github -all-branches      # Analyze ALL branches")
		fmt.Println("  golc -devops Github -fast              # Fast analysis mode")
		flag.PrintDefaults()
		os.Exit(0)
	}

	if *languagesFlag {
		displayLanguages()
		os.Exit(0)
	}

	if *versionflag {
		fmt.Printf("GoLC version: %s %s/%s\n", version1, runtime.GOOS, runtime.GOARCH)
		os.Exit(0)
	}

	// Validate required flags
	if *devopsFlag == "" {
		fmt.Println("\n❌ Please specify the DevOps platform using the -devops flag : <BitBucketSRV>||<BitBucket>||<Github>||<GithubEnterprise>||<Gitlab>||<Azure>||<File>")
		fmt.Println("✅ Example for BitBucket server : golc -devops BitBucketSRV")
		os.Exit(1)
	}

	platformConfig, ok := AppConfig.Platforms[*devopsFlag].(map[string]interface{})
	if !ok {
		fmt.Printf("\n❌ Configuration for DevOps platform '%s' not found\n", *devopsFlag)
		fmt.Println("✅ the -devops flag is : <BitBucketSRV>||<BitBucket>||<Github>||<GithubEnterprise>||<Gitlab>||<Azure>||<File>")
		os.Exit(1)
	}

	return ApplicationFlags{
		DevOps:      *devopsFlag,
		Fast:        *fastFlag,
		AllBranches: *allBranchesFlag,
		Help:        *helpFlag,
		Languages:   *languagesFlag,
		Version:     *versionflag,
	}, platformConfig
}

// setupResultsDirectory handles Results directory creation and backup logic
func setupResultsDirectory(flags ApplicationFlags) string {
	pwd, err := os.Getwd()
	if err != nil {
		fmt.Println("Error:", err)
	}
	DestinationResult := pwd + "/Results"

	logger.Infof("✅ Using configuration for DevOps platform '%s'\n", flags.DevOps)

	_, err = os.Stat(DestinationResult)
	if err == nil {
		fmt.Printf("❗️ Directory <'%s'> already exists. Do you want to delete it? (y/n): ", DestinationResult)
		var response string
		fmt.Scanln(&response)

		if response == "y" || response == "Y" {
			fmt.Printf("❗️ Do you want to create a backup of the directory before deleting? (y/n): ")
			var backupResponse string
			fmt.Scanln(&backupResponse)

			if backupResponse == "y" || backupResponse == "Y" {
				if err := createBackup(DestinationResult, pwd); err != nil {
					fmt.Printf("❌ Error creating backup: %s\n", err)
					os.Exit(1)
				}
			}

			if err := os.RemoveAll(DestinationResult); err != nil {
				fmt.Printf("❌ Error deleting directory: %s\n", err)
				os.Exit(1)
			}
			if err := os.MkdirAll(DestinationResult, os.ModePerm); err != nil {
				panic(err)
			}
			createDirectories(DestinationResult, directoriesToCreate)
		} else {
			os.Exit(1)
		}
	} else if os.IsNotExist(err) {
		if err := os.MkdirAll(DestinationResult, os.ModePerm); err != nil {
			panic(err)
		}
		createDirectories(DestinationResult, directoriesToCreate)
	}

	return DestinationResult
}

func main() {
	var maxTotalCodeLines int
	var maxProject, maxRepo string
	var NumberRepos int
	var startTime time.Time
	var ListDirectory []string
	var ListExclusion []string
	var message0, message1, message2, message3, message4, message5 string

	// Parse and validate command line flags
	flags, platformConfig := parseAndValidateFlags()

	// Setup results directory
	DestinationResult := setupResultsDirectory(flags)
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
			os.Exit(1)

		} else {

			NumberRepos = AnalyseReposListAzure(DestinationResult, platformConfig, gitproject)

		}

	case "github":

		var fileexclusion = ".cloc_github_ignore"
		fileexclusionEX := getFileNameIfExists(fileexclusion)
		var fast bool

		startTime = time.Now()

		if flags.Fast {
			fmt.Println("🚀 Fast mode enabled for Github")
			fast = true
			err := getgithub.FastAnalys(platformConfig, fileexclusionEX)

			if err != nil {
				logger.Errorf("❌ Quick scan Analysis : '%s'", err)
				os.Exit(0)
			}
		} else {
			fast = false

			if flags.AllBranches {
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
					os.Exit(1)
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
					os.Exit(1)

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
			os.Exit(1)

		} else {
			//os.Exit(1)
			NumberRepos = AnalyseReposListGitlab(DestinationResult, platformConfig, gitproject)

		}

	case "bitbucket_dc":

		var fileexclusion = platformConfig["FileExclusion"].(string)
		fileexclusionEX := getFileNameIfExists(fileexclusion)

		startTime = time.Now()
		projects, err := getbibucketdc.GetProjectBitbucketList(platformConfig, fileexclusionEX)
		if err != nil {
			logger.Errorf("❌ Error Get Info Projects in Bitbucket server '%s' : ", err)
			os.Exit(1)
		}

		if len(projects) == 0 {
			logger.Error(errorMessageAnalyse)
			os.Exit(1)

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
			os.Exit(1)

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
				os.Exit(1)
			}
		} else {
			ListExclusion = make([]string, 0)

		}

		if fileload != "0" {
			ListDirectory, err = ReadLines(fileload)
			if err != nil {
				logger.Errorf("❌ Error reading file <.cloc_file_load>:%v", err)
				os.Exit(1)
			}
			if len(ListDirectory) == 0 {
				ListDirectory = append(ListDirectory, platformConfig["Directory"].(string))
			}
		} else {
			if len(platformConfig["Directory"].(string)) == 0 {
				logger.Error("❌ No analysis possible, no directory, specified file or specified loading file")
				os.Exit(1)
			} else {

				ListDirectory = append(ListDirectory, platformConfig["Directory"].(string))
			}
		}
		startTime = time.Now()
		AnalyseReposListFile(ListDirectory, ListExclusion, excludeExtensions, platformConfig["ResultByFile"].(bool), platformConfig["ResultAll"].(bool))
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
		os.Exit(1)
	}

	// Initialize the sum of TotalCodeLines (excluding JSON to match SonarQube behavior)
	totalCodeLinesSum := 0

	// Analyse All file
	for _, file := range files {
		// Check if the file is a JSON file
		if !file.IsDir() && strings.HasSuffix(file.Name(), ".json") {
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

			// Check if this repo has a higher TotalCodeLines (excl. JSON) than the current maximum
			if codeLinesForTotal > maxTotalCodeLines {
				maxTotalCodeLines = codeLinesForTotal
				// Extract project and repo name from file name
				parts := strings.Split(strings.TrimSuffix(file.Name(), ".json"), "_")
				if platformConfig["DevOps"].(string) != "file" {
					maxProject = parts[1]
					maxRepo = parts[2]
				} else {
					maxProject = ""
					maxRepo = parts[1]
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
		logger.Error("  ❌ There is definitely a problem, 0 lines of code are reported ???")
		fmt.Println("\n --------------------------------------------------------------------")
		os.Exit(1)
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
		log.Fatalf("❌ Error creating global report: %v", err)
	}

	// Generate Repository Summary Reports
	// Ensure we pass the base Results directory to generate summary reports,
	// since it creates outputs under <Results>/byfile-report/*.
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

	/*if platformConfig["ResultByFile"].(bool) {
		logger.Infof(" ℹ️  To generate and visualize results on a web interface, follow these steps: ")
		logger.Infof("\t✅ run : ResultByfiles")
	} else {*/

	logger.Infof(" ℹ️  To generate and visualize results on a web interface, follow these steps: ")
	logger.Infof("\t✅ run : ResultsAll")
	//}
	//fmt.Println("\nℹ️  To generate and visualize results on a web interface, follow these steps: ")
	//fmt.Println("\t✅ run : ResultsAll")

}
