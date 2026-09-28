//go:build resultsall
// +build resultsall

package main

import (
	"bytes"
	"encoding/json"
	"html/template"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/SonarSource-Demos/sonar-golc/pkg/utils"
)

// These tests build on the language fixture: repoKeep has Go 1000, YAML 300, JSON 200
// and Kubernetes 100; repoDrop has Java 250. By default JSON and YAML are excluded for
// every repository, so keep counts 1100 and the scan 1350.

var keyKeep = utils.DeselectionKey(orgAcme, repoKeep, branchMain)

const msgApplyRepoLanguage = "applyRepoLanguageChange: %v"

func counted(b bool) *bool { return &b }

func postRepoLanguage(t *testing.T, req RepoLanguageRequest) *httptest.ResponseRecorder {
	t.Helper()
	body, _ := json.Marshal(req)
	rec := httptest.NewRecorder()
	handleRepoLanguages(rec, httptest.NewRequest(http.MethodPost, "/api/repo-languages", bytes.NewReader(body)))
	return rec
}

func TestRepoLanguageExclusionRecountsTheRepositoryAndTotals(t *testing.T) {
	setupLanguageFixture(t)

	resp, err := applyRepoLanguageChange(RepoLanguageRequest{Key: keyKeep, Language: "Kubernetes", Counted: counted(false)})
	if err != nil {
		t.Fatalf(msgApplyRepoLanguage, err)
	}
	if resp.CodeLines != 1000 || strings.Join(resp.ExcludedLanguages, ",") != "Kubernetes" {
		t.Errorf("response = %+v, want keep at 1000 excluding Kubernetes", resp)
	}
	if want := utils.FormatCodeLines(1250); resp.TotalLinesOfCode != want {
		t.Errorf("TotalLinesOfCode = %q, want %q", resp.TotalLinesOfCode, want)
	}

	pd := snapshot()
	if got := repoRow(t, pd, repoKeep).CodeLines; got != 1000 {
		t.Errorf("keep row = %d, want 1000", got)
	}
	k8s := languageRow(t, pd, "Kubernetes")
	if k8s.CountedLines != 0 || k8s.ExcludedInRepos != 1 || !k8s.PartlyExcluded() || k8s.Excluded {
		t.Errorf("Kubernetes row = %+v, want excluded by one repository, not globally", k8s)
	}
	if go_ := languageRow(t, pd, "Go"); go_.Percentage != 80 {
		t.Errorf("Go share = %.1f%%, want 80%% of the 1250 counted", go_.Percentage)
	}
	if pd.ReposWithOwnExclusions != 1 || !pd.SelectionActive {
		t.Errorf("ReposWithOwnExclusions = %d, SelectionActive = %v", pd.ReposWithOwnExclusions, pd.SelectionActive)
	}
	// The global set is untouched, so the Languages card offers no reset of it.
	if !pd.ExcludedLanguagesIsDefault {
		t.Error("a per-repository exclusion should not make the global set non-default")
	}
}

func TestRepoLanguageSwitchBackOnAndResets(t *testing.T) {
	setupLanguageFixture(t)

	for _, change := range []RepoLanguageRequest{
		{Key: keyKeep, Language: "Kubernetes", Counted: counted(false)},
		{Key: keyKeep, Language: "Kubernetes", Counted: counted(true)},
	} {
		if _, err := applyRepoLanguageChange(change); err != nil {
			t.Fatalf(msgApplyRepoLanguage, err)
		}
	}
	if utils.LoadLanguageExclusion(resultsBaseDir).HasRepoExclusions() {
		t.Error("switching the language back on should leave keep with no exclusions of its own")
	}

	if _, err := applyRepoLanguageChange(RepoLanguageRequest{Key: keyKeep, Language: "Kubernetes", Counted: counted(false)}); err != nil {
		t.Fatalf(msgApplyRepoLanguage, err)
	}
	if _, err := applyRepoLanguageChange(RepoLanguageRequest{Key: keyKeep, Reset: true}); err != nil {
		t.Fatalf(msgApplyRepoLanguage, err)
	}
	if utils.LoadLanguageExclusion(resultsBaseDir).HasRepoExclusions() {
		t.Error("Reset should clear the repository's own exclusions")
	}

	if _, err := applyRepoLanguageChange(RepoLanguageRequest{Key: keyKeep, Language: "Kubernetes", Counted: counted(false)}); err != nil {
		t.Fatalf(msgApplyRepoLanguage, err)
	}
	if _, err := applyRepoLanguageChange(RepoLanguageRequest{ResetAll: true}); err != nil {
		t.Fatalf(msgApplyRepoLanguage, err)
	}
	if pd := snapshot(); pd.ReposWithOwnExclusions != 0 || pd.SelectionActive {
		t.Errorf("after ResetAll: %d repositories with exclusions, selection active %v", pd.ReposWithOwnExclusions, pd.SelectionActive)
	}
}

