package scanner

import (
	"bufio"
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
	log := utils.NewLogger()

	for _, file := range files {
		result, err := sc.scanFile(file)
		if err != nil {
			return results, err
		}
		progress.Add(1)
		results = append(results, result)
		log.Debugf("scanned file: path=%s lines=%d loc=%d blank=%d comments=%d",
			result.Metadata.FilePath, result.Lines, result.CodeLines, result.BlankLines, result.Comments)
	}

	return results, nil
}

func (sc *Scanner) createProgressbar(max int) *progressbar.ProgressBar {
	return progressbar.NewOptions(
		max,
		progressbar.OptionSetDescription("Scanning files..."),
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
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				break
			}
			return result, err
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

		result.CodeLines++
	}

	result.Lines = result.CodeLines + result.BlankLines + result.Comments

	return result, nil
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
