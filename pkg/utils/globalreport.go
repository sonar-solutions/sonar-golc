package utils

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jung-kurt/gofpdf"
)

type FileData struct {
	Results []LanguageData1 `json:"Results"`
}

type LanguageData1 struct {
	Language  string `json:"Language"`
	CodeLines int    `json:"CodeLines"`
}

type LanguageData struct {
	Language   string  `json:"Language"`
	CodeLines  int     `json:"CodeLines"`
	Percentage float64 `json:"Percentage"`
	CodeLinesF string  `json:"CodeLinesF"`
}

type Globalinfo struct {
	Organization           string `json:"Organization"`
	TotalLinesOfCode       string `json:"TotalLinesOfCode"`
	LargestRepository      string `json:"LargestRepository"`
	LinesOfCodeLargestRepo string `json:"LinesOfCodeLargestRepo"`
	DevOpsPlatform         string `json:"DevOpsPlatform"`
	NumberRepos            int    `json:"NumberRepos"`
}

func (l *LanguageData) FormatCodeLines() {
	l.CodeLinesF = FormatCodeLines(float64(l.CodeLines))
}

func getTotalCodeLines(languages []LanguageData) int {
	total := 0
	for _, lang := range languages {
		total += lang.CodeLines
	}
	return total
}

// getTotalCodeLinesExcludingJSON returns the sum of CodeLines for all languages except JSON.
// Used for report totals and percentages to match SonarQube standard behavior.
func getTotalCodeLinesExcludingJSON(languages []LanguageData) int {
	total := 0
	for _, lang := range languages {
		if strings.TrimSpace(lang.Language) != LanguageExcludedFromTotalLOC {
			total += lang.CodeLines
		}
	}
	return total
}

func CreateGlobalReport(directory string) error {

	//directory := "Results"
	loggers := NewLogger()

	totals, err := collectLanguageTotals(directory)
	if err != nil {
		loggers.Errorf("❌ Error reading files : %v", err)
		return err
	}

	// Persist code_lines_by_language.json and keep marshaled bytes for later
	outputData, err := writeLanguageTotalsJSON(totals)
	if err != nil {
		loggers.Errorf("❌ Error creating output JSON file : %v", err)
		return err
	}

	// Reading data from the GlobalReport JSON file
	ginfo, err := readGlobalInfoFromFile("Results/GlobalReport.json")
	if err != nil {
		return err
	}

	// JSON data decoding
	var languages []LanguageData
	err = json.Unmarshal(outputData, &languages)
	if err != nil {
		loggers.Errorf("❌ Error decoding JSON data : %v", err)
		return err
	}

	// Create a PDF
	if err := renderGlobalPDF(languages, ginfo); err != nil {
		return err
	}

	loggers.Infof("✅ Global PDF report exported to %s", "Results/GlobalReport.pdf")
	return nil
}

// collectLanguageTotals walks result files and aggregates language totals.
func collectLanguageTotals(directory string) (map[string]int, error) {
	ligneDeCodeParLangage := make(map[string]int)
	err := filepath.Walk(directory, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !isEligibleResultFile(info, path) {
			return nil
		}
		return accumulateLanguageTotalsFromFile(path, ligneDeCodeParLangage)
	})
	if err != nil {
		return nil, err
	}
	return ligneDeCodeParLangage, nil
}

// isEligibleResultFile returns true for top-level Result_*.json files that are not _byfile.
func isEligibleResultFile(info os.FileInfo, path string) bool {
	if info.IsDir() {
		return false
	}
	name := info.Name()
	if !strings.HasPrefix(name, "Result_") {
		return false
	}
	if strings.Contains(name, "_byfile") {
		return false
	}
	return filepath.Ext(path) == ".json"
}

// accumulateLanguageTotalsFromFile parses a file and updates the totals map.
func accumulateLanguageTotalsFromFile(path string, totals map[string]int) error {
	fileData, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var data FileData
	if err := json.Unmarshal(fileData, &data); err != nil {
		return err
	}
	for _, result := range data.Results {
		if lang := strings.TrimSpace(result.Language); lang != "" {
			totals[lang] += result.CodeLines
		}
	}
	return nil
}

