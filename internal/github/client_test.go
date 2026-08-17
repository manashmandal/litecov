package github

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// TestMain no-ops retry backoff for the whole package so tests that exercise
// retries (issue #52) run at normal test speed instead of paying real
// exponential-backoff delays.
func TestMain(m *testing.M) {
	sleep = func(time.Duration) {}
	os.Exit(m.Run())
}

func TestClient_GetChangedFiles(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/owner/repo/pulls/1/files" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		// issue #76: per_page must be sent explicitly so a PR doesn't fall
		// back to GitHub's 30-file-per-page default.
		if got := r.URL.Query().Get("per_page"); got != "100" {
			t.Errorf("per_page = %q, want %q", got, "100")
		}
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Errorf("unexpected auth header: %s", r.Header.Get("Authorization"))
		}
		files := []struct {
			Filename string `json:"filename"`
			Status   string `json:"status"`
			Patch    string `json:"patch,omitempty"`
		}{
			{Filename: "src/parser.go", Status: "modified", Patch: "@@ -1,3 +1,4 @@\n line1\n+line2\n line3"},
			{Filename: "src/utils.go", Status: "added"},
			{Filename: "src/legacy.go", Status: "removed"},
			// A renamed-with-no-content-change file (and binary files, and
			// diffs past GitHub's size limit) carries no "patch" key at all.
			{Filename: "src/binary.png", Status: "modified"},
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

	if len(files) != 4 {
		t.Errorf("got %d files, want 4", len(files))
	}
	if files[0].Path != "src/parser.go" {
		t.Errorf("files[0].Path = %v, want src/parser.go", files[0].Path)
	}
	if files[0].IsAdded {
		t.Error("files[0].IsAdded should be false for status \"modified\"")
	}
	if files[0].IsRemoved {
		t.Error("files[0].IsRemoved should be false for status \"modified\"")
	}
	// issue #7: the per-file "patch" field carries the unified diff hunks and
	// used to be discarded entirely, leaving nothing to feed the diff parser.
	wantPatch := "@@ -1,3 +1,4 @@\n line1\n+line2\n line3"
	if files[0].Patch != wantPatch {
		t.Errorf("files[0].Patch = %q, want %q", files[0].Patch, wantPatch)
	}
	if !files[1].IsAdded {
		t.Error("files[1].IsAdded should be true for status \"added\"")
	}
	if files[1].IsRemoved {
		t.Error("files[1].IsRemoved should be false for status \"added\"")
	}
	// issue #31: the diff's "removed" status must survive decoding so
	// callers can tell a deleted file apart from one that's merely missing
	// coverage data.
	if files[2].IsAdded {
		t.Error("files[2].IsAdded should be false for status \"removed\"")
	}
	if !files[2].IsRemoved {
		t.Error("files[2].IsRemoved should be true for status \"removed\"")
	}
	// issue #7: GitHub omits "patch" for binary files, pure renames and
	// diffs over its size limit. A missing key must decode to "" instead of
	// an error, so callers can treat it as "no coverable changed lines".
	if files[3].Patch != "" {
		t.Errorf("files[3].Patch = %q, want empty string for a file with no patch key", files[3].Patch)
	}
}

