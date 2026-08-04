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

	"github.com/jung-kurt/gofpdf"
)

// TopLanguagesShown is how many of a repository's largest languages are surfaced
// alongside its totals.
const TopLanguagesShown = 3

// LanguageShare is one language's contribution to a single repository.
type LanguageShare struct {
	Language   string `json:"Language"`
	CodeLines  int    `json:"CodeLines"`
	CodeLinesF string `json:"CodeLinesF"`
}

// RankTopLanguages returns a repository's largest languages, biggest first, capped at
// limit.
//
// The language held out of the headline total (JSON) is excluded, matching the CodeLines
// figure these languages appear next to. Including it would let a repository show
// "JSON 240K" beside a code-line count that deliberately does not contain those lines.
//
// Ties break on language name so the output is stable across runs.
func RankTopLanguages(languages []LanguageShare, limit int) []LanguageShare {
	ranked := make([]LanguageShare, 0, len(languages))
	for _, lang := range languages {
		name := strings.TrimSpace(lang.Language)
		if name == "" || name == LanguageExcludedFromTotalLOC || lang.CodeLines <= 0 {
			continue
		}
		ranked = append(ranked, LanguageShare{
			Language:   name,
			CodeLines:  lang.CodeLines,
			CodeLinesF: FormatCodeLines(float64(lang.CodeLines)),
		})
	}

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
	// language held out of the totals. Empty when no by-language result file was found.
	TopLanguages []LanguageShare `json:"TopLanguages,omitempty"`
}

// PrimaryLanguage returns the repository's largest language, or "" when unknown.
func (r RepositoryData) PrimaryLanguage() string {
	if len(r.TopLanguages) == 0 {
		return ""
	}
	return r.TopLanguages[0].Language
}

