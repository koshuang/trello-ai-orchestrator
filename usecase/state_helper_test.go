package usecase

import (
	"testing"

	"github.com/koshuang/trello-ai-orchestrator/domain"
	"github.com/stretchr/testify/assert"
)

func TestFormatAIState(t *testing.T) {
	state := &domain.WorkflowState{
		Status:                domain.StatusNeedsPMClarification,
		LastProcessedActionID: "action_123",
		GitHubIssueURL:        "https://github.com/issue/12",
		PlanPath:              "docs/plans/test.md",
		CurrentUnderstanding:  []string{"Requirement A", "Requirement B"},
		Decisions:             []string{"Use SQLite"},
		OpenQuestions:         []string{"Deployment environment?"},
		NextAction:            "Ask for clarification",
	}

	formatted := FormatAIState(state)
	assert.Contains(t, formatted, "## AI State")
	assert.Contains(t, formatted, "Status: needs_pm_clarification")
	assert.Contains(t, formatted, "Last processed action id: action_123")
	assert.Contains(t, formatted, "GitHub issue: https://github.com/issue/12")
	assert.Contains(t, formatted, "Plan: docs/plans/test.md")
	assert.Contains(t, formatted, "- Requirement A")
	assert.Contains(t, formatted, "- Requirement B")
	assert.Contains(t, formatted, "- Use SQLite")
	assert.Contains(t, formatted, "- Deployment environment?")
	assert.Contains(t, formatted, "Next action:\n- Ask for clarification")
}

func TestUpdateDescriptionWithAIState(t *testing.T) {
	state := &domain.WorkflowState{
		Status:                domain.StatusIssueCreated,
		LastProcessedActionID: "action_999",
		GitHubIssueURL:        "https://github.com/issue/99",
		PlanPath:              "docs/plans/plan.md",
		CurrentUnderstanding:  []string{"Points"},
		Decisions:             []string{"Decision"},
		OpenQuestions:         []string{"Question"},
		NextAction:            "Action",
	}

	t.Run("Insert AI State when none exists", func(t *testing.T) {
		desc := "This is a card description."
		updated := UpdateDescriptionWithAIState(desc, state)
		assert.Contains(t, updated, "This is a card description.\n\n## AI State")
		assert.Contains(t, updated, "Status: issue_created")
	})

	t.Run("Replace AI State when block exists", func(t *testing.T) {
		desc := "Original Text\n\n## AI State\nStatus: new\nLast processed action id: old\n\n## Other Section\nSome other details here."
		updated := UpdateDescriptionWithAIState(desc, state)

		assert.Contains(t, updated, "Original Text\n\n## AI State")
		assert.Contains(t, updated, "Status: issue_created")
		assert.Contains(t, updated, "Last processed action id: action_999")
		// The other sections must be preserved
		assert.Contains(t, updated, "## Other Section\nSome other details here.")
	})
}

func TestParseAIState(t *testing.T) {
	t.Run("Parse valid AI State block", func(t *testing.T) {
		desc := `Some background text.

## AI State
Status: ready_for_implementation
Last processed action id: act_789
GitHub issue: https://github.com/issue/88
Plan: docs/plans/plan.md

Current understanding:
- Need task orchestration
- Local DB only

Decisions:
- Use golang

Open questions:
- Verify signatures?

Next action:
- Awaiting team approval

## Implementation Details
We will build this service.`

		block := ParseAIState(desc)
		assert.NotNil(t, block)
		assert.Equal(t, "ready_for_implementation", block.Status)
		assert.Equal(t, "act_789", block.LastProcessedActionID)
		assert.Equal(t, "https://github.com/issue/88", block.GitHubIssue)
		assert.Equal(t, "docs/plans/plan.md", block.Plan)
		assert.ElementsMatch(t, []string{"Need task orchestration", "Local DB only"}, block.CurrentUnderstanding)
		assert.ElementsMatch(t, []string{"Use golang"}, block.Decisions)
		assert.ElementsMatch(t, []string{"Verify signatures?"}, block.OpenQuestions)
		assert.Equal(t, "Awaiting team approval", block.NextAction)
	})

	t.Run("Return nil if no AI State block", func(t *testing.T) {
		desc := "Just simple description."
		block := ParseAIState(desc)
		assert.Nil(t, block)
	})
}
