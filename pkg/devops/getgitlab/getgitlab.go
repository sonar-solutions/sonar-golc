package getgitlab

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/SonarSource-Demos/sonar-golc/pkg/utils"
	"github.com/briandowns/spinner"
	"github.com/xanzy/go-gitlab"
)

type ProjectBranch struct {
	Org         string
	Namespace   string
	RepoSlug    string
	MainBranch  string
	LargestSize int
}

type ExclusionList struct {
	Repos map[string]bool `json:"repos"`
}

type AnalysisResult struct {
	NumRepositories int
	ProjectBranches []ProjectBranch
}

type AnalyzeProject struct {
	Project       *gitlab.Project
	GitlabClient  *gitlab.Client
	ExclusionList ExclusionRepos
	Spin1         *spinner.Spinner
	Org           string
}

// RepositoryMap represents a map of repositories to ignore
type ExclusionRepos map[string]bool

const PrefixMsg = "Get Project(s)..."
const MessageErro1 = "❌ Failed to list projects for group %s: %v"
const msgGroupToAnalyze = "✅ Group <%s>: %d project(s) to analyze"
const MessageError2 = "❌ Failed to get project %s: %v"
const MessageError2b = "❌ Failed to get project %s in any configured group"
const MessageError3 = "❗️ Project %s is in exclude file"
const MessageError4 = "❗️ Project %s is empty"
const MessageError5 = "❗️ Project %s is archived"
const MessageError6 = "❗️ Project %s is in exclude file or empty or archived"
const Message2 = "Analysis top branch(es) in project <%s> ..."
const Message3 = "✅ %d Project: %s - Number of branches: %d - largest Branch: %s"

const (
	perPage = 100
)