func TestHandleRepoLanguagesRejectsBadRequests(t *testing.T) {
	setupLanguageFixture(t)

	cases := []struct {
		name string
		req  RepoLanguageRequest
		want int
	}{
		{"missing Counted", RepoLanguageRequest{Key: keyKeep, Language: "Go"}, http.StatusBadRequest},
		{"missing Key", RepoLanguageRequest{Language: "Go", Counted: counted(false)}, http.StatusBadRequest},
		{"unknown repository", RepoLanguageRequest{Key: "acme__nope__main", Language: "Go", Counted: counted(false)}, http.StatusNotFound},
		{"language the repository lacks", RepoLanguageRequest{Key: keyKeep, Language: "Java", Counted: counted(false)}, http.StatusNotFound},
		// A repository can only exclude more than the global set, never less.
		{"globally excluded language", RepoLanguageRequest{Key: keyKeep, Language: "YAML", Counted: counted(true)}, http.StatusUnprocessableEntity},
	}
	for _, c := range cases {
		if rec := postRepoLanguage(t, c.req); rec.Code != c.want {
			t.Errorf("%s: status = %d, want %d (body: %s)", c.name, rec.Code, c.want, rec.Body.String())
		}
	}

	rec := httptest.NewRecorder()
	handleRepoLanguages(rec, httptest.NewRequest(http.MethodGet, "/api/repo-languages", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET: status = %d, want 405", rec.Code)
	}
	rec = httptest.NewRecorder()
	handleRepoLanguages(rec, httptest.NewRequest(http.MethodPost, "/api/repo-languages", strings.NewReader("{not json")))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("malformed body: status = %d, want 400", rec.Code)
	}
}

func TestExcludingTheLastLanguageAsksToDeselect(t *testing.T) {
	setupLanguageFixture(t)

	if rec := postRepoLanguage(t, RepoLanguageRequest{Key: keyKeep, Language: "Go", Counted: counted(false)}); rec.Code != http.StatusOK {
		t.Fatalf("excluding Go: status = %d (body: %s)", rec.Code, rec.Body.String())
	}

	// Kubernetes is keep's last counted language: without Deselect the server asks first.
	rec := postRepoLanguage(t, RepoLanguageRequest{Key: keyKeep, Language: "Kubernetes", Counted: counted(false)})
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409 (body: %s)", rec.Code, rec.Body.String())
	}
	var conflict LastLanguageConflict
	if err := json.Unmarshal(rec.Body.Bytes(), &conflict); err != nil {
		t.Fatalf("409 body is not JSON: %v", err)
	}
	if conflict.Key != keyKeep || conflict.Repository != repoKeep || conflict.Language != "Kubernetes" ||
		!strings.Contains(conflict.Error, "deselect the repository instead") {
		t.Errorf("conflict = %+v", conflict)
	}
	if utils.LoadDeselectionSet(resultsBaseDir).Contains(keyKeep) {
		t.Fatal("an unconfirmed request must not deselect the repository")
	}

	// Confirmed: the repository is deselected and nothing else changes - its row shows
	// every language off anyway, and its checkbox brings it back as it was.
	rec = postRepoLanguage(t, RepoLanguageRequest{Key: keyKeep, Language: "Kubernetes", Counted: counted(false), Deselect: true})
	if rec.Code != http.StatusOK {
		t.Fatalf("confirmed: status = %d (body: %s)", rec.Code, rec.Body.String())
	}
	var resp RepoLanguageResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if !resp.Deselected || strings.Join(resp.ExcludedLanguages, ",") != "Go" {
		t.Errorf("response = %+v, want deselected with only Go excluded of its own", resp)
	}
	if !utils.LoadDeselectionSet(resultsBaseDir).Contains(keyKeep) {
		t.Error("the repository should now be deselected")
	}
	if want := utils.FormatCodeLines(250); resp.TotalLinesOfCode != want {
		t.Errorf("TotalLinesOfCode = %q, want %q (drop alone)", resp.TotalLinesOfCode, want)
	}

	// Its detail page says so.
	detail, err := getRepositoryDetailData(repoKeep, branchMain)
	if err != nil {
		t.Fatalf("getRepositoryDetailData: %v", err)
	}
	var buf bytes.Buffer
	if err := parseRepositoryTemplate(t).Execute(&buf, detail); err != nil {
		t.Fatalf("template: %v", err)
	}
	if !detail.Deselected || !strings.Contains(buf.String(), "keep is deselected") {
		t.Error("the detail page of a deselected repository should say it is deselected")
	}
}

