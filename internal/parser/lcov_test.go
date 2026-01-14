package parser

import (
	"os"
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

	// Check first file
	if report.Files[0].Path != "/src/parser.go" {
		t.Errorf("Files[0].Path = %v, want /src/parser.go", report.Files[0].Path)
	}
	if report.Files[0].LinesCovered != 3 {
		t.Errorf("Files[0].LinesCovered = %v, want 3", report.Files[0].LinesCovered)
	}
	if report.Files[0].LinesTotal != 4 {
		t.Errorf("Files[0].LinesTotal = %v, want 4", report.Files[0].LinesTotal)
	}

	// Check second file
	if report.Files[1].Path != "/src/utils.go" {
		t.Errorf("Files[1].Path = %v, want /src/utils.go", report.Files[1].Path)
	}
	if report.Files[1].LinesCovered != 1 {
		t.Errorf("Files[1].LinesCovered = %v, want 1", report.Files[1].LinesCovered)
	}

	// Check totals
	if report.TotalCovered != 4 {
		t.Errorf("TotalCovered = %v, want 4", report.TotalCovered)
	}
	if report.TotalLines != 6 {
		t.Errorf("TotalLines = %v, want 6", report.TotalLines)
	}
}
