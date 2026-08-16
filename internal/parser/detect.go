package parser

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
)

var ErrUnknownFormat = errors.New("unable to detect coverage format")

// ErrUnsupportedFormat is returned when DetectFormat recognizes the input as
// a coverage report but not one litecov has a parser for. Unlike
// ErrUnknownFormat -- "this isn't any coverage format we recognize" -- this
// means "we know what this is, litecov just doesn't read it": Clover XML
// (PHPUnit, and the clover reporter in Jest/Vitest) and JaCoCo's native XML
// both use "<coverage>" or their own root element the way Cobertura does,
// and Codecov routes each to its own processor (clover.py, jacoco.py)
// instead of the Cobertura one. Litecov has no equivalent processors, so it
// reports the mismatch by name instead of guessing Cobertura (see issue
// #71).
var ErrUnsupportedFormat = errors.New("litecov only parses lcov and cobertura")

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
// is exactly "end_of_record" (LCOV), a line containing "<coverage" with
// no clover= attribute (Cobertura), or a line starting with "mode: "
// (a Go coverage profile's required header, e.g. "mode: set"; Codecov's
// own Go processor keys on the same prefix -- services/report/languages/
// go.py). A bare "<?xml" declaration used to be treated as a Cobertura
// signal on its own, but every XML dialect opens with one, so that
// matched Clover and JaCoCo input just as readily as real Cobertura and
// routed both into CoberturaParser. A "<coverage clover=...>" line
// (Clover) or a line containing "<report" (JaCoCo) is now recognized as
// XML litecov doesn't parse and reported as ErrUnsupportedFormat naming
// what was found, instead of being guessed as Cobertura. It consumes
// from r as it scans, so a caller that still needs the content
// afterward (e.g. to hand r to a parser) must rewind it first, the way
// cmd/litecov does with f.Seek(0, 0).
func DetectFormat(r io.Reader) (string, error) {
	scanner := bufio.NewScanner(r)
	// Some tools emit Cobertura XML as a single unindented line; the
	// scanner's default 64KB token limit can be too tight for a large
	// report, so give it more room before it gives up on a line.
	scanner.Buffer(make([]byte, 0, 64*1024), 1<<20)

	for lines := 0; lines < maxDetectLines && scanner.Scan(); lines++ {
		line := scanner.Text()

		if strings.Contains(line, "<coverage") {
			if strings.Contains(line, "clover=") {
				return "", fmt.Errorf("detected Clover XML (root <coverage clover=...>): %w", ErrUnsupportedFormat)
			}
			return "cobertura", nil
		}

		if strings.Contains(line, "<report") {
			return "", fmt.Errorf("detected JaCoCo XML (root <report>): %w", ErrUnsupportedFormat)
		}

		if strings.HasPrefix(line, "SF:") || line == "end_of_record" {
			return "lcov", nil
		}

		if strings.HasPrefix(line, "mode: ") {
			return "go", nil
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
	case "go":
		return &GoProfileParser{}, nil
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
