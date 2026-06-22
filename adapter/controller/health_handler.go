package controller

import (
	"database/sql"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/koshuang/trello-ai-orchestrator/usecase"
)

type HealthHandler struct {
	db        *sql.DB
	stateRepo usecase.WorkflowStateRepository
}

func NewHealthHandler(db *sql.DB, stateRepo usecase.WorkflowStateRepository) *HealthHandler {
	return &HealthHandler{
		db:        db,
		stateRepo: stateRepo,
	}
}

// Health checks the health of the application and its database connection
func (h *HealthHandler) Health(c *gin.Context) {
	if err := h.db.Ping(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status":   "unhealthy",
			"database": "disconnected",
			"error":    err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":   "healthy",
		"database": "connected",
	})
}

// GetState fetches the durable workflow state for a given card ID
func (h *HealthHandler) GetState(c *gin.Context) {
	cardID := c.Param("trelloCardId")
	if cardID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "trelloCardId is required"})
		return
	}

	state, err := h.stateRepo.GetByCardID(c.Request.Context(), cardID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if state == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "no workflow state found for card ID: " + cardID})
		return
	}

	c.JSON(http.StatusOK, state)
}
