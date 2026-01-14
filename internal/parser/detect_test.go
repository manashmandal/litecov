package parser

import (
	"os"
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

func TestGetParser(t *testing.T) {
	tests := []struct {
		format  string
		wantNil bool
		wantErr bool
	}{
		{"lcov", false, false},
		{"cobertura", false, false},
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
