package scanner

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/SonarSource-Demos/sonar-golc/pkg/analyzer"
	"github.com/SonarSource-Demos/sonar-golc/pkg/goloc/language"
)

// TestScanSkipsUnreadableFiles covers the original bug where a single unreadable
// file (broken symlink, missing path, permission denied) would abort the entire
// repository scan and lose every other file's result. The expected behaviour is
// that the bad file is skipped and the rest of the scan succeeds.
func TestScanSkipsUnreadableFiles(t *testing.T) {
	dir := t.TempDir()

	goodPath := filepath.Join(dir, "good.go")
	if err := os.WriteFile(goodPath, []byte("package main\nfunc f() {}\n"), 0644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	// Broken symlink: points at a path that does not exist. os.Open follows it
	// and returns "no such file or directory", the same error users saw on
	// compile_commands.json links pointing into an absent build/ tree.
	brokenLink := filepath.Join(dir, "compile_commands.json")
	if err := os.Symlink(filepath.Join(dir, "build", "compile_commands.json"), brokenLink); err != nil {
		t.Skipf("symlink unsupported on this platform: %v", err)
	}

	languages := language.Languages{
		"Go": language.LanguageInfo{
			LineComments:      []string{"//"},
			MultiLineComments: [][]string{{"/*", "*/"}},
			Extensions:        []string{".go"},
		},
		"JSON": language.LanguageInfo{
			LineComments:      []string{},
			MultiLineComments: [][]string{},
			Extensions:        []string{".json"},
		},
	}

	sc := NewScanner(languages)
	files := []analyzer.FileMetadata{
		{FilePath: brokenLink, Extension: ".json", Language: "JSON"},
		{FilePath: goodPath, Extension: ".go", Language: "Go"},
	}

	results, err := sc.Scan(files)
	if err != nil {
		t.Fatalf("Scan returned error %v, expected nil (bad file should be skipped)", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 successful scan result (good.go), got %d", len(results))
	}
	if results[0].Metadata.FilePath != goodPath {
		t.Fatalf("expected result for %s, got %s", goodPath, results[0].Metadata.FilePath)
	}
}
