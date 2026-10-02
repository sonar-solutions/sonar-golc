package scanner

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/SonarSource-Demos/sonar-golc/assets"
	"github.com/SonarSource-Demos/sonar-golc/pkg/analyzer"
)

// The files below were scanned by SonarQube Server 2026.5 Enterprise with default settings,
// and want is the ncloc it reported for each. The test runs GoLC's real language table,
// analyzer and scanner over the same files, so it checks both halves of the count: that a
// file is picked up under the right language, and that its lines are classified the same.

const (
	parityR = `# Header comment
# another comment

add <- function(a, b) {
  # inside comment
  a + b
}

x <- c(1, 2, 3)   # trailing comment
print(add(1, 2))
y <- "string with # not a comment"
`
	parityRMarkdown = "---\ntitle: \"Report\"\n---\n\nProse.\n\n```{r}\nmean(1:10)\n```\n"

	parityDataWeave = `%dw 2.0
output application/json
// line comment
/* block
   comment */
---
{
  name: payload.name, // trailing
  total: sum(payload.items map $.price)
}
`
	parityPostgreSQL = `-- comment line
/* block
   comment */
CREATE TABLE t (
  id serial PRIMARY KEY,
  name text
);

SELECT id, name FROM t WHERE id = 1; -- trailing
`
	parityBicep = `// Bicep comment
/* block
   comment */
param location string = resourceGroup().location

resource sa 'Microsoft.Storage/storageAccounts@2023-01-01' = {
  name: 'mystorage'
  location: location
  sku: {
    name: 'Standard_LRS'
  }
  kind: 'StorageV2'
}
`
	parityShell  = "#!/bin/bash\n# comment\necho hi\nls -la\n"
	parityJCL    = "//JOB1 JOB (ACCT),'NAME'\n//STEP1 EXEC PGM=IEFBR14\n//* comment\n"
	parityPLI    = " HELLO: PROC OPTIONS(MAIN);\n   PUT LIST('HI');\n END HELLO;\n"
	parityApex   = "public class Foo {\n  Integer x = 1;\n}\n"
	parityDocker = "# comment\nFROM alpine:3.20\n\nRUN apk add --no-cache curl\nCOPY . /app\nCMD [\"sh\"]\n"

	parityMuleXML = `<?xml version="1.0" encoding="UTF-8"?>
<mule xmlns="http://www.mulesoft.org/schema/mule/core"
      xmlns:ee="http://www.mulesoft.org/schema/mule/ee/core">
  <flow name="main">
    <set-payload value="#[payload.name]"/>
    <ee:transform>
      <ee:message>
        <ee:set-payload><![CDATA[%dw 2.0
output application/json
---
payload]]></ee:set-payload>
      </ee:message>
    </ee:transform>
  </flow>
</mule>
`
)

// parityNotebook builds an nbformat 4 notebook with the given metadata and cells.
func parityNotebook(t *testing.T, metadata map[string]any, cells ...map[string]any) string {
	t.Helper()
	data, err := json.Marshal(map[string]any{
		"cells": cells, "metadata": metadata, "nbformat": 4, "nbformat_minor": 5,
	})
	if err != nil {
		t.Fatalf("building notebook: %v", err)
	}
	return string(data)
}

func codeCell(source any) map[string]any {
	return map[string]any{"cell_type": "code", "metadata": map[string]any{}, "outputs": []any{}, "source": source}
}

func markdownCell(source any) map[string]any {
	return map[string]any{"cell_type": "markdown", "metadata": map[string]any{}, "source": source}
}

func pythonMetadata(language string) map[string]any {
	return map[string]any{
		"kernelspec":    map[string]any{"name": language, "language": language, "display_name": language},
		"language_info": map[string]any{"name": language},
	}
}