// Load repository ignore map from file
func LoadExclusionRepos(filename string) (ExclusionRepos, error) {

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

// loadExclusionList returns an empty map when exclusionfile is "0", otherwise delegates to LoadExclusionRepos.
func loadExclusionList(exclusionfile string) (ExclusionRepos, error) {
	if exclusionfile == "0" {
		return make(ExclusionRepos), nil
	}
	return LoadExclusionRepos(exclusionfile)
}

// Function to Get Commit count
func getCommitCount(client *gitlab.Client, projectID int, branchName string, since, until time.Time) (int, error) {
	commits, _, err := client.Commits.ListCommits(projectID, &gitlab.ListCommitsOptions{
		RefName: &branchName,
		Since:   &since,
		Until:   &until,
	})
	if err != nil {
		return 0, err
	}
	return len(commits), nil
}

// Function to Retrieves all projects in the main group as well as those in its subgroups.
// Uses the GitLab API's include_subgroups parameter to avoid manual subgroup traversal,
// which requires separate ListSubGroups permissions that may not be available for all roles.
func getAllGroupProjects(client *gitlab.Client, groupName string) ([]*gitlab.Project, error) {
	group, err := getGroup(client, groupName)
	if err != nil {
		return nil, err
	}
	loggers := utils.NewLogger()
	loggers.Infof("🔍 Group <%s> → resolved to ID %d (full path: %s)", groupName, group.ID, group.FullPath)
	projects, err := listGroupProjectsWithSubgroups(client, group.ID)
	if err != nil {
		return nil, err
	}
	loggers.Infof("🔍 Group <%s>: %d project(s) returned by API (before filtering)", groupName, len(projects))
	return projects, nil
}

// Function to Retrieves information about the primary group.
func getGroup(client *gitlab.Client, groupName string) (*gitlab.Group, error) {
	group, _, err := client.Groups.GetGroup(groupName, nil)
	return group, err
}

// listGroupProjectsWithSubgroups retrieves all projects in a group and all its subgroups
// using the include_subgroups API parameter (single paginated request chain, no separate
// subgroup enumeration needed).
func listGroupProjectsWithSubgroups(client *gitlab.Client, groupID int) ([]*gitlab.Project, error) {
	var projects []*gitlab.Project
	page := 1
	includeSubGroups := true
	for {
		opts := &gitlab.ListGroupProjectsOptions{
			ListOptions: gitlab.ListOptions{
				Page:    page,
				PerPage: perPage,
			},
			IncludeSubGroups: &includeSubGroups,
		}
		projs, resp, err := client.Groups.ListGroupProjects(groupID, opts)
		if err != nil {
			return nil, err
		}
		projects = append(projects, projs...)
		if resp.CurrentPage >= resp.TotalPages {
			break
		}
		page++
	}
	return projects, nil
}


// Function to Get Branch Size
func getBranchSize(client *gitlab.Client, projectID int, branchName string) int {
	tree, resp, err := client.Repositories.ListTree(projectID, &gitlab.ListTreeOptions{
		Ref: &branchName,
	})
	if err != nil && resp.StatusCode != 404 {
		if resp != nil {
			return 2
		} else {
			//if resp != nil && resp.StatusCode == 404 {
			//if resp.StatusCode == 404 {
			return 1
		}
		//return 0
	}
	return len(tree)
}

// Function test if branch exist
func branchExists(gitlabClient *gitlab.Client, projectID interface{}, branchName string) bool {
	_, _, err := gitlabClient.Branches.GetBranch(projectID, branchName)
	return err == nil
}

// Function to Get Most important Branch
func getMainBranch(client *gitlab.Client, projectID int, since, until time.Time) (string, int, int, error) {

	branches := make([]*gitlab.Branch, 0)
	page := 1
	perPage := 100

	// List of project branches
	for {

		opts := &gitlab.ListBranchesOptions{
			ListOptions: gitlab.ListOptions{
				Page:    page,
				PerPage: perPage,
			},
		}

		brs, resp, err := client.Branches.ListBranches(projectID, opts)
		if err != nil {
			return "", 0, 0, err
		}
		branches = append(branches, brs...)
		if resp.CurrentPage >= resp.TotalPages {
			break
		}
		page++
	}

	var largestBranch string
	largestSize := 0

	for _, branch := range branches {
		commitCount, err := getCommitCount(client, projectID, branch.Name, since, until)
		if err != nil {
			return "", 0, 0, err
		}
		if commitCount > largestSize {
			largestSize = commitCount
			largestBranch = branch.Name
		}
	}

	// If no commits are found for all branches, use the default branch
	if largestSize == 0 {

		project, _, err := client.Projects.GetProject(projectID, nil)
		if err != nil {
			return "", 0, len(branches), err
		}
		largestBranch = project.DefaultBranch
		largestSize = getBranchSize(client, project.ID, largestBranch)
		return largestBranch, largestSize, len(branches), nil

	}
	//fmt.Println("Branche", len(branches))
	return largestBranch, largestSize, len(branches), nil
}

// Function to check if a project should be excluded from analysis
func isExcluded(projectName string, exclusionList map[string]bool) bool {

	if _, ok := exclusionList[projectName]; ok {
		return true
	}

	// Check for subdomain match
	for excludedRepo := range exclusionList {
		if strings.HasPrefix(projectName, excludedRepo) {
			return true
		}
	}

	return false

}

func SaveResult(result AnalysisResult) error {
	const resultPath = "Results/config/analysis_result_gitlab.json"
	loggers := utils.NewLogger()
	if err := os.MkdirAll("Results/config", 0755); err != nil {
		loggers.Errorf("❌ Error creating Results/config directory: %v", err)
		return err
	}
	file, err := os.Create(resultPath)
	if err != nil {
		loggers.Errorf("❌ Error creating Analysis file:%v", err)
		return err
	}
	defer file.Close()

	// Create a JSON encoder
	encoder := json.NewEncoder(file)

	// Encode the result and write it to the file
	if err := encoder.Encode(result); err != nil {
		loggers.Errorf("❌ Error encoding JSON file <%s>: %v", resultPath, err)
		return err
	}

	fmt.Print("\n")
	loggers.Info("✅ Result saved successfully!")
	return nil
}

func analyzeProj(analyzeProject AnalyzeProject) (ProjectBranch, int, int, int) {

	largestSize := 0

	if isExcluded(analyzeProject.Project.PathWithNamespace, analyzeProject.ExclusionList) {
		return ProjectBranch{}, 1, 0, 0
	}

	// Check if the project is empty or archived
	if analyzeProject.Project.EmptyRepo || analyzeProject.Project.Archived {
		if analyzeProject.Project.EmptyRepo {
			return ProjectBranch{}, 0, 1, 0
		}
		if analyzeProject.Project.Archived {
			return ProjectBranch{}, 0, 0, 1
		}
	}

	messageB := fmt.Sprintf(Message2, analyzeProject.Project.Name)
	analyzeProject.Spin1.Prefix = messageB
	analyzeProject.Spin1.Start()
	// Retrieve project branches

	largestBranch := analyzeProject.Project.DefaultBranch
	largestSize = getBranchSize(analyzeProject.GitlabClient, analyzeProject.Project.ID, largestBranch)

	projectBranches := ProjectBranch{
		Org:         analyzeProject.Org,
		Namespace:   analyzeProject.Project.PathWithNamespace,
		RepoSlug:    analyzeProject.Project.Name,
		MainBranch:  largestBranch,
		LargestSize: largestSize,
	}

	return projectBranches, 0, 0, 0

}

func processProject(analyzeProject AnalyzeProject, cpt int, spin1 *spinner.Spinner, projectBranches []ProjectBranch, emptyRepos, archivedRepos, excludedProjects *int) ([]ProjectBranch, int) {
	projectBranche, ExcludedProject, EmptyRepos, ArchivedRepos := analyzeProj(analyzeProject)

	loggers := utils.NewLogger()

	if EmptyRepos > 0 {
		(*emptyRepos)++
		return projectBranches, cpt
	}
	if ArchivedRepos > 0 {
		(*archivedRepos)++
		return projectBranches, cpt
	}
	if ExcludedProject > 0 {
		(*excludedProjects)++
		return projectBranches, cpt
	}

	projectBranches = append(projectBranches, projectBranche)
	spin1.Stop()
	loggers.Infof(Message3, cpt, analyzeProject.Project.Name, 1, projectBranche.MainBranch)
	cpt++
	return projectBranches, cpt
}

func isProjectExcludedOrInvalid(project *gitlab.Project, exclusionList ExclusionRepos, emptyRepos, archivedRepos *int) (bool, bool, bool) {
	loggers := utils.NewLogger()
	if isExcluded(project.PathWithNamespace, exclusionList) {
		loggers.Infof("⏭️  Skipping <%s>: in exclusion list", project.Name)
		return true, false, false
	}

	if project.EmptyRepo {
		*emptyRepos++
		loggers.Infof("⏭️  Skipping <%s>: empty repository (no commits)", project.Name)
		return false, true, false
	}

	if project.Archived {
		*archivedRepos++
		loggers.Infof("⏭️  Skipping <%s>: archived", project.Name)
		return false, false, true
	}

	return false, false, false
}

func getMainBranchDetails(gitlabClient *gitlab.Client, project *gitlab.Project, since, until time.Time) (string, int, int, error) {
	mainBranch, largestSize, nbrsize, err := getMainBranch(gitlabClient, project.ID, since, until)
	if err != nil {
		return "", 0, 0, fmt.Errorf("failed to get main branch for project %s: %v", project.Name, err)
	}
	return mainBranch, largestSize, nbrsize, nil
}

// splitAndTrimCSV splits by comma and returns non-empty trimmed values.
func splitAndTrimCSV(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if v := strings.TrimSpace(p); v != "" {
			out = append(out, v)
		}
	}
	return out
}

