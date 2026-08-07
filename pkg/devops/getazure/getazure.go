package getazure

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/SonarSource-Demos/sonar-golc/pkg/utils"
	"github.com/briandowns/spinner"
	"github.com/microsoft/azure-devops-go-api/azuredevops/core"
	"github.com/microsoft/azure-devops-go-api/azuredevops/git"
)

type ProjectBranch struct {
	Org         string
	ProjectKey  string
	RepoSlug    string
	MainBranch  string
	LargestSize int64
}

type AzureConnect struct {
	Ctx        context.Context
	CoreClient core.Client
}

type AnalysisResult struct {
	NumRepositories int
	ProjectBranches []ProjectBranch
}

type ExclusionList struct {
	Projects map[string]bool
	Repos    map[string]bool
}

type SummaryStats struct {
	LargestRepo       string
	LargestRepoBranch string
	NbRepos           int
	EmptyRepo         int
	TotalExclude      int
	TotalArchiv       int
	TotalBranches     int
}

type AnalyzeProject struct {
	Project       core.TeamProjectReference
	AzureClient   core.Client
	Context       context.Context
	ExclusionList *ExclusionList
	Spin1         *spinner.Spinner
	Org           string
}

type ParamsProjectAzure struct {
	Client         core.Client
	Context        context.Context
	Projects       []core.TeamProjectReference
	URL            string
	AccessToken    string
	ApiURL         string
	Organization   string
	Exclusionlist  *utils.ExclusionList
	Excludeproject int
	Spin           *spinner.Spinner
	Period         int
	Stats          bool
	DefaultB       bool
	SingleRepos    string
	SingleBranch   string
}

// RepositoryMap represents a map of repositories to ignore
type ExclusionRepos map[string]bool

const PrefixMsg = "Get Project(s)..."
const MessageErro1 = "/\n❌ Failed to list projects for organization %s: %v\n"
const MessageErro2 = "/\n❌ Failed to list project for organization %s: %v\n"
const Message1 = "\t✅ The number of %s found is: %d\n"
const Message2 = "\t   Analysis top branch(es) in project <%s> ..."
const Message3 = "\r\t\t✅ %d Project: %s - Number of branches: %d - largest Branch: %s "
const Message4 = "Project(s)"
const REF = "refs/heads/"

func loadExclusionFileOrCreateNew(exclusionFile string) (*utils.ExclusionList, error) {
	if exclusionFile == "0" {
		return &utils.ExclusionList{
			Projects: make(map[string]bool),
			Repos:    make(map[string]bool),
		}, nil
	}
	return utils.LoadExclusionList(exclusionFile)
}

func isRepoExcluded(exclusionList *utils.ExclusionList, projectKey, repoKey string) bool {
	_, repoExcluded := exclusionList.Repos[projectKey+"/"+repoKey]
	return repoExcluded
}

// Fonction pour vérifier si un projet est exclu
func isProjectExcluded(exclusionList *utils.ExclusionList, projectKey string) bool {
	_, projectExcluded := exclusionList.Projects[projectKey]
	return projectExcluded
}

func isRepoEmpty(ctx context.Context, gitClient git.Client, projectID string, repoID string) (bool, error) {
	path := "/"
	items, err := gitClient.GetItems(ctx, git.GetItemsArgs{
		RepositoryId:   &repoID,
		Project:        &projectID,
		ScopePath:      &path,
		RecursionLevel: &git.VersionControlRecursionTypeValues.None,
	})
	if err != nil {
		return true, nil
	}

	branches, err := gitClient.GetBranches(ctx, git.GetBranchesArgs{
		RepositoryId: &repoID,
		Project:      &projectID,
	})
	if err != nil {
		return false, err
	}

	if len(*items) == 0 || len(*branches) == 0 {
		return true, nil
	}

	return false, nil
	//fmt.Println("ITEMS:", items)
	//return len(*items) == 0, nil
}

