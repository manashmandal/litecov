package main

import (
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/manashmandal/litecov/internal/comment"
	"github.com/manashmandal/litecov/internal/coverage"
	"github.com/manashmandal/litecov/internal/github"
	"github.com/manashmandal/litecov/internal/parser"
)

func main() {
	fmt.Println("LITECOV STARTED - V5")
	fmt.Printf("os.Args = %v\n", os.Args)
	for i, arg := range os.Args {
		fmt.Printf("  arg[%d] = %q (bytes: %v)\n", i, arg, []byte(arg))
	}
	coverageFile := flag.String("coverage-file", "", "Path to coverage report file")
	format := flag.String("format", "auto", "Coverage format: auto, lcov, cobertura")
	showFiles := flag.String("show-files", "changed", "Files to show: all, changed, threshold:N, worst:N")
	threshold := flag.Float64("threshold", 0, "Minimum coverage threshold for passing status")
	title := flag.String("title", "Coverage Report", "Comment title")
	annotations := flag.Bool("annotations", false, "Output GitHub annotations for uncovered lines")
	baseCoverageFile := flag.String("base-coverage-file", "", "Path to base branch coverage file for comparison")
	baseBranch := flag.String("base-branch", "main", "Base branch name for comparison display")
	flag.Parse()
	fmt.Printf("AFTER PARSE: annotations=%v coverage-file=%q\n", *annotations, *coverageFile)

	// Environment variable overrides for GitHub Action
	if *baseCoverageFile == "" {
		*baseCoverageFile = os.Getenv("INPUT_BASE_COVERAGE_FILE")
	}
	if envBaseBranch := os.Getenv("INPUT_BASE_BRANCH"); envBaseBranch != "" {
		*baseBranch = envBaseBranch
	}

	token := os.Getenv("GITHUB_TOKEN")
	repository := os.Getenv("GITHUB_REPOSITORY")
	eventPath := os.Getenv("GITHUB_EVENT_PATH")
	sha := os.Getenv("GITHUB_SHA")

	if token == "" {
		fmt.Fprintln(os.Stderr, "GITHUB_TOKEN is required")
		os.Exit(1)
	}

	parts := strings.Split(repository, "/")
	if len(parts) != 2 {
		fmt.Fprintf(os.Stderr, "Invalid GITHUB_REPOSITORY: %s\n", repository)
		os.Exit(1)
	}
	owner, repo := parts[0], parts[1]

	prNumber, err := getPRNumber(eventPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to get PR number: %v\n", err)
		os.Exit(1)
	}

	if *coverageFile == "" {
		*coverageFile = detectCoverageFile()
		if *coverageFile == "" {
			fmt.Fprintln(os.Stderr, "No coverage file found. Specify with -coverage-file")
			os.Exit(1)
		}
		fmt.Printf("Auto-detected coverage file: %s\n", *coverageFile)
	}

	f, err := os.Open(*coverageFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to open coverage file: %v\n", err)
		os.Exit(1)
	}
	defer f.Close()

	var p parser.Parser
	if *format == "auto" {
		detected, err := parser.DetectFormat(f)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to detect format: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Detected format: %s\n", detected)
		f.Seek(0, 0)
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

	// Parse base coverage if provided
	var baseReport *coverage.Report
	if *baseCoverageFile != "" {
		if baseFile, err := os.Open(*baseCoverageFile); err == nil {
			defer baseFile.Close()
			if detected, err := parser.DetectFormat(baseFile); err == nil {
				baseFile.Seek(0, 0)
				if bp, err := parser.GetParser(detected); err == nil {
					baseReport, _ = bp.Parse(baseFile)
					if baseReport != nil {
						fmt.Printf("Loaded base coverage from: %s (%.2f%%)\n", *baseCoverageFile, baseReport.Coverage)
					}
				}
			}
		} else {
			fmt.Fprintf(os.Stderr, "Warning: Failed to open base coverage file: %v\n", err)
		}
	}

	gh := github.NewClient(token, owner, repo)

	var changedFiles []string
	if (*showFiles == "changed" || *annotations) && prNumber > 0 {
		changedFiles, err = gh.GetChangedFiles(prNumber)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: Failed to get changed files: %v\n", err)
		}
	}

	if *annotations {
		// Force output to be visible in logs - use fmt.Println which goes to stdout
		fmt.Println("===== ANNOTATION START V2 =====")
		fmt.Printf("Report has %d files, changedFiles has %d entries\n", len(report.Files), len(changedFiles))
		outputAnnotations(report, changedFiles)
		fmt.Println("===== ANNOTATION END V2 =====")
	}

	repoURL := fmt.Sprintf("https://github.com/%s", repository)
	opts := comment.Options{
		Title:        *title,
		ShowFiles:    *showFiles,
		ChangedFiles: changedFiles,
		RepoURL:      repoURL,
		SHA:          sha,
		PRNumber:     prNumber,
		BaseBranch:   *baseBranch,
	}
	if strings.HasPrefix(*showFiles, "threshold:") {
		val, _ := strconv.ParseFloat(strings.TrimPrefix(*showFiles, "threshold:"), 64)
		opts.Threshold = val
	}
	if strings.HasPrefix(*showFiles, "worst:") {
		val, _ := strconv.Atoi(strings.TrimPrefix(*showFiles, "worst:"))
		opts.WorstN = val
	}

	// Generate comment with or without comparison
	var commentBody string
	if baseReport != nil {
		comp := coverage.NewComparison(report, baseReport, changedFiles)
		commentBody = comment.FormatWithComparison(comp, opts)
	} else {
		commentBody = comment.Format(report, opts)
	}

	if prNumber > 0 {
		existingID, _ := gh.FindExistingComment(prNumber, comment.Marker)
		if existingID > 0 {
			fmt.Printf("Updating existing comment (ID: %d)\n", existingID)
			err = gh.UpdateComment(existingID, commentBody)
		} else {
			fmt.Println("Creating new comment")
			err = gh.CreateComment(prNumber, commentBody)
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to post comment: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("Coverage comment posted successfully")
	} else {
		fmt.Println("No PR number found, skipping comment")
	}

	if sha != "" {
		state := "success"
		description := fmt.Sprintf("%.2f%% coverage", report.Coverage)
		if *threshold > 0 && report.Coverage < *threshold {
			state = "failure"
			description = fmt.Sprintf("%.2f%% coverage (minimum: %.2f%%)", report.Coverage, *threshold)
		}
		if err := gh.SetCommitStatus(sha, state, description, "litecov"); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: Failed to set commit status: %v\n", err)
		} else {
			fmt.Printf("Commit status set: %s - %s\n", state, description)
		}
	}

	fmt.Printf("\nCoverage: %.2f%%\n", report.Coverage)
	fmt.Printf("Lines: %d/%d\n", report.TotalCovered, report.TotalLines)
	fmt.Printf("Files: %d\n", len(report.Files))

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

	if *threshold > 0 && report.Coverage < *threshold {
		fmt.Fprintf(os.Stderr, "\nCoverage %.2f%% is below threshold %.2f%%\n", report.Coverage, *threshold)
		os.Exit(1)
	}
}

