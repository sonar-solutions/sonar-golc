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

	// Repositories the user removed from the totals on the results page. An absent
	// file means nothing was deselected, so the walk below covers the full scan.
	deselected := LoadDeselectedRepos(directory)
	deselectedSet := DeselectionKeys(deselected)

	totals, repoTotals, err := collectResultTotals(directory, deselectedSet)
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

	// GlobalReport.json holds the numbers as scanned and is never rewritten here, so
	// the headline figures must be re-derived from the repositories that survived the
	// deselection. With an empty set this reproduces the scanned values.
	rawTotalLOC := ginfo.TotalLinesOfCode
	ginfo = AdjustGlobalInfo(ginfo, languages, repoTotals, len(deselected))

	// Repositories the analysis phase could not complete (clone timeout/failure or
	// counting error). Surfaced in the PDF so a large scan that skips a few problem
	// repos does not silently undercount. Missing file => empty list.
	skippedRepos := LoadSkippedRepos(directory)

	// Per-run repository breakdown (scanned/analyzed/archived/empty). Missing file
	// (older result sets) => nil, and the summary section renders nothing.
	scanSummary := LoadScanSummary(directory)

	// Create a PDF
	if err := renderGlobalPDF(languages, ginfo, skippedRepos, scanSummary, deselected, rawTotalLOC); err != nil {
		return err
	}

	loggers.Infof("✅ Global PDF report exported to %s", "Results/GlobalReport.pdf")
	return nil
}

// RepoTotal is one repository's contribution to the global totals, as recovered
// from its by-language result file. CodeLines excludes the language held out of the
// total (JSON), matching the headline LOC figure everywhere else.
type RepoTotal struct {
	Key       string
	Org       string
	Repo      string
	Branch    string
	CodeLines int
}

// collectLanguageTotals walks result files and aggregates language totals.
func collectLanguageTotals(directory string) (map[string]int, error) {
	totals, _, err := collectResultTotals(directory, nil)
	return totals, err
}

