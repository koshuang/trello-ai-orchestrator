package repository

import (
	"context"
	"database/sql"
	"errors"

	"github.com/koshuang/trello-ai-orchestrator/domain"
)

type SQLiteRepo struct {
	db *sql.DB
}

func NewSQLiteRepo(db *sql.DB) *SQLiteRepo {
	return &SQLiteRepo{db: db}
}

// Exists checks if an event has already been processed (for idempotency)
func (r *SQLiteRepo) Exists(ctx context.Context, provider string, eventID string) (bool, error) {
	var exists bool
	query := "SELECT EXISTS(SELECT 1 FROM processed_events WHERE provider = ? AND event_id = ?)"
	err := r.db.QueryRowContext(ctx, query, provider, eventID).Scan(&exists)
	if err != nil {
		return false, err
	}
	return exists, nil
}

// Create stores a processed event record
func (r *SQLiteRepo) Create(ctx context.Context, event *domain.ProcessedEvent) error {
	query := `
	INSERT INTO processed_events (id, provider, event_id, trello_card_id, event_type, processed_at, status, error)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`
	_, err := r.db.ExecContext(ctx, query,
		event.ID,
		event.Provider,
		event.EventID,
		event.TrelloCardID,
		event.EventType,
		event.ProcessedAt,
		event.Status,
		event.Error,
	)
	return err
}

// GetByCardID retrieves the workflow state of a specific Trello card
func (r *SQLiteRepo) GetByCardID(ctx context.Context, cardID string) (*domain.WorkflowState, error) {
	query := `
	SELECT id, trello_card_id, trello_card_url, trello_card_title, status, last_processed_action_id,
	       github_issue_url, github_issue_number, plan_path, summary, current_understanding,
	       decisions_json, open_questions_json, next_action, updated_by, created_at, updated_at
	FROM workflow_states
	WHERE trello_card_id = ?
	`
	row := r.db.QueryRowContext(ctx, query, cardID)

	var state domain.WorkflowState
	var statusStr string
	var understandingJSON, decisionsJSON, questionsJSON string

	err := row.Scan(
		&state.ID,
		&state.TrelloCardID,
		&state.TrelloCardURL,
		&state.TrelloCardTitle,
		&statusStr,
		&state.LastProcessedActionID,
		&state.GitHubIssueURL,
		&state.GitHubIssueNumber,
		&state.PlanPath,
		&state.Summary,
		&understandingJSON,
		&decisionsJSON,
		&questionsJSON,
		&state.NextAction,
		&state.UpdatedBy,
		&state.CreatedAt,
		&state.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	state.Status = domain.WorkflowStatus(statusStr)
	state.CurrentUnderstanding = domain.UnmarshalSlice(understandingJSON)
	state.Decisions = domain.UnmarshalSlice(decisionsJSON)
	state.OpenQuestions = domain.UnmarshalSlice(questionsJSON)

	return &state, nil
}

// Save inserts or updates a workflow state record (UPSERT)
func (r *SQLiteRepo) Save(ctx context.Context, state *domain.WorkflowState) error {
	understandingJSON := domain.MarshalSlice(state.CurrentUnderstanding)
	decisionsJSON := domain.MarshalSlice(state.Decisions)
	questionsJSON := domain.MarshalSlice(state.OpenQuestions)

	query := `
	INSERT INTO workflow_states (
		trello_card_id, trello_card_url, trello_card_title, status, last_processed_action_id,
		github_issue_url, github_issue_number, plan_path, summary, current_understanding,
		decisions_json, open_questions_json, next_action, updated_by, created_at, updated_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(trello_card_id) DO UPDATE SET
		trello_card_url = excluded.trello_card_url,
		trello_card_title = excluded.trello_card_title,
		status = excluded.status,
		last_processed_action_id = excluded.last_processed_action_id,
		github_issue_url = excluded.github_issue_url,
		github_issue_number = excluded.github_issue_number,
		plan_path = excluded.plan_path,
		summary = excluded.summary,
		current_understanding = excluded.current_understanding,
		decisions_json = excluded.decisions_json,
		open_questions_json = excluded.open_questions_json,
		next_action = excluded.next_action,
		updated_by = excluded.updated_by,
		updated_at = excluded.updated_at
	`
	_, err := r.db.ExecContext(ctx, query,
		state.TrelloCardID,
		state.TrelloCardURL,
		state.TrelloCardTitle,
		state.Status,
		state.LastProcessedActionID,
		state.GitHubIssueURL,
		state.GitHubIssueNumber,
		state.PlanPath,
		state.Summary,
		understandingJSON,
		decisionsJSON,
		questionsJSON,
		state.NextAction,
		state.UpdatedBy,
		state.CreatedAt,
		state.UpdatedAt,
	)
	return err
}
