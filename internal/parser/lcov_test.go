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
	if len(report.Files) != 0 {
		t.Errorf("got %d files, want 0 (no end_of_record)", len(report.Files))
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
