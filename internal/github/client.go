// Package github provides a client for interacting with the GitHub API.
package github

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// httpClient is shared by every request. The zero-value http.Client used
// before has no timeout, so a server that accepts the connection and never
// responds blocks the call forever instead of failing.
var httpClient = &http.Client{Timeout: 30 * time.Second}

// sleep is retry backoff's wait function, a package var so tests can no-op
// it instead of paying real exponential-backoff delays.
var sleep = time.Sleep

const (
	// maxRetries bounds retry attempts on 5xx and rate-limited responses.
	maxRetries = 4
	// baseRetryDelay is the backoff for the first retry; it doubles on each
	// later attempt when GitHub hasn't told us exactly how long to wait.
	baseRetryDelay = 1 * time.Second
)

type Client struct {
	Token   string
	Owner   string
	Repo    string
	BaseURL string
}

func NewClient(token, owner, repo, baseURL string) *Client {
	return &Client{
		Token:   token,
		Owner:   owner,
		Repo:    repo,
		BaseURL: baseURL,
	}
}

// doRequest sends the request, retrying 5xx responses and rate limiting
// (429, or 403 with a rate-limit header) per GitHub's best-practices guidance:
// https://docs.github.com/en/rest/using-the-rest-api/best-practices-for-using-the-rest-api
// body is a byte slice rather than an io.Reader because a retried request
// needs to be re-sent from the start each attempt.
func (c *Client) doRequest(method, path string, body []byte) (*http.Response, error) {
	for attempt := 0; ; attempt++ {
		var bodyReader io.Reader
		if body != nil {
			bodyReader = bytes.NewReader(body)
		}

		req, err := http.NewRequest(method, c.BaseURL+path, bodyReader)
		if err != nil {
			return nil, err
		}

		req.Header.Set("Authorization", "Bearer "+c.Token)
		req.Header.Set("Accept", "application/vnd.github.v3+json")
		req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}

		resp, err := httpClient.Do(req)
		if err != nil {
			return nil, err
		}

		if attempt >= maxRetries || !shouldRetry(resp) {
			return resp, nil
		}

		wait := retryDelay(resp, attempt)
		resp.Body.Close()
		sleep(wait)
	}
}

// shouldRetry reports whether resp is worth retrying: a server error, or
// rate limiting rather than a genuine failure.
func shouldRetry(resp *http.Response) bool {
	if resp.StatusCode >= 500 && resp.StatusCode < 600 {
		return true
	}
	return isRateLimited(resp)
}

// isRateLimited reports whether resp signals GitHub rate limiting rather
// than an ordinary error. 429 always means rate limiting; 403 is ambiguous
// between rate limiting and a real permissions failure, so it only counts
// when the response carries the headers GitHub uses for rate limiting.
func isRateLimited(resp *http.Response) bool {
	if resp.StatusCode == http.StatusTooManyRequests {
		return true
	}
	if resp.StatusCode != http.StatusForbidden {
		return false
	}
	return resp.Header.Get("Retry-After") != "" || resp.Header.Get("X-RateLimit-Remaining") == "0"
}

// retryDelay picks how long to wait before the next attempt, preferring
// GitHub's own timing over a guess: a Retry-After header wins outright, then
// a reset time when the rate limit is exhausted, and only then a plain
// exponential backoff with jitter for ordinary server errors.
func retryDelay(resp *http.Response, attempt int) time.Duration {
	if ra := resp.Header.Get("Retry-After"); ra != "" {
		if secs, err := strconv.Atoi(ra); err == nil && secs > 0 {
			return time.Duration(secs) * time.Second
		}
	}

	if resp.Header.Get("X-RateLimit-Remaining") == "0" {
		if reset := resp.Header.Get("X-RateLimit-Reset"); reset != "" {
			if ts, err := strconv.ParseInt(reset, 10, 64); err == nil {
				if wait := time.Until(time.Unix(ts, 0)); wait > 0 {
					return wait
				}
			}
		}
	}

	backoff := baseRetryDelay << attempt
	return backoff + time.Duration(rand.Int63n(int64(backoff)))
}

// apiError builds the error for a non-success response, calling out rate
// limiting by name so it doesn't read like a permissions failure -- both
// surface as a 403 with an otherwise identical "GitHub API error" message.
func apiError(resp *http.Response) error {
	body, _ := io.ReadAll(resp.Body)
	if isRateLimited(resp) {
		return fmt.Errorf("GitHub API rate limit exceeded: %s - %s", resp.Status, string(body))
	}
	return fmt.Errorf("GitHub API error: %s - %s", resp.Status, string(body))
}

// ChangedFile is one entry from a pull request's file diff.
type ChangedFile struct {
	Path      string
	IsAdded   bool // True when GitHub reports this file's diff status as "added"
	IsRemoved bool // True when GitHub reports this file's diff status as "removed"
	// Patch holds the file's unified diff hunks exactly as GitHub returns
	// them: starting at the first "@@" line, with no "diff --git"/"---"/"+++"
	// header. It's empty for binary files, renames with no content change,
	// and diffs past GitHub's per-file size limit -- callers should treat an
	// empty Patch as "no coverable changed lines", not as an error.
	Patch string
}

func (c *Client) GetChangedFiles(prNumber int) ([]ChangedFile, error) {
	path := fmt.Sprintf("/repos/%s/%s/pulls/%d/files", c.Owner, c.Repo, prNumber)
	resp, err := c.doRequest("GET", path, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, apiError(resp)
	}

	var files []struct {
		Filename string `json:"filename"`
		Status   string `json:"status"`
		Patch    string `json:"patch"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&files); err != nil {
		return nil, err
	}

	result := make([]ChangedFile, len(files))
	for i, f := range files {
		result[i] = ChangedFile{
			Path:      f.Filename,
			IsAdded:   f.Status == "added",
			IsRemoved: f.Status == "removed",
			Patch:     f.Patch,
		}
	}
	return result, nil
}

func (c *Client) FindExistingComment(prNumber int, marker string) (int, error) {
	path := fmt.Sprintf("/repos/%s/%s/issues/%d/comments", c.Owner, c.Repo, prNumber)
	resp, err := c.doRequest("GET", path, nil)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, apiError(resp)
	}

	var comments []struct {
		ID   int    `json:"id"`
		Body string `json:"body"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&comments); err != nil {
		return 0, err
	}

	for _, comment := range comments {
		if strings.HasPrefix(comment.Body, marker) {
			return comment.ID, nil
		}
	}
	return 0, nil
}

func (c *Client) CreateComment(prNumber int, body string) error {
	path := fmt.Sprintf("/repos/%s/%s/issues/%d/comments", c.Owner, c.Repo, prNumber)
	payload, _ := json.Marshal(map[string]string{"body": body})

	resp, err := c.doRequest("POST", path, payload)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		return apiError(resp)
	}
	return nil
}

func (c *Client) UpdateComment(commentID int, body string) error {
	path := fmt.Sprintf("/repos/%s/%s/issues/comments/%d", c.Owner, c.Repo, commentID)
	payload, _ := json.Marshal(map[string]string{"body": body})

	resp, err := c.doRequest("PATCH", path, payload)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return apiError(resp)
	}
	return nil
}

func (c *Client) SetCommitStatus(sha, state, description, context string) error {
	path := fmt.Sprintf("/repos/%s/%s/statuses/%s", c.Owner, c.Repo, sha)
	payload, _ := json.Marshal(map[string]string{
		"state":       state,
		"description": description,
		"context":     context,
	})

	resp, err := c.doRequest("POST", path, payload)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		return apiError(resp)
	}
	return nil
}
