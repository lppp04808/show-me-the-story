package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

const (
	defaultCriticPassZH = "[一致性审稿] 通过 ✓"
	defaultCriticPassEN = "[Consistency critic] Passed ✓"
)

func init() {
	DefaultPromptsZH.ChapterCritic = `你是一位严谨的小说一致性审稿员。你的任务不是评价文笔，而是对照已有状态检查当前草稿是否出现具体矛盾或逻辑断裂。

【角色与设定】
{{.CharacterContext}}{{.WorldviewContext}}{{.RelationContext}}
{{.Foreshadows}}{{.Memory}}{{.OutlineConstraints}}【前情提要】
{{.HistorySummary}}

【本章摘要】
{{.ChapterSummary}}

【本章大纲】
{{.ChapterOutline}}

【当前草稿】
{{.ChapterContent}}

检查范围（只检查这些，其他一律忽略）：
1. 与前情提要或设定直接冲突的角色状态、关系、能力、身份、时间线、地点、道具事实
2. 行为动机或剧情推进出现明显断裂，导致当前草稿在已有信息下无法自洽
3. 已在前文发生/确认过的事件被当作第一次再次发生
4. 按大纲或后续章节脉络本不该现在发生的事件被提前发生

严格要求：
- 只输出具体矛盾，不要评价文风、节奏、感染力、是否精彩
- 不要给泛泛建议，如“可以更细腻”“逻辑还需加强”
- 拿不准时判 PASS
- 每条 issues 必须写成可定位、可复核的具体矛盾句子

请以JSON格式返回（不要输出任何其他文字）：
{"result": "PASS", "issues": []}
或
{"result": "FAIL", "issues": ["具体矛盾描述1", "具体矛盾描述2"]}`

	DefaultPromptsEN.ChapterCritic = `You are a strict novel consistency critic. Your job is not to judge the prose quality, but to compare the current draft against established state and report concrete contradictions or logic breaks.

[Characters and setting]
{{.CharacterContext}}{{.WorldviewContext}}{{.RelationContext}}
{{.Foreshadows}}{{.Memory}}{{.OutlineConstraints}}[Story-so-far]
{{.HistorySummary}}

[Chapter summary]
{{.ChapterSummary}}

[Chapter outline]
{{.ChapterOutline}}

[Current draft]
{{.ChapterContent}}

Check only these categories:
1. Character state, relationships, abilities, identities, timeline, locations, or prop facts that directly conflict with the story-so-far or setting
2. Clear motivation or plot-progression breaks that make the draft internally inconsistent under the existing information
3. Events already established in earlier chapters being written again as if they are happening for the first time
4. Events happening prematurely even though the outline or later-arc constraints place them later

Strict rules:
- Output concrete contradictions only; do not comment on style, pacing, emotional impact, or whether the chapter is good
- Do not give vague advice such as “make it more nuanced” or “the logic needs work”
- When unsure, return PASS
- Every issue must be a specific, checkable contradiction sentence

Return JSON only (no other text):
{"result": "PASS", "issues": []}
or
{"result": "FAIL", "issues": ["concrete contradiction 1", "concrete contradiction 2"]}`

	systemPrompts["chapter_critic_json"] = map[string]string{
		LangZH: "你是一位严谨的小说一致性审稿员。请严格按照要求的JSON格式输出，不要添加任何额外文字。拿不准时视为无矛盾。",
		LangEN: "You are a strict novel consistency critic. Output strict JSON exactly as requested — no extra prose. When unsure, treat as no contradiction.",
	}
}

func criticEnabled(cfg *Config) bool {
	return cfg != nil && cfg.Story.CriticEnabled && strings.TrimSpace(cfg.Story.CriticModel) != ""
}

func chapterAuditStepPlan(cfg *Config) (total, criticStep, factStep, foreshadowStep, memoryStep int) {
	total = 6
	criticStep = 0
	factStep = 4
	foreshadowStep = 5
	memoryStep = 6
	if criticEnabled(cfg) {
		total = 7
		criticStep = 4
		factStep = 5
		foreshadowStep = 6
		memoryStep = 7
	}
	return
}

