package comment

import (
	"fmt"
	"math"
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
	// BaseError holds why a configured base coverage file couldn't be turned
	// into a report -- it wouldn't open, its format couldn't be detected, or
	// it failed to parse -- so Format can say so instead of rendering the
	// same head-only layout it would if no base had been requested at all
	// (issue #39). Left empty when no base was configured, and when one was
	// configured and loaded fine, since that case calls FormatWithComparison
	// instead of Format.
	BaseError string
	// PatchCoverage is the coverage of only the lines this PR added,
	// computed by coverage.CalculatePatchCoverage from the PR diff
	// intersected with the report's covered/uncovered line data. Its zero
	// value (Total == 0) means no patch data was available -- not a PR
	// build, the changed-files fetch failed, or the diff touched no
	// coverable line -- and formatPatchString leaves the summary line's
	// Patch segment out entirely in that case instead of claiming a 0%
	// that was never measured (issue #6).
	PatchCoverage coverage.PatchCoverage
	// FilePatches is PatchCoverage's per-file breakdown, computed by
	// coverage.CalculateFilePatchCoverage and keyed by the same Path a
	// FileCoverage in the report carries. formatImpactedFiles renders a
	// file's patch percentage and patch-scoped uncovered lines when its
	// path has an entry here, so the Impacted Files table reports whether
	// the PR's own lines are tested instead of the file's whole-file
	// average (issue #9). A file with no entry falls back to its
	// whole-file numbers, same as when FilePatches is nil entirely.
	FilePatches map[string]coverage.FilePatch
}

