package utils

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// TopLanguagesShown is how many of a repository's largest languages are surfaced
// alongside its totals.
const TopLanguagesShown = 5

// LanguageShare is one language's contribution to a single repository.
type LanguageShare struct {
	Language   string `json:"Language"`
	CodeLines  int    `json:"CodeLines"`
	CodeLinesF string `json:"CodeLinesF"`
	// Excluded marks a language left out of the repository's total; see RankLanguageChips.
	// Everywhere marks one excluded by the global set, which the repository cannot change.
	Excluded   bool `json:"Excluded,omitempty"`
	Everywhere bool `json:"Everywhere,omitempty"`
}

// RankTopLanguages returns a repository's largest languages, biggest first, capped at
// limit.
//
// The languages held out of the headline total are excluded, matching the CodeLines
// figure these languages appear next to. Including them would let a repository show
// "JSON 240K" beside a code-line count that deliberately does not contain those lines.
//
// Ties break on language name so the output is stable across runs.
func RankTopLanguages(languages []LanguageShare, limit int, excluded LanguageExclusion) []LanguageShare {
	ranked := make([]LanguageShare, 0, len(languages))
	for _, lang := range languages {
		name := strings.TrimSpace(lang.Language)
		if name == "" || excluded.Excludes(name) || lang.CodeLines <= 0 {
			continue
		}
		ranked = append(ranked, LanguageShare{
			Language:   name,
			CodeLines:  lang.CodeLines,
			CodeLinesF: FormatCodeLines(float64(lang.CodeLines)),
		})
	}

	return rankShares(ranked, limit)
}

// RankLanguageChips returns the languages the results page offers a switch for in a
// repository's row: its largest languages, biggest first, capped at limit.
//
// Unlike RankTopLanguages it keeps the excluded languages, flagged, so a row always shows
// the repository's real largest languages: those it excludes of its own can be switched
// back on from the row, and those excluded globally show switched off and locked - they
// are switched on the Languages card, not per repository. excluded is expected to be
// scoped to the repository with ForRepo.
func RankLanguageChips(languages []LanguageShare, limit int, excluded LanguageExclusion) []LanguageShare {
	ranked := make([]LanguageShare, 0, len(languages))
	for _, lang := range languages {
		name := strings.TrimSpace(lang.Language)
		if name == "" || lang.CodeLines <= 0 {
			continue
		}
		ranked = append(ranked, LanguageShare{
			Language:   name,
			CodeLines:  lang.CodeLines,
			CodeLinesF: FormatCodeLines(float64(lang.CodeLines)),
			Excluded:   excluded.Excludes(name),
			Everywhere: excluded.ExcludesEverywhere(name),
		})
	}
	return rankShares(ranked, limit)
}

// rankShares orders languages biggest first, ties by name so the output is stable across
// runs, and caps the list at limit.
func rankShares(ranked []LanguageShare, limit int) []LanguageShare {
	sort.Slice(ranked, func(i, j int) bool {
		if ranked[i].CodeLines != ranked[j].CodeLines {
			return ranked[i].CodeLines > ranked[j].CodeLines
		}
		return ranked[i].Language < ranked[j].Language
	})

	if limit > 0 && len(ranked) > limit {
		ranked = ranked[:limit]
	}
	return ranked
}

// RepositoryData represents a single repository's data for summary reports
type RepositoryData struct {
	Number int `json:"Number"`
	// Key is the repository's identity across the reports — the stem of its result
	// file — and is what a deselection matches on.
	Key         string `json:"Key"`
	Repository  string `json:"Repository"`
	Org         string `json:"Org,omitempty"`
	Branch      string `json:"Branch"`
	Lines       int    `json:"Lines"`
	BlankLines  int    `json:"BlankLines"`
	Comments    int    `json:"Comments"`
	CodeLines   int    `json:"CodeLines"`
	LinesF      string `json:"LinesF"`
	BlankLinesF string `json:"BlankLinesF"`
	CommentsF   string `json:"CommentsF"`
	CodeLinesF  string `json:"CodeLinesF"`
	// TopLanguages are the repository's largest languages, biggest first, excluding the
	// languages held out of its totals. Empty when no by-language result file was found.
	TopLanguages []LanguageShare `json:"TopLanguages,omitempty"`
	// LanguageChips are the languages its row on the results page offers a switch for -
	// see RankLanguageChips. Page-only, so not written to the reports.
	LanguageChips []LanguageShare `json:"-"`
	// Languages names every language the repository has, sorted, and LanguageLines holds
	// each one's code lines - some have none, e.g. files holding only comments. Page-only.
	Languages     []string       `json:"-"`
	LanguageLines map[string]int `json:"-"`
	// Deselected marks a row excluded from the totals. Set only on the results page's
	// table view, where counted and deselected rows are interleaved so a deselected
	// repository keeps its ranked position.
	Deselected bool `json:"Deselected,omitempty"`
}