// trimAndFilter returns a copy with all items trimmed and empty removed.
func trimAndFilter(in []string) []string {
	out := make([]string, 0, len(in))
	for _, s := range in {
		if v := strings.TrimSpace(s); v != "" {
			out = append(out, v)
		}
	}
	return out
}

// toStringSlice attempts to normalize various JSON-decoded representations to []string.
func toStringSlice(raw interface{}) []string {
	switch v := raw.(type) {
	case nil:
		return nil
	case []string:
		return trimAndFilter(v)
	case []interface{}:
		out := make([]string, 0, len(v))
		for _, it := range v {
			if s, ok := it.(string); ok {
				if vs := strings.TrimSpace(s); vs != "" {
					out = append(out, vs)
				}
			}
		}
		return out
	case string:
		return splitAndTrimCSV(v)
	default:
		return nil
	}
}

// filterValidProjects removes excluded, empty or archived projects and updates counters.
func filterValidProjects(projects []*gitlab.Project, exclusionList ExclusionRepos, emptyRepos, archivedRepos, excludedProjects *int) []*gitlab.Project {
	valid := make([]*gitlab.Project, 0, len(projects))
	for _, p := range projects {
		excluded, empty, archived := isProjectExcludedOrInvalid(p, exclusionList, emptyRepos, archivedRepos)
				if excluded {
			(*excludedProjects)++
					continue
				}
				if empty || archived {
					continue
		}
		valid = append(valid, p)
	}
	return valid
}

