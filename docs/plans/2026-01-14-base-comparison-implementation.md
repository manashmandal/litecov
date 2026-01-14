# Base Branch Comparison Implementation Plan

## Overview

Implement full Codecov-style coverage comparison showing:
- Coverage delta from base branch
- Per-file coverage changes (Δ column)
- Patch coverage (coverage of changed lines only)
- Hits/Misses breakdown
- Logo in header

## Architecture

### Storage Strategy: GitHub Actions Cache + Artifacts

Use a two-tier approach:
1. **Primary**: Upload coverage JSON to GitHub Actions cache keyed by branch
2. **Fallback**: Store in PR comment metadata (for cross-workflow access)

### Data Flow

```
┌─────────────────────────────────────────────────────────────────┐
│                         PR Workflow                              │
├─────────────────────────────────────────────────────────────────┤
│  1. Parse coverage file (existing)                              │
│  2. Fetch base branch coverage from cache/artifact              │
│  3. Get changed files from PR (existing)                        │
│  4. Parse git diff to get changed line ranges                   │
│  5. Calculate patch coverage (coverage of changed lines)        │
│  6. Calculate deltas (overall + per-file)                       │
│  7. Generate Codecov-style comment                              │
│  8. Upload current coverage to cache (for future PRs)           │
└─────────────────────────────────────────────────────────────────┘
```

## Implementation Details

### 1. New Data Structures

**internal/coverage/coverage.go**:
```go
type Report struct {
    Files        []FileCoverage
    TotalCovered int
    TotalLines   int
    Coverage     float64
    // New fields
    TotalHits    int     // Same as TotalCovered
    TotalMisses  int     // TotalLines - TotalCovered
}

type FileCoverage struct {
    Path           string
    LinesCovered   int
    LinesTotal     int
    UncoveredLines []int
    CoveredLines   []int  // NEW: needed for patch coverage calc
}

// NEW: Comparison result
type Comparison struct {
    Base           *Report
    Head           *Report
    CoverageDelta  float64         // Head.Coverage - Base.Coverage
    PatchCoverage  float64         // Coverage of changed lines only
    PatchCovered   int
    PatchTotal     int
    FileChanges    []FileChange
}

type FileChange struct {
    Path          string
    HeadCoverage  float64
    BaseCoverage  float64  // 0 if new file
    Delta         float64
    IsNew         bool
    IsDeleted     bool
    PatchCoverage float64  // Coverage of changed lines in this file
}
```

### 2. New Package: internal/diff

**internal/diff/diff.go**:
```go
package diff

// LineRange represents a range of lines
type LineRange struct {
    Start int
    End   int
}

// FileDiff represents changed lines in a file
type FileDiff struct {
    Path         string
    AddedLines   []LineRange  // Lines added in PR
    ModifiedLines []LineRange // Lines modified in PR
}

// ParseGitDiff parses git diff output to extract changed line ranges
func ParseGitDiff(diffOutput string) ([]FileDiff, error)

// GetPRDiff gets the diff between base and head
func GetPRDiff(owner, repo string, prNumber int, token string) ([]FileDiff, error)
```

### 3. New Package: internal/compare

**internal/compare/compare.go**:
```go
package compare

// Compare compares head coverage against base coverage
func Compare(head, base *coverage.Report, changedFiles []diff.FileDiff) *coverage.Comparison

// CalculatePatchCoverage calculates coverage for only changed lines
func CalculatePatchCoverage(file coverage.FileCoverage, changedLines []diff.LineRange) float64
```

### 4. Updated Comment Format

**internal/comment/comment.go**:
```go
func FormatWithComparison(comp *coverage.Comparison, opts Options) string
```

Output format:
```markdown
<!-- litecov -->
## <img src="https://raw.githubusercontent.com/manashmandal/litecov/main/logo.png" height="24" align="absmiddle"> Coverage Report

> ✅ **Coverage:** `91.51%` (+0.07%) | **Patch:** `85.00%` | **Δ Files:** `5`

<details>
<summary>Coverage Diff</summary>

` ` `diff
@@              Coverage Diff              @@
##               main       #123     +/-   ##
=============================================
+ Coverage     91.44%    91.51%   +0.07%
=============================================
  Files           234       234
  Lines         28655     28700      +45
=============================================
+ Hits          26204     26260      +56
- Misses         2451      2440      -11
` ` `

</details>

<details>
<summary>Impacted Files (5)</summary>

| File | Coverage | Δ | Status |
|------|----------|---|--------|
| [`src/parser/lcov.go`](link) | `94.20%` | `+2.10%` | ✅ |
| [`src/coverage/report.go`](link) | `87.50%` | `+5.00%` | ✅ |
| [`src/comment/format.go`](link) | `72.00%` | `-3.00%` | ⚠️ |
| [`internal/diff/diff.go`](link) | `65.00%` | `new` | ❌ |
| [`cmd/main.go`](link) | `100.0%` | `ø` | ✅ |

</details>

---
<sub>📈 Generated by <a href="https://github.com/manashmandal/litecov">LiteCov</a></sub>
```

