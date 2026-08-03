package assets

const (
	ByFileFlag            = "by-file"
	ExcludePathsFlag      = "exclude"
	ExcludeExtensionsFlag = "exclude-extensions"
	IncludeExtensionsFlag = "include-extensions"
	OrderByLangFlag       = "order-by-lang"
	OrderByFileFlag       = "order-by-file"
	OrderByCodeFlag       = "order-by-code"
	OrderByLineFlag       = "order-by-line"
	OrderByBlankFlag      = "order-by-blank"
	OrderByCommentFlag    = "order-by-comment"
	OrderFlag             = "order"
	OutputNameFlag        = "output-name"
	OutputPathFlag        = "output-path"
	ReportFormatsFlag     = "report-formats"
)

// Version is the build's version, stamped at link time by the release build:
//
//	go build -ldflags "-X github.com/SonarSource-Demos/sonar-golc/assets.Version=ver2.0.6" ...
//
// It stays "development" for a local build. The import path must be spelled in full and
// the variable must remain a non-constant package-level string — the linker silently
// ignores an -X flag whose target does not exist, which is how the release script came to
// stamp nothing at all for several releases.
//
// This is the build identity, distinct from golc.go's version1, which is the config-file
// compatibility version and is deliberately not linker-stamped.
var Version = "development"
