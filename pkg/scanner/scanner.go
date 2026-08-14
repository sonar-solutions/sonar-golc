package scanner

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/SonarSource-Demos/sonar-golc/pkg/analyzer"
	"github.com/SonarSource-Demos/sonar-golc/pkg/goloc/language"
	"github.com/SonarSource-Demos/sonar-golc/pkg/utils"
	"github.com/schollz/progressbar/v3"
)

type Scanner struct {
	SupportedLanguages language.Languages
}

type scanResult struct {
	Metadata   analyzer.FileMetadata
	Lines      int
	CodeLines  int
	BlankLines int
	Comments   int
}

func NewScanner(languages language.Languages) *Scanner {
	return &Scanner{
		SupportedLanguages: languages,
	}
}

func (sc *Scanner) Scan(files []analyzer.FileMetadata) ([]scanResult, error) {
	var results []scanResult
	progress := sc.createProgressbar(len(files))
	logger := utils.SharedLogger()

	failed := 0
	for _, file := range files {
		result, err := sc.scanFile(file)
		if err != nil {
			// A single unreadable file (broken symlink, permission denied, transient I/O
			// error) must not abort the entire repository scan. Log and skip so the rest
			// of the repo still produces a JSON output.
			logger.Warnf("⚠️  Skipping unreadable file %s: %v", file.FilePath, err)
			failed++
			progress.Add(1)
			continue
		}
		progress.Add(1)
		results = append(results, result)
	}

	// Distinguish a fully-failed scan from a genuinely empty repository: if every
	// candidate file was unreadable, surface an error so downstream report
	// generation does not silently produce a zero-line report (which would mask
	// real, systemic problems — wrong path, tree-wide permission denial, etc.).
	if failed > 0 && len(results) == 0 && len(files) > 0 {
		logger.Errorf("❌ Scan failed: all %d candidate file(s) were unreadable", failed)
		return results, fmt.Errorf("scan failed: all %d candidate file(s) were unreadable", failed)
	}
	if failed > 0 {
		logger.Warnf("⚠️  Scan completed with %d/%d file(s) skipped", failed, len(files))
	}

	return results, nil
}