func getAllProjects(ctx context.Context, coreClient core.Client, exclusionList *utils.ExclusionList) ([]core.TeamProjectReference, int, error) {
	var allProjects []core.TeamProjectReference
	var excludedCount int
	var continuationToken string

	for {
		// Get the current projects page
		responseValue, err := coreClient.GetProjects(ctx, core.GetProjectsArgs{
			ContinuationToken: &continuationToken,
		})
		if err != nil {
			return nil, 0, err
		}

		for _, project := range responseValue.Value {
			if isProjectExcluded(exclusionList, *project.Name) {
				excludedCount++
				continue
			}

			allProjects = append(allProjects, project)

		}

		// Check if there is a continuation token for the next page
		if responseValue.ContinuationToken == "" {
			break
		}

		// Update the continuation token
		continuationToken = responseValue.ContinuationToken
	}

	return allProjects, excludedCount, nil
}

func getProjectByName(ctx context.Context, coreClient core.Client, projectName string, exclusionList *utils.ExclusionList) ([]core.TeamProjectReference, int, error) {

	var excludedCount int

	if isProjectExcluded(exclusionList, projectName) {
		excludedCount++
		errmessage := fmt.Sprintf(" - Skipping analysis for Project %s , it is excluded", projectName)
		err := fmt.Errorf("%s", errmessage)
		return nil, excludedCount, err
	}

	project, err := coreClient.GetProject(ctx, core.GetProjectArgs{
		ProjectId: &projectName,
	})
	if err != nil {
		return nil, 0, err
	}

	// Create a core.TeamProjectReference from core.TeamProject
	projectReference := core.TeamProjectReference{
		Id:          project.Id,
		Name:        project.Name,
		Description: project.Description,
		Url:         project.Url,
		State:       project.State,
		Revision:    project.Revision,
	}

	return []core.TeamProjectReference{projectReference}, excludedCount, nil
}

