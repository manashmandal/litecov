package parser

import (
	"errors"
	"os"
	"strings"
	"testing"
)

func TestDetectFormat(t *testing.T) {
	tests := []struct {
		name     string
		file     string
		wantType string
	}{
		{"lcov file", "../../testdata/simple.lcov", "lcov"},
		{"cobertura file", "../../testdata/simple.xml", "cobertura"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, err := os.Open(tt.file)
			if err != nil {
				t.Fatalf("failed to open file: %v", err)
			}
			defer f.Close()

			format, err := DetectFormat(f)
			if err != nil {
				t.Fatalf("DetectFormat() error = %v", err)
			}
			if format != tt.wantType {
				t.Errorf("DetectFormat() = %v, want %v", format, tt.wantType)
			}
		})
	}
}

func TestDetectFormat_Unknown(t *testing.T) {
	r := strings.NewReader("random text that is not lcov or xml")
	_, err := DetectFormat(r)
	if err != ErrUnknownFormat {
		t.Errorf("DetectFormat() error = %v, want ErrUnknownFormat", err)
	}
}

func TestDetectFormat_EndOfRecord(t *testing.T) {
	r := strings.NewReader("some stuff\nend_of_record\nmore stuff")
	format, err := DetectFormat(r)
	if err != nil {
		t.Fatalf("DetectFormat() error = %v", err)
	}
	if format != "lcov" {
		t.Errorf("DetectFormat() = %v, want lcov", format)
	}
}

func TestDetectFormat_XMLDeclaration(t *testing.T) {
	r := strings.NewReader("<?xml version=\"1.0\"?><coverage/>")
	format, err := DetectFormat(r)
	if err != nil {
		t.Fatalf("DetectFormat() error = %v", err)
	}
	if format != "cobertura" {
		t.Errorf("DetectFormat() = %v, want cobertura", format)
	}
}

func TestDetectFormat_SFSubstringNotAtLineStart(t *testing.T) {
	// Mirrors the first repro in issue #70: "SF:" appearing anywhere in the
	// scanned content used to be enough to classify arbitrary text as LCOV,
	// even though no line actually starts with it.
	r := strings.NewReader("Coverage summary: see SF:/build/report for details.\n")
	_, err := DetectFormat(r)
	if err != ErrUnknownFormat {
		t.Errorf("DetectFormat() error = %v, want ErrUnknownFormat", err)
	}
}

func TestDetectFormat_MarkerPastOldByteLimit(t *testing.T) {
	// Mirrors the second repro in issue #70: a tracefile whose first SF:
	// record starts past the old 1024-byte read window went undetected
	// even though it's a valid LCOV file.
	preamble := "TN:" + strings.Repeat("x", 1100)
	r := strings.NewReader(preamble + "\nSF:/src/a.js\nDA:1,1\nend_of_record\n")
	format, err := DetectFormat(r)
	if err != nil {
		t.Fatalf("DetectFormat() error = %v", err)
	}
	if format != "lcov" {
		t.Errorf("DetectFormat() = %v, want lcov", format)
	}
}

func TestDetectFormat_XMLDeclarationAloneIsNotCobertura(t *testing.T) {
	// Mirrors the root cause in issue #71: matching on a bare "<?xml"
	// declaration used to classify any XML dialect as Cobertura, since
	// every well-formed XML document opens with one regardless of its
	// root element. With no "<coverage" or "<report" line behind it, this
	// must fall through to ErrUnknownFormat instead of "cobertura".
	r := strings.NewReader("<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n<unknown-root/>\n")
	_, err := DetectFormat(r)
	if err != ErrUnknownFormat {
		t.Errorf("DetectFormat() error = %v, want ErrUnknownFormat", err)
	}
}

func TestDetectFormat_CloverXML(t *testing.T) {
	// Mirrors the first repro in issue #71: Clover XML (PHPUnit, and the
	// clover reporter in Jest/Vitest) uses "<coverage>" as its root
	// element too, so it used to be classified "cobertura" and silently
	// parse into a 0.00% report instead of failing with a clear error.
	r := strings.NewReader(`<?xml version="1.0" encoding="UTF-8"?>
<coverage generated="1700000000" clover="3.2.0">
  <project timestamp="1700000000" name="All files">
    <file name="/src/Calculator.php">
      <line num="10" type="stmt" count="1"/>
      <line num="11" type="stmt" count="0"/>
      <metrics loc="20" statements="2" coveredstatements="1"/>
    </file>
  </project>
</coverage>
`)
	_, err := DetectFormat(r)
	if !errors.Is(err, ErrUnsupportedFormat) {
		t.Errorf("DetectFormat() error = %v, want ErrUnsupportedFormat", err)
	}
}

func TestDetectFormat_JaCoCoXML(t *testing.T) {
	// Mirrors the second repro in issue #71: JaCoCo's native XML root
	// element is "<report>", not "<coverage>". It used to be classified
	// "cobertura" (via the bare "<?xml" match) and fail deep inside
	// CoberturaParser with a raw "expected element type <coverage> but
	// have <report>" error instead of a format-detection error naming
	// what was actually found.
	r := strings.NewReader(`<?xml version="1.0" encoding="UTF-8"?>
<report name="app"><package name="com/x"><sourcefile name="Foo.java">
<line nr="3" mi="0" ci="2" mb="0" cb="0"/><line nr="4" mi="2" ci="0" mb="0" cb="0"/>
</sourcefile></package></report>
`)
	_, err := DetectFormat(r)
	if !errors.Is(err, ErrUnsupportedFormat) {
		t.Errorf("DetectFormat() error = %v, want ErrUnsupportedFormat", err)
	}
}

func TestGetParser(t *testing.T) {
	tests := []struct {
		format  string
		wantNil bool
		wantErr bool
	}{
		{"lcov", false, false},
		{"cobertura", false, false},
		{"xml", false, false},
		{"auto", true, false},
		{"unknown", true, true},
	}

	for _, tt := range tests {
		t.Run(tt.format, func(t *testing.T) {
			p, err := GetParser(tt.format)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetParser(%q) error = %v, wantErr %v", tt.format, err, tt.wantErr)
			}
			if (p == nil) != tt.wantNil {
				t.Errorf("GetParser(%q) parser nil = %v, wantNil %v", tt.format, p == nil, tt.wantNil)
			}
		})
	}
}
