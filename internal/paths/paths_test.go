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

		// Python files
		{"src/mypackage/module.py", true},
		{"lib/utils/helper.py", true},
		{"app/main.py", true},
		{"src/mypackage/test_module.py", false},
		{"src/mypackage/module_test.py", false},
		{"tests/test_something.py", false},
		{"conftest.py", false},
		{"tests/conftest.py", false},
		{"setup.py", false},
		{"__pycache__/module.cpython-39.pyc", false},
		{"src/__pycache__/module.py", false},
		{"venv/lib/python3.9/site-packages/pkg.py", false},
		{".venv/lib/python3.9/site-packages/pkg.py", false},

		// Non-source files
		{".github/workflows/ci.yml", false},
		{"README.md", false},
		{"package.json", false},
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
		"cmd/app/main.go":          true,
		"internal/foo/handler.go":  true,
		"src/mypackage/module.py":  true,
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
	}

	for _, tt := range tests {
		t.Run(tt.coveragePath, func(t *testing.T) {
			if got := FindMatchingChangedFile(tt.coveragePath, changedSet); got != tt.expected {
				t.Errorf("FindMatchingChangedFile(%q) = %q, want %q", tt.coveragePath, got, tt.expected)
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
		{"cmd/app/main.go", "main.go", true},
		{"cmd/app/main.go", "other.go", false},
		{"cmd/app/main.go", "app/main.go", true},
		// No boundary check for these
		{"xmain.go", "main.go", false}, // should fail - no path boundary
		{"main.go", "xmain.go", false}, // suffix longer than path
	}

	for _, tt := range tests {
		t.Run(tt.path+"_"+tt.suffix, func(t *testing.T) {
			if got := HasSuffix(tt.path, tt.suffix); got != tt.expected {
				t.Errorf("HasSuffix(%q, %q) = %v, want %v", tt.path, tt.suffix, got, tt.expected)
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
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			if got := NormalizePathForAnnotation(tt.path); got != tt.expected {
				t.Errorf("NormalizePathForAnnotation(%q) = %q, want %q", tt.path, got, tt.expected)
			}
		})
	}
}
