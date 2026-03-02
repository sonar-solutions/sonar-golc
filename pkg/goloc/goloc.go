package goloc

import (
	"fmt"
	"path/filepath"

	"github.com/SonarSource-Demos/sonar-golc/pkg/analyzer"
	"github.com/SonarSource-Demos/sonar-golc/pkg/filesystem"
	"github.com/SonarSource-Demos/sonar-golc/pkg/getter"
	"github.com/SonarSource-Demos/sonar-golc/pkg/gogit"
	"github.com/SonarSource-Demos/sonar-golc/pkg/goloc/language"
	"github.com/SonarSource-Demos/sonar-golc/pkg/reporter"
	"github.com/SonarSource-Demos/sonar-golc/pkg/reporter/csv"
	"github.com/SonarSource-Demos/sonar-golc/pkg/reporter/json"
	"github.com/SonarSource-Demos/sonar-golc/pkg/reporter/pdf"
	"github.com/SonarSource-Demos/sonar-golc/pkg/reporter/prompt"
	"github.com/SonarSource-Demos/sonar-golc/pkg/scanner"
	"github.com/SonarSource-Demos/sonar-golc/pkg/sorter"
	"github.com/SonarSource-Demos/sonar-golc/pkg/utils"
)

type Params struct {
	Path              string
	ByFile            bool
	ByAll             bool
	ExcludePaths      []string
	ExcludeExtensions []string
	IncludeExtensions []string
	OrderByLang       bool
	OrderByFile       bool
	OrderByCode       bool
	OrderByLine       bool
	OrderByBlank      bool
	OrderByComment    bool
	Order             string
	OutputName        string
	OutputPath        string
	ReportFormats     []string
	Branch            string
	Token             string
	Cloned            bool
	Repopath          string
}

type GCloc struct {
	Params    Params
	analyzer  *analyzer.Analyzer
	scanner   *scanner.Scanner
	sorter    sorter.Sorter
	reporters []reporter.Reporter
	Repopath  string
}

/* func NewGCloc(params Params, languages language.Languages) (*GCloc, error) {
	var path string
	var err error
	loggers := utils.NewLogger()

	if !params.Cloned {
		// The repository is not cloned, clone it
		if len(params.Branch) != 0 {
			path, err = gogit.Getrepos(params.Path, params.Branch, params.Token)
			if err != nil {
				return nil, err
			}
		} else {
			path, err = getter.Getter(params.Path)
			if err != nil {
				return nil, err
			}
		}

		lastPart := filepath.Base(path)
		if lastPart != "" {
			params.OutputName = fmt.Sprintf("%s%s", params.OutputName, lastPart)
		} else {
			loggers.Errorf("❌ Failed to create OutputName")
		}

		excludePaths, err := filesystem.GetExcludePaths(path, params.ExcludePaths)
		if err != nil {
			return nil, err
		}

		analyzer := analyzer.NewAnalyzer(
			path,
			excludePaths,
			utils.ConvertToMap(params.ExcludeExtensions),
			utils.ConvertToMap(params.IncludeExtensions),
			getExtensionsMap(languages),
		)
		scanner := scanner.NewScanner(languages)

		reporters := getReporters(params.ReportFormats, params.OutputName, params.OutputPath, params.ByFile)

		// Mark as cloned
		params.Cloned = true

		fmt.Print("\n")
		loggers.Infof("PATH 1er tour: %s", path)

		return &GCloc{
			Params:    params,
			analyzer:  analyzer,
			scanner:   scanner,
			sorter:    getSorter(params.ByFile, params.Order),
			reporters: reporters,
			Repopath:  path,
		}, nil

	} else {
		// If the repo has already been cloned, use the path stored in params
		path = params.Repopath

		excludePaths, err := filesystem.GetExcludePaths(path, params.ExcludePaths)
		if err != nil {
			return nil, err
		}

		analyzer := analyzer.NewAnalyzer(
			path,
			excludePaths,
			utils.ConvertToMap(params.ExcludeExtensions),
			utils.ConvertToMap(params.IncludeExtensions),
			getExtensionsMap(languages),
		)

		scanner := scanner.NewScanner(languages)

		reporters := getReporters(params.ReportFormats, params.OutputName, params.OutputPath, params.ByFile)

		return &GCloc{
			Params:    params,
			analyzer:  analyzer,
			scanner:   scanner,
			sorter:    getSorter(params.ByFile, params.Order),
			reporters: reporters,
			Repopath:  path,
		}, nil
	}
}*/

func NewGCloc(params Params, languages language.Languages) (*GCloc, error) {
	path, err := getRepoPath(params)
	if err != nil {
		return nil, err
	}

	if params.Branch == "" {
		if lastPart := filepath.Base(path); lastPart != "" {
			params.OutputName = fmt.Sprintf("%s%s", params.OutputName, lastPart)
		} else {
			utils.NewLogger().Errorf("❌ Failed to create OutputName")
		}
	}

	excludePaths, err := filesystem.GetExcludePaths(path, params.ExcludePaths)
	if err != nil {
		return nil, err
	}

	analyzer, scanner, reporters := initAnalyzerScannerReporters(path, params, excludePaths, languages)

	params.Cloned = true

	return &GCloc{
		Params:    params,
		analyzer:  analyzer,
		scanner:   scanner,
		sorter:    getSorter(params.ByFile, params.Order),
		reporters: reporters,
		Repopath:  path,
	}, nil
}

