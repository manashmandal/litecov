package parser

import (
	"errors"
	"os"
	"strings"
	"testing"
)

func TestGoProfileParser_Parse(t *testing.T) {
	f, err := os.Open("../../testdata/simple.out")
	if err != nil {
		t.Fatalf("failed to open test file: %v", err)
	}
	defer f.Close()

	p := &GoProfileParser{}
	report, err := p.Parse(f)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if len(report.Files) != 2 {
		t.Errorf("got %d files, want 2", len(report.Files))
	}

	if report.Files[0].Path != "src/parser.go" {
		t.Errorf("Files[0].Path = %v, want src/parser.go", report.Files[0].Path)
	}
	if report.Files[0].LinesCovered != 3 {
		t.Errorf("Files[0].LinesCovered = %v, want 3", report.Files[0].LinesCovered)
	}
	if report.Files[0].LinesTotal != 4 {
		t.Errorf("Files[0].LinesTotal = %v, want 4", report.Files[0].LinesTotal)
	}

	if report.Files[1].Path != "src/utils.go" {
		t.Errorf("Files[1].Path = %v, want src/utils.go", report.Files[1].Path)
	}
	if report.Files[1].LinesCovered != 1 {
		t.Errorf("Files[1].LinesCovered = %v, want 1", report.Files[1].LinesCovered)
	}

	if report.TotalCovered != 4 {
		t.Errorf("TotalCovered = %v, want 4", report.TotalCovered)
	}
	if report.TotalLines != 6 {
		t.Errorf("TotalLines = %v, want 6", report.TotalLines)
	}
}

func TestGoProfileParser_Parse_IssueRepro(t *testing.T) {
	// The exact repro from issue #73: a block can span more than one
	// physical line (10.20,12.3 covers lines 10-12), which is what makes
	// this format richer than a single DA: record and is the behavior an
	// LCOV-shaped parser can't reuse as-is.
	profile := "mode: set\n" +
		"github.com/x/y/foo.go:10.20,12.3 2 1\n" +
		"github.com/x/y/foo.go:14.2,15.9 1 0\n"

	p := &GoProfileParser{}
	report, err := p.Parse(strings.NewReader(profile))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(report.Files) != 1 {
		t.Fatalf("got %d files, want 1", len(report.Files))
	}
	f := report.Files[0]
	if f.Path != "github.com/x/y/foo.go" {
		t.Errorf("Path = %v, want github.com/x/y/foo.go", f.Path)
	}
	if f.LinesTotal != 5 {
		t.Errorf("LinesTotal = %v, want 5 (lines 10,11,12,14,15)", f.LinesTotal)
	}
	if f.LinesCovered != 3 {
		t.Errorf("LinesCovered = %v, want 3 (lines 10,11,12 from the hit block)", f.LinesCovered)
	}
	if len(f.UncoveredLines) != 2 || f.UncoveredLines[0] != 14 || f.UncoveredLines[1] != 15 {
		t.Errorf("UncoveredLines = %v, want [14 15]", f.UncoveredLines)
	}
}

func TestGoProfileParser_Parse_Empty(t *testing.T) {
	p := &GoProfileParser{}
	report, err := p.Parse(strings.NewReader(""))
	if !errors.Is(err, ErrNoGoCoverageData) {
		t.Fatalf("Parse() error = %v, want ErrNoGoCoverageData", err)
	}
	if report != nil {
		t.Errorf("got report = %v, want nil", report)
	}
}

func TestGoProfileParser_Parse_ModeLineOnly(t *testing.T) {
	// A profile with a header and no blocks -- e.g. a package with no
	// statements to instrument -- has no line data at all, so it must
	// fail the same way an empty file does instead of reporting 0/0 as a
	// 0% report.
	p := &GoProfileParser{}
	report, err := p.Parse(strings.NewReader("mode: set\n"))
	if !errors.Is(err, ErrNoGoCoverageData) {
		t.Fatalf("Parse() error = %v, want ErrNoGoCoverageData", err)
	}
	if report != nil {
		t.Errorf("got report = %v, want nil", report)
	}
}

