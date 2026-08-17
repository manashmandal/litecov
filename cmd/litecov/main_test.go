package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/manashmandal/litecov/internal/coverage"
	"github.com/manashmandal/litecov/internal/diff"
	"github.com/manashmandal/litecov/internal/github"
	"github.com/manashmandal/litecov/internal/parser"
)

func TestCommitStatusForCoverage(t *testing.T) {
	tests := []struct {
		name            string
		coverage        float64
		threshold       float64
		wantState       string
		wantDescription string
	}{
		{
			name:            "no threshold configured always passes",
			coverage:        10,
			threshold:       0,
			wantState:       "success",
			wantDescription: "10.00% coverage",
		},
		{
			name:            "above threshold passes",
			coverage:        85,
			threshold:       80,
			wantState:       "success",
			wantDescription: "85.00% coverage",
		},
		{
			name:            "at threshold passes",
			coverage:        80,
			threshold:       80,
			wantState:       "success",
			wantDescription: "80.00% coverage",
		},
		{
			name:            "below threshold fails",
			coverage:        79.5,
			threshold:       80,
			wantState:       "failure",
			wantDescription: "79.50% coverage (minimum: 80.00%)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state, description := commitStatusForCoverage(tt.coverage, tt.threshold)
			if state != tt.wantState {
				t.Errorf("state = %q, want %q", state, tt.wantState)
			}
			if description != tt.wantDescription {
				t.Errorf("description = %q, want %q", description, tt.wantDescription)
			}
		})
	}
}

func TestCommitStatusForPatchCoverage(t *testing.T) {
	tests := []struct {
		name            string
		patch           coverage.PatchCoverage
		threshold       float64
		wantState       string
		wantDescription string
	}{
		{
			name:            "no threshold configured always passes",
			patch:           coverage.PatchCoverage{Covered: 0, Total: 10},
			threshold:       0,
			wantState:       "success",
			wantDescription: "0.00% patch coverage",
		},
		{
			name:            "above threshold passes",
			patch:           coverage.PatchCoverage{Covered: 9, Total: 10},
			threshold:       80,
			wantState:       "success",
			wantDescription: "90.00% patch coverage",
		},
		{
			name:            "below threshold fails",
			patch:           coverage.PatchCoverage{Covered: 1, Total: 10},
			threshold:       80,
			wantState:       "failure",
			wantDescription: "10.00% patch coverage (minimum: 80.00%)",
		},
		{
			// issue #6: Total == 0 means nothing was measured -- no PR diff,
			// or the diff touched no coverable line -- not a 0% patch. A
			// configured threshold must not fail a PR that had nothing to
			// test.
			name:            "no coverable patch lines always passes regardless of threshold",
			patch:           coverage.PatchCoverage{Covered: 0, Total: 0},
			threshold:       80,
			wantState:       "success",
			wantDescription: "no coverable changes in this patch",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state, description := commitStatusForPatchCoverage(tt.patch, tt.threshold)
			if state != tt.wantState {
				t.Errorf("state = %q, want %q", state, tt.wantState)
			}
			if description != tt.wantDescription {
				t.Errorf("description = %q, want %q", description, tt.wantDescription)
			}
		})
	}
}

// TestCommitStatusForPatchCoverage_CatchesWhatProjectMisses reproduces issue
// #10's repro case: a repo sitting at 85% coverage over 30,000 lines gets a
// PR that adds 200 fully untested lines. Project coverage lands around
// 84.44%, still above an 80% project threshold, so "litecov" reports
// success on its own -- exactly the false green the issue describes. Patch
// coverage for that same PR is 0%, which "litecov/patch" must catch since
// project coverage never will.
func TestCommitStatusForPatchCoverage_CatchesWhatProjectMisses(t *testing.T) {
	const projectThreshold = 80.0

	// 30,000 lines at 85% before the PR (25,500 covered), plus 200 new,
	// wholly uncovered lines.
	projectCoverage := 25500.0 / 30200.0 * 100
	if state, description := commitStatusForCoverage(projectCoverage, projectThreshold); state != "success" {
		t.Fatalf("test setup invalid: project status = %q (%s), want success -- that false green is the bug issue #10 reports", state, description)
	}

	patch := coverage.PatchCoverage{Covered: 0, Total: 200}
	state, description := commitStatusForPatchCoverage(patch, projectThreshold)
	if state != "failure" {
		t.Errorf("patch state = %q, want failure: a passing project status must not mask 0%% patch coverage", state)
	}
	wantDescription := "0.00% patch coverage (minimum: 80.00%)"
	if description != wantDescription {
		t.Errorf("patch description = %q, want %q", description, wantDescription)
	}
}

// TestResolveBaseBranch reproduces issue #75: the base branch name was a
// hardcoded "main" literal, and GITHUB_BASE_REF -- which GitHub Actions sets
// automatically on every pull_request and pull_request_target event -- was
// never read. resolveBaseBranch must prefer an explicit override but fall
// back to GITHUB_BASE_REF, and return "" rather than inventing "main" when
// nothing resolved at all.
func TestResolveBaseBranch(t *testing.T) {
	tests := []struct {
		name            string
		flagValue       string
		inputBaseBranch string
		githubBaseRef   string
		want            string
	}{
		{
			name:            "falls back to GITHUB_BASE_REF when nothing else is set",
			flagValue:       "",
			inputBaseBranch: "",
			githubBaseRef:   "master",
			want:            "master",
		},
		{
			name:            "INPUT_BASE_BRANCH wins over GITHUB_BASE_REF",
			flagValue:       "",
			inputBaseBranch: "develop",
			githubBaseRef:   "master",
			want:            "develop",
		},
		{
			name:            "flag wins over GITHUB_BASE_REF",
			flagValue:       "release",
			inputBaseBranch: "",
			githubBaseRef:   "master",
			want:            "release",
		},
		{
			name:            "INPUT_BASE_BRANCH wins over the flag",
			flagValue:       "release",
			inputBaseBranch: "develop",
			githubBaseRef:   "master",
			want:            "develop",
		},
		{
			name:            "nothing resolved returns empty rather than a guessed main",
			flagValue:       "",
			inputBaseBranch: "",
			githubBaseRef:   "",
			want:            "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveBaseBranch(tt.flagValue, tt.inputBaseBranch, tt.githubBaseRef)
			if got != tt.want {
				t.Errorf("resolveBaseBranch(%q, %q, %q) = %q, want %q", tt.flagValue, tt.inputBaseBranch, tt.githubBaseRef, got, tt.want)
			}
		})
	}
}

