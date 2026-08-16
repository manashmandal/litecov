// Package paths provides shared utilities for path handling and source file detection.
package paths

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// IsSourceFile checks if a file is a source file that should have coverage.
// Recognizes every language the README advertises across LCOV and Cobertura
// XML input: Go, Python, JavaScript/TypeScript, Rust, C/C++, Ruby,
// Java/Kotlin, C#, and Swift. Go and Python additionally exclude their own
// test files, vendor directories, and generated files; the other languages
// are recognized by extension only.
func IsSourceFile(path string) bool {
	// Go files
	if strings.HasSuffix(path, ".go") {
		return isGoSourceFile(path)
	}

	// Python files
	if strings.HasSuffix(path, ".py") {
		return isPythonSourceFile(path)
	}

	// JavaScript/TypeScript files (Jest, Vitest, c8, nyc)
	if strings.HasSuffix(path, ".js") || strings.HasSuffix(path, ".jsx") ||
		strings.HasSuffix(path, ".mjs") || strings.HasSuffix(path, ".cjs") ||
		strings.HasSuffix(path, ".ts") || strings.HasSuffix(path, ".tsx") {
		return true
	}

	// Rust files (grcov, tarpaulin)
	if strings.HasSuffix(path, ".rs") {
		return true
	}

	// C/C++ files (gcov, llvm-cov)
	if strings.HasSuffix(path, ".c") || strings.HasSuffix(path, ".cc") ||
		strings.HasSuffix(path, ".cpp") || strings.HasSuffix(path, ".h") ||
		strings.HasSuffix(path, ".hpp") {
		return true
	}

	// Ruby files (SimpleCov)
	if strings.HasSuffix(path, ".rb") {
		return true
	}

	// Java/Kotlin files (Cobertura, JaCoCo)
	if strings.HasSuffix(path, ".java") || strings.HasSuffix(path, ".kt") {
		return true
	}

	// C# files (Coverlet)
	if strings.HasSuffix(path, ".cs") {
		return true
	}

	// Swift files (llvm-cov via XCTest)
	if strings.HasSuffix(path, ".swift") {
		return true
	}

	return false
}

// isGoSourceFile checks if a Go file is a source file (not test/vendor/generated).
func isGoSourceFile(path string) bool {
	// Skip test files
	if strings.HasSuffix(path, "_test.go") {
		return false
	}
	// Skip vendor directory
	if strings.HasPrefix(path, "vendor/") || strings.Contains(path, "/vendor/") {
		return false
	}
	// Skip testdata directory: the Go toolchain never compiles anything
	// under testdata/, so it can never appear in a coverage profile.
	if strings.HasPrefix(path, "testdata/") || strings.Contains(path, "/testdata/") {
		return false
	}
	// Skip generated files (common patterns). "generated" is anchored to a
	// whole directory segment and "mock_" to a filename prefix, so real
	// source like "regenerated/handler.go" or "mock_data/service.go" isn't
	// excluded just for containing the substring.
	base := filepath.Base(path)
	if strings.HasPrefix(path, "generated/") || strings.Contains(path, "/generated/") ||
		strings.HasSuffix(path, ".pb.go") ||
		strings.HasSuffix(path, "_mock.go") ||
		strings.HasPrefix(base, "mock_") {
		return false
	}
	return true
}

// isPythonSourceFile checks if a Python file is a source file (not test/cache/config).
func isPythonSourceFile(path string) bool {
	base := filepath.Base(path)

	// Skip __pycache__ directories
	if strings.Contains(path, "__pycache__") {
		return false
	}
	// Skip test files
	if strings.HasSuffix(path, "_test.py") ||
		strings.HasPrefix(base, "test_") {
		return false
	}
	// Skip a top level tests/ or test/ directory: common pytest support
	// modules like tests/helpers.py or tests/factories.py don't match a
	// test_*.py naming convention but still aren't source needing coverage.
	if strings.HasPrefix(path, "tests/") || strings.HasPrefix(path, "test/") {
		return false
	}
	// Skip pytest configuration
	if base == "conftest.py" {
		return false
	}
	// Skip setup files
	if base == "setup.py" {
		return false
	}
	// Skip virtualenv/venv directories
	if strings.Contains(path, "/venv/") ||
		strings.Contains(path, "/.venv/") ||
		strings.HasPrefix(path, "venv/") ||
		strings.HasPrefix(path, ".venv/") {
		return false
	}
	// Skip common non-source Python files
	if strings.Contains(path, "/site-packages/") {
		return false
	}
	return true
}

