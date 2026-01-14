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
@@ -10,3 +10,5 @@ func foo() {
+added line 1
+added line 2`,
			expected: []FileDiff{
				{
					Path: "file.go",
					AddedLines: []LineRange{
						{Start: 10, End: 14},
					},
				},
			},
		},
		{
			name: "single file with multiple hunks",
			input: `diff --git a/file.go b/file.go
--- a/file.go
+++ b/file.go
@@ -10,3 +10,5 @@ func foo() {
+added line
@@ -20 +22,3 @@ func bar() {
+another added
+more added`,
			expected: []FileDiff{
				{
					Path: "file.go",
					AddedLines: []LineRange{
						{Start: 10, End: 14},
						{Start: 22, End: 24},
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
+line 1
+line 2
diff --git a/second.go b/second.go
--- a/second.go
+++ b/second.go
@@ -10,1 +10,3 @@ func test() {
+added`,
			expected: []FileDiff{
				{
					Path: "first.go",
					AddedLines: []LineRange{
						{Start: 5, End: 8},
					},
				},
				{
					Path: "second.go",
					AddedLines: []LineRange{
						{Start: 10, End: 12},
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
@@ -0,0 +1,10 @@
+package main
+
+func main() {
+}`,
			expected: []FileDiff{
				{
					Path: "newfile.go",
					AddedLines: []LineRange{
						{Start: 1, End: 10},
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
						{Start: 1, End: 3},
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
						{Start: 1, End: 4},
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
-deleted
@@ -15,2 +12,5 @@ func bar() {
+added 1
+added 2
+added 3`,
			expected: []FileDiff{
				{
					Path: "file.go",
					AddedLines: []LineRange{
						{Start: 12, End: 16},
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
						{Start: 1, End: 2},
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
						{Start: 10, End: 12},
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
						{Start: 10000, End: 10009},
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
						{Start: 1, End: 2},
						{Start: 4, End: 5},
						{Start: 7, End: 8},
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
