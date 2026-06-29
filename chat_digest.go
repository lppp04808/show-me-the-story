package main

import "time"

type ChatSessionDigest struct {
	Summary      string `json:"summary"`
	CoveredSteps int    `json:"covered_steps"`
	UpdatedAt    string `json:"updated_at"`
}

func chatSessionHistorySteps(session *ChatSession) []AgentStep {
	if session == nil || len(session.Messages) == 0 {
		return nil
	}
	steps := make([]AgentStep, 0, len(session.Messages))
	for _, m := range session.Messages {
		switch m.Role {
		case "user":
			steps = append(steps, AgentStep{Role: "user", Content: m.Content})
		case "assistant":
			step := AgentStep{Role: "assistant", Content: m.Content}
			if len(m.ToolCalls) > 0 {
				step.ToolCall = &m.ToolCalls[0]
			}
			steps = append(steps, step)
		case "tool":
			steps = append(steps, AgentStep{
				Role:           "tool",
				ToolResult:     m.ToolResult,
				ToolResultKey:  m.ToolResultKey,
				ToolResultArgs: m.ToolResultArgs,
			})
		}
	}
	return steps
}

func refreshChatSessionDigest(session *ChatSession, lang string) {
	if session == nil {
		return
	}
	steps := chatSessionHistorySteps(session)
	if len(steps) <= agentRecentReplaySteps {
		session.Digest = nil
		return
	}
	covered := len(steps) - agentRecentReplaySteps
	summary := buildHistoryDigestForLang(NormalizeLanguage(lang), steps[:covered])
	if summary == "" {
		session.Digest = nil
		return
	}
	session.Digest = &ChatSessionDigest{
		Summary:      summary,
		CoveredSteps: covered,
		UpdatedAt:    time.Now().Format(time.RFC3339),
	}
}
