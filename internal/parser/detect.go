package parser

import (
	"bufio"
	"errors"
	"io"
	"path/filepath"
	"strings"
)

var ErrUnknownFormat = errors.New("unable to detect coverage format")

func DetectFormat(r io.Reader) (string, error) {
	buf := make([]byte, 1024)
	n, err := bufio.NewReader(r).Read(buf)
	if err != nil && err != io.EOF {
		return "", err
	}

	content := string(buf[:n])

	if strings.Contains(content, "<?xml") || strings.Contains(content, "<coverage") {
		return "cobertura", nil
	}

	if strings.Contains(content, "SF:") || strings.Contains(content, "end_of_record") {
		return "lcov", nil
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