func TestLastLanguageOfTheLastRepositoryCannotDeselectIt(t *testing.T) {
	setupLanguageFixture(t)
	if _, err := applyDeselection([]string{keyKeep}); err != nil {
		t.Fatalf(msgApplyDeselection, err)
	}
	// Java is drop's only language and drop the only counted repository: deselecting it
	// would leave nothing counted, which the repository checkboxes refuse too.
	rec := postRepoLanguage(t, RepoLanguageRequest{Key: keyDrop, Language: "Java", Counted: counted(false), Deselect: true})
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want 422 (body: %s)", rec.Code, rec.Body.String())
	}
	if utils.LoadDeselectionSet(resultsBaseDir).Contains(keyDrop) {
		t.Error("the last counted repository must stay counted")
	}
}

func TestLanguageSwitchesKeepTheirConfirmFlow(t *testing.T) {
	setupLanguageFixture(t)
	out := renderTemplate(t, snapshot())
	// A checkbox applies at once, so nothing can be left unapplied for a switch's reload
	// to discard - the guard that protected against that is gone with the Apply button.
	if strings.Contains(out, "hasUnappliedSelection") {
		t.Error("the unapplied-selection guard should be gone")
	}
	for _, want := range []string{"res.status === 409", "Deselect: true"} {
		if !strings.Contains(out, want) {
			t.Errorf("page script missing %q", want)
		}
	}
}

func TestGlobalToggleKeepsRepoExclusions(t *testing.T) {
	setupLanguageFixture(t)

	if _, err := applyRepoLanguageChange(RepoLanguageRequest{Key: keyKeep, Language: "Kubernetes", Counted: counted(false)}); err != nil {
		t.Fatalf(msgApplyRepoLanguage, err)
	}
	if _, err := applyLanguageExclusion([]string{"JSON"}); err != nil {
		t.Fatalf(msgApplyLanguages, err)
	}
	if got := utils.LoadLanguageExclusion(resultsBaseDir).RepoExclusions()[keyKeep]; strings.Join(got, ",") != "Kubernetes" {
		t.Errorf("keep's own exclusions after a global change = %v, want [Kubernetes]", got)
	}
	// keep: Go 1000 + YAML 300 now counted, Kubernetes still its own exclusion.
	if got := repoRow(t, snapshot(), repoKeep).CodeLines; got != 1300 {
		t.Errorf("keep row = %d, want 1300", got)
	}
}

