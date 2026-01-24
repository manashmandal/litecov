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
	comp := NewComparison(nil, nil, nil)

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

	comp := NewComparison(head, nil, nil)

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
	if !comp.FileChanges[0].IsNew {
		t.Error("File should be marked as new when no base")
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

	comp := NewComparison(head, base, nil)

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
	if !bChange.IsNew {
		t.Error("FileChanges[1].IsNew should be true (new file)")
	}
	if bChange.BaseCoverage != 0 {
		t.Errorf("FileChanges[1].BaseCoverage = %v, want 0", bChange.BaseCoverage)
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
	comp := NewComparison(head, base, changedFiles)

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

	comp := NewComparison(head, nil, []string{})

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
	comp := NewComparison(head, nil, changedFiles)

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
	comp := NewComparison(head, nil, changedFiles)

	// Should only have 1 file change (test files are skipped)
	if len(comp.FileChanges) != 1 {
		t.Errorf("FileChanges length = %v, want 1 (test files should be skipped)", len(comp.FileChanges))
	}
	if comp.FileChanges[0].Path != "internal/foo/a.go" {
		t.Errorf("FileChanges[0].Path = %v, want internal/foo/a.go", comp.FileChanges[0].Path)
	}
}

func TestIsSourceFile(t *testing.T) {
	tests := []struct {
		path     string
		expected bool
	}{
		{"cmd/app/main.go", true},
		{"internal/foo/bar.go", true},
		{"pkg/util/helper.go", true},
		{"cmd/app/main_test.go", false},
		{"internal/foo/bar_test.go", false},
		{".github/workflows/ci.yml", false},
		{"README.md", false},
		{"vendor/github.com/pkg/errors/errors.go", false},
		{"internal/generated/code.go", false},
		{"api/v1/types.pb.go", false},
		{"internal/mocks/mock_service.go", false},
		{"internal/test/service_mock.go", false},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			if got := isSourceFile(tt.path); got != tt.expected {
				t.Errorf("isSourceFile(%q) = %v, want %v", tt.path, got, tt.expected)
			}
		})
	}
}

func TestFindMatchingChangedFile(t *testing.T) {
	changedSet := map[string]bool{
		"cmd/app/main.go":        true,
		"internal/foo/handler.go": true,
	}

	tests := []struct {
		coveragePath string
		expected     string
	}{
		{"cmd/app/main.go", "cmd/app/main.go"},
		{"github.com/user/repo/cmd/app/main.go", "cmd/app/main.go"},
		{"internal/foo/handler.go", "internal/foo/handler.go"},
		{"github.com/user/repo/internal/foo/handler.go", "internal/foo/handler.go"},
		{"internal/other/file.go", ""},
	}

	for _, tt := range tests {
		t.Run(tt.coveragePath, func(t *testing.T) {
			if got := findMatchingChangedFile(tt.coveragePath, changedSet); got != tt.expected {
				t.Errorf("findMatchingChangedFile(%q) = %q, want %q", tt.coveragePath, got, tt.expected)
			}
		})
	}
}
