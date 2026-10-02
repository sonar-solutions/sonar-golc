package scanner

import (
	"encoding/json"
	"os"
	"strings"

	"github.com/SonarSource-Demos/sonar-golc/pkg/analyzer"
	"github.com/SonarSource-Demos/sonar-golc/pkg/utils"
)

// A Jupyter notebook is a JSON document, and almost none of it is code: a 75-line notebook
// measured against SonarQube had 8 lines of code, all of them inside code cells. SonarQube
// counts only the source of the code cells - shell (!) and magic (%) lines included - and
// never the markdown cells, the outputs or the JSON around them. A notebook whose language
// is not Python contributes nothing, so neither does it here.

// notebook holds the parts of the nbformat 4 document that decide what is counted.
type notebook struct {
	Metadata notebookMetadata `json:"metadata"`
	Cells    []notebookCell   `json:"cells"`
}

// notebookMetadata names the notebook's language in either of two places.
type notebookMetadata struct {
	LanguageInfo notebookLanguageInfo `json:"language_info"`
	Kernelspec   notebookKernelspec   `json:"kernelspec"`
}

type notebookLanguageInfo struct {
	Name string `json:"name"`
}

type notebookKernelspec struct {
	Language string `json:"language"`
}

type notebookCell struct {
	CellType string `json:"cell_type"`
	// nbformat allows a cell's source as one string or as a list of lines.
	Source json.RawMessage `json:"source"`
}

// scanNotebook counts the code cells of a notebook. A notebook that is not valid JSON is
// reported with no lines rather than skipped: SonarQube still lists such a file, it just
// finds no code in it.
func (sc *Scanner) scanNotebook(file analyzer.FileMetadata) (scanResult, error) {
	result := scanResult{Metadata: file}

	data, err := os.ReadFile(file.FilePath)
	if err != nil {
		return result, err
	}

	cells, err := notebookCodeCells(data)
	if err != nil {
		utils.SharedLogger().Warnf("⚠️  Could not parse notebook %s, counting no lines for it: %v", file.FilePath, err)
		return result, nil
	}

	// Each cell is counted on its own so that an unclosed block comment in one cell cannot
	// swallow the cells after it.
	for _, cell := range cells {
		if err := sc.countLines(file, strings.NewReader(cell), &result); err != nil {
			return result, err
		}
	}

	result.Lines = result.CodeLines + result.BlankLines + result.Comments

	return result, nil
}

// notebookCodeCells returns the source of every code cell of a Python notebook, or none
// for a notebook in another language. A notebook that declares no language is taken to be
// Python, as SonarQube does.
func notebookCodeCells(data []byte) ([]string, error) {
	var nb notebook
	if err := json.Unmarshal(data, &nb); err != nil {
		return nil, err
	}

	language := nb.Metadata.LanguageInfo.Name
	if language == "" {
		language = nb.Metadata.Kernelspec.Language
	}
	if language != "" && !strings.EqualFold(language, "python") {
		return nil, nil
	}

	var cells []string
	for _, cell := range nb.Cells {
		if cell.CellType != "code" {
			continue
		}
		cells = append(cells, cellSource(cell.Source))
	}

	return cells, nil
}

// cellSource joins a cell's source, given either as a list of lines (each carrying its own
// newline) or as a single string. A source of neither shape counts as empty.
func cellSource(raw json.RawMessage) string {
	var lines []string
	if err := json.Unmarshal(raw, &lines); err == nil {
		return strings.Join(lines, "")
	}

	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return text
	}

	return ""
}
