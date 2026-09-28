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
		{"nothing", NewLanguageExclusion(nil), 1600, "Go,YAML,JSON,Kubernetes"},
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
	if len(repoTotals) != 1 || repoTotals[0].CodeLines != 300 || firstCounted(repoTotals[0].TopLanguages) != "YAML" {
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
	// The breakdown as rendered: JSON was never scanned and Azure Pipelines is only in a
	// deselected repository, so four of the six excluded languages have a marked row.
	breakdown := []LanguageData{
		{Language: "Go", CodeLines: 900},
		{Language: "CloudFormation", CodeLines: 40, Excluded: true},
		{Language: "GitHub Actions", CodeLines: 30, Excluded: true},
		{Language: "Kubernetes", CodeLines: 20, Excluded: true},
		{Language: "YAML", CodeLines: 10, Excluded: true},
	}
	// The real footer is wide enough for the full list.
	if got := footerNote(pdf, tr, many, breakdown, 156); got != many.Note() {
		t.Errorf("footerNote = %q, want the full note when it fits", got)
	}
	// Too narrow: a count of the rows actually marked, never a list cut part-way.
	got := footerNote(pdf, tr, many, breakdown, 60)
	if got != "4 languages are excluded from the total - marked (excl.) in the Language Breakdown." {
		t.Errorf("footerNote = %q, want a count of the four marked rows", got)
	}
	if one := footerNote(pdf, tr, many, breakdown[:2], 60); one != "1 language is excluded from the total - marked (excl.) in the Language Breakdown." {
		t.Errorf("footerNote = %q, want the singular form", one)
	}
	if none := footerNote(pdf, tr, many, breakdown[:1], 60); none != "No language in this report is excluded from the total." {
		t.Errorf("footerNote = %q, want the no-row form", none)
	}
	if strings.Contains(got, "...") {
		t.Error("the note must not be truncated")
	}
}

func TestRepoExclusionsScopeToTheirRepository(t *testing.T) {
	e := DefaultLanguageExclusion().WithRepoExclusions(map[string][]string{
		"acme__svc__main":   {"Kubernetes", "YAML"}, // YAML is already global: dropped
		"acme__empty__main": {"JSON"},               // nothing left: the repo is dropped
	})

	if got := e.RepoExclusions(); len(got) != 1 || strings.Join(got["acme__svc__main"], ",") != "Kubernetes" {
		t.Fatalf("RepoExclusions = %v, want only svc excluding Kubernetes", got)
	}
	if e.Excludes("Kubernetes") {
		t.Error("an unscoped selection should not apply a repository's own exclusions")
	}
	svc := e.ForRepo("acme__svc__main")
	if !svc.Excludes("Kubernetes") || !svc.ExcludedHere("Kubernetes") || svc.ExcludesEverywhere("Kubernetes") {
		t.Error("svc should exclude Kubernetes of its own")
	}
	if !svc.Excludes("YAML") || svc.ExcludedHere("YAML") || !svc.ExcludesEverywhere("YAML") {
		t.Error("svc should exclude YAML through the global set, not of its own")
	}
	if e.ForRepo("acme__other__main").Excludes("Kubernetes") {
		t.Error("another repository should not inherit svc's exclusions")
	}

	// A per-repository exclusion is a departure from the full scan, but not from the
	// default global set.
	if e.IsDefault() || !e.GlobalIsDefault() {
		t.Errorf("IsDefault = %v, GlobalIsDefault = %v; want false, true", e.IsDefault(), e.GlobalIsDefault())
	}
	if e.Matches(DefaultExcludedLanguages) {
		t.Error("a GlobalReport.json total cannot describe per-repository exclusions")
	}
	if e.Fingerprint() == DefaultLanguageExclusion().Fingerprint() {
		t.Error("the fingerprint must change with a per-repository exclusion")
	}
	if !strings.HasSuffix(e.Note(), " 1 repository also excludes languages of its own.") {
		t.Errorf("Note = %q, want it to mention the repository", e.Note())
	}
}

func TestSaveLoadAndClearRepoExclusions(t *testing.T) {
	base := t.TempDir()
	e := NewLanguageExclusion([]string{"JSON"}).WithRepoExclusions(map[string][]string{"k": {"Go"}})
	if err := SaveLanguageExclusion(base, e); err != nil {
		t.Fatal(err)
	}
	if err := SaveRepoLanguageExclusions(base, e); err != nil {
		t.Fatal(err)
	}

	got := LoadLanguageExclusion(base)
	if strings.Join(got.Languages(), ",") != "JSON" || strings.Join(got.RepoExclusions()["k"], ",") != "Go" {
		t.Errorf("reloaded global %v, per-repo %v", got.Languages(), got.RepoExclusions())
	}

	if err := ClearLanguageExclusion(base); err != nil {
		t.Fatal(err)
	}
	if !LoadLanguageExclusion(base).IsDefault() {
		t.Error("a new scan should clear per-repository exclusions along with the global set")
	}
}

