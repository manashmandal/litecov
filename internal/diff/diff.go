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
	diffHeaderRegex = regexp.MustCompile(`^diff --git `)
	oldPathRegex    = regexp.MustCompile(`^--- (.+)$`)
	newPathRegex    = regexp.MustCompile(`^\+\+\+ (.+)$`)
	hunkHeaderRegex = regexp.MustCompile(`^@@ -\d+(?:,\d+)? \+(\d+)(?:,(\d+))? @@`)
	binaryFileRegex = regexp.MustCompile(`^Binary files`)
)

// ParseUnifiedDiff parses unified diff format output to extract changed line ranges.
// Input is the output of `git diff` or the GitHub API diff/patch, in standard
// unified diff format with any amount of context. GitHub always includes three
// lines of context around a change and gives no way to turn that off, so the
// hunk body has to be walked line by line to tell which new-side lines the PR
// actually added rather than just carried along as context.
// It returns a slice of FileDiff containing the new line numbers for lines the
// PR added; unchanged context lines and removed lines are excluded.
func ParseUnifiedDiff(diffOutput string) []FileDiff {
	if diffOutput == "" {
		return nil
	}

	var result []FileDiff
	var currentFile *FileDiff
	var isBinary bool
	var sawHunk bool
	var inHunk bool
	var lineNo int // next new-side line number while walking a hunk body

	lines := strings.Split(diffOutput, "\n")

	for _, line := range lines {
		// git always terminates diff lines with \n; strip a trailing \r so
		// CRLF input doesn't leak onto paths or hunk headers.
		line = strings.TrimSuffix(line, "\r")

		if diffHeaderRegex.MatchString(line) {
			if currentFile != nil && len(currentFile.AddedLines) > 0 {
				result = append(result, *currentFile)
			}
			currentFile = &FileDiff{AddedLines: []LineRange{}}
			isBinary = false
			sawHunk = false
			inHunk = false
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

		// Only trust "---"/"+++" as path lines before the first hunk of this
		// file: the "diff --git" header is ambiguous for quoted paths and for
		// paths containing " b/", so the unambiguous single-path "+++ b/<path>"
		// line (falling back to "--- a/<path>" for deleted files, where "+++"
		// is "/dev/null") is the source of truth instead. Once a hunk starts,
		// added/removed content lines could coincidentally look like path
		// lines, so stop matching them.
		if !sawHunk {
			if matches := oldPathRegex.FindStringSubmatch(line); matches != nil {
				currentFile.Path = gitDiffPath(matches[1])
				continue
			}

			if matches := newPathRegex.FindStringSubmatch(line); matches != nil {
				if path := gitDiffPath(matches[1]); path != "" {
					currentFile.Path = path
				}
				continue
			}
		}

		if matches := hunkHeaderRegex.FindStringSubmatch(line); matches != nil {
			sawHunk = true

			start, err := strconv.Atoi(matches[1])
			if err != nil {
				inHunk = false
				continue
			}

			lineNo = start
			inHunk = true
			continue
		}

		if !inHunk {
			continue
		}

		// Walk the hunk body: a "+" line is one the PR added, a " " line is
		// unchanged context GitHub carried along for readability, and a "-"
		// line only ever existed on the old side, so the three don't play
		// the same role. Only "+" lines get recorded; "+" and " " both
		// advance lineNo since both occupy a line on the new side, while
		// "-" and the "\ No newline at end of file" marker advance nothing.
		// A blank context line sometimes arrives as "" rather than " ", so
		// default to context instead of indexing an empty string.
		prefix := byte(' ')
		if line != "" {
			prefix = line[0]
		}

		switch prefix {
		case '+':
			appendAddedLine(currentFile, lineNo)
			lineNo++
		case ' ':
			lineNo++
		case '-', '\\':
			// Old-side-only line, or a "\ No newline at end of file"
			// marker: neither occupies a new-side line number.
		default:
			// Not a valid hunk body line - the checks above already
			// handle a new "diff --git" header and a new hunk header for
			// this file, so anything else means malformed or truncated
			// input. Stop trusting lineNo until the next hunk header.
			inHunk = false
		}
	}

	if currentFile != nil && len(currentFile.AddedLines) > 0 {
		result = append(result, *currentFile)
	}

	return result
}

// appendAddedLine records new-side line n as added, extending the last range
// in f.AddedLines when n continues it so a run of consecutive "+" lines
// collapses into a single range instead of one per line.
func appendAddedLine(f *FileDiff, n int) {
	if len(f.AddedLines) > 0 {
		if last := &f.AddedLines[len(f.AddedLines)-1]; last.End == n-1 {
			last.End = n
			return
		}
	}
	f.AddedLines = append(f.AddedLines, LineRange{Start: n, End: n})
}

// gitDiffPath turns a "---"/"+++" path spec into a repo-relative path. It
// strips the "a/"/"b/" mnemonic prefix and reverses git's C-style quoting,
// or returns "" for "/dev/null" (the added/deleted-file placeholder).
func gitDiffPath(spec string) string {
	if spec == "/dev/null" {
		return ""
	}

	spec = unquoteGitPath(spec)
	if len(spec) > 2 && (spec[:2] == "a/" || spec[:2] == "b/") {
		return spec[2:]
	}
	return spec
}

// unquoteGitPath reverses git's C-style path quoting. core.quotePath (on by
// default) wraps any path containing non-ASCII bytes, control characters, a
// backslash or a double quote in double quotes, escaping the byte C-style
// (\n, \t, \\, \", ...) or as a three-digit octal sequence (\NNN) per byte.
// Paths that aren't quoted are returned unchanged.
func unquoteGitPath(s string) string {
	if len(s) < 2 || s[0] != '"' || s[len(s)-1] != '"' {
		return s
	}
	inner := s[1 : len(s)-1]

	buf := make([]byte, 0, len(inner))
	for i := 0; i < len(inner); i++ {
		c := inner[i]
		if c != '\\' || i+1 >= len(inner) {
			buf = append(buf, c)
			continue
		}

		i++
		switch inner[i] {
		case 'a':
			buf = append(buf, '\a')
		case 'b':
			buf = append(buf, '\b')
		case 'f':
			buf = append(buf, '\f')
		case 'n':
			buf = append(buf, '\n')
		case 'r':
			buf = append(buf, '\r')
		case 't':
			buf = append(buf, '\t')
		case 'v':
			buf = append(buf, '\v')
		case '\\', '"':
			buf = append(buf, inner[i])
		default:
			if i+2 < len(inner) && isOctalDigit(inner[i]) && isOctalDigit(inner[i+1]) && isOctalDigit(inner[i+2]) {
				val := (inner[i]-'0')<<6 | (inner[i+1]-'0')<<3 | (inner[i+2] - '0')
				buf = append(buf, val)
				i += 2
			} else {
				buf = append(buf, '\\', inner[i])
			}
		}
	}
	return string(buf)
}

func isOctalDigit(b byte) bool {
	return b >= '0' && b <= '7'
}
