package main

import (
	"encoding/json"
	"fmt"
	"strings"
)

const (
	agentRecentReplaySteps = 12
	agentDigestMaxLines    = 14
	agentDigestMaxRunes    = 180
)

func buildAgentConversationMessages(ctx *AgentContext, history []AgentStep, userMessage string) []Message {
	filtered := filterReplayHistory(history)
	recentStart := 0
	if len(filtered) > agentRecentReplaySteps {
		recentStart = len(filtered) - agentRecentReplaySteps
	}

	var messages []Message
	if ctx != nil && ctx.Session != nil && ctx.Session.Digest != nil && strings.TrimSpace(ctx.Session.Digest.Summary) != "" {
		messages = append(messages, Message{Role: "user", Content: ctx.Session.Digest.Summary})
	} else if recentStart > 0 {
		if digest := buildAgentHistoryDigest(ctx, filtered[:recentStart]); digest != "" {
			messages = append(messages, Message{Role: "user", Content: digest})
		}
	}

	toolResultLabel := "[工具结果]"
	if projectLang(ctx) == LangEN {
		toolResultLabel = "[Tool result]"
	}

	lastToolName := ""
	lastToolArgs := ""
	for _, step := range filtered[recentStart:] {
		switch step.Role {
		case "user":
			messages = append(messages, Message{Role: "user", Content: step.Content})
			lastToolName, lastToolArgs = "", ""
		case "assistant":
			if step.ToolCall != nil {
				tcJSON, _ := json.Marshal(step.ToolCall)
				messages = append(messages, Message{Role: "assistant", Content: fmt.Sprintf("<tool_call>\n%s\n</tool_call>", string(tcJSON))})
				lastToolName = step.ToolCall.Name
				lastToolArgs = formatToolArgsPreview(step.ToolCall.Arguments)
			} else {
				messages = append(messages, Message{Role: "assistant", Content: step.Content})
				lastToolName, lastToolArgs = "", ""
			}
		case "tool":
			messages = append(messages, Message{Role: "user", Content: fmt.Sprintf("%s\n%s", toolResultLabel, formatToolResultForReplay(ctx, lastToolName, lastToolArgs, step.ToolResult, step.ToolResultKey, step.ToolResultArgs))})
			lastToolName, lastToolArgs = "", ""
		}
	}

	messages = append(messages, Message{Role: "user", Content: userMessage})
	return messages
}

func filterReplayHistory(history []AgentStep) []AgentStep {
	if len(history) == 0 {
		return nil
	}
	if history[len(history)-1].Role != "user" {
		return append([]AgentStep(nil), history...)
	}
	return append([]AgentStep(nil), history[:len(history)-1]...)
}

func buildAgentHistoryDigest(ctx *AgentContext, steps []AgentStep) string {
	return buildHistoryDigestForLang(projectLang(ctx), steps)
}

func buildHistoryDigestForLang(lang string, steps []AgentStep) string {
	if len(steps) == 0 {
		return ""
	}
	lang = NormalizeLanguage(lang)
	lines := make([]string, 0, len(steps))
	pendingToolName := ""
	pendingToolArgs := ""
	compacted := 0
	for _, step := range steps {
		switch step.Role {
		case "assistant":
			if step.ToolCall != nil {
				pendingToolName = step.ToolCall.Name
				pendingToolArgs = formatToolArgsPreview(step.ToolCall.Arguments)
				continue
			}
			pendingToolName, pendingToolArgs = "", ""
			text := strings.TrimSpace(step.Content)
			if text != "" {
				lines = appendDigestLine(lines, digestSpeakerLine(lang, false, text))
				compacted++
			}
		case "user":
			pendingToolName, pendingToolArgs = "", ""
			text := strings.TrimSpace(step.Content)
			if text != "" {
				lines = appendDigestLine(lines, digestSpeakerLine(lang, true, text))
				compacted++
			}
		case "tool":
			lines = appendDigestLine(lines, formatToolResultForReplayLang(lang, pendingToolName, pendingToolArgs, step.ToolResult, step.ToolResultKey, step.ToolResultArgs))
			pendingToolName, pendingToolArgs = "", ""
			compacted++
		}
	}
	if len(lines) == 0 {
		return ""
	}
	if len(lines) > agentDigestMaxLines {
			lines = lines[len(lines)-agentDigestMaxLines:]
	}
	return agentDigestHeader(lang, compacted) + "\n" + strings.Join(lines, "\n")
}