// isTerminal reports whether f is attached to a terminal. A character device is the
// portable signal for that, so no dependency is needed to ask.
//
// /dev/null and /dev/zero are character devices too and so are read as terminals. That is
// harmless here - the only consequence is drawing a progress bar into a sink that discards
// it - and avoiding it would mean taking on a dependency to answer a question whose wrong
// answer costs nothing.
func isTerminal(f *os.File) bool {
	info, err := f.Stat()

	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

// progressWriter decides where the progress bar draws.
//
// A progress bar animates by repainting the same line, which only makes sense on a terminal.
// Redirected to a file or a CI log every repaint is kept, and the bar becomes the bulk of the
// output - 117 KB around 3 KB of log on a 300-file scan, burying the messages someone is
// actually reading. Off the terminal, draw nothing.
//
// The stream that is tested and the stream that is written to are the same argument, so they
// cannot drift apart. Relying on the library's default writer would leave that agreement
// resting on another module's constructor: progressbar's Default and DefaultBytes write to
// stderr while NewOptions writes to stdout, so a change of constructor or an upgrade could
// silently move the bar to a stream this never guarded - reinstating the flooding while the
// check still looks correct.
func progressWriter(out *os.File) io.Writer {
	if !isTerminal(out) {
		return io.Discard
	}

	return out
}

func (sc *Scanner) createProgressbar(max int) *progressbar.ProgressBar {
	options := []progressbar.Option{
		progressbar.OptionSetDescription("Scanning files..."),
		progressbar.OptionSetWriter(progressWriter(os.Stdout)),
	}

	options = append(options,
		progressbar.OptionShowBytes(true),
		progressbar.OptionShowCount(),
		progressbar.OptionClearOnFinish(),
		progressbar.OptionSetTheme(progressbar.Theme{
			Saucer:        "=",
			SaucerHead:    ">",
			SaucerPadding: " ",
			BarStart:      "[",
			BarEnd:        "]",
		}),
	)

	return progressbar.NewOptions(max, options...)
}

// Old Function using bufio.Scanner Now use bufio.Reader which does not limit the line size.

/*func (sc *Scanner) scanFile(file analyzer.FileMetadata) (scanResult, error) {
	result := scanResult{Metadata: file}
	isInBlockComment := false
	var closeBlockCommentToken string

	f, err := os.Open(file.FilePath)
	if err != nil {
		return result, err
	}
	defer f.Close()

	fileScanner := bufio.NewScanner(f)
	//buffer := make([]byte, 128*1024)
	//fileScanner.Buffer(buffer, 4096*1024)
	buffer := make([]byte, 2048*2048)
	fileScanner.Buffer(buffer, 4096*1024)
	fmt.Println("Hello Scan Buff")
	for fileScanner.Scan() {
		line := strings.TrimSpace(fileScanner.Text())

		if isInBlockComment {
			result.Comments++
			if sc.hasSecondMultiLineComment(line, closeBlockCommentToken) {
				isInBlockComment = false
			}
			continue
		}

		if sc.isBlankLine(line) {
			result.BlankLines++
			continue
		}

		if ok, secondCommentToken := sc.hasFirstMultiLineComment(file, line); ok {
			isInBlockComment = true
			closeBlockCommentToken = secondCommentToken
			result.Comments++
			if sc.hasSecondMultiLineComment(line, closeBlockCommentToken) {
				isInBlockComment = false
			}
			continue
		}

		if sc.hasSingleLineComment(file, line) {
			result.Comments++
			continue
		}

		result.CodeLines++
	}

	result.Lines = result.CodeLines + result.BlankLines + result.Comments

	return result, fileScanner.Err()
}*/

func (sc *Scanner) scanFile(file analyzer.FileMetadata) (scanResult, error) {
	result := scanResult{Metadata: file}
	isInBlockComment := false
	var closeBlockCommentToken string

	f, err := os.Open(file.FilePath)
	if err != nil {
		return result, err
	}
	defer f.Close()

	reader := bufio.NewReader(f)
	firstLine := true
	for {
		line, err := reader.ReadString('\n')
		if err != nil && err != io.EOF {
			return result, err
		}

		// ReadString returns the final chunk together with io.EOF when a file does not end
		// in a newline. Breaking on the error discarded that chunk, losing one line from
		// every such file. An empty chunk means the file did end in a newline and there is
		// genuinely nothing left.
		lastLine := err == io.EOF
		if lastLine && line == "" {
			break
		}

		// A UTF-8 BOM is not Unicode whitespace, so TrimSpace leaves it in place and every
		// prefix test below misses: a file opening with a BOM and then a block comment had
		// its whole licence header counted as code. Visual Studio writes a BOM by default,
		// so this hit 92% of the C# files in a real .NET repository and inflated its count
		// by 50%. Strip it only on the first line - U+FEFF anywhere else is a zero-width
		// no-break space, which is content.
		if firstLine {
			line = strings.TrimPrefix(line, "\ufeff")
			firstLine = false
		}

		line = strings.TrimSpace(line)

		if isInBlockComment {
			result.Comments++
			if sc.hasSecondMultiLineComment(line, closeBlockCommentToken) {
				isInBlockComment = false
			}
			continue
		}

		if sc.isBlankLine(line) {
			result.BlankLines++
			continue
		}

		if ok, secondCommentToken := sc.hasFirstMultiLineComment(file, line); ok {
			isInBlockComment = true
			closeBlockCommentToken = secondCommentToken
			result.Comments++
			if sc.hasSecondMultiLineComment(line, closeBlockCommentToken) {
				isInBlockComment = false
			}
			continue
		}

		if sc.hasSingleLineComment(file, line) {
			result.Comments++
			continue
		}

		// A markup delimiter alone on its line (PHP's "<?php") is not code. It is not a
		// comment or a blank line either, so it is counted in no category at all -
		// meaning Lines below excludes it, as SonarQube's ncloc does.
		if sc.isNonCodeLine(file, line) {
			continue
		}

		result.CodeLines++
	}

	result.Lines = result.CodeLines + result.BlankLines + result.Comments

	return result, nil
}

// isNonCodeLine reports whether the whole trimmed line is a markup delimiter that
// carries no code. Comparison is against the entire line, so a delimiter followed by
// real code on the same line is still counted as code.
func (sc *Scanner) isNonCodeLine(file analyzer.FileMetadata, line string) bool {
	for _, delimiter := range sc.SupportedLanguages[file.Language].NonCodeLines {
		if line == delimiter {
			return true
		}
	}

	return false
}

func (sc *Scanner) hasFirstMultiLineComment(file analyzer.FileMetadata, line string) (bool, string) {
	multiLineComments := sc.SupportedLanguages[file.Language].MultiLineComments

	for _, multiLineComment := range multiLineComments {
		firstCommentToken := multiLineComment[0]
		if strings.HasPrefix(line, firstCommentToken) {
			return true, multiLineComment[1]
		}
	}

	return false, ""
}

func (sc *Scanner) hasSecondMultiLineComment(line, commentToken string) bool {
	return strings.Contains(line, commentToken)
}

func (sc *Scanner) hasSingleLineComment(file analyzer.FileMetadata, line string) bool {
	lineComments := sc.SupportedLanguages[file.Language].LineComments

	for _, lineComment := range lineComments {
		if strings.HasPrefix(line, lineComment) {
			return true
		}
	}

	return false
}

func (sc *Scanner) isBlankLine(line string) bool {
	return len(line) == 0
}
