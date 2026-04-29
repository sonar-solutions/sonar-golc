package getbibucket

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/SonarSource-Demos/sonar-golc/pkg/utils"
	"github.com/briandowns/spinner"
)

type ProjectBranch struct {
	ProjectKey  string
	RepoSlug    string
	MainBranch  string
	LargestSize int
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

type ProjectRepo struct {
	Type string `json:"type"`
	Key  string `json:"key"`
	UUID string `json:"uuid"`
	Name string `json:"name"`
}

type ProjectcsResponse struct {
	Values  []Projectc `json:"values"`
	PageLen int        `json:"pagelen"`
	Size    int        `json:"size"`
	Page    int        `json:"page"`
	Next    string     `json:"next"`
}

type Branch struct {
	Name             string   `json:"name"`
	DefaultMergeType string   `json:"default_merge_strategy"`
	MergeStrategies  []string `json:"merge_strategies"`
	Links            struct {
		Self    Link `json:"self"`
		Commits Link `json:"commits"`
		HTML    Link `json:"html"`
	} `json:"links"`
}

type Link struct {
	Href string `json:"href"`
}

type BranchResponse struct {
	Values  []Branch `json:"values"`
	Pagelen int      `json:"pagelen"`
	Size    int      `json:"size"`
	Page    int      `json:"page"`
	Next    string   `json:"next"`
}

type Reposc struct {
	Name        string      `json:"name"`
	Slug        string      `json:"slug"`
	Description string      `json:"description"`
	Size        int         `json:"size"`
	Language    string      `json:"language"`
	Project     ProjectRepo `json:"project"`
}

type ReposResponse struct {
	Values  []Reposc `json:"values"`
	Pagelen int      `json:"pagelen"`
	Size    int      `json:"size"`
	Page    int      `json:"page"`
	Next    string   `json:"next"`
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

type Response1 struct {
	Values  []FileInfo `json:"values"`
	Pagelen int        `json:"pagelen"`
	Page    int        `json:"page"`
	Next    string     `json:"next"`
}

type Path struct {
	Components []string `json:"components"`
	Name       string   `json:"name"`
	ToString   string   `json:"toString"`
}

type ExclusionList struct {
	Projectcs map[string]bool `json:"Projects"`
	Repos     map[string]bool `json:"repos"`
}

type ParamsReposProjectCloud struct {
	Projects         []Projectc
	URL              string
	BaseAPI          string
	APIVersion       string
	AccessToken      string
	BitbucketURLBase string
	Workspace        string
	NBRepos          int
	ExclusionList    *ExclusionList
	Spin             *spinner.Spinner
	Branch           string
}

type ParamsReposCloud struct {
	Projects         string
	Repos            []Reposc
	URL              string
	BaseAPI          string
	APIVersion       string
	AccessToken      string
	BitbucketURLBase string
	Workspace        string
	ExclusionList    *ExclusionList
	Branch           string
}

type SizeResponse struct {
	Size int `json:"size"`
}

const PrefixMsg = "Get Projects..."

var httpClient = &http.Client{
	Transport: &http.Transport{
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 100,
		IdleConnTimeout:     90 * time.Second,
	},
}

func loadExclusionList(filename string) (*ExclusionList, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	exclusionList := &ExclusionList{
		Projectcs: make(map[string]bool),
		Repos:     make(map[string]bool),
	}

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.Split(line, "/")
		if len(parts) == 1 {
			// Get Projet
			exclusionList.Projectcs[parts[0]] = true
		} else if len(parts) == 2 {
			// Get Repos
			exclusionList.Repos[line] = true
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return exclusionList, nil
}

func isProjectAndRepoExcluded(repoName string, exclusionList ExclusionList) bool {

	excluded, repoExcluded := exclusionList.Repos[repoName]
	return repoExcluded && excluded
}
func isProjectExcluded1(projectName string, exclusionList ExclusionList) bool {
	_, found := exclusionList.Projectcs[projectName]
	return found
}

func GetReposProjectCloud(parms ParamsReposProjectCloud) ([]ProjectBranch, int, int) {

	var largestRepoSize int
	var largestRepoBranch string
	var importantBranches []ProjectBranch
	var message4 string
	emptyRepo := 0
	result := AnalysisResult{}

	spin1 := spinner.New(spinner.CharSets[35], 100*time.Millisecond)
	spin1.Prefix = PrefixMsg
	spin1.Color("green", "bold")

	parms.Spin.Start()
	messageF := fmt.Sprintf("✅ The number of project(s) to analyze is %d\n", len(parms.Projects))
	parms.Spin.FinalMSG = messageF
	parms.Spin.Stop()

	for _, project := range parms.Projects {

		fmt.Printf("\n\t🟢  Analyse Projet: %s \n", project.Name)
		largestRepoSize = 0
		largestRepoBranch = ""

		urlrepos := fmt.Sprintf("%s%s/repositories/%s?q=project.key=\"%s\"&pagelen=100", parms.URL, parms.APIVersion, parms.Workspace, project.Key)

		// Get Repos for each Project

		repos, err := CloudAllRepos(urlrepos, parms.AccessToken, parms.ExclusionList)
		if err != nil {
			fmt.Println("\r❌ Get Repos for each Project:", err)
			spin1.Stop()
			continue
		}
		parms.Spin.Stop()

		parms.NBRepos += len(repos)
		message4 = "Repo(s)"

		fmt.Printf("\t  ✅ The number of %s found is: %d\n", message4, len(repos))

		for _, repo := range repos {
			largestRepoSize = 0
			largestRepoBranch = ""
			var branches []Branch
			var Nobranch int = 0

			isEmpty, err := isRepositoryEmpty(parms.Workspace, repo.Slug, parms.AccessToken, parms.BitbucketURLBase)
			if err != nil {
				fmt.Printf("❌ Error when Testing if repo is empty %s: %v\n", repo.Name, err)
				spin1.Stop()
				continue
			}

			if !isEmpty {

				if len(parms.Branch) == 0 {

					urlrepos := fmt.Sprintf("%s%s/repositories/%s/%s/refs/branches/?pagelen=100", parms.URL, parms.APIVersion, parms.Workspace, repo.Slug)

					branches, err = CloudAllBranches(urlrepos, parms.AccessToken)
					if err != nil {
						fmt.Printf("❌ Error when retrieving branches for repo %s: %v\n", repo.Name, err)
						spin1.Stop()
						continue
					}
				} else {
					urlrepos := fmt.Sprintf("%s%s/repositories/%s/%s/refs/branches/%s", parms.URL, parms.APIVersion, parms.Workspace, repo.Slug, parms.Branch)

					branches, err = ifExistBranches(urlrepos, parms.AccessToken)
					if err != nil {
						fmt.Printf("❗️ The branch <%s> for repository %s not exist - check your config.json file : \n", parms.Branch, repo.Name)
						Nobranch = 1
						continue

					}
				}
				if Nobranch == 0 {
					// Display Number of branches by repo
					fmt.Printf("\r\t✅ Repo: %s - Number of branches: %d\n", repo.Name, len(branches))

					// Finding the branch with the largest size
					if len(branches) > 1 {
						for _, branch := range branches {
							messageB := fmt.Sprintf("\t   Analysis branch <%s> size...", branch.Name)
							spin1.Prefix = messageB
							spin1.Start()

							size, err := fetchBranchSize(parms.Workspace, repo.Slug, branch.Name, parms.AccessToken, parms.URL, parms.APIVersion)
							messageF = ""
							spin1.FinalMSG = messageF

							spin1.Stop()
							if err != nil {
								fmt.Println("❌ Error retrieving branch size:", err)
								spin1.Stop()
								os.Exit(1)
							}

							if size > largestRepoSize {
								largestRepoSize = size
								//largestRepoProject = project.Name
								largestRepoBranch = branch.Name
							}

						}
					} else {
						size1, err1 := fetchBranchSize1(parms.Workspace, repo.Slug, parms.AccessToken, parms.URL, parms.APIVersion)

						if err1 != nil {
							fmt.Println("\n❌ Error retrieving branch size:", err1)
							spin1.Stop()
							os.Exit(1)
						}
						largestRepoSize = size1
						largestRepoBranch = branches[0].Name
					}

					importantBranches = append(importantBranches, ProjectBranch{
						ProjectKey:  project.Key,
						RepoSlug:    repo.Slug,
						MainBranch:  largestRepoBranch,
						LargestSize: largestRepoSize,
					})
					Nobranch = 0
				}
			} else {
				emptyRepo++
				Nobranch = 0
			}
		}

	}
	result.NumProjects = len(parms.Projects)
	result.NumRepositories = parms.NBRepos
	result.ProjectBranches = importantBranches

	// Save Result of Analysis
	file, err := os.Create("Results/config/analysis_repos.json")
	if err != nil {
		fmt.Println("❌ Error creating Analysis file:", err)
		return importantBranches, parms.NBRepos, emptyRepo
	}
	defer file.Close()
	encoder := json.NewEncoder(file)

	err = encoder.Encode(result)
	if err != nil {
		fmt.Println("Error encoding JSON file <Results/config/analysis_repos.json> :", err)
		return importantBranches, parms.NBRepos, emptyRepo
	}
	return importantBranches, parms.NBRepos, emptyRepo
}

func GetRepos(parms ParamsReposCloud) ([]ProjectBranch, int, int) {

	var largestRepoSize int
	var largestRepoBranch string
	var importantBranches []ProjectBranch
	var Nobranch int = 0
	var branches []Branch
	emptyRepo := 0
	nbRepos := 1
	result := AnalysisResult{}

	spin1 := spinner.New(spinner.CharSets[35], 100*time.Millisecond)
	spin1.Prefix = PrefixMsg
	spin1.Color("green", "bold")

	fmt.Printf("\n🟢 Analyse Projet: %s \n", parms.Projects)

	isEmpty, err := isRepositoryEmpty(parms.Workspace, parms.Repos[0].Slug, parms.AccessToken, parms.BitbucketURLBase)
	if err != nil {
		fmt.Printf("❌ Error when Testing if repo is empty %s: %v\n", parms.Repos[0].Name, err)
		spin1.Stop()
		os.Exit(1)
	}

	if !isEmpty {

		if len(parms.Branch) == 0 {
			urlrepos := fmt.Sprintf("%s%s/repositories/%s/%s/refs/branches/?pagelen=100", parms.URL, parms.APIVersion, parms.Workspace, parms.Repos[0].Slug)

			branches, err = CloudAllBranches(urlrepos, parms.AccessToken)
			if err != nil {
				fmt.Printf("❌ Error when retrieving branches for repo %s: %v\n", parms.Repos[0].Name, err)
				spin1.Stop()
				os.Exit(1)
			}

			// Display Number of branches by repo
			//fmt.Printf("\n\t   ✅ Repo: <%s> - Number of branches: %d\n", parms.Repos[0].Name, len(branches))
			fmt.Printf("\n\t   ✅ Repo: <%s> - Number of branches: 1\n", parms.Repos[0].Name)
		} else {

			urlrepos := fmt.Sprintf("%s%s/repositories/%s/%s/refs/branches/%s", parms.URL, parms.APIVersion, parms.Workspace, parms.Repos[0].Slug, parms.Branch)

			branches, err = ifExistBranches(urlrepos, parms.AccessToken)
			if err != nil {
				fmt.Printf("❗️ The branch <%s> for repository %s not exist - check your config.json file : \n", parms.Branch, parms.Repos[0].Slug)
				Nobranch = 1
				os.Exit(1)

			}
		}
		// Finding the branch with the largest size
		if Nobranch == 0 {
			if len(branches) > 1 {
				for _, branch := range branches {
					messageB := fmt.Sprintf("\t   Analysis branch <%s> size...", branch.Name)
					spin1.Prefix = messageB
					spin1.Start()

					size, err := fetchBranchSize(parms.Workspace, parms.Repos[0].Slug, branch.Name, parms.AccessToken, parms.URL, parms.APIVersion)
					messageF := ""
					spin1.FinalMSG = messageF

					spin1.Stop()
					if err != nil {
						fmt.Println("❌ Error retrieving branch size:", err)
						spin1.Stop()
						continue
					}

					if size > largestRepoSize {
						largestRepoSize = size
						//largestRepoProject = project.Name
						largestRepoBranch = branch.Name
					}

				}
			} else {
				size1, err1 := fetchBranchSize1(parms.Workspace, parms.Repos[0].Slug, parms.AccessToken, parms.URL, parms.APIVersion)

				if err1 != nil {
					fmt.Println("\n❌ Error retrieving branch size:", err1)
					spin1.Stop()
					os.Exit(1)
				}
				largestRepoSize = size1
				largestRepoBranch = branches[0].Name
			}
		}
		Nobranch = 0

		//fmt.Printf("\n\t     ✅ The largest branch of the repo is <%s> of size : %s\n", largestRepoBranch, utils.FormatSize(int64(largestRepoSize)))

		importantBranches = append(importantBranches, ProjectBranch{
			ProjectKey:  parms.Projects,
			RepoSlug:    parms.Repos[0].Slug,
			MainBranch:  largestRepoBranch,
			LargestSize: largestRepoSize,
		})
	} else {
		fmt.Println("❌ Repo is empty:", parms.Repos[0].Name)
		return importantBranches, nbRepos, emptyRepo
	}

	result.NumProjects = 1
	result.NumRepositories = nbRepos
	result.ProjectBranches = importantBranches

	// Save Result of Analysis
	file, err := os.Create("Results/config/analysis_repos_bitbucket.json")
	if err != nil {
		fmt.Println("❌ Error creating Analysis file:", err)
		return importantBranches, nbRepos, emptyRepo
	}
	defer file.Close()
	encoder := json.NewEncoder(file)

	err = encoder.Encode(result)
	if err != nil {
		fmt.Println("Error encoding JSON file <Results/config/analysis_repos_bitbucket.json> :", err)
		return importantBranches, nbRepos, emptyRepo
	}

	return importantBranches, nbRepos, emptyRepo

}

func GetProjectBitbucketListCloud(platformConfig map[string]interface{}, exlusionfile string) ([]ProjectBranch, error) {
	var largestRepoSize int
	var totalSize int
	var largestRepoProject, largestRepoBranch, largesRepo string
	var importantBranches []ProjectBranch
	var projects []Projectc
	var exclusionList *ExclusionList
	var err1 error
	var emptyRepo int
	result := AnalysisResult{}

	nbRepos := 0

	bitbucketURLBase := fmt.Sprintf("%s%s/", platformConfig["Url"].(string), platformConfig["Apiver"].(string))
	bitbucketURL := fmt.Sprintf("%s%s/workspaces/%s/projects/?pagelen=100", platformConfig["Url"].(string), platformConfig["Apiver"].(string), platformConfig["Workspace"].(string))

	fmt.Print("\n🔎 Analysis of devops platform objects ...\n")

	spin := spinner.New(spinner.CharSets[35], 100*time.Millisecond)
	spin.Prefix = PrefixMsg
	spin.Color("green", "bold")

	if exlusionfile == "0" {
		exclusionList = &ExclusionList{
			Projectcs: make(map[string]bool),
			Repos:     make(map[string]bool),
		}

	} else {
		exclusionList, err1 = loadExclusionList(exlusionfile)
		if err1 != nil {
			fmt.Printf("\n❌ Error Read Exclusion File <%s>: %v", exlusionfile, err1)
			spin.Stop()
			return nil, err1
		}

	}

	if len(platformConfig["Project"].(string)) == 0 && len(platformConfig["Repos"].(string)) == 0 {

		projects, err1 = CloudAllProjects(bitbucketURL, platformConfig["AccessToken"].(string), exclusionList)
		if err1 != nil {
			fmt.Println("\r❌ Error Get All Projects:", err1)
			spin.Stop()
			return nil, err1
		}
		spin.Stop()

		parms := ParamsReposProjectCloud{
			Projects:         projects,
			URL:              platformConfig["Url"].(string),
			BaseAPI:          platformConfig["Baseapi"].(string),
			APIVersion:       platformConfig["Apiver"].(string),
			AccessToken:      platformConfig["AccessToken"].(string),
			BitbucketURLBase: bitbucketURLBase,
			Workspace:        platformConfig["Workspace"].(string),
			NBRepos:          nbRepos,
			ExclusionList:    exclusionList,
			Spin:             spin,
			Branch:           platformConfig["Branch"].(string),
		}

		importantBranches, nbRepos, emptyRepo = GetReposProjectCloud(parms)

	} else if len(platformConfig["Project"].(string)) > 0 && len(platformConfig["Repos"].(string)) == 0 {

		if isProjectExcluded1(platformConfig["Project"].(string), *exclusionList) {
			fmt.Println("\n❌ Projet", platformConfig["Project"].(string), "is excluded from the analysis... edit <.cloc_bitbucket_ignore> file")
			os.Exit(1)
		} else {
			spin.Start()
			bitbucketURLProject := fmt.Sprintf("%s%s/workspaces/%s/projects/%s", platformConfig["Url"].(string), platformConfig["Apiver"].(string), platformConfig["Workspace"].(string), platformConfig["Project"].(string))

			projects, err := CloudOnelProjects(bitbucketURLProject, platformConfig["AccessToken"].(string), exclusionList)
			if err != nil {
				fmt.Printf("\n❌ Error Get Project:%s - %v", platformConfig["Project"].(string), err)
				spin.Stop()
				return nil, err
			}
			spin.Stop()

			if len(projects) == 0 {
				fmt.Printf("\n❌ Error Project:%s not exist - %v", platformConfig["Project"].(string), err)
				spin.Stop()
				os.Exit(1)
				//return nil, err
			} else {
				parms := ParamsReposProjectCloud{
					Projects:         projects,
					URL:              platformConfig["Url"].(string),
					BaseAPI:          platformConfig["Baseapi"].(string),
					APIVersion:       platformConfig["Apiver"].(string),
					AccessToken:      platformConfig["AccessToken"].(string),
					BitbucketURLBase: bitbucketURLBase,
					Workspace:        platformConfig["Workspace"].(string),
					NBRepos:          nbRepos,
					ExclusionList:    exclusionList,
					Spin:             spin,
					Branch:           platformConfig["Branch"].(string),
				}
				importantBranches, nbRepos, emptyRepo = GetReposProjectCloud(parms)

			}
		}
	} else if len(platformConfig["Project"].(string)) > 0 && len(platformConfig["Repos"].(string)) > 0 {

		Texclude := platformConfig["Project"].(string) + "/" + platformConfig["Repos"].(string)
		if isProjectAndRepoExcluded(Texclude, *exclusionList) {
			fmt.Println("\n❌ Projet ", platformConfig["Project"].(string), "and the repository ", platformConfig["Repos"].(string), "are excluded from the analysis...edit <.cloc_bitbucket_ignore> file")
			os.Exit(1)
		} else {

			bitbucketURLProject := fmt.Sprintf("%s%s/repositories/%s/%s?q=project.key=\"%s\"", platformConfig["Url"].(string), platformConfig["Apiver"].(string), platformConfig["Workspace"].(string), platformConfig["Repos"].(string), platformConfig["Project"].(string))
			Repos, err := fetchOneRepos(bitbucketURLProject, platformConfig["AccessToken"].(string), exclusionList)
			if err != nil {
				fmt.Printf("\n❌ Error Get Repo:%s/%s - %v", platformConfig["Project"].(string), platformConfig["Repos"].(string), err)
				spin.Stop()
				return nil, err
			}
			parms := ParamsReposCloud{
				Projects:         platformConfig["Project"].(string),
				Repos:            Repos,
				URL:              platformConfig["Url"].(string),
				BaseAPI:          platformConfig["Baseapi"].(string),
				APIVersion:       platformConfig["Apiver"].(string),
				AccessToken:      platformConfig["AccessToken"].(string),
				BitbucketURLBase: bitbucketURLBase,
				Workspace:        platformConfig["Workspace"].(string),
				ExclusionList:    exclusionList,
				Branch:           platformConfig["Branch"].(string),
			}

			importantBranches, nbRepos, emptyRepo = GetRepos(parms)
		}
	} else {
		spin.Stop()
		fmt.Println("❌ Error Project name is empty")
		os.Exit(1)
	}

	largestRepoSize = 0
	largestRepoBranch = ""
	largestRepoProject = ""
	largesRepo = ""

	for _, branch := range importantBranches {
		if branch.LargestSize > largestRepoSize {
			largestRepoSize = branch.LargestSize
			largestRepoBranch = branch.MainBranch
			largestRepoProject = branch.ProjectKey
			largesRepo = branch.RepoSlug
		}
		totalSize += branch.LargestSize
	}
	totalSizeMB := utils.FormatSize(int64(totalSize))
	largestRepoSizeMB := utils.FormatSize(int64(largestRepoSize))

	fmt.Printf("\n✅ The largest repo is <%s> in the project <%s> with the branch <%s> and a size of %s\n", largesRepo, largestRepoProject, largestRepoBranch, largestRepoSizeMB)
	fmt.Printf("\r✅ Total size of your organization's repositories: %s\n", totalSizeMB)
	fmt.Printf("\r✅ Total repositories analyzed: %d - Find empty : %d\n", nbRepos-emptyRepo, emptyRepo)

	result.NumProjects = 1
	result.NumRepositories = nbRepos
	result.ProjectBranches = importantBranches

	// Save Result of Analysis
	file, err := os.Create("Results/config/analysis_repos_bitbucketdc.json")
	if err != nil {
		fmt.Println("❌ Error creating Analysis file:", err)
		return importantBranches, nil
	}
	defer file.Close()
	encoder := json.NewEncoder(file)

	err = encoder.Encode(result)
	if err != nil {
		fmt.Println("Error encoding JSON file <Results/config/analysis_repos_bitbucketdc.json> :", err)
		return importantBranches, nil
	}

	return importantBranches, nil
}

func CloudAllProjects(url string, accessToken string, exclusionList *ExclusionList) ([]Projectc, error) {
	var allProjects []Projectc

	for url != "" {
		projectsResp, err := CloudProjects(url, accessToken, true)
		if err != nil {
			return nil, err
		}
		projectResponse := projectsResp.(*ProjectcsResponse)

		for _, project := range projectResponse.Values {
			if len(exclusionList.Projectcs) == 0 && len(exclusionList.Repos) == 0 {
				allProjects = append(allProjects, project)
			} else {
				if !isProjectExcluded(exclusionList, project.Key) {
					allProjects = append(allProjects, project)
				}
			}
		}

		url = projectResponse.Next
	}

	return allProjects, nil
}
func ifExistBranches(repoURL, accessToken string) ([]Branch, error) {

	req, err := http.NewRequest("GET", repoURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var branch Branch
	if resp.StatusCode == http.StatusOK {
		// La requête a réussi, analyser les données de la branche
		err = json.NewDecoder(resp.Body).Decode(&branch)
		if err != nil {
			return nil, err
		}
	} else {
		var errorResponse struct {
			Type  string `json:"type"`
			Error struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		err = json.NewDecoder(resp.Body).Decode(&errorResponse)
		if err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("error from API: %s", errorResponse.Error.Message)
	}

	return []Branch{branch}, nil
}

func CloudOnelProjects(url string, accessToken string, exclusionList *ExclusionList) ([]Projectc, error) {
	var allProjects []Projectc

	projectsResp, err := CloudProjects(url, accessToken, false)
	if err != nil {
		return nil, err
	}
	project := projectsResp.(*Projectc)

	if len(*&project.Key) == 0 {
		fmt.Println("\n❌ Error Project not exist")
		os.Exit(1)
	}

	if len(exclusionList.Projectcs) == 0 && len(exclusionList.Repos) == 0 {
		allProjects = append(allProjects, *project)
	} else {
		if !isProjectExcluded(exclusionList, project.Key) {
			allProjects = append(allProjects, *project)
		}
	}

	return allProjects, nil
}

func CloudProjects(url string, accessToken string, isProjectResponse bool) (interface{}, error) {
	var projectsResp interface{}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if isProjectResponse {
		projectsResp = &ProjectcsResponse{}
	} else {
		projectsResp = &Projectc{}
	}

	err = json.Unmarshal(body, &projectsResp)
	if err != nil {
		return nil, err
	}

	return projectsResp, nil
}

func fetchOneRepos(url string, accessToken string, exclusionList *ExclusionList) ([]Reposc, error) {
	var allRepos []Reposc

	reposResp, err := CloudRepos(url, accessToken, false)
	if err != nil {
		return nil, err
	}
	repo := reposResp.(*Reposc)

	if len(*&repo.Name) == 0 {
		fmt.Println("\n❌ Error Repo or Project not exist")
		os.Exit(1)
	}

	KEYTEST := repo.Project.Key + "/" + repo.Slug

	if len(exclusionList.Projectcs) == 0 && len(exclusionList.Repos) == 0 {
		allRepos = append(allRepos, *repo)
	} else {
		if !isRepoExcluded(exclusionList, KEYTEST) {
			allRepos = append(allRepos, *repo)
		}
	}

	return allRepos, nil
}

func CloudAllRepos(url string, accessToken string, exclusionList *ExclusionList) ([]Reposc, error) {
	var allRepos []Reposc
	for url != "" {
		reposResp, err := CloudRepos(url, accessToken, true)
		if err != nil {
			return nil, err
		}
		ReposResponse := reposResp.(*ReposResponse)
		for _, repo := range ReposResponse.Values {
			KEYTEST := repo.Project.Key + "/" + repo.Slug

			if len(exclusionList.Projectcs) == 0 && len(exclusionList.Repos) == 0 {
				allRepos = append(allRepos, repo)
			} else {
				if !isRepoExcluded(exclusionList, KEYTEST) {
					allRepos = append(allRepos, repo)
				}
			}

		}

		url = ReposResponse.Next
	}
	return allRepos, nil
}

func CloudRepos(url string, accessToken string, isProjectResponse bool) (interface{}, error) {
	var reposResp interface{}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if isProjectResponse {
		reposResp = &ReposResponse{}
	} else {
		reposResp = &Reposc{}
	}

	err = json.Unmarshal(body, &reposResp)
	if err != nil {
		return nil, err
	}

	return reposResp, nil

}

func CloudAllBranches(url string, accessToken string) ([]Branch, error) {
	var allBranches []Branch
	for url != "" {
		branchesResp, err := CloudBranches(url, accessToken)
		if err != nil {
			return nil, err
		}
		allBranches = append(allBranches, branchesResp.Values...)

		url = branchesResp.Next
	}
	return allBranches, nil
}

func CloudBranches(url string, accessToken string) (*BranchResponse, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	//fmt.Println(url)
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var branchesResp BranchResponse
	err = json.Unmarshal(body, &branchesResp)
	if err != nil {
		return nil, err
	}

	return &branchesResp, nil
}

func isProjectExcluded(exclusionList *ExclusionList, project string) bool {
	_, excluded := exclusionList.Projectcs[project]
	return excluded
}

func isRepoExcluded(exclusionList *ExclusionList, repo string) bool {
	_, excluded := exclusionList.Repos[repo]
	return excluded
}
func isRepositoryEmpty(workspace, repoSlug, accessToken, bitbucketURLBase string) (bool, error) {

	urlMain := fmt.Sprintf("%srepositories/%s/%s/src/main/?pagelen=100", bitbucketURLBase, workspace, repoSlug)

	// Récupérer les fichiers de la branche principale
	filesResp, err := fetchFiles(urlMain, accessToken)
	if err != nil {
		return false, fmt.Errorf("❌ Error when testing if repo: %s is empty - Function: %s - %v", repoSlug, "getbibucket-isRepositoryEmpty", err)
	}

	if filesResp == nil {
		urlMaster := fmt.Sprintf("%srepositories/%s/%s/src/master/?pagelen=100", bitbucketURLBase, workspace, repoSlug)
		filesResp, err = fetchFiles(urlMaster, accessToken)
		if err != nil {
			return false, fmt.Errorf("❌ Error when testing if repo: %s is empty - Function: %s - %v", repoSlug, "getbibucket-isRepositoryEmpty", err)
		}
	}

	if len(filesResp.Values) == 0 {
		return true, nil
	}

	return false, nil
}

func fetchFiles(url string, accessToken string) (*Response1, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := httpClient.Do(req)
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
func fetchBranchSize(workspace, repoSlug, branchName, accessToken, url, apiver string) (int, error) {

	url1 := fmt.Sprintf("%s%s/repositories/%s/%s/src/%s/?pagelen=100", url, apiver, workspace, repoSlug, branchName)

	req, err := http.NewRequest("GET", url1, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := httpClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, err
	}

	var filesResp Response1
	err = json.Unmarshal(body, &filesResp)
	if err != nil {
		return 0, err
	}

	var wg sync.WaitGroup
	wg.Add(len(filesResp.Values))

	totalSize := 0
	resultCh := make(chan int)

	for _, file := range filesResp.Values {
		go func(fileInfo FileInfo) {
			defer wg.Done()
			if fileInfo.Type == "commit_file" {
				resultCh <- fileInfo.Size
			} else if fileInfo.Type == "commit_directory" {
				dirSize, err := fetchDirectorySize(workspace, repoSlug, branchName, fileInfo.Path, accessToken, url, apiver)
				if err != nil {
					fmt.Println("Error:", err)
					return
				}
				resultCh <- dirSize
			}
		}(file)
	}

	go func() {
		wg.Wait()
		close(resultCh)
	}()

	for size := range resultCh {
		totalSize += size
	}

	return totalSize, nil
}

func fetchBranchSize1(workspace, repoSlug, accessToken, url, apiver string) (int, error) {

	url1 := fmt.Sprintf("%s%s/repositories/%s/%s/?fields=size", url, apiver, workspace, repoSlug)

	req, err := http.NewRequest("GET", url1, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := httpClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, err
	}

	var data SizeResponse

	err = json.Unmarshal(body, &data)
	if err != nil {
		return 0, err
	}

	totalSize := data.Size

	return totalSize, nil

}

func fetchDirectorySize(workspace string, repoSlug string, branchName string, components string, accessToken string, url string, apiver string) (int, error) {

	url1 := fmt.Sprintf("%s%s/reposiories/%s/%s/src/%s/%s/?pagelen=100", url, apiver, workspace, repoSlug, branchName, components)

	req, err := http.NewRequest("GET", url1, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := httpClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, err
	}

	var filesResp Response1
	err = json.Unmarshal(body, &filesResp)
	if err != nil {
		return 0, err
	}

	var wg sync.WaitGroup
	wg.Add(len(filesResp.Values))

	totalSize := 0
	resultCh := make(chan int)

	for _, file := range filesResp.Values {
		go func(fileInfo FileInfo) {
			defer wg.Done()
			if fileInfo.Type == "commit_file" {
				resultCh <- fileInfo.Size
			} else if fileInfo.Type == "commit_directory" {
				subdirSize, err := fetchDirectorySize(workspace, repoSlug, branchName, fileInfo.Path, accessToken, url, apiver)
				if err != nil {
					fmt.Println("Error:", err)
					return
				}
				resultCh <- subdirSize
			}
		}(file)
	}

	go func() {
		wg.Wait()
		close(resultCh)
	}()

	for size := range resultCh {
		totalSize += size
	}

	return totalSize, nil
}
