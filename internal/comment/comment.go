package comment

import (
	"fmt"
	"net/url"
	"sort"
	"strings"

	"github.com/manashmandal/litecov/internal/coverage"
	"github.com/manashmandal/litecov/internal/paths"
)

const Marker = "<!-- litecov -->"

type Options struct {
	Title        string
	ShowFiles    string
	ChangedFiles []string
	Threshold    float64
	WorstN       int
	RepoURL      string
	SHA          string
	PRNumber     int
	BaseBranch   string
}

func Format(report *coverage.Report, opts Options) string {
	var sb strings.Builder

	sb.WriteString(Marker)
	sb.WriteString("\n")

	sb.WriteString(formatHeader(opts))
	sb.WriteString(formatQuickSummary(report))
	sb.WriteString(formatCoverageDiff(report))

	filesToShow := filterFiles(report.Files, opts)

	// Add files with no coverage when showing changed files
	if opts.ShowFiles == "changed" && len(opts.ChangedFiles) > 0 {
		missingFiles := findMissingFiles(report, opts.ChangedFiles)
		for _, path := range missingFiles {
			filesToShow = append(filesToShow, coverage.FileCoverage{
				Path:         path,
				LinesCovered: 0,
				LinesTotal:   0,
			})
		}
	}

	sb.WriteString(formatImpactedFiles(filesToShow, opts))

	sb.WriteString(formatFooter())

	return sb.String()
}

// findMissingFiles returns changed source files that are not in the coverage report
func findMissingFiles(report *coverage.Report, changedFiles []string) []string {
	// Build map of covered files
	coveredPaths := make(map[string]bool)
	for _, f := range report.Files {
		coveredPaths[f.Path] = true
		// Also add normalized paths for suffix matching
	}

	var missing []string
	for _, changedFile := range changedFiles {
		if !paths.IsSourceFile(changedFile) {
			continue
		}
		// Check if file is in coverage report (direct or suffix match)
		found := false
		if coveredPaths[changedFile] {
			found = true
		} else {
			for coveredPath := range coveredPaths {
				if paths.HasSuffix(coveredPath, changedFile) || paths.HasSuffix(changedFile, coveredPath) {
					found = true
					break
				}
			}
		}
		if !found {
			missing = append(missing, changedFile)
		}
	}
	return missing
}

func FormatWithComparison(comp *coverage.Comparison, opts Options) string {
	if comp == nil || comp.Head == nil {
		return ""
	}

	var sb strings.Builder

	sb.WriteString(Marker)
	sb.WriteString("\n")

	sb.WriteString(formatHeader(opts))
	sb.WriteString(formatQuickSummaryWithDelta(comp))
	sb.WriteString(formatCoverageDiffWithComparison(comp, opts))
	sb.WriteString(formatImpactedFilesWithDelta(comp.FileChanges, opts))
	sb.WriteString(formatFooter())

	return sb.String()
}

func formatHeader(opts Options) string {
	title := opts.Title
	if title == "" {
		title = "Coverage Report"
	}
	logo := `<img src="https://raw.githubusercontent.com/manashmandal/litecov/main/logo.png" height="24" align="absmiddle">`
	return fmt.Sprintf("## %s %s\n\n", logo, title)
}

func formatQuickSummary(report *coverage.Report) string {
	emoji := getStatusEmoji(report.Coverage)
	return fmt.Sprintf("> %s **Coverage:** `%.2f%%` | **Lines:** `%d/%d` | **Files:** `%d`\n\n",
		emoji, report.Coverage, report.TotalCovered, report.TotalLines, len(report.Files))
}

func formatQuickSummaryWithDelta(comp *coverage.Comparison) string {
	emoji := getStatusEmoji(comp.Head.Coverage)
	// A present-but-empty base report leaves nothing to diff against, same
	// as no base report at all: showing CoverageDelta here would claim an
	// improvement over a measurement that was never taken (issue #32).
	delta := formatDeltaString(comp.CoverageDelta, comp.Base != nil && !comp.NoBaseFiles)
	return fmt.Sprintf("> %s **Coverage:** `%.2f%%`%s | **Lines:** `%d/%d` | **Files:** `%d`\n\n",
		emoji, comp.Head.Coverage, delta, comp.Head.TotalCovered, comp.Head.TotalLines, len(comp.Head.Files))
}

