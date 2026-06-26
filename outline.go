package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

type OutlineResponse struct {
	Title         string           `json:"title"`
	CorePrompt    string           `json:"core_prompt"`
	StorySynopsis string           `json:"story_synopsis"`
	Chapters      []OutlineChapter `json:"chapters"`
}

type OutlineChapter struct {
	Num     int    `json:"num"`
	Title   string `json:"title"`
	Outline string `json:"outline"`
}

type outlineAPICallFunc func(ctx context.Context, apiCfg *APIConfig, systemPrompt, userPrompt string, logger *LogBroadcaster) string

var outlineAPICall outlineAPICallFunc = func(ctx context.Context, apiCfg *APIConfig, systemPrompt, userPrompt string, logger *LogBroadcaster) string {
	if logger != nil {
		return CallAPIWithRetryLog(ctx, apiCfg, systemPrompt, userPrompt, logger)
	}
	return CallAPIWithRetry(ctx, apiCfg, systemPrompt, userPrompt)
}

func parseOutlineResponse(rawResp string) (*OutlineResponse, error) {
	rawResp = cleanJSONResponse(rawResp)
	var resp OutlineResponse
	if err := json.Unmarshal([]byte(rawResp), &resp); err != nil {
		return nil, fmt.Errorf("解析大纲JSON失败: %w\n原始响应: %s", err, rawResp)
	}
	return &resp, nil
}

