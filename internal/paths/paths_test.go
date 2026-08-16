package paths

import "testing"

func TestIsSourceFile(t *testing.T) {
	tests := []struct {
		path     string
		expected bool
	}{
		// Go files
		{"cmd/app/main.go", true},
		{"internal/foo/bar.go", true},
		{"pkg/util/helper.go", true},
		{"cmd/app/main_test.go", false},
		{"internal/foo/bar_test.go", false},
		{".github/workflows/ci.yml", false},
		{"README.md", false},
		{"vendor/github.com/pkg/errors/errors.go", false},
		{"internal/vendor/code.go", false},
		{"internal/generated/code.go", false},
		{"api/v1/types.pb.go", false},
		{"internal/mocks/mock_service.go", false},
		{"internal/test/service_mock.go", false},

		// Issue #16: testdata/ is never compiled by the Go toolchain, so
		// it can never appear in a coverage profile and must not be
		// reported as source with no coverage.
		{"internal/foo/testdata/sample.go", false},
		{"testdata/sample.go", false},
		// Issue #16: "generated" and "mock_" used to be matched as a
		// substring anywhere in the path, so real source living in a
		// similarly-named directory was silently excluded from coverage
		// reporting instead of being flagged as missing tests.
		{"internal/mock_data/service.go", true},
		{"internal/regenerated/handler.go", true},
		{"internal/generated_docs_helper.go", true},

		// Python files
		{"src/mypackage/module.py", true},
		{"lib/utils/helper.py", true},
		{"app/main.py", true},
		{"src/mypackage/test_module.py", false},
		{"src/mypackage/module_test.py", false},
		{"tests/test_something.py", false},
		{"conftest.py", false},
		{"tests/conftest.py", false},
		// Issue #16: pytest support modules under a top level tests/
		// directory (not just files matching test_*.py) are not source
		// that needs coverage.
		{"tests/helpers.py", false},
		{"tests/factories.py", false},
		{"setup.py", false},
		{"__pycache__/module.cpython-39.pyc", false},
		{"src/__pycache__/module.py", false},
		{"venv/lib/python3.9/site-packages/pkg.py", false},
		{".venv/lib/python3.9/site-packages/pkg.py", false},

		// Non-source files
		{".github/workflows/ci.yml", false},
		{"README.md", false},
		{"package.json", false},

		// Issue #15: IsSourceFile used to recognize only Go and Python,
		// silently skipping "no coverage" reporting for every other
		// language the README advertises. These are the exact paths from
		// the issue's repro.
		{"src/index.js", true},
		{"src/index.ts", true},
		{"src/App.tsx", true},
		{"src/main.rs", true},
		{"src/engine.cpp", true},
		{"src/engine.c", true},
		{"app/models/user.rb", true},
		{"src/main/Foo.java", true},
		{"src/Service.cs", true},
		{"src/Main.kt", true},

		// JavaScript/TypeScript - remaining extensions
		{"src/App.jsx", true},
		{"src/module.mjs", true},
		{"src/module.cjs", true},

		// C/C++ - remaining extensions
		{"src/engine.cc", true},
		{"include/engine.h", true},
		{"include/engine.hpp", true},

		// Swift files
		{"Sources/App/ContentView.swift", true},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			if got := IsSourceFile(tt.path); got != tt.expected {
				t.Errorf("IsSourceFile(%q) = %v, want %v", tt.path, got, tt.expected)
			}
		})
	}
}

func TestFindMatchingChangedFile(t *testing.T) {
	changedSet := map[string]bool{
		"cmd/app/main.go":         true,
		"internal/foo/handler.go": true,
		"src/mypackage/module.py": true,
	}

	tests := []struct {
		coveragePath string
		expected     string
	}{
		// Go files - exact match
		{"cmd/app/main.go", "cmd/app/main.go"},
		{"internal/foo/handler.go", "internal/foo/handler.go"},
		// Go files - suffix match
		{"github.com/user/repo/cmd/app/main.go", "cmd/app/main.go"},
		{"github.com/user/repo/internal/foo/handler.go", "internal/foo/handler.go"},
		// Python files - exact match
		{"src/mypackage/module.py", "src/mypackage/module.py"},
		// Python files - suffix match (pytest-cov absolute path)
		{"/home/runner/work/repo/src/mypackage/module.py", "src/mypackage/module.py"},
		// No match
		{"internal/other/file.go", ""},
		{"src/other/module.py", ""},
		// Issue #13: a vendored file with the same tail must not steal the
		// match meant for the repo's own internal/foo/handler.go.
		{"vendor/github.com/other/lib/internal/foo/handler.go", ""},
	}

	for _, tt := range tests {
		t.Run(tt.coveragePath, func(t *testing.T) {
			if got := FindMatchingChangedFile(tt.coveragePath, changedSet); got != tt.expected {
				t.Errorf("FindMatchingChangedFile(%q) = %q, want %q", tt.coveragePath, got, tt.expected)
			}
		})
	}
}

