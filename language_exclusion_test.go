//go:build resultsall
// +build resultsall

package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/SonarSource-Demos/sonar-golc/pkg/utils"
)

// The language fixture extends the deselection one: repoKeep also carries plain YAML,
// JSON and a Kubernetes manifest. Its 1600 code lines split as Go 1000, YAML 300, JSON 200
// and Kubernetes 100; repoDrop is still Java 250.
//
// By default JSON and YAML are held out, so the scan totals 1000 + 100 + 250 = 1350 -
// the figure the scanner writes to GlobalReport.json.
var (
	defaultLanguagesLOC = utils.FormatCodeLines(1350)
	yamlCountedLOC      = utils.FormatCodeLines(1650) // JSON alone excluded
	everythingLOC       = utils.FormatCodeLines(1850) // nothing excluded
)

const msgApplyLanguages = "applyLanguageExclusion: %v"

func setupLanguageFixture(t *testing.T) {
	t.Helper()
	setupResultsFixture(t)

	writeJSON := func(path string, v any) {
		data, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, data, 0644); err != nil {
			t.Fatal(err)
		}
	}

	writeJSON("Results/byfile-report/Result_acme__keep__main_byfile.json", map[string]int{
		"TotalLines": 1900, "TotalBlankLines": 150, "TotalComments": 150, "TotalCodeLines": 1600,
	})
	writeJSON("Results/bylanguage-report/Result_acme__keep__main.json", map[string]any{
		"TotalCodeLines": 1600,
		"Results": []map[string]any{
			{"Language": "Go", "CodeLines": 1000},
			{"Language": "YAML", "CodeLines": 300},
			{"Language": "JSON", "CodeLines": 200},
			{"Language": "Kubernetes", "CodeLines": 100},
		},
	})
	writeJSON("Results/GlobalReport.json", map[string]any{
		"Organization":           orgAcme,
		"TotalLinesOfCode":       defaultLanguagesLOC,
		"LargestRepository":      repoKeep,
		"LinesOfCodeLargestRepo": utils.FormatCodeLines(1100),
		"DevOpsPlatform":         "github",
		"NumberRepos":            2,
	})
}

func languageRow(t *testing.T, pd PageData, name string) LanguageData {
	t.Helper()
	for _, lang := range pd.Languages {
		if lang.Language == name {
			return lang
		}
	}
	t.Fatalf("language %q not listed; got %+v", name, pd.Languages)
	return LanguageData{}
}

func repoRow(t *testing.T, pd PageData, name string) RepositoryData {
	t.Helper()
	for _, repo := range pd.Repositories {
		if repo.Repository == name {
			return repo
		}
	}
	t.Fatalf("repository %q not listed", name)
	return RepositoryData{}
}

func TestPageHoldsOutJSONAndYAMLByDefault(t *testing.T) {
	setupLanguageFixture(t)

	pd, err := loadApplicationData()
	if err != nil {
		t.Fatalf("loadApplicationData: %v", err)
	}

	if pd.GlobalReport.TotalLinesOfCode != defaultLanguagesLOC {
		t.Errorf("TotalLinesOfCode = %q, want %q", pd.GlobalReport.TotalLinesOfCode, defaultLanguagesLOC)
	}
	for _, name := range []string{"JSON", "YAML"} {
		if lang := languageRow(t, pd, name); !lang.Excluded || lang.Percentage != 0 {
			t.Errorf("%s = %+v, want excluded with no share", name, lang)
		}
	}
	// An IaC dialect of YAML is counted by SonarQube by default, so it stays in.
	if lang := languageRow(t, pd, "Kubernetes"); lang.Excluded || lang.Percentage == 0 {
		t.Errorf("Kubernetes = %+v, want counted", lang)
	}
	if got := repoRow(t, pd, repoKeep).CodeLines; got != 1100 {
		t.Errorf("keep CodeLines = %d, want 1100 (Go + Kubernetes)", got)
	}
	if !pd.ExcludedLanguagesIsDefault || strings.Join(pd.ExcludedLanguages, ",") != "JSON,YAML" {
		t.Errorf("ExcludedLanguages = %v (default %v), want the default JSON,YAML",
			pd.ExcludedLanguages, pd.ExcludedLanguagesIsDefault)
	}
	if pd.ExcludedLanguagesCodeLines != utils.FormatCodeLines(500) {
		t.Errorf("ExcludedLanguagesCodeLines = %q, want 500", pd.ExcludedLanguagesCodeLines)
	}
}