// TestClient_GetChangedFiles_Paginates covers issue #76's verified repro: the
// PR files endpoint returns files 30 at a time by default, and reading only
// the first page silently dropped everything past file 30 on a larger PR.
// GetChangedFiles must now follow the Link "next" header until GitHub stops
// sending one, across a PR that needs more than one page at the 100/page
// this now requests.
func TestClient_GetChangedFiles_Paginates(t *testing.T) {
	const totalFiles = 120
	const perPage = 100

	var requests int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&requests, 1)

		if got := r.URL.Query().Get("per_page"); got != strconv.Itoa(perPage) {
			t.Errorf("per_page = %q, want %q", got, strconv.Itoa(perPage))
		}
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		if page == 0 {
			page = 1
		}

		start := (page - 1) * perPage
		end := start + perPage
		if end > totalFiles {
			end = totalFiles
		}

		files := make([]struct {
			Filename string `json:"filename"`
			Status   string `json:"status"`
		}, 0, end-start)
		for i := start; i < end; i++ {
			files = append(files, struct {
				Filename string `json:"filename"`
				Status   string `json:"status"`
			}{Filename: fmt.Sprintf("file%d.go", i), Status: "modified"})
		}

		if end < totalFiles {
			next := fmt.Sprintf("http://%s%s?page=%d&per_page=%d", r.Host, r.URL.Path, page+1, perPage)
			w.Header().Set("Link", fmt.Sprintf(`<%s>; rel="next"`, next))
		}
		json.NewEncoder(w).Encode(files)
	}))
	defer server.Close()

	client := &Client{Token: "test-token", Owner: "owner", Repo: "repo", BaseURL: server.URL}

	files, err := client.GetChangedFiles(1)
	if err != nil {
		t.Fatalf("GetChangedFiles() error = %v", err)
	}
	if len(files) != totalFiles {
		t.Errorf("got %d files, want %d (a PR spanning more than one page)", len(files), totalFiles)
	}
	if got := atomic.LoadInt32(&requests); got != 2 {
		t.Errorf("server received %d requests, want 2 (120 files at 100/page)", got)
	}
	if len(files) == totalFiles {
		if files[0].Path != "file0.go" {
			t.Errorf("files[0].Path = %q, want file0.go", files[0].Path)
		}
		wantLast := fmt.Sprintf("file%d.go", totalFiles-1)
		if files[totalFiles-1].Path != wantLast {
			t.Errorf("files[%d].Path = %q, want %q", totalFiles-1, files[totalFiles-1].Path, wantLast)
		}
	}
}

// TestNextPageLink covers the Link-header parsing GetChangedFiles' pagination
// depends on (issue #76): picking out the "next" URL among possibly several
// rel values, and returning "" once GitHub stops sending one.
func TestNextPageLink(t *testing.T) {
	tests := []struct {
		name   string
		header string
		want   string
	}{
		{
			name:   "next among next and last",
			header: `<https://api.github.com/repos/o/r/pulls/1/files?page=2&per_page=100>; rel="next", <https://api.github.com/repos/o/r/pulls/1/files?page=3&per_page=100>; rel="last"`,
			want:   "https://api.github.com/repos/o/r/pulls/1/files?page=2&per_page=100",
		},
		{
			name:   "last page has prev and first but no next",
			header: `<https://api.github.com/repos/o/r/pulls/1/files?page=2&per_page=100>; rel="prev", <https://api.github.com/repos/o/r/pulls/1/files?page=1&per_page=100>; rel="first"`,
			want:   "",
		},
		{
			name:   "single page has no Link header at all",
			header: "",
			want:   "",
		},
		{
			name:   "next with no other rel values",
			header: `<https://api.github.com/repos/o/r/pulls/1/files?page=2>; rel="next"`,
			want:   "https://api.github.com/repos/o/r/pulls/1/files?page=2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := nextPageLink(tt.header); got != tt.want {
				t.Errorf("nextPageLink(%q) = %q, want %q", tt.header, got, tt.want)
			}
		})
	}
}

