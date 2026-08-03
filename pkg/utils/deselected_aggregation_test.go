package utils

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// writeByLanguageResult writes one repository's by-language result file, the input
// the global report is aggregated from.
func writeByLanguageResult(t *testing.T, dir, fileName string, langs []LanguageData1) {
	t.Helper()
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(FileData{Results: langs})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, fileName), data, 0644); err != nil {
		t.Fatal(err)
	}
}

func TestCollectResultTotalsExcludesDeselectedRepos(t *testing.T) {
	base := t.TempDir()
	byLang := filepath.Join(base, "bylanguage-report")

	writeByLanguageResult(t, byLang, "Result_org__keep__main.json", []LanguageData1{
		{Language: "Go", CodeLines: 100},
		{Language: "JSON", CodeLines: 900}, // held out of the headline total
	})
	writeByLanguageResult(t, byLang, "Result_org__drop__main.json", []LanguageData1{
		{Language: "Go", CodeLines: 50},
		{Language: testLangJava, CodeLines: 25},
	})

	deselected := DeselectionSet{DeselectionKey("org", testRepoDrop, testBranchMain): true}
	totals, repoTotals, err := collectResultTotals(base, deselected)
	if err != nil {
		t.Fatalf("collectResultTotals: %v", err)
	}

	// The deselected repository must contribute to neither the language breakdown...
	if totals[testLangJava] != 0 {
		t.Errorf("Java = %d, want 0: the only Java came from the deselected repo", totals[testLangJava])
	}
	if totals["Go"] != 100 {
		t.Errorf("Go = %d, want 100 (deselected repo's 50 must not count)", totals["Go"])
	}
	// ...nor the per-repository totals that drive the largest-repo figure.
	if len(repoTotals) != 1 {
		t.Fatalf("got %d repo totals, want 1", len(repoTotals))
	}
	if repoTotals[0].Repo != testRepoKeep {
		t.Errorf("repo = %q, want keep", repoTotals[0].Repo)
	}
	// JSON is excluded from a repository's contribution, as everywhere else.
	if repoTotals[0].CodeLines != 100 {
		t.Errorf("CodeLines = %d, want 100 (JSON excluded)", repoTotals[0].CodeLines)
	}
	if repoTotals[0].Key != DeselectionKey("org", testRepoKeep, testBranchMain) {
		t.Errorf("Key = %q, want %q", repoTotals[0].Key, DeselectionKey("org", testRepoKeep, testBranchMain))
	}
}

func TestCollectResultTotalsWithNilSetCountsEverything(t *testing.T) {
	base := t.TempDir()
	byLang := filepath.Join(base, "bylanguage-report")

	writeByLanguageResult(t, byLang, "Result_org__a__main.json", []LanguageData1{{Language: "Go", CodeLines: 10}})
	writeByLanguageResult(t, byLang, "Result_org__b__main.json", []LanguageData1{{Language: "Go", CodeLines: 20}})

	totals, repoTotals, err := collectResultTotals(base, nil)
	if err != nil {
		t.Fatalf("collectResultTotals: %v", err)
	}
	if totals["Go"] != 30 {
		t.Errorf("Go = %d, want 30", totals["Go"])
	}
	if len(repoTotals) != 2 {
		t.Errorf("got %d repo totals, want 2", len(repoTotals))
	}
}

func TestCollectLanguageTotalsUnchangedByDeselectionFeature(t *testing.T) {
	// collectLanguageTotals is the pre-existing entry point; it must keep counting
	// every result file regardless of any persisted selection.
	base := t.TempDir()
	byLang := filepath.Join(base, "bylanguage-report")
	writeByLanguageResult(t, byLang, "Result_org__a__main.json", []LanguageData1{{Language: "Go", CodeLines: 7}})

	if err := SaveDeselectedRepos(base, []DeselectedRepo{
		{Key: DeselectionKey("org", "a", testBranchMain), Repo: "a"},
	}); err != nil {
		t.Fatal(err)
	}

	totals, err := collectLanguageTotals(base)
	if err != nil {
		t.Fatalf("collectLanguageTotals: %v", err)
	}
	if totals["Go"] != 7 {
		t.Errorf("Go = %d, want 7: collectLanguageTotals must not apply a selection", totals["Go"])
	}
}

