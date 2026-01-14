package comment

import (
	"strings"
	"testing"

	"github.com/manashmandal/litecov/internal/coverage"
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
		":bar_chart: Coverage Report",
		"57.50%",
		"115/200",
		"src/parser.go",
		"src/utils.go",
		Marker,
		"<details>",
		"Coverage Diff",
		"Impacted Files (2)",
		":warning:",
		":x:",
		"LiteCov",
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

	if !strings.Contains(result, ":bar_chart: Test") {
		t.Error("missing title")
	}
	if strings.Contains(result, "Impacted Files") {
		t.Error("should not have impacted files section when no files")
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

func TestFormat_StatusEmoji_HighCoverage(t *testing.T) {
	report := &coverage.Report{
		Files: []coverage.FileCoverage{
			{Path: "good.go", LinesCovered: 85, LinesTotal: 100},
		},
		TotalCovered: 85,
		TotalLines:   100,
		Coverage:     85.0,
	}

	result := Format(report, Options{ShowFiles: "all"})

	if !strings.Contains(result, ":white_check_mark:") {
		t.Error("should have checkmark for high coverage")
	}
}

func TestFormat_StatusEmoji_MediumCoverage(t *testing.T) {
	report := &coverage.Report{
		Files: []coverage.FileCoverage{
			{Path: "medium.go", LinesCovered: 65, LinesTotal: 100},
		},
		TotalCovered: 65,
		TotalLines:   100,
		Coverage:     65.0,
	}

	result := Format(report, Options{ShowFiles: "all"})

	if !strings.Contains(result, ":warning:") {
		t.Error("should have warning for medium coverage")
	}
}

func TestFormat_StatusEmoji_LowCoverage(t *testing.T) {
	report := &coverage.Report{
		Files: []coverage.FileCoverage{
			{Path: "bad.go", LinesCovered: 30, LinesTotal: 100},
		},
		TotalCovered: 30,
		TotalLines:   100,
		Coverage:     30.0,
	}

	result := Format(report, Options{ShowFiles: "all"})

	if !strings.Contains(result, ":x:") {
		t.Error("should have X for low coverage")
	}
}

func TestFormat_WithHyperlinks(t *testing.T) {
	report := &coverage.Report{
		Files: []coverage.FileCoverage{
			{Path: "src/parser.go", LinesCovered: 75, LinesTotal: 100, UncoveredLines: []int{10, 11, 12}},
		},
		TotalCovered: 75,
		TotalLines:   100,
		Coverage:     75.0,
	}

	opts := Options{
		Title:     "Coverage Report",
		ShowFiles: "all",
		RepoURL:   "https://github.com/test/repo",
		SHA:       "abc123",
	}

	result := Format(report, opts)

	if !strings.Contains(result, "https://github.com/test/repo/blob/abc123/src/parser.go") {
		t.Error("missing file hyperlink")
	}
}

func TestFormatUncoveredLines_Ranges(t *testing.T) {
	lines := []int{1, 2, 3, 5, 7, 8, 9, 10}
	result := formatUncoveredLines(lines, "", "", "")

	if !strings.Contains(result, "L1-3") {
		t.Error("missing range L1-3")
	}
	if !strings.Contains(result, "L5") {
		t.Error("missing single L5")
	}
	if !strings.Contains(result, "L7-10") {
		t.Error("missing range L7-10")
	}
}

func TestFormatUncoveredLines_TooMany(t *testing.T) {
	lines := []int{1, 3, 5, 7, 9, 11, 13, 15}
	result := formatUncoveredLines(lines, "", "", "")

	if !strings.Contains(result, "+3 more") {
		t.Error("should truncate with 'more' indicator")
	}
}

func TestFormatUncoveredLines_Empty(t *testing.T) {
	result := formatUncoveredLines(nil, "", "", "")
	if result != "-" {
		t.Errorf("expected '-', got %s", result)
	}
}

func TestFormatRange_WithHyperlink(t *testing.T) {
	result := formatRange(10, 15, "https://github.com/test/repo", "abc123", "file.go")

	if !strings.Contains(result, "https://github.com/test/repo/blob/abc123/file.go#L10-L15") {
		t.Error("missing hyperlink in range")
	}
}

func TestFormatRange_SingleLine(t *testing.T) {
	result := formatRange(10, 10, "", "", "")

	if result != "L10" {
		t.Errorf("expected L10, got %s", result)
	}
}

func TestFormatRange_SingleLineWithHyperlink(t *testing.T) {
	result := formatRange(10, 10, "https://github.com/test/repo", "abc123", "file.go")

	expected := "[L10](https://github.com/test/repo/blob/abc123/file.go#L10)"
	if result != expected {
		t.Errorf("expected %s, got %s", expected, result)
	}
}

func TestGetStatusEmoji(t *testing.T) {
	tests := []struct {
		coverage float64
		expected string
	}{
		{100, ":white_check_mark:"},
		{80, ":white_check_mark:"},
		{79.99, ":warning:"},
		{50, ":warning:"},
		{49.99, ":x:"},
		{0, ":x:"},
	}

	for _, tt := range tests {
		result := getStatusEmoji(tt.coverage)
		if result != tt.expected {
			t.Errorf("getStatusEmoji(%.2f) = %s, expected %s", tt.coverage, result, tt.expected)
		}
	}
}

func TestGroupConsecutiveLines(t *testing.T) {
	tests := []struct {
		name     string
		lines    []int
		expected []LineRange
	}{
		{
			name:     "empty",
			lines:    nil,
			expected: nil,
		},
		{
			name:     "single line",
			lines:    []int{5},
			expected: []LineRange{{Start: 5, End: 5}},
		},
		{
			name:     "consecutive lines",
			lines:    []int{1, 2, 3, 4},
			expected: []LineRange{{Start: 1, End: 4}},
		},
		{
			name:  "gaps",
			lines: []int{1, 2, 5, 6, 7, 10},
			expected: []LineRange{
				{Start: 1, End: 2},
				{Start: 5, End: 7},
				{Start: 10, End: 10},
			},
		},
		{
			name:  "unsorted input",
			lines: []int{5, 1, 3, 2, 4},
			expected: []LineRange{
				{Start: 1, End: 5},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GroupConsecutiveLines(tt.lines)
			if len(result) != len(tt.expected) {
				t.Errorf("expected %d ranges, got %d", len(tt.expected), len(result))
				return
			}
			for i, r := range result {
				if r.Start != tt.expected[i].Start || r.End != tt.expected[i].End {
					t.Errorf("range %d: expected {%d, %d}, got {%d, %d}",
						i, tt.expected[i].Start, tt.expected[i].End, r.Start, r.End)
				}
			}
		})
	}
}

func TestFormatHeader(t *testing.T) {
	report := &coverage.Report{}

	t.Run("with title", func(t *testing.T) {
		opts := Options{Title: "My Report"}
		result := formatHeader(report, opts)
		if !strings.Contains(result, "My Report") {
			t.Error("missing custom title")
		}
	})

	t.Run("default title", func(t *testing.T) {
		opts := Options{}
		result := formatHeader(report, opts)
		if !strings.Contains(result, "Coverage Report") {
			t.Error("missing default title")
		}
	})
}

func TestFormatQuickSummary(t *testing.T) {
	report := &coverage.Report{
		TotalCovered: 850,
		TotalLines:   1000,
		Coverage:     85.0,
		Files:        make([]coverage.FileCoverage, 10),
	}

	result := formatQuickSummary(report)

	if !strings.Contains(result, "85.00%") {
		t.Error("missing coverage percentage")
	}
	if !strings.Contains(result, "850/1000") {
		t.Error("missing lines fraction")
	}
	if !strings.Contains(result, "10") {
		t.Error("missing files count")
	}
	if !strings.Contains(result, ":white_check_mark:") {
		t.Error("missing status emoji")
	}
}

func TestFormatCoverageDiff(t *testing.T) {
	report := &coverage.Report{
		TotalCovered: 500,
		TotalLines:   1000,
		Coverage:     50.0,
		Files:        make([]coverage.FileCoverage, 5),
	}

	result := formatCoverageDiff(report)

	if !strings.Contains(result, "<details>") {
		t.Error("missing details tag")
	}
	if !strings.Contains(result, "Coverage Diff") {
		t.Error("missing summary")
	}
	if !strings.Contains(result, "```diff") {
		t.Error("missing diff code block")
	}
	if !strings.Contains(result, "@@") {
		t.Error("missing @@ markers")
	}
	if !strings.Contains(result, "50.00%") {
		t.Error("missing coverage")
	}
}

func TestFormatImpactedFiles(t *testing.T) {
	files := []coverage.FileCoverage{
		{Path: "file1.go", LinesCovered: 90, LinesTotal: 100},
		{Path: "file2.go", LinesCovered: 40, LinesTotal: 100},
	}
	opts := Options{}

	result := formatImpactedFiles(files, opts)

	if !strings.Contains(result, "Impacted Files (2)") {
		t.Error("missing impacted files count")
	}
	if !strings.Contains(result, "file1.go") {
		t.Error("missing file1")
	}
	if !strings.Contains(result, "file2.go") {
		t.Error("missing file2")
	}
	if !strings.Contains(result, ":white_check_mark:") {
		t.Error("missing checkmark for high coverage file")
	}
	if !strings.Contains(result, ":x:") {
		t.Error("missing X for low coverage file")
	}
}

func TestFormatImpactedFiles_Empty(t *testing.T) {
	result := formatImpactedFiles(nil, Options{})
	if result != "" {
		t.Error("expected empty string for no files")
	}
}

func TestFormatFileName(t *testing.T) {
	t.Run("without hyperlink", func(t *testing.T) {
		result := formatFileName("test.go", Options{})
		if result != "`test.go`" {
			t.Errorf("expected `test.go`, got %s", result)
		}
	})

	t.Run("with hyperlink", func(t *testing.T) {
		opts := Options{
			RepoURL: "https://github.com/test/repo",
			SHA:     "abc123",
		}
		result := formatFileName("test.go", opts)
		expected := "[`test.go`](https://github.com/test/repo/blob/abc123/test.go)"
		if result != expected {
			t.Errorf("expected %s, got %s", expected, result)
		}
	})
}

func TestFormatFooter(t *testing.T) {
	result := formatFooter()

	if !strings.Contains(result, "---") {
		t.Error("missing horizontal rule")
	}
	if !strings.Contains(result, "LiteCov") {
		t.Error("missing LiteCov branding")
	}
	if !strings.Contains(result, "https://github.com/manashmandal/litecov") {
		t.Error("missing repo URL")
	}
}
