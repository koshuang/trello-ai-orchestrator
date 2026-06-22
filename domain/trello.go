package domain

import "time"

// TrelloWebhookPayload is received on POST /webhooks/trello
type TrelloWebhookPayload struct {
	Action TrelloAction `json:"action"`
}

type TrelloAction struct {
	ID              string           `json:"id"`
	IDMemberCreator string           `json:"idMemberCreator"`
	Type            string           `json:"type"`
	Date            time.Time        `json:"date"`
	Data            TrelloActionData `json:"data"`
	MemberCreator   TrelloMember     `json:"memberCreator"`
}

type TrelloActionData struct {
	Text  string          `json:"text"`
	Card  TrelloCardShort `json:"card"`
	Board TrelloBoard     `json:"board"`
}

type TrelloCardShort struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	ShortLink string `json:"shortLink"`
}

type TrelloBoard struct {
	ID string `json:"id"`
}

type TrelloMember struct {
	ID       string `json:"id"`
	Username string `json:"username"`
	FullName string `json:"fullName"`
}

// TrelloCardContext represents the complete data fetched for a card
type TrelloCardContext struct {
	ID          string              `json:"id"`
	Title       string              `json:"title"`
	Description string              `json:"description"`
	URL         string              `json:"url"`
	Labels      []TrelloLabel       `json:"labels"`
	Members     []TrelloMember      `json:"members"`
	Checklists  []TrelloChecklist   `json:"checklists"`
	Attachments []TrelloAttachment  `json:"attachments"`
	Comments    []TrelloCommentInfo `json:"comments"`
	AIState     *TrelloAIStateBlock `json:"ai_state"`
}

type TrelloLabel struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Color string `json:"color"`
}

type TrelloChecklist struct {
	ID    string            `json:"id"`
	Name  string            `json:"name"`
	Items []TrelloCheckItem `json:"checkItems"`
}

type TrelloCheckItem struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	State string `json:"state"` // "complete" or "incomplete"
}

type TrelloAttachment struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	URL  string `json:"url"`
}

type TrelloCommentInfo struct {
	ID        string    `json:"id"`
	Text      string    `json:"text"`
	CreatorID string    `json:"creator_id"`
	Username  string    `json:"username"`
	Date      time.Time `json:"date"`
}

// TrelloAIStateBlock represents the parsed ## AI State section from card description
type TrelloAIStateBlock struct {
	Status                string   `json:"status"`
	LastProcessedActionID string   `json:"last_processed_action_id"`
	GitHubIssue           string   `json:"github_issue"`
	Plan                  string   `json:"plan"`
	CurrentUnderstanding  []string `json:"current_understanding"`
	Decisions             []string `json:"decisions"`
	OpenQuestions         []string `json:"open_questions"`
	NextAction            string   `json:"next_action"`
}