func generateChapterCritic(ctx context.Context, apiCfg *APIConfig, cfg *Config, state *Progress, idx int, content, chapterSummary string, settings *ProjectSettings) (string, error) {
	if !criticEnabled(cfg) {
		return `{"result":"PASS","issues":[]}`, nil
	}
	ch := state.Chapters[idx]
	lang := cfg.Language
	foreshadowContext := formatActiveForeshadowsForChapterLang(state.Foreshadows, ch.Num, lang)
	characterContext := buildCharacterContextForLang(settings, ch.Outline, lang)
	worldviewContext := buildWorldviewContextForLang(settings, ch.Outline, lang)
	relationContext := buildRelationContextForLang(settings, ch.Outline, lang)
	outlineConstraints := buildOutlineConstraintsForLang(state, idx, lang)
	memoryContext := buildMemoryForLang(state, idx, lang)
	historySummary := buildHistorySummaryForLang(state, idx, lang)

	userPrompt := RenderPrompt(cfg.Prompts.ChapterCritic, map[string]string{
		"CharacterContext":   characterContext,
		"WorldviewContext":   worldviewContext,
		"RelationContext":    relationContext,
		"Foreshadows":        foreshadowContext,
		"Memory":             memoryContext,
		"OutlineConstraints": outlineConstraints,
		"HistorySummary":     historySummary,
		"ChapterSummary":     chapterSummary,
		"ChapterOutline":     ch.Outline,
		"ChapterContent":     content,
	})
	userPrompt = appendIfMissingPlaceholder(cfg.Prompts.ChapterCritic, userPrompt, "{{.RelationContext}}", relationContext)
	userPrompt = appendIfMissingPlaceholder(cfg.Prompts.ChapterCritic, userPrompt, "{{.Foreshadows}}", foreshadowContext)
	userPrompt = appendIfMissingPlaceholder(cfg.Prompts.ChapterCritic, userPrompt, "{{.Memory}}", memoryContext)
	userPrompt = appendIfMissingPlaceholder(cfg.Prompts.ChapterCritic, userPrompt, "{{.OutlineConstraints}}", outlineConstraints)

	criticCfg := *apiCfg
	criticCfg.Model = strings.TrimSpace(cfg.Story.CriticModel)
	criticCfg.UseStream = false
	if criticCfg.MaxTokens == 0 || criticCfg.MaxTokens > 2048 {
		criticCfg.MaxTokens = 2048
	}
	return CallAPI(ctx, &criticCfg, SystemPromptFor(lang, "chapter_critic_json"), userPrompt)
}

func generateChapterCriticWithRetryLog(ctx context.Context, apiCfg *APIConfig, cfg *Config, state *Progress, idx int, content, chapterSummary string, settings *ProjectSettings, logger *LogBroadcaster) string {
	retryCount := 0
	for {
		if ctx.Err() != nil {
			return ""
		}
		result, err := generateChapterCritic(ctx, apiCfg, cfg, state, idx, content, chapterSummary, settings)
		if err == nil && result != "" {
			return result
		}
		if isFatalAPIError(err) {
			logger.ErrorKey("log.fatal_no_retry", err)
			return ""
		}

		retryCount++
		waitTime := getWaitTime(retryCount)
		logger.WarnBilingual(
			fmt.Sprintf("一致性审稿失败: %v。第 %d 次重试，等待 %ds...", err, retryCount, waitTime),
			fmt.Sprintf("Consistency critic failed: %v. Retry %d, waiting %ds...", err, retryCount, waitTime),
		)
		select {
		case <-time.After(time.Duration(waitTime) * time.Second):
		case <-ctx.Done():
			return ""
		}
	}
}

func parseChapterCriticResult(raw string) (failed bool, issues string) {
	cleaned := cleanJSONResponse(raw)
	var resp struct {
		Result string   `json:"result"`
		Issues []string `json:"issues"`
	}
	if jsonStr := extractJSON(cleaned); jsonStr != "" {
		if err := json.Unmarshal([]byte(jsonStr), &resp); err == nil && resp.Result != "" {
			return strings.EqualFold(strings.TrimSpace(resp.Result), "FAIL"), strings.Join(resp.Issues, "；")
		}
	}
	return strings.Contains(raw, "FAIL"), truncate(raw, 300)
}

func buildRelationContextForLang(settings *ProjectSettings, chapterOutline, lang string) string {
	if settings == nil || len(settings.Relations) == 0 {
		return ""
	}
	nameByID := make(map[string]string, len(settings.Characters)+len(settings.Organizations)+len(settings.Worldview))
	for _, c := range settings.Characters {
		nameByID[c.ID] = c.Name
	}
	for _, o := range settings.Organizations {
		nameByID[o.ID] = o.Name
	}
	for _, w := range settings.Worldview {
		nameByID[w.ID] = w.Name
	}

	var relevant []Relation
	for _, rel := range settings.Relations {
		sourceName := stripNameMarks(nameByID[rel.SourceID])
		targetName := stripNameMarks(nameByID[rel.TargetID])
		if sourceName != "" && strings.Contains(chapterOutline, sourceName) {
			relevant = append(relevant, rel)
			continue
		}
		if targetName != "" && strings.Contains(chapterOutline, targetName) {
			relevant = append(relevant, rel)
		}
	}
	if len(relevant) == 0 {
		relevant = settings.Relations
	}
	if len(relevant) > 12 {
		relevant = relevant[:12]
	}

	var sb strings.Builder
	if NormalizeLanguage(lang) == LangEN {
		sb.WriteString("[Relationship map]\n")
	} else {
		sb.WriteString("【关系图谱】\n")
	}
	for _, rel := range relevant {
		sourceName := nameByID[rel.SourceID]
		if sourceName == "" {
			sourceName = rel.SourceID
		}
		targetName := nameByID[rel.TargetID]
		if targetName == "" {
			targetName = rel.TargetID
		}
		sb.WriteString(fmt.Sprintf("- %s --%s--> %s\n", sourceName, rel.Label, targetName))
	}
	sb.WriteString("\n")
	return sb.String()
}
