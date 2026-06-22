package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/koshuang/trello-ai-orchestrator/config"
	"github.com/koshuang/trello-ai-orchestrator/domain"
)

type GitHubClient struct {
	cfg        *config.Config
	httpClient *http.Client
}

func NewGitHubClient(cfg *config.Config) *GitHubClient {
	return &GitHubClient{
		cfg:        cfg,
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

// CreateIssue creates a new issue on GitHub
func (c *GitHubClient) CreateIssue(ctx context.Context, payload *domain.GitHubIssuePayload) (*domain.GitHubIssueResponse, error) {
	u := fmt.Sprintf("https://api.github.com/repos/%s/%s/issues", c.cfg.GitHubOwner, c.cfg.GitHubRepo)
	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", u, bytes.NewBuffer(bodyBytes))
	if err != nil {
		return nil, err
	}

	c.setHeaders(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("github create issue returned status: %d", resp.StatusCode)
	}

	var issueResp domain.GitHubIssueResponse
	if err := json.NewDecoder(resp.Body).Decode(&issueResp); err != nil {
		return nil, err
	}

	return &issueResp, nil
}

// UpdateIssue updates an existing GitHub issue (e.g. description or title)
func (c *GitHubClient) UpdateIssue(ctx context.Context, number int, payload *domain.GitHubIssuePayload) error {
	u := fmt.Sprintf("https://api.github.com/repos/%s/%s/issues/%d", c.cfg.GitHubOwner, c.cfg.GitHubRepo, number)
	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, "PATCH", u, bytes.NewBuffer(bodyBytes))
	if err != nil {
		return err
	}

	c.setHeaders(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("github update issue returned status: %d", resp.StatusCode)
	}

	return nil
}

func (c *GitHubClient) setHeaders(req *http.Request) {
	req.Header.Set("Authorization", "Bearer "+c.cfg.GitHubToken)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", "Go-Trello-AI-Orchestrator")
	req.Header.Set("Content-Type", "application/json")
}
