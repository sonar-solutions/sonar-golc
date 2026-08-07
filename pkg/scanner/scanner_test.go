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

// A PHP file's opening and closing tags are markup, not code, and SonarQube does not
// count a line holding only one of them towards ncloc. The expectations below were
// measured against SonarQube Enterprise 2026.4 on exactly these three files: each
// reported ncloc=2, and the third reported one comment line.
func TestScanDoesNotCountLoneMarkupDelimiters(t *testing.T) {
	dir := t.TempDir()

	cases := []struct {
		name                         string
		content                      string
		code, comments, blank, lines int
	}{
		{
			name:    "open and close tags alone are not code",
			content: "<?php\n$a = 1;\n$b = 2;\n?>\n",
			code:    2, comments: 0, blank: 0, lines: 2,
		},
		{
			name:    "a tag sharing a line with code is still code",
			content: "<?php $c = 3;\n$d = 4;\n",
			code:    2, comments: 0, blank: 0, lines: 2,
		},
		{
			name:    "comments and blanks are unaffected",
			content: "<?php\n// a comment\n$e = 5;\n\n$f = 6;\n",
			code:    2, comments: 1, blank: 1, lines: 4,
		},
	}

	sc := NewScanner(language.Languages{
		"PHP": {
			LineComments:      []string{"//", "#"},
			MultiLineComments: [][]string{{"/*", "*/"}},
			Extensions:        []string{".php"},
			NonCodeLines:      []string{"<?php", "<?", "?>"},
		},
	})

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			path := filepath.Join(dir, "f.php")
			if err := os.WriteFile(path, []byte(c.content), 0644); err != nil {
				t.Fatal(err)
			}

			got, err := sc.scanFile(analyzer.FileMetadata{
				FilePath: path, Extension: ".php", Language: "PHP",
			})
			if err != nil {
				t.Fatal(err)
			}
			if got.CodeLines != c.code {
				t.Errorf("CodeLines = %d, want %d", got.CodeLines, c.code)
			}
			if got.Comments != c.comments {
				t.Errorf("Comments = %d, want %d", got.Comments, c.comments)
			}
			if got.BlankLines != c.blank {
				t.Errorf("BlankLines = %d, want %d", got.BlankLines, c.blank)
			}
			if got.Lines != c.lines {
				t.Errorf("Lines = %d, want %d", got.Lines, c.lines)
			}
		})
	}
}

// A language without NonCodeLines must be untouched by the delimiter check.
func TestScanCountsDelimiterLikeLinesForOtherLanguages(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "f.txt")
	if err := os.WriteFile(path, []byte("<?php\nplain\n"), 0644); err != nil {
		t.Fatal(err)
	}

	sc := NewScanner(language.Languages{
		"Plain": {Extensions: []string{".txt"}},
	})

	got, err := sc.scanFile(analyzer.FileMetadata{
		FilePath: path, Extension: ".txt", Language: "Plain",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.CodeLines != 2 {
		t.Errorf("CodeLines = %d, want 2 (no NonCodeLines configured)", got.CodeLines)
	}
}

// A file that does not end in a newline still has a final line. ReadString returns that
// chunk together with io.EOF, and breaking on the error used to discard it, losing one
// line from every such file.
func TestScanCountsFinalLineWithoutTrailingNewline(t *testing.T) {
	dir := t.TempDir()

	sc := NewScanner(language.Languages{
		"Go": {
			LineComments:      []string{"//"},
			MultiLineComments: [][]string{{"/*", "*/"}},
			Extensions:        []string{".go"},
		},
	})

	cases := []struct {
		name                         string
		content                      string
		code, comments, blank, lines int
	}{
		{
			name:    "code on the final line, no newline",
			content: "a := 1\nb := 2",
			code:    2, lines: 2,
		},
		{
			name:    "same file with a trailing newline counts the same",
			content: "a := 1\nb := 2\n",
			code:    2, lines: 2,
		},
		{
			name:    "a comment on the final line, no newline",
			content: "a := 1\n// trailing note",
			code:    1, comments: 1, lines: 2,
		},
		{
			name:    "whitespace-only final line is blank, not code",
			content: "a := 1\n   ",
			code:    1, blank: 1, lines: 2,
		},
		{
			name:    "a single line with no newline at all",
			content: "only := 1",
			code:    1, lines: 1,
		},
		{
			name:    "empty file",
			content: "",
			lines:   0,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			path := filepath.Join(dir, "f.go")
			if err := os.WriteFile(path, []byte(c.content), 0644); err != nil {
				t.Fatal(err)
			}

			got, err := sc.scanFile(analyzer.FileMetadata{
				FilePath: path, Extension: ".go", Language: "Go",
			})
			if err != nil {
				t.Fatal(err)
			}
			if got.CodeLines != c.code {
				t.Errorf("CodeLines = %d, want %d", got.CodeLines, c.code)
			}
			if got.Comments != c.comments {
				t.Errorf("Comments = %d, want %d", got.Comments, c.comments)
			}
			if got.BlankLines != c.blank {
				t.Errorf("BlankLines = %d, want %d", got.BlankLines, c.blank)
			}
			if got.Lines != c.lines {
				t.Errorf("Lines = %d, want %d", got.Lines, c.lines)
			}
		})
	}
}

// An unterminated block comment that runs to the end of the file must still have its final
// line counted as a comment.
func TestScanCountsFinalLineInsideBlockComment(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "f.go")
	if err := os.WriteFile(path, []byte("a := 1\n/* open\nstill open"), 0644); err != nil {
		t.Fatal(err)
	}

	sc := NewScanner(language.Languages{
		"Go": {
			LineComments:      []string{"//"},
			MultiLineComments: [][]string{{"/*", "*/"}},
			Extensions:        []string{".go"},
		},
	})

	got, err := sc.scanFile(analyzer.FileMetadata{
		FilePath: path, Extension: ".go", Language: "Go",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.CodeLines != 1 || got.Comments != 2 {
		t.Errorf("got code=%d comments=%d, want code=1 comments=2",
			got.CodeLines, got.Comments)
	}
}