func TestRepoLanguageExclusionProducesSelectionReports(t *testing.T) {
	setupLanguageFixture(t)

	if rec := serveReport(t, reportGlobal); rec.Code != http.StatusOK {
		t.Fatalf("full scan request failed: %d", rec.Code)
	}
	before, _ := os.ReadFile(fullScanVariant.globalPDFPath())

	if _, err := applyRepoLanguageChange(RepoLanguageRequest{Key: keyKeep, Language: "Kubernetes", Counted: counted(false)}); err != nil {
		t.Fatalf(msgApplyRepoLanguage, err)
	}

	if rec := serveReport(t, reportGlobal); rec.Code != http.StatusOK {
		t.Fatalf("full scan request failed: %d", rec.Code)
	}
	after, _ := os.ReadFile(fullScanVariant.globalPDFPath())
	if !bytes.Equal(before, after) {
		t.Error("a per-repository exclusion must leave the full-scan report alone")
	}

	if rec := serveReport(t, reportGlobalCustomized); rec.Code != http.StatusOK {
		t.Fatalf("selection request failed: %d", rec.Code)
	}
	text := pdfText(t, customizedVariant.globalPDFPath())
	for _, want := range []string{utils.FormatCodeLines(1250), "Per-repository Language Exclusions (1)", repoKeep, "Kubernetes",
		"1 repository also excludes languages of its own."} {
		if !strings.Contains(text, want) {
			t.Errorf("selection PDF missing %q", want)
		}
	}

	if rec := serveReport(t, "repository-summary-customized.csv"); rec.Code != http.StatusOK {
		t.Fatalf("selection CSV request failed: %d", rec.Code)
	}
	csv, _ := os.ReadFile(customizedVariant.summaryCSVPath())
	if !strings.Contains(string(csv), ",1000,Go,1000,") || strings.Contains(string(csv), "Kubernetes") {
		t.Errorf("selection CSV should count keep at 1000 with Go its only language:\n%s", csv)
	}

	out := renderTemplate(t, snapshot())
	if !strings.Contains(out, "Current selection &mdash; 1 repository with its own language exclusions") {
		t.Error("the reports menu should describe the per-repository exclusion")
	}
}

func TestRepositoryRowRendersLanguageChips(t *testing.T) {
	setupLanguageFixture(t)
	if _, err := applyRepoLanguageChange(RepoLanguageRequest{Key: keyKeep, Language: "Kubernetes", Counted: counted(false)}); err != nil {
		t.Fatalf(msgApplyRepoLanguage, err)
	}
	out := renderTemplate(t, snapshot())

	for _, want := range []string{
		`data-key="` + keyKeep + `" value="Go" aria-label="Count Go in keep" checked>`,
		`data-key="` + keyKeep + `" value="Kubernetes" aria-label="Count Kubernetes in keep">`,
		`repo-lang-chip excluded`,
		`id="repoPager"`,
		`id="btnResetRepoLanguages"`,
		`excl. in 1`,
		`1 repository excludes languages of its own`,
		// The row order is saved just before the reload a change triggers, and restored
		// once, so a repository whose Code Lines just dropped does not move down the table.
		`if (keepOrder) state.order = repositoryRows().map(row => row.dataset.key);`,
		`restoreRowOrder(savedTable.order)`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("rendered page missing %q", want)
		}
	}
	// A globally excluded language still shows in the row, switched off and locked: it is
	// switched on the Languages card, not per repository.
	if !strings.Contains(out, `data-key="`+keyKeep+`" value="YAML" aria-label="Count YAML in keep" disabled data-locked>`) {
		t.Error("a globally excluded language should show as a locked, switched-off chip")
	}
	if !strings.Contains(out, "Excluded for all repositories — switch it on in the Languages card") {
		t.Error("a locked chip should say where it is switched")
	}
}

func TestRepositoryDetailOffersLanguageSwitches(t *testing.T) {
	setupLanguageFixture(t)
	if _, err := applyRepoLanguageChange(RepoLanguageRequest{Key: keyKeep, Language: "Kubernetes", Counted: counted(false)}); err != nil {
		t.Fatalf(msgApplyRepoLanguage, err)
	}

	detail, err := getRepositoryDetailData(repoKeep, branchMain)
	if err != nil {
		t.Fatalf("getRepositoryDetailData: %v", err)
	}
	if detail.RepoKey != keyKeep || detail.TotalCodeLines != 1000 {
		t.Errorf("detail = key %q, %d code lines; want %q and 1000", detail.RepoKey, detail.TotalCodeLines, keyKeep)
	}

	var buf bytes.Buffer
	if err := parseRepositoryTemplate(t).Execute(&buf, detail); err != nil {
		t.Fatalf("template: %v", err)
	}
	out := buf.String()
	for _, want := range []string{
		`value="Kubernetes"` + "\n" + `                                     aria-label="Count Kubernetes in keep">`,
		`value="YAML"` + "\n" + `                                     aria-label="Count YAML in keep" disabled>`,
		"excluded for all repositories",
		"excluded here",
		`id="detailLangStatus"`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("detail page missing %q", want)
		}
	}
}

// parseRepositoryTemplate parses the detail page exactly as the /repository/ handler does.
func parseRepositoryTemplate(t *testing.T) *template.Template {
	t.Helper()
	return template.Must(template.New("repository").Parse(repositoryDetailTemplate))
}

