package parser

import (
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
