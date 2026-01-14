package github

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type Client struct {
	Token   string
	Owner   string
	Repo    string
	BaseURL string
}

func NewClient(token, owner, repo string) *Client {
	return &Client{
		Token:   token,
		Owner:   owner,
		Repo:    repo,
		BaseURL: "https://api.github.com",
	}
}

func (c *Client) doRequest(method, path string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequest(method, c.BaseURL+path, body)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	return http.DefaultClient.Do(req)
}

func (c *Client) GetChangedFiles(prNumber int) ([]string, error) {
	path := fmt.Sprintf("/repos/%s/%s/pulls/%d/files", c.Owner, c.Repo, prNumber)
	resp, err := c.doRequest("GET", path, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("GitHub API error: %s - %s", resp.Status, string(body))
	}

	var files []struct {
		Filename string `json:"filename"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&files); err != nil {
		return nil, err
	}

	result := make([]string, len(files))
	for i, f := range files {
		result[i] = f.Filename
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
		body, _ := io.ReadAll(resp.Body)
		return 0, fmt.Errorf("GitHub API error: %s - %s", resp.Status, string(body))
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

	resp, err := c.doRequest("POST", path, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("GitHub API error: %s - %s", resp.Status, string(respBody))
	}
	return nil
}

func (c *Client) UpdateComment(commentID int, body string) error {
	path := fmt.Sprintf("/repos/%s/%s/issues/comments/%d", c.Owner, c.Repo, commentID)
	payload, _ := json.Marshal(map[string]string{"body": body})

	resp, err := c.doRequest("PATCH", path, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("GitHub API error: %s - %s", resp.Status, string(respBody))
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

	resp, err := c.doRequest("POST", path, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("GitHub API error: %s - %s", resp.Status, string(respBody))
	}
	return nil
}
