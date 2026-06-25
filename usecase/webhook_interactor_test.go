package usecase

import (
	"context"
	"errors"
	"testing"

	"github.com/koshuang/trello-ai-orchestrator/config"
	"github.com/koshuang/trello-ai-orchestrator/domain"
	"github.com/stretchr/testify/assert"
)

// Mock repos and gateways
type mockStateRepo struct {
	states map[string]*domain.WorkflowState
	saved  *domain.WorkflowState
}

func (m *mockStateRepo) GetByCardID(ctx context.Context, cardID string) (*domain.WorkflowState, error) {
	return m.states[cardID], nil
}

func (m *mockStateRepo) Save(ctx context.Context, state *domain.WorkflowState) error {
	m.saved = state
	m.states[state.TrelloCardID] = state
	return nil
}

type mockEventRepo struct {
	exists  bool
	created *domain.ProcessedEvent
}

func (m *mockEventRepo) Exists(ctx context.Context, provider string, eventID string) (bool, error) {
	return m.exists, nil
}

func (m *mockEventRepo) Create(ctx context.Context, event *domain.ProcessedEvent) error {
	m.created = event
	return nil
}

type mockTrelloGateway struct {
	cardContext   *domain.TrelloCardContext
	commentCardID string
	commentText   string
	updatedDescID string
	updatedDesc   string
}

func (m *mockTrelloGateway) FetchCardContext(ctx context.Context, cardID string) (*domain.TrelloCardContext, error) {
	if m.cardContext != nil {
		return m.cardContext, nil
	}
	return &domain.TrelloCardContext{
		ID:          cardID,
		Title:       "Test Card Title",
		Description: "Card Description",
		URL:         "https://trello.com/c/test",
	}, nil
}

func (m *mockTrelloGateway) AddComment(ctx context.Context, cardID string, text string) error {
	m.commentCardID = cardID
	m.commentText = text
	return nil
}

func (m *mockTrelloGateway) UpdateCardDescription(ctx context.Context, cardID string, description string) error {
	m.updatedDescID = cardID
	m.updatedDesc = description
	return nil
}

type mockGitHubGateway struct {
	createdPayload *domain.GitHubIssuePayload
	updatedNumber  int
	updatedPayload *domain.GitHubIssuePayload
}

func (m *mockGitHubGateway) CreateIssue(ctx context.Context, payload *domain.GitHubIssuePayload) (*domain.GitHubIssueResponse, error) {
	m.createdPayload = payload
	return &domain.GitHubIssueResponse{
		Number:  42,
		HTMLURL: "https://github.com/issues/42",
		Title:   payload.Title,
	}, nil
}

func (m *mockGitHubGateway) UpdateIssue(ctx context.Context, number int, payload *domain.GitHubIssuePayload) error {
	m.updatedNumber = number
	m.updatedPayload = payload
	return nil
}

type mockLLMGateway struct {
	response *domain.LLMResponse
	err      error
}

func (m *mockLLMGateway) Decide(ctx context.Context, input *domain.LLMInput) (*domain.LLMResponse, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.response, nil
}

func getBaseConfig() *config.Config {
	return &config.Config{
		TrelloAPIKey:          "key",
		TrelloToken:           "token",
		BotTrelloMemberIDs:    []string{"bot_user_1"},
		AutoReplyEnabled:      false,
		AutoCreateGitHubIssue: false,
		AutoCreatePlan:        false,
	}
}

func getSamplePayload() *domain.TrelloWebhookPayload {
	return &domain.TrelloWebhookPayload{
		Action: domain.TrelloAction{
			ID:   "action_123",
			Type: "commentCard",
			MemberCreator: domain.TrelloMember{
				ID:       "user_1",
				Username: "user_one",
			},
			Data: domain.TrelloActionData{
				Text: "@bot_user_1 please process this task.",
				Card: domain.TrelloCardShort{
					ID:        "card_123",
					Name:      "Sample Card Name",
					ShortLink: "samplelink",
				},
			},
		},
	}
}

