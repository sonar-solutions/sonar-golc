package utils

import (
	"bytes"
	"compress/zlib"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

// TestRenderGlobalPDF_NoMojibakeStatCard renders the global PDF with an accented
// largest-repository name and asserts the stat card text is translated to the font
// encoding — no raw multi-byte UTF-8 sequence should survive into the (decompressed)
// PDF content stream. Guards the gofpdf latin1 mojibake bug for the stat cards.
func TestRenderGlobalPDF_NoMojibakeStatCard(t *testing.T) {
	_, cleanup := setupGlobalReportEnv(t)
	defer cleanup()
	if err := os.MkdirAll("Results", 0755); err != nil {
		t.Fatalf("mkdir Results: %v", err)
	}

	ginfo := Globalinfo{
		Organization:           "verify-org", // ASCII, so any leak is from the stat card
		TotalLinesOfCode:       "29",
		LargestRepository:      "café-service", // accented, user-controlled
		LinesOfCodeLargestRepo: "18",
		DevOpsPlatform:         "azure",
		NumberRepos:            2,
	}
	langs := []LanguageData{{Language: "Golang", CodeLines: 29, Percentage: 100, CodeLinesF: "29"}}
	if err := renderGlobalPDF(langs, ginfo, nil, nil, nil, ginfo.TotalLinesOfCode, "Results/GlobalReport.pdf"); err != nil {
		t.Fatalf("renderGlobalPDF: %v", err)
	}

	raw, err := os.ReadFile("Results/GlobalReport.pdf")
	if err != nil {
		t.Fatalf("read pdf: %v", err)
	}
	var text bytes.Buffer
	for _, m := range regexp.MustCompile(`(?s)stream\r?\n(.*?)\r?\nendstream`).FindAllSubmatch(raw, -1) {
		if zr, err := zlib.NewReader(bytes.NewReader(m[1])); err == nil {
			_, _ = io.Copy(&text, zr)
			_ = zr.Close()
		}
	}
	// Guard against a false pass: the stat-card text must actually be in the
	// extracted stream, otherwise the assertion below is meaningless.
	if !bytes.Contains(text.Bytes(), []byte("Largest Repo")) {
		t.Fatal("could not find stat-card text in decompressed PDF; extraction likely failed")
	}
	// é -> C3 A9 in UTF-8; after translation it becomes single-byte cp1252 0xE9.
	if bytes.Contains(text.Bytes(), []byte{0xC3, 0xA9}) {
		t.Error("untranslated UTF-8 (é) leaked into GlobalReport.pdf stat card")
	}
}

const resultMainJSON = "Result_org__repo__main.json"

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
	byfile := filepath.Join(dir, "Result_org__repo__main_byfile.json")
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
	if isEligibleResultFile(fiBy, byfile) {
		t.Errorf("did not expect %s to be eligible", byfile)
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
	fileLOC, err := accumulateLanguageTotalsFromFile(path, totals)
	if err != nil {
		t.Fatalf("accumulateLanguageTotalsFromFile error: %v", err)
	}
	if totals["Go"] != 100 || totals["Java"] != 50 {
		t.Errorf("unexpected totals: %+v", totals)
	}
	if fileLOC != 150 {
		t.Errorf("accumulateLanguageTotalsFromFile fileLOC = %d, want 150", fileLOC)
	}
	if _, ok := totals[""]; ok {
		t.Errorf("blank language should be ignored")
	}
}

func TestCollectLanguageTotalsSkipsByfile(t *testing.T) {
	dir, cleanup := setupGlobalReportEnv(t)
	defer cleanup()
	// arrange result files
	writeResultJSON(t, dir, "Result_org__a__main.json", FileData{
		Results: []LanguageData1{{Language: "Go", CodeLines: 10}},
	})
	writeResultJSON(t, dir, "Result_org__b__main_byfile.json", FileData{
		Results: []LanguageData1{{Language: "Go", CodeLines: 1000}},
	})
	writeResultJSON(t, dir, "random.json", FileData{
		Results: []LanguageData1{{Language: "Go", CodeLines: 999}},
	})
	totals, err := collectLanguageTotals(dir)
	if err != nil {
		t.Fatalf("collectLanguageTotals error: %v", err)
	}
	if totals["Go"] != 10 {
		t.Errorf("expected 10, got %d", totals["Go"])
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

	// write language totals
	data, err := writeLanguageTotalsJSON(map[string]int{"Go": 100}, "Results/code_lines_by_language.json")
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

// TestParseResultFileName exercises the canonical Result_<Org>__<Repo>__<Branch>
// parser shared by pkg/reporter/pdf and the LargestRepository selection loop in
// golc.go. The non-empty `wantRepo` cases would all regress to blank under the
// pre-double-underscore single-_ split.
func TestParseResultFileName(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		wantOrg    string
		wantRepo   string
		wantBranch string
		wantOK     bool
	}{
		{"simple", "Result_org__repo__main.json", "org", "repo", "main", true},
		{"underscore in org", "Result_my_group__repo__main.json", "my_group", "repo", "main", true},
		{"underscore in repo", "Result_org__my_app__main.json", "org", "my_app", "main", true},
		{"underscore in branch", "Result_org__repo__feat_xyz.json", "org", "repo", "feat_xyz", true},
		{"byfile pdf", "Result_org__repo__main_byfile.pdf", "org", "repo", "main", true},
		{"plain pdf", "Result_org__repo__main.pdf", "org", "repo", "main", true},
		{"literal __ in branch survives", "Result_org__repo__feat__xyz.json", "org", "repo", "feat__xyz", true},
		{"missing branch", "Result_org__repo.json", "", "", "", false},
		{"missing prefix", "other_org__repo__main.json", "", "", "", false},
		{"legacy single-_ rejected", "Result_org_repo_main.json", "", "", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			org, repo, branch, ok := ParseResultFileName(tt.input)
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			if !ok {
				return
			}
			if org != tt.wantOrg || repo != tt.wantRepo || branch != tt.wantBranch {
				t.Errorf("got (%q, %q, %q), want (%q, %q, %q)",
					org, repo, branch, tt.wantOrg, tt.wantRepo, tt.wantBranch)
			}
		})
	}
}