func generateOutline(ctx context.Context, apiCfg *APIConfig, cfg *Config, settings *ProjectSettings, logger *LogBroadcaster, checkpointPath string) (*OutlineResponse, error) {
	totalChapters := cfg.Story.ChapterCount
	if totalChapters <= 0 {
		totalChapters = 1
	}
	batchSize := outlineBatchSizeDefault
	fingerprint := BuildInitialOutlineFingerprint(cfg, settings)

	var result *OutlineResponse
	cp, err := LoadOutlineCheckpoint(checkpointPath)
	if err != nil {
		return nil, err
	}
	if ValidateOutlineCheckpoint(cp, cfg, nil, settings) && cp.Mode == outlineCheckpointModeInitial && len(cp.CompletedChapters) > 0 {
		result = &OutlineResponse{
			Title:         cp.Title,
			CorePrompt:    cp.CorePrompt,
			StorySynopsis: cp.StorySynopsis,
			Chapters:      cloneOutlineChapters(cp.CompletedChapters),
		}
		if cp.CurrentBatchSize > 0 {
			batchSize = cp.CurrentBatchSize
		}
		if logger != nil {
			logger.InfoBilingual(
				fmt.Sprintf("检测到未完成的大纲断点，已恢复前 %d/%d 章，将从第 %d 章继续。", len(result.Chapters), totalChapters, cp.NextStartNum),
				fmt.Sprintf("Recovered unfinished outline checkpoint: %d/%d chapters restored; resuming from chapter %d.", len(result.Chapters), totalChapters, cp.NextStartNum),
			)
		}
	} else {
		_ = DeleteOutlineCheckpoint(checkpointPath)
	}

	if result == nil {
		firstBatch, err := generateOutlineFirstBatch(ctx, apiCfg, cfg, settings, batchSize, totalChapters, logger)
		if err != nil {
			if errors.Is(err, errOutlineBatchMalformed) {
				if logger != nil {
					logger.Warn(formatOutlineBatchReduceLog(1, min(batchSize, totalChapters), cfg.Language))
				}
				batchSize = outlineBatchSizeReduced
				firstBatch, err = generateOutlineFirstBatch(ctx, apiCfg, cfg, settings, batchSize, totalChapters, logger)
			}
			if err != nil {
				return nil, err
			}
		}

		result = &OutlineResponse{
			Title:         firstBatch.Title,
			CorePrompt:    firstBatch.CorePrompt,
			StorySynopsis: firstBatch.StorySynopsis,
			Chapters:      append([]OutlineChapter(nil), firstBatch.Chapters...),
		}
		if err := SaveOutlineCheckpoint(checkpointPath, &OutlineCheckpoint{
			Mode:              outlineCheckpointModeInitial,
			Fingerprint:       fingerprint,
			Title:             result.Title,
			CorePrompt:        result.CorePrompt,
			StorySynopsis:     result.StorySynopsis,
			TotalChapters:     totalChapters,
			NextStartNum:      len(result.Chapters) + 1,
			CurrentBatchSize:  batchSize,
			CompletedChapters: cloneOutlineChapters(result.Chapters),
		}); err != nil {
			return nil, err
		}
	}

	if len(result.Chapters) >= totalChapters {
		result.Chapters = result.Chapters[:totalChapters]
		_ = DeleteOutlineCheckpoint(checkpointPath)
		return result, nil
	}

	for startNum := len(result.Chapters) + 1; startNum <= totalChapters; {
		remaining := totalChapters - startNum + 1
		currentBatchSize := batchSize
		if remaining < currentBatchSize {
			currentBatchSize = remaining
		}
		if logger != nil {
			logger.Info(formatOutlineBatchProgress(startNum, currentBatchSize, len(result.Chapters), totalChapters, cfg.Language))
		}
		chapters, err := generateOutlineChaptersOnly(ctx, apiCfg, cfg, settings, cfg.Prompts.ContinuationOutlineGeneration, map[string]string{
			"Title":             result.Title,
			"StoryType":         cfg.Story.Type,
			"CorePrompt":        result.CorePrompt,
			"StorySynopsis":     result.StorySynopsis,
			"WritingStyle":      cfg.Story.WritingStyle,
			"WritingPOV":        cfg.Story.WritingPOV,
			"ExistingOutline":   formatOutlineContext(result.Chapters, cfg.Language),
			"NewChapterCount":   fmt.Sprintf("%d", currentBatchSize),
			"StartNum":          fmt.Sprintf("%d", startNum),
			"TotalChapterCount": fmt.Sprintf("%d", totalChapters),
		}, logger)
		if err != nil {
			if errors.Is(err, errOutlineBatchMalformed) && batchSize > outlineBatchSizeReduced && currentBatchSize > outlineBatchSizeReduced {
				if logger != nil {
					logger.Warn(formatOutlineBatchReduceLog(startNum, currentBatchSize, cfg.Language))
				}
				batchSize = outlineBatchSizeReduced
				if saveErr := SaveOutlineCheckpoint(checkpointPath, &OutlineCheckpoint{
					Mode:              outlineCheckpointModeInitial,
					Fingerprint:       fingerprint,
					Title:             result.Title,
					CorePrompt:        result.CorePrompt,
					StorySynopsis:     result.StorySynopsis,
					TotalChapters:     totalChapters,
					NextStartNum:      startNum,
					CurrentBatchSize:  batchSize,
					CompletedChapters: cloneOutlineChapters(result.Chapters),
				}); saveErr != nil {
					return nil, saveErr
				}
				continue
			}
			return nil, err
		}
		result.Chapters = append(result.Chapters, chapters...)
		if logger != nil {
			logger.Info(formatOutlineBatchDone(startNum, len(chapters), len(result.Chapters), totalChapters, cfg.Language))
		}
		if err := SaveOutlineCheckpoint(checkpointPath, &OutlineCheckpoint{
			Mode:              outlineCheckpointModeInitial,
			Fingerprint:       fingerprint,
			Title:             result.Title,
			CorePrompt:        result.CorePrompt,
			StorySynopsis:     result.StorySynopsis,
			TotalChapters:     totalChapters,
			NextStartNum:      len(result.Chapters) + 1,
			CurrentBatchSize:  batchSize,
			CompletedChapters: cloneOutlineChapters(result.Chapters),
		}); err != nil {
			return nil, err
		}
		startNum += len(chapters)
	}

	_ = DeleteOutlineCheckpoint(checkpointPath)
	return result, nil
}

