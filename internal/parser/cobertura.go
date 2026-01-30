package parser

import (
	"encoding/xml"
	"io"
	"path/filepath"
	"strings"

	"github.com/manashmandal/litecov/internal/coverage"
)

type CoberturaParser struct{}

type coberturaXML struct {
	XMLName  xml.Name           `xml:"coverage"`
	Sources  []string           `xml:"sources>source"`
	Packages []coberturaPackage `xml:"packages>package"`
}

type coberturaPackage struct {
	Name    string           `xml:"name,attr"`
	Classes []coberturaClass `xml:"classes>class"`
}

type coberturaClass struct {
	Name     string          `xml:"name,attr"`
	Filename string          `xml:"filename,attr"`
	Lines    []coberturaLine `xml:"lines>line"`
}

type coberturaLine struct {
	Number int `xml:"number,attr"`
	Hits   int `xml:"hits,attr"`
}

func (p *CoberturaParser) Parse(r io.Reader) (*coverage.Report, error) {
	var cov coberturaXML
	if err := xml.NewDecoder(r).Decode(&cov); err != nil {
		return nil, err
	}

	fileMap := make(map[string]*coverage.FileCoverage)
	linesSeen := make(map[string]map[int]bool)

	for _, pkg := range cov.Packages {
		for _, class := range pkg.Classes {
			// Resolve the filename using sources if available
			filename := resolveFilename(class.Filename, cov.Sources)

			fc, exists := fileMap[filename]
			if !exists {
				fc = &coverage.FileCoverage{Path: filename}
				fileMap[filename] = fc
				linesSeen[filename] = make(map[int]bool)
			}
			for _, line := range class.Lines {
				// Skip duplicate lines (same line number seen in multiple classes)
				if linesSeen[filename][line.Number] {
					continue
				}
				linesSeen[filename][line.Number] = true
				fc.LinesTotal++
				if line.Hits > 0 {
					fc.LinesCovered++
				} else {
					fc.UncoveredLines = append(fc.UncoveredLines, line.Number)
				}
			}
		}
	}

	report := &coverage.Report{}
	for _, fc := range fileMap {
		report.Files = append(report.Files, *fc)
	}

	report.Calculate()
	return report, nil
}

// resolveFilename resolves a filename from coverage data using the sources list.
// For pytest-cov, filenames are relative to the source directories.
// This function attempts to create a meaningful relative path.
func resolveFilename(filename string, sources []string) string {
	// If filename is already absolute, try to make it relative using sources
	if filepath.IsAbs(filename) {
		for _, source := range sources {
			source = strings.TrimSpace(source)
			if source == "" {
				continue
			}
			// If filename starts with source, extract relative path
			if strings.HasPrefix(filename, source) {
				rel := strings.TrimPrefix(filename, source)
				rel = strings.TrimPrefix(rel, "/")
				if rel != "" {
					return rel
				}
			}
		}
		// Couldn't resolve with sources, return as-is
		return filename
	}

	// For relative filenames (common in pytest-cov), we can prepend source info
	// if it helps identify the path structure
	if len(sources) > 0 {
		source := strings.TrimSpace(sources[0])
		if source != "" {
			// Try to extract a meaningful project-relative path from the source
			// e.g., source="/home/runner/work/repo/src" -> we want paths relative to repo root
			projectPath := extractProjectPath(source)
			if projectPath != "" && projectPath != "/" {
				return filepath.Join(projectPath, filename)
			}
		}
	}

	return filename
}

// extractProjectPath attempts to extract a project-relative path from a source directory.
// e.g., "/home/runner/work/myrepo/myrepo/src" might return "src"
// e.g., "/home/runner/work/myrepo/myrepo/python" might return "python"
func extractProjectPath(source string) string {
	// GitHub Actions workspace pattern: /home/runner/work/{repo}/{repo}/...
	// The path after the repeated repo name is relative to repo root
	parts := strings.Split(source, "/")
	for i := 0; i < len(parts)-1; i++ {
		// Look for repeated directory name (repo name appears twice in GHA)
		if parts[i] != "" && parts[i] == parts[i+1] {
			// Everything after the second occurrence is repo-relative
			if i+2 < len(parts) {
				return strings.Join(parts[i+2:], "/")
			}
		}
	}

	// Common Python project markers
	markers := []string{"/src/", "/lib/", "/app/", "/tests/", "/test/", "/python/"}
	for _, marker := range markers {
		if idx := strings.LastIndex(source, marker); idx >= 0 {
			return source[idx+1:] // Return everything after the slash before marker
		}
	}

	// Check if source ends with a known directory
	base := filepath.Base(source)
	knownDirs := []string{"src", "lib", "app", "tests", "test", "python", "py"}
	for _, dir := range knownDirs {
		if base == dir {
			return base
		}
	}

	return ""
}
