# LiteCov Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Build a GitHub Action that parses coverage reports (LCOV/Cobertura) and posts PR comments with coverage statistics.

**Architecture:** Single Go binary with parsers for coverage formats, GitHub API client for posting comments and setting commit status. Runs as a composite GitHub Action with pre-built binary.

**Tech Stack:** Go 1.22+, GitHub REST API, LCOV format, Cobertura XML

---

## Task 1: Project Setup

**Files:**
- Create: `go.mod`
- Create: `go.sum`
- Create: `.gitignore`

**Step 1: Initialize Go module**

```bash
go mod init github.com/litecov/litecov
```

**Step 2: Create .gitignore**

Create `.gitignore`:
```
# Binaries
litecov
*.exe
dist/

# Test artifacts
coverage.out
coverage.lcov
coverage.xml

# IDE
.idea/
.vscode/
*.swp

# OS
.DS_Store
```

**Step 3: Commit**

```bash
git add go.mod .gitignore
git commit -m "chore: initialize Go module"
```

---

## Task 2: Coverage Data Structures

**Files:**
- Create: `internal/coverage/coverage.go`
- Create: `internal/coverage/coverage_test.go`

**Step 1: Write the test for coverage calculation**

Create `internal/coverage/coverage_test.go`:
```go
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

func TestReport_Percentage(t *testing.T) {
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
```

**Step 2: Run test to verify it fails**

```bash
go test ./internal/coverage/... -v
```
Expected: FAIL - package doesn't exist

**Step 3: Write implementation**

Create `internal/coverage/coverage.go`:
```go
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
```

**Step 4: Run test to verify it passes**

```bash
go test ./internal/coverage/... -v
```
Expected: PASS

**Step 5: Commit**

```bash
git add internal/coverage/
git commit -m "feat(coverage): add coverage data structures"
```

---

## Task 3: LCOV Parser

**Files:**
- Create: `internal/parser/parser.go`
- Create: `internal/parser/lcov.go`
- Create: `internal/parser/lcov_test.go`
- Create: `testdata/simple.lcov`

**Step 1: Create parser interface**

Create `internal/parser/parser.go`:
```go
package parser

import (
	"io"

	"github.com/litecov/litecov/internal/coverage"
)

// Parser parses coverage reports into a standard format
type Parser interface {
	Parse(r io.Reader) (*coverage.Report, error)
}
```

**Step 2: Create test data file**

Create `testdata/simple.lcov`:
```
SF:/src/parser.go
DA:1,1
DA:2,1
DA:3,0
DA:4,1
LF:4
LH:3
end_of_record
SF:/src/utils.go
DA:1,1
DA:2,0
LF:2
LH:1
end_of_record
```

**Step 3: Write LCOV parser test**

Create `internal/parser/lcov_test.go`:
```go
package parser

import (
	"os"
	"testing"
)

func TestLCOVParser_Parse(t *testing.T) {
	f, err := os.Open("../../testdata/simple.lcov")
	if err != nil {
		t.Fatalf("failed to open test file: %v", err)
	}
	defer f.Close()

	p := &LCOVParser{}
	report, err := p.Parse(f)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if len(report.Files) != 2 {
		t.Errorf("got %d files, want 2", len(report.Files))
	}

	// Check first file
	if report.Files[0].Path != "/src/parser.go" {
		t.Errorf("Files[0].Path = %v, want /src/parser.go", report.Files[0].Path)
	}
	if report.Files[0].LinesCovered != 3 {
		t.Errorf("Files[0].LinesCovered = %v, want 3", report.Files[0].LinesCovered)
	}
	if report.Files[0].LinesTotal != 4 {
		t.Errorf("Files[0].LinesTotal = %v, want 4", report.Files[0].LinesTotal)
	}

	// Check second file
	if report.Files[1].Path != "/src/utils.go" {
		t.Errorf("Files[1].Path = %v, want /src/utils.go", report.Files[1].Path)
	}
	if report.Files[1].LinesCovered != 1 {
		t.Errorf("Files[1].LinesCovered = %v, want 1", report.Files[1].LinesCovered)
	}

	// Check totals
	if report.TotalCovered != 4 {
		t.Errorf("TotalCovered = %v, want 4", report.TotalCovered)
	}
	if report.TotalLines != 6 {
		t.Errorf("TotalLines = %v, want 6", report.TotalLines)
	}
}
```

**Step 4: Run test to verify it fails**

```bash
go test ./internal/parser/... -v
```
Expected: FAIL - LCOVParser not defined

**Step 5: Write LCOV parser implementation**