// FindMatchingChangedFile returns the matching changed file path, or empty string if not found.
// It performs exact match first, then suffix matching for paths with different prefixes.
//
// changedSet is a map, and Go randomizes map iteration order, so suffix matching
// collects every candidate before deciding anything rather than returning on the
// first one it happens to visit. A match is only accepted when exactly one
// candidate remains: zero candidates is "not found", and two or more is ambiguous
// and is treated as "not found" too, with a warning naming the candidates so the
// ambiguity is visible instead of being resolved arbitrarily.
func FindMatchingChangedFile(coveragePath string, changedSet map[string]bool) string {
	if changedSet[coveragePath] {
		return coveragePath
	}
	// Try suffix matching for paths that may have different prefixes
	var candidates []string
	for changedPath := range changedSet {
		if HasSuffix(coveragePath, changedPath) || HasSuffix(changedPath, coveragePath) {
			candidates = append(candidates, changedPath)
		}
	}
	switch len(candidates) {
	case 0:
		return ""
	case 1:
		return candidates[0]
	default:
		sort.Strings(candidates)
		fmt.Fprintf(os.Stderr, "Warning: coverage path %q matches multiple changed files (%s), skipping\n",
			coveragePath, strings.Join(candidates, ", "))
		return ""
	}
}

// HasSuffix checks if path ends with suffix (with proper path boundary),
// anchored so the remaining prefix looks like something a coverage tool
// wraps around a repo-relative path (an absolute filesystem path, or a Go
// module/VCS host like "github.com") rather than an unrelated sibling
// directory. Without this, a vendored "internal/foo.go" or a "web/src/x.py"
// would match a changed file that only shares a filename and directory
// segment, not a real path.
func HasSuffix(path, suffix string) bool {
	if len(suffix) > len(path) {
		return false
	}
	if path == suffix {
		return true
	}
	// Check suffix with path boundary (/)
	if len(path) > len(suffix) && path[len(path)-len(suffix)-1] == '/' {
		if path[len(path)-len(suffix):] != suffix {
			return false
		}
		return isPathWrapperPrefix(path[:len(path)-len(suffix)-1])
	}
	return false
}

// isPathWrapperPrefix reports whether prefix is the kind of thing wrapped
// around a repo-relative path rather than a genuine sibling directory: an
// absolute filesystem path (pytest-cov style), or a dotted host segment
// like "github.com" or "gitlab.com" (Go module style).
func isPathWrapperPrefix(prefix string) bool {
	if prefix == "" || strings.HasPrefix(prefix, "/") {
		return true
	}
	first := prefix
	if idx := strings.IndexByte(prefix, '/'); idx >= 0 {
		first = prefix[:idx]
	}
	return strings.Contains(first, ".")
}

// NormalizePathForAnnotation converts a Go module or Python package path to a repo-relative path.
// e.g., "github.com/user/repo/internal/foo.go" -> "internal/foo.go"
// e.g., "/home/runner/work/repo/src/mypackage/module.py" -> "src/mypackage/module.py"
func NormalizePathForAnnotation(path string) string {
	// Common directory markers for Go and Python projects
	markers := []string{
		// Go markers
		"/internal/", "/cmd/", "/pkg/", "/api/",
		// Python markers
		"/src/", "/lib/", "/app/",
		// Common test directories
		"/test/", "/tests/",
	}
	for _, marker := range markers {
		if idx := strings.Index(path, marker); idx >= 0 {
			return path[idx+1:] // +1 to skip the leading slash
		}
	}
	// If no marker found but path contains github.com or similar,
	// try to extract after the third slash (github.com/user/repo/...)
	parts := strings.SplitN(path, "/", 4)
	if len(parts) == 4 && (strings.Contains(parts[0], ".") || parts[0] == "github") {
		return parts[3]
	}
	return path
}
