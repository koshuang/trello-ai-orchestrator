package usecase

import (
	"context"

	"github.com/koshuang/trello-ai-orchestrator/domain"
)

type WorkflowStateRepository interface {
	GetByCardID(ctx context.Context, cardID string) (*domain.WorkflowState, error)
	Save(ctx context.Context, state *domain.WorkflowState) error
}

type ProcessedEventRepository interface {
	Exists(ctx context.Context, provider string, eventID string) (bool, error)
	Create(ctx context.Context, event *domain.ProcessedEvent) error
}

type TrelloGateway interface {
	FetchCardContext(ctx context.Context, cardID string) (*domain.TrelloCardContext, error)
	AddComment(ctx context.Context, cardID string, text string) error
	UpdateCardDescription(ctx context.Context, cardID string, description string) error
}

type GitHubGateway interface {
	CreateIssue(ctx context.Context, payload *domain.GitHubIssuePayload) (*domain.GitHubIssueResponse, error)
	UpdateIssue(ctx context.Context, number int, payload *domain.GitHubIssuePayload) error
}

type LLMGateway interface {
	Decide(ctx context.Context, input *domain.LLMInput) (*domain.LLMResponse, error)
}