// deselectKeepByItsLastLanguage leaves keep deselected with both its counted languages
// switched off, as the confirm flow does.
func deselectKeepByItsLastLanguage(t *testing.T) {
	t.Helper()
	for _, req := range []RepoLanguageRequest{
		{Key: keyKeep, Language: "Go", Counted: counted(false)},
		{Key: keyKeep, Language: "Kubernetes", Counted: counted(false), Deselect: true},
	} {
		if _, err := applyRepoLanguageChange(req); err != nil {
			t.Fatalf(msgApplyRepoLanguage, err)
		}
	}
	if !utils.LoadDeselectionSet(resultsBaseDir).Contains(keyKeep) {
		t.Fatal("setup: keep should be deselected")
	}
}

func TestDeselectedRepositoryChipsShowEveryLanguageOff(t *testing.T) {
	setupLanguageFixture(t)
	deselectKeepByItsLastLanguage(t)

	out := renderTemplate(t, snapshot())
	for _, lang := range []string{"Go", "Kubernetes"} {
		if !strings.Contains(out, `value="`+lang+`" aria-label="Count `+lang+` in keep">`) {
			t.Errorf("%s chip should render switched off", lang)
		}
		if strings.Contains(out, `value="`+lang+`" aria-label="Count `+lang+` in keep" checked>`) {
			t.Errorf("%s chip should not render switched on", lang)
		}
	}
	if !strings.Contains(out, "Switch on to count Kubernetes and select keep again") {
		t.Error("an off chip of a deselected repository should say it selects the repository again")
	}
}

func TestSwitchingALanguageOnSelectsTheRepositoryAgain(t *testing.T) {
	setupLanguageFixture(t)
	deselectKeepByItsLastLanguage(t)

	resp, err := applyRepoLanguageChange(RepoLanguageRequest{Key: keyKeep, Language: "Kubernetes", Counted: counted(true)})
	if err != nil {
		t.Fatalf(msgApplyRepoLanguage, err)
	}
	if !resp.Reselected || resp.Deselected {
		t.Errorf("response = %+v, want Reselected", resp)
	}
	if utils.LoadDeselectionSet(resultsBaseDir).Contains(keyKeep) {
		t.Error("switching a language on should select the repository again")
	}
	// keep counts Kubernetes alone; Go stays switched off.
	if got := repoRow(t, snapshot(), repoKeep).CodeLines; got != 100 {
		t.Errorf("keep row = %d, want 100 (Kubernetes only)", got)
	}
	if want := utils.FormatCodeLines(350); resp.TotalLinesOfCode != want {
		t.Errorf("TotalLinesOfCode = %q, want %q", resp.TotalLinesOfCode, want)
	}
}

func TestCheckboxBringsARepositoryBackAsItWas(t *testing.T) {
	setupLanguageFixture(t)
	deselectKeepByItsLastLanguage(t) // Go off of its own, then Kubernetes off deselected it

	if _, err := applyDeselection(nil); err != nil {
		t.Fatalf(msgApplyDeselection, err)
	}
	if got := utils.LoadLanguageExclusion(resultsBaseDir).RepoExclusions()[keyKeep]; strings.Join(got, ",") != "Go" {
		t.Errorf("keep's own exclusions = %v, want [Go] as before it was deselected", got)
	}
	if got := repoRow(t, snapshot(), repoKeep).CodeLines; got != 100 {
		t.Errorf("keep row = %d, want 100 (Kubernetes counted again)", got)
	}
}