Create `internal/parser/lcov.go`:
```go
package parser

import (
	"bufio"
	"io"
	"strconv"
	"strings"

	"github.com/litecov/litecov/internal/coverage"
)

// LCOVParser parses LCOV format coverage reports
type LCOVParser struct{}

// Parse reads LCOV format and returns a coverage report
func (p *LCOVParser) Parse(r io.Reader) (*coverage.Report, error) {
	report := &coverage.Report{}
	scanner := bufio.NewScanner(r)

	var current *coverage.FileCoverage

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		switch {
		case strings.HasPrefix(line, "SF:"):
			// Start of new file
			current = &coverage.FileCoverage{
				Path: strings.TrimPrefix(line, "SF:"),
			}

		case strings.HasPrefix(line, "DA:"):
			// Line data: DA:line_number,hit_count
			if current == nil {
				continue
			}
			parts := strings.Split(strings.TrimPrefix(line, "DA:"), ",")
			if len(parts) >= 2 {
				hits, _ := strconv.Atoi(parts[1])
				current.LinesTotal++
				if hits > 0 {
					current.LinesCovered++
				}
			}

		case strings.HasPrefix(line, "LF:"):
			// Lines found (total) - we calculate ourselves, but validate
			if current != nil {
				lf, _ := strconv.Atoi(strings.TrimPrefix(line, "LF:"))
				if lf > 0 && current.LinesTotal != lf {
					current.LinesTotal = lf
				}
			}

		case strings.HasPrefix(line, "LH:"):
			// Lines hit (covered) - we calculate ourselves, but validate
			if current != nil {
				lh, _ := strconv.Atoi(strings.TrimPrefix(line, "LH:"))
				if lh > 0 && current.LinesCovered != lh {
					current.LinesCovered = lh
				}
			}

		case line == "end_of_record":
			// End of file record
			if current != nil {
				report.Files = append(report.Files, *current)
				current = nil
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	report.Calculate()
	return report, nil
}
```

**Step 6: Run test to verify it passes**

```bash
go test ./internal/parser/... -v
```
Expected: PASS

**Step 7: Commit**

```bash
git add internal/parser/ testdata/
git commit -m "feat(parser): add LCOV parser"
```

---

## Task 4: Cobertura XML Parser

**Files:**
- Create: `internal/parser/cobertura.go`
- Create: `internal/parser/cobertura_test.go`
- Create: `testdata/simple.xml`

**Step 1: Create test data file**

Create `testdata/simple.xml`:
```xml
<?xml version="1.0" ?>
<coverage version="1.0" lines-valid="6" lines-covered="4" line-rate="0.666667" branches-valid="0" branches-covered="0" branch-rate="0" complexity="0">
    <packages>
        <package name="src" line-rate="0.666667">
            <classes>
                <class name="parser.go" filename="src/parser.go" line-rate="0.75">
                    <lines>
                        <line number="1" hits="1"/>
                        <line number="2" hits="1"/>
                        <line number="3" hits="0"/>
                        <line number="4" hits="1"/>
                    </lines>
                </class>
                <class name="utils.go" filename="src/utils.go" line-rate="0.5">
                    <lines>
                        <line number="1" hits="1"/>
                        <line number="2" hits="0"/>
                    </lines>
                </class>
            </classes>
        </package>
    </packages>
</coverage>
```

**Step 2: Write Cobertura parser test**

Create `internal/parser/cobertura_test.go`:
```go
package parser

import (
	"os"
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

	// Check first file
	if report.Files[0].Path != "src/parser.go" {
		t.Errorf("Files[0].Path = %v, want src/parser.go", report.Files[0].Path)
	}
	if report.Files[0].LinesCovered != 3 {
		t.Errorf("Files[0].LinesCovered = %v, want 3", report.Files[0].LinesCovered)
	}
	if report.Files[0].LinesTotal != 4 {
		t.Errorf("Files[0].LinesTotal = %v, want 4", report.Files[0].LinesTotal)
	}

	// Check totals
	if report.TotalCovered != 4 {
		t.Errorf("TotalCovered = %v, want 4", report.TotalCovered)
	}
	if report.TotalLines != 6 {
		t.Errorf("TotalLines = %v, want 6", report.TotalLines)
	}
}
```

**Step 3: Run test to verify it fails**

```bash
go test ./internal/parser/... -v
```
Expected: FAIL - CoberturaParser not defined

**Step 4: Write Cobertura parser implementation**