func formatDeltaString(delta float64, hasBase bool) string {
	if !hasBase {
		return ""
	}
	if delta == 0 {
		return ""
	}
	if delta > 0 {
		return fmt.Sprintf(" (+%.2f%%)", delta)
	}
	return fmt.Sprintf(" (%.2f%%)", delta)
}

func formatCoverageDiff(report *coverage.Report) string {
	var sb strings.Builder

	sb.WriteString("<details>\n")
	sb.WriteString("<summary>Coverage Diff</summary>\n\n")
	sb.WriteString("```diff\n")
	sb.WriteString("@@         Coverage Summary            @@\n")
	sb.WriteString("==========================================\n")
	sb.WriteString(fmt.Sprintf("  Coverage              %.2f%%\n", report.Coverage))
	sb.WriteString(fmt.Sprintf("  Lines           %d/%d\n", report.TotalCovered, report.TotalLines))
	sb.WriteString(fmt.Sprintf("  Files                   %d\n", len(report.Files)))
	sb.WriteString("==========================================\n")
	sb.WriteString("```\n\n")
	sb.WriteString("</details>\n\n")

	return sb.String()
}

func formatCoverageDiffWithComparison(comp *coverage.Comparison, opts Options) string {
	var sb strings.Builder

	sb.WriteString("<details>\n")
	sb.WriteString("<summary>Coverage Diff</summary>\n\n")
	sb.WriteString("```diff\n")

	baseBranch := opts.BaseBranch
	if baseBranch == "" {
		baseBranch = "main"
	}
	prRef := fmt.Sprintf("#%d", opts.PRNumber)
	if opts.PRNumber == 0 {
		prRef = "HEAD"
	}

	sb.WriteString("@@              Coverage Diff              @@\n")
	sb.WriteString(fmt.Sprintf("##           %8s   %8s     +/-   ##\n", baseBranch, prRef))
	sb.WriteString("=============================================\n")

	// A present-but-empty base report (NoBaseFiles) is treated the same as
	// no base report at all: falling into the single-column branch below
	// avoids presenting comp.Base.Coverage's 0 fallback as a real
	// measurement (issue #32).
	if comp.Base != nil && !comp.NoBaseFiles {
		coverageDiff := comp.Head.Coverage - comp.Base.Coverage
		prefix := " "
		if coverageDiff > 0 {
			prefix = "+"
		} else if coverageDiff < 0 {
			prefix = "-"
		}
		sb.WriteString(fmt.Sprintf("%s Coverage     %6.2f%%   %6.2f%%   %+.2f%%\n",
			prefix, comp.Base.Coverage, comp.Head.Coverage, coverageDiff))
	} else {
		sb.WriteString(fmt.Sprintf("  Coverage              %6.2f%%\n", comp.Head.Coverage))
	}

	sb.WriteString("=============================================\n")

	if comp.Base != nil && !comp.NoBaseFiles {
		filesDiff := len(comp.Head.Files) - len(comp.Base.Files)
		sb.WriteString(fmt.Sprintf("  Files           %4d      %4d   %+5d\n",
			len(comp.Base.Files), len(comp.Head.Files), filesDiff))

		linesDiff := comp.Head.TotalLines - comp.Base.TotalLines
		sb.WriteString(fmt.Sprintf("  Lines          %5d     %5d   %+5d\n",
			comp.Base.TotalLines, comp.Head.TotalLines, linesDiff))
	} else {
		sb.WriteString(fmt.Sprintf("  Files                     %4d\n", len(comp.Head.Files)))
		sb.WriteString(fmt.Sprintf("  Lines                    %5d\n", comp.Head.TotalLines))
	}

	sb.WriteString("=============================================\n")

	if comp.Base != nil && !comp.NoBaseFiles {
		hitsDiff := comp.Head.Hits() - comp.Base.Hits()
		hitsPrefix := " "
		if hitsDiff > 0 {
			hitsPrefix = "+"
		} else if hitsDiff < 0 {
			hitsPrefix = "-"
		}
		sb.WriteString(fmt.Sprintf("%s Hits          %5d     %5d   %+5d\n",
			hitsPrefix, comp.Base.Hits(), comp.Head.Hits(), hitsDiff))

		missesDiff := comp.Head.Misses() - comp.Base.Misses()
		missesPrefix := " "
		if missesDiff < 0 {
			missesPrefix = "+"
		} else if missesDiff > 0 {
			missesPrefix = "-"
		}
		sb.WriteString(fmt.Sprintf("%s Misses        %5d     %5d   %+5d\n",
			missesPrefix, comp.Base.Misses(), comp.Head.Misses(), missesDiff))
	} else {
		sb.WriteString(fmt.Sprintf("  Hits                     %5d\n", comp.Head.Hits()))
		sb.WriteString(fmt.Sprintf("  Misses                   %5d\n", comp.Head.Misses()))
	}

	sb.WriteString("```\n\n")
	sb.WriteString("</details>\n\n")

	return sb.String()
}