// analyzeMainBranchForProjects computes the main branch and size for each project.
func analyzeMainBranchForProjects(client *gitlab.Client, projects []*gitlab.Project, org string, since, until time.Time, spin1 *spinner.Spinner) ([]ProjectBranch, int) {
	var result []ProjectBranch
	totalBranches := 0
	loggers := utils.NewLogger()
	cpt := 1
	for _, project := range projects {
				messageB := fmt.Sprintf(Message2, project.Name)
				spin1.Prefix = messageB
				spin1.Start()

		mainBranch, largestSize, nbrsize, err := getMainBranchDetails(client, project, since, until)
				if err != nil {
					spin1.Stop()
			loggers.Errorf("%s", err.Error())
			continue
				}
		result = append(result, ProjectBranch{
			Org:         org,
					Namespace:   project.PathWithNamespace,
					RepoSlug:    project.Name,
					MainBranch:  mainBranch,
					LargestSize: largestSize,
				})
		totalBranches += nbrsize
				spin1.Stop()
				loggers.Infof(Message3, cpt, project.Name, nbrsize, mainBranch)
				cpt++
	}
	return result, totalBranches
}

// analyzeSpecificBranchForProjects computes the size for a given branch across projects.
func analyzeSpecificBranchForProjects(client *gitlab.Client, projects []*gitlab.Project, org, branch string, spin1 *spinner.Spinner) ([]ProjectBranch, int) {
	var result []ProjectBranch
	loggers := utils.NewLogger()
	cpt := 1
	totalBranches := 0
	for _, project := range projects {
		messageB := fmt.Sprintf(Message2, project.Name)
		spin1.Prefix = messageB
		spin1.Start()

		if !branchExists(client, project.ID, branch) {
			spin1.Stop()
			continue
		}
		largestSize := getBranchSize(client, project.ID, branch)
		result = append(result, ProjectBranch{
			Org:         org,
			Namespace:   project.PathWithNamespace,
			RepoSlug:    project.Name,
			MainBranch:  branch,
			LargestSize: largestSize,
		})
		spin1.Stop()
		loggers.Infof(Message3, cpt, project.Name, 1, branch)
		cpt++
		totalBranches++
	}
	return result, totalBranches
}

// findValidProjectAcrossOrgs searches each org for the named project and
// returns the first valid (non-excluded, non-empty, non-archived) project with its org.
func findValidProjectAcrossOrgs(client *gitlab.Client, orgs []string, projectName string, exclusions ExclusionRepos, emptyRepos, archivedRepos, excludedProjects *int) (*gitlab.Project, string, bool) {
	for _, org := range orgs {
		namespase := org + "/" + projectName
		project, _, err := client.Projects.GetProject(namespase, nil)
		if err != nil {
			continue
		}
		valid := filterValidProjects([]*gitlab.Project{project}, exclusions, emptyRepos, archivedRepos, excludedProjects)
		if len(valid) == 0 {
			continue
		}
		return valid[0], org, true
	}
	return nil, "", false
}