// TestResolveThreshold guards issue #80: good-threshold and warn-threshold
// used to have no way to reach comment.Options at all, so
// good-threshold: 95 in a workflow's `with:` block was silently ignored.
func TestResolveThreshold(t *testing.T) {
	tests := []struct {
		name          string
		flagValue     float64
		inputEnvValue string
		want          float64
	}{
		{
			name:          "INPUT_ env var is used when the flag is unset",
			flagValue:     0,
			inputEnvValue: "95",
			want:          95,
		},
		{
			name:          "flag wins over the INPUT_ env var",
			flagValue:     90,
			inputEnvValue: "95",
			want:          90,
		},
		{
			name:          "neither set stays at 0, comment.Options' own default applies",
			flagValue:     0,
			inputEnvValue: "",
			want:          0,
		},
		{
			name:          "an unparseable INPUT_ env var is ignored, not an error",
			flagValue:     0,
			inputEnvValue: "not-a-number",
			want:          0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveThreshold(tt.flagValue, tt.inputEnvValue)
			if got != tt.want {
				t.Errorf("resolveThreshold(%v, %q) = %v, want %v", tt.flagValue, tt.inputEnvValue, got, tt.want)
			}
		})
	}
}

// TestParseShowFiles guards issue #85. threshold: and worst: each used to
// discard their strconv parse error, so an unparseable suffix silently left
// the corresponding comment.Options field at 0 instead of failing the run,
// and a value that matched neither prefix and wasn't "all" or "changed"
// either fell all the way through filterFiles' default case and rendered
// every file, the same as show-files: all.
func TestParseShowFiles(t *testing.T) {
	tests := []struct {
		name          string
		showFiles     string
		wantThreshold float64
		wantWorstN    int
		wantErr       bool
	}{
		{name: "all", showFiles: "all"},
		{name: "changed", showFiles: "changed"},
		{name: "valid threshold", showFiles: "threshold:80", wantThreshold: 80},
		{name: "threshold at lower bound", showFiles: "threshold:0", wantThreshold: 0},
		{name: "threshold at upper bound", showFiles: "threshold:100", wantThreshold: 100},
		{name: "threshold non-numeric renders an empty table without this", showFiles: "threshold:abc", wantErr: true},
		{name: "threshold below zero", showFiles: "threshold:-1", wantErr: true},
		{name: "threshold above 100", showFiles: "threshold:150", wantErr: true},
		{name: "threshold NaN", showFiles: "threshold:NaN", wantErr: true},
		{name: "valid worst", showFiles: "worst:2", wantWorstN: 2},
		{name: "worst non-numeric renders an empty table without this", showFiles: "worst:abc", wantErr: true},
		{name: "worst with trailing garbage after the leading digit", showFiles: "worst:3abc", wantErr: true},
		{name: "worst zero", showFiles: "worst:0", wantErr: true},
		{name: "worst negative", showFiles: "worst:-1", wantErr: true},
		{name: "unrecognized mode used to silently behave like all", showFiles: "bogusmode", wantErr: true},
		{name: "empty value", showFiles: "", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotThreshold, gotWorstN, err := parseShowFiles(tt.showFiles)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("parseShowFiles(%q) returned no error, want one", tt.showFiles)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseShowFiles(%q) returned unexpected error: %v", tt.showFiles, err)
			}
			if gotThreshold != tt.wantThreshold {
				t.Errorf("threshold = %v, want %v", gotThreshold, tt.wantThreshold)
			}
			if gotWorstN != tt.wantWorstN {
				t.Errorf("worstN = %v, want %v", gotWorstN, tt.wantWorstN)
			}
		})
	}
}

// TestCommitStatusTargetURL guards issue #48: SetCommitStatus never sent
// target_url, so the check row on a PR had no "Details" link back to the
// run that produced it.
func TestCommitStatusTargetURL(t *testing.T) {
	tests := []struct {
		name       string
		serverURL  string
		repository string
		runID      string
		want       string
	}{
		{
			name:       "no run ID means no Actions run to link to",
			serverURL:  "https://github.com",
			repository: "owner/repo",
			runID:      "",
			want:       "",
		},
		{
			name:       "github.com run",
			serverURL:  "https://github.com",
			repository: "owner/repo",
			runID:      "123456789",
			want:       "https://github.com/owner/repo/actions/runs/123456789",
		},
		{
			name:       "GitHub Enterprise Server run links back to the enterprise host",
			serverURL:  "https://ghe.example.com",
			repository: "owner/repo",
			runID:      "42",
			want:       "https://ghe.example.com/owner/repo/actions/runs/42",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := commitStatusTargetURL(tt.serverURL, tt.repository, tt.runID)
			if got != tt.want {
				t.Errorf("commitStatusTargetURL(%q, %q, %q) = %q, want %q", tt.serverURL, tt.repository, tt.runID, got, tt.want)
			}
		})
	}
}

// TestCommitStatusContext reproduces issue #54: the project coverage status
// context was the fixed string literal "litecov", so a second litecov step
// in one workflow -- the normal setup for a monorepo -- posted its status
// under the same context as the first and the last one to finish silently
// overwrote it. An empty flagName must keep the existing "litecov" context
// so current single-invocation users see no change.
func TestCommitStatusContext(t *testing.T) {
	tests := []struct {
		name     string
		flagName string
		want     string
	}{
		{"no flag keeps the existing context", "", "litecov"},
		{"backend flag", "backend", "litecov/backend"},
		{"frontend flag", "frontend", "litecov/frontend"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := commitStatusContext(tt.flagName)
			if got != tt.want {
				t.Errorf("commitStatusContext(%q) = %q, want %q", tt.flagName, got, tt.want)
			}
		})
	}
}

// TestPatchCommitStatusContext is TestCommitStatusContext's counterpart for
// the patch coverage status, "litecov/patch" by default (issue #54).
func TestPatchCommitStatusContext(t *testing.T) {
	tests := []struct {
		name     string
		flagName string
		want     string
	}{
		{"no flag keeps the existing context", "", "litecov/patch"},
		{"backend flag", "backend", "litecov/patch/backend"},
		{"frontend flag", "frontend", "litecov/patch/frontend"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := patchCommitStatusContext(tt.flagName)
			if got != tt.want {
				t.Errorf("patchCommitStatusContext(%q) = %q, want %q", tt.flagName, got, tt.want)
			}
		})
	}
}