func formatImpactedFiles(files []coverage.FileCoverage, opts Options) string {
	if len(files) == 0 {
		if opts.ShowFiles == "changed" {
			return formatNoChangedFiles()
		}
		return ""
	}

	var sb strings.Builder

	sb.WriteString("<details>\n")
	sb.WriteString(fmt.Sprintf("<summary>Impacted Files (%d)</summary>\n\n", len(files)))
	sb.WriteString("| File | Coverage | Uncovered Lines | Status |\n")
	sb.WriteString("|------|----------|-----------------|--------|\n")

	for _, f := range files {
		pct := f.Percentage()
		emoji := getStatusEmoji(pct)
		fileName := formatFileName(f.Path, opts)
		coverageStr := fmt.Sprintf("`%.2f%%`", pct)
		uncoveredStr := formatUncoveredLines(f.UncoveredLines, opts.RepoURL, opts.SHA, f.Path)
		// Mark files with no coverage data
		if f.LinesTotal == 0 {
			coverageStr = "`⚠️ no tests`"
			uncoveredStr = "-"
			emoji = "❌"
		}
		sb.WriteString(fmt.Sprintf("| %s | %s | %s | %s |\n", fileName, coverageStr, uncoveredStr, emoji))
	}

	sb.WriteString("\n</details>\n\n")

	return sb.String()
}

// formatNoChangedFiles renders the Impacted Files section for show-files:
// changed when the PR's changed files were determined successfully but none
// of them matched the coverage report. Rendering an explanatory section
// beats both a silently blank one and, per issue #20, silently substituting
// every file in the report.
func formatNoChangedFiles() string {
	var sb strings.Builder

	sb.WriteString("<details>\n")
	sb.WriteString("<summary>Impacted Files (0)</summary>\n\n")
	sb.WriteString("No changed files matched the coverage report.\n\n")
	sb.WriteString("</details>\n\n")

	return sb.String()
}