// newSpin1 builds a configured spinner instance for inner loops.
func newSpin1() *spinner.Spinner {
			spin1 := spinner.New(spinner.CharSets[35], 100*time.Millisecond)
			spin1.Color("green", "bold")
	return spin1
}

// nonDefaultCtx groups parameters for non-default branch analysis to keep signatures small.
type nonDefaultCtx struct {
	client           *gitlab.Client
	config           map[string]interface{}
	exclusions       ExclusionRepos
	orgs             []string
	since, until     time.Time
	spin             *spinner.Spinner
	emptyRepos       *int
	archivedRepos    *int
	excludedProjects *int
}

// nonDefaultAllProjectsAllBranches analyzes main branches for all projects across orgs.
func nonDefaultAllProjectsAllBranches(ctx nonDefaultCtx) ([]ProjectBranch, int) {
	return analyzeOrgsWithProjects(ctx, func(org string, projects []*gitlab.Project, spin1 *spinner.Spinner) ([]ProjectBranch, int) {
		valid := filterValidProjects(projects, ctx.exclusions, ctx.emptyRepos, ctx.archivedRepos, ctx.excludedProjects)
		utils.NewLogger().Infof(msgGroupToAnalyze, org, len(valid))
		return analyzeMainBranchForProjects(ctx.client, valid, org, ctx.since, ctx.until, spin1)
	})
}

// nonDefaultSpecificProjectAllBranches analyzes main branch for a single project across orgs.
func nonDefaultSpecificProjectAllBranches(ctx nonDefaultCtx) ([]ProjectBranch, int) {
	var projectBranches []ProjectBranch
	totalBranches := 0

	ctx.spin.Stop()
	spin1 := spinner.New(spinner.CharSets[35], 100*time.Millisecond)
	spin1.Color("green", "bold")
	found := false

	for _, org := range ctx.orgs {
		namespase := org + "/" + ctx.config["Project"].(string)
		project, _, err := ctx.client.Projects.GetProject(namespase, nil)
			if err != nil {
			continue
		}
		valid := filterValidProjects([]*gitlab.Project{project}, ctx.exclusions, ctx.emptyRepos, ctx.archivedRepos, ctx.excludedProjects)
		if len(valid) == 0 {
			continue
		}
		branches, totalB := analyzeMainBranchForProjects(ctx.client, valid, org, ctx.since, ctx.until, spin1)
		projectBranches = append(projectBranches, branches...)
		totalBranches += totalB
		found = true
	}
	if !found {
		utils.NewLogger().Fatalf(MessageError2b, ctx.config["Project"].(string))
	}
	return projectBranches, totalBranches
}

// nonDefaultSpecificProjectSpecificBranch analyzes a specific branch for a single project across orgs.
func nonDefaultSpecificProjectSpecificBranch(ctx nonDefaultCtx) ([]ProjectBranch, int) {
	var projectBranches []ProjectBranch
	totalBranches := 0
	branch := ctx.config["Branch"].(string)
	if project, org, ok := findValidProjectAcrossOrgs(ctx.client, ctx.orgs, ctx.config["Project"].(string), ctx.exclusions, ctx.emptyRepos, ctx.archivedRepos, ctx.excludedProjects); ok {
		projectBranches = append(projectBranches, ProjectBranch{
			Org:         org,
			Namespace:   project.PathWithNamespace,
			RepoSlug:    ctx.config["Project"].(string),
			MainBranch:  branch,
			LargestSize: 1,
		})
		totalBranches = 1
	} else {
		utils.NewLogger().Fatalf(MessageError2b, ctx.config["Project"].(string))
	}
	return projectBranches, totalBranches
}