func TestApplyLanguageExclusionRecountsEveryTotal(t *testing.T) {
	setupLanguageFixture(t)

	resp, err := applyLanguageExclusion([]string{"JSON"})
	if err != nil {
		t.Fatalf(msgApplyLanguages, err)
	}
	if resp.TotalLinesOfCode != yamlCountedLOC {
		t.Errorf("TotalLinesOfCode = %q, want %q", resp.TotalLinesOfCode, yamlCountedLOC)
	}
	// Nothing is deselected, so the whole-scan figure moves with the headline rather than
	// staying at what GlobalReport.json recorded under the defaults.
	if resp.RawTotalLinesOfCode != yamlCountedLOC {
		t.Errorf("RawTotalLinesOfCode = %q, want %q", resp.RawTotalLinesOfCode, yamlCountedLOC)
	}

	pd := snapshot()
	if got := repoRow(t, pd, repoKeep).CodeLines; got != 1400 {
		t.Errorf("keep CodeLines = %d, want 1400 once YAML counts", got)
	}
	if pd.GlobalReport.LinesOfCodeLargestRepo != utils.FormatCodeLines(1400) {
		t.Errorf("LinesOfCodeLargestRepo = %q, want 1.40K", pd.GlobalReport.LinesOfCodeLargestRepo)
	}
	if languageRow(t, pd, "YAML").Excluded {
		t.Error("YAML should be counted after switching it on")
	}
	if pd.NoteLOCExcluded != "Excluded from the total: JSON." {
		t.Errorf("NoteLOCExcluded = %q", pd.NoteLOCExcluded)
	}

	// The selection is persisted, so a restarted dashboard shows the same totals.
	if got := utils.LoadLanguageExclusion(resultsBaseDir).Languages(); strings.Join(got, ",") != "JSON" {
		t.Errorf("persisted selection = %v, want [JSON]", got)
	}
}

func TestApplyLanguageExclusionCanCountEverything(t *testing.T) {
	setupLanguageFixture(t)

	resp, err := applyLanguageExclusion([]string{})
	if err != nil {
		t.Fatalf(msgApplyLanguages, err)
	}
	if resp.TotalLinesOfCode != everythingLOC {
		t.Errorf("TotalLinesOfCode = %q, want %q", resp.TotalLinesOfCode, everythingLOC)
	}
	// An empty selection must survive a reload rather than read back as the defaults.
	pd, err := loadApplicationData()
	if err != nil {
		t.Fatalf("loadApplicationData: %v", err)
	}
	if pd.GlobalReport.TotalLinesOfCode != everythingLOC || len(pd.ExcludedLanguages) != 0 {
		t.Errorf("after reload: total %q, excluded %v; want %q and none",
			pd.GlobalReport.TotalLinesOfCode, pd.ExcludedLanguages, everythingLOC)
	}
}

func TestApplyLanguageExclusionIgnoresUnknownLanguages(t *testing.T) {
	setupLanguageFixture(t)

	resp, err := applyLanguageExclusion([]string{"JSON", "Cobol", "  "})
	if err != nil {
		t.Fatalf(msgApplyLanguages, err)
	}
	if resp.Ignored != 2 {
		t.Errorf("Ignored = %d, want 2", resp.Ignored)
	}
	if strings.Join(resp.ExcludedLanguages, ",") != "JSON" {
		t.Errorf("ExcludedLanguages = %v, want only the scanned JSON", resp.ExcludedLanguages)
	}
}

func TestApplyLanguageExclusionRefusesToExcludeEverything(t *testing.T) {
	setupLanguageFixture(t)

	_, err := applyLanguageExclusion([]string{"Go", "Java", "JSON", "Kubernetes", "YAML"})
	if err != errAllLanguagesExcluded {
		t.Fatalf("err = %v, want errAllLanguagesExcluded", err)
	}
	// A refused request must leave the previous selection in place.
	if _, statErr := os.Stat(utils.ExcludedLanguagesPath(resultsBaseDir)); !os.IsNotExist(statErr) {
		t.Errorf("a refused selection must not be persisted (err=%v)", statErr)
	}
}

