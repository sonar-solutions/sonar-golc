//go:build resultsall
// +build resultsall

package main

import (
	"archive/zip"
	"bytes"
	"compress/zlib"
	"encoding/json"
	"fmt"
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

// Fixture identities and report names, named rather than repeated as literals so a
// rename is one edit and a typo is a compile error.
const (
	orgAcme    = "acme"
	repoKeep   = "keep"
	repoDrop   = "drop"
	branchMain = "main"

	reportGlobal           = "global-report.pdf"
	reportGlobalCustomized = "global-report-customized.pdf"
	reportSummaryPDF       = "repository-summary.pdf"
	reportSummaryCSV       = "repository-summary.csv"

	msgApplyDeselection = "applyDeselection: %v"
)

// keyDrop is the deselection key the page submits for the fixture's smaller repository.
var keyDrop = utils.DeselectionKey(orgAcme, repoDrop, branchMain)

// The fixture's arithmetic, named so assertions read as intent rather than magic
// strings: repoKeep contributes 1000 code lines and repoDrop 250, so the full scan totals
// 1250 and deselecting repoDrop leaves 1000.
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
			{"Org": orgAcme, "RepoSlug": repoKeep, "MainBranch": branchMain},
			{"Org": orgAcme, "RepoSlug": repoDrop, "MainBranch": branchMain},
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
		"Organization":           orgAcme,
		"TotalLinesOfCode":       "1.25K",
		"LargestRepository":      repoKeep,
		"LinesOfCodeLargestRepo": "1.00K",
		"DevOpsPlatform":         "github",
		"NumberRepos":            2,
	})
}

// tablePageData builds a PageData the way loadApplicationData does, so template tests
// exercise the same field wiring the server produces instead of setting fields by hand and
// drifting from it. `all` is in ranked order; the named keys are the deselected ones.
func tablePageData(platform string, all []RepositoryData, deselectedKeys ...string) PageData {
	set := utils.DeselectionSet{}
	for _, k := range deselectedKeys {
		set[k] = true
	}
	kept, removed := partitionDeselected(all, set)
	keys := make([]string, 0, len(removed))
	for _, r := range removed {
		keys = append(keys, r.Key)
	}
	return PageData{
		Platform:            platform,
		Repositories:        kept,
		Deselected:          removed,
		TableRows:           buildTableRows(all, set),
		DeselectedKeys:      keys,
		DeselectedCount:     len(removed),
		ScannedRepositories: len(all),
		TopLanguagesShown:   utils.TopLanguagesShown,
	}
}

