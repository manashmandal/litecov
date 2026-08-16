package parser

import (
	"bufio"
	"io"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/manashmandal/litecov/internal/coverage"
)

type LCOVParser struct {
	// SourcePrefix is prepended to file paths if set
	SourcePrefix string
}

func (p *LCOVParser) Parse(r io.Reader) (*coverage.Report, error) {
	report := &coverage.Report{}
	scanner := bufio.NewScanner(r)

	// fileIndex tracks the position in report.Files that each path was
	// first inserted at, so a repeated SF: record for the same path -- the
	// normal shape of a tracefile built by concatenating per-shard or
	// per-suite lcov output -- merges into that entry instead of being
	// appended as a duplicate. lineHits tracks, per path, which line
	// numbers have been seen and whether any record hit them, which is
	// what the merge is computed from.
	fileIndex := make(map[string]int)
	lineHits := make(map[string]map[int]bool)

	var current *coverage.FileCoverage
	var currentLines map[int]bool
	firstLine := true

	for scanner.Scan() {
		text := scanner.Text()
		if firstLine {
			// A UTF-8 BOM isn't whitespace, so TrimSpace below won't strip
			// it. Left in place it hides in front of the first record's
			// "SF:" prefix and that record silently never matches.
			text = strings.TrimPrefix(text, "\uFEFF")
			firstLine = false
		}
		line := strings.TrimSpace(text)
		if line == "" {
			continue
		}

		switch {
		case strings.HasPrefix(line, "SF:"):
			filePath := strings.TrimPrefix(line, "SF:")
			// If path is relative and we have a source prefix, prepend it
			if p.SourcePrefix != "" && !filepath.IsAbs(filePath) {
				filePath = filepath.Join(p.SourcePrefix, filePath)
			}
			current = &coverage.FileCoverage{
				Path: filePath,
			}
			currentLines = make(map[int]bool)

		case strings.HasPrefix(line, "DA:"):
			if current == nil {
				continue
			}
			parts := strings.Split(strings.TrimPrefix(line, "DA:"), ",")
			if len(parts) >= 2 {
				lineNum, err := strconv.Atoi(parts[0])
				// The lcov tracefile format defines the DA: line number as a
				// non-zero integer. Atoi returns 0 on a parse failure, which
				// -- left unchecked -- lands a non-numeric line right next to
				// a literal "0" as the same bogus line-0 entry: it inflates
				// LinesTotal for data that names no real line, and if unhit
				// it surfaces as a line=0 GitHub annotation that points at
				// nothing. Skip the record instead of counting it.
				if err != nil || lineNum < 1 {
					continue
				}
				hit, ok := parseExecutionCount(parts[1])
				// strconv.Atoi rejects scientific notation (some gcov and JS
				// toolchains emit "1e+06" for very high counts) and whitespace
				// around the value (tracefiles assembled by concatenating
				// shards can carry it). Discarding that error, as this used to,
				// defaulted the count to 0 and recorded a line that actually ran
				// as a miss. Skip the record instead when it still can't parse.
				if !ok {
					continue
				}
				// A line can appear in more than one DA: record within the
				// same SF:/end_of_record block, e.g. a tracefile produced
				// by concatenating per-suite runs before upload. OR the hit
				// into whatever this line has already seen in this record
				// -- one map entry per line, a hit beats a miss -- instead
				// of treating every DA: as a distinct line. LinesTotal,
				// LinesCovered and UncoveredLines are derived from this map
				// at end_of_record so a repeated line is counted once.
				currentLines[lineNum] = currentLines[lineNum] || hit
			}

		// LF: and LH: are lcov's own summary of the DA: records in this
		// block, not an independent source of truth -- they're derived the
		// same way finalizeRecord derives LinesTotal/LinesCovered below, and
		// a tracefile that concatenates shards can leave them stale or
		// wrong. Trusting them over the DA: records they summarize lets the
		// reported totals disagree with the line list the same parse
		// produced, so they're ignored here the same way TN:, FNF:, FNH:,
		// BRF: and BRH: already are.

		case line == "end_of_record":
			if current != nil {
				finalizeRecord(current, currentLines)
				// An SF: record with no usable DA: rows -- a file excluded by
				// an ignore pattern, a generated file, a header, or a record
				// whose DA: rows were all rejected as malformed -- finalizes
				// with LinesTotal == 0. Codecov drops these instead of
				// reporting them as a file (shared/reports/resources.py:
				// "dont append empty files"), so skip the merge here too
				// rather than adding a phantom 0% entry to report.Files.
				if current.LinesTotal > 0 {
					mergeFileRecord(report, fileIndex, lineHits, current, currentLines)
				}
				current = nil
				currentLines = nil
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	// Flush a trailing record that has no closing end_of_record, e.g. a
	// tracefile truncated by a killed test run or a cut-short upload.
	if current != nil {
		finalizeRecord(current, currentLines)
		// See the empty-record comment on the end_of_record case above --
		// a record with no line data is dropped rather than merged.
		if current.LinesTotal > 0 {
			mergeFileRecord(report, fileIndex, lineHits, current, currentLines)
		}
	}

	report.Calculate()
	return report, nil
}

// parseExecutionCount parses the hit count field of a DA: record and
// reports whether it denotes a line that ran at least once. The field is
// normally a plain integer, but some gcov and JS toolchains emit
// scientific notation (e.g. "1e+06") for very high counts, and a
// tracefile assembled by concatenating shards can leave whitespace
// around the value, so raw is trimmed and, when strconv.Atoi rejects it,
// retried with strconv.ParseFloat. Only the sign of the count matters to
// the caller, so float64's precision loss on enormous values isn't a
// concern. ok is false only when neither parse succeeds, so the caller
// can skip the record rather than guess at whether the line ran.
func parseExecutionCount(raw string) (hit bool, ok bool) {
	raw = strings.TrimSpace(raw)
	if n, err := strconv.Atoi(raw); err == nil {
		return n > 0, true
	}
	f, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return false, false
	}
	return f > 0, true
}

// finalizeRecord derives rec's LinesTotal, LinesCovered and UncoveredLines
// from lines, the per-line hit map built while scanning rec's DA: records.
// This is what collapses a line repeated across several DA: records in the
// same block down to one entry instead of the raw per-record counts, which
// double count the line in LinesTotal and can still report it as uncovered
// after a later record hit it. The DA: records are the only source for these
// totals -- see the LF:/LH: comment above -- so there's nothing else to
// reconcile them against.
func finalizeRecord(rec *coverage.FileCoverage, lines map[int]bool) {
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

	rec.LinesTotal = len(lines)
	rec.LinesCovered = covered
	rec.UncoveredLines = uncovered
}

// mergeFileRecord adds rec to report.Files, or, if a prior record already
// covered rec.Path, folds rec's line data into that entry instead of
// appending a duplicate. lines is the covered/uncovered status of each DA:
// line number seen in rec, keyed by line number. A line counts as covered
// in the merged result if any record hit it, matching how lcov -a combines
// tracefiles and how Codecov's LCOV processor merges same-named files.
func mergeFileRecord(report *coverage.Report, fileIndex map[string]int, lineHits map[string]map[int]bool, rec *coverage.FileCoverage, lines map[int]bool) {
	idx, seen := fileIndex[rec.Path]
	if !seen {
		fileIndex[rec.Path] = len(report.Files)
		lineHits[rec.Path] = lines
		report.Files = append(report.Files, *rec)
		return
	}

	merged := lineHits[rec.Path]
	for lineNum, covered := range lines {
		merged[lineNum] = merged[lineNum] || covered
	}

	linesCovered := 0
	var uncovered []int
	for lineNum, covered := range merged {
		if covered {
			linesCovered++
		} else {
			uncovered = append(uncovered, lineNum)
		}
	}
	sort.Ints(uncovered)

	existing := &report.Files[idx]
	existing.LinesTotal = len(merged)
	existing.LinesCovered = linesCovered
	existing.UncoveredLines = uncovered
}
