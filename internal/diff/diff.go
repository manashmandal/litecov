package diff

import (
	"regexp"
	"strconv"
	"strings"
)

// LineRange represents a range of line numbers (inclusive)
type LineRange struct {
	Start int
	End   int
}

// FileDiff represents changed lines in a file
type FileDiff struct {
	Path       string
	AddedLines []LineRange
}

var (
	diffHeaderRegex = regexp.MustCompile(`^diff --git a/.+ b/(.+)$`)
	hunkHeaderRegex = regexp.MustCompile(`^@@ -\d+(?:,\d+)? \+(\d+)(?:,(\d+))? @@`)
	binaryFileRegex = regexp.MustCompile(`^Binary files`)
)

// ParseUnifiedDiff parses unified diff format output to extract changed line ranges.
// Input is the output of `git diff --unified=0` or GitHub API diff.
// It returns a slice of FileDiff containing the new line numbers for added/modified lines.
func ParseUnifiedDiff(diffOutput string) []FileDiff {
	if diffOutput == "" {
		return nil
	}

	var result []FileDiff
	var currentFile *FileDiff
	var isBinary bool

	lines := strings.Split(diffOutput, "\n")

	for _, line := range lines {
		if matches := diffHeaderRegex.FindStringSubmatch(line); matches != nil {
			if currentFile != nil && len(currentFile.AddedLines) > 0 {
				result = append(result, *currentFile)
			}
			currentFile = &FileDiff{
				Path:       matches[1],
				AddedLines: []LineRange{},
			}
			isBinary = false
			continue
		}

		if binaryFileRegex.MatchString(line) {
			isBinary = true
			continue
		}

		if isBinary {
			continue
		}

		if currentFile == nil {
			continue
		}

		if matches := hunkHeaderRegex.FindStringSubmatch(line); matches != nil {
			start, err := strconv.Atoi(matches[1])
			if err != nil {
				continue
			}

			count := 1
			if matches[2] != "" {
				count, err = strconv.Atoi(matches[2])
				if err != nil {
					continue
				}
			}

			if count == 0 {
				continue
			}

			lineRange := LineRange{
				Start: start,
				End:   start + count - 1,
			}
			currentFile.AddedLines = append(currentFile.AddedLines, lineRange)
		}
	}

	if currentFile != nil && len(currentFile.AddedLines) > 0 {
		result = append(result, *currentFile)
	}

	return result
}