func getRepoPath(params Params) (string, error) {
	if params.Cloned {
		return params.Repopath, nil
	}

	if len(params.Branch) != 0 {
		return gogit.Getrepos(params.Path, params.Branch, params.Token)
	}
	return getter.Getter(params.Path)
}

func initAnalyzerScannerReporters(path string, params Params, excludePaths []string, languages language.Languages) (*analyzer.Analyzer, *scanner.Scanner, []reporter.Reporter) {
	analyzer := analyzer.NewAnalyzer(
		path,
		excludePaths,
		utils.ConvertToMap(params.ExcludeExtensions),
		utils.ConvertToMap(params.IncludeExtensions),
		getExtensionsMap(languages),
	)
	scanner := scanner.NewScanner(languages)

	reporters := getReporters(params.ReportFormats, params.OutputName, params.OutputPath, params.ByFile)

	return analyzer, scanner, reporters
}

func (gc *GCloc) Run() error {

	files, err := gc.analyzer.MatchingFiles()
	if err != nil {
		return err
	}

	scanResult, err := gc.scanner.Scan(files)
	if err != nil {
		return err
	}

	summary := gc.scanner.Summary(scanResult)

	sortedSummary := gc.sortSummary(summary)

	return gc.generateReports(sortedSummary)
}

func (gc *GCloc) ChangeLanguages(languages language.Languages) {
	extensions := getExtensionsMap(languages)
	gc.scanner.SupportedLanguages = languages
	gc.analyzer.SupportedExtensions = extensions
}

func (gc *GCloc) sortSummary(summary *scanner.Summary) *sorter.SortedSummary {
	params := gc.Params

	if params.OrderByCode {
		return gc.sorter.OrderByCodeLines(summary)
	}

	if params.OrderByLang {
		return gc.sorter.OrderByLanguage(summary)
	}

	if params.OrderByLine {
		return gc.sorter.OrderByLines(summary)
	}

	if params.OrderByComment {
		return gc.sorter.OrderByComments(summary)
	}

	if params.OrderByBlank {
		return gc.sorter.OrderByBlankLines(summary)
	}

	if params.OrderByFile {
		if languageSorter, ok := gc.sorter.(sorter.LanguageSorter); ok {
			return languageSorter.OrderByFiles(summary)
		}
	}

	return gc.sorter.OrderByCodeLines(summary)
}

func (gc *GCloc) generateReports(sortedSummary *sorter.SortedSummary) error {

	if gc.Params.ByFile {
		for _, reporter := range gc.reporters {
			if err := reporter.GenerateReportByFile(sortedSummary); err != nil {
				return err
			}
		}
		return nil
	}
	for _, reporter := range gc.reporters {
		if err := reporter.GenerateReportByLanguage(sortedSummary); err != nil {
			return err
		}
	}

	return nil
}

func getExtensionsMap(languages language.Languages) map[string]string {
	extensions := map[string]string{}

	for language, languageInfo := range languages {
		for _, extension := range languageInfo.Extensions {
			extensions[extension] = language
		}
	}

	return extensions
}

func getSorter(byFile bool, order string) sorter.Sorter {
	if byFile {
		return sorter.NewFileSorter(order)
	}

	return sorter.NewLanguageSorter(order)
}

func getReporters(reportFormats []string, outputName, outputPath string, byfile bool) []reporter.Reporter {
	var reporters []reporter.Reporter
	indicemode := "_byfile"

	for _, format := range reportFormats {
		switch format {
		case "prompt":
			reporters = append(reporters, prompt.PromptReporter{})
		case "json":
			//loggers := utils.NewLogger()

			if byfile {

				typereportPath := "/byfile-report"
				reporters = append(reporters, json.JsonReporter{
					OutputName: outputName + indicemode,
					OutputPath: outputPath + typereportPath,
				})

				reporters = append(reporters, csv.CsvReporter{
					OutputName: outputName + indicemode,
					OutputPath: outputPath + typereportPath + "/csv-report",
				})
				reporters = append(reporters, pdf.PdfReporter{
					OutputName: outputName + indicemode,
					OutputPath: outputPath + typereportPath + "/pdf-report",
				})

				/*reporterG := pdf.PdfReporter{
					OutputName: "Results-Report" + indicemode + ".pdf",
					OutputPath: outputPath + typereportPath,
				}

				if err := reporterG.GenerateGlobalReportByFile(); err != nil {
					loggers.Fatalf("❌ goloc : Global Report PDF generation failed: %v\n", err)
				}*/
			} else {

				typereportPath := "/bylanguage-report"
				reporters = append(reporters, json.JsonReporter{
					OutputName: outputName,
					OutputPath: outputPath + typereportPath,
				})
			}

		default:
			fmt.Printf("%s report format not supported\n", format)
		}
	}

	return reporters
}