func GetRepoAzureList(platformConfig map[string]interface{}, exclusionFile string) ([]ProjectBranch, error) {

	var importantBranches []ProjectBranch
	var totalExclude, totalArchiv, emptyRepo, TotalBranches, nbRepos int
	var totalSize int64
	var largestRepoBranch, largesRepo string
	var exclusionList *utils.ExclusionList
	var err error
	loggers := utils.SharedLogger()

	ApiURL := platformConfig["Url"].(string) + platformConfig["Organization"].(string)

	loggers.Infof("🔎 Analysis of devops platform objects ...\n")

	spin := spinner.New(spinner.CharSets[35], 100*time.Millisecond)
	spin.Prefix = PrefixMsg
	spin.Color("green", "bold")
	spin.Start()

	exclusionList, err = loadExclusionFileOrCreateNew(exclusionFile)
	if err != nil {
		loggers.Errorf("\n❌ Error Read Exclusion File <%s>: %v", exclusionFile, err)
		spin.Stop()
		return nil, err
	}

	// Create a connection to your organization. A configured username switches the
	// credentials to user:token basic auth and turns on the NTLM negotiator, which is what
	// an Azure DevOps Server behind Windows authentication needs; see ntlm.go.
	username, _ := platformConfig["Users"].(string)
	connection := azureConnection(ApiURL, username, platformConfig["AccessToken"].(string))
	ctx := context.Background()

	// Create a client to interact with the Core area
	coreClient, err := core.NewClient(ctx, connection)
	if err != nil {
		log.Fatal(err)
	}

	gitClient, err := git.NewClient(ctx, connection)
	if err != nil {
		loggers.Fatalf("Error creating Git client: %v", err)
	}

	azureConnect := AzureConnect{
		Ctx:        ctx,
		CoreClient: coreClient,
	}

	/* --------------------- Analysis all projects with a default branche  ---------------------  */
	if platformConfig["Project"].(string) == "" {

		// Get All Project
		projects, exludedprojects, err := getAllProjects(ctx, coreClient, exclusionList)

		if err != nil {
			spin.Stop()
			loggers.Fatalf(MessageErro1, platformConfig["Organization"].(string), err)
		}
		spin.Stop()
		spin1 := spinner.New(spinner.CharSets[35], 100*time.Millisecond)
		spin1.Color("green", "bold")

		loggers.Infof(Message1, Message4, len(projects)+exludedprojects)

		// Set Parmams
		params := getCommonParams(azureConnect, platformConfig, projects, exclusionList, exludedprojects, spin, ApiURL)
		// Analyse Get important Branch
		importantBranches, emptyRepo, nbRepos, TotalBranches, totalExclude, totalArchiv, err = getRepoAnalyse(params, gitClient)
		if err != nil {
			spin.Stop()
			return nil, err
		}

	} else {
		projects, exludedprojects, err := getProjectByName(ctx, coreClient, platformConfig["Project"].(string), exclusionList)
		if err != nil {
			spin.Stop()
			log.Fatalf(MessageErro2, platformConfig["Organization"].(string), err)
		}

		spin.Stop()
		spin1 := spinner.New(spinner.CharSets[35], 100*time.Millisecond)
		spin1.Color("green", "bold")

		loggers.Infof(Message1, Message4, 1+exludedprojects)

		// Set Parmams
		params := getCommonParams(azureConnect, platformConfig, projects, exclusionList, exludedprojects, spin, ApiURL)
		// Analyse Get important Branch
		importantBranches, emptyRepo, nbRepos, TotalBranches, totalExclude, totalArchiv, err = getRepoAnalyse(params, gitClient)
		if err != nil {
			spin.Stop()
			return nil, err
		}
	}

	if len(importantBranches) == 1 && platformConfig["Repos"].(string) != "" {
		// If there is only one important branch and SingleRepos is set, use it directly
		if platformConfig["DefaultBranch"].(bool) {
			largestRepoBranch = importantBranches[0].MainBranch
		} else {
			largestRepoBranch = strings.TrimPrefix(importantBranches[0].MainBranch, "refs/heads/")
		}
		largesRepo = importantBranches[0].RepoSlug
	} else {
		largestRepoBranch, largesRepo = findLargestRepository(importantBranches, &totalSize)
	}

	result := AnalysisResult{
		NumRepositories: nbRepos,
		ProjectBranches: importantBranches,
	}
	if err := SaveResult(result); err != nil {
		loggers.Errorf("❌ Error Save Result of Analysis : %v", err)
		os.Exit(1)
	}

	stats := SummaryStats{
		LargestRepo:       largesRepo,
		LargestRepoBranch: largestRepoBranch,
		NbRepos:           nbRepos,
		EmptyRepo:         emptyRepo,
		TotalExclude:      totalExclude,
		TotalArchiv:       totalArchiv,
		TotalBranches:     TotalBranches,
	}

	// Persist the per-run repository breakdown for the ResultsAll page and the
	// global PDF report. Analyzed is the number of repos that will actually be
	// analyzed (one important branch per repo); Scanned is derived from the sum.
	//
	// nbRepos is the true count of discovered repos (analyzed + filtered), so any
	// repo discovered but dropped inside the analysis loop (no usable default
	// branch, or filtered out by single-branch selection) without landing in a
	// category is surfaced as Skipped, keeping Scanned equal to the real total.
	discoverySkipped := azureDiscoverySkipped(nbRepos, emptyRepo, totalExclude, totalArchiv, len(importantBranches))
	utils.PersistScanSummary("Results", utils.NewScanSummary("azure", len(importantBranches), totalArchiv, emptyRepo, totalExclude, discoverySkipped))

	printSummary(platformConfig["Organization"].(string), stats)

	return importantBranches, nil
}

func getCommonParams(azureConnect AzureConnect, platformConfig map[string]interface{}, project []core.TeamProjectReference, exclusionList *utils.ExclusionList, excludeproject int, spin *spinner.Spinner, apiURL string) ParamsProjectAzure {
	return ParamsProjectAzure{
		Client:   azureConnect.CoreClient,
		Context:  azureConnect.Ctx,
		Projects: project,

		URL:            platformConfig["Url"].(string),
		AccessToken:    platformConfig["AccessToken"].(string),
		ApiURL:         apiURL,
		Organization:   platformConfig["Organization"].(string),
		Exclusionlist:  exclusionList,
		Excludeproject: excludeproject,
		Spin:           spin,
		Period:         int(platformConfig["Period"].(float64)),
		Stats:          platformConfig["Stats"].(bool),
		DefaultB:       platformConfig["DefaultBranch"].(bool),
		SingleRepos:    platformConfig["Repos"].(string),
		SingleBranch:   platformConfig["Branch"].(string),
	}
}

