package coverage

import (
	"testing"

	"github.com/manashmandal/litecov/internal/diff"
)

func TestFileCoverage_Percentage(t *testing.T) {
	tests := []struct {
		name     string
		covered  int
		total    int
		expected float64
	}{
		{"full coverage", 100, 100, 100.0},
		{"half coverage", 50, 100, 50.0},
		{"no coverage", 0, 100, 0.0},
		{"zero lines", 0, 0, 0.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fc := FileCoverage{
				LinesCovered: tt.covered,
				LinesTotal:   tt.total,
			}
			if got := fc.Percentage(); got != tt.expected {
				t.Errorf("Percentage() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestFileCoverage_CoveredLines(t *testing.T) {
	fc := FileCoverage{
		Path:           "src/main.go",
		LinesCovered:   3,
		LinesTotal:     5,
		UncoveredLines: []int{2, 4},
		CoveredLines:   []int{1, 3, 5},
	}

	if len(fc.CoveredLines) != 3 {
		t.Errorf("CoveredLines length = %v, want 3", len(fc.CoveredLines))
	}
	if fc.CoveredLines[0] != 1 {
		t.Errorf("CoveredLines[0] = %v, want 1", fc.CoveredLines[0])
	}
}

func TestReport_Calculate(t *testing.T) {
	report := &Report{
		Files: []FileCoverage{
			{Path: "a.go", LinesCovered: 80, LinesTotal: 100},
			{Path: "b.go", LinesCovered: 20, LinesTotal: 100},
		},
	}
	report.Calculate()

	if report.TotalCovered != 100 {
		t.Errorf("TotalCovered = %v, want 100", report.TotalCovered)
	}
	if report.TotalLines != 200 {
		t.Errorf("TotalLines = %v, want 200", report.TotalLines)
	}
	if report.Coverage != 50.0 {
		t.Errorf("Coverage = %v, want 50.0", report.Coverage)
	}
}

func TestReport_Calculate_Empty(t *testing.T) {
	report := &Report{Files: []FileCoverage{}}
	report.Calculate()

	if report.TotalCovered != 0 {
		t.Errorf("TotalCovered = %v, want 0", report.TotalCovered)
	}
	if report.TotalLines != 0 {
		t.Errorf("TotalLines = %v, want 0", report.TotalLines)
	}
	if report.Coverage != 0 {
		t.Errorf("Coverage = %v, want 0", report.Coverage)
	}
}

func TestReport_Calculate_SingleFile(t *testing.T) {
	report := &Report{
		Files: []FileCoverage{
			{Path: "single.go", LinesCovered: 75, LinesTotal: 100},
		},
	}
	report.Calculate()

	if report.Coverage != 75.0 {
		t.Errorf("Coverage = %v, want 75.0", report.Coverage)
	}
}

func TestFileCoverage_Path(t *testing.T) {
	fc := FileCoverage{
		Path:         "src/main.go",
		LinesCovered: 50,
		LinesTotal:   100,
	}

	if fc.Path != "src/main.go" {
		t.Errorf("Path = %v, want src/main.go", fc.Path)
	}
}

func TestReport_Hits(t *testing.T) {
	report := &Report{
		TotalCovered: 75,
		TotalLines:   100,
	}

	if got := report.Hits(); got != 75 {
		t.Errorf("Hits() = %v, want 75", got)
	}
}

func TestReport_Misses(t *testing.T) {
	report := &Report{
		TotalCovered: 75,
		TotalLines:   100,
	}

	if got := report.Misses(); got != 25 {
		t.Errorf("Misses() = %v, want 25", got)
	}
}

func TestReport_HitsAndMisses_Zero(t *testing.T) {
	report := &Report{
		TotalCovered: 0,
		TotalLines:   0,
	}

	if got := report.Hits(); got != 0 {
		t.Errorf("Hits() = %v, want 0", got)
	}
	if got := report.Misses(); got != 0 {
		t.Errorf("Misses() = %v, want 0", got)
	}
}

func TestNewComparison_NilHead(t *testing.T) {
	comp := NewComparison(nil, nil, nil, nil, nil)

	if comp.Head != nil {
		t.Errorf("Head = %v, want nil", comp.Head)
	}
	if comp.Base != nil {
		t.Errorf("Base = %v, want nil", comp.Base)
	}
	if comp.CoverageDelta != 0 {
		t.Errorf("CoverageDelta = %v, want 0", comp.CoverageDelta)
	}
}

func TestNewComparison_NilBase(t *testing.T) {
	head := &Report{
		Files: []FileCoverage{
			{Path: "a.go", LinesCovered: 80, LinesTotal: 100},
		},
		Coverage: 80.0,
	}

	comp := NewComparison(head, nil, nil, nil, nil)

	if comp.Head != head {
		t.Error("Head should match provided report")
	}
	if comp.Base != nil {
		t.Error("Base should be nil")
	}
	if comp.CoverageDelta != 0 {
		t.Errorf("CoverageDelta = %v, want 0 (no base)", comp.CoverageDelta)
	}
	if len(comp.FileChanges) != 1 {
		t.Errorf("FileChanges length = %v, want 1", len(comp.FileChanges))
	}
	// No PR diff status was supplied (addedFiles is nil), so IsNew must stay
	// false: a missing base report means the base measurement is unknown,
	// not that the file was added by this PR (issue #32).
	if comp.FileChanges[0].IsNew {
		t.Error("File should not be marked new without PR diff status, even with no base report")
	}
	if !comp.FileChanges[0].NoBaseData {
		t.Error("File should be marked NoBaseData when there is no base report")
	}
}

func TestNewComparison_WithBase(t *testing.T) {
	head := &Report{
		Files: []FileCoverage{
			{Path: "a.go", LinesCovered: 90, LinesTotal: 100},
			{Path: "b.go", LinesCovered: 50, LinesTotal: 100},
		},
		Coverage: 70.0,
	}
	base := &Report{
		Files: []FileCoverage{
			{Path: "a.go", LinesCovered: 80, LinesTotal: 100},
		},
		Coverage: 80.0,
	}

	comp := NewComparison(head, base, nil, nil, nil)

	if comp.CoverageDelta != -10.0 {
		t.Errorf("CoverageDelta = %v, want -10.0", comp.CoverageDelta)
	}
	if len(comp.FileChanges) != 2 {
		t.Errorf("FileChanges length = %v, want 2", len(comp.FileChanges))
	}

	aChange := comp.FileChanges[0]
	if aChange.Path != "a.go" {
		t.Errorf("FileChanges[0].Path = %v, want a.go", aChange.Path)
	}
	if aChange.HeadCoverage != 90.0 {
		t.Errorf("FileChanges[0].HeadCoverage = %v, want 90.0", aChange.HeadCoverage)
	}
	if aChange.BaseCoverage != 80.0 {
		t.Errorf("FileChanges[0].BaseCoverage = %v, want 80.0", aChange.BaseCoverage)
	}
	if aChange.Delta != 10.0 {
		t.Errorf("FileChanges[0].Delta = %v, want 10.0", aChange.Delta)
	}
	if aChange.IsNew {
		t.Error("FileChanges[0].IsNew should be false")
	}

	bChange := comp.FileChanges[1]
	if bChange.Path != "b.go" {
		t.Errorf("FileChanges[1].Path = %v, want b.go", bChange.Path)
	}
	// No addedFiles info was supplied, so IsNew stays false: b.go's absence
	// from base is unknown, not asserted as "added by this PR" (issue #32).
	if bChange.IsNew {
		t.Error("FileChanges[1].IsNew should be false without PR diff status")
	}
	if !bChange.NoBaseData {
		t.Error("FileChanges[1].NoBaseData should be true: b.go has no base entry")
	}
	if bChange.BaseCoverage != 0 {
		t.Errorf("FileChanges[1].BaseCoverage = %v, want 0", bChange.BaseCoverage)
	}
	if bChange.Delta != 0 {
		t.Errorf("FileChanges[1].Delta = %v, want 0 (sentinel, not HeadCoverage - 0)", bChange.Delta)
	}
}

func TestNewComparison_NoStatements(t *testing.T) {
	// A file with no coverable lines (e.g. an empty __init__.py) has
	// nothing to measure. Percentage() falls back to 0 for LinesTotal ==
	// 0, so FileChange needs its own signal to tell that apart from a file
	// that was actually measured at 0%. See issue #35.
	head := &Report{
		Files: []FileCoverage{
			{Path: "src/__init__.py", LinesCovered: 0, LinesTotal: 0},
			{Path: "src/a.go", LinesCovered: 100, LinesTotal: 100},
		},
		Coverage: 100.0,
	}

	comp := NewComparison(head, nil, nil, nil, nil)

	if len(comp.FileChanges) != 2 {
		t.Fatalf("FileChanges length = %v, want 2", len(comp.FileChanges))
	}

	var initChange, aChange FileChange
	for _, fc := range comp.FileChanges {
		switch fc.Path {
		case "src/__init__.py":
			initChange = fc
		case "src/a.go":
			aChange = fc
		}
	}

	if !initChange.NoStatements {
		t.Error("FileChanges[__init__.py].NoStatements should be true for a file with LinesTotal == 0")
	}
	if aChange.NoStatements {
		t.Error("FileChanges[a.go].NoStatements should be false for a file with coverable lines")
	}
}

func TestNewComparison_WithChangedFiles(t *testing.T) {
	head := &Report{
		Files: []FileCoverage{
			{Path: "a.go", LinesCovered: 90, LinesTotal: 100},
			{Path: "b.go", LinesCovered: 50, LinesTotal: 100},
			{Path: "c.go", LinesCovered: 70, LinesTotal: 100},
		},
		Coverage: 70.0,
	}
	base := &Report{
		Files: []FileCoverage{
			{Path: "a.go", LinesCovered: 80, LinesTotal: 100},
			{Path: "c.go", LinesCovered: 60, LinesTotal: 100},
		},
		Coverage: 70.0,
	}

	changedFiles := []string{"a.go", "b.go"}
	comp := NewComparison(head, base, changedFiles, nil, nil)

	if len(comp.FileChanges) != 2 {
		t.Errorf("FileChanges length = %v, want 2", len(comp.FileChanges))
	}

	for _, fc := range comp.FileChanges {
		if fc.Path == "c.go" {
			t.Error("c.go should not be in FileChanges (not in changedFiles)")
		}
	}
}

func TestNewComparison_EmptyChangedFiles(t *testing.T) {
	head := &Report{
		Files: []FileCoverage{
			{Path: "a.go", LinesCovered: 90, LinesTotal: 100},
			{Path: "b.go", LinesCovered: 50, LinesTotal: 100},
		},
		Coverage: 70.0,
	}

	comp := NewComparison(head, nil, []string{}, nil, nil)

	if len(comp.FileChanges) != 2 {
		t.Errorf("FileChanges length = %v, want 2 (empty changedFiles means all files)", len(comp.FileChanges))
	}
}

func TestFileChange_Delta(t *testing.T) {
	tests := []struct {
		name          string
		headCoverage  float64
		baseCoverage  float64
		expectedDelta float64
	}{
		{"improved", 90.0, 80.0, 10.0},
		{"decreased", 70.0, 80.0, -10.0},
		{"unchanged", 80.0, 80.0, 0.0},
		{"new file", 50.0, 0.0, 50.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fc := FileChange{
				HeadCoverage: tt.headCoverage,
				BaseCoverage: tt.baseCoverage,
				Delta:        tt.headCoverage - tt.baseCoverage,
			}
			if fc.Delta != tt.expectedDelta {
				t.Errorf("Delta = %v, want %v", fc.Delta, tt.expectedDelta)
			}
		})
	}
}

func TestNewComparison_MissingFiles(t *testing.T) {
	head := &Report{
		Files: []FileCoverage{
			{Path: "internal/foo/a.go", LinesCovered: 90, LinesTotal: 100},
		},
		Coverage: 90.0,
	}

	// Changed files include files not in the coverage report
	changedFiles := []string{"internal/foo/a.go", "cmd/app/main.go", "internal/bar/b.go"}
	comp := NewComparison(head, nil, changedFiles, nil, nil)

	// Should have 3 file changes: 1 covered + 2 missing
	if len(comp.FileChanges) != 3 {
		t.Errorf("FileChanges length = %v, want 3", len(comp.FileChanges))
	}

	// Find the missing files
	var missingCount int
	for _, fc := range comp.FileChanges {
		if fc.NoCoverage {
			missingCount++
			if fc.HeadCoverage != 0 {
				t.Errorf("Missing file %s HeadCoverage = %v, want 0", fc.Path, fc.HeadCoverage)
			}
		}
	}
	if missingCount != 2 {
		t.Errorf("Missing files count = %v, want 2", missingCount)
	}
}

func TestNewComparison_MissingFiles_SkipsTestFiles(t *testing.T) {
	head := &Report{
		Files:    []FileCoverage{},
		Coverage: 0,
	}

	// Changed files include test files which should be skipped
	changedFiles := []string{"internal/foo/a.go", "internal/foo/a_test.go", "cmd/app/main_test.go"}
	comp := NewComparison(head, nil, changedFiles, nil, nil)

	// Should only have 1 file change (test files are skipped)
	if len(comp.FileChanges) != 1 {
		t.Errorf("FileChanges length = %v, want 1 (test files should be skipped)", len(comp.FileChanges))
	}
	if comp.FileChanges[0].Path != "internal/foo/a.go" {
		t.Errorf("FileChanges[0].Path = %v, want internal/foo/a.go", comp.FileChanges[0].Path)
	}
}

func TestNewComparison_IsNewFromAddedFiles(t *testing.T) {
	// Core of issue #32: a file missing from the base report is not
	// necessarily added by this PR. It can be missing because the base run
	// was partial, crashed, or excluded it. IsNew must come from the PR
	// diff's own status (addedFiles), not from base-report absence.
	head := &Report{
		Files: []FileCoverage{
			{Path: "added.go", LinesCovered: 40, LinesTotal: 50},
			{Path: "orphaned.go", LinesCovered: 30, LinesTotal: 50},
		},
		Coverage: 70.0,
	}

	addedFiles := map[string]bool{"added.go": true}

	comp := NewComparison(head, nil, nil, addedFiles, nil)

	var addedChange, orphanedChange FileChange
	for _, fc := range comp.FileChanges {
		switch fc.Path {
		case "added.go":
			addedChange = fc
		case "orphaned.go":
			orphanedChange = fc
		}
	}

	if !addedChange.IsNew {
		t.Error("added.go should be IsNew: the PR diff reports it as added")
	}
	if orphanedChange.IsNew {
		t.Error("orphaned.go should not be IsNew: the PR diff does not report it as added, even though there is no base report for it")
	}
	if !orphanedChange.NoBaseData {
		t.Error("orphaned.go should be NoBaseData: there is no base report at all")
	}
}

func TestNewComparison_EmptyBaseReport(t *testing.T) {
	// Reproduces the issue #32 repro: a base report that parses cleanly but
	// to zero files (a truncated LCOV, a Cobertura with no packages) must
	// not be treated as "base coverage is 0%". head.Coverage - 0 would
	// report every point of head coverage as improvement over a
	// measurement that was never taken.
	head := &Report{
		Files: []FileCoverage{
			{Path: "a.go", LinesCovered: 90, LinesTotal: 100},
			{Path: "b.go", LinesCovered: 80, LinesTotal: 100},
		},
		Coverage: 85.0,
	}
	base := &Report{
		Files:    []FileCoverage{},
		Coverage: 0,
	}

	comp := NewComparison(head, base, nil, nil, nil)

	if !comp.NoBaseFiles {
		t.Error("NoBaseFiles should be true when the base report has zero files")
	}
	if comp.CoverageDelta != 0 {
		t.Errorf("CoverageDelta = %v, want 0 (not head.Coverage - 0)", comp.CoverageDelta)
	}
}

func TestNewComparison_BaseOnlyFileNotDropped(t *testing.T) {
	// Reproduces issue #31's "no filter" repro: base has a.go and b.go both
	// at 100%, the PR deletes b.go, so head only has a.go. With no
	// changed-file filter, b.go used to vanish from FileChanges entirely
	// instead of explaining where its coverage went.
	head := &Report{
		Files: []FileCoverage{
			{Path: "a.go", LinesCovered: 10, LinesTotal: 10},
		},
		Coverage: 100.0,
	}
	base := &Report{
		Files: []FileCoverage{
			{Path: "a.go", LinesCovered: 10, LinesTotal: 10},
			{Path: "b.go", LinesCovered: 10, LinesTotal: 10},
		},
		Coverage: 100.0,
	}

	comp := NewComparison(head, base, nil, nil, nil)

	if len(comp.FileChanges) != 2 {
		t.Fatalf("FileChanges length = %v, want 2 (b.go must not be dropped)", len(comp.FileChanges))
	}

	var bChange FileChange
	found := false
	for _, fc := range comp.FileChanges {
		if fc.Path == "b.go" {
			bChange = fc
			found = true
		}
	}
	if !found {
		t.Fatal("b.go should appear in FileChanges even though it's absent from head")
	}
	// No PR diff data was supplied to say why b.go is gone, so it gets the
	// general "unknown" signal rather than an asserted IsDeleted.
	if !bChange.NoCoverage {
		t.Error("b.go should be NoCoverage: it's absent from head with no PR diff data confirming why")
	}
	if bChange.IsDeleted {
		t.Error("b.go should not be IsDeleted without PR diff data confirming removal")
	}
	if bChange.BaseCoverage != 100.0 {
		t.Errorf("b.go BaseCoverage = %v, want 100.0", bChange.BaseCoverage)
	}
}

func TestNewComparison_BaseOnlyFileSkippedWhenFilteringChanged(t *testing.T) {
	// A base-only file that isn't part of the PR's changed-file list is out
	// of scope for a "changed files" view, the same reasoning issue #20
	// applied to not falling back to every file in the report.
	head := &Report{
		Files: []FileCoverage{
			{Path: "a.go", LinesCovered: 10, LinesTotal: 10},
		},
		Coverage: 100.0,
	}
	base := &Report{
		Files: []FileCoverage{
			{Path: "a.go", LinesCovered: 10, LinesTotal: 10},
			{Path: "b.go", LinesCovered: 10, LinesTotal: 10},
		},
		Coverage: 100.0,
	}

	comp := NewComparison(head, base, []string{"a.go"}, nil, nil)

	for _, fc := range comp.FileChanges {
		if fc.Path == "b.go" {
			t.Error("b.go should not appear when filtering by changed files and it's not in the diff")
		}
	}
}

func TestNewComparison_IsDeletedFromRemovedFiles(t *testing.T) {
	// Core of issue #31: a file GitHub's diff marks "removed" must not
	// render as an untested 0% (NoCoverage). It gets its own IsDeleted
	// signal instead, with its last known coverage carried from base.
	head := &Report{
		Files: []FileCoverage{
			{Path: "a.go", LinesCovered: 10, LinesTotal: 10},
		},
		Coverage: 100.0,
	}
	base := &Report{
		Files: []FileCoverage{
			{Path: "a.go", LinesCovered: 10, LinesTotal: 10},
			{Path: "b.go", LinesCovered: 10, LinesTotal: 10},
		},
		Coverage: 100.0,
	}

	changedFiles := []string{"b.go"}
	removedFiles := map[string]bool{"b.go": true}

	comp := NewComparison(head, base, changedFiles, nil, removedFiles)

	if len(comp.FileChanges) != 1 {
		t.Fatalf("FileChanges length = %v, want 1", len(comp.FileChanges))
	}

	bChange := comp.FileChanges[0]
	if bChange.Path != "b.go" {
		t.Fatalf("FileChanges[0].Path = %v, want b.go", bChange.Path)
	}
	if !bChange.IsDeleted {
		t.Error("b.go should be IsDeleted: the PR diff reports its status as removed")
	}
	if bChange.NoCoverage {
		t.Error("b.go should not also be NoCoverage: IsDeleted already explains the missing head data")
	}
	if bChange.BaseCoverage != 100.0 {
		t.Errorf("b.go BaseCoverage = %v, want 100.0", bChange.BaseCoverage)
	}
}

func TestNewComparison_BaseMatchTolerantOfPrefix(t *testing.T) {
	// Reproduces issue #30: the base file lookup used exact string equality
	// while every other path comparison in the pipeline
	// (paths.FindMatchingChangedFile, findFileInReport) tolerates a prefix
	// difference between the two paths. An absolute head path from a
	// GitHub Actions checkout and a repo-relative base path from a
	// differently-configured base run still name the same file and must
	// match.
	head := &Report{
		Files: []FileCoverage{
			{Path: "/home/runner/work/repo/repo/src/a.py", LinesCovered: 90, LinesTotal: 100},
		},
		Coverage: 90.0,
	}
	base := &Report{
		Files: []FileCoverage{
			{Path: "src/a.py", LinesCovered: 80, LinesTotal: 100},
		},
		Coverage: 80.0,
	}

	comp := NewComparison(head, base, nil, nil, nil)

	if len(comp.FileChanges) != 1 {
		t.Fatalf("FileChanges length = %v, want 1", len(comp.FileChanges))
	}

	fc := comp.FileChanges[0]
	if fc.NoBaseData {
		t.Error("NoBaseData should be false: src/a.py exists in base under a different path prefix")
	}
	if fc.BaseCoverage != 80.0 {
		t.Errorf("BaseCoverage = %v, want 80.0", fc.BaseCoverage)
	}
	if fc.Delta != 10.0 {
		t.Errorf("Delta = %v, want 10.0 (90.0 - 80.0); an exact-match lookup reports this as a fabricated new file instead", fc.Delta)
	}
	if fc.IsNew {
		t.Error("IsNew should be false: no PR diff status was supplied, and a prefix mismatch is not an added file")
	}
}

func TestNewComparison_BaseMatchAmbiguousSuffixSkipped(t *testing.T) {
	// The suffix-match fallback added for issue #30 must not resolve an
	// ambiguous match by picking an arbitrary base file. Same filename at
	// two directory depths in a monorepo (mirrors paths_test.go's
	// TestFindMatchingChangedFileAmbiguous) must fall back to NoBaseData
	// rather than silently attaching one candidate's coverage over the
	// other. The two base files themselves still show up as their own
	// unmatched entries (issue #31 behavior): the match is genuinely
	// ambiguous, so the pipeline cannot claim they survived into head
	// either.
	head := &Report{
		Files: []FileCoverage{
			{Path: "github.com/foo/bar/pkg/internal/x.go", LinesCovered: 50, LinesTotal: 100},
		},
		Coverage: 50.0,
	}
	base := &Report{
		Files: []FileCoverage{
			{Path: "internal/x.go", LinesCovered: 10, LinesTotal: 100},
			{Path: "pkg/internal/x.go", LinesCovered: 90, LinesTotal: 100},
		},
		Coverage: 50.0,
	}

	comp := NewComparison(head, base, nil, nil, nil)

	var headChange FileChange
	found := false
	for _, fc := range comp.FileChanges {
		if fc.Path == "github.com/foo/bar/pkg/internal/x.go" {
			headChange = fc
			found = true
		}
	}
	if !found {
		t.Fatal("head file should have a FileChange entry")
	}
	if !headChange.NoBaseData {
		t.Error("NoBaseData should be true: two base files ambiguously suffix-match, so neither should be picked")
	}
	if headChange.BaseCoverage != 0 {
		t.Errorf("BaseCoverage = %v, want 0 (sentinel; an ambiguous match must not attach either candidate)", headChange.BaseCoverage)
	}
	if headChange.Delta != 0 {
		t.Errorf("Delta = %v, want 0 (sentinel; an ambiguous match must not compute a delta from either candidate)", headChange.Delta)
	}
}

func TestPatchCoverage_Percentage(t *testing.T) {
	tests := []struct {
		name     string
		covered  int
		total    int
		expected float64
	}{
		{"full coverage", 20, 20, 100.0},
		{"half coverage", 10, 20, 50.0},
		{"no coverage", 0, 20, 0.0},
		{"zero total", 0, 0, 0.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := PatchCoverage{Covered: tt.covered, Total: tt.total}
			if got := p.Percentage(); got != tt.expected {
				t.Errorf("Percentage() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestCalculatePatchCoverage_NilReport(t *testing.T) {
	result := CalculatePatchCoverage(nil, map[string][]diff.LineRange{"a.go": {{Start: 1, End: 5}}})
	if result.Total != 0 || result.Covered != 0 {
		t.Errorf("CalculatePatchCoverage(nil, ...) = %+v, want zero value", result)
	}
}

func TestCalculatePatchCoverage_NoAddedLines(t *testing.T) {
	report := &Report{
		Files: []FileCoverage{
			{Path: "a.go", CoveredLines: []int{1, 2}, UncoveredLines: []int{3}},
		},
	}
	result := CalculatePatchCoverage(report, nil)
	if result.Total != 0 || result.Covered != 0 {
		t.Errorf("CalculatePatchCoverage(report, nil) = %+v, want zero value (no PR diff data)", result)
	}
}

func TestCalculatePatchCoverage_Intersection(t *testing.T) {
	// issue #6: patchTotal = |addedLines ∩ (CoveredLines ∪ UncoveredLines)|,
	// patchCovered = |addedLines ∩ CoveredLines|. Lines 1 and 5 are
	// coverable but outside the diff and must not count.
	report := &Report{
		Files: []FileCoverage{
			{
				Path:           "a.go",
				CoveredLines:   []int{1, 2, 4},
				UncoveredLines: []int{3, 5},
			},
		},
	}
	addedLines := map[string][]diff.LineRange{
		"a.go": {{Start: 2, End: 4}}, // added lines 2, 3, 4
	}

	result := CalculatePatchCoverage(report, addedLines)

	if result.Total != 3 {
		t.Errorf("Total = %v, want 3 (added lines 2, 3, 4 are all coverable)", result.Total)
	}
	if result.Covered != 2 {
		t.Errorf("Covered = %v, want 2 (added lines 2 and 4 are covered; 3 is not)", result.Covered)
	}
}

func TestCalculatePatchCoverage_UninstrumentedAddedLinesExcluded(t *testing.T) {
	// A blank line, comment, or closing brace shows up in the diff's added
	// lines but in neither CoveredLines nor UncoveredLines: it must not
	// count against Total, or 100% patch coverage would be unreachable for
	// any real change (issue #6).
	report := &Report{
		Files: []FileCoverage{
			{Path: "a.go", CoveredLines: []int{2}, UncoveredLines: nil},
		},
	}
	addedLines := map[string][]diff.LineRange{
		"a.go": {{Start: 1, End: 3}}, // 1 and 3 are e.g. blank lines/braces, only 2 is instrumented
	}

	result := CalculatePatchCoverage(report, addedLines)

	if result.Total != 1 {
		t.Errorf("Total = %v, want 1 (only line 2 is instrumented)", result.Total)
	}
	if result.Covered != 1 {
		t.Errorf("Covered = %v, want 1", result.Covered)
	}
}

func TestCalculatePatchCoverage_FileWithNoCoverableAddedLinesExcludedFromAggregate(t *testing.T) {
	// A docs file with no coverage data at all contributes nothing rather
	// than dragging the aggregate down as a false 0% (issue #6).
	report := &Report{
		Files: []FileCoverage{
			{Path: "tested.go", CoveredLines: []int{1}, UncoveredLines: nil},
			{Path: "README.md", CoveredLines: nil, UncoveredLines: nil},
		},
	}
	addedLines := map[string][]diff.LineRange{
		"tested.go": {{Start: 1, End: 1}},
		"README.md": {{Start: 1, End: 20}},
	}

	result := CalculatePatchCoverage(report, addedLines)

	if result.Total != 1 || result.Covered != 1 {
		t.Errorf("CalculatePatchCoverage = %+v, want {Covered:1 Total:1} (README.md has no coverable lines)", result)
	}
	if got := result.Percentage(); got != 100.0 {
		t.Errorf("Percentage() = %v, want 100.0", got)
	}
}

func TestCalculatePatchCoverage_MultipleFilesAggregate(t *testing.T) {
	report := &Report{
		Files: []FileCoverage{
			{Path: "a.go", CoveredLines: []int{1}, UncoveredLines: []int{2}},
			{Path: "b.go", CoveredLines: []int{10}, UncoveredLines: nil},
		},
	}
	addedLines := map[string][]diff.LineRange{
		"a.go": {{Start: 1, End: 2}},
		"b.go": {{Start: 10, End: 10}},
	}

	result := CalculatePatchCoverage(report, addedLines)

	if result.Total != 3 {
		t.Errorf("Total = %v, want 3", result.Total)
	}
	if result.Covered != 2 {
		t.Errorf("Covered = %v, want 2", result.Covered)
	}
}

func TestCalculatePatchCoverage_PathPrefixMismatchTolerated(t *testing.T) {
	// addedLines is keyed by the PR diff's paths, which can carry a
	// different prefix than the coverage report's paths -- the same
	// reasoning issue #30 applies to NewComparison's base lookup.
	report := &Report{
		Files: []FileCoverage{
			{Path: "/home/runner/work/repo/repo/src/a.go", CoveredLines: []int{1}, UncoveredLines: nil},
		},
	}
	addedLines := map[string][]diff.LineRange{
		"src/a.go": {{Start: 1, End: 1}},
	}

	result := CalculatePatchCoverage(report, addedLines)

	if result.Total != 1 || result.Covered != 1 {
		t.Errorf("CalculatePatchCoverage = %+v, want {Covered:1 Total:1}", result)
	}
}

func TestCalculatePatchCoverage_AmbiguousMatchSkipped(t *testing.T) {
	// Mirrors TestNewComparison_BaseMatchAmbiguousSuffixSkipped: two added
	// files at different directory depths both suffix-match the same report
	// path, so paths.FindMatchingChangedFile refuses to pick one and the
	// file contributes nothing rather than guessing.
	report := &Report{
		Files: []FileCoverage{
			{Path: "github.com/foo/bar/pkg/internal/x.go", CoveredLines: []int{1}, UncoveredLines: nil},
		},
	}
	addedLines := map[string][]diff.LineRange{
		"internal/x.go":     {{Start: 1, End: 1}},
		"pkg/internal/x.go": {{Start: 1, End: 1}},
	}

	result := CalculatePatchCoverage(report, addedLines)

	if result.Total != 0 || result.Covered != 0 {
		t.Errorf("CalculatePatchCoverage = %+v, want zero value (ambiguous match must not guess)", result)
	}
}

func TestCalculateFilePatchCoverage_NilReport(t *testing.T) {
	result := CalculateFilePatchCoverage(nil, map[string][]diff.LineRange{"a.go": {{Start: 1, End: 5}}})
	if len(result) != 0 {
		t.Errorf("CalculateFilePatchCoverage(nil, ...) = %+v, want empty map", result)
	}
}

func TestCalculateFilePatchCoverage_NoAddedLines(t *testing.T) {
	report := &Report{
		Files: []FileCoverage{
			{Path: "a.go", CoveredLines: []int{1, 2}, UncoveredLines: []int{3}},
		},
	}
	result := CalculateFilePatchCoverage(report, nil)
	if len(result) != 0 {
		t.Errorf("CalculateFilePatchCoverage(report, nil) = %+v, want empty map (no PR diff data)", result)
	}
}

// TestCalculateFilePatchCoverage_WellCoveredFileWithUntestedPatch reproduces
// issue #9: a large, well-covered file's whole-file percentage stays high
// even when the lines this PR added to it are untested.
// CalculateFilePatchCoverage must report that file's own patch numbers
// instead of letting its whole-file average hide them.
func TestCalculateFilePatchCoverage_WellCoveredFileWithUntestedPatch(t *testing.T) {
	report := &Report{
		Files: []FileCoverage{
			{
				Path:           "big.go",
				LinesCovered:   950,
				LinesTotal:     1000,
				CoveredLines:   []int{1, 2, 3}, // stand-ins for the file's other 950 covered lines
				UncoveredLines: []int{500, 501, 502},
			},
		},
	}
	addedLines := map[string][]diff.LineRange{
		"big.go": {{Start: 500, End: 502}}, // the PR's 3 new lines, none of them tested
	}

	result := CalculateFilePatchCoverage(report, addedLines)

	fp, ok := result["big.go"]
	if !ok {
		t.Fatal("missing patch entry for big.go")
	}
	if fp.Coverage.Total != 3 || fp.Coverage.Covered != 0 {
		t.Errorf("Coverage = %+v, want {Covered:0 Total:3}", fp.Coverage)
	}
	if got := fp.Coverage.Percentage(); got != 0.0 {
		t.Errorf("Percentage() = %v, want 0.0 (whole-file 95%% must not leak into the patch number)", got)
	}
	if len(fp.UncoveredLines) != 3 {
		t.Fatalf("UncoveredLines = %v, want 3 entries", fp.UncoveredLines)
	}
	for i, want := range []int{500, 501, 502} {
		if fp.UncoveredLines[i] != want {
			t.Errorf("UncoveredLines[%d] = %d, want %d", i, fp.UncoveredLines[i], want)
		}
	}
}

func TestCalculateFilePatchCoverage_KeyedByReportPath(t *testing.T) {
	// The result must be indexable by report.Files[i].Path directly, not by
	// addedLines's key, since the two can differ after suffix matching
	// (issue #9, the same reasoning as CalculatePatchCoverage's
	// PathPrefixMismatchTolerated case).
	report := &Report{
		Files: []FileCoverage{
			{Path: "/home/runner/work/repo/repo/src/a.go", CoveredLines: []int{1}, UncoveredLines: []int{2}},
		},
	}
	addedLines := map[string][]diff.LineRange{
		"src/a.go": {{Start: 1, End: 2}},
	}

	result := CalculateFilePatchCoverage(report, addedLines)

	fp, ok := result["/home/runner/work/repo/repo/src/a.go"]
	if !ok {
		t.Fatalf("missing patch entry keyed by report path, got %+v", result)
	}
	if fp.Coverage.Total != 2 || fp.Coverage.Covered != 1 {
		t.Errorf("Coverage = %+v, want {Covered:1 Total:2}", fp.Coverage)
	}
}

func TestCalculateFilePatchCoverage_NoCoverableAddedLinesExcluded(t *testing.T) {
	// A file whose added lines are all blank lines/comments/braces -- none
	// of them instrumented -- gets no entry, the same as it contributing
	// nothing to CalculatePatchCoverage's aggregate (issue #9).
	report := &Report{
		Files: []FileCoverage{
			{Path: "a.go", CoveredLines: []int{2}, UncoveredLines: nil},
		},
	}
	addedLines := map[string][]diff.LineRange{
		"a.go": {{Start: 10, End: 12}}, // none of these are lines the report has data for
	}

	result := CalculateFilePatchCoverage(report, addedLines)

	if len(result) != 0 {
		t.Errorf("CalculateFilePatchCoverage = %+v, want empty map", result)
	}
}

func TestCalculateFilePatchCoverage_MultipleFilesEachOwnEntry(t *testing.T) {
	report := &Report{
		Files: []FileCoverage{
			{Path: "a.go", CoveredLines: []int{1}, UncoveredLines: []int{2}},
			{Path: "b.go", CoveredLines: []int{10, 11}, UncoveredLines: nil},
			{Path: "untouched.go", CoveredLines: []int{1}, UncoveredLines: nil},
		},
	}
	addedLines := map[string][]diff.LineRange{
		"a.go": {{Start: 1, End: 2}},
		"b.go": {{Start: 10, End: 11}},
	}

	result := CalculateFilePatchCoverage(report, addedLines)

	if len(result) != 2 {
		t.Fatalf("got %d entries, want 2 (a.go and b.go, not untouched.go)", len(result))
	}
	if fp := result["a.go"]; fp.Coverage.Total != 2 || fp.Coverage.Covered != 1 {
		t.Errorf("a.go Coverage = %+v, want {Covered:1 Total:2}", fp.Coverage)
	}
	if fp := result["b.go"]; fp.Coverage.Total != 2 || fp.Coverage.Covered != 2 {
		t.Errorf("b.go Coverage = %+v, want {Covered:2 Total:2}", fp.Coverage)
	}
	if _, ok := result["untouched.go"]; ok {
		t.Error("untouched.go should have no patch entry, its diff has no added lines")
	}
}
