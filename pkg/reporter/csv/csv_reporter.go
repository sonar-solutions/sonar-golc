package csv

import (
	"encoding/csv"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/SonarSource-Demos/sonar-golc/pkg/sorter"
	"github.com/SonarSource-Demos/sonar-golc/pkg/utils"
)

type CsvReporter struct {
	OutputName string
	OutputPath string
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

func (c CsvReporter) GenerateReportByLanguage(summary *sorter.SortedSummary) error {
	csvReport := &report{
		TotalFiles:      summary.TotalFiles,
		TotalLines:      summary.TotalLines,
		TotalBlankLines: summary.TotalBlankLines,
		TotalComments:   summary.TotalComments,
		TotalCodeLines:  summary.TotalCodeLines,
		Results:         []languageResult{},
	}

	for _, r := range summary.Results {
		csvReport.Results = append(csvReport.Results.([]languageResult), languageResult{
			Language:   r.Name,
			Files:      summary.FilesByLanguage[r.Name],
			Lines:      r.Lines,
			BlankLines: r.BlankLines,
			Comments:   r.Comments,
			CodeLines:  r.CodeLines,
		})
	}

	return c.writeCsv(csvReport)
}

func (c CsvReporter) GenerateReportByFile(summary *sorter.SortedSummary) error {
	csvReport := &report{
		TotalLines:      summary.TotalLines,
		TotalBlankLines: summary.TotalBlankLines,
		TotalComments:   summary.TotalComments,
		TotalCodeLines:  summary.TotalCodeLines,
		Results:         []fileResult{},
	}

	for _, r := range summary.Results {
		cleanedName := utils.CleanFileName(r.Name)
		csvReport.Results = append(csvReport.Results.([]fileResult), fileResult{
			File:       cleanedName,
			Lines:      r.Lines,
			BlankLines: r.BlankLines,
			Comments:   r.Comments,
			CodeLines:  r.CodeLines,
		})
	}

	return c.writeCsv(csvReport)
}

func (c CsvReporter) writeCsv(csvReport *report) error {
	loggers := utils.SharedLogger()
	outputName := strings.Replace(c.OutputName, "/", "_", -1)
	if !strings.HasSuffix(outputName, ".csv") {
		outputName += ".csv"
	}

	path := filepath.Join(c.OutputPath+"/", outputName)
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	// Write header
	writer.Write([]string{"File", "Lines", "Blank Lines", "Comments", "Code Lines"})

	// Write results
	switch results := csvReport.Results.(type) {
	case []languageResult:
		//	writer.Write([]string{"Language", "Files", "Lines", "Blank Lines", "Comments", "Code Lines"})
		for _, r := range results {
			writer.Write([]string{
				r.Language,
				strconv.Itoa(r.Files),
				strconv.Itoa(r.Lines),
				strconv.Itoa(r.BlankLines),
				strconv.Itoa(r.Comments),
				strconv.Itoa(r.CodeLines),
			})
		}
	case []fileResult:
		//writer.Write([]string{"File", "Lines", "Blank Lines", "Comments", "Code Lines"})
		for _, r := range results {
			writer.Write([]string{
				r.File,
				strconv.Itoa(r.Lines),
				strconv.Itoa(r.BlankLines),
				strconv.Itoa(r.Comments),
				strconv.Itoa(r.CodeLines),
			})
		}
	default:
		return nil
	}

	loggers.Infof("\t✅ CSV report exported to %s", path)
	return nil
}