// nonDefaultAllProjectsSpecificBranch analyzes a specific branch for all projects across orgs.
func nonDefaultAllProjectsSpecificBranch(ctx nonDefaultCtx) ([]ProjectBranch, int) {
	branch := ctx.config["Branch"].(string)
	return analyzeOrgsWithProjects(ctx, func(org string, projects []*gitlab.Project, spin1 *spinner.Spinner) ([]ProjectBranch, int) {
		valid := filterValidProjects(projects, ctx.exclusions, ctx.emptyRepos, ctx.archivedRepos, ctx.excludedProjects)
		utils.NewLogger().Infof(msgGroupToAnalyze, org, len(valid))
		return analyzeSpecificBranchForProjects(ctx.client, valid, org, branch, spin1)
	})
}

// analyzeOrgsWithProjects is a generic iterator over orgs+projects to reduce duplication.
func analyzeOrgsWithProjects(ctx nonDefaultCtx, perOrg func(org string, projects []*gitlab.Project, spin1 *spinner.Spinner) ([]ProjectBranch, int)) ([]ProjectBranch, int) {
	var projectBranches []ProjectBranch
	totalBranches := 0
	loggers := utils.NewLogger()
	for _, org := range ctx.orgs {
		projects, _, spin1, err := getProjectsAndAnalyze(ctx.client, org, ctx.spin)
			if err != nil {
			loggers.Errorf("%s", err.Error())
			continue
		}
		branches, totalB := perOrg(org, projects, spin1)
		projectBranches = append(projectBranches, branches...)
		totalBranches += totalB
	}
	return projectBranches, totalBranches
}

// listAllAccessibleGroups returns the full paths of every top-level GitLab group
// the authenticated user is a member of. Using TopLevelOnly avoids double-counting
// subgroup projects (include_subgroups=true already covers them).
func listAllAccessibleGroups(client *gitlab.Client) ([]string, error) {
	var paths []string
	page := 1
	topLevelOnly := true
	for {
		opts := &gitlab.ListGroupsOptions{
			ListOptions:  gitlab.ListOptions{Page: page, PerPage: perPage},
			TopLevelOnly: &topLevelOnly,
		}
		groups, resp, err := client.Groups.ListGroups(opts)
		if err != nil {
			return nil, err
		}
		for _, g := range groups {
			paths = append(paths, g.FullPath)
		}
		if resp.CurrentPage >= resp.TotalPages {
			break
		}
		page++
	}
	return paths, nil
}

// normalizeGitLabGroup converts a display name to a GitLab-compatible path
// by trimming whitespace and replacing spaces with hyphens.
// GitLab paths never contain spaces; users sometimes type the display name.
func normalizeGitLabGroup(s string) string {
	return strings.ReplaceAll(strings.TrimSpace(s), " ", "-")
}

// getOrganizationsFromConfig returns the list of GitLab groups to analyze.
// Supports:
// - Organizations: array or mixed []interface{}
// - Organization: string (comma-separated) or array-like
func getOrganizationsFromConfig(platformConfig map[string]interface{}) []string {
	normalize := func(orgs []string) []string {
		out := make([]string, len(orgs))
		for i, o := range orgs {
			out[i] = normalizeGitLabGroup(o)
		}
		return out
	}
	// Prefer explicit Organizations if available
	if raw, ok := platformConfig["Organizations"]; ok {
		if orgs := toStringSlice(raw); len(orgs) > 0 {
			return normalize(orgs)
		}
	}
	// Fallback to Organization (string csv or array-like)
	if raw, ok := platformConfig["Organization"]; ok {
		return normalize(toStringSlice(raw))
	}
	return nil
}

// defaultCtx groups parameters for default branch analysis to keep signatures small.
type defaultCtx struct {
	client           *gitlab.Client
	config           map[string]interface{}
	exclusions       ExclusionRepos
	orgs             []string
	since, until     time.Time
	spin             *spinner.Spinner
	emptyRepos       *int
	archivedRepos    *int
	excludedProjects *int
}