func Format(report *coverage.Report, opts Options) string {
	var sb strings.Builder

	sb.WriteString(Marker)
	sb.WriteString("\n")

	sb.WriteString(formatHeader(opts))
	sb.WriteString(formatBaseError(opts))
	sb.WriteString(formatQuickSummary(report, opts))

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
	// Collapsed and last, matching Codecov: the status line and the files
	// table above are what a reviewer needs without clicking anything, so
	// only this supporting diff block goes behind a <details> (issue #21).
	sb.WriteString(formatCoverageDiff(report))

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
	sb.WriteString(formatQuickSummaryWithDelta(comp, opts))
	sb.WriteString(formatImpactedFilesWithDelta(comp.FileChanges, opts))
	// Collapsed and last, same ordering as Format (issue #21).
	sb.WriteString(formatCoverageDiffWithComparison(comp, opts))
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

// formatBaseError renders a note when a base coverage file was configured
// but litecov couldn't turn it into a report. Without this, Format renders
// exactly the layout it would for no base having been requested at all, so
// a broken base-coverage-file input made the comparison silently disappear
// instead of explaining why (issue #39).
func formatBaseError(opts Options) string {
	if opts.BaseError == "" {
		return ""
	}
	return fmt.Sprintf("> ⚠️ **Base coverage unavailable:** a base coverage file was configured but could not be read (%s). Showing head coverage only, no comparison.\n\n",
		opts.BaseError)
}

func formatQuickSummary(report *coverage.Report, opts Options) string {
	emoji := getStatusEmoji(report.Coverage)
	return fmt.Sprintf("> %s **Coverage:** `%.2f%%`%s | **Lines:** `%d/%d` | **Files:** `%d`\n\n",
		emoji, report.Coverage, formatPatchString(opts.PatchCoverage), report.TotalCovered, report.TotalLines, len(report.Files))
}

func formatQuickSummaryWithDelta(comp *coverage.Comparison, opts Options) string {
	emoji := getStatusEmoji(comp.Head.Coverage)
	// A present-but-empty base report leaves nothing to diff against, same
	// as no base report at all: showing CoverageDelta here would claim an
	// improvement over a measurement that was never taken (issue #32).
	delta := formatDeltaString(comp.CoverageDelta, comp.Base != nil && !comp.NoBaseFiles)
	return fmt.Sprintf("> %s **Coverage:** `%.2f%%`%s%s | **Lines:** `%d/%d` | **Files:** `%d`\n\n",
		emoji, comp.Head.Coverage, delta, formatPatchString(opts.PatchCoverage), comp.Head.TotalCovered, comp.Head.TotalLines, len(comp.Head.Files))
}

// formatPatchString renders the summary line's Patch segment: the coverage
// percentage of only the lines this PR added, Codecov's headline number for
// a PR comment (https://docs.codecov.com/docs/coverage-percentages). Empty
// when patch.Total is 0 -- no PR diff was available, or the diff touched no
// coverable line -- so the summary doesn't claim a measurement that was
// never taken, the same reasoning formatDeltaString applies to a missing
// base (issue #6).
func formatPatchString(patch coverage.PatchCoverage) string {
	if patch.Total == 0 {
		return ""
	}
	return fmt.Sprintf(" | **Patch:** `%.2f%%`", patch.Percentage())
}

func formatDeltaString(delta float64, hasBase bool) string {
	if !hasBase {
		return ""
	}
	rounded := roundToDisplayPrecision(delta)
	if rounded == 0 {
		return " (ø)"
	}
	if rounded > 0 {
		return fmt.Sprintf(" (+%.2f%%)", rounded)
	}
	return fmt.Sprintf(" (%.2f%%)", rounded)
}

// roundToDisplayPrecision rounds a percentage-point delta to the two decimal
// places the comment actually renders it at. Comparing the raw delta to 0
// let a change too small to show at that precision, like 0.004, print as a
// signed +0.00% instead of the "no visible change" it actually is; rounding
// before comparing makes the decision match what gets displayed (issue #38).
func roundToDisplayPrecision(delta float64) float64 {
	return math.Round(delta*100) / 100
}

func formatCoverageDiff(report *coverage.Report) string {
	var sb strings.Builder

	sb.WriteString("<details>\n")
	sb.WriteString("<summary>Additional details and impacted files</summary>\n\n")
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
	sb.WriteString("<summary>Additional details and impacted files</summary>\n\n")
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
		// Rounded to display precision before comparing, same as
		// formatDeltaString: a raw delta of e.g. 0.004 is 0.00% once printed,
		// so it gets ø and a blank prefix instead of a misleading +0.00%
		// (issue #38).
		coverageDiff := roundToDisplayPrecision(comp.Head.Coverage - comp.Base.Coverage)
		prefix := " "
		deltaStr := "ø"
		if coverageDiff > 0 {
			prefix = "+"
			deltaStr = fmt.Sprintf("+%.2f%%", coverageDiff)
		} else if coverageDiff < 0 {
			prefix = "-"
			deltaStr = fmt.Sprintf("%.2f%%", coverageDiff)
		}
		sb.WriteString(fmt.Sprintf("%s Coverage     %6.2f%%   %6.2f%%   %s\n",
			prefix, comp.Base.Coverage, comp.Head.Coverage, deltaStr))
	} else {
		sb.WriteString(fmt.Sprintf("  Coverage              %6.2f%%\n", comp.Head.Coverage))
	}

	sb.WriteString("=============================================\n")

	if comp.Base != nil && !comp.NoBaseFiles {
		filesDiff := len(comp.Head.Files) - len(comp.Base.Files)
		sb.WriteString(fmt.Sprintf("  Files           %4d      %4d   %5s\n",
			len(comp.Base.Files), len(comp.Head.Files), formatIntDelta(filesDiff)))

		linesDiff := comp.Head.TotalLines - comp.Base.TotalLines
		sb.WriteString(fmt.Sprintf("  Lines          %5d     %5d   %5s\n",
			comp.Base.TotalLines, comp.Head.TotalLines, formatIntDelta(linesDiff)))
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
		sb.WriteString(fmt.Sprintf("%s Hits          %5d     %5d   %5s\n",
			hitsPrefix, comp.Base.Hits(), comp.Head.Hits(), formatIntDelta(hitsDiff)))

		missesDiff := comp.Head.Misses() - comp.Base.Misses()
		missesPrefix := " "
		if missesDiff < 0 {
			missesPrefix = "+"
		} else if missesDiff > 0 {
			missesPrefix = "-"
		}
		sb.WriteString(fmt.Sprintf("%s Misses        %5d     %5d   %5s\n",
			missesPrefix, comp.Base.Misses(), comp.Head.Misses(), formatIntDelta(missesDiff)))
	} else {
		sb.WriteString(fmt.Sprintf("  Hits                     %5d\n", comp.Head.Hits()))
		sb.WriteString(fmt.Sprintf("  Misses                   %5d\n", comp.Head.Misses()))
	}

	sb.WriteString("```\n\n")
	sb.WriteString("</details>\n\n")

	return sb.String()
}

