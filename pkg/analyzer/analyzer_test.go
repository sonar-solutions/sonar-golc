package analyzer

import (
	"path/filepath"
	"testing"
)

func TestFolderSegmentContainsKeyword(t *testing.T) {
	tests := []struct {
		segment string
		kw      string
		want    bool
	}{
		// Exact match
		{"test", "test", true},
		{"vendor", "vendor", true},
		// Delimiter-split matches
		{"integration-test", "test", true},
		{"test-helpers", "test", true},
		{"my_test_suite", "test", true},
		{"my.test.helpers", "test", true},
		// Multiple delimiters
		{"my.test_helpers", "test", true},
		// Case insensitivity
		{"Integration-Test", "test", true},
		{"TEST", "test", true},
		// Prefix substring — must NOT match
		{"protest", "test", false},
		{"testing", "test", false},
		// Suffix substring — must NOT match
		{"latest", "test", false},
		{"contest", "test", false},
		// Unrelated segments
		{"src", "test", false},
		{"main", "vendor", false},
		// Generated keyword
		{"generated-client", "generated", true},
		{"generated_code", "generated", true},
		{"ungenerated", "generated", false},
	}

	for _, tc := range tests {
		got := folderSegmentContainsKeyword(tc.segment, tc.kw)
		if got != tc.want {
			t.Errorf("folderSegmentContainsKeyword(%q, %q) = %v, want %v", tc.segment, tc.kw, got, tc.want)
		}
	}
}

func TestCanAdd_FolderKeywords(t *testing.T) {
	extensions := map[string]string{".go": "Golang"}
	a := NewAnalyzer(
		"/repo",
		nil,
		nil,
		nil,
		extensions,
		[]string{"test", "vendor"},
		nil,
	)

	tests := []struct {
		path string
		want bool
	}{
		// Keyword matches at any depth
		{filepath.Join("/repo", "test", "foo.go"), false},
		{filepath.Join("/repo", "src", "integration-test", "foo.go"), false},
		{filepath.Join("/repo", "src", "test_helpers", "foo.go"), false},
		{filepath.Join("/repo", "vendor", "lib", "foo.go"), false},
		// Non-matching paths
		{filepath.Join("/repo", "src", "main.go"), true},
		{filepath.Join("/repo", "protest", "foo.go"), true},  // substring, not whole word
		{filepath.Join("/repo", "latest", "foo.go"), true},   // substring, not whole word
	}

	for _, tc := range tests {
		got := a.canAdd(tc.path, ".go")
		if got != tc.want {
			t.Errorf("canAdd(%q) = %v, want %v", tc.path, got, tc.want)
		}
	}
}

func TestCanAdd_FileNamePatterns(t *testing.T) {
	extensions := map[string]string{".go": "Golang", ".js": "JavaScript"}
	a := NewAnalyzer(
		"/repo",
		nil,
		nil,
		nil,
		extensions,
		nil,
		[]string{"*_test.go", "*.min.js"},
	)

	tests := []struct {
		path string
		ext  string
		want bool
	}{
		{filepath.Join("/repo", "src", "foo_test.go"), ".go", false},
		{filepath.Join("/repo", "src", "bundle.min.js"), ".js", false},
		{filepath.Join("/repo", "src", "main.go"), ".go", true},
		{filepath.Join("/repo", "src", "app.js"), ".js", true},
	}

	for _, tc := range tests {
		got := a.canAdd(tc.path, tc.ext)
		if got != tc.want {
			t.Errorf("canAdd(%q) = %v, want %v", tc.path, got, tc.want)
		}
	}
}
