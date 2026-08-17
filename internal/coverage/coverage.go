package coverage

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/manashmandal/litecov/internal/diff"
	"github.com/manashmandal/litecov/internal/paths"
)

type FileCoverage struct {
	Path           string
	LinesCovered   int
	LinesTotal     int
	UncoveredLines []int
	CoveredLines   []int
}

func (fc *FileCoverage) Percentage() float64 {
	if fc.LinesTotal == 0 {
		return 0
	}
	return float64(fc.LinesCovered) / float64(fc.LinesTotal) * 100
}

type Report struct {
	Files        []FileCoverage
	TotalCovered int
	TotalLines   int
	Coverage     float64
}

func (r *Report) Calculate() {
	r.TotalCovered = 0
	r.TotalLines = 0
	for _, f := range r.Files {
		r.TotalCovered += f.LinesCovered
		r.TotalLines += f.LinesTotal
	}
	if r.TotalLines == 0 {
		r.Coverage = 0
		return
	}
	r.Coverage = float64(r.TotalCovered) / float64(r.TotalLines) * 100
}

func (r *Report) Hits() int {
	return r.TotalCovered
}

func (r *Report) Misses() int {
	return r.TotalLines - r.TotalCovered
}

// Comparison holds the result of comparing head vs base coverage
type Comparison struct {
	Head          *Report
	Base          *Report
	CoverageDelta float64
	FileChanges   []FileChange
	// NoBaseFiles is true when Base is non-nil but parsed to zero files (a
	// truncated LCOV, a Cobertura with no packages). CoverageDelta is left
	// unset in that case: Base.Coverage falls back to 0 the same way a
	// single file's Percentage() does for LinesTotal == 0, and computing
	// head.Coverage - 0 would report every point of head coverage as
	// improvement over a base that was never actually measured (issue #32).
	NoBaseFiles bool
}

// FileChange represents coverage change for a single file
type FileChange struct {
	Path         string
	HeadCoverage float64
	BaseCoverage float64
	Delta        float64
	// IsNew is true when GitHub's PR diff reports this file's status as
	// "added". It comes from the diff, not from the file's absence in the
	// base coverage report: a file can be missing from the base report
	// because the base run was partial, crashed, or excluded the file, none
	// of which mean the PR added it (issue #32).
	IsNew bool
	// IsDeleted is true when GitHub's PR diff reports this file's status as
	// "removed". HeadCoverage stays at its zero-value sentinel because the
	// file no longer exists at head, not because a still-present file's
	// coverage data went missing: that's what NoCoverage is for, and
	// conflating the two rendered a deleted file as an untested 0% (issue
	// #31).
	IsDeleted    bool
	NoCoverage   bool // True if the file is absent from the coverage report entirely: coverage is unknown, not 0%
	NoStatements bool // True if the file has no coverable lines (LinesTotal == 0): coverage is undefined, not 0%
	// NoBaseData is true when there is no entry for this file in the base
	// report, whether because base is nil, because the base report has no
	// files, or because this path just never matched one. BaseCoverage and
	// Delta stay at their zero-value sentinel in that case: computing
	// HeadCoverage - 0 would describe an unmeasured base as if it were a
	// real 0% (issue #32).
	NoBaseData bool
}

