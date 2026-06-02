package pdf

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/SonarSource-Demos/sonar-golc/pkg/sorter"
	"github.com/SonarSource-Demos/sonar-golc/pkg/utils"
	"github.com/jung-kurt/gofpdf"
)


type PdfReporter struct {
	OutputName string
	OutputPath string
}

// parseResultFileName parses a `Result_<Org>_<Repo>_<Branch>.json` (or matching
// PDF output name with optional `_byfile` suffix) and returns the components.
// Any of Org/Repo/Branch may itself contain '_' (GitLab group names, repo slugs,
// branch names), so we strip the known prefix/suffix and treat the first segment
// as Org, the last as Branch, and join everything in between as Repo. Returns
// ok=false when the name does not contain at least three segments.
func parseResultFileName(name string) (org, repo, branch string, ok bool) {
	if !strings.HasPrefix(name, "Result_") {
		return "", "", "", false
	}
	base := strings.TrimPrefix(name, "Result_")
	for _, suffix := range []string{".json", ".pdf"} {
		base = strings.TrimSuffix(base, suffix)
	}
	base = strings.TrimSuffix(base, "_byfile")
	parts := strings.Split(base, "_")
	if len(parts) < 3 {
		return "", "", "", false
	}
	return parts[0], strings.Join(parts[1:len(parts)-1], "_"), parts[len(parts)-1], true
}

type languageResult struct {
	Language   string
	Files      int
	Lines      int
	BlankLines int
	Comments   int
	CodeLines  int
}

type fileResult struct {
	File       string
	Lines      int
	BlankLines int
	Comments   int
	CodeLines  int
}

type report struct {
	TotalFiles      int
	TotalLines      int
	TotalBlankLines int
	TotalComments   int
	TotalCodeLines  int
	Results         interface{}
}

type JsonData struct {
	TotalLines      int       `json:"TotalLines"`
	TotalBlankLines int       `json:"TotalBlankLines"`
	TotalComments   int       `json:"TotalComments"`
	TotalCodeLines  int       `json:"TotalCodeLines"`
	Results1        []Result1 `json:"Results"`
}

type Result1 struct {
	File       string `json:"File"`
	Lines      int    `json:"Lines"`
	BlankLines int    `json:"BlankLines"`
	Comments   int    `json:"Comments"`
	CodeLines  int    `json:"CodeLines"`
}

func (p PdfReporter) GenerateReportByLanguage(summary *sorter.SortedSummary) error {
	pdfReport := &report{
		TotalFiles:      summary.TotalFiles,
		TotalLines:      summary.TotalLines,
		TotalBlankLines: summary.TotalBlankLines,
		TotalComments:   summary.TotalComments,
		TotalCodeLines:  summary.TotalCodeLines,
		Results:         []languageResult{},
	}

	for _, r := range summary.Results {
		pdfReport.Results = append(pdfReport.Results.([]languageResult), languageResult{
			Language:   r.Name,
			Files:      summary.FilesByLanguage[r.Name],
			Lines:      r.Lines,
			BlankLines: r.BlankLines,
			Comments:   r.Comments,
			CodeLines:  r.CodeLines,
		})
	}

	return p.writePdf(pdfReport)
}

func (p PdfReporter) GenerateReportByFile(summary *sorter.SortedSummary) error {
	pdfReport := &report{
		TotalLines:      summary.TotalLines,
		TotalBlankLines: summary.TotalBlankLines,
		TotalComments:   summary.TotalComments,
		TotalCodeLines:  summary.TotalCodeLines,
		Results:         []fileResult{},
	}

	for _, r := range summary.Results {
		cleanedName := utils.CleanFileName(r.Name)
		pdfReport.Results = append(pdfReport.Results.([]fileResult), fileResult{
			File:       cleanedName,
			Lines:      r.Lines,
			BlankLines: r.BlankLines,
			Comments:   r.Comments,
			CodeLines:  r.CodeLines,
		})
	}

	return p.writePdf(pdfReport)
}

