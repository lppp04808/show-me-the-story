package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
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

var ErrOutlineChapterNotPending = errors.New("outline chapter not pending")
var ErrOutlineChapterNotFound = errors.New("outline chapter not found")
var ErrOutlineNoChaptersSelected = errors.New("no outline chapters selected")
var ErrOutlineDeleteRangeNotPending = errors.New("outline delete range not pending")

func persistOutlineChapterMutation(cfg *Config, state *Progress, progressPath, cfgPath string) error {
	cfg.Story.ChapterCount = len(state.Chapters)
	if state.StoryConfigSnapshot != nil {
		snapshot := *state.StoryConfigSnapshot
		snapshot.ChapterCount = len(state.Chapters)
		state.StoryConfigSnapshot = &snapshot
	} else {
		snapshot := cfg.Story
		state.StoryConfigSnapshot = &snapshot
	}
	if err := saveConfig(cfgPath, cfg); err != nil {
		return fmt.Errorf("保存配置失败: %w", err)
	}
	if err := SaveProgress(progressPath, state); err != nil {
		return fmt.Errorf("保存进度失败: %w", err)
	}
	return nil
}

func DeletePendingOutlineChapterAction(cfg *Config, state *Progress, progressPath, cfgPath string, chapterNum int) error {
	_, err := DeletePendingOutlineChaptersAction(cfg, state, progressPath, cfgPath, []int{chapterNum})
	return err
}

func DeletePendingOutlineChaptersAction(cfg *Config, state *Progress, progressPath, cfgPath string, chapterNums []int) (int, error) {
	deletedCount, err := DeletePendingOutlineChapters(state, chapterNums)
	if err != nil {
		return 0, err
	}
	if err := persistOutlineChapterMutation(cfg, state, progressPath, cfgPath); err != nil {
		return 0, err
	}
	return deletedCount, nil
}

func DeletePendingOutlineChaptersFromAction(cfg *Config, state *Progress, progressPath, cfgPath string, startNum int) (int, error) {
	deletedCount, err := DeletePendingOutlineChaptersFrom(state, startNum)
	if err != nil {
		return 0, err
	}
	if err := persistOutlineChapterMutation(cfg, state, progressPath, cfgPath); err != nil {
		return 0, err
	}
	return deletedCount, nil
}

func DeletePendingOutlineChapter(state *Progress, chapterNum int) error {
	_, err := DeletePendingOutlineChapters(state, []int{chapterNum})
	return err
}

func DeletePendingOutlineChapters(state *Progress, chapterNums []int) (int, error) {
	nums := normalizeOutlineChapterNums(chapterNums)
	if len(nums) == 0 {
		return 0, ErrOutlineNoChaptersSelected
	}

	indexByNum := make(map[int]int, len(state.Chapters))
	for i, ch := range state.Chapters {
		indexByNum[ch.Num] = i
	}
	for _, num := range nums {
		idx, ok := indexByNum[num]
		if !ok {
			return 0, ErrOutlineChapterNotFound
		}
		if state.Chapters[idx].Status != StatusPending {
			return 0, ErrOutlineChapterNotPending
		}
	}

	deleteSet := make(map[int]bool, len(nums))
	deletedBeforeCurrent := 0
	for _, num := range nums {
		deleteSet[num] = true
		if indexByNum[num] < state.CurrentChapterIndex {
			deletedBeforeCurrent++
		}
	}

	kept := make([]ChapterState, 0, len(state.Chapters)-len(nums))
	for _, ch := range state.Chapters {
		if deleteSet[ch.Num] {
			continue
		}
		ch.Num = len(kept) + 1
		kept = append(kept, ch)
	}
	state.Chapters = kept
	state.CurrentChapterIndex -= deletedBeforeCurrent
	if state.CurrentChapterIndex < 0 {
		state.CurrentChapterIndex = 0
	}
	if state.CurrentChapterIndex > len(state.Chapters) {
		state.CurrentChapterIndex = len(state.Chapters)
	}
	shiftOutlineReferencesAfterDeletes(state, nums)
	return len(nums), nil
}

func DeletePendingOutlineChaptersFrom(state *Progress, startNum int) (int, error) {
	startIdx := -1
	for i, ch := range state.Chapters {
		if ch.Num == startNum {
			startIdx = i
			break
		}
	}
	if startIdx == -1 {
		return 0, ErrOutlineChapterNotFound
	}
	for i := startIdx; i < len(state.Chapters); i++ {
		if state.Chapters[i].Status != StatusPending {
			return 0, ErrOutlineDeleteRangeNotPending
		}
	}
	chapterNums := make([]int, 0, len(state.Chapters)-startIdx)
	for i := startIdx; i < len(state.Chapters); i++ {
		chapterNums = append(chapterNums, state.Chapters[i].Num)
	}
	return DeletePendingOutlineChapters(state, chapterNums)
}

