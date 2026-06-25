package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/koshuang/trello-ai-orchestrator/config"
	"github.com/koshuang/trello-ai-orchestrator/domain"
)

type LLMClient struct {
	cfg        *config.Config
	httpClient *http.Client
}

func NewLLMClient(cfg *config.Config) *LLMClient {
	return &LLMClient{
		cfg:        cfg,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

// Decide executes the decision engine using LLM or fallback stub
func (c *LLMClient) Decide(ctx context.Context, input *domain.LLMInput) (*domain.LLMResponse, error) {
	if c.cfg.LLMAPIKey == "" {
		log.Println("[LLM] LLM_API_KEY is empty. Running in basic decision engine stub mode.")
		return c.decideStub(input)
	}

	prompt := c.buildPrompt(input)

	var responseText string
	var err error

	if strings.ToLower(c.cfg.LLMProvider) == "anthropic" {
		responseText, err = c.callClaude(ctx, prompt)
	} else {
		responseText, err = c.callGemini(ctx, prompt)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to call LLM: %w", err)
	}

	return c.parseResponse(responseText)
}

func (c *LLMClient) callGemini(ctx context.Context, prompt string) (string, error) {
	// Using standard Gemini 1.5 Flash API
	u := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/gemini-1.5-flash:generateContent?key=%s", c.cfg.LLMAPIKey)

	payload := map[string]interface{}{
		"contents": []interface{}{
			map[string]interface{}{
				"parts": []interface{}{
					map[string]interface{}{
						"text": prompt,
					},
				},
			},
		},
		"generationConfig": map[string]interface{}{
			"responseMimeType": "application/json",
		},
	}

	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", u, bytes.NewBuffer(bodyBytes))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("gemini API returned status %d: %s", resp.StatusCode, string(respBody))
	}

	var rawResponse struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&rawResponse); err != nil {
		return "", err
	}

	if len(rawResponse.Candidates) == 0 || len(rawResponse.Candidates[0].Content.Parts) == 0 {
		return "", fmt.Errorf("empty response received from Gemini")
	}

	return rawResponse.Candidates[0].Content.Parts[0].Text, nil
}

func (c *LLMClient) callClaude(ctx context.Context, prompt string) (string, error) {
	u := "https://api.anthropic.com/v1/messages"

	payload := map[string]interface{}{
		"model":      "claude-3-5-sonnet-20241022",
		"max_tokens": 4000,
		"system":     "You are a Trello workflow manager. You must return raw JSON only following the requested schema. Do not output anything else.",
		"messages": []interface{}{
			map[string]interface{}{
				"role":    "user",
				"content": prompt,
			},
		},
	}

	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", u, bytes.NewBuffer(bodyBytes))
	if err != nil {
		return "", err
	}
	req.Header.Set("x-api-key", c.cfg.LLMAPIKey)
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("anthropic API returned status %d: %s", resp.StatusCode, string(respBody))
	}

	var rawResponse struct {
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&rawResponse); err != nil {
		return "", err
	}

	if len(rawResponse.Content) == 0 {
		return "", fmt.Errorf("empty response received from Claude")
	}

	return rawResponse.Content[0].Text, nil
}