// TestResolveGitHubHost guards issue #49: litecov hardcoded api.github.com
// and github.com, so it could not run on GitHub Enterprise Server, where
// GITHUB_API_URL and GITHUB_SERVER_URL point at the enterprise host instead.
func TestResolveGitHubHost(t *testing.T) {
	tests := []struct {
		name         string
		envValue     string
		defaultValue string
		want         string
	}{
		{
			name:         "empty env value falls back to the default",
			envValue:     "",
			defaultValue: "https://api.github.com",
			want:         "https://api.github.com",
		},
		{
			name:         "GITHUB_API_URL on github.com matches the default anyway",
			envValue:     "https://api.github.com",
			defaultValue: "https://api.github.com",
			want:         "https://api.github.com",
		},
		{
			name:         "GitHub Enterprise Server API host wins over the default",
			envValue:     "https://ghe.example.com/api/v3",
			defaultValue: "https://api.github.com",
			want:         "https://ghe.example.com/api/v3",
		},
		{
			name:         "GitHub Enterprise Server web host wins over the default",
			envValue:     "https://ghe.example.com",
			defaultValue: "https://github.com",
			want:         "https://ghe.example.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveGitHubHost(tt.envValue, tt.defaultValue)
			if got != tt.want {
				t.Errorf("resolveGitHubHost(%q, %q) = %q, want %q", tt.envValue, tt.defaultValue, got, tt.want)
			}
		})
	}
}

// TestGetPRNumber reproduces issue #53: getPRNumber used to find the PR
// number by scanning the raw event JSON for the first `"number":` substring,
// which picked up whichever nested object's number happened to appear first
// in the payload and missed the key entirely when GitHub's pretty-printed
// JSON put whitespace before the colon.
func TestGetPRNumber(t *testing.T) {
	tests := []struct {
		name      string
		eventName string
		content   string
		want      int
	}{
		{
			name:      "pull_request: top-level number",
			eventName: "pull_request",
			content:   `{"action":"opened","number":42,"pull_request":{"number":42}}`,
			want:      42,
		},
		{
			name:      "pull_request_target: same shape as pull_request",
			eventName: "pull_request_target",
			content:   `{"action":"opened","number":42,"pull_request":{"number":42}}`,
			want:      42,
		},
		{
			name:      "a nested object's number ahead of the top-level one no longer wins, from the issue's repro",
			eventName: "milestone",
			content:   `{"action":"opened","milestone":{"number":7,"title":"v2.0"},"number":42,"pull_request":{"number":42}}`,
			want:      42,
		},
		{
			name:      "whitespace before the colon is no longer missed, from the issue's repro",
			eventName: "pull_request",
			content:   "{\n  \"action\" : \"opened\",\n  \"number\" : 42,\n  \"pull_request\" : { \"number\" : 42 }\n}",
			want:      42,
		},
		{
			name:      "a number-like string in the PR title is not mistaken for the key",
			eventName: "pull_request",
			content:   `{"action":"opened","pull_request":{"title":"fix \"number\": 999999 parsing","number":42},"number":42}`,
			want:      42,
		},
		{
			name:      "pull_request_review: nested under pull_request.number",
			eventName: "pull_request_review",
			content:   `{"action":"submitted","review":{"id":1},"pull_request":{"number":42}}`,
			want:      42,
		},
		{
			name:      "check_suite: nested under check_suite.pull_requests[0].number",
			eventName: "check_suite",
			content:   `{"action":"completed","check_suite":{"pull_requests":[{"number":42}]}}`,
			want:      42,
		},
		{
			name:      "check_suite with no associated pull requests",
			eventName: "check_suite",
			content:   `{"action":"completed","check_suite":{"pull_requests":[]}}`,
			want:      0,
		},
		{
			name:      "push: no PR number in the payload at all",
			eventName: "push",
			content:   `{"ref":"refs/heads/main","commits":[]}`,
			want:      0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "event.json")
			if err := os.WriteFile(path, []byte(tt.content), 0644); err != nil {
				t.Fatalf("WriteFile: %v", err)
			}

			got, err := getPRNumber(path, tt.eventName)
			if err != nil {
				t.Fatalf("getPRNumber returned an error: %v", err)
			}
			if got != tt.want {
				t.Errorf("getPRNumber(%q, %q) = %d, want %d", tt.content, tt.eventName, got, tt.want)
			}
		})
	}

	t.Run("empty event path", func(t *testing.T) {
		got, err := getPRNumber("", "pull_request")
		if err != nil || got != 0 {
			t.Errorf("getPRNumber(\"\", ...) = (%d, %v), want (0, nil)", got, err)
		}
	})

	t.Run("missing event file", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "does-not-exist.json")
		got, err := getPRNumber(path, "pull_request")
		if err != nil || got != 0 {
			t.Errorf("getPRNumber(missing) = (%d, %v), want (0, nil)", got, err)
		}
	})
}

// TestResolveSHA reproduces issue #33: commit statuses and PR comment links
// were built from GITHUB_SHA, which on a pull_request-shaped event is the
// ephemeral merge commit on refs/pull/N/merge, not the PR's head commit. A
// status posted on the merge commit never shows up in the PR's checks list
// and can never be picked as a required status check under branch
// protection, and a comment link built from it points at a commit the PR
// doesn't actually contain.
func TestResolveSHA(t *testing.T) {
	tests := []struct {
		name      string
		githubSHA string
		content   string
		want      string
	}{
		{
			name:      "pull_request: head SHA from the event payload wins over the merge commit, from the issue's repro",
			githubSHA: "merge0000000000000000000000000000000000",
			content:   `{"action":"opened","number":42,"pull_request":{"number":42,"head":{"sha":"headc0mmit00000000000000000000000000"}}}`,
			want:      "headc0mmit00000000000000000000000000",
		},
		{
			name:      "pull_request_target: same shape as pull_request",
			githubSHA: "merge0000000000000000000000000000000000",
			content:   `{"action":"opened","number":42,"pull_request":{"number":42,"head":{"sha":"headc0mmit00000000000000000000000000"}}}`,
			want:      "headc0mmit00000000000000000000000000",
		},
		{
			name:      "pull_request_review: head SHA nested the same way, under pull_request.head.sha",
			githubSHA: "merge0000000000000000000000000000000000",
			content:   `{"action":"submitted","review":{"id":1},"pull_request":{"number":42,"head":{"sha":"headc0mmit00000000000000000000000000"}}}`,
			want:      "headc0mmit00000000000000000000000000",
		},
		{
			name:      "push: no pull_request object in the payload, GITHUB_SHA is already the head commit",
			githubSHA: "abc123",
			content:   `{"ref":"refs/heads/main","commits":[]}`,
			want:      "abc123",
		},
		{
			name:      "check_suite: no top-level pull_request object, GITHUB_SHA is left untouched",
			githubSHA: "abc123",
			content:   `{"action":"completed","check_suite":{"pull_requests":[{"number":42}]}}`,
			want:      "abc123",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "event.json")
			if err := os.WriteFile(path, []byte(tt.content), 0644); err != nil {
				t.Fatalf("WriteFile: %v", err)
			}

			event, err := loadEvent(path)
			if err != nil {
				t.Fatalf("loadEvent returned an error: %v", err)
			}

			got := resolveSHA(tt.githubSHA, event)
			if got != tt.want {
				t.Errorf("resolveSHA(%q, loadEvent(%q)) = %q, want %q", tt.githubSHA, tt.content, got, tt.want)
			}
		})
	}

	t.Run("empty event path falls back to GITHUB_SHA", func(t *testing.T) {
		event, err := loadEvent("")
		if err != nil {
			t.Fatalf("loadEvent(\"\") returned an error: %v", err)
		}
		got := resolveSHA("abc123", event)
		if got != "abc123" {
			t.Errorf("resolveSHA(\"abc123\", loadEvent(\"\")) = %q, want %q", got, "abc123")
		}
	})

	t.Run("missing event file falls back to GITHUB_SHA", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "does-not-exist.json")
		event, err := loadEvent(path)
		if err == nil {
			t.Fatal("loadEvent(missing) returned a nil error, want the read failure reported")
		}
		got := resolveSHA("abc123", event)
		if got != "abc123" {
			t.Errorf("resolveSHA(\"abc123\", loadEvent(missing)) = %q, want %q", got, "abc123")
		}
	})

	t.Run("malformed JSON returns an error, GITHUB_SHA is left untouched", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "event.json")
		if err := os.WriteFile(path, []byte("{not valid json"), 0644); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
		event, err := loadEvent(path)
		if err == nil {
			t.Fatal("loadEvent(malformed) returned a nil error, want the parse failure reported")
		}
		got := resolveSHA("abc123", event)
		if got != "abc123" {
			t.Errorf("resolveSHA(\"abc123\", loadEvent(malformed)) = %q, want %q", got, "abc123")
		}
	})
}