func TestResetReturnsEverySelectionToTheFullScan(t *testing.T) {
	setupLanguageFixture(t)
	deselectKeepByItsLastLanguage(t)
	if _, err := applyLanguageExclusion([]string{"JSON"}); err != nil { // YAML counted globally
		t.Fatalf(msgApplyLanguages, err)
	}

	rec := httptest.NewRecorder()
	handleResetSelection(rec, httptest.NewRequest(http.MethodPost, "/api/reset-selection", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d (body: %s)", rec.Code, rec.Body.String())
	}

	if len(utils.LoadDeselectedRepos(resultsBaseDir)) != 0 {
		t.Error("reset should select every repository")
	}
	if ex := utils.LoadLanguageExclusion(resultsBaseDir); !ex.IsDefault() {
		t.Errorf("reset should leave the default languages and no repository exclusions, got %v / %v",
			ex.Languages(), ex.RepoExclusions())
	}
	pd := snapshot()
	if pd.GlobalReport.TotalLinesOfCode != defaultLanguagesLOC || pd.SelectionActive {
		t.Errorf("after reset: total %q, selection active %v; want %q and none",
			pd.GlobalReport.TotalLinesOfCode, pd.SelectionActive, defaultLanguagesLOC)
	}
	if _, err := os.Stat(customizedVariant.globalPDFPath()); !os.IsNotExist(err) {
		t.Errorf("reset should remove the selection reports (err=%v)", err)
	}

	rec = httptest.NewRecorder()
	handleResetSelection(rec, httptest.NewRequest(http.MethodGet, "/api/reset-selection", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET: status = %d, want 405", rec.Code)
	}
}

func TestResetButtonIsOfferedForAnySelection(t *testing.T) {
	setupLanguageFixture(t)
	pd, err := loadApplicationData()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(renderTemplate(t, pd), `id="btnResetSelection" class="btn btn-sm btn-outline-danger" disabled`) {
		t.Error("with nothing to reset the button should be disabled")
	}
	// A language selection alone, no repository deselected, is something to reset.
	if _, err := applyRepoLanguageChange(RepoLanguageRequest{Key: keyKeep, Language: "Kubernetes", Counted: counted(false)}); err != nil {
		t.Fatalf(msgApplyRepoLanguage, err)
	}
	out := renderTemplate(t, snapshot())
	if strings.Contains(out, `id="btnResetSelection" class="btn btn-sm btn-outline-danger" disabled`) {
		t.Error("with a language selection the reset button should be enabled")
	}
	if !strings.Contains(out, "fetch('/api/reset-selection'") {
		t.Error("the reset button should call the full reset")
	}
}

func TestDeselectedRowShowsZeroAndEveryLanguageOff(t *testing.T) {
	setupLanguageFixture(t)
	if _, err := applyDeselection([]string{keyKeep}); err != nil {
		t.Fatalf(msgApplyDeselection, err)
	}
	pd := snapshot()
	var row RepositoryData
	for _, r := range pd.TableRows {
		if r.Key == keyKeep {
			row = r
		}
	}
	if row.CodeLines != 0 || row.CodeLinesF != "0" {
		t.Errorf("deselected row code lines = %d (%q), want 0", row.CodeLines, row.CodeLinesF)
	}
	for _, chip := range row.LanguageChips {
		if !chip.Excluded {
			t.Errorf("%s chip should show off on a deselected row", chip.Language)
		}
	}
	// The repository keeps its real figures everywhere else.
	if pd.DeselectedCodeLines != utils.FormatCodeLines(1100) {
		t.Errorf("DeselectedCodeLines = %q, want keep's real 1.10K", pd.DeselectedCodeLines)
	}
	// Its detail page shows the same.
	detail, err := getRepositoryDetailData(repoKeep, branchMain)
	if err != nil {
		t.Fatal(err)
	}
	if detail.TotalCodeLines != 0 {
		t.Errorf("detail TotalCodeLines = %d, want 0", detail.TotalCodeLines)
	}
	for _, lang := range detail.Languages {
		if !lang.Excluded {
			t.Errorf("detail %s should show off", lang.Language)
		}
	}
}

func TestSwitchingOnInACheckboxDeselectedRepositoryCountsThatLanguageAlone(t *testing.T) {
	setupLanguageFixture(t)
	if _, err := applyDeselection([]string{keyKeep}); err != nil {
		t.Fatalf(msgApplyDeselection, err)
	}
	resp, err := applyRepoLanguageChange(RepoLanguageRequest{Key: keyKeep, Language: "Kubernetes", Counted: counted(true)})
	if err != nil {
		t.Fatalf(msgApplyRepoLanguage, err)
	}
	if !resp.Reselected || strings.Join(resp.ExcludedLanguages, ",") != "Go" {
		t.Errorf("response = %+v, want reselected counting Kubernetes alone", resp)
	}
	if got := repoRow(t, snapshot(), repoKeep).CodeLines; got != 100 {
		t.Errorf("keep row = %d, want 100", got)
	}
}

func TestReselectingKeepsExclusionsThatLeaveSomethingCounted(t *testing.T) {
	setupLanguageFixture(t)
	// Kubernetes off of its own, then deselected by its checkbox: selecting it again must
	// keep the exclusion, since Go still counts.
	if _, err := applyRepoLanguageChange(RepoLanguageRequest{Key: keyKeep, Language: "Kubernetes", Counted: counted(false)}); err != nil {
		t.Fatalf(msgApplyRepoLanguage, err)
	}
	if _, err := applyDeselection([]string{keyKeep}); err != nil {
		t.Fatalf(msgApplyDeselection, err)
	}
	if _, err := applyDeselection(nil); err != nil {
		t.Fatalf(msgApplyDeselection, err)
	}
	if got := utils.LoadLanguageExclusion(resultsBaseDir).RepoExclusions()[keyKeep]; strings.Join(got, ",") != "Kubernetes" {
		t.Errorf("keep's own exclusions = %v, want [Kubernetes] kept", got)
	}
}

func TestLastLanguageOfAnAlreadyDeselectedRepositoryNeedsNoPrompt(t *testing.T) {
	setupLanguageFixture(t)
	if _, err := applyDeselection([]string{keyKeep}); err != nil {
		t.Fatalf(msgApplyDeselection, err)
	}
	if rec := postRepoLanguage(t, RepoLanguageRequest{Key: keyKeep, Language: "Go", Counted: counted(false)}); rec.Code != http.StatusOK {
		t.Fatalf("excluding Go: status = %d", rec.Code)
	}
	// Already out of every total, so switching off its last language just records it.
	rec := postRepoLanguage(t, RepoLanguageRequest{Key: keyKeep, Language: "Kubernetes", Counted: counted(false)})
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 without a prompt (body: %s)", rec.Code, rec.Body.String())
	}
}

func TestLanguagesCardShowsCountedLines(t *testing.T) {
	setupLanguageFixture(t)
	// Kubernetes off in keep, its only repository: 100 scanned, 0 counted.
	if _, err := applyRepoLanguageChange(RepoLanguageRequest{Key: keyKeep, Language: "Kubernetes", Counted: counted(false)}); err != nil {
		t.Fatalf(msgApplyRepoLanguage, err)
	}
	pd := snapshot()
	k8s := languageRow(t, pd, "Kubernetes")
	if k8s.CountedLinesF != "0" || k8s.RelativePct != 0 {
		t.Errorf("Kubernetes = %+v, want 0 counted lines and no bar", k8s)
	}
	if pd.RepoExcludedCodeLines != "100" {
		t.Errorf("RepoExcludedCodeLines = %q, want 100", pd.RepoExcludedCodeLines)
	}
	out := renderTemplate(t, pd)
	for _, want := range []string{
		"0 of 100 scanned lines count: 1 repository excludes Kubernetes of its own",
		"1 repository excludes languages of its own · 100 LOC",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("rendered card missing %q", want)
		}
	}
}

