package utils

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

const resultMainJSON = "Result_org_repo_main.json"

// helper to set up a temp workspace and chdir into it
func setupGlobalReportEnv(t *testing.T) (string, func()) {
	t.Helper()
	tempDir, err := os.MkdirTemp("", "test_globalreport_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	orig, _ := os.Getwd()
	if err := os.Chdir(tempDir); err != nil {
		t.Fatalf("failed to chdir: %v", err)
	}
	cleanup := func() {
		_ = os.Chdir(orig)
		_ = os.RemoveAll(tempDir)
	}
	return tempDir, cleanup
}

func writeResultJSON(t *testing.T, dir, name string, data FileData) {
	t.Helper()
	path := filepath.Join(dir, name)
	b, _ := json.Marshal(data)
	if err := os.WriteFile(path, b, 0644); err != nil {
		t.Fatalf("failed to write %s: %v", path, err)
	}
}

func TestIsEligibleResultFile(t *testing.T) {
	dir, cleanup := setupGlobalReportEnv(t)
	defer cleanup()
	// create candidate files
	result := filepath.Join(dir, resultMainJSON)
	byfile := filepath.Join(dir, "Result_org_repo_main_byfile.json")
	other := filepath.Join(dir, "random.json")
	os.WriteFile(result, []byte("{}"), 0644)
	os.WriteFile(byfile, []byte("{}"), 0644)
	os.WriteFile(other, []byte("{}"), 0644)

	fiRes, _ := os.Stat(result)
	fiBy, _ := os.Stat(byfile)
	fiO, _ := os.Stat(other)
	if !isEligibleResultFile(fiRes, result) {
		t.Errorf("expected %s to be eligible", result)
	}
	// byfile is now eligible so language totals are populated when only byfile-report exists
	if !isEligibleResultFile(fiBy, byfile) {
		t.Errorf("expected %s to be eligible (byfile reports used for language totals)", byfile)
	}
	if isEligibleResultFile(fiO, other) {
		t.Errorf("did not expect %s to be eligible", other)
	}
}

func TestAccumulateLanguageTotalsFromFile(t *testing.T) {
	dir, cleanup := setupGlobalReportEnv(t)
	defer cleanup()
	path := filepath.Join(dir, resultMainJSON)
	writeResultJSON(t, dir, resultMainJSON, FileData{
		Results: []LanguageData1{
			{Language: "Go", CodeLines: 100},
			{Language: "Java", CodeLines: 50},
			{Language: " ", CodeLines: 999}, // ignored
		},
	})
	totals := map[string]int{}
	if err := accumulateLanguageTotalsFromFile(path, totals); err != nil {
		t.Fatalf("accumulateLanguageTotalsFromFile error: %v", err)
	}
	if totals["Go"] != 100 || totals["Java"] != 50 {
		t.Errorf("unexpected totals: %+v", totals)
	}
	if _, ok := totals[""]; ok {
		t.Errorf("blank language should be ignored")
	}
}

func TestCollectLanguageTotalsAggregatesByfileAndBylanguage(t *testing.T) {
	dir, cleanup := setupGlobalReportEnv(t)
	defer cleanup()
	// arrange result files: both by-language and by-file are aggregated
	writeResultJSON(t, dir, "Result_org_a_main.json", FileData{
		Results: []LanguageData1{{Language: "Go", CodeLines: 10}},
	})
	writeResultJSON(t, dir, "Result_org_b_main_byfile.json", FileData{
		Results: []LanguageData1{{Language: "Go", CodeLines: 1000}},
	})
	writeResultJSON(t, dir, "random.json", FileData{
		Results: []LanguageData1{{Language: "Go", CodeLines: 999}},
	})
	totals, err := collectLanguageTotals(dir)
	if err != nil {
		t.Fatalf("collectLanguageTotals error: %v", err)
	}
	// Only Result_* files are included (random.json is skipped); both by-language and byfile are summed
	if totals["Go"] != 1010 {
		t.Errorf("expected 1010 (10+1000), got %d", totals["Go"])
	}
}

func TestWriteLanguageTotalsJSONAndReadGlobalInfoAndRenderPDF(t *testing.T) {
	_, cleanup := setupGlobalReportEnv(t)
	defer cleanup()
	// prepare required directories/files
	_ = os.MkdirAll("Logs", 0755)
	_ = os.MkdirAll("Results", 0755)
	// write GlobalReport.json that renderGlobalPDF will read
	gr := Globalinfo{
		Organization:           "org",
		TotalLinesOfCode:       "100",
		LargestRepository:      "repo",
		LinesOfCodeLargestRepo: "60",
		DevOpsPlatform:         "gitlab",
		NumberRepos:            1,
	}
	bgr, _ := json.Marshal(gr)
	if err := os.WriteFile("Results/GlobalReport.json", bgr, 0644); err != nil {
		t.Fatalf("failed writing GlobalReport.json: %v", err)
	}

	// write language totals (directory as first arg for Windows-safe paths)
	data, err := writeLanguageTotalsJSON("Results", map[string]int{"Go": 100})
	if err != nil {
		t.Fatalf("writeLanguageTotalsJSON error: %v", err)
	}
	// ensure file exists
	if _, err := os.Stat("Results/code_lines_by_language.json"); err != nil {
		t.Fatalf("expected code_lines_by_language.json: %v", err)
	}

	// validate render pipeline by calling CreateGlobalReport which uses our helpers
	if err := CreateGlobalReport("Results"); err != nil {
		t.Fatalf("CreateGlobalReport error: %v", err)
	}
	// ensure PDF file exists and is non-empty
	info, err := os.Stat("Results/GlobalReport.pdf")
	if err != nil {
		t.Fatalf("expected GlobalReport.pdf: %v", err)
	}
	if info.Size() == 0 {
		t.Fatalf("GlobalReport.pdf is empty")
	}

	// also check we can unmarshal the bytes returned earlier (data)
	var langs []LanguageData1
	if err := json.Unmarshal(data, &langs); err != nil {
		t.Fatalf("unexpected language json: %v", err)
	}
}