// handleDefaultBranchCase processes analysis when DefaultBranch is true.
func handleDefaultBranchCase(ctx defaultCtx) ([]ProjectBranch, int) {
	var projectBranches []ProjectBranch
	totalBranches := 0
	loggers := utils.NewLogger()

	if ctx.config["Project"].(string) == "" {
		for _, org := range ctx.orgs {
			projects, err := getAllGroupProjects(ctx.client, org)
			if err != nil {
				loggers.Errorf(MessageErro1, org, err)
				continue
			}
			ctx.spin.Stop()
			spin1 := newSpin1()

			valid := filterValidProjects(projects, ctx.exclusions, ctx.emptyRepos, ctx.archivedRepos, ctx.excludedProjects)
			// Always log after filtering — gives accurate progress total and makes
			// 0-project groups (all empty/archived) visible for debugging.
			loggers.Infof(msgGroupToAnalyze, org, len(valid))
			branches, totalB := analyzeMainBranchForProjects(ctx.client, valid, org, ctx.since, ctx.until, spin1)
			projectBranches = append(projectBranches, branches...)
			totalBranches += totalB
		}
		return projectBranches, totalBranches
	}

	// Specific project with default branch: try across all groups
	ctx.spin.Stop()
	spin1 := newSpin1()
	if project, org, ok := findValidProjectAcrossOrgs(ctx.client, ctx.orgs, ctx.config["Project"].(string), ctx.exclusions, ctx.emptyRepos, ctx.archivedRepos, ctx.excludedProjects); ok {
		branches, totalB := analyzeMainBranchForProjects(ctx.client, []*gitlab.Project{project}, org, ctx.since, ctx.until, spin1)
		projectBranches = append(projectBranches, branches...)
		totalBranches += totalB
	} else {
		loggers.Fatalf(MessageError2b, ctx.config["Project"].(string))
	}
	return projectBranches, totalBranches
}

// handleNonDefaultBranchCase processes analysis when DefaultBranch is false.
func handleNonDefaultBranchCase(ctx nonDefaultCtx) ([]ProjectBranch, int) {
	switch {
	case ctx.config["Project"].(string) == "" && ctx.config["Branch"].(string) == "":
		return nonDefaultAllProjectsAllBranches(ctx)
	case ctx.config["Project"].(string) != "" && ctx.config["Branch"].(string) == "":
		return nonDefaultSpecificProjectAllBranches(ctx)
	case ctx.config["Project"].(string) != "" && ctx.config["Branch"].(string) != "":
		return nonDefaultSpecificProjectSpecificBranch(ctx)
	default: // ctx.config["Project"] == "" && ctx.config["Branch"] != ""
		return nonDefaultAllProjectsSpecificBranch(ctx)
	}
}

