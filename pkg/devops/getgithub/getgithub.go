package getgithub

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/SonarSource-Demos/sonar-golc/assets"
	"github.com/SonarSource-Demos/sonar-golc/pkg/utils"
	"github.com/briandowns/spinner"
	"github.com/google/go-github/v82/github"
)

type ExclusionList struct {
	Repos map[string]bool `json:"repos"`
}
type PlatformConfig struct {
	Organization string
	URL          string
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

// RepositoryMap represents a map of repositories to ignore
type ExclusionRepos map[string]bool

type ParamsReposGithub struct {
	Repos         []*github.Repository
	URL           string
	BaseAPI       string
	Apiver        string
	AccessToken   string
	Organization  string
	NBRepos       int
	ExclusionList ExclusionRepos
	Spin          *spinner.Spinner
	Branch        string
	Period        int
	Stats         bool
	DefaultB      bool
}
type Repository struct {
	ID            int    `json:"id"`
	Name          string `json:"name"`
	Path          string `json:"full_name"`
	SizeR         int64  `json:"size"`
	Language      string `json:"language"`
	DefaultBranch string `json:"default_branch"`
	Archived      bool   `json:"archived"`
	LOC           map[string]int
}

type ProjectBranch struct {
	Org         string
	RepoSlug    string
	MainBranch  string
	LargestSize int64
}

type AnalysisResult struct {
	NumRepositories int
	ProjectBranches []ProjectBranch
}

type TreeItem struct {
	Path string `json:"path"`
	Mode string `json:"mode"`
	Type string `json:"type"`
	Sha  string `json:"sha"`
	Size int    `json:"size,omitempty"`
}

type TreeResponse struct {
	Sha       string     `json:"sha"`
	Url       string     `json:"url"`
	Tree      []TreeItem `json:"tree"`
	Truncated bool       `json:"truncated"`
}

type Branch struct {
	Name      string     `json:"name"`
	Commit    CommitInfo `json:"commit"`
	Protected bool       `json:"protected"`
}

type CommitInfo struct {
	Sha string `json:"sha"`
	URL string `json:"url"`
}

type Lastanalyse struct {
	LastRepos  string
	LastBranch string
}

type RepoBranch struct {
	ID       int64            `json:"id"`
	Name     string           `json:"name"`
	Branches []*github.Branch `json:"branches"`
}

type LanguageInfo1 struct {
	Language  string
	CodeLines int
}

const PrefixMsg = "Get Repo(s)..."
const MessageApiRate = "❗️ Rate limit exceeded. Waiting for rate limit reset..."
const ApiHeader1 = "application/vnd.github.v3+json"
const ErrorMesssage1 = "❌ Error saving repositories in file Results/config/analysis_repos_github.json: %v\n"

// maxSecondaryRateLimitRetries bounds how many times a single call is retried
// after a GitHub secondary ("abuse") rate-limit error, so a persistently failing
// call can never loop forever.
const maxSecondaryRateLimitRetries = 5

// defaultSecondaryRateLimitWait is used when GitHub returns a secondary rate-limit
// error without a usable Retry-After hint.
const defaultSecondaryRateLimitWait = 60 * time.Second

// withRateLimitSleep enriches ctx so the go-github client transparently sleeps
// until the PRIMARY (hourly) rate limit resets and then retries, instead of
// returning a *github.RateLimitError. This covers both go-github's pre-emptive
// short-circuit ("API rate limit ... still exceeded ... not making remote request")
// and a live 403 from the API — the failure mode that previously caused every
// repository processed after the quota was exhausted to be silently skipped.
func withRateLimitSleep(ctx context.Context) context.Context {
	return context.WithValue(ctx, github.SleepUntilPrimaryRateLimitResetWhenRateLimited, true)
}

// secondaryRateLimitPause reports how long to wait before retrying after a GitHub
// secondary ("abuse") rate-limit error, and whether the error is in fact such a
// limit. Primary rate limits are handled transparently by the client via
// withRateLimitSleep, so callers only need this for the secondary case.
func secondaryRateLimitPause(err error) (time.Duration, bool) {
	var abuseErr *github.AbuseRateLimitError
	if !errors.As(err, &abuseErr) {
		return 0, false
	}
	wait := abuseErr.GetRetryAfter()
	if wait <= 0 {
		wait = defaultSecondaryRateLimitWait
	}
	return wait, true
}

// repoIsEmpty reports whether a repository has no content to analyze.
//
// repo.GetSize() (kilobytes, from the listing response) is a zero-cost fast path:
// any repo with size > 0 definitely has content. GitHub computes size
// asynchronously, though, so a repository that *does* contain commits can
// transiently report size 0 — right after creation/import, or on GitHub
// Enterprise Server where size recalculation lags. Treating size 0 as
// unconditionally empty would therefore silently drop such repos (the same
// symptom this change set out to remove). So size 0 is only a *candidate*: it is
// confirmed with a single lightweight ListCommits call, which is guarded by the
// client's primary rate-limit sleep and a secondary-limit retry. If the check
// cannot be completed, we err on the side of "not empty" so the repo is analyzed
// rather than dropped.
func repoIsEmpty(ctx context.Context, client *github.Client, repo *github.Repository, org string) bool {
	if repo.GetSize() > 0 {
		return false
	}
	repoName := repo.GetName()
	opt := &github.CommitsListOptions{ListOptions: github.ListOptions{PerPage: 1}}
	for attempt := 0; ; attempt++ {
		commits, _, err := client.Repositories.ListCommits(ctx, org, repoName, opt)
		if err == nil {
			return len(commits) == 0
		}
		if wait, ok := secondaryRateLimitPause(err); ok && attempt < maxSecondaryRateLimitRetries {
			time.Sleep(wait)
			continue
		}
		// "Git Repository is empty." is GitHub's explicit signal for a repo with no
		// commits. Any other error is inconclusive, so treat the repo as non-empty
		// and let normal analysis proceed rather than dropping it as empty.
		var ghErr *github.ErrorResponse
		if errors.As(err, &ghErr) && ghErr.Message == "Git Repository is empty." {
			return true
		}
		utils.SharedLogger().Debugf("→ repo %s: size-0 empty-check inconclusive (%v); treating as non-empty", repoName, err)
		return false
	}
}

// Load repository ignore map from file
func loadExclusionRepos1(filename string) (ExclusionRepos, error) {
	ignoreMap := make(ExclusionRepos)

	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		repoName := strings.TrimSpace(scanner.Text())
		if repoName != "" {
			ignoreMap[repoName] = true
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return ignoreMap, nil
}

// Check if a repository should be ignored
func shouldIgnore(repoName string, ignoreMap ExclusionRepos) bool {
	_, ignored := ignoreMap[repoName]
	return ignored
}

func SaveResult(result AnalysisResult) error {
	// Open or create the file
	file, err := os.Create("Results/config/analysis_result_github.json")
	if err != nil {
		fmt.Println("❌ Error creating Analysis file:", err)
		return err
	}
	defer file.Close()

	// Create a JSON encoder
	encoder := json.NewEncoder(file)

	// Encode the result and write it to the file
	if err := encoder.Encode(result); err != nil {
		fmt.Println("❌ Error encoding JSON file <Results/config/analysis_result_github.json> :", err)
		return err
	}

	fmt.Println("✅ Result saved successfully!")
	return nil
}

func SaveBranch(branch RepoBranch) error {
	// Open or create the file
	file, err := os.Create("Results/config/analysis_branch_github.json")
	if err != nil {
		fmt.Println("❌ Error creating Analysis Branch file:", err)
		return err
	}
	defer file.Close()

	// Create a JSON encoder
	encoder := json.NewEncoder(file)

	// Encode the Branch and write it to the file
	if err := encoder.Encode(branch); err != nil {
		fmt.Println("❌ Error encoding JSON file <Results/config/analysis_branch_github.json> :", err)
		return err
	}

	//	fmt.Println("✅ Branch saved successfully!")
	return nil
}

func SaveCommit(repos []*github.RepositoryCommit) error {
	// Open or create the file
	file, err := os.Create("Results/config/analysis_commit_github.json")
	if err != nil {
		fmt.Println("❌ Error creating Analysis Repos file:", err)
		return err
	}
	defer file.Close()

	// Create a JSON encoder
	encoder := json.NewEncoder(file)

	// Encode the Branch and write it to the file
	if err := encoder.Encode(repos); err != nil {
		fmt.Println("❌ Error encoding JSON file <Results/config/analysis_commit_github.json> :", err)
		return err
	}

	//fmt.Println("✅ Commits saved successfully!")
	return nil
}
func SaveRepos(repos []*github.Repository) error {
	// Open or create the file
	file, err := os.Create("Results/config/analysis_repos_github.json")
	if err != nil {
		fmt.Println("❌ Error creating Analysis Repos file:", err)
		return err
	}
	defer file.Close()

	// Create a JSON encoder
	encoder := json.NewEncoder(file)

	// Encode the Branch and write it to the file
	if err := encoder.Encode(repos); err != nil {
		fmt.Println("❌ Error encoding JSON file <Results/config/analysis_repos_github.json> :", err)
		return err
	}

	fmt.Println("✅ \r Repos saved successfully!")
	return nil
}

func SaveLast(last Lastanalyse) error {
	// Open or create the file
	file, err := os.Create("Results/config/analysis_last_github.json")
	if err != nil {
		fmt.Println("❌ Error creating Analysis Last file:", err)
		return err
	}
	defer file.Close()

	// Create a JSON encoder
	encoder := json.NewEncoder(file)

	// Encode the Branch and write it to the file
	if err := encoder.Encode(last); err != nil {
		fmt.Println("❌ Error encoding JSON file <Results/config/analysis_last_github.json> :", err)
		return err
	}

	fmt.Println("✅ Last saved successfully!")
	return nil
}

func GetReposGithub(parms ParamsReposGithub, ctx context.Context, client *github.Client) ([]ProjectBranch, int, int, int, int, int) {
	var TotalBranches, notAnalyzedCount, emptyRepo, cpt, cptarchiv int
	var importantBranches []ProjectBranch
	cpt = 1
	loggers := utils.SharedLogger()

	spin1 := spinner.New(spinner.CharSets[35], 100*time.Millisecond)
	spin1.Color("green", "bold")

	message4 := "Repo(s)"
	loggers.Infof("\t  ✅ The number of %s found is: %d\n", message4, parms.NBRepos)

	for _, repo := range parms.Repos {
		repoName := *repo.Name
		if repo.GetArchived() {
			loggers.Debugf("→ repo %s: skipped (archived)", repoName)
			cptarchiv++
			continue
		}
		if len(parms.ExclusionList) != 0 && shouldIgnore(repoName, parms.ExclusionList) {
			loggers.Infof("\t   ✅ Skipping analysis for repository '%s' as per ignore list.\n", repoName)
			notAnalyzedCount++
			continue
		}
		// Emptiness uses repo size (already in the listing response) as a zero-cost
		// fast path, confirming the size-0 candidates with a single ListCommits so
		// a size-not-yet-computed repo is not silently dropped. See repoIsEmpty.
		if repoIsEmpty(ctx, client, repo, parms.Organization) {
			loggers.Infof("\t   ✅ Skipping repository '%s' — detected as empty", repoName)
			emptyRepo++
		} else {
			loggers.Debugf("→ repo %s: analyzing", repoName)
			largestRepoBranch, repoBranches := analyzeRepoBranches(parms, ctx, client, repo, cpt, spin1)
			importantBranches = append(importantBranches, ProjectBranch{
				Org:         parms.Organization,
				RepoSlug:    repoName,
				MainBranch:  largestRepoBranch,
				LargestSize: int64(len(repoBranches)),
			})
			TotalBranches += len(repoBranches)
		}
		cpt++
	}

	result := AnalysisResult{
		NumRepositories: parms.NBRepos,
		ProjectBranches: importantBranches,
	}
	if err := SaveResult(result); err != nil {
		loggers.Errorf("❌ Error Save Result of Analysis : %v", err)
		os.Exit(1)
	}

	return importantBranches, emptyRepo, parms.NBRepos, TotalBranches, notAnalyzedCount, cptarchiv
}

func analyzeRepoBranches(parms ParamsReposGithub, ctx context.Context, client *github.Client, repo *github.Repository, cpt int, spin1 *spinner.Spinner) (string, []*github.Branch) {
	var branches []*github.Branch
	loggers := utils.SharedLogger()

	opt := &github.BranchListOptions{
		ListOptions: github.ListOptions{PerPage: 100},
	}

	messageB := fmt.Sprintf("\t   Analysis top branch(es) in repository <%s> ...", *repo.Name)
	spin1.Prefix = messageB
	spin1.Start()

	var largestRepoBranch string
	var err error
	var nbrbranche int

	if parms.DefaultB {
		// If DefaultBranch is true, retrieve the default branch of the repository
		branch, _, _ := client.Repositories.GetBranch(ctx, parms.Organization, *repo.Name, *repo.DefaultBranch, 0)
		branches = append(branches, branch)
		largestRepoBranch = *repo.DefaultBranch
		nbrbranche = 1

	} else if len(parms.Branch) != 0 {
		// If branch name is provided in params, try to get information about the specified branch
		branch, _, err := client.Repositories.GetBranch(ctx, parms.Organization, *repo.Name, parms.Branch, 0)
		if err == nil {
			// If branch exists, use it
			largestRepoBranch = parms.Branch
			branches = append(branches, branch)
			nbrbranche = len(branches)
		} else {
			// If branch does not exist, use default branch
			branches, err = getAllBranches(ctx, client, *repo.Name, parms.Organization, opt)
			if err != nil {
				//fmt.Printf("❌ Error when retrieving branches for repo %v: %v\n", *repo.Name, err)
				loggers.Errorf("❌ Error when retrieving branches for repo %v: %v\n", *repo.Name, err)
				spin1.Stop()
				return "", nil
			}
			largestRepoBranch = *repo.DefaultBranch
			nbrbranche = len(branches)
		}
	} else {
		// If DefaultBranch is false and branch name is not provided, get all branches
		branches, err = getAllBranches(ctx, client, *repo.Name, parms.Organization, opt)
		if err != nil {
			loggers.Errorf("❌ Error when retrieving branches for repo %v: %v\n", *repo.Name, err)
			spin1.Stop()
			return "", nil
		}
		largestRepoBranch = *repo.DefaultBranch
		nbrbranche = len(branches)
	}

	spin1.Stop()

	loggers.Infof("\r\t\t\t\t✅ %d Repo: %s - Targeted branches: %d - largest Branch: %s ", cpt, *repo.Name, nbrbranche, largestRepoBranch)

	return largestRepoBranch, branches
}

func getAllBranches(ctx context.Context, client *github.Client, repoName, organization string, opt *github.BranchListOptions) ([]*github.Branch, error) {
	var branches []*github.Branch
	secondaryRetries := 0
	for {
		branchPage, resp, err := client.Repositories.ListBranches(ctx, organization, repoName, opt)
		if err != nil {
			// Primary rate limits are waited out transparently by the client
			// (see withRateLimitSleep); only secondary limits surface here.
			if wait, ok := secondaryRateLimitPause(err); ok && secondaryRetries < maxSecondaryRateLimitRetries {
				fmt.Println(MessageApiRate)
				secondaryRetries++
				time.Sleep(wait)
				continue
			}
			return nil, err
		}
		secondaryRetries = 0
		branches = append(branches, branchPage...)
		if resp.NextPage == 0 {
			break
		}
		opt.Page = resp.NextPage
	}
	return branches, nil
}

// Get Infos for all Repositories in Organization

// GetRepoGithubListAllBranches retrieves ALL branches for ALL repositories in the organization

// getAllRepositories fetches all repositories from GitHub organization
func getAllRepositories(client *github.Client, ctx context.Context, orgName string) ([]*github.Repository, error) {
	opt := &github.RepositoryListByOrgOptions{
		Type:        "all",
		ListOptions: github.ListOptions{PerPage: 100},
	}

	var allRepos []*github.Repository
	for {
		repos, resp, err := client.Repositories.ListByOrg(ctx, orgName, opt)
		if err != nil {
			return nil, fmt.Errorf("error listing repositories: %v", err)
		}
		allRepos = append(allRepos, repos...)
		if resp.NextPage == 0 {
			break
		}
		opt.Page = resp.NextPage
	}
	return allRepos, nil
}

// processRepositoryBranches processes a single repository to get all its branches
func processRepositoryBranches(client *github.Client, ctx context.Context, repo *github.Repository, orgName, exclusionfile string, stats *RepoProcessingStats) ([]ProjectBranch, error) {
	var branches []ProjectBranch

	if repo.GetArchived() {
		stats.TotalArchiv++
		return branches, nil
	}

	repoName := repo.GetName()

	// Skip if in exclusion list
	if exclusionfile != "0" {
		exclusionList, err := loadExclusionList(exclusionfile)
		if err == nil && exclusionList.Repos[repoName] {
			stats.TotalExclude++
			return branches, nil
		}
	}

	// Check if repo is empty (size 0 is only a candidate — confirm it so a repo
	// whose size has not yet been computed is not dropped; see repoIsEmpty).
	if repoIsEmpty(ctx, client, repo, orgName) {
		stats.EmptyRepo++
		return branches, nil
	}

	// Get ALL branches for this repository
	branchOpt := &github.BranchListOptions{
		ListOptions: github.ListOptions{PerPage: 100},
	}

	var repoBranches []*github.Branch
	for {
		branchList, resp, err := client.Repositories.ListBranches(ctx, orgName, repoName, branchOpt)
		if err != nil {
			return branches, fmt.Errorf("error getting branches for repo %s: %v", repoName, err)
		}
		repoBranches = append(repoBranches, branchList...)
		if resp.NextPage == 0 {
			break
		}
		branchOpt.Page = resp.NextPage
	}

	// Create a ProjectBranch entry for EACH branch
	for _, branch := range repoBranches {
		branches = append(branches, ProjectBranch{
			Org:         orgName,
			RepoSlug:    repoName,
			MainBranch:  branch.GetName(),
			LargestSize: int64(repo.GetSize()),
		})
	}

	stats.TotalBranches += len(repoBranches)
	stats.NbRepos++
	stats.TotalSize += int64(repo.GetSize())

	return branches, nil
}

// RepoProcessingStats holds statistics during repository processing
type RepoProcessingStats struct {
	TotalSize     int64
	TotalExclude  int
	TotalArchiv   int
	EmptyRepo     int
	TotalBranches int
	NbRepos       int
}

func GetRepoGithubListAllBranches(platformConfig map[string]interface{}, exclusionfile string, fast bool) ([]ProjectBranch, error) {
	loggers := utils.SharedLogger()
	stats := &RepoProcessingStats{}

	client := github.NewClient(nil).WithAuthToken(platformConfig["AccessToken"].(string))
	ctx := withRateLimitSleep(context.Background())
	orgName := platformConfig["Organization"].(string)

	// Get all repositories first
	allRepos, err := getAllRepositories(client, ctx, orgName)
	if err != nil {
		return nil, err
	}

	spin1 := spinner.New(spinner.CharSets[35], 100*time.Millisecond)
	spin1.Color("green", "bold")

	var allBranches []ProjectBranch

	// Process each repository to get ALL its branches
	for i, repo := range allRepos {
		messageB := fmt.Sprintf("   🌿 Getting all branches for repo %d/%d: %s ", i+1, len(allRepos), repo.GetName())
		spin1.Suffix = messageB
		spin1.Start()

		branches, err := processRepositoryBranches(client, ctx, repo, orgName, exclusionfile, stats)
		if err != nil {
			loggers.Errorf("❌ Error processing repo %s: %v", repo.GetName(), err)
			spin1.Stop()
			continue
		}

		allBranches = append(allBranches, branches...)

		spin1.Stop()
		if len(branches) > 0 {
			loggers.Infof("\r\t\t\t\t✅ %d Repo: %s - Found %d branches", i+1, repo.GetName(), len(branches))
		}
	}

	// Find largest repo info
	largesRepo, largestRepoBranch := findLargestRepository(allBranches, &stats.TotalSize)

	// Create analysis result file
	result := AnalysisResult{
		NumRepositories: stats.NbRepos,
		ProjectBranches: allBranches,
	}
	if err := SaveResult(result); err != nil {
		loggers.Errorf("❌ Error Save Result of Analysis : %v", err)
		return nil, err
	}

	// Save summary statistics (unused but here for completeness)
	_ = SummaryStats{
		LargestRepo:       largesRepo,
		LargestRepoBranch: largestRepoBranch,
		NbRepos:           stats.NbRepos,
		EmptyRepo:         stats.EmptyRepo,
		TotalExclude:      stats.TotalExclude,
		TotalArchiv:       stats.TotalArchiv,
		TotalBranches:     stats.TotalBranches,
	}

	// In all-branches mode NbRepos is incremented once per analyzed repo, so it is
	// already the analyzed count (empty/archived/excluded are tracked separately).
	saveGithubScanSummary(stats.NbRepos, stats.TotalArchiv, stats.EmptyRepo, stats.TotalExclude)

	loggers.Infof("✅ Analysis completed:")
	loggers.Infof("   - Repositories analyzed: %d", stats.NbRepos)
	loggers.Infof("   - Total branches found: %d", stats.TotalBranches)
	loggers.Infof("   - Branches to be analyzed: %d", len(allBranches))
	loggers.Infof("   - Empty repositories skipped: %d", stats.EmptyRepo)
	loggers.Infof("   - Archived repositories skipped: %d", stats.TotalArchiv)
	loggers.Infof("   - Excluded repositories: %d", stats.TotalExclude)

	return allBranches, nil
}

func GetRepoGithubList(platformConfig map[string]interface{}, exclusionfile string, fast bool) ([]ProjectBranch, error) {
	//var largestRepoSize int64
	var totalSize int64
	var totalExclude, totalArchiv, emptyRepo, TotalBranches, nbRepos int
	var largestRepoBranch, largesRepo string
	var importantBranches []ProjectBranch
	var repositories []*github.Repository
	var exclusionList ExclusionRepos
	var err1 error
	loggers := utils.SharedLogger()

	opt := &github.RepositoryListByOrgOptions{
		ListOptions: github.ListOptions{PerPage: 100},
	}
	opt1 := &github.RepositoryListByAuthenticatedUserOptions{
		ListOptions: github.ListOptions{PerPage: 100},
		Affiliation: "owner",
	}

	//fmt.Print("\n🔎 Analysis of devops platform objects ...\n")
	loggers.Infof("🔎 Analysis of devops platform objects ...\n")

	spin := spinner.New(spinner.CharSets[35], 100*time.Millisecond)
	spin.Prefix = PrefixMsg
	spin.Color("green", "bold")
	spin.Start()

	exclusionList, err1 = loadExclusionFile(exclusionfile, spin)
	if err1 != nil {
		return nil, err1
	}

	ctx, client := initializeGithubClient(platformConfig)

	// Use comma-ok assertions: a config.json (or hand-edited config) that omits
	// these keys must yield a clear error path, not a nil-interface panic (issue #81).
	reposCfg, _ := platformConfig["Repos"].(string)
	orgFlag, _ := platformConfig["Org"].(bool)
	orgName, _ := platformConfig["Organization"].(string)
	if len(reposCfg) == 0 {
		if orgFlag {

			repositories, err1 = fetchAllRepositories(ctx, client, orgName, opt)
		} else {
			repositories, err1 = fetchUserRepositories(ctx, client, opt1)
			// Personal account: the per-repo API calls (ListCommits, ListBranches,
			// ListRepositoryEvents) need an owner. If the user did not fill the
			// Organization field (it does not apply to personal accounts), resolve
			// it from the authenticated user so downstream calls don't 404.
			if err1 == nil && strings.TrimSpace(orgName) == "" {
				user, _, uerr := client.Users.Get(ctx, "")
				if uerr != nil {
					loggers.Errorf("❌ Failed to resolve authenticated user for personal-account analysis: %v", uerr)
					return importantBranches, uerr
				}
				platformConfig["Organization"] = user.GetLogin()
				loggers.Infof("👤 Personal account detected — using authenticated user '%s' as repository owner", user.GetLogin())
			}
		}
	} else {
		repositories, err1 = fetchSpecificRepositories(ctx, client, platformConfig)
	}

	if err1 != nil {
		return importantBranches, nil
	}

	params := getCommonParams(platformConfig, repositories, exclusionList, spin)
	sortRepositoriesByUpdatedAt(repositories)

	if err := SaveRepos(repositories); err != nil {
		loggers.Errorf(ErrorMesssage1, err)
	}

	importantBranches, emptyRepo, nbRepos, TotalBranches, totalExclude, totalArchiv = GetReposGithub(params, ctx, client)

	largestRepoBranch, largesRepo = findLargestRepository(importantBranches, &totalSize)

	config := PlatformConfig{
		Organization: platformConfig["Organization"].(string),
		URL:          platformConfig["Url"].(string),
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

	// Here NbRepos is the total discovered, so the analyzed count is the remainder
	// after removing empty/excluded/archived (matches the printed summary line).
	saveGithubScanSummary(nbRepos-emptyRepo-totalExclude-totalArchiv, totalArchiv, emptyRepo, totalExclude)

	printSummary(config, stats)

	return importantBranches, nil
}

// loadExclusionList loads exclusion list for the new all-branches function
func loadExclusionList(exclusionfile string) (ExclusionList, error) {
	var exclusionList ExclusionList

	if exclusionfile == "0" {
		exclusionList.Repos = make(map[string]bool)
		return exclusionList, nil
	}

	exclusionRepos, err := loadExclusionRepos1(exclusionfile)
	if err != nil {
		return exclusionList, err
	}

	exclusionList.Repos = exclusionRepos
	return exclusionList, nil
}

func loadExclusionFile(exclusionfile string, spin *spinner.Spinner) (ExclusionRepos, error) {
	var exclusionList ExclusionRepos
	var err error
	loggers := utils.SharedLogger()

	if exclusionfile == "0" {
		exclusionList = make(map[string]bool)
	} else {
		exclusionList, err = loadExclusionRepos1(exclusionfile)
		if err != nil {
			loggers.Errorf("\n❌ Error Read Exclusion File <%s>: %v", exclusionfile, err)
			spin.Stop()
			return nil, err
		}
	}
	return exclusionList, nil
}

func initializeGithubClient(platformConfig map[string]interface{}) (context.Context, *github.Client) {
	ctx := withRateLimitSleep(context.Background())
	accessToken := platformConfig["AccessToken"].(string)
	url := platformConfig["Url"].(string)

	// Check if this is GitHub Enterprise Server (not GitHub cloud)
	if url != "https://api.github.com/" && url != "https://api.github.com" {
		// This URL covers two distinct GitHub variants that share the same config
		// shape: classic GitHub Enterprise Server (self-hosted, API at
		// "<host>/api/v3/") and GitHub Enterprise Cloud with data residency
		// (API at the bare host "https://api.<subdomain>.ghe.com/", no "/api/v3"
		// segment). Deliberately do NOT append "/api/v3/" ourselves here:
		// go-github's own WithEnterpriseURLs already adds it for a classic GHES
		// host, but skips it whenever the host starts with "api." (or contains
		// ".api.") — which is exactly the data-residency shape. Pre-appending it
		// would defeat that check and send data-residency tenants to a 404.
		baseURL := url
		if !strings.HasSuffix(baseURL, "/") {
			baseURL += "/"
		}

		// Create client for GitHub Enterprise Server / GHE.com data residency
		client, err := github.NewClient(nil).WithAuthToken(accessToken).WithEnterpriseURLs(baseURL, baseURL)
		if err != nil {
			loggers := utils.SharedLogger()
			loggers.Errorf("❌ Failed to create GitHub Enterprise client: %v", err)
			// Fallback to regular client
			client = github.NewClient(nil).WithAuthToken(accessToken)
		}
		return ctx, client
	}

	// GitHub Cloud (default)
	client := github.NewClient(nil).WithAuthToken(accessToken)
	return ctx, client
}

func fetchUserRepositories(ctx context.Context, client *github.Client, opt *github.RepositoryListByAuthenticatedUserOptions) ([]*github.Repository, error) {
	var repositories []*github.Repository

	for {
		repos, resp, err := client.Repositories.ListByAuthenticatedUser(ctx, opt)
		if err != nil {
			return nil, err
		}
		repositories = append(repositories, repos...)
		if resp.NextPage == 0 {
			break
		}
		opt.Page = resp.NextPage
	}
	return repositories, nil
}

func fetchAllRepositories(ctx context.Context, client *github.Client, organization string, opt *github.RepositoryListByOrgOptions) ([]*github.Repository, error) {
	var repositories []*github.Repository
	loggers := utils.SharedLogger()
	for {
		repos, resp, err := client.Repositories.ListByOrg(ctx, organization, opt)
		if err != nil {
			loggers.Errorf("❌ Error fetching repositories: %v\n", err)
			return nil, err
		}
		repositories = append(repositories, repos...)
		if resp.NextPage == 0 {
			break
		}
		opt.Page = resp.NextPage
	}
	return repositories, nil
}

// parseRepoList splits the comma-separated "Repos" configuration field into a
// clean, de-duplicated list of repository names, ignoring blank entries and
// surrounding whitespace (e.g. "repo1, repo2 ,repo3").
func parseRepoList(repos string) []string {
	var list []string
	seen := make(map[string]bool)
	for _, r := range strings.Split(repos, ",") {
		name := strings.TrimSpace(r)
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		list = append(list, name)
	}
	return list
}

// fetchSpecificRepositories fetches the explicit repositories named in the
// (comma-separated) "Repos" configuration field. Repositories that cannot be
// fetched (e.g. typo, missing access) are logged and skipped so that one bad
// name does not abort the whole analysis.
func fetchSpecificRepositories(ctx context.Context, client *github.Client, platformConfig map[string]interface{}) ([]*github.Repository, error) {
	loggers := utils.SharedLogger()
	organization := platformConfig["Organization"].(string)
	names := parseRepoList(platformConfig["Repos"].(string))

	var repositories []*github.Repository
	for _, name := range names {
		repos, _, err := client.Repositories.Get(ctx, organization, name)
		if err != nil {
			loggers.Errorf("❌ Error fetching repository <%s>: %v\n", name, err)
			continue
		}
		repositories = append(repositories, repos)
	}

	if len(repositories) == 0 {
		return nil, fmt.Errorf("none of the requested repositories could be fetched: %s", platformConfig["Repos"].(string))
	}
	return repositories, nil
}

func getCommonParams(platformConfig map[string]interface{}, repositories []*github.Repository, exclusionList ExclusionRepos, spin *spinner.Spinner) ParamsReposGithub {
	return ParamsReposGithub{
		Repos:         repositories,
		URL:           platformConfig["Url"].(string),
		BaseAPI:       platformConfig["Baseapi"].(string),
		Apiver:        platformConfig["Apiver"].(string),
		AccessToken:   platformConfig["AccessToken"].(string),
		Organization:  platformConfig["Organization"].(string),
		NBRepos:       len(repositories),
		ExclusionList: exclusionList,
		Spin:          spin,
		Branch:        platformConfig["Branch"].(string),
		Period:        int(platformConfig["Period"].(float64)),
		Stats:         platformConfig["Stats"].(bool),
		DefaultB:      platformConfig["DefaultBranch"].(bool),
	}
}

func findLargestRepository(importantBranches []ProjectBranch, totalSize *int64) (string, string) {
	var largestRepoSize int64
	var largestRepoBranch, largesRepo string

	for _, branch := range importantBranches {
		if branch.LargestSize > largestRepoSize {
			largestRepoSize = branch.LargestSize
			largestRepoBranch = branch.MainBranch
			largesRepo = branch.RepoSlug
		}
		*totalSize += branch.LargestSize
	}
	//return largestRepoSize, largestRepoBranch, largesRepo
	return largestRepoBranch, largesRepo
}

// saveGithubScanSummary persists the per-run repository breakdown for the
// ResultsAll page and the global PDF report. Analyzed is the number of repos that
// will actually be analyzed; Scanned is derived from the sum of all categories.
func saveGithubScanSummary(analyzed, archived, empty, excluded int) {
	utils.PersistScanSummary("Results", utils.NewScanSummary("github", analyzed, archived, empty, excluded, 0))
}

func printSummary(config PlatformConfig, stats SummaryStats) {
	loggers := utils.SharedLogger()

	fmt.Printf("\n")
	loggers.Infof("✅ The largest Repository is <%s> in the organization <%s> with the branch <%s> ", stats.LargestRepo, config.Organization, stats.LargestRepoBranch)
	loggers.Infof("✅ Total Repositories that will be analyzed: %d - Find empty : %d - Excluded : %d - Archived : %d", stats.NbRepos-stats.EmptyRepo-stats.TotalExclude-stats.TotalArchiv, stats.EmptyRepo, stats.TotalExclude, stats.TotalArchiv)
	loggers.Infof("✅ Total Branches that will be analyzed: %d\n", stats.TotalBranches)
}

// func FastAnalys(url, baseapi, apiver, accessToken, organization, exlusionfile, repos, branchmain string, period int) error {
func FastAnalys(platformConfig map[string]interface{}, exlusionfile string) error {

	var totalExclude int
	var totalArchiv int
	var repositories []*github.Repository
	var exclusionList ExclusionRepos
	var err1 error
	var emptyRepo int
	nbRepos := 0
	loggers := utils.SharedLogger()
	opt := &github.RepositoryListByOrgOptions{
		ListOptions: github.ListOptions{PerPage: 100},
	} // Number Object by page in API Request

	//fmt.Print("\n🔎 Analysis of devops platform objects ...\n")
	loggers.Infof("🔎 Analysis of devops platform objects ...\n")

	spin := spinner.New(spinner.CharSets[35], 100*time.Millisecond)
	spin.Prefix = PrefixMsg
	spin.Color("green", "bold")
	spin.Start()

	// Test if exclusion file exist
	if exlusionfile == "0" {
		exclusionList = make(map[string]bool)

	} else {
		exclusionList, err1 = loadExclusionRepos1(exlusionfile)
		if err1 != nil {
			loggers.Errorf("❌ Error Read Exclusion File <%s>: %v", exlusionfile, err1)
			spin.Stop()
			//return nil, err1
		}

	}

	if len(platformConfig["Repos"].(string)) == 0 {

		ctx, client := initializeGithubClient(platformConfig)

		// Get all Repositories in Organization
		for {
			repos, resp, err := client.Repositories.ListByOrg(ctx, platformConfig["Organization"].(string), opt)

			if err != nil {
				loggers.Errorf("❌ Error fetching repositories: %v\n", err)
				//return importantBranches, nil
			}

			repositories = append(repositories, repos...)

			if resp.NextPage == 0 {
				break
			}
			opt.Page = resp.NextPage

		}

		parms := ParamsReposGithub{
			Repos:         repositories,
			URL:           platformConfig["Url"].(string),
			BaseAPI:       platformConfig["Baseapi"].(string),
			Apiver:        platformConfig["Apiver"].(string),
			AccessToken:   platformConfig["AccessToken"].(string),
			Organization:  platformConfig["Organization"].(string),
			NBRepos:       len(repositories),
			ExclusionList: exclusionList,
			Spin:          spin,
			Branch:        platformConfig["Branch"].(string),
			Period:        int(platformConfig["Period"].(float64)),
			Stats:         platformConfig["Stats"].(bool),
		}

		sortRepositoriesByUpdatedAt(repositories)

		// Save List of Repos
		err := SaveRepos(repositories)
		if err != nil {
			loggers.Errorf(ErrorMesssage1, err)
		}

		nbRepos, emptyRepo, totalExclude, totalArchiv, err = GetGithubLanguages(parms, ctx, client, int(platformConfig["Factor"].(float64)))
		if err != nil {
			return err
		}

	} else {

		ctx, client := initializeGithubClient(platformConfig)

		reposSlice, err := fetchSpecificRepositories(ctx, client, platformConfig)
		if err != nil {
			loggers.Errorf("❌ Error fetching repository: %v\n", err)
		}

		parms := ParamsReposGithub{
			Repos:         reposSlice,
			URL:           platformConfig["Url"].(string),
			BaseAPI:       platformConfig["Baseapi"].(string),
			Apiver:        platformConfig["Apiver"].(string),
			AccessToken:   platformConfig["AccessToken"].(string),
			Organization:  platformConfig["Organization"].(string),
			NBRepos:       len(reposSlice),
			ExclusionList: exclusionList,
			Spin:          spin,
			Branch:        platformConfig["Branch"].(string),
			Period:        int(platformConfig["Period"].(float64)),
			Stats:         platformConfig["Stats"].(bool),
		}
		nbRepos, emptyRepo, totalExclude, totalArchiv, err = GetGithubLanguages(parms, ctx, client, int(platformConfig["Factor"].(float64)))
		if err != nil {
			return err
		}

	}

	//fmt.Printf("\r✅ Total Repositories that will be analyzed: %d - Find empty : %d - Excluded : %d - Archived : %d\n", nbRepos-emptyRepo-totalExclude-totalArchiv, emptyRepo, totalExclude, totalArchiv)
	loggers.Infof("\r✅ Total Repositories that will be analyzed: %d - Find empty : %d - Excluded : %d - Archived : %d\n", nbRepos-emptyRepo-totalExclude-totalArchiv, emptyRepo, totalExclude, totalArchiv)
	return nil
}

func GetGithubLanguages(parms ParamsReposGithub, ctx context.Context, client *github.Client, factor int) (int, int, int, int, error) {

	cptarchiv := 0        // Counter archiv repos
	notAnalyzedCount := 0 // Counter Number of repositories excluded
	emptyRepo := 0        // Counter Number of repositories empty
	parms.Spin.Stop()
	spin1 := spinner.New(spinner.CharSets[35], 100*time.Millisecond)
	spin1.Color("green", "bold")

	message4 := "Repo(s)"
	fmt.Printf("\t  ✅ The number of %s found is: %d\n", message4, parms.NBRepos)

	for _, repo := range parms.Repos {

		repoName := *repo.Name

		// Test if repo is archived
		if repo.GetArchived() {
			cptarchiv++
			continue
		}

		// Test is repo is excluded
		if len(parms.ExclusionList) != 0 {
			if shouldIgnore(repoName, parms.ExclusionList) {
				fmt.Printf("\t   ✅ Skipping analysis for repository '%s' as per ignore list.\n", repoName)
				notAnalyzedCount++ // Increment the counter for repositories analyzed
				continue
			}
		}
		// Next Step : Test is Repository is empty — size fast path with a confirming
		// ListCommits for size-0 candidates (see repoIsEmpty), so a repo whose size
		// is not yet computed is analyzed rather than silently skipped.
		isEmpty := repoIsEmpty(ctx, client, repo, parms.Organization)
		if !isEmpty {
			// Create a temporary platform config map for client initialization
			tempConfig := map[string]interface{}{
				"AccessToken": parms.AccessToken,
				"Url":         parms.URL,
			}
			ctx, client := initializeGithubClient(tempConfig)

			totalFiles := 0
			totalLines := 0
			totalBlankLines := 0
			totalComments := 0
			totalCodeLines := 0
			results := make([]map[string]interface{}, 0)
			supportedLanguages := assets.Languages

			languages, _, err := client.Repositories.ListLanguages(ctx, parms.Organization, repoName)
			if err != nil {
				mess := fmt.Sprintf("\r❌ failed to fetch languages. Status code: %v\n", err)
				return 0, 0, 0, 0, fmt.Errorf("%s", mess)
			}

			for lang, lines := range languages {
				if _, ok := supportedLanguages[lang]; ok {
					totalLines += lines / factor
					totalCodeLines += lines / factor
					result := map[string]interface{}{
						"Language":   lang,
						"Files":      1, // Assuming each language file is counted as 1
						"Lines":      lines / factor,
						"BlankLines": 0, // Placeholder for now
						"Comments":   0, // Placeholder for now
						"CodeLines":  lines / factor,
					}
					results = append(results, result)
				}
			}

			output := map[string]interface{}{
				"TotalFiles":      totalFiles,
				"TotalLines":      totalLines,
				"TotalBlankLines": totalBlankLines,
				"TotalComments":   totalComments,
				"TotalCodeLines":  totalCodeLines,
				"Results":         results,
			}

			// Marshal the output to JSON
			jsonData, err := json.MarshalIndent(output, "", "    ")
			if err != nil {
				mess := fmt.Sprintf("\r❌ Error marshaling JSON: %v\n", err)
				return 0, 0, 0, 0, fmt.Errorf("%s", mess)
			}

			// Write JSON data to file
			Resultfile := fmt.Sprintf("Results/Result_%s_%s.json", parms.Organization, repoName)
			file, err := os.Create(Resultfile)
			if err != nil {
				mess := fmt.Sprintf("\r❌ Error creating file: %v\n", err)
				return 0, 0, 0, 0, fmt.Errorf("%s", mess)
			}
			defer file.Close()

			_, err = file.Write(jsonData)
			if err != nil {
				mess := fmt.Sprintf("\r❌ Error writing JSON to file: %v\n", err)
				return 0, 0, 0, 0, fmt.Errorf("%s", mess)
			}

			fmt.Println("\t  ✅  JSON data written to :", Resultfile)

		} else {
			utils.SharedLogger().Infof("\t   ✅ Skipping repository '%s' — detected as empty", repoName)
			emptyRepo++
		}
	}

	return parms.NBRepos, emptyRepo, notAnalyzedCount, cptarchiv, nil
}

func sortRepositoriesByUpdatedAt(repos []*github.Repository) {
	sort.Slice(repos, func(i, j int) bool {
		timeI := repos[i].GetUpdatedAt().Time
		timeJ := repos[j].GetUpdatedAt().Time
		return timeI.After(timeJ)
	})
}

func GithubAllBranches(url, AccessToken, apiver string) ([]Branch, error) {

	var branches []Branch

	for {
		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Accept", ApiHeader1)
		req.Header.Set("Authorization", "token "+AccessToken)
		req.Header.Set("X-GitHub-Api-Version", apiver)

		resp, err := utils.HTTPClient.Do(req)
		if err != nil {
			return nil, err
		}

		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			return nil, fmt.Errorf("\n❌ Failed to list branches. Status code: %d", resp.StatusCode)
		}

		var branchList []Branch
		err = json.NewDecoder(resp.Body).Decode(&branchList)
		nextPageURL := getNextPage(resp.Header)
		resp.Body.Close()
		if err != nil {
			return nil, err
		}
		branches = append(branches, branchList...)

		if nextPageURL == "" {
			break
		}
		url = nextPageURL
	}

	return branches, nil
}

// manage pagination
func getNextPage(header http.Header) string {
	linkHeader := header.Get("Link")
	if linkHeader == "" {
		return ""
	}

	links := strings.Split(linkHeader, ",")
	for _, link := range links {
		parts := strings.Split(strings.TrimSpace(link), ";")
		if len(parts) == 2 && strings.Contains(parts[1], `rel="next"`) {
			return strings.Trim(parts[0], "<>")
		}
	}

	return ""
}
