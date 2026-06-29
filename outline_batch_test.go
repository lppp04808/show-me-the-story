package main

import (
	"context"
	"fmt"
	"testing"
)

func TestGenerateOutlineUsesReducedBatchAfterMalformedFirstBatch(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Story.ChapterCount = 25
	cfg.Story.TargetWordsPerChapter = 1000
	logger := NewLogBroadcaster()

	origCall := outlineAPICall
	defer func() { outlineAPICall = origCall }()

	calls := []string{}
	outlineAPICall = func(ctx context.Context, apiCfg *APIConfig, systemPrompt, userPrompt string, logger *LogBroadcaster) string {
		calls = append(calls, userPrompt)
		switch len(calls) {
		case 1:
			return `{"title":"Book","core_prompt":"cp","story_synopsis":"syn","chapters":[{"num":1,"title":"c1","outline":"` + longOutline() + `"}]}`
		case 2:
			return batchResponseWithMeta("Book", "cp", "syn", 1, 10)
		case 3:
			return batchChaptersResponse(11, 10)
		case 4:
			return batchChaptersResponse(21, 5)
		default:
			panic(fmt.Sprintf("unexpected call %d", len(calls)))
		}
	}

	resp, err := generateOutline(context.Background(), &APIConfig{}, cfg, nil, logger, t.TempDir()+"/progress.json")
	if err != nil {
		t.Fatalf("generateOutline() error = %v", err)
	}
	if resp.Title != "Book" || resp.CorePrompt != "cp" || resp.StorySynopsis != "syn" {
		t.Fatalf("unexpected meta: %+v", resp)
	}
	if len(resp.Chapters) != 25 {
		t.Fatalf("len(resp.Chapters) = %d, want 25", len(resp.Chapters))
	}
	if resp.Chapters[0].Num != 1 || resp.Chapters[24].Num != 25 {
		t.Fatalf("chapter numbering mismatch: first=%d last=%d", resp.Chapters[0].Num, resp.Chapters[24].Num)
	}
}

func TestCreateManualOutlineAction(t *testing.T) {
	projectDir := t.TempDir()
	progressPath := projectDir + "/progress.json"
	cfgPath := projectDir + "/config.json"
	cfg := DefaultConfig()
	cfg.Language = LangZH
	cfg.Story.Title = "手动书名"
	cfg.Story.StorySynopsis = "手动简介"
	state := &Progress{Phase: "writing"}

	if err := CreateManualOutlineAction(cfg, state, progressPath, cfgPath, 3); err != nil {
		t.Fatalf("CreateManualOutlineAction() error = %v", err)
	}
	if len(state.Chapters) != 3 {
		t.Fatalf("len(state.Chapters) = %d, want 3", len(state.Chapters))
	}
	if state.Phase != "outline" {
		t.Fatalf("state.Phase = %q, want outline", state.Phase)
	}
	if state.Title != cfg.Story.Title || state.StorySynopsis != cfg.Story.StorySynopsis {
		t.Fatalf("state meta mismatch: %+v", state)
	}
	if state.Chapters[0].Title != "第1章" || state.Chapters[1].Status != StatusPending {
		t.Fatalf("unexpected manual scaffold: %+v", state.Chapters)
	}
	if state.StoryConfigSnapshot == nil || state.StoryConfigSnapshot.ChapterCount != 3 {
		t.Fatalf("snapshot not updated: %+v", state.StoryConfigSnapshot)
	}

	loaded, err := LoadProgress(progressPath)
	if err != nil {
		t.Fatalf("LoadProgress() error = %v", err)
	}
	if len(loaded.Chapters) != 3 || loaded.Chapters[2].Title != "第3章" {
		t.Fatalf("loaded scaffold mismatch: %+v", loaded.Chapters)
	}
	loadedCfg, err := LoadConfig(cfgPath)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	if loadedCfg.Story.ChapterCount != 3 {
		t.Fatalf("loaded config chapter_count = %d, want 3", loadedCfg.Story.ChapterCount)
	}
}