func findLargestRepository(importantBranches []ProjectBranch, totalSize *int64) (string, string) {

	var largestRepoBranch, largesRepo string
	var largestRepoSize int64 = 0

	for _, branch := range importantBranches {
		if branch.LargestSize > int64(largestRepoSize) {
			largestRepoSize = branch.LargestSize
			largestRepoBranch = branch.MainBranch
			largesRepo = branch.RepoSlug

		}
		*totalSize += branch.LargestSize
	}
	return largestRepoBranch, largesRepo
}

func printSummary(Org string, stats SummaryStats) {

	loggers := utils.SharedLogger()

	loggers.Infof("✅ The largest Repository is <%s> in the organization <%s> with the branch <%s> ", stats.LargestRepo, Org, stats.LargestRepoBranch)
	loggers.Infof("✅ Total Repositories that will be analyzed: %d - Find empty : %d - Excluded : %d - Archived : %d", stats.NbRepos-stats.EmptyRepo-stats.TotalExclude-stats.TotalArchiv, stats.EmptyRepo, stats.TotalExclude, stats.TotalArchiv)
	loggers.Infof("✅ Total Branches that will be analyzed: %d\n", stats.TotalBranches)
}

func getRepoAnalyse(params ParamsProjectAzure, gitClient git.Client) ([]ProjectBranch, int, int, int, int, int, error) {

	var emptyRepos = 0
	var totalexclude = 0
	var importantBranches []ProjectBranch
	var NBRrepo, TotalBranches int
	var messageF = ""
	loggers := utils.SharedLogger()

	NBRrepos := 0
	cptarchiv := 0

	cpt := 1

	message4 := "Repo(s)"

	spin1 := spinner.New(spinner.CharSets[35], 100*time.Millisecond)
	spin1.Prefix = PrefixMsg
	spin1.Color("green", "bold")

	params.Spin.Start()
	if params.Excludeproject > 0 {
		messageF = fmt.Sprintf("\t✅ The number of project(s) to analyze is %d - Excluded : %d\n\n", len(params.Projects), params.Excludeproject)
	} else {
		messageF = fmt.Sprintf("\t✅ The number of project(s) to analyze is %d\n\n", len(params.Projects))
	}
	params.Spin.FinalMSG = messageF
	params.Spin.Stop()

	// Get Repository in each Project
	for _, project := range params.Projects {

		loggers.Infof("\t🟢  Analyse Project: %s \n", *project.Name)

		archivedCount, emptyCount, excludedCount, repos, err := listReposForProject(params, *project.Name, gitClient)

		if err != nil {
			if len(params.SingleRepos) == 0 {
				loggers.Errorf("\r❌ Get Repos for each Project:%v", err)
				spin1.Stop()
				continue
			} else {
				errmessage := fmt.Sprintf(" Get Repo %s for Project %s %v", params.SingleRepos, *project.Name, err)
				spin1.Stop()
				return importantBranches, emptyRepos, NBRrepos, TotalBranches, totalexclude, cptarchiv, fmt.Errorf("%s", errmessage)
			}
		}

		// Accumulate into the function-level counters. Using distinct local names
		// above avoids shadowing these accumulators inside the loop body.
		totalexclude += excludedCount
		emptyRepos += emptyCount
		cptarchiv += archivedCount

		spin1.Stop()
		// NBRrepo counts every repo discovered for the project (analyzed + filtered)
		// so the summary total reconciles with the per-category counts.
		NBRrepo = len(repos) + emptyCount + archivedCount + excludedCount
		if emptyCount+archivedCount+excludedCount > 0 {
			loggers.Infof("\t  ✅ The number of %s found is: %d - Empty: %d, Archived: %d, Excluded: %d\n", message4, NBRrepo, emptyCount, archivedCount, excludedCount)
		} else {
			loggers.Infof("\t  ✅ The number of %s found is: %d\n", message4, NBRrepo)
		}

		for _, repo := range repos {
			loggers.Debugf("→ repo %s/%s: analyzing", *project.Name, *repo.Name)
			largestRepoBranch, repobranches, brsize, err := analyzeRepoBranches(params, *project.Name, *repo.Name, gitClient, cpt, spin1)

			if err != nil {
				if params.SingleBranch != "" {
					// Skip this repository if SingleBranch is set but not found
					continue
				}
				branchName, ok := defaultBranchName(&repo)
				if !ok {
					// Branch analysis failed and the repo has no usable default
					// branch (e.g. disabled/archived). Skip it instead of
					// dereferencing a nil pointer and crashing the whole run.
					loggers.Errorf("\r❌ Skipping repo %s/%s: branch analysis failed and no default branch is set: %v", *project.Name, *repo.Name, err)
					continue
				}
				largestRepoBranch = branchName
			} else {
				// Check if SingleBranch is set and the returned branch is not SingleBranch
				if params.SingleBranch != "" && !params.DefaultB && largestRepoBranch != params.SingleBranch {
					// Skip this repository if the most important branch is not the SingleBranch
					continue
				}
			}

			importantBranches = append(importantBranches, ProjectBranch{
				Org:         params.Organization,
				ProjectKey:  *project.Name,
				RepoSlug:    *repo.Name,
				MainBranch:  largestRepoBranch,
				LargestSize: brsize,
			})
			TotalBranches += repobranches

			cpt++
		}

		NBRrepos += NBRrepo

	}

	return importantBranches, emptyRepos, NBRrepos, TotalBranches, totalexclude, cptarchiv, nil

}

