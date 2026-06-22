package domain

import "time"

type ProcessedEvent struct {
	ID           string    `json:"id"`
	Provider     string    `json:"provider"` // e.g. "trello"
	EventID      string    `json:"event_id"`
	TrelloCardID string    `json:"trello_card_id"`
	EventType    string    `json:"event_type"` // e.g. "commentCard"
	ProcessedAt  time.Time `json:"processed_at"`
	Status       string    `json:"status"` // "success" or "skipped" or "failed"
	Error        string    `json:"error,omitempty"`
}