func (c *LLMClient) buildPrompt(input *domain.LLMInput) string {
	cardCtxJSON, _ := json.MarshalIndent(input.CardContext, "", "  ")
	stateJSON, _ := json.MarshalIndent(input.WorkflowState, "", "  ")

	return fmt.Sprintf(`You are a senior backend workflow assistant orchestrating a Trello board task.
A new comment has been made on the Trello card.

New Trello Comment:
%q

Current Trello Card Context:
%s

Durable Workflow State (from DB):
%s

Operating Rules:
%s

Analyze the situation and reply ONLY in structured JSON matching this schema:
{
  "action": "reply_comment | create_github_issue | update_github_issue | create_plan | update_plan | update_state_only | ignore | ask_kos",
  "reason": "short explanation",
  "reply_comment": "comment text if action is reply_comment",
  "state_update": {
    "status": "status_value", // e.g. needs_pm_clarification, issue_created, plan_created, etc.
    "summary": "overall task summary",
    "current_understanding": ["understanding points..."],
    "decisions": ["decisions made..."],
    "open_questions": ["unresolved questions..."],
    "next_action": "next concrete action description"
  },
  "github_issue": {
    "title": "GitHub issue title",
    "body": "GitHub issue markdown description",
    "labels": ["label1", "label2"]
  },
  "plan": {
    "path": "docs/plans/trello-{cardId}-{slug}.md",
    "content": "Full plan markdown text"
  }
}

Important Instructions:
- Output valid JSON only. Do not wrap the JSON in generic conversations.
- Do not run risky actions if requirements are ambiguous. Set action to 'ask_kos' or 'update_state_only'.
- Match status field to one of these: new, needs_triage, needs_pm_clarification, ready_for_issue, issue_created, plan_created, ready_for_implementation, implementation_in_progress, waiting_for_review, done, ignored, error.
`, input.NewComment, string(cardCtxJSON), string(stateJSON), input.OperatingRules)
}

func (c *LLMClient) parseResponse(text string) (*domain.LLMResponse, error) {
	// Strip out markdown code blocks if the LLM output is wrapped in ```json
	re := regexp.MustCompile(`(?s)(?:^|[\r\n])\x60\x60\x60(?:json)?\s*(.*?)\s*\x60\x60\x60`)
	matches := re.FindStringSubmatch(text)
	var jsonStr string
	if len(matches) > 1 {
		jsonStr = matches[1]
	} else {
		jsonStr = strings.TrimSpace(text)
	}

	var response domain.LLMResponse
	if err := json.Unmarshal([]byte(jsonStr), &response); err != nil {
		return nil, fmt.Errorf("failed to parse JSON from LLM: %w (raw response: %s)", err, text)
	}

	return &response, nil
}

