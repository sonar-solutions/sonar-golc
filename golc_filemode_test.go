//go:build engine
// +build engine

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/SonarSource-Demos/sonar-golc/pkg/utils"
)

// Two scanned directories that share a name used to write the same result file, so the
// second replaced the first and its lines disappeared from the totals without a warning.
// Each must now produce its own result, under a name the results page can list.
func TestFileModeKeepsSameNamedDirectoriesApart(t *testing.T) {
	root := t.TempDir()
	write := func(rel, content string) {
		path := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("team-a/app/main.go", "package main\n\nfunc main() {}\n")                 // 2 lines of code
	write("team-b/app/main.go", "package main\n\nfunc main() {\n\tprintln(1)\n}\n") // 4 lines of code

	dest := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dest, "bylanguage-report"), 0o755); err != nil {
		t.Fatal(err)
	}

	dirs := utils.FileModeDirs([]string{
		filepath.Join(root, "team-a", "app"),
		filepath.Join(root, "team-b", "app"),
	})
	AnalyseReposListFile(dirs, analysisOptions{}, dest)
	if err := saveFileAnalysisResult(dest, "org", dirs); err != nil {
		t.Fatalf("saveFileAnalysisResult: %v", err)
	}

	for name, want := range map[string]int{"team-a_app": 2, "team-b_app": 4} {
		data, err := os.ReadFile(filepath.Join(dest, "bylanguage-report", "Result_"+name+".json"))
		if err != nil {
			t.Errorf("no result for %s: %v", name, err)
			continue
		}
		var result struct{ TotalCodeLines int }
		if err := json.Unmarshal(data, &result); err != nil {
			t.Fatalf("decoding %s: %v", name, err)
		}
		if result.TotalCodeLines != want {
			t.Errorf("%s has %d lines of code, want %d", name, result.TotalCodeLines, want)
		}
	}

	// The results page finds each repository's files through these names.
	data, err := os.ReadFile(filepath.Join(dest, "config", "analysis_result_file.json"))
	if err != nil {
		t.Fatal(err)
	}
	var listed fileAnalysisResult
	if err := json.Unmarshal(data, &listed); err != nil {
		t.Fatal(err)
	}
	var slugs []string
	for _, b := range listed.ProjectBranches {
		slugs = append(slugs, b.RepoSlug)
	}
	if len(slugs) != 2 || slugs[0] != "team-a_app" || slugs[1] != "team-b_app" {
		t.Errorf("listed repositories = %v, want [team-a_app team-b_app]", slugs)
	}
}
