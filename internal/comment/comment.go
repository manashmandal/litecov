package comment

import (
	"fmt"
	"sort"
	"strings"

	"github.com/manashmandal/litecov/internal/coverage"
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
	sb.WriteString(formatImpactedFiles(filesToShow, opts))

	sb.WriteString(formatFooter())

	return sb.String()
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
	delta := formatDeltaString(comp.CoverageDelta, comp.Base != nil)
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

	if comp.Base != nil {
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

	if comp.Base != nil {
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

	if comp.Base != nil {
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
		return ""
	}

	var sb strings.Builder

	sb.WriteString("<details>\n")
	sb.WriteString(fmt.Sprintf("<summary>Impacted Files (%d)</summary>\n\n", len(files)))
	sb.WriteString("| File | Coverage | Status |\n")
	sb.WriteString("|------|----------|--------|\n")

	for _, f := range files {
		pct := f.Percentage()
		emoji := getStatusEmoji(pct)
		fileName := formatFileName(f.Path, opts)
		sb.WriteString(fmt.Sprintf("| %s | `%.2f%%` | %s |\n", fileName, pct, emoji))
	}

	sb.WriteString("\n</details>\n\n")

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
		emoji := getStatusEmoji(fc.HeadCoverage)
		fileName := formatFileName(fc.Path, opts)
		deltaStr := formatFileDelta(fc)
		sb.WriteString(fmt.Sprintf("| %s | `%.2f%%` | %s | %s |\n", fileName, fc.HeadCoverage, deltaStr, emoji))
	}

	sb.WriteString("\n</details>\n\n")

	return sb.String()
}

func formatFileDelta(fc coverage.FileChange) string {
	if fc.IsNew {
		return "`new`"
	}
	if fc.Delta == 0 {
		return "`\u00f8`"
	}
	if fc.Delta > 0 {
		return fmt.Sprintf("`+%.2f%%`", fc.Delta)
	}
	return fmt.Sprintf("`%.2f%%`", fc.Delta)
}

func formatFileName(path string, opts Options) string {
	if opts.RepoURL != "" && opts.SHA != "" {
		return fmt.Sprintf("[`%s`](%s/blob/%s/%s)", path, opts.RepoURL, opts.SHA, path)
	}
	return fmt.Sprintf("`%s`", path)
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
		if start == end {
			return fmt.Sprintf("[L%d](%s/blob/%s/%s#L%d)", start, repoURL, sha, filePath, start)
		}
		return fmt.Sprintf("[L%d-%d](%s/blob/%s/%s#L%d-L%d)", start, end, repoURL, sha, filePath, start, end)
	}
	if start == end {
		return fmt.Sprintf("L%d", start)
	}
	return fmt.Sprintf("L%d-%d", start, end)
}

func filterFiles(files []coverage.FileCoverage, opts Options) []coverage.FileCoverage {
	switch {
	case opts.ShowFiles == "all":
		return files

	case opts.ShowFiles == "changed":
		if len(opts.ChangedFiles) == 0 {
			return files
		}
		changedSet := make(map[string]bool)
		for _, f := range opts.ChangedFiles {
			changedSet[f] = true
		}
		var result []coverage.FileCoverage
		for _, f := range files {
			if changedSet[f.Path] {
				result = append(result, f)
			}
		}
		return result

	case strings.HasPrefix(opts.ShowFiles, "threshold:"):
		var result []coverage.FileCoverage
		for _, f := range files {
			if f.Percentage() < opts.Threshold {
				result = append(result, f)
			}
		}
		return result

	case strings.HasPrefix(opts.ShowFiles, "worst:"):
		sorted := make([]coverage.FileCoverage, len(files))
		copy(sorted, files)
		sort.Slice(sorted, func(i, j int) bool {
			return sorted[i].Percentage() < sorted[j].Percentage()
		})
		if opts.WorstN > len(sorted) {
			return sorted
		}
		return sorted[:opts.WorstN]

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
