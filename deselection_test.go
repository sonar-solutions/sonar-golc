//go:build resultsall
// +build resultsall

package main

import (
	"bytes"
	"compress/zlib"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/SonarSource-Demos/sonar-golc/pkg/utils"
)

// pdfText extracts the drawn text strings from a PDF so assertions can check what a
// reader would actually see, rather than only that a file exists.
func pdfText(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	var decoded strings.Builder
	for _, m := range regexp.MustCompile(`(?s)stream\r?\n(.*?)endstream`).FindAllSubmatch(raw, -1) {
		r, err := zlib.NewReader(bytes.NewReader(m[1]))
		if err != nil {
			continue
		}
		var buf bytes.Buffer
		if _, err := buf.ReadFrom(r); err == nil {
			decoded.Write(buf.Bytes())
		}
		_ = r.Close()
	}
	// PDF string literals escape parentheses, so "(filtered)" is stored as
	// "\(filtered\)". The escape-aware alternation is required: a plain `\((.*?)\)`
	// stops at the escaped closing paren and truncates the label, which silently
	// breaks any assertion about parenthesized text.
	var text strings.Builder
	for _, m := range regexp.MustCompile(`\(((?:\\.|[^\\()])*)\)`).FindAllStringSubmatch(decoded.String(), -1) {
		text.WriteString(m[1])
	}
	return strings.NewReplacer(`\(`, "(", `\)`, ")", `\\`, `\`).Replace(text.String())
}

// The fixture's arithmetic, named so assertions read as intent rather than magic
// strings: "keep" contributes 1000 code lines and "drop" 250, so the full scan totals
// 1250 and deselecting "drop" leaves 1000.
var (
	rawTotalLOC      = utils.FormatCodeLines(1250)
	filteredTotalLOC = utils.FormatCodeLines(1000)
)

// setupResultsFixture builds a minimal but complete Results tree in a temp working
// directory: two repositories, one large and one small, with the by-file and
// by-language reports and the global artifacts the page reads.
func setupResultsFixture(t *testing.T) {
	t.Helper()

	dir := t.TempDir()
	origWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(origWD) })

	for _, sub := range []string{
		"Results/config",
		"Results/byfile-report/csv-report",
		"Results/byfile-report/pdf-report",
		"Results/bylanguage-report",
	} {
		if err := os.MkdirAll(sub, 0755); err != nil {
			t.Fatal(err)
		}
	}

	writeJSON := func(path string, v any) {
		data, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, data, 0644); err != nil {
			t.Fatal(err)
		}
	}

	writeJSON("Results/config/analysis_result_github.json", map[string]any{
		"NumRepositories": 2,
		"ProjectBranches": []map[string]any{
			{"Org": "acme", "RepoSlug": "keep", "MainBranch": "main"},
			{"Org": "acme", "RepoSlug": "drop", "MainBranch": "main"},
		},
	})

	writeJSON("Results/byfile-report/Result_acme__keep__main_byfile.json", map[string]int{
		"TotalLines": 1200, "TotalBlankLines": 100, "TotalComments": 100, "TotalCodeLines": 1000,
	})
	writeJSON("Results/byfile-report/Result_acme__drop__main_byfile.json", map[string]int{
		"TotalLines": 300, "TotalBlankLines": 20, "TotalComments": 30, "TotalCodeLines": 250,
	})

	writeJSON("Results/bylanguage-report/Result_acme__keep__main.json", map[string]any{
		"Results": []map[string]any{{"Language": "Go", "CodeLines": 1000}},
	})
	writeJSON("Results/bylanguage-report/Result_acme__drop__main.json", map[string]any{
		"Results": []map[string]any{{"Language": "Java", "CodeLines": 250}},
	})

	writeJSON("Results/code_lines_by_language.json", []map[string]any{
		{"Language": "Go", "CodeLines": 1000},
		{"Language": "Java", "CodeLines": 250},
	})
	writeJSON("Results/GlobalReport.json", map[string]any{
		"Organization":           "acme",
		"TotalLinesOfCode":       "1.25K",
		"LargestRepository":      "keep",
		"LinesOfCodeLargestRepo": "1.00K",
		"DevOpsPlatform":         "github",
		"NumberRepos":            2,
	})
}

func TestApplyDeselectionFiltersEveryTotal(t *testing.T) {
	setupResultsFixture(t)

	resp, err := applyDeselection([]string{utils.DeselectionKey("acme", "drop", "main")})
	if err != nil {
		t.Fatalf("applyDeselection: %v", err)
	}

	if resp.DeselectedCount != 1 {
		t.Errorf("DeselectedCount = %d, want 1", resp.DeselectedCount)
	}
	if resp.CountedRepositories != 1 {
		t.Errorf("CountedRepositories = %d, want 1", resp.CountedRepositories)
	}
	// 1000 kept, 250 removed. The raw figure must still be reported so the drop is
	// visible rather than silent.
	if want := utils.FormatCodeLines(1000); resp.TotalLinesOfCode != want {
		t.Errorf("TotalLinesOfCode = %q, want %q", resp.TotalLinesOfCode, want)
	}
	if resp.RawTotalLinesOfCode != "1.25K" {
		t.Errorf("RawTotalLinesOfCode = %q, want 1.25K", resp.RawTotalLinesOfCode)
	}

	// The published view must agree with the response.
	pd := snapshot()
	if len(pd.Repositories) != 1 || pd.Repositories[0].Repository != "keep" {
		t.Errorf("Repositories = %+v, want only keep", pd.Repositories)
	}
	if len(pd.Deselected) != 1 || pd.Deselected[0].Repository != "drop" {
		t.Errorf("Deselected = %+v, want only drop", pd.Deselected)
	}

	// The regenerated language breakdown must have dropped Java entirely, since the
	// only Java came from the deselected repository. This is the check that the page
	// chart and the reports describe the same repository set.
	for _, lang := range pd.RawLanguages {
		if strings.TrimSpace(lang.Language) == "Java" && lang.CodeLines != 0 {
			t.Errorf("Java still present with %d lines after deselecting its only repo", lang.CodeLines)
		}
	}

	// Applying a selection must NOT generate reports: that work now happens when a
	// report is requested, so the user never waits for PDFs they may not download.
	if _, err := os.Stat(customizedVariant.globalPDFPath()); !os.IsNotExist(err) {
		t.Errorf("applying a selection should not generate reports, but %s exists (err=%v)",
			customizedVariant.globalPDFPath(), err)
	}
}

func TestApplyDeselectionIsReversible(t *testing.T) {
	setupResultsFixture(t)

	if _, err := applyDeselection([]string{utils.DeselectionKey("acme", "drop", "main")}); err != nil {
		t.Fatalf("applyDeselection: %v", err)
	}

	// Resetting to the full scan must restore the original figures exactly — that is
	// what makes the feature safe to experiment with in front of a customer.
	resp, err := applyDeselection(nil)
	if err != nil {
		t.Fatalf("applyDeselection(nil): %v", err)
	}
	if resp.DeselectedCount != 0 {
		t.Errorf("DeselectedCount = %d, want 0", resp.DeselectedCount)
	}
	if resp.CountedRepositories != 2 {
		t.Errorf("CountedRepositories = %d, want 2", resp.CountedRepositories)
	}

	pd := snapshot()
	if pd.GlobalReport.TotalLinesOfCode != "1.25K" {
		t.Errorf("TotalLinesOfCode = %q, want the scanned 1.25K", pd.GlobalReport.TotalLinesOfCode)
	}
	if pd.GlobalReport.NumberRepos != 2 {
		t.Errorf("NumberRepos = %d, want 2", pd.GlobalReport.NumberRepos)
	}
	if len(pd.Deselected) != 0 {
		t.Errorf("Deselected = %+v, want empty", pd.Deselected)
	}
	if got := utils.LoadDeselectedRepos(resultsBaseDir); len(got) != 0 {
		t.Errorf("persisted selection = %+v, want empty", got)
	}
}

func TestApplyDeselectionIgnoresUnknownKeys(t *testing.T) {
	setupResultsFixture(t)

	// Keys come from the browser, so anything not matching an analyzed repository
	// must be dropped rather than persisted as a no-op entry.
	resp, err := applyDeselection([]string{"not__a__repo", utils.DeselectionKey("acme", "drop", "main")})
	if err != nil {
		t.Fatalf("applyDeselection: %v", err)
	}
	if resp.Ignored != 1 {
		t.Errorf("Ignored = %d, want 1", resp.Ignored)
	}
	if resp.DeselectedCount != 1 {
		t.Errorf("DeselectedCount = %d, want 1", resp.DeselectedCount)
	}
	for _, rec := range utils.LoadDeselectedRepos(resultsBaseDir) {
		if rec.Key == "not__a__repo" {
			t.Error("unknown key was persisted")
		}
	}
}

func TestApplyDeselectionDeduplicatesKeys(t *testing.T) {
	setupResultsFixture(t)

	key := utils.DeselectionKey("acme", "drop", "main")
	resp, err := applyDeselection([]string{key, key})
	if err != nil {
		t.Fatalf("applyDeselection: %v", err)
	}
	if resp.DeselectedCount != 1 {
		t.Errorf("DeselectedCount = %d, want 1", resp.DeselectedCount)
	}
}

func TestApplyDeselectionRefusesToDeselectEverything(t *testing.T) {
	setupResultsFixture(t)

	_, err := applyDeselection([]string{
		utils.DeselectionKey("acme", "keep", "main"),
		utils.DeselectionKey("acme", "drop", "main"),
	})
	if err == nil {
		t.Fatal("expected an error when every repository is deselected")
	}

	// The rejected request must leave nothing behind: no persisted selection and no
	// regenerated zero-LOC reports.
	if got := utils.LoadDeselectedRepos(resultsBaseDir); len(got) != 0 {
		t.Errorf("persisted selection = %+v, want empty after a rejected request", got)
	}
}

func TestHandleDeselectedRejectsBadInput(t *testing.T) {
	setupResultsFixture(t)

	t.Run("malformed body", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/deselected", strings.NewReader("{not json"))
		rec := httptest.NewRecorder()
		handleDeselected(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
		}
	})

	t.Run("wrong method", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodDelete, "/api/deselected", nil)
		rec := httptest.NewRecorder()
		handleDeselected(rec, req)
		if rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
		}
	})

	t.Run("deselecting everything", func(t *testing.T) {
		body, _ := json.Marshal(DeselectionRequest{Keys: []string{
			utils.DeselectionKey("acme", "keep", "main"),
			utils.DeselectionKey("acme", "drop", "main"),
		}})
		req := httptest.NewRequest(http.MethodPost, "/api/deselected", bytes.NewReader(body))
		rec := httptest.NewRecorder()
		handleDeselected(rec, req)
		if rec.Code != http.StatusInternalServerError {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
		}
	})
}

func TestHandleDeselectedRoundTrip(t *testing.T) {
	setupResultsFixture(t)

	pd, err := loadApplicationData()
	if err != nil {
		t.Fatalf("loadApplicationData: %v", err)
	}
	publish(pd)

	body, _ := json.Marshal(DeselectionRequest{Keys: []string{utils.DeselectionKey("acme", "drop", "main")}})
	rec := httptest.NewRecorder()
	handleDeselected(rec, httptest.NewRequest(http.MethodPost, "/api/deselected", bytes.NewReader(body)))
	if rec.Code != http.StatusOK {
		t.Fatalf("POST status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}

	// GET must then report what was deselected.
	rec = httptest.NewRecorder()
	handleDeselected(rec, httptest.NewRequest(http.MethodGet, "/api/deselected", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET status = %d, want 200", rec.Code)
	}
	var got []RepositoryData
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("GET body is not valid JSON: %v", err)
	}
	if len(got) != 1 || got[0].Repository != "drop" {
		t.Errorf("GET returned %+v, want one entry for drop", got)
	}
}

// serveReport drives the real download handler and returns the recorder.
func serveReport(t *testing.T, name string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	handleReport(rec, httptest.NewRequest(http.MethodGet, "/reports/"+name, nil))
	return rec
}

func TestReportGeneratedOnFirstRequest(t *testing.T) {
	setupResultsFixture(t)

	// Nothing has generated reports yet.
	if _, err := os.Stat(fullScanVariant.globalPDFPath()); !os.IsNotExist(err) {
		t.Fatalf("fixture should start without a global PDF (err=%v)", err)
	}

	rec := serveReport(t, "global-report.pdf")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}
	if rec.Body.Len() == 0 {
		t.Error("served an empty report")
	}
	// The download name must say which variant this is, since the file gets detached
	// from the dashboard and shared.
	if cd := rec.Header().Get("Content-Disposition"); !strings.Contains(cd, "full-scan") {
		t.Errorf("Content-Disposition = %q, want it to identify the full-scan variant", cd)
	}
	if info, err := os.Stat(fullScanVariant.globalPDFPath()); err != nil || info.Size() == 0 {
		t.Errorf("requesting the report should have generated it (err=%v)", err)
	}
}

func TestReportNotRegeneratedWhenAlreadyCurrent(t *testing.T) {
	setupResultsFixture(t)

	if rec := serveReport(t, "global-report.pdf"); rec.Code != http.StatusOK {
		t.Fatalf("first request failed: %d", rec.Code)
	}
	first, err := os.Stat(fullScanVariant.globalPDFPath())
	if err != nil {
		t.Fatal(err)
	}

	// Make a stale rebuild detectable: any regeneration rewrites the file.
	if err := os.Chtimes(fullScanVariant.globalPDFPath(),
		first.ModTime().Add(-time.Hour), first.ModTime().Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	marker, err := os.Stat(fullScanVariant.globalPDFPath())
	if err != nil {
		t.Fatal(err)
	}

	if rec := serveReport(t, "global-report.pdf"); rec.Code != http.StatusOK {
		t.Fatalf("second request failed: %d", rec.Code)
	}

	after, err := os.Stat(fullScanVariant.globalPDFPath())
	if err != nil {
		t.Fatal(err)
	}
	if !after.ModTime().Equal(marker.ModTime()) {
		t.Error("an unchanged selection should not trigger a rebuild")
	}
}

func TestSelectionChangeInvalidatesCachedReport(t *testing.T) {
	setupResultsFixture(t)

	if rec := serveReport(t, "global-report.pdf"); rec.Code != http.StatusOK {
		t.Fatalf("first request failed: %d", rec.Code)
	}

	// The full-scan report must be rebuilt after a selection change too, because its
	// freshness stamp is not only about the selection — but its *content* must not
	// change, since it always covers every repository.
	if _, err := applyDeselection([]string{utils.DeselectionKey("acme", "drop", "main")}); err != nil {
		t.Fatalf("applyDeselection: %v", err)
	}

	rec := serveReport(t, "global-report.pdf")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	fullScanText := pdfText(t, fullScanVariant.globalPDFPath())
	if !strings.Contains(fullScanText, rawTotalLOC) {
		t.Errorf("the full-scan report must still show %s, the total across all repositories", rawTotalLOC)
	}
	if strings.Contains(fullScanText, "Deselected Repositories") {
		t.Error("the full-scan report must not mention deselections — it covers the whole scan")
	}
}

func TestCustomizedReportIsSeparateFromOriginal(t *testing.T) {
	setupResultsFixture(t)

	if _, err := applyDeselection([]string{utils.DeselectionKey("acme", "drop", "main")}); err != nil {
		t.Fatalf("applyDeselection: %v", err)
	}

	if rec := serveReport(t, "global-report.pdf"); rec.Code != http.StatusOK {
		t.Fatalf("full-scan request failed: %d", rec.Code)
	}
	rec := serveReport(t, "global-report-customized.pdf")
	if rec.Code != http.StatusOK {
		t.Fatalf("customized request failed: %d (%s)", rec.Code, rec.Body.String())
	}
	if cd := rec.Header().Get("Content-Disposition"); !strings.Contains(cd, "selection") {
		t.Errorf("Content-Disposition = %q, want it to identify the customized variant", cd)
	}

	// Two distinct files, so handing over one never destroys the other.
	if fullScanVariant.globalPDFPath() == customizedVariant.globalPDFPath() {
		t.Fatal("variants must not share an output path")
	}

	full := pdfText(t, fullScanVariant.globalPDFPath())
	custom := pdfText(t, customizedVariant.globalPDFPath())

	if !strings.Contains(full, rawTotalLOC) {
		t.Errorf("full-scan report should show the unfiltered total %s", rawTotalLOC)
	}
	if !strings.Contains(custom, filteredTotalLOC) {
		t.Errorf("customized report should show the filtered total %s", filteredTotalLOC)
	}
	if !strings.Contains(custom, "Deselected") || !strings.Contains(custom, "drop") {
		t.Error("customized report must disclose what was excluded")
	}
	// The headline stat card must say the number is filtered, so a reader glancing at
	// the first page cannot mistake it for the whole scan.
	if !strings.Contains(custom, "Total LOC (filtered)") {
		t.Error("customized report's headline card must be labelled as filtered")
	}
	if strings.Contains(full, "Total LOC (filtered)") {
		t.Error("full-scan report's headline card must not be labelled as filtered")
	}
	// The customized report must also state the unfiltered total for comparison.
	if !strings.Contains(custom, rawTotalLOC) {
		t.Errorf("customized report should state the unfiltered total %s", rawTotalLOC)
	}

	// The customized language totals must not clobber the full-scan ones.
	if fullScanVariant.languageTotalsPath() == customizedVariant.languageTotalsPath() {
		t.Error("variants must not share a language-totals path")
	}
}

func TestCustomizedReportFallsBackWhenNothingDeselected(t *testing.T) {
	setupResultsFixture(t)

	// A link left over from a selection that has since been reset must still serve
	// something sensible rather than 404.
	rec := serveReport(t, "global-report-customized.pdf")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}
	if cd := rec.Header().Get("Content-Disposition"); !strings.Contains(cd, "full-scan") {
		t.Errorf("Content-Disposition = %q, want the full-scan name when nothing is deselected", cd)
	}
}

func TestUnknownReportIs404(t *testing.T) {
	setupResultsFixture(t)
	if rec := serveReport(t, "not-a-report.pdf"); rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

func TestPageLanguagesIgnoreStaleAggregateWhenFiltered(t *testing.T) {
	setupResultsFixture(t)

	// A deliberately wrong aggregate file, as would be left behind by an earlier
	// unfiltered generation. The page must compute from the per-repository files instead.
	stale := []map[string]any{{"Language": "Cobol", "CodeLines": 999999}}
	data, _ := json.Marshal(stale)
	if err := os.WriteFile(codeLinesLanguageFile, data, 0644); err != nil {
		t.Fatal(err)
	}

	if _, err := applyDeselection([]string{utils.DeselectionKey("acme", "drop", "main")}); err != nil {
		t.Fatalf("applyDeselection: %v", err)
	}

	for _, lang := range snapshot().RawLanguages {
		if lang.Language == "Cobol" {
			t.Fatal("page used the stale aggregate file instead of the per-repository results")
		}
	}
	// Java came only from the deselected repository, so it must be gone.
	for _, lang := range snapshot().RawLanguages {
		if lang.Language == "Java" {
			t.Errorf("Java should not appear: its only repository was deselected")
		}
	}
}

func TestReportsDropdownOffersBothVariantsWhenFiltered(t *testing.T) {
	out := renderTemplate(t, PageData{
		Platform:            "github",
		Repositories:        []RepositoryData{{Number: 1, Key: "acme__keep__main", Repository: "keep", Branch: "main"}},
		Deselected:          []RepositoryData{{Number: 1, Key: "acme__drop__main", Repository: "drop", Branch: "main"}},
		DeselectedKeys:      []string{"acme__drop__main"},
		DeselectedCount:     1,
		ScannedRepositories: 2,
	})
	for _, want := range []string{
		"/reports/global-report.pdf",
		"/reports/global-report-customized.pdf",
		"/reports/repository-summary-customized.pdf",
		"/reports/repository-summary-customized.csv",
		"Full scan",
		"Current selection",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("dropdown missing %q", want)
		}
	}
}

func TestReportsDropdownOffersOnlyOriginalWhenUnfiltered(t *testing.T) {
	out := renderTemplate(t, PageData{
		Platform:     "github",
		Repositories: []RepositoryData{{Number: 1, Key: "acme__keep__main", Repository: "keep", Branch: "main"}},
	})
	if strings.Contains(out, "-customized.pdf") {
		t.Error("customized report links should not be offered when nothing is deselected")
	}
	if !strings.Contains(out, "/reports/global-report.pdf") {
		t.Error("the full-scan report must always be offered")
	}
}

func TestPartitionDeselectedPage(t *testing.T) {
	repos := []RepositoryData{
		{Number: 1, Key: "acme__a__main", Repository: "a"},
		{Number: 2, Key: "acme__b__main", Repository: "b"},
		{Number: 3, Key: "acme__c__main", Repository: "c"},
	}
	kept, removed := partitionDeselected(repos, utils.DeselectionSet{"acme__b__main": true})

	if len(kept) != 2 || len(removed) != 1 {
		t.Fatalf("kept %d removed %d, want 2 and 1", len(kept), len(removed))
	}
	if kept[0].Number != 1 || kept[1].Number != 2 || removed[0].Number != 1 {
		t.Error("each group should be renumbered from 1")
	}
}

func TestAdjustGlobalInfoPageUntouchedWhenNothingDeselected(t *testing.T) {
	in := Globalinfo{
		TotalLinesOfCode:       "9.99M",
		LargestRepository:      "mono",
		LinesOfCodeLargestRepo: "1.00M",
		NumberRepos:            7,
	}
	if got := adjustGlobalInfo(in, []LanguageData{{Language: "Go", CodeLines: 1}}, nil, 0); got != in {
		t.Errorf("adjustGlobalInfo changed an unfiltered report:\n got %+v\nwant %+v", got, in)
	}
}

func TestRepositoryTableRendersSelectionControls(t *testing.T) {
	out := renderTemplate(t, PageData{
		Platform: "github",
		Repositories: []RepositoryData{
			{Number: 1, Key: "acme__keep__main", Repository: "keep", Branch: "main", CodeLines: 1000, CodeLinesF: "1.00K"},
		},
		Deselected: []RepositoryData{
			{Number: 1, Key: "acme__drop__main", Repository: "drop", Branch: "main", CodeLines: 250, CodeLinesF: "250"},
		},
		DeselectedKeys:      []string{"acme__drop__main"},
		DeselectedCount:     1,
		DeselectedCodeLines: "250",
		RawTotalLinesOfCode: "1.25K",
	})

	for _, want := range []string{
		`id="btnApplySelection"`,
		`id="btnResetSelection"`,
		`id="selectAllCheckbox"`,
		`class="form-check-input repo-select"`,
		`value="acme__keep__main"`,
		`value="acme__drop__main"`,
		"deselected-row",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("rendered page missing %q", want)
		}
	}

	// The banner has to state both what was removed and the unfiltered total.
	if !strings.Contains(out, "1.25K") {
		t.Error("rendered page should show the unfiltered total for comparison")
	}
}

func TestRepositoryTableOmitsBannerWhenUnfiltered(t *testing.T) {
	out := renderTemplate(t, PageData{
		Platform: "github",
		Repositories: []RepositoryData{
			{Number: 1, Key: "acme__keep__main", Repository: "keep", Branch: "main"},
		},
	})
	if strings.Contains(out, "deselected-row") {
		t.Error("no deselected rows should render on an unfiltered page")
	}
	if strings.Contains(out, "are excluded from every") {
		t.Error("the exclusion banner should be omitted on an unfiltered page")
	}
}

func TestClearedSelectionSurvivesReload(t *testing.T) {
	setupResultsFixture(t)

	// Simulate the scan clearing a stale selection, as golc does at the end of a run.
	if err := utils.SaveDeselectedRepos(resultsBaseDir, []utils.DeselectedRepo{
		{Key: utils.DeselectionKey("acme", "drop", "main"), Repo: "drop"},
	}); err != nil {
		t.Fatal(err)
	}
	if err := utils.ClearDeselectedRepos(resultsBaseDir); err != nil {
		t.Fatal(err)
	}

	pd, err := loadApplicationData()
	if err != nil {
		t.Fatalf("loadApplicationData: %v", err)
	}
	if pd.DeselectedCount != 0 {
		t.Errorf("DeselectedCount = %d, want 0 after a clear", pd.DeselectedCount)
	}
	if len(pd.Repositories) != 2 {
		t.Errorf("got %d repositories, want 2", len(pd.Repositories))
	}
	if _, err := os.Stat(filepath.Join(resultsBaseDir, "config", "deselected_repos.json")); !os.IsNotExist(err) {
		t.Errorf("selection file should be gone, stat err = %v", err)
	}
}