func generateOutlineFirstBatch(ctx context.Context, apiCfg *APIConfig, cfg *Config, settings *ProjectSettings, batchSize int, totalChapters int, logger *LogBroadcaster) (*OutlineResponse, error) {
	batchCount := min(batchSize, totalChapters)
	chapterCountStr := fmt.Sprintf("%d", totalChapters)
	targetWordsStr := fmt.Sprintf("%d", cfg.Story.TargetWordsPerChapter)
	data := mergeOutlinePromptData(map[string]string{
		"StoryType":         cfg.Story.Type,
		"ChapterCount":      chapterCountStr,
		"TargetWords":       targetWordsStr,
		"WritingStyle":      cfg.Story.WritingStyle,
		"WritingPOV":        cfg.Story.WritingPOV,
		"StorySynopsis":     cfg.Story.StorySynopsis,
		"BatchStart":        "1",
		"BatchCount":        fmt.Sprintf("%d", batchCount),
		"BatchEnd":          fmt.Sprintf("%d", batchCount),
		"TotalChapterCount": chapterCountStr,
	}, cfg, settings)

	systemPrompt := SystemPromptFor(cfg.Language, "outline_editor_json")
	minLen, _ := calcOutlineLengthRange(cfg.Story.TargetWordsPerChapter)
	batchHint := buildOutlineBatchHint(1, batchCount, totalChapters, cfg.Language)

	var lastResp *OutlineResponse
	var lastShort []int
	for attempt := 0; attempt < outlineGenMaxAttempts; attempt++ {
		if logger != nil {
			logger.Info(formatOutlineBatchProgress(1, batchCount, 0, totalChapters, cfg.Language))
		}
		userPrompt := finalizeOutlinePrompt(cfg.Prompts.OutlineGeneration,
			RenderPrompt(cfg.Prompts.OutlineGeneration, data), cfg, settings, batchHint)
		if attempt > 0 {
			if logger != nil {
				logger.Info(formatOutlineBatchProgress(1, batchCount, 0, totalChapters, cfg.Language))
			}
			userPrompt += formatShortOutlineRetryFeedback(lastShort, minLen, cfg.Language)
		}

		var rawResp string
		rawResp = outlineAPICall(ctx, apiCfg, systemPrompt, userPrompt, logger)
		if rawResp == "" {
			return nil, fmt.Errorf("API 调用失败或被取消")
		}

		resp, err := parseOutlineResponse(rawResp)
		if err != nil {
			return nil, fmt.Errorf("%w: %v", errOutlineBatchMalformed, err)
		}
		if err = validateOutlineBatch(resp.Chapters, 1, batchCount); err != nil {
			return nil, err
		}
		lastResp = resp
		lastShort = validateOutlineChapterLengths(resp.Chapters, minLen)
		if len(lastShort) == 0 {
			if logger != nil {
				logger.Info(formatOutlineBatchDone(1, len(resp.Chapters), len(resp.Chapters), totalChapters, cfg.Language))
			}
			return resp, nil
		}
		if logger != nil {
			logger.WarnKey("log.outline_chapters_too_short", strings.Join(intSliceToStr(lastShort), ", "), minLen)
		}
	}

	if logger != nil && len(lastShort) > 0 {
		logger.WarnKey("log.outline_chapters_still_short", strings.Join(intSliceToStr(lastShort), ", "), minLen)
	}
	return lastResp, nil
}

func intSliceToStr(nums []int) []string {
	out := make([]string, len(nums))
	for i, n := range nums {
		out[i] = fmt.Sprintf("%d", n)
	}
	return out
}

func generateOutlineChaptersOnly(ctx context.Context, apiCfg *APIConfig, cfg *Config, settings *ProjectSettings, template string, baseData map[string]string, logger *LogBroadcaster) ([]OutlineChapter, error) {
	data := mergeOutlinePromptData(baseData, cfg, settings)
	systemPrompt := SystemPromptFor(cfg.Language, "outline_editor_json")
	minLen, _ := calcOutlineLengthRange(cfg.Story.TargetWordsPerChapter)
	startNum, batchCount, totalChapterCount := parseOutlineBatchMeta(baseData)
	batchHint := buildOutlineBatchHint(startNum, batchCount, totalChapterCount, cfg.Language)

	var lastChapters []OutlineChapter
	var lastShort []int
	for attempt := 0; attempt < outlineGenMaxAttempts; attempt++ {
		userPrompt := finalizeOutlinePrompt(template, RenderPrompt(template, data), cfg, settings, batchHint)
		if attempt > 0 {
			if logger != nil && batchCount > 0 {
				logger.Info(formatOutlineBatchProgress(startNum, batchCount, startNum-1, totalChapterCount, cfg.Language))
			}
			userPrompt += formatShortOutlineRetryFeedback(lastShort, minLen, cfg.Language)
		}

		apiCfg.NeedJSON = true
		rawResp := outlineAPICall(ctx, apiCfg, systemPrompt, userPrompt, logger)
		apiCfg.NeedJSON = false
		if rawResp == "" {
			return nil, fmt.Errorf("API 调用失败或被取消")
		}

		var resp struct {
			Chapters []OutlineChapter `json:"chapters"`
		}
		rawResp = cleanJSONResponse(rawResp)
		if err := json.Unmarshal([]byte(rawResp), &resp); err != nil {
			return nil, fmt.Errorf("%w: 解析大纲JSON失败: %v\n原始响应: %s", errOutlineBatchMalformed, err, rawResp)
		}
		if err := validateOutlineBatch(resp.Chapters, startNum, batchCount); err != nil {
			return nil, err
		}
		lastChapters = resp.Chapters
		lastShort = validateOutlineChapterLengths(resp.Chapters, minLen)
		if len(lastShort) == 0 {
			return resp.Chapters, nil
		}
		logger.WarnKey("log.outline_chapters_too_short", strings.Join(intSliceToStr(lastShort), ", "), minLen)
	}

	if len(lastShort) > 0 {
		logger.WarnKey("log.outline_chapters_still_short", strings.Join(intSliceToStr(lastShort), ", "), minLen)
	}
	return lastChapters, nil
}