func normalizeOutlineChapterNums(nums []int) []int {
	filtered := make([]int, 0, len(nums))
	for _, num := range nums {
		if num > 0 {
			filtered = append(filtered, num)
		}
	}
	if len(filtered) == 0 {
		return nil
	}
	sort.Ints(filtered)
	out := filtered[:1]
	for _, num := range filtered[1:] {
		if num != out[len(out)-1] {
			out = append(out, num)
		}
	}
	return out
}

func shiftOutlineReferencesAfterDeletes(state *Progress, deletedChapterNums []int) {
	deleteSet := make(map[int]bool, len(deletedChapterNums))
	for _, num := range deletedChapterNums {
		deleteSet[num] = true
	}
	for i := range state.Foreshadows {
		if state.Foreshadows[i].PlantChapter > 0 {
			state.Foreshadows[i].PlantChapter -= countDeletedChaptersLE(deletedChapterNums, state.Foreshadows[i].PlantChapter)
		}
		if state.Foreshadows[i].TargetChapter > 0 {
			state.Foreshadows[i].TargetChapter -= countDeletedChaptersLE(deletedChapterNums, state.Foreshadows[i].TargetChapter)
		}
		if len(state.Foreshadows[i].Events) == 0 {
			continue
		}
		filtered := state.Foreshadows[i].Events[:0]
		for _, ev := range state.Foreshadows[i].Events {
			if deleteSet[ev.Chapter] {
				continue
			}
			if ev.Chapter > 0 {
				ev.Chapter -= countDeletedChaptersLT(deletedChapterNums, ev.Chapter)
			}
			filtered = append(filtered, ev)
		}
		state.Foreshadows[i].Events = filtered
	}
	for i := range state.MemoryEntries {
		if state.MemoryEntries[i].Chapter > 0 {
			state.MemoryEntries[i].Chapter -= countDeletedChaptersLT(deletedChapterNums, state.MemoryEntries[i].Chapter)
		}
	}
}

func countDeletedChaptersLE(deletedChapterNums []int, chapterNum int) int {
	return sort.Search(len(deletedChapterNums), func(i int) bool { return deletedChapterNums[i] > chapterNum })
}

func countDeletedChaptersLT(deletedChapterNums []int, chapterNum int) int {
	return sort.SearchInts(deletedChapterNums, chapterNum)
}

func CreateManualOutlineAction(cfg *Config, state *Progress, progressPath, cfgPath string, chapterCount int) error {
	if chapterCount <= 0 {
		return fmt.Errorf("章节数必须大于 0")
	}

	cfg.Story.ChapterCount = chapterCount
	state.Phase = "outline"
	state.Title = cfg.Story.Title
	state.CorePrompt = ""
	state.StorySynopsis = cfg.Story.StorySynopsis
	state.CurrentChapterIndex = 0
	state.LastForeshadowOutlineReport = nil
	state.LastOutlineCharacterReport = nil
	state.PendingWritingConflict = nil
	state.Chapters = buildManualOutlineChapters(1, chapterCount, cfg.Language)

	snapshot := cfg.Story
	state.StoryConfigSnapshot = &snapshot

	if err := saveConfig(cfgPath, cfg); err != nil {
		return fmt.Errorf("保存配置失败: %w", err)
	}
	if err := SaveProgress(progressPath, state); err != nil {
		return fmt.Errorf("保存进度失败: %w", err)
	}
	_ = DeleteOutlineCheckpoint(OutlineCheckpointPath(progressPath))
	return nil
}

func AppendManualOutlineChaptersAction(cfg *Config, state *Progress, progressPath, cfgPath string, chapterCount int) error {
	if chapterCount <= 0 {
		return fmt.Errorf("章节数必须大于 0")
	}
	startNum := len(state.Chapters) + 1
	return appendManualOutlineChapterStates(cfg, state, progressPath, cfgPath, buildManualOutlineChapters(startNum, chapterCount, cfg.Language))
}

func AppendManualOutlineChaptersFromTextAction(cfg *Config, state *Progress, progressPath, cfgPath, content string) error {
	chapters, err := parseManualOutlineChapters(content, len(state.Chapters)+1, cfg.Language)
	if err != nil {
		return err
	}
	return appendManualOutlineChapterStates(cfg, state, progressPath, cfgPath, chapters)
}

func appendManualOutlineChapterStates(cfg *Config, state *Progress, progressPath, cfgPath string, chapters []ChapterState) error {
	if len(chapters) == 0 {
		return fmt.Errorf("没有可追加的章节")
	}

	state.Chapters = append(state.Chapters, chapters...)
	cfg.Story.ChapterCount = len(state.Chapters)
	if state.StoryConfigSnapshot != nil {
		snapshot := *state.StoryConfigSnapshot
		snapshot.ChapterCount = len(state.Chapters)
		state.StoryConfigSnapshot = &snapshot
	} else {
		snapshot := cfg.Story
		state.StoryConfigSnapshot = &snapshot
	}

	if err := saveConfig(cfgPath, cfg); err != nil {
		return fmt.Errorf("保存配置失败: %w", err)
	}
	if err := SaveProgress(progressPath, state); err != nil {
		return fmt.Errorf("保存进度失败: %w", err)
	}
	_ = DeleteOutlineCheckpoint(OutlineCheckpointPath(progressPath))
	return nil
}

