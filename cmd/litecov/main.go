package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/manashmandal/litecov/internal/comment"
	"github.com/manashmandal/litecov/internal/coverage"
	"github.com/manashmandal/litecov/internal/diff"
	"github.com/manashmandal/litecov/internal/github"
	"github.com/manashmandal/litecov/internal/parser"
	"github.com/manashmandal/litecov/internal/paths"
)

func main() {
	coverageFile := flag.String("coverage-file", "", "Path to coverage report file")
	format := flag.String("format", "auto", "Coverage format: auto, lcov, cobertura, go")
	showFiles := flag.String("show-files", "changed", "Files to show: all, changed, threshold:N, worst:N")
	threshold := flag.Float64("threshold", 0, "Minimum coverage threshold for passing status")
	patchThreshold := flag.Float64("patch-threshold", 0, "Minimum patch coverage threshold for passing status")
	goodThreshold := flag.Float64("good-threshold", 0, "Coverage % at/above which the report shows a passing status (0 = default of 80)")
	warnThreshold := flag.Float64("warn-threshold", 0, "Coverage % at/above which the report shows a warning instead of a failing status (0 = default of 50)")
	title := flag.String("title", "Coverage Report", "Comment title")
	flagName := flag.String("flag", "", "Identifies this invocation when litecov runs more than once on one PR, scoping the comment marker and commit status context")
	annotations := flag.Bool("annotations", false, "Output GitHub annotations for uncovered lines")
	baseCoverageFile := flag.String("base-coverage-file", "", "Path to base branch coverage file for comparison")
	baseBranch := flag.String("base-branch", "", "Base branch name for comparison display")
	pathPrefix := flag.String("path-prefix", "", "Prefix to strip from every coverage report path, e.g. \"backend/\"")
	pathFixesInput := flag.String("path-fixes", "", "Newline separated \"before::after\" path rewrite rules, matching Codecov's fixes:")
	flag.Parse()

	// Environment variable overrides for GitHub Action
	if *baseCoverageFile == "" {
		*baseCoverageFile = os.Getenv("INPUT_BASE_COVERAGE_FILE")
	}
	*baseBranch = resolveBaseBranch(*baseBranch, os.Getenv("INPUT_BASE_BRANCH"), os.Getenv("GITHUB_BASE_REF"))
	if *pathPrefix == "" {
		*pathPrefix = os.Getenv("INPUT_PATH_PREFIX")
	}
	if *pathFixesInput == "" {
		*pathFixesInput = os.Getenv("INPUT_PATH_FIXES")
	}
	if *flagName == "" {
		*flagName = os.Getenv("INPUT_FLAG")
	}
	*goodThreshold = resolveThreshold(*goodThreshold, os.Getenv("INPUT_GOOD_THRESHOLD"))
	*warnThreshold = resolveThreshold(*warnThreshold, os.Getenv("INPUT_WARN_THRESHOLD"))
	pathFixRules := paths.ParsePathFixes(*pathFixesInput)

	token := os.Getenv("GITHUB_TOKEN")
	repository := os.Getenv("GITHUB_REPOSITORY")
	eventPath := os.Getenv("GITHUB_EVENT_PATH")
	eventName := os.Getenv("GITHUB_EVENT_NAME")
	sha := os.Getenv("GITHUB_SHA")
	runID := os.Getenv("GITHUB_RUN_ID")
	// GITHUB_API_URL and GITHUB_SERVER_URL are "https://api.github.com" and
	// "https://github.com" on github.com runners, but point at the
	// enterprise host instead on GitHub Enterprise Server (issue #49).
	apiBaseURL := resolveGitHubHost(os.Getenv("GITHUB_API_URL"), "https://api.github.com")
	serverURL := resolveGitHubHost(os.Getenv("GITHUB_SERVER_URL"), "https://github.com")
	// The commit statuses' target_url, the check row's "Details" link.
	// Built from serverURL (already GHES-correct) and repository rather than
	// raw env reads, so a check on GitHub Enterprise Server links back to the
	// enterprise host instead of github.com (issue #48).
	targetURL := commitStatusTargetURL(serverURL, repository, runID)

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

	prNumber, err := getPRNumber(eventPath, eventName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to get PR number: %v\n", err)
		os.Exit(1)
	}

	// GITHUB_SHA is the merge commit on a pull_request-shaped event, not the
	// PR's head commit; prefer the head SHA out of the event payload when
	// one is available and leave sha as GITHUB_SHA otherwise (issue #33).
	event, err := loadEvent(eventPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to read event payload for head SHA: %v\n", err)
	} else {
		sha = resolveSHA(sha, event)
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
		p, _ = parser.GetParserWithPath(detected, *coverageFile)
	} else {
		p, err = parser.GetParserWithPath(*format, *coverageFile)
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
	normalizeReportPaths(report.Files, *pathPrefix, pathFixRules)

	// Parse base coverage if provided. baseErr is nil both when no base was
	// requested and when one was requested and loaded fine; it's non-nil
	// only for a base that was requested but couldn't be read, which is
	// what lets the comment say so instead of looking identical to no base
	// having been configured at all (issue #39).
	baseReport, baseErr := loadBaseReport(*baseCoverageFile, *pathPrefix, pathFixRules)

	gh := github.NewClient(token, owner, repo, apiBaseURL)

	var changedFiles []string
	var addedFiles map[string]bool
	var removedFiles map[string]bool
	var patchedLines map[string][]diff.LineRange
	if *showFiles == "changed" && prNumber > 0 {
		var changed []github.ChangedFile
		changed, err = gh.GetChangedFiles(prNumber)
		if err != nil {
			// A missing pull-requests: read permission, a rate limit, or a
			// fork PR with a restricted token all land here. Continuing with
			// a nil slice used to make the "changed" filter fall back to
			// every file in the report, which posts a comment that looks
			// like a normal, correctly-scoped run (issue #20). Fail instead
			// of posting a report whose scope silently doesn't match what
			// show-files: changed promises.
			fmt.Fprintf(os.Stderr, "Failed to get changed files: %v\n", err)
			os.Exit(1)
		}
		// GitHub's changed-file paths are already clean, but normalize them
		// too so both sides of every later comparison went through the
		// same rules (issue #19). addedFiles and removedFiles are keyed the
		// same way so NewComparison's IsNew (issue #32) and IsDeleted
		// (issue #31) lookups line up.
		changedFiles = make([]string, len(changed))
		addedFiles = make(map[string]bool, len(changed))
		removedFiles = make(map[string]bool, len(changed))
		patchedLines = make(map[string][]diff.LineRange, len(changed))
		for i, cf := range changed {
			normalized := paths.NormalizeCoveragePath(cf.Path)
			changedFiles[i] = normalized
			if cf.IsAdded {
				addedFiles[normalized] = true
			}
			if cf.IsRemoved {
				removedFiles[normalized] = true
			}
			// cf.Patch is empty for a binary file, a rename with no content
			// change, or a diff past GitHub's per-file size limit;
			// ParseFilePatch already treats that as "no coverable changed
			// lines" and returns nil, so patchedLines just has no entry for
			// the file rather than a wrong one (issue #6).
			if lines := diff.ParseFilePatch(normalized, cf.Patch); len(lines) > 0 {
				patchedLines[normalized] = lines
			}
		}
	}

	if *annotations {
		// Only filter annotations by changed files if show-files is "changed"
		annotationFiles := changedFiles
		if *showFiles != "changed" {
			annotationFiles = nil // nil means show all files
		}
		outputAnnotations(report, annotationFiles)
	}

	repoURL := fmt.Sprintf("%s/%s", serverURL, repository)
	opts := comment.Options{
		Title:         *title,
		Flag:          *flagName,
		ShowFiles:     *showFiles,
		ChangedFiles:  changedFiles,
		RepoURL:       repoURL,
		SHA:           sha,
		PRNumber:      prNumber,
		BaseBranch:    *baseBranch,
		PatchCoverage: coverage.CalculatePatchCoverage(report, patchedLines),
		FilePatches:   coverage.CalculateFilePatchCoverage(report, patchedLines),
		GoodThreshold: *goodThreshold,
		WarnThreshold: *warnThreshold,
	}
	if baseErr != nil {
		opts.BaseError = baseErr.Error()
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
		comp := coverage.NewComparison(report, baseReport, changedFiles, addedFiles, removedFiles)
		commentBody = comment.FormatWithComparison(comp, opts)
	} else {
		commentBody = comment.Format(report, opts)
	}

	if prNumber > 0 {
		if err := postCoverageComment(gh, prNumber, comment.MarkerFor(*flagName), commentBody); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to post comment: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("Coverage comment posted successfully")
	} else {
		fmt.Println(noPRNumberWarning(eventName))
	}

	if sha != "" {
		state, description := commitStatusForCoverage(report.Coverage, *threshold)
		if err := gh.SetCommitStatus(sha, state, description, commitStatusContext(*flagName), targetURL); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: Failed to set commit status: %v\n", err)
		} else {
			fmt.Printf("Commit status set: %s - %s\n", state, description)
		}

		// A second, independent status for patch coverage, Codecov's
		// codecov/patch check (issue #10). Project coverage barely moves for
		// a PR that adds a few hundred untested lines to a large, well
		// covered repo, so litecov alone could never catch that; this status
		// is scoped to just the lines the PR added.
		patchState, patchDescription := commitStatusForPatchCoverage(opts.PatchCoverage, *patchThreshold)
		if err := gh.SetCommitStatus(sha, patchState, patchDescription, patchCommitStatusContext(*flagName), targetURL); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: Failed to set patch commit status: %v\n", err)
		} else {
			fmt.Printf("Patch commit status set: %s - %s\n", patchState, patchDescription)
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

	// patch.Total == 0 is guarded the same way commitStatusForPatchCoverage
	// guards it: nothing to measure must not be treated as a failing score.
	if *patchThreshold > 0 && opts.PatchCoverage.Total > 0 && opts.PatchCoverage.Percentage() < *patchThreshold {
		fmt.Fprintf(os.Stderr, "\nPatch coverage %.2f%% is below threshold %.2f%%\n", opts.PatchCoverage.Percentage(), *patchThreshold)
		os.Exit(1)
	}
}

// postCoverageComment creates a new PR comment, or updates the existing one
// if a previous run already left one starting with marker.
//
// FindExistingComment returns (0, err) on a transport failure, a 403, a
// 5xx, or a malformed body -- the same zero ID it returns for "no comment
// exists yet". Falling through to CreateComment on that error used to turn
// any transient lookup failure into a second, permanent coverage comment on
// the PR (issue #37).
func postCoverageComment(gh *github.Client, prNumber int, marker, body string) error {
	existingID, err := gh.FindExistingComment(prNumber, marker)
	if err != nil {
		return fmt.Errorf("looking up existing comment: %w", err)
	}
	if existingID > 0 {
		fmt.Printf("Updating existing comment (ID: %d)\n", existingID)
		return gh.UpdateComment(existingID, body)
	}
	fmt.Println("Creating new comment")
	return gh.CreateComment(prNumber, body)
}

// commitStatusForCoverage decides the state and description for the
// project-wide "litecov" commit status. threshold <= 0 means no threshold
// was configured, so the status is always "success".
func commitStatusForCoverage(coverage, threshold float64) (state, description string) {
	if threshold > 0 && coverage < threshold {
		return "failure", fmt.Sprintf("%.2f%% coverage (minimum: %.2f%%)", coverage, threshold)
	}
	return "success", fmt.Sprintf("%.2f%% coverage", coverage)
}

// commitStatusForPatchCoverage decides the state and description for the
// "litecov/patch" commit status, Codecov's codecov/patch check
// (https://docs.codecov.com/docs/commit-status): the coverage of only the
// lines a PR added, as opposed to commitStatusForCoverage's whole-project
// number. Before this, a PR that added hundreds of untested lines to a
// large, well covered repo could move project coverage by a fraction of a
// percent and still pass (issue #10).
//
// patch.Total == 0 means there was nothing to measure -- no PR diff was
// available, or the diff touched no coverable line -- so this reports
// success rather than a 0% that was never actually computed, the same
// sentinel formatPatchStatusLine applies to the PR comment (issue #6).
// threshold <= 0 means patch-threshold wasn't configured, so a measured
// patch is always "success" too.
func commitStatusForPatchCoverage(patch coverage.PatchCoverage, threshold float64) (state, description string) {
	if patch.Total == 0 {
		return "success", "no coverable changes in this patch"
	}
	pct := patch.Percentage()
	if threshold > 0 && pct < threshold {
		return "failure", fmt.Sprintf("%.2f%% patch coverage (minimum: %.2f%%)", pct, threshold)
	}
	return "success", fmt.Sprintf("%.2f%% patch coverage", pct)
}

// commitStatusContext returns the status context for the project-wide
// coverage commit status: "litecov" by default, or "litecov/<flagName>"
// when the flag input scopes this invocation to run alongside others
// against the same commit. Before this, the context was the fixed string
// literal "litecov", so a second litecov step in one workflow -- the normal
// setup for a monorepo -- posted its status under the same context as the
// first, and the last one to finish silently overwrote it (issue #54).
func commitStatusContext(flagName string) string {
	if flagName == "" {
		return "litecov"
	}
	return "litecov/" + flagName
}

// patchCommitStatusContext is commitStatusContext's counterpart for the
// patch coverage commit status: "litecov/patch" by default, or
// "litecov/patch/<flagName>" when scoped the same way (issue #54).
func patchCommitStatusContext(flagName string) string {
	if flagName == "" {
		return "litecov/patch"
	}
	return "litecov/patch/" + flagName
}

// commitStatusTargetURL builds the target_url shared by both commit
// statuses: the link GitHub shows as the check row's "Details" button.
// Before this, SetCommitStatus never set target_url at all, so the check
// showed a description like "73.12% coverage (minimum: 80.00%)" and
// dead-ended there, with no way to get from a failing check to the report,
// the comment, or the job log (issue #48). It points at the Actions run
// rather than the PR comment because the "litecov" status is set whenever
// sha is non-empty, regardless of whether a PR (and so a comment) exists.
// runID is GITHUB_RUN_ID, which GitHub Actions sets on every run; it's only
// empty on a direct binary invocation outside Actions, the one case this
// returns "" for -- SetCommitStatus treats "" as "omit target_url" rather
// than send GitHub a broken link.
func commitStatusTargetURL(serverURL, repository, runID string) string {
	if runID == "" {
		return ""
	}
	return fmt.Sprintf("%s/%s/actions/runs/%s", serverURL, repository, runID)
}

// resolveThreshold decides the value for a good-threshold/warn-threshold
// flag: flagValue as-is when it's already non-zero, otherwise inputEnvValue
// (the matching INPUT_GOOD_THRESHOLD/INPUT_WARN_THRESHOLD action input)
// parsed as a float. Both flags default to 0, "not configured," so this
// only ever overrides that default -- it can't be told apart from a direct
// invocation that explicitly passed 0, the same ambiguity *threshold and
// *patchThreshold already live with. inputEnvValue that fails to parse
// (empty, because the input wasn't set, or malformed) leaves flagValue
// untouched rather than erroring, so a comment.Options field simply stays
// at its own "not configured" default of 0 (issue #80).
func resolveThreshold(flagValue float64, inputEnvValue string) float64 {
	if flagValue != 0 {
		return flagValue
	}
	val, err := strconv.ParseFloat(inputEnvValue, 64)
	if err != nil {
		return flagValue
	}
	return val
}

// resolveBaseBranch decides the base branch name shown in the diff header.
// inputBaseBranch (INPUT_BASE_BRANCH, the action.yml input) wins when set,
// matching how every other INPUT_ override in main already takes priority
// over its flag. flagValue -- the -base-branch flag, which entrypoint.sh
// never sets but a direct binary invocation might -- is next. githubBaseRef
// (GITHUB_BASE_REF), which GitHub Actions sets automatically on every
// pull_request and pull_request_target event, is consulted last and used to
// never be read at all: with neither override set, the base branch was just
// the flag's hardcoded "main" default, which named the wrong branch for any
// repo whose default branch isn't main (issue #75). Returns "" when none of
// the three resolved to anything, so the caller can say the base is unknown
// instead of guessing.
func resolveBaseBranch(flagValue, inputBaseBranch, githubBaseRef string) string {
	if inputBaseBranch != "" {
		return inputBaseBranch
	}
	if flagValue != "" {
		return flagValue
	}
	return githubBaseRef
}

// resolveGitHubHost decides a GitHub host URL from the environment variable
// GitHub Actions exports for it (envValue), falling back to defaultValue --
// the github.com host -- when envValue is empty. GITHUB_API_URL and
// GITHUB_SERVER_URL are both set on every Actions run, github.com ones
// included, so an empty envValue only happens on a direct binary invocation
// outside Actions. litecov used to hardcode the github.com host for both, so
// on GitHub Enterprise Server -- where GITHUB_API_URL is
// "https://HOSTNAME/api/v3" instead -- it sent the enterprise GITHUB_TOKEN to
// api.github.com, where the token isn't valid (issue #49).
func resolveGitHubHost(envValue, defaultValue string) string {
	if envValue != "" {
		return envValue
	}
	return defaultValue
}

// gitHubEventPayload is the subset of a GitHub Actions event payload that
// carries a pull request number and, for events triggered directly by a
// pull request, its head commit SHA. Where the number lives depends on the
// event type: a top-level "number" for pull_request and
// pull_request_target, "pull_request.number" for pull_request_review, and
// "check_suite.pull_requests[0].number" for check_suite. The head SHA, when
// the event has one, is always at "pull_request.head.sha" (issue #33).
type gitHubEventPayload struct {
	Number      int `json:"number"`
	PullRequest struct {
		Number int `json:"number"`
		Head   struct {
			SHA string `json:"sha"`
		} `json:"head"`
	} `json:"pull_request"`
	CheckSuite struct {
		PullRequests []struct {
			Number int `json:"number"`
		} `json:"pull_requests"`
	} `json:"check_suite"`
}

// loadEvent reads and parses the GITHUB_EVENT_PATH payload at eventPath.
// Returns a zero-value payload, not an error, for an empty eventPath -- a
// direct binary invocation outside Actions -- since that's not a failure,
// just nothing to parse. err is non-nil only when eventPath was set but the
// file couldn't be read or didn't parse as JSON, so a caller can tell "no
// event data" apart from "the event data is broken."
func loadEvent(eventPath string) (gitHubEventPayload, error) {
	var event gitHubEventPayload
	if eventPath == "" {
		return event, nil
	}
	data, err := os.ReadFile(eventPath)
	if err != nil {
		return event, err
	}
	err = json.Unmarshal(data, &event)
	return event, err
}

// getPRNumber reads the PR number out of the GITHUB_EVENT_PATH payload,
// picking the field named eventName's shape (GITHUB_EVENT_NAME) puts it in
// rather than scanning the raw JSON text for the first "number" key. The
// text scan it replaces, strings.Index(content, `"number":`), returned
// whichever "number" appeared first in the payload -- wrong for an event
// like "milestone" whose "milestone.number" sits ahead of the pull request's
// own top-level "number" -- and missed the key entirely when GitHub's
// pretty-printed JSON put whitespace before the colon, `"number" : 42`
// instead of `"number":42`, leaving prNumber at 0 and the comment silently
// skipped on an otherwise green run (issue #53).
func getPRNumber(eventPath, eventName string) (int, error) {
	if eventPath == "" {
		return 0, nil
	}
	data, err := os.ReadFile(eventPath)
	if err != nil {
		return 0, nil
	}

	var event gitHubEventPayload
	if err := json.Unmarshal(data, &event); err != nil {
		return 0, nil
	}

	switch eventName {
	case "pull_request_review":
		return event.PullRequest.Number, nil
	case "check_suite":
		if len(event.CheckSuite.PullRequests) > 0 {
			return event.CheckSuite.PullRequests[0].Number, nil
		}
		return 0, nil
	default:
		return event.Number, nil
	}
}

// resolveSHA decides which commit SHA the PR comment's links and the commit
// statuses are attached to. On a pull_request-shaped event -- pull_request,
// pull_request_target, pull_request_review -- githubSHA (GITHUB_SHA) is not
// the PR's head commit, it's the ephemeral merge commit on
// refs/pull/N/merge. That commit isn't part of the PR branch, so a status
// posted against it never appears in the PR's checks list and can never be
// picked as a required status check under branch protection, and a blob or
// line link built from it points at a commit the PR doesn't contain
// (issue #33). event.PullRequest.Head.SHA -- present in the event payload
// for exactly those event types -- is the actual PR head commit and wins
// whenever it's set. A push or any other event whose payload has no
// "pull_request" object leaves it empty, so githubSHA is returned
// unchanged, which is already correct there.
func resolveSHA(githubSHA string, event gitHubEventPayload) string {
	if event.PullRequest.Head.SHA != "" {
		return event.PullRequest.Head.SHA
	}
	return githubSHA
}

// noPRNumberWarning formats the line main prints when getPRNumber resolved
// to 0, so there's no PR to comment on. It's a GitHub Actions ::warning::
// workflow command instead of a plain fmt.Println, so it lands in the run's
// Annotations panel rather than being one more line in a log nobody reads.
// Before this, the message was silent twice over: the "litecov" commit
// status only needs a SHA and posts regardless of whether a PR was found,
// so the job still went green, and the log line read exactly like a normal
// run finishing its work. That combination is how a workflow-trigger
// regression that dropped every PR comment on this repo went three weeks
// unnoticed (issue #82).
func noPRNumberWarning(eventName string) string {
	return fmt.Sprintf("::warning title=No Pull Request::No pull request found for this run (event: %s); litecov posts comments on pull_request events, see README for supported triggers", eventName)
}

// normalizeReportPaths rewrites every file path in files so coverage-tool
// quirks (Windows separators, an unclean ".." segment, an absolute
// GITHUB_WORKSPACE prefix, or the subdirectory the report was generated in)
// don't stop them from matching GitHub's changed-file paths later on.
// prefix and fixes come from the path-prefix and path-fixes inputs and are
// both optional (issue #19).
func normalizeReportPaths(files []coverage.FileCoverage, prefix string, fixes []paths.PathFix) {
	for i := range files {
		files[i].Path = paths.NormalizeAndFixPath(files[i].Path, prefix, fixes)
	}
}

// loadBaseReport parses the base coverage file at path the same way the head
// report is parsed: format auto-detected, then handed to a parser built with
// GetParserWithPath so a relative SF: path picks up a source prefix from
// path the same way the head parser's does from *coverageFile. Building the
// base parser with plain GetParser (no path) instead used to leave the base
// report's paths unprefixed while the head report's carried the coverage
// file's directory as a prefix, so identical LCOV input produced different
// paths on each side and NewComparison's lookup missed every file, each one
// showing up as unmatched on both the head and base side of the comparison
// (issue #29).
//
// Returns (nil, nil) when path is empty: no base comparison was requested.
// Returns (nil, err) when a base *was* requested but couldn't be turned into
// a report -- the file wouldn't open, its format couldn't be detected, or it
// failed to parse. Every one of those used to return (nil, nil) the same as
// the empty-path case, with only the open failure even reaching stderr, so a
// misconfigured or corrupt base file made the comparison silently disappear
// from the PR comment instead of explaining why (issue #39). err is logged
// here so the reason shows up in the run's logs, and the caller threads it
// into the PR comment so it shows up there too.
func loadBaseReport(path, pathPrefix string, fixes []paths.PathFix) (*coverage.Report, error) {
	if path == "" {
		return nil, nil
	}
	baseFile, err := os.Open(path)
	if err != nil {
		err = fmt.Errorf("opening base coverage file: %w", err)
		fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
		return nil, err
	}
	defer baseFile.Close()

	detected, err := parser.DetectFormat(baseFile)
	if err != nil {
		err = fmt.Errorf("detecting base coverage format: %w", err)
		fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
		return nil, err
	}
	baseFile.Seek(0, 0)

	bp, err := parser.GetParserWithPath(detected, path)
	if err != nil {
		err = fmt.Errorf("getting a %s parser for the base coverage file: %w", detected, err)
		fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
		return nil, err
	}

	baseReport, err := bp.Parse(baseFile)
	if err != nil {
		err = fmt.Errorf("parsing base coverage file: %w", err)
		fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
		return nil, err
	}

	normalizeReportPaths(baseReport.Files, pathPrefix, fixes)
	fmt.Printf("Loaded base coverage from: %s (%.2f%%)\n", path, baseReport.Coverage)
	return baseReport, nil
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
	changedSet := make(map[string]bool)
	for _, f := range changedFiles {
		changedSet[f] = true
	}

	// Track which changed files have coverage data
	coveredChangedFiles := make(map[string]bool)

	for _, file := range report.Files {
		// Normalize path: strip Go module prefix to get repo-relative path
		// Coverage paths may be like "github.com/user/repo/internal/foo.go"
		// but we need "internal/foo.go" for GitHub annotations
		relativePath := paths.NormalizePathForAnnotation(file.Path)

		// Check if file is in changed set (use normalized path for matching)
		matchedPath := ""
		if len(changedFiles) > 0 {
			matchedPath = paths.FindMatchingChangedFile(relativePath, changedSet)
			if matchedPath == "" {
				continue
			}
			coveredChangedFiles[matchedPath] = true
		}

		if len(file.UncoveredLines) == 0 {
			continue
		}

		annotationPath := relativePath
		if matchedPath != "" {
			annotationPath = matchedPath
		}

		ranges := comment.GroupConsecutiveLines(file.UncoveredLines)
		for _, r := range ranges {
			if r.Start == r.End {
				fmt.Printf("::warning file=%s,line=%d,title=Uncovered::Line %d not covered by tests\n",
					annotationPath, r.Start, r.Start)
			} else {
				fmt.Printf("::warning file=%s,line=%d,endLine=%d,title=Uncovered::Lines %d-%d not covered by tests\n",
					annotationPath, r.Start, r.End, r.Start, r.End)
			}
		}
	}

	// Output annotations for changed files that have no coverage data at all
	// These are files that were never executed by any test
	for _, changedFile := range changedFiles {
		if coveredChangedFiles[changedFile] {
			continue
		}
		// Only annotate source files (skip test files, configs, etc.)
		if !paths.IsSourceFile(changedFile) {
			continue
		}
		fmt.Printf("::warning file=%s,line=1,title=No Coverage::File has no test coverage\n", changedFile)
	}
}
