package getazure

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/SonarSource-Demos/sonar-golc/pkg/utils"
	"github.com/briandowns/spinner"
	"github.com/google/uuid"
	"github.com/microsoft/azure-devops-go-api/azuredevops/core"
	"github.com/microsoft/azure-devops-go-api/azuredevops/git"
)

func strPtr(s string) *string { return &s }

// fakeGitClient embeds git.Client (so it satisfies the 121-method interface)
// and only overrides the calls exercised by the branch-resolution and
// repo-analysis logic (GetRepository, GetRepositories, GetItems, GetBranches,
// GetCommits). Any un-overridden method is nil and will panic if called — which
// is the signal that a test reached code it should not.
type fakeGitClient struct {
	git.Client
	repo      *git.GitRepository
	repoErr   error
	repos     *[]git.GitRepository
	items     *[]git.GitItem
	branches  *[]git.GitBranchStats
	branchErr error
	commits   map[string]int // branch name -> commit count in window
}

func (f *fakeGitClient) GetRepository(_ context.Context, _ git.GetRepositoryArgs) (*git.GitRepository, error) {
	return f.repo, f.repoErr
}
func (f *fakeGitClient) GetRepositories(_ context.Context, _ git.GetRepositoriesArgs) (*[]git.GitRepository, error) {
	return f.repos, nil
}
func (f *fakeGitClient) GetItems(_ context.Context, _ git.GetItemsArgs) (*[]git.GitItem, error) {
	return f.items, nil
}
func (f *fakeGitClient) GetBranches(_ context.Context, _ git.GetBranchesArgs) (*[]git.GitBranchStats, error) {
	return f.branches, f.branchErr
}
func (f *fakeGitClient) GetCommits(_ context.Context, a git.GetCommitsArgs) (*[]git.GitCommitRef, error) {
	v := ""
	if a.SearchCriteria != nil && a.SearchCriteria.ItemVersion != nil && a.SearchCriteria.ItemVersion.Version != nil {
		v = *a.SearchCriteria.ItemVersion.Version
	}
	out := make([]git.GitCommitRef, f.commits[v])
	return &out, nil
}

func branchStats(names ...*string) *[]git.GitBranchStats {
	out := make([]git.GitBranchStats, 0, len(names))
	for _, n := range names {
		out = append(out, git.GitBranchStats{Name: n})
	}
	return &out
}

func TestHandleNonDefaultBranch(t *testing.T) {
	tests := []struct {
		name       string
		branches   *[]git.GitBranchStats
		commits    map[string]int
		defaultB   string
		wantBranch string
		wantNbr    int
		wantErr    bool
	}{
		{
			name:       "picks branch with most commits",
			branches:   branchStats(strPtr("main"), strPtr("develop")),
			commits:    map[string]int{"main": 2, "develop": 5},
			wantBranch: "develop", wantNbr: 2,
		},
		{
			name:       "no commits, no default -> first branch",
			branches:   branchStats(strPtr("main"), strPtr("develop")),
			commits:    map[string]int{},
			wantBranch: "main", wantNbr: 2,
		},
		{
			name:       "no commits, default set -> default branch",
			branches:   branchStats(strPtr("main"), strPtr("develop")),
			commits:    map[string]int{},
			defaultB:   "refs/heads/develop",
			wantBranch: "develop", wantNbr: 2,
		},
		{
			name:       "nil branch name skipped, picks next valid",
			branches:   branchStats(nil, strPtr("release")),
			commits:    map[string]int{"release": 3},
			wantBranch: "release", wantNbr: 2,
		},
		{
			name:     "no analyzable branch -> error",
			branches: branchStats(nil),
			commits:  map[string]int{},
			wantErr:  true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fc := &fakeGitClient{branches: tt.branches, commits: tt.commits}
			branch, _, nbr, err := handleNonDefaultBranch(context.Background(), fc, "proj", "repo", "2020-01-01T00:00:00Z", tt.defaultB)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got branch=%q", branch)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if branch != tt.wantBranch {
				t.Errorf("branch = %q, want %q", branch, tt.wantBranch)
			}
			if nbr != tt.wantNbr {
				t.Errorf("nbrbranch = %d, want %d", nbr, tt.wantNbr)
			}
		})
	}
}

