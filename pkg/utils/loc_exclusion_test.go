package utils

import (
	"os"
	"strings"
	"testing"

	"github.com/jung-kurt/gofpdf"
)

func TestZeroLanguageExclusionIsTheDefault(t *testing.T) {
	var e LanguageExclusion
	for _, lang := range []string{"JSON", "YAML", " YAML "} {
		if !e.Excludes(lang) {
			t.Errorf("the zero value should exclude %q", lang)
		}
	}
	// IaC dialects of YAML and JSON are counted by SonarQube by default.
	for _, lang := range []string{"Kubernetes", "CloudFormation", "GitHub Actions", "Go"} {
		if e.Excludes(lang) {
			t.Errorf("the zero value should count %q", lang)
		}
	}
	if !e.IsDefault() {
		t.Error("the zero value should report itself as the default")
	}
}

func TestExplicitLanguageExclusions(t *testing.T) {
	none := NewLanguageExclusion([]string{})
	if none.Excludes("JSON") || none.IsDefault() || len(none.Languages()) != 0 {
		t.Errorf("an empty selection should exclude nothing: %v", none.Languages())
	}

	// Built in any order, the default set is still recognised as the default.
	if !NewLanguageExclusion([]string{"YAML", "JSON"}).IsDefault() {
		t.Error("JSON and YAML in any order should be the default selection")
	}

	custom := NewLanguageExclusion([]string{" XML ", "", "JSON"})
	if got := strings.Join(custom.Languages(), ","); got != "JSON,XML" {
		t.Errorf("Languages() = %q, want JSON,XML (trimmed, sorted, blanks dropped)", got)
	}
	if custom.Excludes("YAML") {
		t.Error("a custom selection should not fall back to the defaults")
	}
}

func TestLanguageExclusionNote(t *testing.T) {
	cases := []struct {
		e    LanguageExclusion
		want string
	}{
		{DefaultLanguageExclusion(), "JSON and YAML are excluded from the total to reproduce standard SonarQube behavior."},
		{NewLanguageExclusion(nil), "All languages are counted in the total."},
		{NewLanguageExclusion([]string{"JSON"}), "Excluded from the total: JSON."},
		{NewLanguageExclusion([]string{"XML", "JSON", "YAML"}), "Excluded from the total: JSON, XML and YAML."},
	}
	for _, c := range cases {
		if got := c.e.Note(); got != c.want {
			t.Errorf("Note() = %q, want %q", got, c.want)
		}
	}
}

func TestSaveLoadAndClearLanguageExclusion(t *testing.T) {
	base := t.TempDir()

	// Nothing saved: the defaults.
	if !LoadLanguageExclusion(base).IsDefault() {
		t.Error("a missing file should load as the defaults")
	}

	// An empty selection must survive a reload rather than read back as the defaults.
	if err := SaveLanguageExclusion(base, NewLanguageExclusion(nil)); err != nil {
		t.Fatalf("SaveLanguageExclusion: %v", err)
	}
	if got := LoadLanguageExclusion(base); got.IsDefault() || len(got.Languages()) != 0 {
		t.Errorf("empty selection reloaded as %v", got.Languages())
	}

	if err := SaveLanguageExclusion(base, NewLanguageExclusion([]string{"JSON"})); err != nil {
		t.Fatalf("SaveLanguageExclusion: %v", err)
	}
	if got := strings.Join(LoadLanguageExclusion(base).Languages(), ","); got != "JSON" {
		t.Errorf("reloaded %q, want JSON", got)
	}

	if err := ClearLanguageExclusion(base); err != nil {
		t.Fatalf("ClearLanguageExclusion: %v", err)
	}
	if !LoadLanguageExclusion(base).IsDefault() {
		t.Error("a cleared selection should load as the defaults")
	}
	if err := ClearLanguageExclusion(base); err != nil {
		t.Errorf("clearing twice should be a no-op, got %v", err)
	}
}