// formatIntDelta renders an exact integer delta (Files, Lines, Hits, Misses
// counts, which have no display-precision rounding to worry about) with a
// leading sign, except at 0: printing "+0" for a row that didn't change
// reads as a change that happened (issue #38).
func formatIntDelta(n int) string {
	if n == 0 {
		return "0"
	}
	return fmt.Sprintf("%+d", n)
}

// sortImpactedFiles orders files by ascending coverage percentage, worst
// first, with path as the tiebreaker so files tied on percentage still
// render in a stable order instead of whatever order sort.Slice's comparator
// happens to leave them in. Before this, formatImpactedFiles rendered rows
// in whatever order the coverage parser produced them, which carried no
// meaning: the worst file could land anywhere in the table, and files
// findMissingFiles appends always landed at the end regardless of how bad
// they were (issue #56). A copy is sorted, not files itself, since files is
// the caller's report.Files or a filtered slice sharing its backing array,
// the same concern formatUncoveredLines has about UncoveredLines (issue
// #74).
func sortImpactedFiles(files []coverage.FileCoverage) []coverage.FileCoverage {
	sorted := make([]coverage.FileCoverage, len(files))
	copy(sorted, files)
	sort.Slice(sorted, func(i, j int) bool {
		pi, pj := sorted[i].Percentage(), sorted[j].Percentage()
		if pi != pj {
			return pi < pj
		}
		return sorted[i].Path < sorted[j].Path
	})
	return sorted
}

func formatImpactedFiles(files []coverage.FileCoverage, opts Options) string {
	if len(files) == 0 {
		if opts.ShowFiles == "changed" {
			return formatNoChangedFiles()
		}
		return ""
	}

	files = sortImpactedFiles(files)

	var sb strings.Builder

	// Rendered inline, not behind a click: Codecov keeps its per-file table
	// visible and collapses only the supporting Coverage Diff block, which
	// formatCoverageDiff renders separately at the bottom (issue #21).
	sb.WriteString(fmt.Sprintf("**Impacted Files (%d)**\n\n", len(files)))
	sb.WriteString("| File | Coverage | Uncovered Lines | Status |\n")
	sb.WriteString("|------|----------|-----------------|--------|\n")

	for _, f := range files {
		pct := f.Percentage()
		uncovered := f.UncoveredLines
		// A file with patch data reports the coverage of only the lines
		// this PR added to it, not the whole file's average: a large,
		// well-covered file that gained a few untested lines must not
		// render as a passing percentage with a green check just because
		// its pre-existing lines are tested (issue #9). Falls back to the
		// whole-file numbers when this file has no patch data -- not a PR
		// build, the file wasn't part of the diff, or none of its added
		// lines were ever instrumented.
		if patch, ok := opts.FilePatches[f.Path]; ok {
			pct = patch.Coverage.Percentage()
			uncovered = patch.UncoveredLines
		}
		emoji := getStatusEmoji(pct)
		fileName := formatFileName(f.Path, opts)
		coverageStr := fmt.Sprintf("`%.2f%%`", pct)
		uncoveredStr := formatUncoveredLines(uncovered, opts.RepoURL, opts.SHA, f.Path)
		// Mark files with no coverage data
		if f.LinesTotal == 0 {
			coverageStr = "`⚠️ no tests`"
			uncoveredStr = "-"
			emoji = "❌"
		}
		sb.WriteString(fmt.Sprintf("| %s | %s | %s | %s |\n", fileName, coverageStr, uncoveredStr, emoji))
	}

	sb.WriteString("\n")

	return sb.String()
}

