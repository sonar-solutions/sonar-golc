package analyzer

import (
	"os"
	"path/filepath"
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