func appendDigestLine(lines []string, line string) []string {
	line = strings.TrimSpace(line)
	if line == "" {
		return lines
	}
	return append(lines, "- "+truncate(line, agentDigestMaxRunes))
}

func digestSpeakerLine(lang string, isUser bool, text string) string {
	if lang == LangEN {
		if isUser {
			return "User: " + text
		}
		return "Assistant: " + text
	}
	if isUser {
		return "用户：" + text
	}
	return "助手：" + text
}

func agentDigestHeader(lang string, compacted int) string {
	if lang == LangEN {
		return fmt.Sprintf("[Earlier conversation digest — %d compacted steps; replay keeps only the recent window and tool-result summaries]", compacted)
	}
	return fmt.Sprintf("【更早对话摘要——已压缩 %d 条历史步骤；回放时仅保留最近窗口与工具结果摘要】", compacted)
}

func formatToolResultForReplay(ctx *AgentContext, toolName, toolArgs, result, resultKey string, resultArgs []string) string {
	return formatToolResultForReplayLang(projectLang(ctx), toolName, toolArgs, result, resultKey, resultArgs)
}

func formatToolResultForReplayLang(lang, toolName, toolArgs, result, resultKey string, resultArgs []string) string {
	lang = NormalizeLanguage(lang)
	if resultKey != "" {
		return localizedToolResultMessage(lang, resultKey, resultArgs)
	}
	base := summarizeRawToolResult(toolName, result)
	if toolName == "" {
		return base
	}
	if toolArgs != "" {
		if lang == LangEN {
			return fmt.Sprintf("Tool %s(%s): %s", toolName, toolArgs, base)
		}
		return fmt.Sprintf("工具 %s(%s)：%s", toolName, toolArgs, base)
	}
	if lang == LangEN {
		return fmt.Sprintf("Tool %s: %s", toolName, base)
	}
	return fmt.Sprintf("工具 %s：%s", toolName, base)
}

func formatReplayToolResultMessage(ctx *AgentContext, toolName string, args json.RawMessage, result, resultKey string, resultArgs []string) string {
	label := "[工具结果]"
	if projectLang(ctx) == LangEN {
		label = "[Tool result]"
	}
	return fmt.Sprintf("%s\n%s", label, formatToolResultForReplay(ctx, toolName, formatToolArgsPreview(args), result, resultKey, resultArgs))
}

func localizedToolResultMessage(lang, key string, args []string) string {
	if len(args) == 0 {
		return T(lang, key)
	}
	vals := make([]any, len(args))
	for i, arg := range args {
		vals[i] = arg
	}
	return T(lang, key, vals...)
}

func summarizeRawToolResult(toolName, result string) string {
	result = strings.TrimSpace(result)
	if result == "" {
		return "(empty)"
	}
	if head := firstMeaningfulLine(result); head != "" {
		switch toolName {
		case "read_chapter", "read_outline", "read_characters", "read_character", "read_worldview", "read_organizations", "read_foreshadows", "read_project_config", "read_skills", "search_project":
			return truncate(head, 120)
		}
	}
	return truncate(result, 120)
}

func firstMeaningfulLine(s string) string {
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			return line
		}
	}
	return ""
}

func formatToolArgsPreview(args json.RawMessage) string {
	raw := strings.TrimSpace(string(args))
	if raw == "" || raw == "{}" || raw == "null" {
		return ""
	}
	return truncate(raw, 80)
}