### 5. Base Coverage Storage

**Option A: GitHub API (Recommended)**

Store base coverage as a JSON artifact or in a special branch:

```go
// internal/storage/storage.go
package storage

type Storage interface {
    SaveCoverage(branch string, report *coverage.Report) error
    LoadCoverage(branch string) (*coverage.Report, error)
}

// GitHubStorage uses GitHub API to store/retrieve coverage
type GitHubStorage struct {
    client *github.Client
    owner  string
    repo   string
}

func (s *GitHubStorage) SaveCoverage(branch string, report *coverage.Report) error {
    // Save to refs/coverage/<branch> or as workflow artifact
}

func (s *GitHubStorage) LoadCoverage(branch string) (*coverage.Report, error) {
    // Load from refs/coverage/<branch> or workflow artifact
}
```

**Option B: Workflow Artifact Cache**

Use actions/cache to store coverage:
- Key: `coverage-{branch}-{sha}`
- Restore keys: `coverage-{branch}-`

### 6. Action Inputs Update

**action.yml**:
```yaml
inputs:
  # ... existing inputs ...

  compare-against:
    description: 'Branch to compare against (default: base branch of PR)'
    required: false
    default: ''

  base-coverage-file:
    description: 'Path to base branch coverage file (if available locally)'
    required: false

  show-patch-coverage:
    description: 'Calculate and show patch coverage'
    required: false
    default: 'true'

  logo:
    description: 'Show logo in comment header'
    required: false
    default: 'true'
```

### 7. Main Flow Update

**cmd/litecov/main.go**:
```go
func main() {
    // ... existing setup ...

    // Parse current coverage
    headReport, err := parser.Parse(coverageFile)

    // Get base coverage
    var baseReport *coverage.Report
    if *baseCoverageFile != "" {
        baseReport, _ = parser.Parse(*baseCoverageFile)
    } else {
        // Try to fetch from storage
        storage := storage.NewGitHubStorage(gh)
        baseReport, _ = storage.LoadCoverage(baseBranch)
    }

    // Get changed files and diff
    var fileDiffs []diff.FileDiff
    if prNumber > 0 && *showPatchCoverage {
        fileDiffs, _ = diff.GetPRDiff(owner, repo, prNumber, token)
    }

    // Compare
    var comparison *coverage.Comparison
    if baseReport != nil || len(fileDiffs) > 0 {
        comparison = compare.Compare(headReport, baseReport, fileDiffs)
    }

    // Format comment
    var commentBody string
    if comparison != nil {
        commentBody = comment.FormatWithComparison(comparison, opts)
    } else {
        commentBody = comment.Format(headReport, opts)
    }

    // ... post comment ...

    // Save current coverage for future comparisons (on main branch)
    if os.Getenv("GITHUB_REF") == "refs/heads/main" {
        storage.SaveCoverage("main", headReport)
    }
}
```

## File Changes Summary

| File | Action | Description |
|------|--------|-------------|
| `internal/coverage/coverage.go` | Modify | Add CoveredLines, Comparison types |
| `internal/diff/diff.go` | Create | Git diff parsing |
| `internal/diff/diff_test.go` | Create | Tests for diff parsing |
| `internal/compare/compare.go` | Create | Coverage comparison logic |
| `internal/compare/compare_test.go` | Create | Tests for comparison |
| `internal/storage/storage.go` | Create | Base coverage storage |
| `internal/storage/storage_test.go` | Create | Tests for storage |
| `internal/comment/comment.go` | Modify | Add FormatWithComparison |
| `internal/comment/comment_test.go` | Modify | Update tests |
| `cmd/litecov/main.go` | Modify | Integrate comparison flow |
| `action.yml` | Modify | Add new inputs |

## Implementation Phases

### Phase 1: Core Comparison (MVP)
1. Add CoveredLines to FileCoverage
2. Create internal/diff package (parse git diff)
3. Create internal/compare package
4. Update comment formatting with deltas
5. Add logo to header

### Phase 2: Base Coverage Storage
1. Create internal/storage package
2. Implement GitHub-based storage
3. Auto-save on main branch pushes
4. Auto-load for PR comparisons

### Phase 3: Polish
1. Patch coverage calculation
2. Unicode emoji option
3. Configurable thresholds for status emojis
4. Full test coverage