func TestGenerateRepositorySummaryReportsSplitsDeselected(t *testing.T) {
	// GenerateRepositorySummaryReports reads from hard-coded "Results/..." paths, so
	// the test runs inside a temp working directory.
	dir := t.TempDir()
	origWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(origWD) })

	for _, sub := range []string{
		"Results/config",
		"Results/byfile-report/csv-report",
		"Results/byfile-report/pdf-report",
		"Results/bylanguage-report",
	} {
		if err := os.MkdirAll(sub, 0755); err != nil {
			t.Fatal(err)
		}
	}

	inventory := AnalysisResult{
		NumRepositories: 2,
		ProjectBranches: []ProjectBranch{
			{Org: "org", RepoSlug: testRepoKeep, MainBranch: testBranchMain},
			{Org: "org", RepoSlug: testRepoDrop, MainBranch: testBranchMain},
		},
	}
	invJSON, _ := json.Marshal(inventory)
	if err := os.WriteFile("Results/config/analysis_result_github.json", invJSON, 0644); err != nil {
		t.Fatal(err)
	}

	writeByFile := func(name string, code int) {
		body, _ := json.Marshal(map[string]int{
			"TotalLines": code * 2, "TotalBlankLines": 1, "TotalComments": 2, "TotalCodeLines": code,
		})
		if err := os.WriteFile(filepath.Join("Results/byfile-report", name), body, 0644); err != nil {
			t.Fatal(err)
		}
	}
	writeByFile("Result_org__keep__main_byfile.json", 100)
	writeByFile("Result_org__drop__main_byfile.json", 40)

	if err := SaveDeselectedRepos("Results", []DeselectedRepo{
		{Key: DeselectionKey("org", testRepoDrop, testBranchMain), Org: "org", Repo: testRepoDrop, Branch: testBranchMain},
	}); err != nil {
		t.Fatal(err)
	}

	if err := GenerateRepositorySummaryReports("Results"); err != nil {
		t.Fatalf("GenerateRepositorySummaryReports: %v", err)
	}

	data, err := os.ReadFile("Results/byfile-report/repository_summary.json")
	if err != nil {
		t.Fatalf("reading generated summary: %v", err)
	}
	var summary RepositorySummaryReport
	if err := json.Unmarshal(data, &summary); err != nil {
		t.Fatalf("generated summary is not valid JSON: %v", err)
	}

	if summary.TotalRepositories != 1 {
		t.Errorf("TotalRepositories = %d, want 1", summary.TotalRepositories)
	}
	if summary.TotalCodeLines != 100 {
		t.Errorf("TotalCodeLines = %d, want 100 (deselected 40 must not count)", summary.TotalCodeLines)
	}
	// The removed repository has to remain visible and quantified, otherwise the
	// report understates the scan without saying so.
	if summary.DeselectedRepositories != 1 {
		t.Errorf("DeselectedRepositories = %d, want 1", summary.DeselectedRepositories)
	}
	if summary.DeselectedCodeLines != 40 {
		t.Errorf("DeselectedCodeLines = %d, want 40", summary.DeselectedCodeLines)
	}
	if len(summary.Deselected) != 1 || summary.Deselected[0].Repository != testRepoDrop {
		t.Errorf("Deselected = %+v, want one entry for drop", summary.Deselected)
	}

	// The PDF and CSV must exist, since those are the artifacts handed to a customer.
	for _, path := range []string{
		"Results/byfile-report/pdf-report/repository_summary.pdf",
		"Results/byfile-report/csv-report/repository_summary.csv",
	} {
		if info, err := os.Stat(path); err != nil || info.Size() == 0 {
			t.Errorf("expected non-empty %s (err=%v)", path, err)
		}
	}
}

