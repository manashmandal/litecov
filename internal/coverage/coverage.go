package coverage

// FileCoverage represents coverage data for a single file
type FileCoverage struct {
	Path         string
	LinesCovered int
	LinesTotal   int
}

// Percentage returns the coverage percentage (0-100)
func (fc *FileCoverage) Percentage() float64 {
	if fc.LinesTotal == 0 {
		return 0.0
	}
	return float64(fc.LinesCovered) / float64(fc.LinesTotal) * 100.0
}

// Report represents the complete coverage report
type Report struct {
	Files        []FileCoverage
	TotalCovered int
	TotalLines   int
	Coverage     float64
}

// Calculate computes totals from individual file coverage
func (r *Report) Calculate() {
	r.TotalCovered = 0
	r.TotalLines = 0
	for _, f := range r.Files {
		r.TotalCovered += f.LinesCovered
		r.TotalLines += f.LinesTotal
	}
	if r.TotalLines == 0 {
		r.Coverage = 0.0
	} else {
		r.Coverage = float64(r.TotalCovered) / float64(r.TotalLines) * 100.0
	}
}
