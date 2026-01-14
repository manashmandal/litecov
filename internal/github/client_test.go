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
