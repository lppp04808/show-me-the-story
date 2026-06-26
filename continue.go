package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
)

type ContinueAnalysis struct {
	Title         string            `json:"title"`
	StoryType     string            `json:"story_type"`
	CorePrompt    string            `json:"core_prompt"`
	StorySynopsis string            `json:"story_synopsis"`
	WritingStyle  string            `json:"writing_style"`
	WritingPOV    string            `json:"writing_pov"`
	Chapters      []ContinueChapter `json:"chapters"`
}

type ContinueChapter struct {
	Num     int    `json:"num"`
	Title   string `json:"title"`
	Outline string `json:"outline,omitempty"`
	Summary string `json:"summary,omitempty"`
	Content string `json:"content,omitempty"`
}

func AnalyzeExistingContent(ctx context.Context, apiCfg *APIConfig, cfg *Config, content string) (*ContinueAnalysis, error) {
	userPrompt := RenderPrompt(cfg.Prompts.ContentAnalysis, map[string]string{
		"ExistingContent": content,
	})

	systemPrompt := SystemPromptFor(cfg.Language, "content_analyst_json")

	apiCfg.NeedJSON = true
	rawResp := CallAPIWithRetry(ctx, apiCfg, systemPrompt, userPrompt)
	apiCfg.NeedJSON = false
	if rawResp == "" {
		return nil, fmt.Errorf("API 调用失败或被取消")
	}
	rawResp = cleanJSONResponse(rawResp)

	var resp ContinueAnalysis
	if err := json.Unmarshal([]byte(rawResp), &resp); err != nil {
		return nil, fmt.Errorf("解析分析结果JSON失败: %w", err)
	}

	return &resp, nil
}

func splitContentByChapters(content string, chapters []ContinueChapter) []string {
	if len(chapters) == 0 {
		return nil
	}

	re := regexp.MustCompile(`(?m)^[\s]*(第[一二三四五六七八九十百千\d]+章|Chapter\s+\d+|#\s+Chapter\s+\d+|第\d+章)`)
	matches := re.FindAllStringIndex(content, -1)

	if len(matches) == 0 {
		return []string{content}
	}

	segments := make([]string, 0, len(matches))
	for i, match := range matches {
		start := match[0]
		end := len(content)
		if i+1 < len(matches) {
			end = matches[i+1][0]
		}
		seg := strings.TrimSpace(content[start:end])
		if seg != "" {
			segments = append(segments, seg)
		}
	}

	if len(segments) == 0 {
		return []string{content}
	}

	return segments
}

func ImportContinueAction(cfg *Config, state *Progress, analysis *ContinueAnalysis, content string, progressPath string, cfgPath string) error {
	state.Title = analysis.Title
	state.CorePrompt = analysis.CorePrompt
	state.StorySynopsis = analysis.StorySynopsis

	segments := splitContentByChapters(content, analysis.Chapters)

	state.Chapters = make([]ChapterState, 0, len(analysis.Chapters))
	for i, ch := range analysis.Chapters {
		chapterContent := ""
		if i < len(segments) {
			chapterContent = segments[i]
		}
		state.Chapters = append(state.Chapters, ChapterState{
			Num:     i + 1,
			Title:   ch.Title,
			Outline: ch.Outline,
			Content: chapterContent,
			Summary: ch.Summary,
			Status:  StatusAccepted,
		})
	}

	state.CurrentChapterIndex = len(analysis.Chapters)
	state.Phase = "outline"

	snapshot := StoryConfig{
		Type:                  analysis.StoryType,
		Title:                 analysis.Title,
		ChapterCount:          len(state.Chapters),
		TargetWordsPerChapter: cfg.Story.TargetWordsPerChapter,
		WritingStyle:          analysis.WritingStyle,
		WritingPOV:            analysis.WritingPOV,
		StorySynopsis:         analysis.StorySynopsis,
	}
	state.StoryConfigSnapshot = &snapshot

	oldStory := cfg.Story

	cfg.Story.Type = analysis.StoryType
	cfg.Story.Title = analysis.Title
	cfg.Story.WritingStyle = analysis.WritingStyle
	cfg.Story.WritingPOV = analysis.WritingPOV
	cfg.Story.StorySynopsis = analysis.StorySynopsis

	if err := SaveProgress(progressPath, state); err != nil {
		cfg.Story = oldStory
		return fmt.Errorf("保存进度失败: %w", err)
	}

	if err := saveConfig(cfgPath, cfg); err != nil {
		cfg.Story = oldStory
		return fmt.Errorf("保存配置失败: %w", err)
	}

	return nil
}

