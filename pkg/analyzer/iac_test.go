package analyzer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	return path
}

func TestRefineLanguageDetectsIaCDialects(t *testing.T) {
	dir := t.TempDir()

	cases := []struct {
		name     string
		file     string
		content  string
		language string
		want     string
	}{
		{
			name:     "kubernetes manifest needs both apiVersion and kind",
			file:     "k8s/deploy.yaml",
			content:  "apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: web\n",
			language: "YAML", want: "Kubernetes",
		},
		{
			name:     "apiVersion alone is not Kubernetes",
			file:     "misc/part.yaml",
			content:  "apiVersion: v1\nsomething: else\n",
			language: "YAML", want: "YAML",
		},
		{
			name:     "cloudformation by template version",
			file:     "infra/stack.yaml",
			content:  "AWSTemplateFormatVersion: '2010-09-09'\nResources:\n  Bucket:\n    Type: AWS::S3::Bucket\n",
			language: "YAML", want: "CloudFormation",
		},
		{
			name:     "cloudformation by AWS resource type alone",
			file:     "infra/res.yaml",
			content:  "Resources:\n  Queue:\n    Type: AWS::SQS::Queue\n",
			language: "YAML", want: "CloudFormation",
		},
		{
			name:     "ansible playbook",
			file:     "playbooks/site.yaml",
			content:  "- hosts: web\n  become: true\n  tasks:\n    - name: install\n",
			language: "YAML", want: "Ansible",
		},
		{
			name:     "azure pipeline",
			file:     "ci/pipeline.yaml",
			content:  "trigger:\n  - main\nstages:\n  - stage: build\n",
			language: "YAML", want: "Azure Pipelines",
		},
		{
			name:     "github actions is detected by location, not content",
			file:     ".github/workflows/ci.yml",
			content:  "name: ci\non: [push]\njobs:\n  build:\n    runs-on: ubuntu-latest\n",
			language: "YAML", want: "GitHub Actions",
		},
		{
			name:     "plain yaml stays yaml",
			file:     "config/app.yaml",
			content:  "server: localhost\nport: 8080\n",
			language: "YAML", want: "YAML",
		},
		{
			name:     "arm template",
			file:     "arm/azuredeploy.json",
			content:  "{\n  \"$schema\": \"https://schema.management.azure.com/schemas/2019-04-01/deploymentTemplate.json#\"\n}\n",
			language: "JSON", want: "Azure Resource Manager",
		},
		{
			name:     "cloudformation JSON by template version",
			file:     "infra/stack.json",
			content:  "{\n  \"AWSTemplateFormatVersion\": \"2010-09-09\",\n  \"Resources\": {}\n}\n",
			language: "JSON", want: "CloudFormation",
		},
		{
			name:     "cloudformation JSON by AWS resource type",
			file:     "infra/res.json",
			content:  "{\n  \"Resources\": {\n    \"Bucket\": { \"Type\": \"AWS::S3::Bucket\" }\n  }\n}\n",
			language: "JSON", want: "CloudFormation",
		},
		{
			name:     "cloudformation YAML still works with the shared patterns",
			file:     "infra/indented.yaml",
			content:  "Resources:\n  Bucket:\n    Type: AWS::S3::Bucket\n",
			language: "YAML", want: "CloudFormation",
		},
		{
			name:     "plain json stays json",
			file:     "data/values.json",
			content:  "{\n  \"a\": 1\n}\n",
			language: "JSON", want: "JSON",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			path := writeFile(t, dir, c.file, c.content)
			if got := RefineLanguage(path, c.language); got != c.want {
				t.Errorf("RefineLanguage(%s) = %q, want %q", c.file, got, c.want)
			}
		})
	}
}

// Refinement must not touch anything else, and must not need the file to exist: opening
// a file for every source line in a repository would be pure cost.
func TestRefineLanguageLeavesOtherLanguagesAlone(t *testing.T) {
	for _, lang := range []string{"Golang", "Java", "Python", "PHP", "Terraform", ""} {
		if got := RefineLanguage("/no/such/file.ext", lang); got != lang {
			t.Errorf("RefineLanguage(%q) = %q, want it unchanged", lang, got)
		}
	}
}

// An unreadable YAML file keeps its extension-derived language rather than being dropped
// or misreported.
func TestRefineLanguageFallsBackWhenUnreadable(t *testing.T) {
	if got := RefineLanguage(filepath.Join(t.TempDir(), "missing.yaml"), "YAML"); got != "YAML" {
		t.Errorf("got %q, want YAML", got)
	}
	if got := RefineLanguage(filepath.Join(t.TempDir(), "missing.json"), "JSON"); got != "JSON" {
		t.Errorf("got %q, want JSON", got)
	}
}

// readHead caps the bytes it reads, not just the lines. A single-line file has no newline
// to stop at, so without a byte budget the whole thing would be pulled into memory - and
// a minified JSON is exactly that shape. Proven behaviourally: a marker pushed past the
// cap must not be found.
func TestRefineLanguageStopsReadingAtTheByteCap(t *testing.T) {
	dir := t.TempDir()

	marker := `"$schema": "https://schema.management.azure.com/deploymentTemplate.json#"`

	within := writeFile(t, dir, "near.json", "{"+marker+"}")
	if got := RefineLanguage(within, "JSON"); got != "Azure Resource Manager" {
		t.Errorf("a marker inside the cap should be found, got %q", got)
	}

	// One line, no newline anywhere, with the marker beyond headBytes.
	padded := writeFile(t, dir, "far.json",
		"{"+strings.Repeat(" ", headBytes+1024)+marker+"}")
	if got := RefineLanguage(padded, "JSON"); got != "JSON" {
		t.Errorf("a marker past the byte cap should not be read, got %q", got)
	}

	if info, err := os.Stat(padded); err == nil && info.Size() <= headBytes {
		t.Fatalf("test file is only %d bytes; it must exceed the %d-byte cap to be meaningful",
			info.Size(), headBytes)
	}
}