// TestNoPRNumberWarning guards against the message regressing back to a
// plain, easy-to-miss log line. Before issue #82's fix, main printed "No PR
// number found, skipping comment" with fmt.Println -- indistinguishable
// from any other status line and absent from the run's Annotations panel --
// while the commit status still posted "success" on the same run. That
// combination is how a push-triggered workflow with no PR to comment on
// looked identical to a fully successful run for three weeks straight.
func TestNoPRNumberWarning(t *testing.T) {
	tests := []struct {
		name      string
		eventName string
		want      string
	}{
		{
			name:      "push event, from the issue's repro",
			eventName: "push",
			want:      "::warning title=No Pull Request::No pull request found for this run (event: push); litecov posts comments on pull_request events, see README for supported triggers",
		},
		{
			name:      "empty event name from a direct binary invocation outside Actions",
			eventName: "",
			want:      "::warning title=No Pull Request::No pull request found for this run (event: ); litecov posts comments on pull_request events, see README for supported triggers",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := noPRNumberWarning(tt.eventName)
			if got != tt.want {
				t.Errorf("noPRNumberWarning(%q) = %q, want %q", tt.eventName, got, tt.want)
			}
			if !strings.HasPrefix(got, "::warning") {
				t.Errorf("noPRNumberWarning(%q) = %q, want a GitHub Actions ::warning:: workflow command so it shows up in the run's Annotations panel, not a plain line", tt.eventName, got)
			}
		})
	}
}

func TestLoadBaseReport_NoReport(t *testing.T) {
	// An empty path is the only case where (nil, nil) is correct: no base
	// comparison was requested at all. Every other way loadBaseReport can
	// come back empty is a base that WAS requested but couldn't be read, and
	// must return a non-nil error instead so the caller -- and the PR
	// comment -- can tell the two apart (see
	// TestLoadBaseReport_HardFailuresReturnError, issue #39).
	report, err := loadBaseReport("", "", nil)
	if report != nil || err != nil {
		t.Errorf("loadBaseReport(\"\") = (%v, %v), want (nil, nil)", report, err)
	}
}

// TestLoadBaseReport_HardFailuresReturnError reproduces issue #39: a base
// coverage file that couldn't be opened, whose format couldn't be detected,
// or that failed to parse into any files used to make loadBaseReport return
// nil the same way an empty path does -- with no error to explain why, and
// for two of these three cases, not even a line on stderr. The PR comment
// then rendered as if no base had been configured at all, instead of saying
// the comparison was requested but broken.
func TestLoadBaseReport_HardFailuresReturnError(t *testing.T) {
	tests := []struct {
		name    string
		content string
	}{
		{name: "unrecognized content", content: "this is not a coverage report\njust plain text\n"},
		{name: "lcov with no SF: record, from the issue's repro", content: "end_of_record\n"},
		{name: "cobertura with no packages, from the issue's repro", content: "<coverage><packages></packages></coverage>"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "base.txt")
			if err := os.WriteFile(path, []byte(tt.content), 0644); err != nil {
				t.Fatalf("WriteFile: %v", err)
			}

			report, err := loadBaseReport(path, "", nil)
			if err == nil {
				t.Fatal("loadBaseReport returned a nil error, want the failure reported")
			}
			if report != nil {
				t.Errorf("loadBaseReport returned a non-nil report alongside an error: %v", report)
			}
		})
	}

	t.Run("missing file", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "does-not-exist.lcov")
		report, err := loadBaseReport(path, "", nil)
		if err == nil {
			t.Fatal("loadBaseReport returned a nil error, want the open failure reported")
		}
		if report != nil {
			t.Errorf("loadBaseReport returned a non-nil report alongside an error: %v", report)
		}
	})
}

