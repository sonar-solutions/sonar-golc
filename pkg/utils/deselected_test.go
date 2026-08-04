package utils

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Shared fixture identities for this package's tests, named rather than repeated as
// literals across every case.
const (
	testBranchMain = "main"
	testOrgAcme    = "acme"
	testRepoKeep   = "keep"
	testRepoDrop   = "drop"
	testLangJava   = "Java"
)

func TestDeselectionKeyRoundTripsThroughResultFileName(t *testing.T) {
	// The key must be recoverable from the result file name, because the walk that
	// builds the global report has only the file name to go on while the summary
	// reports build the key from the inventory. If these two ever disagree, a
	// repository would be dropped from one report and kept in the other.
	cases := []struct {
		name           string
		org            string
		repo           string
		branch         string
		resultFileName string
	}{
		{"plain", "my-org", "api-service", testBranchMain, "Result_my-org__api-service__main.json"},
		{"underscores everywhere", "my_group", "my_repo", "feat_x", "Result_my_group__my_repo__feat_x.json"},
		{"empty org", "", "repo", testBranchMain, "Result___repo__main.json"},
		{"byfile variant", "org", "repo", testBranchMain, "Result_org__repo__main_byfile.json"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			want := DeselectionKey(tc.org, tc.repo, tc.branch)
			got, ok := DeselectionKeyFromResultFileName(tc.resultFileName)
			if !ok {
				t.Fatalf("DeselectionKeyFromResultFileName(%q) not recognised", tc.resultFileName)
			}
			if got != want {
				t.Errorf("key mismatch for %q: from file %q, from fields %q", tc.resultFileName, got, want)
			}
		})
	}
}

