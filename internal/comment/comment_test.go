package comment

import (
	"strings"
	"testing"

	"github.com/litecov/litecov/internal/coverage"
)

func TestFormat(t *testing.T) {
	report := &coverage.Report{
		Files: []coverage.FileCoverage{
			{Path: "src/parser.go", LinesCovered: 75, LinesTotal: 100},
			{Path: "src/utils.go", LinesCovered: 40, LinesTotal: 100},
		},
		TotalCovered: 115,
		TotalLines:   200,
		Coverage:     57.5,
	}

	opts := Options{
		Title:     "Coverage Report",
		ShowFiles: "all",
	}

	result := Format(report, opts)

	checks := []string{
		"## Coverage Report",
		"57.50%",
		"115/200",
		"src/parser.go",
		"src/utils.go",
		":warning:",
		Marker,
	}

	for _, check := range checks {
		if !strings.Contains(result, check) {
			t.Errorf("missing %q in output", check)
		}
	}
}

func TestFormat_NoFiles(t *testing.T) {
	report := &coverage.Report{
		Files:        []coverage.FileCoverage{},
		TotalCovered: 0,
		TotalLines:   0,
		Coverage:     0,
	}

	result := Format(report, Options{Title: "Test", ShowFiles: "all"})

	if !strings.Contains(result, "## Test") {
		t.Error("missing title")
	}
	if strings.Contains(result, "| File |") {
		t.Error("should not have file table when no files")
	}
}

func TestFormat_FilterChanged(t *testing.T) {
	report := &coverage.Report{
		Files: []coverage.FileCoverage{
			{Path: "src/parser.go", LinesCovered: 75, LinesTotal: 100},
			{Path: "src/utils.go", LinesCovered: 40, LinesTotal: 100},
			{Path: "src/other.go", LinesCovered: 90, LinesTotal: 100},
		},
		TotalCovered: 205,
		TotalLines:   300,
		Coverage:     68.33,
	}

	opts := Options{
		Title:        "Coverage Report",
		ShowFiles:    "changed",
		ChangedFiles: []string{"src/parser.go", "src/utils.go"},
	}

	result := Format(report, opts)

	if !strings.Contains(result, "src/parser.go") {
		t.Error("missing changed file parser.go")
	}
	if !strings.Contains(result, "src/utils.go") {
		t.Error("missing changed file utils.go")
	}
	if strings.Contains(result, "src/other.go") {
		t.Error("should not contain unchanged file other.go")
	}
}

func TestFormat_ChangedNoFilter(t *testing.T) {
	report := &coverage.Report{
		Files: []coverage.FileCoverage{
			{Path: "src/a.go", LinesCovered: 50, LinesTotal: 100},
		},
	}

	opts := Options{ShowFiles: "changed", ChangedFiles: nil}
	result := Format(report, opts)

	if !strings.Contains(result, "src/a.go") {
		t.Error("should show all files when ChangedFiles is empty")
	}
}

func TestFormat_Threshold(t *testing.T) {
	report := &coverage.Report{
		Files: []coverage.FileCoverage{
			{Path: "src/good.go", LinesCovered: 90, LinesTotal: 100},
			{Path: "src/bad.go", LinesCovered: 40, LinesTotal: 100},
		},
		TotalCovered: 130,
		TotalLines:   200,
		Coverage:     65.0,
	}

	opts := Options{
		Title:     "Coverage Report",
		ShowFiles: "threshold:50",
		Threshold: 50,
	}

	result := Format(report, opts)

	if strings.Contains(result, "src/good.go") {
		t.Error("should not contain file above threshold")
	}
	if !strings.Contains(result, "src/bad.go") {
		t.Error("missing file below threshold")
	}
}

func TestFormat_Worst(t *testing.T) {
	report := &coverage.Report{
		Files: []coverage.FileCoverage{
			{Path: "src/best.go", LinesCovered: 95, LinesTotal: 100},
			{Path: "src/good.go", LinesCovered: 80, LinesTotal: 100},
			{Path: "src/ok.go", LinesCovered: 60, LinesTotal: 100},
			{Path: "src/bad.go", LinesCovered: 40, LinesTotal: 100},
		},
		TotalCovered: 275,
		TotalLines:   400,
		Coverage:     68.75,
	}

	opts := Options{
		Title:     "Coverage Report",
		ShowFiles: "worst:2",
		WorstN:    2,
	}

	result := Format(report, opts)

	if strings.Contains(result, "src/best.go") {
		t.Error("should not contain best file")
	}
	if strings.Contains(result, "src/good.go") {
		t.Error("should not contain good file")
	}
	if !strings.Contains(result, "src/bad.go") {
		t.Error("missing worst file")
	}
	if !strings.Contains(result, "src/ok.go") {
		t.Error("missing second worst file")
	}
}

func TestFormat_WorstMoreThanFiles(t *testing.T) {
	report := &coverage.Report{
		Files: []coverage.FileCoverage{
			{Path: "a.go", LinesCovered: 50, LinesTotal: 100},
		},
	}

	opts := Options{ShowFiles: "worst:10", WorstN: 10}
	result := Format(report, opts)

	if !strings.Contains(result, "a.go") {
		t.Error("should show all files when WorstN > len(files)")
	}
}

func TestFormat_DefaultFilter(t *testing.T) {
	report := &coverage.Report{
		Files: []coverage.FileCoverage{
			{Path: "a.go", LinesCovered: 50, LinesTotal: 100},
		},
	}

	opts := Options{ShowFiles: "unknown"}
	result := Format(report, opts)

	if !strings.Contains(result, "a.go") {
		t.Error("default filter should return all files")
	}
}

func TestFormat_HighCoverageNoWarning(t *testing.T) {
	report := &coverage.Report{
		Files: []coverage.FileCoverage{
			{Path: "good.go", LinesCovered: 80, LinesTotal: 100},
		},
	}

	result := Format(report, Options{ShowFiles: "all"})

	if strings.Contains(result, ":warning:") {
		t.Error("should not have warning for high coverage file")
	}
}
