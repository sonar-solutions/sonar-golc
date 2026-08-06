package analyzer

import (
	"bufio"
	"io"
	"os"
	"strings"
)

// SonarQube excludes minified JavaScript and CSS from analysis, so those files never
// reach ncloc. Counting them makes GoLC over-estimate, and by a lot: a committed bundle
// is routinely tens of thousands of lines.
//
// This is a direct port of SonarJS's filter, so the two agree by construction:
// https://github.com/SonarSource/SonarJS/blob/master/packages/analysis/src/common/filter/filter-minified.ts
//
//	const DEFAULT_AVERAGE_LINE_LENGTH_THRESHOLD = 200;
//	isMinified = hasMinifiedFilename(path)
//	          || (isMinifiableFilename(path) && hasExcessiveAverageLineLength(input))
//
// Note the two halves are different tests. The name check alone would miss a minified
// bundle called "vendor.js", and the content check is only ever applied to .js and .css -
// a long-lined .ts file is still counted, because SonarJS counts it too.
const averageLineLengthThreshold = 200

// minifiedSuffixes is hasMinifiedFilename. The hyphen spellings are as common as the dot
// ones and a "*.min.*" glob does not match them.
var minifiedSuffixes = []string{".min.js", "-min.js", ".min.css", "-min.css"}

// minifiableSuffixes is isMinifiableFilename: only these are content-checked.
var minifiableSuffixes = []string{".js", ".css"}

// looksMinified reports whether SonarQube would treat this file as minified and leave it
// out of ncloc. A file it cannot read is not treated as minified: dropping lines because
// of an I/O error would be worse than counting them.
func looksMinified(path string) bool {
	lower := strings.ToLower(path)

	for _, suffix := range minifiedSuffixes {
		if strings.HasSuffix(lower, suffix) {
			return true
		}
	}

	if !hasAnySuffix(lower, minifiableSuffixes) {
		return false
	}

	average, ok := averageLineLength(path)

	return ok && average > averageLineLengthThreshold
}

func hasAnySuffix(lower string, suffixes []string) bool {
	for _, suffix := range suffixes {
		if strings.HasSuffix(lower, suffix) {
			return true
		}
	}

	return false
}

// averageLineLength mirrors SonarJS's getAverageLineLength: split on \n, drop a trailing
// empty element, then divide the total length of the remaining lines by their count.
//
// Reading is capped: a minified file is by definition one enormous line, and the average
// of a prefix is a sound estimate precisely because the lines are uniform. The cap is
// well above the threshold, so a prefix that long can only come from very long lines.
func averageLineLength(path string) (float64, bool) {
	f, err := os.Open(path)
	if err != nil {
		return 0, false
	}
	defer f.Close()

	reader := bufio.NewReader(io.LimitReader(f, minifiedSampleBytes))
	var total, lines int
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			// A final chunk with no newline is still a line, unless it is empty - which is
			// what dropping the trailing empty element amounts to.
			if len(line) > 0 {
				total += len(line)
				lines++
			}

			break
		}
		total += len(line) - 1 // the \n is not part of the line's length
		lines++
	}

	if lines == 0 {
		return 0, false
	}

	return float64(total) / float64(lines), true
}

// minifiedSampleBytes bounds the read. 256 KiB spans well over a thousand lines at the
// 200-character threshold, so a normal source file is measured in full while a bundle is
// sampled rather than loaded.
const minifiedSampleBytes = 256 * 1024
