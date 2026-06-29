package main

import (
	"strings"
	"testing"
)

func TestRefreshChatSessionDigestPersistsSummary(t *testing.T) {
	session := &ChatSession{}
	for i := 0; i < 16; i++ {
		session.Messages = append(session.Messages, ChatMessage{Role: "user", Content: "user"})
		session.Messages = append(session.Messages, ChatMessage{Role: "assistant", Content: "assistant"})
	}

	refreshChatSessionDigest(session, LangZH)
	if session.Digest == nil {
		t.Fatal("refreshChatSessionDigest() = nil digest")
	}
	if session.Digest.CoveredSteps <= 0 {
		t.Fatalf("refreshChatSessionDigest() covered_steps = %d, want > 0", session.Digest.CoveredSteps)
	}
	if !strings.Contains(session.Digest.Summary, "更早对话摘要") {
		t.Fatalf("digest summary = %q, want digest header", session.Digest.Summary)
	}
}

func TestBuildAgentConversationMessagesUsesPersistedDigest(t *testing.T) {
	ctx := &AgentContext{Config: &Config{Language: LangZH}, Session: &ChatSession{Digest: &ChatSessionDigest{Summary: "【持久摘要】"}}}
	history := []AgentStep{
		{Role: "user", Content: "旧消息1"},
		{Role: "assistant", Content: "旧回复1"},
		{Role: "user", Content: "本轮消息"},
	}
	msgs := buildAgentConversationMessages(ctx, history, "本轮消息")
	if len(msgs) < 2 {
		t.Fatalf("buildAgentConversationMessages() len = %d, want >= 2", len(msgs))
	}
	if msgs[0].Content != "【持久摘要】" {
		t.Fatalf("first message = %q, want persisted digest", msgs[0].Content)
	}
	if msgs[len(msgs)-1].Content != "本轮消息" {
		t.Fatalf("last message = %q, want current user message", msgs[len(msgs)-1].Content)
	}
}