func TestAppendManualOutlineChaptersAction(t *testing.T) {
	projectDir := t.TempDir()
	progressPath := projectDir + "/progress.json"
	cfgPath := projectDir + "/config.json"
	cfg := DefaultConfig()
	cfg.Language = LangEN
	cfg.Story.ChapterCount = 2
	state := &Progress{
		Phase: "outline",
		Chapters: []ChapterState{{Num: 1, Title: "Chapter 1", Outline: "o1", Status: StatusAccepted}, {Num: 2, Title: "Chapter 2", Outline: "o2", Status: StatusPending}},
		StoryConfigSnapshot: &StoryConfig{ChapterCount: 2},
	}
	if err := saveConfig(cfgPath, cfg); err != nil {
		t.Fatalf("saveConfig() error = %v", err)
	}
	if err := SaveProgress(progressPath, state); err != nil {
		t.Fatalf("SaveProgress() error = %v", err)
	}

	if err := AppendManualOutlineChaptersAction(cfg, state, progressPath, cfgPath, 2); err != nil {
		t.Fatalf("AppendManualOutlineChaptersAction() error = %v", err)
	}
	if len(state.Chapters) != 4 {
		t.Fatalf("len(state.Chapters) = %d, want 4", len(state.Chapters))
	}
	if state.Chapters[2].Num != 3 || state.Chapters[2].Title != "Chapter 3" || state.Chapters[3].Num != 4 {
		t.Fatalf("unexpected appended chapters: %+v", state.Chapters[2:])
	}
	if cfg.Story.ChapterCount != 4 {
		t.Fatalf("cfg.Story.ChapterCount = %d, want 4", cfg.Story.ChapterCount)
	}
	if state.StoryConfigSnapshot == nil || state.StoryConfigSnapshot.ChapterCount != 4 {
		t.Fatalf("snapshot not updated: %+v", state.StoryConfigSnapshot)
	}
}

func TestAppendManualOutlineChaptersFromTextAction(t *testing.T) {
	projectDir := t.TempDir()
	progressPath := projectDir + "/progress.json"
	cfgPath := projectDir + "/config.json"
	cfg := DefaultConfig()
	cfg.Language = LangZH
	cfg.Story.ChapterCount = 1
	state := &Progress{
		Phase: "outline",
		Chapters: []ChapterState{{Num: 1, Title: "第1章", Outline: "已有", Status: StatusAccepted}},
		StoryConfigSnapshot: &StoryConfig{ChapterCount: 1},
	}
	if err := saveConfig(cfgPath, cfg); err != nil {
		t.Fatalf("saveConfig() error = %v", err)
	}
	if err := SaveProgress(progressPath, state); err != nil {
		t.Fatalf("SaveProgress() error = %v", err)
	}

	content := "第66章《返程结算》\n学校副本通关后，白光没有把众人送回公共区。\n\n第67章《新房门》\n十分钟后，幸存者陆续来到公共区域。"
	if err := AppendManualOutlineChaptersFromTextAction(cfg, state, progressPath, cfgPath, content); err != nil {
		t.Fatalf("AppendManualOutlineChaptersFromTextAction() error = %v", err)
	}
	if len(state.Chapters) != 3 {
		t.Fatalf("len(state.Chapters) = %d, want 3", len(state.Chapters))
	}
	if state.Chapters[1].Num != 2 || state.Chapters[1].Title != "返程结算" {
		t.Fatalf("unexpected chapter 2: %+v", state.Chapters[1])
	}
	if state.Chapters[2].Num != 3 || state.Chapters[2].Title != "新房门" {
		t.Fatalf("unexpected chapter 3: %+v", state.Chapters[2])
	}
	if state.Chapters[1].Outline == "" || state.Chapters[2].Outline == "" {
		t.Fatalf("parsed outlines should not be empty: %+v", state.Chapters[1:])
	}
}

func TestDeletePendingOutlineChapter(t *testing.T) {
	state := &Progress{
		CurrentChapterIndex: 2,
		Chapters: []ChapterState{
			{Num: 1, Title: "c1", Outline: "o1", Status: StatusAccepted},
			{Num: 2, Title: "c2", Outline: "o2", Status: StatusPending},
			{Num: 3, Title: "c3", Outline: "o3", Status: StatusPending},
		},
		Foreshadows: []Foreshadow{{ID: 1, PlantChapter: 2, TargetChapter: 3, Events: []ForeshadowEvent{{Chapter: 2, Note: "x"}, {Chapter: 3, Note: "y"}}}},
		MemoryEntries: []MemoryEntry{{ID: 1, Chapter: 3, Content: "m"}},
	}
	if err := DeletePendingOutlineChapter(state, 2); err != nil {
		t.Fatalf("DeletePendingOutlineChapter() error = %v", err)
	}
	if len(state.Chapters) != 2 {
		t.Fatalf("len(state.Chapters) = %d, want 2", len(state.Chapters))
	}
	if state.Chapters[1].Num != 2 || state.Chapters[1].Title != "c3" {
		t.Fatalf("unexpected remaining chapter: %+v", state.Chapters[1])
	}
	if state.CurrentChapterIndex != 1 {
		t.Fatalf("CurrentChapterIndex = %d, want 1", state.CurrentChapterIndex)
	}
	if state.Foreshadows[0].PlantChapter != 1 || state.Foreshadows[0].TargetChapter != 2 {
		t.Fatalf("foreshadow chapters not shifted: %+v", state.Foreshadows[0])
	}
	if len(state.Foreshadows[0].Events) != 1 || state.Foreshadows[0].Events[0].Chapter != 2 {
		t.Fatalf("foreshadow events not shifted: %+v", state.Foreshadows[0].Events)
	}
	if state.MemoryEntries[0].Chapter != 2 {
		t.Fatalf("memory chapter not shifted: %+v", state.MemoryEntries[0])
	}
}