func TestCountsMatchSonarQubeNcloc(t *testing.T) {
	analysis := parityNotebook(t, pythonMetadata("python"),
		markdownCell([]string{"# Title\n", "\n", "Some prose.\n"}),
		codeCell([]string{"# comment\n", "import os\n", "\n", "x = 1\n", "print(x)"}),
		markdownCell([]string{"More prose"}),
		codeCell([]string{"def f(a):\n", "    return a * 2\n", "\n", "f(3)"}),
		codeCell([]string{"%matplotlib inline\n", "!pip install numpy"}),
	)
	twoLines := []string{"import os\n", "print(1)"}

	type parityFile struct {
		path     string
		content  string
		language string
		ncloc    int
	}
	files := []parityFile{
		{"r/script.R", parityR, "R", 6},
		{"r/lower.r", parityR, "R", 6},
		{"dw/transform.dwl", parityDataWeave, "DataWeave", 7},
		{"dw/modules/Utils.dwl", "%dw 2.0\nfun double(n: Number) = n * 2\n", "DataWeave", 2},
		{"src/main/mule/flow.xml", parityMuleXML, "XML", 15},
		{"sql/a.pgsql", parityPostgreSQL, "PostgreSQL", 5},
		{"sql/b.psql", parityPostgreSQL, "PostgreSQL", 5},
		{"bicep/main.bicep", parityBicep, "Bicep", 9},
		{"docker1/Dockerfile", parityDocker, "Docker", 4},
		{"docker2/Containerfile", parityDocker, "Docker", 4},
		{"docker3/containerfile", parityDocker, "Docker", 4},
		{"docker4/app.containerfile", parityDocker, "Docker", 4},
		{"docker5/Dockerfile.prod", parityDocker, "Docker", 4},
		{"docker6/Dockerfile-dev", parityDocker, "Docker", 4},
		{"docker7/Dockerfile_dev", parityDocker, "Docker", 4},
		{"docker8/dockerfile.dev", parityDocker, "Docker", 4},
		{"docker9/Containerfile.prod", parityDocker, "Docker", 4},
		{"docker10/Dockerfile.prod.bak", parityDocker, "Docker", 4},
		{"nb/analysis.ipynb", analysis, "IPython Notebooks", 8},
		{"nb/nometa.ipynb", parityNotebook(t, map[string]any{}, codeCell(twoLines)), "IPython Notebooks", 2},
		{"nb/string_source.ipynb", parityNotebook(t, map[string]any{},
			codeCell("import os\nprint(1)\n\n# c\nx=2")), "IPython Notebooks", 3},
		{"nb/r_kernel.ipynb", parityNotebook(t, pythonMetadata("R"),
			codeCell([]string{"x <- 1\n", "print(x)"})), "IPython Notebooks", 0},
		{"nb/julia.ipynb", parityNotebook(t, pythonMetadata("julia"), codeCell(twoLines)), "IPython Notebooks", 0},
		{"nb/broken.ipynb", `{"cells": [`, "IPython Notebooks", 0},

		// SonarQube matches suffixes regardless of case.
		{"case/Upper.PY", "x = 1\nprint(x)\n", "Python", 2},
		{"case/Upper.GO", "package main\n\nfunc main() {}\n", "Golang", 2},
		{"case/c.PGSQL", parityPostgreSQL, "PostgreSQL", 5},
		{"case/Upper.BICEP", parityBicep, "Bicep", 9},
		{"case/CASE.DWL", parityDataWeave, "DataWeave", 7},
		{"case/Upper.IPYNB", analysis, "IPython Notebooks", 8},
		{"case/UPPER.SH", parityShell, "Shell", 2},
		{"case/b.JCL", parityJCL, "JCL", 2},
		{"case1/app.Containerfile", parityDocker, "Docker", 4},
		{"case2/app.Dockerfile", parityDocker, "Docker", 4},

		// The suffixes SonarQube's defaults keep, beside the ones they drop below.
		{"shell/t.sh", parityShell, "Shell", 2},
		{"shell/t.bash", parityShell, "Shell", 2},
		{"jcl/a.jcl", parityJCL, "JCL", 2},
		{"pli/a.pli", parityPLI, "PL/I", 3},
		{"apex/Foo.apex", parityApex, "Apex", 3},
		{"apex/Bar.cls", parityApex, "Apex", 3},
		{"vb6/Mod.bas", "Attribute VB_Name = \"Mod\"\nSub Main()\n  MsgBox \"hi\"\nEnd Sub\n", "VB6", 4},
	}
	// SonarQube counts no lines for these, so GoLC must not pick them up at all.
	ignored := []parityFile{
		{path: "r/report.Rmd", content: parityRMarkdown},
		{path: "r/lower.rmd", content: parityRMarkdown},
		{path: "docker11/MyDockerfile", content: parityDocker},
		{path: "docker12/api.Dockerfile.j2", content: parityDocker},
		{path: "docker13/DOCKERFILE", content: parityDocker},
		{path: "case/upper.RMD", content: parityRMarkdown},
		// Not in SonarQube's default suffixes.
		{path: "shell/t.zsh", content: parityShell},
		{path: "shell/t.ksh", content: parityShell},
		{path: "shell/t.fish", content: parityShell},
		{path: "jcl/c.job", content: parityJCL},
		{path: "jcl/d.jjob", content: parityJCL},
		{path: "pli/b.pl1", content: parityPLI},
	}

	dir := t.TempDir()
	for _, f := range append(append([]parityFile{}, files...), ignored...) {
		path := filepath.Join(dir, filepath.FromSlash(f.path))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("setup: %v", err)
		}
		if err := os.WriteFile(path, []byte(f.content), 0o644); err != nil {
			t.Fatalf("setup: %v", err)
		}
	}

	extensions := map[string]string{}
	for lang, info := range assets.Languages {
		for _, extension := range info.Extensions {
			extensions[extension] = lang
		}
	}
	matched, err := analyzer.NewAnalyzer(dir, nil, nil, nil, extensions, nil, nil).MatchingFiles()
	if err != nil {
		t.Fatalf("MatchingFiles: %v", err)
	}
	results, err := NewScanner(assets.Languages).Scan(matched)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}

	got := map[string]scanResult{}
	for _, r := range results {
		rel, _ := filepath.Rel(dir, r.Metadata.FilePath)
		got[filepath.ToSlash(rel)] = r
	}

	for _, f := range files {
		r, ok := got[f.path]
		if !ok {
			t.Errorf("%s was not counted; SonarQube reports %d lines of code", f.path, f.ncloc)
			continue
		}
		if r.Metadata.Language != f.language {
			t.Errorf("%s reported as %q, want %q", f.path, r.Metadata.Language, f.language)
		}
		if r.CodeLines != f.ncloc {
			t.Errorf("%s has %d lines of code, SonarQube reports %d", f.path, r.CodeLines, f.ncloc)
		}
	}
	for _, f := range ignored {
		if r, ok := got[f.path]; ok {
			t.Errorf("%s was counted as %s; SonarQube counts no lines for it", f.path, r.Metadata.Language)
		}
	}
}
