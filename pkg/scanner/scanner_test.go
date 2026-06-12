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

// TestScanFailsWhenAllFilesUnreadable covers the degenerate case where every
// candidate file fails to open (e.g. wrong path, tree-wide permission denial,
// shallow clone missing every symlink target). Scan must return an error so the
// downstream report does not silently report a zero-line repository, which
// would hide a systemic problem behind a "genuinely empty repo" appearance.
func TestScanFailsWhenAllFilesUnreadable(t *testing.T) {
	dir := t.TempDir()

	brokenLink1 := filepath.Join(dir, "compile_commands.json")
	if err := os.Symlink(filepath.Join(dir, "build", "compile_commands.json"), brokenLink1); err != nil {
		t.Skipf("symlink unsupported on this platform: %v", err)
	}
	brokenLink2 := filepath.Join(dir, "generated.go")
	if err := os.Symlink(filepath.Join(dir, "build", "generated.go"), brokenLink2); err != nil {
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
		{FilePath: brokenLink1, Extension: ".json", Language: "JSON"},
		{FilePath: brokenLink2, Extension: ".go", Language: "Go"},
	}

	results, err := sc.Scan(files)
	if err == nil {
		t.Fatalf("Scan returned nil error with all files unreadable, expected an error")
	}
	if len(results) != 0 {
		t.Fatalf("expected 0 results when every file fails, got %d", len(results))
	}
}

// TestScanEmptyFileListReturnsNoError ensures that a genuinely empty repository
// (zero candidate files) is not conflated with a fully-failed scan. Both produce
// an empty result slice, but only the latter must return an error.
func TestScanEmptyFileListReturnsNoError(t *testing.T) {
	languages := language.Languages{
		"Go": language.LanguageInfo{
			LineComments: []string{"//"},
			Extensions:   []string{".go"},
		},
	}

	sc := NewScanner(languages)
	results, err := sc.Scan(nil)
	if err != nil {
		t.Fatalf("Scan with no files returned error %v, expected nil", err)
	}
	if len(results) != 0 {
		t.Fatalf("expected 0 results for empty input, got %d", len(results))
	}
}