Create `internal/parser/cobertura.go`:
```go
package parser

import (
	"encoding/xml"
	"io"

	"github.com/litecov/litecov/internal/coverage"
)

// CoberturaParser parses Cobertura XML format coverage reports
type CoberturaParser struct{}

// coberturaXML represents the Cobertura XML structure
type coberturaXML struct {
	XMLName  xml.Name           `xml:"coverage"`
	Packages []coberturaPackage `xml:"packages>package"`
}

type coberturaPackage struct {
	Name    string            `xml:"name,attr"`
	Classes []coberturaClass  `xml:"classes>class"`
}

type coberturaClass struct {
	Name     string          `xml:"name,attr"`
	Filename string          `xml:"filename,attr"`
	Lines    []coberturaLine `xml:"lines>line"`
}

type coberturaLine struct {
	Number int `xml:"number,attr"`
	Hits   int `xml:"hits,attr"`
}

// Parse reads Cobertura XML format and returns a coverage report
func (p *CoberturaParser) Parse(r io.Reader) (*coverage.Report, error) {
	var cov coberturaXML
	decoder := xml.NewDecoder(r)
	if err := decoder.Decode(&cov); err != nil {
		return nil, err
	}

	report := &coverage.Report{}

	for _, pkg := range cov.Packages {
		for _, class := range pkg.Classes {
			fc := coverage.FileCoverage{
				Path:       class.Filename,
				LinesTotal: len(class.Lines),
			}
			for _, line := range class.Lines {
				if line.Hits > 0 {
					fc.LinesCovered++
				}
			}
			report.Files = append(report.Files, fc)
		}
	}

	report.Calculate()
	return report, nil
}
```

**Step 5: Run test to verify it passes**

```bash
go test ./internal/parser/... -v
```
Expected: PASS

**Step 6: Commit**

```bash
git add internal/parser/cobertura.go internal/parser/cobertura_test.go testdata/simple.xml
git commit -m "feat(parser): add Cobertura XML parser"
```

---

## Task 5: Format Auto-Detection

**Files:**
- Create: `internal/parser/detect.go`
- Create: `internal/parser/detect_test.go`

**Step 1: Write detection test**

Create `internal/parser/detect_test.go`:
```go
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
		wantErr bool
	}{
		{"lcov", false},
		{"cobertura", false},
		{"auto", false},
		{"unknown", true},
	}

	for _, tt := range tests {
		t.Run(tt.format, func(t *testing.T) {
			_, err := GetParser(tt.format)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetParser(%q) error = %v, wantErr %v", tt.format, err, tt.wantErr)
			}
		})
	}
}
```

**Step 2: Run test to verify it fails**

```bash
go test ./internal/parser/... -v -run TestDetect
```
Expected: FAIL - DetectFormat not defined

**Step 3: Write detection implementation**

Create `internal/parser/detect.go`:
```go
package parser

import (
	"bufio"
	"fmt"
	"io"
	"strings"
)

// DetectFormat reads the first few lines to determine coverage format
func DetectFormat(r io.Reader) (string, error) {
	reader := bufio.NewReader(r)

	// Read first 1KB to detect format
	buf := make([]byte, 1024)
	n, err := reader.Read(buf)
	if err != nil && err != io.EOF {
		return "", err
	}

	content := string(buf[:n])

	// Check for XML
	if strings.Contains(content, "<?xml") || strings.Contains(content, "<coverage") {
		return "cobertura", nil
	}

	// Check for LCOV markers
	if strings.Contains(content, "SF:") || strings.Contains(content, "end_of_record") {
		return "lcov", nil
	}

	return "", fmt.Errorf("unable to detect coverage format")
}

// GetParser returns the appropriate parser for the given format
func GetParser(format string) (Parser, error) {
	switch format {
	case "lcov":
		return &LCOVParser{}, nil
	case "cobertura", "xml":
		return &CoberturaParser{}, nil
	case "auto":
		// For auto, caller should use DetectFormat first
		return nil, nil
	default:
		return nil, fmt.Errorf("unknown format: %s", format)
	}
}
```

**Step 4: Run test to verify it passes**

```bash
go test ./internal/parser/... -v
```
Expected: PASS

**Step 5: Commit**

```bash
git add internal/parser/detect.go internal/parser/detect_test.go
git commit -m "feat(parser): add format auto-detection"
```

---

## Task 6: Comment Formatter

**Files:**
- Create: `internal/comment/comment.go`
- Create: `internal/comment/comment_test.go`

**Step 1: Write comment formatter test**