func TestLoadBaseReport_SourcePrefixMatchesHead(t *testing.T) {
	// Reproduces issue #29: the head parser is built with
	// GetParserWithPath(detected, *coverageFile), which sets LCOVParser's
	// SourcePrefix from the coverage file's own location
	// (js/coverage/lcov.info -> "js"), so a relative SF: path picks up that
	// prefix. loadBaseReport used to build its parser with plain GetParser
	// (no path), leaving SourcePrefix empty, so identical LCOV input
	// produced "js/src/a.js" on the head side and "src/a.js" on the base
	// side. NewComparison's lookup then missed on the file entirely, even
	// though nothing about its actual coverage changed between head and
	// base.
	const lcovSrc = "SF:src/a.js\nDA:1,1\nDA:2,1\nDA:3,0\nDA:4,0\nend_of_record\n"

	dir := t.TempDir()
	coverageFile := filepath.Join(dir, "js", "coverage", "lcov.info")
	if err := os.MkdirAll(filepath.Dir(coverageFile), 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(coverageFile, []byte(lcovSrc), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	headParser, err := parser.GetParserWithPath("lcov", coverageFile)
	if err != nil {
		t.Fatalf("GetParserWithPath: %v", err)
	}
	head, err := headParser.Parse(strings.NewReader(lcovSrc))
	if err != nil {
		t.Fatalf("head Parse: %v", err)
	}

	base, err := loadBaseReport(coverageFile, "", nil)
	if err != nil {
		t.Fatalf("loadBaseReport: %v", err)
	}
	if base == nil {
		t.Fatal("loadBaseReport returned nil")
	}

	if len(head.Files) != 1 || len(base.Files) != 1 {
		t.Fatalf("head.Files = %d, base.Files = %d, want 1 each", len(head.Files), len(base.Files))
	}
	if head.Files[0].Path != base.Files[0].Path {
		t.Fatalf("head path %q != base path %q: base report must pick up the same source prefix as head", head.Files[0].Path, base.Files[0].Path)
	}

	comp := coverage.NewComparison(head, base, nil, nil, nil)
	if len(comp.FileChanges) != 1 {
		t.Fatalf("FileChanges length = %d, want 1 (same file matched on both sides, not one entry per side)", len(comp.FileChanges))
	}

	fc := comp.FileChanges[0]
	if fc.NoBaseData {
		t.Error("NoBaseData should be false: base has a matching entry for this file")
	}
	if fc.NoCoverage {
		t.Error("NoCoverage should be false: the file is present in both head and base")
	}
	if fc.Delta != 0 {
		t.Errorf("Delta = %v, want 0: head and base coverage are identical", fc.Delta)
	}
	if fc.BaseCoverage != fc.HeadCoverage {
		t.Errorf("BaseCoverage = %v, HeadCoverage = %v, want equal", fc.BaseCoverage, fc.HeadCoverage)
	}
}

// TestResolveCoverageFiles reproduces issue #89: -coverage-file only ever
// accepted one literal path, so a comma separated list or a glob pattern --
// the natural way to name a monorepo's several coverage files, or what a
// matrix job produces -- errored trying to open the whole string as one
// filename instead of being expanded into the files it refers to.
func TestResolveCoverageFiles(t *testing.T) {
	dir := t.TempDir()
	pkgA := filepath.Join(dir, "mono", "pkg-a", "coverage.lcov")
	pkgB := filepath.Join(dir, "mono", "pkg-b", "coverage.lcov")
	for _, p := range []string{pkgA, pkgB} {
		if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
			t.Fatalf("MkdirAll: %v", err)
		}
		if err := os.WriteFile(p, []byte("SF:a.go\nDA:1,1\nend_of_record\n"), 0644); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
	}
	doesNotExist := filepath.Join(dir, "does-not-exist.lcov")

	tests := []struct {
		name    string
		pattern string
		want    []string
		wantErr bool
	}{
		{name: "empty pattern resolves to nothing", pattern: "", want: nil},
		{name: "single literal path", pattern: pkgA, want: []string{pkgA}},
		{
			name:    "comma separated literal paths, whitespace trimmed",
			pattern: " " + pkgA + " , " + pkgB + " ",
			want:    []string{pkgA, pkgB},
		},
		{
			name:    "glob expands to every match, from the issue's own repro",
			pattern: filepath.Join(dir, "mono", "*", "coverage.lcov"),
			want:    []string{pkgA, pkgB},
		},
		{
			name:    "empty entries between commas are skipped",
			pattern: pkgA + ",,",
			want:    []string{pkgA},
		},
		{
			name:    "a path matched twice is kept once, in first-seen order",
			pattern: pkgA + "," + pkgA,
			want:    []string{pkgA},
		},
		{
			name:    "nonexistent literal path is kept as-is, not silently dropped",
			pattern: doesNotExist,
			want:    []string{doesNotExist},
		},
		{
			name:    "malformed glob pattern is an error",
			pattern: "[",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveCoverageFiles(tt.pattern)
			if tt.wantErr {
				if err == nil {
					t.Fatal("resolveCoverageFiles returned a nil error, want the glob failure reported")
				}
				return
			}
			if err != nil {
				t.Fatalf("resolveCoverageFiles: %v", err)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("resolveCoverageFiles(%q) = %v, want %v", tt.pattern, got, tt.want)
			}
			for i, want := range tt.want {
				if got[i] != want {
					t.Errorf("resolveCoverageFiles(%q)[%d] = %q, want %q", tt.pattern, i, got[i], want)
				}
			}
		})
	}
}

// TestParseCoverageFile_PerFileSourcePrefix reproduces the other half of
// issue #89: once -coverage-file can name more than one file, each one
// must still be parsed with its own path, not a path shared across every
// match. GetParserWithPath resolves a relative LCOV SF: record against the
// coverage file's own directory (extractSourcePrefix in
// internal/parser/detect.go), so two packages that each report on a file
// named src/a.go need that resolved to two different paths -- collapsing
// them to one would merge unrelated files' line data together.
func TestParseCoverageFile_PerFileSourcePrefix(t *testing.T) {
	dir := t.TempDir()
	pkgA := filepath.Join(dir, "mono", "pkg-a", "coverage", "lcov.info")
	pkgB := filepath.Join(dir, "mono", "pkg-b", "coverage", "lcov.info")
	const lcovSrc = "SF:src/a.go\nDA:1,1\nDA:2,0\nend_of_record\n"
	for _, p := range []string{pkgA, pkgB} {
		if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
			t.Fatalf("MkdirAll: %v", err)
		}
		if err := os.WriteFile(p, []byte(lcovSrc), 0644); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
	}

	reportA, err := parseCoverageFile(pkgA, "auto")
	if err != nil {
		t.Fatalf("parseCoverageFile(pkgA): %v", err)
	}
	reportB, err := parseCoverageFile(pkgB, "auto")
	if err != nil {
		t.Fatalf("parseCoverageFile(pkgB): %v", err)
	}

	if len(reportA.Files) != 1 || len(reportB.Files) != 1 {
		t.Fatalf("reportA.Files = %d, reportB.Files = %d, want 1 each", len(reportA.Files), len(reportB.Files))
	}

	wantA := filepath.Join(dir, "mono", "pkg-a", "src", "a.go")
	wantB := filepath.Join(dir, "mono", "pkg-b", "src", "a.go")
	if reportA.Files[0].Path != wantA {
		t.Errorf("reportA path = %q, want %q", reportA.Files[0].Path, wantA)
	}
	if reportB.Files[0].Path != wantB {
		t.Errorf("reportB path = %q, want %q", reportB.Files[0].Path, wantB)
	}
}