func reviseOutline(ctx context.Context, apiCfg *APIConfig, cfg *Config, state *Progress, settings *ProjectSettings, userFeedback, progressPath, cfgPath string, logger *LogBroadcaster) error {
	lang := cfg.Language
	en := NormalizeLanguage(lang) == LangEN

	lockedChapters := ""
	for _, ch := range state.Chapters {
		if ch.Status == StatusAccepted {
			lockedChapters += formatChapterLine(ch.Num, ch.Title, ch.Outline, lang)
		}
	}
	if lockedChapters == "" {
		if en {
			lockedChapters = "(no locked chapters)"
		} else {
			lockedChapters = "无已锁定章节。"
		}
	}

	currentOutline := ""
	for _, ch := range state.Chapters {
		currentOutline += formatChapterLine(ch.Num, ch.Title, ch.Outline, lang)
	}

	data := mergeOutlinePromptData(map[string]string{
		"CurrentOutline": currentOutline,
		"UserFeedback":   userFeedback,
		"LockedChapters": lockedChapters,
	}, cfg, settings)

	systemPrompt := SystemPromptFor(lang, "outline_editor_locked_json")
	minLen, _ := calcOutlineLengthRange(cfg.Story.TargetWordsPerChapter)

	var resp OutlineResponse
	var lastShort []int
	for attempt := 0; attempt < outlineGenMaxAttempts; attempt++ {
		userPrompt := finalizeOutlinePrompt(cfg.Prompts.OutlineRevision,
			RenderPrompt(cfg.Prompts.OutlineRevision, data), cfg, settings, "")
		if attempt > 0 {
			userPrompt += formatShortOutlineRetryFeedback(lastShort, minLen, lang)
		}

		rawResp := CallAPIWithRetry(ctx, apiCfg, systemPrompt, userPrompt)
		if rawResp == "" {
			return fmt.Errorf("API 调用失败或被取消")
		}
		parsed, err := parseOutlineResponse(rawResp)
		if err != nil {
			return err
		}
		resp = *parsed
		lastShort = validateOutlineChapterLengths(resp.Chapters, minLen)
		if len(lastShort) == 0 {
			break
		}
		if logger != nil {
			logger.WarnKey("log.outline_chapters_too_short", strings.Join(intSliceToStr(lastShort), ", "), minLen)
		}
	}

	return applyOutlineRevision(cfg, state, resp, "outline_revision", PendingConfigChangesPath(progressPath), cfgPath, logger)
}

func applyOutlineRevision(cfg *Config, state *Progress, resp OutlineResponse, source, pendingPath, cfgPath string, logger *LogBroadcaster) error {
	lockedMap := make(map[int]bool)
	for _, ch := range state.Chapters {
		if ch.Status == StatusAccepted {
			lockedMap[ch.Num] = true
		}
	}

	for _, newCh := range resp.Chapters {
		for i, existingCh := range state.Chapters {
			if existingCh.Num == newCh.Num && !lockedMap[newCh.Num] {
				state.Chapters[i].Title = newCh.Title
				state.Chapters[i].Outline = newCh.Outline
			}
		}
	}

	return applyOutlineMetaWithGuard(cfg, state, resp, source, pendingPath, cfgPath, logger)
}

