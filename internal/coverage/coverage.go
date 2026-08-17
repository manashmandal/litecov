package coverage

import "github.com/manashmandal/litecov/internal/paths"

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
	IsNew        bool
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
// inferred from base-report absence).
func NewComparison(head, base *Report, changedFiles []string, addedFiles map[string]bool) *Comparison {
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

	// Build a map of head files for quick lookup
	headFileMap := make(map[string]*FileCoverage)
	for i := range head.Files {
		headFileMap[head.Files[i].Path] = &head.Files[i]
	}

	changedFileSet := make(map[string]bool)
	for _, f := range changedFiles {
		changedFileSet[f] = true
	}

	filterByChanged := len(changedFiles) > 0

	// Track which changed files we've seen in coverage data
	coveredChangedFiles := make(map[string]bool)

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

		if baseFile, exists := baseFileMap[headFile.Path]; exists {
			fc.BaseCoverage = baseFile.Percentage()
			fc.Delta = fc.HeadCoverage - fc.BaseCoverage
		} else {
			// No base entry for this path: BaseCoverage and Delta stay at
			// their zero-value sentinel rather than computing HeadCoverage -
			// 0, which would assert a measurement that was never taken
			// (issue #32).
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
				NoCoverage:   true,
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

	return comp
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