// writeLanguageTotalsJSON writes Results/code_lines_by_language.json and returns the serialized bytes.
func writeLanguageTotalsJSON(totals map[string]int) ([]byte, error) {
	loggers := NewLogger()
	var resultats []LanguageData1
	for lang, total := range totals {
		resultats = append(resultats, LanguageData1{
			Language:  lang,
			CodeLines: total,
		})
	}
	outputData, err := json.MarshalIndent(resultats, "", "  ")
	if err != nil {
		return nil, err
	}
	const outputFile = "Results/code_lines_by_language.json"
	if err := os.WriteFile(outputFile, outputData, 0644); err != nil {
		return nil, err
	}
	loggers.Infof("✅ Results analysis recorded in %s", outputFile)
	return outputData, nil
}

// readGlobalInfoFromFile reads Results/GlobalReport.json into Globalinfo.
func readGlobalInfoFromFile(path string) (Globalinfo, error) {
	loggers := NewLogger()
	data, err := os.ReadFile(path)
	if err != nil {
		loggers.Errorf("❌ Error reading GlobalReport.json file : %v", err)
		return Globalinfo{}, err
	}
	var g Globalinfo
	if err := json.Unmarshal(data, &g); err != nil {
		loggers.Errorf("❌ Error decoding JSON GlobalReport.json file : %v", err)
		return Globalinfo{}, err
	}
	return g, nil
}