func TestBotCommentIgnored(t *testing.T) {
	cfg := getBaseConfig()
	stateRepo := &mockStateRepo{states: make(map[string]*domain.WorkflowState)}
	eventRepo := &mockEventRepo{}
	trelloGate := &mockTrelloGateway{}
	githubGate := &mockGitHubGateway{}
	llmGate := &mockLLMGateway{}

	interactor := NewWebhookInteractor(cfg, stateRepo, eventRepo, trelloGate, githubGate, llmGate)

	payload := getSamplePayload()
	payload.Action.MemberCreator.ID = "bot_user_1" // Match bot ID

	err := interactor.ProcessEvent(context.Background(), payload)
	assert.NoError(t, err)
	assert.Nil(t, stateRepo.saved) // Should not process or save state
}

func TestDuplicateEventSkipped(t *testing.T) {
	cfg := getBaseConfig()
	stateRepo := &mockStateRepo{states: make(map[string]*domain.WorkflowState)}
	eventRepo := &mockEventRepo{exists: true} // Event already exists
	trelloGate := &mockTrelloGateway{}
	githubGate := &mockGitHubGateway{}
	llmGate := &mockLLMGateway{}

	interactor := NewWebhookInteractor(cfg, stateRepo, eventRepo, trelloGate, githubGate, llmGate)
	payload := getSamplePayload()

	err := interactor.ProcessEvent(context.Background(), payload)
	assert.NoError(t, err)
	assert.Nil(t, stateRepo.saved) // Should skip state saving
}

func TestWorkflowStateCreatedForNewCard(t *testing.T) {
	cfg := getBaseConfig()
	stateRepo := &mockStateRepo{states: make(map[string]*domain.WorkflowState)}
	eventRepo := &mockEventRepo{exists: false}
	trelloGate := &mockTrelloGateway{}
	githubGate := &mockGitHubGateway{}
	llmGate := &mockLLMGateway{
		response: &domain.LLMResponse{
			Action: "update_state_only",
			Reason: "new card detected",
			StateUpdate: domain.LLMStateUpdate{
				Status:               "needs_triage",
				Summary:              "triage card",
				CurrentUnderstanding: []string{"Item 1"},
				NextAction:           "Wait for feedback",
			},
		},
	}

	interactor := NewWebhookInteractor(cfg, stateRepo, eventRepo, trelloGate, githubGate, llmGate)
	payload := getSamplePayload()

	err := interactor.ProcessEvent(context.Background(), payload)
	assert.NoError(t, err)
	assert.NotNil(t, stateRepo.saved)
	assert.Equal(t, "card_123", stateRepo.saved.TrelloCardID)
	assert.Equal(t, domain.StatusNeedsTriage, stateRepo.saved.Status)
	assert.Equal(t, "action_123", stateRepo.saved.LastProcessedActionID)
	assert.Equal(t, "success", eventRepo.created.Status)
}

func TestWorkflowStateUpdatedForExistingCard(t *testing.T) {
	cfg := getBaseConfig()
	initialState := &domain.WorkflowState{
		TrelloCardID:          "card_123",
		TrelloCardTitle:       "Title",
		Status:                domain.StatusNeedsTriage,
		LastProcessedActionID: "action_111",
		CurrentUnderstanding:  []string{"Old Item"},
	}
	stateRepo := &mockStateRepo{
		states: map[string]*domain.WorkflowState{"card_123": initialState},
	}
	eventRepo := &mockEventRepo{exists: false}
	trelloGate := &mockTrelloGateway{}
	githubGate := &mockGitHubGateway{}
	llmGate := &mockLLMGateway{
		response: &domain.LLMResponse{
			Action: "update_state_only",
			Reason: "updated decisions",
			StateUpdate: domain.LLMStateUpdate{
				Status:               "ready_for_issue",
				Summary:              "summary changed",
				CurrentUnderstanding: []string{"Old Item", "New Item"},
				Decisions:            []string{"Plan OK"},
			},
		},
	}

	interactor := NewWebhookInteractor(cfg, stateRepo, eventRepo, trelloGate, githubGate, llmGate)
	payload := getSamplePayload()

	err := interactor.ProcessEvent(context.Background(), payload)
	assert.NoError(t, err)
	assert.NotNil(t, stateRepo.saved)
	assert.Equal(t, domain.StatusReadyForIssue, stateRepo.saved.Status)
	assert.Equal(t, "action_123", stateRepo.saved.LastProcessedActionID)
	assert.ElementsMatch(t, []string{"Old Item", "New Item"}, stateRepo.saved.CurrentUnderstanding)
}

