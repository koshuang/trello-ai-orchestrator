package controller

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/koshuang/trello-ai-orchestrator/adapter/gateway"
	"github.com/koshuang/trello-ai-orchestrator/config"
	"github.com/koshuang/trello-ai-orchestrator/domain"
	"github.com/koshuang/trello-ai-orchestrator/usecase"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// HTTP-level integration test for Trello webhook endpoint.
// Starts a real Gin server with mocked dependencies and sends actual HTTP
// requests, exercising the full handler -> interactor -> gateway chain.
//
// Run:
//
//	go test -v -run "Integration" ./adapter/controller/ -count=1
// ---------------------------------------------------------------------------

// -- mock implementations that satisfy usecase interface contracts --

type mockStateRepo struct {
	states map[string]*domain.WorkflowState
	saved  *domain.WorkflowState
}

func (m *mockStateRepo) GetByCardID(_ context.Context, cardID string) (*domain.WorkflowState, error) {
	if m.states == nil {
		return nil, nil
	}
	s, ok := m.states[cardID]
	if !ok {
		return nil, nil
	}
	return s, nil
}

func (m *mockStateRepo) Save(_ context.Context, state *domain.WorkflowState) error {
	m.saved = state
	if m.states == nil {
		m.states = make(map[string]*domain.WorkflowState)
	}
	m.states[state.TrelloCardID] = state
	return nil
}

type mockEventRepo struct {
	exists  bool
	created *domain.ProcessedEvent
}

func (m *mockEventRepo) Exists(_ context.Context, provider string, eventID string) (bool, error) {
	return m.exists, nil
}