func formatImpactedFilesWithDelta(fileChanges []coverage.FileChange, opts Options) string {
	if len(fileChanges) == 0 {
		return ""
	}

	var sb strings.Builder

	sb.WriteString("<details>\n")
	sb.WriteString(fmt.Sprintf("<summary>Impacted Files (%d)</summary>\n\n", len(fileChanges)))
	sb.WriteString("| File | Coverage | \u0394 | Status |\n")
	sb.WriteString("|------|----------|---|--------|\n")

	for _, fc := range fileChanges {
		fileName := formatFileName(fc.Path, opts)
		deltaStr := formatFileDelta(fc)
		coverageStr := fmt.Sprintf("`%.2f%%`", fc.HeadCoverage)
		emoji := getStatusEmoji(fc.HeadCoverage)
		// A file with no coverable lines has nothing to measure, not 0%
		// coverage: HeadCoverage falls back to 0 for LinesTotal == 0, which
		// would otherwise render identically to a file whose statements
		// were never hit (issue #35).
		if fc.NoStatements {
			coverageStr = "`no statements`"
			emoji = "➖"
		}
		// A file absent from the coverage report has nothing to measure
		// either: HeadCoverage falls back to 0 the same way, which would
		// otherwise render identically to a file whose statements were all
		// missed. Absence has several causes besides "untested": excluded
		// from instrumentation, not built by the job that produced the
		// report, a path that never matched. Render it as unknown rather
		// than asserting a failing grade (issue #34).
		if fc.NoCoverage {
			coverageStr = "`no coverage data`"
			emoji = "❓"
		}
		// A file the PR diff reports as removed has no head measurement for
		// a known reason: it doesn't exist anymore. Show what it measured
		// in base instead of NoCoverage's "unknown", which would say the
		// same thing about a deleted file as about one that's still there
		// but wasn't measured (issue #31).
		if fc.IsDeleted {
			if fc.NoBaseData {
				coverageStr = "`removed`"
			} else {
				coverageStr = fmt.Sprintf("`%.2f%%`", fc.BaseCoverage)
			}
			emoji = "🗑️"
		}
		sb.WriteString(fmt.Sprintf("| %s | %s | %s | %s |\n", fileName, coverageStr, deltaStr, emoji))
	}

	sb.WriteString("\n</details>\n\n")

	return sb.String()
}

func formatFileDelta(fc coverage.FileChange) string {
	if fc.IsDeleted {
		// Confirmed removed by the PR diff, so "unknown" would undersell
		// what's known here: the file is gone, not just unmeasured
		// (issue #31).
		return "`removed`"
	}
	if fc.NoCoverage {
		return "`unknown`"
	}
	if fc.IsNew {
		return "`new`"
	}
	if fc.NoBaseData {
		// No base entry, and the PR diff didn't call this file added
		// either: the prior measurement is unknown, not a delta from 0%
		// (issue #32).
		return "`unknown`"
	}
	if fc.Delta == 0 {
		return "`ø`"
	}
	if fc.Delta > 0 {
		return fmt.Sprintf("`+%.2f%%`", fc.Delta)
	}
	return fmt.Sprintf("`%.2f%%`", fc.Delta)
}

// formatFileName renders a coverage report path as an inline code span,
// linked to the file's blob at opts.SHA when a repo URL is configured. The
// path is normalized to repo-relative first (see NormalizePathForAnnotation)
// since a Go coverage profile carries the module prefix and a coverage.py
// report carries the CI's absolute checkout path, neither of which line up
// with a github.com/<owner>/<repo>/blob/<sha>/<path> URL as-is (issue #18).
func formatFileName(path string, opts Options) string {
	displayPath := paths.NormalizePathForAnnotation(path)
	if opts.RepoURL != "" && opts.SHA != "" {
		return fmt.Sprintf("[`%s`](%s/blob/%s/%s)", displayPath, opts.RepoURL, opts.SHA, encodeURLPath(displayPath))
	}
	return fmt.Sprintf("`%s`", displayPath)
}

func formatFooter() string {
	return "---\n<sub>\U0001F4C8 Generated by [LiteCov](https://github.com/manashmandal/litecov)</sub>\n"
}

func getStatusEmoji(coverage float64) string {
	switch {
	case coverage >= 80:
		return "\u2705"
	case coverage >= 50:
		return "\u26A0\uFE0F"
	default:
		return "\u274C"
	}
}

func formatUncoveredLines(lines []int, repoURL, sha, filePath string) string {
	if len(lines) == 0 {
		return "-"
	}

	sort.Ints(lines)

	var ranges []string
	start := lines[0]
	end := lines[0]

	for i := 1; i < len(lines); i++ {
		if lines[i] == end+1 {
			end = lines[i]
		} else {
			ranges = append(ranges, formatRange(start, end, repoURL, sha, filePath))
			start = lines[i]
			end = lines[i]
		}
	}
	ranges = append(ranges, formatRange(start, end, repoURL, sha, filePath))

	if len(ranges) > 5 {
		return strings.Join(ranges[:5], ", ") + fmt.Sprintf(" +%d more", len(ranges)-5)
	}
	return strings.Join(ranges, ", ")
}