func TestGetMostImportantBranch(t *testing.T) {
	t.Run("default-branch mode with default present uses it", func(t *testing.T) {
		fc := &fakeGitClient{
			repo:     &git.GitRepository{DefaultBranch: strPtr("refs/heads/main")},
			branches: branchStats(strPtr("main")),
			commits:  map[string]int{"main": 3},
		}
		branch, _, nbr, err := getMostImportantBranch(context.Background(), fc, "proj", "repo", -1, true, "")
		if err != nil || branch != "main" || nbr != 1 {
			t.Fatalf("got (%q, %d, %v), want (main, 1, nil)", branch, nbr, err)
		}
	})

	t.Run("default-branch mode with nil default falls back to biggest branch", func(t *testing.T) {
		fc := &fakeGitClient{
			repo:     &git.GitRepository{DefaultBranch: nil}, // the bug trigger
			branches: branchStats(strPtr("main"), strPtr("develop")),
			commits:  map[string]int{"main": 2, "develop": 5},
		}
		branch, _, nbr, err := getMostImportantBranch(context.Background(), fc, "proj", "repo", -1, true, "")
		if err != nil || branch != "develop" || nbr != 2 {
			t.Fatalf("got (%q, %d, %v), want (develop, 2, nil)", branch, nbr, err)
		}
	})

	t.Run("single-branch mode resolves the named branch", func(t *testing.T) {
		fc := &fakeGitClient{
			repo:     &git.GitRepository{DefaultBranch: strPtr("refs/heads/main")},
			branches: branchStats(strPtr("main"), strPtr("feature")),
			commits:  map[string]int{"feature": 1},
		}
		branch, _, nbr, err := getMostImportantBranch(context.Background(), fc, "proj", "repo", -1, false, "feature")
		if err != nil || branch != "feature" || nbr != 1 {
			t.Fatalf("got (%q, %d, %v), want (feature, 1, nil)", branch, nbr, err)
		}
	})

	t.Run("GetRepository error is propagated", func(t *testing.T) {
		fc := &fakeGitClient{repoErr: context.DeadlineExceeded}
		if _, _, _, err := getMostImportantBranch(context.Background(), fc, "proj", "repo", -1, true, ""); err == nil {
			t.Fatal("expected error to propagate")
		}
	})
}

func TestHandleDefaultOrSingleBranch_NilSize(t *testing.T) {
	// commitCount == 0 forces the fallback that reads repo.Size; a nil Size must
	// not panic and must leave commitSize at 0.
	fc := &fakeGitClient{
		repo:    &git.GitRepository{Size: nil},
		commits: map[string]int{"main": 0},
	}
	branch, size, nbr, err := handleDefaultOrSingleBranch(context.Background(), fc, "proj", "repo", "main", "", "2020-01-01T00:00:00Z")
	if err != nil || branch != "main" || size != 0 || nbr != 1 {
		t.Fatalf("nil size: got (%q, %d, %d, %v), want (main, 0, 1, nil)", branch, size, nbr, err)
	}

	// With a Size present, commitSize should reflect it.
	var sz uint64 = 512
	fc2 := &fakeGitClient{repo: &git.GitRepository{Size: &sz}, commits: map[string]int{"main": 0}}
	if _, size, _, err := handleDefaultOrSingleBranch(context.Background(), fc2, "proj", "repo", "main", "", "2020-01-01T00:00:00Z"); err != nil || size != 512 {
		t.Fatalf("with size: got (%d, %v), want (512, nil)", size, err)
	}
}