func TestGitHubIssueNotDuplicatedIfAlreadyExists(t *testing.T) {
	cfg := getBaseConfig()
	cfg.AutoCreateGitHubIssue = true // Enable issue creation

	initialState := &domain.WorkflowState{
		TrelloCardID:   "card_123",
		GitHubIssueURL: "https://github.com/issues/55", // Already has issue
	}
	stateRepo := &mockStateRepo{
		states: map[string]*domain.WorkflowState{"card_123": initialState},
	}
	eventRepo := &mockEventRepo{exists: false}
	trelloGate := &mockTrelloGateway{}
	githubGate := &mockGitHubGateway{}
	llmGate := &mockLLMGateway{
		response: &domain.LLMResponse{
			Action: "create_github_issue", // LLM still decides to create issue
			Reason: "need issue",
			StateUpdate: domain.LLMStateUpdate{
				Status: "issue_created",
			},
			GitHubIssue: &domain.LLMGitHubIssue{
				Title: "Task Title",
				Body:  "Issue details",
			},
		},
	}

	interactor := NewWebhookInteractor(cfg, stateRepo, eventRepo, trelloGate, githubGate, llmGate)
	payload := getSamplePayload()

	err := interactor.ProcessEvent(context.Background(), payload)
	assert.NoError(t, err)
	assert.Nil(t, githubGate.createdPayload)             // GitHub API should NOT be called
	assert.Equal(t, "https://github.com/issues/55", stateRepo.saved.GitHubIssueURL) // URL kept
}

func TestSafeModeDoesNotPostWrites(t *testing.T) {
	cfg := getBaseConfig()
	cfg.AutoReplyEnabled = false
	cfg.AutoCreateGitHubIssue = false

	stateRepo := &mockStateRepo{states: make(map[string]*domain.WorkflowState)}
	eventRepo := &mockEventRepo{exists: false}
	trelloGate := &mockTrelloGateway{}
	githubGate := &mockGitHubGateway{}
	llmGate := &mockLLMGateway{
		response: &domain.LLMResponse{
			Action:       "reply_comment",
			Reason:       "comment reply needed",
			ReplyComment: "Safe reply comment",
			StateUpdate: domain.LLMStateUpdate{
				Status: "needs_pm_clarification",
			},
		},
	}

	interactor := NewWebhookInteractor(cfg, stateRepo, eventRepo, trelloGate, githubGate, llmGate)
	payload := getSamplePayload()

	err := interactor.ProcessEvent(context.Background(), payload)
	assert.NoError(t, err)
	assert.Empty(t, trelloGate.commentText) // Should not post reply comment
	assert.NotNil(t, stateRepo.saved)       // State should still be saved locally
	assert.Equal(t, domain.StatusNeedsPMClarification, stateRepo.saved.Status)
}

func TestInvalidLLMJSONHandledSafely(t *testing.T) {
	cfg := getBaseConfig()
	stateRepo := &mockStateRepo{states: make(map[string]*domain.WorkflowState)}
	eventRepo := &mockEventRepo{exists: false}
	trelloGate := &mockTrelloGateway{}
	githubGate := &mockGitHubGateway{}
	llmGate := &mockLLMGateway{
		err: errors.New("invalid JSON response format"),
	}

	interactor := NewWebhookInteractor(cfg, stateRepo, eventRepo, trelloGate, githubGate, llmGate)
	payload := getSamplePayload()

	err := interactor.ProcessEvent(context.Background(), payload)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid JSON response format")
	assert.Nil(t, stateRepo.saved) // State should not save on failure

	// An event should be created marking failure status
	assert.NotNil(t, eventRepo.created)
	assert.Equal(t, "failed", eventRepo.created.Status)
	assert.Contains(t, eventRepo.created.Error, "invalid JSON response format")
}
