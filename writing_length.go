package main

import (
	"context"
	"fmt"
	"strings"
)

const (
	chapterLengthToleranceAbsolute = 1000
	chapterLengthTolerancePercent  = 15
	chapterGenMaxLengthAttempts    = 2
)

// calcChapterLengthRange returns acceptable chapter prose length bounds (runes/characters).
// Tolerance is max(±1000, ±15% of target). ponytail: fixed policy; tune constants if needed.
func calcChapterLengthRange(targetWordsPerChapter int) (minLen, maxLen int) {
	if targetWordsPerChapter < 1 {
		targetWordsPerChapter = 2500
	}
	tol := chapterLengthToleranceAbsolute
	if pct := targetWordsPerChapter * chapterLengthTolerancePercent / 100; pct > tol {
		tol = pct
	}
	minLen = targetWordsPerChapter - tol
	if minLen < 1 {
		minLen = 1
	}
	maxLen = targetWordsPerChapter + tol
	return minLen, maxLen
}

func chapterLengthInRange(content string, minLen, maxLen int) bool {
	n := len([]rune(content))
	return n >= minLen && n <= maxLen
}

func formatChapterLengthRequirementBlock(minLen, maxLen, target int, lang string) string {
	if NormalizeLanguage(lang) == LangEN {
		return fmt.Sprintf("Chapter prose must be %d–%d words (target %d; tolerance ±%d words or ±%d%%, whichever is larger). Exceeding %d words is unacceptable — stay inside this chapter's outline only.", minLen, maxLen, target, chapterLengthToleranceAbsolute, chapterLengthTolerancePercent, maxLen)
	}
	return fmt.Sprintf("正文字数须严格控制在 %d–%d 字（目标 %d 字；允许误差 ±%d 字或 ±%d%%，取较大者）。超过 %d 字不可接受，只写本章大纲范围内的情节。", minLen, maxLen, target, chapterLengthToleranceAbsolute, chapterLengthTolerancePercent, maxLen)
}

func formatChapterLengthRetryFeedback(actual, minLen, maxLen int, lang string) string {
	if actual > maxLen {
		if NormalizeLanguage(lang) == LangEN {
			return fmt.Sprintf("IMPORTANT: The previous draft was %d words, exceeding the %d–%d word limit. Regenerate this chapter within the limit. Do not advance into later chapters; compress redundant description and keep only this chapter's outline beats.", actual, minLen, maxLen)
		}
		return fmt.Sprintf("重要：上一稿为 %d 字，超出 %d–%d 字上限。请重新撰写并严格控制在范围内；不要写入后续章节内容，精简冗余描写，只保留本章大纲情节。", actual, minLen, maxLen)
	}
	if NormalizeLanguage(lang) == LangEN {
		return fmt.Sprintf("IMPORTANT: The previous draft was only %d words, below the %d–%d word range. Expand with concrete scene, action, and dialogue while staying inside this chapter's outline.", actual, minLen, maxLen)
	}
	return fmt.Sprintf("重要：上一稿仅 %d 字，低于 %d–%d 字下限。请在不超出本章大纲的前提下补充具体场景、动作与对话。", actual, minLen, maxLen)
}

func mergeWritingConstraints(a, b string) string {
	a = strings.TrimSpace(a)
	b = strings.TrimSpace(b)
	if a == "" {
		return b
	}
	if b == "" {
		return a
	}
	return a + "\n\n" + b
}

func finalizeChapterWritingPrompt(template, rendered string, minLen, maxLen, target int, lang string) string {
	if !strings.Contains(template, "{{.TargetWordsMin}}") {
		rendered += "\n\n" + formatChapterLengthRequirementBlock(minLen, maxLen, target, lang)
	}
	return rendered
}

func generateChapterContentWithLengthControl(ctx context.Context, apiCfg *APIConfig, cfg *Config, state *Progress, idx int, settings *ProjectSettings, extraWritingConstraints string, logger *LogBroadcaster) (string, error) {
	snapshot := state.StoryConfigSnapshot
	if snapshot == nil {
		snapshot = &cfg.Story
	}
	minLen, maxLen := calcChapterLengthRange(snapshot.TargetWordsPerChapter)
	lang := cfg.Language
	lengthFeedback := ""

	for attempt := 0; attempt <= chapterGenMaxLengthAttempts; attempt++ {
		if ctx.Err() != nil {
			return "", fmt.Errorf("任务已取消")
		}
		constraints := mergeWritingConstraints(extraWritingConstraints, lengthFeedback)
		content := generateChapterContentStreamWithRetryLog(ctx, apiCfg, cfg, state, idx, settings, constraints, logger)
		if content == "" {
			return "", fmt.Errorf("正文生成失败或被取消")
		}
		actualLen := len([]rune(content))
		if chapterLengthInRange(content, minLen, maxLen) {
			return content, nil
		}
		if attempt < chapterGenMaxLengthAttempts {
			if logger != nil {
				logger.WarnKey("log.chapter_length_retry", actualLen, minLen, maxLen, attempt+1)
			}
			lengthFeedback = formatChapterLengthRetryFeedback(actualLen, minLen, maxLen, lang)
			continue
		}

		if logger != nil {
			logger.WarnKey("log.chapter_length_off_range", actualLen, minLen, maxLen)
		}
		return content, nil
	}
	return "", fmt.Errorf("正文生成失败或被取消")
}
