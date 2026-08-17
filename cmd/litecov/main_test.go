package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/manashmandal/litecov/internal/coverage"
	"github.com/manashmandal/litecov/internal/parser"
)

func TestLoadBaseReport_NoReport(t *testing.T) {
	tests := []struct {
		name string
		path string
	}{
		{"empty path", ""},
		{"missing file", filepath.Join(t.TempDir(), "does-not-exist.lcov")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := loadBaseReport(tt.path, "", nil); got != nil {
				t.Errorf("loadBaseReport(%q) = %v, want nil", tt.path, got)
			}
		})
	}
}

func TestLoadBaseReport_SourcePrefixMatchesHead(t *testing.T) {
	// Reproduces issue #29: the head parser is built with
	// GetParserWithPath(detected, *coverageFile), which sets LCOVParser's
	// SourcePrefix from the coverage file's own location
	// (js/coverage/lcov.info -> "js"), so a relative SF: path picks up that
	// prefix. loadBaseReport used to build its parser with plain GetParser
	// (no path), leaving SourcePrefix empty, so identical LCOV input
	// produced "js/src/a.js" on the head side and "src/a.js" on the base
	// side. NewComparison's lookup then missed on the file entirely, even
	// though nothing about its actual coverage changed between head and
	// base.
	const lcovSrc = "SF:src/a.js\nDA:1,1\nDA:2,1\nDA:3,0\nDA:4,0\nend_of_record\n"

	dir := t.TempDir()
	coverageFile := filepath.Join(dir, "js", "coverage", "lcov.info")
	if err := os.MkdirAll(filepath.Dir(coverageFile), 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(coverageFile, []byte(lcovSrc), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	headParser, err := parser.GetParserWithPath("lcov", coverageFile)
	if err != nil {
		t.Fatalf("GetParserWithPath: %v", err)
	}
	head, err := headParser.Parse(strings.NewReader(lcovSrc))
	if err != nil {
		t.Fatalf("head Parse: %v", err)
	}

	base := loadBaseReport(coverageFile, "", nil)
	if base == nil {
		t.Fatal("loadBaseReport returned nil")
	}

	if len(head.Files) != 1 || len(base.Files) != 1 {
		t.Fatalf("head.Files = %d, base.Files = %d, want 1 each", len(head.Files), len(base.Files))
	}
	if head.Files[0].Path != base.Files[0].Path {
		t.Fatalf("head path %q != base path %q: base report must pick up the same source prefix as head", head.Files[0].Path, base.Files[0].Path)
	}

	comp := coverage.NewComparison(head, base, nil, nil, nil)
	if len(comp.FileChanges) != 1 {
		t.Fatalf("FileChanges length = %d, want 1 (same file matched on both sides, not one entry per side)", len(comp.FileChanges))
	}

	fc := comp.FileChanges[0]
	if fc.NoBaseData {
		t.Error("NoBaseData should be false: base has a matching entry for this file")
	}
	if fc.NoCoverage {
		t.Error("NoCoverage should be false: the file is present in both head and base")
	}
	if fc.Delta != 0 {
		t.Errorf("Delta = %v, want 0: head and base coverage are identical", fc.Delta)
	}
	if fc.BaseCoverage != fc.HeadCoverage {
		t.Errorf("BaseCoverage = %v, HeadCoverage = %v, want equal", fc.BaseCoverage, fc.HeadCoverage)
	}
}
