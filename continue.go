package main

import (
	"encoding/json"
	"fmt"
)

type ContinueAnalysis struct {
	Title                string            `json:"title"`
	CorePrompt           string            `json:"core_prompt"`
	CoreRequirements     string            `json:"core_requirements"`
	WritingStyle         string            `json:"writing_style"`
	CharacterSetting     string            `json:"character_setting"`
	WorldSetting         string            `json:"world_setting"`
	Chapters             []ContinueChapter `json:"chapters"`
	ContinuationChapters []ContinueChapter `json:"continuation_chapters"`
}

type ContinueChapter struct {
	Num     int    `json:"num"`
	Title   string `json:"title"`
	Summary string `json:"summary,omitempty"`
	Outline string `json:"outline,omitempty"`
	Content string `json:"content,omitempty"`
}

func AnalyzeExistingContent(cfg *Config, content string, continuationCount int) (*ContinueAnalysis, error) {
	userPrompt := RenderPrompt(cfg.Prompts.ContentAnalysis, map[string]string{
		"ExistingContent":    content,
		"ContinuationCount": fmt.Sprintf("%d", continuationCount),
	})

	systemPrompt := "你是一位专业的小说分析编辑。请严格按照要求的JSON格式输出，不要添加任何额外文字或markdown代码块标记。"

	rawResp := CallAPIWithRetry(cfg, systemPrompt, userPrompt)
	rawResp = cleanJSONResponse(rawResp)

	var resp ContinueAnalysis
	if err := json.Unmarshal([]byte(rawResp), &resp); err != nil {
		return nil, fmt.Errorf("解析分析结果JSON失败: %w", err)
	}

	return &resp, nil
}

func ImportContinueAction(cfg *Config, state *Progress, analysis *ContinueAnalysis, progressPath string, cfgPath string) error {
	state.Title = analysis.Title
	state.CorePrompt = analysis.CorePrompt
	state.CoreRequirements = analysis.CoreRequirements

	state.Chapters = make([]ChapterState, 0, len(analysis.Chapters)+len(analysis.ContinuationChapters))

	for i, ch := range analysis.Chapters {
		state.Chapters = append(state.Chapters, ChapterState{
			Num:     i + 1,
			Title:   ch.Title,
			Outline: "",
			Content: ch.Content,
			Summary: ch.Summary,
			Status:  StatusAccepted,
		})
	}

	offset := len(analysis.Chapters)
	for i, ch := range analysis.ContinuationChapters {
		state.Chapters = append(state.Chapters, ChapterState{
			Num:     offset + i + 1,
			Title:   ch.Title,
			Outline: ch.Outline,
			Status:  StatusPending,
		})
	}

	state.CurrentChapterIndex = len(analysis.Chapters)
	state.Phase = "writing"

	snapshot := StoryConfig{
		Type:                  cfg.Story.Type,
		Title:                 analysis.Title,
		ChapterCount:          len(state.Chapters),
		TargetWordsPerChapter: cfg.Story.TargetWordsPerChapter,
		WritingStyle:          analysis.WritingStyle,
		CharacterSetting:      analysis.CharacterSetting,
		WorldSetting:          analysis.WorldSetting,
		CoreRequirements:      analysis.CoreRequirements,
	}
	state.StoryConfigSnapshot = &snapshot

	oldWritingStyle := cfg.Story.WritingStyle
	oldCharacterSetting := cfg.Story.CharacterSetting
	oldWorldSetting := cfg.Story.WorldSetting
	oldCoreRequirements := cfg.Story.CoreRequirements
	oldTitle := cfg.Story.Title

	cfg.Story.WritingStyle = analysis.WritingStyle
	cfg.Story.CharacterSetting = analysis.CharacterSetting
	cfg.Story.WorldSetting = analysis.WorldSetting
	cfg.Story.CoreRequirements = analysis.CoreRequirements
	cfg.Story.Title = analysis.Title

	if err := SaveProgress(progressPath, state); err != nil {
		cfg.Story.WritingStyle = oldWritingStyle
		cfg.Story.CharacterSetting = oldCharacterSetting
		cfg.Story.WorldSetting = oldWorldSetting
		cfg.Story.CoreRequirements = oldCoreRequirements
		cfg.Story.Title = oldTitle
		return fmt.Errorf("保存进度失败: %w", err)
	}

	if err := saveConfig(cfgPath, cfg); err != nil {
		cfg.Story.WritingStyle = oldWritingStyle
		cfg.Story.CharacterSetting = oldCharacterSetting
		cfg.Story.WorldSetting = oldWorldSetting
		cfg.Story.CoreRequirements = oldCoreRequirements
		cfg.Story.Title = oldTitle
		return fmt.Errorf("保存配置失败: %w", err)
	}

	return nil
}
