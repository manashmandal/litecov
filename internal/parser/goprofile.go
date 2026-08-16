package parser

import (
	"bufio"
	"errors"
	"io"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/manashmandal/litecov/internal/coverage"
)

// ErrNoGoCoverageData is returned when a parse produces no file with any
// block data: an empty profile, one with only a "mode:" line, or one whose
// every line fails to match the block pattern. Mirrors ErrNoCoverageData in
// lcov.go -- a zero-file result means the input wasn't really a Go coverage
// profile, not that it's a genuine 0% report.
var ErrNoGoCoverageData = errors.New("no coverage data found: input contains no valid Go coverage blocks")

// goProfileLineRE matches one block line of a Go coverage profile, e.g.
//
//	github.com/x/y/foo.go:10.20,12.3 2 1
//
// in file:startLine.startCol,endLine.endCol numStmt count order. (.+) is
// greedy, so it absorbs any colons the import-path-style filename itself
// contains and leaves the anchored position suffix to match against
// whatever's left -- the same approach golang.org/x/tools/cover uses to
// parse the format it defines.
var goProfileLineRE = regexp.MustCompile(`^(.+):(\d+)\.(\d+),(\d+)\.(\d+) (\d+) (\d+)$`)

// GoProfileParser parses the profile written by
// `go test -coverprofile=coverage.out`: a "mode: set|count|atomic" header
// line followed by one line per statement block naming the file, the
// block's line.col range, its statement count, and how many times it ran.
// See https://pkg.go.dev/cmd/cover for the format `go tool cover` itself
// implements.
type GoProfileParser struct{}

func (p *GoProfileParser) Parse(r io.Reader) (*coverage.Report, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1<<20)

	// lineHits tracks, per file, which line numbers a block touched and
	// whether any block covering that line ran. A block's line range is
	// exploded into individual line numbers below, and a line shared by
	// two blocks (e.g. a closing brace shared with the next statement)
	// ORs their hit status together -- a hit beats a miss, the same merge
	// LCOVParser uses for a line seen in more than one DA: record.
	lineHits := make(map[string]map[int]bool)
	// order preserves the order files were first seen, so the report
	// lists them in file order the way LCOVParser's fileIndex does,
	// rather than the random order map iteration would produce.
	var order []string

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "mode: ") {
			continue
		}

		m := goProfileLineRE.FindStringSubmatch(line)
		if m == nil {
			// A line that's neither the mode header nor a valid block
			// doesn't belong to this format at all -- skip it rather than
			// abort, the same tolerance LCOVParser gives a malformed DA:
			// or SF: row.
			continue
		}

		file := m[1]
		startLine, errStart := strconv.Atoi(m[2])
		endLine, errEnd := strconv.Atoi(m[4])
		count, errCount := strconv.Atoi(m[7])
		// The regexp already guarantees these are all-digit, so Atoi can
		// only fail here on overflow; still checked, plus the range must
		// name real lines, the same non-zero/non-inverted validation
		// DA: line numbers get in LCOVParser.
		if errStart != nil || errEnd != nil || errCount != nil || startLine < 1 || endLine < startLine {
			continue
		}

		hits, seen := lineHits[file]
		if !seen {
			hits = make(map[int]bool)
			lineHits[file] = hits
			order = append(order, file)
		}

		hit := count > 0
		for ln := startLine; ln <= endLine; ln++ {
			hits[ln] = hits[ln] || hit
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	report := &coverage.Report{}
	for _, file := range order {
		hits := lineHits[file]
		fc := coverage.FileCoverage{Path: file, LinesTotal: len(hits)}
		var uncovered []int
		for ln, hit := range hits {
			if hit {
				fc.LinesCovered++
			} else {
				uncovered = append(uncovered, ln)
			}
		}
		sort.Ints(uncovered)
		fc.UncoveredLines = uncovered
		report.Files = append(report.Files, fc)
	}

	report.Calculate()

	// See ErrNoGoCoverageData's doc comment: a parse that ends with no
	// files at all means the input was never a real Go coverage profile,
	// not that it's a genuine 0% report.
	if len(report.Files) == 0 {
		return nil, ErrNoGoCoverageData
	}

	return report, nil
}