func GetRepoGitLabList(platformConfig map[string]interface{}, exclusionfile string) ([]ProjectBranch, error) {

	var projectBranches []ProjectBranch
	var emptyRepos, archivedRepos int
	var TotalBranches int = 0 // Counter Number of Branches on All Repositories
	var exclusionList ExclusionRepos
	var err1 error
	var totalSize int
	loggers := utils.NewLogger()

	excludedProjects := 0
	result := AnalysisResult{}

	// Calculating the period
	until := time.Now()
	since := until.AddDate(0, int(platformConfig["Period"].(float64)), 0)
	baseURL := strings.TrimRight(platformConfig["Url"].(string), "/") + "/"
	ApiURL := baseURL + platformConfig["Baseapi"].(string) + platformConfig["Apiver"].(string)

	loggers.Infof("🔎 Analysis of devops platform objects ...\n")

	spin := spinner.New(spinner.CharSets[35], 100*time.Millisecond)
	spin.Prefix = PrefixMsg
	spin.Color("green", "bold")
	spin.Start()

	exclusionList, err1 = loadExclusionList(exclusionfile)
	if err1 != nil {
		loggers.Errorf("❌ Error Read Exclusion File <%s>: %v", exclusionfile, err1)
		spin.Stop()
		return nil, err1
	}

	gitlabClient, err := gitlab.NewClient(platformConfig["AccessToken"].(string), gitlab.WithBaseURL(ApiURL))
	if err != nil {
		loggers.Fatalf("❌ Failed to create client: %v", err)
	}

	orgs := getOrganizationsFromConfig(platformConfig)
	if len(orgs) == 0 {
		loggers.Infof("ℹ️  No group configured — discovering all accessible top-level groups...")
		discovered, err := listAllAccessibleGroups(gitlabClient)
		if err != nil {
			spin.Stop()
			loggers.Fatalf("❌ Failed to list accessible groups: %v", err)
		}
		if len(discovered) == 0 {
			spin.Stop()
			loggers.Fatalf("❌ No accessible GitLab groups found for this token.")
		}
		loggers.Infof("✅ Found %d accessible group(s): %s", len(discovered), strings.Join(discovered, ", "))
		orgs = discovered
	}

	// Delegate to specialized handlers to keep complexity low
	if platformConfig["DefaultBranch"].(bool) {
		dctx := defaultCtx{
			client:           gitlabClient,
			config:           platformConfig,
			exclusions:       exclusionList,
			orgs:             orgs,
			since:            since,
			until:            until,
			spin:             spin,
			emptyRepos:       &emptyRepos,
			archivedRepos:    &archivedRepos,
			excludedProjects: &excludedProjects,
		}
		branches, totalB := handleDefaultBranchCase(dctx)
		projectBranches = append(projectBranches, branches...)
		TotalBranches += totalB
	} else {
		ctx := nonDefaultCtx{
			client:           gitlabClient,
			config:           platformConfig,
			exclusions:       exclusionList,
			orgs:             orgs,
			since:            since,
			until:            until,
			spin:             spin,
			emptyRepos:       &emptyRepos,
			archivedRepos:    &archivedRepos,
			excludedProjects: &excludedProjects,
		}
		branches, totalB := handleNonDefaultBranchCase(ctx)
		projectBranches = append(projectBranches, branches...)
		TotalBranches += totalB
	}
	largestRepoSize := 0
	largestRepoBranch := ""
	largesRepo := ""

	for _, branch := range projectBranches {
		if branch.LargestSize > largestRepoSize {
			largestRepoSize = branch.LargestSize
			largestRepoBranch = branch.MainBranch
			largesRepo = branch.RepoSlug
		}
		totalSize += branch.LargestSize
	}

	result.NumRepositories = len(projectBranches)
	result.ProjectBranches = projectBranches
	// Save Result of Analysis
	err = SaveResult(result)
	if err != nil {
		loggers.Errorf("❌ Error Save Result of Analysis :%v", err)
		os.Exit(1)

	}

	//fmt.Printf("\n✅ The largest Repository is <%s> in the Organizationa <%s> with the branch <%s> \n", largesRepo, platformConfig["Organization"].(string), largestRepoBranch)
	//fmt.Printf("\r✅ TotalProject(s) that will be analyzed: %d - Find empty : %d - Excluded : %d - Archived : %d\n", len(projectBranches), emptyRepos, excludedProjects, archivedRepos)
	//fmt.Printf("\r✅ Total Branches that will be analyzed: %d\n", TotalBranches)

	fmt.Print("\n")
	loggers.Infof("✅ The largest Repository is <%s> with the branch <%s>", largesRepo, largestRepoBranch)
	loggers.Infof("✅ TotalProject(s) that will be analyzed: %d - Find empty : %d - Excluded : %d - Archived : %d", len(projectBranches), emptyRepos, excludedProjects, archivedRepos)
	loggers.Infof("✅ Total Branches that will be analyzed: %d\n", TotalBranches)
	return projectBranches, nil
}

func getProjectsAndAnalyze(gitlabClient *gitlab.Client, organization string, spin *spinner.Spinner) ([]*gitlab.Project, int, *spinner.Spinner, error) {

	cpt := 1
	loggers := utils.NewLogger()

	projects, err := getAllGroupProjects(gitlabClient, organization)
	if err != nil {
		spin.Stop()
		loggers.Errorf(MessageErro1, organization, err)
		return nil, cpt, nil, err
	}

	spin.Stop()
	spin1 := spinner.New(spinner.CharSets[35], 100*time.Millisecond)
	spin1.Color("green", "bold")

	return projects, cpt, spin1, nil
}