func TestDeletePendingOutlineChapters(t *testing.T) {
	state := &Progress{
		CurrentChapterIndex: 3,
		Chapters: []ChapterState{
			{Num: 1, Title: "c1", Outline: "o1", Status: StatusAccepted},
			{Num: 2, Title: "c2", Outline: "o2", Status: StatusPending},
			{Num: 3, Title: "c3", Outline: "o3", Status: StatusPending},
			{Num: 4, Title: "c4", Outline: "o4", Status: StatusPending},
		},
		Foreshadows: []Foreshadow{{ID: 1, PlantChapter: 3, TargetChapter: 4, Events: []ForeshadowEvent{{Chapter: 2, Note: "a"}, {Chapter: 4, Note: "b"}}}},
		MemoryEntries: []MemoryEntry{{ID: 1, Chapter: 4, Content: "m"}},
	}
	deleted, err := DeletePendingOutlineChapters(state, []int{2, 4, 2})
	if err != nil {
		t.Fatalf("DeletePendingOutlineChapters() error = %v", err)
	}
	if deleted != 2 {
		t.Fatalf("deleted = %d, want 2", deleted)
	}
	if len(state.Chapters) != 2 || state.Chapters[1].Num != 2 || state.Chapters[1].Title != "c3" {
		t.Fatalf("unexpected chapters: %+v", state.Chapters)
	}
	if state.CurrentChapterIndex != 2 {
		t.Fatalf("CurrentChapterIndex = %d, want 2", state.CurrentChapterIndex)
	}
	if state.Foreshadows[0].PlantChapter != 2 || state.Foreshadows[0].TargetChapter != 2 {
		t.Fatalf("foreshadow chapters not shifted: %+v", state.Foreshadows[0])
	}
	if len(state.Foreshadows[0].Events) != 0 {
		t.Fatalf("foreshadow events not filtered: %+v", state.Foreshadows[0].Events)
	}
	if state.MemoryEntries[0].Chapter != 3 {
		t.Fatalf("memory chapter not shifted: %+v", state.MemoryEntries[0])
	}
}

func TestDeletePendingOutlineChaptersFrom(t *testing.T) {
	state := &Progress{Chapters: []ChapterState{{Num: 1, Status: StatusAccepted}, {Num: 2, Status: StatusPending}, {Num: 3, Status: StatusPending}}}
	deleted, err := DeletePendingOutlineChaptersFrom(state, 2)
	if err != nil {
		t.Fatalf("DeletePendingOutlineChaptersFrom() error = %v", err)
	}
	if deleted != 2 || len(state.Chapters) != 1 {
		t.Fatalf("unexpected delete result: deleted=%d chapters=%+v", deleted, state.Chapters)
	}
}

func batchResponseWithMeta(title, corePrompt, synopsis string, start, count int) string {
	return fmt.Sprintf(`{"title":%q,"core_prompt":%q,"story_synopsis":%q,"chapters":%s}`,
		title, corePrompt, synopsis, batchChaptersArray(start, count))
}

func batchChaptersResponse(start, count int) string {
	return fmt.Sprintf(`{"chapters":%s}`, batchChaptersArray(start, count))
}

func batchChaptersArray(start, count int) string {
	out := "["
	for i := 0; i < count; i++ {
		if i > 0 {
			out += ","
		}
		num := start + i
		out += fmt.Sprintf(`{"num":%d,"title":"C%d","outline":%q}`, num, num, longOutline())
	}
	out += "]"
	return out
}

func longOutline() string {
	return "这是一个足够长的大纲段落，用于满足最小长度要求。这里包含场景、冲突、转折、人物和钩子。主角在雨夜进入陌生城镇，先被迫面对眼前的现实困局，再在盟友与敌人的夹击下做出阶段性选择，过程中暴露新的秘密并埋下后续章节的悬念，同时明确章末去向与情绪张力。"
}
