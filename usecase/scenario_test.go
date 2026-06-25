package usecase

import (
	"context"
	"fmt"
	"testing"

	"github.com/koshuang/trello-ai-orchestrator/adapter/gateway"
	"github.com/koshuang/trello-ai-orchestrator/config"
	"github.com/koshuang/trello-ai-orchestrator/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Scenario-based webhook integration test suite.
// Run: go test -v -run "Scenario" ./usecase/

type scenario struct {
	cfg         *config.Config
	stateRepo   *mockStateRepo
	eventRepo   *mockEventRepo
	trelloGate  *mockTrelloGateway
	githubGate  *mockGitHubGateway
	llmGate     *mockLLMGateway
	useMockLLM  bool
	interactor  *WebhookInteractor
}

func newScenario(t *testing.T, opts ...scenarioOption) *scenario {
	t.Helper()
	s := &scenario{
		cfg:        getBaseConfig(),
		stateRepo:  &mockStateRepo{states: make(map[string]*domain.WorkflowState)},
		eventRepo:  &mockEventRepo{},
		trelloGate: &mockTrelloGateway{},
		githubGate: &mockGitHubGateway{},
		llmGate:    &mockLLMGateway{},
	}
	for _, opt := range opts {
		opt(s)
	}
	if s.useMockLLM {
		s.interactor = NewWebhookInteractor(
			s.cfg, s.stateRepo, s.eventRepo,
			s.trelloGate, s.githubGate, s.llmGate,
		)
	} else {
		realLLM := gateway.NewLLMClient(s.cfg)
		s.interactor = NewWebhookInteractor(
			s.cfg, s.stateRepo, s.eventRepo,
			s.trelloGate, s.githubGate, realLLM,
		)
	}
	return s
}

type scenarioOption func(*scenario)

func withConfig(fn func(c *config.Config)) scenarioOption {
	return func(s *scenario) { fn(s.cfg) }
}

func withExistingState(state *domain.WorkflowState) scenarioOption {
	return func(s *scenario) {
		s.stateRepo.states[state.TrelloCardID] = state
	}
}

func withLLMResponse(resp *domain.LLMResponse) scenarioOption {
	return func(s *scenario) {
		s.llmGate.response = resp
		s.useMockLLM = true
	}
}

func withTrelloContext(ctx *domain.TrelloCardContext) scenarioOption {
	return func(s *scenario) {
		s.trelloGate.cardContext = ctx
	}
}

func withExistingEvent() scenarioOption {
	return func(s *scenario) {
		s.eventRepo.exists = true
	}
}

func (s *scenario) exec(t *testing.T, payload *domain.TrelloWebhookPayload) error {
	t.Helper()
	return s.interactor.ProcessEvent(context.Background(), payload)
}

func makeComment(userID, username, commentText, cardID, cardName, shortLink string) *domain.TrelloWebhookPayload {
	return &domain.TrelloWebhookPayload{
		Action: domain.TrelloAction{
			ID:   "action_" + cardID,
			Type: "commentCard",
			MemberCreator: domain.TrelloMember{
				ID:       userID,
				Username: username,
				FullName: username,
			},
			Data: domain.TrelloActionData{
				Text: commentText,
				Card: domain.TrelloCardShort{
					ID:        cardID,
					Name:      cardName,
					ShortLink: shortLink,
				},
			},
		},
	}
}

func TestScenario_NoMention_Ignored(t *testing.T) {
	s := newScenario(t, withConfig(func(c *config.Config) {
		c.BotTrelloMemberIDs = []string{"my-bot"}
	}))

	payload := makeComment("user_1", "john", "please fix this bug", "card_001", "Bug Report", "br001")
	err := s.exec(t, payload)

	require.NoError(t, err)
	assert.Nil(t, s.stateRepo.saved, "no state should be saved when comment has no @mention")
	assert.Equal(t, "success", s.eventRepo.created.Status)
}

func TestScenario_AtMention_TriggersProcessing(t *testing.T) {
	s := newScenario(t, withConfig(func(c *config.Config) {
		c.BotTrelloMemberIDs = []string{"my-bot"}
	}))

	payload := makeComment("user_1", "john", "@my-bot please create issue for the login bug", "card_002", "Login Bug", "lb002")
	err := s.exec(t, payload)

	require.NoError(t, err)
	require.NotNil(t, s.stateRepo.saved)
	assert.Equal(t, "card_002", s.stateRepo.saved.TrelloCardID)
	assert.Equal(t, domain.StatusIssueCreated, s.stateRepo.saved.Status)
}

func TestScenario_BotComment_Ignored(t *testing.T) {
	s := newScenario(t, withConfig(func(c *config.Config) {
		c.BotTrelloMemberIDs = []string{"bot-user"}
	}))

	payload := makeComment("bot_id", "bot-user", "@bot-user doing something", "card_003", "Test", "t003")
	err := s.exec(t, payload)

	require.NoError(t, err)
	assert.Nil(t, s.stateRepo.saved)
}

func TestScenario_DuplicateEvent_Skipped(t *testing.T) {
	s := newScenario(t, withExistingEvent())

	payload := makeComment("user_1", "john", "hello", "card_004", "Duplicate", "d004")
	err := s.exec(t, payload)

	require.NoError(t, err)
	assert.Nil(t, s.stateRepo.saved)
}

func TestScenario_FixedCommand_CreateIssue(t *testing.T) {
	s := newScenario(t, withConfig(func(c *config.Config) {
		c.BotTrelloMemberIDs = []string{"my-bot"}
		c.AutoCreateGitHubIssue = true
	}))

	payload := makeComment("user_1", "john", "@my-bot !issue Fix the login timeout", "card_005", "Fix Login", "fl005")
	err := s.exec(t, payload)

	require.NoError(t, err)
	require.NotNil(t, s.githubGate.createdPayload)
	assert.Equal(t, "Fix the login timeout", s.githubGate.createdPayload.Title)
	assert.Equal(t, "issue_created", string(s.stateRepo.saved.Status))
}

func TestScenario_FixedCommand_CreatePlan(t *testing.T) {
	s := newScenario(t, withConfig(func(c *config.Config) {
		c.BotTrelloMemberIDs = []string{"my-bot"}
		c.AutoCreatePlan = true
	}))

	payload := makeComment("user_1", "john", "@my-bot !plan refactor auth module", "card_006", "Auth Refactor", "ar006")
	err := s.exec(t, payload)

	require.NoError(t, err)
	require.NotNil(t, s.stateRepo.saved)
	assert.Equal(t, "plan_created", string(s.stateRepo.saved.Status))
	assert.Contains(t, s.stateRepo.saved.PlanPath, "docs/plans/")
}

func TestScenario_FixedCommand_RunScript(t *testing.T) {
	s := newScenario(t, withConfig(func(c *config.Config) {
		c.BotTrelloMemberIDs = []string{"my-bot"}
		c.AutoReplyEnabled = true
	}))

	payload := makeComment("user_1", "john", "@my-bot !run deploy-staging", "card_007", "Deploy", "dp007")
	err := s.exec(t, payload)

	require.NoError(t, err)
	assert.Equal(t, "card_007", s.trelloGate.commentCardID)
	assert.Contains(t, s.trelloGate.commentText, "deploy-staging")
	assert.Equal(t, "implementation_in_progress", string(s.stateRepo.saved.Status))
}

func TestScenario_FixedCommand_Reply(t *testing.T) {
	s := newScenario(t, withConfig(func(c *config.Config) {
		c.BotTrelloMemberIDs = []string{"my-bot"}
		c.AutoReplyEnabled = true
	}))

	payload := makeComment("user_1", "john", "@my-bot !reply Got it, working on it now", "card_008", "Task", "tk008")
	err := s.exec(t, payload)

	require.NoError(t, err)
	assert.Contains(t, s.trelloGate.commentText, "Got it, working on it now")
}

func TestScenario_SafeMode_PreventsWrites(t *testing.T) {
	s := newScenario(t, withConfig(func(c *config.Config) {
		c.BotTrelloMemberIDs = []string{"my-bot"}
	}))

	payload := makeComment("user_1", "john", "@my-bot !issue Fix the login bug", "card_009", "Safe Test", "st009")
	err := s.exec(t, payload)

	require.NoError(t, err)
	assert.Nil(t, s.githubGate.createdPayload)
	assert.Empty(t, s.trelloGate.commentCardID)
	require.NotNil(t, s.stateRepo.saved)
	assert.Equal(t, "issue_created", string(s.stateRepo.saved.Status))
}

func TestScenario_ExistingIssue_NoDuplicate(t *testing.T) {
	s := newScenario(t,
		withConfig(func(c *config.Config) {
			c.BotTrelloMemberIDs = []string{"my-bot"}
			c.AutoCreateGitHubIssue = true
		}),
		withExistingState(&domain.WorkflowState{
			TrelloCardID:   "card_010",
			GitHubIssueURL: "https://github.com/owner/repo/issues/42",
		}),
	)

	payload := makeComment("user_1", "john", "@my-bot !issue Another attempt", "card_010", "Duplicate", "dp010")
	err := s.exec(t, payload)

	require.NoError(t, err)
	assert.Nil(t, s.githubGate.createdPayload)
	assert.Equal(t, "https://github.com/owner/repo/issues/42", s.stateRepo.saved.GitHubIssueURL)
}

func TestScenario_AIKeyword_AskKos(t *testing.T) {
	s := newScenario(t, withConfig(func(c *config.Config) {
		c.BotTrelloMemberIDs = []string{"my-bot"}
	}))

	payload := makeComment("user_1", "john", "@my-bot run claude analyze this PR", "card_011", "PR Analysis", "pr011")
	err := s.exec(t, payload)

	require.NoError(t, err)
	require.NotNil(t, s.stateRepo.saved)
	assert.Equal(t, "needs_triage", string(s.stateRepo.saved.Status))
}

func TestScenario_UnsupportedActionType_Ignored(t *testing.T) {
	s := newScenario(t)
	payload := makeComment("user_1", "john", "hello", "card_012", "Test", "t012")
	payload.Action.Type = "updateCard"

	err := s.exec(t, payload)

	require.NoError(t, err)
	assert.Nil(t, s.stateRepo.saved)
}

func TestScenario_NewCard_CreatesState(t *testing.T) {
	s := newScenario(t, withConfig(func(c *config.Config) {
		c.BotTrelloMemberIDs = []string{"my-bot"}
	}))

	payload := makeComment("user_1", "john", "@my-bot hello world", "card_new_001", "New Card", "nc001")
	err := s.exec(t, payload)

	require.NoError(t, err)
	require.NotNil(t, s.stateRepo.saved)
	assert.Equal(t, "card_new_001", s.stateRepo.saved.TrelloCardID)
	assert.NotEmpty(t, s.stateRepo.saved.TrelloCardURL)
	assert.Equal(t, domain.WorkflowStatus("needs_triage"), s.stateRepo.saved.Status)
}

func TestScenario_ExistingCard_UpdatesState(t *testing.T) {
	initial := &domain.WorkflowState{
		TrelloCardID:          "card_013",
		TrelloCardTitle:       "Old Title",
		Status:                domain.StatusNeedsTriage,
		LastProcessedActionID: "action_old",
		CurrentUnderstanding:  []string{"initial understanding"},
	}

	s := newScenario(t,
		withConfig(func(c *config.Config) {
			c.BotTrelloMemberIDs = []string{"my-bot"}
		}),
		withExistingState(initial),
	)

	payload := makeComment("user_1", "john", "@my-bot !issue detailed task", "card_013", "Updated Title", "ut013")
	err := s.exec(t, payload)

	require.NoError(t, err)
	require.NotNil(t, s.stateRepo.saved)
	assert.Equal(t, "card_013", s.stateRepo.saved.TrelloCardID)
	assert.Equal(t, domain.StatusIssueCreated, s.stateRepo.saved.Status)
	assert.NotEqual(t, "action_old", s.stateRepo.saved.LastProcessedActionID)
}

func TestScenario_LLMError_RecordsFailedEvent(t *testing.T) {
	s := newScenario(t,
		withConfig(func(c *config.Config) {
			c.BotTrelloMemberIDs = []string{"my-bot"}
		}),
		func(s *scenario) { s.useMockLLM = true },
	)
	s.llmGate.err = fmt.Errorf("LLM API timeout")

	payload := makeComment("user_1", "john", "@my-bot do something smart", "card_014", "AI Task", "ai014")
	err := s.exec(t, payload)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "LLM API timeout")
	assert.Nil(t, s.stateRepo.saved)
	require.NotNil(t, s.eventRepo.created)
	assert.Equal(t, "failed", s.eventRepo.created.Status)
	assert.Contains(t, s.eventRepo.created.Error, "LLM API timeout")
}