Create `internal/comment/comment_test.go`:
```go
package comment

import (
	"strings"
	"testing"

	"github.com/litecov/litecov/internal/coverage"
)

func TestFormat(t *testing.T) {
	report := &coverage.Report{
		Files: []coverage.FileCoverage{
			{Path: "src/parser.go", LinesCovered: 75, LinesTotal: 100},
			{Path: "src/utils.go", LinesCovered: 40, LinesTotal: 100},
		},
		TotalCovered: 115,
		TotalLines:   200,
		Coverage:     57.5,
	}

	opts := Options{
		Title:     "Coverage Report",
		ShowFiles: "all",
	}

	result := Format(report, opts)

	// Check header
	if !strings.Contains(result, "## Coverage Report") {
		t.Error("missing title header")
	}

	// Check coverage percentage
	if !strings.Contains(result, "57.50%") {
		t.Error("missing coverage percentage")
	}

	// Check lines ratio
	if !strings.Contains(result, "115/200") {
		t.Error("missing lines ratio")
	}

	// Check files are listed
	if !strings.Contains(result, "src/parser.go") {
		t.Error("missing parser.go file")
	}
	if !strings.Contains(result, "src/utils.go") {
		t.Error("missing utils.go file")
	}

	// Check warning indicator for low coverage file
	if !strings.Contains(result, ":warning:") {
		t.Error("missing warning indicator for low coverage file")
	}

	// Check marker for comment updates
	if !strings.Contains(result, "<!-- litecov -->") {
		t.Error("missing litecov marker")
	}
}

func TestFormat_FilterChanged(t *testing.T) {
	report := &coverage.Report{
		Files: []coverage.FileCoverage{
			{Path: "src/parser.go", LinesCovered: 75, LinesTotal: 100},
			{Path: "src/utils.go", LinesCovered: 40, LinesTotal: 100},
			{Path: "src/other.go", LinesCovered: 90, LinesTotal: 100},
		},
		TotalCovered: 205,
		TotalLines:   300,
		Coverage:     68.33,
	}

	opts := Options{
		Title:        "Coverage Report",
		ShowFiles:    "changed",
		ChangedFiles: []string{"src/parser.go", "src/utils.go"},
	}

	result := Format(report, opts)

	if !strings.Contains(result, "src/parser.go") {
		t.Error("missing changed file parser.go")
	}
	if !strings.Contains(result, "src/utils.go") {
		t.Error("missing changed file utils.go")
	}
	if strings.Contains(result, "src/other.go") {
		t.Error("should not contain unchanged file other.go")
	}
}
```

**Step 2: Run test to verify it fails**

```bash
go test ./internal/comment/... -v
```
Expected: FAIL - package doesn't exist

**Step 3: Write comment formatter implementation**

Create `internal/comment/comment.go`:
```go
package comment

import (
	"fmt"
	"strings"

	"github.com/litecov/litecov/internal/coverage"
)

const marker = "<!-- litecov -->"

// Options configures comment formatting
type Options struct {
	Title        string
	ShowFiles    string   // all, changed, threshold:N, worst:N
	ChangedFiles []string // files changed in PR (for ShowFiles=changed)
	Threshold    float64  // threshold for ShowFiles=threshold:N
	WorstN       int      // N for ShowFiles=worst:N
}

// Format generates a markdown comment from a coverage report
func Format(report *coverage.Report, opts Options) string {
	var sb strings.Builder

	// Marker for identifying our comments
	sb.WriteString(marker)
	sb.WriteString("\n")

	// Header
	sb.WriteString(fmt.Sprintf("## %s\n\n", opts.Title))

	// Summary table
	sb.WriteString("| Metric | Value |\n")
	sb.WriteString("|--------|-------|\n")
	sb.WriteString(fmt.Sprintf("| **Coverage** | `%.2f%%` |\n", report.Coverage))
	sb.WriteString(fmt.Sprintf("| **Lines** | `%d/%d` |\n", report.TotalCovered, report.TotalLines))
	sb.WriteString(fmt.Sprintf("| **Files** | `%d` |\n", len(report.Files)))
	sb.WriteString("\n")

	// Filter files based on ShowFiles option
	filesToShow := filterFiles(report.Files, opts)

	if len(filesToShow) > 0 {
		sb.WriteString("| File | Coverage | |\n")
		sb.WriteString("|------|----------|---|\n")
		for _, f := range filesToShow {
			pct := f.Percentage()
			indicator := ""
			if pct < 50 {
				indicator = " :warning:"
			}
			sb.WriteString(fmt.Sprintf("| `%s` | `%.2f%%` |%s |\n", f.Path, pct, indicator))
		}
		sb.WriteString("\n")
	}

	// Footer
	sb.WriteString("<sub>Generated by [LiteCov](https://github.com/litecov/litecov)</sub>\n")

	return sb.String()
}

// filterFiles returns files matching the ShowFiles criteria
func filterFiles(files []coverage.FileCoverage, opts Options) []coverage.FileCoverage {
	switch {
	case opts.ShowFiles == "all":
		return files

	case opts.ShowFiles == "changed":
		if len(opts.ChangedFiles) == 0 {
			return files
		}
		changedSet := make(map[string]bool)
		for _, f := range opts.ChangedFiles {
			changedSet[f] = true
		}
		var result []coverage.FileCoverage
		for _, f := range files {
			if changedSet[f.Path] {
				result = append(result, f)
			}
		}
		return result

	case strings.HasPrefix(opts.ShowFiles, "threshold:"):
		var result []coverage.FileCoverage
		for _, f := range files {
			if f.Percentage() < opts.Threshold {
				result = append(result, f)
			}
		}
		return result

	case strings.HasPrefix(opts.ShowFiles, "worst:"):
		// Sort by coverage ascending and take first N
		sorted := make([]coverage.FileCoverage, len(files))
		copy(sorted, files)
		// Simple bubble sort for small N
		for i := 0; i < len(sorted)-1; i++ {
			for j := 0; j < len(sorted)-i-1; j++ {
				if sorted[j].Percentage() > sorted[j+1].Percentage() {
					sorted[j], sorted[j+1] = sorted[j+1], sorted[j]
				}
			}
		}
		if opts.WorstN > len(sorted) {
			return sorted
		}
		return sorted[:opts.WorstN]

	default:
		return files
	}
}

// GetMarker returns the comment marker for identifying litecov comments
func GetMarker() string {
	return marker
}
```

