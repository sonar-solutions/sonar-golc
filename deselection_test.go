//go:build resultsall
// +build resultsall

package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SonarSource-Demos/sonar-golc/pkg/utils"
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

	// The regenerated artifacts must exist on disk for the download links.
	for _, path := range []string{
		"Results/GlobalReport.pdf",
		"Results/byfile-report/pdf-report/repository_summary.pdf",
		"Results/byfile-report/csv-report/repository_summary.csv",
	} {
		if info, err := os.Stat(path); err != nil || info.Size() == 0 {
			t.Errorf("expected non-empty %s (err=%v)", path, err)
		}
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