func (p PdfReporter) writePdf(pdfReport *report) error {
	var branch string
	loggers := utils.NewLogger()
	outputName := strings.Replace(p.OutputName, "/", "_", -1)
	if !strings.HasSuffix(outputName, ".pdf") {
		outputName += ".pdf"
	}

	repoName := "unknown"
	branch = "main"
	if _, r, b, ok := parseResultFileName(outputName); ok {
		repoName = r
		branch = b
	}
	Title2 := "Repository Files Details: " + repoName + " for branch : " + branch

	path := filepath.Join(p.OutputPath+"/", outputName)
	pdf := gofpdf.New("P", "mm", "A4", "")
	const (
		widthFile      = 100 // Width for the "File" column
		widthColumns   = 24  // Width for other columns (Lines, Blank Lines, Comments, Code Lines)
		height         = 10  // Line height
		maxRowsPerPage = 19  // Adjust according to content and font
	)

	// First page with title, image and overall statistics
	pdf.AddPage()

	pdf.SetFont("Times", "B", 12)
	pdf.Cell(0, 10, "Analysis Report")
	pdf.Ln(20)

	if logoPath := utils.GetLogoPath(); logoPath != "" {
		pdf.Image(logoPath, 10, 20, 50, 0, false, "", 0, "")
	}

	pdf.Ln(10)

	// Titre
	pdf.SetFont("Times", "B", 12)
	pdf.Cell(0, 10, Title2)
	pdf.Ln(10)

	// Total Code Lines excluding JSON (SonarQube standard behavior)
	totalCodeLinesForReport := pdfReport.TotalCodeLines
	if fileResults, ok := pdfReport.Results.([]fileResult); ok {
		jsonCodeLines := 0
		for _, r := range fileResults {
			if strings.HasSuffix(strings.ToLower(r.File), ".json") {
				jsonCodeLines += r.CodeLines
			}
		}
		totalCodeLinesForReport = pdfReport.TotalCodeLines - jsonCodeLines
	}

	//Global statistics
	pdf.SetFont("Times", "", 10)
	pdf.Cell(0, 10, "Total Lines: "+strconv.Itoa(pdfReport.TotalLines))
	pdf.Ln(5)
	pdf.Cell(0, 10, "Total Blank Lines: "+strconv.Itoa(pdfReport.TotalBlankLines))
	pdf.Ln(5)
	pdf.Cell(0, 10, "Total Comments: "+strconv.Itoa(pdfReport.TotalComments))
	pdf.Ln(5)
	pdf.Cell(0, 10, "Total Code Lines: "+strconv.Itoa(totalCodeLinesForReport))
	pdf.Ln(5)
	pdf.SetFont("Times", "", 8)
	pdf.Cell(0, 8, "Note: "+utils.NoteExcludedFromTotal)
	pdf.Ln(10)

	// Table Headers
	pdf.SetFont("Times", "B", 10)
	pdf.Cell(widthFile, height, "File")
	pdf.Cell(widthColumns, height, "Lines")
	pdf.Cell(widthColumns, height, "Blank Lines")
	pdf.Cell(widthColumns, height, "Comments")
	pdf.Cell(widthColumns, height, "Code Lines")
	pdf.Ln(height)

	pdf.Line(10, pdf.GetY(), 200, pdf.GetY())
	pdf.SetFont("Times", "", 9)

	for i, result := range pdfReport.Results.([]fileResult) {
		if i%maxRowsPerPage == 0 && i > 0 {
			pdf.AddPage()
			pdf.SetFont("Times", "B", 10)
			pdf.Cell(widthFile, height, "File")
			pdf.Cell(widthColumns, height, "Lines")
			pdf.Cell(widthColumns, height, "Blank Lines")
			pdf.Cell(widthColumns, height, "Comments")
			pdf.Cell(widthColumns, height, "Code Lines")
			pdf.Ln(height)

			pdf.Line(10, pdf.GetY(), 200, pdf.GetY())
			pdf.SetFont("Times", "", 9)
		}

		if len(result.File) > 73 {

			pdf.Cell(widthFile, height, result.File[:73])
			pdf.Ln(height)
			pdf.Cell(widthFile, height, result.File[73:])
		} else {
			pdf.Cell(widthFile, height, result.File)
		}
		//pdf.Cell(widthFile, height, result.File)
		pdf.Cell(widthColumns, height, strconv.Itoa(result.Lines))
		pdf.Cell(widthColumns, height, strconv.Itoa(result.BlankLines))
		pdf.Cell(widthColumns, height, strconv.Itoa(result.Comments))
		pdf.Cell(widthColumns, height, strconv.Itoa(result.CodeLines))
		pdf.Ln(height)

		pdf.Line(10, pdf.GetY(), 200, pdf.GetY())
	}

	if err := pdf.OutputFileAndClose(path); err != nil {
		return err
	}

	loggers.Infof("\t✅ PDF report exported to %s", path)
	return nil
}

