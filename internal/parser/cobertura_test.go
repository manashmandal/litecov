package parser

import (
	"errors"
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

func TestCoberturaParser_Parse_CoveredLines(t *testing.T) {
	// issue #6: CoveredLines has to carry the actual hit line numbers, not
	// just feed LinesCovered's count, since patch coverage intersects a PR
	// diff's added lines against it. report.Files's order isn't guaranteed
	// (Parse walks a map), so this looks the file up by path instead of
	// indexing.
	xml := `<?xml version="1.0"?>
<coverage><packages><package name="src"><classes>
<class name="app" filename="src/app.go"><lines>
<line number="1" hits="1"/>
<line number="2" hits="0"/>
<line number="3" hits="1"/>
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
	f := report.Files[0]
	if len(f.CoveredLines) != 2 || f.CoveredLines[0] != 1 || f.CoveredLines[1] != 3 {
		t.Errorf("CoveredLines = %v, want [1 3]", f.CoveredLines)
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

func TestCoberturaParser_Parse_DuplicateLineAcrossClassesMergesToHit(t *testing.T) {
	// Mirrors the repro in issue #26: com/example/Outer.java compiles to two
	// <class> elements (an outer class and an inner class), a normal shape
	// for JVM sources, and both report line 1 and line 5. The class walked
	// first records them as misses; the second records them as hits. A hit
	// from either class must win instead of the first class walked
	// shadowing the rest.
	xml := `<?xml version="1.0"?>
<coverage><packages><package name="com.example"><classes>
<class name="com.example.Outer$Inner" filename="com/example/Outer.java"><lines>
<line number="1" hits="0"/>
<line number="5" hits="0"/>
<line number="12" hits="3"/>
</lines></class>
<class name="com.example.Outer" filename="com/example/Outer.java"><lines>
<line number="1" hits="9"/>
<line number="5" hits="9"/>
<line number="20" hits="1"/>
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
	f := report.Files[0]
	if f.LinesTotal != 4 {
		t.Errorf("LinesTotal = %v, want 4 (lines 1, 5, 12, 20; lines 1 and 5 must not double count)", f.LinesTotal)
	}
	if f.LinesCovered != 4 {
		t.Errorf("LinesCovered = %v, want 4 (lines 1 and 5 were hit by the outer class)", f.LinesCovered)
	}
	if len(f.UncoveredLines) != 0 {
		t.Errorf("UncoveredLines = %v, want none", f.UncoveredLines)
	}
	// issue #6: CoveredLines has to reflect the same merged hit/miss OR as
	// LinesCovered above, not just its count -- patch coverage intersects a
	// PR diff's added lines against it.
	if len(f.CoveredLines) != 4 || f.CoveredLines[0] != 1 || f.CoveredLines[1] != 5 ||
		f.CoveredLines[2] != 12 || f.CoveredLines[3] != 20 {
		t.Errorf("CoveredLines = %v, want [1 5 12 20]", f.CoveredLines)
	}
}

func TestCoberturaParser_Parse_DuplicateLineAcrossClassesKeepsGenuineMiss(t *testing.T) {
	// A line missed by every <class> that reports it must stay uncovered
	// after merging; only a line hit by at least one class flips to
	// covered. Also checks that merge order doesn't matter: the hit is seen
	// first here and the miss second, the reverse of the case above.
	xml := `<?xml version="1.0"?>
<coverage><packages><package name="p"><classes>
<class name="A" filename="shared.cs"><lines>
<line number="2" hits="4"/>
</lines></class>
<class name="B" filename="shared.cs"><lines>
<line number="2" hits="0"/>
<line number="3" hits="0"/>
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
	f := report.Files[0]
	if f.LinesTotal != 2 {
		t.Errorf("LinesTotal = %v, want 2 (line 2 must not be counted twice)", f.LinesTotal)
	}
	if f.LinesCovered != 1 {
		t.Errorf("LinesCovered = %v, want 1 (line 2 was hit by class A)", f.LinesCovered)
	}
	if len(f.UncoveredLines) != 1 || f.UncoveredLines[0] != 3 {
		t.Errorf("UncoveredLines = %v, want [3]", f.UncoveredLines)
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
	// Mirrors the repro in issue #58: a report with no <class> elements
	// carries no coverage data at all, so it must fail with
	// ErrNoCoberturaCoverageData instead of coming back as a silent 0%
	// report -- the same fix TestLCOVParser_Parse_Empty already covers for
	// LCOV and TestGoProfileParser_Parse_Empty covers for Go profiles.
	tests := []struct {
		name string
		xml  string
	}{
		{
			name: "empty packages",
			xml:  `<?xml version="1.0"?><coverage><packages></packages></coverage>`,
		},
		{
			name: "bare coverage element",
			xml:  `<?xml version="1.0"?><coverage/>`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &CoberturaParser{}
			report, err := p.Parse(strings.NewReader(tt.xml))
			if !errors.Is(err, ErrNoCoberturaCoverageData) {
				t.Fatalf("Parse() error = %v, want ErrNoCoberturaCoverageData", err)
			}
			if report != nil {
				t.Errorf("got report = %v, want nil", report)
			}
		})
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

func TestCoberturaParser_Parse_EmptyClassSkipped(t *testing.T) {
	// Mirrors the repro in issue #51: coverage.py emits a <class> with an
	// empty <lines/> for a file with no statements, e.g. an empty
	// __init__.py, and reports line-rate="1" for it. That class used to
	// still produce a coverage.FileCoverage entry with LinesTotal == 0,
	// which Percentage() then renders as a false 0.00% row instead of the
	// 100% coverage.py actually reported. Codecov's own parser refuses to
	// add a file with no measurable lines to the report at all
	// (shared/reports/resources.py: "dont append empty files"), so litecov
	// should too -- the same drop TestLCOVParser_Parse_EmptyRecordSkipped
	// already covers for LCOV.
	xml := `<?xml version="1.0"?>
<coverage><packages><package name="mypkg"><classes>
<class name="__init__.py" filename="mypkg/__init__.py" line-rate="1">
  <methods/>
  <lines/>
</class>
<class name="calc.py" filename="mypkg/calc.py" line-rate="0.545">
  <lines>
    <line number="1" hits="1"/>
    <line number="2" hits="0"/>
  </lines>
</class>
</classes></package></packages></coverage>`
	p := &CoberturaParser{}
	report, err := p.Parse(strings.NewReader(xml))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(report.Files) != 1 {
		t.Fatalf("got %d files, want 1 (class with no lines must be dropped)", len(report.Files))
	}
	if report.Files[0].Path != "mypkg/calc.py" {
		t.Errorf("Files[0].Path = %q, want %q", report.Files[0].Path, "mypkg/calc.py")
	}
	if report.TotalLines != 2 {
		t.Errorf("TotalLines = %v, want 2 (the empty file must not contribute 0/0)", report.TotalLines)
	}
}

func TestCoberturaParser_Parse_AllLinesRejectedSkipped(t *testing.T) {
	// A class whose only <line> entries are all rejected as malformed --
	// same as TestCoberturaParser_Parse_ZeroAndMissingLineNumbersDoNotCollide
	// -- also finalizes with LinesTotal == 0 and must be dropped the same
	// way as a class with an empty <lines/> altogether.
	xml := `<?xml version="1.0"?>
<coverage><packages><package name="pkg"><classes>
<class name="Broken" filename="broken.py"><lines>
<line number="0" hits="1"/>
<line hits="1"/>
</lines></class>
<class name="Real" filename="real.py"><lines>
<line number="1" hits="1"/>
</lines></class>
</classes></package></packages></coverage>`
	p := &CoberturaParser{}
	report, err := p.Parse(strings.NewReader(xml))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(report.Files) != 1 {
		t.Fatalf("got %d files, want 1 (class with only rejected lines must be dropped)", len(report.Files))
	}
	if report.Files[0].Path != "real.py" {
		t.Errorf("Files[0].Path = %q, want %q", report.Files[0].Path, "real.py")
	}
}

func TestCoberturaParser_Parse_SourcePrefixResolution(t *testing.T) {
	// Mirrors issue #50: resolveFilename stripped a <source> prefix with a
	// raw strings.HasPrefix and no path-boundary check, and returned on the
	// first matching source rather than the longest (most specific) one.
	tests := []struct {
		name     string
		sources  []string
		filename string
		want     string
	}{
		{
			name:     "prefix must match on a path boundary, not mid-segment",
			sources:  []string{"/build/src"},
			filename: "/build/srcgen/Generated.cs",
			// "/build/srcgen" is a sibling directory, not something under
			// "/build/src". The raw HasPrefix check used to strip "/build/src"
			// off anyway, leaving the nonsense "gen/Generated.cs". With no
			// real match, the filename comes back unresolved.
			want: "/build/srcgen/Generated.cs",
		},
		{
			name:     "longest matching source wins over a shallower one",
			sources:  []string{"/build/src", "/build/src/MyApp"},
			filename: "/build/src/MyApp/Real.cs",
			// Both sources match. The deeper, more specific one is the one
			// that actually produced this file and must be the one
			// stripped, leaving "Real.cs" rather than "MyApp/Real.cs".
			want: "Real.cs",
		},
		{
			name:     "source list order must not affect which one wins",
			sources:  []string{"/build/src/MyApp", "/build/src"},
			filename: "/build/src/MyApp/Real.cs",
			want:     "Real.cs",
		},
		{
			name:     "single boundary match still resolves normally",
			sources:  []string{"/build/src"},
			filename: "/build/src/MyApp/Real.cs",
			want:     "MyApp/Real.cs",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var sourcesXML strings.Builder
			for _, s := range tt.sources {
				sourcesXML.WriteString("<source>" + s + "</source>")
			}
			xmlDoc := `<?xml version="1.0"?>
<coverage><sources>` + sourcesXML.String() + `</sources><packages><package name="p"><classes>
<class name="A" filename="` + tt.filename + `"><lines><line number="1" hits="1"/></lines></class>
</classes></package></packages></coverage>`

			p := &CoberturaParser{}
			report, err := p.Parse(strings.NewReader(xmlDoc))
			if err != nil {
				t.Fatalf("Parse() error = %v", err)
			}
			if len(report.Files) != 1 {
				t.Fatalf("got %d files, want 1", len(report.Files))
			}
			if report.Files[0].Path != tt.want {
				t.Errorf("Path = %q, want %q", report.Files[0].Path, tt.want)
			}
		})
	}
}

func TestCoberturaParser_Parse_NestedSourcesRepro(t *testing.T) {
	// The exact repro from issue #50: two <source> roots, one nested inside
	// the other, and two classes -- one under a same-named sibling directory
	// that must not resolve at all, one under the nested root that must
	// resolve against the deeper, more specific source.
	xml := `<?xml version="1.0"?>
<coverage>
  <sources>
    <source>/build/src</source>
    <source>/build/src/MyApp</source>
  </sources>
  <packages><package name="p"><classes>
    <class name="A" filename="/build/srcgen/Generated.cs"><lines><line number="1" hits="1"/></lines></class>
    <class name="B" filename="/build/src/MyApp/Real.cs"><lines><line number="1" hits="1"/></lines></class>
  </classes></package></packages>
</coverage>`
	p := &CoberturaParser{}
	report, err := p.Parse(strings.NewReader(xml))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(report.Files) != 2 {
		t.Fatalf("got %d files, want 2", len(report.Files))
	}
	got := map[string]bool{}
	for _, f := range report.Files {
		got[f.Path] = true
	}
	if !got["/build/srcgen/Generated.cs"] {
		t.Errorf("files = %v, want the sibling directory left unresolved as /build/srcgen/Generated.cs", got)
	}
	if !got["Real.cs"] {
		t.Errorf("files = %v, want Real.cs resolved against the deeper /build/src/MyApp source", got)
	}
}

func TestCoberturaParser_Parse_RelativeFilenameSourceResolution(t *testing.T) {
	// Mirrors issue #41: resolveFilename prepends a source's project-relative
	// path onto a relative <class filename="...">, but the filename itself
	// never says which <source> it came from. The old code always used
	// sources[0], so a class that actually lived under a later source still
	// got the first source's prefix -- misreporting its location.
	tests := []struct {
		name     string
		sources  []string
		filename string
		want     string
	}{
		{
			name:     "single source still prepends its project path",
			sources:  []string{"/home/runner/work/repo/repo/src"},
			filename: "mypkg/calc.py",
			want:     "src/mypkg/calc.py",
		},
		{
			name:     "sources that agree on the same project path still prepend it",
			sources:  []string{"/home/runner/work/repo/repo/src", "/home/ci/build/repo/repo/src"},
			filename: "mypkg/calc.py",
			want:     "src/mypkg/calc.py",
		},
		{
			name: "sources that disagree leave the filename bare rather than guess wrong",
			sources: []string{
				"/home/runner/work/repo/repo/src",
				"/home/runner/work/repo/repo/lib",
			},
			filename: "otherpkg/util.py",
			// The old behavior returned "src/otherpkg/util.py" -- the first
			// source's prefix -- even for a class whose true root is the
			// second source, "lib". Nothing in a relative filename says
			// which source produced it, so guessing either one risks being
			// wrong; the bare filename at least still suffix-matches the
			// real changed-file path downstream (internal/paths).
			want: "otherpkg/util.py",
		},
		{
			name:     "blank sources leave the filename bare",
			sources:  []string{"   ", ""},
			filename: "mypkg/calc.py",
			want:     "mypkg/calc.py",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var sourcesXML strings.Builder
			for _, s := range tt.sources {
				sourcesXML.WriteString("<source>" + s + "</source>")
			}
			xmlDoc := `<?xml version="1.0"?>
<coverage><sources>` + sourcesXML.String() + `</sources><packages><package name="p"><classes>
<class name="A" filename="` + tt.filename + `"><lines><line number="1" hits="1"/></lines></class>
</classes></package></packages></coverage>`

			p := &CoberturaParser{}
			report, err := p.Parse(strings.NewReader(xmlDoc))
			if err != nil {
				t.Fatalf("Parse() error = %v", err)
			}
			if len(report.Files) != 1 {
				t.Fatalf("got %d files, want 1", len(report.Files))
			}
			if report.Files[0].Path != tt.want {
				t.Errorf("Path = %q, want %q", report.Files[0].Path, tt.want)
			}
		})
	}
}

func TestCoberturaParser_Parse_MultipleSourcesRepro(t *testing.T) {
	// The exact repro from issue #41: two <source> roots and two classes
	// with relative filenames, one truly under each root. resolveFilename
	// can't tell which class came from which source, so when the sources
	// disagree neither class gets a guessed prefix -- not just the one
	// (otherpkg/util.py) that would have been wrong. That's a change from
	// the old output, which reported "src/mypkg/calc.py" (right, by luck)
	// and "src/otherpkg/util.py" (wrong: its real path is
	// lib/otherpkg/util.py). Guessing per-source is what Codecov's
	// cobertura processor does, by checking which candidate names a real
	// file in the commit; litecov's parser has no access to that here, so
	// leaving both bare is the safe option -- it still lets the suffix
	// matcher in internal/paths recover the correct changed-file path for
	// each one instead of silently dropping the misattributed file.
	xml := `<coverage version="7.10.7" lines-valid="4" lines-covered="3" line-rate="0.75">
  <sources>
    <source>/home/runner/work/repo/repo/src</source>
    <source>/home/runner/work/repo/repo/lib</source>
  </sources>
  <packages>
    <package name="mypkg"><classes>
      <class name="calc.py" filename="mypkg/calc.py"><methods/>
        <lines><line number="1" hits="1"/><line number="2" hits="0"/></lines>
      </class>
    </classes></package>
    <package name="otherpkg"><classes>
      <class name="util.py" filename="otherpkg/util.py"><methods/>
        <lines><line number="1" hits="1"/><line number="2" hits="1"/></lines>
      </class>
    </classes></package>
  </packages>
</coverage>`
	p := &CoberturaParser{}
	report, err := p.Parse(strings.NewReader(xml))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(report.Files) != 2 {
		t.Fatalf("got %d files, want 2", len(report.Files))
	}
	got := map[string]bool{}
	for _, f := range report.Files {
		got[f.Path] = true
	}
	if got["src/otherpkg/util.py"] {
		t.Errorf("files = %v, util.py must not be misreported under the src root it doesn't belong to", got)
	}
	if !got["otherpkg/util.py"] || !got["mypkg/calc.py"] {
		t.Errorf("files = %v, want both classes left with their bare relative filenames", got)
	}
}

func TestCoberturaParser_Parse_SourceProjectPathDoesNotInventDirectories(t *testing.T) {
	// Mirrors the repro table in issue #40: extractProjectPath used to guess
	// a repo-relative prefix for a source directory from a list of
	// substring markers, a list of known directory basenames, and a
	// repeated-path-segment scan that fired on any repeated segment
	// anywhere in the source instead of only the GitHub Actions
	// work/{repo}/{repo} pair it was written for. Each source below is a
	// real value one of those three guesses got wrong.
	tests := []struct {
		name     string
		source   string
		filename string
	}{
		{
			// Docker WORKDIR /app, repo mounted at the root. The knownDirs
			// check matched the "app" basename and prepended it, producing
			// "app/mypkg/calc.py" -- a path that doesn't exist in the repo.
			name:     "Docker WORKDIR basename is not a repo subdirectory",
			source:   "/app",
			filename: "mypkg/calc.py",
		},
		{
			// Same knownDirs bug from the other direction: the workspace
			// happens to end in "tests", which used to be read as a
			// project root literally named "tests".
			name:     "CI workspace ending in a known basename",
			source:   "/home/jenkins/agent/workspace/build/tests",
			filename: "pkg/a.py",
		},
		{
			// "ci/ci" is a repeated segment, but nothing here is the GitHub
			// Actions work/{repo}/{repo} checkout root; the old repeated-
			// segment scan didn't check what preceded the pair and returned
			// "build/myrepo" anyway.
			name:     "repeated segment not preceded by work is not a GHA workspace",
			source:   "/home/ci/ci/build/myrepo",
			filename: "pkg/a.py",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			xmlDoc := `<?xml version="1.0"?>
<coverage><sources><source>` + tt.source + `</source></sources><packages><package name="p"><classes>
<class name="A" filename="` + tt.filename + `"><lines><line number="1" hits="1"/></lines></class>
</classes></package></packages></coverage>`

			p := &CoberturaParser{}
			report, err := p.Parse(strings.NewReader(xmlDoc))
			if err != nil {
				t.Fatalf("Parse() error = %v", err)
			}
			if len(report.Files) != 1 {
				t.Fatalf("got %d files, want 1", len(report.Files))
			}
			// None of these sources resolve to a real prefix, so the
			// correct repo-relative path is the filename unchanged.
			if report.Files[0].Path != tt.filename {
				t.Errorf("Path = %q, want %q (source must not invent a directory prefix)", report.Files[0].Path, tt.filename)
			}
		})
	}
}

func TestCoberturaParser_Parse_ConditionCoverageBranches(t *testing.T) {
	// Mirrors issue #25: coberturaLine only decoded number and hits, so a
	// line whose branches were partially taken read the same as a fully
	// covered one. Codecov's Cobertura processor treats a branch="true"
	// line with a condition-coverage ratio as branch coverage, not a hit
	// count (services/report/languages/cobertura.py) -- a line is only as
	// covered as that ratio, the same way lineHit already demotes an LCOV
	// BRDA: line with an untaken branch (issue #63).
	tests := []struct {
		name        string
		xml         string
		wantTotal   int
		wantCovered int
		wantUncov   []int
	}{
		{
			name: "one of two branches taken demotes a hit line to uncovered",
			xml: `<?xml version="1.0"?>
<coverage><packages><package name="mypkg"><classes>
<class name="calc.py" filename="mypkg/calc.py"><lines>
<line number="6" hits="1" branch="true" condition-coverage="50% (1/2)"/>
</lines></class>
</classes></package></packages></coverage>`,
			wantTotal:   1,
			wantCovered: 0,
			wantUncov:   []int{6},
		},
		{
			name: "zero of two branches taken is already uncovered via hits",
			xml: `<?xml version="1.0"?>
<coverage><packages><package name="mypkg"><classes>
<class name="calc.py" filename="mypkg/calc.py"><lines>
<line number="13" hits="0" branch="true" condition-coverage="0% (0/2)"/>
</lines></class>
</classes></package></packages></coverage>`,
			wantTotal:   1,
			wantCovered: 0,
			wantUncov:   []int{13},
		},
		{
			name: "all branches taken still counts as a clean hit",
			xml: `<?xml version="1.0"?>
<coverage><packages><package name="mypkg"><classes>
<class name="calc.py" filename="mypkg/calc.py"><lines>
<line number="9" hits="3" branch="true" condition-coverage="100% (2/2)"/>
</lines></class>
</classes></package></packages></coverage>`,
			wantTotal:   1,
			wantCovered: 1,
			wantUncov:   nil,
		},
		{
			name: "branch false ignores condition-coverage and uses hits",
			xml: `<?xml version="1.0"?>
<coverage><packages><package name="mypkg"><classes>
<class name="calc.py" filename="mypkg/calc.py"><lines>
<line number="1" hits="1" branch="false" condition-coverage="50% (1/2)"/>
</lines></class>
</classes></package></packages></coverage>`,
			wantTotal:   1,
			wantCovered: 1,
			wantUncov:   nil,
		},
		{
			name: "branch true with malformed condition-coverage falls back to hits",
			xml: `<?xml version="1.0"?>
<coverage><packages><package name="mypkg"><classes>
<class name="calc.py" filename="mypkg/calc.py"><lines>
<line number="1" hits="1" branch="true" condition-coverage="n/a"/>
</lines></class>
</classes></package></packages></coverage>`,
			wantTotal:   1,
			wantCovered: 1,
			wantUncov:   nil,
		},
		{
			name: "branch attribute is case-insensitive like Codecov's own check",
			xml: `<?xml version="1.0"?>
<coverage><packages><package name="mypkg"><classes>
<class name="calc.py" filename="mypkg/calc.py"><lines>
<line number="6" hits="1" branch="True" condition-coverage="50% (1/2)"/>
</lines></class>
</classes></package></packages></coverage>`,
			wantTotal:   1,
			wantCovered: 0,
			wantUncov:   []int{6},
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
			if len(fc.UncoveredLines) != len(tt.wantUncov) {
				t.Errorf("UncoveredLines = %v, want %v", fc.UncoveredLines, tt.wantUncov)
			}
		})
	}
}

func TestCoberturaParser_Parse_ConditionCoveragePartialRepro(t *testing.T) {
	// The exact repro from issue #25: coverage.py 7.10.7 output for a module
	// with one taken and one untaken branch on line 6, and a fully missed
	// branch on line 13. Before the fix this reported 6/11 (54.55%) with
	// line 6 counted as a full hit even though only one of its two branches
	// ran; the 1-of-4 branch coverage on the input showed up nowhere.
	xml := `<?xml version="1.0"?>
<coverage version="7.10.7" lines-valid="11" lines-covered="6" line-rate="0.5455"
          branches-valid="4" branches-covered="1" branch-rate="0.25">
  <packages><package name="mypkg"><classes>
    <class name="calc.py" filename="mypkg/calc.py"><lines>
      <line number="1" hits="1"/>
      <line number="2" hits="1"/>
      <line number="3" hits="1"/>
      <line number="4" hits="1"/>
      <line number="5" hits="1"/>
      <line number="6" hits="1" branch="true" condition-coverage="50% (1/2)" missing-branches="7"/>
      <line number="7" hits="0"/>
      <line number="12" hits="0"/>
      <line number="13" hits="0" branch="true" condition-coverage="0% (0/2)" missing-branches="14,15"/>
      <line number="14" hits="0"/>
      <line number="15" hits="0"/>
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
	if fc.LinesTotal != 11 {
		t.Errorf("LinesTotal = %v, want 11", fc.LinesTotal)
	}
	if fc.LinesCovered != 5 {
		t.Errorf("LinesCovered = %v, want 5 (line 6 no longer counts as a full hit)", fc.LinesCovered)
	}
	wantUncovered := []int{6, 7, 12, 13, 14, 15}
	if len(fc.UncoveredLines) != len(wantUncovered) {
		t.Fatalf("UncoveredLines = %v, want %v", fc.UncoveredLines, wantUncovered)
	}
	for i, n := range wantUncovered {
		if fc.UncoveredLines[i] != n {
			t.Errorf("UncoveredLines = %v, want %v", fc.UncoveredLines, wantUncovered)
			break
		}
	}
	if report.TotalCovered != 5 || report.TotalLines != 11 {
		t.Errorf("TotalCovered/TotalLines = %v/%v, want 5/11", report.TotalCovered, report.TotalLines)
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
