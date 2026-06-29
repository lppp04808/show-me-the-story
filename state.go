package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type ChapterState struct {
	Num                   int    `json:"num"`
	Title                 string `json:"title"`
	Outline               string `json:"outline"`
	Content               string `json:"content"`
	Summary               string `json:"summary"`
	Status                string `json:"status"` // pending | writing | review | accepted
	SelectedForeshadowIDs []int  `json:"selected_foreshadow_ids,omitempty"`
}

type ForeshadowStatus string

const (
	ForeshadowPlanted     ForeshadowStatus = "planted"
	ForeshadowProgressing ForeshadowStatus = "progressing"
	ForeshadowResolved    ForeshadowStatus = "resolved"
	ForeshadowAbandoned   ForeshadowStatus = "abandoned"
)

type ForeshadowEvent struct {
	Chapter int    `json:"chapter"`
	Note    string `json:"note"`
}

type Foreshadow struct {
	ID            int               `json:"id"`
	Name          string            `json:"name"`
	Description   string            `json:"description"`
	PlantChapter  int               `json:"plant_chapter"`
	TargetChapter int               `json:"target_chapter"`
	Status        ForeshadowStatus  `json:"status"`
	Events        []ForeshadowEvent `json:"events"`
	Resolution    string            `json:"resolution"`
}

type ForeshadowOutlineConflict struct {
	ForeshadowID   int    `json:"foreshadow_id"`
	ForeshadowName string `json:"foreshadow_name"`
	ConflictType   string `json:"conflict_type"`
	Description    string `json:"description"`
	SuggestedFix   string `json:"suggested_fix"`
}

type ForeshadowOutlineReport struct {
	HasConflicts bool                        `json:"has_conflicts"`
	Conflicts    []ForeshadowOutlineConflict `json:"conflicts"`
	Summary      string                      `json:"summary"`
}

type ConflictActionOption struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Description string `json:"description"`
}

type WritingConflict struct {
	ChapterIndex     int                    `json:"chapter_index"`
	ChapterNum       int                    `json:"chapter_num"`
	ChapterTitle     string                 `json:"chapter_title"`
	Issues           []string               `json:"issues"`
	Summary          string                 `json:"summary"`
	RootCause        string                 `json:"root_cause"`
	Reconcilable     bool                   `json:"reconcilable"`
	SuggestedActions []ConflictActionOption `json:"suggested_actions"`
}

type MemoryEntry struct {
	ID       int    `json:"id"`
	Content  string `json:"content"`
	Category string `json:"category"` // character | location | item | event | promise | other
	Chapter  int    `json:"chapter"`
	Position int    `json:"position"`
}

type StageSummary struct {
	StartChapter int    `json:"start_chapter"`
	EndChapter   int    `json:"end_chapter"`
	Summary      string `json:"summary"`
	SourceHash   string `json:"source_hash,omitempty"`
}

type Progress struct {
	Phase                       string                   `json:"phase"`
	Title                       string                   `json:"title"`
	CorePrompt                  string                   `json:"core_prompt"`
	StorySynopsis               string                   `json:"story_synopsis"`
	Chapters                    []ChapterState           `json:"chapters"`
	CurrentChapterIndex         int                      `json:"current_chapter_index"`
	StoryConfigSnapshot         *StoryConfig             `json:"story_config_snapshot,omitempty"`
	Foreshadows                 []Foreshadow             `json:"foreshadows,omitempty"`
	LastForeshadowOutlineReport *ForeshadowOutlineReport `json:"last_foreshadow_outline_report,omitempty"`
	LastOutlineCharacterReport  *OutlineCharacterReport  `json:"last_outline_character_report,omitempty"`
	PendingWritingConflict      *WritingConflict         `json:"pending_writing_conflict,omitempty"`
	MemoryEntries               []MemoryEntry            `json:"memory_entries,omitempty"`
	MemoryMaxTokens             int                      `json:"memory_max_tokens,omitempty"`
	StageSummaries              []StageSummary           `json:"stage_summaries,omitempty"`
}

func ProgressWithoutContent(p *Progress) *Progress {
	if p == nil {
		return nil
	}
	clone := *p
	if len(p.Chapters) == 0 {
		return &clone
	}
	clone.Chapters = make([]ChapterState, len(p.Chapters))
	for i, ch := range p.Chapters {
		clone.Chapters[i] = ch
		clone.Chapters[i].Content = ""
	}
	return &clone
}