// NewComparison creates a comparison between head and base reports.
// changedFiles is an optional list of file paths that changed in the PR.
// addedFiles is an optional set of paths that GitHub's PR diff reports as
// newly added, used for IsNew (see its doc comment for why that isn't
// inferred from base-report absence). removedFiles is the same for paths
// the diff reports as removed, used for IsDeleted.
func NewComparison(head, base *Report, changedFiles []string, addedFiles, removedFiles map[string]bool) *Comparison {
	if head == nil {
		return &Comparison{}
	}

	comp := &Comparison{
		Head: head,
		Base: base,
	}

	if base != nil {
		if len(base.Files) > 0 {
			comp.CoverageDelta = head.Coverage - base.Coverage
		} else {
			// A present-but-empty base report (a truncated LCOV, a
			// Cobertura with no packages) parses to zero files, and
			// base.Coverage falls back to 0 the same way Percentage() does
			// for a single file with LinesTotal == 0. head.Coverage - 0
			// would report every point of head coverage as improvement over
			// a base that was never actually measured (issue #32).
			comp.NoBaseFiles = true
		}
	}

	baseFileMap := make(map[string]*FileCoverage)
	if base != nil {
		for i := range base.Files {
			baseFileMap[base.Files[i].Path] = &base.Files[i]
		}
	}

	changedFileSet := make(map[string]bool)
	for _, f := range changedFiles {
		changedFileSet[f] = true
	}

	filterByChanged := len(changedFiles) > 0

	// Track which changed files we've seen in coverage data
	coveredChangedFiles := make(map[string]bool)
	// Track which base files got attached to a head file in the loop below,
	// exact match or suffix fallback, so the base-only pass further down
	// doesn't also report a suffix-matched file as missing from head: the
	// same prefix mismatch that broke the forward base lookup broke that
	// reverse check too, since it also compared paths for exact equality
	// (issue #30).
	matchedBasePaths := make(map[string]bool)

	for _, headFile := range head.Files {
		matchedChangedFile := ""
		if filterByChanged {
			matchedChangedFile = paths.FindMatchingChangedFile(headFile.Path, changedFileSet)
			if matchedChangedFile == "" {
				continue
			}
			coveredChangedFiles[matchedChangedFile] = true
		}

		filePath := headFile.Path
		if matchedChangedFile != "" {
			filePath = matchedChangedFile
		}

		fc := FileChange{
			Path:         filePath,
			HeadCoverage: headFile.Percentage(),
			// headFile.Percentage() falls back to 0 for LinesTotal == 0, so
			// this is what lets the comparison table tell "nothing to
			// measure" apart from "measured, nothing covered" (issue #35).
			NoStatements: headFile.LinesTotal == 0,
			IsNew:        addedFiles[filePath],
		}

		baseFile, exists := baseFileMap[headFile.Path]
		if !exists {
			// Exact match failed: base and head reports can use different
			// path prefixes (an absolute vs. relative path, a different
			// runner workspace, a different Cobertura <sources> element, a
			// base produced by another tool), so fall back to the same
			// suffix matching the rest of the pipeline already tolerates
			// (paths.FindMatchingChangedFile, findFileInReport) instead of
			// treating a file that merely changed prefix as having no base
			// data at all (issue #30).
			if bf := findBaseFileBySuffix(base, headFile.Path); bf != nil {
				baseFile, exists = bf, true
			}
		}
		if exists {
			fc.BaseCoverage = baseFile.Percentage()
			fc.Delta = fc.HeadCoverage - fc.BaseCoverage
			matchedBasePaths[baseFile.Path] = true
		} else {
			// No base entry for this path, exact or suffix-matched:
			// BaseCoverage and Delta stay at their zero-value sentinel
			// rather than computing HeadCoverage - 0, which would assert a
			// measurement that was never taken (issue #32).
			fc.NoBaseData = true
		}

		comp.FileChanges = append(comp.FileChanges, fc)
	}

	// Add changed files that have no entry in the coverage report at all.
	// HeadCoverage is left at its zero value as a sentinel, same as
	// NoStatements above; NoCoverage is what callers must check before
	// treating it as a real 0% measurement (issue #34).
	if filterByChanged {
		for _, changedFile := range changedFiles {
			if coveredChangedFiles[changedFile] {
				continue
			}
			// Only include source files that should have coverage
			if !paths.IsSourceFile(changedFile) {
				continue
			}
			fc := FileChange{
				Path:         changedFile,
				HeadCoverage: 0,
				BaseCoverage: 0,
				Delta:        0,
				IsNew:        addedFiles[changedFile],
			}
			if removedFiles[changedFile] {
				// GitHub's diff confirms the PR removed this file: there is
				// no head coverage because the file is gone, not because a
				// still-present file's coverage data is missing. NoCoverage's
				// "unknown" would say the same thing about a deleted file as
				// about one that's still there but wasn't measured (issue
				// #31).
				fc.IsDeleted = true
			} else {
				fc.NoCoverage = true
			}
			// Check if file existed in base
			if baseFile := findFileInReport(base, changedFile); baseFile != nil {
				fc.BaseCoverage = baseFile.Percentage()
				fc.Delta = -fc.BaseCoverage
			} else {
				fc.NoBaseData = true
			}
			comp.FileChanges = append(comp.FileChanges, fc)
		}
	}

	// A file present in base but absent from head produces no FileChange
	// from either loop above unless it's also in changedFiles, so its
	// coverage silently vanishes from the comment instead of explaining
	// where it went (issue #31). Skipped when filtering by changed files: a
	// base-only file the PR's diff never touched is out of scope for a
	// "changed files" view, the same reasoning issue #20 applied to not
	// falling back to every file in the report.
	if base != nil && !filterByChanged {
		for i := range base.Files {
			baseFile := &base.Files[i]
			if matchedBasePaths[baseFile.Path] {
				continue
			}
			comp.FileChanges = append(comp.FileChanges, FileChange{
				Path:         baseFile.Path,
				BaseCoverage: baseFile.Percentage(),
				Delta:        -baseFile.Percentage(),
				NoCoverage:   true,
			})
		}
	}

	return comp
}

