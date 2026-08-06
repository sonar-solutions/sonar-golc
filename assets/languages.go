package assets

import "github.com/SonarSource-Demos/sonar-golc/pkg/goloc/language"

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
		Extensions:        []string{".cls", ".trigger"},
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
	"COBOL": {
		LineComments:      []string{"*"},
		MultiLineComments: [][]string{},
		Extensions:        []string{".cbl", ".CBL", ".ccp", ".cob", ".COB", ".cobol", ".cpy"},
	},
	"C#": {
		LineComments:      []string{"//"},
		MultiLineComments: [][]string{{"/*", "*/"}},
		Extensions:        []string{".cs", ".razor"},
	},
	"CSS": {
		LineComments:      []string{},
		MultiLineComments: [][]string{{"/*", "*/"}},
		Extensions:        []string{".css", ".less", ".sass"},
	},
	"Dart": {
		LineComments:      []string{"//"},
		MultiLineComments: [][]string{{"/*", "*/"}},
		Extensions:        []string{".dart"},
	},
	"Docker": {
		LineComments:      []string{"#"},
		MultiLineComments: [][]string{},
		Extensions:        []string{"Dockerfile", "dockerfile", ".dockerfile"},
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
		Extensions:        []string{".html", ".htm", ".cshtml", ".vbhtml", ".aspx", ".ascx", ".rhtml", ".erb", ".shtml", ".shtm", ".cmp", ".twig"},
	},
	"JCL": {
		LineComments:      []string{"//*"},
		MultiLineComments: [][]string{},
		Extensions:        []string{".jcl", ".JCL", ".jjob", ".job"},
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
	"Kotlin": {
		LineComments:      []string{"//"},
		MultiLineComments: [][]string{{"/*", "*/"}},
		Extensions:        []string{".kt", ".kts"},
	},
	"Objective-C": {
		LineComments:      []string{"//"},
		MultiLineComments: [][]string{{"/*", "*/"}},
		Extensions:        []string{".m", ".mm"},
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
	"PL/I": {
		LineComments:      []string{},
		MultiLineComments: [][]string{{"/*", "*/"}},
		Extensions:        []string{".pl1", ".pli"},
	},
	// Suffixes follow sonar.powershell.file.suffixes (ps1,psm1,psd1).
	"PowerShell": {
		LineComments:      []string{"#"},
		MultiLineComments: [][]string{{"<#", "#>"}},
		Extensions:        []string{".ps1", ".psm1", ".psd1"},
	},
	"Python": {
		LineComments:      []string{"#"},
		MultiLineComments: [][]string{{"\"\"\"", "\"\"\""}, {"'''", "'''"}},
		Extensions:        []string{".py"},
	},
	"RPG": {
		LineComments:      []string{"*"},
		MultiLineComments: [][]string{},
		// sonar.rpg.file.suffixes lists the RPG IV suffixes and their uppercase spellings;
		// extension lookup here is case-sensitive, so both cases are needed.
		Extensions: []string{".rpg", ".rpgle", ".sqlrpgle", ".RPG", ".RPGLE", ".SQLRPGLE"},
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
	"Scss": {
		LineComments:      []string{"//"},
		MultiLineComments: [][]string{{"/*", "*/"}},
		Extensions:        []string{".scss"},
	},
	"Shell": {
		LineComments:      []string{"#"},
		MultiLineComments: [][]string{},
		Extensions:        []string{".sh", ".bash", ".zsh", ".fish", ".ksh"},
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
	"TypeScript": {
		LineComments:      []string{"//"},
		MultiLineComments: [][]string{{"/*", "*/"}},
		Extensions:        []string{".ts", ".tsx", ".cts", ".mts"},
	},
	"VB6": {
		LineComments:      []string{"'"},
		MultiLineComments: [][]string{},
		Extensions:        []string{".bas", ".frm", ".cls", ".ctl"},
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
		Extensions:        []string{".xml", ".XML", ".xsd", ".xsl", ".config"},
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