type storedChapterState struct {
	Num                   int    `json:"num"`
	Title                 string `json:"title,omitempty"`
	Outline               string `json:"outline,omitempty"`
	Content               string `json:"content,omitempty"`
	ContentPath           string `json:"content_path,omitempty"`
	Summary               string `json:"summary,omitempty"`
	Status                string `json:"status,omitempty"`
	SelectedForeshadowIDs []int  `json:"selected_foreshadow_ids,omitempty"`
}

type storedProgress struct {
	StorageVersion              int                      `json:"storage_version,omitempty"`
	Phase                       string                   `json:"phase"`
	Title                       string                   `json:"title"`
	CorePrompt                  string                   `json:"core_prompt"`
	StorySynopsis               string                   `json:"story_synopsis"`
	Chapters                    []storedChapterState     `json:"chapters"`
	CurrentChapterIndex         int                      `json:"current_chapter_index"`
	StoryConfigSnapshot         *StoryConfig             `json:"story_config_snapshot,omitempty"`
	Foreshadows                 []Foreshadow             `json:"foreshadows,omitempty"`
	LastForeshadowOutlineReport *ForeshadowOutlineReport `json:"last_foreshadow_outline_report,omitempty"`
	LastOutlineCharacterReport  *OutlineCharacterReport  `json:"last_outline_character_report,omitempty"`
	PendingWritingConflict      *WritingConflict         `json:"pending_writing_conflict,omitempty"`
	MemoryEntries               []MemoryEntry            `json:"memory_entries,omitempty"`
	MemoryMaxTokens             int                      `json:"memory_max_tokens,omitempty"`
	StageSummaries              []StageSummary           `json:"stage_summaries,omitempty"`
}

const (
	StatusPending  = "pending"
	StatusWriting  = "writing"
	StatusReview   = "review"
	StatusAccepted = "accepted"

	progressStorageVersion = 3
	chapterContentDirName  = "chapters"
)

func LoadProgress(path string) (*Progress, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("读取进度文件失败: %w", err)
	}

	var stored storedProgress
	if err := json.Unmarshal(data, &stored); err != nil {
		return nil, fmt.Errorf("解析进度文件失败: %w", err)
	}

	projectDir := filepath.Dir(path)
	p := &Progress{
		Phase:                  stored.Phase,
		Title:                  stored.Title,
		CorePrompt:             stored.CorePrompt,
		StorySynopsis:          stored.StorySynopsis,
		CurrentChapterIndex:    stored.CurrentChapterIndex,
		StoryConfigSnapshot:    stored.StoryConfigSnapshot,
		PendingWritingConflict: stored.PendingWritingConflict,
		MemoryMaxTokens:        stored.MemoryMaxTokens,
	}
	if err := loadProgressSidecars(projectDir, p, &stored); err != nil {
		return nil, err
	}

	p.Chapters = make([]ChapterState, len(stored.Chapters))
	for i, ch := range stored.Chapters {
		meta, err := loadStoredChapter(projectDir, ch)
		if err != nil {
			return nil, err
		}
		content := meta.Content
		if meta.ContentPath != "" {
			body, err := os.ReadFile(resolveStoredChapterContentPath(projectDir, meta.ContentPath))
			if err != nil {
				return nil, fmt.Errorf("读取第 %d 章正文失败: %w", meta.Num, err)
			}
			content = string(body)
		}
		p.Chapters[i] = ChapterState{
			Num:                   meta.Num,
			Title:                 meta.Title,
			Outline:               meta.Outline,
			Content:               content,
			Summary:               meta.Summary,
			Status:                meta.Status,
			SelectedForeshadowIDs: meta.SelectedForeshadowIDs,
		}
	}

	return p, nil
}