// CountsCode reports whether a language contributes code lines to the repository. A
// language with none cannot keep a repository counting anything, whatever its switch says.
func (r RepositoryData) CountsCode(language string) bool {
	return r.LanguageLines[language] > 0
}

// PrimaryLanguage returns the repository's largest language, or "" when unknown.
func (r RepositoryData) PrimaryLanguage() string {
	if len(r.TopLanguages) == 0 {
		return ""
	}
	return r.TopLanguages[0].Language
}

// AnalysisResult represents the structure of analysis result files
type AnalysisResult struct {
	NumRepositories int             `json:"NumRepositories"`
	ProjectBranches []ProjectBranch `json:"ProjectBranches"`
}

// ProjectBranch represents a project branch with repository information
type ProjectBranch struct {
	Org          string `json:"Org"`
	ProjectKey   string `json:"ProjectKey"`
	RepoSlug     string `json:"RepoSlug"`
	MainBranch   string `json:"MainBranch"`
	SizeRepo     string `json:"SizeRepo"`
	TotalCommits int    `json:"TotalCommits"`
}

// RepositorySummaryReport contains summary data and repositories. Every Total
// covers Repositories only; repositories the user deselected on the results page are
// listed separately in Deselected, with their own totals, so a filtered report still
// shows what was left out and what the unfiltered figures were.
type RepositorySummaryReport struct {
	TotalRepositories int              `json:"TotalRepositories"`
	TotalLines        int              `json:"TotalLines"`
	TotalBlankLines   int              `json:"TotalBlankLines"`
	TotalComments     int              `json:"TotalComments"`
	TotalCodeLines    int              `json:"TotalCodeLines"`
	TotalLinesF       string           `json:"TotalLinesF"`
	TotalBlankLinesF  string           `json:"TotalBlankLinesF"`
	TotalCommentsF    string           `json:"TotalCommentsF"`
	TotalCodeLinesF   string           `json:"TotalCodeLinesF"`
	Repositories      []RepositoryData `json:"Repositories"`
	// ExcludedNote says which languages every code-line figure above leaves out.
	ExcludedNote string `json:"ExcludedNote"`

	DeselectedRepositories int              `json:"DeselectedRepositories,omitempty"`
	DeselectedCodeLines    int              `json:"DeselectedCodeLines,omitempty"`
	DeselectedCodeLinesF   string           `json:"DeselectedCodeLinesF,omitempty"`
	Deselected             []RepositoryData `json:"Deselected,omitempty"`
}

// isMainBranch checks if a branch name is a main/default branch
func isMainBranch(branchName string) bool {
	mainBranches := []string{"main", "master", "develop", "development", "default"}
	for _, main := range mainBranches {
		if branchName == main {
			return true
		}
	}
	return false
}

// detectPlatformAndReadAnalysis reports which platform produced the results and returns
// the raw inventory. Delegates to the shared platform table; kept as a name the existing
// tests exercise.
func detectPlatformAndReadAnalysis() (string, []byte, error) {
	spec, data, err := DetectPlatform(resultsDirName)
	return spec.Name, data, err
}

// getFirstPartForPlatform returns the leading component of a result file name for a
// platform. Delegates to the shared platform table.
func getFirstPartForPlatform(platform string, branch ProjectBranch, repoSlug string) string {
	spec, ok := PlatformSpecFor(platform)
	if !ok {
		return repoSlug
	}
	return spec.FirstPart(branch)
}

// getRepositoryData collects all repository data from the result files. The layout logic
// lives in repository_reader.go so the results page and these reports cannot disagree
// about it.
func getRepositoryData() ([]RepositoryData, error) {
	return ReadRepositoryData(resultsDirName)
}

// PartitionDeselected splits repositories into those still counted and those the
// user deselected on the results page, renumbering each group from 1 so both render
// as standalone tables. Order within each group is preserved.
func PartitionDeselected(repositories []RepositoryData, deselected DeselectionSet) (kept, removed []RepositoryData) {
	for _, repo := range repositories {
		if deselected.Contains(repo.Key) {
			removed = append(removed, repo)
		} else {
			kept = append(kept, repo)
		}
	}
	for i := range kept {
		kept[i].Number = i + 1
	}
	for i := range removed {
		removed[i].Number = i + 1
	}
	return kept, removed
}

// DeselectedRecords converts repositories into the persisted deselection records
// used by the reports to describe what was removed.
func DeselectedRecords(repositories []RepositoryData) []DeselectedRepo {
	records := make([]DeselectedRepo, 0, len(repositories))
	for _, repo := range repositories {
		records = append(records, DeselectedRepo{
			Key:    repo.Key,
			Org:    repo.Org,
			Repo:   repo.Repository,
			Branch: repo.Branch,
		})
	}
	return records
}