// formatNoChangedFiles renders the Impacted Files section for show-files:
// changed when the PR's changed files were determined successfully but none
// of them matched the coverage report. Rendering an explanatory section
// beats both a silently blank one and, per issue #20, silently substituting
// every file in the report.
func formatNoChangedFiles() string {
	var sb strings.Builder

	sb.WriteString("**Impacted Files (0)**\n\n")
	sb.WriteString("No changed files matched the coverage report.\n\n")

	return sb.String()
}

// sortFileChangesByDelta orders fileChanges by ascending Delta, worst first,
// with path as the tiebreaker: the comparison-path counterpart of
// sortImpactedFiles, ranking by coverage change instead of a single
// percentage. Codecov's own comparison table ranks the same way, most
// negative first, so the files bleeding coverage sit at the top instead of
// wherever the parser happened to emit them (issue #56). A copy is sorted,
// not fileChanges itself, for the same reason sortImpactedFiles copies
// (issue #74).
func sortFileChangesByDelta(fileChanges []coverage.FileChange) []coverage.FileChange {
	sorted := make([]coverage.FileChange, len(fileChanges))
	copy(sorted, fileChanges)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].Delta != sorted[j].Delta {
			return sorted[i].Delta < sorted[j].Delta
		}
		return sorted[i].Path < sorted[j].Path
	})
	return sorted
}

func formatImpactedFilesWithDelta(fileChanges []coverage.FileChange, opts Options) string {
	if len(fileChanges) == 0 {
		return ""
	}

	fileChanges = sortFileChangesByDelta(fileChanges)

	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("**Impacted Files (%d)**\n\n", len(fileChanges)))
	// Uncovered Lines sits next to Δ instead of being replaced by it: before
	// this, the no-base table's Uncovered Lines and this table's Δ were
	// mutually exclusive, so the only actionable column in the report
	// disappeared in whichever mode a PR run actually used (issue #44).
	sb.WriteString("| File | Coverage | \u0394 | Uncovered Lines | Status |\n")
	sb.WriteString("|------|----------|---|-----------------|--------|\n")

	for _, fc := range fileChanges {
		fileName := formatFileName(fc.Path, opts)
		deltaStr := formatFileDelta(fc)
		coverageStr := fmt.Sprintf("`%.2f%%`", fc.HeadCoverage)
		emoji := getStatusEmoji(fc.HeadCoverage)
		// Same patch-scoped fallback as formatImpactedFiles: a file with
		// patch data reports only the PR-added lines that are still
		// uncovered, not every uncovered line the whole file has ever had
		// (issue #9).
		uncovered := fc.UncoveredLines
		if patch, ok := opts.FilePatches[fc.Path]; ok {
			uncovered = patch.UncoveredLines
		}
		uncoveredStr := formatUncoveredLines(uncovered, opts.RepoURL, opts.SHA, fc.Path)
		// A file with no coverable lines has nothing to measure, not 0%
		// coverage: HeadCoverage falls back to 0 for LinesTotal == 0, which
		// would otherwise render identically to a file whose statements
		// were never hit (issue #35).
		if fc.NoStatements {
			coverageStr = "`no statements`"
			emoji = "➖"
			uncoveredStr = "-"
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
			uncoveredStr = "-"
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
			uncoveredStr = "-"
		}
		sb.WriteString(fmt.Sprintf("| %s | %s | %s | %s | %s |\n", fileName, coverageStr, deltaStr, uncoveredStr, emoji))
	}

	sb.WriteString("\n")

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
	// Rounded to display precision before comparing, same as
	// formatDeltaString: a raw delta of e.g. 0.004 is 0.00% once printed, so
	// it gets ø instead of a misleading +0.00% (issue #38).
	rounded := roundToDisplayPrecision(fc.Delta)
	if rounded == 0 {
		return "`ø`"
	}
	if rounded > 0 {
		return fmt.Sprintf("`+%.2f%%`", rounded)
	}
	return fmt.Sprintf("`%.2f%%`", rounded)
}