func TestHandleExcludedLanguages(t *testing.T) {
	setupLanguageFixture(t)
	pd, err := loadApplicationData()
	if err != nil {
		t.Fatalf("loadApplicationData: %v", err)
	}
	publish(pd)

	post := func(body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		handleExcludedLanguages(rec, httptest.NewRequest(http.MethodPost, "/api/excluded-languages", strings.NewReader(body)))
		return rec
	}

	if rec := post("{not json"); rec.Code != http.StatusBadRequest {
		t.Errorf("malformed body: status = %d, want 400", rec.Code)
	}
	if rec := post(`{"Languages":["Go","Java","JSON","Kubernetes","YAML"]}`); rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("excluding everything: status = %d, want 422", rec.Code)
	}
	rec := httptest.NewRecorder()
	handleExcludedLanguages(rec, httptest.NewRequest(http.MethodDelete, "/api/excluded-languages", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("wrong method: status = %d, want 405", rec.Code)
	}

	body, _ := json.Marshal(LanguageExclusionRequest{Languages: []string{"JSON"}})
	if rec := post(string(body)); rec.Code != http.StatusOK {
		t.Fatalf("POST status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	handleExcludedLanguages(rec, httptest.NewRequest(http.MethodGet, "/api/excluded-languages", nil))
	var got LanguageExclusionResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("GET body is not valid JSON: %v", err)
	}
	if strings.Join(got.ExcludedLanguages, ",") != "JSON" || got.TotalLinesOfCode != yamlCountedLOC {
		t.Errorf("GET = %+v, want JSON excluded and total %s", got, yamlCountedLOC)
	}
	if strings.Join(got.AvailableLanguages, ",") != "Go,JSON,Java,Kubernetes,YAML" {
		t.Errorf("AvailableLanguages = %v", got.AvailableLanguages)
	}
	if strings.Join(got.DefaultLanguages, ",") != "JSON,YAML" {
		t.Errorf("DefaultLanguages = %v", got.DefaultLanguages)
	}
}

func TestLanguageSelectionCombinesWithDeselection(t *testing.T) {
	setupLanguageFixture(t)

	if _, err := applyDeselection([]string{keyDrop}); err != nil {
		t.Fatalf(msgApplyDeselection, err)
	}
	resp, err := applyLanguageExclusion([]string{"JSON"})
	if err != nil {
		t.Fatalf(msgApplyLanguages, err)
	}
	// keep alone, with YAML now counted; the whole scan adds drop's 250 back.
	if want := utils.FormatCodeLines(1400); resp.TotalLinesOfCode != want {
		t.Errorf("TotalLinesOfCode = %q, want %q", resp.TotalLinesOfCode, want)
	}
	if resp.RawTotalLinesOfCode != yamlCountedLOC {
		t.Errorf("RawTotalLinesOfCode = %q, want %q", resp.RawTotalLinesOfCode, yamlCountedLOC)
	}
}

func TestLanguageSelectionLeavesFullScanReportAlone(t *testing.T) {
	setupLanguageFixture(t)

	if rec := serveReport(t, reportGlobal); rec.Code != http.StatusOK {
		t.Fatalf("first request failed: %d", rec.Code)
	}
	before, err := os.ReadFile(fullScanVariant.globalPDFPath())
	if err != nil {
		t.Fatal(err)
	}

	// The full scan is the original report: every repository, default languages. A
	// language selection must neither alter it nor trigger a rebuild of it.
	if _, err := applyLanguageExclusion([]string{"JSON"}); err != nil {
		t.Fatalf(msgApplyLanguages, err)
	}
	if rec := serveReport(t, reportGlobal); rec.Code != http.StatusOK {
		t.Fatalf("second request failed: %d", rec.Code)
	}
	after, err := os.ReadFile(fullScanVariant.globalPDFPath())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Error("the full-scan report changed after a language selection")
	}
	text := pdfText(t, fullScanVariant.globalPDFPath())
	if !strings.Contains(text, defaultLanguagesLOC) || !strings.Contains(text, "JSON and YAML are excluded") {
		t.Errorf("the full-scan report should keep the default total %s and note", defaultLanguagesLOC)
	}

	if rec := serveReport(t, reportSummaryCSV); rec.Code != http.StatusOK {
		t.Fatalf("summary CSV request failed: %d", rec.Code)
	}
	csv, err := os.ReadFile(fullScanVariant.summaryCSVPath())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(csv), "1100") || strings.Contains(string(csv), "1400") {
		t.Errorf("full-scan CSV should count keep at the default 1100:\n%s", csv)
	}
}