func buildPageFocusHint(lang, page string) string {
	switch page {
	case "config":
		if lang == LangEN {
			return "Focus on settings/config work first: prefer project-config, character, worldview, organisation, relation, and foreshadow tools. Avoid drifting into chapter-generation or destructive actions unless the user explicitly asks."
		}
		return "当前应优先聚焦设定与配置：优先使用故事配置、角色、世界观、组织、关系、伏笔相关工具。除非用户明确要求，否则不要把注意力转移到章节生成或危险删除操作上。"
	case "outline":
		if lang == LangEN {
			return "Focus on outline planning first: prefer outline read/generate/revise/confirm tools and project-config reads. Avoid pulling full chapter prose unless the user explicitly needs it."
		}
		return "当前应优先聚焦大纲规划：优先使用大纲读取/生成/修订/确认工具，以及故事配置读取。除非用户明确需要，否则不要拉取整章正文。"
	case "writing":
		if lang == LangEN {
			return "Focus on the current writing frontier: prefer chapter read/generate/revise/confirm tools plus foreshadow/context lookups. Avoid broad config rewrites unless the user explicitly requests them."
		}
		return "当前应优先聚焦写作前沿：优先使用章节读取/生成/修订/确认工具，以及伏笔和上下文查询。除非用户明确要求，否则不要转去大范围改配置。"
	case "relations":
		if lang == LangEN {
			return "Focus on entity/relationship management first: prefer character, organisation, worldview, and relation tools."
		}
		return "当前应优先聚焦实体与关系管理：优先使用角色、组织、世界观、关系相关工具。"
	case "skills":
		if lang == LangEN {
			return "Focus on skill visibility and toggles first: prefer read_skills and toggle_skill."
		}
		return "当前应优先聚焦技能查看与开关：优先使用 read_skills 和 toggle_skill。"
	default:
		return ""
	}
}

func toolVisibleInPage(page, name string) bool {
	if page == "" {
		return true
	}
	switch page {
	case "config":
		return inToolSet(name,
			"read_project_config", "update_project_config", "search_project",
			"read_characters", "read_character", "create_character", "update_character", "delete_character",
			"read_worldview", "create_worldview", "update_worldview", "delete_worldview",
			"read_organizations", "create_organization", "update_organization", "delete_organization",
			"create_relation", "update_relation", "delete_relation",
			"read_foreshadows", "create_foreshadow", "update_foreshadow", "delete_foreshadow",
			"read_skills", "toggle_skill", "generate_outline",
		)
	case "outline":
		return inToolSet(name,
			"read_project_config", "read_outline", "read_chapter", "search_project",
			"generate_outline", "confirm_outline", "revise_outline", "edit_chapter_outline",
			"read_characters", "read_worldview", "read_foreshadows",
		)
	case "writing":
		return inToolSet(name,
			"read_outline", "read_chapter", "read_foreshadows", "search_project",
			"generate_chapter", "confirm_chapter", "edit_chapter_content", "revise_chapter",
			"delete_chapter", "delete_chapters_from",
			"read_characters", "read_worldview", "read_organizations",
		)
	case "relations":
		return inToolSet(name,
			"search_project", "read_characters", "read_character", "read_worldview", "read_organizations",
			"create_organization", "update_organization", "delete_organization",
			"create_relation", "update_relation", "delete_relation",
			"create_character", "update_character", "create_worldview", "update_worldview",
		)
	case "skills":
		return inToolSet(name, "read_skills", "toggle_skill", "read_project_config")
	default:
		return true
	}
}

func filterToolsForPage(tools []Tool, page string) []Tool {
	if page == "" {
		return tools
	}
	filtered := make([]Tool, 0, len(tools))
	for _, tool := range tools {
		if toolVisibleInPage(page, tool.Name) {
			filtered = append(filtered, tool)
		}
	}
	if len(filtered) == 0 {
		return tools
	}
	return filtered
}

func inToolSet(name string, allowed ...string) bool {
	for _, item := range allowed {
		if name == item {
			return true
		}
	}
	return false
}