func TestClient_FindExistingComment(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// issue #36: per_page must be sent explicitly so a busy PR doesn't
		// fall back to GitHub's 30-comment-per-page default.
		if got := r.URL.Query().Get("per_page"); got != "100" {
			t.Errorf("per_page = %q, want %q", got, "100")
		}
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

func TestClient_FindExistingComment_NotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		comments := []struct {
			ID   int    `json:"id"`
			Body string `json:"body"`
		}{
			{ID: 1, Body: "Some other comment"},
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
	if id != 0 {
		t.Errorf("FindExistingComment() = %v, want 0", id)
	}
}

// TestClient_FindExistingComment_Paginates covers issue #36's verified
// repro: the comments endpoint returns comments oldest-first, 30 at a time
// by default, and reading only the first page meant the litecov comment
// fell out of view as soon as a PR accumulated more comments than that.
// Past that point every push fell through to CreateComment instead of
// updating the existing comment in place. FindExistingComment must now
// request per_page=100 and follow the Link "next" header until it finds
// the marker or GitHub stops sending a next page.
func TestClient_FindExistingComment_Paginates(t *testing.T) {
	const totalComments = 150
	const perPage = 100
	const markerIndex = totalComments - 1 // last comment, forcing page 2

	var requests int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&requests, 1)

		if got := r.URL.Query().Get("per_page"); got != strconv.Itoa(perPage) {
			t.Errorf("per_page = %q, want %q", got, strconv.Itoa(perPage))
		}
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		if page == 0 {
			page = 1
		}

		start := (page - 1) * perPage
		end := start + perPage
		if end > totalComments {
			end = totalComments
		}

		comments := make([]struct {
			ID   int    `json:"id"`
			Body string `json:"body"`
		}, 0, end-start)
		for i := start; i < end; i++ {
			body := fmt.Sprintf("comment %d", i)
			if i == markerIndex {
				body = "<!-- litecov -->\n## Coverage Report"
			}
			comments = append(comments, struct {
				ID   int    `json:"id"`
				Body string `json:"body"`
			}{ID: i, Body: body})
		}

		if end < totalComments {
			next := fmt.Sprintf("http://%s%s?page=%d&per_page=%d", r.Host, r.URL.Path, page+1, perPage)
			w.Header().Set("Link", fmt.Sprintf(`<%s>; rel="next"`, next))
		}
		json.NewEncoder(w).Encode(comments)
	}))
	defer server.Close()

	client := &Client{Token: "test-token", Owner: "owner", Repo: "repo", BaseURL: server.URL}

	id, err := client.FindExistingComment(1, "<!-- litecov -->")
	if err != nil {
		t.Fatalf("FindExistingComment() error = %v", err)
	}
	if id != markerIndex {
		t.Errorf("FindExistingComment() = %v, want %v (the marker comment on page 2)", id, markerIndex)
	}
	if got := atomic.LoadInt32(&requests); got != 2 {
		t.Errorf("server received %d requests, want 2 (marker found on page 2, no need to request further)", got)
	}
}

func TestClient_CreateComment(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/repos/owner/repo/issues/1/comments" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]int{"id": 123})
	}))
	defer server.Close()

	client := &Client{
		Token:   "test-token",
		Owner:   "owner",
		Repo:    "repo",
		BaseURL: server.URL,
	}

	err := client.CreateComment(1, "test body")
	if err != nil {
		t.Fatalf("CreateComment() error = %v", err)
	}
}

// TestClient_SetCommitStatus covers issue #48: SetCommitStatus never sent
// target_url, so the check row on a PR had no "Details" link to get from a
// failing check to the report or the job log. targetURL == "" (no
// GITHUB_RUN_ID to build one from, e.g. a direct binary invocation outside
// Actions) must omit the key rather than send GitHub an empty string.
func TestClient_SetCommitStatus(t *testing.T) {
	tests := []struct {
		name       string
		targetURL  string
		wantHasKey bool
	}{
		{
			name:       "no target URL omits the key",
			targetURL:  "",
			wantHasKey: false,
		},
		{
			name:       "target URL is sent as-is",
			targetURL:  "https://github.com/owner/repo/actions/runs/123",
			wantHasKey: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var body map[string]any
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "POST" {
					t.Errorf("expected POST, got %s", r.Method)
				}
				if r.URL.Path != "/repos/owner/repo/statuses/abc123" {
					t.Errorf("unexpected path: %s", r.URL.Path)
				}
				json.NewDecoder(r.Body).Decode(&body)
				w.WriteHeader(http.StatusCreated)
				json.NewEncoder(w).Encode(map[string]string{"state": "success"})
			}))
			defer server.Close()

			client := &Client{
				Token:   "test-token",
				Owner:   "owner",
				Repo:    "repo",
				BaseURL: server.URL,
			}

			err := client.SetCommitStatus("abc123", "success", "85% coverage", "litecov", tt.targetURL)
			if err != nil {
				t.Fatalf("SetCommitStatus() error = %v", err)
			}

			got, hasKey := body["target_url"]
			if hasKey != tt.wantHasKey {
				t.Errorf("target_url present = %v, want %v (body: %v)", hasKey, tt.wantHasKey, body)
			}
			if hasKey && got != tt.targetURL {
				t.Errorf("target_url = %v, want %v", got, tt.targetURL)
			}
		})
	}
}

