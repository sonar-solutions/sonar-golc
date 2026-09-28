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

	// Confirmed: the repository is deselected, and the exclusion is recorded too, so its
	// switches show every language off rather than one still on.
	rec = postRepoLanguage(t, RepoLanguageRequest{Key: keyKeep, Language: "Kubernetes", Counted: counted(false), Deselect: true})
	if rec.Code != http.StatusOK {
		t.Fatalf("confirmed: status = %d (body: %s)", rec.Code, rec.Body.String())
	}
	var resp RepoLanguageResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if !resp.Deselected || strings.Join(resp.ExcludedLanguages, ",") != "Go,Kubernetes" {
		t.Errorf("response = %+v, want deselected with Go and Kubernetes excluded of its own", resp)
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

func TestLanguageSwitchesGuardUnappliedSelection(t *testing.T) {
	setupLanguageFixture(t)
	out := renderTemplate(t, snapshot())
	// Switching reloads the page; unapplied repository checkboxes must not be lost to it.
	for _, want := range []string{
		"function hasUnappliedSelection()",
		"if (hasUnappliedSelection()) {",
		"Apply or reset your repository selection first",
		"res.status === 409",
		"Deselect: true",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("page script missing %q", want)
		}
	}
	if n := strings.Count(out, "if (hasUnappliedSelection()) {"); n != 2 {
		t.Errorf("guard appears %d times, want 2 (Languages card and repository chips)", n)
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
		// The row order is saved and restored across the reload a switch triggers, so a
		// repository whose Code Lines just dropped does not move down the table.
		`order: repositoryRows().map(row => row.dataset.key)`,
		`restoreRowOrder(savedTable.order)`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("rendered page missing %q", want)
		}
	}
	// Globally excluded languages are switched on the Languages card, not per repository.
	if strings.Contains(out, `data-key="`+keyKeep+`" value="YAML"`) {
		t.Error("a globally excluded language should not get a repository chip")
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

func TestSelectingAnEmptyRepositoryAgainCountsItInFull(t *testing.T) {
	for _, reselect := range []struct {
		name string
		keys []string
	}{
		{"its checkbox", []string{}},
		{"reset to full scan", nil},
	} {
		t.Run(reselect.name, func(t *testing.T) {
			setupLanguageFixture(t)
			deselectKeepByItsLastLanguage(t)

			if _, err := applyDeselection(reselect.keys); err != nil {
				t.Fatalf(msgApplyDeselection, err)
			}
			if got := utils.LoadLanguageExclusion(resultsBaseDir).RepoExclusions()[keyKeep]; len(got) != 0 {
				t.Errorf("keep's own exclusions = %v, want cleared so it does not come back at zero", got)
			}
			if got := repoRow(t, snapshot(), repoKeep).CodeLines; got != 1100 {
				t.Errorf("keep row = %d, want 1100 (counted in full under the global set)", got)
			}
		})
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
