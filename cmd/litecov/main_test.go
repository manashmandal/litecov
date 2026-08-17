package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/manashmandal/litecov/internal/coverage"
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
		listComments func(w http.ResponseWriter, r *http.Request)
		wantErr      bool
		wantCreate   int
		wantUpdate   int
	}{
		{
			name: "no existing comment creates one",
			listComments: func(w http.ResponseWriter, r *http.Request) {
				json.NewEncoder(w).Encode([]struct{}{})
			},
			wantCreate: 1,
		},
		{
			name: "existing comment gets updated in place",
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
			name: "lookup failure is reported instead of falling through to create",
			listComments: func(w http.ResponseWriter, r *http.Request) {
				// No rate-limit headers, so doRequest doesn't retry this (it
				// reads as a plain permissions failure, not rate limiting).
				w.WriteHeader(http.StatusForbidden)
				w.Write([]byte("forbidden"))
			},
			wantErr: true,
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
			err := postCoverageComment(gh, 1, marker, "report body")

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
