package parser

import (
	"os"
	"strings"
	"testing"
)

func TestCoberturaParser_Parse(t *testing.T) {
	f, err := os.Open("../../testdata/simple.xml")
	if err != nil {
		t.Fatalf("failed to open test file: %v", err)
	}
	defer f.Close()

	p := &CoberturaParser{}
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

	if report.TotalCovered != 4 {
		t.Errorf("TotalCovered = %v, want 4", report.TotalCovered)
	}
	if report.TotalLines != 6 {
		t.Errorf("TotalLines = %v, want 6", report.TotalLines)
	}
}

func TestCoberturaParser_Parse_InvalidXML(t *testing.T) {
	p := &CoberturaParser{}
	_, err := p.Parse(strings.NewReader("not valid xml"))
	if err == nil {
		t.Error("expected error for invalid XML")
	}
}

func TestCoberturaParser_Parse_Empty(t *testing.T) {
	xml := `<?xml version="1.0"?><coverage><packages></packages></coverage>`
	p := &CoberturaParser{}
	report, err := p.Parse(strings.NewReader(xml))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(report.Files) != 0 {
		t.Errorf("got %d files, want 0", len(report.Files))
	}
}

func TestCoberturaParser_Parse_ZeroHits(t *testing.T) {
	xml := `<?xml version="1.0"?>
<coverage>
  <packages>
    <package name="pkg">
      <classes>
        <class name="Test" filename="test.go">
          <lines>
            <line number="1" hits="0"/>
            <line number="2" hits="0"/>
          </lines>
        </class>
      </classes>
    </package>
  </packages>
</coverage>`
	p := &CoberturaParser{}
	report, err := p.Parse(strings.NewReader(xml))
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
