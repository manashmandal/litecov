package coverage

import "testing"

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