// collectResultTotals walks result files once and returns both the per-language
// totals and each repository's contribution, skipping any repository in deselected.
// Both come from the same pass so the language breakdown and the headline totals can
// never disagree about which repositories were counted.
func collectResultTotals(directory string, deselected DeselectionSet) (map[string]int, []RepoTotal, error) {
	ligneDeCodeParLangage := make(map[string]int)
	var repoTotals []RepoTotal

	err := filepath.Walk(directory, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !isEligibleResultFile(info, path) {
			return nil
		}

		name := info.Name()
		key, ok := DeselectionKeyFromResultFileName(name)
		if ok && deselected.Contains(key) {
			return nil
		}

		repoLOC, err := accumulateLanguageTotalsFromFile(path, ligneDeCodeParLangage)
		if err != nil {
			return err
		}

		org, repo, branch, parsed := ParseResultFileName(name)
		if !parsed {
			// file platform: Result_<Repo>.json carries no org or branch.
			repo = key
		}
		repoTotals = append(repoTotals, RepoTotal{
			Key:       key,
			Org:       org,
			Repo:      repo,
			Branch:    branch,
			CodeLines: repoLOC,
		})
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	return ligneDeCodeParLangage, repoTotals, nil
}

// AdjustGlobalInfo re-derives the headline figures from the repositories that
// survived a deselection. TotalLinesOfCode and the largest-repository pair are
// recomputed from the filtered data; NumberRepos is reduced by the deselected count
// rather than replaced by len(repoTotals), so the platform-specific meaning of the
// scanned count is preserved.
//
// With nothing deselected it returns ginfo untouched rather than recomputing an
// identical value: the scan's own figures stay authoritative, so enabling this
// feature cannot shift a number on an unfiltered report.
func AdjustGlobalInfo(ginfo Globalinfo, languages []LanguageData, repoTotals []RepoTotal, deselectedCount int) Globalinfo {
	if deselectedCount == 0 {
		return ginfo
	}

	ginfo.TotalLinesOfCode = FormatCodeLines(float64(getTotalCodeLinesExcludingJSON(languages)))

	maxLOC := 0
	largest := ""
	for _, rt := range repoTotals {
		if rt.CodeLines > maxLOC {
			maxLOC = rt.CodeLines
			largest = rt.Repo
		}
	}
	ginfo.LargestRepository = largest
	ginfo.LinesOfCodeLargestRepo = FormatCodeLines(float64(maxLOC))

	ginfo.NumberRepos -= deselectedCount
	if ginfo.NumberRepos < 0 {
		ginfo.NumberRepos = 0
	}
	return ginfo
}

// ParseResultFileName parses a `Result_<Org>__<Repo>__<Branch>.json` (or matching
// PDF output name with optional `_byfile` suffix) and returns the components.
// The double-underscore field separator keeps single `_` free to appear inside
// any component (GitLab group names, repo slugs, branch names like feat_xyz), so
// all three fields are recovered unambiguously by a fixed-N split. Returns
// ok=false when the name does not have exactly three `__`-separated segments —
// including all legacy single-`_` names from before the delimiter change, which
// are intentionally skipped and will be regenerated on the next analysis run.
func ParseResultFileName(name string) (org, repo, branch string, ok bool) {
	if !strings.HasPrefix(name, "Result_") {
		return "", "", "", false
	}
	base := strings.TrimPrefix(name, "Result_")
	for _, suffix := range []string{".json", ".pdf"} {
		base = strings.TrimSuffix(base, suffix)
	}
	base = strings.TrimSuffix(base, "_byfile")
	parts := strings.SplitN(base, "__", 3)
	if len(parts) != 3 {
		return "", "", "", false
	}
	return parts[0], parts[1], parts[2], true
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

// accumulateLanguageTotalsFromFile parses a file and updates the totals map. It
// returns that single file's contribution to the headline LOC figure — its code
// lines excluding the language held out of the total (JSON) — so a caller tracking
// per-repository totals does not have to parse the file a second time.
func accumulateLanguageTotalsFromFile(path string, totals map[string]int) (int, error) {
	fileData, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	var data FileData
	if err := json.Unmarshal(fileData, &data); err != nil {
		return 0, err
	}
	fileLOC := 0
	for _, result := range data.Results {
		lang := strings.TrimSpace(result.Language)
		if lang == "" {
			continue
		}
		totals[lang] += result.CodeLines
		if lang != LanguageExcludedFromTotalLOC {
			fileLOC += result.CodeLines
		}
	}
	return fileLOC, nil
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
	// Sorted rather than left in Go's randomized map order, so regenerating from the
	// same result files always produces the same file. Without this, clearing a
	// selection and rebuilding would reshuffle the language list, making a real
	// difference impossible to spot in a diff.
	sort.Slice(resultats, func(i, j int) bool {
		if resultats[i].CodeLines != resultats[j].CodeLines {
			return resultats[i].CodeLines > resultats[j].CodeLines
		}
		return resultats[i].Language < resultats[j].Language
	})
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

// prepareLanguagesForPDF sorts languages descending by LOC, computes percentages and formatted values,
// and returns the sorted slice together with the maximum LOC value (for bar-width scaling).
func prepareLanguagesForPDF(languages []LanguageData) ([]LanguageData, int) {
	sort.Slice(languages, func(i, j int) bool {
		return languages[i].CodeLines > languages[j].CodeLines
	})

	maxLOC := 0
	for _, l := range languages {
		if l.CodeLines > maxLOC {
			maxLOC = l.CodeLines
		}
	}

	totalExcl := getTotalCodeLinesExcludingJSON(languages)
	for i := range languages {
		if strings.TrimSpace(languages[i].Language) == LanguageExcludedFromTotalLOC || totalExcl == 0 {
			languages[i].Percentage = 0
		} else {
			languages[i].Percentage = float64(languages[i].CodeLines) / float64(totalExcl) * 100
		}
		languages[i].CodeLinesF = FormatCodeLines(float64(languages[i].CodeLines))
	}
	return languages, maxLOC
}

// renderLanguageRows draws the per-language table rows, adding a page break with a continuation
// header when necessary.
func renderLanguageRows(pdf *gofpdf.Fpdf, languages []LanguageData, maxLOC int, drawColHeaders func(),
	marginL, pageH, marginB, colNum, colLang, colLOC, colPct, colBar, rowH float64,
	barColors [][3]int) {
	for i, lang := range languages {
		if pdf.GetY()+rowH > pageH-marginB-8 {
			pdf.AddPage()
			pdf.SetFillColor(0, 115, 186)
			pdf.Rect(marginL, pdf.GetY(), colNum+colLang+colLOC+colPct+colBar, 7, "F")
			pdf.SetFont("Helvetica", "B", 9)
			pdf.SetTextColor(255, 255, 255)
			pdf.SetX(marginL + 2)
			pdf.CellFormat(colNum+colLang+colLOC+colPct+colBar-2, 7, "Language Breakdown (continued)", "", 1, "L", false, 0, "")
			pdf.Ln(1)
			drawColHeaders()
		}
		renderLanguageRow(pdf, lang, i, maxLOC, barColors, marginL, colNum, colLang, colLOC, colPct, colBar, rowH)
	}
}

// renderLanguageRow draws a single language row in the breakdown table.
func renderLanguageRow(pdf *gofpdf.Fpdf, lang LanguageData, i, maxLOC int, barColors [][3]int,
	marginL, colNum, colLang, colLOC, colPct, colBar, rowH float64) {
	rowY := pdf.GetY()
	if i%2 == 0 {
		pdf.SetFillColor(255, 255, 255)
	} else {
		pdf.SetFillColor(244, 247, 251)
	}
	pdf.Rect(marginL, rowY, colNum+colLang+colLOC+colPct+colBar, rowH, "F")

	bc := barColors[i%len(barColors)]

	pdf.SetFont("Helvetica", "", 8)
	pdf.SetTextColor(130, 130, 140)
	pdf.SetXY(marginL, rowY)
	pdf.CellFormat(colNum, rowH, fmt.Sprintf("%d", i+1), "0", 0, "C", false, 0, "")

	langDisplay := lang.Language
	if strings.TrimSpace(lang.Language) == LanguageExcludedFromTotalLOC {
		langDisplay = lang.Language + " (excl.)"
	}
	pdf.SetFont("Helvetica", "B", 8)
	pdf.SetTextColor(20, 20, 30)
	pdf.CellFormat(colLang, rowH, langDisplay, "0", 0, "L", false, 0, "")

	pdf.SetFont("Helvetica", "", 8)
	pdf.CellFormat(colLOC, rowH, lang.CodeLinesF, "0", 0, "R", false, 0, "")

	pctStr := fmt.Sprintf("%.1f%%", lang.Percentage)
	if strings.TrimSpace(lang.Language) == LanguageExcludedFromTotalLOC {
		pctStr = "-"
	}
	pdf.CellFormat(colPct, rowH, pctStr, "0", 0, "R", false, 0, "")

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

// renderScanSummarySection appends a "Scan Summary" section to the global PDF,
// showing how many repositories were scanned versus analyzed and how many were
// filtered out (archived/disabled, empty) or could not be completed (skipped).
// When no summary was persisted (older result sets) it renders nothing.
// deselectedCount is reported as its own column rather than folded into Excluded:
// Excluded means "filtered out before analysis and never counted", and merging the
// two would break the Scanned = Analyzed + Archived + Empty + Excluded + Skipped
// invariant that ScanSummary guarantees.
func renderScanSummarySection(pdf *gofpdf.Fpdf, summary *ScanSummary, skippedCount, deselectedCount int, marginL, contentW float64) {
	if summary == nil {
		return
	}

	// Repos that failed during the analysis phase were part of the projected
	// analyzed set, so subtract them for the displayed analyzed count and fold them
	// together with the discovery-phase skips so the row reconciles.
	analyzed := summary.Analyzed - skippedCount
	if analyzed < 0 {
		analyzed = 0
	}
	totalSkipped := summary.Skipped + skippedCount

	pdf.Ln(8)

	// Section header bar (blue, matching the language-breakdown style).
	pdf.SetFillColor(0, 115, 186)
	pdf.Rect(marginL, pdf.GetY(), contentW, 8, "F")
	pdf.SetFont("Helvetica", "B", 10)
	pdf.SetTextColor(255, 255, 255)
	pdf.SetX(marginL + 2)
	pdf.CellFormat(contentW-2, 8, "Scan Summary", "", 1, "L", false, 0, "")
	pdf.Ln(2)

	labels := []string{"Scanned", "Analyzed", "Archived", "Empty", "Excluded", "Skipped"}
	values := []int{summary.Scanned, analyzed, summary.Archived, summary.Empty, summary.Excluded, totalSkipped}
	if deselectedCount > 0 {
		labels = append(labels, "Deselected")
		values = append(values, deselectedCount)
	}
	colW := contentW / float64(len(labels))

	// Value row
	pdf.SetFont("Helvetica", "B", 12)
	pdf.SetTextColor(30, 30, 40)
	pdf.SetX(marginL)
	for _, v := range values {
		pdf.CellFormat(colW, 8, fmt.Sprintf("%d", v), "1", 0, "C", false, 0, "")
	}
	pdf.Ln(-1)

	// Label row
	pdf.SetFont("Helvetica", "", 8)
	pdf.SetTextColor(110, 110, 120)
	pdf.SetX(marginL)
	for _, l := range labels {
		pdf.CellFormat(colW, 6, l, "1", 0, "C", false, 0, "")
	}
	pdf.Ln(-1)
	pdf.SetTextColor(0, 0, 0)
}

// renderSkippedReposSection appends a "Skipped Repositories" section to the global
// PDF. When no repositories were skipped it renders a short confirmation line; when
// some were, it lists each with its branch and the reason it was skipped (clone
// timeout, clone failure, or analysis error).
func renderSkippedReposSection(pdf *gofpdf.Fpdf, tr func(string) string, skippedRepos []SkippedRepo, marginL, contentW float64) {
	pdf.Ln(8)

	// Section header bar (amber to read as a warning, distinct from the blue
	// "Language Breakdown" header).
	pdf.SetFillColor(214, 137, 16)
	pdf.Rect(marginL, pdf.GetY(), contentW, 8, "F")
	pdf.SetFont("Helvetica", "B", 10)
	pdf.SetTextColor(255, 255, 255)
	pdf.SetX(marginL + 2)
	pdf.CellFormat(contentW-2, 8, fmt.Sprintf("Skipped Repositories (%d)", len(skippedRepos)), "", 1, "L", false, 0, "")
	pdf.Ln(2)

	if len(skippedRepos) == 0 {
		pdf.SetFont("Helvetica", "I", 8)
		pdf.SetTextColor(110, 110, 120)
		pdf.SetX(marginL)
		pdf.CellFormat(contentW, 6, tr("None — all targeted repositories were analyzed."), "", 1, "L", false, 0, "")
		pdf.SetTextColor(0, 0, 0)
		return
	}

	const (
		colNum    = 10.0
		colRepo   = 55.0
		colBranch = 38.0
	)
	colReason := contentW - colNum - colRepo - colBranch

	drawHeaders := func() {
		pdf.SetFillColor(245, 226, 196)
		pdf.SetFont("Helvetica", "B", 8)
		pdf.SetTextColor(60, 45, 20)
		pdf.SetX(marginL)
		pdf.CellFormat(colNum, 6, "#", "0", 0, "C", true, 0, "")
		pdf.CellFormat(colRepo, 6, "REPOSITORY", "0", 0, "L", true, 0, "")
		pdf.CellFormat(colBranch, 6, "BRANCH", "0", 0, "L", true, 0, "")
		pdf.CellFormat(colReason, 6, "REASON", "0", 1, "L", true, 0, "")
	}
	drawHeaders()

	// Truncate a cell value so it fits its column width (gofpdf has no native
	// ellipsis); width is estimated from the current font's string width.
	fit := func(s string, w float64) string {
		if pdf.GetStringWidth(s) <= w-2 {
			return s
		}
		for len(s) > 1 && pdf.GetStringWidth(s+"...") > w-2 {
			s = s[:len(s)-1]
		}
		return s + "..."
	}

	pdf.SetFont("Helvetica", "", 8)
	pdf.SetTextColor(40, 40, 50)
	for i, r := range skippedRepos {
		// Repeat the header after an auto page break so long lists stay readable.
		if pdf.GetY() > 270 {
			pdf.AddPage()
			drawHeaders()
			pdf.SetFont("Helvetica", "", 8)
			pdf.SetTextColor(40, 40, 50)
		}
		repo := r.RepoSlug
		if r.ProjectKey != "" {
			repo = r.ProjectKey + "/" + r.RepoSlug
		}
		pdf.SetX(marginL)
		pdf.CellFormat(colNum, 6, fmt.Sprintf("%d", i+1), "0", 0, "C", false, 0, "")
		pdf.CellFormat(colRepo, 6, fit(tr(repo), colRepo), "0", 0, "L", false, 0, "")
		pdf.CellFormat(colBranch, 6, fit(tr(r.Branch), colBranch), "0", 0, "L", false, 0, "")
		pdf.CellFormat(colReason, 6, fit(tr(r.Reason), colReason), "0", 1, "L", false, 0, "")
	}
	pdf.SetTextColor(0, 0, 0)
}

// renderDeselectedReposSection appends a "Deselected Repositories" section listing
// the repositories the user removed from the totals on the results page, together
// with the unfiltered total for comparison. It renders nothing when the selection is
// untouched, so an ordinary report is unchanged.
//
// This section is what keeps a filtered PDF honest: a reader must be able to see
// that the headline LOC is not the whole scan, and what the whole scan came to.
func renderDeselectedReposSection(pdf *gofpdf.Fpdf, tr func(string) string, deselected []DeselectedRepo, rawTotalLOC string, marginL, contentW float64) {
	if len(deselected) == 0 {
		return
	}

	pdf.Ln(8)

	// Slate header — this is a deliberate user choice, not a warning like the
	// amber skipped-repositories section.
	pdf.SetFillColor(72, 84, 104)
	pdf.Rect(marginL, pdf.GetY(), contentW, 8, "F")
	pdf.SetFont("Helvetica", "B", 10)
	pdf.SetTextColor(255, 255, 255)
	pdf.SetX(marginL + 2)
	pdf.CellFormat(contentW-2, 8, fmt.Sprintf("Deselected Repositories (%d)", len(deselected)), "", 1, "L", false, 0, "")
	pdf.Ln(2)

	pdf.SetFont("Helvetica", "I", 8)
	pdf.SetTextColor(90, 90, 100)
	pdf.SetX(marginL)
	pdf.MultiCell(contentW, 4, tr(fmt.Sprintf(
		"Excluded from every total in this report by user selection. Total lines of code across all scanned repositories was %s.",
		rawTotalLOC)), "", "L", false)
	pdf.Ln(1)

	const (
		colNum    = 10.0
		colRepo   = 70.0
		colBranch = 45.0
	)
	colOrg := contentW - colNum - colRepo - colBranch

	drawHeaders := func() {
		pdf.SetFillColor(223, 227, 234)
		pdf.SetFont("Helvetica", "B", 8)
		pdf.SetTextColor(45, 52, 64)
		pdf.SetX(marginL)
		pdf.CellFormat(colNum, 6, "#", "0", 0, "C", true, 0, "")
		pdf.CellFormat(colRepo, 6, "REPOSITORY", "0", 0, "L", true, 0, "")
		pdf.CellFormat(colBranch, 6, "BRANCH", "0", 0, "L", true, 0, "")
		pdf.CellFormat(colOrg, 6, "ORG / PROJECT", "0", 1, "L", true, 0, "")
	}
	drawHeaders()

	fit := func(s string, w float64) string {
		if pdf.GetStringWidth(s) <= w-2 {
			return s
		}
		for len(s) > 1 && pdf.GetStringWidth(s+"...") > w-2 {
			s = s[:len(s)-1]
		}
		return s + "..."
	}

	pdf.SetFont("Helvetica", "", 8)
	pdf.SetTextColor(40, 40, 50)
	for i, r := range deselected {
		if pdf.GetY() > 270 {
			pdf.AddPage()
			drawHeaders()
			pdf.SetFont("Helvetica", "", 8)
			pdf.SetTextColor(40, 40, 50)
		}
		pdf.SetX(marginL)
		pdf.CellFormat(colNum, 6, fmt.Sprintf("%d", i+1), "0", 0, "C", false, 0, "")
		pdf.CellFormat(colRepo, 6, fit(tr(r.Repo), colRepo), "0", 0, "L", false, 0, "")
		pdf.CellFormat(colBranch, 6, fit(tr(r.Branch), colBranch), "0", 0, "L", false, 0, "")
		pdf.CellFormat(colOrg, 6, fit(tr(r.Org), colOrg), "0", 1, "L", false, 0, "")
	}
	pdf.SetTextColor(0, 0, 0)
}

// renderGlobalPDF generates the GlobalReport.pdf from languages and global info.
// deselected lists repositories the user removed from the totals on the results
// page, and rawTotalLOC is the unfiltered total shown alongside them for comparison.
func renderGlobalPDF(languages []LanguageData, ginfo Globalinfo, skippedRepos []SkippedRepo, summary *ScanSummary,
	deselected []DeselectedRepo, rawTotalLOC string) error {
	loggers := NewLogger()

	languages, maxLOC := prepareLanguagesForPDF(languages)

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

	// gofpdf's core fonts render text as Windows-1252, so UTF-8 strings (an em
	// dash, or an accented repository/organization name) would otherwise appear as
	// mojibake. tr converts UTF-8 into the font's encoding before it is drawn.
	tr := pdf.UnicodeTranslatorFromDescriptor("")

	logoPath := GetLogoPath()
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
		pdf.CellFormat(140, 5, tr("Organization: "+org), "", 0, "L", false, 0, "")
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
	// Translate before valOrNA so its byte-based truncation operates on the
	// single-byte Windows-1252 encoding (byte slice == rune slice) — this both
	// prevents mojibake for an accented largest-repo name and avoids splitting a
	// multi-byte UTF-8 sequence.
	// When repositories have been deselected the headline figures no longer describe
	// the whole scan, so the card titles say so rather than letting a reader assume
	// they do.
	locTitle, reposTitle := "Total LOC", "Repositories"
	if len(deselected) > 0 {
		locTitle, reposTitle = "Total LOC (filtered)", "Repositories (kept)"
	}
	cards := []statCard{
		{locTitle, valOrNA(tr(ginfo.TotalLinesOfCode)), 0, 115, 186},
		{reposTitle, fmt.Sprintf("%d", ginfo.NumberRepos), 0, 168, 110},
		{"Largest Repo", valOrNA(tr(ginfo.LargestRepository)), 241, 146, 49},
		{"Largest Repo LOC", valOrNA(tr(ginfo.LinesOfCodeLargestRepo)), 167, 86, 180},
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
	renderLanguageRows(pdf, languages, maxLOC, drawColHeaders, marginL, pageH, marginB, colNum, colLang, colLOC, colPct, colBar, rowH, barColors)

	// ── Scan summary section ─────────────────────────────────────────
	renderScanSummarySection(pdf, summary, len(skippedRepos), len(deselected), marginL, contentW)

	// ── Skipped repositories section ─────────────────────────────────
	renderSkippedReposSection(pdf, tr, skippedRepos, marginL, contentW)

	// ── Deselected repositories section ──────────────────────────────
	renderDeselectedReposSection(pdf, tr, deselected, rawTotalLOC, marginL, contentW)

	if err := pdf.OutputFileAndClose("Results/GlobalReport.pdf"); err != nil {
		loggers.Errorf("Error saving PDF file: %v", err)
		return err
	}
	return nil
}