**Step 4: Run test to verify it passes**

```bash
go test ./internal/comment/... -v
```
Expected: PASS

**Step 5: Commit**

```bash
git add internal/comment/
git commit -m "feat(comment): add PR comment formatter"
```

---

## Task 7: GitHub API Client

**Files:**
- Create: `internal/github/client.go`
- Create: `internal/github/client_test.go`

**Step 1: Write GitHub client test**

Create `internal/github/client_test.go`:
```go
package github

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClient_GetChangedFiles(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/owner/repo/pulls/1/files" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		files := []struct {
			Filename string `json:"filename"`
		}{
			{Filename: "src/parser.go"},
			{Filename: "src/utils.go"},
		}
		json.NewEncoder(w).Encode(files)
	}))
	defer server.Close()

	client := &Client{
		Token:   "test-token",
		Owner:   "owner",
		Repo:    "repo",
		BaseURL: server.URL,
	}

	files, err := client.GetChangedFiles(1)
	if err != nil {
		t.Fatalf("GetChangedFiles() error = %v", err)
	}

	if len(files) != 2 {
		t.Errorf("got %d files, want 2", len(files))
	}
	if files[0] != "src/parser.go" {
		t.Errorf("files[0] = %v, want src/parser.go", files[0])
	}
}

func TestClient_FindExistingComment(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		comments := []struct {
			ID   int    `json:"id"`
			Body string `json:"body"`
		}{
			{ID: 1, Body: "Some other comment"},
			{ID: 42, Body: "<!-- litecov -->\n## Coverage Report"},
		}
		json.NewEncoder(w).Encode(comments)
	}))
	defer server.Close()

	client := &Client{
		Token:   "test-token",
		Owner:   "owner",
		Repo:    "repo",
		BaseURL: server.URL,
	}

	id, err := client.FindExistingComment(1, "<!-- litecov -->")
	if err != nil {
		t.Fatalf("FindExistingComment() error = %v", err)
	}
	if id != 42 {
		t.Errorf("FindExistingComment() = %v, want 42", id)
	}
}
```

**Step 2: Run test to verify it fails**

```bash
go test ./internal/github/... -v
```
Expected: FAIL - package doesn't exist

**Step 3: Write GitHub client implementation**

Create `internal/github/client.go`:
```go
package github

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// Client handles GitHub API interactions
type Client struct {
	Token   string
	Owner   string
	Repo    string
	BaseURL string // For testing, defaults to https://api.github.com
}

// NewClient creates a new GitHub API client
func NewClient(token, owner, repo string) *Client {
	return &Client{
		Token:   token,
		Owner:   owner,
		Repo:    repo,
		BaseURL: "https://api.github.com",
	}
}

func (c *Client) doRequest(method, path string, body io.Reader) (*http.Response, error) {
	url := c.BaseURL + path
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	return http.DefaultClient.Do(req)
}

// GetChangedFiles returns the list of files changed in a PR
func (c *Client) GetChangedFiles(prNumber int) ([]string, error) {
	path := fmt.Sprintf("/repos/%s/%s/pulls/%d/files", c.Owner, c.Repo, prNumber)
	resp, err := c.doRequest("GET", path, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("GitHub API error: %s - %s", resp.Status, string(body))
	}

	var files []struct {
		Filename string `json:"filename"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&files); err != nil {
		return nil, err
	}

	result := make([]string, len(files))
	for i, f := range files {
		result[i] = f.Filename
	}
	return result, nil
}