func TestScenario_MultipleMentions_SameCard(t *testing.T) {
	s := newScenario(t,
		withConfig(func(c *config.Config) {
			c.BotTrelloMemberIDs = []string{"my-bot"}
		}),
		withLLMResponse(&domain.LLMResponse{
			Action: "update_state_only",
			Reason: "first comment",
			StateUpdate: domain.LLMStateUpdate{
				Status:               "needs_triage",
				CurrentUnderstanding: []string{"first comment"},
			},
		}),
	)

	payload1 := makeComment("user_1", "alice", "@my-bot first task", "card_015", "Multiple", "ml015")
	err := s.exec(t, payload1)
	require.NoError(t, err)
	stateAfterFirst := s.stateRepo.saved
	firstActionID := stateAfterFirst.LastProcessedActionID

	s.llmGate = &mockLLMGateway{
		response: &domain.LLMResponse{
			Action: "update_state_only",
			Reason: "second comment processed",
			StateUpdate: domain.LLMStateUpdate{
				Status:               "ready_for_issue",
				Summary:              "Two comments received",
				CurrentUnderstanding: []string{"Requirement refined"},
			},
		},
	}
	s.interactor = NewWebhookInteractor(
		s.cfg, s.stateRepo, s.eventRepo,
		s.trelloGate, s.githubGate, s.llmGate,
	)

	payload2 := makeComment("user_2", "bob", "@my-bot also needs this", "card_015", "Multiple", "ml015")
	payload2.Action.ID = "action_card_015_2"
	err = s.exec(t, payload2)

	require.NoError(t, err)
	require.NotNil(t, s.stateRepo.saved)
	assert.Equal(t, s.stateRepo.saved.LastProcessedActionID, "action_card_015_2")
	assert.NotEqual(t, s.stateRepo.saved.LastProcessedActionID, firstActionID,
		"second comment should update last processed action ID")
}
