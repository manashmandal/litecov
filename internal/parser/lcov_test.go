package parser

import (
	"errors"
	"os"
	"strings"
	"testing"
)

func TestLCOVParser_Parse(t *testing.T) {
	f, err := os.Open("../../testdata/simple.lcov")
	if err != nil {
		t.Fatalf("failed to open test file: %v", err)
	}
	defer f.Close()

	p := &LCOVParser{}
	report, err := p.Parse(f)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if len(report.Files) != 2 {
		t.Errorf("got %d files, want 2", len(report.Files))
	}

	if report.Files[0].Path != "/src/parser.go" {
		t.Errorf("Files[0].Path = %v, want /src/parser.go", report.Files[0].Path)
	}
	if report.Files[0].LinesCovered != 3 {
		t.Errorf("Files[0].LinesCovered = %v, want 3", report.Files[0].LinesCovered)
	}
	if report.Files[0].LinesTotal != 4 {
		t.Errorf("Files[0].LinesTotal = %v, want 4", report.Files[0].LinesTotal)
	}

	if report.Files[1].Path != "/src/utils.go" {
		t.Errorf("Files[1].Path = %v, want /src/utils.go", report.Files[1].Path)
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

func TestLCOVParser_Parse_CoveredLines(t *testing.T) {
	// issue #6: CoveredLines has to carry the actual hit line numbers, not
	// just feed LinesCovered's count, since patch coverage intersects a PR
	// diff's added lines against it.
	f, err := os.Open("../../testdata/simple.lcov")
	if err != nil {
		t.Fatalf("failed to open test file: %v", err)
	}
	defer f.Close()

	p := &LCOVParser{}
	report, err := p.Parse(f)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if len(report.Files[0].CoveredLines) != 3 ||
		report.Files[0].CoveredLines[0] != 1 ||
		report.Files[0].CoveredLines[1] != 2 ||
		report.Files[0].CoveredLines[2] != 4 {
		t.Errorf("Files[0].CoveredLines = %v, want [1 2 4]", report.Files[0].CoveredLines)
	}
	if len(report.Files[1].CoveredLines) != 1 || report.Files[1].CoveredLines[0] != 1 {
		t.Errorf("Files[1].CoveredLines = %v, want [1]", report.Files[1].CoveredLines)
	}
}

func TestLCOVParser_Parse_Empty(t *testing.T) {
	// Mirrors the repro in issue #68: an empty tracefile has no coverage
	// data at all, so it must fail with ErrNoCoverageData rather than
	// come back as a silent 0% report.
	p := &LCOVParser{}
	report, err := p.Parse(strings.NewReader(""))
	if !errors.Is(err, ErrNoCoverageData) {
		t.Fatalf("Parse() error = %v, want ErrNoCoverageData", err)
	}
	if report != nil {
		t.Errorf("got report = %v, want nil", report)
	}
}

func TestLCOVParser_Parse_NotLCOV(t *testing.T) {
	// Mirrors the repro in issue #68: content that isn't LCOV at all (a Go
	// coverage.out profile here) has no SF:/DA: records to match, so every
	// line falls through the switch and the parse must fail instead of
	// reporting 0/0 lines as 0% coverage.
	goCoverage := "mode: set\ngithub.com/manashmandal/litecov/internal/parser/lcov.go:19.36,20.24 1 1\n"
	p := &LCOVParser{}
	report, err := p.Parse(strings.NewReader(goCoverage))
	if !errors.Is(err, ErrNoCoverageData) {
		t.Fatalf("Parse() error = %v, want ErrNoCoverageData", err)
	}
	if report != nil {
		t.Errorf("got report = %v, want nil", report)
	}
}

func TestLCOVParser_Parse_PlainTextWithSFSubstring(t *testing.T) {
	// Mirrors the last repro in issue #68: DetectFormat classifies any
	// content containing the substring "SF:" as LCOV, so plain text that
	// merely mentions it (but has no line that starts with "SF:") can reach
	// LCOVParser with zero real records. It must fail the same way as any
	// other non-LCOV input.
	text := "Coverage summary: see SF:/build/report for the raw log.\n"
	p := &LCOVParser{}
	report, err := p.Parse(strings.NewReader(text))
	if !errors.Is(err, ErrNoCoverageData) {
		t.Fatalf("Parse() error = %v, want ErrNoCoverageData", err)
	}
	if report != nil {
		t.Errorf("got report = %v, want nil", report)
	}
}

func TestLCOVParser_Parse_ZeroHits(t *testing.T) {
	lcov := `SF:/src/test.go
DA:1,0
DA:2,0
end_of_record`
	p := &LCOVParser{}
	report, err := p.Parse(strings.NewReader(lcov))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(report.Files) != 1 {
		t.Fatalf("got %d files, want 1", len(report.Files))
	}
	if report.Files[0].LinesCovered != 0 {
		t.Errorf("LinesCovered = %v, want 0", report.Files[0].LinesCovered)
	}
	if report.Files[0].LinesTotal != 2 {
		t.Errorf("LinesTotal = %v, want 2", report.Files[0].LinesTotal)
	}
}

func TestLCOVParser_Parse_MalformedDA(t *testing.T) {
	lcov := `SF:/src/test.go
DA:invalid
DA:1,5
end_of_record`
	p := &LCOVParser{}
	report, err := p.Parse(strings.NewReader(lcov))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if report.Files[0].LinesTotal != 1 {
		t.Errorf("LinesTotal = %v, want 1 (should skip malformed line)", report.Files[0].LinesTotal)
	}
}

func TestLCOVParser_Parse_NoEndOfRecord(t *testing.T) {
	lcov := `SF:/src/test.go
DA:1,1`
	p := &LCOVParser{}
	report, err := p.Parse(strings.NewReader(lcov))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(report.Files) != 1 {
		t.Fatalf("got %d files, want 1 (trailing record without end_of_record must still be flushed)", len(report.Files))
	}
	if report.Files[0].Path != "/src/test.go" {
		t.Errorf("Files[0].Path = %v, want /src/test.go", report.Files[0].Path)
	}
	if report.Files[0].LinesCovered != 1 {
		t.Errorf("LinesCovered = %v, want 1", report.Files[0].LinesCovered)
	}
	if report.Files[0].LinesTotal != 1 {
		t.Errorf("LinesTotal = %v, want 1", report.Files[0].LinesTotal)
	}
}

func TestLCOVParser_Parse_MultipleRecordsLastMissingEndOfRecord(t *testing.T) {
	// Mirrors the repro in issue #61: a truncated tracefile where every
	// record but the last is properly closed.
	lcov := `SF:/src/a.js
DA:1,1
DA:2,1
LF:2
LH:2
end_of_record
SF:/src/b.js
DA:1,0
DA:2,0
DA:3,0
LF:3
LH:0`
	p := &LCOVParser{}
	report, err := p.Parse(strings.NewReader(lcov))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(report.Files) != 2 {
		t.Fatalf("got %d files, want 2 (trailing record must not be dropped)", len(report.Files))
	}
	if report.Files[1].Path != "/src/b.js" {
		t.Errorf("Files[1].Path = %v, want /src/b.js", report.Files[1].Path)
	}
	if report.Files[1].LinesCovered != 0 {
		t.Errorf("Files[1].LinesCovered = %v, want 0", report.Files[1].LinesCovered)
	}
	if report.Files[1].LinesTotal != 3 {
		t.Errorf("Files[1].LinesTotal = %v, want 3", report.Files[1].LinesTotal)
	}
	if report.TotalCovered != 2 {
		t.Errorf("TotalCovered = %v, want 2", report.TotalCovered)
	}
	if report.TotalLines != 5 {
		t.Errorf("TotalLines = %v, want 5", report.TotalLines)
	}
}

func TestLCOVParser_Parse_LF_LH_Ignored(t *testing.T) {
	// Mirrors the repro in issue #62: LF:/LH: are lcov's own summary of the
	// DA: records in this block, not an independent source of truth. A
	// stale or wrong header must not override the totals derived from the
	// DA: records themselves.
	lcov := `SF:/src/test.go
DA:1,1
DA:2,1
DA:3,0
LF:10
LH:5
end_of_record`
	p := &LCOVParser{}
	report, err := p.Parse(strings.NewReader(lcov))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(report.Files) != 1 {
		t.Fatalf("got %d files, want 1", len(report.Files))
	}
	if report.Files[0].LinesTotal != 3 {
		t.Errorf("LinesTotal = %v, want 3 (from DA:, LF:10 must be ignored)", report.Files[0].LinesTotal)
	}
	if report.Files[0].LinesCovered != 2 {
		t.Errorf("LinesCovered = %v, want 2 (from DA:, LH:5 must be ignored)", report.Files[0].LinesCovered)
	}
	if len(report.Files[0].UncoveredLines) != 1 || report.Files[0].UncoveredLines[0] != 3 {
		t.Errorf("UncoveredLines = %v, want [3]", report.Files[0].UncoveredLines)
	}
}

func TestLCOVParser_Parse_DABeforeSF(t *testing.T) {
	lcov := `DA:1,1
SF:/src/test.go
DA:2,1
end_of_record`
	p := &LCOVParser{}
	report, err := p.Parse(strings.NewReader(lcov))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(report.Files) != 1 {
		t.Fatalf("got %d files, want 1", len(report.Files))
	}
	if report.Files[0].LinesTotal != 1 {
		t.Errorf("LinesTotal = %v, want 1", report.Files[0].LinesTotal)
	}
}

func TestLCOVParser_Parse_LeadingBOM(t *testing.T) {
	// Mirrors the repro in issue #67: a UTF-8 BOM before the first "SF:"
	// must not hide the record it belongs to.
	lcov := "\uFEFFSF:/src/a.js\nDA:1,1\nDA:2,0\nend_of_record"
	p := &LCOVParser{}
	report, err := p.Parse(strings.NewReader(lcov))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(report.Files) != 1 {
		t.Fatalf("got %d files, want 1 (BOM must not drop the first record)", len(report.Files))
	}
	if report.Files[0].Path != "/src/a.js" {
		t.Errorf("Files[0].Path = %v, want /src/a.js", report.Files[0].Path)
	}
	if report.Files[0].LinesCovered != 1 {
		t.Errorf("LinesCovered = %v, want 1", report.Files[0].LinesCovered)
	}
	if report.Files[0].LinesTotal != 2 {
		t.Errorf("LinesTotal = %v, want 2", report.Files[0].LinesTotal)
	}
}

func TestLCOVParser_Parse_MergesDuplicateSFRecords(t *testing.T) {
	// Mirrors the repro in issue #60: three shard records for the same
	// file, each hitting a different one of its three lines. Merged, the
	// file is fully covered; it must not come out as three separate 33%
	// entries that each double-count the file's own line total.
	lcov := `TN:
SF:src/app.ts
DA:1,1
DA:2,0
DA:3,0
LF:3
LH:1
end_of_record
TN:
SF:src/app.ts
DA:1,0
DA:2,1
DA:3,0
LF:3
LH:1
end_of_record
TN:
SF:src/app.ts
DA:1,0
DA:2,0
DA:3,1
LF:3
LH:1
end_of_record`
	p := &LCOVParser{}
	report, err := p.Parse(strings.NewReader(lcov))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(report.Files) != 1 {
		t.Fatalf("got %d files, want 1 (duplicate SF: records must merge, not append)", len(report.Files))
	}
	f := report.Files[0]
	if f.LinesTotal != 3 {
		t.Errorf("LinesTotal = %v, want 3", f.LinesTotal)
	}
	if f.LinesCovered != 3 {
		t.Errorf("LinesCovered = %v, want 3 (every line was hit by at least one record)", f.LinesCovered)
	}
	if len(f.UncoveredLines) != 0 {
		t.Errorf("UncoveredLines = %v, want none", f.UncoveredLines)
	}
	if report.TotalCovered != 3 {
		t.Errorf("TotalCovered = %v, want 3", report.TotalCovered)
	}
	if report.TotalLines != 3 {
		t.Errorf("TotalLines = %v, want 3", report.TotalLines)
	}
	if report.Coverage != 100 {
		t.Errorf("Coverage = %v, want 100", report.Coverage)
	}
}

func TestLCOVParser_Parse_MergeKeepsGenuinelyUncoveredLines(t *testing.T) {
	// A line that no record ever hits must stay uncovered after merging;
	// only lines hit by at least one record flip to covered.
	lcov := `SF:src/app.ts
DA:1,1
DA:2,0
end_of_record
SF:src/app.ts
DA:1,0
DA:2,0
end_of_record`
	p := &LCOVParser{}
	report, err := p.Parse(strings.NewReader(lcov))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(report.Files) != 1 {
		t.Fatalf("got %d files, want 1", len(report.Files))
	}
	f := report.Files[0]
	if f.LinesCovered != 1 {
		t.Errorf("LinesCovered = %v, want 1", f.LinesCovered)
	}
	if f.LinesTotal != 2 {
		t.Errorf("LinesTotal = %v, want 2", f.LinesTotal)
	}
	if len(f.UncoveredLines) != 1 || f.UncoveredLines[0] != 2 {
		t.Errorf("UncoveredLines = %v, want [2]", f.UncoveredLines)
	}
}

func TestLCOVParser_Parse_MergeCoveredLines(t *testing.T) {
	// issue #6: mergeFileRecord has its own CoveredLines derivation,
	// separate from finalizeRecord's single-record path, so a duplicate SF:
	// record (the normal shape of a tracefile assembled from shards) needs
	// its own check that CoveredLines comes out right too.
	lcov := `SF:src/app.ts
DA:1,1
DA:2,0
DA:3,0
end_of_record
SF:src/app.ts
DA:1,0
DA:2,1
DA:3,0
end_of_record`
	p := &LCOVParser{}
	report, err := p.Parse(strings.NewReader(lcov))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	f := report.Files[0]
	if len(f.CoveredLines) != 2 || f.CoveredLines[0] != 1 || f.CoveredLines[1] != 2 {
		t.Errorf("CoveredLines = %v, want [1 2] (hit by at least one record, sorted)", f.CoveredLines)
	}
}

func TestLCOVParser_Parse_DuplicateDA(t *testing.T) {
	// Mirrors the repro in issue #59: line 5 is hit by one DA: record and
	// missed by another in the same SF:/end_of_record block. A hit beats a
	// miss, so line 5 must count as covered and the block has two lines
	// total, not three.
	lcov := `SF:/src/a.js
DA:5,1
DA:5,0
DA:6,1
end_of_record`
	p := &LCOVParser{}
	report, err := p.Parse(strings.NewReader(lcov))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(report.Files) != 1 {
		t.Fatalf("got %d files, want 1", len(report.Files))
	}
	f := report.Files[0]
	if f.LinesTotal != 2 {
		t.Errorf("LinesTotal = %v, want 2 (line 5 must not be counted twice)", f.LinesTotal)
	}
	if f.LinesCovered != 2 {
		t.Errorf("LinesCovered = %v, want 2 (line 5 was hit by one of its records)", f.LinesCovered)
	}
	if len(f.UncoveredLines) != 0 {
		t.Errorf("UncoveredLines = %v, want none", f.UncoveredLines)
	}
}

func TestLCOVParser_Parse_DuplicateDAReverseOrder(t *testing.T) {
	// Same as TestLCOVParser_Parse_DuplicateDA but with the miss recorded
	// before the hit, per the issue's second repro. Order must not matter.
	lcov := `SF:/src/a.js
DA:5,0
DA:5,1
end_of_record`
	p := &LCOVParser{}
	report, err := p.Parse(strings.NewReader(lcov))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	f := report.Files[0]
	if f.LinesTotal != 1 {
		t.Errorf("LinesTotal = %v, want 1", f.LinesTotal)
	}
	if f.LinesCovered != 1 {
		t.Errorf("LinesCovered = %v, want 1", f.LinesCovered)
	}
	if len(f.UncoveredLines) != 0 {
		t.Errorf("UncoveredLines = %v, want none", f.UncoveredLines)
	}
}

func TestLCOVParser_Parse_DuplicateDAUncoveredNotDuplicated(t *testing.T) {
	// A line missed by every DA: record that names it must appear in
	// UncoveredLines exactly once, not once per record. The issue notes
	// this duplication was leaking into comment.GroupConsecutiveLines as
	// two overlapping ::warning annotations for the same line.
	lcov := `SF:/src/a.js
DA:5,0
DA:5,0
DA:6,1
end_of_record`
	p := &LCOVParser{}
	report, err := p.Parse(strings.NewReader(lcov))
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
	if len(f.UncoveredLines) != 1 || f.UncoveredLines[0] != 5 {
		t.Errorf("UncoveredLines = %v, want [5]", f.UncoveredLines)
	}
}

func TestLCOVParser_Parse_ZeroLineNumberSkipped(t *testing.T) {
	// Mirrors the first repro in issue #65: DA:0,0 is not a real line -- the
	// lcov format defines the line number as a non-zero integer -- so it
	// must be dropped instead of counted as an uncovered line 0.
	lcov := `SF:/src/a.js
DA:0,0
DA:1,1
end_of_record`
	p := &LCOVParser{}
	report, err := p.Parse(strings.NewReader(lcov))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	f := report.Files[0]
	if f.LinesTotal != 1 {
		t.Errorf("LinesTotal = %v, want 1 (DA:0,0 must not be counted)", f.LinesTotal)
	}
	if f.LinesCovered != 1 {
		t.Errorf("LinesCovered = %v, want 1", f.LinesCovered)
	}
	if len(f.UncoveredLines) != 0 {
		t.Errorf("UncoveredLines = %v, want none (no line=0 annotation)", f.UncoveredLines)
	}
}

func TestLCOVParser_Parse_NonNumericLineNumberSkipped(t *testing.T) {
	// Mirrors the second repro in issue #65: a non-numeric line number
	// silently becomes 0 from strconv.Atoi's error return. Both bogus rows
	// must be dropped, leaving only the genuine DA:1,1.
	lcov := `SF:/src/a.js
DA:abc,1
DA:undefined,1
DA:1,1
end_of_record`
	p := &LCOVParser{}
	report, err := p.Parse(strings.NewReader(lcov))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	f := report.Files[0]
	if f.LinesTotal != 1 {
		t.Errorf("LinesTotal = %v, want 1 (non-numeric DA: rows must not be counted)", f.LinesTotal)
	}
	if f.LinesCovered != 1 {
		t.Errorf("LinesCovered = %v, want 1", f.LinesCovered)
	}
}

func TestLCOVParser_Parse_EmptyLines(t *testing.T) {
	lcov := `SF:/src/test.go

DA:1,1

end_of_record`
	p := &LCOVParser{}
	report, err := p.Parse(strings.NewReader(lcov))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(report.Files) != 1 {
		t.Fatalf("got %d files, want 1", len(report.Files))
	}
}

func TestLCOVParser_Parse_ScientificNotationHits(t *testing.T) {
	// Mirrors the first repro in issue #66: some gcov and JS toolchains
	// write very high execution counts in scientific notation, which
	// strconv.Atoi rejects. Discarding that error used to default the
	// count to 0 and record line 1 as a miss even though it ran a
	// million times.
	lcov := `SF:/src/a.js
DA:1,1e+06
DA:2,1
end_of_record`
	p := &LCOVParser{}
	report, err := p.Parse(strings.NewReader(lcov))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	f := report.Files[0]
	if f.LinesCovered != 2 {
		t.Errorf("LinesCovered = %v, want 2 (line 1's 1e+06 hits must count as covered)", f.LinesCovered)
	}
	if f.LinesTotal != 2 {
		t.Errorf("LinesTotal = %v, want 2", f.LinesTotal)
	}
	if len(f.UncoveredLines) != 0 {
		t.Errorf("UncoveredLines = %v, want none", f.UncoveredLines)
	}
}

func TestLCOVParser_Parse_WhitespaceInHits(t *testing.T) {
	// Mirrors the second repro in issue #66: whitespace after the comma,
	// which a tracefile assembled by hand or by a shell script can carry,
	// also makes strconv.Atoi fail and must not be counted as a miss.
	lcov := `SF:/src/a.js
DA:1, 5
DA:2,1
end_of_record`
	p := &LCOVParser{}
	report, err := p.Parse(strings.NewReader(lcov))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	f := report.Files[0]
	if f.LinesCovered != 2 {
		t.Errorf("LinesCovered = %v, want 2 (line 1's ' 5' hits must count as covered)", f.LinesCovered)
	}
	if f.LinesTotal != 2 {
		t.Errorf("LinesTotal = %v, want 2", f.LinesTotal)
	}
}

func TestLCOVParser_Parse_UnparseableHitsSkipped(t *testing.T) {
	// A hits field that isn't a number at all has nothing to fall back to,
	// unlike the two repros above, and must be dropped the same way an
	// unparseable DA: line number already is, instead of being counted as
	// a miss.
	lcov := `SF:/src/a.js
DA:1,abc
DA:2,1
end_of_record`
	p := &LCOVParser{}
	report, err := p.Parse(strings.NewReader(lcov))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	f := report.Files[0]
	if f.LinesTotal != 1 {
		t.Errorf("LinesTotal = %v, want 1 (DA:1,abc must be skipped, not counted as a miss)", f.LinesTotal)
	}
	if f.LinesCovered != 1 {
		t.Errorf("LinesCovered = %v, want 1", f.LinesCovered)
	}
}

func TestLCOVParser_Parse_EmptyRecordSkipped(t *testing.T) {
	// Mirrors the repro in issue #69: an SF: record with no usable DA: rows
	// -- a file excluded by an ignore pattern, a generated file, or a
	// header with LF:/LH: but no line data -- finalizes with LinesTotal ==
	// 0 and must be dropped rather than kept as a phantom 0% file.
	lcov := `SF:/src/empty.js
LF:0
LH:0
end_of_record
SF:/src/a.js
DA:1,1
end_of_record`
	p := &LCOVParser{}
	report, err := p.Parse(strings.NewReader(lcov))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(report.Files) != 1 {
		t.Fatalf("got %d files, want 1 (empty record must be dropped)", len(report.Files))
	}
	if report.Files[0].Path != "/src/a.js" {
		t.Errorf("Files[0].Path = %v, want /src/a.js", report.Files[0].Path)
	}
	if report.TotalLines != 1 {
		t.Errorf("TotalLines = %v, want 1", report.TotalLines)
	}
	if report.Coverage != 100 {
		t.Errorf("Coverage = %v, want 100", report.Coverage)
	}
}

func TestLCOVParser_Parse_AllDAMalformedRecordSkipped(t *testing.T) {
	// A record whose only DA: rows are all rejected -- one non-numeric,
	// one naming line 0 -- also finalizes with LinesTotal == 0 and must be
	// dropped the same way as a record with no DA: rows at all.
	lcov := `SF:/src/broken.js
DA:invalid
DA:0,1
end_of_record
SF:/src/a.js
DA:1,1
end_of_record`
	p := &LCOVParser{}
	report, err := p.Parse(strings.NewReader(lcov))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(report.Files) != 1 {
		t.Fatalf("got %d files, want 1 (record with only malformed DA: rows must be dropped)", len(report.Files))
	}
	if report.Files[0].Path != "/src/a.js" {
		t.Errorf("Files[0].Path = %v, want /src/a.js", report.Files[0].Path)
	}
}

func TestLCOVParser_Parse_EmptyTrailingRecordSkipped(t *testing.T) {
	// Same as TestLCOVParser_Parse_EmptyRecordSkipped but for the trailing
	// record with no closing end_of_record, which is flushed by the
	// separate code path after the scan loop. A real record precedes it so
	// the report still has coverage data overall -- see
	// TestLCOVParser_Parse_OnlyEmptyTrailingRecord for the case where the
	// empty trailing record is the only thing in the file.
	lcov := `SF:/src/a.js
DA:1,1
end_of_record
SF:/src/empty.js`
	p := &LCOVParser{}
	report, err := p.Parse(strings.NewReader(lcov))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(report.Files) != 1 {
		t.Fatalf("got %d files, want 1 (empty trailing record must be dropped)", len(report.Files))
	}
	if report.Files[0].Path != "/src/a.js" {
		t.Errorf("Files[0].Path = %v, want /src/a.js", report.Files[0].Path)
	}
}

func TestLCOVParser_Parse_BRDAPartialAndMissedBranches(t *testing.T) {
	// Mirrors the repro in issue #63: line 3 has one branch taken and one
	// "-" (never reached), line 4 has both branches taken 0 times. Neither
	// counts as a clean hit even though DA: shows both lines executed, so
	// the file must read 0/2 (0%), not 2/2 (100%) with BRDA: ignored.
	lcov := `SF:/src/a.js
FN:3,foo
FNDA:1,foo
FNF:1
FNH:1
DA:3,1
DA:4,1
BRDA:3,0,0,1
BRDA:3,0,1,-
BRDA:4,0,0,0
BRDA:4,0,1,0
BRF:4
BRH:1
LF:2
LH:2
end_of_record`
	p := &LCOVParser{}
	report, err := p.Parse(strings.NewReader(lcov))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(report.Files) != 1 {
		t.Fatalf("got %d files, want 1", len(report.Files))
	}
	f := report.Files[0]
	if f.LinesTotal != 2 {
		t.Errorf("LinesTotal = %v, want 2", f.LinesTotal)
	}
	if f.LinesCovered != 0 {
		t.Errorf("LinesCovered = %v, want 0 (a missed branch must stop a DA: hit from counting as clean)", f.LinesCovered)
	}
	if len(f.UncoveredLines) != 2 || f.UncoveredLines[0] != 3 || f.UncoveredLines[1] != 4 {
		t.Errorf("UncoveredLines = %v, want [3 4]", f.UncoveredLines)
	}
	if report.Coverage != 0 {
		t.Errorf("Coverage = %v, want 0", report.Coverage)
	}
}

func TestLCOVParser_Parse_BRDAAllBranchesTaken(t *testing.T) {
	// A line whose every branch was taken must still count as a clean hit;
	// BRDA: data should only ever demote a line, never add a hit that DA:
	// didn't already report.
	lcov := `SF:/src/a.js
DA:3,1
BRDA:3,0,0,1
BRDA:3,0,1,1
end_of_record`
	p := &LCOVParser{}
	report, err := p.Parse(strings.NewReader(lcov))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	f := report.Files[0]
	if f.LinesCovered != 1 {
		t.Errorf("LinesCovered = %v, want 1 (all branches taken)", f.LinesCovered)
	}
	if len(f.UncoveredLines) != 0 {
		t.Errorf("UncoveredLines = %v, want none", f.UncoveredLines)
	}
}

func TestLCOVParser_Parse_BRDADuplicateBranchOred(t *testing.T) {
	// The same block:branch pair reported twice (e.g. concatenated shards)
	// must OR together like duplicate DA: records do: a taken in either
	// row makes the branch taken, so all of line 3's branches end up
	// covered and it counts as a clean hit.
	lcov := `SF:/src/a.js
DA:3,1
BRDA:3,0,0,0
BRDA:3,0,0,1
BRDA:3,0,1,1
end_of_record`
	p := &LCOVParser{}
	report, err := p.Parse(strings.NewReader(lcov))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	f := report.Files[0]
	if f.LinesCovered != 1 {
		t.Errorf("LinesCovered = %v, want 1 (duplicate branch row must OR, not overwrite, with a hit)", f.LinesCovered)
	}
}

func TestLCOVParser_Parse_BRDANoMatchingDA(t *testing.T) {
	// A BRDA: line number with no DA: record for it is unusual input --
	// real producers always pair the two -- and must not synthesize a new
	// line entry or otherwise change the totals.
	lcov := `SF:/src/a.js
DA:1,1
BRDA:99,0,0,0
end_of_record`
	p := &LCOVParser{}
	report, err := p.Parse(strings.NewReader(lcov))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	f := report.Files[0]
	if f.LinesTotal != 1 {
		t.Errorf("LinesTotal = %v, want 1 (BRDA: with no matching DA: must not add a line)", f.LinesTotal)
	}
	if f.LinesCovered != 1 {
		t.Errorf("LinesCovered = %v, want 1", f.LinesCovered)
	}
}

func TestLCOVParser_Parse_OnlyEmptyTrailingRecord(t *testing.T) {
	// A lone SF: record with no DA: data and nothing else -- flushed by the
	// trailing-record path since there's no end_of_record -- leaves zero
	// files in the report, same as TestLCOVParser_Parse_Empty, and must
	// fail with ErrNoCoverageData rather than succeed with an empty report.
	lcov := `SF:/src/empty.js`
	p := &LCOVParser{}
	report, err := p.Parse(strings.NewReader(lcov))
	if !errors.Is(err, ErrNoCoverageData) {
		t.Fatalf("Parse() error = %v, want ErrNoCoverageData", err)
	}
	if report != nil {
		t.Errorf("got report = %v, want nil", report)
	}
}