func TestParseCoverageFile_OpenFailure(t *testing.T) {
	path := filepath.Join(t.TempDir(), "does-not-exist.lcov")
	report, err := parseCoverageFile(path, "auto")
	if err == nil {
		t.Fatal("parseCoverageFile returned a nil error, want the open failure reported")
	}
	if report != nil {
		t.Errorf("parseCoverageFile returned a non-nil report alongside an error: %v", report)
	}
}

// TestCoverageFileGlob_MergesAcrossPackages is issue #89's repro end to
// end, through the three pieces main wires together: resolveCoverageFiles
// expanding the glob, parseCoverageFile reading each match with its own
// source prefix, and coverage.MergeReports combining the results. Before
// the fix, -coverage-file='mono/*/coverage.lcov' failed to open a literal
// file with that name, and -coverage-file passed twice silently kept only
// the second package's report with no warning that the first was dropped.
func TestCoverageFileGlob_MergesAcrossPackages(t *testing.T) {
	dir := t.TempDir()
	pkgA := filepath.Join(dir, "mono", "pkg-a", "coverage", "lcov.info")
	pkgB := filepath.Join(dir, "mono", "pkg-b", "coverage", "lcov.info")
	if err := os.MkdirAll(filepath.Dir(pkgA), 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(pkgB), 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	// pkg-a: one file, 4 of 6 lines covered.
	if err := os.WriteFile(pkgA, []byte("SF:src/a.go\nDA:1,1\nDA:2,1\nDA:3,1\nDA:4,1\nDA:5,0\nDA:6,0\nend_of_record\n"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	// pkg-b: two files, 3 of 7 lines covered each.
	pkgBContent := "SF:src/b.go\nDA:1,1\nDA:2,1\nDA:3,1\nDA:4,0\nDA:5,0\nDA:6,0\nDA:7,0\nend_of_record\n" +
		"SF:src/c.go\nDA:1,1\nDA:2,1\nDA:3,1\nDA:4,0\nDA:5,0\nDA:6,0\nDA:7,0\nend_of_record\n"
	if err := os.WriteFile(pkgB, []byte(pkgBContent), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	pattern := filepath.Join(dir, "mono", "*", "coverage", "lcov.info")
	files, err := resolveCoverageFiles(pattern)
	if err != nil {
		t.Fatalf("resolveCoverageFiles: %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("resolveCoverageFiles matched %d files, want 2: %v", len(files), files)
	}

	reports := make([]*coverage.Report, 0, len(files))
	for _, f := range files {
		r, err := parseCoverageFile(f, "auto")
		if err != nil {
			t.Fatalf("parseCoverageFile(%s): %v", f, err)
		}
		reports = append(reports, r)
	}

	merged := coverage.MergeReports(reports)

	if len(merged.Files) != 3 {
		t.Fatalf("merged.Files length = %d, want 3 (a.go from pkg-a, b.go and c.go from pkg-b)", len(merged.Files))
	}
	if merged.TotalCovered != 10 {
		t.Errorf("TotalCovered = %d, want 10 (4 from pkg-a + 3 + 3 from pkg-b)", merged.TotalCovered)
	}
	if merged.TotalLines != 20 {
		t.Errorf("TotalLines = %d, want 20 (6 from pkg-a + 7 + 7 from pkg-b)", merged.TotalLines)
	}
	if merged.Coverage != 50.0 {
		t.Errorf("Coverage = %v, want 50.0", merged.Coverage)
	}
}

// TestWriteGitHubOutput reproduces issue #88's two GITHUB_OUTPUT bugs.
// os.OpenFile was called without os.O_CREATE, so a GITHUB_OUTPUT file that
// doesn't already exist failed to open instead of being created, and the
// open error was checked with "if err == nil" and no else, so that failure
// -- and any other -- was silent: a workflow reading the outputs saw empty
// strings with nothing in the log explaining why.
//
// This also stands in for the fix's main change: moving the write ahead of
// postCoverageComment, GetChangedFiles, and the threshold checks in main, so
// none of their os.Exit(1) paths can skip it. writeGitHubOutput no longer
// touches any of those, which is what makes calling it immediately after
// the report is parsed possible; confirmed by hand against the built binary
// using the issue's own repro (fake GITHUB_TOKEN, 401 on the comment POST,
// GITHUB_OUTPUT still gets all four keys).
func TestWriteGitHubOutput(t *testing.T) {
	report := &coverage.Report{
		Files:        []coverage.FileCoverage{{Path: "a.go"}, {Path: "b.go"}},
		TotalCovered: 4,
		TotalLines:   6,
		Coverage:     4.0 / 6.0 * 100,
	}
	const want = "coverage=66.67\nlines-covered=4\nlines-total=6\nfiles-count=2\n"

	t.Run("empty path is a no-op", func(t *testing.T) {
		// GITHUB_OUTPUT is unset on a direct binary invocation outside
		// Actions; that's not a failure.
		if err := writeGitHubOutput("", report); err != nil {
			t.Errorf("writeGitHubOutput(\"\", ...) = %v, want nil", err)
		}
	})

	t.Run("creates a file that doesn't exist yet", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "new-output.txt")
		if err := writeGitHubOutput(path, report); err != nil {
			t.Fatalf("writeGitHubOutput: %v", err)
		}
		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("ReadFile: %v", err)
		}
		if string(got) != want {
			t.Errorf("file content = %q, want %q", got, want)
		}
	})

	t.Run("appends to an existing file instead of overwriting it", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "output.txt")
		if err := os.WriteFile(path, []byte("previous-step-output=1\n"), 0644); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
		if err := writeGitHubOutput(path, report); err != nil {
			t.Fatalf("writeGitHubOutput: %v", err)
		}
		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("ReadFile: %v", err)
		}
		if string(got) != "previous-step-output=1\n"+want {
			t.Errorf("file content = %q, want the existing line preserved and litecov's four keys appended after it", got)
		}
	})

	t.Run("an open failure is reported instead of swallowed", func(t *testing.T) {
		// A parent directory that doesn't exist is an open failure
		// os.O_CREATE can't paper over, standing in for any other reason
		// GITHUB_OUTPUT couldn't be opened.
		path := filepath.Join(t.TempDir(), "missing-parent", "output.txt")
		if err := writeGitHubOutput(path, report); err == nil {
			t.Error("writeGitHubOutput returned nil, want the open failure reported so the caller can log it")
		}
	})
}

