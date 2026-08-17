package parser

import (
	"bufio"
	"errors"
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

// ErrNoCoverageData is returned when a parse produces no file with any line
// data: content that isn't LCOV at all (a Go coverage.out, Cobertura XML, an
// empty file, or plain text that DetectFormat's "SF:" heuristic misclassifies
// as LCOV) or an LCOV tracefile whose only SF: records are empty. Codecov
// treats this as a processing error rather than a real result --
// services/report/__init__.py catches ReportEmptyError and records
// UploadErrorCode.REPORT_EMPTY instead of letting the coverage number fall
// to zero -- so the caller should reject the report instead of publishing it
// as 0% coverage. A tracefile that legitimately covers zero lines still
// produces file entries with LinesTotal > 0 (see the end_of_record and
// trailing-record handling below), so it stays distinguishable from this.
var ErrNoCoverageData = errors.New("no coverage data found: input contains no valid LCOV records")

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
	var currentBranches map[int]map[string]bool
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
			currentBranches = make(map[int]map[string]bool)

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

		case strings.HasPrefix(line, "BRDA:"):
			if current == nil {
				continue
			}
			parts := strings.Split(strings.TrimPrefix(line, "BRDA:"), ",")
			if len(parts) >= 4 {
				lineNum, err := strconv.Atoi(parts[0])
				// Same line-number validation as DA: above -- a malformed or
				// non-positive line number names no real line.
				if err != nil || lineNum < 1 {
					continue
				}
				// block:branch identifies one branch of the line. taken is
				// "-" when the branch was never reached (dead code, e.g. an
				// else arm that can't execute) or a hit count otherwise;
				// Codecov's LCOV processor treats both "-" and "0" as not
				// taken, so only a positive count counts as taken here.
				branchID := strings.TrimSpace(parts[1]) + ":" + strings.TrimSpace(parts[2])
				taken := false
				if raw := strings.TrimSpace(parts[3]); raw != "-" {
					hit, ok := parseExecutionCount(raw)
					// An unparseable taken count can't be classified either
					// way, so the record is dropped instead of guessed at --
					// same as an unparseable DA: hit count above.
					if !ok {
						continue
					}
					taken = hit
				}
				if currentBranches[lineNum] == nil {
					currentBranches[lineNum] = make(map[string]bool)
				}
				// A block:branch pair can repeat across shards the same way
				// a DA: line can; OR it into what's already been seen for
				// this branch in this record, a taken beats a not-taken.
				currentBranches[lineNum][branchID] = currentBranches[lineNum][branchID] || taken
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
				applyBranchCoverage(currentLines, currentBranches)
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
				currentBranches = nil
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	// Flush a trailing record that has no closing end_of_record, e.g. a
	// tracefile truncated by a killed test run or a cut-short upload.
	if current != nil {
		applyBranchCoverage(currentLines, currentBranches)
		finalizeRecord(current, currentLines)
		// See the empty-record comment on the end_of_record case above --
		// a record with no line data is dropped rather than merged.
		if current.LinesTotal > 0 {
			mergeFileRecord(report, fileIndex, lineHits, current, currentLines)
		}
	}

	report.Calculate()

	// See ErrNoCoverageData's doc comment: a parse that ends with no files at
	// all means the input was never a real LCOV tracefile, not that it's a
	// genuine 0% report.
	if len(report.Files) == 0 {
		return nil, ErrNoCoverageData
	}

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

// applyBranchCoverage folds branches, the per-line block:branch taken map
// built while scanning a record's BRDA: records, into lines, the same
// record's DA: hit map -- before finalizeRecord and mergeFileRecord derive
// LinesTotal/LinesCovered/UncoveredLines from lines, so both the
// single-record and the merged-duplicate-SF: path pick up the branch data
// without needing their own copy of this logic. A DA: line that ran but
// left one of its branches untaken is downgraded from hit to not-hit here;
// see issue #63. A BRDA: line number with no matching DA: entry has
// nothing to downgrade and is left alone.
func applyBranchCoverage(lines map[int]bool, branches map[int]map[string]bool) {
	for lineNum, branchHits := range branches {
		hit, ok := lines[lineNum]
		if !ok {
			continue
		}
		lines[lineNum] = lineHit(hit, branchHits)
	}
}

// lineHit reports whether a line counts as a clean covered hit once its
// branches are accounted for. This mirrors Codecov's LCOV processor
// (services/report/languages/lcov.py), which replaces a branch line's
// coverage with its taken/total branch fraction: a line is only as
// covered as its least-covered branch, so any branch left untaken (a
// BRDA: taken field of "-" or "0") demotes the line even though its DA:
// record shows it executed. A line with no branch data falls back to its
// plain DA: hit status unchanged.
func lineHit(daHit bool, branches map[string]bool) bool {
	if len(branches) == 0 {
		return daHit
	}
	for _, taken := range branches {
		if !taken {
			return false
		}
	}
	return true
}

// finalizeRecord derives rec's LinesTotal, LinesCovered, UncoveredLines and
// CoveredLines from lines, the per-line hit map built while scanning rec's
// DA: records. This is what collapses a line repeated across several DA:
// records in the same block down to one entry instead of the raw per-record
// counts, which double count the line in LinesTotal and can still report it
// as uncovered after a later record hit it. The DA: records are the only
// source for these totals -- see the LF:/LH: comment above -- so there's
// nothing else to reconcile them against.
//
// CoveredLines exists alongside UncoveredLines so a caller can intersect a
// PR diff's added lines against both to compute patch coverage: the added
// lines that landed in CoveredLines are what's tested, and the union of the
// two is what's coverable at all (issue #6).
func finalizeRecord(rec *coverage.FileCoverage, lines map[int]bool) {
	covered := 0
	var uncovered []int
	var coveredLines []int
	for lineNum, hit := range lines {
		if hit {
			covered++
			coveredLines = append(coveredLines, lineNum)
		} else {
			uncovered = append(uncovered, lineNum)
		}
	}
	sort.Ints(uncovered)
	sort.Ints(coveredLines)

	rec.LinesTotal = len(lines)
	rec.LinesCovered = covered
	rec.UncoveredLines = uncovered
	rec.CoveredLines = coveredLines
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
	var coveredLines []int
	for lineNum, covered := range merged {
		if covered {
			linesCovered++
			coveredLines = append(coveredLines, lineNum)
		} else {
			uncovered = append(uncovered, lineNum)
		}
	}
	sort.Ints(uncovered)
	sort.Ints(coveredLines)

	existing := &report.Files[idx]
	existing.LinesTotal = len(merged)
	existing.LinesCovered = linesCovered
	existing.UncoveredLines = uncovered
	existing.CoveredLines = coveredLines
}