// primaryLanguageCell renders the main language with its own code lines, e.g.
// "C++ 761.37K", so the report shows how much of the repository that language accounts for
// rather than just naming it. Fitted to the column width so the line count survives
// truncation. A dash when no language was recorded.
func (r RepositoryData) primaryLanguageCell(pdf *gofpdf.Fpdf, tr func(string) string, w float64) string {
	if len(r.TopLanguages) == 0 {
		return "-"
	}
	return fitLabelWithValue(pdf, tr(r.TopLanguages[0].Language), tr(r.TopLanguages[0].CodeLinesF), w)
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

// detectPlatformAndReadAnalysis detects the platform and reads the analysis file.
//
// The candidates are an ordered slice, not a map: when a Results directory holds
// inventories from more than one platform (a re-scan against a different platform
// without clearing Results), map iteration order would make the pick random and the
// summary reports could describe a different platform than the results page, which
// resolves the same ambiguity in this same order.
func detectPlatformAndReadAnalysis() (string, []byte, error) {
	// Platform keys must match the cases in getFirstPartForPlatform, since the
	// returned platform selects the result-file naming convention.
	platformFiles := []struct{ platform, fileName string }{
		{"github", "Results/config/analysis_result_github.json"},
		{"gitlab", "Results/config/analysis_result_gitlab.json"},
		{"bitbucket", "Results/config/analysis_result_bitbucket.json"},
		{"bitbucketdc", "Results/config/analysis_result_bitbucket_dc.json"},
		{"azure", "Results/config/analysis_result_azure.json"},
		{"file", "Results/config/analysis_result_file.json"},
	}

	for _, candidate := range platformFiles {
		if data, err := os.ReadFile(candidate.fileName); err == nil {
			return candidate.platform, data, nil
		}
	}

	return "", nil, fmt.Errorf("no analysis result file found for any supported platform")
}

// getFirstPartForPlatform returns the first part of filename for different platforms
func getFirstPartForPlatform(platform string, branch ProjectBranch, repoSlug string) string {
	switch platform {
	case "azure":
		return branch.ProjectKey
	case "bitbucketdc":
		return branch.ProjectKey
	case "bitbucket":
		return branch.ProjectKey
	case "gitlab":
		return branch.Org
	case "github":
		return branch.Org
	default:
		return repoSlug
	}
}

// getRepositoryData collects all repository data from byfile reports
func getRepositoryData() ([]RepositoryData, error) {
	var repositories []RepositoryData

	// Detect platform and read analysis results
	platform, analysisFile, err := detectPlatformAndReadAnalysis()
	if err != nil {
		return nil, fmt.Errorf("error reading analysis result file: %v", err)
	}

	var analysisResult AnalysisResult
	err = json.Unmarshal(analysisFile, &analysisResult)
	if err != nil {
		return nil, fmt.Errorf("error decoding JSON analysis result file for platform %s: %v", platform, err)
	}

	// Group by repository to avoid duplicate entries (needed for --all-branches mode)
	repoMap := make(map[string]ProjectBranch)

	// First pass: Group by repository and prefer main/master/default branches
	for _, branch := range analysisResult.ProjectBranches {
		repoKey := branch.RepoSlug

		// If we haven't seen this repo, or if this is a main branch, use it
		if existing, exists := repoMap[repoKey]; !exists || isMainBranch(branch.MainBranch) {
			// Only override if current is main branch, or existing is not main branch
			if !exists || isMainBranch(branch.MainBranch) || !isMainBranch(existing.MainBranch) {
				repoMap[repoKey] = branch
			}
		}
	}

	// Process each unique repository (now showing only one branch per repository)
	i := 0
	for _, branch := range repoMap {
		i++
		// Construct file paths using platform-specific naming.
		// File mode uses a shorter pattern (no project key or branch suffix)
		// because goloc names files as Result_<dirname>_byfile.json in that mode.
		// Components are sanitized before being interpolated, matching the results
		// page. Without it, a GitLab subgroup or a `release/1.0` branch turns the file
		// name into a nested path and this lookup misses a file the page finds.
		firstPart := getFirstPartForPlatform(platform, branch, branch.RepoSlug)
		var byfilePath, byLanguagePath, org string
		if platform == "file" {
			slug := SanitizeResultComponent(branch.RepoSlug)
			byfilePath = fmt.Sprintf("Results/byfile-report/Result_%s_byfile.json", slug)
			byLanguagePath = fmt.Sprintf("Results/bylanguage-report/Result_%s.json", slug)
		} else {
			org = firstPart
			byfilePath = fmt.Sprintf("Results/byfile-report/Result_%s__%s__%s_byfile.json",
				SanitizeResultComponent(firstPart),
				SanitizeResultComponent(branch.RepoSlug),
				SanitizeResultComponent(branch.MainBranch))
			byLanguagePath = fmt.Sprintf("Results/bylanguage-report/Result_%s__%s__%s.json",
				SanitizeResultComponent(firstPart),
				SanitizeResultComponent(branch.RepoSlug),
				SanitizeResultComponent(branch.MainBranch))
		}

		// Built from the inventory fields through the shared key function rather than
		// read back out of the file path. The page and these reports construct their
		// paths separately, so a key recovered from a path inherits every difference
		// between them — which is how a repository could be deselected on the page and
		// still counted here.
		key := DeselectionKeyForRepo(platform, firstPart, branch.RepoSlug, branch.MainBranch)

		// Read the byfile report
		fileData, err := os.ReadFile(byfilePath)
		if err != nil {
			// Skip this repository if file doesn't exist
			continue
		}

		// Parse the JSON structure
		var reportData struct {
			TotalLines      int `json:"TotalLines"`
			TotalBlankLines int `json:"TotalBlankLines"`
			TotalComments   int `json:"TotalComments"`
			TotalCodeLines  int `json:"TotalCodeLines"`
		}

		err = json.Unmarshal(fileData, &reportData)
		if err != nil {
			// Skip this repository if JSON is invalid
			continue
		}

		// Code lines for report total: exclude JSON to match SonarQube behavior. The same
		// parse yields the repository's largest languages — the per-language list is
		// already in hand here, so surfacing the top few costs no extra read.
		codeLinesForReport := reportData.TotalCodeLines
		var topLanguages []LanguageShare
		if langData, err := os.ReadFile(byLanguagePath); err == nil {
			var byLang struct {
				Results []LanguageShare `json:"Results"`
			}
			if json.Unmarshal(langData, &byLang) == nil {
				for _, r := range byLang.Results {
					if strings.TrimSpace(r.Language) == LanguageExcludedFromTotalLOC {
						codeLinesForReport = reportData.TotalCodeLines - r.CodeLines
						break
					}
				}
				topLanguages = RankTopLanguages(byLang.Results, TopLanguagesShown)
			}
		}

		// Create repository data entry (CodeLines excludes JSON for report total)
		repo := RepositoryData{
			Number:       i,
			Key:          key,
			Repository:   branch.RepoSlug,
			Org:          org,
			Branch:       branch.MainBranch,
			Lines:        reportData.TotalLines,
			BlankLines:   reportData.TotalBlankLines,
			Comments:     reportData.TotalComments,
			CodeLines:    codeLinesForReport,
			LinesF:       FormatCodeLines(float64(reportData.TotalLines)),
			BlankLinesF:  FormatCodeLines(float64(reportData.TotalBlankLines)),
			CommentsF:    FormatCodeLines(float64(reportData.TotalComments)),
			CodeLinesF:   FormatCodeLines(float64(codeLinesForReport)),
			TopLanguages: topLanguages,
		}

		repositories = append(repositories, repo)
	}

	// Sort repositories by Code Lines (descending) by default
	sort.Slice(repositories, func(i, j int) bool {
		return repositories[i].CodeLines > repositories[j].CodeLines
	})

	// Update numbers after sorting
	for i := range repositories {
		repositories[i].Number = i + 1
	}

	return repositories, nil
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

// truncateText truncates text to maxLength and adds "..." if needed
func truncateText(text string, maxLength int) string {
	if len(text) > maxLength {
		return text[:maxLength-3] + "..."
	}
	return text
}

// Column widths for the repository table, summing to the 190mm content width. Repository
// and Branch each gave up 8mm to make room for the language column; three languages will
// not fit at this width, so the PDF shows the primary one and the CSV/JSON carry the rest.
const (
	colPDFNum      = 10.0
	colPDFRepo     = 42.0
	colPDFBranch   = 22.0
	colPDFLanguage = 28.0
	colPDFMetric   = 22.0 // Lines, Comments, Blank, Code Lines
)

// createPDFTableHeader creates the standard table header for repository reports
func createPDFTableHeader(pdf *gofpdf.Fpdf, codeLinesHeader string) {
	pdf.SetFont("Arial", "B", 8)
	pdf.SetFillColor(51, 153, 255)
	pdf.CellFormat(colPDFNum, 8, "#", "1", 0, "C", true, 0, "")
	pdf.CellFormat(colPDFRepo, 8, "Repository", "1", 0, "C", true, 0, "")
	pdf.CellFormat(colPDFBranch, 8, "Branch", "1", 0, "C", true, 0, "")
	pdf.CellFormat(colPDFLanguage, 8, "Main Language", "1", 0, "C", true, 0, "")
	pdf.CellFormat(colPDFMetric, 8, "Lines", "1", 0, "C", true, 0, "")
	pdf.CellFormat(colPDFMetric, 8, "Comments", "1", 0, "C", true, 0, "")
	pdf.CellFormat(colPDFMetric, 8, "Blank", "1", 0, "C", true, 0, "")
	pdf.CellFormat(colPDFMetric, 8, codeLinesHeader, "1", 1, "C", true, 0, "")
}

// createRepositoryPDFRow creates a single row in the PDF table. tr converts UTF-8
// into the font's Windows-1252 encoding so accented repository/branch names render
// correctly instead of as mojibake; it is applied before truncation so the
// byte-based length limit matches the rendered single-byte characters.
func createRepositoryPDFRow(pdf *gofpdf.Fpdf, tr func(string) string, repo RepositoryData, fill bool) {
	repoName := truncateText(tr(repo.Repository), 22)
	branchName := truncateText(tr(repo.Branch), 12)

	// The main language with its own line count ("C++ 761.37K"); a dash when no
	// by-language result file was found, so an unknown reads as unknown rather than as an
	// empty cell. Fitted by measured width, shortening the language name rather than the
	// number: see fitLabelWithValue for why the figure must be the part that survives.
	language := repo.primaryLanguageCell(pdf, tr, colPDFLanguage)

	pdf.CellFormat(colPDFNum, 6, strconv.Itoa(repo.Number), "1", 0, "C", fill, 0, "")
	pdf.CellFormat(colPDFRepo, 6, repoName, "1", 0, "L", fill, 0, "")
	pdf.CellFormat(colPDFBranch, 6, branchName, "1", 0, "C", fill, 0, "")
	pdf.CellFormat(colPDFLanguage, 6, language, "1", 0, "L", fill, 0, "")
	pdf.CellFormat(colPDFMetric, 6, repo.LinesF, "1", 0, "R", fill, 0, "")
	pdf.CellFormat(colPDFMetric, 6, repo.CommentsF, "1", 0, "R", fill, 0, "")
	pdf.CellFormat(colPDFMetric, 6, repo.BlankLinesF, "1", 0, "R", fill, 0, "")
	pdf.CellFormat(colPDFMetric, 6, repo.CodeLinesF, "1", 1, "R", fill, 0, "")
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
func createReportFilePaths(directory string) (csvPath, jsonPath, pdfPath string) {
	baseOutputPath := filepath.Join(directory, "byfile-report")
	csvPath = filepath.Join(baseOutputPath, "csv-report")
	jsonPath = baseOutputPath
	pdfPath = filepath.Join(baseOutputPath, "pdf-report")
	return
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

// generateRepositoryPDFReport creates a PDF report of all repositories
func generateRepositoryPDFReport(summary *RepositorySummaryReport, outputPath string) error {
	const codeLinesHeader = "Code Lines"
	const (
		rowH         = 6.0
		pageH        = 297.0
		bottomMargin = 15.0
	)

	// Create PDF with manual page-break management to avoid blank pages caused
	// by the mismatch between header content height and a fixed row-count limit.
	pdf := gofpdf.New("P", "mm", "A4", "")
	pdf.SetAutoPageBreak(false, 0)
	pdf.AddPage()

	// gofpdf core fonts render text as Windows-1252, so UTF-8 repository/branch
	// names must be translated before drawing to avoid mojibake.
	tr := pdf.UnicodeTranslatorFromDescriptor("")

	// Add logo if it exists
	logoPath := GetLogoPath()
	if _, err := os.Stat(logoPath); err == nil {
		pdf.Image(logoPath, 10, 10, 50, 0, false, "", 0, "")
	}

	pdf.Ln(15)

	// Title
	pdf.SetFont("Arial", "B", 16)
	pdf.Cell(0, 10, "Repository Summary Report")
	pdf.Ln(15)

	// Summary section
	pdf.SetFont("Arial", "B", 12)
	pdf.SetFillColor(51, 153, 255)
	pdf.CellFormat(190, 8, "Summary", "1", 1, "C", true, 0, "")

	pdf.SetFont("Arial", "", 10)
	pdf.SetFillColor(220, 230, 241)

	summaryData := []string{
		fmt.Sprintf("Total Repositories: %d", summary.TotalRepositories),
		fmt.Sprintf("Total Lines: %s", summary.TotalLinesF),
		fmt.Sprintf("Total Code Lines: %s", summary.TotalCodeLinesF),
		fmt.Sprintf("Total Comments: %s", summary.TotalCommentsF),
		fmt.Sprintf("Total Blank Lines: %s", summary.TotalBlankLinesF),
		NoteExcludedFromTotal,
	}

	// Stated up front, next to the totals it changes, so a reader cannot take the
	// figures above for the whole scan.
	if summary.DeselectedRepositories > 0 {
		summaryData = append(summaryData, fmt.Sprintf(
			"Deselected by user: %d repositories (%s code lines) — excluded from the totals above",
			summary.DeselectedRepositories, summary.DeselectedCodeLinesF))
	}

	for _, data := range summaryData {
		pdf.CellFormat(190, 6, tr(data), "1", 1, "L", true, 0, "")
	}

	pdf.Ln(5)

	// Initial table header
	createPDFTableHeader(pdf, codeLinesHeader)

	// Table data — page breaks triggered by remaining space, not row count
	pdf.SetFont("Arial", "", 8)
	pdf.SetFillColor(240, 240, 240)

	rowCount := 0
	for _, repo := range summary.Repositories {
		// Break before the row would overflow the page
		if pdf.GetY()+rowH > pageH-bottomMargin {
			pdf.AddPage()
			createPDFTableHeader(pdf, codeLinesHeader)
			pdf.SetFont("Arial", "", 8)
			pdf.SetFillColor(240, 240, 240)
			rowCount = 0
		}

		fill := rowCount%2 == 0
		createRepositoryPDFRow(pdf, tr, repo, fill)
		rowCount++
	}

	renderDeselectedTable(pdf, tr, summary, codeLinesHeader, rowH, pageH, bottomMargin)

	// Save PDF
	filePath := filepath.Join(outputPath, "repository_summary.pdf")
	return pdf.OutputFileAndClose(filePath)
}

// renderDeselectedTable appends the repositories the user removed from the totals as
// a separate table after the counted ones, on its own page so the two can never be
// read as one list. It renders nothing when the selection is untouched.
func renderDeselectedTable(pdf *gofpdf.Fpdf, tr func(string) string, summary *RepositorySummaryReport,
	codeLinesHeader string, rowH, pageH, bottomMargin float64) {
	if len(summary.Deselected) == 0 {
		return
	}

	pdf.AddPage()

	pdf.SetFont("Arial", "B", 13)
	pdf.Cell(0, 10, tr(fmt.Sprintf("Deselected Repositories (%d)", len(summary.Deselected))))
	pdf.Ln(10)

	pdf.SetFont("Arial", "", 9)
	pdf.SetFillColor(235, 238, 243)
	pdf.MultiCell(190, 5, tr(fmt.Sprintf(
		"Analyzed but excluded from every total in this report by user selection on the results page. "+
			"Their combined %s code lines are not included in the figures on page 1.",
		summary.DeselectedCodeLinesF)), "1", "L", true)
	pdf.Ln(4)

	createPDFTableHeader(pdf, codeLinesHeader)
	pdf.SetFont("Arial", "", 8)
	pdf.SetFillColor(240, 240, 240)

	rowCount := 0
	for _, repo := range summary.Deselected {
		if pdf.GetY()+rowH > pageH-bottomMargin {
			pdf.AddPage()
			createPDFTableHeader(pdf, codeLinesHeader)
			pdf.SetFont("Arial", "", 8)
			pdf.SetFillColor(240, 240, 240)
			rowCount = 0
		}
		createRepositoryPDFRow(pdf, tr, repo, rowCount%2 == 0)
		rowCount++
	}
}

// SummaryReportOptions selects which repositories the summary reports cover and where
// they are written, so the same generator can produce both the full-scan reports and
// reports reflecting the user's current selection without either overwriting the other.
type SummaryReportOptions struct {
	// Deselected repositories to leave out of the totals. Empty means the full scan.
	Deselected DeselectionSet
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
	repositories, err := getRepositoryData()
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
	csvOutputPath, jsonOutputPath, pdfOutputPath := createReportFilePaths(opts.OutputDir)
	for _, dir := range []string{csvOutputPath, jsonOutputPath, pdfOutputPath} {
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

	pdfFilePath := filepath.Join(pdfOutputPath, "repository_summary.pdf")
	generateReportWithErrorHandling("PDF", pdfFilePath, func() error {
		return generateRepositoryPDFReport(summary, pdfOutputPath)
	})

	loggers.Infof("✅ Repository summary reports generated successfully")
	return nil
}
