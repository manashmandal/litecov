package parser

import (
	"bufio"
	"errors"
	"io"
	"path/filepath"
	"strings"
)

var ErrUnknownFormat = errors.New("unable to detect coverage format")

// maxDetectLines bounds how many lines DetectFormat reads before giving up.
// A real LCOV or Cobertura file's first marker line shows up within the
// first few lines; this just keeps a large file with no marker at all from
// being scanned in full just to fail detection.
const maxDetectLines = 1000

// DetectFormat reports which coverage format r looks like, without fully
// parsing it. It scans r line by line -- rather than substring-matching
// the first 1024 bytes, which used to classify any text containing "SF:"
// anywhere in that prefix as LCOV and miss a real tracefile whose first
// record started past it -- looking for a line that starts with "SF:" or
// is exactly "end_of_record" (LCOV), or a line containing "<?xml" or
// "<coverage" (Cobertura). It consumes from r as it scans, so a caller
// that still needs the content afterward (e.g. to hand r to a parser)
// must rewind it first, the way cmd/litecov does with f.Seek(0, 0).
func DetectFormat(r io.Reader) (string, error) {
	scanner := bufio.NewScanner(r)
	// Some tools emit Cobertura XML as a single unindented line; the
	// scanner's default 64KB token limit can be too tight for a large
	// report, so give it more room before it gives up on a line.
	scanner.Buffer(make([]byte, 0, 64*1024), 1<<20)

	for lines := 0; lines < maxDetectLines && scanner.Scan(); lines++ {
		line := scanner.Text()

		if strings.Contains(line, "<?xml") || strings.Contains(line, "<coverage") {
			return "cobertura", nil
		}

		if strings.HasPrefix(line, "SF:") || line == "end_of_record" {
			return "lcov", nil
		}
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}

	return "", ErrUnknownFormat
}

func GetParser(format string) (Parser, error) {
	return GetParserWithPath(format, "")
}

// GetParserWithPath returns a parser for the given format, using the coverage
// file path to help resolve relative source paths.
func GetParserWithPath(format, coverageFilePath string) (Parser, error) {
	switch format {
	case "lcov":
		parser := &LCOVParser{}
		// Extract source prefix from coverage file path
		// e.g., "js/coverage/lcov.info" -> "js"
		if coverageFilePath != "" {
			parser.SourcePrefix = extractSourcePrefix(coverageFilePath)
		}
		return parser, nil
	case "cobertura", "xml":
		return &CoberturaParser{}, nil
	case "auto":
		return nil, nil
	default:
		return nil, errors.New("unknown format: " + format)
	}
}

// extractSourcePrefix extracts the source directory from a coverage file path.
// It looks for common coverage output directories and returns the path before them.
// e.g., "js/coverage/lcov.info" -> "js"
// e.g., "python/coverage.xml" -> "" (coverage file is in source dir)
func extractSourcePrefix(coveragePath string) string {
	// Common coverage output directories to look for
	coverageDirs := []string{"/coverage/", "/coverage-reports/", "/__coverage__/"}

	for _, dir := range coverageDirs {
		if idx := strings.Index(coveragePath, dir); idx > 0 {
			prefix := coveragePath[:idx]
			// Clean up the prefix
			prefix = strings.TrimPrefix(prefix, "./")
			if prefix != "" && prefix != "." {
				return prefix
			}
		}
	}

	// Also check if path starts with a directory that's not a common coverage dir
	dir := filepath.Dir(coveragePath)
	if dir != "" && dir != "." {
		base := filepath.Base(dir)
		// If the directory containing the file is a coverage dir, check parent
		if base == "coverage" || base == "__coverage__" {
			parentDir := filepath.Dir(dir)
			if parentDir != "" && parentDir != "." {
				return parentDir
			}
		}
	}

	return ""
}