func SaveProgress(path string, p *Progress) error {
	projectDir := filepath.Dir(path)
	if err := os.MkdirAll(chapterContentDir(projectDir), 0755); err != nil {
		return fmt.Errorf("创建章节存储目录失败: %w", err)
	}

	stored := storedProgress{
		StorageVersion:         progressStorageVersion,
		Phase:                  p.Phase,
		Title:                  p.Title,
		CorePrompt:             p.CorePrompt,
		StorySynopsis:          p.StorySynopsis,
		CurrentChapterIndex:    p.CurrentChapterIndex,
		StoryConfigSnapshot:    p.StoryConfigSnapshot,
		PendingWritingConflict: p.PendingWritingConflict,
		MemoryMaxTokens:        p.MemoryMaxTokens,
		Chapters:               make([]storedChapterState, len(p.Chapters)),
	}

	keepFiles := make(map[string]struct{}, len(p.Chapters)*2)
	for i, ch := range p.Chapters {
		contentPath := ""
		if ch.Content != "" {
			relPath := chapterContentRelativePath(ch.Num)
			if err := writeFileAtomic(resolveStoredChapterContentPath(projectDir, relPath), []byte(ch.Content)); err != nil {
				return fmt.Errorf("保存第 %d 章正文失败: %w", ch.Num, err)
			}
			contentPath = relPath
			keepFiles[filepath.Base(relPath)] = struct{}{}
		}
		stored.Chapters[i] = storedChapterState{Num: ch.Num, ContentPath: contentPath}

		meta := storedChapterState{
			Num:                   ch.Num,
			Title:                 ch.Title,
			Outline:               ch.Outline,
			Summary:               ch.Summary,
			Status:                ch.Status,
			ContentPath:           contentPath,
			SelectedForeshadowIDs: ch.SelectedForeshadowIDs,
		}
		metaPath := ChapterMetaPath(projectDir, ch.Num)
		if err := saveJSONSidecar(metaPath, meta); err != nil {
			return fmt.Errorf("保存第 %d 章元数据失败: %w", ch.Num, err)
		}
		keepFiles[filepath.Base(metaPath)] = struct{}{}
	}

	if err := cleanupChapterStorageFiles(projectDir, keepFiles); err != nil {
		return fmt.Errorf("清理章节存储目录失败: %w", err)
	}
	if err := saveProgressSidecars(projectDir, p); err != nil {
		return err
	}

	data, err := json.MarshalIndent(stored, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化进度失败: %w", err)
	}
	if err := writeFileAtomic(path, data); err != nil {
		return fmt.Errorf("保存进度文件失败: %w", err)
	}
	return nil
}

func loadStoredChapter(projectDir string, fallback storedChapterState) (storedChapterState, error) {
	metaPath := ChapterMetaPath(projectDir, fallback.Num)
	var meta storedChapterState
	if err := loadJSONSidecar(metaPath, &meta); err == nil {
		if meta.Num == 0 {
			meta.Num = fallback.Num
		}
		return meta, nil
	} else if !os.IsNotExist(err) {
		return storedChapterState{}, fmt.Errorf("读取第 %d 章元数据失败: %w", fallback.Num, err)
	}
	return fallback, nil
}

func loadProgressSidecars(projectDir string, p *Progress, stored *storedProgress) error {
	p.Foreshadows = stored.Foreshadows
	if err := loadJSONSidecar(foreshadowsPath(projectDir), &p.Foreshadows); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("读取伏笔数据失败: %w", err)
	}

	p.MemoryEntries = stored.MemoryEntries
	if err := loadJSONSidecar(memoryEntriesPath(projectDir), &p.MemoryEntries); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("读取记忆数据失败: %w", err)
	}

	p.StageSummaries = stored.StageSummaries
	if err := loadJSONSidecar(stageSummariesPath(projectDir), &p.StageSummaries); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("读取阶段摘要失败: %w", err)
	}

	p.LastForeshadowOutlineReport = stored.LastForeshadowOutlineReport
	if err := loadJSONSidecar(foreshadowOutlineReportPath(projectDir), &p.LastForeshadowOutlineReport); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("读取伏笔一致性报告失败: %w", err)
	}

	p.LastOutlineCharacterReport = stored.LastOutlineCharacterReport
	if err := loadJSONSidecar(outlineCharacterReportPath(projectDir), &p.LastOutlineCharacterReport); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("读取大纲人物报告失败: %w", err)
	}

	return nil
}

