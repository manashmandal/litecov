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
	// An empty path is the only case where (nil, nil) is correct: no base
	// comparison was requested at all. Every other way loadBaseReport can
	// come back empty is a base that WAS requested but couldn't be read, and
	// must return a non-nil error instead so the caller -- and the PR
	// comment -- can tell the two apart (see
	// TestLoadBaseReport_HardFailuresReturnError, issue #39).
	report, err := loadBaseReport("", "", nil)
	if report != nil || err != nil {
		t.Errorf("loadBaseReport(\"\") = (%v, %v), want (nil, nil)", report, err)
	}
}

// TestLoadBaseReport_HardFailuresReturnError reproduces issue #39: a base
// coverage file that couldn't be opened, whose format couldn't be detected,
// or that failed to parse into any files used to make loadBaseReport return
// nil the same way an empty path does -- with no error to explain why, and
// for two of these three cases, not even a line on stderr. The PR comment
// then rendered as if no base had been configured at all, instead of saying
// the comparison was requested but broken.
func TestLoadBaseReport_HardFailuresReturnError(t *testing.T) {
	tests := []struct {
		name    string
		content string
	}{
		{name: "unrecognized content", content: "this is not a coverage report\njust plain text\n"},
		{name: "lcov with no SF: record, from the issue's repro", content: "end_of_record\n"},
		{name: "cobertura with no packages, from the issue's repro", content: "<coverage><packages></packages></coverage>"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "base.txt")
			if err := os.WriteFile(path, []byte(tt.content), 0644); err != nil {
				t.Fatalf("WriteFile: %v", err)
			}

			report, err := loadBaseReport(path, "", nil)
			if err == nil {
				t.Fatal("loadBaseReport returned a nil error, want the failure reported")
			}
			if report != nil {
				t.Errorf("loadBaseReport returned a non-nil report alongside an error: %v", report)
			}
		})
	}

	t.Run("missing file", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "does-not-exist.lcov")
		report, err := loadBaseReport(path, "", nil)
		if err == nil {
			t.Fatal("loadBaseReport returned a nil error, want the open failure reported")
		}
		if report != nil {
			t.Errorf("loadBaseReport returned a non-nil report alongside an error: %v", report)
		}
	})
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

	base, err := loadBaseReport(coverageFile, "", nil)
	if err != nil {
		t.Fatalf("loadBaseReport: %v", err)
	}
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
