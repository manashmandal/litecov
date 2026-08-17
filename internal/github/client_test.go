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

func TestClient_SetCommitStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/repos/owner/repo/statuses/abc123" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
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

	err := client.SetCommitStatus("abc123", "success", "85% coverage", "litecov")
	if err != nil {
		t.Fatalf("SetCommitStatus() error = %v", err)
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
	err := client.SetCommitStatus("sha", "success", "desc", "ctx")
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