func formatRange(start, end int, repoURL, sha, filePath string) string {
	if repoURL != "" && sha != "" {
		// Same repo-relative normalization as formatFileName, plus percent
		// escaping: a path segment left as-is in the URL breaks the link on
		// a space, '#', or '?' in the filename (issue #18).
		linkPath := encodeURLPath(paths.NormalizePathForAnnotation(filePath))
		if start == end {
			return fmt.Sprintf("[L%d](%s/blob/%s/%s#L%d)", start, repoURL, sha, linkPath, start)
		}
		return fmt.Sprintf("[L%d-%d](%s/blob/%s/%s#L%d-L%d)", start, end, repoURL, sha, linkPath, start, end)
	}
	if start == end {
		return fmt.Sprintf("L%d", start)
	}
	return fmt.Sprintf("L%d-%d", start, end)
}

// encodeURLPath percent-encodes each segment of a repo-relative path so a
// filename containing a space, '#', or '?' can't produce a malformed
// markdown link, while leaving the '/' segment separators intact.
func encodeURLPath(p string) string {
	segments := strings.Split(p, "/")
	for i, seg := range segments {
		segments[i] = url.PathEscape(seg)
	}
	return strings.Join(segments, "/")
}

func filterFiles(files []coverage.FileCoverage, opts Options) []coverage.FileCoverage {
	switch {
	case opts.ShowFiles == "all":
		return files

	case opts.ShowFiles == "changed":
		// No fallback to the full file list when ChangedFiles is empty: by
		// the time Format reaches this point, main.go has already exited if
		// the changed-files fetch itself failed, so an empty list here only
		// ever means the PR touched nothing the coverage report knows
		// about. Substituting every file would silently widen what the
		// report claims to cover (issue #20).
		changedSet := make(map[string]bool)
		for _, f := range opts.ChangedFiles {
			changedSet[f] = true
		}
		var result []coverage.FileCoverage
		for _, f := range files {
			// Use suffix matching to handle different path prefixes
			matched := paths.FindMatchingChangedFile(f.Path, changedSet)
			if matched != "" {
				result = append(result, f)
			}
		}
		return result

	case strings.HasPrefix(opts.ShowFiles, "threshold:"):
		var result []coverage.FileCoverage
		for _, f := range files {
			// A file with no coverable lines has nothing to compare against
			// the threshold: Percentage() falls back to 0 for LinesTotal ==
			// 0, which would otherwise always read as "below threshold"
			// (issue #35).
			if f.LinesTotal == 0 {
				continue
			}
			if f.Percentage() < opts.Threshold {
				result = append(result, f)
			}
		}
		return result

	case strings.HasPrefix(opts.ShowFiles, "worst:"):
		// Files with no coverable lines have nothing to rank: Percentage()
		// falls back to 0 for LinesTotal == 0, which would otherwise always
		// sort them to the top as the worst files in the repo (issue #35).
		var measured []coverage.FileCoverage
		for _, f := range files {
			if f.LinesTotal > 0 {
				measured = append(measured, f)
			}
		}
		sort.Slice(measured, func(i, j int) bool {
			return measured[i].Percentage() < measured[j].Percentage()
		})
		if opts.WorstN > len(measured) {
			return measured
		}
		return measured[:opts.WorstN]

	default:
		return files
	}
}

// LineRange represents a contiguous range of line numbers.
type LineRange struct {
	Start int
	End   int
}

// GroupConsecutiveLines groups consecutive line numbers into ranges.
func GroupConsecutiveLines(lines []int) []LineRange {
	if len(lines) == 0 {
		return nil
	}

	sorted := make([]int, len(lines))
	copy(sorted, lines)
	sort.Ints(sorted)

	var ranges []LineRange
	start := sorted[0]
	end := sorted[0]

	for i := 1; i < len(sorted); i++ {
		if sorted[i] == end+1 {
			end = sorted[i]
		} else {
			ranges = append(ranges, LineRange{Start: start, End: end})
			start = sorted[i]
			end = sorted[i]
		}
	}
	ranges = append(ranges, LineRange{Start: start, End: end})

	return ranges
}
