package utils

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// SkippedRepo records a repository (branch) that could not be analyzed during the
// analysis phase — e.g. its clone timed out, the clone failed, or line counting
// errored. These are surfaced in the ResultsAll web page and the PDF report so a
// large org-wide scan that skips a few problem repos does not silently undercount.
type SkippedRepo struct {
	ProjectKey string `json:"ProjectKey"`
	RepoSlug   string `json:"RepoSlug"`
	Branch     string `json:"Branch"`
	Reason     string `json:"Reason"`
}

// SkippedReport is the on-disk envelope persisted to Results/config/analysis_skipped.json.
type SkippedReport struct {
	SkippedRepositories []SkippedRepo `json:"SkippedRepositories"`
}

// SkippedReposPath returns the canonical location of the skipped-repos file for a
// given base Results directory.
func SkippedReposPath(baseResultsDir string) string {
	return filepath.Join(baseResultsDir, "config", "analysis_skipped.json")
}

// SaveSkippedRepos writes the skipped-repos list under <baseResultsDir>/config. The
// file is always (re)written — including with an empty list — so a clean re-run
// clears any stale entries from a previous analysis.
func SaveSkippedRepos(baseResultsDir string, repos []SkippedRepo) error {
	if repos == nil {
		repos = []SkippedRepo{}
	}
	path := SkippedReposPath(baseResultsDir)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(SkippedReport{SkippedRepositories: repos}, "", "    ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// LoadSkippedRepos reads the skipped-repos list for a base Results directory. A
// missing or unreadable file yields an empty slice (not an error): reports should
// render fine on older result sets that predate this feature.
func LoadSkippedRepos(baseResultsDir string) []SkippedRepo {
	data, err := os.ReadFile(SkippedReposPath(baseResultsDir))
	if err != nil {
		return nil
	}
	var rep SkippedReport
	if err := json.Unmarshal(data, &rep); err != nil {
		return nil
	}
	return rep.SkippedRepositories
}