func TestRowOrderIsKeptForOneReloadOnly(t *testing.T) {
	setupLanguageFixture(t)
	// html/template strips script comments, so this reads the rendered script as served.
	out := renderTemplate(t, snapshot())

	// Every reload the page itself triggers keeps the rows, through one function.
	if n := strings.Count(out, "window.location.reload();"); n != 1 {
		t.Errorf("found %d direct reloads, want only the one inside reloadKeepingRows", n)
	}
	if n := strings.Count(out, "reloadKeepingRows();"); n != 4 {
		t.Errorf("reloadKeepingRows is called %d times, want 4 (checkboxes, reset, Languages card, repository switches)", n)
	}
	// Only that function saves the order; the save on every page change leaves it out,
	// so a saved order is used for one load and never freezes the table.
	if n := strings.Count(out, "saveTableState(true);"); n != 1 {
		t.Errorf("saveTableState(true) appears %d times, want once, in reloadKeepingRows", n)
	}
	if !strings.Contains(out, "saveTableState();") {
		t.Error("showPage should still save the page and sort")
	}
	// The header icon follows the sort actually applied.
	if strings.Contains(out, "updateSortingIcons('codelines', 'desc');") ||
		!strings.Contains(out, "updateSortingIcons(currentSort.column, currentSort.direction);") {
		t.Error("the load handler should show the applied sort, not reset it to Code Lines")
	}
}