// findBaseFileBySuffix falls back to suffix matching when headPath has no
// exact entry in base's file map: a prefix difference between the two
// reports (absolute vs. relative paths, a different runner workspace, a
// different Cobertura <sources> element, a base produced by another tool)
// otherwise turns an existing file into one with no base data (issue #30).
//
// Candidates are collected before deciding anything, the same way
// paths.FindMatchingChangedFile does, and a match is only accepted when
// exactly one base file's path suffix-matches headPath: zero candidates
// stays "no match", and two or more is ambiguous and is treated as "no
// match" too, with a warning naming the candidates so the ambiguity is
// visible instead of being resolved by picking one arbitrarily.
func findBaseFileBySuffix(base *Report, headPath string) *FileCoverage {
	if base == nil {
		return nil
	}
	var candidates []*FileCoverage
	for i := range base.Files {
		if paths.HasSuffix(headPath, base.Files[i].Path) || paths.HasSuffix(base.Files[i].Path, headPath) {
			candidates = append(candidates, &base.Files[i])
		}
	}
	switch len(candidates) {
	case 0:
		return nil
	case 1:
		return candidates[0]
	default:
		names := make([]string, len(candidates))
		for i, c := range candidates {
			names[i] = c.Path
		}
		sort.Strings(names)
		fmt.Fprintf(os.Stderr, "Warning: head path %q matches multiple base files (%s), skipping\n",
			headPath, strings.Join(names, ", "))
		return nil
	}
}

// findFileInReport finds a file in a report by path suffix matching
func findFileInReport(report *Report, path string) *FileCoverage {
	if report == nil {
		return nil
	}
	for i := range report.Files {
		if report.Files[i].Path == path ||
			paths.HasSuffix(report.Files[i].Path, path) ||
			paths.HasSuffix(path, report.Files[i].Path) {
			return &report.Files[i]
		}
	}
	return nil
}

// PatchCoverage is coverage of only the lines a PR added, as opposed to
// Report.Coverage's whole-project number or FileCoverage.Percentage's
// whole-file one. Codecov calls this "patch coverage" and treats it as a
// PR comment's headline number, because it answers a question project
// coverage can't: is the code this PR actually wrote tested, independent of
// how large or how well-tested the rest of the repository already is
// (https://docs.codecov.com/docs/coverage-percentages).
type PatchCoverage struct {
	Covered int
	Total   int
}

// Percentage returns p's coverage percentage, or 0 if Total is 0. A Total of
// 0 means there was nothing coverable to measure -- no PR diff was
// available, or the diff only touched lines the coverage tool never
// instruments (blank lines, comments, imports, closing braces) -- the same
// sentinel FileCoverage.Percentage uses for LinesTotal == 0. Callers that
// need to tell "nothing to measure" apart from "measured, 0% covered" must
// check Total == 0 themselves before printing this.
func (p PatchCoverage) Percentage() float64 {
	if p.Total == 0 {
		return 0
	}
	return float64(p.Covered) / float64(p.Total) * 100
}