// TestNewClient covers issue #49: NewClient used to hardcode BaseURL to
// api.github.com, which meant an enterprise host's token was always sent to
// github.com's API instead. The base URL is now the caller's responsibility
// (main.go resolves it from GITHUB_API_URL), so NewClient must just carry
// whatever it's given, github.com default or GitHub Enterprise Server host
// alike, through unchanged.
func TestNewClient(t *testing.T) {
	tests := []struct {
		name    string
		baseURL string
	}{
		{name: "github.com default", baseURL: "https://api.github.com"},
		{name: "GitHub Enterprise Server host", baseURL: "https://ghe.example.com/api/v3"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := NewClient("token", "owner", "repo", tt.baseURL)
			if c.Token != "token" {
				t.Errorf("Token = %v, want token", c.Token)
			}
			if c.Owner != "owner" {
				t.Errorf("Owner = %v, want owner", c.Owner)
			}
			if c.Repo != "repo" {
				t.Errorf("Repo = %v, want repo", c.Repo)
			}
			if c.BaseURL != tt.baseURL {
				t.Errorf("BaseURL = %v, want %v", c.BaseURL, tt.baseURL)
			}
		})
	}
}

func TestClient_UpdateComment(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "PATCH" {
			t.Errorf("expected PATCH, got %s", r.Method)
		}
		if r.URL.Path != "/repos/owner/repo/issues/comments/42" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]int{"id": 42})
	}))
	defer server.Close()

	client := &Client{
		Token:   "test-token",
		Owner:   "owner",
		Repo:    "repo",
		BaseURL: server.URL,
	}

	err := client.UpdateComment(42, "updated body")
	if err != nil {
		t.Fatalf("UpdateComment() error = %v", err)
	}
}

func TestClient_GetChangedFiles_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte("not found"))
	}))
	defer server.Close()

	client := &Client{Token: "test", Owner: "o", Repo: "r", BaseURL: server.URL}
	_, err := client.GetChangedFiles(1)
	if err == nil {
		t.Error("expected error for 404 response")
	}
}

func TestClient_FindExistingComment_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte("forbidden"))
	}))
	defer server.Close()

	client := &Client{Token: "test", Owner: "o", Repo: "r", BaseURL: server.URL}
	_, err := client.FindExistingComment(1, "marker")
	if err == nil {
		t.Error("expected error for 403 response")
	}
}

func TestClient_CreateComment_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte("unauthorized"))
	}))
	defer server.Close()

	client := &Client{Token: "test", Owner: "o", Repo: "r", BaseURL: server.URL}
	err := client.CreateComment(1, "body")
	if err == nil {
		t.Error("expected error for 401 response")
	}
}

func TestClient_UpdateComment_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte("not found"))
	}))
	defer server.Close()

	client := &Client{Token: "test", Owner: "o", Repo: "r", BaseURL: server.URL}
	err := client.UpdateComment(999, "body")
	if err == nil {
		t.Error("expected error for 404 response")
	}
}

func TestClient_SetCommitStatus_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("server error"))
	}))
	defer server.Close()

	client := &Client{Token: "test", Owner: "o", Repo: "r", BaseURL: server.URL}
	err := client.SetCommitStatus("sha", "success", "desc", "ctx", "")
	if err == nil {
		t.Error("expected error for 500 response")
	}
}

func TestClient_GetChangedFiles_InvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("not json"))
	}))
	defer server.Close()

	client := &Client{Token: "test", Owner: "o", Repo: "r", BaseURL: server.URL}
	_, err := client.GetChangedFiles(1)
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestClient_FindExistingComment_InvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("not json"))
	}))
	defer server.Close()

	client := &Client{Token: "test", Owner: "o", Repo: "r", BaseURL: server.URL}
	_, err := client.FindExistingComment(1, "marker")
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestClient_doRequest_InvalidURL(t *testing.T) {
	client := &Client{Token: "test", Owner: "o", Repo: "r", BaseURL: "://invalid"}
	_, err := client.GetChangedFiles(1)
	if err == nil {
		t.Error("expected error for invalid URL")
	}
}

