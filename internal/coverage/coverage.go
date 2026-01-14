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

	changedFileSet := make(map[string]bool)
	for _, f := range changedFiles {
		changedFileSet[f] = true
	}

	filterByChanged := len(changedFiles) > 0

	for _, headFile := range head.Files {
		if filterByChanged && !changedFileSet[headFile.Path] {
			continue
		}

		fc := FileChange{
			Path:         headFile.Path,
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

	return comp
}
