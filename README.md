<p align="center">
  <img src="logo.png" alt="LiteCov" width="200">
</p>

# LiteCov

Lightweight code coverage reporter for GitHub Actions. Zero infrastructure, one-line setup.

## Quick Start

```yaml
- uses: manashmandal/litecov@v1
```

That's it. LiteCov will auto-detect your coverage file and post a PR comment.

## Features

- **Zero infrastructure** - No server, database, or external services
- **Auto-detection** - Finds coverage files automatically
- **Multiple formats** - Supports LCOV, Cobertura XML, and Go coverage profiles
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

      - uses: manashmandal/litecov@v1
        with:
          coverage-file: coverage.out
```

### With Options

```yaml
- uses: manashmandal/litecov@v1
  with:
    coverage-file: coverage.lcov
    format: lcov
    show-files: changed
    threshold: 80
    title: Test Coverage
```

### Python Example

```yaml
- name: Run tests
  run: pytest --cov=src --cov-report=xml

- uses: manashmandal/litecov@v1
  with:
    coverage-file: coverage.xml
```

### JavaScript Example

```yaml
- name: Run tests
  run: npm test -- --coverage --coverageReporters=lcov

- uses: manashmandal/litecov@v1
  with:
    coverage-file: coverage/lcov.info
```

### Java Example

JaCoCo doesn't export Cobertura XML. Convert its native report with [cover2cover.py](https://github.com/rix0rrr/cover2cover) before handing it to litecov:

```yaml
- name: Run tests
  run: mvn test jacoco:report

- name: Convert JaCoCo report to Cobertura XML
  run: |
    curl -sL https://raw.githubusercontent.com/rix0rrr/cover2cover/master/cover2cover.py -o cover2cover.py
    python3 cover2cover.py target/site/jacoco/jacoco.xml src/main/java > target/site/jacoco/cobertura.xml

- uses: manashmandal/litecov@v1
  with:
    coverage-file: target/site/jacoco/cobertura.xml
```

### Comparing Against a Base Branch

LiteCov has no server or cache, so it can't look up the base branch's coverage
on its own. Give it a `base-coverage-file` and it will render the Codecov-style
delta column; without one it falls back to the plain report. The base file has
to come from somewhere, so compute it in the same workflow run and pass it
across jobs with an artifact:

```yaml
name: Coverage
on: [pull_request]

jobs:
  base-coverage:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with:
          ref: ${{ github.event.pull_request.base.sha }}

      - uses: actions/setup-go@v5
        with:
          go-version: '1.21'

      - run: go test -coverprofile=coverage.out ./...

      - uses: actions/upload-artifact@v4
        with:
          name: base-coverage
          path: coverage.out

  pr-coverage:
    needs: base-coverage
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - uses: actions/setup-go@v5
        with:
          go-version: '1.21'

      - run: go test -coverprofile=coverage.out ./...

      - uses: actions/download-artifact@v4
        with:
          name: base-coverage
          path: base

      - uses: manashmandal/litecov@v1
        with:
          coverage-file: coverage.out
          base-coverage-file: base/coverage.out
          base-branch: ${{ github.event.pull_request.base.ref }}
```

`base-coverage` checks out the PR's base commit and tests it. `pr-coverage`
tests the PR head, downloads that artifact, and hands both files to litecov
so it can diff them.

## Inputs

| Input | Default | Description |
|-------|---------|-------------|
| `coverage-file` | Auto-detect | Path to coverage report |
| `format` | `auto` | Format: `auto`, `lcov`, `cobertura`, `go` |
| `show-files` | `changed` | Files to show (see below) |
| `threshold` | `0` | Minimum coverage % to pass |
| `patch-threshold` | `0` | Minimum patch coverage % to pass, independent of `threshold` |
| `good-threshold` | `0` (uses 80) | Coverage % at/above which the report marks a file or the project as passing |
| `warn-threshold` | `0` (uses 50) | Coverage % at/above which the report warns instead of failing |
| `title` | `Coverage Report` | Comment header |
| `flag` | None | Identifies this run when litecov runs more than once on one PR (e.g. `backend`, `frontend`); scopes the comment marker and commit status context so runs don't overwrite each other |
| `annotations` | `false` | Output GitHub annotations for uncovered lines |
| `base-coverage-file` | None | Path to a base branch coverage file, enables comparison mode |
| `base-branch` | PR base branch | Base branch name shown in the diff header |
| `token` | `GITHUB_TOKEN` | GitHub token |

### Show Files Options

- `changed` - Only files modified in the PR (default)
- `all` - All files in coverage report
- `threshold:N` - Files below N% coverage (e.g., `threshold:80`)
- `worst:N` - N files with lowest coverage (e.g., `worst:10`)

## Outputs

| Output | Description |
|--------|-------------|
| `coverage` | Coverage percentage |
| `lines-covered` | Covered lines count |
| `lines-total` | Total lines count |
| `files-count` | Number of files |

### Using Outputs

```yaml
- uses: manashmandal/litecov@v1
  id: coverage

