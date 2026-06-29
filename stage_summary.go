package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

const stageSummaryWindow = 20

func updateStageSummaryAfterChapter(ctx context.Context, apiCfg *APIConfig, cfg *Config, state *Progress, idx int, progressPath string, logger *LogBroadcaster) {
	if ctx.Err() != nil || idx < 0 || idx >= len(state.Chapters) {
		return
	}
	ch := state.Chapters[idx]
	if strings.TrimSpace(ch.Content) == "" || strings.TrimSpace(ch.Summary) == "" {
		return
	}
	endChapter := ch.Num
	if endChapter <= 0 || endChapter%stageSummaryWindow != 0 {
		return
	}
	startChapter := endChapter - stageSummaryWindow + 1
	chapters := collectStageSummaryChapters(state, startChapter, endChapter)
	if len(chapters) != stageSummaryWindow {
		return
	}
	lang := cfg.Language
	payload := buildStageSummaryPayload(chapters)
	hash := hashPayload(payload)
	for _, ss := range state.StageSummaries {
		if ss.StartChapter == startChapter && ss.EndChapter == endChapter && ss.SourceHash == hash {
			return
		}
	}
	userPrompt := RenderPrompt(cfg.Prompts.StageSummaryUpdate, map[string]string{
		"Title":                 preferUserValue(cfg.Story.Title, state.Title),
		"StartChapter":          fmt.Sprintf("%d", startChapter),
		"EndChapter":            fmt.Sprintf("%d", endChapter),
		"ExistingStageSummaries": formatExistingStageSummaries(state.StageSummaries, lang),
		"StageChapterSummaries": formatStageChapterSummaries(chapters, lang),
	})
	systemPrompt := SystemPromptFor(lang, "stage_summary_manager")
	if systemPrompt == "" {
		return
	}
	result := CallAPIWithRetryLog(ctx, apiCfg, systemPrompt, userPrompt, logger)
	if result == "" {
		return
	}
	var parsed struct {
		Summary string `json:"summary"`
	}
	if err := json.Unmarshal([]byte(cleanJSONResponse(result)), &parsed); err != nil {
		return
	}
	summary := strings.TrimSpace(parsed.Summary)
	if summary == "" {
		return
	}
	updated := false
	for i := range state.StageSummaries {
		if state.StageSummaries[i].StartChapter == startChapter && state.StageSummaries[i].EndChapter == endChapter {
			state.StageSummaries[i].Summary = summary
			state.StageSummaries[i].SourceHash = hash
			updated = true
			break
		}
	}
	if !updated {
		state.StageSummaries = append(state.StageSummaries, StageSummary{
			StartChapter: startChapter,
			EndChapter:   endChapter,
			Summary:      summary,
			SourceHash:   hash,
		})
	}
	if err := SaveProgress(progressPath, state); err != nil {
		logger.InfoKey("log.memory_save_failed")
	}
}

func rebuildStageSummaries(ctx context.Context, apiCfg *APIConfig, cfg *Config, state *Progress, progressPath string, logger *LogBroadcaster) {
	state.StageSummaries = nil
	for idx := range state.Chapters {
		updateStageSummaryAfterChapter(ctx, apiCfg, cfg, state, idx, progressPath, logger)
	}
}

func collectStageSummaryChapters(state *Progress, startChapter, endChapter int) []ChapterState {
	out := make([]ChapterState, 0, endChapter-startChapter+1)
	for _, ch := range state.Chapters {
		if ch.Num < startChapter || ch.Num > endChapter {
			continue
		}
		if ch.Summary == "" {
			return nil
		}
		out = append(out, ch)
	}
	return out
}

func formatStageChapterSummaries(chapters []ChapterState, lang string) string {
	var sb strings.Builder
	for _, ch := range chapters {
		if NormalizeLanguage(lang) == LangEN {
			sb.WriteString(fmt.Sprintf("[Chapter %d summary]: %s\n", ch.Num, ch.Summary))
		} else {
			sb.WriteString(fmt.Sprintf("[第%d章摘要]: %s\n", ch.Num, ch.Summary))
		}
	}
	return sb.String()
}

func formatExistingStageSummaries(items []StageSummary, lang string) string {
	if len(items) == 0 {
		if NormalizeLanguage(lang) == LangEN {
			return "(empty — no stage summaries yet)"
		}
		return "（空——尚无阶段摘要）"
	}
	var sb strings.Builder
	for _, ss := range items {
		if NormalizeLanguage(lang) == LangEN {
			sb.WriteString(fmt.Sprintf("[Stage Ch.%d-%d] %s\n", ss.StartChapter, ss.EndChapter, ss.Summary))
		} else {
			sb.WriteString(fmt.Sprintf("[阶段 第%d-%d章] %s\n", ss.StartChapter, ss.EndChapter, ss.Summary))
		}
	}
	return sb.String()
}

func buildStageSummaryPayload(chapters []ChapterState) []map[string]any {
	out := make([]map[string]any, 0, len(chapters))
	for _, ch := range chapters {
		out = append(out, map[string]any{
			"num":     ch.Num,
			"title":   ch.Title,
			"summary": ch.Summary,
		})
	}
	return out
}