// TestFindMatchingChangedFileAmbiguous covers issue #14: when a coverage path
// suffix-matches more than one changed file (e.g. the same filename changed
// at two directory depths in a monorepo), the match used to be resolved by
// ranging over the changedSet map, so the same input returned a different
// changed file across runs depending on Go's randomized map iteration order.
// It must now report no match instead of guessing.
func TestFindMatchingChangedFileAmbiguous(t *testing.T) {
	changedSet := map[string]bool{
		"internal/x.go":     true,
		"pkg/internal/x.go": true,
	}

	tests := []struct {
		coveragePath string
		expected     string
	}{
		// Both "internal/x.go" and "pkg/internal/x.go" are legitimately
		// suffix-anchored by the "github.com/foo/bar" module prefix, so this
		// is a genuine ambiguity rather than the false-positive kind fixed
		// for issue #13.
		{"github.com/foo/bar/pkg/internal/x.go", ""},
	}

	for _, tt := range tests {
		t.Run(tt.coveragePath, func(t *testing.T) {
			// Call repeatedly: the historical bug depended on map iteration
			// order, so a single call could return the expected "" by luck.
			for i := 0; i < 10; i++ {
				if got := FindMatchingChangedFile(tt.coveragePath, changedSet); got != tt.expected {
					t.Fatalf("FindMatchingChangedFile(%q) = %q, want %q (run %d)", tt.coveragePath, got, tt.expected, i)
				}
			}
		})
	}
}

func TestHasSuffix(t *testing.T) {
	tests := []struct {
		path     string
		suffix   string
		expected bool
	}{
		{"cmd/app/main.go", "cmd/app/main.go", true},
		{"github.com/user/repo/cmd/app/main.go", "cmd/app/main.go", true},
		{"/home/runner/work/repo/src/module.py", "src/module.py", true},
		{"cmd/app/main.go", "other.go", false},
		// Not anchored to the repo root: "cmd/" and "app/" are ordinary
		// sibling directories, not a wrapper prefix, so these must not match.
		{"cmd/app/main.go", "main.go", false},
		{"cmd/app/main.go", "app/main.go", false},
		// No boundary check for these
		{"xmain.go", "main.go", false}, // should fail - no path boundary
		{"main.go", "xmain.go", false}, // suffix longer than path
		// Issue #13: unrelated directories must not match just because a
		// "/" boundary happens to line up.
		{"vendor/github.com/other/lib/internal/foo.go", "internal/foo.go", false},
		{"web/src/index.py", "src/index.py", false},
		{"x/y/a/b.go", "a/b.go", false},
	}

	for _, tt := range tests {
		t.Run(tt.path+"_"+tt.suffix, func(t *testing.T) {
			if got := HasSuffix(tt.path, tt.suffix); got != tt.expected {
				t.Errorf("HasSuffix(%q, %q) = %v, want %v", tt.path, tt.suffix, got, tt.expected)
			}
		})
	}
}

// TestNormalizeCoveragePath covers issue #19: coverage report paths were
// compared against GitHub's changed file list exactly as both sides
// produced them, so a report using backslash separators (a Windows runner,
// e.g. Coverlet) or containing an unclean ".." segment (coverage.py and
// istanbul both emit these in some configurations) never matched.
func TestNormalizeCoveragePath(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		expected string
	}{
		// The exact repro from issue #19.
		{"backslash separators from a Windows runner", `src\Foo\Bar.py`, "src/Foo/Bar.py"},
		{"unclean path with a .. segment", "src/Foo/../Foo/Bar.py", "src/Foo/Bar.py"},
		// Combination of both in one path.
		{"backslashes and a .. segment together", `src\Foo\..\Foo\Bar.py`, "src/Foo/Bar.py"},
		// Leading "./" is stripped.
		{"leading dot-slash", "./src/foo.py", "src/foo.py"},
		// GITHUB_WORKSPACE checks the repo out to .../<repo>/<repo>, so a
		// doubled path segment is stripped when the path is absolute.
		{"doubled GITHUB_WORKSPACE segment", "/home/runner/work/repo/repo/src/foo.py", "src/foo.py"},
		// Already clean paths are left alone.
		{"already clean relative path", "internal/foo.go", "internal/foo.go"},
		{"empty path", "", ""},
		// An absolute path that isn't a recognized GITHUB_WORKSPACE
		// checkout must keep its leading "/": HasSuffix's
		// isPathWrapperPrefix relies on that leading "/" to tell an
		// absolute-path wrapper apart from a genuine sibling directory, and
		// stripping it unconditionally here would break that check for
		// every other absolute-path coverage tool (e.g. pytest-cov).
		{"absolute path without a doubled segment stays absolute",
			"/home/runner/work/repo/src/mypackage/module.py",
			"/home/runner/work/repo/src/mypackage/module.py"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NormalizeCoveragePath(tt.path); got != tt.expected {
				t.Errorf("NormalizeCoveragePath(%q) = %q, want %q", tt.path, got, tt.expected)
			}
		})
	}
}

