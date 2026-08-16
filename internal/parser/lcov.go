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
				lineNum, _ := strconv.Atoi(parts[0])
				hits, _ := strconv.Atoi(parts[1])
				current.LinesTotal++
				if hits > 0 {
					current.LinesCovered++
					currentLines[lineNum] = true
				} else {
					current.UncoveredLines = append(current.UncoveredLines, lineNum)
					if !currentLines[lineNum] {
						currentLines[lineNum] = false
					}
				}
			}

		case strings.HasPrefix(line, "LF:"):
			if current != nil {
				lf, _ := strconv.Atoi(strings.TrimPrefix(line, "LF:"))
				if lf > 0 && current.LinesTotal != lf {
					current.LinesTotal = lf
				}
			}

		case strings.HasPrefix(line, "LH:"):
			if current != nil {
				lh, _ := strconv.Atoi(strings.TrimPrefix(line, "LH:"))
				if lh > 0 && current.LinesCovered != lh {
					current.LinesCovered = lh
				}
			}

		case line == "end_of_record":
			if current != nil {
				mergeFileRecord(report, fileIndex, lineHits, current, currentLines)
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
		mergeFileRecord(report, fileIndex, lineHits, current, currentLines)
	}

	report.Calculate()
	return report, nil
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
