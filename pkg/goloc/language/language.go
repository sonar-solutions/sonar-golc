package language

type LanguageInfo struct {
	LineComments      []string
	MultiLineComments [][]string
	Extensions        []string

	// NonCodeLines are markup delimiters that carry no code when they sit on a line by
	// themselves, such as PHP's "<?php" and "?>". SonarQube does not count such a line
	// towards ncloc, so neither does GoLC. Matching is on the whole trimmed line: a
	// delimiter sharing a line with code (`<?php $a = 1;`) is still a line of code.
	NonCodeLines []string

	// ContentDetected marks a language that is never resolved from a file extension but
	// from the content of a file another language already claims - the IaC dialects of
	// YAML and JSON, which SonarQube analyses by default while plain YAML and JSON are
	// opt-in. Such a language has no Extensions.
	ContentDetected bool
}

type Languages map[string]LanguageInfo