func (c *LLMClient) decideStub(input *domain.LLMInput) (*domain.LLMResponse, error) {
	comment := input.NewComment
	commentLower := strings.ToLower(comment)

	// Strip leading @mentions for command matching
	cleaned := stripMentions(commentLower)

	action := "update_state_only"
	status := "needs_triage"
	reason := "Fixed behavior (stub decision engine)"
	var githubIssue *domain.LLMGitHubIssue
	var plan *domain.LLMPlan
	replyComment := ""

	if strings.HasPrefix(cleaned, "!issue") {
		action = "create_github_issue"
		status = "issue_created"
		title := extractArg(comment, "!issue", input.CardContext.Title)
		githubIssue = &domain.LLMGitHubIssue{
			Title:  title,
			Body:   fmt.Sprintf("Created from Trello Card: %s\n\nDescription:\n%s", input.CardContext.URL, input.CardContext.Description),
			Labels: []string{"trello-sync"},
		}
	} else if strings.HasPrefix(cleaned, "!plan") {
		action = "create_plan"
		status = "plan_created"
		slug := slugify(input.CardContext.Title)
		plan = &domain.LLMPlan{
			Path: fmt.Sprintf("docs/plans/trello-%s-%s.md", input.CardContext.ID, slug),
			Content: fmt.Sprintf(`# Plan: %s

Trello: %s
GitHub Issue: %s

## Background
Proposed plan from !plan command.

## Requirements
- %s
`, input.CardContext.Title, input.CardContext.URL, input.WorkflowState.GitHubIssueURL, extractArg(comment, "!plan", "")),
		}
	} else if strings.HasPrefix(cleaned, "!run") {
		action = "reply_comment"
		status = "implementation_in_progress"
		scriptName := extractArg(comment, "!run", "default")
		replyComment = fmt.Sprintf("Acknowledged !run command. Triggering script: %s", scriptName)
	} else if strings.HasPrefix(cleaned, "!reply") {
		action = "reply_comment"
		status = "needs_pm_clarification"
		replyComment = extractArg(comment, "!reply", "")
		if replyComment == "" {
			replyComment = "Hello, thanks for the ping! How can I help?"
		}
	} else if strings.Contains(cleaned, "create issue") || strings.Contains(cleaned, "github issue") {
		action = "create_github_issue"
		status = "issue_created"
		githubIssue = &domain.LLMGitHubIssue{
			Title:  fmt.Sprintf("Task: %s", input.CardContext.Title),
			Body:   fmt.Sprintf("Created from Trello Card: %s\n\nDescription:\n%s", input.CardContext.URL, input.CardContext.Description),
			Labels: []string{"trello-sync"},
		}
	} else if strings.Contains(cleaned, "create plan") || strings.Contains(cleaned, "implementation plan") {
		action = "create_plan"
		status = "plan_created"
		slug := slugify(input.CardContext.Title)
		plan = &domain.LLMPlan{
			Path: fmt.Sprintf("docs/plans/trello-%s-%s.md", input.CardContext.ID, slug),
			Content: fmt.Sprintf(`# Plan: %s

Trello: %s
GitHub Issue: %s

## Background
Proposed plan background details.

## Current Understanding
- ...

## Requirements
- ...
`, input.CardContext.Title, input.CardContext.URL, input.WorkflowState.GitHubIssueURL),
		}
	} else if strings.Contains(cleaned, "run claude") || strings.Contains(cleaned, "run codex") {
		action = "ask_kos"
		status = "needs_triage"
		reason = "AI behavior requested but API key not configured. Would route to Claude/Codex when LLM is enabled."
	} else if strings.Contains(cleaned, "reply") || strings.Contains(cleaned, "clarify") {
		action = "reply_comment"
		status = "needs_pm_clarification"
		replyComment = "Hello, thanks for the comment! Could you please clarify the requirements further?"
	} else if strings.Contains(commentLower, "ignore") {
		action = "ignore"
		status = "ignored"
	} else {
		reason = "No fixed pattern matched; would fall back to AI behavior when LLM is configured."
		action = "update_state_only"
		status = "needs_triage"
	}

	understanding := []string{fmt.Sprintf("Comment analyzed: %q", input.NewComment)}
	decisions := []string{fmt.Sprintf("Action selected: %s", action)}
	openQuestions := []string{}
	if reason == "No fixed pattern matched; would fall back to AI behavior when LLM is configured." {
		openQuestions = append(openQuestions, "Would benefit from LLM-driven analysis")
	}

	return &domain.LLMResponse{
		Action:       action,
		Reason:       reason,
		ReplyComment: replyComment,
		StateUpdate: domain.LLMStateUpdate{
			Status:               status,
			Summary:              fmt.Sprintf("Stub processed comment: %q", input.NewComment),
			CurrentUnderstanding: understanding,
			Decisions:            decisions,
			OpenQuestions:        openQuestions,
			NextAction:           "Verify results",
		},
		GitHubIssue: githubIssue,
		Plan:        plan,
	}, nil
}

// stripMentions removes leading @mentions from text for command matching
func stripMentions(text string) string {
	// Match one or more leading @mention tokens (e.g. "@bot !plan foo" → "!plan foo")
	re := regexp.MustCompile(`^(?:@\S+\s+)+`)
	if loc := re.FindStringIndex(text); loc != nil {
		return text[loc[1]:]
	}
	return text
}

// extractArg("!issue Fix login bug", "!issue", "default") → "Fix login bug"
func extractArg(comment, prefix, fallback string) string {
	lower := strings.ToLower(comment)
	idx := strings.Index(lower, strings.ToLower(prefix))
	if idx < 0 {
		return fallback
	}
	after := strings.TrimSpace(comment[idx+len(prefix):])
	if after == "" {
		return fallback
	}
	return after
}

func slugify(s string) string {
	s = strings.ToLower(s)
	s = strings.ReplaceAll(s, " ", "-")
	reg := regexp.MustCompile("[^a-z0-9_-]")
	s = reg.ReplaceAllString(s, "")
	if len(s) > 30 {
		s = s[:30]
	}
	return strings.Trim(s, "-")
}