func saveProgressSidecars(projectDir string, p *Progress) error {
	if err := saveOptionalJSONSidecar(foreshadowsPath(projectDir), p.Foreshadows, len(p.Foreshadows) > 0); err != nil {
		return fmt.Errorf("保存伏笔数据失败: %w", err)
	}
	if err := saveOptionalJSONSidecar(memoryEntriesPath(projectDir), p.MemoryEntries, len(p.MemoryEntries) > 0); err != nil {
		return fmt.Errorf("保存记忆数据失败: %w", err)
	}
	if err := saveOptionalJSONSidecar(stageSummariesPath(projectDir), p.StageSummaries, len(p.StageSummaries) > 0); err != nil {
		return fmt.Errorf("保存阶段摘要失败: %w", err)
	}
	if err := saveOptionalJSONSidecar(foreshadowOutlineReportPath(projectDir), p.LastForeshadowOutlineReport, p.LastForeshadowOutlineReport != nil); err != nil {
		return fmt.Errorf("保存伏笔一致性报告失败: %w", err)
	}
	if err := saveOptionalJSONSidecar(outlineCharacterReportPath(projectDir), p.LastOutlineCharacterReport, p.LastOutlineCharacterReport != nil); err != nil {
		return fmt.Errorf("保存大纲人物报告失败: %w", err)
	}
	return nil
}

func chapterContentDir(projectDir string) string {
	return filepath.Join(projectDir, chapterContentDirName)
}

func chapterContentRelativePath(num int) string {
	return filepath.ToSlash(filepath.Join(chapterContentDirName, fmt.Sprintf("%04d.md", num)))
}

func chapterMetaRelativePath(num int) string {
	return filepath.ToSlash(filepath.Join(chapterContentDirName, fmt.Sprintf("%04d.json", num)))
}

func resolveStoredChapterContentPath(projectDir, relPath string) string {
	return filepath.Join(projectDir, filepath.FromSlash(relPath))
}

func ChapterContentPath(projectDir string, num int) string {
	return resolveStoredChapterContentPath(projectDir, chapterContentRelativePath(num))
}

func ChapterMetaPath(projectDir string, num int) string {
	return resolveStoredChapterContentPath(projectDir, chapterMetaRelativePath(num))
}

func DeleteChapterContentDir(projectDir string) error {
	return os.RemoveAll(chapterContentDir(projectDir))
}

func DeleteProgressSidecars(projectDir string) error {
	for _, path := range []string{
		foreshadowsPath(projectDir),
		memoryEntriesPath(projectDir),
		stageSummariesPath(projectDir),
		foreshadowOutlineReportPath(projectDir),
		outlineCharacterReportPath(projectDir),
	} {
		if err := deleteFile(path); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

func cleanupChapterStorageFiles(projectDir string, keepFiles map[string]struct{}) error {
	dir := chapterContentDir(projectDir)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		switch filepath.Ext(name) {
		case ".md", ".json":
		default:
			continue
		}
		if _, ok := keepFiles[name]; ok {
			continue
		}
		if err := deleteFile(filepath.Join(dir, name)); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

func saveJSONSidecar(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(path, data)
}

func saveOptionalJSONSidecar(path string, value any, keep bool) error {
	if !keep {
		if err := deleteFile(path); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	return saveJSONSidecar(path, value)
}

func loadJSONSidecar(path string, target any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, target)
}

func foreshadowsPath(projectDir string) string {
	return filepath.Join(projectDir, "foreshadows.json")
}

func memoryEntriesPath(projectDir string) string {
	return filepath.Join(projectDir, "memory_entries.json")
}

func stageSummariesPath(projectDir string) string {
	return filepath.Join(projectDir, "stage_summaries.json")
}

func foreshadowOutlineReportPath(projectDir string) string {
	return filepath.Join(projectDir, "foreshadow_outline_report.json")
}

func outlineCharacterReportPath(projectDir string) string {
	return filepath.Join(projectDir, "outline_character_report.json")
}

// ChapterMarkdownPath returns the markdown file path for a chapter inside the project directory.
func ChapterMarkdownPath(projectDir string, num int) string {
	return filepath.Join(projectDir, fmt.Sprintf("Chapter_%02d.md", num))
}

func SaveChapterMarkdown(projectDir string, ch ChapterState, title string) {
	return
	// content := fmt.Sprintf("# 第 %d 章: %s\n\n> **本章摘要**：%s\n\n---\n\n%s", ch.Num, ch.Title, ch.Summary, ch.Content)
	// _ = os.WriteFile(ChapterMarkdownPath(projectDir, ch.Num), []byte(content), 0644)
}

// ForeshadowRoadmapPath returns the foreshadow roadmap markdown path inside the project directory.
func ForeshadowRoadmapPath(projectDir string) string {
	return filepath.Join(projectDir, "Foreshadows.md")
}
