package utils

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// These tests pin the result-file layout for every platform. Two independent copies of
// this logic used to exist and drifted apart; the table below is what makes a future
// divergence a test failure rather than a silently missing repository.

func TestPlatformSpecNamingPerPlatform(t *testing.T) {
	branch := ProjectBranch{
		Org:        "acme-org",
		ProjectKey: "PROJ",
		RepoSlug:   "svc",
		MainBranch: testBranchMain,
	}

	cases := []struct {
		platform      string
		wantFirstPart string
		wantByFile    string
		wantByLang    string
	}{
		// GitHub and GitLab group by organization.
		{"github", "acme-org", "Result_acme-org__svc__main_byfile.json", "Result_acme-org__svc__main.json"},
		{"gitlab", "acme-org", "Result_acme-org__svc__main_byfile.json", "Result_acme-org__svc__main.json"},
		// Azure and Bitbucket group by project key.
		{"bitbucket", "PROJ", "Result_PROJ__svc__main_byfile.json", "Result_PROJ__svc__main.json"},
		{"bitbucket_dc", "PROJ", "Result_PROJ__svc__main_byfile.json", "Result_PROJ__svc__main.json"},
		{"azure", "PROJ", "Result_PROJ__svc__main_byfile.json", "Result_PROJ__svc__main.json"},
		// The file platform names results by directory alone.
		{"file", "svc", "Result_svc_byfile.json", "Result_svc.json"},
	}

	for _, tc := range cases {
		t.Run(tc.platform, func(t *testing.T) {
			spec, ok := PlatformSpecFor(tc.platform)
			if !ok {
				t.Fatalf("no spec for platform %q", tc.platform)
			}
			if got := spec.FirstPart(branch); got != tc.wantFirstPart {
				t.Errorf("FirstPart = %q, want %q", got, tc.wantFirstPart)
			}
			if got := filepath.Base(spec.ByFilePath("Results", branch)); got != tc.wantByFile {
				t.Errorf("ByFilePath = %q, want %q", got, tc.wantByFile)
			}
			if got := filepath.Base(spec.ByLanguagePath("Results", branch)); got != tc.wantByLang {
				t.Errorf("ByLanguagePath = %q, want %q", got, tc.wantByLang)
			}
			// The inventory file name must match what the scanners write.
			wantInv := filepath.Join("Results", "config", "analysis_result_"+tc.platform+".json")
			if got := spec.InventoryPath("Results"); got != wantInv {
				t.Errorf("InventoryPath = %q, want %q", got, wantInv)
			}
		})
	}
}

func TestPlatformSpecFallsBackWhenProjectKeyMissing(t *testing.T) {
	// An empty first component names a file that matches nothing, so the repository would
	// vanish from the reports. Azure falls back to the repository, Bitbucket to the org.
	noKey := ProjectBranch{Org: "acme-org", RepoSlug: "svc", MainBranch: testBranchMain}

	cases := map[string]string{
		"azure":        "svc",
		"bitbucket":    "acme-org",
		"bitbucket_dc": "acme-org",
	}
	for platform, want := range cases {
		spec, _ := PlatformSpecFor(platform)
		if got := spec.FirstPart(noKey); got != want {
			t.Errorf("%s: FirstPart with no project key = %q, want %q", platform, got, want)
		}
		if got := spec.FirstPart(noKey); got == "" {
			t.Errorf("%s: first component must never be empty", platform)
		}
	}
}

func TestPlatformSpecKeyMatchesFileName(t *testing.T) {
	// The deselection key must be the stem of the file the numbers are read from, or a
	// repository deselected on the page stays counted in the reports.
	for _, spec := range platformSpecs {
		branch := ProjectBranch{Org: "group/sub", ProjectKey: "PROJ", RepoSlug: "svc", MainBranch: "release/1.0"}
		stem := spec.resultStem(branch)
		fromName, ok := DeselectionKeyFromResultFileName(stem + ".json")
		if !ok {
			t.Errorf("%s: %q not recognised as a result file", spec.Name, stem)
			continue
		}
		if got := spec.DeselectionKey(branch); got != fromName {
			t.Errorf("%s: key from fields %q != key from file name %q", spec.Name, got, fromName)
		}
	}
}