// TestGetRepoAnalyse_Fallback drives the getRepoAnalyse error fallback: when
// per-repo branch analysis fails, a repo with a nil default branch is skipped
// (no panic), while a repo with a valid default branch falls back to it.
func TestGetRepoAnalyse_Fallback(t *testing.T) {
	id1, id2 := uuid.New(), uuid.New()
	fc := &fakeGitClient{
		// GetRepository (inside getMostImportantBranch) errors, so branch
		// analysis fails for every repo and the fallback path is exercised.
		repoErr: context.DeadlineExceeded,
		repos: &[]git.GitRepository{
			{Id: &id1, Name: strPtr("nildef"), DefaultBranch: nil},                  // -> skipped
			{Id: &id2, Name: strPtr("hasdef"), DefaultBranch: strPtr("refs/heads/main")}, // -> uses "main"
		},
		items:    &[]git.GitItem{{Path: strPtr("/")}}, // non-empty -> repos not "empty"
		branches: branchStats(strPtr("main")),         // non-empty
	}

	spin := spinner.New(spinner.CharSets[35], 100*time.Millisecond)
	spin.Writer = io.Discard
	params := ParamsProjectAzure{
		Context:       context.Background(),
		Projects:      []core.TeamProjectReference{{Name: strPtr("proj1")}},
		Organization:  "org",
		Exclusionlist: utils.NewExclusionList(nil, nil),
		Spin:          spin,
		DefaultB:      true,
		Period:        -1,
	}

	branches, empty, nbRepos, _, _, _, err := getRepoAnalyse(params, fc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if empty != 0 || nbRepos != 2 {
		t.Errorf("empty=%d nbRepos=%d, want empty=0 nbRepos=2", empty, nbRepos)
	}
	if len(branches) != 1 {
		t.Fatalf("expected 1 analyzed repo (nildef skipped), got %d: %+v", len(branches), branches)
	}
	if branches[0].RepoSlug != "hasdef" || branches[0].MainBranch != "main" {
		t.Errorf("got repo=%q branch=%q, want hasdef/main", branches[0].RepoSlug, branches[0].MainBranch)
	}
}

// TestGetRepoAnalyse_EmptyRepos drives getRepoAnalyse with a repo that has no
// content so it is counted as empty and excluded from the analyzed set, covering
// the empty-counting branch and the "found … empty/archived/excluded" log path.
func TestGetRepoAnalyse_EmptyRepos(t *testing.T) {
	id := uuid.New()
	fc := &fakeGitClient{
		repos:    &[]git.GitRepository{{Id: &id, Name: strPtr("emptyrepo")}},
		items:    &[]git.GitItem{},        // no items -> repo is empty
		branches: &[]git.GitBranchStats{}, // no branches
	}

	spin := spinner.New(spinner.CharSets[35], 100*time.Millisecond)
	spin.Writer = io.Discard
	params := ParamsProjectAzure{
		Context:       context.Background(),
		Projects:      []core.TeamProjectReference{{Name: strPtr("proj1")}},
		Organization:  "org",
		Exclusionlist: utils.NewExclusionList(nil, nil),
		Spin:          spin,
		Period:        -1,
	}

	branches, empty, nbRepos, _, excluded, archived, err := getRepoAnalyse(params, fc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if empty != 1 || nbRepos != 1 || len(branches) != 0 {
		t.Errorf("empty=%d nbRepos=%d analyzed=%d, want 1/1/0", empty, nbRepos, len(branches))
	}
	if excluded != 0 || archived != 0 {
		t.Errorf("excluded=%d archived=%d, want 0/0", excluded, archived)
	}
}

func TestDefaultBranchName(t *testing.T) {
	tests := []struct {
		name     string
		repo     *git.GitRepository
		wantName string
		wantOK   bool
	}{
		{"nil repo", nil, "", false},
		{"nil default branch", &git.GitRepository{DefaultBranch: nil}, "", false},
		{"empty default branch", &git.GitRepository{DefaultBranch: strPtr("")}, "", false},
		{"prefixed", &git.GitRepository{DefaultBranch: strPtr("refs/heads/main")}, "main", true},
		{"already trimmed", &git.GitRepository{DefaultBranch: strPtr("develop")}, "develop", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotName, gotOK := defaultBranchName(tt.repo)
			if gotName != tt.wantName || gotOK != tt.wantOK {
				t.Errorf("defaultBranchName() = (%q, %v), want (%q, %v)", gotName, gotOK, tt.wantName, tt.wantOK)
			}
		})
	}
}

func TestIsRepoExcluded(t *testing.T) {
	el := utils.NewExclusionList(nil, []string{"PROJ/my-repo", "OTHER/other-repo"})

	tests := []struct {
		projectKey string
		repoKey    string
		want       bool
	}{
		{"PROJ", "my-repo", true},
		{"OTHER", "other-repo", true},
		{"PROJ", "other-repo", false},
		{"NONE", "my-repo", false},
	}
	for _, tc := range tests {
		got := isRepoExcluded(el, tc.projectKey, tc.repoKey)
		if got != tc.want {
			t.Errorf("isRepoExcluded(%q, %q) = %v, want %v", tc.projectKey, tc.repoKey, got, tc.want)
		}
	}
}

func TestIsProjectExcluded(t *testing.T) {
	el := utils.NewExclusionList([]string{"excl-proj"}, nil)

	if !isProjectExcluded(el, "excl-proj") {
		t.Error("expected excl-proj to be excluded")
	}
	if isProjectExcluded(el, "other") {
		t.Error("expected other to not be excluded")
	}
}

func TestFindLargestRepository(t *testing.T) {
	branches := []ProjectBranch{
		{RepoSlug: "repo-a", MainBranch: "main", LargestSize: 100},
		{RepoSlug: "repo-b", MainBranch: "develop", LargestSize: 5000},
		{RepoSlug: "repo-c", MainBranch: "feature", LargestSize: 200},
	}

	var totalSize int64
	branch, slug := findLargestRepository(branches, &totalSize)

	if slug != "repo-b" {
		t.Errorf("expected repo-b, got %q", slug)
	}
	if branch != "develop" {
		t.Errorf("expected develop, got %q", branch)
	}
	if totalSize != 5300 {
		t.Errorf("expected totalSize=5300, got %d", totalSize)
	}
}

func TestFindLargestRepository_Empty(t *testing.T) {
	var totalSize int64
	branch, slug := findLargestRepository(nil, &totalSize)
	if slug != "" || branch != "" {
		t.Errorf("expected empty strings for nil input, got %q %q", slug, branch)
	}
	if totalSize != 0 {
		t.Errorf("expected totalSize=0, got %d", totalSize)
	}
}

func TestFindLargestRepository_SingleEntry(t *testing.T) {
	branches := []ProjectBranch{
		{RepoSlug: "only-repo", MainBranch: "main", LargestSize: 42},
	}

	var totalSize int64
	branch, slug := findLargestRepository(branches, &totalSize)

	if slug != "only-repo" {
		t.Errorf("expected only-repo, got %q", slug)
	}
	if branch != "main" {
		t.Errorf("expected main, got %q", branch)
	}
	if totalSize != 42 {
		t.Errorf("expected totalSize=42, got %d", totalSize)
	}
}

func TestContains(t *testing.T) {
	slice := []string{"alpha", "beta", "gamma"}

	tests := []struct {
		item string
		want bool
	}{
		{"alpha", true},
		{"beta", true},
		{"gamma", true},
		{"delta", false},
		{"", false},
	}
	for _, tc := range tests {
		got := contains(slice, tc.item)
		if got != tc.want {
			t.Errorf("contains(%q) = %v, want %v", tc.item, got, tc.want)
		}
	}
}

func TestContains_EmptySlice(t *testing.T) {
	if contains(nil, "anything") {
		t.Error("expected false for nil slice")
	}
	if contains([]string{}, "anything") {
		t.Error("expected false for empty slice")
	}
}

func TestLoadExclusionFileOrCreateNew_Zero(t *testing.T) {
	el, err := loadExclusionFileOrCreateNew("0")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(el.Projects) != 0 || len(el.Repos) != 0 {
		t.Error("expected empty exclusion list for '0'")
	}
}

func TestLoadExclusionFileOrCreateNew_MissingFile(t *testing.T) {
	_, err := loadExclusionFileOrCreateNew("/nonexistent/path/excl.txt")
	if err == nil {
		t.Error("expected error for missing file")
	}
}

func TestParseSingleRepos(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{"empty", "", nil},
		{"single", "repo1", []string{"repo1"}},
		{"multiple", "repo1,repo2,repo3", []string{"repo1", "repo2", "repo3"}},
		{"whitespace trimmed", "repo1, repo2 ,  repo3", []string{"repo1", "repo2", "repo3"}},
		{"blanks skipped", "repo1,,repo2,   ,repo3", []string{"repo1", "repo2", "repo3"}},
		{"only blanks", " , , ", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseSingleRepos(tt.input)
			if len(got) != len(tt.want) {
				t.Fatalf("parseSingleRepos(%q) = %v, want %v", tt.input, got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("parseSingleRepos(%q)[%d] = %q, want %q", tt.input, i, got[i], tt.want[i])
				}
			}
		})
	}
}
