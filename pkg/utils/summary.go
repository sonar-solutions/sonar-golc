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
// By construction Scanned == Analyzed + Archived + Empty + Excluded + Skipped, so
// the breakdown always reconciles regardless of each platform's internal counting.
//
// Skipped here counts repositories discovered during the analysis-discovery phase
// that were neither analyzed nor filtered into a category — e.g. a repo with no
// usable default branch, or one dropped by a single-branch selection. Repositories
// that fail later during the analysis phase (clone timeout/failure) are tracked
// separately in SkippedRepo; the reports add those two skip sources together.
type ScanSummary struct {
	Platform string `json:"Platform"`
	Scanned  int    `json:"Scanned"`
	Analyzed int    `json:"Analyzed"`
	Archived int    `json:"Archived"`
	Empty    int    `json:"Empty"`
	Excluded int    `json:"Excluded"`
	Skipped  int    `json:"Skipped"`
}

// NewScanSummary builds a ScanSummary from a platform's per-run counts. It exists
// so every platform shares one field mapping instead of repeating the struct
// literal; Scanned is left to SaveScanSummary to derive.
func NewScanSummary(platform string, analyzed, archived, empty, excluded, skipped int) ScanSummary {
	return ScanSummary{
		Platform: platform,
		Analyzed: analyzed,
		Archived: archived,
		Empty:    empty,
		Excluded: excluded,
		Skipped:  skipped,
	}
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
	summary.Scanned = summary.Analyzed + summary.Archived + summary.Empty + summary.Excluded + summary.Skipped

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

// PersistScanSummary saves the scan summary and logs (rather than propagates) any
// error. A failed summary write must never abort an otherwise-successful scan, so
// every platform funnels through this helper instead of repeating the same
// save-and-log boilerplate.
func PersistScanSummary(baseResultsDir string, summary ScanSummary) {
	if err := SaveScanSummary(baseResultsDir, summary); err != nil {
		NewLogger().Errorf("❌ Error saving scan summary: %v", err)
	}
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