func truncate(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen]) + "..."
}

func cleanJSONResponse(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "```json") {
		s = strings.TrimPrefix(s, "```json")
	} else if strings.HasPrefix(s, "```") {
		s = strings.TrimPrefix(s, "```")
	}
	if strings.HasSuffix(s, "```") {
		s = strings.TrimSuffix(s, "```")
	}
	return strings.TrimSpace(s)
}

func GenerateOutlineAction(ctx context.Context, apiCfg *APIConfig, cfg *Config, state *Progress, settings *ProjectSettings, progressPath, cfgPath string, logger *LogBroadcaster) error {
	var err error
	if err = validateAPIConfig(apiCfg); err != nil {
		return err
	}
	checkpointPath := OutlineCheckpointPath(progressPath)
	defer func() {
		if err == nil && ctx.Err() == nil {
			_ = DeleteOutlineCheckpoint(checkpointPath)
		}
	}()
	for _, ch := range state.Chapters {
		if ch.Status == StatusAccepted {
			return fmt.Errorf("存在已确认章节，无法整体重新生成大纲（会覆盖已完成内容）。如需追加章节请使用「生成后续大纲」")
		}
	}

	logger.StepInfo(1, 2, "正在调用 AI 生成大纲...")

	var outlineResp *OutlineResponse

	apiCfg.NeedJSON = true

	outlineResp, err = generateOutline(ctx, apiCfg, cfg, settings, logger, checkpointPath)

	apiCfg.NeedJSON = false
	if err != nil {
		return fmt.Errorf("生成大纲失败: %w", err)
	}

	logger.StepInfo(2, 2, "正在保存大纲...")

	state.Chapters = make([]ChapterState, len(outlineResp.Chapters))
	for i, ch := range outlineResp.Chapters {
		state.Chapters[i] = ChapterState{
			Num:     ch.Num,
			Title:   ch.Title,
			Outline: ch.Outline,
			Status:  StatusPending,
		}
	}

	if err = applyOutlineMetaWithGuard(cfg, state, *outlineResp, "outline_generation", PendingConfigChangesPath(progressPath), cfgPath, logger); err != nil {
		return err
	}

	snapshot := cfg.Story
	state.StoryConfigSnapshot = &snapshot

	if err = SaveProgress(progressPath, state); err != nil {
		return fmt.Errorf("保存进度失败: %w", err)
	}

	runOutlinePostProcessChecks(ctx, apiCfg, cfg, state, settings, progressPath, logger)

	logger.SuccessKey("log.outline_generate_summary", len(state.Chapters), state.Title)
	return nil
}

func ReviseOutlineAction(ctx context.Context, apiCfg *APIConfig, cfg *Config, state *Progress, settings *ProjectSettings, progressPath, cfgPath, feedback string, logger *LogBroadcaster) error {
	logger.StepInfo(1, 2, "正在根据意见修订大纲...")

	if err := reviseOutline(ctx, apiCfg, cfg, state, settings, feedback, progressPath, cfgPath, logger); err != nil {
		return fmt.Errorf("修订大纲失败: %w", err)
	}

	logger.StepInfo(2, 2, "正在保存修订后的大纲...")

	if err := SaveProgress(progressPath, state); err != nil {
		return fmt.Errorf("保存进度失败: %w", err)
	}

	runOutlinePostProcessChecks(ctx, apiCfg, cfg, state, settings, progressPath, logger)

	logger.SuccessKey("log.outline_revise_summary", len(state.Chapters))
	return nil
}

func ConfirmOutlineAction(state *Progress, progressPath string) error {
	if len(state.Chapters) == 0 {
		return fmt.Errorf("大纲为空")
	}

	state.Phase = "writing"
	return SaveProgress(progressPath, state)
}

func EditChapterOutline(state *Progress, chapterNum int, title, outline string) error {
	idx := -1
	for i, ch := range state.Chapters {
		if ch.Num == chapterNum {
			idx = i
			break
		}
	}
	if idx == -1 {
		return fmt.Errorf("章节 %d 不存在", chapterNum)
	}
	if state.Chapters[idx].Status != StatusPending {
		return fmt.Errorf("只能编辑待定（pending）状态的章节大纲")
	}
	state.Chapters[idx].Title = title
	state.Chapters[idx].Outline = outline
	return nil
}