func TestLoadLanguageExclusionFallsBackOnCorruptFile(t *testing.T) {
	base := t.TempDir()
	if err := os.MkdirAll(base+"/config", 0755); err != nil {
		t.Fatal(err)
	}
	for _, body := range []string{"{not json", `{"Other": 1}`} {
		if err := os.WriteFile(ExcludedLanguagesPath(base), []byte(body), 0644); err != nil {
			t.Fatal(err)
		}
		if !LoadLanguageExclusion(base).IsDefault() {
			t.Errorf("%q should load as the defaults", body)
		}
	}
}

func TestReadRepositoryDataSubtractsEveryExcludedLanguage(t *testing.T) {
	base := t.TempDir()
	branch := ProjectBranch{Org: "acme", RepoSlug: "svc", MainBranch: testBranchMain}
	writeRepoFixture(t, base, "github", branch, 1600, []LanguageShare{
		{Language: "Go", CodeLines: 1000},
		{Language: "YAML", CodeLines: 300},
		{Language: "JSON", CodeLines: 200},
		{Language: "Kubernetes", CodeLines: 100},
	})

	cases := []struct {
		name     string
		excluded LanguageExclusion
		want     int
		top      string
	}{
		{"defaults", DefaultLanguageExclusion(), 1100, "Go,Kubernetes"},
		{"JSON only", NewLanguageExclusion([]string{"JSON"}), 1400, "Go,YAML,Kubernetes"},
		{"nothing", NewLanguageExclusion(nil), 1600, "Go,YAML,JSON"},
	}
	for _, c := range cases {
		repos, err := ReadRepositoryDataWith(base, c.excluded)
		if err != nil {
			t.Fatalf("%s: ReadRepositoryDataWith: %v", c.name, err)
		}
		if repos[0].CodeLines != c.want {
			t.Errorf("%s: CodeLines = %d, want %d", c.name, repos[0].CodeLines, c.want)
		}
		var top []string
		for _, l := range repos[0].TopLanguages {
			top = append(top, l.Language)
		}
		if got := strings.Join(top, ","); got != c.top {
			t.Errorf("%s: TopLanguages = %s, want %s", c.name, got, c.top)
		}
	}

	// ReadRepositoryData applies the selection persisted under the same base.
	if err := SaveLanguageExclusion(base, NewLanguageExclusion(nil)); err != nil {
		t.Fatal(err)
	}
	repos, err := ReadRepositoryData(base)
	if err != nil {
		t.Fatalf("ReadRepositoryData: %v", err)
	}
	if repos[0].CodeLines != 1600 {
		t.Errorf("ReadRepositoryData ignored the persisted selection: CodeLines = %d", repos[0].CodeLines)
	}
}

func TestAdjustGlobalInfoRecountsForLanguageSelection(t *testing.T) {
	in := Globalinfo{TotalLinesOfCode: "1.10K", LargestRepository: "svc", LinesOfCodeLargestRepo: "1.10K", NumberRepos: 1}
	languages := []LanguageData{{Language: "Go", CodeLines: 1000}, {Language: "YAML", CodeLines: 300}}
	repoTotals := []RepoTotal{{Repo: "svc", CodeLines: 1300}}

	// Nothing deselected, but YAML now counts: the headline must be recomputed even so.
	got := AdjustGlobalInfo(in, languages, repoTotals, 0, NewLanguageExclusion(nil))
	if got.TotalLinesOfCode != FormatCodeLines(1300) || got.LinesOfCodeLargestRepo != FormatCodeLines(1300) {
		t.Errorf("got %+v, want 1.30K for both totals", got)
	}
	if got.NumberRepos != 1 {
		t.Errorf("NumberRepos = %d, want 1 (a language selection removes no repository)", got.NumberRepos)
	}
}

