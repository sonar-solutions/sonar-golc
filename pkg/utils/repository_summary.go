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

// RepositoryData represents a single repository's data for summary reports
type RepositoryData struct {
	Number      int    `json:"Number"`
	Repository  string `json:"Repository"`
	Branch      string `json:"Branch"`
	Lines       int    `json:"Lines"`
	BlankLines  int    `json:"BlankLines"`
	Comments    int    `json:"Comments"`
	CodeLines   int    `json:"CodeLines"`
	LinesF      string `json:"LinesF"`
	BlankLinesF string `json:"BlankLinesF"`
	CommentsF   string `json:"CommentsF"`
	CodeLinesF  string `json:"CodeLinesF"`
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

// RepositorySummaryReport contains summary data and repositories
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

// detectPlatformAndReadAnalysis detects the platform and reads the analysis file
func detectPlatformAndReadAnalysis() (string, []byte, error) {
	// Define platform-specific filename patterns
	platformFiles := map[string]string{
		"github":      "Results/config/analysis_result_github.json",
		"azure":       "Results/config/analysis_result_azure.json",
		"bitbucket":   "Results/config/analysis_result_bitbucket.json",
		"gitlab":      "Results/config/analysis_result_gitlab.json",
		"bitbucketdc": "Results/config/analysis_repos_bitbucketdc.json", // Different naming pattern
	}

	for platform, fileName := range platformFiles {
		if data, err := os.ReadFile(fileName); err == nil {
			return platform, data, nil
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
		// Construct filename for byfile report using platform-specific logic
		firstPart := getFirstPartForPlatform(platform, branch, branch.RepoSlug)
		fileName := fmt.Sprintf("Results/byfile-report/Result_%s_%s_%s_byfile.json",
			firstPart, branch.RepoSlug, branch.MainBranch)

		// Read the byfile report
		fileData, err := os.ReadFile(fileName)
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

		// Code lines for report total: exclude JSON to match SonarQube behavior
		codeLinesForReport := reportData.TotalCodeLines
		byLanguagePath := fmt.Sprintf("Results/bylanguage-report/Result_%s_%s_%s.json",
			firstPart, branch.RepoSlug, branch.MainBranch)
		if langData, err := os.ReadFile(byLanguagePath); err == nil {
			var byLang struct {
				Results []struct {
					Language  string `json:"Language"`
					CodeLines int    `json:"CodeLines"`
				} `json:"Results"`
			}
			if json.Unmarshal(langData, &byLang) == nil {
				for _, r := range byLang.Results {
					if strings.TrimSpace(r.Language) == LanguageExcludedFromTotalLOC {
						codeLinesForReport = reportData.TotalCodeLines - r.CodeLines
						break
					}
				}
			}
		}

		// Create repository data entry (CodeLines excludes JSON for report total)
		repo := RepositoryData{
			Number:      i,
			Repository:  branch.RepoSlug,
			Branch:      branch.MainBranch,
			Lines:       reportData.TotalLines,
			BlankLines:  reportData.TotalBlankLines,
			Comments:    reportData.TotalComments,
			CodeLines:   codeLinesForReport,
			LinesF:      FormatCodeLines(float64(reportData.TotalLines)),
			BlankLinesF: FormatCodeLines(float64(reportData.TotalBlankLines)),
			CommentsF:   FormatCodeLines(float64(reportData.TotalComments)),
			CodeLinesF:  FormatCodeLines(float64(codeLinesForReport)),
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

// truncateText truncates text to maxLength and adds "..." if needed
func truncateText(text string, maxLength int) string {
	if len(text) > maxLength {
		return text[:maxLength-3] + "..."
	}
	return text
}

// createPDFTableHeader creates the standard table header for repository reports
func createPDFTableHeader(pdf *gofpdf.Fpdf, codeLinesHeader string) {
	pdf.SetFont("Arial", "B", 9)
	pdf.SetFillColor(51, 153, 255)
	pdf.CellFormat(10, 8, "#", "1", 0, "C", true, 0, "")
	pdf.CellFormat(50, 8, "Repository", "1", 0, "C", true, 0, "")
	pdf.CellFormat(30, 8, "Branch", "1", 0, "C", true, 0, "")
	pdf.CellFormat(25, 8, "Lines", "1", 0, "C", true, 0, "")
	pdf.CellFormat(25, 8, "Comments", "1", 0, "C", true, 0, "")
	pdf.CellFormat(25, 8, "Blank", "1", 0, "C", true, 0, "")
	pdf.CellFormat(25, 8, codeLinesHeader, "1", 1, "C", true, 0, "")
}

// createRepositoryPDFRow creates a single row in the PDF table
func createRepositoryPDFRow(pdf *gofpdf.Fpdf, repo RepositoryData, fill bool) {
	repoName := truncateText(repo.Repository, 20)
	branchName := truncateText(repo.Branch, 12)

	pdf.CellFormat(10, 6, strconv.Itoa(repo.Number), "1", 0, "C", fill, 0, "")
	pdf.CellFormat(50, 6, repoName, "1", 0, "L", fill, 0, "")
	pdf.CellFormat(30, 6, branchName, "1", 0, "C", fill, 0, "")
	pdf.CellFormat(25, 6, repo.LinesF, "1", 0, "R", fill, 0, "")
	pdf.CellFormat(25, 6, repo.CommentsF, "1", 0, "R", fill, 0, "")
	pdf.CellFormat(25, 6, repo.BlankLinesF, "1", 0, "R", fill, 0, "")
	pdf.CellFormat(25, 6, repo.CodeLinesF, "1", 1, "R", fill, 0, "")
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

	// Write header
	header := []string{"#", "Repository", "Branch", "Lines", "Blank Lines", "Comments", codeLinesHeader}
	writer.Write(header)

	// Write repository data
	for _, repo := range summary.Repositories {
		row := []string{
			strconv.Itoa(repo.Number),
			repo.Repository,
			repo.Branch,
			strconv.Itoa(repo.Lines),
			strconv.Itoa(repo.BlankLines),
			strconv.Itoa(repo.Comments),
			strconv.Itoa(repo.CodeLines),
		}
		writer.Write(row)
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

	return nil
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

	// Add logo if it exists
	logoPath := "imgs/Logob.png"
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

	for _, data := range summaryData {
		pdf.CellFormat(190, 6, data, "1", 1, "L", true, 0, "")
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
		createRepositoryPDFRow(pdf, repo, fill)
		rowCount++
	}

	// Save PDF
	filePath := filepath.Join(outputPath, "repository_summary.pdf")
	return pdf.OutputFileAndClose(filePath)
}

// GenerateRepositorySummaryReports generates CSV, JSON, and PDF reports for all repositories
func GenerateRepositorySummaryReports(directory string) error {
	loggers := NewLogger()

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

	// Get output paths using helper function
	csvOutputPath, jsonOutputPath, pdfOutputPath := createReportFilePaths(directory)

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