func (m *mockEventRepo) Create(_ context.Context, event *domain.ProcessedEvent) error {
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

func (m *mockTrelloGateway) FetchCardContext(_ context.Context, cardID string) (*domain.TrelloCardContext, error) {
	if m.cardContext != nil {
		return m.cardContext, nil
	}
	return &domain.TrelloCardContext{
		ID:          cardID,
		Title:       "Test Card Title",
		Description: "Card Description",
		URL:         "https://trello.com/c/test123",
	}, nil
}

func (m *mockTrelloGateway) AddComment(_ context.Context, cardID string, text string) error {
	m.commentCardID = cardID
	m.commentText = text
	return nil
}

func (m *mockTrelloGateway) UpdateCardDescription(_ context.Context, cardID string, description string) error {
	m.updatedDescID = cardID
	m.updatedDesc = description
	return nil
}

type mockGitHubGateway struct {
	createdPayload *domain.GitHubIssuePayload
}

func (m *mockGitHubGateway) CreateIssue(_ context.Context, payload *domain.GitHubIssuePayload) (*domain.GitHubIssueResponse, error) {
	m.createdPayload = payload
	return &domain.GitHubIssueResponse{
		Number:  42,
		HTMLURL: "https://github.com/issues/42",
		Title:   payload.Title,
		State:   "open",
	}, nil
}

func (m *mockGitHubGateway) UpdateIssue(_ context.Context, _ int, _ *domain.GitHubIssuePayload) error {
	return nil
}

// --------------- test helpers ---------------

func setupServer(t *testing.T, botIDs []string, autoCreateIssue bool) (*httptest.Server, *mockStateRepo, *mockTrelloGateway, *mockGitHubGateway) {
	t.Helper()

	// Config requires TRELLO_API_KEY / TRELLO_TOKEN for validation.
	os.Setenv("TRELLO_API_KEY", "test-key")
	os.Setenv("TRELLO_TOKEN", "test-token")
	t.Cleanup(func() {
		os.Unsetenv("TRELLO_API_KEY")
		os.Unsetenv("TRELLO_TOKEN")
	})

	cfg, err := config.LoadConfig()
	require.NoError(t, err)
	cfg.BotTrelloMemberIDs = botIDs
	cfg.AutoCreateGitHubIssue = autoCreateIssue

	stateRepo := &mockStateRepo{}
	eventRepo := &mockEventRepo{}
	trelloGate := &mockTrelloGateway{}
	githubGate := &mockGitHubGateway{}
	llmGate := gateway.NewLLMClient(cfg)

	interactor := usecase.NewWebhookInteractor(
		cfg, stateRepo, eventRepo, trelloGate, githubGate, llmGate,
	)
	handler := NewWebhookHandler(cfg, interactor)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/webhooks/trello", handler.HandlePost)

	server := httptest.NewServer(r)
	t.Cleanup(server.Close)
	return server, stateRepo, trelloGate, githubGate
}

func sendWebhook(t *testing.T, server *httptest.Server, payload interface{}) *http.Response {
	t.Helper()
	body, err := json.Marshal(payload)
	require.NoError(t, err)
	req, err := http.NewRequest(http.MethodPost, server.URL+"/webhooks/trello", bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	return resp
}

// --------------- Integration scenarios ---------------

func TestIntegration_NoMention_ReturnsOK(t *testing.T) {
	server, stateRepo, _, _ := setupServer(t, []string{"my-bot"}, false)

	payload := map[string]interface{}{
		"action": map[string]interface{}{
			"id":   "int_001",
			"type": "commentCard",
			"memberCreator": map[string]interface{}{
				"id":       "user_1",
				"username": "john",
				"fullName": "John",
			},
			"data": map[string]interface{}{
				"text": "please fix this bug",
				"card": map[string]interface{}{
					"id":        "card_int_001",
					"name":      "Bug Report",
					"shortLink": "br001",
				},
			},
		},
	}

	resp := sendWebhook(t, server, payload)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Nil(t, stateRepo.saved, "no state saved when comment has no @mention")
}

func TestIntegration_AtMention_TriggersProcessing(t *testing.T) {
	server, stateRepo, _, _ := setupServer(t, []string{"my-bot"}, false)

	payload := map[string]interface{}{
		"action": map[string]interface{}{
			"id":   "int_002",
			"type": "commentCard",
			"memberCreator": map[string]interface{}{
				"id":       "user_1",
				"username": "john",
				"fullName": "John",
			},
			"data": map[string]interface{}{
				"text": "@my-bot please create issue for the login bug",
				"card": map[string]interface{}{
					"id":        "card_int_002",
					"name":      "Login Bug",
					"shortLink": "lb002",
				},
			},
		},
	}

	resp := sendWebhook(t, server, payload)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	require.NotNil(t, stateRepo.saved)
	assert.Equal(t, "card_int_002", stateRepo.saved.TrelloCardID)
}

func TestIntegration_FixedCommand_CreateIssue(t *testing.T) {
	server, stateRepo, trelloGate, githubGate := setupServer(t, []string{"my-bot"}, true)

	payload := map[string]interface{}{
		"action": map[string]interface{}{
			"id":   "int_003",
			"type": "commentCard",
			"memberCreator": map[string]interface{}{
				"id":       "user_1",
				"username": "john",
				"fullName": "John",
			},
			"data": map[string]interface{}{
				"text": "@my-bot !issue Fix the login timeout",
				"card": map[string]interface{}{
					"id":        "card_int_003",
					"name":      "Fix Login",
					"shortLink": "fl003",
				},
			},
		},
	}

	resp := sendWebhook(t, server, payload)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	require.NotNil(t, githubGate.createdPayload)
	assert.Equal(t, "Fix the login timeout", githubGate.createdPayload.Title)
	assert.Equal(t, "issue_created", string(stateRepo.saved.Status))
	assert.Equal(t, "card_int_003", trelloGate.updatedDescID, "should sync AI state to Trello description")
}

func TestIntegration_BotComment_Ignored(t *testing.T) {
	server, stateRepo, _, _ := setupServer(t, []string{"bot-user"}, false)

	payload := map[string]interface{}{
		"action": map[string]interface{}{
			"id":   "int_004",
			"type": "commentCard",
			"memberCreator": map[string]interface{}{
				"id":       "bot_id",
				"username": "bot-user",
				"fullName": "Bot",
			},
			"data": map[string]interface{}{
				"text": "@bot-user doing something",
				"card": map[string]interface{}{
					"id":        "card_int_004",
					"name":      "Test",
					"shortLink": "t004",
				},
			},
		},
	}

	resp := sendWebhook(t, server, payload)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Nil(t, stateRepo.saved)
}

func TestIntegration_UnsupportedActionType_Ignored(t *testing.T) {
	server, stateRepo, _, _ := setupServer(t, nil, false)

	payload := map[string]interface{}{
		"action": map[string]interface{}{
			"id":   "int_005",
			"type": "updateCard",
			"memberCreator": map[string]interface{}{
				"id":       "user_1",
				"username": "john",
			},
			"data": map[string]interface{}{
				"card": map[string]interface{}{
					"id":   "card_int_005",
					"name": "Test",
				},
			},
		},
	}

	resp := sendWebhook(t, server, payload)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Nil(t, stateRepo.saved)
}

func TestIntegration_SafeMode_PreventsWrites(t *testing.T) {
	server, stateRepo, trelloGate, githubGate := setupServer(t, []string{"my-bot"}, false)

	payload := map[string]interface{}{
		"action": map[string]interface{}{
			"id":   "int_006",
			"type": "commentCard",
			"memberCreator": map[string]interface{}{
				"id":       "user_1",
				"username": "john",
				"fullName": "John",
			},
			"data": map[string]interface{}{
				"text": "@my-bot !issue Fix the login bug",
				"card": map[string]interface{}{
					"id":        "card_int_006",
					"name":      "Safe Test",
					"shortLink": "st006",
				},
			},
		},
	}

	resp := sendWebhook(t, server, payload)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Nil(t, githubGate.createdPayload)
	assert.Empty(t, trelloGate.commentCardID)
	require.NotNil(t, stateRepo.saved)
	assert.Equal(t, "issue_created", string(stateRepo.saved.Status))
}

func TestIntegration_InvalidJSON_Returns400(t *testing.T) {
	server, _, _, _ := setupServer(t, nil, false)

	body := bytes.NewReader([]byte(`{invalid json`))
	req, err := http.NewRequest(http.MethodPost, server.URL+"/webhooks/trello", body)
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}
