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

func TestRepoLanguageRefusesToExcludeEverything(t *testing.T) {
	setupLanguageFixture(t)

	if rec := postRepoLanguage(t, RepoLanguageRequest{Key: keyKeep, Language: "Go", Counted: counted(false)}); rec.Code != http.StatusOK {
		t.Fatalf("excluding Go: status = %d (body: %s)", rec.Code, rec.Body.String())
	}
	// Kubernetes is keep's last counted language: excluding it too is a deselection.
	rec := postRepoLanguage(t, RepoLanguageRequest{Key: keyKeep, Language: "Kubernetes", Counted: counted(false)})
	if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), "deselect the repository instead") {
		t.Errorf("status = %d, body %q; want 422 pointing at deselection", rec.Code, rec.Body.String())
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