// parseSingleRepos splits the comma-separated "Repos" configuration field into
// a clean slice of repository names, ignoring blank entries and surrounding
// whitespace (e.g. "repo1, repo2 ,repo3").
func parseSingleRepos(singleRepos string) []string {
	var list []string
	if singleRepos == "" {
		return list
	}
	for _, r := range strings.Split(singleRepos, ",") {
		if name := strings.TrimSpace(r); name != "" {
			list = append(list, name)
		}
	}
	return list
}

// azureRepoList mirrors the subset of the Azure DevOps "list repositories" REST
// response we need. The pinned SDK (v1.0.0-b5) drops isDisabled from its
// git.GitRepository struct, so disabled repositories — the ADO equivalent of an
// "archived" repo: no default branch, cannot be cloned — can only be detected
// through a raw REST call.
type azureRepoList struct {
	Value []struct {
		Id         string `json:"id"`
		IsDisabled bool   `json:"isDisabled"`
	} `json:"value"`
}

// fetchDisabledRepoIDs returns the set of disabled repository IDs for a project,
// keyed by lowercased UUID. Disabled repos must be excluded from the analysis the
// same way archived repositories are on the other platforms. On any error
// (insufficient permissions, older server, network) it logs a warning and returns
// an empty set so the scan proceeds rather than failing.
func fetchDisabledRepoIDs(parms ParamsProjectAzure, projectKey string) map[string]bool {
	loggers := utils.SharedLogger()
	disabled := make(map[string]bool)

	warn := func(reason string, args ...interface{}) {
		loggers.Warnf("⚠️  Skipping disabled-repo detection for project %s: "+reason, append([]interface{}{projectKey}, args...)...)
	}

	// Bound the call: the shared HTTP client has no request timeout, so a server
	// that accepts the connection but never responds would otherwise hang the
	// whole scan instead of degrading gracefully.
	ctx, cancel := context.WithTimeout(parms.Context, 30*time.Second)
	defer cancel()

	endpoint := fmt.Sprintf("%s/%s/_apis/git/repositories?api-version=6.0", parms.ApiURL, url.PathEscape(projectKey))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		warn("%v", err)
		return disabled
	}
	req.SetBasicAuth("", parms.AccessToken)

	resp, err := utils.HTTPClient.Do(req)
	if err != nil {
		warn("%v", err)
		return disabled
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		warn("API returned %s", resp.Status)
		return disabled
	}

	var list azureRepoList
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		warn("%v", err)
		return disabled
	}

	for _, repo := range list.Value {
		if repo.IsDisabled {
			disabled[strings.ToLower(repo.Id)] = true
		}
	}
	return disabled
}