// TestPostCoverageComment reproduces issue #37: FindExistingComment returns
// (0, err) on a transport failure, a 403, a 5xx, or a malformed body -- the
// same zero ID it returns for "no comment exists yet". postCoverageComment
// used to look only at the ID, so a lookup failure was indistinguishable
// from "no comment exists" and fell through to CreateComment, permanently
// duplicating the coverage comment on the PR.
func TestPostCoverageComment(t *testing.T) {
	const marker = "<!-- litecov -->"

	tests := []struct {
		name         string
		marker       string
		listComments func(w http.ResponseWriter, r *http.Request)
		wantErr      bool
		wantCreate   int
		wantUpdate   int
	}{
		{
			name:   "no existing comment creates one",
			marker: marker,
			listComments: func(w http.ResponseWriter, r *http.Request) {
				json.NewEncoder(w).Encode([]struct{}{})
			},
			wantCreate: 1,
		},
		{
			name:   "existing comment gets updated in place",
			marker: marker,
			listComments: func(w http.ResponseWriter, r *http.Request) {
				comments := []struct {
					ID   int    `json:"id"`
					Body string `json:"body"`
				}{{ID: 42, Body: marker + "\nold report"}}
				json.NewEncoder(w).Encode(comments)
			},
			wantUpdate: 1,
		},
		{
			name:   "lookup failure is reported instead of falling through to create",
			marker: marker,
			listComments: func(w http.ResponseWriter, r *http.Request) {
				// No rate-limit headers, so doRequest doesn't retry this (it
				// reads as a plain permissions failure, not rate limiting).
				w.WriteHeader(http.StatusForbidden)
				w.Write([]byte("forbidden"))
			},
			wantErr: true,
		},
		{
			// issue #54: two litecov steps in one workflow used to share the
			// same fixed marker, so the second step's lookup matched the
			// first step's comment and PATCHed over it. A comment already on
			// the PR under one flag's marker must not match a lookup for a
			// different flag's marker; the second step must create its own
			// comment instead of overwriting the first.
			name:   "a different flag's existing comment does not match, creates a new one instead",
			marker: "<!-- litecov:frontend -->",
			listComments: func(w http.ResponseWriter, r *http.Request) {
				comments := []struct {
					ID   int    `json:"id"`
					Body string `json:"body"`
				}{{ID: 42, Body: "<!-- litecov:backend -->\nbackend report"}}
				json.NewEncoder(w).Encode(comments)
			},
			wantCreate: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var createCalls, updateCalls int
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.Method {
				case http.MethodGet:
					tt.listComments(w, r)
				case http.MethodPost:
					createCalls++
					w.WriteHeader(http.StatusCreated)
					json.NewEncoder(w).Encode(map[string]int{"id": 99})
				case http.MethodPatch:
					updateCalls++
					w.WriteHeader(http.StatusOK)
					json.NewEncoder(w).Encode(map[string]int{"id": 42})
				}
			}))
			defer server.Close()

			gh := &github.Client{Token: "test-token", Owner: "owner", Repo: "repo", BaseURL: server.URL}
			err := postCoverageComment(gh, 1, tt.marker, "report body")

			if (err != nil) != tt.wantErr {
				t.Errorf("postCoverageComment() error = %v, wantErr %v", err, tt.wantErr)
			}
			if createCalls != tt.wantCreate {
				t.Errorf("CreateComment called %d times, want %d", createCalls, tt.wantCreate)
			}
			if updateCalls != tt.wantUpdate {
				t.Errorf("UpdateComment called %d times, want %d", updateCalls, tt.wantUpdate)
			}
		})
	}
}

// TestCommentPostOutcome covers issue #42: a fork PR's read-only
// GITHUB_TOKEN makes CreateComment/UpdateComment return a 403, which used
// to be a hard failure that took down the whole run for every fork
// contributor. A permission error must now produce a clear, non-fatal
// message instead of the raw wrapped API error, while every other failure
// stays fatal exactly as before. It also covers issue #43: that message
// must print the job permissions: block that fixes the case where the
// 403 came from a restricted-default token rather than a fork PR.
func TestCommentPostOutcome(t *testing.T) {
	tests := []struct {
		name        string
		err         error
		wantMessage string
		wantFatal   bool
	}{
		{
			// Wrapped the way postCoverageComment wraps a FindExistingComment
			// failure, to confirm errors.Is still matches through that layer.
			name: "permission denied is reported, non-fatal, and names the fix",
			err:  fmt.Errorf("looking up existing comment: %w", github.ErrPermissionDenied),
			wantMessage: "Cannot comment: GITHUB_TOKEN can't write pull request comments (fork PR, or a job permissions: block missing pull-requests: write). " +
				"Skipping comment. If this isn't a fork PR, add:\n" +
				"  permissions:\n    contents: read\n    pull-requests: write\n    statuses: write",
			wantFatal: false,
		},
		{
			name:        "any other error stays fatal",
			err:         errors.New("connection reset by peer"),
			wantMessage: "Failed to post comment: connection reset by peer",
			wantFatal:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			message, fatal := commentPostOutcome(tt.err)
			if message != tt.wantMessage {
				t.Errorf("message = %q, want %q", message, tt.wantMessage)
			}
			if fatal != tt.wantFatal {
				t.Errorf("fatal = %v, want %v", fatal, tt.wantFatal)
			}
		})
	}
}

// TestCommitStatusFailureMessage covers issue #43: SetCommitStatus failing
// with a 403 used to surface only the raw wrapped API error ("GitHub API
// permission denied: 403 Forbidden - {...}"), leaving the user to guess
// what to change. A permission error must now name the cause and print the
// job permissions: block that fixes it when the token isn't a fork PR's;
// any other failure keeps relaying the plain error as before.
func TestCommitStatusFailureMessage(t *testing.T) {
	tests := []struct {
		name        string
		action      string
		err         error
		wantMessage string
	}{
		{
			name:   "permission denied names the fix",
			action: "set commit status",
			err:    fmt.Errorf("posting status: %w: 403 Forbidden - {\"message\":\"Resource not accessible by integration\"}", github.ErrPermissionDenied),
			wantMessage: "Warning: Failed to set commit status: GITHUB_TOKEN can't write commit statuses (fork PR, or a job permissions: block missing statuses: write). " +
				"If this isn't a fork PR, add:\n" +
				"  permissions:\n    contents: read\n    pull-requests: write\n    statuses: write",
		},
		{
			name:        "action names the patch status call site",
			action:      "set patch commit status",
			err:         github.ErrPermissionDenied,
			wantMessage: "Warning: Failed to set patch commit status: GITHUB_TOKEN can't write commit statuses (fork PR, or a job permissions: block missing statuses: write). If this isn't a fork PR, add:\n  permissions:\n    contents: read\n    pull-requests: write\n    statuses: write",
		},
		{
			name:        "any other error is relayed as before",
			action:      "set commit status",
			err:         errors.New("connection reset by peer"),
			wantMessage: "Warning: Failed to set commit status: connection reset by peer",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			message := commitStatusFailureMessage(tt.action, tt.err)
			if message != tt.wantMessage {
				t.Errorf("message = %q, want %q", message, tt.wantMessage)
			}
		})
	}
}

