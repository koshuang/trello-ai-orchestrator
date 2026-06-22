package controller

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"io"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/koshuang/trello-ai-orchestrator/config"
	"github.com/koshuang/trello-ai-orchestrator/domain"
	"github.com/koshuang/trello-ai-orchestrator/usecase"
)

type WebhookHandler struct {
	cfg        *config.Config
	interactor *usecase.WebhookInteractor
}

func NewWebhookHandler(cfg *config.Config, interactor *usecase.WebhookInteractor) *WebhookHandler {
	return &WebhookHandler{
		cfg:        cfg,
		interactor: interactor,
	}
}

// HandleHead handles Trello's webhook verification HEAD requests
func (h *WebhookHandler) HandleHead(c *gin.Context) {
	c.Status(http.StatusOK)
}

// HandlePost processes incoming Trello webhook events
func (h *WebhookHandler) HandlePost(c *gin.Context) {
	// 1. Read Raw Body for signature verification
	rawBody, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "unable to read body"})
		return
	}
	// Restore request body reader so JSON binding works afterwards
	c.Request.Body = io.NopCloser(bytes.NewBuffer(rawBody))

	// 2. Trello Webhook Verification (HMAC-SHA1 of raw body + request URL)
	if h.cfg.TrelloWebhookSecret != "" {
		sigHeader := c.GetHeader("x-trello-webhook")
		if sigHeader == "" {
			log.Println("[Webhook] Request rejected: missing x-trello-webhook header")
			c.JSON(http.StatusUnauthorized, gin.H{"error": "missing signature header"})
			return
		}

		// Reconstruct full request URL as registered on Trello.
		// NOTE: Trello uses the exact callback URL. Make sure it matches.
		scheme := "http"
		if c.Request.TLS != nil || c.Request.Header.Get("X-Forwarded-Proto") == "https" {
			scheme = "https"
		}
		requestURL := scheme + "://" + c.Request.Host + c.Request.URL.RequestURI()

		if !verifyTrelloSignature(rawBody, requestURL, h.cfg.TrelloWebhookSecret, sigHeader) {
			log.Printf("[Webhook] Request rejected: signature verification failed for URL: %s", requestURL)
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid signature"})
			return
		}
	}

	// 3. Bind JSON Payload
	var payload domain.TrelloWebhookPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid json payload: " + err.Error()})
		return
	}

	// 4. Pre-filter relevant events (we prioritize commentCard for now)
	actionType := payload.Action.Type
	if actionType != "commentCard" {
		log.Printf("[Webhook] Ignored action type %s (only commentCard is processed)", actionType)
		c.JSON(http.StatusOK, gin.H{"status": "ignored", "reason": "unsupported action type"})
		return
	}

	// 5. Trigger Webhook Interactor
	err = h.interactor.ProcessEvent(c.Request.Context(), &payload)
	if err != nil {
		log.Printf("[Webhook] Error processing event %s: %v", payload.Action.ID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "message": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "processed"})
}

func verifyTrelloSignature(body []byte, requestURL, secret, headerSig string) bool {
	// The signature is created by hashing the raw request body concatenated with the URL
	mac := hmac.New(sha1.New, []byte(secret))
	mac.Write(body)
	mac.Write([]byte(requestURL))
	expectedSig := base64.StdEncoding.EncodeToString(mac.Sum(nil))

	return expectedSig == headerSig
}