// azureDiscoverySkipped returns the number of repositories that were discovered
// (nbRepos includes analyzed + all filtered categories) but dropped inside the
// analysis loop without being analyzed or filed under archived/empty/excluded —
// e.g. no usable default branch, or filtered out by a single-branch selection.
// Surfacing this keeps the scan summary's Scanned total equal to the real count.
func azureDiscoverySkipped(nbRepos, empty, excluded, archived, analyzed int) int {
	skipped := nbRepos - empty - excluded - archived - analyzed
	if skipped < 0 {
		return 0
	}
	return skipped
}

func listReposForProject(parms ParamsProjectAzure, projectKey string, gitClient git.Client) (int, int, int, []git.GitRepository, error) {
	var allRepos []git.GitRepository
	var archivedCount, emptyCount, excludedCount int
	loggers := utils.SharedLogger()

	// Disabled (archived-equivalent) repos are detected via a raw REST call
	// because the pinned SDK does not expose the isDisabled flag.
	disabledRepos := fetchDisabledRepoIDs(parms, projectKey)

	// Convert SingleRepos (comma-separated) to a clean slice, ignoring blank
	// entries and surrounding whitespace (e.g. "repo1, repo2 ,repo3").
	singleReposList := parseSingleRepos(parms.SingleRepos)

	// Get repositories
	loggers.Debugf("→ project %s: fetching repos from API", projectKey)
	repos, err := gitClient.GetRepositories(parms.Context, git.GetRepositoriesArgs{
		Project: &projectKey,
	})
	if err != nil {
		loggers.Errorf("Error get GetRepositories ")
		return 0, 0, 0, nil, err
	}
	loggers.Debugf("→ project %s: API returned %d repos", projectKey, len(*repos))

	for _, repo := range *repos {
		repoName := *repo.Name

		// If SingleRepos is specified, skip repositories not in the list
		if len(parms.SingleRepos) > 0 && !contains(singleReposList, repoName) {
			loggers.Debugf("→ repo %s/%s: skipped (not in single-repo filter)", projectKey, repoName)
			continue
		}

		// check if exclude
		if isRepoExcluded(parms.Exclusionlist, projectKey, repoName) {
			loggers.Debugf("→ repo %s/%s: skipped (excluded)", projectKey, repoName)
			excludedCount++
			continue
		}
		repoID := repo.Id.String()

		// Skip disabled repos (ADO equivalent of archived): they have no default
		// branch and cannot be cloned, so they must not inflate the analyzed count.
		if disabledRepos[strings.ToLower(repoID)] {
			loggers.Debugf("→ repo %s/%s: skipped (disabled/archived)", projectKey, repoName)
			archivedCount++
			continue
		}

		isEmpty, err := isRepoEmpty(parms.Context, gitClient, projectKey, repoID)

		if err != nil {
			return 0, 0, 0, nil, err
		}
		if isEmpty {
			loggers.Debugf("→ repo %s/%s: skipped (empty)", projectKey, repoName)
			emptyCount++
			continue
		}

		allRepos = append(allRepos, repo)
	}

	return archivedCount, emptyCount, excludedCount, allRepos, nil
}

// Helper function to check if a slice contains a string
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