// TestClient_doRequest_Timeout covers issue #52's verified repro: a server
// that accepts the connection and never responds used to block the call
// forever, because http.DefaultClient has no timeout. httpClient.Timeout is
// lowered for the duration of the test so the assertion doesn't itself wait
// out a real 30s timeout.
func TestClient_doRequest_Timeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(300 * time.Millisecond)
	}))
	defer server.Close()

	origTimeout := httpClient.Timeout
	httpClient.Timeout = 20 * time.Millisecond
	defer func() { httpClient.Timeout = origTimeout }()

	client := &Client{Token: "test", Owner: "o", Repo: "r", BaseURL: server.URL}
	start := time.Now()
	_, err := client.GetChangedFiles(1)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected a timeout error, got nil")
	}
	if elapsed > 250*time.Millisecond {
		t.Errorf("doRequest took %v to fail, want well under the server's 300ms response delay", elapsed)
	}
}

func TestIsRateLimited(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		headers map[string]string
		want    bool
	}{
		{name: "429 is always rate limiting", status: http.StatusTooManyRequests, want: true},
		{
			name:    "403 with Retry-After is rate limiting",
			status:  http.StatusForbidden,
			headers: map[string]string{"Retry-After": "30"},
			want:    true,
		},
		{
			name:    "403 with X-RateLimit-Remaining 0 is rate limiting",
			status:  http.StatusForbidden,
			headers: map[string]string{"X-RateLimit-Remaining": "0"},
			want:    true,
		},
		{
			name:   "403 with no rate-limit headers is a permissions failure",
			status: http.StatusForbidden,
			want:   false,
		},
		{
			name:    "403 with X-RateLimit-Remaining nonzero is a permissions failure",
			status:  http.StatusForbidden,
			headers: map[string]string{"X-RateLimit-Remaining": "10"},
			want:    false,
		},
		{name: "500 is not rate limiting", status: http.StatusInternalServerError, want: false},
		{name: "200 is not rate limiting", status: http.StatusOK, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := &http.Response{StatusCode: tt.status, Header: http.Header{}}
			for k, v := range tt.headers {
				resp.Header.Set(k, v)
			}
			if got := isRateLimited(resp); got != tt.want {
				t.Errorf("isRateLimited() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestClient_doRequest_RetriesOn5xxThenSucceeds covers issue #52: a single
// 502 used to fail the call outright. It should now be retried until it
// succeeds (bounded by maxRetries).
func TestClient_doRequest_RetriesOn5xxThenSucceeds(t *testing.T) {
	var attempts int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&attempts, 1) < 3 {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]int{"id": 1})
	}))
	defer server.Close()

	client := &Client{Token: "test", Owner: "o", Repo: "r", BaseURL: server.URL}
	if err := client.CreateComment(1, "body"); err != nil {
		t.Fatalf("CreateComment() error = %v, want nil after transient 502s recover", err)
	}
	if got := atomic.LoadInt32(&attempts); got != 3 {
		t.Errorf("server received %d requests, want 3 (2 failed + 1 that succeeded)", got)
	}
}

// TestClient_doRequest_RetriesExhausted covers the "bounded attempts" half
// of issue #52's suggested fix: a persistently failing endpoint must still
// eventually return an error instead of retrying forever.
func TestClient_doRequest_RetriesExhausted(t *testing.T) {
	var attempts int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&attempts, 1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	client := &Client{Token: "test", Owner: "o", Repo: "r", BaseURL: server.URL}
	err := client.CreateComment(1, "body")
	if err == nil {
		t.Fatal("expected an error once retries are exhausted")
	}
	if got := atomic.LoadInt32(&attempts); got != maxRetries+1 {
		t.Errorf("server received %d requests, want %d (1 initial + %d retries)", got, maxRetries+1, maxRetries)
	}
}

// TestClient_doRequest_PermissionErrorNotRetried guards against retrying
// every 403 blindly: issue #52 notes that a rate limit and a genuine
// permissions failure both surface as 403, so only the rate-limit-flavored
// one should be retried. A plain 403 must fail on the first attempt.
func TestClient_doRequest_PermissionErrorNotRetried(t *testing.T) {
	var attempts int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&attempts, 1)
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(`{"message": "Resource not accessible by integration"}`))
	}))
	defer server.Close()

	client := &Client{Token: "test", Owner: "o", Repo: "r", BaseURL: server.URL}
	err := client.CreateComment(1, "body")
	if err == nil {
		t.Fatal("expected an error for a permissions failure")
	}
	if got := atomic.LoadInt32(&attempts); got != 1 {
		t.Errorf("server received %d requests, want 1 (a permissions failure is not retried)", got)
	}
	if strings.Contains(err.Error(), "rate limit") {
		t.Errorf("error = %q, should not read as rate limiting", err.Error())
	}
}