func GenerateContinuationOutline(ctx context.Context, apiCfg *APIConfig, cfg *Config, state *Progress, settings *ProjectSettings, newChapterCount int, progressPath string, logger *LogBroadcaster) error {
	logger.StepInfo(1, 2, "正在构建已有章节上下文...")

	lang := cfg.Language
	snapshot := state.StoryConfigSnapshot
	if snapshot == nil {
		snapshot = &cfg.Story
	}
	checkpointPath := OutlineCheckpointPath(progressPath)

	fingerprint := BuildContinuationOutlineFingerprint(cfg, state, settings, newChapterCount)

	startNum := len(state.Chapters) + 1
	batchSize := outlineBatchSizeDefault
	generated := make([]OutlineChapter, 0, newChapterCount)
	cp, err := LoadOutlineCheckpoint(checkpointPath)
	if err != nil {
		return err
	}
	if cp != nil && cp.Mode == outlineCheckpointModeContinuation && cp.Fingerprint == fingerprint && len(cp.CompletedChapters) > 0 {
		generated = cloneOutlineChapters(cp.CompletedChapters)
		if cp.CurrentBatchSize > 0 {
			batchSize = cp.CurrentBatchSize
		}
		if logger != nil {
			logger.InfoBilingual(
				fmt.Sprintf("检测到未完成的续写大纲断点，已恢复本次新增章节 %d/%d，将从第 %d 章继续。", len(generated), newChapterCount, cp.NextStartNum),
				fmt.Sprintf("Recovered unfinished continuation checkpoint: %d/%d new chapters restored; resuming from chapter %d.", len(generated), newChapterCount, cp.NextStartNum),
			)
		}
	} else {
		_ = DeleteOutlineCheckpoint(checkpointPath)
	}

	for nextNum := startNum + len(generated); nextNum < startNum+newChapterCount; {
		remaining := startNum + newChapterCount - nextNum
		currentBatchSize := batchSize
		if remaining < currentBatchSize {
			currentBatchSize = remaining
		}
		if logger != nil {
			logger.Info(formatOutlineBatchProgress(nextNum, currentBatchSize, len(generated), newChapterCount, lang))
		}
		existingOutline := buildContinuationExistingOutline(state.Chapters, generated, lang)
		chapters, err := generateOutlineChaptersOnly(ctx, apiCfg, cfg, settings, cfg.Prompts.ContinuationOutlineGeneration, map[string]string{
			"Title":             state.Title,
			"StoryType":         snapshot.Type,
			"CorePrompt":        state.CorePrompt,
			"StorySynopsis":     state.StorySynopsis,
			"WritingStyle":      snapshot.WritingStyle,
			"WritingPOV":        snapshot.WritingPOV,
			"ExistingOutline":   existingOutline,
			"NewChapterCount":   fmt.Sprintf("%d", currentBatchSize),
			"StartNum":          fmt.Sprintf("%d", nextNum),
			"TotalChapterCount": fmt.Sprintf("%d", len(state.Chapters)+newChapterCount),
		}, logger)
		if err != nil {
			if errors.Is(err, errOutlineBatchMalformed) && batchSize > outlineBatchSizeReduced && currentBatchSize > outlineBatchSizeReduced {
				if logger != nil {
					logger.Warn(formatOutlineBatchReduceLog(nextNum, currentBatchSize, lang))
				}
				batchSize = outlineBatchSizeReduced
				if saveErr := SaveOutlineCheckpoint(checkpointPath, &OutlineCheckpoint{
					Mode:                 outlineCheckpointModeContinuation,
					Fingerprint:          fingerprint,
					Title:                state.Title,
					CorePrompt:           state.CorePrompt,
					StorySynopsis:        state.StorySynopsis,
					TotalChapters:        len(state.Chapters) + newChapterCount,
					RequestedNewChapters: newChapterCount,
					NextStartNum:         nextNum,
					CurrentBatchSize:     batchSize,
					CompletedChapters:    cloneOutlineChapters(generated),
				}); saveErr != nil {
					return saveErr
				}
				continue
			}
			return err
		}
		generated = append(generated, chapters...)
		if logger != nil {
			logger.Info(formatOutlineBatchDone(nextNum, len(chapters), len(generated), newChapterCount, lang))
		}
		if err := SaveOutlineCheckpoint(checkpointPath, &OutlineCheckpoint{
			Mode:                 outlineCheckpointModeContinuation,
			Fingerprint:          fingerprint,
			Title:                state.Title,
			CorePrompt:           state.CorePrompt,
			StorySynopsis:        state.StorySynopsis,
			TotalChapters:        len(state.Chapters) + newChapterCount,
			RequestedNewChapters: newChapterCount,
			NextStartNum:         startNum + len(generated),
			CurrentBatchSize:     batchSize,
			CompletedChapters:    cloneOutlineChapters(generated),
		}); err != nil {
			return err
		}
		nextNum += len(chapters)
	}

	logger.StepInfo(2, 2, "正在保存续写大纲...")

	for _, ch := range generated {
		state.Chapters = append(state.Chapters, ChapterState{
			Num:     ch.Num,
			Title:   ch.Title,
			Outline: ch.Outline,
			Status:  StatusPending,
		})
	}

	if err := SaveProgress(progressPath, state); err != nil {
		return fmt.Errorf("保存进度失败: %w", err)
	}

	runOutlinePostProcessChecks(ctx, apiCfg, cfg, state, settings, progressPath, logger)

	logger.InfoKey("log.continuation_outline_summary", len(generated), len(state.Chapters))
	return nil
}

func buildContinuationExistingOutline(existing []ChapterState, generated []OutlineChapter, lang string) string {
	var sb strings.Builder
	for _, ch := range existing {
		status := ""
		if ch.Status == StatusAccepted {
			status = "✅"
		}
		if NormalizeLanguage(lang) == LangEN {
			sb.WriteString(fmt.Sprintf("Chapter %d \"%s\"%s: %s\n", ch.Num, ch.Title, status, ch.Outline))
		} else {
			sb.WriteString(fmt.Sprintf("第%d章《%s》%s: %s\n", ch.Num, ch.Title, status, ch.Outline))
		}
	}
	for _, ch := range generated {
		sb.WriteString(formatChapterLine(ch.Num, ch.Title, ch.Outline, lang))
	}
	return sb.String()
}