func TestRankLanguageChipsKeepsARepositorysOwnExclusions(t *testing.T) {
	shares := []LanguageShare{
		{Language: "JSON", CodeLines: 9000}, // global: shown, off and locked
		{Language: "Go", CodeLines: 700},
		{Language: "Kubernetes", CodeLines: 100},
		{Language: "Shell", CodeLines: 50},
	}
	scoped := DefaultLanguageExclusion().WithRepoExclusions(map[string][]string{"k": {"Kubernetes"}}).ForRepo("k")

	chips := RankLanguageChips(shares, 5, scoped)
	var got []string
	for _, c := range chips {
		name := c.Language
		switch {
		case c.Everywhere:
			name += "(locked)"
		case c.Excluded:
			name += "(off)"
		}
		got = append(got, name)
	}
	if strings.Join(got, ",") != "JSON(locked),Go,Kubernetes(off),Shell" {
		t.Errorf("chips = %v, want JSON(locked),Go,Kubernetes(off),Shell", got)
	}
	// The report ranking leaves the repository's own exclusion out altogether.
	top := RankTopLanguages(shares, 5, scoped)
	if len(top) != 2 || top[0].Language != "Go" || top[1].Language != "Shell" {
		t.Errorf("RankTopLanguages = %+v, want Go and Shell", top)
	}
}

func TestReadRepositoryDataAppliesRepoExclusions(t *testing.T) {
	base := t.TempDir()
	branch := ProjectBranch{Org: "acme", RepoSlug: "svc", MainBranch: testBranchMain}
	writeRepoFixture(t, base, "github", branch, 1600, []LanguageShare{
		{Language: "Go", CodeLines: 1000},
		{Language: "YAML", CodeLines: 300},
		{Language: "JSON", CodeLines: 200},
		{Language: "Kubernetes", CodeLines: 100},
	})
	key := DeselectionKey("acme", "svc", testBranchMain)
	excluded := DefaultLanguageExclusion().WithRepoExclusions(map[string][]string{key: {"Kubernetes"}})

	repos, err := ReadRepositoryDataWith(base, excluded)
	if err != nil {
		t.Fatalf("ReadRepositoryDataWith: %v", err)
	}
	repo := repos[0]
	if repo.CodeLines != 1000 {
		t.Errorf("CodeLines = %d, want 1000 (Go only: JSON and YAML global, Kubernetes its own)", repo.CodeLines)
	}
	if len(repo.TopLanguages) != 1 || repo.TopLanguages[0].Language != "Go" {
		t.Errorf("TopLanguages = %+v, want Go alone", repo.TopLanguages)
	}
	// Every language the repository has, by size: the global ones locked, its own off.
	var chips []string
	for _, c := range repo.LanguageChips {
		state := "on"
		if c.Everywhere {
			state = "locked"
		} else if c.Excluded {
			state = "off"
		}
		chips = append(chips, c.Language+":"+state)
	}
	if strings.Join(chips, ",") != "Go:on,YAML:locked,JSON:locked,Kubernetes:off" {
		t.Errorf("LanguageChips = %v", chips)
	}
	if strings.Join(repo.Languages, ",") != "Go,JSON,Kubernetes,YAML" {
		t.Errorf("Languages = %v", repo.Languages)
	}
}

func TestCollectResultTotalsCountsEachRepositoryUnderItsOwnExclusions(t *testing.T) {
	base := t.TempDir()
	for _, repo := range []string{"a", "b"} {
		writeRepoFixture(t, base, "github", ProjectBranch{Org: "acme", RepoSlug: repo, MainBranch: testBranchMain}, 150,
			[]LanguageShare{{Language: "Go", CodeLines: 100}, {Language: "Shell", CodeLines: 50}})
	}
	excluded := DefaultLanguageExclusion().WithRepoExclusions(map[string][]string{
		DeselectionKey("acme", "a", testBranchMain): {"Shell"},
	})

	totals, repoTotals, err := CollectResultTotals(base, nil, excluded)
	if err != nil {
		t.Fatalf("CollectResultTotals: %v", err)
	}
	if totals["Shell"] != 100 {
		t.Errorf("totals[Shell] = %d, want every line listed", totals["Shell"])
	}
	if counted := CountedByLanguage(repoTotals); counted["Shell"] != 50 || counted["Go"] != 200 {
		t.Errorf("CountedByLanguage = %v, want Shell counted in b only", counted)
	}
	if repoTotalsSum(repoTotals) != 250 {
		t.Errorf("sum = %d, want 250", repoTotalsSum(repoTotals))
	}
}

func TestRepoNoteNamesTheRepositorysOwnExclusions(t *testing.T) {
	e := DefaultLanguageExclusion().WithRepoExclusions(map[string][]string{"k": {"Python", "Shell"}})
	want := "JSON and YAML are excluded from the total to reproduce standard SonarQube behavior. This repository also excludes Python and Shell."
	if got := e.ForRepo("k").RepoNote(); got != want {
		t.Errorf("RepoNote = %q, want %q", got, want)
	}
	if got := e.ForRepo("other").RepoNote(); got != DefaultLanguageExclusion().Note() {
		t.Errorf("a repository without exclusions of its own got %q", got)
	}
}