func TestDeselectedRepositoriesOwnExclusionsAreNotCounted(t *testing.T) {
	setupLanguageFixture(t)
	if _, err := applyRepoLanguageChange(RepoLanguageRequest{Key: keyKeep, Language: "Kubernetes", Counted: counted(false)}); err != nil {
		t.Fatalf(msgApplyRepoLanguage, err)
	}
	if _, err := applyDeselection([]string{keyKeep}); err != nil {
		t.Fatalf(msgApplyDeselection, err)
	}

	pd := snapshot()
	if pd.ReposWithOwnExclusions != 0 {
		t.Errorf("ReposWithOwnExclusions = %d, want 0: keep is deselected", pd.ReposWithOwnExclusions)
	}
	if strings.Contains(pd.NoteLOCExcluded, "also exclude") {
		t.Errorf("note %q counts a deselected repository", pd.NoteLOCExcluded)
	}
	if strings.Contains(pd.SelectionLabel, "own language exclusions") {
		t.Errorf("reports label %q counts a deselected repository", pd.SelectionLabel)
	}

	if rec := serveReport(t, reportGlobalCustomized); rec.Code != http.StatusOK {
		t.Fatalf("selection request failed: %d", rec.Code)
	}
	text := pdfText(t, customizedVariant.globalPDFPath())
	if strings.Contains(text, "also excludes languages of its own") || strings.Contains(text, "Per-repository Language Exclusions") {
		t.Error("the selection PDF should not mention a deselected repository's own exclusions")
	}
	// They are kept, though: selecting keep again brings them back.
	if got := utils.LoadLanguageExclusion(resultsBaseDir).RepoExclusions()[keyKeep]; strings.Join(got, ",") != "Kubernetes" {
		t.Errorf("keep's own exclusions = %v, want [Kubernetes] kept", got)
	}
}

func TestLanguagesWithoutCodeLinesDoNotKeepARepositoryCounted(t *testing.T) {
	setupLanguageFixture(t)
	// keep also has a language whose files hold no code at all.
	data, _ := json.Marshal(map[string]any{"TotalCodeLines": 1600, "Results": []map[string]any{
		{"Language": "Go", "CodeLines": 1000}, {"Language": "YAML", "CodeLines": 300},
		{"Language": "JSON", "CodeLines": 200}, {"Language": "Kubernetes", "CodeLines": 100},
		{"Language": "Text", "CodeLines": 0},
	}})
	if err := os.WriteFile("Results/bylanguage-report/Result_acme__keep__main.json", data, 0644); err != nil {
		t.Fatal(err)
	}

	if _, err := applyRepoLanguageChange(RepoLanguageRequest{Key: keyKeep, Language: "Kubernetes", Counted: counted(false)}); err != nil {
		t.Fatalf(msgApplyRepoLanguage, err)
	}
	// Go is keep's last language with code: Text must not let it pass unasked.
	if rec := postRepoLanguage(t, RepoLanguageRequest{Key: keyKeep, Language: "Go", Counted: counted(false)}); rec.Code != http.StatusConflict {
		t.Errorf("excluding Go: status = %d, want 409 (body: %s)", rec.Code, rec.Body.String())
	}

	// Nor can Text alone bring a deselected keep back.
	if _, err := applyDeselection([]string{keyKeep}); err != nil {
		t.Fatalf(msgApplyDeselection, err)
	}
	rec := postRepoLanguage(t, RepoLanguageRequest{Key: keyKeep, Language: "Text", Counted: counted(true)})
	if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), "no code lines") {
		t.Errorf("switching Text on: status = %d, body %q; want 422", rec.Code, rec.Body.String())
	}
}

func TestDetailSwitchesOfADeselectedRepositorySayTheySelectItAgain(t *testing.T) {
	setupLanguageFixture(t)
	if _, err := applyDeselection([]string{keyKeep}); err != nil {
		t.Fatalf(msgApplyDeselection, err)
	}
	detail, err := getRepositoryDetailData(repoKeep, branchMain)
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := parseRepositoryTemplate(t).Execute(&buf, detail); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "Switch on to count Go and select keep again") {
		t.Error("a deselected repository's detail switch should say it selects the repository again")
	}
}
