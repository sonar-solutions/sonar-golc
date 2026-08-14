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

		// camelCase is deliberately NOT split. It is tempting, because it would let the
		// existing 'tests' keyword reach .NET's 'Foo.UnitTests' - the '.' split only
		// yields the single token 'unittests'. But the same split would make every
		// segment below match a build-output keyword and silently drop production code,
		// and measured over 2.0M lines of SonarQube ncloc the .NET keywords cost more
		// than they recovered (see the note on PRESET_TEST_KEYWORDS in golc-launcher.go).
		{"Core.UnitTests", "tests", false},
		{"Core.UnitTests", "test", false},
		{"Core.Tests", "tests", true},
		{"LatestBuild", "build", false},
		{"BinTree", "bin", false},
		{"OutputFormatter", "out", false},
		// 'Integration.Vsix' is production code in a .NET solution, and the '.' split
		// means the shipped 'integration' keyword does reach it. Asserted so the cost is
		// visible in the test suite rather than discovered in a customer's numbers.
		{"Integration.Vsix", "integration", true},
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
		{filepath.Join("/repo", "protest", "foo.go"), true}, // substring, not whole word
		{filepath.Join("/repo", "latest", "foo.go"), true},  // substring, not whole word
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

// The generated-code patterns the UI ships by default. GoLC has no build model, so a
// generator's output is recognised by name or not at all; these are the conventions that
// only a generator produces, so a hand-written file cannot collide with them.
func TestCanAdd_GeneratedCodeDefaultPatterns(t *testing.T) {
	extensions := map[string]string{
		".cs": "C#", ".vb": "VB.NET", ".go": "Golang", ".py": "Python", ".cc": "C++", ".h": "C Header",
	}
	a := NewAnalyzer(
		"/repo",
		nil,
		nil,
		nil,
		extensions,
		nil,
		[]string{"*.Designer.*", "*.g.cs", "*.generated.*", "*.pb.go", "*_pb2.py", "*.pb.cc", "*.pb.h"},
	)

	tests := []struct {
		name string
		path string
		ext  string
		want bool
	}{
		{"resx designer", filepath.Join("/repo", "src", "Resources.Designer.cs"), ".cs", false},
		{"VB designer", filepath.Join("/repo", "src", "Settings.Designer.vb"), ".vb", false},
		{"XAML codegen", filepath.Join("/repo", "src", "MainWindow.g.cs"), ".cs", false},
		{"generated convention", filepath.Join("/repo", "src", "Client.generated.cs"), ".cs", false},
		{"protobuf Go", filepath.Join("/repo", "api", "service.pb.go"), ".go", false},
		{"protobuf Python", filepath.Join("/repo", "api", "service_pb2.py"), ".py", false},
		{"protobuf C++ source", filepath.Join("/repo", "api", "service.pb.cc"), ".cc", false},
		{"protobuf C++ header", filepath.Join("/repo", "api", "service.pb.h"), ".h", false},

		// Hand-written files that must survive. "Designer" and "generator" as ordinary
		// words, and a Go file whose name merely ends in "pb".
		{"a class about designers", filepath.Join("/repo", "src", "DesignerService.cs"), ".cs", true},
		{"a class about generation", filepath.Join("/repo", "src", "CodeGenerator.cs"), ".cs", true},
		{"not a protobuf output", filepath.Join("/repo", "src", "pb.go"), ".go", true},
		{"ordinary source", filepath.Join("/repo", "src", "Program.cs"), ".cs", true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := a.canAdd(tc.path, tc.ext); got != tc.want {
				t.Errorf("canAdd(%q) = %v, want %v", tc.path, got, tc.want)
			}
		})
	}
}
