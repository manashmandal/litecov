// Package paths provides shared utilities for path handling and source file detection.
package paths

import (
	"path/filepath"
	"strings"
)

// IsSourceFile checks if a file is a source file that should have coverage.
// Supports Go and Python files, excludes test files, vendor directories, and generated files.
func IsSourceFile(path string) bool {
	// Go files
	if strings.HasSuffix(path, ".go") {
		return isGoSourceFile(path)
	}

	// Python files
	if strings.HasSuffix(path, ".py") {
		return isPythonSourceFile(path)
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
	// Skip generated files (common patterns)
	if strings.Contains(path, "generated") ||
		strings.HasSuffix(path, ".pb.go") ||
		strings.HasSuffix(path, "_mock.go") ||
		strings.Contains(path, "mock_") {
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
func FindMatchingChangedFile(coveragePath string, changedSet map[string]bool) string {
	if changedSet[coveragePath] {
		return coveragePath
	}
	// Try suffix matching for paths that may have different prefixes
	for changedPath := range changedSet {
		if HasSuffix(coveragePath, changedPath) || HasSuffix(changedPath, coveragePath) {
			return changedPath
		}
	}
	return ""
}

// HasSuffix checks if path ends with suffix (with proper path boundary).
func HasSuffix(path, suffix string) bool {
	if len(suffix) > len(path) {
		return false
	}
	if path == suffix {
		return true
	}
	// Check suffix with path boundary (/)
	if len(path) > len(suffix) && path[len(path)-len(suffix)-1] == '/' {
		return path[len(path)-len(suffix):] == suffix
	}
	return false
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
