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

// MarkerFor returns the HTML comment marker litecov writes at the top of a
// PR comment and later searches for to find that comment again: the bare
// Marker when flag is empty, or a flag-scoped variant, "<!-- litecov:<flag>
// -->", otherwise. Before this, every invocation wrote the same fixed
// Marker, so a second litecov step in one workflow -- the normal setup for
// a monorepo posting separate backend and frontend reports -- found the
// first step's comment and PATCHed over it instead of posting its own
// (issue #54).
func MarkerFor(flag string) string {
	if flag == "" {
		return Marker
	}
	return fmt.Sprintf("<!-- litecov:%s -->", flag)
}

type Options struct {
	Title string
	// Flag identifies this invocation when litecov runs more than once
	// against the same PR, e.g. one step per service in a monorepo. Empty by
	// default, which keeps the plain Marker; set, it scopes the comment
	// marker MarkerFor renders to "<!-- litecov:<Flag> -->" so multiple runs
	// against one commit each get their own comment instead of overwriting
	// each other's (issue #54).
	Flag         string
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
	// coverable line -- and formatPatchStatusLine leaves the Patch status
	// line out of the headline entirely in that case instead of claiming a
	// 0% that was never measured (issue #6).
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
	// GoodThreshold and WarnThreshold are the coverage percentages
	// getStatusEmoji checks a percentage against: at or above GoodThreshold
	// gets ✅, at or above WarnThreshold gets ⚠️, anything lower gets ❌.
	// Unset (<=0) falls back to the fixed 80/50 getStatusEmoji used before
	// these fields existed, the same "not configured" convention main.go's
	// threshold and patch-threshold flags already use. Distinct from
	// Threshold above: that one only filters which files show-files:
	// threshold:N includes and never touches which glyph a row gets, so a
	// repo could set threshold: 95 for its pass/fail gate and still see
	// every file at 80% marked ✅ (issue #80).
	GoodThreshold float64
	WarnThreshold float64
}