func TestDetectPlatformIsOrderedNotRandom(t *testing.T) {
	// With inventories from several platforms present, the pick must be deterministic:
	// otherwise the reports can describe a different platform than the page.
	base := t.TempDir()
	if err := os.MkdirAll(filepath.Join(base, "config"), 0755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"azure", "github", "gitlab"} {
		body, _ := json.Marshal(AnalysisResult{})
		if err := os.WriteFile(filepath.Join(base, "config", "analysis_result_"+name+".json"), body, 0644); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 5; i++ {
		spec, _, err := DetectPlatform(base)
		if err != nil {
			t.Fatalf("DetectPlatform: %v", err)
		}
		if spec.Name != "github" {
			t.Fatalf("detected %q, want the first spec in table order (github)", spec.Name)
		}
	}
}

func TestDetectPlatformReportsNothingFound(t *testing.T) {
	if _, _, err := DetectPlatform(t.TempDir()); err == nil {
		t.Error("expected an error when no inventory exists")
	}
}

func TestPreferredBranchesPrefersMain(t *testing.T) {
	// An all-branches scan records several entries per repository; the summaries show one.
	got := PreferredBranches([]ProjectBranch{
		{RepoSlug: "svc", MainBranch: "feature/x"},
		{RepoSlug: "svc", MainBranch: testBranchMain},
		{RepoSlug: "other", MainBranch: "release/2"},
	})
	if len(got) != 2 {
		t.Fatalf("got %d repositories, want 2", len(got))
	}
	if got["svc"].MainBranch != testBranchMain {
		t.Errorf("svc resolved to %q, want the main branch", got["svc"].MainBranch)
	}
	// A repository with no main-ish branch still yields its only entry.
	if got["other"].MainBranch != "release/2" {
		t.Errorf("other resolved to %q, want release/2", got["other"].MainBranch)
	}
}

// writeRepoFixture lays down the inventory and result documents for one repository.
func writeRepoFixture(t *testing.T, base, platform string, branch ProjectBranch, code int, langs []LanguageShare) {
	t.Helper()
	spec, ok := PlatformSpecFor(platform)
	if !ok {
		t.Fatalf("unknown platform %q", platform)
	}
	for _, dir := range []string{
		filepath.Join(base, "config"),
		filepath.Join(base, "byfile-report"),
		filepath.Join(base, "bylanguage-report"),
	} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
	}
	inv, _ := json.Marshal(AnalysisResult{NumRepositories: 1, ProjectBranches: []ProjectBranch{branch}})
	if err := os.WriteFile(spec.InventoryPath(base), inv, 0644); err != nil {
		t.Fatal(err)
	}
	byFile, _ := json.Marshal(map[string]int{
		"TotalLines": code * 2, "TotalBlankLines": 3, "TotalComments": 4, "TotalCodeLines": code,
	})
	if err := os.WriteFile(spec.ByFilePath(base, branch), byFile, 0644); err != nil {
		t.Fatal(err)
	}
	byLang, _ := json.Marshal(map[string]any{"Results": langs})
	if err := os.WriteFile(spec.ByLanguagePath(base, branch), byLang, 0644); err != nil {
		t.Fatal(err)
	}
}

func TestReadRepositoryDataEndToEnd(t *testing.T) {
	base := t.TempDir()
	branch := ProjectBranch{Org: "acme", ProjectKey: "PROJ", RepoSlug: "svc", MainBranch: testBranchMain}
	// TotalCodeLines is the sum of every language including the held-out one, as the
	// scanner writes it: 700 + 200 + 5000. The headline figure must come out at 900.
	writeRepoFixture(t, base, "azure", branch, 5900, []LanguageShare{
		{Language: "Go", CodeLines: 700},
		{Language: "YAML", CodeLines: 200},
		{Language: LanguageExcludedFromTotalLOC, CodeLines: 5000},
	})

	repos, err := ReadRepositoryData(base)
	if err != nil {
		t.Fatalf("ReadRepositoryData: %v", err)
	}
	if len(repos) != 1 {
		t.Fatalf("got %d repositories, want 1", len(repos))
	}
	repo := repos[0]

	if repo.Repository != "svc" || repo.Branch != testBranchMain {
		t.Errorf("identity = %q/%q, want svc/main", repo.Repository, repo.Branch)
	}
	// Azure groups by project key, so that is what Org names.
	if repo.Org != "PROJ" {
		t.Errorf("Org = %q, want the project key PROJ", repo.Org)
	}
	// The held-out language is subtracted from the headline figure and excluded from the
	// language ranking.
	if repo.CodeLines != 900 {
		t.Errorf("CodeLines = %d, want 900 (5900 total less the 5000 held out)", repo.CodeLines)
	}
	if repo.CodeLinesF != FormatCodeLines(900) {
		t.Errorf("CodeLinesF = %q, want %q", repo.CodeLinesF, FormatCodeLines(900))
	}
	if repo.PrimaryLanguage() != "Go" {
		t.Errorf("PrimaryLanguage = %q, want Go", repo.PrimaryLanguage())
	}
	if len(repo.TopLanguages) != 2 {
		t.Errorf("TopLanguages = %+v, want Go and YAML only", repo.TopLanguages)
	}
	if repo.Key != (PlatformSpec{Name: "azure", firstPart: projectKeyOrRepo}).DeselectionKey(branch) {
		t.Errorf("Key = %q, does not match the platform rule", repo.Key)
	}
	if repo.Number != 1 {
		t.Errorf("Number = %d, want 1", repo.Number)
	}
}