func TestGenerateRepositorySummaryReportsHonoursDeselectionForSubgroupOrg(t *testing.T) {
	// Regression test for the key-divergence bug: a GitLab subgroup org ("group/subgroup")
	// and a slashed branch ("release/1.0") both put a path separator in a key component.
	// The results page and this generator built their file paths separately, so the key
	// they derived diverged and a repository deselected on the page stayed counted here.
	dir := t.TempDir()
	origWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(origWD) })

	for _, sub := range []string{
		"Results/config", "Results/byfile-report/csv-report", "Results/byfile-report/pdf-report",
	} {
		if err := os.MkdirAll(sub, 0755); err != nil {
			t.Fatal(err)
		}
	}

	const (
		org    = "group/subgroup"
		branch = "release/1.0"
	)
	invJSON, _ := json.Marshal(AnalysisResult{
		NumRepositories: 2,
		ProjectBranches: []ProjectBranch{
			{Org: org, RepoSlug: testRepoKeep, MainBranch: branch},
			{Org: org, RepoSlug: testRepoDrop, MainBranch: branch},
		},
	})
	if err := os.WriteFile("Results/config/analysis_result_gitlab.json", invJSON, 0644); err != nil {
		t.Fatal(err)
	}

	// File names are spelled out as the reporters actually write them —
	// strings.Replace(OutputName, "/", "_", -1) — and deliberately NOT built by calling
	// SanitizeResultComponent. Deriving the fixture from the function under test would
	// make this pass for any substitution rule, including one that never finds the file.
	const (
		onDiskOrg    = "group_subgroup"
		onDiskBranch = "release_1.0"
	)
	writeByFile := func(repo string, code int) {
		name := fmt.Sprintf("Result_%s__%s__%s_byfile.json", onDiskOrg, repo, onDiskBranch)
		body, _ := json.Marshal(map[string]int{
			"TotalLines": code * 2, "TotalBlankLines": 1, "TotalComments": 2, "TotalCodeLines": code,
		})
		if err := os.WriteFile(filepath.Join("Results/byfile-report", name), body, 0644); err != nil {
			t.Fatal(err)
		}
	}
	writeByFile(testRepoKeep, 100)
	writeByFile(testRepoDrop, 40)

	// The key the results page would submit for the deselected repository.
	pageKey := DeselectionKeyForRepo("gitlab", org, testRepoDrop, branch)
	if err := SaveDeselectedRepos("Results", []DeselectedRepo{
		{Key: pageKey, Org: org, Repo: testRepoDrop, Branch: branch},
	}); err != nil {
		t.Fatal(err)
	}

	if err := GenerateRepositorySummaryReports("Results"); err != nil {
		t.Fatalf("GenerateRepositorySummaryReports: %v", err)
	}

	data, err := os.ReadFile("Results/byfile-report/repository_summary.json")
	if err != nil {
		t.Fatalf("reading generated summary: %v", err)
	}
	var summary RepositorySummaryReport
	if err := json.Unmarshal(data, &summary); err != nil {
		t.Fatal(err)
	}

	// Both repositories must be found at all — separators mapped to "_" is what makes
	// the file names resolve.
	if summary.TotalRepositories+summary.DeselectedRepositories != 2 {
		t.Fatalf("found %d repositories in total, want 2 (subgroup file names must resolve)",
			summary.TotalRepositories+summary.DeselectedRepositories)
	}
	if summary.TotalCodeLines != 100 {
		t.Errorf("TotalCodeLines = %d, want 100: the deselected repo is still counted", summary.TotalCodeLines)
	}
	if summary.DeselectedRepositories != 1 || len(summary.Deselected) != 1 {
		t.Fatalf("Deselected = %d/%+v, want exactly one", summary.DeselectedRepositories, summary.Deselected)
	}
	if summary.Deselected[0].Repository != testRepoDrop {
		t.Errorf("deselected %q, want drop", summary.Deselected[0].Repository)
	}
	// The key this generator derives must equal the one the page submitted.
	if summary.Deselected[0].Key != pageKey {
		t.Errorf("key mismatch: report derived %q, page submitted %q", summary.Deselected[0].Key, pageKey)
	}
}

func TestGenerateRepositorySummaryReportsOmitsDeselectedFieldsWhenUnfiltered(t *testing.T) {
	dir := t.TempDir()
	origWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(origWD) })

	for _, sub := range []string{
		"Results/config", "Results/byfile-report/csv-report", "Results/byfile-report/pdf-report",
	} {
		if err := os.MkdirAll(sub, 0755); err != nil {
			t.Fatal(err)
		}
	}

	invJSON, _ := json.Marshal(AnalysisResult{
		NumRepositories: 1,
		ProjectBranches: []ProjectBranch{{Org: "org", RepoSlug: "only", MainBranch: testBranchMain}},
	})
	if err := os.WriteFile("Results/config/analysis_result_github.json", invJSON, 0644); err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(map[string]int{
		"TotalLines": 10, "TotalBlankLines": 1, "TotalComments": 2, "TotalCodeLines": 7,
	})
	if err := os.WriteFile("Results/byfile-report/Result_org__only__main_byfile.json", body, 0644); err != nil {
		t.Fatal(err)
	}

	if err := GenerateRepositorySummaryReports("Results"); err != nil {
		t.Fatalf("GenerateRepositorySummaryReports: %v", err)
	}

	data, err := os.ReadFile("Results/byfile-report/repository_summary.json")
	if err != nil {
		t.Fatal(err)
	}
	// An unfiltered report must not gain deselection fields at all, so existing
	// consumers of this JSON see no change.
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"Deselected", "DeselectedRepositories", "DeselectedCodeLines", "DeselectedCodeLinesF"} {
		if _, present := raw[key]; present {
			t.Errorf("unfiltered report should not contain %q", key)
		}
	}
}
