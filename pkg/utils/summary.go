package utils

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// ScanSummary records the per-run repository breakdown produced during the
// analysis-discovery phase of a platform scan (Azure, GitHub, GitLab, Bitbucket
// Cloud/DC). It is surfaced in the ResultsAll web page and the global PDF report
// so a large org-wide scan makes clear how many repositories were analyzed versus
// filtered out (archived/disabled, empty, or excluded).
//
// By construction Scanned == Analyzed + Archived + Empty + Excluded, so the
// breakdown always reconciles regardless of each platform's internal counting.
// Repositories that could not be completed during the analysis phase (clone
// timeout/failure) are tracked separately in SkippedRepo and rendered alongside
// this summary.
type ScanSummary struct {
	Platform string `json:"Platform"`
	Scanned  int    `json:"Scanned"`
	Analyzed int    `json:"Analyzed"`
	Archived int    `json:"Archived"`
	Empty    int    `json:"Empty"`
	Excluded int    `json:"Excluded"`
}

// ScanSummaryPath returns the canonical location of the scan-summary file for a
// given base Results directory.
func ScanSummaryPath(baseResultsDir string) string {
	return filepath.Join(baseResultsDir, "config", "analysis_summary.json")
}

// SaveScanSummary writes the scan summary under <baseResultsDir>/config. The
// Scanned total is (re)derived from the category counts so the persisted
// breakdown always reconciles. The file is always overwritten so a clean re-run
// clears any stale summary from a previous analysis.
func SaveScanSummary(baseResultsDir string, summary ScanSummary) error {
	summary.Scanned = summary.Analyzed + summary.Archived + summary.Empty + summary.Excluded

	path := ScanSummaryPath(baseResultsDir)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(summary, "", "    ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// LoadScanSummary reads the scan summary for a base Results directory. A missing
// or unreadable file yields nil (not an error): reports should render fine on
// older result sets that predate this feature.
func LoadScanSummary(baseResultsDir string) *ScanSummary {
	data, err := os.ReadFile(ScanSummaryPath(baseResultsDir))
	if err != nil {
		return nil
	}
	var summary ScanSummary
	if err := json.Unmarshal(data, &summary); err != nil {
		return nil
	}
	return &summary
}