// TestFindMatchingChangedFileNormalized proves the actual fix for issue
// #19's repro: FindMatchingChangedFile is not itself normalization-aware
// (it stays a plain string matcher), but every caller runs
// NormalizeCoveragePath over a report's paths before handing them to it, so
// the two repro paths now resolve to the changed file instead of nothing.
func TestFindMatchingChangedFileNormalized(t *testing.T) {
	changedSet := map[string]bool{"src/Foo/Bar.py": true}

	tests := []struct {
		name         string
		coveragePath string
	}{
		{"backslash separators", `src\Foo\Bar.py`},
		{"unclean .. segment", "src/Foo/../Foo/Bar.py"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			normalized := NormalizeCoveragePath(tt.coveragePath)
			if got := FindMatchingChangedFile(normalized, changedSet); got != "src/Foo/Bar.py" {
				t.Errorf("FindMatchingChangedFile(NormalizeCoveragePath(%q), ...) = %q, want %q",
					tt.coveragePath, got, "src/Foo/Bar.py")
			}
		})
	}
}

// TestStripPathPrefix covers the path-prefix input (issue #19): a coverage
// report generated inside a subdirectory needs that subdirectory stripped
// before it lines up with GitHub's repo-root-relative changed file paths.
func TestStripPathPrefix(t *testing.T) {
	tests := []struct {
		path     string
		prefix   string
		expected string
	}{
		{"backend/src/foo.py", "backend/", "src/foo.py"},
		// Trailing slash on prefix is optional.
		{"backend/src/foo.py", "backend", "src/foo.py"},
		// Exact match strips down to nothing.
		{"backend", "backend/", ""},
		// No prefix configured is a no-op.
		{"src/foo.py", "", "src/foo.py"},
		// No match leaves the path untouched.
		{"other/foo.py", "backend/", "other/foo.py"},
		// A "/" boundary is required: "backend" must not strip a sibling
		// directory that merely starts with the same letters.
		{"backendx/foo.py", "backend", "backendx/foo.py"},
	}

	for _, tt := range tests {
		t.Run(tt.path+"_"+tt.prefix, func(t *testing.T) {
			if got := StripPathPrefix(tt.path, tt.prefix); got != tt.expected {
				t.Errorf("StripPathPrefix(%q, %q) = %q, want %q", tt.path, tt.prefix, got, tt.expected)
			}
		})
	}
}

