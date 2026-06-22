package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	TrelloAPIKey           string
	TrelloToken            string
	TrelloWebhookSecret    string
	TrelloBoardID          string
	GitHubToken            string
	GitHubOwner            string
	GitHubRepo             string
	LLMAPIKey              string
	LLMProvider            string // "gemini" or "anthropic", default "gemini"
	DatabaseURL            string // e.g. "./orchestrator.db"
	BotTrelloMemberIDs     []string
	AutoReplyEnabled       bool
	AutoCreateGitHubIssue  bool
	AutoCreatePlan         bool
	Port                   string
}

func LoadConfig() (*Config, error) {
	port := getEnv("PORT", "8080")
	dbURL := getEnv("DATABASE_URL", "./orchestrator.db")
	llmProvider := getEnv("LLM_PROVIDER", "gemini")

	botIDsStr := os.Getenv("BOT_TRELLO_MEMBER_IDS")
	var botIDs []string
	if botIDsStr != "" {
		parts := strings.Split(botIDsStr, ",")
		for _, part := range parts {
			trimmed := strings.TrimSpace(part)
			if trimmed != "" {
				botIDs = append(botIDs, trimmed)
			}
		}
	}

	cfg := &Config{
		TrelloAPIKey:           os.Getenv("TRELLO_API_KEY"),
		TrelloToken:            os.Getenv("TRELLO_TOKEN"),
		TrelloWebhookSecret:    os.Getenv("TRELLO_WEBHOOK_SECRET"),
		TrelloBoardID:          os.Getenv("TRELLO_BOARD_ID"),
		GitHubToken:            os.Getenv("GITHUB_TOKEN"),
		GitHubOwner:            os.Getenv("GITHUB_OWNER"),
		GitHubRepo:             os.Getenv("GITHUB_REPO"),
		LLMAPIKey:              os.Getenv("LLM_API_KEY"),
		LLMProvider:            llmProvider,
		DatabaseURL:            dbURL,
		BotTrelloMemberIDs:     botIDs,
		AutoReplyEnabled:       getEnvBool("AUTO_REPLY_ENABLED", false),
		AutoCreateGitHubIssue:  getEnvBool("AUTO_CREATE_GITHUB_ISSUE", false),
		AutoCreatePlan:         getEnvBool("AUTO_CREATE_PLAN", false),
		Port:                   port,
	}

	// Basic validation
	if cfg.TrelloAPIKey == "" {
		return nil, fmt.Errorf("missing TRELLO_API_KEY environment variable")
	}
	if cfg.TrelloToken == "" {
		return nil, fmt.Errorf("missing TRELLO_TOKEN environment variable")
	}

	return cfg, nil
}

func getEnv(key, defaultVal string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return defaultVal
}

func getEnvBool(key string, defaultVal bool) bool {
	valStr := os.Getenv(key)
	if valStr == "" {
		return defaultVal
	}
	val, err := strconv.ParseBool(valStr)
	if err != nil {
		return defaultVal
	}
	return val
}
