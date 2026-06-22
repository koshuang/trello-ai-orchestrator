package main

import (
	"log"
	"net/http"
	"os"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	"github.com/koshuang/trello-ai-orchestrator/config"
	"github.com/koshuang/trello-ai-orchestrator/infrastructure/database"
	"github.com/koshuang/trello-ai-orchestrator/adapter/controller"
	"github.com/koshuang/trello-ai-orchestrator/adapter/gateway"
	"github.com/koshuang/trello-ai-orchestrator/adapter/repository"
	"github.com/koshuang/trello-ai-orchestrator/usecase"
)

func main() {
	log.Println("[Main] Starting Trello AI Orchestrator Service...")

	// 1. Load env variables from .env file (if it exists)
	if err := godotenv.Load(); err != nil {
		log.Println("[Main] Info: .env file not found, using system environment variables")
	}

	// 2. Load and validate Configuration
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("[Main] Configuration loading failed: %v", err)
	}

	// 3. Initialize SQLite Database
	db, err := database.InitDB(cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("[Main] Database initialization failed: %v", err)
	}
	defer db.Close()

	// 4. Initialize Repository
	repo := repository.NewSQLiteRepo(db)

	// 5. Initialize Gateways (Clients)
	trelloClient := gateway.NewTrelloClient(cfg)
	githubClient := gateway.NewGitHubClient(cfg)
	llmClient := gateway.NewLLMClient(cfg)

	// 6. Initialize Interactor (Usecase)
	interactor := usecase.NewWebhookInteractor(
		cfg,
		repo,
		repo,
		trelloClient,
		githubClient,
		llmClient,
	)

	// 7. Initialize Handlers (Controllers)
	webhookHandler := controller.NewWebhookHandler(cfg, interactor)
	healthHandler := controller.NewHealthHandler(db, repo)

	// 8. Set up Gin HTTP router
	// Set Gin mode based on environment
	if os.Getenv("GIN_MODE") == "release" {
		gin.SetMode(gin.ReleaseMode)
	}
	r := gin.Default()

	// 9. Routes Setup
	r.GET("/health", healthHandler.Health)
	r.GET("/workflow-states/:trelloCardId", healthHandler.GetState)

	// Trello webhooks (require HEAD validation and POST callback)
	r.HEAD("/webhooks/trello", webhookHandler.HandleHead)
	r.POST("/webhooks/trello", webhookHandler.HandlePost)

	// Root landing
	r.GET("/", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"service": "Trello AI Orchestrator",
			"version": "1.0.0",
			"status":  "running",
		})
	})

	// 10. Start Server
	addr := ":" + cfg.Port
	log.Printf("[Main] Server starting on port %s...", cfg.Port)
	if err := r.Run(addr); err != nil {
		log.Fatalf("[Main] Server failed to run: %v", err)
	}
}
