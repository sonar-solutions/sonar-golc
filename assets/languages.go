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
		Extensions:        []string{".cpp", ".cc"},
	},
	"C++ Header": {
		LineComments:      []string{"//"},
		MultiLineComments: [][]string{{"/*", "*/"}},
		Extensions:        []string{".hh", ".hpp"},
	},
	"COBOL": {
		LineComments:      []string{"*"},
		MultiLineComments: [][]string{},
		Extensions:        []string{".cbl", ".CBL", ".ccp", ".cob", ".COB", ".cobol", ".cpy"},
	},
	"C#": {
		LineComments:      []string{"//"},
		MultiLineComments: [][]string{{"/*", "*/"}},
		Extensions:        []string{".cs"},
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
	"HTML": {
		LineComments:      []string{},
		MultiLineComments: [][]string{{"<!--", "-->"}},
		Extensions:        []string{".html", ".htm", ".cshtml", ".vbhtml", ".aspx", ".ascx", ".rhtml", ".erb", ".shtml", ".shtm", ".cmp"},
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
		Extensions:        []string{".js", ".jsx", ".jsp", ".jspf"},
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
		Extensions:        []string{".pkb"},
	},
	"PHP": {
		LineComments:      []string{"//", "#"},
		MultiLineComments: [][]string{{"/*", "*/"}},
		Extensions:        []string{".php", ".php3", ".php4", ".php5", ".phtml", ".inc"},
	},
	"PL/I": {
		LineComments:      []string{},
		MultiLineComments: [][]string{{"/*", "*/"}},
		Extensions:        []string{".pl1", ".pli"},
	},
	"Python": {
		LineComments:      []string{"#"},
		MultiLineComments: [][]string{{"\"\"\"", "\"\"\""}, {"'''", "'''"}},
		Extensions:        []string{".py"},
	},
	"RPG": {
		LineComments:      []string{"*"},
		MultiLineComments: [][]string{},
		Extensions:        []string{".rpg"},
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
		Extensions:        []string{".ts", ".tsx"},
	},
	"VB6": {
		LineComments:      []string{"'"},
		MultiLineComments: [][]string{},
		Extensions:        []string{".bas", ".frm", ".cls"},
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
		Extensions:        []string{".xml", ".XML"},
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
}
