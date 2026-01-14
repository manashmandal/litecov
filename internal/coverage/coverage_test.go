package coverage

import "testing"

func TestFileCoverage_Percentage(t *testing.T) {
	tests := []struct {
		name     string
		covered  int
		total    int
		expected float64
	}{
		{"full coverage", 100, 100, 100.0},
		{"half coverage", 50, 100, 50.0},
		{"no coverage", 0, 100, 0.0},
		{"zero lines", 0, 0, 0.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fc := FileCoverage{
				LinesCovered: tt.covered,
				LinesTotal:   tt.total,
			}
			if got := fc.Percentage(); got != tt.expected {
				t.Errorf("Percentage() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestReport_Calculate(t *testing.T) {
	report := &Report{
		Files: []FileCoverage{
			{Path: "a.go", LinesCovered: 80, LinesTotal: 100},
			{Path: "b.go", LinesCovered: 20, LinesTotal: 100},
		},
	}
	report.Calculate()

	if report.TotalCovered != 100 {
		t.Errorf("TotalCovered = %v, want 100", report.TotalCovered)
	}
	if report.TotalLines != 200 {
		t.Errorf("TotalLines = %v, want 200", report.TotalLines)
	}
	if report.Coverage != 50.0 {
		t.Errorf("Coverage = %v, want 50.0", report.Coverage)
	}
}

func TestReport_Calculate_Empty(t *testing.T) {
	report := &Report{Files: []FileCoverage{}}
	report.Calculate()

	if report.TotalCovered != 0 {
		t.Errorf("TotalCovered = %v, want 0", report.TotalCovered)
	}
	if report.TotalLines != 0 {
		t.Errorf("TotalLines = %v, want 0", report.TotalLines)
	}
	if report.Coverage != 0 {
		t.Errorf("Coverage = %v, want 0", report.Coverage)
	}
}

func TestReport_Calculate_SingleFile(t *testing.T) {
	report := &Report{
		Files: []FileCoverage{
			{Path: "single.go", LinesCovered: 75, LinesTotal: 100},
		},
	}
	report.Calculate()

	if report.Coverage != 75.0 {
		t.Errorf("Coverage = %v, want 75.0", report.Coverage)
	}
}
