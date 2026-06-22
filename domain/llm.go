package domain

type LLMInput struct {
	NewComment     string            `json:"new_comment"`
	CardContext    TrelloCardContext `json:"card_context"`
	WorkflowState  WorkflowState     `json:"workflow_state"`
	OperatingRules string            `json:"operating_rules"`
}

type LLMResponse struct {
	Action       string          `json:"action"` // reply_comment | create_github_issue | update_github_issue | create_plan | update_plan | update_state_only | ignore | ask_kos
	Reason       string          `json:"reason"`
	ReplyComment string          `json:"reply_comment,omitempty"`
	StateUpdate  LLMStateUpdate  `json:"state_update"`
	GitHubIssue  *LLMGitHubIssue `json:"github_issue,omitempty"`
	Plan         *LLMPlan        `json:"plan,omitempty"`
}

type LLMStateUpdate struct {
	Status               string   `json:"status"`
	Summary              string   `json:"summary"`
	CurrentUnderstanding []string `json:"current_understanding"`
	Decisions            []string `json:"decisions"`
	OpenQuestions        []string `json:"open_questions"`
	NextAction           string   `json:"next_action"`
}

type LLMGitHubIssue struct {
	Title  string   `json:"title"`
	Body   string   `json:"body"`
	Labels []string `json:"labels,omitempty"`
}

type LLMPlan struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}
