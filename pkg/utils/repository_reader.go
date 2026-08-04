package utils

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// resultsDirName is the results tree the reports have always read from, relative to the
// binary's working directory.
const resultsDirName = "Results"

// The results page and the report generators each used to carry their own copy of
// "find the inventory, work out the result file names, read them". The copies drifted:
// they disagreed about the platform key for Bitbucket Data Center, about what to do when
// a project key is missing, and about whether to sanitize path components. That drift
// produced real bugs — a repository deselected on the page stayed counted in the PDF, and
// the summary reports were never generated for Bitbucket Data Center at all.
//
// This file is the single description of that layout. Both callers read through it, so a
// disagreement between them is no longer expressible.

// PlatformSpec describes where one DevOps platform's analysis inventory lives and how its
// per-repository result files are named.
type PlatformSpec struct {
	// Name is the canonical platform key, matching the DevOps value in config.json and
	// the suffix of the inventory file. Using one string for both is what removes the old
	// "bitbucketdc" versus "bitbucket_dc" mismatch.
	Name string

	// firstPart derives the leading component of a result file name. GitHub and GitLab
	// group by organization; Azure and Bitbucket group by project key.
	firstPart func(ProjectBranch) string

	// singleComponent marks the `file` platform, whose results are named
	// Result_<Repo>.json with no organization or branch.
	singleComponent bool
}

// platformSpecs is ordered, and detection takes the first match. Order matters when a
// Results directory holds inventories from more than one platform — a re-scan against a
// different platform without clearing Results — because an unordered lookup would pick
// randomly and the reports could then describe a different platform than the page.
var platformSpecs = []PlatformSpec{
	{Name: "github", firstPart: func(b ProjectBranch) string { return b.Org }},
	{Name: "gitlab", firstPart: func(b ProjectBranch) string { return b.Org }},
	{Name: "bitbucket", firstPart: projectKeyOrOrg},
	{Name: "bitbucket_dc", firstPart: projectKeyOrOrg},
	{Name: "azure", firstPart: projectKeyOrRepo},
	{Name: "file", firstPart: func(b ProjectBranch) string { return b.RepoSlug }, singleComponent: true},
}

// projectKeyOrOrg is the Bitbucket rule: group by project key, falling back to the
// organization. The fallback matters because an empty first component produces a file name
// that matches nothing, so the repository would silently vanish from the reports.
func projectKeyOrOrg(b ProjectBranch) string {
	if b.ProjectKey != "" {
		return b.ProjectKey
	}
	return b.Org
}

// projectKeyOrRepo is the Azure rule: group by project key, falling back to the repository
// name for the same reason.
func projectKeyOrRepo(b ProjectBranch) string {
	if b.ProjectKey != "" {
		return b.ProjectKey
	}
	return b.RepoSlug
}

// PlatformSpecFor returns the spec for a platform key.
func PlatformSpecFor(name string) (PlatformSpec, bool) {
	for _, spec := range platformSpecs {
		if spec.Name == name {
			return spec, true
		}
	}
	return PlatformSpec{}, false
}

// InventoryPath returns the analysis inventory file for this platform.
func (p PlatformSpec) InventoryPath(baseResultsDir string) string {
	return filepath.Join(baseResultsDir, "config", "analysis_result_"+p.Name+".json")
}

// FirstPart returns the leading component of this repository's result file name — the
// organization for GitHub and GitLab, the project key for Azure and Bitbucket.
func (p PlatformSpec) FirstPart(b ProjectBranch) string {
	if p.firstPart == nil {
		return b.Org
	}
	return p.firstPart(b)
}

// resultStem returns the result file name without its extension or role suffix.
// Components are sanitized so a GitLab subgroup or a `release/1.0` branch cannot turn the
// name into a nested path — and so that the name matches what the reporters wrote.
func (p PlatformSpec) resultStem(b ProjectBranch) string {
	if p.singleComponent {
		return "Result_" + SanitizeResultComponent(b.RepoSlug)
	}
	return "Result_" + SanitizeResultComponent(p.FirstPart(b)) +
		keySeparator + SanitizeResultComponent(b.RepoSlug) +
		keySeparator + SanitizeResultComponent(b.MainBranch)
}

// ByFilePath returns this repository's per-file result document.
func (p PlatformSpec) ByFilePath(baseResultsDir string, b ProjectBranch) string {
	return filepath.Join(baseResultsDir, "byfile-report", p.resultStem(b)+"_byfile.json")
}

// ByLanguagePath returns this repository's per-language result document.
func (p PlatformSpec) ByLanguagePath(baseResultsDir string, b ProjectBranch) string {
	return filepath.Join(baseResultsDir, "bylanguage-report", p.resultStem(b)+".json")
}

// DeselectionKey returns the key a deselection matches on, derived from the inventory
// fields through the same rules that name the files.
func (p PlatformSpec) DeselectionKey(b ProjectBranch) string {
	return DeselectionKeyForRepo(p.Name, p.FirstPart(b), b.RepoSlug, b.MainBranch)
}