// renderGlobalPDF generates the GlobalReport.pdf from languages and global info.
func renderGlobalPDF(languages []LanguageData, ginfo Globalinfo) error {
	loggers := NewLogger()

	// Sort by CodeLines descending
	sort.Slice(languages, func(i, j int) bool {
		return languages[i].CodeLines > languages[j].CodeLines
	})

	// Max LOC for relative bar widths
	maxLOC := 0
	for _, l := range languages {
		if l.CodeLines > maxLOC {
			maxLOC = l.CodeLines
		}
	}

	// Populate percentage and formatted LOC
	totalExcl := getTotalCodeLinesExcludingJSON(languages)
	for i := range languages {
		if strings.TrimSpace(languages[i].Language) == LanguageExcludedFromTotalLOC || totalExcl == 0 {
			languages[i].Percentage = 0
		} else {
			languages[i].Percentage = float64(languages[i].CodeLines) / float64(totalExcl) * 100
		}
		languages[i].CodeLinesF = FormatCodeLines(float64(languages[i].CodeLines))
	}

	const (
		pageW    = 210.0
		pageH    = 297.0
		marginL  = 15.0
		marginR  = 15.0
		marginT  = 28.0
		marginB  = 15.0
		contentW = pageW - marginL - marginR
	)

	pdf := gofpdf.New("P", "mm", "A4", "")
	pdf.AliasNbPages("{nb}")
	pdf.SetMargins(marginL, marginT, marginR)
	pdf.SetAutoPageBreak(true, marginB+8)

	logoPath := "imgs/Logob.png"
	_, logoErr := os.Stat(logoPath)

	pdf.SetHeaderFunc(func() {
		// Dark navy band
		pdf.SetFillColor(10, 14, 26)
		pdf.Rect(0, 0, pageW, 22, "F")
		// Blue accent stripe
		pdf.SetFillColor(0, 115, 186)
		pdf.Rect(0, 22, pageW, 1.5, "F")
		// Logo (right side)
		if logoErr == nil {
			pdf.Image(logoPath, 163, 2, 32, 0, false, "", 0, "")
		}
		// Report title
		pdf.SetFont("Helvetica", "B", 12)
		pdf.SetTextColor(255, 255, 255)
		pdf.SetXY(marginL, 7)
		pdf.CellFormat(140, 7, "GoLC - Global Lines of Code Report", "", 0, "L", false, 0, "")
		// Org subtitle
		pdf.SetFont("Helvetica", "", 8)
		pdf.SetTextColor(170, 200, 225)
		pdf.SetXY(marginL, 15)
		org := ginfo.Organization
		if len(org) > 50 {
			org = org[:47] + "..."
		}
		pdf.CellFormat(140, 5, "Organization: "+org, "", 0, "L", false, 0, "")
		pdf.SetTextColor(0, 0, 0)
		pdf.SetY(marginT)
	})

	pdf.SetFooterFunc(func() {
		pdf.SetY(-12)
		pdf.SetFont("Helvetica", "I", 7)
		pdf.SetTextColor(150, 150, 150)
		pdf.SetX(marginL)
		note := NoteExcludedFromTotal
		if len(note) > 85 {
			note = note[:82] + "..."
		}
		pdf.CellFormat(contentW-22, 4, note, "", 0, "L", false, 0, "")
		pdf.SetX(marginL + contentW - 22)
		pdf.CellFormat(22, 4, fmt.Sprintf("Page %d / {nb}", pdf.PageNo()), "", 0, "R", false, 0, "")
		pdf.SetTextColor(0, 0, 0)
	})

	pdf.AddPage()

	// ── Stat cards ────────────────────────────────────────────────────
	cardW := (contentW - 12.0) / 4 // 3 gaps of 4mm
	cardH := 22.0
	cardsY := pdf.GetY()

	type statCard struct {
		title, value string
		r, g, b      int
	}
	valOrNA := func(s string) string {
		if s == "" {
			return "N/A"
		}
		if len(s) > 18 {
			return s[:15] + "..."
		}
		return s
	}
	cards := []statCard{
		{"Total LOC", valOrNA(ginfo.TotalLinesOfCode), 0, 115, 186},
		{"Repositories", fmt.Sprintf("%d", ginfo.NumberRepos), 0, 168, 110},
		{"Largest Repo", valOrNA(ginfo.LargestRepository), 241, 146, 49},
		{"Largest Repo LOC", valOrNA(ginfo.LinesOfCodeLargestRepo), 167, 86, 180},
	}
	for i, c := range cards {
		x := marginL + float64(i)*(cardW+4)
		pdf.SetFillColor(246, 248, 252)
		pdf.Rect(x, cardsY, cardW, cardH, "F")
		pdf.SetFillColor(c.r, c.g, c.b)
		pdf.Rect(x, cardsY, 3, cardH, "F")
		pdf.SetFont("Helvetica", "", 7)
		pdf.SetTextColor(110, 110, 120)
		pdf.SetXY(x+5, cardsY+4)
		pdf.CellFormat(cardW-6, 4, c.title, "", 0, "L", false, 0, "")
		pdf.SetFont("Helvetica", "B", 10)
		pdf.SetTextColor(10, 14, 26)
		pdf.SetXY(x+5, cardsY+10)
		pdf.CellFormat(cardW-6, 6, c.value, "", 0, "L", false, 0, "")
	}
	pdf.SetXY(marginL, cardsY+cardH+6)

	// ── Language breakdown section header ────────────────────────────
	pdf.SetFillColor(0, 115, 186)
	pdf.Rect(marginL, pdf.GetY(), contentW, 8, "F")
	pdf.SetFont("Helvetica", "B", 10)
	pdf.SetTextColor(255, 255, 255)
	pdf.SetX(marginL + 2)
	pdf.CellFormat(contentW-2, 8, "Language Breakdown", "", 1, "L", false, 0, "")
	pdf.Ln(2)

	// Column widths
	const (
		colNum  = 10.0
		colLang = 52.0
		colLOC  = 30.0
		colPct  = 22.0
	)
	colBar := contentW - colNum - colLang - colLOC - colPct

	drawColHeaders := func() {
		pdf.SetFillColor(220, 230, 242)
		pdf.SetFont("Helvetica", "B", 8)
		pdf.SetTextColor(40, 40, 50)
		pdf.SetX(marginL)
		pdf.CellFormat(colNum, 6, "#", "0", 0, "C", true, 0, "")
		pdf.CellFormat(colLang, 6, "LANGUAGE", "0", 0, "L", true, 0, "")
		pdf.CellFormat(colLOC, 6, "LOC", "0", 0, "R", true, 0, "")
		pdf.CellFormat(colPct, 6, "SHARE %", "0", 0, "R", true, 0, "")
		pdf.CellFormat(colBar, 6, "BAR", "0", 1, "L", true, 0, "")
	}
	drawColHeaders()

	barColors := [][3]int{
		{0, 115, 186}, {0, 168, 110}, {241, 146, 49}, {167, 86, 180},
		{231, 76, 60}, {22, 160, 133}, {52, 73, 94}, {200, 130, 20},
	}

	rowH := 7.0
	for i, lang := range languages {
		// Manual page-break guard
		if pdf.GetY()+rowH > pageH-marginB-8 {
			pdf.AddPage()
			pdf.SetFillColor(0, 115, 186)
			pdf.Rect(marginL, pdf.GetY(), contentW, 7, "F")
			pdf.SetFont("Helvetica", "B", 9)
			pdf.SetTextColor(255, 255, 255)
			pdf.SetX(marginL + 2)
			pdf.CellFormat(contentW-2, 7, "Language Breakdown (continued)", "", 1, "L", false, 0, "")
			pdf.Ln(1)
			drawColHeaders()
		}

		rowY := pdf.GetY()
		if i%2 == 0 {
			pdf.SetFillColor(255, 255, 255)
		} else {
			pdf.SetFillColor(244, 247, 251)
		}
		pdf.Rect(marginL, rowY, contentW, rowH, "F")

		bc := barColors[i%len(barColors)]

		// Row number
		pdf.SetFont("Helvetica", "", 8)
		pdf.SetTextColor(130, 130, 140)
		pdf.SetXY(marginL, rowY)
		pdf.CellFormat(colNum, rowH, fmt.Sprintf("%d", i+1), "0", 0, "C", false, 0, "")

		// Language name
		langDisplay := lang.Language
		if strings.TrimSpace(lang.Language) == LanguageExcludedFromTotalLOC {
			langDisplay = lang.Language + " (excl.)"
		}
		pdf.SetFont("Helvetica", "B", 8)
		pdf.SetTextColor(20, 20, 30)
		pdf.CellFormat(colLang, rowH, langDisplay, "0", 0, "L", false, 0, "")

		// LOC
		pdf.SetFont("Helvetica", "", 8)
		pdf.SetTextColor(20, 20, 30)
		pdf.CellFormat(colLOC, rowH, lang.CodeLinesF, "0", 0, "R", false, 0, "")

		// Share %
		pctStr := fmt.Sprintf("%.1f%%", lang.Percentage)
		if strings.TrimSpace(lang.Language) == LanguageExcludedFromTotalLOC {
			pctStr = "-"
		}
		pdf.CellFormat(colPct, rowH, pctStr, "0", 0, "R", false, 0, "")

		// Bar
		barX := marginL + colNum + colLang + colLOC + colPct
		trackW := colBar - 4
		midY := rowY + (rowH-3)/2
		pdf.SetFillColor(218, 228, 240)
		pdf.Rect(barX+2, midY, trackW, 3, "F")
		if maxLOC > 0 && lang.CodeLines > 0 {
			fillW := float64(lang.CodeLines) / float64(maxLOC) * trackW
			if fillW < 1 {
				fillW = 1
			}
			pdf.SetFillColor(bc[0], bc[1], bc[2])
			pdf.Rect(barX+2, midY, fillW, 3, "F")
		}

		pdf.SetXY(marginL, rowY+rowH)
	}

	if err := pdf.OutputFileAndClose("Results/GlobalReport.pdf"); err != nil {
		loggers.Errorf("Error saving PDF file: %v", err)
		return err
	}
	return nil
}
