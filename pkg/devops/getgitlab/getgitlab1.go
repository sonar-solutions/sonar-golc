package getgitlab

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/SonarSource-Demos/sonar-golc/pkg/utils"
)

const baseURL = "gitlab.com/api/v4"

type Repository struct {
	ID            int    `json:"id"`
	Name          string `json:"name"`
	DefaultBranch string `json:"default_branch"`
	Path          string `json:"path_with_namespace"`
	Empty         bool   `json:"empty_repo"`
}

type Branch struct {
	Name string `json:"name"`
}

func FetchRepositoriesGitlab(url string, page int, accessToken string) ([]Repository, string, error) {

	var repos []Repository

	url1 := fmt.Sprintf("%s&page=%d", url, page)

	req, _ := http.NewRequest("GET", url1, nil)
	req.Header.Set("Authorization", ""+accessToken+":")

	resp, err := utils.HTTPClient.Do(req)
	if err != nil {
		fmt.Print("-- Stack: getgitlab.FetchRepositoriesGitlab Request API -- ")
		return nil, "", err
	}

	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	err = json.Unmarshal(body, &repos)
	if err != nil {
		fmt.Print("-- Stack: getgitlab.FetchRepositoriesGitlab JSON Load -- ")
		return nil, "", err
	}

	nextPageURL := ""
	linkHeader := resp.Header.Get("Link")
	if linkHeader != "" {
		links := strings.Split(linkHeader, ",")
		for _, link := range links {
			parts := strings.Split(strings.TrimSpace(link), ";")
			if len(parts) == 2 && strings.TrimSpace(parts[1]) == `rel="next"` {
				nextPageURL = strings.Trim(parts[0], "<>")
			}
		}
	}

	return repos, nextPageURL, nil
}

// Get Infos for all Repositories in Organization for Main Branch
func GetRepoGitlabList(accessToken, organization string) ([]Repository, error) {
	var url = ""
	var repositories []Repository

	url = fmt.Sprintf("https://%s@%s/groups/%s/projects?include_subgroups=true", accessToken, baseURL, organization)

	page := 1
	for {
		repos, nextPageURL, err := FetchRepositoriesGitlab(url, page, accessToken)
		if err != nil {
			log.Fatal(err)
		}
		repositories = append(repositories, repos...)
		if nextPageURL == "" {
			break
		}
		page++
	}

	return repositories, nil
}

func GetRepositoryBranches(accessToken, repositoryName string, repositoryID int) ([]Branch, error) {
	var branches []Branch
	idrepo := strconv.Itoa(repositoryID)

	url := fmt.Sprintf("https://%s@%s/projects/%s/repository/branches", accessToken, baseURL, idrepo)

	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("Authorization", ""+accessToken+":")

	resp, err := utils.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	// Check if response is a string
	var branchString string
	err = json.Unmarshal(body, &branchString)
	if err == nil {
		// If response is a string, consider it as a single branch name
		branches = append(branches, Branch{Name: branchString})
		return branches, nil
	}

	// If response is not a string, try unmarshaling it as an array
	err = json.Unmarshal(body, &branches)
	if err != nil {
		return nil, err
	}

	return branches, nil
}