// formatFileName renders a coverage report path as an inline code span,
// linked to the file's blob at opts.SHA when a repo URL is configured. The
// path is normalized to repo-relative first (see NormalizePathForAnnotation)
// since a Go coverage profile carries the module prefix and a coverage.py
// report carries the CI's absolute checkout path, neither of which line up
// with a github.com/<owner>/<repo>/blob/<sha>/<path> URL as-is (issue #18).
// The displayed path is escaped for the table cell it lands in: GitHub
// splits a row into cells on any unescaped '|' before it parses inline
// markup, so a pipe in the path still breaks the row even from inside a
// code span, and a backtick in the path needs a wider fence than the
// default single backtick or it closes the span early and dumps the rest
// of the link as literal text (issue #27).
func formatFileName(path string, opts Options) string {
	displayPath := paths.NormalizePathForAnnotation(path)
	span := codeSpan(escapeTablePipes(displayPath))
	if opts.RepoURL != "" && opts.SHA != "" {
		return fmt.Sprintf("[%s](%s/blob/%s/%s)", span, opts.RepoURL, opts.SHA, encodeURLPath(displayPath))
	}
	return span
}

// escapeTablePipes backslash-escapes '|' so a path can sit inside a
// markdown table cell without ending it early. GitHub finds cell
// boundaries by scanning a row's raw text for unescaped pipes before it
// parses any inline markup, so this has to happen even for a pipe that
// will end up inside a code span (issue #27).
func escapeTablePipes(s string) string {
	return strings.ReplaceAll(s, "|", "\\|")
}

// codeSpan wraps content in the narrowest backtick fence that can hold it
// without the fence closing early on a backtick already in content: one
// more backtick than content's longest backtick run, per CommonMark's code
// span rule. A single space is added on each side when content starts or
// ends with a backtick, so that backtick doesn't fuse with the fence into
// one longer run -- CommonMark trims that padding back off when rendering
// (issue #27).
func codeSpan(content string) string {
	longestRun, run := 0, 0
	for _, r := range content {
		if r == '`' {
			run++
			if run > longestRun {
				longestRun = run
			}
		} else {
			run = 0
		}
	}
	fence := strings.Repeat("`", longestRun+1)
	if strings.HasPrefix(content, "`") || strings.HasSuffix(content, "`") {
		content = " " + content + " "
	}
	return fence + content + fence
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

	// Copy before sorting: lines is the caller's FileCoverage.UncoveredLines
	// slice, and formatting it for display must not reorder the report the
	// caller still holds (issue #74).
	sorted := make([]int, len(lines))
	copy(sorted, lines)
	sort.Ints(sorted)
	lines = sorted

	// rangeSizes tracks the line count behind each entry in ranges, in the
	// same order, so the truncation count below can be expressed in lines
	// rather than ranges (issue #23).
	var ranges []string
	var rangeSizes []int
	start := lines[0]
	end := lines[0]

	for i := 1; i < len(lines); i++ {
		if lines[i] == end+1 {
			end = lines[i]
		} else {
			ranges = append(ranges, formatRange(start, end, repoURL, sha, filePath))
			rangeSizes = append(rangeSizes, end-start+1)
			start = lines[i]
			end = lines[i]
		}
	}
	ranges = append(ranges, formatRange(start, end, repoURL, sha, filePath))
	rangeSizes = append(rangeSizes, end-start+1)

	if len(ranges) > 5 {
		shown := 0
		for _, n := range rangeSizes[:5] {
			shown += n
		}
		hidden := len(lines) - shown
		return strings.Join(ranges[:5], ", ") + fmt.Sprintf(" +%d more lines", hidden)
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
