package getazure

import (
	"testing"

	"github.com/SonarSource-Demos/sonar-golc/pkg/utils"
)

func makeExclusionList(projects, repos []string) *utils.ExclusionList {
	el := &utils.ExclusionList{
		Projects: make(map[string]bool),
		Repos:    make(map[string]bool),
	}
	for _, p := range projects {
		el.Projects[p] = true
	}
	for _, r := range repos {
		el.Repos[r] = true
	}
	return el
}

func TestIsRepoExcluded(t *testing.T) {
	el := makeExclusionList(nil, []string{"PROJ/my-repo", "OTHER/other-repo"})

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
	el := makeExclusionList([]string{"excl-proj"}, nil)

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
