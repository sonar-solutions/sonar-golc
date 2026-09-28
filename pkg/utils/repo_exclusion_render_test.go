package utils

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRenderRepoLanguageExclusionsSectionListsCountedRepositories(t *testing.T) {
	pdf := newSectionPDF(t)
	tr := pdf.UnicodeTranslatorFromDescriptor("")
	excluded := DefaultLanguageExclusion().WithRepoExclusions(map[string][]string{
		"acme__svc__main":   {"Python", "Shell"},
		"acme__other__main": {"Go"}, // not among the counted repositories below
	})
	repoTotals := []RepoTotal{
		{Key: "acme__svc__main", Repo: "svc", Branch: testBranchMain},
		{Key: "acme__plain__main", Repo: "plain", Branch: testBranchMain},
	}
	renderRepoLanguageExclusionsSection(pdf, tr, excluded, repoTotals, 15, 180)

	text := pdfSectionText(t, pdf)
	for _, want := range []string{"Per-repository Language Exclusions (1)", "svc", testBranchMain, "Python and Shell", "EXCLUDED LANGUAGES"} {
		if !strings.Contains(text, want) {
			t.Errorf("section missing %q", want)
		}
	}
	if strings.Contains(text, "other") {
		t.Error("a repository that is not counted should not be listed")
	}
}

func TestRenderRepoLanguageExclusionsSectionRendersNothingWithoutExclusions(t *testing.T) {
	pdf := newSectionPDF(t)
	tr := pdf.UnicodeTranslatorFromDescriptor("")
	before := pdf.GetY()
	renderRepoLanguageExclusionsSection(pdf, tr, DefaultLanguageExclusion(), []RepoTotal{{Key: "k", Repo: "r"}}, 15, 180)
	if pdf.GetY() != before {
		t.Error("with no repository excluding anything of its own the section should draw nothing")
	}
}

func TestRenderRepoLanguageExclusionsSectionBreaksPages(t *testing.T) {
	pdf := newSectionPDF(t)
	tr := pdf.UnicodeTranslatorFromDescriptor("")
	byRepo := map[string][]string{}
	var repoTotals []RepoTotal
	for i := 0; i < 20; i++ {
		key := fmt.Sprintf("acme__repo-%02d__main", i)
		byRepo[key] = []string{"Go"}
		repoTotals = append(repoTotals, RepoTotal{Key: key, Repo: fmt.Sprintf("repo-%02d", i), Branch: testBranchMain})
	}
	pdf.SetY(250)
	renderRepoLanguageExclusionsSection(pdf, tr, DefaultLanguageExclusion().WithRepoExclusions(byRepo), repoTotals, 15, 180)

	if pages := pdf.PageNo(); pages < 2 {
		t.Fatalf("a list starting near the page bottom should break, got %d page(s)", pages)
	}
	if text := pdfSectionText(t, pdf); strings.Count(text, "EXCLUDED LANGUAGES") < 2 {
		t.Error("column headers should repeat after a page break")
	}
}

func TestPartlyExcludedLanguagesAreCountedAndLabelled(t *testing.T) {
	languages := withCountedLines(
		[]LanguageData{{Language: "Go", CodeLines: 1000}, {Language: "YAML", CodeLines: 300}},
		[]RepoTotal{{CountedLanguages: map[string]int{"Go": 700}}, {CountedLanguages: map[string]int{"Go": 0}}},
	)
	goLang := languages[0]
	if goLang.CountedLines != 700 || !goLang.PartlyExcluded() {
		t.Errorf("Go = %+v, want 700 counted and partly excluded", goLang)
	}
	// Without a per-repository recount a language counts in full, or not at all when the
	// global set excludes it.
	if n := (LanguageData{Language: "YAML", CodeLines: 300}).counted(DefaultLanguageExclusion()); n != 0 {
		t.Errorf("YAML counted = %d, want 0 under the defaults", n)
	}

	prepared, maxLOC := prepareLanguagesForPDF(languages, DefaultLanguageExclusion())
	if prepared[0].Percentage != 100 || !prepared[1].Excluded {
		t.Errorf("prepared = %+v, want Go at 100%% of the counted lines and YAML excluded", prepared)
	}

	pdf := newSectionPDF(t)
	renderLanguageRow(pdf, prepared[0], 0, maxLOC, [][3]int{{0, 0, 0}}, 15, 10, 60, 30, 20, 60, 7)
	if text := pdfSectionText(t, pdf); !strings.Contains(text, "Go (partly excl.)") {
		t.Errorf("a partly excluded language should be labelled, got %q", text)
	}
}