// CalculatePatchCoverage aggregates patch coverage across every file in
// report. addedLines is keyed by the path GitHub's PR diff reports for each
// file (see internal/diff.ParseFilePatch) and holds the new-side line
// numbers that file's diff added.
//
// For each report file, addedLines's line numbers are intersected with the
// file's coverable lines -- CoveredLines and UncoveredLines together -- to
// get that file's contribution to Total, and with CoveredLines alone for
// Covered:
//
//	patchTotal   = |addedLines ∩ (CoveredLines ∪ UncoveredLines)|
//	patchCovered = |addedLines ∩ CoveredLines|
//
// Only coverable lines count. A line the diff added but the coverage tool
// never instrumented -- a blank line, a comment, an import, a closing brace
// -- shows up in addedLines but in neither CoveredLines nor UncoveredLines,
// so it contributes to neither Covered nor Total: counting it against Total
// would make 100% patch coverage unreachable for any real change. Codecov's
// own success message names this explicitly: "All modified and coverable
// lines are covered by tests."
//
// A file with no coverable added lines -- because addedLines has no entry
// for it, or because none of its added lines were ever instrumented --
// contributes 0 to both Covered and Total, the same as not being in the
// aggregate at all, rather than counting as 0% (see Percentage).
//
// addedLines's paths are matched against report's file paths with
// paths.FindMatchingChangedFile, the same suffix-tolerant matching
// NewComparison uses, so a prefix difference between the PR diff's paths
// and the coverage report's doesn't hide a file's patch lines the same way
// it would hide the file from a changed-files filter (issue #6).
func CalculatePatchCoverage(report *Report, addedLines map[string][]diff.LineRange) PatchCoverage {
	var result PatchCoverage
	if report == nil || len(addedLines) == 0 {
		return result
	}

	changedSet := make(map[string]bool, len(addedLines))
	for path := range addedLines {
		changedSet[path] = true
	}

	for i := range report.Files {
		f := &report.Files[i]
		matched := paths.FindMatchingChangedFile(f.Path, changedSet)
		if matched == "" {
			continue
		}

		added := addedLineSet(addedLines[matched])
		if len(added) == 0 {
			continue
		}

		for _, ln := range f.CoveredLines {
			if added[ln] {
				result.Covered++
				result.Total++
			}
		}
		for _, ln := range f.UncoveredLines {
			if added[ln] {
				result.Total++
			}
		}
	}

	return result
}

// FilePatch is one report file's contribution to patch coverage: the tally
// CalculatePatchCoverage sums into its report-wide aggregate, plus which of
// the file's UncoveredLines fall inside the PR's added lines. It lets the
// Impacted Files table report a file's own patch percentage and
// patch-scoped uncovered lines instead of FileCoverage.Percentage and
// FileCoverage.UncoveredLines, which describe the whole file regardless of
// what the PR actually touched (issue #9).
type FilePatch struct {
	Coverage       PatchCoverage
	UncoveredLines []int
}

// CalculateFilePatchCoverage is CalculatePatchCoverage broken down per file
// instead of summed into one report-wide total. See CalculatePatchCoverage's
// doc comment for the intersection rule -- only coverable added lines count
// -- and for why paths are matched with paths.FindMatchingChangedFile
// instead of compared for equality.
//
// The result is keyed by each report file's own Path (report.Files[i].Path),
// not by addedLines's key, so a caller already holding a FileCoverage or
// walking Report.Files can look itself up directly instead of repeating the
// path match.
//
// A file with no coverable added lines has no entry in the result, the same
// as it contributing nothing to CalculatePatchCoverage's aggregate.
func CalculateFilePatchCoverage(report *Report, addedLines map[string][]diff.LineRange) map[string]FilePatch {
	result := make(map[string]FilePatch)
	if report == nil || len(addedLines) == 0 {
		return result
	}

	changedSet := make(map[string]bool, len(addedLines))
	for path := range addedLines {
		changedSet[path] = true
	}

	for i := range report.Files {
		f := &report.Files[i]
		matched := paths.FindMatchingChangedFile(f.Path, changedSet)
		if matched == "" {
			continue
		}

		added := addedLineSet(addedLines[matched])
		if len(added) == 0 {
			continue
		}

		var fp FilePatch
		for _, ln := range f.CoveredLines {
			if added[ln] {
				fp.Coverage.Covered++
				fp.Coverage.Total++
			}
		}
		for _, ln := range f.UncoveredLines {
			if added[ln] {
				fp.Coverage.Total++
				fp.UncoveredLines = append(fp.UncoveredLines, ln)
			}
		}
		if fp.Coverage.Total > 0 {
			result[f.Path] = fp
		}
	}

	return result
}

// addedLineSet expands ranges (inclusive Start/End pairs) into a line-number
// membership set, so intersecting them against a file's CoveredLines and
// UncoveredLines is a map lookup per line instead of a sorted merge.
func addedLineSet(ranges []diff.LineRange) map[int]bool {
	set := make(map[int]bool)
	for _, r := range ranges {
		for ln := r.Start; ln <= r.End; ln++ {
			set[ln] = true
		}
	}
	return set
}