func TestCollectResultTotalsCountsRepositoriesUnderTheSelection(t *testing.T) {
	base := t.TempDir()
	branch := ProjectBranch{Org: "acme", RepoSlug: "svc", MainBranch: testBranchMain}
	writeRepoFixture(t, base, "github", branch, 1300, []LanguageShare{
		{Language: "Go", CodeLines: 1000},
		{Language: "YAML", CodeLines: 300},
	})

	totals, repoTotals, err := CollectResultTotals(base, nil, NewLanguageExclusion([]string{"Go"}))
	if err != nil {
		t.Fatalf("CollectResultTotals: %v", err)
	}
	// Per-language totals always list every language, so an excluded one can be shown.
	if totals["Go"] != 1000 || totals["YAML"] != 300 {
		t.Errorf("totals = %v, want every language listed", totals)
	}
	if len(repoTotals) != 1 || repoTotals[0].CodeLines != 300 || repoTotals[0].PrimaryLanguage != "YAML" {
		t.Errorf("repoTotals = %+v, want 300 with YAML primary once Go is excluded", repoTotals)
	}
}

func TestLanguageExclusionMatchesRecordedSelection(t *testing.T) {
	def := DefaultLanguageExclusion()
	if def.Matches(nil) {
		t.Error("a file that does not record its selection must never match - it may predate the YAML default")
	}
	if !def.Matches([]string{"YAML", "JSON"}) {
		t.Error("the recorded defaults, in any order, should match the default selection")
	}
	if def.Matches([]string{"JSON"}) || NewLanguageExclusion(nil).Matches([]string{"JSON", "YAML"}) {
		t.Error("different selections must not match")
	}
	if !NewLanguageExclusion(nil).Matches([]string{}) {
		t.Error("a recorded empty selection should match counting everything")
	}
}

func TestAdjustGlobalInfoRecountsLegacyGlobalReport(t *testing.T) {
	// Written by a scanner that still counted plain YAML: 1000 Go + 300 YAML.
	legacy := Globalinfo{TotalLinesOfCode: "1.30K", LargestRepository: "svc", LinesOfCodeLargestRepo: "1.30K", NumberRepos: 1}
	languages := []LanguageData{{Language: "Go", CodeLines: 1000}, {Language: "YAML", CodeLines: 300}}
	repoTotals := []RepoTotal{{Repo: "svc", CodeLines: 1000}}

	got := AdjustGlobalInfo(legacy, languages, repoTotals, 0, DefaultLanguageExclusion())
	if got.TotalLinesOfCode != FormatCodeLines(1000) || got.LinesOfCodeLargestRepo != FormatCodeLines(1000) {
		t.Errorf("got %+v, want the headline recounted to 1.00K like the rows", got)
	}
	if strings.Join(got.ExcludedLanguages, ",") != "JSON,YAML" {
		t.Errorf("ExcludedLanguages = %v, want the selection it was recounted under", got.ExcludedLanguages)
	}
}

func TestFooterNoteKeepsTheLanguageListWhole(t *testing.T) {
	pdf := gofpdf.New("P", "mm", "A4", "")
	pdf.AddPage()
	pdf.SetFont("Helvetica", "I", 7)
	tr := pdf.UnicodeTranslatorFromDescriptor("")

	many := NewLanguageExclusion([]string{"Azure Pipelines", "CloudFormation", "GitHub Actions", "JSON", "Kubernetes", "YAML"})
	// The real footer is wide enough for the full list.
	if got := footerNote(pdf, tr, many, 156); got != many.Note() {
		t.Errorf("footerNote = %q, want the full note when it fits", got)
	}
	// Too narrow: a count, never a list cut part-way.
	got := footerNote(pdf, tr, many, 60)
	if got != "6 languages are excluded from the total - marked (excl.) in the Language Breakdown." {
		t.Errorf("footerNote = %q, want the count fallback", got)
	}
	if strings.Contains(got, "...") {
		t.Error("the note must not be truncated")
	}
}