func TestGoProfileParser_Parse_NotGoProfile(t *testing.T) {
	// Content that isn't a Go coverage profile at all (an LCOV tracefile
	// here) has no line matching the block pattern, so the parse must
	// fail instead of succeeding with an empty report.
	lcov := "SF:/src/a.js\nDA:1,1\nend_of_record\n"
	p := &GoProfileParser{}
	report, err := p.Parse(strings.NewReader(lcov))
	if !errors.Is(err, ErrNoGoCoverageData) {
		t.Fatalf("Parse() error = %v, want ErrNoGoCoverageData", err)
	}
	if report != nil {
		t.Errorf("got report = %v, want nil", report)
	}
}

func TestGoProfileParser_Parse_ModeCountHigherThanOne(t *testing.T) {
	// mode: count (and atomic) record an actual execution count rather
	// than set's 0-or-1, so a block hit 5 times must still just count as
	// covered -- any positive count is a hit, not a fraction of one.
	profile := `mode: count
src/a.go:1.1,1.5 1 5
src/a.go:2.1,2.5 1 0`
	p := &GoProfileParser{}
	report, err := p.Parse(strings.NewReader(profile))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	f := report.Files[0]
	if f.LinesTotal != 2 {
		t.Errorf("LinesTotal = %v, want 2", f.LinesTotal)
	}
	if f.LinesCovered != 1 {
		t.Errorf("LinesCovered = %v, want 1", f.LinesCovered)
	}
	if len(f.UncoveredLines) != 1 || f.UncoveredLines[0] != 2 {
		t.Errorf("UncoveredLines = %v, want [2]", f.UncoveredLines)
	}
}

func TestGoProfileParser_Parse_ModeAtomic(t *testing.T) {
	profile := `mode: atomic
src/a.go:1.1,1.5 1 1`
	p := &GoProfileParser{}
	report, err := p.Parse(strings.NewReader(profile))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(report.Files) != 1 {
		t.Fatalf("got %d files, want 1", len(report.Files))
	}
	if report.Files[0].LinesCovered != 1 {
		t.Errorf("LinesCovered = %v, want 1", report.Files[0].LinesCovered)
	}
}

func TestGoProfileParser_Parse_OverlappingBlocksOred(t *testing.T) {
	// Line 3 falls inside both blocks: the first (lines 1-3) never ran,
	// the second (lines 3-4) did. A hit in either block must make line 3
	// covered, the same OR-the-hits merge LCOVParser applies to a line
	// repeated across DA: records.
	profile := `mode: set
src/a.go:1.1,3.2 2 0
src/a.go:3.5,4.2 1 1`
	p := &GoProfileParser{}
	report, err := p.Parse(strings.NewReader(profile))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	f := report.Files[0]
	if f.LinesTotal != 4 {
		t.Errorf("LinesTotal = %v, want 4", f.LinesTotal)
	}
	if f.LinesCovered != 2 {
		t.Errorf("LinesCovered = %v, want 2 (lines 3 and 4)", f.LinesCovered)
	}
	if len(f.UncoveredLines) != 2 || f.UncoveredLines[0] != 1 || f.UncoveredLines[1] != 2 {
		t.Errorf("UncoveredLines = %v, want [1 2]", f.UncoveredLines)
	}
}

func TestGoProfileParser_Parse_MalformedLineSkipped(t *testing.T) {
	profile := `mode: set
this is not a valid block line
src/a.go:1.1,1.5 1 1`
	p := &GoProfileParser{}
	report, err := p.Parse(strings.NewReader(profile))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(report.Files) != 1 {
		t.Fatalf("got %d files, want 1 (malformed line must be skipped, not fatal)", len(report.Files))
	}
	if report.Files[0].LinesTotal != 1 {
		t.Errorf("LinesTotal = %v, want 1", report.Files[0].LinesTotal)
	}
}

func TestGoProfileParser_Parse_InvertedRangeSkipped(t *testing.T) {
	// endLine before startLine names no real range and must be dropped
	// rather than crash on the reversed for loop, the same defensive
	// check LCOVParser applies to a DA: line number.
	profile := `mode: set
src/a.go:5.1,3.2 1 1
src/a.go:1.1,1.5 1 1`
	p := &GoProfileParser{}
	report, err := p.Parse(strings.NewReader(profile))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(report.Files) != 1 {
		t.Fatalf("got %d files, want 1", len(report.Files))
	}
	if report.Files[0].LinesTotal != 1 {
		t.Errorf("LinesTotal = %v, want 1 (the inverted-range block must be skipped)", report.Files[0].LinesTotal)
	}
}
