package assets

import "github.com/SonarSource-Demos/sonar-golc/pkg/goloc/language"

// Suffixes are listed in lower case once: the analyzer compares them regardless of case, as
// SonarQube does (see analyzer.ExtensionKey). Exact file names such as Dockerfile keep
// their case. Each list follows the sonar.<lang>.file.suffixes default of SonarQube
// 2026.5; a suffix SonarQube does not analyse by default is left out, because counting
// it would over-report.

// pythonMultiLineComments is shared by Python and IPython Notebooks, whose code cells are
// Python.
var pythonMultiLineComments = [][]string{{`"""`, `"""`}, {"'''", "'''"}}

var Languages = language.Languages{
	"ActionScript": {
		LineComments:      []string{"//"},
		MultiLineComments: [][]string{{"/*", "*/"}},
		Extensions:        []string{".as"},
	},
	"Abap": {
		LineComments:      []string{"*", "\""},
		MultiLineComments: [][]string{},
		Extensions:        []string{".abap", ".ab4", ".flow", ".asprog"},
	},
	"Apex": {
		LineComments:      []string{"//"},
		MultiLineComments: [][]string{{"/*", "*/"}},
		Extensions:        []string{".cls", ".trigger", ".apex"},
	},
	"C": {
		LineComments:      []string{"//"},
		MultiLineComments: [][]string{{"/*", "*/"}},
		Extensions:        []string{".c"},
	},
	"C Header": {
		LineComments:      []string{"//"},
		MultiLineComments: [][]string{{"/*", "*/"}},
		Extensions:        []string{".h"},
	},
	"C++": {
		LineComments:      []string{"//"},
		MultiLineComments: [][]string{{"/*", "*/"}},
		Extensions:        []string{".cpp", ".cc", ".cxx", ".c++", ".ipp", ".ixx", ".mxx", ".cppm", ".ccm", ".cxxm", ".c++m"},
	},
	"C++ Header": {
		LineComments:      []string{"//"},
		MultiLineComments: [][]string{{"/*", "*/"}},
		Extensions:        []string{".hh", ".hpp", ".hxx", ".h++"},
	},
	// SonarQube ships sonar.cobol.file.suffixes empty, so a stock instance counts no COBOL
	// until it is configured. Anyone with COBOL configures it, so these common suffixes stay.
	"COBOL": {
		LineComments:      []string{"*"},
		MultiLineComments: [][]string{},
		Extensions:        []string{".cbl", ".ccp", ".cob", ".cobol", ".cpy"},
	},
	// SonarQube reports Bicep under azureresourcemanager (sonar.azureresourcemanager.file.suffixes
	// is .bicep), but it cannot share GoLC's Azure Resource Manager entry: that one is
	// content-detected from JSON, which has no comment syntax, while Bicep has // and /* */.
	"Bicep": {
		LineComments:      []string{"//"},
		MultiLineComments: [][]string{{"/*", "*/"}},
		Extensions:        []string{".bicep"},
	},
	"C#": {
		LineComments:      []string{"//"},
		MultiLineComments: [][]string{{"/*", "*/"}},
		Extensions:        []string{".cs", ".razor"},
	},
	"CSS": {
		LineComments:      []string{},
		MultiLineComments: [][]string{{"/*", "*/"}},
		Extensions:        []string{".css"},
	},
	"Dart": {
		LineComments:      []string{"//"},
		MultiLineComments: [][]string{{"/*", "*/"}},
		Extensions:        []string{".dart"},
	},
	// sonar.dataweave.file.suffixes (dwl). SonarQube counts .dwl files whether or not the
	// project has a mule-artifact.json; the Mule XML configuration stays plain XML.
	"DataWeave": {
		LineComments:      []string{"//"},
		MultiLineComments: [][]string{{"/*", "*/"}},
		Extensions:        []string{".dwl"},
	},
	// sonar.docker.file.patterns. Variants such as Dockerfile.prod or Dockerfile-dev have
	// no suffix of their own; analyzer.getFileExtension maps them onto these names.
	"Docker": {
		LineComments:      []string{"#"},
		MultiLineComments: [][]string{},
		Extensions:        []string{"Dockerfile", "dockerfile", ".dockerfile", "Containerfile", "containerfile", ".containerfile"},
	},
	"Flex": {
		LineComments:      []string{"//"},
		MultiLineComments: [][]string{{"/*", "*/"}},
		Extensions:        []string{".as"},
	},
	"Golang": {
		LineComments:      []string{"//"},
		MultiLineComments: [][]string{{"/*", "*/"}},
		Extensions:        []string{".go"},
	},
	// Suffixes follow sonar.gosu.file.suffixes (gs,gsx,gsp).
	"Gosu": {
		LineComments:      []string{"//"},
		MultiLineComments: [][]string{{"/*", "*/"}},
		Extensions:        []string{".gs", ".gsx", ".gsp"},
	},
	// Suffixes follow sonar.groovy.file.suffixes (groovy,gvy,gy,gsh). SonarQube also
	// claims *Jenkinsfile via sonar.groovy.file.patterns; an extensionless path falls
	// back to its base name here, so the bare name matches the common case.
	"Groovy": {
		LineComments:      []string{"//"},
		MultiLineComments: [][]string{{"/*", "*/"}},
		Extensions:        []string{".groovy", ".gvy", ".gy", ".gsh", "Jenkinsfile"},
	},
	"HTML": {
		LineComments:      []string{},
		MultiLineComments: [][]string{{"<!--", "-->"}},
		Extensions:        []string{".html", ".htm", ".cshtml", ".vbhtml", ".aspx", ".ascx", ".rhtml", ".erb", ".shtml", ".shtm", ".cmp"},
	},
	// sonar.ipynb.file.suffixes (ipynb). Only the code cells of a Python notebook count;
	// see scanner.notebookCodeCells. The comment syntax is Python's, applied to those cells.
	"IPython Notebooks": {
		LineComments:      []string{"#"},
		MultiLineComments: pythonMultiLineComments,
		Extensions:        []string{".ipynb"},
		JupyterNotebook:   true,
	},
	"JCL": {
		LineComments:      []string{"//*"},
		MultiLineComments: [][]string{},
		Extensions:        []string{".jcl"},
	},
	"Java": {
		LineComments:      []string{"//"},
		MultiLineComments: [][]string{{"/*", "*/"}},
		Extensions:        []string{".java", ".jav"},
	},
	"JavaScript": {
		LineComments:      []string{"//"},
		MultiLineComments: [][]string{{"/*", "*/"}},
		Extensions:        []string{".js", ".jsx", ".cjs", ".mjs"},
	},
	// .jsp/.jspf used to be counted as JavaScript, which both mislabelled them and
	// applied the wrong comment syntax: a JSP comment is <%-- --%> and its template
	// text uses <!-- -->, neither of which is "//". SonarQube reports these under its
	// own jsp language (sonar.jsp.file.suffixes), so they are separated here too.
	"JSP": {
		LineComments:      []string{},
		MultiLineComments: [][]string{{"<%--", "--%>"}, {"<!--", "-->"}},
		Extensions:        []string{".jsp", ".jspf", ".jspx"},
	},
	"JSON": {
		LineComments:      []string{},
		MultiLineComments: [][]string{},
		Extensions:        []string{".json"},
	},
	// SonarQube reports Less and Sass under its css language, but they cannot share
	// GoLC's CSS entry: both support "//" line comments, which plain CSS does not, so
	// folding them in would count every such comment as code. Same tokens as Scss.
	"Less": {
		LineComments:      []string{"//"},
		MultiLineComments: [][]string{{"/*", "*/"}},
		Extensions:        []string{".less"},
	},
	"Kotlin": {
		LineComments:      []string{"//"},
		MultiLineComments: [][]string{{"/*", "*/"}},
		Extensions:        []string{".kt", ".kts"},
	},
	"Objective-C": {
		LineComments:      []string{"//"},
		MultiLineComments: [][]string{{"/*", "*/"}},
		// sonar.objc.file.suffixes is .m alone; Objective-C++ .mm is not analysed by default.
		Extensions: []string{".m"},
	},
	"Oracle PL/SQL": {
		LineComments:      []string{"--"},
		MultiLineComments: [][]string{{"/*", "*/"}},
		Extensions:        []string{".pkb", ".pks"},
	},
	"PHP": {
		LineComments:      []string{"//", "#"},
		MultiLineComments: [][]string{{"/*", "*/"}},
		Extensions:        []string{".php", ".php3", ".php4", ".php5", ".phtml", ".inc"},
		// Measured against SonarQube: a line holding only an open or close tag is not
		// counted, while a tag sharing a line with code is.
		NonCodeLines: []string{"<?php", "<?", "?>"},
	},
	// sonar.postgres.file.suffixes (pgsql,psql). .sql stays with the SQL entry, as it does
	// with SonarQube, where PL/SQL owns it by default.
	"PostgreSQL": {
		LineComments:      []string{"--"},
		MultiLineComments: [][]string{{"/*", "*/"}},
		Extensions:        []string{".pgsql", ".psql"},
	},
	"PL/I": {
		LineComments:      []string{},
		MultiLineComments: [][]string{{"/*", "*/"}},
		Extensions:        []string{".pli"},
	},
	// Suffixes follow sonar.powershell.file.suffixes (ps1,psm1,psd1).
	"PowerShell": {
		LineComments:      []string{"#"},
		MultiLineComments: [][]string{{"<#", "#>"}},
		Extensions:        []string{".ps1", ".psm1", ".psd1"},
	},
	"Python": {
		LineComments:      []string{"#"},
		MultiLineComments: pythonMultiLineComments,
		Extensions:        []string{".py"},
	},
	"RPG": {
		LineComments:      []string{"*"},
		MultiLineComments: [][]string{},
		Extensions:        []string{".rpg", ".rpgle", ".sqlrpgle"},
	},
	// sonar.r.file.suffixes also lists Rmd and rmd, but SonarQube reports no lines of code
	// for R Markdown - not even its R chunks - so those are deliberately left out.
	"R": {
		LineComments:      []string{"#"},
		MultiLineComments: [][]string{},
		Extensions:        []string{".r"},
	},
	"Ruby": {
		LineComments:      []string{"#"},
		MultiLineComments: [][]string{{"=begin", "=end"}},
		Extensions:        []string{".rb"},
	},
	"Rust": {
		LineComments:      []string{"//"},
		MultiLineComments: [][]string{{"/*", "*/"}},
		Extensions:        []string{".rs"},
	},
	"Scala": {
		LineComments:      []string{"//"},
		MultiLineComments: [][]string{{"/*", "*/"}},
		Extensions:        []string{".scala"},
	},
	"Sass": {
		LineComments:      []string{"//"},
		MultiLineComments: [][]string{{"/*", "*/"}},
		Extensions:        []string{".sass"},
	},
	"Scss": {
		LineComments:      []string{"//"},
		MultiLineComments: [][]string{{"/*", "*/"}},
		Extensions:        []string{".scss"},
	},
	"Shell": {
		LineComments:      []string{"#"},
		MultiLineComments: [][]string{},
		Extensions:        []string{".sh", ".bash"},
	},
	"SQL": {
		LineComments:      []string{"--"},
		MultiLineComments: [][]string{{"/*", "*/"}},
		Extensions:        []string{".sql"},
	},
	"Swift": {
		LineComments:      []string{"//"},
		MultiLineComments: [][]string{{"/*", "*/"}},
		Extensions:        []string{".swift"},
	},
	"Terraform": {
		LineComments:      []string{"#", "//"},
		MultiLineComments: [][]string{{"/*", "*/"}},
		Extensions:        []string{".tf"},
	},
	"T-SQL": {
		LineComments:      []string{"--"},
		MultiLineComments: [][]string{{"/*", "*/"}},
		Extensions:        []string{".tsql"},
	},
	// SonarQube reports Twig under its html language, but a Twig comment is {# #}, which
	// the HTML entry does not know. Template files also contain markup, so both forms are
	// recognised here.
	"Twig": {
		LineComments:      []string{},
		MultiLineComments: [][]string{{"{#", "#}"}, {"<!--", "-->"}},
		Extensions:        []string{".twig"},
	},
	"TypeScript": {
		LineComments:      []string{"//"},
		MultiLineComments: [][]string{{"/*", "*/"}},
		Extensions:        []string{".ts", ".tsx", ".cts", ".mts"},
	},
	// .cls is a VB6 class module too, but SonarQube gives it to Apex, so VB6 leaves it.
	"VB6": {
		LineComments:      []string{"'"},
		MultiLineComments: [][]string{},
		Extensions:        []string{".bas", ".frm", ".ctl"},
	},
	"Visual Basic .NET": {
		LineComments:      []string{"'"},
		MultiLineComments: [][]string{},
		Extensions:        []string{".vb"},
	},
	"Vue": {
		LineComments:      []string{},
		MultiLineComments: [][]string{{"<!--", "-->"}},
		Extensions:        []string{".vue"},
	},
	"XML": {
		LineComments:      []string{},
		MultiLineComments: [][]string{{"<!--", "-->"}},
		Extensions:        []string{".xml", ".xsd", ".xsl", ".config"},
	},
	"XHTML": {
		LineComments:      []string{},
		MultiLineComments: [][]string{{"<!--", "-->"}},
		Extensions:        []string{".xhtml"},
	},
	"YAML": {
		LineComments:      []string{"#"},
		MultiLineComments: [][]string{},
		Extensions:        []string{".yaml", ".yml"},
	},

	// --------------------------------------------------------- content-detected (IaC)
	//
	// SonarQube ships plain YAML and JSON analysis OFF (sonar.yaml.activate and
	// sonar.json.activate both default to false) but every IaC analyzer ON. So a stock
	// SonarQube bills a Kubernetes manifest and ignores the plain YAML beside it.
	// Reporting all .yaml as one language cannot match that, whichever way it is
	// counted, so these dialects are recognised from file content instead and reported
	// separately. They intentionally declare no extensions - see analyzer.RefineLanguage.
	//
	// Comment syntax is inherited from the host format: # for YAML, none for JSON.
	"Ansible": {
		LineComments:      []string{"#"},
		MultiLineComments: [][]string{},
		Extensions:        []string{},
		ContentDetected:   true,
	},
	"Azure Pipelines": {
		LineComments:      []string{"#"},
		MultiLineComments: [][]string{},
		Extensions:        []string{},
		ContentDetected:   true,
	},
	"CloudFormation": {
		LineComments:      []string{"#"},
		MultiLineComments: [][]string{},
		Extensions:        []string{},
		ContentDetected:   true,
	},
	"GitHub Actions": {
		LineComments:      []string{"#"},
		MultiLineComments: [][]string{},
		Extensions:        []string{},
		ContentDetected:   true,
	},
	"Kubernetes": {
		LineComments:      []string{"#"},
		MultiLineComments: [][]string{},
		Extensions:        []string{},
		ContentDetected:   true,
	},
	// Azure Resource Manager templates are JSON, which has no comment syntax.
	"Azure Resource Manager": {
		LineComments:      []string{},
		MultiLineComments: [][]string{},
		Extensions:        []string{},
		ContentDetected:   true,
	},
}