// TestBuildAnnotationLines covers issue #46 (GitHub's Actions toolkit caps
// each step at 10 warning annotations and silently drops the rest, so which
// ten survived used to depend on report.Files' parse order rather than
// which uncovered lines actually mattered to the PR) and issue #47 (a file
// being in the changed set was enough to warn about every uncovered range
// in it, including ranges the PR's diff never touched, so a one-line fix to
// a large, sparsely-tested file warned about hundreds of unrelated lines).
// buildAnnotationLines must clip every range to the PR's diff before it can
// become a pending annotation, rank a changed file with no coverage at all
// ahead of any range that survives that, then truncate to 10 and say how
// many it left out.
func TestBuildAnnotationLines(t *testing.T) {
	bigLines := make([]int, 50)
	for i := range bigLines {
		bigLines[i] = i + 1
	}

	manyLines := make([]int, 12)
	for i := range manyLines {
		manyLines[i] = i*2 + 1 // 1, 3, 5, ... 23: 12 single-line ranges, none adjacent
	}
	first10 := make([]string, 10)
	for i, n := range manyLines[:10] {
		first10[i] = fmt.Sprintf("::warning file=big.go,line=%d,title=Uncovered::Line %d not covered by tests", n, n)
	}

	tests := []struct {
		name         string
		report       *coverage.Report
		changedFiles []string
		patchedLines map[string][]diff.LineRange
		hasPR        bool
		want         []string
	}{
		{
			name: "no uncovered lines produces no annotations",
			report: &coverage.Report{
				Files: []coverage.FileCoverage{{Path: "a.go"}},
			},
			want: nil,
		},
		{
			// main.go passes changedFiles=nil, patchedLines=nil here when
			// show-files isn't "changed" -- there's no changed-file list to
			// scope annotations to at all. Before this, a nil changedFiles
			// skipped the per-file filter entirely, so every uncovered line
			// in the whole report was annotated on every PR, unrelated.go
			// included, regardless of how old it was (issue #47).
			name: "no changed-file scoping at all skips annotations instead of covering the whole repo",
			report: &coverage.Report{
				Files: []coverage.FileCoverage{
					{Path: "unrelated.go", UncoveredLines: []int{1, 2, 3}},
				},
			},
			want: nil,
		},
		{
			name: "ranked by range size within a tier, not report order",
			report: &coverage.Report{
				Files: []coverage.FileCoverage{
					{Path: "b.go", UncoveredLines: []int{10}},
					{Path: "a.go", UncoveredLines: []int{1, 2, 3}},
				},
			},
			changedFiles: []string{"a.go", "b.go"},
			patchedLines: map[string][]diff.LineRange{
				"a.go": {{Start: 1, End: 3}},
				"b.go": {{Start: 10, End: 10}},
			},
			want: []string{
				"::warning file=a.go,line=1,endLine=3,title=Uncovered::Lines 1-3 not covered by tests",
				"::warning file=b.go,line=10,title=Uncovered::Line 10 not covered by tests",
			},
		},
		{
			name: "a range outside the PR's diff produces no annotation, however large",
			report: &coverage.Report{
				Files: []coverage.FileCoverage{
					{Path: "big.go", UncoveredLines: bigLines},
					{Path: "small.go", UncoveredLines: []int{5}},
				},
			},
			changedFiles: []string{"big.go", "small.go"},
			patchedLines: map[string][]diff.LineRange{
				"small.go": {{Start: 4, End: 6}},
			},
			want: []string{
				"::warning file=small.go,line=5,title=Uncovered::Line 5 not covered by tests",
			},
		},
		{
			name: "an uncovered range is clipped to the lines the diff actually touched",
			report: &coverage.Report{
				Files: []coverage.FileCoverage{
					{Path: "mixed.go", UncoveredLines: []int{8, 9, 10, 11, 12}},
				},
			},
			changedFiles: []string{"mixed.go"},
			patchedLines: map[string][]diff.LineRange{
				// Lines 8-9 and 12 predate the PR; only 10-11 are its own.
				"mixed.go": {{Start: 10, End: 11}},
			},
			want: []string{
				"::warning file=mixed.go,line=10,endLine=11,title=Uncovered::Lines 10-11 not covered by tests",
			},
		},
		{
			name: "no patch data for a changed file suppresses its ranges instead of falling back to the whole file",
			report: &coverage.Report{
				Files: []coverage.FileCoverage{
					{Path: "a.go", UncoveredLines: []int{1, 2, 3}},
				},
			},
			changedFiles: []string{"a.go"},
			want:         nil,
		},
		{
			name: "a changed file with no coverage at all outranks every range, patch or not",
			report: &coverage.Report{
				Files: []coverage.FileCoverage{
					{Path: "covered.go", UncoveredLines: []int{1, 2, 3, 4, 5}},
				},
			},
			changedFiles: []string{"empty.go", "covered.go"},
			patchedLines: map[string][]diff.LineRange{
				"covered.go": {{Start: 1, End: 5}},
			},
			want: []string{
				"::warning file=empty.go,line=1,title=No Coverage::File has no test coverage",
				"::warning file=covered.go,line=1,endLine=5,title=Uncovered::Lines 1-5 not covered by tests",
			},
		},
		{
			name: "more than 10 annotations are truncated with a suppressed-count notice",
			report: &coverage.Report{
				Files: []coverage.FileCoverage{{Path: "big.go", UncoveredLines: manyLines}},
			},
			changedFiles: []string{"big.go"},
			patchedLines: map[string][]diff.LineRange{
				"big.go": {{Start: 1, End: 23}},
			},
			want: append(append([]string{}, first10...),
				"::notice::2 more coverage annotations not shown (GitHub caps annotations at 10 per step)"),
		},
		{
			name: "the truncation notice points at the PR comment when a PR exists",
			report: &coverage.Report{
				Files: []coverage.FileCoverage{{Path: "big.go", UncoveredLines: manyLines}},
			},
			changedFiles: []string{"big.go"},
			patchedLines: map[string][]diff.LineRange{
				"big.go": {{Start: 1, End: 23}},
			},
			hasPR: true,
			want: append(append([]string{}, first10...),
				"::notice::2 more coverage annotations not shown (GitHub caps annotations at 10 per step); see the PR comment for the full list"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildAnnotationLines(tt.report, tt.changedFiles, tt.patchedLines, tt.hasPR)
			if len(got) != len(tt.want) {
				t.Fatalf("got %d lines, want %d\ngot:  %v\nwant: %v", len(got), len(tt.want), got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("line %d = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}