func analyzeRepoBranches(parms ParamsProjectAzure, projectKey string, repo string, gitClient git.Client, cpt int, spin1 *spinner.Spinner) (string, int, int64, error) {

	var largestRepoBranch string
	var nbrbranch int
	var err error
	var brsize int64
	loggers := utils.SharedLogger()

	largestRepoBranch, brsize, nbrbranch, err = getMostImportantBranch(parms.Context, gitClient, projectKey, repo, parms.Period, parms.DefaultB, parms.SingleBranch)
	if err != nil {
		spin1.Stop()
		return "", 0, 1, err
	}

	spin1.Stop()

	// Print analysis summary
	loggers.Infof("\t\t✅ Repo %d: %s - Number of branches: %d - Largest Branch: %s\n", cpt, repo, nbrbranch, largestRepoBranch)

	return largestRepoBranch, nbrbranch, brsize, nil

}

// defaultBranchName safely extracts a repository's default branch name, with
// the "refs/heads/" prefix stripped. The Azure DevOps API marks DefaultBranch
// as omitempty, so GetRepository can legitimately return it as nil — for
// example for a disabled/archived repository, or one whose HEAD ref was never
// initialized. Dereferencing it unconditionally previously crashed the whole
// analysis with "panic: runtime error: invalid memory address or nil pointer
// dereference". The bool result reports whether a usable default branch exists.
func defaultBranchName(repo *git.GitRepository) (string, bool) {
	if repo == nil || repo.DefaultBranch == nil || *repo.DefaultBranch == "" {
		return "", false
	}
	return strings.TrimPrefix(*repo.DefaultBranch, REF), true
}

func getMostImportantBranch(ctx context.Context, gitClient git.Client, projectID string, repoID string, periode int, DefaultB bool, Singlebranch string) (string, int64, int, error) {

	since := time.Now().AddDate(0, periode, 0)
	sinceStr := since.Format(time.RFC3339)

	// Get default branch
	repo, err := gitClient.GetRepository(ctx, git.GetRepositoryArgs{
		RepositoryId: &repoID,
		Project:      &projectID,
	})
	if err != nil {
		return "", 0, 0, err
	}

	branchName, hasDefault := defaultBranchName(repo)

	switch {
	case DefaultB && hasDefault:
		// Default-branch mode with a known default branch: analyze it directly.
		return handleDefaultOrSingleBranch(ctx, gitClient, projectID, repoID, branchName, "", sinceStr)
	case Singlebranch != "":
		return handleDefaultOrSingleBranch(ctx, gitClient, projectID, repoID, "", Singlebranch, sinceStr)
	default:
		// Either we are not in default-branch mode, or the repository has no
		// default branch set (e.g. a disabled/archived repo, or one whose HEAD
		// ref was never initialized). Instead of skipping it, scan every branch
		// and pick the most active one. branchName is the fallback default and
		// may be empty when none is set.
		return handleNonDefaultBranch(ctx, gitClient, projectID, repoID, sinceStr, branchName)
	}
}
func handleNonDefaultBranch(ctx context.Context, gitClient git.Client, projectID string, repoID string, sinceStr string, defaultBranch string) (string, int64, int, error) {

	var mostImportantBranch string
	var maxCommits int
	var totalCommitSize int64

	branches, err := gitClient.GetBranches(ctx, git.GetBranchesArgs{
		RepositoryId: &repoID,
		Project:      &projectID,
	})
	if err != nil {
		return "", 0, 0, err
	}

	// firstBranch is the ultimate fallback: the first branch with a usable name.
	// It keeps the repository in the analysis when no branch has commits in the
	// window and the repo has no default branch set.
	var firstBranch string
	for _, branch := range *branches {
		if branch.Name == nil {
			continue
		}
		name := strings.TrimPrefix(*branch.Name, REF)
		if firstBranch == "" {
			firstBranch = name
		}

		commitCount, branchCommitSize, err := getCommitDetails(ctx, gitClient, projectID, repoID, *branch.Name, sinceStr)
		if err != nil {
			return "", 0, 0, err
		}

		if commitCount > maxCommits {
			maxCommits = commitCount
			mostImportantBranch = name
			totalCommitSize = branchCommitSize
		}
	}

	if maxCommits == 0 {
		// No branch had commits in the analysis window. Prefer the repo's
		// default branch when known, otherwise fall back to the first branch so
		// the repository is still analyzed rather than dropped.
		if trimmed := strings.TrimPrefix(defaultBranch, REF); trimmed != "" {
			mostImportantBranch = trimmed
		} else {
			mostImportantBranch = firstBranch
		}
	}

	if mostImportantBranch == "" {
		return "", 0, 0, fmt.Errorf("repository %s has no analyzable branch", repoID)
	}

	return mostImportantBranch, totalCommitSize, len(*branches), nil
}

