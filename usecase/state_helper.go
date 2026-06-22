package usecase

import (
	"fmt"
	"strings"

	"github.com/koshuang/trello-ai-orchestrator/domain"
)

// FormatAIState generates a markdown string representing the AI State
func FormatAIState(state *domain.WorkflowState) string {
	var sb strings.Builder
	sb.WriteString("## AI State\n")
	sb.WriteString(fmt.Sprintf("Status: %s\n", state.Status))
	sb.WriteString(fmt.Sprintf("Last processed action id: %s\n", state.LastProcessedActionID))
	sb.WriteString(fmt.Sprintf("GitHub issue: %s\n", state.GitHubIssueURL))
	sb.WriteString(fmt.Sprintf("Plan: %s\n\n", state.PlanPath))

	sb.WriteString("Current understanding:\n")
	if len(state.CurrentUnderstanding) == 0 {
		sb.WriteString("- None\n")
	} else {
		for _, item := range state.CurrentUnderstanding {
			sb.WriteString(fmt.Sprintf("- %s\n", item))
		}
	}
	sb.WriteString("\n")

	sb.WriteString("Decisions:\n")
	if len(state.Decisions) == 0 {
		sb.WriteString("- None\n")
	} else {
		for _, item := range state.Decisions {
			sb.WriteString(fmt.Sprintf("- %s\n", item))
		}
	}
	sb.WriteString("\n")

	sb.WriteString("Open questions:\n")
	if len(state.OpenQuestions) == 0 {
		sb.WriteString("- None\n")
	} else {
		for _, item := range state.OpenQuestions {
			sb.WriteString(fmt.Sprintf("- %s\n", item))
		}
	}
	sb.WriteString("\n")

	nextAct := state.NextAction
	if nextAct == "" {
		nextAct = "None"
	}
	sb.WriteString(fmt.Sprintf("Next action:\n- %s\n", nextAct))
	return sb.String()
}

// UpdateDescriptionWithAIState replaces or appends ## AI State section in the description
func UpdateDescriptionWithAIState(desc string, state *domain.WorkflowState) string {
	newBlock := FormatAIState(state)

	const marker = "## AI State"
	idx := strings.Index(desc, marker)
	if idx == -1 {
		trimmed := strings.TrimSpace(desc)
		if trimmed == "" {
			return newBlock
		}
		return trimmed + "\n\n" + newBlock
	}

	remaining := desc[idx+len(marker):]
	lines := strings.Split(remaining, "\n")
	var afterBlockLines []string
	foundNextHeader := false

	for i, line := range lines {
		if foundNextHeader {
			afterBlockLines = append(afterBlockLines, line)
			continue
		}
		trimmedLine := strings.TrimSpace(line)
		if strings.HasPrefix(trimmedLine, "#") {
			foundNextHeader = true
			afterBlockLines = append(afterBlockLines, lines[i:]...)
			break
		}
	}

	beforeBlock := strings.TrimSuffix(desc[:idx], "\n")

	var result string
	if len(afterBlockLines) > 0 {
		afterBlock := strings.Join(afterBlockLines, "\n")
		result = beforeBlock + "\n\n" + newBlock + "\n" + afterBlock
	} else {
		result = beforeBlock + "\n\n" + newBlock
	}

	return strings.TrimSpace(result)
}

// ParseAIState parses a ## AI State block from a description markdown
func ParseAIState(desc string) *domain.TrelloAIStateBlock {
	const marker = "## AI State"
	idx := strings.Index(desc, marker)
	if idx == -1 {
		return nil
	}

	remaining := desc[idx+len(marker):]
	lines := strings.Split(remaining, "\n")

	block := &domain.TrelloAIStateBlock{}
	var currentSection string

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(trimmed, "#") {
			break
		}

		if strings.HasPrefix(strings.ToLower(trimmed), "status:") {
			block.Status = strings.TrimSpace(trimmed[7:])
			currentSection = ""
		} else if strings.HasPrefix(strings.ToLower(trimmed), "last processed action id:") {
			block.LastProcessedActionID = strings.TrimSpace(trimmed[25:])
			currentSection = ""
		} else if strings.HasPrefix(strings.ToLower(trimmed), "github issue:") {
			block.GitHubIssue = strings.TrimSpace(trimmed[13:])
			currentSection = ""
		} else if strings.HasPrefix(strings.ToLower(trimmed), "plan:") {
			block.Plan = strings.TrimSpace(trimmed[5:])
			currentSection = ""
		} else if strings.HasPrefix(strings.ToLower(trimmed), "current understanding:") {
			currentSection = "understanding"
		} else if strings.HasPrefix(strings.ToLower(trimmed), "decisions:") {
			currentSection = "decisions"
		} else if strings.HasPrefix(strings.ToLower(trimmed), "open questions:") {
			currentSection = "questions"
		} else if strings.HasPrefix(strings.ToLower(trimmed), "next action:") {
			currentSection = "next_action"
		} else if strings.HasPrefix(trimmed, "-") || strings.HasPrefix(trimmed, "*") {
			item := strings.TrimSpace(trimmed[1:])
			if strings.ToLower(item) == "none" {
				continue
			}
			switch currentSection {
			case "understanding":
				block.CurrentUnderstanding = append(block.CurrentUnderstanding, item)
			case "decisions":
				block.Decisions = append(block.Decisions, item)
			case "questions":
				block.OpenQuestions = append(block.OpenQuestions, item)
			case "next_action":
				block.NextAction = item
			}
		} else {
			if currentSection == "next_action" {
				block.NextAction = trimmed
			}
		}
	}

	return block
}