func TestLanguageSelectionProducesCustomizedReports(t *testing.T) {
	setupLanguageFixture(t)

	if _, err := applyLanguageExclusion([]string{"JSON"}); err != nil {
		t.Fatalf(msgApplyLanguages, err)
	}

	rec := serveReport(t, reportGlobalCustomized)
	if rec.Code != http.StatusOK {
		t.Fatalf("customized request failed: %d", rec.Code)
	}
	if cd := rec.Header().Get("Content-Disposition"); !strings.Contains(cd, "selection") {
		t.Errorf("Content-Disposition = %q, want the selection variant", cd)
	}
	text := pdfText(t, customizedVariant.globalPDFPath())
	if !strings.Contains(text, yamlCountedLOC) {
		t.Errorf("the selection report should be recounted to %s", yamlCountedLOC)
	}
	if !strings.Contains(text, "Excluded from the total: JSON.") {
		t.Error("the selection report footer should name the languages it leaves out")
	}

	if rec := serveReport(t, "repository-summary-customized.csv"); rec.Code != http.StatusOK {
		t.Fatalf("customized CSV request failed: %d", rec.Code)
	}
	csv, err := os.ReadFile(customizedVariant.summaryCSVPath())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(csv), "1400") {
		t.Errorf("selection CSV should count keep at 1400 once YAML counts:\n%s", csv)
	}

	// Offered on the page even though no repository is deselected.
	out := renderTemplate(t, snapshot())
	if !strings.Contains(out, "global-report-customized.pdf") ||
		!strings.Contains(out, "Current selection &mdash; JSON excluded") {
		t.Error("the reports menu should offer the selection reports for a language change")
	}
	if !strings.Contains(out, "all 2 repositories, default languages") {
		t.Error("the reports menu should say the full scan uses the default languages")
	}
}

func TestReturningToDefaultLanguagesRemovesCustomizedReports(t *testing.T) {
	setupLanguageFixture(t)

	if _, err := applyLanguageExclusion([]string{"JSON"}); err != nil {
		t.Fatalf(msgApplyLanguages, err)
	}
	if rec := serveReport(t, reportGlobalCustomized); rec.Code != http.StatusOK {
		t.Fatalf("customized request failed: %d", rec.Code)
	}

	// A deselection keeps them alive even once the languages are back to the defaults.
	if _, err := applyDeselection([]string{keyDrop}); err != nil {
		t.Fatalf(msgApplyDeselection, err)
	}
	if _, err := applyLanguageExclusion(utils.DefaultExcludedLanguages); err != nil {
		t.Fatalf(msgApplyLanguages, err)
	}
	if !strings.Contains(renderTemplate(t, snapshot()), "global-report-customized.pdf") {
		t.Error("with a repository still deselected the selection reports must stay on offer")
	}

	// Clearing the deselection leaves nothing to describe: the files go.
	if _, err := applyDeselection(nil); err != nil {
		t.Fatalf(msgApplyDeselection, err)
	}
	if _, err := os.Stat(customizedVariant.globalPDFPath()); !os.IsNotExist(err) {
		t.Errorf("customized reports should be removed once both selections are back to the full scan (err=%v)", err)
	}

	// And the other order: languages last.
	if _, err := applyLanguageExclusion([]string{"JSON"}); err != nil {
		t.Fatalf(msgApplyLanguages, err)
	}
	if rec := serveReport(t, reportGlobalCustomized); rec.Code != http.StatusOK {
		t.Fatalf("customized request failed: %d", rec.Code)
	}
	if _, err := applyLanguageExclusion(utils.DefaultExcludedLanguages); err != nil {
		t.Fatalf(msgApplyLanguages, err)
	}
	if _, err := os.Stat(customizedVariant.globalPDFPath()); !os.IsNotExist(err) {
		t.Errorf("returning to the default languages should remove the customized reports (err=%v)", err)
	}
	if strings.Contains(renderTemplate(t, snapshot()), "global-report-customized.pdf") {
		t.Error("with no selection the menu should offer only the full scan")
	}
}

func TestRepositoryDetailFollowsLanguageSelection(t *testing.T) {
	setupLanguageFixture(t)

	detail, err := getRepositoryDetailData(repoKeep, branchMain)
	if err != nil {
		t.Fatalf("getRepositoryDetailData: %v", err)
	}
	if detail.TotalCodeLines != 1100 {
		t.Errorf("default TotalCodeLines = %d, want 1100", detail.TotalCodeLines)
	}

	if _, err := applyLanguageExclusion(nil); err != nil {
		t.Fatalf(msgApplyLanguages, err)
	}
	detail, err = getRepositoryDetailData(repoKeep, branchMain)
	if err != nil {
		t.Fatalf("getRepositoryDetailData: %v", err)
	}
	if detail.TotalCodeLines != 1600 {
		t.Errorf("TotalCodeLines = %d, want 1600 with nothing excluded", detail.TotalCodeLines)
	}
	for _, lang := range detail.Languages {
		if lang.Excluded {
			t.Errorf("%s still marked excluded", lang.Language)
		}
	}
}

