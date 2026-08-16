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
		"Coverage Report",
		"57.50%",
		"115/200",
		"src/parser.go",
		"src/utils.go",
		Marker,
		"<details>",
		"Coverage Diff",
		"Impacted Files (2)",
		"\u26A0\uFE0F", // warning emoji
		"\u274C",       // x emoji
		"LiteCov",
		"logo.png",
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

	if !strings.Contains(result, "Test") {
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

func TestFormat_ChangedEmptyDoesNotFallBackToAll(t *testing.T) {
	// Issue #20: an empty ChangedFiles used to make filterFiles fall back to
	// every file in the report, so show-files: changed silently reported the
	// whole repo instead of an empty, honestly-scoped result.
	report := &coverage.Report{
		Files: []coverage.FileCoverage{
			{Path: "src/a.go", LinesCovered: 50, LinesTotal: 100},
		},
	}

	opts := Options{ShowFiles: "changed", ChangedFiles: nil}
	result := Format(report, opts)

	if strings.Contains(result, "src/a.go") {
		t.Error("should not fall back to showing every file when ChangedFiles is empty")
	}
	if !strings.Contains(result, "Impacted Files (0)") {
		t.Error("missing empty Impacted Files section")
	}
	if !strings.Contains(result, "No changed files matched the coverage report.") {
		t.Error("missing explanation that no changed files matched")
	}
}

func TestFindMissingFiles(t *testing.T) {
	tests := []struct {
		name         string
		coveredPaths []string
		changedFiles []string
		expected     []string
	}{
		{
			name:         "new file with no coverage entry is missing",
			coveredPaths: []string{"src/parser.go"},
			changedFiles: []string{"src/new_file.go"},
			expected:     []string{"src/new_file.go"},
		},
		{
			name:         "exact match is not missing",
			coveredPaths: []string{"internal/parser.go"},
			changedFiles: []string{"internal/parser.go"},
			expected:     nil,
		},
		{
			name:         "suffix match through a module wrapper prefix is not missing",
			coveredPaths: []string{"github.com/user/repo/internal/parser.go"},
			changedFiles: []string{"internal/parser.go"},
			expected:     nil,
		},
		{
			// Issue #12: "parser.go" is a raw substring of "myparser.go" but
			// not the same file, so it must still be reported missing.
			name:         "filename that is only a raw substring of an unrelated covered path is still missing",
			coveredPaths: []string{"internal/myparser.go"},
			changedFiles: []string{"parser.go"},
			expected:     []string{"parser.go"},
		},
		{
			name:         "non-source files are skipped entirely",
			coveredPaths: nil,
			changedFiles: []string{"README.md"},
			expected:     nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var files []coverage.FileCoverage
			for _, p := range tt.coveredPaths {
				files = append(files, coverage.FileCoverage{Path: p, LinesCovered: 1, LinesTotal: 1})
			}
			report := &coverage.Report{Files: files}

			got := findMissingFiles(report, tt.changedFiles)

			if len(got) != len(tt.expected) {
				t.Fatalf("findMissingFiles() = %v, want %v", got, tt.expected)
			}
			for i := range got {
				if got[i] != tt.expected[i] {
					t.Errorf("findMissingFiles()[%d] = %q, want %q", i, got[i], tt.expected[i])
				}
			}
		})
	}
}