func TestDeselectionKeyNormalizesPathSeparators(t *testing.T) {
	// A GitLab subgroup ("group/subgroup") and a branch like "release/1.0" both put a
	// path separator inside a key component. The results page sanitizes components
	// before building a file path while the report generators historically did not, so
	// an unnormalized key let the page and the reports disagree about the same
	// repository — the deselection applied on the page but not in the PDF.
	//
	// The key must therefore be separator-free, whatever it is built from.
	cases := []struct{ org, repo, branch string }{
		{"group/subgroup", "svc", testBranchMain},
		{testOrgAcme, "svc", "release/1.0"},
		{"back\\slash", "svc", testBranchMain},
		{"../escape", "svc", testBranchMain},
	}
	for _, tc := range cases {
		key := DeselectionKey(tc.org, tc.repo, tc.branch)
		// No separator means the key can never be read as a nested path, which is also
		// what makes traversal impossible without a special case for "..".
		if strings.ContainsAny(key, `/\`) {
			t.Errorf("DeselectionKey(%q,%q,%q) = %q: must not contain a path separator",
				tc.org, tc.repo, tc.branch, key)
		}
		// Exactly three fields must remain, so the key stays parseable.
		if got := strings.Count(key, keySeparator); got != 2 {
			t.Errorf("DeselectionKey(%q,%q,%q) = %q: want 2 field separators, got %d",
				tc.org, tc.repo, tc.branch, key, got)
		}
	}
}

func TestSanitizeResultComponentMatchesWhatReportersWrite(t *testing.T) {
	// The reporters write result files after replacing "/" with "_" (see
	// strings.Replace(OutputName, "/", "_", -1) in pkg/reporter/{json,csv,pdf}). The
	// readers must apply the same substitution: deleting the separator instead would
	// be equally traversal-safe but would look for a file that does not exist, so a
	// GitLab subgroup or a slashed branch would vanish from the reports.
	const writerRule = "_"
	if got := SanitizeResultComponent("group/subgroup"); got != "group"+writerRule+"subgroup" {
		t.Errorf("SanitizeResultComponent(\"group/subgroup\") = %q, want %q — must match the reporters' substitution",
			got, "group"+writerRule+"subgroup")
	}
	if got := SanitizeResultComponent("release/1.0"); got != "release_1.0" {
		t.Errorf("SanitizeResultComponent(\"release/1.0\") = %q, want %q", got, "release_1.0")
	}
}

func TestDeselectionKeyAgreesWithSanitizedFileName(t *testing.T) {
	// The results page builds its file path from sanitized components; the reports
	// build the key from the raw inventory fields. Both must land on the same key, or
	// a repository deselected on the page stays counted in the generated reports.
	cases := []struct{ org, repo, branch string }{
		{testOrgAcme, "svc", testBranchMain},
		{"group/subgroup", "svc", testBranchMain},
		{testOrgAcme, "svc", "release/1.0"},
		{"my_group", "my_repo", "feat_x"},
	}
	for _, tc := range cases {
		fromFields := DeselectionKey(tc.org, tc.repo, tc.branch)

		// What the page ends up with: components sanitized for the path, then the key
		// recovered from that file name.
		sanitizedName := "Result_" + SanitizeResultComponent(tc.org) +
			"__" + SanitizeResultComponent(tc.repo) +
			"__" + SanitizeResultComponent(tc.branch) + ".json"
		fromFileName, ok := DeselectionKeyFromResultFileName(sanitizedName)
		if !ok {
			t.Errorf("%q not recognised as a result file", sanitizedName)
			continue
		}
		if fromFields != fromFileName {
			t.Errorf("key mismatch for %q/%q/%q: from fields %q, from file name %q",
				tc.org, tc.repo, tc.branch, fromFields, fromFileName)
		}
	}
}

func TestSanitizeResultComponent(t *testing.T) {
	cases := map[string]string{
		"plain":          "plain",
		"group/subgroup": "group_subgroup",
		"back\\slash":    "back_slash",
		"../escape":      ".._escape",
		"keeps_under":    "keeps_under",
		"keeps-dash.1":   "keeps-dash.1",
	}
	for in, want := range cases {
		if got := SanitizeResultComponent(in); got != want {
			t.Errorf("SanitizeResultComponent(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestDeselectionKeyFromResultFileNameFilePlatform(t *testing.T) {
	// The file platform writes single-component names, so the key is the repo alone.
	got, ok := DeselectionKeyFromResultFileName("Result_my-directory.json")
	if !ok {
		t.Fatal("single-component result file should be recognised")
	}
	if want := FileDeselectionKey("my-directory"); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestDeselectionKeyFromResultFileNameRejectsNonResultFiles(t *testing.T) {
	for _, name := range []string{"GlobalReport.json", "code_lines_by_language.json", "Result_.json", ""} {
		if _, ok := DeselectionKeyFromResultFileName(name); ok {
			t.Errorf("DeselectionKeyFromResultFileName(%q) should not be recognised", name)
		}
	}
}

func TestDeselectionSetContainsIsNilSafe(t *testing.T) {
	var set DeselectionSet
	if set.Contains("anything") {
		t.Error("nil set should not contain anything")
	}
}

func TestSaveLoadAndClearDeselectedRepos(t *testing.T) {
	base := t.TempDir()

	// Absent file means nothing deselected, not an error: reports must render on
	// result sets that predate this feature.
	if got := LoadDeselectedRepos(base); got != nil {
		t.Errorf("expected nil for missing file, got %+v", got)
	}
	if set := LoadDeselectionSet(base); len(set) != 0 {
		t.Errorf("expected empty set for missing file, got %+v", set)
	}

	repos := []DeselectedRepo{
		{Key: DeselectionKey("org", "dead-repo", testBranchMain), Org: "org", Repo: "dead-repo", Branch: testBranchMain},
		{Key: DeselectionKey("org", "vendored", "master"), Org: "org", Repo: "vendored", Branch: "master"},
	}
	if err := SaveDeselectedRepos(base, repos); err != nil {
		t.Fatalf("SaveDeselectedRepos: %v", err)
	}

	loaded := LoadDeselectedRepos(base)
	if len(loaded) != 2 {
		t.Fatalf("loaded %d repos, want 2", len(loaded))
	}
	set := LoadDeselectionSet(base)
	for _, r := range repos {
		if !set.Contains(r.Key) {
			t.Errorf("set missing key %q", r.Key)
		}
	}

	// Clearing must restore the "nothing deselected" state, since that is what makes
	// a filtered report reversible.
	if err := ClearDeselectedRepos(base); err != nil {
		t.Fatalf("ClearDeselectedRepos: %v", err)
	}
	if got := LoadDeselectedRepos(base); got != nil {
		t.Errorf("expected nil after clear, got %+v", got)
	}

	// Clearing again is not an error — a fresh scan calls it unconditionally.
	if err := ClearDeselectedRepos(base); err != nil {
		t.Errorf("ClearDeselectedRepos on absent file: %v", err)
	}
}

func TestSaveDeselectedReposWritesEmptyList(t *testing.T) {
	base := t.TempDir()
	if err := SaveDeselectedRepos(base, nil); err != nil {
		t.Fatalf("SaveDeselectedRepos(nil): %v", err)
	}

	data, err := os.ReadFile(DeselectedReposPath(base))
	if err != nil {
		t.Fatalf("reading persisted file: %v", err)
	}
	var rep DeselectedReport
	if err := json.Unmarshal(data, &rep); err != nil {
		t.Fatalf("persisted file is not valid JSON: %v", err)
	}
	if rep.DeselectedRepositories == nil {
		t.Error("empty list should serialize as [], not null")
	}
}

func TestLoadDeselectedReposIgnoresCorruptFile(t *testing.T) {
	base := t.TempDir()
	path := DeselectedReposPath(base)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{not json"), 0644); err != nil {
		t.Fatal(err)
	}
	// A corrupt file must degrade to "nothing deselected" rather than break the page.
	if got := LoadDeselectedRepos(base); got != nil {
		t.Errorf("expected nil for corrupt file, got %+v", got)
	}
}

func TestPartitionDeselected(t *testing.T) {
	repos := []RepositoryData{
		{Number: 1, Key: "org__a__main", Repository: "a", CodeLines: 300},
		{Number: 2, Key: "org__b__main", Repository: "b", CodeLines: 200},
		{Number: 3, Key: "org__c__main", Repository: "c", CodeLines: 100},
	}
	set := DeselectionSet{"org__b__main": true}

	kept, removed := PartitionDeselected(repos, set)

	if len(kept) != 2 || len(removed) != 1 {
		t.Fatalf("kept %d removed %d, want 2 and 1", len(kept), len(removed))
	}
	if removed[0].Repository != "b" {
		t.Errorf("removed %q, want b", removed[0].Repository)
	}
	// Both groups render as standalone tables, so each is numbered from 1.
	if kept[0].Number != 1 || kept[1].Number != 2 {
		t.Errorf("kept not renumbered from 1: %d, %d", kept[0].Number, kept[1].Number)
	}
	if removed[0].Number != 1 {
		t.Errorf("removed not renumbered from 1: %d", removed[0].Number)
	}
}

func TestPartitionDeselectedEmptySetKeepsEverything(t *testing.T) {
	repos := []RepositoryData{
		{Number: 1, Key: "org__a__main", Repository: "a"},
		{Number: 2, Key: "org__b__main", Repository: "b"},
	}
	kept, removed := PartitionDeselected(repos, nil)
	if len(kept) != 2 || removed != nil {
		t.Errorf("kept %d, removed %+v; want all kept and none removed", len(kept), removed)
	}
}

func TestDeselectedRecords(t *testing.T) {
	records := DeselectedRecords([]RepositoryData{
		{Key: "org__a__main", Repository: "a", Org: "org", Branch: testBranchMain},
	})
	if len(records) != 1 {
		t.Fatalf("got %d records, want 1", len(records))
	}
	want := DeselectedRepo{Key: "org__a__main", Org: "org", Repo: "a", Branch: testBranchMain}
	if records[0] != want {
		t.Errorf("got %+v, want %+v", records[0], want)
	}
}

func TestAdjustGlobalInfoUntouchedWhenNothingDeselected(t *testing.T) {
	// Enabling this feature must not move a number on an unfiltered report, so the
	// scan's own figures are returned verbatim rather than recomputed.
	in := Globalinfo{
		TotalLinesOfCode:       "1.23M",
		LargestRepository:      "monolith",
		LinesOfCodeLargestRepo: "800.00K",
		NumberRepos:            42,
	}
	got := AdjustGlobalInfo(in, []LanguageData{{Language: "Go", CodeLines: 5}}, []RepoTotal{{Repo: "other", CodeLines: 5}}, 0)
	if got != in {
		t.Errorf("AdjustGlobalInfo changed an unfiltered report:\n got %+v\nwant %+v", got, in)
	}
}

func TestAdjustGlobalInfoRecomputesWhenDeselected(t *testing.T) {
	in := Globalinfo{
		TotalLinesOfCode:       "1.23M",
		LargestRepository:      "dead-monolith",
		LinesOfCodeLargestRepo: "800.00K",
		NumberRepos:            10,
	}
	languages := []LanguageData{
		{Language: "Go", CodeLines: 900},
		{Language: testLangJava, CodeLines: 100},
		{Language: LanguageExcludedFromTotalLOC, CodeLines: 5000}, // held out of the total
	}
	repoTotals := []RepoTotal{
		{Repo: "small", CodeLines: 100},
		{Repo: "big", CodeLines: 900},
	}

	got := AdjustGlobalInfo(in, languages, repoTotals, 3)

	if got.TotalLinesOfCode != FormatCodeLines(1000) {
		t.Errorf("TotalLinesOfCode = %q, want %q (JSON must stay excluded)", got.TotalLinesOfCode, FormatCodeLines(1000))
	}
	if got.LargestRepository != "big" {
		t.Errorf("LargestRepository = %q, want big", got.LargestRepository)
	}
	if got.LinesOfCodeLargestRepo != FormatCodeLines(900) {
		t.Errorf("LinesOfCodeLargestRepo = %q, want %q", got.LinesOfCodeLargestRepo, FormatCodeLines(900))
	}
	if got.NumberRepos != 7 {
		t.Errorf("NumberRepos = %d, want 7", got.NumberRepos)
	}
}

func TestRankTopLanguagesExcludesHeldOutLanguage(t *testing.T) {
	// JSON is held out of the headline LOC figure, so it must not appear as a repository's
	// top language either — it would sit next to a code-line count that deliberately does
	// not include those lines.
	got := RankTopLanguages([]LanguageShare{
		{Language: LanguageExcludedFromTotalLOC, CodeLines: 900_000},
		{Language: "Go", CodeLines: 300},
		{Language: testLangJava, CodeLines: 200},
		{Language: "XML", CodeLines: 100},
		{Language: "Shell", CodeLines: 50},
		{Language: "  ", CodeLines: 999}, // blank name, ignored
		{Language: "Empty", CodeLines: 0},
	}, 3)

	if len(got) != 3 {
		t.Fatalf("got %d languages, want 3", len(got))
	}
	want := []string{"Go", testLangJava, "XML"}
	for i, w := range want {
		if got[i].Language != w {
			t.Errorf("position %d = %q, want %q", i, got[i].Language, w)
		}
	}
	if got[0].CodeLinesF != FormatCodeLines(300) {
		t.Errorf("CodeLinesF = %q, want %q", got[0].CodeLinesF, FormatCodeLines(300))
	}
}

func TestRankTopLanguagesTiesAreStable(t *testing.T) {
	// Equal line counts must order deterministically, or the page and the reports could
	// list the same repository's languages differently.
	for i := 0; i < 5; i++ {
		got := RankTopLanguages([]LanguageShare{
			{Language: "Zig", CodeLines: 100},
			{Language: "Ada", CodeLines: 100},
			{Language: "Perl", CodeLines: 100},
		}, 3)
		if got[0].Language != "Ada" || got[1].Language != "Perl" || got[2].Language != "Zig" {
			t.Fatalf("unstable tie order: %+v", got)
		}
	}
}

func TestRankTopLanguagesHandlesFewerThanLimit(t *testing.T) {
	got := RankTopLanguages([]LanguageShare{{Language: "Go", CodeLines: 5}}, 3)
	if len(got) != 1 {
		t.Errorf("got %d, want 1 — a repository with one language must not be padded", len(got))
	}
	if empty := RankTopLanguages(nil, 3); len(empty) != 0 {
		t.Errorf("got %d, want 0 for no language data", len(empty))
	}
}

func TestRankTopRepositories(t *testing.T) {
	repos := make([]RepoTotal, 0, 40)
	for i := 1; i <= 40; i++ {
		repos = append(repos, RepoTotal{Repo: fmt.Sprintf("repo%02d", i), CodeLines: i * 10})
	}
	repos = append(repos, RepoTotal{Repo: "empty", CodeLines: 0})

	got := RankTopRepositories(repos, TopRepositoriesShown)

	if len(got) != TopRepositoriesShown {
		t.Fatalf("got %d repositories, want %d", len(got), TopRepositoriesShown)
	}
	if got[0].Repo != "repo40" {
		t.Errorf("first = %q, want repo40 (largest)", got[0].Repo)
	}
	// Descending, and the zero-LOC repository must be dropped rather than padding the list.
	for i := 1; i < len(got); i++ {
		if got[i].CodeLines > got[i-1].CodeLines {
			t.Fatalf("not sorted descending at %d: %+v", i, got)
		}
		if got[i].CodeLines == 0 {
			t.Errorf("zero-LOC repository should not be listed")
		}
	}
}

func TestRankTopRepositoriesTiesAreStable(t *testing.T) {
	for i := 0; i < 5; i++ {
		got := RankTopRepositories([]RepoTotal{
			{Repo: "zulu", CodeLines: 7},
			{Repo: "alpha", CodeLines: 7},
		}, 30)
		if got[0].Repo != "alpha" || got[1].Repo != "zulu" {
			t.Fatalf("unstable tie order: %+v", got)
		}
	}
}

func TestRankTopRepositoriesFewerThanLimit(t *testing.T) {
	got := RankTopRepositories([]RepoTotal{{Repo: "only", CodeLines: 1}}, 30)
	if len(got) != 1 {
		t.Errorf("got %d, want 1", len(got))
	}
}

func TestAdjustGlobalInfoNumberReposFloorsAtZero(t *testing.T) {
	got := AdjustGlobalInfo(Globalinfo{NumberRepos: 2}, nil, nil, 5)
	if got.NumberRepos != 0 {
		t.Errorf("NumberRepos = %d, want 0", got.NumberRepos)
	}
}
