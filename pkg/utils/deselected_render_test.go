package utils

import (
	"bytes"
	"compress/zlib"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/jung-kurt/gofpdf"
)

// pdfSectionText renders a PDF to a temp file and extracts its drawn text, so a section's
// content can be asserted rather than only that rendering did not panic.
//
// The escape-aware capture is required: PDF escapes parentheses, and a plain `\((.*?)\)`
// truncates at the escaped closing paren, quietly dropping any parenthesized label.
func pdfSectionText(t *testing.T, pdf *gofpdf.Fpdf) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "section.pdf")
	if err := pdf.OutputFileAndClose(path); err != nil {
		t.Fatalf("writing pdf: %v", err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading pdf: %v", err)
	}

	var decoded strings.Builder
	for _, m := range regexp.MustCompile(`(?s)stream\r?\n(.*?)endstream`).FindAllSubmatch(raw, -1) {
		zr, err := zlib.NewReader(bytes.NewReader(m[1]))
		if err != nil {
			continue
		}
		var buf bytes.Buffer
		if _, err := buf.ReadFrom(zr); err == nil {
			decoded.Write(buf.Bytes())
		}
		_ = zr.Close()
	}

	var text strings.Builder
	for _, m := range regexp.MustCompile(`\(((?:\\.|[^\\()])*)\)`).FindAllStringSubmatch(decoded.String(), -1) {
		text.WriteString(m[1])
	}
	return strings.NewReplacer(`\(`, "(", `\)`, ")", `\\`, `\`).Replace(text.String())
}

// newSectionPDF returns a PDF ready for a section renderer to draw into.
func newSectionPDF(t *testing.T) *gofpdf.Fpdf {
	t.Helper()
	pdf := gofpdf.New("P", "mm", "A4", "")
	pdf.SetMargins(15, 15, 15)
	pdf.SetAutoPageBreak(true, 15)
	pdf.AddPage()
	pdf.SetFont("Helvetica", "", 8)
	return pdf
}

func TestRenderDeselectedReposSectionListsWhatWasRemoved(t *testing.T) {
	pdf := newSectionPDF(t)
	tr := pdf.UnicodeTranslatorFromDescriptor("")

	deselected := []DeselectedRepo{
		{Key: "acme__dead-repo__main", Org: testOrgAcme, Repo: "dead-repo", Branch: testBranchMain},
		{Key: "acme__vendored__master", Org: testOrgAcme, Repo: "vendored", Branch: "master"},
	}
	renderDeselectedReposSection(pdf, tr, deselected, "4.20M", 15, 180)

	text := pdfSectionText(t, pdf)

	// The count, every removed repository, and the unfiltered total must all be present:
	// this section is what stops a filtered report from understating a scan silently.
	if !strings.Contains(text, "Deselected Repositories (2)") {
		t.Errorf("section heading missing or wrong count: %q", text)
	}
	for _, want := range []string{"dead-repo", "vendored", testBranchMain, "master", testOrgAcme, "4.20M"} {
		if !strings.Contains(text, want) {
			t.Errorf("section missing %q", want)
		}
	}
	if !strings.Contains(text, "ORG / PROJECT") {
		t.Error("section should have column headers")
	}
}

func TestRenderDeselectedReposSectionRendersNothingWhenUntouched(t *testing.T) {
	pdf := newSectionPDF(t)
	tr := pdf.UnicodeTranslatorFromDescriptor("")

	before := pdf.GetY()
	renderDeselectedReposSection(pdf, tr, nil, "4.20M", 15, 180)

	// An ordinary report must be completely unaffected — not even whitespace advanced.
	if pdf.GetY() != before {
		t.Errorf("cursor moved from %v to %v with nothing deselected", before, pdf.GetY())
	}
	if text := pdfSectionText(t, pdf); strings.Contains(text, "Deselected") {
		t.Errorf("nothing should be drawn, got %q", text)
	}
}

func TestRenderDeselectedReposSectionPaginatesLongLists(t *testing.T) {
	pdf := newSectionPDF(t)
	tr := pdf.UnicodeTranslatorFromDescriptor("")

	// Enough rows to spill past one page, exercising the header-repeat path.
	deselected := make([]DeselectedRepo, 0, 80)
	for i := 0; i < 80; i++ {
		deselected = append(deselected, DeselectedRepo{
			Repo:   "repo-" + string(rune('a'+i%26)) + strings.Repeat("x", i%5),
			Branch: testBranchMain,
			Org:    testOrgAcme,
		})
	}
	renderDeselectedReposSection(pdf, tr, deselected, "9.99M", 15, 180)

	if pages := pdf.PageNo(); pages < 2 {
		t.Errorf("80 rows should span more than one page, got %d", pages)
	}
	if text := pdfSectionText(t, pdf); strings.Count(text, "ORG / PROJECT") < 2 {
		t.Error("column headers should repeat after a page break")
	}
}

func TestRenderDeselectedReposSectionTruncatesLongNames(t *testing.T) {
	pdf := newSectionPDF(t)
	tr := pdf.UnicodeTranslatorFromDescriptor("")

	renderDeselectedReposSection(pdf, tr, []DeselectedRepo{{
		Repo:   strings.Repeat("very-long-repository-name-", 8),
		Branch: strings.Repeat("very-long-branch-name-", 8),
		Org:    strings.Repeat("very-long-org-name-", 8),
	}}, "1.00K", 15, 180)

	// Over-long values must be ellipsised rather than overrun into the next column.
	if text := pdfSectionText(t, pdf); !strings.Contains(text, "...") {
		t.Errorf("long values should be truncated with an ellipsis: %q", text)
	}
}

func TestRenderTopRepositoriesSectionShowsSharesAndLanguages(t *testing.T) {
	pdf := newSectionPDF(t)
	tr := pdf.UnicodeTranslatorFromDescriptor("")

	repoTotals := []RepoTotal{
		{Repo: "big", Branch: testBranchMain, CodeLines: 900, PrimaryLanguage: "Go"},
		{Repo: "small", Branch: "develop", CodeLines: 100, PrimaryLanguage: testLangJava},
	}
	renderTopRepositoriesSection(pdf, tr, repoTotals, 1000, 15, 180)

	text := pdfSectionText(t, pdf)
	for _, want := range []string{"Top 2 Repositories by Lines of Code", "big", "small", "Go", testLangJava,
		testBranchMain, "develop", "90.0%", "10.0%", "MAIN LANGUAGE", "SHARE %"} {
		if !strings.Contains(text, want) {
			t.Errorf("section missing %q", want)
		}
	}
}

func TestRenderTopRepositoriesSectionHandlesMissingData(t *testing.T) {
	pdf := newSectionPDF(t)
	tr := pdf.UnicodeTranslatorFromDescriptor("")

	// No language, and a zero overall total: neither may produce a blank cell or a
	// division by zero.
	renderTopRepositoriesSection(pdf, tr, []RepoTotal{{Repo: "mystery", Branch: testBranchMain, CodeLines: 5}}, 0, 15, 180)

	text := pdfSectionText(t, pdf)
	if !strings.Contains(text, "mystery") {
		t.Error("repository should still be listed")
	}
	if !strings.Contains(text, "-") {
		t.Error("unknown language and unknown share should render as a dash")
	}
}

func TestRenderTopRepositoriesSectionRendersNothingWithoutRepos(t *testing.T) {
	pdf := newSectionPDF(t)
	tr := pdf.UnicodeTranslatorFromDescriptor("")

	before := pdf.GetY()
	// Only zero-LOC repositories, which rank out entirely.
	renderTopRepositoriesSection(pdf, tr, []RepoTotal{{Repo: "empty", CodeLines: 0}}, 100, 15, 180)

	if pdf.GetY() != before {
		t.Errorf("cursor moved from %v to %v with nothing to list", before, pdf.GetY())
	}
}

// manyRepoTotals builds n ranked repositories for the top-repositories table.
func manyRepoTotals(n int) []RepoTotal {
	repoTotals := make([]RepoTotal, 0, n)
	for i := 0; i < n; i++ {
		repoTotals = append(repoTotals, RepoTotal{
			Repo: "repo" + string(rune('A'+i%26)), Branch: testBranchMain, CodeLines: (i + 1) * 10, PrimaryLanguage: "Go",
		})
	}
	return repoTotals
}

func TestRenderTopRepositoriesSectionStatesTotalWhenTruncated(t *testing.T) {
	pdf := newSectionPDF(t)
	tr := pdf.UnicodeTranslatorFromDescriptor("")

	renderTopRepositoriesSection(pdf, tr, manyRepoTotals(TopRepositoriesShown+12), 100000, 15, 180)

	text := pdfSectionText(t, pdf)
	// A reader must be able to tell the list is a subset, not the whole scan.
	if !strings.Contains(text, "of 42") {
		t.Errorf("heading should state the full repository count when truncated: %q", text)
	}
	// Exactly the cap is listed, not everything.
	if strings.Contains(text, "Top 42") {
		t.Error("heading should state the capped count, not the total")
	}
}

func TestRenderTopRepositoriesSectionPaginates(t *testing.T) {
	pdf := newSectionPDF(t)
	tr := pdf.UnicodeTranslatorFromDescriptor("")

	// In a real report this section follows the language breakdown, so it can start well
	// down the page. Starting near the bottom is what forces the page-break path.
	pdf.SetY(240)
	renderTopRepositoriesSection(pdf, tr, manyRepoTotals(TopRepositoriesShown), 100000, 15, 180)

	if pages := pdf.PageNo(); pages < 2 {
		t.Fatalf("a table starting near the page bottom should break, got %d page(s)", pages)
	}
	if text := pdfSectionText(t, pdf); strings.Count(text, "MAIN LANGUAGE") < 2 {
		t.Error("column headers should repeat after a page break")
	}
}

func TestFitLabelWithValueKeepsTheNumber(t *testing.T) {
	// The cell exists to report a figure, so the figure must survive truncation. Trimming
	// the whole "name value" string from the end removes the number first — and worse,
	// half-trims it: "Objective-C++ 123.45K" became "Objective-C++ 123...", which reads as
	// 123 lines rather than 123 thousand. A shortened language name is still recognisable;
	// a mangled number is a wrong figure in a customer-facing report.
	pdf := newSectionPDF(t)
	tr := pdf.UnicodeTranslatorFromDescriptor("")

	cases := []struct{ name, value string }{
		{"Python", "67.14K"},                          // fits as-is
		{"JavaScript", "2.87K"},                       // fits as-is
		{"Objective-C++", "123.45K"},                  // needs truncation at 28mm
		{"Visual Basic .NET", "1.23M"},                // needs more truncation
		{"An Absurdly Long Language Name", "999.99M"}, // extreme
	}
	for _, tc := range cases {
		got := fitLabelWithValue(pdf, tr(tc.name), tr(tc.value), colPDFLanguage)
		if !strings.HasSuffix(got, tc.value) {
			t.Errorf("fitLabelWithValue(%q, %q) = %q: the value must survive intact",
				tc.name, tc.value, got)
		}
		if w := pdf.GetStringWidth(got); w > colPDFLanguage-2 {
			t.Errorf("fitLabelWithValue(%q, %q) = %q: %.1fmm exceeds the %.0fmm column",
				tc.name, tc.value, got, w, colPDFLanguage-2)
		}
	}
}

func TestFitLabelWithValueLeavesShortLabelsAlone(t *testing.T) {
	pdf := newSectionPDF(t)
	if got := fitLabelWithValue(pdf, "Go", "12", colPDFLanguage); got != "Go 12" {
		t.Errorf("got %q, want %q — a label that fits must not be altered", got, "Go 12")
	}
}

func TestCollectResultTotalsExported(t *testing.T) {
	// The exported wrapper is what the results page uses to compute its language
	// breakdown in memory, so it must behave identically to the internal walk.
	base := t.TempDir()
	byLang := filepath.Join(base, "bylanguage-report")
	writeByLanguageResult(t, byLang, "Result_org__keep__main.json", []LanguageData1{
		{Language: "Go", CodeLines: 40},
		{Language: "JSON", CodeLines: 500},
	})
	writeByLanguageResult(t, byLang, "Result_org__drop__main.json", []LanguageData1{
		{Language: "Rust", CodeLines: 10},
	})

	totals, repoTotals, err := CollectResultTotals(base, DeselectionSet{
		DeselectionKey("org", testRepoDrop, testBranchMain): true,
	})
	if err != nil {
		t.Fatalf("CollectResultTotals: %v", err)
	}
	if totals["Go"] != 40 {
		t.Errorf("Go = %d, want 40", totals["Go"])
	}
	if _, present := totals["Rust"]; present {
		t.Error("the deselected repository's language must not be counted")
	}
	if len(repoTotals) != 1 || repoTotals[0].PrimaryLanguage != "Go" {
		t.Errorf("repoTotals = %+v, want one entry with Go as primary (JSON held out)", repoTotals)
	}
}

func TestPrimaryLanguage(t *testing.T) {
	if got := (RepositoryData{}).PrimaryLanguage(); got != "" {
		t.Errorf("PrimaryLanguage() = %q, want empty for a repository with no language data", got)
	}
	// The languages are already ranked, so the first is the primary one.
	repo := RepositoryData{TopLanguages: []LanguageShare{
		{Language: "Go", CodeLines: 90},
		{Language: testLangJava, CodeLines: 10},
	}}
	if got := repo.PrimaryLanguage(); got != "Go" {
		t.Errorf("PrimaryLanguage() = %q, want Go", got)
	}
}

func TestWriteLanguageTotalsJSONReportsUnwritableTarget(t *testing.T) {
	// A file where a parent directory is expected: the failure must be reported rather
	// than silently producing no language totals.
	base := t.TempDir()
	blocker := filepath.Join(base, "not-a-dir")
	if err := os.WriteFile(blocker, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}

	if _, err := writeLanguageTotalsJSON(map[string]int{"Go": 1},
		filepath.Join(blocker, "code_lines_by_language.json")); err == nil {
		t.Error("expected an error when the target directory cannot be created")
	}
}

func TestDeselectionKeyForRepoFilePlatform(t *testing.T) {
	// The file platform's result files carry only the directory name, so its key must
	// ignore the org and branch it is handed.
	got := DeselectionKeyForRepo("file", "ignored", "my-dir", "ignored-branch")
	if want := FileDeselectionKey("my-dir"); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestRepositoryCSVRowPadsMissingLanguages(t *testing.T) {
	// Every row must have the same width, or a spreadsheet misaligns the columns for
	// repositories with fewer than the shown number of languages.
	full := repositoryCSVRow(RepositoryData{
		Number: 1, Repository: "r", Branch: testBranchMain,
		TopLanguages: []LanguageShare{
			{Language: "Go", CodeLines: 3},
			{Language: testLangJava, CodeLines: 2},
			{Language: "XML", CodeLines: 1},
		},
	})
	sparse := repositoryCSVRow(RepositoryData{Number: 2, Repository: "r2", Branch: testBranchMain})
	one := repositoryCSVRow(RepositoryData{
		Number: 3, Repository: "r3", Branch: testBranchMain,
		TopLanguages: []LanguageShare{{Language: "Go", CodeLines: 3}},
	})

	if len(full) != len(sparse) || len(full) != len(one) {
		t.Fatalf("row widths differ: full=%d sparse=%d one=%d", len(full), len(sparse), len(one))
	}
	if full[len(full)-2] != "XML" {
		t.Errorf("third language should be in the penultimate column, got %q", full[len(full)-2])
	}
	for _, cell := range sparse[len(sparse)-TopLanguagesShown*2:] {
		if cell != "" {
			t.Errorf("missing languages should pad with empty cells, got %q", cell)
		}
	}
}

func TestRenderDeselectedTableSkippedWhenNothingDeselected(t *testing.T) {
	pdf := newSectionPDF(t)
	tr := pdf.UnicodeTranslatorFromDescriptor("")

	pagesBefore := pdf.PageNo()
	renderDeselectedTable(pdf, tr, &RepositorySummaryReport{}, "Code Lines", 6, 297, 15)

	// The section starts with AddPage, so an untouched selection must not add one.
	if pdf.PageNo() != pagesBefore {
		t.Errorf("page count changed from %d to %d with nothing deselected", pagesBefore, pdf.PageNo())
	}
}

func TestRenderDeselectedTablePaginatesLongLists(t *testing.T) {
	pdf := newSectionPDF(t)
	tr := pdf.UnicodeTranslatorFromDescriptor("")

	deselected := make([]RepositoryData, 0, 90)
	for i := 0; i < 90; i++ {
		deselected = append(deselected, RepositoryData{
			Number: i + 1, Repository: "repo", Branch: testBranchMain,
			LinesF: "1", CommentsF: "1", BlankLinesF: "1", CodeLinesF: "1",
		})
	}
	summary := &RepositorySummaryReport{
		DeselectedRepositories: len(deselected),
		DeselectedCodeLinesF:   "90",
		Deselected:             deselected,
	}
	renderDeselectedTable(pdf, tr, summary, "Code Lines", 6, 297, 15)

	if pdf.PageNo() < 3 {
		t.Errorf("90 rows should span several pages, got %d", pdf.PageNo())
	}
	text := pdfSectionText(t, pdf)
	if !strings.Contains(text, "Deselected Repositories (90)") {
		t.Errorf("heading missing or wrong count: %q", text)
	}
	if strings.Count(text, "Repository") < 2 {
		t.Error("table headers should repeat after a page break")
	}
}

func TestWriteLanguageTotalsJSONCreatesMissingDirectory(t *testing.T) {
	// Variant reports write their language totals into a directory that may not exist yet.
	base := t.TempDir()
	target := filepath.Join(base, "customized", "code_lines_by_language.json")

	data, err := writeLanguageTotalsJSON(map[string]int{"Go": 12, testLangJava: 30}, target)
	if err != nil {
		t.Fatalf("writeLanguageTotalsJSON: %v", err)
	}

	var langs []LanguageData1
	if err := json.Unmarshal(data, &langs); err != nil {
		t.Fatalf("returned bytes are not valid JSON: %v", err)
	}
	// Sorted biggest-first so regenerating the same input yields the same file.
	if len(langs) != 2 || langs[0].Language != testLangJava {
		t.Errorf("expected Java first, got %+v", langs)
	}
	if _, err := os.Stat(target); err != nil {
		t.Errorf("file should have been created: %v", err)
	}
}
