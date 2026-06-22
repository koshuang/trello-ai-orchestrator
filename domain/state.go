package domain

import (
	"encoding/json"
	"time"
)

type WorkflowStatus string

const (
	StatusNew                      WorkflowStatus = "new"
	StatusNeedsTriage              WorkflowStatus = "needs_triage"
	StatusNeedsPMClarification    WorkflowStatus = "needs_pm_clarification"
	StatusReadyForIssue            WorkflowStatus = "ready_for_issue"
	StatusIssueCreated             WorkflowStatus = "issue_created"
	StatusPlanCreated              WorkflowStatus = "plan_created"
	StatusReadyForImplementation   WorkflowStatus = "ready_for_implementation"
	StatusImplementationInProgress WorkflowStatus = "implementation_in_progress"
	StatusWaitingForReview         WorkflowStatus = "waiting_for_review"
	StatusDone                     WorkflowStatus = "done"
	StatusIgnored                  WorkflowStatus = "ignored"
	StatusError                    WorkflowStatus = "error"
)

type WorkflowState struct {
	ID                    int64          `json:"id"`
	TrelloCardID          string         `json:"trello_card_id"`
	TrelloCardURL         string         `json:"trello_card_url"`
	TrelloCardTitle       string         `json:"trello_card_title"`
	Status                WorkflowStatus `json:"status"`
	LastProcessedActionID string         `json:"last_processed_action_id"`
	GitHubIssueURL        string         `json:"github_issue_url"`
	GitHubIssueNumber     int            `json:"github_issue_number"`
	PlanPath              string         `json:"plan_path"`
	Summary               string         `json:"summary"`
	CurrentUnderstanding  []string       `json:"current_understanding"` // represented as JSON array in DB
	Decisions             []string       `json:"decisions"`             // represented as JSON array in DB
	OpenQuestions         []string       `json:"open_questions"`         // represented as JSON array in DB
	NextAction            string         `json:"next_action"`
	UpdatedBy             string         `json:"updated_by"`
	CreatedAt             time.Time      `json:"created_at"`
	UpdatedAt             time.Time      `json:"updated_at"`
}

// Helpers to serialize/deserialize string slices to JSON for DB usage
func MarshalSlice(slice []string) string {
	if slice == nil {
		return "[]"
	}
	bytes, err := json.Marshal(slice)
	if err != nil {
		return "[]"
	}
	return string(bytes)
}

func UnmarshalSlice(data string) []string {
	var slice []string
	if data == "" {
		return []string{}
	}
	err := json.Unmarshal([]byte(data), &slice)
	if err != nil {
		return []string{}
	}
	return slice
}
