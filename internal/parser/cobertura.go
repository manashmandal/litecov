package parser

import (
	"encoding/xml"
	"io"
	"path/filepath"
	"sort"
	"strconv"
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

// Number and Hits are decoded as strings, not int: the Cobertura DTD types
// both as CDATA, and encoding/xml aborts the entire Decode call the moment
// one <line> anywhere in the report fails to parse as Go's int (see issue
// #57). Producers in the wild emit "1.0", "undefined" or an out-of-range
// value for either attribute, so they're parsed defensively in Parse below
// instead of trusted to the struct tag.
type coberturaLine struct {
	Number string `xml:"number,attr"`
	Hits   string `xml:"hits,attr"`
}

func (p *CoberturaParser) Parse(r io.Reader) (*coverage.Report, error) {
	var cov coberturaXML
	if err := xml.NewDecoder(r).Decode(&cov); err != nil {
		return nil, err
	}

	fileMap := make(map[string]*coverage.FileCoverage)
	// linesHit tracks, per resolved filename, whether each line number has
	// been hit by any <class> that reports it. One source file legitimately
	// maps to several <class> elements: every JVM class in a file gets its
	// own <class> with filename pointing at the shared source (an outer
	// class and its inner classes), and a linked or shared .NET source
	// compiled into more than one assembly produces the same shape. So the
	// same line number can appear more than once with a different hit
	// count per class. OR-ing the hit status across every occurrence -- a
	// hit beats a miss -- instead of keeping only the first one seen
	// matches how Codecov merges these (merge_line takes the larger of two
	// numeric coverage values); see issue #26. LinesTotal, LinesCovered and
	// UncoveredLines are derived from this map once every class has been
	// walked, in finalizeCoberturaFile below.
	linesHit := make(map[string]map[int]bool)

	for _, pkg := range cov.Packages {
		for _, class := range pkg.Classes {
			// A <class> with no filename attribute, or filename="", has no
			// file to attribute lines to. Without this check it falls
			// through to fileMap[""] below and merges with every other
			// filename-less class in the report (see issue #64). Codecov's
			// cobertura parser skips these classes outright; match that.
			if strings.TrimSpace(class.Filename) == "" {
				continue
			}
			// Resolve the filename using sources if available
			filename := resolveFilename(class.Filename, cov.Sources)

			fc, exists := fileMap[filename]
			if !exists {
				fc = &coverage.FileCoverage{Path: filename}
				fileMap[filename] = fc
				linesHit[filename] = make(map[int]bool)
			}
			for _, line := range class.Lines {
				lineNum, ok := parseCoberturaLineNumber(line.Number)
				if !ok {
					continue
				}
				hit := parseCoberturaHits(line.Hits)
				linesHit[filename][lineNum] = linesHit[filename][lineNum] || hit
			}
		}
	}

	report := &coverage.Report{}
	for _, fc := range fileMap {
		finalizeCoberturaFile(fc, linesHit[fc.Path])
		// A <class> with an empty <lines/> -- e.g. coverage.py reporting
		// line-rate="1" for a file with no statements, like an empty
		// __init__.py -- finalizes with LinesTotal == 0. Codecov drops these
		// instead of reporting them as a file (shared/reports/resources.py:
		// "dont append empty files"), so skip it here too rather than adding
		// a phantom 0% entry to report.Files; see issue #51. This mirrors
		// the empty-record skip LCOVParser already applies for the same
		// reason.
		if fc.LinesTotal == 0 {
			continue
		}
		report.Files = append(report.Files, *fc)
	}

	report.Calculate()
	return report, nil
}

// finalizeCoberturaFile derives fc's LinesTotal, LinesCovered and
// UncoveredLines from lines, the per-line hit map accumulated across every
// <class> that resolves to fc.Path (see the linesHit comment in Parse).
// Deriving the totals here, once, instead of incrementing them inside the
// class/line loop is what collapses a line number repeated across classes
// into a single entry instead of counting it once per class and letting an
// earlier class's miss shadow a later class's hit.
func finalizeCoberturaFile(fc *coverage.FileCoverage, lines map[int]bool) {
	covered := 0
	var uncovered []int
	for lineNum, hit := range lines {
		if hit {
			covered++
		} else {
			uncovered = append(uncovered, lineNum)
		}
	}
	sort.Ints(uncovered)

	fc.LinesTotal = len(lines)
	fc.LinesCovered = covered
	fc.UncoveredLines = uncovered
}

// parseCoberturaLineNumber parses a Cobertura <line> number attribute and
// reports whether it names a real source line. Lines are 1-indexed, so a
// missing attribute (decodes to ""), a non-numeric value like "undefined"
// (some coverage merge tools emit this), and a value that parses but isn't
// greater than 0 are all rejected here. Without this, a missing/invalid
// number and a genuine number="0" both fall through to the same line 0 and
// collide in the dedup below, silently dropping one of them.
func parseCoberturaLineNumber(raw string) (int, bool) {
	n, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || n <= 0 {
		return 0, false
	}
	return n, true
}

// parseCoberturaHits parses a Cobertura <line> hits attribute and reports
// whether the line counts as covered. hits is normally a plain integer, so
// strconv.Atoi is tried first; a value it rejects -- a float like "1.0", or
// an integer too large to fit an int -- is retried with strconv.ParseFloat,
// since only the sign matters here. ParseFloat saturates an out-of-range
// magnitude to +/-Inf rather than erroring, and returns 0 for a value that
// isn't a number at all (including ""), so its result can be compared
// against 0 without inspecting the error: an empty or unparseable hits
// attribute ends up not covered instead of aborting the report.
func parseCoberturaHits(raw string) bool {
	raw = strings.TrimSpace(raw)
	if n, err := strconv.Atoi(raw); err == nil {
		return n > 0
	}
	f, _ := strconv.ParseFloat(raw, 64)
	return f > 0
}

// resolveFilename resolves a filename from coverage data using the sources list.
// For pytest-cov, filenames are relative to the source directories.
// This function attempts to create a meaningful relative path.
func resolveFilename(filename string, sources []string) string {
	// If filename is already absolute, try to make it relative using sources
	if filepath.IsAbs(filename) {
		filename = filepath.Clean(filename)
		bestSourceLen := -1
		bestRel := ""
		for _, source := range sources {
			source = strings.TrimSpace(source)
			if source == "" {
				continue
			}
			source = filepath.Clean(source)
			rel, ok := trimSourceDir(filename, source)
			if !ok {
				continue
			}
			// Prefer the longest (most specific) matching source. With
			// nested roots like "/build/src" and "/build/src/MyApp" both
			// configured, the deeper one is the one that actually produced
			// this file; returning on the first match instead let a
			// shallower parent win and left the result still carrying the
			// inner directory, e.g. "MyApp/Real.cs" instead of "Real.cs".
			// See issue #50.
			if len(source) > bestSourceLen {
				bestSourceLen = len(source)
				bestRel = rel
			}
		}
		if bestSourceLen >= 0 {
			return bestRel
		}
		// Couldn't resolve with sources, return as-is
		return filename
	}

	// For relative filenames (common in pytest-cov), we can prepend source info
	// if it helps identify the path structure. A relative filename carries no
	// link back to which <source> produced it, so this only guesses when every
	// source agrees on the same project-relative path. With two roots like
	// "src" and "lib" (coverage.py --source=src,lib, or a `coverage combine`
	// across roots), a class that actually lives under the second root used to
	// get the first root's prefix regardless -- misreporting its location and
	// breaking the suffix match in internal/paths, so the file silently
	// disappeared from the comment and from annotations even when the PR
	// touched it. See issue #41. resolveAgreedProjectPath returns "" the
	// moment two sources disagree, and the bare filename below still lets
	// that suffix match succeed against whichever file it really is, unlike
	// guessing a prefix that names the wrong directory.
	if projectPath := resolveAgreedProjectPath(sources); projectPath != "" {
		return filepath.Join(projectPath, filename)
	}

	return filename
}

// resolveAgreedProjectPath returns the project-relative path every source in
// sources extracts to via extractProjectPath, or "" if the sources disagree
// (or none of them yield one). A source that extractProjectPath can't read
// anything from is skipped rather than treated as a disagreement -- it has
// no opinion either way.
func resolveAgreedProjectPath(sources []string) string {
	agreed := ""
	for _, source := range sources {
		source = strings.TrimSpace(source)
		if source == "" {
			continue
		}
		candidate := extractProjectPath(source)
		if candidate == "" || candidate == "/" {
			continue
		}
		if agreed == "" {
			agreed = candidate
		} else if candidate != agreed {
			return ""
		}
	}
	return agreed
}

// trimSourceDir reports whether filename lives inside the directory source
// and, if so, returns filename's path relative to source. filename and
// source are assumed already filepath.Clean'd.
//
// The match must land on a full path-segment boundary -- source itself
// followed by "/" -- not just a shared string prefix: a source of
// "/build/src" must match "/build/src/MyApp/Real.cs" but not a same-named
// sibling like "/build/srcgen/Generated.cs". A plain
// strings.HasPrefix(filename, source) allowed exactly that, silently eating
// the first few characters of the sibling directory's name; see issue #50.
func trimSourceDir(filename, source string) (rel string, ok bool) {
	if source == "/" {
		if len(filename) <= 1 {
			return "", false
		}
		return filename[1:], true
	}
	if !strings.HasPrefix(filename, source) || len(filename) == len(source) {
		return "", false
	}
	if filename[len(source)] != '/' {
		return "", false
	}
	return filename[len(source)+1:], true
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