var (
	manualOutlineHeadingPattern = regexp.MustCompile(`(?m)^\s*(第\s*[一二三四五六七八九十百千两零〇\d]+\s*章(?:\s*[《「『"]?[^\n《》「」『』"]+[》」』"]?)?|Chapter\s+\d+(?:[^\n]*)?)\s*$`)
	manualOutlineTitleZH      = regexp.MustCompile(`^\s*第\s*[一二三四五六七八九十百千两零〇\d]+\s*章(?:\s*[《「『"]?([^\n《》「」『』"]+)[》」』"]?)?\s*$`)
	manualOutlineTitleEN      = regexp.MustCompile(`(?i)^\s*chapter\s+\d+\s*(?:[:：-]\s*|["“”])?(.*?)["”]?\s*$`)
)

func parseManualOutlineChapters(content string, startNum int, lang string) ([]ChapterState, error) {
	content = strings.TrimSpace(strings.ReplaceAll(content, "\r\n", "\n"))
	if content == "" {
		return nil, fmt.Errorf("内容不能为空")
	}

	matches := manualOutlineHeadingPattern.FindAllStringIndex(content, -1)
	if len(matches) == 0 {
		return nil, fmt.Errorf("未识别到章节标题，请按“第66章《标题》”这样的格式粘贴")
	}

	chapters := make([]ChapterState, 0, len(matches))
	for i, match := range matches {
		end := len(content)
		if i+1 < len(matches) {
			end = matches[i+1][0]
		}
		segment := strings.TrimSpace(content[match[0]:end])
		if segment == "" {
			continue
		}
		parts := strings.SplitN(segment, "\n", 2)
		heading := strings.TrimSpace(parts[0])
		outline := ""
		if len(parts) > 1 {
			outline = strings.TrimSpace(parts[1])
		}
		if outline == "" {
			return nil, fmt.Errorf("章节 %q 缺少内容", heading)
		}
		num := startNum + len(chapters)
		chapters = append(chapters, ChapterState{
			Num:     num,
			Title:   extractManualOutlineTitle(heading, num, lang),
			Outline: outline,
			Status:  StatusPending,
		})
	}
	if len(chapters) == 0 {
		return nil, fmt.Errorf("未识别到可追加的章节")
	}
	return chapters, nil
}

func extractManualOutlineTitle(heading string, num int, lang string) string {
	if m := manualOutlineTitleZH.FindStringSubmatch(heading); len(m) > 1 {
		if title := strings.TrimSpace(m[1]); title != "" {
			return title
		}
	}
	if m := manualOutlineTitleEN.FindStringSubmatch(heading); len(m) > 1 {
		if title := strings.TrimSpace(strings.Trim(m[1], `"“”`)); title != "" {
			return title
		}
	}
	return defaultManualOutlineChapterTitle(num, lang)
}

func buildManualOutlineChapters(startNum, count int, lang string) []ChapterState {
	chapters := make([]ChapterState, count)
	for i := 0; i < count; i++ {
		num := startNum + i
		chapters[i] = ChapterState{
			Num:     num,
			Title:   defaultManualOutlineChapterTitle(num, lang),
			Status:  StatusPending,
			Outline: "",
		}
	}
	return chapters
}

func defaultManualOutlineChapterTitle(num int, lang string) string {
	if NormalizeLanguage(lang) == LangEN {
		return fmt.Sprintf("Chapter %d", num)
	}
	return fmt.Sprintf("第%d章", num)
}

func firstIncompleteOutlineChapter(state *Progress) int {
	for _, ch := range state.Chapters {
		if strings.TrimSpace(ch.Title) == "" || strings.TrimSpace(ch.Outline) == "" {
			return ch.Num
		}
	}
	return 0
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

func EditChapterOutline(state *Progress, chapterNum int, title, outline string, selectedForeshadowIDs *[]int) error {
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
	if state.Chapters[idx].Status == StatusWriting {
		return fmt.Errorf("写作中的章节暂不能编辑大纲")
	}
	state.Chapters[idx].Title = title
	state.Chapters[idx].Outline = outline
	if selectedForeshadowIDs != nil {
		state.Chapters[idx].SelectedForeshadowIDs = normalizeForeshadowIDs(*selectedForeshadowIDs)
	}
	return nil
}

func normalizeForeshadowIDs(ids []int) []int {
	if len(ids) == 0 {
		return nil
	}
	seen := make(map[int]bool, len(ids))
	out := make([]int, 0, len(ids))
	for _, id := range ids {
		if id <= 0 || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