func handleDefaultOrSingleBranch(ctx context.Context, gitClient git.Client, projectID string, repoID string, defaultBranch string, singleBranch string, sinceStr string) (string, int64, int, error) {

	var branchName string

	if defaultBranch != "" {
		branchName = defaultBranch
	} else {
		// Vérifier si Singlebranch existe dans les branches
		branches, err := gitClient.GetBranches(ctx, git.GetBranchesArgs{
			RepositoryId: &repoID,
			Project:      &projectID,
		})
		if err != nil {
			return "", 0, 0, err
		}
		branchExists := false
		for _, branch := range *branches {
			if strings.TrimPrefix(*branch.Name, REF) == singleBranch {
				branchExists = true
				break
			}
		}
		if !branchExists {
			return "", 0, 0, fmt.Errorf("branch %s not found in repository %s", singleBranch, repoID)
		}
		branchName = singleBranch
	}

	commitCount, commitSize, err := getCommitDetails(ctx, gitClient, projectID, repoID, branchName, sinceStr)
	if err != nil {
		return "", 0, 0, err
	}

	if commitCount == 0 {
		repo, err := gitClient.GetRepository(ctx, git.GetRepositoryArgs{
			RepositoryId: &repoID,
			Project:      &projectID,
		})
		if err != nil {
			return "", 0, 0, err
		}
		// Size is an omitempty pointer and may be nil for some repositories.
		if repo.Size != nil {
			commitSize = int64(*repo.Size)
		}
	}

	return branchName, commitSize, 1, nil
}

func getCommitDetails(ctx context.Context, gitClient git.Client, projectID string, repoID string, branchName string, sinceStr string) (int, int64, error) {
	commitCount, err := getCommitCount(ctx, gitClient, projectID, repoID, branchName, sinceStr)
	if err != nil {
		return 0, 0, err
	}

	commitSize := int64(commitCount)
	return commitCount, commitSize, nil
}

func getCommitCount(ctx context.Context, gitClient git.Client, projectID string, repoID string, branchName string, sinceStr string) (int, error) {
	totalCommits := 0
	pages := 100
	skip := 0

	for {
		searchCriteria := git.GitQueryCommitsCriteria{
			ItemVersion: &git.GitVersionDescriptor{
				Version:        &branchName,
				VersionType:    &git.GitVersionTypeValues.Branch,
				VersionOptions: &git.GitVersionOptionsValues.None,
			},
			FromDate: &sinceStr,
			Top:      &pages,
			Skip:     &skip,
		}

		commits, err := gitClient.GetCommits(ctx, git.GetCommitsArgs{
			RepositoryId:   &repoID,
			Project:        &projectID,
			SearchCriteria: &searchCriteria,
		})
		if err != nil {
			return 0, err
		}

		totalCommits += len(*commits)

		if len(*commits) < pages {
			break
		}

		skip += pages
	}

	return totalCommits, nil
}
func SaveResult(result AnalysisResult) error {

	loggers := utils.SharedLogger()
	// Open or create the file
	file, err := os.Create("Results/config/analysis_result_azure.json")
	if err != nil {
		loggers.Errorf("❌ Error creating Analysis file: %v", err)
		return err
	}
	defer file.Close()

	// Create a JSON encoder
	encoder := json.NewEncoder(file)

	// Encode the result and write it to the file
	if err := encoder.Encode(result); err != nil {
		loggers.Errorf("❌ Error encoding JSON file <Results/config/analysis_result_azure.json> : %v", err)
		return err
	}

	fmt.Print("\n")
	loggers.Infof("✅ Result saved successfully!\n")
	return nil
}
