package parser

import (
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

func TestLCOVParser_Parse_Empty(t *testing.T) {
	p := &LCOVParser{}
	report, err := p.Parse(strings.NewReader(""))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(report.Files) != 0 {
		t.Errorf("got %d files, want 0", len(report.Files))
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

func TestLCOVParser_Parse_LF_LH(t *testing.T) {
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
	if report.Files[0].LinesTotal != 10 {
		t.Errorf("LinesTotal = %v, want 10 (from LF)", report.Files[0].LinesTotal)
	}
	if report.Files[0].LinesCovered != 5 {
		t.Errorf("LinesCovered = %v, want 5 (from LH)", report.Files[0].LinesCovered)
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