func Format(report *coverage.Report, opts Options) string {
	var sb strings.Builder

	sb.WriteString(MarkerFor(opts.Flag))
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

	sb.WriteString(formatFooter(opts))

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

	sb.WriteString(MarkerFor(opts.Flag))
	sb.WriteString("\n")

	sb.WriteString(formatHeader(opts))
	sb.WriteString(formatBaseMissingWarning(comp))
	sb.WriteString(formatQuickSummaryWithDelta(comp, opts))
	sb.WriteString(formatImpactedFilesWithDelta(comp.FileChanges, opts))
	// Collapsed and last, same ordering as Format (issue #21).
	sb.WriteString(formatCoverageDiffWithComparison(comp, opts))
	sb.WriteString(formatFooter(opts))

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

// formatBaseMissingWarning renders a note when a base comparison was
// configured and the base coverage file parsed without error but produced
// zero files to compare against (coverage.Comparison.NoBaseFiles: a
// truncated LCOV, a Cobertura with no packages, issue #32). Without this,
// FormatWithComparison silently fell back to head-only numbers with nothing
// in the comment telling the reader why no delta appeared -- the same gap
// formatBaseError closes for a base file that couldn't be read at all
// (issue #39), just for the read-fine-but-empty case instead (issue #77).
func formatBaseMissingWarning(comp *coverage.Comparison) string {
	if !comp.NoBaseFiles {
		return ""
	}
	return "> ⚠️ **Base report missing:** the configured base coverage has no files to compare against. Showing head coverage only, no comparison.\n\n"
}

// formatQuickSummary renders the status headline for a report with no base
// comparison: a Patch status line when patch data is available, then a
// Project status line, matching Codecov's own layout of one plain-sentence
// check per line instead of every number crammed into a single blockquote
// (issue #77).
func formatQuickSummary(report *coverage.Report, opts Options) string {
	var sb strings.Builder

	sb.WriteString(formatPatchStatusLine(opts.PatchCoverage))

	emoji := getStatusEmoji(report.Coverage, opts)
	sb.WriteString(fmt.Sprintf("> %s Project coverage is `%.2f%%`.%s\n",
		emoji, report.Coverage, formatComparingClause(false, opts)))
	sb.WriteString(fmt.Sprintf("> **Lines:** `%d/%d` | **Files:** `%d`\n\n",
		report.TotalCovered, report.TotalLines, len(report.Files)))

	return sb.String()
}

// formatQuickSummaryWithDelta is formatQuickSummary's comparison-path
// counterpart: the same Patch and Project status lines, with the Project
// line also carrying the coverage delta and, when opts.SHA is known, which
// base and head are actually being compared (issue #77).
func formatQuickSummaryWithDelta(comp *coverage.Comparison, opts Options) string {
	var sb strings.Builder

	sb.WriteString(formatPatchStatusLine(opts.PatchCoverage))

	emoji := getStatusEmoji(comp.Head.Coverage, opts)
	// A present-but-empty base report leaves nothing to diff against, same
	// as no base report at all: showing CoverageDelta here would claim an
	// improvement over a measurement that was never taken (issue #32).
	hasBase := comp.Base != nil && !comp.NoBaseFiles
	delta := formatDeltaString(comp.CoverageDelta, hasBase)
	sb.WriteString(fmt.Sprintf("> %s Project coverage is `%.2f%%`%s.%s\n",
		emoji, comp.Head.Coverage, delta, formatComparingClause(hasBase, opts)))
	sb.WriteString(fmt.Sprintf("> **Lines:** `%d/%d` | **Files:** `%d`\n\n",
		comp.Head.TotalCovered, comp.Head.TotalLines, len(comp.Head.Files)))

	return sb.String()
}

// formatPatchStatusLine renders the status headline's Patch line: Codecov's
// headline check for a PR comment
// (https://docs.codecov.com/docs/coverage-percentages), its own status line
// with a pass/fail glyph instead of a "| **Patch:** X%" segment folded into
// the Coverage line (issue #77). All-covered gets Codecov's own success
// wording verbatim; anything short of that names how many lines are
// missing, the number a reviewer needs to go find them. Empty when
// patch.Total is 0 -- no PR diff was available, or the diff touched no
// coverable line -- so the summary doesn't claim a measurement that was
// never taken, the same reasoning formatDeltaString applies to a missing
// base (issue #6).
func formatPatchStatusLine(patch coverage.PatchCoverage) string {
	if patch.Total == 0 {
		return ""
	}
	if patch.Covered == patch.Total {
		return "> ✅ All modified and coverable lines are covered by tests.\n"
	}
	missing := patch.Total - patch.Covered
	unit := "lines"
	if missing == 1 {
		unit = "line"
	}
	return fmt.Sprintf("> ❌ Patch coverage is `%.2f%%` with `%d %s` in your changes missing coverage. Please review.\n",
		patch.Percentage(), missing, unit)
}

// formatComparingClause renders the Project status line's commit reference:
// which head commit the report measures and, once a real base comparison is
// active, what it's being compared against (issue #77). Empty when
// opts.SHA is unset -- a local run or a test comment has no commit to point
// at, and "head ()" would link to nothing.
//
// The base side names a branch, not a commit: LiteCov has no mechanism to
// learn which commit a configured base-coverage-file was actually generated
// from (that gap belongs to issue #24, obtaining a base report at all), so
// claiming a specific base commit here would assert a fact nothing verifies.
func formatComparingClause(hasBase bool, opts Options) string {
	if opts.SHA == "" {
		return ""
	}
	head := formatCommitLink(opts.SHA, opts.RepoURL)
	if !hasBase {
		return fmt.Sprintf(" Head commit: %s.", head)
	}
	baseBranch := opts.BaseBranch
	if baseBranch == "" {
		// Same "BASE" fallback formatCoverageDiffWithComparison uses when
		// nothing supplied a real branch name (issue #75).
		baseBranch = "BASE"
	}
	return fmt.Sprintf(" Comparing base (%s) to head (%s).", formatBranchLink(baseBranch, opts.RepoURL), head)
}

// formatCommitLink renders a short, linked reference to sha, the same
// short-sha-in-backticks convention Codecov's own comparison line uses
// (https://docs.codecov.com/docs/comparing-commits). Linked to the commit's
// page on GitHub when a repo URL is configured, unlinked otherwise, the same
// fallback formatFileName and formatRange use for their own links.
func formatCommitLink(sha, repoURL string) string {
	short := sha
	if len(short) > 7 {
		short = short[:7]
	}
	if repoURL == "" {
		return fmt.Sprintf("`%s`", short)
	}
	return fmt.Sprintf("[`%s`](%s/commit/%s)", short, repoURL, sha)
}

// formatBranchLink is formatCommitLink's counterpart for a branch name,
// linked to the branch's tree on GitHub when a repo URL is configured.
func formatBranchLink(branch, repoURL string) string {
	if repoURL == "" {
		return fmt.Sprintf("`%s`", branch)
	}
	return fmt.Sprintf("[`%s`](%s/tree/%s)", branch, repoURL, url.PathEscape(branch))
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

// diffLabelWidth is the width of a Coverage Diff/Summary row's label
// column: a 1-character +/- prefix, a space, then the widest metric name
// left-justified. "Coverage", "Branches" and "Partials" are tied at 8
// characters, so anchoring this to "Coverage" still covers the label set
// now that Branches/Partials can render too (issue #78). The value columns
// that follow can't be a constant like this one -- they have to size
// themselves to whatever numbers a given report produced -- but the label
// set is fixed, so its width is (issue #45).
const diffLabelWidth = len("Coverage") + 2

// diffColumnWidth returns the width to give every numeric column in a
// Coverage Diff/Summary block: the longest value string that will actually
// render, plus a fixed gutter. Codecov sizes every column in the block off
// the same number -- base, head, delta, whichever metric -- instead of
// tuning each row by hand, which is what let Files, Lines, Hits and Misses
// end up on four different grids before this (issue #45). An empty string,
// which formatIntDelta returns for an unmoved row, doesn't affect the
// width.
func diffColumnWidth(values ...string) int {
	const gutter = 3 // matches the 3-space gutter in Codecov's own comment
	width := 0
	for _, v := range values {
		width = max(width, len(v))
	}
	return width + gutter
}

// diffRow3 renders one three-column Coverage Diff row: a +/- prefix, the
// label, then base, head and delta each right-justified to colWidth. Every
// row in a block is built through this with the same colWidth, so they
// land on one shared grid instead of each picking its own (issue #45).
func diffRow3(prefix, label string, colWidth int, base, head, delta string) string {
	return fmt.Sprintf("%s %-*s%*s%*s%*s\n", prefix, diffLabelWidth-2, label, colWidth, base, colWidth, head, colWidth, delta)
}

// diffRow1 is diffRow3's single-column counterpart, for a report with
// nothing to compare against.
func diffRow1(label string, colWidth int, value string) string {
	return fmt.Sprintf("  %-*s%*s\n", diffLabelWidth-2, label, colWidth, value)
}

// diffTitleLine centers title inside a "@@ ... @@" banner totalWidth
// characters wide, so it lines up with the ==== separator below it and the
// row grid below that instead of the three disagreeing on how wide the
// block is (issue #45). Callers keep totalWidth at least len(title)+6 so
// there's always at least one space of padding on each side.
func diffTitleLine(title string, totalWidth int) string {
	pad := totalWidth - 4 - len(title) // 4 chars for the two "@@" markers
	if pad < 0 {
		pad = 0
	}
	left := pad / 2
	return fmt.Sprintf("@@%s%s%s@@\n", strings.Repeat(" ", left), title, strings.Repeat(" ", pad-left))
}

func formatCoverageDiff(report *coverage.Report) string {
	var sb strings.Builder

	sb.WriteString("<details>\n")
	sb.WriteString("<summary>Additional details and impacted files</summary>\n\n")
	sb.WriteString("```diff\n")

	coverageStr := fmt.Sprintf("%.2f%%", report.Coverage)
	linesStr := fmt.Sprintf("%d/%d", report.TotalCovered, report.TotalLines)
	filesStr := fmt.Sprintf("%d", len(report.Files))

	// One column width for the whole block, sized to the widest of the
	// three values instead of the hand-picked padding each row used to
	// carry on its own -- the same misalignment formatCoverageDiffWithComparison
	// had (issue #45).
	colWidth := diffColumnWidth(coverageStr, linesStr, filesStr)
	totalWidth := max(diffLabelWidth+colWidth, len("Coverage Summary")+6)

	sb.WriteString(diffTitleLine("Coverage Summary", totalWidth))
	sb.WriteString(strings.Repeat("=", totalWidth) + "\n")
	sb.WriteString(diffRow1("Coverage", colWidth, coverageStr))
	sb.WriteString(diffRow1("Lines", colWidth, linesStr))
	sb.WriteString(diffRow1("Files", colWidth, filesStr))
	sb.WriteString(strings.Repeat("=", totalWidth) + "\n")
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
		// Every source that could have supplied a real branch name --
		// base-branch and GITHUB_BASE_REF alike -- came back empty, so this
		// says so instead of inventing "main" for a repo whose default
		// branch is something else (issue #75).
		baseBranch = "BASE"
	}
	prRef := fmt.Sprintf("#%d", opts.PRNumber)
	if opts.PRNumber == 0 {
		prRef = "HEAD"
	}

	// A present-but-empty base report (NoBaseFiles) is treated the same as
	// no base report at all: falling into the single-column branch below
	// avoids presenting comp.Base.Coverage's 0 fallback as a real
	// measurement (issue #32).
	hasBase := comp.Base != nil && !comp.NoBaseFiles

	headCoverage := fmt.Sprintf("%.2f%%", comp.Head.Coverage)
	headFiles := fmt.Sprintf("%d", len(comp.Head.Files))
	headLines := fmt.Sprintf("%d", comp.Head.TotalLines)
	headHits := fmt.Sprintf("%d", comp.Head.Hits())
	headMisses := fmt.Sprintf("%d", comp.Head.Misses())

	// hasBranchData decides whether the Branches and Partials rows render
	// at all. Codecov omits both when the underlying language or report
	// format carries no branch data instead of printing a row of zeroes,
	// and neither the LCOV nor the Cobertura parser sets Report.Branches
	// today (see its doc comment), so every report currently produced
	// takes this path (issue #78).
	hasBranchData := comp.Head.Branches != 0 || (hasBase && comp.Base.Branches != 0)
	var headBranches, headPartials string
	if hasBranchData {
		headBranches = fmt.Sprintf("%d", comp.Head.Branches)
		headPartials = fmt.Sprintf("%d", comp.Head.Partials)
	}

	var baseCoverage, baseFiles, baseLines, baseHits, baseMisses string
	var baseBranches, basePartials string
	var coverageDelta, filesDelta, linesDelta, hitsDelta, missesDelta string
	var branchesDelta, partialsDelta string
	coveragePrefix, hitsPrefix, missesPrefix := " ", " ", " "
	partialsPrefix := " "

	if hasBase {
		baseCoverage = fmt.Sprintf("%.2f%%", comp.Base.Coverage)
		baseFiles = fmt.Sprintf("%d", len(comp.Base.Files))
		baseLines = fmt.Sprintf("%d", comp.Base.TotalLines)
		baseHits = fmt.Sprintf("%d", comp.Base.Hits())
		baseMisses = fmt.Sprintf("%d", comp.Base.Misses())

		// Rounded to display precision before comparing, same as
		// formatDeltaString: a raw delta of e.g. 0.004 is 0.00% once printed,
		// so it gets ø and a blank prefix instead of a misleading +0.00%
		// (issue #38).
		coverageDiff := roundToDisplayPrecision(comp.Head.Coverage - comp.Base.Coverage)
		coverageDelta = "ø"
		if coverageDiff > 0 {
			coveragePrefix = "+"
			coverageDelta = fmt.Sprintf("+%.2f%%", coverageDiff)
		} else if coverageDiff < 0 {
			coveragePrefix = "-"
			coverageDelta = fmt.Sprintf("%.2f%%", coverageDiff)
		}

		filesDelta = formatIntDelta(len(comp.Head.Files) - len(comp.Base.Files))
		linesDelta = formatIntDelta(comp.Head.TotalLines - comp.Base.TotalLines)

		if hasBranchData {
			baseBranches = fmt.Sprintf("%d", comp.Base.Branches)
			basePartials = fmt.Sprintf("%d", comp.Base.Partials)
			branchesDelta = formatIntDelta(comp.Head.Branches - comp.Base.Branches)

			// Fewer partials is an improvement, the same convention Misses
			// uses below: a falling count gets the "+" (good) prefix, a
			// rising one gets "-" (bad).
			partialsDiff := comp.Head.Partials - comp.Base.Partials
			if partialsDiff < 0 {
				partialsPrefix = "+"
			} else if partialsDiff > 0 {
				partialsPrefix = "-"
			}
			partialsDelta = formatIntDelta(partialsDiff)
		}

		hitsDiff := comp.Head.Hits() - comp.Base.Hits()
		if hitsDiff > 0 {
			hitsPrefix = "+"
		} else if hitsDiff < 0 {
			hitsPrefix = "-"
		}
		hitsDelta = formatIntDelta(hitsDiff)

		missesDiff := comp.Head.Misses() - comp.Base.Misses()
		if missesDiff < 0 {
			missesPrefix = "+"
		} else if missesDiff > 0 {
			missesPrefix = "-"
		}
		missesDelta = formatIntDelta(missesDiff)
	}

	// One column width for every numeric cell in the block, base, head and
	// delta alike, sized to the widest one that will actually render. A
	// six-digit Lines count or a 100.00% Coverage figure now widens every
	// column together instead of leaving the rest on their old, narrower
	// grid (issue #45).
	colWidth := diffColumnWidth(
		headCoverage, headFiles, headLines, headHits, headMisses, headBranches, headPartials,
		baseCoverage, baseFiles, baseLines, baseHits, baseMisses, baseBranches, basePartials,
		coverageDelta, filesDelta, linesDelta, hitsDelta, missesDelta, branchesDelta, partialsDelta,
	)
	valueCols := 1
	if hasBase {
		valueCols = 3
	}
	rowWidth := diffLabelWidth + colWidth*valueCols

	// The ## line's own content can need more room than the value grid
	// does -- a base branch name has no length limit -- and used to
	// overflow the hard-coded 45-column @@/==== rule around it instead of
	// the rule growing to fit (issue #45).
	hashLine := fmt.Sprintf("##           %8s   %8s     +/-   ", baseBranch, prRef)
	totalWidth := max(rowWidth, len(hashLine)+2, len("Coverage Diff")+6)

	sb.WriteString(diffTitleLine("Coverage Diff", totalWidth))
	sb.WriteString(fmt.Sprintf("%-*s##\n", totalWidth-2, hashLine))
	sb.WriteString(strings.Repeat("=", totalWidth) + "\n")

	if hasBase {
		sb.WriteString(diffRow3(coveragePrefix, "Coverage", colWidth, baseCoverage, headCoverage, coverageDelta))
	} else {
		sb.WriteString(diffRow1("Coverage", colWidth, headCoverage))
	}
	sb.WriteString(strings.Repeat("=", totalWidth) + "\n")

	if hasBase {
		sb.WriteString(diffRow3(" ", "Files", colWidth, baseFiles, headFiles, filesDelta))
		sb.WriteString(diffRow3(" ", "Lines", colWidth, baseLines, headLines, linesDelta))
		if hasBranchData {
			sb.WriteString(diffRow3(" ", "Branches", colWidth, baseBranches, headBranches, branchesDelta))
		}
	} else {
		sb.WriteString(diffRow1("Files", colWidth, headFiles))
		sb.WriteString(diffRow1("Lines", colWidth, headLines))
		if hasBranchData {
			sb.WriteString(diffRow1("Branches", colWidth, headBranches))
		}
	}
	sb.WriteString(strings.Repeat("=", totalWidth) + "\n")

	if hasBase {
		sb.WriteString(diffRow3(hitsPrefix, "Hits", colWidth, baseHits, headHits, hitsDelta))
		sb.WriteString(diffRow3(missesPrefix, "Misses", colWidth, baseMisses, headMisses, missesDelta))
		if hasBranchData {
			sb.WriteString(diffRow3(partialsPrefix, "Partials", colWidth, basePartials, headPartials, partialsDelta))
		}
	} else {
		sb.WriteString(diffRow1("Hits", colWidth, headHits))
		sb.WriteString(diffRow1("Misses", colWidth, headMisses))
		if hasBranchData {
			sb.WriteString(diffRow1("Partials", colWidth, headPartials))
		}
	}

	sb.WriteString("```\n\n")
	sb.WriteString("</details>\n\n")

	return sb.String()
}

// formatIntDelta renders an exact integer delta (Files, Lines, Hits, Misses
// counts, which have no display-precision rounding to worry about) with a
// leading sign, except at 0: it returns "" so the caller's %5s verb pads
// the cell with spaces instead of printing a placeholder. Codecov's own
// diff block leaves an unmoved row's cell blank rather than a "0" that
// reads as a measurement (issue #55); a bare digit was itself an
// improvement over the "+0" this replaced, which read as a change that
// happened (issue #38).
func formatIntDelta(n int) string {
	if n == 0 {
		return ""
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

// githubCommentBodyLimit is GitHub's hard cap on an issue/PR comment body:
// posting anything longer gets HTTP 422 "body is too long (maximum is 65536
// characters)" back from the API, which internal/github.Client turns into an
// error that main.go treats as fatal -- the whole action fails and no
// comment gets posted at all (issue #22).
const githubCommentBodyLimit = 65536

// impactedFilesByteBudget bounds how much of githubCommentBodyLimit the
// Impacted Files table -- one row per file, up to five hyperlinks each,
// roughly 900 bytes a row once RepoURL and SHA are both set -- is allowed to
// spend building formatImpactedFiles/formatImpactedFilesWithDelta's own
// strings.Builder. Everything else the comment renders (header, summary
// lines, the Coverage Diff block, the footer) stays a small, fixed size no
// matter how many files the report covers, so reserving a flat margin here
// for that fixed part is enough to keep the whole assembled body under
// githubCommentBodyLimit without either function needing to know how much
// of the budget Format/FormatWithComparison's other sections will spend
// (issue #22).
const impactedFilesByteBudget = githubCommentBodyLimit - 8192

// formatMoreFilesRow renders the Impacted Files table's overflow row: a
// single cell saying how many files the byte-budget cap left out, with the
// rest of the row's cells left empty so the table still has the same column
// count as every row above it. Codecov's own comment does the same thing
// once a report has more impacted files than it renders inline, e.g.
// spulec/moto#4200's "... and 1 more" (issue #22).
func formatMoreFilesRow(hidden, columns int) string {
	unit := "files"
	if hidden == 1 {
		unit = "file"
	}
	cells := make([]string, columns)
	cells[0] = fmt.Sprintf("_... and %d more %s not shown (comment size limit)_", hidden, unit)
	return "| " + strings.Join(cells, " | ") + " |\n"
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

	for i, f := range files {
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
		emoji := getStatusEmoji(pct, opts)
		fileName := formatFileName(f.Path, opts)
		coverageStr := fmt.Sprintf("`%.2f%%`", pct)
		uncoveredStr := formatUncoveredLines(uncovered, opts.RepoURL, opts.SHA, f.Path)
		// Mark files with no coverage data
		if f.LinesTotal == 0 {
			coverageStr = "`⚠️ no tests`"
			uncoveredStr = "-"
			emoji = "❌"
		}
		row := fmt.Sprintf("| %s | %s | %s | %s |\n", fileName, coverageStr, uncoveredStr, emoji)
		// Stop before a row would push the table past its byte budget,
		// rather than finding out from GitHub's 422 that the whole comment
		// was too big to post (issue #22).
		if sb.Len()+len(row) > impactedFilesByteBudget {
			sb.WriteString(formatMoreFilesRow(len(files)-i, 4))
			break
		}
		sb.WriteString(row)
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

	for i, fc := range fileChanges {
		fileName := formatFileName(fc.Path, opts)
		deltaStr := formatFileDelta(fc)
		coverageStr := fmt.Sprintf("`%.2f%%`", fc.HeadCoverage)
		emoji := getStatusEmoji(fc.HeadCoverage, opts)
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
		row := fmt.Sprintf("| %s | %s | %s | %s | %s |\n", fileName, coverageStr, deltaStr, uncoveredStr, emoji)
		// Same byte-budget cap as formatImpactedFiles, applied to the
		// comparison table's rows (issue #22).
		if sb.Len()+len(row) > impactedFilesByteBudget {
			sb.WriteString(formatMoreFilesRow(len(fileChanges)-i, 5))
			break
		}
		sb.WriteString(row)
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

// formatFooter renders the comment's closing section. Codecov's own footer
// carries a legend defining every glyph used above it
// (https://docs.codecov.com/docs/codecov-delta): what the cell format means,
// what each symbol stands for, which commits were compared. LiteCov's footer
// carried none of that -- a reader hitting ø in a column headed Δ, or ⚠️
// against a file at 79%, had nothing in the comment saying what either
// meant (issue #79). This adds the glyph legend and the ✅/⚠️/❌ cutoffs, in
// the same blockquote style as formatBaseError and formatBaseMissingWarning.
func formatFooter(opts Options) string {
	good, warn := effectiveThresholds(opts)
	legend := fmt.Sprintf("> **Legend** `ø = not affected`, `new = file not in the base report`, `- = no uncovered lines`\n"+
		"> ✅ ≥ %.0f%%   ⚠️ ≥ %.0f%%   ❌ < %.0f%%\n",
		good, warn, warn)
	return "---\n" + legend + "<sub>\U0001F4C8 Generated by [LiteCov](https://github.com/manashmandal/litecov)</sub>\n"
}

// highCoverageThreshold and mediumCoverageThreshold are the default
// pass/warn cutoffs getStatusEmoji and formatFooter's legend fall back to
// through effectiveThresholds when Options doesn't configure its own
// (issue #79, made configurable by issue #80).
const (
	highCoverageThreshold   float64 = 80
	mediumCoverageThreshold float64 = 50
)

// effectiveThresholds resolves the pass/warn cutoffs getStatusEmoji and
// formatFooter's legend both apply: opts.GoodThreshold and
// opts.WarnThreshold when configured (>0), highCoverageThreshold and
// mediumCoverageThreshold's 80/50 defaults otherwise. Sharing one resolver
// between the two keeps the legend from stating a cutoff the emoji don't
// actually use -- the same drift issue #79 already guarded against for the
// old hard-coded pair, now that the pair can move (issue #80).
func effectiveThresholds(opts Options) (good, warn float64) {
	good, warn = highCoverageThreshold, mediumCoverageThreshold
	if opts.GoodThreshold > 0 {
		good = opts.GoodThreshold
	}
	if opts.WarnThreshold > 0 {
		warn = opts.WarnThreshold
	}
	return good, warn
}

func getStatusEmoji(coverage float64, opts Options) string {
	good, warn := effectiveThresholds(opts)
	switch {
	case coverage >= good:
		return "\u2705"
	case coverage >= warn:
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
		// A zero or negative WorstN reaching this slice bound used to panic
		// instead of degrading to "show everything measured" the way an
		// out-of-range positive N already does (issue #84).
		if opts.WorstN <= 0 || opts.WorstN > len(measured) {
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