func (p PdfReporter) GenerateGlobalReportByFile() error {
	loggers := utils.NewLogger()
	dir := "./Results/byfile-report/"
	/*	pwd, err1 := os.Getwd()
		if err1 != nil {
			return fmt.Errorf("error getting current directory: %v", err1)
		}*/

	DestinationResultPDF := filepath.Join(p.OutputPath, "pdf-report", "GlobalReportByFile.pdf")
	pdf := gofpdf.New("P", "mm", "A4", "")
	const maxTablesPerPage = 7 // Maximum tables per page

	pdf.AddPage()

	pdf.SetFont("Times", "B", 12)
	pdf.Cell(0, 10, "Full Analysis Report")
	pdf.Ln(20)

	if logoPath := utils.GetLogoPath(); logoPath != "" {
		pdf.Image(logoPath, 10, 20, 50, 0, false, "", 0, "")
	}

	pdf.Ln(10)

	pdf.SetFont("Times", "B", 12)

	// Table Header
	pdf.CellFormat(0, 10, "Repository Gobal report", "", 1, "L", false, 0, "")
	pdf.SetFont("Times", "B", 12)

	// Counter for the number of tables added to the current page
	tableCount := 0

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if strings.HasPrefix(info.Name(), "Result_") && strings.HasSuffix(info.Name(), ".json") {
			fileContent, err := os.ReadFile(path)
			if err != nil {
				return err
			}

			var data JsonData
			if err := json.Unmarshal(fileContent, &data); err != nil {
				return err
			}

			if _, repoName, branch, ok := parseResultFileName(info.Name()); ok {
				// Add repository info
				pdf.SetFont("Times", "B", 10)
				pdf.CellFormat(0, 10, fmt.Sprintf("Repository: %s - Branch: %s", repoName, branch), "", 1, "", false, 0, "")

				// Set up table headers
				pdf.SetFont("Times", "B", 10)
				pdf.CellFormat(40, 10, "TotalLines", "1", 0, "C", false, 0, "")
				pdf.CellFormat(40, 10, "TotalBlankLines", "1", 0, "C", false, 0, "")
				pdf.CellFormat(40, 10, "TotalComments", "1", 0, "C", false, 0, "")
				pdf.CellFormat(40, 10, "TotalCodeLines", "1", 1, "C", false, 0, "")

				// Add values to the table
				pdf.SetFont("Times", "B", 9)
				pdf.CellFormat(40, 10, fmt.Sprintf("%d", data.TotalLines), "1", 0, "C", false, 0, "")
				pdf.CellFormat(40, 10, fmt.Sprintf("%d", data.TotalBlankLines), "1", 0, "C", false, 0, "")
				pdf.CellFormat(40, 10, fmt.Sprintf("%d", data.TotalComments), "1", 0, "C", false, 0, "")
				pdf.CellFormat(40, 10, fmt.Sprintf("%d", data.TotalCodeLines), "1", 1, "C", false, 0, "")

				// Increment the table count
				tableCount++

				// Check if we need to add a new page
				if tableCount >= maxTablesPerPage {
					pdf.AddPage()
					if logoPath := utils.GetLogoPath(); logoPath != "" {
						pdf.Image(logoPath, 10, 5, 50, 0, false, "", 0, "")
					}

					pdf.Ln(10)
					pdf.SetFont("Times", "B", 12)
					pdf.CellFormat(0, 15, "Repository Gobal report", "", 1, "L", false, 0, "")
					tableCount = 0 // reset the table count for the new page
				}
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("error walking the path: %v", err)
	}

	if err := pdf.OutputFileAndClose(DestinationResultPDF); err != nil {
		return fmt.Errorf("error creating PDF: %v", err)
	}

	loggers.Infof("\t✅ PDF report exported to %s", DestinationResultPDF)
	return nil
}
