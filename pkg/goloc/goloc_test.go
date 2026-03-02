package goloc

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/SonarSource-Demos/sonar-golc/assets"
)

func TestFileIdentification(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "goloc_file_identification_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create test files with various detection patterns
	testFiles := []struct {
		path     string
		content  string
		expected string
	}{
		// Extension-based detection
		{"main.go", "package main\nfunc main() {}", "Golang"},
		{"lib.rs", "fn main() {}", "Rust"},
		{"script.sh", "#!/bin/bash\necho hello", "Bash"},
		{"config.yaml", "key: value", "YAML"},
		{"style.css", ".foo { color: red; }", "CSS"},

		// Filename-based detection
		{"playbook.yml", "---\n- hosts: all", "Ansible"},
		{"playbook.yaml", "---\n- hosts: all", "Ansible"},
		{"site.yml", "---\n- hosts: localhost", "Ansible"},
		{"Dockerfile", "FROM alpine\nRUN echo test", "Dockerfile"},
		{"azuredeploy.json", "{\"content\": \"template\"}", "ARM Template"},
		{"mainTemplate.json", "{\"content\": \"template\"}", "ARM Template"},
		{"workflow.yml", "name: workflow\non: push", "GitHub Actions"},
		{"workflow.yaml", "name: workflow\non: push", "GitHub Actions"},
		{"values.yaml", "replicaCount: 1", "Helm Chart"},
		{"Chart.yaml", "name: mychart\nversion: 1.0", "Helm Chart"},

		// Path pattern detection (.github/workflows/)
		{".github/workflows/ci.yml", "name: CI\non: push", "GitHub Actions"},
		{".github/workflows/deploy.yaml", "name: Deploy\non: push", "GitHub Actions"},

		// Path pattern detection (charts/)
		{"charts/values.yaml", "replicaCount: 2", "Helm Chart"},
		{"helm/mychart/charts/config.yml", "key: value", "Helm Chart"},
	}

	for _, tf := range testFiles {
		fullPath := filepath.Join(tempDir, tf.path)
		if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
			t.Fatalf("Failed to create dir for %s: %v", tf.path, err)
		}
		if err := os.WriteFile(fullPath, []byte(tf.content), 0644); err != nil {
			t.Fatalf("Failed to create file %s: %v", tf.path, err)
		}
	}

	// Create analyzer with real language definitions
	params := Params{}
	a, _, _ := initAnalyzerScannerReporters(tempDir, params, nil, assets.Languages)

	files, err := a.MatchingFiles()
	if err != nil {
		t.Fatalf("MatchingFiles failed: %v", err)
	}

	// Build map of relative path -> language
	got := make(map[string]string)
	for _, f := range files {
		rel, err := filepath.Rel(tempDir, f.FilePath)
		if err != nil {
			rel = f.FilePath
		}
		// Normalize path separators for Windows
		rel = filepath.ToSlash(rel)
		got[rel] = f.Language
	}

	// Verify each file was correctly identified
	for _, tf := range testFiles {
		rel := filepath.ToSlash(tf.path)
		lang, ok := got[rel]
		if !ok {
			t.Errorf("File %s: not found in matching files", tf.path)
			continue
		}
		if lang != tf.expected {
			t.Errorf("File %s: got language %q, want %q", tf.path, lang, tf.expected)
		}
	}
}

func TestFileIdentificationExcludesUnsupportedFiles(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "goloc_exclude_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create mix of supported and unsupported files
	unsupported := []string{"random.txt", "data.bin", "README.md", "config.conf"}
	for _, name := range unsupported {
		path := filepath.Join(tempDir, name)
		if err := os.WriteFile(path, []byte("content"), 0644); err != nil {
			t.Fatalf("Failed to create %s: %v", name, err)
		}
	}

	supported := []string{"main.go", "script.sh"}
	for _, name := range supported {
		path := filepath.Join(tempDir, name)
		if err := os.WriteFile(path, []byte("content"), 0644); err != nil {
			t.Fatalf("Failed to create %s: %v", name, err)
		}
	}

	params := Params{}
	a, _, _ := initAnalyzerScannerReporters(tempDir, params, nil, assets.Languages)

	files, err := a.MatchingFiles()
	if err != nil {
		t.Fatalf("MatchingFiles failed: %v", err)
	}

	// Should only have supported files
	if len(files) != len(supported) {
		t.Errorf("Got %d files, want %d (supported only)", len(files), len(supported))
	}

	for _, f := range files {
		base := filepath.Base(f.FilePath)
		found := false
		for _, s := range supported {
			if base == s {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Unexpected file matched: %s", base)
		}
	}
}

func TestFileIdentificationExcludePaths(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "goloc_exclude_paths_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create files in root and in vendor/
	os.WriteFile(filepath.Join(tempDir, "main.go"), []byte("package main"), 0644)
	os.MkdirAll(filepath.Join(tempDir, "vendor", "pkg"), 0755)
	os.WriteFile(filepath.Join(tempDir, "vendor", "pkg", "helper.go"), []byte("package pkg"), 0644)

	params := Params{}
	excludePaths := []string{filepath.Join(tempDir, "vendor")}
	a, _, _ := initAnalyzerScannerReporters(tempDir, params, excludePaths, assets.Languages)

	files, err := a.MatchingFiles()
	if err != nil {
		t.Fatalf("MatchingFiles failed: %v", err)
	}

	// Should only have main.go, not vendor/pkg/helper.go
	if len(files) != 1 {
		t.Errorf("Got %d files, want 1 (vendor should be excluded)", len(files))
	}
	if len(files) > 0 && filepath.Base(files[0].FilePath) != "main.go" {
		t.Errorf("Expected main.go, got %s", files[0].FilePath)
	}
}
