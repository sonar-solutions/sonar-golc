package analyzer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The expectations here are SonarJS's, from
// packages/analysis/src/common/filter/filter-minified.ts: a minified name, OR a .js/.css
// file whose average line length exceeds 200.
func TestLooksMinified(t *testing.T) {
	dir := t.TempDir()

	shortLines := strings.Repeat("const a = 1;\n", 50)             // ~12 chars per line
	longLines := strings.Repeat(strings.Repeat("x", 400)+"\n", 20) // 400 chars per line
	oneHugeLine := strings.Repeat("y", 5000)                       // no newline at all

	cases := []struct {
		file    string
		content string
		want    bool
		why     string
	}{
		{"app.min.js", shortLines, true, "minified name wins regardless of content"},
		{"app-min.js", shortLines, true, "the hyphen spelling is minified too"},
		{"app.min.css", shortLines, true, "same for css"},
		{"app-min.css", shortLines, true, "same for css, hyphen spelling"},
		{"APP.MIN.JS", shortLines, true, "suffix matching is case-insensitive"},
		{"bundle.js", longLines, true, "long average line length, no telltale name"},
		{"bundle.css", longLines, true, "css is content-checked as well"},
		{"vendor.js", oneHugeLine, true, "a single enormous line is the classic bundle"},
		{"app.js", shortLines, false, "ordinary source"},
		{"styles.css", shortLines, false, "ordinary stylesheet"},
		{"generated.ts", longLines, false, "only .js and .css are content-checked"},
		{"data.json", longLines, false, "json is not content-checked either"},
	}

	for _, c := range cases {
		t.Run(c.file, func(t *testing.T) {
			path := filepath.Join(dir, c.file)
			if err := os.WriteFile(path, []byte(c.content), 0o644); err != nil {
				t.Fatal(err)
			}
			if got := looksMinified(path); got != c.want {
				t.Errorf("looksMinified(%s) = %v, want %v (%s)", c.file, got, c.want, c.why)
			}
		})
	}
}

// A file that cannot be read must not be dropped: losing its lines to an I/O error would
// be worse than counting them.
func TestLooksMinifiedKeepsUnreadableFiles(t *testing.T) {
	if looksMinified(filepath.Join(t.TempDir(), "missing.js")) {
		t.Error("an unreadable .js file should not be treated as minified")
	}
}

// The threshold is a boundary, so check both sides of it rather than only the extremes.
func TestAverageLineLengthBoundary(t *testing.T) {
	dir := t.TempDir()

	for _, c := range []struct {
		name     string
		lineLen  int
		minified bool
	}{
		{"just-under.js", averageLineLengthThreshold - 1, false},
		{"exactly-at.js", averageLineLengthThreshold, false}, // strictly greater than
		{"just-over.js", averageLineLengthThreshold + 1, true},
	} {
		t.Run(c.name, func(t *testing.T) {
			path := filepath.Join(dir, c.name)
			content := strings.Repeat(strings.Repeat("z", c.lineLen)+"\n", 10)
			if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
				t.Fatal(err)
			}
			if got := looksMinified(path); got != c.minified {
				t.Errorf("line length %d: got %v, want %v", c.lineLen, got, c.minified)
			}
		})
	}
}

// A trailing newline must not be counted as an extra empty line, which would drag the
// average down. SonarJS pops a trailing empty element for the same reason.
func TestAverageLineLengthIgnoresTrailingNewline(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "f.js")
	if err := os.WriteFile(path, []byte("abcd\nefgh\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, ok := averageLineLength(path)
	if !ok {
		t.Fatal("expected the file to be readable")
	}
	if got != 4 {
		t.Errorf("average = %v, want 4 (two 4-character lines, not three lines)", got)
	}
}
