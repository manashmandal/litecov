package diff

import (
	"testing"
)

func TestParseUnifiedDiff(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []FileDiff
	}{
		{
			name:     "empty diff",
			input:    "",
			expected: nil,
		},
		{
			name: "single file with single hunk",
			input: `diff --git a/file.go b/file.go
--- a/file.go
+++ b/file.go
@@ -10,2 +10,4 @@ func foo() {
 context before
+added line 1
+added line 2
 context after`,
			expected: []FileDiff{
				{
					Path: "file.go",
					AddedLines: []LineRange{
						{Start: 11, End: 12},
					},
				},
			},
		},
		{
			name: "single file with multiple hunks",
			input: `diff --git a/file.go b/file.go
--- a/file.go
+++ b/file.go
@@ -10,2 +10,3 @@ func foo() {
 context
+added line
 context
@@ -20,1 +22,3 @@ func bar() {
 context
+another added
+more added`,
			expected: []FileDiff{
				{
					Path: "file.go",
					AddedLines: []LineRange{
						{Start: 11, End: 11},
						{Start: 23, End: 24},
					},
				},
			},
		},
		{
			name: "multiple files",
			input: `diff --git a/first.go b/first.go
--- a/first.go
+++ b/first.go
@@ -5,2 +5,4 @@ package main
 context
+line 1
+line 2
 context
diff --git a/second.go b/second.go
--- a/second.go
+++ b/second.go
@@ -10,1 +10,3 @@ func test() {
 context
+added
 context`,
			expected: []FileDiff{
				{
					Path: "first.go",
					AddedLines: []LineRange{
						{Start: 6, End: 7},
					},
				},
				{
					Path: "second.go",
					AddedLines: []LineRange{
						{Start: 11, End: 11},
					},
				},
			},
		},
		{
			name: "new file (no old lines)",
			input: `diff --git a/newfile.go b/newfile.go
new file mode 100644
--- /dev/null
+++ b/newfile.go
@@ -0,0 +1,4 @@
+package main
+
+func main() {
+}`,
			expected: []FileDiff{
				{
					Path: "newfile.go",
					AddedLines: []LineRange{
						{Start: 1, End: 4},
					},
				},
			},
		},
		{
			name: "deleted file (no new lines)",
			input: `diff --git a/deleted.go b/deleted.go
deleted file mode 100644
--- a/deleted.go
+++ /dev/null
@@ -1,10 +0,0 @@
-package main
-
-func main() {
-}`,
			expected: nil,
		},
		{
			name: "single line change (no count in hunk header)",
			input: `diff --git a/file.go b/file.go
--- a/file.go
+++ b/file.go
@@ -5 +5 @@ package main
+modified line`,
			expected: []FileDiff{
				{
					Path: "file.go",
					AddedLines: []LineRange{
						{Start: 5, End: 5},
					},
				},
			},
		},
		{
			name: "binary file should be skipped",
			input: `diff --git a/image.png b/image.png
Binary files a/image.png and b/image.png differ
diff --git a/file.go b/file.go
--- a/file.go
+++ b/file.go
@@ -1,2 +1,3 @@ package main
+added line`,
			expected: []FileDiff{
				{
					Path: "file.go",
					AddedLines: []LineRange{
						{Start: 1, End: 1},
					},
				},
			},
		},
		{
			name: "file with path containing spaces",
			input: `diff --git a/path with spaces/file.go b/path with spaces/file.go
--- a/path with spaces/file.go
+++ b/path with spaces/file.go
@@ -1,2 +1,4 @@ package main
+line 1
+line 2`,
			expected: []FileDiff{
				{
					Path: "path with spaces/file.go",
					AddedLines: []LineRange{
						{Start: 1, End: 2},
					},
				},
			},
		},
		{
			name: "hunk with zero new lines (pure deletion)",
			input: `diff --git a/file.go b/file.go
--- a/file.go
+++ b/file.go
@@ -5,3 +5,0 @@ func foo() {
-deleted line 1
-deleted line 2
-deleted line 3`,
			expected: nil,
		},
		{
			name: "mixed additions and deletions in same file",
			input: `diff --git a/file.go b/file.go
--- a/file.go
+++ b/file.go
@@ -5,3 +5,0 @@ func foo() {
-deleted 1
-deleted 2
-deleted 3
@@ -15,2 +12,5 @@ func bar() {
 context
+added 1
+added 2
+added 3
 context`,
			expected: []FileDiff{
				{
					Path: "file.go",
					AddedLines: []LineRange{
						{Start: 13, End: 15},
					},
				},
			},
		},
		{
			name: "nested directory path",
			input: `diff --git a/internal/pkg/subpkg/file.go b/internal/pkg/subpkg/file.go
--- a/internal/pkg/subpkg/file.go
+++ b/internal/pkg/subpkg/file.go
@@ -1,1 +1,2 @@ package subpkg
+new line`,
			expected: []FileDiff{
				{
					Path: "internal/pkg/subpkg/file.go",
					AddedLines: []LineRange{
						{Start: 1, End: 1},
					},
				},
			},
		},
		{
			name: "hunk header with context text containing @@ symbols",
			input: `diff --git a/file.go b/file.go
--- a/file.go
+++ b/file.go
@@ -10,2 +10,3 @@ func processRegex(pattern string) { // @@ special @@
+added line`,
			expected: []FileDiff{
				{
					Path: "file.go",
					AddedLines: []LineRange{
						{Start: 10, End: 10},
					},
				},
			},
		},
		{
			name: "large line numbers",
			input: `diff --git a/bigfile.go b/bigfile.go
--- a/bigfile.go
+++ b/bigfile.go
@@ -10000,5 +10000,10 @@ func largefunc() {
+added lines`,
			expected: []FileDiff{
				{
					Path: "bigfile.go",
					AddedLines: []LineRange{
						{Start: 10000, End: 10000},
					},
				},
			},
		},
		{
			name: "file only with deletions should not appear",
			input: `diff --git a/removed_content.go b/removed_content.go
--- a/removed_content.go
+++ b/removed_content.go
@@ -1,5 +1,0 @@ package main
-line 1
-line 2
-line 3
-line 4
-line 5`,
			expected: nil,
		},
		{
			name: "consecutive hunks",
			input: `diff --git a/file.go b/file.go
--- a/file.go
+++ b/file.go
@@ -1,1 +1,2 @@ first
+a
@@ -3,1 +4,2 @@ second
+b
@@ -5,1 +7,2 @@ third
+c`,
			expected: []FileDiff{
				{
					Path: "file.go",
					AddedLines: []LineRange{
						{Start: 1, End: 1},
						{Start: 4, End: 4},
						{Start: 7, End: 7},
					},
				},
			},
		},
		{
			name: "quoted path with octal-escaped non-ASCII bytes",
			input: `diff --git "a/src/caf\303\251.go" "b/src/caf\303\251.go"
--- "a/src/caf\303\251.go"
+++ "b/src/caf\303\251.go"
@@ -3,1 +3,2 @@ func foo() {
+added line`,
			expected: []FileDiff{
				{
					Path: "src/café.go",
					AddedLines: []LineRange{
						{Start: 3, End: 3},
					},
				},
			},
		},
		{
			name: "path containing a literal \" b/\" sequence",
			input: `diff --git a/src/a b/c.go b/src/a b/c.go
--- a/src/a b/c.go
+++ b/src/a b/c.go
@@ -1,2 +1,3 @@ package main
+added line`,
			expected: []FileDiff{
				{
					Path: "src/a b/c.go",
					AddedLines: []LineRange{
						{Start: 1, End: 1},
					},
				},
			},
		},
		{
			name:  "CRLF line endings do not leave a trailing carriage return on the path",
			input: "diff --git a/file.go b/file.go\r\n--- a/file.go\r\n+++ b/file.go\r\n@@ -1,2 +1,3 @@ package main\r\n+added line\r\n",
			expected: []FileDiff{
				{
					Path: "file.go",
					AddedLines: []LineRange{
						{Start: 1, End: 1},
					},
				},
			},
		},
		{
			name: "renamed file uses the new path, not the old one",
			input: `diff --git a/old.go b/new.go
--- a/old.go
+++ b/new.go
@@ -1,1 +1,2 @@ package main
+added line`,
			expected: []FileDiff{
				{
					Path: "new.go",
					AddedLines: []LineRange{
						{Start: 1, End: 1},
					},
				},
			},
		},
		{
			name: "context lines around an added line are not reported as added",
			input: `diff --git a/file.go b/file.go
--- a/file.go
+++ b/file.go
@@ -8,7 +8,7 @@ func foo() {
 context 1
 context 2
 context 3
-old line
+new line
 context 4
 context 5
 context 6`,
			expected: []FileDiff{
				{
					Path: "file.go",
					AddedLines: []LineRange{
						{Start: 11, End: 11},
					},
				},
			},
		},
		{
			name: "deletion-only hunk with GitHub-style context reports no added lines",
			input: `diff --git a/file.go b/file.go
--- a/file.go
+++ b/file.go
@@ -10,7 +10,6 @@ func foo() {
 context 1
 context 2
 context 3
-removed line
 context 4
 context 5
 context 6`,
			expected: nil,
		},
		{
			name: "no newline at end of file marker does not advance the line counter",
			input: `diff --git a/file.go b/file.go
--- a/file.go
+++ b/file.go
@@ -1,2 +1,2 @@ func foo() {
 context
-old last line
\ No newline at end of file
+new last line`,
			expected: []FileDiff{
				{
					Path: "file.go",
					AddedLines: []LineRange{
						{Start: 2, End: 2},
					},
				},
			},
		},
		{
			// Built from the real "patch" field GitHub's REST API returned for
			// cmd/litecov/main.go in this repo's PR #4 (three lines of context,
			// the format every GitHub-sourced diff uses). Regression test for
			// https://github.com/manashmandal/litecov/issues/8: the old parser
			// reported the full hunk spans, 96 lines, as added; only 65 lines
			// were actually touched by the PR.
			name: "real GitHub patch with three lines of context",
			input: `diff --git a/cmd/litecov/main.go b/cmd/litecov/main.go
--- a/cmd/litecov/main.go
+++ b/cmd/litecov/main.go
@@ -259,32 +259,58 @@ func outputAnnotations(report *coverage.Report, changedFiles []string) {
 		changedSet[f] = true
 	}
 
+	// Track which changed files have coverage data
+	coveredChangedFiles := make(map[string]bool)
+
 	for _, file := range report.Files {
 		// Normalize path: strip Go module prefix to get repo-relative path
 		// Coverage paths may be like "github.com/user/repo/internal/foo.go"
 		// but we need "internal/foo.go" for GitHub annotations
 		relativePath := normalizePathForAnnotation(file.Path)
 
 		// Check if file is in changed set (use normalized path for matching)
-		if len(changedFiles) > 0 && !isPathInChangedSet(relativePath, changedSet) {
-			continue
+		matchedPath := ""
+		if len(changedFiles) > 0 {
+			matchedPath = findMatchingChangedFile(relativePath, changedSet)
+			if matchedPath == "" {
+				continue
+			}
+			coveredChangedFiles[matchedPath] = true
 		}
 
 		if len(file.UncoveredLines) == 0 {
 			continue
 		}
 
+		annotationPath := relativePath
+		if matchedPath != "" {
+			annotationPath = matchedPath
+		}
+
 		ranges := comment.GroupConsecutiveLines(file.UncoveredLines)
 		for _, r := range ranges {
 			if r.Start == r.End {
 				fmt.Printf("::warning file=%s,line=%d,title=Uncovered::Line %d not covered by tests\n",
-					relativePath, r.Start, r.Start)
+					annotationPath, r.Start, r.Start)
 			} else {
 				fmt.Printf("::warning file=%s,line=%d,endLine=%d,title=Uncovered::Lines %d-%d not covered by tests\n",
-					relativePath, r.Start, r.End, r.Start, r.End)
+					annotationPath, r.Start, r.End, r.Start, r.End)
 			}
 		}
 	}
+
+	// Output annotations for changed files that have no coverage data at all
+	// These are files that were never executed by any test
+	for _, changedFile := range changedFiles {
+		if coveredChangedFiles[changedFile] {
+			continue
+		}
+		// Only annotate source files (skip test files, configs, etc.)
+		if !isSourceFile(changedFile) {
+			continue
+		}
+		fmt.Printf("::warning file=%s,line=1,title=No Coverage::File has no test coverage\n", changedFile)
+	}
 }
 
 // normalizePathForAnnotation converts a Go module path to a repo-relative path
@@ -320,3 +346,38 @@ func isPathInChangedSet(path string, changedSet map[string]bool) bool {
 	}
 	return false
 }
+
+// findMatchingChangedFile returns the matching changed file path, or empty string if not found
+func findMatchingChangedFile(coveragePath string, changedSet map[string]bool) string {
+	if changedSet[coveragePath] {
+		return coveragePath
+	}
+	// Try suffix matching for paths that may have different prefixes
+	for changedPath := range changedSet {
+		if strings.HasSuffix(coveragePath, changedPath) || strings.HasSuffix(changedPath, coveragePath) {
+			return changedPath
+		}
+	}
+	return ""
+}
+
+// isSourceFile checks if a file is a source file that should have coverage
+func isSourceFile(path string) bool {
+	// Only check Go files for now (can be extended for other languages)
+	if !strings.HasSuffix(path, ".go") {
+		return false
+	}
+	// Skip test files
+	if strings.HasSuffix(path, "_test.go") {
+		return false
+	}
+	// Skip generated files (common patterns)
+	if strings.Contains(path, "/vendor/") ||
+		strings.Contains(path, "generated") ||
+		strings.Contains(path, ".pb.go") ||
+		strings.Contains(path, "_mock.go") ||
+		strings.Contains(path, "mock_") {
+		return false
+	}
+	return true
+}`,
			expected: []FileDiff{
				{
					Path: "cmd/litecov/main.go",
					AddedLines: []LineRange{
						{Start: 262, End: 264},
						{Start: 272, End: 278},
						{Start: 285, End: 289},
						{Start: 294, End: 294},
						{Start: 297, End: 297},
						{Start: 301, End: 313},
						{Start: 349, End: 383},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ParseUnifiedDiff(tt.input)

			if len(result) != len(tt.expected) {
				t.Fatalf("expected %d files, got %d", len(tt.expected), len(result))
			}

			for i, expectedFile := range tt.expected {
				if result[i].Path != expectedFile.Path {
					t.Errorf("file %d: expected path %q, got %q", i, expectedFile.Path, result[i].Path)
				}

				if len(result[i].AddedLines) != len(expectedFile.AddedLines) {
					t.Fatalf("file %d: expected %d line ranges, got %d",
						i, len(expectedFile.AddedLines), len(result[i].AddedLines))
				}

				for j, expectedRange := range expectedFile.AddedLines {
					if result[i].AddedLines[j].Start != expectedRange.Start {
						t.Errorf("file %d, range %d: expected start %d, got %d",
							i, j, expectedRange.Start, result[i].AddedLines[j].Start)
					}
					if result[i].AddedLines[j].End != expectedRange.End {
						t.Errorf("file %d, range %d: expected end %d, got %d",
							i, j, expectedRange.End, result[i].AddedLines[j].End)
					}
				}
			}
		})
	}
}

func TestLineRange(t *testing.T) {
	lr := LineRange{Start: 10, End: 20}
	if lr.Start != 10 {
		t.Errorf("expected Start to be 10, got %d", lr.Start)
	}
	if lr.End != 20 {
		t.Errorf("expected End to be 20, got %d", lr.End)
	}
}

func TestFileDiff(t *testing.T) {
	fd := FileDiff{
		Path: "test.go",
		AddedLines: []LineRange{
			{Start: 1, End: 5},
			{Start: 10, End: 15},
		},
	}

	if fd.Path != "test.go" {
		t.Errorf("expected Path to be 'test.go', got %q", fd.Path)
	}
	if len(fd.AddedLines) != 2 {
		t.Errorf("expected 2 AddedLines, got %d", len(fd.AddedLines))
	}
}
