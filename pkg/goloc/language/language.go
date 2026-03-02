package language

// PathPattern defines a path prefix and extensions for filename-based language detection
// (e.g., .github/workflows/*.yml for GitHub Actions)
type PathPattern struct {
	Prefix    string   // Path must contain this prefix (e.g., ".github/workflows/")
	Extensions []string // File must have one of these extensions (e.g., ".yml", ".yaml")
}

type LanguageInfo struct {
	LineComments      []string
	MultiLineComments [][]string
	Extensions        []string
	Filenames         []string       // Specific filenames for basename match (e.g., "playbook.yml", "Dockerfile")
	PathPatterns      []PathPattern  // Path-based patterns (e.g., .github/workflows/*.yml)
}

type Languages map[string]LanguageInfo