// FindExistingComment looks for an existing comment with the given marker
func (c *Client) FindExistingComment(prNumber int, marker string) (int, error) {
	path := fmt.Sprintf("/repos/%s/%s/issues/%d/comments", c.Owner, c.Repo, prNumber)
	resp, err := c.doRequest("GET", path, nil)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return 0, fmt.Errorf("GitHub API error: %s - %s", resp.Status, string(body))
	}

	var comments []struct {
		ID   int    `json:"id"`
		Body string `json:"body"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&comments); err != nil {
		return 0, err
	}

	for _, comment := range comments {
		if strings.HasPrefix(comment.Body, marker) {
			return comment.ID, nil
		}
	}
	return 0, nil
}

// CreateComment creates a new comment on a PR
func (c *Client) CreateComment(prNumber int, body string) error {
	path := fmt.Sprintf("/repos/%s/%s/issues/%d/comments", c.Owner, c.Repo, prNumber)
	payload := map[string]string{"body": body}
	jsonBody, _ := json.Marshal(payload)

	resp, err := c.doRequest("POST", path, bytes.NewReader(jsonBody))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("GitHub API error: %s - %s", resp.Status, string(respBody))
	}
	return nil
}

// UpdateComment updates an existing comment
func (c *Client) UpdateComment(commentID int, body string) error {
	path := fmt.Sprintf("/repos/%s/%s/issues/comments/%d", c.Owner, c.Repo, commentID)
	payload := map[string]string{"body": body}
	jsonBody, _ := json.Marshal(payload)

	resp, err := c.doRequest("PATCH", path, bytes.NewReader(jsonBody))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("GitHub API error: %s - %s", resp.Status, string(respBody))
	}
	return nil
}

// SetCommitStatus sets the commit status for a SHA
func (c *Client) SetCommitStatus(sha, state, description, context string) error {
	path := fmt.Sprintf("/repos/%s/%s/statuses/%s", c.Owner, c.Repo, sha)
	payload := map[string]string{
		"state":       state,
		"description": description,
		"context":     context,
	}
	jsonBody, _ := json.Marshal(payload)

	resp, err := c.doRequest("POST", path, bytes.NewReader(jsonBody))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("GitHub API error: %s - %s", resp.Status, string(respBody))
	}
	return nil
}
```

**Step 4: Run test to verify it passes**

```bash
go test ./internal/github/... -v
```
Expected: PASS

**Step 5: Commit**

```bash
git add internal/github/
git commit -m "feat(github): add GitHub API client"
```

---

## Task 8: CLI Main Command

**Files:**
- Create: `cmd/litecov/main.go`

**Step 1: Write main command**

Create `cmd/litecov/main.go`:
```go
package main

import (
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/litecov/litecov/internal/comment"
	"github.com/litecov/litecov/internal/coverage"
	"github.com/litecov/litecov/internal/github"
	"github.com/litecov/litecov/internal/parser"
)

func main() {
	// Flags
	coverageFile := flag.String("coverage-file", "", "Path to coverage report file")
	format := flag.String("format", "auto", "Coverage format: auto, lcov, cobertura")
	showFiles := flag.String("show-files", "changed", "Files to show: all, changed, threshold:N, worst:N")
	threshold := flag.Float64("threshold", 0, "Minimum coverage threshold for passing status")
	title := flag.String("title", "Coverage Report", "Comment title")
	flag.Parse()

	// Get GitHub context from environment
	token := os.Getenv("GITHUB_TOKEN")
	repository := os.Getenv("GITHUB_REPOSITORY") // owner/repo
	eventPath := os.Getenv("GITHUB_EVENT_PATH")
	sha := os.Getenv("GITHUB_SHA")

	if token == "" {
		fmt.Fprintln(os.Stderr, "GITHUB_TOKEN is required")
		os.Exit(1)
	}

	// Parse repository
	parts := strings.Split(repository, "/")
	if len(parts) != 2 {
		fmt.Fprintf(os.Stderr, "Invalid GITHUB_REPOSITORY: %s\n", repository)
		os.Exit(1)
	}
	owner, repo := parts[0], parts[1]

	// Get PR number from event
	prNumber, err := getPRNumber(eventPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to get PR number: %v\n", err)
		os.Exit(1)
	}

	// Auto-detect coverage file if not specified
	if *coverageFile == "" {
		*coverageFile = detectCoverageFile()
		if *coverageFile == "" {
			fmt.Fprintln(os.Stderr, "No coverage file found. Specify with -coverage-file")
			os.Exit(1)
		}
	}

	// Open and parse coverage file
	f, err := os.Open(*coverageFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to open coverage file: %v\n", err)
		os.Exit(1)
	}
	defer f.Close()

	// Detect format if auto
	var p parser.Parser
	if *format == "auto" {
		detected, err := parser.DetectFormat(f)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to detect format: %v\n", err)
			os.Exit(1)
		}
		f.Seek(0, 0) // Reset file pointer
		p, _ = parser.GetParser(detected)
	} else {
		p, err = parser.GetParser(*format)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Unknown format: %s\n", *format)
			os.Exit(1)
		}
	}

	report, err := p.Parse(f)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to parse coverage: %v\n", err)
		os.Exit(1)
	}

	// Create GitHub client
	gh := github.NewClient(token, owner, repo)

	// Get changed files if needed
	var changedFiles []string
	if *showFiles == "changed" && prNumber > 0 {
		changedFiles, err = gh.GetChangedFiles(prNumber)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: Failed to get changed files: %v\n", err)
		}
	}

	// Parse show-files options
	opts := comment.Options{
		Title:        *title,
		ShowFiles:    *showFiles,
		ChangedFiles: changedFiles,
	}
	if strings.HasPrefix(*showFiles, "threshold:") {
		val, _ := strconv.ParseFloat(strings.TrimPrefix(*showFiles, "threshold:"), 64)
		opts.Threshold = val
	}
	if strings.HasPrefix(*showFiles, "worst:") {
		val, _ := strconv.Atoi(strings.TrimPrefix(*showFiles, "worst:"))
		opts.WorstN = val
	}

	// Format comment
	commentBody := comment.Format(report, opts)

	// Post or update comment
	if prNumber > 0 {
		existingID, _ := gh.FindExistingComment(prNumber, comment.GetMarker())
		if existingID > 0 {
			err = gh.UpdateComment(existingID, commentBody)
		} else {
			err = gh.CreateComment(prNumber, commentBody)
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to post comment: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("Coverage comment posted successfully")
	}

	// Set commit status
	if sha != "" {
		state := "success"
		description := fmt.Sprintf("%.2f%% coverage", report.Coverage)
		if *threshold > 0 && report.Coverage < *threshold {
			state = "failure"
			description = fmt.Sprintf("%.2f%% coverage (minimum: %.2f%%)", report.Coverage, *threshold)
		}
		if err := gh.SetCommitStatus(sha, state, description, "litecov"); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: Failed to set commit status: %v\n", err)
		}
	}

	// Output results
	fmt.Printf("Coverage: %.2f%%\n", report.Coverage)
	fmt.Printf("Lines: %d/%d\n", report.TotalCovered, report.TotalLines)
	fmt.Printf("Files: %d\n", len(report.Files))

	// Set outputs for GitHub Actions
	if ghOutput := os.Getenv("GITHUB_OUTPUT"); ghOutput != "" {
		f, err := os.OpenFile(ghOutput, os.O_APPEND|os.O_WRONLY, 0644)
		if err == nil {
			fmt.Fprintf(f, "coverage=%.2f\n", report.Coverage)
			fmt.Fprintf(f, "lines-covered=%d\n", report.TotalCovered)
			fmt.Fprintf(f, "lines-total=%d\n", report.TotalLines)
			fmt.Fprintf(f, "files-count=%d\n", len(report.Files))
			f.Close()
		}
	}

	// Exit with failure if below threshold
	if *threshold > 0 && report.Coverage < *threshold {
		os.Exit(1)
	}
}

func getPRNumber(eventPath string) (int, error) {
	if eventPath == "" {
		return 0, nil
	}
	data, err := os.ReadFile(eventPath)
	if err != nil {
		return 0, err
	}
	// Simple JSON parsing for PR number
	content := string(data)
	// Look for "number": N in pull_request context
	if idx := strings.Index(content, `"number":`); idx >= 0 {
		start := idx + 9
		end := start
		for end < len(content) && (content[end] >= '0' && content[end] <= '9') {
			end++
		}
		if end > start {
			return strconv.Atoi(content[start:end])
		}
	}
	return 0, nil
}

func detectCoverageFile() string {
	candidates := []string{
		"coverage.lcov",
		"lcov.info",
		"coverage/lcov.info",
		"coverage.xml",
		"cobertura.xml",
		"coverage/cobertura.xml",
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	return ""
}
```

**Step 2: Verify it compiles**

```bash
go build ./cmd/litecov
```
Expected: Binary created successfully

**Step 3: Commit**

```bash
git add cmd/litecov/
git commit -m "feat(cli): add main command"
```

---

## Task 9: GitHub Action Definition

**Files:**
- Create: `action.yml`
- Create: `Dockerfile`

**Step 1: Create action.yml**

Create `action.yml`:
```yaml
name: 'LiteCov'
description: 'Lightweight code coverage reporter - posts PR comments with coverage stats'
author: 'litecov'

branding:
  icon: 'check-circle'
  color: 'green'

inputs:
  coverage-file:
    description: 'Path to coverage report file (auto-detected if not specified)'
    required: false
  format:
    description: 'Coverage format: auto, lcov, cobertura'
    required: false
    default: 'auto'
  show-files:
    description: 'Files to show: all, changed, threshold:N, worst:N'
    required: false
    default: 'changed'
  threshold:
    description: 'Minimum coverage threshold for passing status (0-100)'
    required: false
    default: '0'
  title:
    description: 'Comment title'
    required: false
    default: 'Coverage Report'
  token:
    description: 'GitHub token'
    required: false
    default: ${{ github.token }}

outputs:
  coverage:
    description: 'Total coverage percentage'
  lines-covered:
    description: 'Number of covered lines'
  lines-total:
    description: 'Total number of lines'
  files-count:
    description: 'Number of files with coverage data'

runs:
  using: 'docker'
  image: 'Dockerfile'
  args:
    - -coverage-file=${{ inputs.coverage-file }}
    - -format=${{ inputs.format }}
    - -show-files=${{ inputs.show-files }}
    - -threshold=${{ inputs.threshold }}
    - -title=${{ inputs.title }}
  env:
    GITHUB_TOKEN: ${{ inputs.token }}
```

**Step 2: Create Dockerfile**

Create `Dockerfile`:
```dockerfile
FROM golang:1.22-alpine AS builder

WORKDIR /app
COPY go.mod go.sum* ./
RUN go mod download 2>/dev/null || true
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /litecov ./cmd/litecov

FROM alpine:3.19
RUN apk --no-cache add ca-certificates
COPY --from=builder /litecov /litecov
ENTRYPOINT ["/litecov"]
```

**Step 3: Commit**

```bash
git add action.yml Dockerfile
git commit -m "feat(action): add GitHub Action definition"
```

---

## Task 10: README and Documentation

**Files:**
- Create: `README.md`

**Step 1: Create README**

Create `README.md`:
```markdown
# LiteCov

Lightweight code coverage reporter for GitHub Actions. Zero infrastructure, one-line setup.

## Quick Start

```yaml
- uses: litecov/litecov@v1
```

That's it. LiteCov will auto-detect your coverage file and post a PR comment.

## Features

- **Zero infrastructure** - No server, database, or external services
- **Auto-detection** - Finds coverage files automatically
- **Multiple formats** - Supports LCOV and Cobertura XML
- **PR comments** - Posts coverage summary as a comment
- **Commit status** - Sets coverage status on commits
- **Configurable** - Filter files, set thresholds, customize output

## Usage

### Basic

```yaml
name: CI
on: [pull_request]

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Run tests with coverage
        run: go test -coverprofile=coverage.out ./...

      - name: Convert to LCOV
        run: |
          go install github.com/jandelgado/gcov2lcov@latest
          gcov2lcov -infile=coverage.out -outfile=coverage.lcov

      - uses: litecov/litecov@v1
```

### With Options

```yaml
- uses: litecov/litecov@v1
  with:
    coverage-file: coverage.lcov
    format: lcov
    show-files: changed
    threshold: 80
    title: Test Coverage
```

## Inputs

| Input | Default | Description |
|-------|---------|-------------|
| `coverage-file` | Auto-detect | Path to coverage report |
| `format` | `auto` | Format: `auto`, `lcov`, `cobertura` |
| `show-files` | `changed` | Files to show (see below) |
| `threshold` | `0` | Minimum coverage % to pass |
| `title` | `Coverage Report` | Comment header |
| `token` | `GITHUB_TOKEN` | GitHub token |

### Show Files Options

- `changed` - Only files modified in the PR
- `all` - All files in coverage report
- `threshold:N` - Files below N% coverage
- `worst:N` - N files with lowest coverage

## Outputs

| Output | Description |
|--------|-------------|
| `coverage` | Coverage percentage |
| `lines-covered` | Covered lines count |
| `lines-total` | Total lines count |
| `files-count` | Number of files |

## Supported Formats

### LCOV

Generated by Jest, Vitest, Go (with gcov2lcov), Rust, C/C++, Ruby.

### Cobertura XML

Generated by pytest-cov, coverage.py, JaCoCo, Coverlet.

## License

MIT
```

**Step 2: Commit**

```bash
git add README.md
git commit -m "docs: add README"
```

---

## Task 11: Final Testing & Cleanup

**Step 1: Run all tests**

```bash
go test ./... -v -cover
```
Expected: All tests pass

**Step 2: Build and verify binary size**

```bash
go build -ldflags="-s -w" -o litecov ./cmd/litecov
ls -lh litecov
```
Expected: Binary under 10MB

**Step 3: Create go.sum if needed**

```bash
go mod tidy
```

**Step 4: Final commit**

```bash
git add go.sum
git commit -m "chore: finalize go.sum"
```

---

## Summary

After completing all tasks, you will have:

1. **Parsers** - LCOV and Cobertura XML parsing
2. **Comment formatter** - Codecov-style markdown output
3. **GitHub client** - API integration for comments and status
4. **CLI** - Main command with all options
5. **GitHub Action** - Ready to use in workflows
6. **Documentation** - README with examples

Total estimated binary size: ~5MB
Total files: ~15
