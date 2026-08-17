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
// needs to be re-sent from the start each attempt. path is normally relative
// to c.BaseURL, but a caller walking a paginated response (issue #76) passes
// the full "next" URL GitHub returned instead, which is used as-is rather
// than appended to BaseURL a second time.
func (c *Client) doRequest(method, path string, body []byte) (*http.Response, error) {
	reqURL := path
	if !strings.HasPrefix(path, "http://") && !strings.HasPrefix(path, "https://") {
		reqURL = c.BaseURL + path
	}

	for attempt := 0; ; attempt++ {
		var bodyReader io.Reader
		if body != nil {
			bodyReader = bytes.NewReader(body)
		}

		req, err := http.NewRequest(method, reqURL, bodyReader)
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

const (
	// changedFilesPerPage is the page size requested for GetChangedFiles.
	// GitHub's default is 30 files/page, which without an explicit
	// per_page silently truncated any PR touching more than 30 files
	// (issue #76).
	changedFilesPerPage = 100
	// maxChangedFilesPages bounds pagination to GitHub's own documented
	// ceiling for this endpoint ("Responses include a maximum of 3000
	// files"), 30 pages at 100 files each. Guards against looping forever
	// if a Link header were ever to point back at a page already seen.
	maxChangedFilesPages = 30
)

// nextPageLink extracts the "next" URL from a GitHub Link header (RFC 5988),
// or "" once there's no next page. A paginated response's header looks like:
//
//	<https://api.github.com/...?page=2>; rel="next", <https://api.github.com/...?page=4>; rel="last"
func nextPageLink(header string) string {
	for _, part := range strings.Split(header, ",") {
		segments := strings.Split(part, ";")
		if len(segments) < 2 {
			continue
		}
		link := strings.TrimSpace(segments[0])
		link = strings.TrimPrefix(link, "<")
		link = strings.TrimSuffix(link, ">")
		for _, seg := range segments[1:] {
			if strings.TrimSpace(seg) == `rel="next"` {
				return link
			}
		}
	}
	return ""
}

// GetChangedFiles returns every file touched by the pull request, following
// GitHub's pagination until it stops sending a "next" link. The endpoint
// defaults to 30 files/page, so reading only the first page (as this used
// to) silently dropped everything past file 30 on larger PRs (issue #76).
func (c *Client) GetChangedFiles(prNumber int) ([]ChangedFile, error) {
	path := fmt.Sprintf("/repos/%s/%s/pulls/%d/files?per_page=%d", c.Owner, c.Repo, prNumber, changedFilesPerPage)

	var result []ChangedFile
	for page := 0; path != "" && page < maxChangedFilesPages; page++ {
		resp, err := c.doRequest("GET", path, nil)
		if err != nil {
			return nil, err
		}

		if resp.StatusCode != http.StatusOK {
			err := apiError(resp)
			resp.Body.Close()
			return nil, err
		}

		var files []struct {
			Filename string `json:"filename"`
			Status   string `json:"status"`
			Patch    string `json:"patch"`
		}
		decodeErr := json.NewDecoder(resp.Body).Decode(&files)
		next := resp.Header.Get("Link")
		resp.Body.Close()
		if decodeErr != nil {
			return nil, decodeErr
		}

		for _, f := range files {
			result = append(result, ChangedFile{
				Path:      f.Filename,
				IsAdded:   f.Status == "added",
				IsRemoved: f.Status == "removed",
				Patch:     f.Patch,
			})
		}

		path = nextPageLink(next)
	}

	return result, nil
}

const (
	// commentsPerPage is the page size requested for FindExistingComment.
	// GitHub's default is 30 comments/page, which without an explicit
	// per_page dropped the marker comment off page one as soon as a PR
	// picked up more comments than that (issue #36).
	commentsPerPage = 100
	// maxCommentsPages bounds pagination so an unusually long comment
	// thread can't loop forever chasing "next" links.
	maxCommentsPages = 100
)

// FindExistingComment returns the ID of the first comment on the PR whose
// body starts with marker, walking every page of comments (issue #36).
// GitHub returns comments oldest-first, 30/page by default; reading only
// the first page meant the litecov comment fell out of view on any PR that
// accumulated more comments than that, and every push after fell through to
// CreateComment instead of updating the existing comment in place. Returns
// 0 with a nil error when no page contains a match.
func (c *Client) FindExistingComment(prNumber int, marker string) (int, error) {
	path := fmt.Sprintf("/repos/%s/%s/issues/%d/comments?per_page=%d", c.Owner, c.Repo, prNumber, commentsPerPage)

	for page := 0; path != "" && page < maxCommentsPages; page++ {
		resp, err := c.doRequest("GET", path, nil)
		if err != nil {
			return 0, err
		}

		if resp.StatusCode != http.StatusOK {
			err := apiError(resp)
			resp.Body.Close()
			return 0, err
		}

		var comments []struct {
			ID   int    `json:"id"`
			Body string `json:"body"`
		}
		decodeErr := json.NewDecoder(resp.Body).Decode(&comments)
		next := resp.Header.Get("Link")
		resp.Body.Close()
		if decodeErr != nil {
			return 0, decodeErr
		}

		for _, comment := range comments {
			if strings.HasPrefix(comment.Body, marker) {
				return comment.ID, nil
			}
		}

		path = nextPageLink(next)
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