- name: Check coverage
  run: |
    echo "Coverage: ${{ steps.coverage.outputs.coverage }}%"
    echo "Lines: ${{ steps.coverage.outputs.lines-covered }}/${{ steps.coverage.outputs.lines-total }}"
```

## PR Comment

LiteCov posts a clean, informative comment on your PR with clickable links:

````
<!-- litecov -->
## <img src="https://raw.githubusercontent.com/manashmandal/litecov/main/logo.png" height="24" align="absmiddle"> Coverage Report

> ⚠️ Project coverage is `70.67%`. Head commit: [`a1b2c3d`](https://github.com/manashmandal/litecov/commit/a1b2c3d).
> **Lines:** `159/225` | **Files:** `2`

**Impacted Files (2)**

| File | Coverage | Uncovered Lines | Status |
|------|----------|-----------------|--------|
| [`src/utils.go`](https://github.com/manashmandal/litecov/blob/a1b2c3d/src/utils.go) | `45.00%` | [L12-15](https://github.com/manashmandal/litecov/blob/a1b2c3d/src/utils.go#L12-L15), [L30-35](https://github.com/manashmandal/litecov/blob/a1b2c3d/src/utils.go#L30-L35), [L50](https://github.com/manashmandal/litecov/blob/a1b2c3d/src/utils.go#L50) | ❌ |
| [`src/parser.go`](https://github.com/manashmandal/litecov/blob/a1b2c3d/src/parser.go) | `91.20%` | [L45-47](https://github.com/manashmandal/litecov/blob/a1b2c3d/src/parser.go#L45-L47), [L102](https://github.com/manashmandal/litecov/blob/a1b2c3d/src/parser.go#L102) | ✅ |

<details>
<summary>Additional details and impacted files</summary>

```diff
@@ Coverage Summary @@
======================
  Coverage    70.67%
  Lines      159/225
  Files            2
======================
```

</details>

---
> **Legend** `ø = not affected`, `new = file not in the base report`, `- = no uncovered lines`
> ✅ ≥ 80%   ⚠️ ≥ 50%   ❌ < 50%
<sub>📈 Generated by [LiteCov](https://github.com/manashmandal/litecov)</sub>
````

- Files are hyperlinked to the GitHub blob view
- Uncovered lines are clickable and link to specific line ranges
- Coverage is ✅ at or above `good-threshold` (default 80%), ⚠️ at or above `warn-threshold` (default 50%), and ❌ below that

## Supported Formats

### Go Coverage Profile

Generated by:
- **Go**: `go test -coverprofile=coverage.out` (mode `set`, `count`, or `atomic`), no converter needed

### LCOV

Generated by:
- **JavaScript**: Jest, Vitest, c8, nyc
- **Rust**: grcov, tarpaulin
- **C/C++**: gcov, llvm-cov
- **Ruby**: SimpleCov (with lcov formatter)

### Cobertura XML

Generated by:
- **Python**: pytest-cov, coverage.py
- **Java**: Cobertura, or JaCoCo converted with cover2cover.py (JaCoCo has no Cobertura export)
- **.NET**: Coverlet

## Auto-Detection

LiteCov looks for coverage files in this order:
1. `coverage.lcov`
2. `lcov.info`
3. `coverage/lcov.info`
4. `coverage.xml`
5. `cobertura.xml`
6. `coverage/cobertura.xml`

## Threshold Enforcement

LiteCov posts two independent commit statuses, `litecov` for project-wide
coverage and `litecov/patch` for coverage of just the lines the PR added
(Codecov calls these `codecov/project` and `codecov/patch`). Each has its own
threshold, so a PR that adds untested code fails even when it barely moves
the project number:

```yaml
- uses: manashmandal/litecov@v1
  with:
    threshold: 80
    patch-threshold: 80
```

If coverage drops below either threshold, the action will:
1. Set that status to "failure"
2. Exit with code 1 (failing the workflow)

`patch-threshold` is checked only against lines the PR actually touched. A
PR with no coverable changes (docs, config, a diff outside any coverage
tool's instrumentation) always passes `litecov/patch`, since there is
nothing to measure. Leaving either threshold at `0` (the default) disables
enforcement for that status.

## License

MIT
