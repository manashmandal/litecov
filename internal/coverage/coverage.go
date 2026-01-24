package coverage

type FileCoverage struct {
	Path           string
	LinesCovered   int
	LinesTotal     int
	UncoveredLines []int
	CoveredLines   []int
}

func (fc *FileCoverage) Percentage() float64 {
	if fc.LinesTotal == 0 {
		return 0
	}
	return float64(fc.LinesCovered) / float64(fc.LinesTotal) * 100
}

type Report struct {
	Files        []FileCoverage
	TotalCovered int
	TotalLines   int
	Coverage     float64
}

func (r *Report) Calculate() {
	r.TotalCovered = 0
	r.TotalLines = 0
	for _, f := range r.Files {
		r.TotalCovered += f.LinesCovered
		r.TotalLines += f.LinesTotal
	}
	if r.TotalLines == 0 {
		r.Coverage = 0
		return
	}
	r.Coverage = float64(r.TotalCovered) / float64(r.TotalLines) * 100
}

func (r *Report) Hits() int {
	return r.TotalCovered
}

func (r *Report) Misses() int {
	return r.TotalLines - r.TotalCovered
}

// Comparison holds the result of comparing head vs base coverage
type Comparison struct {
	Head          *Report
	Base          *Report
	CoverageDelta float64
	FileChanges   []FileChange
}

// FileChange represents coverage change for a single file
type FileChange struct {
	Path         string
	HeadCoverage float64
	BaseCoverage float64
	Delta        float64
	IsNew        bool
	NoCoverage   bool // True if file has no coverage data (completely untested)
}

// NewComparison creates a comparison between head and base reports
// changedFiles is optional list of file paths that changed in the PR
func NewComparison(head, base *Report, changedFiles []string) *Comparison {
	if head == nil {
		return &Comparison{}
	}

	comp := &Comparison{
		Head: head,
		Base: base,
	}

	if base != nil {
		comp.CoverageDelta = head.Coverage - base.Coverage
	}

	baseFileMap := make(map[string]*FileCoverage)
	if base != nil {
		for i := range base.Files {
			baseFileMap[base.Files[i].Path] = &base.Files[i]
		}
	}

	// Build a map of head files for quick lookup
	headFileMap := make(map[string]*FileCoverage)
	for i := range head.Files {
		headFileMap[head.Files[i].Path] = &head.Files[i]
	}

	changedFileSet := make(map[string]bool)
	for _, f := range changedFiles {
		changedFileSet[f] = true
	}

	filterByChanged := len(changedFiles) > 0

	// Track which changed files we've seen in coverage data
	coveredChangedFiles := make(map[string]bool)

	for _, headFile := range head.Files {
		matchedChangedFile := ""
		if filterByChanged {
			matchedChangedFile = findMatchingChangedFile(headFile.Path, changedFileSet)
			if matchedChangedFile == "" {
				continue
			}
			coveredChangedFiles[matchedChangedFile] = true
		}

		filePath := headFile.Path
		if matchedChangedFile != "" {
			filePath = matchedChangedFile
		}

		fc := FileChange{
			Path:         filePath,
			HeadCoverage: headFile.Percentage(),
		}

		if baseFile, exists := baseFileMap[headFile.Path]; exists {
			fc.BaseCoverage = baseFile.Percentage()
			fc.IsNew = false
		} else {
			fc.BaseCoverage = 0
			fc.IsNew = true
		}

		fc.Delta = fc.HeadCoverage - fc.BaseCoverage
		comp.FileChanges = append(comp.FileChanges, fc)
	}

	// Add changed files that are missing from coverage (0% coverage)
	if filterByChanged {
		for _, changedFile := range changedFiles {
			if coveredChangedFiles[changedFile] {
				continue
			}
			// Only include source files that should have coverage
			if !isSourceFile(changedFile) {
				continue
			}
			fc := FileChange{
				Path:         changedFile,
				HeadCoverage: 0,
				BaseCoverage: 0,
				Delta:        0,
				IsNew:        true,
				NoCoverage:   true,
			}
			// Check if file existed in base
			if baseFile := findFileInReport(base, changedFile); baseFile != nil {
				fc.BaseCoverage = baseFile.Percentage()
				fc.Delta = -fc.BaseCoverage
				fc.IsNew = false
			}
			comp.FileChanges = append(comp.FileChanges, fc)
		}
	}

	return comp
}

// findMatchingChangedFile returns the matching changed file path, or empty string if not found
func findMatchingChangedFile(coveragePath string, changedSet map[string]bool) string {
	if changedSet[coveragePath] {
		return coveragePath
	}
	// Try suffix matching for paths that may have different prefixes
	for changedPath := range changedSet {
		if hasSuffix(coveragePath, changedPath) || hasSuffix(changedPath, coveragePath) {
			return changedPath
		}
	}
	return ""
}

// hasSuffix checks if path ends with suffix (with proper path boundary)
func hasSuffix(path, suffix string) bool {
	if len(suffix) > len(path) {
		return false
	}
	if path == suffix {
		return true
	}
	// Check suffix with path boundary (/)
	if len(path) > len(suffix) && path[len(path)-len(suffix)-1] == '/' {
		return path[len(path)-len(suffix):] == suffix
	}
	return false
}

// isSourceFile checks if a file is a source file that should have coverage
func isSourceFile(path string) bool {
	// Only check Go files for now (can be extended for other languages)
	if len(path) < 3 || path[len(path)-3:] != ".go" {
		return false
	}
	// Skip test files
	if len(path) >= 8 && path[len(path)-8:] == "_test.go" {
		return false
	}
	// Skip vendor at start of path
	if len(path) >= 7 && path[0:7] == "vendor/" {
		return false
	}
	// Skip vendor, generated, mock files
	for i := 0; i < len(path); i++ {
		if i+8 <= len(path) && path[i:i+8] == "/vendor/" {
			return false
		}
		if i+9 <= len(path) && path[i:i+9] == "generated" {
			return false
		}
		if i+6 <= len(path) && path[i:i+6] == ".pb.go" {
			return false
		}
		if i+8 <= len(path) && path[i:i+8] == "_mock.go" {
			return false
		}
		if i+5 <= len(path) && path[i:i+5] == "mock_" {
			return false
		}
	}
	return true
}

// findFileInReport finds a file in a report by path suffix matching
func findFileInReport(report *Report, path string) *FileCoverage {
	if report == nil {
		return nil
	}
	for i := range report.Files {
		if report.Files[i].Path == path ||
			hasSuffix(report.Files[i].Path, path) ||
			hasSuffix(path, report.Files[i].Path) {
			return &report.Files[i]
		}
	}
	return nil
}