// DetectPlatform finds which platform produced the results under baseResultsDir and
// returns its spec together with the raw inventory bytes.
func DetectPlatform(baseResultsDir string) (PlatformSpec, []byte, error) {
	for _, spec := range platformSpecs {
		if data, err := os.ReadFile(spec.InventoryPath(baseResultsDir)); err == nil {
			return spec, data, nil
		}
	}
	return PlatformSpec{}, nil, fmt.Errorf("no analysis result file found for any supported platform")
}

// PreferredBranches collapses an inventory to one entry per repository, preferring a
// main/master/default branch. An all-branches scan records several entries per repository,
// and the summaries report one row each.
func PreferredBranches(branches []ProjectBranch) map[string]ProjectBranch {
	preferred := make(map[string]ProjectBranch, len(branches))
	for _, branch := range branches {
		existing, seen := preferred[branch.RepoSlug]
		if !seen || isMainBranch(branch.MainBranch) || !isMainBranch(existing.MainBranch) {
			preferred[branch.RepoSlug] = branch
		}
	}
	return preferred
}

// ReadRepositoryData collects every analyzed repository from the result files under
// baseResultsDir, largest first.
//
// A repository whose result documents are missing or unreadable is skipped rather than
// failing the whole read: one unreadable file should not cost the user the entire report.
func ReadRepositoryData(baseResultsDir string) ([]RepositoryData, error) {
	spec, inventory, err := DetectPlatform(baseResultsDir)
	if err != nil {
		return nil, fmt.Errorf("error reading analysis result file: %w", err)
	}

	var analysis AnalysisResult
	if err := json.Unmarshal(inventory, &analysis); err != nil {
		return nil, fmt.Errorf("error decoding JSON analysis result file for platform %s: %w", spec.Name, err)
	}

	var repositories []RepositoryData
	for _, branch := range PreferredBranches(analysis.ProjectBranches) {
		repo, ok := readRepository(baseResultsDir, spec, branch)
		if !ok {
			continue
		}
		repositories = append(repositories, repo)
	}

	// Largest first, ties broken by key. The tiebreak matters: sort.Slice gives no
	// ordering guarantee for equal elements and the input arrives from a map, so
	// repositories with the same size — commonly several at zero — could be ordered
	// differently between runs, Go versions or callers. That turns every report diff into
	// noise and makes row numbers move for no reason.
	sort.Slice(repositories, func(i, j int) bool {
		if repositories[i].CodeLines != repositories[j].CodeLines {
			return repositories[i].CodeLines > repositories[j].CodeLines
		}
		return repositories[i].Key < repositories[j].Key
	})
	for i := range repositories {
		repositories[i].Number = i + 1
	}
	return repositories, nil
}

// readRepository builds one repository's row from its result documents, reporting ok=false
// when they cannot be read.
func readRepository(baseResultsDir string, spec PlatformSpec, branch ProjectBranch) (RepositoryData, bool) {
	fileData, err := os.ReadFile(spec.ByFilePath(baseResultsDir, branch))
	if err != nil {
		return RepositoryData{}, false
	}

	var report struct {
		TotalLines      int `json:"TotalLines"`
		TotalBlankLines int `json:"TotalBlankLines"`
		TotalComments   int `json:"TotalComments"`
		TotalCodeLines  int `json:"TotalCodeLines"`
	}
	if err := json.Unmarshal(fileData, &report); err != nil {
		return RepositoryData{}, false
	}

	// The per-language document answers two questions from one parse: how much of the
	// total is the language held out of it (JSON), and which languages are the largest.
	codeLines := report.TotalCodeLines
	var topLanguages []LanguageShare
	if langData, err := os.ReadFile(spec.ByLanguagePath(baseResultsDir, branch)); err == nil {
		var byLang struct {
			Results []LanguageShare `json:"Results"`
		}
		if json.Unmarshal(langData, &byLang) == nil {
			for _, r := range byLang.Results {
				if strings.TrimSpace(r.Language) == LanguageExcludedFromTotalLOC {
					codeLines = report.TotalCodeLines - r.CodeLines
					break
				}
			}
			topLanguages = RankTopLanguages(byLang.Results, TopLanguagesShown)
		}
	}

	// Org carries the naming component rather than the raw inventory Org, so that on Azure
	// and Bitbucket it names the project the repository is actually grouped under. For
	// GitHub and GitLab the two are the same value.
	org := spec.FirstPart(branch)
	if spec.singleComponent {
		org = ""
	}

	return RepositoryData{
		Key:          spec.DeselectionKey(branch),
		Repository:   branch.RepoSlug,
		Org:          org,
		Branch:       branch.MainBranch,
		Lines:        report.TotalLines,
		BlankLines:   report.TotalBlankLines,
		Comments:     report.TotalComments,
		CodeLines:    codeLines,
		LinesF:       FormatCodeLines(float64(report.TotalLines)),
		BlankLinesF:  FormatCodeLines(float64(report.TotalBlankLines)),
		CommentsF:    FormatCodeLines(float64(report.TotalComments)),
		CodeLinesF:   FormatCodeLines(float64(codeLines)),
		TopLanguages: topLanguages,
	}, true
}
