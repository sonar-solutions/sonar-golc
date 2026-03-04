package goloc

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/SonarSource-Demos/sonar-golc/pkg/analyzer"
)

func TestFindGitRoot(t *testing.T) {
	tmp := t.TempDir()

	// Not in a git repo -> empty string
	if got := findGitRoot(tmp); got != "" {
		t.Errorf("findGitRoot(%q) = %q, want \"\" (not in repo)", tmp, got)
	}

	// Create .git at root
	gitDir := filepath.Join(tmp, ".git")
	if err := os.MkdirAll(gitDir, 0755); err != nil {
		t.Fatalf("mkdir .git: %v", err)
	}

	// From repo root -> returns root
	abs, _ := filepath.Abs(tmp)
	if got := findGitRoot(tmp); got != abs {
		t.Errorf("findGitRoot(repo root) = %q, want %q", got, abs)
	}

	// From subdir -> returns root
	sub := filepath.Join(tmp, "a", "b")
	if err := os.MkdirAll(sub, 0755); err != nil {
		t.Fatalf("mkdir sub: %v", err)
	}
	if got := findGitRoot(sub); got != abs {
		t.Errorf("findGitRoot(subdir) = %q, want %q", got, abs)
	}

	// From a file inside repo -> returns root
	foo := filepath.Join(tmp, "file.go")
	if err := os.WriteFile(foo, []byte("package main"), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if got := findGitRoot(foo); got != abs {
		t.Errorf("findGitRoot(file) = %q, want %q", got, abs)
	}
}

func TestFindGitRoot_OutsideRepo(t *testing.T) {
	// Path that does not exist or is outside any repo
	got := findGitRoot("/nonexistent/path/outside/repo")
	if got != "" {
		t.Errorf("findGitRoot(nonexistent) = %q, want \"\"", got)
	}
}

func TestBuildGitignoreFunc_NotInRepo(t *testing.T) {
	tmp := t.TempDir()
	// No .git -> buildGitignoreFunc returns nil
	fn := buildGitignoreFunc(tmp)
	if fn != nil {
		t.Error("buildGitignoreFunc(path not in repo) should return nil")
	}
}

func TestBuildGitignoreFunc_RespectsIgnoreRules(t *testing.T) {
	tmp := t.TempDir()

	// Create .git (required for git repo)
	if err := os.MkdirAll(filepath.Join(tmp, ".git"), 0755); err != nil {
		t.Fatalf("mkdir .git: %v", err)
	}
	// .gitignore: ignore vendor/ and *.log
	gitignoreContent := "vendor/\n*.log\n"
	if err := os.WriteFile(filepath.Join(tmp, ".gitignore"), []byte(gitignoreContent), 0644); err != nil {
		t.Fatalf("write .gitignore: %v", err)
	}

	// Create files: included and ignored
	files := map[string]string{
		"main.go":       "package main",
		"other.go":      "package other",
		"debug.log":     "log content",
		"vendor/foo.go": "package vendor",
	}
	for path, body := range files {
		full := filepath.Join(tmp, path)
		if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
			t.Fatalf("mkdir for %s: %v", path, err)
		}
		if err := os.WriteFile(full, []byte(body), 0644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}

	ignoreFunc := buildGitignoreFunc(tmp)
	if ignoreFunc == nil {
		t.Fatal("buildGitignoreFunc(repo) should not return nil")
	}

	// Build analyzer with .go supported; no other exclusions
	extensions := map[string]string{".go": "Go"}
	a := analyzer.NewAnalyzer(tmp, nil, nil, nil, extensions)
	a.SetIgnoreFunc(ignoreFunc)

	got, err := a.MatchingFiles()
	if err != nil {
		t.Fatalf("MatchingFiles: %v", err)
	}

	// We expect only main.go and other.go (vendor/foo.go and debug.log are ignored)
	if len(got) != 2 {
		t.Errorf("MatchingFiles returned %d files, want 2; got: %v", len(got), filePaths(got))
	}
	paths := filePaths(got)
	hasMain := false
	hasOther := false
	for _, p := range paths {
		if filepath.Base(p) == "main.go" {
			hasMain = true
		}
		if filepath.Base(p) == "other.go" {
			hasOther = true
		}
	}
	if !hasMain || !hasOther {
		t.Errorf("expected main.go and other.go in %v", paths)
	}
	for _, p := range paths {
		if filepath.Base(p) == "foo.go" || filepath.Base(p) == "debug.log" {
			t.Errorf("ignored file should not appear: %s", p)
		}
	}
}

func TestBuildGitignoreFunc_RespectsNestedGitignore(t *testing.T) {
	tmp := t.TempDir()

	if err := os.MkdirAll(filepath.Join(tmp, ".git"), 0755); err != nil {
		t.Fatalf("mkdir .git: %v", err)
	}
	// Root .gitignore ignores "secret/"
	if err := os.WriteFile(filepath.Join(tmp, ".gitignore"), []byte("secret/\n"), 0644); err != nil {
		t.Fatalf("write .gitignore: %v", err)
	}

	// secret/ignored.go and public/visible.go
	for _, dir := range []string{"secret", "public"} {
		if err := os.MkdirAll(filepath.Join(tmp, dir), 0755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}
	if err := os.WriteFile(filepath.Join(tmp, "secret", "ignored.go"), []byte("package secret"), 0644); err != nil {
		t.Fatalf("write secret file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmp, "public", "visible.go"), []byte("package public"), 0644); err != nil {
		t.Fatalf("write public file: %v", err)
	}

	ignoreFunc := buildGitignoreFunc(tmp)
	if ignoreFunc == nil {
		t.Fatal("buildGitignoreFunc should not return nil")
	}
	extensions := map[string]string{".go": "Go"}
	a := analyzer.NewAnalyzer(tmp, nil, nil, nil, extensions)
	a.SetIgnoreFunc(ignoreFunc)

	got, err := a.MatchingFiles()
	if err != nil {
		t.Fatalf("MatchingFiles: %v", err)
	}
	if len(got) != 1 {
		t.Errorf("MatchingFiles returned %d files, want 1 (only public/visible.go); got: %v", len(got), filePaths(got))
	}
	if len(got) > 0 && filepath.Base(got[0].FilePath) != "visible.go" {
		t.Errorf("expected visible.go, got %s", got[0].FilePath)
	}
}

// TestBuildGitignoreFunc_PrefixEdgeCase ensures that a path that only shares a prefix with the repo root
// (e.g. repo at .../proj, path .../project/file.go) is not considered under the repo.
func TestBuildGitignoreFunc_PrefixEdgeCase(t *testing.T) {
	tmp := t.TempDir()
	// Repo root at .../proj
	repoRoot := filepath.Join(tmp, "proj")
	if err := os.MkdirAll(filepath.Join(repoRoot, ".git"), 0755); err != nil {
		t.Fatalf("mkdir .git: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoRoot, ".gitignore"), []byte("*.log\n"), 0644); err != nil {
		t.Fatalf("write .gitignore: %v", err)
	}
	// Sibling directory "project" (not under "proj")
	projectDir := filepath.Join(tmp, "project")
	if err := os.MkdirAll(projectDir, 0755); err != nil {
		t.Fatalf("mkdir project: %v", err)
	}
	fileInProject := filepath.Join(projectDir, "file.go")
	if err := os.WriteFile(fileInProject, []byte("package main"), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	ignoreFunc := buildGitignoreFunc(repoRoot)
	if ignoreFunc == nil {
		t.Fatal("buildGitignoreFunc should not return nil")
	}

	// Path .../project/file.go must not be treated as under repo .../proj (no separator after "proj")
	ignored := ignoreFunc(fileInProject, false)
	if ignored {
		t.Errorf("path %q (under project/) should not be considered under repo %q; ignoreFunc should return false", fileInProject, repoRoot)
	}
}

func TestMatchingFiles_WithoutIgnoreFunc_IncludesAllSupported(t *testing.T) {
	tmp := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tmp, ".git"), 0755); err != nil {
		t.Fatalf("mkdir .git: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmp, ".gitignore"), []byte("*.go\n"), 0644); err != nil {
		t.Fatalf("write .gitignore: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmp, "main.go"), []byte("package main"), 0644); err != nil {
		t.Fatalf("write main.go: %v", err)
	}

	// Analyzer WITHOUT SetIgnoreFunc -> .gitignore is not applied, file is included
	extensions := map[string]string{".go": "Go"}
	a := analyzer.NewAnalyzer(tmp, nil, nil, nil, extensions)
	got, err := a.MatchingFiles()
	if err != nil {
		t.Fatalf("MatchingFiles: %v", err)
	}
	if len(got) != 1 {
		t.Errorf("without ignore func expected 1 file, got %d", len(got))
	}
}

func filePaths(files []analyzer.FileMetadata) []string {
	out := make([]string, len(files))
	for i, f := range files {
		out[i] = f.FilePath
	}
	return out
}
