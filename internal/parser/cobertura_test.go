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

	if report.TotalCovered != 4 {
		t.Errorf("TotalCovered = %v, want 4", report.TotalCovered)
	}
	if report.TotalLines != 6 {
		t.Errorf("TotalLines = %v, want 6", report.TotalLines)
	}
}

func TestCoberturaParser_Parse_DuplicateFiles(t *testing.T) {
	xml := `<?xml version="1.0"?>
<coverage>
  <packages>
    <package name="pkg">
      <classes>
        <class name="Class1" filename="shared.go">
          <lines>
            <line number="1" hits="1"/>
            <line number="2" hits="0"/>
          </lines>
        </class>
        <class name="Class2" filename="shared.go">
          <lines>
            <line number="3" hits="1"/>
            <line number="4" hits="1"/>
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
		t.Errorf("got %d files, want 1 (should merge duplicates)", len(report.Files))
	}
	if report.Files[0].LinesTotal != 4 {
		t.Errorf("LinesTotal = %v, want 4", report.Files[0].LinesTotal)
	}
	if report.Files[0].LinesCovered != 3 {
		t.Errorf("LinesCovered = %v, want 3", report.Files[0].LinesCovered)
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
	if len(report.Files[0].UncoveredLines) != 2 {
		t.Errorf("UncoveredLines = %v, want [1, 2]", report.Files[0].UncoveredLines)
	}
}

func TestCoberturaParser_Parse_MalformedAttributes(t *testing.T) {
	// Mirrors the repros in issue #57: coberturaLine used to type number and
	// hits as Go int, but the Cobertura DTD types both as CDATA. encoding/xml
	// aborts the whole Decode call the moment one <line> attribute anywhere
	// in the report fails to parse as int, so a float hits, a non-numeric
	// number, or a hits value too large for an int used to fail the entire
	// report instead of just that line.
	tests := []struct {
		name        string
		xml         string
		wantTotal   int
		wantCovered int
	}{
		{
			name: "float hits counts as covered",
			xml: `<?xml version="1.0"?>
<coverage><packages><package name="pkg"><classes>
<class name="Test" filename="a.py"><lines>
<line number="1" hits="1.0"/>
</lines></class>
</classes></package></packages></coverage>`,
			wantTotal:   1,
			wantCovered: 1,
		},
		{
			name: "undefined line number is skipped, not fatal",
			xml: `<?xml version="1.0"?>
<coverage><packages><package name="pkg"><classes>
<class name="Test" filename="a.py"><lines>
<line number="undefined" hits="1"/>
<line number="2" hits="1"/>
</lines></class>
</classes></package></packages></coverage>`,
			wantTotal:   1,
			wantCovered: 1,
		},
		{
			name: "hits too large for an int still counts as covered",
			xml: `<?xml version="1.0"?>
<coverage><packages><package name="pkg"><classes>
<class name="Test" filename="a.py"><lines>
<line number="1" hits="99999999999999999999"/>
</lines></class>
</classes></package></packages></coverage>`,
			wantTotal:   1,
			wantCovered: 1,
		},
		{
			name: "empty hits is an explicit miss, not fatal",
			xml: `<?xml version="1.0"?>
<coverage><packages><package name="pkg"><classes>
<class name="Test" filename="a.py"><lines>
<line number="1" hits=""/>
</lines></class>
</classes></package></packages></coverage>`,
			wantTotal:   1,
			wantCovered: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &CoberturaParser{}
			report, err := p.Parse(strings.NewReader(tt.xml))
			if err != nil {
				t.Fatalf("Parse() error = %v", err)
			}
			if len(report.Files) != 1 {
				t.Fatalf("got %d files, want 1", len(report.Files))
			}
			fc := report.Files[0]
			if fc.LinesTotal != tt.wantTotal {
				t.Errorf("LinesTotal = %v, want %v", fc.LinesTotal, tt.wantTotal)
			}
			if fc.LinesCovered != tt.wantCovered {
				t.Errorf("LinesCovered = %v, want %v", fc.LinesCovered, tt.wantCovered)
			}
		})
	}
}

func TestCoberturaParser_Parse_ZeroAndMissingLineNumbersDoNotCollide(t *testing.T) {
	// Mirrors the line-0 collision in issue #57: a <line number="0"/> and a
	// <line> with no number attribute at all both used to decode to line 0
	// (Go's int zero value), so the dedup in Parse treated the second as a
	// repeat of the first and silently dropped it -- and the survivor was
	// L0, which isn't a real line, feeding a bogus entry into
	// UncoveredLines and from there into comment links and
	// "::warning file=...,line=0" annotations. Neither names a real line
	// (Cobertura lines are 1-indexed), so both must be skipped instead.
	xml := `<?xml version="1.0"?>
<coverage><packages><package name="pkg"><classes>
<class name="Test" filename="a.py"><lines>
<line number="0" hits="0"/>
<line hits="0"/>
<line number="5" hits="1"/>
</lines></class>
</classes></package></packages></coverage>`
	p := &CoberturaParser{}
	report, err := p.Parse(strings.NewReader(xml))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(report.Files) != 1 {
		t.Fatalf("got %d files, want 1", len(report.Files))
	}
	fc := report.Files[0]
	if fc.LinesTotal != 1 {
		t.Errorf("LinesTotal = %v, want 1 (line 0 entries must not be counted)", fc.LinesTotal)
	}
	if fc.LinesCovered != 1 {
		t.Errorf("LinesCovered = %v, want 1", fc.LinesCovered)
	}
	for _, n := range fc.UncoveredLines {
		if n == 0 {
			t.Errorf("UncoveredLines = %v, must not contain line 0", fc.UncoveredLines)
		}
	}
}

func TestCoberturaParser_Parse_ClassWithoutFilenameSkipped(t *testing.T) {
	// Mirrors issue #64: Parse used class.Filename as the fileMap key without
	// checking it was set. A <class> with no filename attribute, or with
	// filename="", decoded to Path "", so every such class in the report
	// merged into one bogus entry instead of being dropped. Codecov's own
	// cobertura parser skips these classes outright, so litecov should too.
	tests := []struct {
		name string
		xml  string
	}{
		{
			name: "filename attribute absent",
			xml: `<?xml version="1.0"?>
<coverage><packages><package name="p"><classes>
<class name="A"><lines><line number="1" hits="1"/></lines></class>
<class name="C" filename="real.py"><lines><line number="1" hits="1"/></lines></class>
</classes></package></packages></coverage>`,
		},
		{
			name: "filename attribute empty",
			xml: `<?xml version="1.0"?>
<coverage><packages><package name="p"><classes>
<class name="A" filename=""><lines><line number="1" hits="1"/></lines></class>
<class name="C" filename="real.py"><lines><line number="1" hits="1"/></lines></class>
</classes></package></packages></coverage>`,
		},
		{
			name: "filename attribute whitespace-only",
			xml: `<?xml version="1.0"?>
<coverage><packages><package name="p"><classes>
<class name="A" filename="   "><lines><line number="1" hits="1"/></lines></class>
<class name="C" filename="real.py"><lines><line number="1" hits="1"/></lines></class>
</classes></package></packages></coverage>`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &CoberturaParser{}
			report, err := p.Parse(strings.NewReader(tt.xml))
			if err != nil {
				t.Fatalf("Parse() error = %v", err)
			}
			if len(report.Files) != 1 {
				t.Fatalf("got %d files, want 1 (class with no filename must be skipped)", len(report.Files))
			}
			if report.Files[0].Path != "real.py" {
				t.Errorf("Path = %q, want %q", report.Files[0].Path, "real.py")
			}
			if report.Files[0].LinesTotal != 1 {
				t.Errorf("LinesTotal = %v, want 1", report.Files[0].LinesTotal)
			}
		})
	}
}

func TestCoberturaParser_Parse_UncoveredLines(t *testing.T) {
	xml := `<?xml version="1.0"?>
<coverage>
  <packages>
    <package name="pkg">
      <classes>
        <class name="Test" filename="test.go">
          <lines>
            <line number="1" hits="1"/>
            <line number="2" hits="0"/>
            <line number="3" hits="1"/>
            <line number="4" hits="0"/>
            <line number="5" hits="0"/>
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
	want := []int{2, 4, 5}
	if len(report.Files[0].UncoveredLines) != len(want) {
		t.Errorf("UncoveredLines = %v, want %v", report.Files[0].UncoveredLines, want)
	}
}
