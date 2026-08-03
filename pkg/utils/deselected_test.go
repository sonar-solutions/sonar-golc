package utils

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
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
		{"plain", "my-org", "api-service", "main", "Result_my-org__api-service__main.json"},
		{"underscores everywhere", "my_group", "my_repo", "feat_x", "Result_my_group__my_repo__feat_x.json"},
		{"empty org", "", "repo", "main", "Result___repo__main.json"},
		{"byfile variant", "org", "repo", "main", "Result_org__repo__main_byfile.json"},
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
		{Key: DeselectionKey("org", "dead-repo", "main"), Org: "org", Repo: "dead-repo", Branch: "main"},
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
		{Key: "org__a__main", Repository: "a", Org: "org", Branch: "main"},
	})
	if len(records) != 1 {
		t.Fatalf("got %d records, want 1", len(records))
	}
	want := DeselectedRepo{Key: "org__a__main", Org: "org", Repo: "a", Branch: "main"}
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
		{Language: "Java", CodeLines: 100},
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

func TestAdjustGlobalInfoNumberReposFloorsAtZero(t *testing.T) {
	got := AdjustGlobalInfo(Globalinfo{NumberRepos: 2}, nil, nil, 5)
	if got.NumberRepos != 0 {
		t.Errorf("NumberRepos = %d, want 0", got.NumberRepos)
	}
}