func TestAccumulateLanguageTotalsFromFileReportsUnreadableFiles(t *testing.T) {
	dir := t.TempDir()
	if _, _, _, err := accumulateLanguageTotalsFromFile(filepath.Join(dir, "missing.json"), map[string]int{}, map[string]int{}, DefaultLanguageExclusion()); err == nil {
		t.Error("a missing file should be an error")
	}
	bad := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(bad, []byte("{not json"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := accumulateLanguageTotalsFromFile(bad, map[string]int{}, map[string]int{}, DefaultLanguageExclusion()); err == nil {
		t.Error("an unreadable file should be an error")
	}
}

func TestWithoutReposDropsOnlyTheGivenRepositories(t *testing.T) {
	e := DefaultLanguageExclusion().WithRepoExclusions(map[string][]string{"a": {"Go"}, "b": {"Shell"}})
	got := e.WithoutRepos(DeselectionSet{"a": true}).RepoExclusions()
	if len(got) != 1 || strings.Join(got["b"], ",") != "Shell" {
		t.Errorf("WithoutRepos = %v, want only b", got)
	}
	if n := len(e.RepoExclusions()); n != 2 {
		t.Errorf("the original selection should be untouched, has %d repositories", n)
	}
}

func TestNotesCountSeveralRepositories(t *testing.T) {
	e := DefaultLanguageExclusion().WithRepoExclusions(map[string][]string{"a": {"Go"}, "b": {"Shell"}})
	if !strings.HasSuffix(e.Note(), " 2 repositories also exclude languages of their own.") {
		t.Errorf("Note = %q", e.Note())
	}

	// A repository's own exclusion that the global set has since taken over leaves
	// nothing of its own to name.
	own := NewLanguageExclusion(nil).WithRepoExclusions(map[string][]string{"k": {"YAML"}})
	own.set = nil // the global set is now the defaults, which exclude YAML
	if got := own.ForRepo("k").RepoNote(); got != DefaultLanguageExclusion().Note() {
		t.Errorf("RepoNote = %q, want the global note alone", got)
	}
}

func TestRepoExclusionPersistenceErrors(t *testing.T) {
	// A base "directory" that is a file: nothing can be created under it.
	base := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(base, nil, 0644); err != nil {
		t.Fatal(err)
	}
	e := DefaultLanguageExclusion().WithRepoExclusions(map[string][]string{"k": {"Go"}})
	if err := SaveRepoLanguageExclusions(base, e); err == nil {
		t.Error("saving under a file should fail")
	}
	if err := ClearLanguageExclusion(base); err == nil {
		t.Error("clearing under a file should fail")
	}

	// An unreadable per-repository file means no repository excludes anything.
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "config"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(RepoExcludedLanguagesPath(dir), []byte("{not json"), 0644); err != nil {
		t.Fatal(err)
	}
	if LoadLanguageExclusion(dir).HasRepoExclusions() {
		t.Error("a corrupt per-repository file should load as no exclusions")
	}
}

func TestRepositoryLanguageHelpers(t *testing.T) {
	r := RepositoryData{LanguageLines: map[string]int{"Go": 10, "Text": 0}}
	if !r.CountsCode("Go") || r.CountsCode("Text") || r.CountsCode("Java") {
		t.Error("CountsCode should be true only for a language with code lines")
	}
	chips := RankLanguageChips([]LanguageShare{{Language: " ", CodeLines: 5}, {Language: "Go", CodeLines: 3}}, 5, DefaultLanguageExclusion())
	if len(chips) != 1 || chips[0].Language != "Go" {
		t.Errorf("chips = %+v, want Go alone - a blank name is skipped", chips)
	}
}
