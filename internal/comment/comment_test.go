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
		"Additional details and impacted files",
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

// TestFormat_ImpactedFilesNotCollapsed reproduces issue #21: every number in
// the report used to render behind a collapsed <details> block, so a
// reviewer needed two clicks to see which file lost coverage. The Impacted
// Files table must be visible without expanding anything; only the
// supporting Coverage Diff block stays collapsed, and it must come after the
// table, not before it.
func TestFormat_ImpactedFilesNotCollapsed(t *testing.T) {
	report := &coverage.Report{
		Files: []coverage.FileCoverage{
			{Path: "src/a.go", LinesCovered: 90, LinesTotal: 100},
		},
		TotalCovered: 90,
		TotalLines:   100,
		Coverage:     90.0,
	}

	result := Format(report, Options{Title: "Coverage Report", ShowFiles: "all"})

	filesIdx := strings.Index(result, "Impacted Files (1)")
	if filesIdx == -1 {
		t.Fatal("missing Impacted Files section")
	}
	detailsIdx := strings.Index(result, "<details>")
	if detailsIdx == -1 {
		t.Fatal("missing collapsed section")
	}
	if filesIdx > detailsIdx {
		t.Error("Impacted Files table must render before the collapsed <details> block, not inside it")
	}
	if !strings.Contains(result[:detailsIdx], "src/a.go") {
		t.Error("file row must be visible before the collapsed section, not hidden inside it")
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

// TestFormat_BaseError reproduces issue #39: a base coverage file that
// couldn't be read used to make Format render exactly the layout it would
// for no base having been requested at all, so the comparison silently
// disappeared with no indication anything had gone wrong. BaseError set
// means the comment must say so, while still showing head coverage as
// normal.
func TestFormat_BaseError(t *testing.T) {
	report := &coverage.Report{
		Files:        []coverage.FileCoverage{{Path: "src/a.go", LinesCovered: 5, LinesTotal: 10}},
		TotalCovered: 5,
		TotalLines:   10,
		Coverage:     50,
	}

	opts := Options{
		Title:     "Coverage Report",
		ShowFiles: "all",
		BaseError: "parsing base coverage file: no coverage data found: input contains no valid LCOV records",
	}

	result := Format(report, opts)

	if !strings.Contains(result, "Base coverage unavailable") {
		t.Error("comment does not explain that the configured base coverage file could not be read")
	}
	if !strings.Contains(result, "no coverage data found") {
		t.Error("comment does not surface the underlying reason")
	}
	// The rest of the report still renders: a broken base shouldn't take
	// down the head coverage numbers with it.
	if !strings.Contains(result, "50.00%") {
		t.Error("missing head coverage percentage")
	}
	if !strings.Contains(result, "src/a.go") {
		t.Error("missing head coverage file")
	}
}

// TestFormat_NoBaseError checks the common case doesn't regress: no
// BaseError set means no "unavailable" note in the comment.
func TestFormat_NoBaseError(t *testing.T) {
	report := &coverage.Report{
		Files:        []coverage.FileCoverage{{Path: "src/a.go", LinesCovered: 5, LinesTotal: 10}},
		TotalCovered: 5,
		TotalLines:   10,
		Coverage:     50,
	}

	result := Format(report, Options{Title: "Coverage Report", ShowFiles: "all"})

	if strings.Contains(result, "Base coverage unavailable") {
		t.Error("comment should not mention base coverage when BaseError is unset")
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

func TestFormatBaseError(t *testing.T) {
	t.Run("empty when no error", func(t *testing.T) {
		if result := formatBaseError(Options{}); result != "" {
			t.Errorf("formatBaseError with no BaseError = %q, want empty", result)
		}
	})

	t.Run("explains the failure when set", func(t *testing.T) {
		result := formatBaseError(Options{BaseError: "opening base coverage file: open base.lcov: no such file or directory"})
		if !strings.Contains(result, "Base coverage unavailable") {
			t.Error("missing explanation that base coverage is unavailable")
		}
		if !strings.Contains(result, "no such file or directory") {
			t.Error("missing underlying reason")
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

	result := formatQuickSummary(report, Options{})

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
	if strings.Contains(result, "Patch") {
		t.Error("should not show Patch when no patch data was computed")
	}
}

func TestFormatQuickSummary_WithPatchCoverage(t *testing.T) {
	// issue #6: the summary line's Patch segment shows the coverage of only
	// the lines the PR added, not the whole-project number formatQuickSummary
	// already prints.
	report := &coverage.Report{
		TotalCovered: 850,
		TotalLines:   1000,
		Coverage:     85.0,
		Files:        make([]coverage.FileCoverage, 10),
	}
	opts := Options{PatchCoverage: coverage.PatchCoverage{Covered: 17, Total: 20}}

	result := formatQuickSummary(report, opts)

	if !strings.Contains(result, "**Patch:** `85.00%`") {
		t.Errorf("missing patch coverage segment in %q", result)
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
	if !strings.Contains(result, "Additional details and impacted files") {
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
	// Issue #21: the table must render inline, not behind a <details> click.
	if strings.Contains(result, "<details>") {
		t.Error("Impacted Files table should not be collapsed behind <details>")
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

// TestFormatImpactedFiles_UsesPatchCoverageWhenAvailable reproduces issue #9:
// a file at 95% whole-file coverage whose PR-added lines are all untested
// must not render as 95% with a passing status. When FilePatches has an
// entry for a file, its patch percentage and patch-scoped uncovered lines
// replace the whole-file numbers.
func TestFormatImpactedFiles_UsesPatchCoverageWhenAvailable(t *testing.T) {
	files := []coverage.FileCoverage{
		{
			Path:           "big.go",
			LinesCovered:   950,
			LinesTotal:     1000,
			UncoveredLines: []int{10, 500, 501, 502, 900}, // 500-502 are the PR's new lines
		},
	}
	opts := Options{
		FilePatches: map[string]coverage.FilePatch{
			"big.go": {
				Coverage:       coverage.PatchCoverage{Covered: 0, Total: 3},
				UncoveredLines: []int{500, 501, 502},
			},
		},
	}

	result := formatImpactedFiles(files, opts)

	if !strings.Contains(result, "`0.00%`") {
		t.Errorf("expected the file's 0%% patch coverage, not its 95%% whole-file average, in %q", result)
	}
	if strings.Contains(result, "95.00%") {
		t.Errorf("whole-file coverage leaked into the row despite patch data being available: %q", result)
	}
	if !strings.Contains(result, "❌") {
		t.Error("expected a failing status for 0% patch coverage, not a passing one derived from whole-file coverage")
	}
	if strings.Contains(result, "L10") || strings.Contains(result, "L900") {
		t.Errorf("Uncovered Lines must be restricted to the diff, not the whole file's uncovered lines: %q", result)
	}
	if !strings.Contains(result, "L500-502") {
		t.Errorf("missing the patch's own uncovered range in %q", result)
	}
}

// TestFormatImpactedFiles_NoPatchDataFallsBackToWholeFile keeps the
// pre-issue-#9 behavior for a file FilePatches has no entry for: no PR diff
// was available, the file wasn't part of it, or none of its added lines were
// ever instrumented.
func TestFormatImpactedFiles_NoPatchDataFallsBackToWholeFile(t *testing.T) {
	files := []coverage.FileCoverage{
		{Path: "a.go", LinesCovered: 90, LinesTotal: 100, UncoveredLines: []int{5, 6}},
	}

	result := formatImpactedFiles(files, Options{FilePatches: map[string]coverage.FilePatch{}})

	if !strings.Contains(result, "`90.00%`") {
		t.Errorf("expected whole-file 90%% when no patch entry exists, got %q", result)
	}
	if !strings.Contains(result, "L5-6") {
		t.Errorf("expected whole-file uncovered lines when no patch entry exists, got %q", result)
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
	// Issue #21: this section must not be collapsed either.
	if strings.Contains(result, "<details>") {
		t.Error("empty Impacted Files section should not be collapsed behind <details>")
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

	comp := coverage.NewComparison(head, base, nil, nil, nil)
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

// TestFormatWithComparison_ImpactedFilesNotCollapsed is the comparison-path
// counterpart of TestFormat_ImpactedFilesNotCollapsed (issue #21): the
// Impacted Files table must render before the collapsed Coverage Diff
// block here too.
func TestFormatWithComparison_ImpactedFilesNotCollapsed(t *testing.T) {
	head := &coverage.Report{
		Files: []coverage.FileCoverage{
			{Path: "src/a.go", LinesCovered: 90, LinesTotal: 100},
		},
		TotalCovered: 90,
		TotalLines:   100,
		Coverage:     90.0,
	}
	comp := coverage.NewComparison(head, nil, nil, nil, nil)

	result := FormatWithComparison(comp, Options{Title: "Coverage Report"})

	filesIdx := strings.Index(result, "Impacted Files (1)")
	if filesIdx == -1 {
		t.Fatal("missing Impacted Files section")
	}
	detailsIdx := strings.Index(result, "<details>")
	if detailsIdx == -1 {
		t.Fatal("missing collapsed section")
	}
	if filesIdx > detailsIdx {
		t.Error("Impacted Files table must render before the collapsed <details> block, not inside it")
	}
	if !strings.Contains(result[:detailsIdx], "src/a.go") {
		t.Error("file row must be visible before the collapsed section, not hidden inside it")
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

	comp := coverage.NewComparison(head, nil, nil, nil, nil)
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

func TestFormatWithComparison_EmptyBase(t *testing.T) {
	// Reproduces the issue #32 repro: an empty-but-present base report must
	// not render as if every point of head coverage were an improvement
	// over a real 0% base.
	head := &coverage.Report{
		Files: []coverage.FileCoverage{
			{Path: "a.go", LinesCovered: 90, LinesTotal: 100},
			{Path: "b.go", LinesCovered: 80, LinesTotal: 100},
		},
		TotalCovered: 170,
		TotalLines:   200,
		Coverage:     85.0,
	}
	base := &coverage.Report{
		Files:    []coverage.FileCoverage{},
		Coverage: 0,
	}

	comp := coverage.NewComparison(head, base, nil, nil, nil)
	opts := Options{Title: "Coverage"}

	result := FormatWithComparison(comp, opts)

	if !strings.Contains(result, "85.00%") {
		t.Error("missing head coverage percentage")
	}
	if strings.Contains(result, "+85.00%") {
		t.Error("should not report head coverage as an 85-point improvement over an empty base report")
	}
}

func TestFormatQuickSummaryWithDelta(t *testing.T) {
	tests := []struct {
		name     string
		comp     *coverage.Comparison
		opts     Options
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
			// issue #38: an exactly-zero delta must show ø, not disappear
			// from the header.
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
			contains: []string{"80.00%", "(ø)"},
			excludes: []string{"(+", "(-"},
		},
		{
			// issue #38 repro: base 91230/100000, head 91234/100000. The raw
			// delta of 0.004 rounds to 0.00 at display precision and must
			// render as ø, not "(+0.00%)".
			name: "sub-precision delta rounds to ø",
			comp: &coverage.Comparison{
				Head: &coverage.Report{
					TotalCovered: 91234,
					TotalLines:   100000,
					Coverage:     91.234,
					Files:        make([]coverage.FileCoverage, 1),
				},
				Base: &coverage.Report{
					Coverage: 91.23,
				},
				CoverageDelta: 0.004,
			},
			contains: []string{"91.23%", "(ø)"},
			excludes: []string{"(+", "(-", "+0.00%"},
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
		{
			// issue #32: a present-but-empty base report must render like
			// no base at all, not like a real 0% measurement.
			name: "empty base report",
			comp: &coverage.Comparison{
				Head: &coverage.Report{
					TotalCovered: 85,
					TotalLines:   100,
					Coverage:     85.0,
					Files:        make([]coverage.FileCoverage, 5),
				},
				Base:          &coverage.Report{Coverage: 0, Files: []coverage.FileCoverage{}},
				NoBaseFiles:   true,
				CoverageDelta: 0,
			},
			contains: []string{"85.00%"},
			excludes: []string{"(+", "(-"},
		},
		{
			// issue #6: patch coverage renders alongside the delta when both
			// are available, independent of one another.
			name: "with patch coverage",
			comp: &coverage.Comparison{
				Head: &coverage.Report{
					TotalCovered: 85,
					TotalLines:   100,
					Coverage:     85.0,
					Files:        make([]coverage.FileCoverage, 5),
				},
				Base:          &coverage.Report{Coverage: 80.0},
				CoverageDelta: 5.0,
			},
			opts:     Options{PatchCoverage: coverage.PatchCoverage{Covered: 9, Total: 10}},
			contains: []string{"85.00%", "(+5.00%)", "**Patch:** `90.00%`"},
		},
		{
			// A patch total of 0 means no patch data was available, not a
			// genuine 0% patch, so the segment must not appear at all.
			name: "no patch data",
			comp: &coverage.Comparison{
				Head: &coverage.Report{
					TotalCovered: 85,
					TotalLines:   100,
					Coverage:     85.0,
					Files:        make([]coverage.FileCoverage, 5),
				},
				Base:          &coverage.Report{Coverage: 80.0},
				CoverageDelta: 5.0,
			},
			contains: []string{"85.00%"},
			excludes: []string{"Patch"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatQuickSummaryWithDelta(tt.comp, tt.opts)

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

	t.Run("empty base", func(t *testing.T) {
		// issue #32: a base report present but with zero files must render
		// the same single-column format as no base at all, not a diff
		// against a fabricated 0% base.
		comp := &coverage.Comparison{
			Head: &coverage.Report{
				TotalCovered: 170,
				TotalLines:   200,
				Coverage:     85.0,
				Files:        make([]coverage.FileCoverage, 2),
			},
			Base:        &coverage.Report{Coverage: 0, Files: []coverage.FileCoverage{}},
			NoBaseFiles: true,
		}

		opts := Options{}

		result := formatCoverageDiffWithComparison(comp, opts)

		if !strings.Contains(result, "85.00%") {
			t.Error("missing head coverage")
		}
		if strings.Contains(result, "+85.00%") {
			t.Error("should not render the empty base as a real 0% measurement")
		}
	})

	t.Run("sub-precision coverage delta renders as ø", func(t *testing.T) {
		// issue #38 repro: base 91230/100000 (91.23%), head 91234/100000
		// (91.234%). The raw delta is 0.004, which rounds to 0.00 at the two
		// decimals actually displayed, so the Coverage row must render ø
		// with a blank prefix, not "+0.00%". Files and Lines are unchanged
		// too, so their delta column must read plain 0, not +0.
		comp := &coverage.Comparison{
			Head: &coverage.Report{
				TotalCovered: 91234,
				TotalLines:   100000,
				Coverage:     91.234,
				Files:        make([]coverage.FileCoverage, 1),
			},
			Base: &coverage.Report{
				TotalCovered: 91230,
				TotalLines:   100000,
				Coverage:     91.23,
				Files:        make([]coverage.FileCoverage, 1),
			},
		}

		result := formatCoverageDiffWithComparison(comp, Options{})

		if strings.Contains(result, "+0.00%") {
			t.Error("a sub-precision coverage delta should not render as +0.00%")
		}
		if !strings.Contains(result, "ø") {
			t.Error("a sub-precision coverage delta should render as ø")
		}
		if strings.Contains(result, "+ Coverage") {
			t.Error("a sub-precision coverage delta should not get a + diff prefix")
		}
		if strings.Contains(result, "+0") {
			t.Error("unchanged Files/Lines counts should not render with a + sign")
		}
		if !strings.Contains(result, "+4") || !strings.Contains(result, "-4") {
			t.Error("a real Hits/Misses change should still render with its sign")
		}
	})

	t.Run("identical head and base render ø and 0, not +0.00% or +0", func(t *testing.T) {
		// issue #38: an exactly-zero delta must not force a + sign either --
		// %+.2f%% and %+d print "+0.00%"/"+0" for a real 0 just as readily
		// as for a sub-precision one.
		comp := &coverage.Comparison{
			Head: &coverage.Report{
				TotalCovered: 100,
				TotalLines:   100,
				Coverage:     100.0,
				Files:        make([]coverage.FileCoverage, 3),
			},
			Base: &coverage.Report{
				TotalCovered: 100,
				TotalLines:   100,
				Coverage:     100.0,
				Files:        make([]coverage.FileCoverage, 3),
			},
		}

		result := formatCoverageDiffWithComparison(comp, Options{})

		if strings.Contains(result, "+0") {
			t.Error("an unchanged report should not render any +0 delta")
		}
		if strings.Contains(result, "+0.00%") {
			t.Error("an exactly-zero coverage delta should not render as +0.00%")
		}
		if !strings.Contains(result, "ø") {
			t.Error("an exactly-zero coverage delta should render as ø")
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
	// Issue #21: the table must render inline, not behind a <details> click.
	if strings.Contains(result, "<details>") {
		t.Error("Impacted Files table should not be collapsed behind <details>")
	}
}

// TestFormatImpactedFilesWithDelta_UncoveredLines reproduces issue #44: the
// comparison table used to drop Uncovered Lines entirely in favor of Δ, so a
// PR run configured with a base coverage file lost the only actionable
// column in the report. Both must render in the same row.
func TestFormatImpactedFilesWithDelta_UncoveredLines(t *testing.T) {
	fileChanges := []coverage.FileChange{
		{
			Path:           "src/parser.go",
			HeadCoverage:   94.00,
			BaseCoverage:   92.00,
			Delta:          2.00,
			UncoveredLines: []int{12, 13, 40},
		},
	}

	result := formatImpactedFilesWithDelta(fileChanges, Options{})

	if !strings.Contains(result, "Uncovered Lines") {
		t.Error("comparison table is missing the Uncovered Lines column header")
	}
	if !strings.Contains(result, "L12-13") {
		t.Errorf("missing uncovered line range in %q", result)
	}
	if !strings.Contains(result, "L40") {
		t.Errorf("missing uncovered line 40 in %q", result)
	}
	if !strings.Contains(result, "`+2.00%`") {
		t.Errorf("Δ must still render alongside Uncovered Lines, not be replaced by it: %q", result)
	}
}

// TestFormatImpactedFilesWithDelta_UsesPatchCoverageWhenAvailable is the
// comparison-table counterpart of
// TestFormatImpactedFiles_UsesPatchCoverageWhenAvailable (issue #9): a file
// with patch data must show only its PR-added uncovered lines, not every
// uncovered line the whole file has ever had.
func TestFormatImpactedFilesWithDelta_UsesPatchCoverageWhenAvailable(t *testing.T) {
	fileChanges := []coverage.FileChange{
		{
			Path:           "big.go",
			HeadCoverage:   95.00,
			BaseCoverage:   95.00,
			Delta:          0,
			UncoveredLines: []int{10, 500, 501, 502, 900},
		},
	}
	opts := Options{
		FilePatches: map[string]coverage.FilePatch{
			"big.go": {
				Coverage:       coverage.PatchCoverage{Covered: 0, Total: 3},
				UncoveredLines: []int{500, 501, 502},
			},
		},
	}

	result := formatImpactedFilesWithDelta(fileChanges, opts)

	if strings.Contains(result, "L10") || strings.Contains(result, "L900") {
		t.Errorf("Uncovered Lines must be restricted to the diff, not the whole file's uncovered lines: %q", result)
	}
	if !strings.Contains(result, "L500-502") {
		t.Errorf("missing the patch's own uncovered range in %q", result)
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

func TestFormatImpactedFilesWithDelta_NoCoverage(t *testing.T) {
	// A changed file absent from the coverage report is not a measured 0%:
	// it could be genuinely untested, excluded from instrumentation, or
	// simply not built by the job that produced the report. Before this,
	// HeadCoverage's fallback-to-0 rendered a flat `0.00%` with a failing
	// ❌ and a delta column asserting "no tests", none of which the tool
	// actually knows. See issue #34.
	fileChanges := []coverage.FileChange{
		{Path: "internal/bar/b.go", HeadCoverage: 0, BaseCoverage: 0, Delta: 0, IsNew: true, NoCoverage: true},
		{Path: "src/a.py", HeadCoverage: 100, BaseCoverage: 100, Delta: 0},
	}

	result := formatImpactedFilesWithDelta(fileChanges, Options{})

	if strings.Contains(result, "`0.00%`") {
		t.Error("a file missing from the coverage report should not render as a flat 0.00%")
	}
	if strings.Contains(result, "❌") {
		t.Error("a file missing from the coverage report should get a neutral status, not ❌")
	}
	if strings.Contains(result, "no tests") {
		t.Error("absence from the report should not be asserted as \"no tests\"")
	}
	if !strings.Contains(result, "❓") {
		t.Error("missing neutral unknown status for a file absent from the coverage report")
	}
}

func TestFormatImpactedFilesWithDelta_IsDeleted(t *testing.T) {
	// A file the PR diff reports as removed must not render like
	// TestFormatImpactedFilesWithDelta_NoCoverage's "unknown" 0%: the tool
	// knows exactly why there's no head measurement, so it shows the last
	// known (base) coverage and a distinct removed marker instead. See
	// issue #31.
	fileChanges := []coverage.FileChange{
		{Path: "internal/bar/b.go", BaseCoverage: 100, Delta: -100, IsDeleted: true},
		{Path: "src/a.py", HeadCoverage: 100, BaseCoverage: 100, Delta: 0},
	}

	result := formatImpactedFilesWithDelta(fileChanges, Options{})

	if !strings.Contains(result, "`100.00%`") {
		t.Error("a deleted file should show its last known (base) coverage")
	}
	if strings.Contains(result, "`no coverage data`") {
		t.Error("a deleted file should not render as NoCoverage's \"no coverage data\"")
	}
	if strings.Contains(result, "❓") {
		t.Error("a deleted file should get its own removed marker, not NoCoverage's ❓")
	}
	if !strings.Contains(result, "`removed`") {
		t.Error("missing the removed marker for a deleted file")
	}
	if !strings.Contains(result, "🗑️") {
		t.Error("missing the removed status emoji for a deleted file")
	}
}

func TestFormatImpactedFilesWithDelta_IsDeleted_NoBaseData(t *testing.T) {
	// A file confirmed removed by the diff but with no matching base
	// coverage entry either: there's no percentage to show, so the
	// coverage column falls back to the same "removed" text as the delta
	// column instead of a misleading 0.00%.
	fileChanges := []coverage.FileChange{
		{Path: "internal/bar/b.go", IsDeleted: true, NoBaseData: true},
	}

	result := formatImpactedFilesWithDelta(fileChanges, Options{})

	if strings.Contains(result, "`0.00%`") {
		t.Error("a deleted file with no base data should not render as a flat 0.00%")
	}
	if !strings.Contains(result, "`removed`") {
		t.Error("missing the removed marker for a deleted file with no base data")
	}
}

// TestFormatImpactedFilesWithDelta_UncoveredLines_NoHeadData checks that a
// row with no head measurement (NoStatements, NoCoverage, or IsDeleted)
// renders "-" for Uncovered Lines instead of stale or zero-value line data,
// matching how those rows already fall back for Coverage (issue #44).
func TestFormatImpactedFilesWithDelta_UncoveredLines_NoHeadData(t *testing.T) {
	fileChanges := []coverage.FileChange{
		{Path: "src/__init__.py", NoStatements: true},
		{Path: "internal/bar/b.go", NoCoverage: true},
		{Path: "internal/bar/c.go", IsDeleted: true, BaseCoverage: 100},
	}

	result := formatImpactedFilesWithDelta(fileChanges, Options{})

	lines := strings.Split(result, "\n")
	for _, path := range []string{"src/__init__.py", "internal/bar/b.go", "internal/bar/c.go"} {
		found := false
		for _, line := range lines {
			if !strings.Contains(line, path) {
				continue
			}
			found = true
			cells := strings.Split(line, "|")
			if len(cells) < 5 {
				t.Fatalf("row for %s has too few columns: %q", path, line)
			}
			if got := strings.TrimSpace(cells[4]); got != "-" {
				t.Errorf("Uncovered Lines for %s = %q, want \"-\"", path, got)
			}
		}
		if !found {
			t.Errorf("missing row for %s", path)
		}
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
		{
			// issue #34: absence from the report is unknown, not a delta to
			// zero, and takes priority over IsNew.
			name:     "no coverage data",
			fc:       coverage.FileChange{IsNew: true, NoCoverage: true, Delta: 0},
			expected: "`unknown`",
		},
		{
			// issue #31: a file the PR diff confirms was removed gets its
			// own "removed" label rather than NoCoverage's "unknown",
			// since the tool knows exactly why head data is missing.
			name:     "removed file",
			fc:       coverage.FileChange{IsDeleted: true, BaseCoverage: 100, Delta: -100},
			expected: "`removed`",
		},
		{
			// issue #32: no base entry, and the PR diff didn't call this
			// file added either. The prior measurement is unknown, not a
			// delta from 0%.
			name:     "no base data",
			fc:       coverage.FileChange{IsNew: false, NoBaseData: true, Delta: 0},
			expected: "`unknown`",
		},
		{
			// issue #38: a delta that rounds to 0.00% at display precision
			// must render as ø, not "+0.00%" -- a change that small isn't
			// visible at two decimals and shouldn't be printed as one.
			name:     "sub-precision positive delta rounds to ø",
			fc:       coverage.FileChange{IsNew: false, Delta: 0.004},
			expected: "`ø`",
		},
		{
			name:     "sub-precision negative delta rounds to ø",
			fc:       coverage.FileChange{IsNew: false, Delta: -0.004},
			expected: "`ø`",
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
			// issue #38: an exactly-zero delta must say so with ø, not drop
			// out of the header as if no base had been configured at all.
			name:     "zero delta with base",
			delta:    0,
			hasBase:  true,
			expected: " (ø)",
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
		{
			// issue #38 repro: a raw delta of 0.004 rounds to 0.00 at the
			// two decimals actually displayed, so it must render as ø
			// instead of the misleading "(+0.00%)" the unrounded comparison
			// produced.
			name:     "sub-precision positive delta rounds to ø",
			delta:    0.004,
			hasBase:  true,
			expected: " (ø)",
		},
		{
			name:     "sub-precision negative delta rounds to ø",
			delta:    -0.004,
			hasBase:  true,
			expected: " (ø)",
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