func TestApplyDeselectionFiltersEveryTotal(t *testing.T) {
	setupResultsFixture(t)

	resp, err := applyDeselection([]string{keyDrop})
	if err != nil {
		t.Fatalf(msgApplyDeselection, err)
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
	if len(pd.Repositories) != 1 || pd.Repositories[0].Repository != repoKeep {
		t.Errorf("Repositories = %+v, want only keep", pd.Repositories)
	}
	if len(pd.Deselected) != 1 || pd.Deselected[0].Repository != repoDrop {
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

	if _, err := applyDeselection([]string{keyDrop}); err != nil {
		t.Fatalf(msgApplyDeselection, err)
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
	resp, err := applyDeselection([]string{"not__a__repo", keyDrop})
	if err != nil {
		t.Fatalf(msgApplyDeselection, err)
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

	key := keyDrop
	resp, err := applyDeselection([]string{key, key})
	if err != nil {
		t.Fatalf(msgApplyDeselection, err)
	}
	if resp.DeselectedCount != 1 {
		t.Errorf("DeselectedCount = %d, want 1", resp.DeselectedCount)
	}
}

func TestApplyDeselectionRefusesToDeselectEverything(t *testing.T) {
	setupResultsFixture(t)

	_, err := applyDeselection([]string{
		utils.DeselectionKey(orgAcme, repoKeep, branchMain),
		keyDrop,
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
			utils.DeselectionKey(orgAcme, repoKeep, branchMain),
			keyDrop,
		}})
		req := httptest.NewRequest(http.MethodPost, "/api/deselected", bytes.NewReader(body))
		rec := httptest.NewRecorder()
		handleDeselected(rec, req)
		// 422, not 500: the request is well-formed and the server is healthy — the
		// instruction is just one that can never be carried out, so a caller must not be
		// told to retry it.
		if rec.Code != http.StatusUnprocessableEntity {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusUnprocessableEntity)
		}
		if !strings.Contains(rec.Body.String(), "at least one must remain counted") {
			t.Errorf("response should explain the constraint, got %q", rec.Body.String())
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

	body, _ := json.Marshal(DeselectionRequest{Keys: []string{keyDrop}})
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
	if len(got) != 1 || got[0].Repository != repoDrop {
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

	rec := serveReport(t, reportGlobal)
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

	if rec := serveReport(t, reportGlobal); rec.Code != http.StatusOK {
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

	if rec := serveReport(t, reportGlobal); rec.Code != http.StatusOK {
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

	if rec := serveReport(t, reportGlobal); rec.Code != http.StatusOK {
		t.Fatalf("first request failed: %d", rec.Code)
	}

	// The full-scan report must be rebuilt after a selection change too, because its
	// freshness stamp is not only about the selection — but its *content* must not
	// change, since it always covers every repository.
	if _, err := applyDeselection([]string{keyDrop}); err != nil {
		t.Fatalf(msgApplyDeselection, err)
	}

	rec := serveReport(t, reportGlobal)
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

	if _, err := applyDeselection([]string{keyDrop}); err != nil {
		t.Fatalf(msgApplyDeselection, err)
	}

	if rec := serveReport(t, reportGlobal); rec.Code != http.StatusOK {
		t.Fatalf("full-scan request failed: %d", rec.Code)
	}
	rec := serveReport(t, reportGlobalCustomized)
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
	if !strings.Contains(custom, "Deselected") || !strings.Contains(custom, repoDrop) {
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
	rec := serveReport(t, reportGlobalCustomized)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}
	if cd := rec.Header().Get("Content-Disposition"); !strings.Contains(cd, "full-scan") {
		t.Errorf("Content-Disposition = %q, want the full-scan name when nothing is deselected", cd)
	}
}

func TestResetRemovesStaleCustomizedReports(t *testing.T) {
	setupResultsFixture(t)

	if _, err := applyDeselection([]string{keyDrop}); err != nil {
		t.Fatalf(msgApplyDeselection, err)
	}
	if rec := serveReport(t, reportGlobalCustomized); rec.Code != http.StatusOK {
		t.Fatalf("customized request failed: %d", rec.Code)
	}
	if _, err := os.Stat(customizedVariant.globalPDFPath()); err != nil {
		t.Fatalf("customized report should exist before the reset: %v", err)
	}

	if _, err := applyDeselection(nil); err != nil {
		t.Fatalf("applyDeselection(nil): %v", err)
	}

	// The customized reports are filtered and understated, sit under ordinary file
	// names, and the ZIP archives the whole tree — leaving them behind means a reset
	// user can still hand one over believing it is current.
	if _, err := os.Stat(customizedReportsDir); !os.IsNotExist(err) {
		t.Errorf("%s should be removed when the selection is reset (err=%v)", customizedReportsDir, err)
	}
}

func TestZipExcludesStaleCustomizedReportsAfterReset(t *testing.T) {
	setupResultsFixture(t)

	if _, err := applyDeselection([]string{keyDrop}); err != nil {
		t.Fatalf(msgApplyDeselection, err)
	}
	if rec := serveReport(t, reportGlobalCustomized); rec.Code != http.StatusOK {
		t.Fatalf("customized request failed: %d", rec.Code)
	}
	if _, err := applyDeselection(nil); err != nil {
		t.Fatalf("applyDeselection(nil): %v", err)
	}

	rec := httptest.NewRecorder()
	zipResults(rec, httptest.NewRequest(http.MethodGet, "/download", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("zipResults status = %d, want 200", rec.Code)
	}

	archive, err := zip.NewReader(bytes.NewReader(rec.Body.Bytes()), int64(rec.Body.Len()))
	if err != nil {
		t.Fatalf("reading archive: %v", err)
	}
	for _, f := range archive.File {
		if strings.Contains(filepath.ToSlash(f.Name), "/"+utils.CustomizedReportsDirName+"/") {
			t.Errorf("archive still contains a stale customized report: %s", f.Name)
		}
	}
}

func TestResetWaitsForInFlightRebuildBeforePurging(t *testing.T) {
	setupResultsFixture(t)

	if _, err := applyDeselection([]string{keyDrop}); err != nil {
		t.Fatalf(msgApplyDeselection, err)
	}

	// Stand in for a background rebuild that is mid-write: hold regenerateMu, and only
	// create the customized artifact while holding it. A reset that removes the directory
	// without taking the lock would delete it *before* this write lands, leaving the
	// understated report on disk — which is the race being guarded against.
	regenerateMu.Lock()

	resetDone := make(chan error, 1)
	go func() {
		_, err := applyDeselection(nil)
		resetDone <- err
	}()

	// Give the reset a chance to reach the removal. It must block on the lock rather than
	// proceed; if the guard is missing it will have already deleted the directory here.
	time.Sleep(150 * time.Millisecond)

	if err := os.MkdirAll(filepath.Dir(customizedVariant.globalPDFPath()), 0755); err != nil {
		regenerateMu.Unlock()
		t.Fatal(err)
	}
	if err := os.WriteFile(customizedVariant.globalPDFPath(), []byte("stale filtered report"), 0644); err != nil {
		regenerateMu.Unlock()
		t.Fatal(err)
	}

	regenerateMu.Unlock()

	if err := <-resetDone; err != nil {
		t.Fatalf("reset failed: %v", err)
	}

	// The write happened while the lock was held, so the reset's removal must have run
	// after it and taken it with it.
	if _, err := os.Stat(customizedReportsDir); !os.IsNotExist(err) {
		t.Errorf("reset must remove customized reports written by an in-flight rebuild (err=%v)", err)
	}
}

func TestSyncReportVariantsConvergesFromInconsistentState(t *testing.T) {
	setupResultsFixture(t)

	// A customized directory with no selection to justify it — what a crash between
	// saving an empty selection and deleting the directory would leave behind.
	if err := os.MkdirAll(filepath.Dir(customizedVariant.globalPDFPath()), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(customizedVariant.globalPDFPath(), []byte("orphaned report"), 0644); err != nil {
		t.Fatal(err)
	}

	regenerateMu.Lock()
	err := syncReportVariantsLocked()
	regenerateMu.Unlock()
	if err != nil {
		t.Fatalf("syncReportVariantsLocked: %v", err)
	}

	if _, err := os.Stat(customizedReportsDir); !os.IsNotExist(err) {
		t.Errorf("an orphaned customized directory should be removed (err=%v)", err)
	}
	// ...while the full-scan reports are brought up to date.
	if info, err := os.Stat(fullScanVariant.globalPDFPath()); err != nil || info.Size() == 0 {
		t.Errorf("full-scan report should have been generated (err=%v)", err)
	}
}

func TestSyncReportVariantsKeepsCustomizedWhenSelectionApplies(t *testing.T) {
	setupResultsFixture(t)

	if _, err := applyDeselection([]string{keyDrop}); err != nil {
		t.Fatalf(msgApplyDeselection, err)
	}

	regenerateMu.Lock()
	err := syncReportVariantsLocked()
	regenerateMu.Unlock()
	if err != nil {
		t.Fatalf("syncReportVariantsLocked: %v", err)
	}

	for _, path := range []string{
		fullScanVariant.globalPDFPath(),
		customizedVariant.globalPDFPath(),
		customizedVariant.summaryCSVPath(),
	} {
		if info, err := os.Stat(path); err != nil || info.Size() == 0 {
			t.Errorf("expected non-empty %s (err=%v)", path, err)
		}
	}
}

func TestNewScanClearsCustomizedReports(t *testing.T) {
	setupResultsFixture(t)

	if _, err := applyDeselection([]string{keyDrop}); err != nil {
		t.Fatalf(msgApplyDeselection, err)
	}
	if rec := serveReport(t, reportGlobalCustomized); rec.Code != http.StatusOK {
		t.Fatalf("customized request failed: %d", rec.Code)
	}

	// What an analysis run does at the end of a scan. Reports describing the previous
	// scan's selection must not survive it.
	if err := utils.ClearDeselectedRepos(resultsBaseDir); err != nil {
		t.Fatalf("ClearDeselectedRepos: %v", err)
	}
	if _, err := os.Stat(customizedReportsDir); !os.IsNotExist(err) {
		t.Errorf("a new scan should remove %s (err=%v)", customizedReportsDir, err)
	}
}

func TestGlobalPDFListsTopRepositories(t *testing.T) {
	setupResultsFixture(t)

	if rec := serveReport(t, reportGlobal); rec.Code != http.StatusOK {
		t.Fatalf("request failed: %d", rec.Code)
	}
	text := pdfText(t, fullScanVariant.globalPDFPath())

	if !strings.Contains(text, "Repositories by Lines of Code") {
		t.Fatal("global report should list the largest repositories")
	}
	for _, want := range []string{repoKeep, repoDrop, "MAIN LANGUAGE", "SHARE %"} {
		if !strings.Contains(text, want) {
			t.Errorf("top-repositories table missing %q", want)
		}
	}
	// The section header states how many are shown, and both fixture repos qualify.
	if !strings.Contains(text, "Top 2 Repositories") {
		t.Error("heading should state how many repositories are listed")
	}
	// Primary languages come from the per-repository files.
	if !strings.Contains(text, "Go") || !strings.Contains(text, "Java") {
		t.Error("each listed repository should show its main language")
	}
}

func TestGlobalPDFTopRepositoriesRespectSelection(t *testing.T) {
	setupResultsFixture(t)

	if _, err := applyDeselection([]string{keyDrop}); err != nil {
		t.Fatalf(msgApplyDeselection, err)
	}
	if rec := serveReport(t, reportGlobalCustomized); rec.Code != http.StatusOK {
		t.Fatalf("customized request failed: %d", rec.Code)
	}

	custom := pdfText(t, customizedVariant.globalPDFPath())
	// A report that excludes a repository from its totals must not then rank it among
	// the largest — that would contradict the very figures on its first page.
	if !strings.Contains(custom, "Top 1 Repositories") {
		t.Error("customized report should rank only the repositories it counts")
	}
	if strings.Contains(custom, "Java") {
		t.Error("the deselected repository's language should not appear in the ranking")
	}

	// The full scan still ranks both.
	if rec := serveReport(t, reportGlobal); rec.Code != http.StatusOK {
		t.Fatalf("full-scan request failed: %d", rec.Code)
	}
	if full := pdfText(t, fullScanVariant.globalPDFPath()); !strings.Contains(full, "Top 2 Repositories") {
		t.Error("the full-scan report should still rank every repository")
	}
}

func TestSummaryPDFAndCSVCarryLanguages(t *testing.T) {
	setupResultsFixture(t)

	if rec := serveReport(t, reportSummaryPDF); rec.Code != http.StatusOK {
		t.Fatalf("pdf request failed: %d", rec.Code)
	}
	pdf := pdfText(t, fullScanVariant.summaryPDFPath())
	if !strings.Contains(pdf, "Main Language") {
		t.Error("summary PDF should have a Main Language column")
	}
	if !strings.Contains(pdf, "Go") {
		t.Error("summary PDF should show each repository's main language")
	}

	if rec := serveReport(t, reportSummaryCSV); rec.Code != http.StatusOK {
		t.Fatalf("csv request failed: %d", rec.Code)
	}
	csv, err := os.ReadFile(fullScanVariant.summaryCSVPath())
	if err != nil {
		t.Fatal(err)
	}
	// The CSV carries all three languages in fixed columns so a spreadsheet can pivot
	// on them, where the PDF has room for only the primary one.
	for _, want := range []string{"Language 1", "Language 3 Code Lines", ",Go,1000"} {
		if !strings.Contains(string(csv), want) {
			t.Errorf("summary CSV missing %q", want)
		}
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

	if _, err := applyDeselection([]string{keyDrop}); err != nil {
		t.Fatalf(msgApplyDeselection, err)
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
		Repositories:        []RepositoryData{{Number: 1, Key: "acme__keep__main", Repository: repoKeep, Branch: branchMain}},
		Deselected:          []RepositoryData{{Number: 1, Key: "acme__drop__main", Repository: repoDrop, Branch: branchMain}},
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
	out := renderTemplate(t, tablePageData("github", []RepositoryData{
		{Key: "acme__keep__main", Repository: repoKeep, Branch: branchMain},
	}))
	if strings.Contains(out, "-customized.pdf") {
		t.Error("customized report links should not be offered when nothing is deselected")
	}
	if !strings.Contains(out, "/reports/global-report.pdf") {
		t.Error("the full-scan report must always be offered")
	}
}

func TestRepositoryTableShowsTopLanguages(t *testing.T) {
	setupResultsFixture(t)

	pd, err := loadApplicationData()
	if err != nil {
		t.Fatalf("loadApplicationData: %v", err)
	}

	var keep *RepositoryData
	for i := range pd.Repositories {
		if pd.Repositories[i].Repository == repoKeep {
			keep = &pd.Repositories[i]
		}
	}
	if keep == nil {
		t.Fatal("fixture repository 'keep' missing")
	}
	if len(keep.TopLanguages) != 1 || keep.TopLanguages[0].Language != "Go" {
		t.Errorf("TopLanguages = %+v, want one entry for Go", keep.TopLanguages)
	}
	if keep.PrimaryLanguage() != "Go" {
		t.Errorf("PrimaryLanguage() = %q, want Go", keep.PrimaryLanguage())
	}

	out := renderTemplate(t, pd)
	if !strings.Contains(out, "Top Languages") {
		t.Error("repository table should have a Top Languages column")
	}
	if !strings.Contains(out, `data-column="language"`) {
		t.Error("the Top Languages column should be sortable")
	}
	if !strings.Contains(out, `data-language="Go"`) {
		t.Error("rows should carry the primary language as a sort key")
	}
	if !strings.Contains(out, "Go") || !strings.Contains(out, keep.TopLanguages[0].CodeLinesF) {
		t.Error("the cell should show the language and its code lines")
	}
}

func TestDeselectedRowKeepsItsPosition(t *testing.T) {
	// A deselected repository must stay where it ranks. Moving it to the bottom loses the
	// size ordering the table is read for, and makes the row hard to find again to undo.
	all := []RepositoryData{
		{Key: "acme__big__main", Repository: "big", Branch: branchMain, CodeLines: 900},
		{Key: "acme__mid__main", Repository: "mid", Branch: branchMain, CodeLines: 500},
		{Key: "acme__small__main", Repository: "small", Branch: branchMain, CodeLines: 100},
	}
	// Deselect the middle one — the position most likely to be disturbed.
	out := renderTemplate(t, tablePageData("github", all, "acme__mid__main"))

	body := out[strings.Index(out, `<tbody id="repositoryTableBody">`):]
	body = body[:strings.Index(body, "</tbody>")]

	order := regexp.MustCompile(`data-key="([^"]*)"`).FindAllStringSubmatch(body, -1)
	got := make([]string, 0, len(order))
	for _, m := range order {
		got = append(got, m[1])
	}
	want := []string{"acme__big__main", "acme__mid__main", "acme__small__main"}
	if len(got) != len(want) {
		t.Fatalf("rendered %d rows, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("row %d = %q, want %q (deselected row must not move)", i, got[i], want[i])
		}
	}

	// Numbering skips the deselected row rather than renumbering around it, which is what
	// the em dash in its row column stands for.
	nums := regexp.MustCompile(`<td class="row-num">([^<]*)</td>`).FindAllStringSubmatch(body, -1)
	gotNums := make([]string, 0, len(nums))
	for _, m := range nums {
		gotNums = append(gotNums, m[1])
	}
	wantNums := []string{"1", "&mdash;", "2"}
	for i := range wantNums {
		if i >= len(gotNums) || gotNums[i] != wantNums[i] {
			t.Errorf("row numbers = %v, want %v", gotNums, wantNums)
			break
		}
	}

	// And it is still the deselected one, muted and re-selectable.
	if !strings.Contains(body, "deselected-row") || strings.Count(body, "checked") != 2 {
		t.Errorf("expected one unchecked deselected row and two checked rows; body=%q", body)
	}
}

func TestRepositoryTableShowsDashWhenLanguagesUnknown(t *testing.T) {
	// A repository whose by-language result file is missing must read as unknown rather
	// than as an empty cell that looks like a rendering bug.
	out := renderTemplate(t, tablePageData("github", []RepositoryData{
		{Key: "acme__nolang__main", Repository: "nolang", Branch: branchMain},
	}))
	if !strings.Contains(out, `class="top-languages"><span class="text-muted">&mdash;</span>`) {
		t.Error("a repository with no language data should render an em dash")
	}
}

func TestRepositoryTableColumnCountsLineUp(t *testing.T) {
	// The totals row spans the table with a colspan, so adding a column without
	// adjusting it would silently misalign every figure in the footer.
	out := renderTemplate(t, tablePageData("github", []RepositoryData{
		{Key: "acme__keep__main", Repository: repoKeep, Branch: branchMain},
	}))

	section := out[strings.Index(out, `<tbody id="repositoryTableBody">`):]
	header := out[strings.Index(out, `<thead class="table-dark">`):strings.Index(out, `<tbody id="repositoryTableBody">`)]

	headerCells := strings.Count(header, "<th ")
	footer := section[strings.Index(section, `<tr id="totalsRow">`):]
	footer = footer[:strings.Index(footer, "</tr>")]

	footerCells := strings.Count(footer, "<td")
	spanned := 0
	for _, m := range regexp.MustCompile(`colspan="(\d+)"`).FindAllStringSubmatch(footer, -1) {
		n := 0
		fmt.Sscanf(m[1], "%d", &n)
		spanned += n - 1 // the cell itself is already counted
	}

	if headerCells != footerCells+spanned {
		t.Errorf("footer spans %d columns but the header has %d", footerCells+spanned, headerCells)
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
	pd := tablePageData("github", []RepositoryData{
		{Key: "acme__keep__main", Repository: repoKeep, Branch: branchMain, CodeLines: 1000, CodeLinesF: "1.00K"},
		{Key: "acme__drop__main", Repository: repoDrop, Branch: branchMain, CodeLines: 250, CodeLinesF: "250"},
	}, "acme__drop__main")
	pd.DeselectedCodeLines = "250"
	pd.RawTotalLinesOfCode = rawTotalLOC
	out := renderTemplate(t, pd)

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
			{Number: 1, Key: "acme__keep__main", Repository: repoKeep, Branch: branchMain},
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
		{Key: keyDrop, Repo: repoDrop},
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
