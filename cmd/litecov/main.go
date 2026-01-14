package main

import (
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/litecov/litecov/internal/comment"
	"github.com/litecov/litecov/internal/github"
	"github.com/litecov/litecov/internal/parser"
)

func main() {
	coverageFile := flag.String("coverage-file", "", "Path to coverage report file")
	format := flag.String("format", "auto", "Coverage format: auto, lcov, cobertura")
	showFiles := flag.String("show-files", "changed", "Files to show: all, changed, threshold:N, worst:N")
	threshold := flag.Float64("threshold", 0, "Minimum coverage threshold for passing status")
	title := flag.String("title", "Coverage Report", "Comment title")
	flag.Parse()

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

	gh := github.NewClient(token, owner, repo)

	var changedFiles []string
	if *showFiles == "changed" && prNumber > 0 {
		changedFiles, err = gh.GetChangedFiles(prNumber)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: Failed to get changed files: %v\n", err)
		}
	}

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

	commentBody := comment.Format(report, opts)

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