func getPRNumber(eventPath string) (int, error) {
	if eventPath == "" {
		return 0, nil
	}
	data, err := os.ReadFile(eventPath)
	if err != nil {
		return 0, nil
	}

	content := string(data)
	if idx := strings.Index(content, `"number":`); idx >= 0 {
		start := idx + 9
		for start < len(content) && (content[start] == ' ' || content[start] == '\t') {
			start++
		}
		end := start
		for end < len(content) && content[end] >= '0' && content[end] <= '9' {
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
		"coverage/coverage.xml",
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	return ""
}

func outputAnnotations(report *coverage.Report, changedFiles []string) {
	fmt.Println("+++++ INSIDE outputAnnotations V2 +++++")
	changedSet := make(map[string]bool)
	for _, f := range changedFiles {
		changedSet[f] = true
	}

	fmt.Printf("Processing %d files in report\n", len(report.Files))

	annotationCount := 0
	for i, file := range report.Files {
		// Normalize path: strip Go module prefix to get repo-relative path
		// Coverage paths may be like "github.com/user/repo/internal/foo.go"
		// but we need "internal/foo.go" for GitHub annotations
		relativePath := normalizePathForAnnotation(file.Path)

		fmt.Printf("File[%d]: %s -> %s (uncovered: %d)\n", i, file.Path, relativePath, len(file.UncoveredLines))

		// Check if file is in changed set (use normalized path for matching)
		if len(changedFiles) > 0 && !isPathInChangedSet(relativePath, changedSet) {
			continue
		}

		if len(file.UncoveredLines) == 0 {
			continue
		}

		ranges := comment.GroupConsecutiveLines(file.UncoveredLines)
		for _, r := range ranges {
			annotationCount++
			if r.Start == r.End {
				fmt.Printf("::warning file=%s,line=%d,title=Uncovered::Line %d not covered by tests\n",
					relativePath, r.Start, r.Start)
			} else {
				fmt.Printf("::warning file=%s,line=%d,endLine=%d,title=Uncovered::Lines %d-%d not covered by tests\n",
					relativePath, r.Start, r.End, r.Start, r.End)
			}
		}
	}
	fmt.Printf("Total annotations emitted: %d\n", annotationCount)
}

// normalizePathForAnnotation converts a Go module path to a repo-relative path
// e.g., "github.com/user/repo/internal/foo.go" -> "internal/foo.go"
func normalizePathForAnnotation(path string) string {
	// Common Go module path patterns to strip
	// Look for known directory markers that indicate repo structure
	markers := []string{"/internal/", "/cmd/", "/pkg/", "/api/", "/test/", "/tests/"}
	for _, marker := range markers {
		if idx := strings.Index(path, marker); idx >= 0 {
			return path[idx+1:] // +1 to skip the leading slash
		}
	}
	// If no marker found but path contains github.com or similar,
	// try to extract after the third slash (github.com/user/repo/...)
	parts := strings.SplitN(path, "/", 4)
	if len(parts) == 4 && (strings.Contains(parts[0], ".") || parts[0] == "github") {
		return parts[3]
	}
	return path
}

// isPathInChangedSet checks if the given path matches any file in the changed set
func isPathInChangedSet(path string, changedSet map[string]bool) bool {
	if changedSet[path] {
		return true
	}
	// Also try suffix matching for paths that may have different prefixes
	for changedPath := range changedSet {
		if strings.HasSuffix(path, changedPath) || strings.HasSuffix(changedPath, path) {
			return true
		}
	}
	return false
}