func TestLanguageCardRendersSwitches(t *testing.T) {
	setupLanguageFixture(t)
	pd, err := loadApplicationData()
	if err != nil {
		t.Fatalf("loadApplicationData: %v", err)
	}
	out := renderTemplate(t, pd)

	for _, want := range []string{
		`id="langSelectionSummary"`,
		`class="form-check-input lang-toggle"`,
		`value="YAML" aria-label="Count YAML in the total">`, // excluded: rendered unchecked
		`value="Go" aria-label="Count Go in the total" checked>`,
		`lang-bar-row lang-excluded`,
		`lang-excluded-badge`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("rendered page missing %q", want)
		}
	}
	// On the defaults there is nothing to reset to.
	if strings.Contains(out, `id="btnResetLanguages"`) {
		t.Error("the reset link should only appear once the selection differs from the defaults")
	}
	// Excluded languages are left out of the chart, which shows what the total is made of.
	if chart := out[strings.Index(out, "labels: ["):]; strings.Contains(chart[:strings.Index(chart, "]")], `"YAML"`) {
		t.Error("an excluded language should not appear in the chart")
	}

	if _, err := applyLanguageExclusion([]string{"JSON"}); err != nil {
		t.Fatalf(msgApplyLanguages, err)
	}
	out = renderTemplate(t, snapshot())
	if !strings.Contains(out, `id="btnResetLanguages"`) {
		t.Error("the reset link should appear once the selection differs from the defaults")
	}
}

func TestLanguageRequestBodyIsBounded(t *testing.T) {
	setupLanguageFixture(t)
	huge := bytes.Repeat([]byte("a"), 2<<20)
	rec := httptest.NewRecorder()
	handleExcludedLanguages(rec, httptest.NewRequest(http.MethodPost, "/api/excluded-languages",
		bytes.NewReader(append(append([]byte(`{"Languages":["`), huge...), []byte(`"]}`)...))))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("oversized body: status = %d, want 400", rec.Code)
	}
}

func TestResetKeepsDefaultsTheScanDidNotFind(t *testing.T) {
	// The deselection fixture has only Go and Java - neither JSON nor YAML.
	setupResultsFixture(t)

	// Switching the absent defaults "on" changes nothing and must not count as unknown.
	resp, err := applyLanguageExclusion([]string{})
	if err != nil {
		t.Fatalf(msgApplyLanguages, err)
	}
	if resp.Ignored != 0 {
		t.Errorf("Ignored = %d, want 0", resp.Ignored)
	}
	if !utils.LoadLanguageExclusion(resultsBaseDir).IsDefault() {
		t.Error("defaults the scan never found should stay in the selection")
	}

	// Excluding Java then resetting must land exactly on the defaults, or the page keeps
	// offering a reset that never takes.
	if _, err := applyLanguageExclusion([]string{"Java"}); err != nil {
		t.Fatalf(msgApplyLanguages, err)
	}
	if _, err := applyLanguageExclusion(utils.DefaultExcludedLanguages); err != nil {
		t.Fatalf(msgApplyLanguages, err)
	}
	pd := snapshot()
	if !pd.ExcludedLanguagesIsDefault {
		t.Errorf("after a reset the selection should be the default, got %v", utils.LoadLanguageExclusion(resultsBaseDir).Languages())
	}
	// Nothing the scan found is excluded, so the summary names nothing.
	if len(pd.ExcludedLanguages) != 0 {
		t.Errorf("ExcludedLanguages = %v, want none of this scan's languages", pd.ExcludedLanguages)
	}
	if out := renderTemplate(t, pd); !strings.Contains(out, "Every language is counted in the total.") {
		t.Error("the summary should say every scanned language counts")
	}
}

func TestSelectionLabel(t *testing.T) {
	def := utils.DefaultLanguageExclusion()
	cases := []struct {
		deselected int
		excluded   utils.LanguageExclusion
		present    []string
		want       string
	}{
		{0, def, []string{"YAML"}, ""},
		{1, def, []string{"YAML"}, "1 repository deselected"},
		{3, def, nil, "3 repositories deselected"},
		{0, utils.NewLanguageExclusion([]string{"JSON"}), []string{"JSON"}, "JSON excluded"},
		{2, utils.NewLanguageExclusion(nil), nil, "2 repositories deselected · every language counted"},
	}
	for _, c := range cases {
		if got := selectionLabel(c.deselected, c.excluded, c.present); got != c.want {
			t.Errorf("selectionLabel(%d, %v, %v) = %q, want %q", c.deselected, c.excluded.Languages(), c.present, got, c.want)
		}
	}
}