// TestClient_CreateComment_PermissionDenied covers issue #42's verified
// repro: a fork PR's read-only GITHUB_TOKEN gets a 403 "Resource not
// accessible by integration" from CreateComment. The returned error must
// satisfy errors.Is(err, ErrPermissionDenied) so main can tell it apart
// from a network failure or an exhausted retry and skip instead of failing
// the whole run.
func TestClient_CreateComment_PermissionDenied(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(`{"message": "Resource not accessible by integration"}`))
	}))
	defer server.Close()

	client := &Client{Token: "test", Owner: "o", Repo: "r", BaseURL: server.URL}
	err := client.CreateComment(1, "body")
	if !errors.Is(err, ErrPermissionDenied) {
		t.Errorf("CreateComment() error = %v, want errors.Is(err, ErrPermissionDenied)", err)
	}
}

// TestApiError_RateLimitMessage covers issue #52's complaint that a rate
// limit is reported as "GitHub API error: 403 Forbidden - ...", which reads
// identically to a permissions failure. The message must now name rate
// limiting explicitly so the two are distinguishable.
func TestApiError_RateLimitMessage(t *testing.T) {
	resp := &http.Response{
		StatusCode: http.StatusForbidden,
		Status:     "403 Forbidden",
		Header:     http.Header{"X-Ratelimit-Remaining": []string{"0"}},
		Body:       io.NopCloser(strings.NewReader("rate limit exceeded")),
	}
	err := apiError(resp)
	if !strings.Contains(err.Error(), "rate limit") {
		t.Errorf("error = %q, want it to mention rate limiting", err.Error())
	}
}

// TestApiError_PermissionDeniedWraps covers issue #42's detection step: a
// 403 or 404 that isn't rate limiting must wrap ErrPermissionDenied so
// callers can distinguish "the token can't do this" from a transient
// failure with errors.Is, while a rate-limited 403 and other status codes
// must not.
func TestApiError_PermissionDeniedWraps(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		headers http.Header
		want    bool
	}{
		{name: "plain 403 is permission denied", status: http.StatusForbidden, want: true},
		{name: "404 is permission denied", status: http.StatusNotFound, want: true},
		{
			name:    "rate-limited 403 is not permission denied",
			status:  http.StatusForbidden,
			headers: http.Header{"X-Ratelimit-Remaining": []string{"0"}},
			want:    false,
		},
		{name: "401 is not permission denied", status: http.StatusUnauthorized, want: false},
		{name: "500 is not permission denied", status: http.StatusInternalServerError, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			headers := tt.headers
			if headers == nil {
				headers = http.Header{}
			}
			resp := &http.Response{
				StatusCode: tt.status,
				Status:     fmt.Sprintf("%d %s", tt.status, http.StatusText(tt.status)),
				Header:     headers,
				Body:       io.NopCloser(strings.NewReader("body")),
			}
			err := apiError(resp)
			if got := errors.Is(err, ErrPermissionDenied); got != tt.want {
				t.Errorf("errors.Is(apiError(%d), ErrPermissionDenied) = %v, want %v (err = %q)", tt.status, got, tt.want, err)
			}
		})
	}
}

// TestClient_doRequest_SetsAPIVersionHeader covers the issue's minor note:
// requests set an Accept header but not X-GitHub-Api-Version, the currently
// documented way to pin the REST API version.
func TestClient_doRequest_SetsAPIVersionHeader(t *testing.T) {
	var got string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Get("X-GitHub-Api-Version")
		json.NewEncoder(w).Encode([]struct{}{})
	}))
	defer server.Close()

	client := &Client{Token: "test", Owner: "o", Repo: "r", BaseURL: server.URL}
	if _, err := client.GetChangedFiles(1); err != nil {
		t.Fatalf("GetChangedFiles() error = %v", err)
	}
	if got != "2022-11-28" {
		t.Errorf("X-GitHub-Api-Version = %q, want %q", got, "2022-11-28")
	}
}