func TestReadRepositoryDataOrdersTiesDeterministically(t *testing.T) {
	// Repositories of equal size — commonly several at zero — must come out in a defined
	// order. sort.Slice makes no promise for equal elements and the input arrives from a
	// map, so without a tiebreak the order can differ between runs, Go versions or
	// callers, turning every report diff into noise and moving row numbers for no reason.
	base := t.TempDir()
	spec, _ := PlatformSpecFor("github")
	branches := []ProjectBranch{
		{Org: "acme", RepoSlug: "zulu", MainBranch: testBranchMain},
		{Org: "acme", RepoSlug: "alpha", MainBranch: testBranchMain},
		{Org: "acme", RepoSlug: "mike", MainBranch: testBranchMain},
	}
	for _, b := range branches {
		writeRepoFixture(t, base, "github", b, 0, nil) // all tied at zero
	}
	inv, _ := json.Marshal(AnalysisResult{NumRepositories: len(branches), ProjectBranches: branches})
	if err := os.WriteFile(spec.InventoryPath(base), inv, 0644); err != nil {
		t.Fatal(err)
	}

	var first []string
	for run := 0; run < 5; run++ {
		repos, err := ReadRepositoryData(base)
		if err != nil {
			t.Fatalf("ReadRepositoryData: %v", err)
		}
		order := make([]string, 0, len(repos))
		for _, r := range repos {
			order = append(order, r.Repository)
		}
		if first == nil {
			first = order
			// Ties resolve by key, which for these repositories means alphabetical.
			if want := []string{"alpha", "mike", "zulu"}; !equalStrings(order, want) {
				t.Errorf("tie order = %v, want %v (by key)", order, want)
			}
			continue
		}
		if !equalStrings(order, first) {
			t.Fatalf("order changed between runs: %v then %v", first, order)
		}
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestReadRepositoryDataSkipsUnreadableRepositories(t *testing.T) {
	// One missing result document must cost that repository, not the whole report.
	base := t.TempDir()
	present := ProjectBranch{Org: "acme", RepoSlug: "present", MainBranch: testBranchMain}
	writeRepoFixture(t, base, "github", present, 100, []LanguageShare{{Language: "Go", CodeLines: 100}})

	// Add a second repository to the inventory without writing its result documents.
	spec, _ := PlatformSpecFor("github")
	inv, _ := json.Marshal(AnalysisResult{NumRepositories: 2, ProjectBranches: []ProjectBranch{
		present,
		{Org: "acme", RepoSlug: "missing", MainBranch: testBranchMain},
	}})
	if err := os.WriteFile(spec.InventoryPath(base), inv, 0644); err != nil {
		t.Fatal(err)
	}

	repos, err := ReadRepositoryData(base)
	if err != nil {
		t.Fatalf("ReadRepositoryData: %v", err)
	}
	if len(repos) != 1 || repos[0].Repository != "present" {
		t.Errorf("got %+v, want only the readable repository", repos)
	}
}

func TestReadRepositoryDataSortsBySizeAndNumbers(t *testing.T) {
	base := t.TempDir()
	spec, _ := PlatformSpecFor("github")
	branches := []ProjectBranch{
		{Org: "acme", RepoSlug: "small", MainBranch: testBranchMain},
		{Org: "acme", RepoSlug: "big", MainBranch: testBranchMain},
		{Org: "acme", RepoSlug: "mid", MainBranch: testBranchMain},
	}
	for i, b := range branches {
		writeRepoFixture(t, base, "github", b, (i+1)*100, []LanguageShare{{Language: "Go", CodeLines: (i + 1) * 100}})
	}
	inv, _ := json.Marshal(AnalysisResult{NumRepositories: len(branches), ProjectBranches: branches})
	if err := os.WriteFile(spec.InventoryPath(base), inv, 0644); err != nil {
		t.Fatal(err)
	}

	repos, err := ReadRepositoryData(base)
	if err != nil {
		t.Fatalf("ReadRepositoryData: %v", err)
	}
	want := []string{"mid", "big", "small"} // 300, 200, 100
	for i, name := range want {
		if repos[i].Repository != name {
			t.Errorf("position %d = %q, want %q (largest first)", i, repos[i].Repository, name)
		}
		if repos[i].Number != i+1 {
			t.Errorf("position %d numbered %d, want %d", i, repos[i].Number, i+1)
		}
	}
}