// TestParsePathFixes covers the path-fixes input (issue #19), one
// "before::after" rule per line matching codecov.yml's fixes: shorthand.
func TestParsePathFixes(t *testing.T) {
	raw := "before/::after/\n" +
		"::root/\n" +
		"drop/::\n" +
		"\n" +
		"  \n" +
		"not-a-rule\n"

	got := ParsePathFixes(raw)
	want := []PathFix{
		{Before: "before/", After: "after/"},
		{Before: "", After: "root/"},
		{Before: "drop/", After: ""},
	}

	if len(got) != len(want) {
		t.Fatalf("ParsePathFixes() returned %d rules, want %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("ParsePathFixes()[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
}

// TestApplyPathFixes covers the three fixes: forms from codecov.yml that
// issue #19 asks path-fixes to mirror: moving a path, moving the root, and
// reducing the root.
func TestApplyPathFixes(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		fixes    []PathFix
		expected string
	}{
		{
			name:     "move path: before/::after/",
			path:     "before/foo.py",
			fixes:    []PathFix{{Before: "before/", After: "after/"}},
			expected: "after/foo.py",
		},
		{
			name:     "move root: ::after/ prepends to every path",
			path:     "src/foo.py",
			fixes:    []PathFix{{Before: "", After: "backend/"}},
			expected: "backend/src/foo.py",
		},
		{
			name:     "reduce root: before/:: strips with nothing put back",
			path:     "before/foo.py",
			fixes:    []PathFix{{Before: "before/", After: ""}},
			expected: "foo.py",
		},
		{
			name:     "no matching rule leaves the path untouched",
			path:     "src/foo.py",
			fixes:    []PathFix{{Before: "other/", After: "else/"}},
			expected: "src/foo.py",
		},
		{
			name:     "no rules configured is a no-op",
			path:     "src/foo.py",
			fixes:    nil,
			expected: "src/foo.py",
		},
		{
			name: "first matching rule wins",
			path: "before/foo.py",
			fixes: []PathFix{
				{Before: "before/", After: "first/"},
				{Before: "before/", After: "second/"},
			},
			expected: "first/foo.py",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ApplyPathFixes(tt.path, tt.fixes); got != tt.expected {
				t.Errorf("ApplyPathFixes(%q, %+v) = %q, want %q", tt.path, tt.fixes, got, tt.expected)
			}
		})
	}
}

// TestNormalizeAndFixPath covers the full per-file pipeline main.go runs
// over a parsed report (issue #19): baseline normalization, then
// path-prefix, then path-fixes.
func TestNormalizeAndFixPath(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		prefix   string
		fixes    []PathFix
		expected string
	}{
		{
			name:     "baseline normalization only",
			path:     `src\Foo\..\Foo\Bar.py`,
			prefix:   "",
			fixes:    nil,
			expected: "src/Foo/Bar.py",
		},
		{
			name:     "subdirectory report needs the prefix stripped",
			path:     "backend/src/foo.py",
			prefix:   "backend/",
			fixes:    nil,
			expected: "src/foo.py",
		},
		{
			name:     "backslashes normalized before the prefix is stripped",
			path:     `backend\src\foo.py`,
			prefix:   "backend/",
			fixes:    nil,
			expected: "src/foo.py",
		},
		{
			name:     "prefix and fixes both apply, in order",
			path:     "backend/old/foo.py",
			prefix:   "backend/",
			fixes:    []PathFix{{Before: "old/", After: "new/"}},
			expected: "new/foo.py",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NormalizeAndFixPath(tt.path, tt.prefix, tt.fixes); got != tt.expected {
				t.Errorf("NormalizeAndFixPath(%q, %q, %+v) = %q, want %q",
					tt.path, tt.prefix, tt.fixes, got, tt.expected)
			}
		})
	}
}

func TestNormalizePathForAnnotation(t *testing.T) {
	tests := []struct {
		path     string
		expected string
	}{
		// Go module paths
		{"github.com/user/repo/internal/foo.go", "internal/foo.go"},
		{"github.com/user/repo/cmd/app/main.go", "cmd/app/main.go"},
		{"github.com/user/repo/pkg/util/helper.go", "pkg/util/helper.go"},
		{"gitlab.com/user/repo/api/handler.go", "api/handler.go"},
		// Python paths (pytest-cov generates absolute paths)
		{"/home/runner/work/repo/src/mypackage/module.py", "src/mypackage/module.py"},
		{"/home/runner/work/repo/lib/utils.py", "lib/utils.py"},
		{"/home/runner/work/repo/app/main.py", "app/main.py"},
		{"/home/runner/work/repo/tests/test_module.py", "tests/test_module.py"},
		// Already relative paths
		{"internal/foo.go", "internal/foo.go"},
		{"src/module.py", "src/module.py"},
		// No markers found
		{"simple.go", "simple.go"},
		{"module.py", "module.py"},
		// Regression for #17: the marker scan must pick whichever marker
		// occurs earliest in the path, not the first entry in the markers
		// slice to match anywhere in it.
		{"/home/runner/work/repo/repo/src/api/handler.py", "src/api/handler.py"},
		{"/home/runner/work/repo/repo/app/src/module.py", "app/src/module.py"},
		{"github.com/user/repo/pkg/internal/cache.go", "pkg/internal/cache.go"},
		// Regression for #17: the doubled GITHUB_WORKSPACE segment
		// (/home/runner/work/<repo>/<repo>) is stripped even when nothing
		// below it matches a known marker directory.
		{"/home/runner/work/repo/repo/module.py", "module.py"},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			if got := NormalizePathForAnnotation(tt.path); got != tt.expected {
				t.Errorf("NormalizePathForAnnotation(%q) = %q, want %q", tt.path, got, tt.expected)
			}
		})
	}
}
