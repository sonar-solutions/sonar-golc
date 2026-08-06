package analyzer

import (
	"bufio"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Infrastructure-as-code dialects of YAML and JSON cannot be told apart by extension -
// a Kubernetes manifest, an Ansible playbook and an arbitrary settings file are all
// ".yaml". SonarQube distinguishes them by content, and the distinction matters for the
// line count it produces: every IaC analyzer is on by default, while plain YAML and JSON
// analysis are off (sonar.yaml.activate and sonar.json.activate both default to false).
// Reporting every .yaml under one label therefore cannot match SonarQube whichever way
// it is counted, so the dialect is recognised here and reported as its own language.
//
// Detection reads only the head of a file and never fails the analysis: an unreadable or
// unrecognised file keeps the language its extension gave it.

// headLines and headBytes cap how much of a file is read; IaC markers appear in the first
// few keys. The byte cap is the one that matters: a minified single-line JSON has no
// newline to stop at, so a line limit alone would pull the whole file - possibly many
// megabytes - into memory on the first read.
const (
	headLines = 60
	headBytes = 64 * 1024
)

var (
	// A Kubernetes manifest has both of these at the top level.
	reK8sAPIVersion = regexp.MustCompile(`(?m)^apiVersion:\s*\S`)
	reK8sKind       = regexp.MustCompile(`(?m)^kind:\s*\S`)

	// CloudFormation: the version key, or a resource with an AWS:: type. A template may
	// be YAML or JSON, so both patterns have to tolerate the JSON spelling - an indented,
	// quoted key ("  \"AWSTemplateFormatVersion\":") and a quoted value ("\"Type\": \"AWS::").
	// Anchoring at column 0 or requiring a colon straight after Type matches YAML only.
	reCFNVersion  = regexp.MustCompile(`(?m)^\s*["']?AWSTemplateFormatVersion["']?\s*:`)
	reCFNResource = regexp.MustCompile(`["']?Type["']?\s*:\s*["']?AWS::`)

	// An Ansible playbook is a list of plays keyed by hosts, or a task file.
	reAnsibleHosts = regexp.MustCompile(`(?m)^\s*-?\s*hosts:\s*\S`)
	reAnsibleTasks = regexp.MustCompile(`(?m)^\s*(tasks|roles|become|gather_facts):`)

	// Azure Pipelines: stages/jobs/steps combined with a pool or trigger.
	reAzurePipeline = regexp.MustCompile(`(?m)^(stages|jobs|steps):`)
	reAzurePool     = regexp.MustCompile(`(?m)^(pool|trigger|pr|variables|resources):`)

	// ARM templates are JSON whose $schema names a deployment template.
	reARMSchema = regexp.MustCompile(`"\$schema"\s*:\s*"[^"]*deploymentTemplate`)
)

// RefineLanguage returns the language to report for a file. It only ever refines YAML and
// JSON; every other language is returned untouched without the file being opened.
func RefineLanguage(path, language string) string {
	switch language {
	case "YAML":
		return refineYAML(path)
	case "JSON":
		return refineJSON(path)
	default:
		return language
	}
}

func refineYAML(path string) string {
	// GitHub Actions is defined by location rather than content: any workflow file under
	// .github/workflows is one, whatever keys it happens to use.
	if inGitHubWorkflows(path) {
		return "GitHub Actions"
	}

	head, ok := readHead(path)
	if !ok {
		return "YAML"
	}

	switch {
	case reK8sAPIVersion.MatchString(head) && reK8sKind.MatchString(head):
		return "Kubernetes"
	case reCFNVersion.MatchString(head) || reCFNResource.MatchString(head):
		return "CloudFormation"
	case reAnsibleHosts.MatchString(head) && reAnsibleTasks.MatchString(head):
		return "Ansible"
	case reAzurePipeline.MatchString(head) && reAzurePool.MatchString(head):
		return "Azure Pipelines"
	default:
		return "YAML"
	}
}

func refineJSON(path string) string {
	head, ok := readHead(path)
	if !ok {
		return "JSON"
	}

	switch {
	case reARMSchema.MatchString(head):
		return "Azure Resource Manager"
	case reCFNVersion.MatchString(head) || reCFNResource.MatchString(head):
		return "CloudFormation"
	default:
		return "JSON"
	}
}

// inGitHubWorkflows reports whether the path sits in a .github/workflows directory.
func inGitHubWorkflows(path string) bool {
	dir := filepath.ToSlash(filepath.Dir(path))

	return strings.HasSuffix(dir, "/.github/workflows") || dir == ".github/workflows" ||
		strings.Contains(dir, "/.github/workflows/")
}

// readHead returns the first headLines lines of a file. The bool is false when the file
// cannot be read, so the caller can fall back to the extension-derived language rather
// than treat an I/O problem as a detection result.
func readHead(path string) (string, bool) {
	f, err := os.Open(path)
	if err != nil {
		return "", false
	}
	defer f.Close()

	// io.LimitReader bounds the total bytes regardless of where newlines fall, so a file
	// with no newline at all cannot be read past headBytes.
	var b strings.Builder
	reader := bufio.NewReader(io.LimitReader(f, headBytes))
	for i := 0; i < headLines; i++ {
		line, err := reader.ReadString('\n')
		b.WriteString(line)
		if err != nil {
			break
		}
	}

	return b.String(), true
}