// generateReportWithErrorHandling generates a report and handles errors consistently
func generateReportWithErrorHandling(reportType, filePath string, generateFunc func() error) {
	loggers := NewLogger()
	if err := generateFunc(); err != nil {
		loggers.Errorf("❌ Error generating %s report: %v", reportType, err)
	} else {
		loggers.Infof("✅ Repository summary %s report exported to %s", reportType, filePath)
	}
}

// createReportFilePaths creates the file paths for all report types
func createReportFilePaths(directory string) (csvPath, jsonPath string) {
	baseOutputPath := filepath.Join(directory, "byfile-report")
	csvPath = filepath.Join(baseOutputPath, "csv-report")
	jsonPath = baseOutputPath
	return
}

// legacySummaryPDFPath is where earlier versions wrote a repository summary PDF. It is no
// longer produced - the CSV and JSON carry the same table, and the global report the
// headline figures - so a copy left in a result set is removed rather than zipped and
// handed on as if it were current.
func legacySummaryPDFPath(directory string) string {
	return filepath.Join(directory, "byfile-report", "pdf-report", "repository_summary.pdf")
}

// RemoveLegacySummaryPDF deletes the repository summary PDF an earlier version left under
// directory, if any. The per-repository PDFs beside it are kept.
func RemoveLegacySummaryPDF(directory string) error {
	if err := os.Remove(legacySummaryPDFPath(directory)); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// calculateTotals calculates summary totals from repositories
func calculateTotals(repositories []RepositoryData) (totalLines, totalBlankLines, totalComments, totalCodeLines int) {
	for _, repo := range repositories {
		totalLines += repo.Lines
		totalBlankLines += repo.BlankLines
		totalComments += repo.Comments
		totalCodeLines += repo.CodeLines
	}
	return
}

// generateRepositoryCSVReport creates a CSV report of all repositories
func generateRepositoryCSVReport(summary *RepositorySummaryReport, outputPath string) error {
	const codeLinesHeader = "Code Lines"

	// Create CSV file
	filePath := filepath.Join(outputPath, "repository_summary.csv")
	file, err := os.Create(filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	// Write header. The top languages occupy fixed columns rather than one packed cell
	// so a spreadsheet can sort or pivot on them.
	header := []string{"#", "Repository", "Branch", "Lines", "Blank Lines", "Comments", codeLinesHeader}
	for i := 1; i <= TopLanguagesShown; i++ {
		header = append(header, fmt.Sprintf("Language %d", i), fmt.Sprintf("Language %d Code Lines", i))
	}
	writer.Write(header)

	// Write repository data
	for _, repo := range summary.Repositories {
		writer.Write(repositoryCSVRow(repo))
	}

	// Write totals row
	totalRow := []string{
		"TOTAL",
		fmt.Sprintf("%d repositories", summary.TotalRepositories),
		"",
		strconv.Itoa(summary.TotalLines),
		strconv.Itoa(summary.TotalBlankLines),
		strconv.Itoa(summary.TotalComments),
		strconv.Itoa(summary.TotalCodeLines),
	}
	writer.Write(totalRow)

	// Deselected repositories follow the totals, flagged in the first column so a
	// spreadsheet filter separates them and they can never be mistaken for counted
	// rows. Omitted entirely when the selection is untouched.
	for _, repo := range summary.Deselected {
		row := repositoryCSVRow(repo)
		row[0] = "DESELECTED"
		writer.Write(row)
	}

	return nil
}

// repositoryCSVRow builds one repository's CSV row, padding the language columns so
// every row has the same width regardless of how many languages a repository has.
func repositoryCSVRow(repo RepositoryData) []string {
	row := []string{
		strconv.Itoa(repo.Number),
		repo.Repository,
		repo.Branch,
		strconv.Itoa(repo.Lines),
		strconv.Itoa(repo.BlankLines),
		strconv.Itoa(repo.Comments),
		strconv.Itoa(repo.CodeLines),
	}
	for i := 0; i < TopLanguagesShown; i++ {
		if i < len(repo.TopLanguages) {
			row = append(row, repo.TopLanguages[i].Language, strconv.Itoa(repo.TopLanguages[i].CodeLines))
		} else {
			row = append(row, "", "")
		}
	}
	return row
}

// generateRepositoryJSONReport creates a JSON report of all repositories
func generateRepositoryJSONReport(summary *RepositorySummaryReport, outputPath string) error {
	// Marshal to JSON with indentation
	jsonData, err := json.MarshalIndent(summary, "", "  ")
	if err != nil {
		return err
	}

	// Write to file
	filePath := filepath.Join(outputPath, "repository_summary.json")
	return os.WriteFile(filePath, jsonData, 0644)
}

// SummaryReportOptions selects which repositories the summary reports cover and where
// they are written, so the same generator can produce both the full-scan reports and
// reports reflecting the user's current selection without either overwriting the other.
type SummaryReportOptions struct {
	// Deselected repositories to leave out of the totals. Empty means the full scan.
	Deselected DeselectionSet
	// Excluded languages to leave out of every repository's code lines. The zero value
	// is the default set.
	Excluded LanguageExclusion
	// OutputDir is the base directory the byfile-report tree is written under. Empty
	// means the directory passed to the generator.
	OutputDir string
}

// GenerateRepositorySummaryReports generates CSV, JSON, and PDF reports for all
// repositories, applying any selection persisted under directory. Kept for existing
// callers (an analysis run); use GenerateRepositorySummaryReportsWith to control the
// selection and output location explicitly.
func GenerateRepositorySummaryReports(directory string) error {
	return GenerateRepositorySummaryReportsWith(directory, SummaryReportOptions{
		Deselected: LoadDeselectionSet(directory),
		Excluded:   LoadLanguageExclusion(directory),
	})
}

// GenerateRepositorySummaryReportsWith generates the summary reports for an explicit
// selection and output location. An empty selection always reproduces the full scan.
func GenerateRepositorySummaryReportsWith(directory string, opts SummaryReportOptions) error {
	loggers := NewLogger()

	if opts.OutputDir == "" {
		opts.OutputDir = directory
	}

	// Get repository data
	repositories, err := ReadRepositoryDataWith(resultsDirName, opts.Excluded)
	if err != nil {
		// If we can't find analysis result files, this might be the File platform
		// or no repositories were analyzed. Skip repository summary generation.
		loggers.Infof("ℹ️ Skipping repository summary reports: %v", err)
		return nil
	}

	if len(repositories) == 0 {
		loggers.Infof("⚠️ No repositories found for summary report generation")
		return nil
	}

	// Repositories the user removed from the totals on the results page. An empty
	// selection means every repository below is counted.
	repositories, deselectedRepos := PartitionDeselected(repositories, opts.Deselected)

	// Calculate totals using helper function
	totalLines, totalBlankLines, totalComments, totalCodeLines := calculateTotals(repositories)

	// Create summary report structure
	summary := &RepositorySummaryReport{
		TotalRepositories: len(repositories),
		TotalLines:        totalLines,
		TotalBlankLines:   totalBlankLines,
		TotalComments:     totalComments,
		TotalCodeLines:    totalCodeLines,
		TotalLinesF:       FormatCodeLines(float64(totalLines)),
		TotalBlankLinesF:  FormatCodeLines(float64(totalBlankLines)),
		TotalCommentsF:    FormatCodeLines(float64(totalComments)),
		TotalCodeLinesF:   FormatCodeLines(float64(totalCodeLines)),
		Repositories:      repositories,
		ExcludedNote:      opts.Excluded.Note(),
	}

	// Left entirely absent when the selection is untouched, so an unfiltered report
	// is byte-identical to one produced before this feature existed.
	if len(deselectedRepos) > 0 {
		_, _, _, deselectedCodeLines := calculateTotals(deselectedRepos)
		summary.DeselectedRepositories = len(deselectedRepos)
		summary.DeselectedCodeLines = deselectedCodeLines
		summary.DeselectedCodeLinesF = FormatCodeLines(float64(deselectedCodeLines))
		summary.Deselected = deselectedRepos
	}

	// Get output paths using helper function
	csvOutputPath, jsonOutputPath := createReportFilePaths(opts.OutputDir)
	for _, dir := range []string{csvOutputPath, jsonOutputPath} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			loggers.Errorf("❌ Error creating report directory %s: %v", dir, err)
			return err
		}
	}

	// Generate reports with consistent error handling
	csvFilePath := filepath.Join(csvOutputPath, "repository_summary.csv")
	generateReportWithErrorHandling("CSV", csvFilePath, func() error {
		return generateRepositoryCSVReport(summary, csvOutputPath)
	})

	jsonFilePath := filepath.Join(jsonOutputPath, "repository_summary.json")
	generateReportWithErrorHandling("JSON", jsonFilePath, func() error {
		return generateRepositoryJSONReport(summary, jsonOutputPath)
	})

	if err := RemoveLegacySummaryPDF(opts.OutputDir); err != nil {
		loggers.Errorf("❌ Error removing the obsolete repository summary PDF: %v", err)
	}

	loggers.Infof("✅ Repository summary reports generated successfully")
	return nil
}