func TestFormat_ChangedFile_NotMatchedBySubstring(t *testing.T) {
	// Issue #12: a new, untested file must not be dropped from the report
	// just because its name is a substring of an unrelated covered path.
	report := &coverage.Report{
		Files: []coverage.FileCoverage{
			{Path: "internal/myparser.go", LinesCovered: 10, LinesTotal: 10},
		},
	}
	report.Calculate()

	opts := Options{
		ShowFiles:    "changed",
		ChangedFiles: []string{"parser.go"},
	}

	result := Format(report, opts)

	if !strings.Contains(result, "Impacted Files") {
		t.Fatal("expected an Impacted Files section listing the new file")
	}
	if !strings.Contains(result, "parser.go") {
		t.Error("missing new file parser.go")
	}
	if !strings.Contains(result, "no tests") {
		t.Error("parser.go should be flagged as having no tests")
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

func TestFormat_Threshold_ExcludesZeroStatementFiles(t *testing.T) {
	// A file with no coverable lines (e.g. an empty __init__.py) has
	// nothing to compare against the threshold. Percentage() falls back to
	// 0 for LinesTotal == 0, which used to always read as "below
	// threshold" and get listed next to files that were actually tested
	// and failed. See issue #35.
	report := &coverage.Report{
		Files: []coverage.FileCoverage{
			{Path: "src/__init__.py", LinesCovered: 0, LinesTotal: 0},
			{Path: "src/bad.go", LinesCovered: 40, LinesTotal: 100},
		},
		TotalCovered: 40,
		TotalLines:   100,
		Coverage:     40.0,
	}

	opts := Options{
		ShowFiles: "threshold:80",
		Threshold: 80,
	}

	result := Format(report, opts)

	if strings.Contains(result, "src/__init__.py") {
		t.Error("threshold:N should not select a file with no coverable lines")
	}
	if !strings.Contains(result, "src/bad.go") {
		t.Error("missing file genuinely below threshold")
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

func TestFormat_Worst_ExcludesZeroStatementFiles(t *testing.T) {
	// A file with no coverable lines has nothing to rank. Percentage()
	// falls back to 0 for LinesTotal == 0, which used to sort it straight
	// to the top as the worst file in the repo and crowd out files that
	// were actually tested and failed. See issue #35.
	report := &coverage.Report{
		Files: []coverage.FileCoverage{
			{Path: "src/__init__.py", LinesCovered: 0, LinesTotal: 0},
			{Path: "src/good.go", LinesCovered: 95, LinesTotal: 100},
			{Path: "src/bad.go", LinesCovered: 40, LinesTotal: 100},
		},
		TotalCovered: 135,
		TotalLines:   200,
		Coverage:     67.5,
	}

	opts := Options{
		ShowFiles: "worst:1",
		WorstN:    1,
	}

	result := Format(report, opts)

	if strings.Contains(result, "src/__init__.py") {
		t.Error("worst:N should not rank a file with no coverable lines as the worst file")
	}
	if !strings.Contains(result, "src/bad.go") {
		t.Error("worst:1 should surface the genuinely worst-covered file")
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

	if !strings.Contains(result, "\u2705") {
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

	if !strings.Contains(result, "\u26A0\uFE0F") {
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

	if !strings.Contains(result, "\u274C") {
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

func TestFormat_BlobLinksAreNormalizedAndNotDoubled(t *testing.T) {
	// Issue #18 repro: a Go coverage profile carries the module prefix and a
	// coverage.py report carries the CI's absolute checkout path. Before the
	// fix these produced a doubled "//" in the link (from the leading "/" on
	// the absolute path) and a duplicated "github.com/user/repo" segment
	// (from the module prefix stacking on top of RepoURL), both 404s.
	report := &coverage.Report{Files: []coverage.FileCoverage{
		{Path: "/home/runner/work/repo/repo/src/module.py", LinesCovered: 1, LinesTotal: 2, UncoveredLines: []int{7}},
		{Path: "github.com/user/repo/internal/foo.go", LinesCovered: 1, LinesTotal: 2, UncoveredLines: []int{9}},
	}}
	report.Calculate()

	result := Format(report, Options{
		ShowFiles: "all",
		RepoURL:   "https://github.com/user/repo",
		SHA:       "abc123",
	})

	if strings.Contains(result, "blob/abc123//") {
		t.Error("blob link has a doubled slash from an unnormalized absolute path")
	}
	if strings.Contains(result, "/blob/abc123/github.com/") {
		t.Error("blob link has a duplicated module segment from an unnormalized module path")
	}
	if strings.Contains(result, "/home/runner") {
		t.Error("output contains the unnormalized absolute CI checkout path")
	}

	wantLinks := []string{
		"https://github.com/user/repo/blob/abc123/src/module.py",
		"https://github.com/user/repo/blob/abc123/src/module.py#L7",
		"https://github.com/user/repo/blob/abc123/internal/foo.go",
		"https://github.com/user/repo/blob/abc123/internal/foo.go#L9",
	}
	for _, link := range wantLinks {
		if !strings.Contains(result, link) {
			t.Errorf("missing normalized link %q in output:\n%s", link, result)
		}
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

func TestFormatRange_NormalizesAndEscapesPath(t *testing.T) {
	// Issue #18: formatRange is what interpolates the coverage path into the
	// blob URL for a line link; it needs the same repo-relative
	// normalization and percent escaping as formatFileName.
	tests := []struct {
		name     string
		filePath string
		expected string
	}{
		{
			name:     "Go module prefix is stripped",
			filePath: "github.com/user/repo/internal/foo.go",
			expected: "[L9](https://github.com/user/repo/blob/abc123/internal/foo.go#L9)",
		},
		{
			name:     "absolute CI checkout path is stripped",
			filePath: "/home/runner/work/repo/repo/src/module.py",
			expected: "[L9](https://github.com/user/repo/blob/abc123/src/module.py#L9)",
		},
		{
			name:     "space in filename is percent-escaped",
			filePath: "src/my file.go",
			expected: "[L9](https://github.com/user/repo/blob/abc123/src/my%20file.go#L9)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatRange(9, 9, "https://github.com/user/repo", "abc123", tt.filePath)
			if got != tt.expected {
				t.Errorf("formatRange(%q) = %q, want %q", tt.filePath, got, tt.expected)
			}
		})
	}
}

func TestGetStatusEmoji(t *testing.T) {
	tests := []struct {
		coverage float64
		expected string
	}{
		{100, "\u2705"},
		{80, "\u2705"},
		{79.99, "\u26A0\uFE0F"},
		{50, "\u26A0\uFE0F"},
		{49.99, "\u274C"},
		{0, "\u274C"},
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
	t.Run("with title", func(t *testing.T) {
		opts := Options{Title: "My Report"}
		result := formatHeader(opts)
		if !strings.Contains(result, "My Report") {
			t.Error("missing custom title")
		}
		if !strings.Contains(result, "logo.png") {
			t.Error("missing logo")
		}
	})

	t.Run("default title", func(t *testing.T) {
		opts := Options{}
		result := formatHeader(opts)
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
	if !strings.Contains(result, "\u2705") {
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
	if !strings.Contains(result, "\u2705") {
		t.Error("missing checkmark for high coverage file")
	}
	if !strings.Contains(result, "\u274C") {
		t.Error("missing X for low coverage file")
	}
}

func TestFormatImpactedFiles_Empty(t *testing.T) {
	result := formatImpactedFiles(nil, Options{})
	if result != "" {
		t.Error("expected empty string for no files")
	}
}

func TestFormatImpactedFiles_EmptyChanged(t *testing.T) {
	// Issue #20: show-files: changed with no matching files must render an
	// explanation, not an empty string indistinguishable from "all: none".
	result := formatImpactedFiles(nil, Options{ShowFiles: "changed"})

	if !strings.Contains(result, "Impacted Files (0)") {
		t.Error("missing empty Impacted Files section")
	}
	if !strings.Contains(result, "No changed files matched the coverage report.") {
		t.Error("missing explanation that no changed files matched")
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

func TestFormatFileName_NormalizesAndEscapesPath(t *testing.T) {
	// Issue #18: a Go coverage profile carries the module prefix and a
	// coverage.py report carries the CI's absolute checkout path. Both the
	// File column text and its blob link must show the repo-relative path,
	// and the link must percent-escape any character that would otherwise
	// break the markdown link syntax.
	opts := Options{RepoURL: "https://github.com/user/repo", SHA: "abc123"}

	tests := []struct {
		name     string
		path     string
		opts     Options
		expected string
	}{
		{
			name:     "Go module prefix is stripped from link and text",
			path:     "github.com/user/repo/internal/foo.go",
			opts:     opts,
			expected: "[`internal/foo.go`](https://github.com/user/repo/blob/abc123/internal/foo.go)",
		},
		{
			name:     "absolute CI checkout path is stripped from link and text",
			path:     "/home/runner/work/repo/repo/src/module.py",
			opts:     opts,
			expected: "[`src/module.py`](https://github.com/user/repo/blob/abc123/src/module.py)",
		},
		{
			name:     "already repo-relative path is unchanged",
			path:     "internal/foo.go",
			opts:     opts,
			expected: "[`internal/foo.go`](https://github.com/user/repo/blob/abc123/internal/foo.go)",
		},
		{
			name:     "unnormalized path is shown relative even without a link",
			path:     "github.com/user/repo/internal/foo.go",
			opts:     Options{},
			expected: "`internal/foo.go`",
		},
		{
			name:     "space and # are percent-escaped in the link but not the display text",
			path:     "src/my file #2.go",
			opts:     opts,
			expected: "[`src/my file #2.go`](https://github.com/user/repo/blob/abc123/src/my%20file%20%232.go)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatFileName(tt.path, tt.opts)
			if got != tt.expected {
				t.Errorf("formatFileName(%q) = %q, want %q", tt.path, got, tt.expected)
			}
		})
	}
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
	if !strings.Contains(result, "\U0001F4C8") {
		t.Error("missing chart emoji in footer")
	}
}

func TestFormatWithComparison(t *testing.T) {
	head := &coverage.Report{
		Files: []coverage.FileCoverage{
			{Path: "src/parser.go", LinesCovered: 80, LinesTotal: 100},
			{Path: "src/new.go", LinesCovered: 65, LinesTotal: 100},
		},
		TotalCovered: 145,
		TotalLines:   200,
		Coverage:     72.5,
	}

	base := &coverage.Report{
		Files: []coverage.FileCoverage{
			{Path: "src/parser.go", LinesCovered: 75, LinesTotal: 100},
		},
		TotalCovered: 75,
		TotalLines:   100,
		Coverage:     75.0,
	}

	comp := coverage.NewComparison(head, base, nil)
	opts := Options{
		Title:      "PR Coverage",
		PRNumber:   123,
		BaseBranch: "main",
	}

	result := FormatWithComparison(comp, opts)

	checks := []string{
		Marker,
		"PR Coverage",
		"logo.png",
		"72.50%",
		"(-2.50%)",
		"Coverage Diff",
		"main",
		"#123",
		"Impacted Files",
		"\u0394", // Delta column header
		"LiteCov",
	}

	for _, check := range checks {
		if !strings.Contains(result, check) {
			t.Errorf("missing %q in output", check)
		}
	}
}

func TestFormatWithComparison_Nil(t *testing.T) {
	result := FormatWithComparison(nil, Options{})
	if result != "" {
		t.Error("expected empty string for nil comparison")
	}

	comp := &coverage.Comparison{Head: nil}
	result = FormatWithComparison(comp, Options{})
	if result != "" {
		t.Error("expected empty string for nil head")
	}
}

func TestFormatWithComparison_NoBase(t *testing.T) {
	head := &coverage.Report{
		Files: []coverage.FileCoverage{
			{Path: "src/parser.go", LinesCovered: 80, LinesTotal: 100},
		},
		TotalCovered: 80,
		TotalLines:   100,
		Coverage:     80.0,
	}

	comp := coverage.NewComparison(head, nil, nil)
	opts := Options{
		Title: "Coverage",
	}

	result := FormatWithComparison(comp, opts)

	if !strings.Contains(result, "80.00%") {
		t.Error("missing coverage percentage")
	}
	if strings.Contains(result, "(+") || strings.Contains(result, "(-") {
		t.Error("should not show delta when no base")
	}
}

func TestFormatQuickSummaryWithDelta(t *testing.T) {
	tests := []struct {
		name     string
		comp     *coverage.Comparison
		contains []string
		excludes []string
	}{
		{
			name: "positive delta",
			comp: &coverage.Comparison{
				Head: &coverage.Report{
					TotalCovered: 85,
					TotalLines:   100,
					Coverage:     85.0,
					Files:        make([]coverage.FileCoverage, 5),
				},
				Base: &coverage.Report{
					Coverage: 80.0,
				},
				CoverageDelta: 5.0,
			},
			contains: []string{"85.00%", "(+5.00%)"},
		},
		{
			name: "negative delta",
			comp: &coverage.Comparison{
				Head: &coverage.Report{
					TotalCovered: 75,
					TotalLines:   100,
					Coverage:     75.0,
					Files:        make([]coverage.FileCoverage, 5),
				},
				Base: &coverage.Report{
					Coverage: 80.0,
				},
				CoverageDelta: -5.0,
			},
			contains: []string{"75.00%", "(-5.00%)"},
		},
		{
			name: "zero delta",
			comp: &coverage.Comparison{
				Head: &coverage.Report{
					TotalCovered: 80,
					TotalLines:   100,
					Coverage:     80.0,
					Files:        make([]coverage.FileCoverage, 5),
				},
				Base: &coverage.Report{
					Coverage: 80.0,
				},
				CoverageDelta: 0,
			},
			contains: []string{"80.00%"},
			excludes: []string{"(+", "(-"},
		},
		{
			name: "no base",
			comp: &coverage.Comparison{
				Head: &coverage.Report{
					TotalCovered: 80,
					TotalLines:   100,
					Coverage:     80.0,
					Files:        make([]coverage.FileCoverage, 5),
				},
				Base:          nil,
				CoverageDelta: 0,
			},
			contains: []string{"80.00%"},
			excludes: []string{"(+", "(-"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatQuickSummaryWithDelta(tt.comp)

			for _, check := range tt.contains {
				if !strings.Contains(result, check) {
					t.Errorf("missing %q in output", check)
				}
			}
			for _, check := range tt.excludes {
				if strings.Contains(result, check) {
					t.Errorf("should not contain %q in output", check)
				}
			}
		})
	}
}

func TestFormatCoverageDiffWithComparison(t *testing.T) {
	t.Run("with base", func(t *testing.T) {
		comp := &coverage.Comparison{
			Head: &coverage.Report{
				TotalCovered: 260,
				TotalLines:   300,
				Coverage:     86.67,
				Files:        make([]coverage.FileCoverage, 10),
			},
			Base: &coverage.Report{
				TotalCovered: 200,
				TotalLines:   250,
				Coverage:     80.0,
				Files:        make([]coverage.FileCoverage, 8),
			},
		}

		opts := Options{
			PRNumber:   42,
			BaseBranch: "develop",
		}

		result := formatCoverageDiffWithComparison(comp, opts)

		checks := []string{
			"develop",
			"#42",
			"Coverage Diff",
			"```diff",
			"80.00%",
			"86.67%",
			"Hits",
			"Misses",
		}

		for _, check := range checks {
			if !strings.Contains(result, check) {
				t.Errorf("missing %q in output", check)
			}
		}
	})

	t.Run("without base", func(t *testing.T) {
		comp := &coverage.Comparison{
			Head: &coverage.Report{
				TotalCovered: 80,
				TotalLines:   100,
				Coverage:     80.0,
				Files:        make([]coverage.FileCoverage, 5),
			},
			Base: nil,
		}

		opts := Options{}

		result := formatCoverageDiffWithComparison(comp, opts)

		if !strings.Contains(result, "80.00%") {
			t.Error("missing coverage")
		}
		if !strings.Contains(result, "HEAD") {
			t.Error("should use HEAD when no PR number")
		}
		if !strings.Contains(result, "main") {
			t.Error("should default to main branch")
		}
	})
}

func TestFormatImpactedFilesWithDelta(t *testing.T) {
	fileChanges := []coverage.FileChange{
		{Path: "improved.go", HeadCoverage: 94.20, BaseCoverage: 92.10, Delta: 2.10, IsNew: false},
		{Path: "new.go", HeadCoverage: 65.00, BaseCoverage: 0, Delta: 65.00, IsNew: true},
		{Path: "same.go", HeadCoverage: 80.00, BaseCoverage: 80.00, Delta: 0, IsNew: false},
		{Path: "worse.go", HeadCoverage: 70.00, BaseCoverage: 75.00, Delta: -5.00, IsNew: false},
	}

	opts := Options{
		RepoURL: "https://github.com/test/repo",
		SHA:     "abc123",
	}

	result := formatImpactedFilesWithDelta(fileChanges, opts)

	checks := []string{
		"Impacted Files (4)",
		"\u0394",   // Delta column header
		"`+2.10%`", // positive delta
		"`new`",    // new file indicator
		"`\u00f8`", // zero delta (ø)
		"`-5.00%`", // negative delta
		"improved.go",
		"new.go",
		"same.go",
		"worse.go",
		"\u2705",       // checkmark
		"\u26A0\uFE0F", // warning
	}

	for _, check := range checks {
		if !strings.Contains(result, check) {
			t.Errorf("missing %q in output", check)
		}
	}
}

func TestFormatImpactedFilesWithDelta_NoStatements(t *testing.T) {
	// A file with no coverable lines has nothing to measure, not 0%
	// coverage. Before this was tracked on FileChange, HeadCoverage's
	// fallback-to-0 rendered a flat `0.00%` with a failing ❌, identical to
	// a file whose statements were never hit. See issue #35.
	fileChanges := []coverage.FileChange{
		{Path: "src/__init__.py", HeadCoverage: 0, BaseCoverage: 0, Delta: 0, NoStatements: true},
		{Path: "src/a.py", HeadCoverage: 100, BaseCoverage: 100, Delta: 0},
	}

	result := formatImpactedFilesWithDelta(fileChanges, Options{})

	if strings.Contains(result, "`0.00%`") {
		t.Error("a file with no coverable lines should not render as a flat 0.00%")
	}
	if !strings.Contains(result, "`no statements`") {
		t.Error("missing neutral coverage label for a file with no coverable lines")
	}
	if strings.Contains(result, "❌") {
		t.Error("a file with no coverable lines should get a neutral status, not ❌")
	}
}

func TestFormatImpactedFilesWithDelta_Empty(t *testing.T) {
	result := formatImpactedFilesWithDelta(nil, Options{})
	if result != "" {
		t.Error("expected empty string for no files")
	}
}

func TestFormatFileDelta(t *testing.T) {
	tests := []struct {
		name     string
		fc       coverage.FileChange
		expected string
	}{
		{
			name:     "new file",
			fc:       coverage.FileChange{IsNew: true, Delta: 50.0},
			expected: "`new`",
		},
		{
			name:     "zero delta",
			fc:       coverage.FileChange{IsNew: false, Delta: 0},
			expected: "`\u00f8`",
		},
		{
			name:     "positive delta",
			fc:       coverage.FileChange{IsNew: false, Delta: 5.25},
			expected: "`+5.25%`",
		},
		{
			name:     "negative delta",
			fc:       coverage.FileChange{IsNew: false, Delta: -3.50},
			expected: "`-3.50%`",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatFileDelta(tt.fc)
			if result != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, result)
			}
		})
	}
}

func TestFormatDeltaString(t *testing.T) {
	tests := []struct {
		name     string
		delta    float64
		hasBase  bool
		expected string
	}{
		{
			name:     "no base",
			delta:    5.0,
			hasBase:  false,
			expected: "",
		},
		{
			name:     "zero delta with base",
			delta:    0,
			hasBase:  true,
			expected: "",
		},
		{
			name:     "positive delta",
			delta:    2.50,
			hasBase:  true,
			expected: " (+2.50%)",
		},
		{
			name:     "negative delta",
			delta:    -1.75,
			hasBase:  true,
			expected: " (-1.75%)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatDeltaString(tt.delta, tt.hasBase)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}
