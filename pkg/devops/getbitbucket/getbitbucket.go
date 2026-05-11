package getbibucket

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/SonarSource-Demos/sonar-golc/pkg/utils"
	"github.com/briandowns/spinner"
	"github.com/ktrysmt/go-bitbucket"
)

type ProjectBranch struct {
	Org         string
	ProjectKey  string
	RepoSlug    string
	MainBranch  string
	LargestSize int
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

type AnalysisResult struct {
	NumProjects     int
	NumRepositories int
	ProjectBranches []ProjectBranch
}

type Projectc struct {
	Key         string `json:"key"`
	UUID        string `json:"uuid"`
	IsPrivate   bool   `json:"is_private"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Links       struct {
		Self struct {
			Href string `json:"href"`
		} `json:"self"`
	} `json:"links"`
}

type ProjectcsResponse struct {
	Values []Projectc `json:"values"`
	Next   string     `json:"next"`
}
type ExclusionList struct {
	Projects map[string]bool
	Repos    map[string]bool
}

type ParamsProjectBitbucket struct {
	Client           *bitbucket.Client
	Projects         []Projectc
	Workspace        string
	URL              string
	BaseAPI          string
	APIVersion       string
	AccessToken      string
	Users            string
	Username         string // Bitbucket username for git operations (different from email)
	BitbucketURLBase string
	Organization     string
	Exclusionlist    *utils.ExclusionList
	Excludeproject   int
	Spin             *spinner.Spinner
	Period           int
	Stats            bool
	DefaultB         bool
	SingleRepos      string
	SingleBranch     string
}

type Response1 struct {
	Values  []FileInfo `json:"values"`
	Pagelen int        `json:"pagelen"`
	Page    int        `json:"page"`
	Next    string     `json:"next"`
}
type FileInfo struct {
	Path     string `json:"path"`
	Commit   Commit `json:"commit"`
	Type     string `json:"type"`
	Size     int    `json:"size,omitempty"`
	MimeType string `json:"mimetype,omitempty"`
	Links    struct {
		Self struct {
			Href string `json:"href"`
		} `json:"self"`
	} `json:"links"`
}
type Commit struct {
	Hash  string `json:"hash"`
	Links struct {
		Self struct {
			Href string `json:"href"`
		} `json:"self"`
	} `json:"links"`
	Type string `json:"type"`
}

type Reposize struct {
	Size int `json:"size"`
}

const PrefixMsg = "Get Projects..."

func isRepoExcluded(exclusionList *utils.ExclusionList, projectKey, repoKey string) bool {
	_, repoExcluded := exclusionList.Repos[projectKey+"/"+repoKey]
	return repoExcluded
}

// Fonction pour vérifier si un projet est exclu
func isProjectExcluded(exclusionList *utils.ExclusionList, projectKey string) bool {
	_, projectExcluded := exclusionList.Projects[projectKey]
	return projectExcluded
}

func GetProjectBitbucketListCloud(platformConfig map[string]interface{}, exclusionFile string) ([]ProjectBranch, error) {

	var totalExclude, totalArchiv, emptyRepo, TotalBranches, exludedprojects int
	var nbRepos int

	var largestRepoBranch, largesRepo string
	var importantBranches []ProjectBranch
	var projects []Projectc
	var exclusionList *utils.ExclusionList
	var err error
	var totalSize int
	loggers := utils.SharedLogger()

	//	result := AnalysisResult{}

	loggers.Infof("🔎 Analysis of devops platform objects ...\n")

	spin := spinner.New(spinner.CharSets[35], 100*time.Millisecond)
	spin.Prefix = "Processing"
	spin.Color("green", "bold")

	exclusionList, err = loadExclusionFileOrCreateNew(exclusionFile)
	if err != nil {
		loggers.Errorf("❌ Error Read Exclusion File <%s>: %v", exclusionFile, err)
		spin.Stop()
		return nil, err
	}

	// Get Users and AccessToken for authentication
	users := ""
	if usersVal, ok := platformConfig["Users"]; ok && usersVal != nil {
		users = usersVal.(string)
	}
	accessToken := platformConfig["AccessToken"].(string)

	// Create client - will be used for some operations, but we'll use direct HTTP for auth-sensitive calls
	client := bitbucket.NewOAuthbearerToken(accessToken)

	project := platformConfig["Project"].(string)
	repos := platformConfig["Repos"].(string)
	bitbucketURLBase := fmt.Sprintf("%s%s/", platformConfig["Url"].(string), platformConfig["Apiver"].(string))

	if len(project) == 0 && len(repos) == 0 {
		// Get All Project - use direct HTTP with Basic Auth if Users is provided
		projects, exludedprojects, err = getAllProjectsWithAuth(platformConfig["Workspace"].(string), accessToken, users, bitbucketURLBase, exclusionList)
		if err != nil {
			loggers.Errorf("\r❌ Error Get All Projects:%v", err)
			spin.Stop()
			return nil, err
		}
	} else if len(project) != 0 {
		//else if len(project) != 0 && len(repos) == 0 {
		projects, exludedprojects, err = getSepecificProjectsWithAuth(platformConfig["Workspace"].(string), project, accessToken, users, bitbucketURLBase, exclusionList)
		if err != nil {
			spin.Stop()
			return nil, err
		}
	}
	spin.Stop()

	params := getCommonParams(client, platformConfig, projects, exclusionList, exludedprojects, spin, bitbucketURLBase)
	importantBranches, emptyRepo, nbRepos, TotalBranches, totalExclude, totalArchiv, err = getRepoAnalyse(params)
	if err != nil {
		spin.Stop()
		return nil, err
	}

	largestRepoBranch, largesRepo = findLargestRepository(importantBranches, &totalSize)

	result := AnalysisResult{
		NumRepositories: nbRepos,
		ProjectBranches: importantBranches,
	}
	if err := SaveResult(result); err != nil {
		fmt.Println("❌ Error Save Result of Analysis :", err)
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

	printSummary(params.Organization, stats)

	return importantBranches, nil
}

func findLargestRepository(importantBranches []ProjectBranch, totalSize *int) (string, string) {

	var largestRepoBranch, largesRepo string
	largestRepoSize := 0

	for _, branch := range importantBranches {
		if branch.LargestSize > largestRepoSize {
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

func loadExclusionFileOrCreateNew(exclusionFile string) (*utils.ExclusionList, error) {
	if exclusionFile == "0" {
		return &utils.ExclusionList{
			Projects: make(map[string]bool),
			Repos:    make(map[string]bool),
		}, nil
	}
	return utils.LoadExclusionList(exclusionFile)
}

// getAuthHeader returns the Authorization header value
// Uses Basic Auth if Users is provided, otherwise Bearer token
func getAuthHeader(users, accessToken string) string {
	if users != "" && users != "XXXXX" {
		// Use Basic Auth: base64(username:token)
		authString := fmt.Sprintf("%s:%s", users, accessToken)
		authB64 := base64.StdEncoding.EncodeToString([]byte(authString))
		return "Basic " + authB64
	}
	// Use Bearer token (default)
	return "Bearer " + accessToken
}

// GetBitbucketUsername fetches the Bitbucket username from the API
// This is needed for git operations, as they require username:token format
// Returns the username, or empty string if unable to fetch
func GetBitbucketUsername(users, accessToken, bitbucketURLBase string) string {
	url := fmt.Sprintf("%suser", bitbucketURLBase)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return ""
	}
	req.Header.Set("Authorization", getAuthHeader(users, accessToken))
	req.Header.Set("Accept", "application/json")

	resp, err := utils.HTTPClient.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return ""
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return ""
	}

	var userResponse struct {
		Username string `json:"username"`
	}

	err = json.Unmarshal(body, &userResponse)
	if err != nil {
		return ""
	}

	return userResponse.Username
}

func GetSize(parms ParamsProjectBitbucket, repo *bitbucket.Repository) (int, error) {

	url := fmt.Sprintf("%srepositories/%s/%s/?fields=size", parms.BitbucketURLBase, parms.Workspace, repo.Slug)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Authorization", getAuthHeader(parms.Users, parms.AccessToken))

	resp, err := utils.HTTPClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, err
	}

	if strings.Contains(string(body), "error") || strings.Contains(string(body), "size not found") {
		// Branch does not exist, return nil response
		return 0, nil
	}

	var Repostruct Reposize
	err = json.Unmarshal(body, &Repostruct)
	if err != nil {
		return 0, err
	}

	return Repostruct.Size, nil

}

func getCommonParams(client *bitbucket.Client, platformConfig map[string]interface{}, project []Projectc, exclusionList *utils.ExclusionList, excludeproject int, spin *spinner.Spinner, bitbucketURLBase string) ParamsProjectBitbucket {
	users := ""
	if usersVal, ok := platformConfig["Users"]; ok && usersVal != nil {
		users = usersVal.(string)
	}

	// Fetch Bitbucket username for git operations
	// For git clone, we need the username (not email), which may differ from workspace
	accessToken := platformConfig["AccessToken"].(string)
	username := GetBitbucketUsername(users, accessToken, bitbucketURLBase)
	// Fallback to workspace if username fetch fails (workspace is often the same as username)
	if username == "" {
		username = platformConfig["Workspace"].(string)
	}

	return ParamsProjectBitbucket{
		Client:           client,
		Projects:         project,
		Workspace:        platformConfig["Workspace"].(string),
		URL:              platformConfig["Url"].(string),
		BaseAPI:          platformConfig["Baseapi"].(string),
		APIVersion:       platformConfig["Apiver"].(string),
		AccessToken:      accessToken,
		Users:            users,
		Username:         username,
		BitbucketURLBase: bitbucketURLBase,
		Organization:     platformConfig["Organization"].(string),
		Exclusionlist:    exclusionList,
		Excludeproject:   excludeproject,
		Spin:             spin,
		Period:           int(platformConfig["Period"].(float64)),
		Stats:            platformConfig["Stats"].(bool),
		DefaultB:         platformConfig["DefaultBranch"].(bool),
		SingleRepos:      platformConfig["Repos"].(string),
		SingleBranch:     platformConfig["Branch"].(string),
	}
}

func getAllProjects(client *bitbucket.Client, workspace string, exclusionList *utils.ExclusionList) ([]Projectc, int, error) {

	var projects []Projectc
	var excludedCount int

	projectsRes, err := client.Workspaces.Projects(workspace)
	if err != nil {
		return nil, 0, err
	}

	for _, project := range projectsRes.Items {
		if isProjectExcluded(exclusionList, project.Key) {
			excludedCount++
			continue
		}

		projects = append(projects, Projectc{
			Key:         project.Key,
			UUID:        project.Uuid,
			IsPrivate:   project.Is_private,
			Name:        project.Name,
			Description: project.Description,
		})
	}

	return projects, excludedCount, nil
}

// getAllProjectsWithAuth uses direct HTTP calls with Basic Auth support
func getAllProjectsWithAuth(workspace, accessToken, users, bitbucketURLBase string, exclusionList *utils.ExclusionList) ([]Projectc, int, error) {
	var projects []Projectc
	var excludedCount int
	loggers := utils.SharedLogger()

	url := fmt.Sprintf("%sworkspaces/%s/projects", bitbucketURLBase, workspace)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Authorization", getAuthHeader(users, accessToken))

	loggers.Debugf("GET %s", url)
	resp, err := utils.HTTPClient.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	loggers.Debugf("GET %s → %s", url, resp.Status)

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, 0, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, 0, err
	}

	var projectsResponse struct {
		Values  []Projectc `json:"values"`
		Next    string     `json:"next"`
		Pagelen int        `json:"pagelen"`
		Size    int        `json:"size"`
		Page    int        `json:"page"`
	}

	err = json.Unmarshal(body, &projectsResponse)
	if err != nil {
		return nil, 0, err
	}

	for _, project := range projectsResponse.Values {
		if isProjectExcluded(exclusionList, project.Key) {
			excludedCount++
			continue
		}
		projects = append(projects, project)
	}

	// Handle pagination if needed
	nextURL := projectsResponse.Next
	for nextURL != "" {
		req, err := http.NewRequest("GET", nextURL, nil)
		if err != nil {
			break
		}
		req.Header.Set("Authorization", getAuthHeader(users, accessToken))

		resp, err := utils.HTTPClient.Do(req)
		if err != nil {
			break
		}

		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			break
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			break
		}

		var nextPage struct {
			Values []Projectc `json:"values"`
			Next   string     `json:"next"`
		}
		err = json.Unmarshal(body, &nextPage)
		if err != nil {
			break
		}

		for _, project := range nextPage.Values {
			if isProjectExcluded(exclusionList, project.Key) {
				excludedCount++
				continue
			}
			projects = append(projects, project)
		}

		nextURL = nextPage.Next
	}

	return projects, excludedCount, nil
}

// getSepecificProjectsWithAuth uses direct HTTP calls with Basic Auth support
func getSepecificProjectsWithAuth(workspace, projectKeys, accessToken, users, bitbucketURLBase string, exclusionList *utils.ExclusionList) ([]Projectc, int, error) {
	var projects []Projectc
	var excludedCount int
	loggers := utils.SharedLogger()

	// Split projectKeys by comma if multiple projects are specified
	projectKeyList := strings.Split(projectKeys, ",")

	for _, projectKey := range projectKeyList {
		projectKey = strings.TrimSpace(projectKey)
		if projectKey == "" {
			continue
		}

		url := fmt.Sprintf("%sworkspaces/%s/projects/%s", bitbucketURLBase, workspace, projectKey)

		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			continue
		}
		req.Header.Set("Authorization", getAuthHeader(users, accessToken))

		loggers.Debugf("GET %s", url)
		resp, err := utils.HTTPClient.Do(req)
		if err != nil {
			continue
		}
		loggers.Debugf("GET %s → %s", url, resp.Status)

		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			return nil, 0, fmt.Errorf("%s - HTTP %d", projectKey, resp.StatusCode)
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			continue
		}

		var project Projectc
		err = json.Unmarshal(body, &project)
		if err != nil {
			continue
		}

		if isProjectExcluded(exclusionList, project.Key) {
			excludedCount++
			continue
		}

		projects = append(projects, project)
	}

	return projects, excludedCount, nil
}

func getSepecificProjects(client *bitbucket.Client, workspace, projectKeys string, exclusionList *utils.ExclusionList) ([]Projectc, int, error) {

	var projects []Projectc
	var excludedCount int

	projectsRes, err := client.Workspaces.GetProject(&bitbucket.ProjectOptions{
		Owner: workspace,
		Key:   projectKeys,
	})
	if err != nil {
		errmessage := fmt.Sprintf("%s - %v", projectKeys, err)
		err1 := fmt.Errorf("%s", errmessage)
		return nil, 0, err1
	}

	if isProjectExcluded(exclusionList, projectsRes.Key) {
		excludedCount++
		errmessage := fmt.Sprintf(" - Skipping analysis for Project %s , it is excluded", projectKeys)
		err = fmt.Errorf("%s", errmessage)
		return projects, excludedCount, err

	} else {

		projects = append(projects, Projectc{
			Key:         projectsRes.Key,
			UUID:        projectsRes.Uuid,
			IsPrivate:   projectsRes.Is_private,
			Name:        projectsRes.Name,
			Description: projectsRes.Description,
		})
	}

	return projects, excludedCount, nil
}

func getRepoAnalyse(params ParamsProjectBitbucket) ([]ProjectBranch, int, int, int, int, int, error) {

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
		messageF = fmt.Sprintf("✅ The number of project(s) to analyze is %d - Excluded : %d\n\n", len(params.Projects), params.Excludeproject)
	} else {
		messageF = fmt.Sprintf("✅ The number of project(s) to analyze is %d\n\n", len(params.Projects))
	}
	params.Spin.FinalMSG = messageF
	params.Spin.Stop()

	// Get Repository in each Project
	for _, project := range params.Projects {

		fmt.Print("\n")
		loggers.Infof("\t🟢  Analyse Project: %s ", project.Name)

		archivedCount, emptyOrArchivedCount, excludedCount, repos, err := listReposForProject(params, project.Key)
		if err != nil {
			if len(params.SingleRepos) == 0 {
				loggers.Errorf("❌ Get Repos for each Project:%v", err)
				spin1.Stop()
				continue
			} else {
				errmessage := fmt.Sprintf(" Get Repo %s for Project %s %v", params.SingleRepos, project.Key, err)
				spin1.Stop()
				return importantBranches, emptyRepos, NBRrepos, TotalBranches, totalexclude, cptarchiv, fmt.Errorf("%s", errmessage)
			}
		}
		emptyRepos = emptyRepos + emptyOrArchivedCount
		cptarchiv = cptarchiv + archivedCount
		totalexclude = totalexclude + excludedCount

		spin1.Stop()
		NBRrepo = len(repos) + emptyOrArchivedCount + archivedCount
		if emptyOrArchivedCount > 0 || archivedCount > 0 {
			loggers.Infof("\t  ✅ The number of %s found is: %d - Find empty %d:", message4, NBRrepo, emptyOrArchivedCount)
		} else {
			loggers.Infof("\t  ✅ The number of %s found is: %d", message4, NBRrepo)
		}

		for _, repo := range repos {

			largestRepoBranch, repobranches, brsize, err := analyzeRepoBranches(params, repo, cpt, spin1)
			if err != nil {
				largestRepoBranch = repo.Mainbranch.Name
				brsize = 1
				TotalBranches++
			} else {
				TotalBranches += len(repobranches)
			}

			importantBranches = append(importantBranches, ProjectBranch{
				Org:         params.Organization,
				ProjectKey:  project.Key,
				RepoSlug:    repo.Slug,
				MainBranch:  largestRepoBranch,
				LargestSize: brsize,
			})

			cpt++
		}

		NBRrepos += NBRrepo

	}

	return importantBranches, emptyRepos, NBRrepos, TotalBranches, totalexclude, cptarchiv, nil

}
func listReposForProject(parms ParamsProjectBitbucket, projectKey string) (int, int, int, []*bitbucket.Repository, error) {
	var allRepos []*bitbucket.Repository
	var archivedCount, excludedCount, emptyOrArchivedCount int
	loggers := utils.SharedLogger()

	// Use direct HTTP calls with Basic Auth support
	url := fmt.Sprintf("%srepositories/%s?q=project.key=\"%s\"&pagelen=100", parms.BitbucketURLBase, parms.Workspace, projectKey)

	for url != "" {
		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			return 0, 0, 0, nil, err
		}
		req.Header.Set("Authorization", getAuthHeader(parms.Users, parms.AccessToken))

		resp, err := utils.HTTPClient.Do(req)
		if err != nil {
			return 0, 0, 0, nil, err
		}

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			return 0, 0, 0, nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return 0, 0, 0, nil, err
		}

		var reposResponse struct {
			Values []struct {
				Type       string `json:"type"`
				FullName   string `json:"full_name"`
				Links      struct {
					Self struct {
						Href string `json:"href"`
					} `json:"self"`
				} `json:"links"`
				Name       string `json:"name"`
				Slug       string `json:"slug"`
				UUID       string `json:"uuid"`
				IsPrivate  bool   `json:"is_private"`
				IsArchived bool   `json:"is_archived"`
				Mainbranch struct {
					Name string `json:"name"`
				} `json:"mainbranch"`
				Project struct {
					Key string `json:"key"`
				} `json:"project"`
			} `json:"values"`
			Next    string `json:"next"`
			Pagelen int    `json:"pagelen"`
			Size    int    `json:"size"`
			Page    int    `json:"page"`
		}

		err = json.Unmarshal(body, &reposResponse)
		if err != nil {
			return 0, 0, 0, nil, err
		}

		// Convert to bitbucket.Repository format
		var reposResItems []bitbucket.Repository
		for _, repo := range reposResponse.Values {
			if repo.IsArchived {
				loggers.Debugf("→ repo %s/%s: skipped (archived)", projectKey, repo.Slug)
				archivedCount++
				if parms.SingleRepos == repo.Slug {
					return archivedCount, 0, 0, nil, nil
				}
				continue
			}
			bbRepo := bitbucket.Repository{
				Full_name:  repo.FullName,
				Slug:       repo.Slug,
				Uuid:       repo.UUID,
				Is_private: repo.IsPrivate,
			}
			if repo.Mainbranch.Name != "" {
				bbRepo.Mainbranch = bitbucket.RepositoryBranch{
					Name: repo.Mainbranch.Name,
				}
			}
			reposResItems = append(reposResItems, bbRepo)
		}

		// Create a RepositoriesRes-like structure for listRepos
		reposRes := &bitbucket.RepositoriesRes{
			Items:   reposResItems,
			Pagelen: int32(reposResponse.Pagelen),
			Page:    int32(reposResponse.Page),
			Size:    int32(reposResponse.Size),
		}

		eoc, exc, repos, err := listRepos(parms, projectKey, reposRes)
		if err != nil {
			return 0, 0, 0, nil, err
		}
		emptyOrArchivedCount += eoc
		excludedCount += exc
		allRepos = append(allRepos, repos...)

		url = reposResponse.Next
	}

	return archivedCount, emptyOrArchivedCount, excludedCount, allRepos, nil
}

func listRepos(parms ParamsProjectBitbucket, projectKey string, reposRes *bitbucket.RepositoriesRes) (int, int, []*bitbucket.Repository, error) {
	var allRepos []*bitbucket.Repository
	var excludedCount, emptyOrArchivedCount int
	loggers := utils.SharedLogger()

	if len(parms.SingleRepos) == 0 {

		for _, repo := range reposRes.Items {
			repoCopy := repo
			if isRepoExcluded(parms.Exclusionlist, projectKey, repo.Slug) {
				loggers.Debugf("→ repo %s/%s: skipped (excluded)", projectKey, repo.Slug)
				excludedCount++
				continue
			}

			isEmpty, err := isRepositoryEmpty(parms.Workspace, repo.Slug, repo.Mainbranch.Name, parms.AccessToken, parms.Users, parms.BitbucketURLBase)
			if err != nil {
				loggers.Errorf("❌ Error when Testing if repo is empty %s: %v\n", repo.Slug, err)
			}
			if isEmpty {
				loggers.Debugf("→ repo %s/%s: skipped (empty)", projectKey, repo.Slug)
				emptyOrArchivedCount++
				continue
			}
			loggers.Debugf("→ repo %s/%s: analyzing", projectKey, repo.Slug)
			allRepos = append(allRepos, &repoCopy)
		}
	} else {

		var repoFound bool
		for _, repo := range reposRes.Items {

			if repo.Slug == parms.SingleRepos {
				repoFound = true
				repoCopy := repo

				if isRepoExcluded(parms.Exclusionlist, projectKey, repo.Slug) {
					excludedCount++
					errmessage := fmt.Sprintf(" - Skipping analysis for Repo %s , it is excluded", repo.Slug)
					err := fmt.Errorf("%s", errmessage)
					return 0, excludedCount, allRepos, err
				}

				isEmpty, err := isRepositoryEmpty(parms.Workspace, repo.Slug, repo.Mainbranch.Name, parms.AccessToken, parms.Users, parms.BitbucketURLBase)
				if err != nil {
					loggers.Errorf("❌ Error when Testing if repo is empty %s: %v\n", repo.Slug, err)
				}
				if isEmpty {
					emptyOrArchivedCount++
					errmessage := fmt.Sprintf(" - Skipping analysis for Repo %s , it is empty", repo.Slug)
					err := fmt.Errorf("%s", errmessage)
					return emptyOrArchivedCount, excludedCount, allRepos, err
				}

				allRepos = append(allRepos, &repoCopy)
				break
			}
		}

		if !repoFound {
			excludedCount++
		}
	}
	return emptyOrArchivedCount, excludedCount, allRepos, nil
}

// Test is Repository is empty
func isRepositoryEmpty(workspace, repoSlug, mainbranch, accessToken, users, bitbucketURLBase string) (bool, error) {

	urlMain := fmt.Sprintf("%srepositories/%s/%s/src/%s/?pagelen=100", bitbucketURLBase, workspace, repoSlug, mainbranch)

	filesResp, err := fetchFiles(urlMain, accessToken, users)
	if err != nil {
		return false, fmt.Errorf("❌ Error when testing if repo: %s is empty - Function: %s - %v", repoSlug, "getbibucket-isRepositoryEmpty", err)
	}

	if filesResp == nil {
		urlMaster := fmt.Sprintf("%srepositories/%s/%s/src/master/?pagelen=100", bitbucketURLBase, workspace, repoSlug)
		filesResp, err = fetchFiles(urlMaster, accessToken, users)
		if err != nil {
			return false, fmt.Errorf("❌ Error when testing if repo: %s is empty - Function: %s - %v", repoSlug, "getbibucket-isRepositoryEmpty", err)
		}
	}

	// Check if filesResp is still nil after both attempts
	if filesResp == nil {
		// If both main branch and master branch calls returned nil, consider repository as empty or inaccessible
		return true, nil
	}

	if len(filesResp.Values) == 0 {
		return true, nil
	}

	return false, nil
}

func fetchFiles(url string, accessToken string, users string) (*Response1, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", getAuthHeader(users, accessToken))

	resp, err := utils.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if strings.Contains(string(body), "error") || strings.Contains(string(body), "Commit not found") {
		// Branch does not exist, return nil response
		return nil, nil
	}

	var filesResp Response1
	err = json.Unmarshal(body, &filesResp)
	if err != nil {
		return nil, err
	}

	return &filesResp, nil
}

func analyzeRepoBranches(parms ParamsProjectBitbucket, repo *bitbucket.Repository, cpt int, spin1 *spinner.Spinner) (string, []*bitbucket.RepositoryBranch, int, error) {

	var repoBranches []*bitbucket.RepositoryBranch
	var largestRepoBranch string
	var err error
	var brsize, nbrbranche int
	loggers := utils.SharedLogger()

	spin1.Prefix = "\r Analyzing branches"
	spin1.Start()

	if parms.DefaultB || len(parms.SingleBranch) != 0 {
		var branchName string
		if parms.DefaultB {
			branchName = repo.Mainbranch.Name
		} else if len(parms.SingleBranch) != 0 {
			branchName = parms.SingleBranch
		}
		repoBranches, largestRepoBranch, brsize, err = getSingleBranches(parms, branchName, repo.Slug, spin1)
		if err != nil {
			spin1.Stop()
			return "", nil, 0, err
		}
		nbrbranche = 1

	} else {
		repoBranches, err := getAllBranches(parms.Client, parms.Workspace, repo.Slug)
		if err != nil {
			spin1.Stop()
			return "", nil, 0, err
		}

		// Determine the largest branch based on the number of commits
		largestRepoBranch, brsize = determineLargestBranch(parms, repo, repoBranches)
		nbrbranche = len(repoBranches)

	}

	spin1.Stop()

	// Print analysis summary
	loggers.Infof("\t\t✅ Repo %d: %s - Number of branches: %d - Largest Branch: %s", cpt, repo.Slug, nbrbranche, largestRepoBranch)

	return largestRepoBranch, repoBranches, brsize, nil
}

func getSingleBranches(parms ParamsProjectBitbucket, singlebranch string, repoSlug string, spin1 *spinner.Spinner) ([]*bitbucket.RepositoryBranch, string, int, error) {

	var repoBranches1 []*bitbucket.RepositoryBranch

	branchesRes1, err := parms.Client.Repositories.Repository.ListBranches(&bitbucket.RepositoryBranchOptions{
		Owner:      parms.Workspace,
		RepoSlug:   repoSlug,
		BranchName: singlebranch,
	})
	if err != nil {
		spin1.Stop()
		return repoBranches1, "", 0, err
	}
	for _, branch := range branchesRes1.Branches {
		branchCopy := branch
		repoBranches1 = append(repoBranches1, &branchCopy)
	}

	return repoBranches1, singlebranch, 1, nil

}

func getAllBranches(client *bitbucket.Client, workspace, repoSlug string) ([]*bitbucket.RepositoryBranch, error) {
	var allBranches []*bitbucket.RepositoryBranch
	options := &bitbucket.RepositoryBranchOptions{
		Owner:    workspace,
		RepoSlug: repoSlug,
		Pagelen:  100,
	}
	page := 1

	for {
		// Set the page number for pagination
		options.PageNum = page

		// Get a page of branches for the repository
		branchesRes, err := client.Repositories.Repository.ListBranches(options)
		if err != nil {
			return nil, err
		}

		// Convert branchesRes.Values to []*bitbucket.RepositoryBranch
		for i := range branchesRes.Branches {
			branch := branchesRes.Branches[i]
			allBranches = append(allBranches, &branch)
		}

		// Check if there are more pages to fetch
		if len(branchesRes.Branches) < options.Pagelen {
			break
		}

		page++
	}

	return allBranches, nil
}
func determineLargestBranch(parms ParamsProjectBitbucket, repo *bitbucket.Repository, branches []*bitbucket.RepositoryBranch) (string, int) {
	var largestRepoBranch string
	var maxCommits, branchSize int
	loggers := utils.SharedLogger()

	for _, branch := range branches {
		commits, err := getCommitsForLastMonth(parms.Client, parms.Workspace, repo.Slug, branch.Name, parms.Period)
		if err != nil {
			loggers.Errorf("❌ Error getting commits for branch %s: %v\n", branch.Name, err)
			continue
		}
		if len(commits) == 0 {
			branchSize, _ = GetSize(parms, repo)
		} else {
			branchSize = len(commits)
		}

		if branchSize > maxCommits {
			maxCommits = branchSize
			largestRepoBranch = branch.Name
		}
	}

	// If no branch has commits, use the default main branch
	if largestRepoBranch == "" {
		largestRepoBranch = repo.Mainbranch.Name
	}

	return largestRepoBranch, branchSize
}

func getCommitsForLastMonth(client *bitbucket.Client, workspace, repoSlug, branchName string, periode int) ([]interface{}, error) {
	now := time.Now()
	lastMonth := now.AddDate(0, -periode, 0)
	loggers := utils.SharedLogger()

	commits, err := client.Repositories.Commits.GetCommits(&bitbucket.CommitsOptions{
		Owner:       workspace,
		RepoSlug:    repoSlug,
		Branchortag: branchName,
	})
	if err != nil {
		return nil, err
	}

	var recentCommits []interface{}
	for _, commit := range commits.(map[string]interface{})["values"].([]interface{}) {
		dateStr := commit.(map[string]interface{})["date"].(string)
		commitDate, err := time.Parse(time.RFC3339, dateStr)
		if err != nil {
			loggers.Errorf("❌ Error parsing commit date: %v\n", err)
			continue
		}

		if commitDate.After(lastMonth) {
			recentCommits = append(recentCommits, commit)
		}
	}

	return recentCommits, nil
}

func SaveResult(result AnalysisResult) error {

	loggers := utils.SharedLogger()
	// Open or create the file
	file, err := os.Create("Results/config/analysis_result_bitbucket.json")
	if err != nil {
		loggers.Errorf("❌ Error creating Analysis file:%v", err)
		return err
	}
	defer file.Close()

	// Create a JSON encoder
	encoder := json.NewEncoder(file)

	// Encode the result and write it to the file
	if err := encoder.Encode(result); err != nil {
		loggers.Errorf("❌ Error encoding JSON file <Results/config/analysis_result_bitbucket.json> :%v", err)
		return err
	}
	fmt.Print("\n")
	loggers.Infof("✅ Result saved successfully!\n")
	return nil
}
