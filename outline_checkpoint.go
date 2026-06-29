package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	outlineCheckpointModeInitial      = "initial"
	outlineCheckpointModeContinuation = "continuation"
)

type OutlineCheckpoint struct {
	Mode                 string           `json:"mode"`
	Fingerprint          string           `json:"fingerprint"`
	Title                string           `json:"title,omitempty"`
	CorePrompt           string           `json:"core_prompt,omitempty"`
	StorySynopsis        string           `json:"story_synopsis,omitempty"`
	TotalChapters        int              `json:"total_chapters"`
	RequestedNewChapters int              `json:"requested_new_chapters,omitempty"`
	NextStartNum         int              `json:"next_start_num"`
	CurrentBatchSize     int              `json:"current_batch_size"`
	CompletedChapters    []OutlineChapter `json:"completed_chapters,omitempty"`
	UpdatedAt            string           `json:"updated_at,omitempty"`
}

type OutlineCheckpointInfo struct {
	Exists               bool   `json:"exists"`
	Mode                 string `json:"mode,omitempty"`
	CompletedCount       int    `json:"completed_count,omitempty"`
	TotalChapters        int    `json:"total_chapters,omitempty"`
	RequestedNewChapters int    `json:"requested_new_chapters,omitempty"`
	NextStartNum         int    `json:"next_start_num,omitempty"`
}

func OutlineCheckpointPath(progressPath string) string {
	return filepath.Join(filepath.Dir(progressPath), "outline_checkpoint.json")
}

func LoadOutlineCheckpoint(path string) (*OutlineCheckpoint, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("读取大纲断点失败: %w", err)
	}
	var cp OutlineCheckpoint
	if err := json.Unmarshal(data, &cp); err != nil {
		return nil, fmt.Errorf("解析大纲断点失败: %w", err)
	}
	return &cp, nil
}

func SaveOutlineCheckpoint(path string, cp *OutlineCheckpoint) error {
	cp.UpdatedAt = time.Now().Format(time.RFC3339)
	data, err := json.MarshalIndent(cp, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化大纲断点失败: %w", err)
	}
	if err := writeFileAtomic(path, data); err != nil {
		return fmt.Errorf("保存大纲断点失败: %w", err)
	}
	return nil
}

func DeleteOutlineCheckpoint(path string) error {
	if err := deleteFile(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func cloneOutlineChapters(chapters []OutlineChapter) []OutlineChapter {
	if len(chapters) == 0 {
		return nil
	}
	return append([]OutlineChapter(nil), chapters...)
}

func initialOutlineCheckpointInfo(cfg *Config, progressPath string, settings *ProjectSettings) *OutlineCheckpointInfo {
	cp, err := LoadOutlineCheckpoint(OutlineCheckpointPath(progressPath))
	if err != nil || cp == nil {
		return &OutlineCheckpointInfo{Exists: false}
	}
	if !ValidateOutlineCheckpoint(cp, cfg, nil, settings) || cp.Mode != outlineCheckpointModeInitial || len(cp.CompletedChapters) == 0 {
		return &OutlineCheckpointInfo{Exists: false}
	}
	return checkpointInfoFrom(cp)
}

func continuationOutlineCheckpointInfo(cfg *Config, state *Progress, progressPath string, settings *ProjectSettings) *OutlineCheckpointInfo {
	cp, err := LoadOutlineCheckpoint(OutlineCheckpointPath(progressPath))
	if err != nil || cp == nil {
		return &OutlineCheckpointInfo{Exists: false}
	}
	if cp.Mode != outlineCheckpointModeContinuation || len(cp.CompletedChapters) == 0 {
		return &OutlineCheckpointInfo{Exists: false}
	}
	if cp.Fingerprint != BuildContinuationOutlineFingerprint(cfg, state, settings, cp.RequestedNewChapters, "") {
		return &OutlineCheckpointInfo{Exists: false}
	}
	return checkpointInfoFrom(cp)
}

func BuildInitialOutlineFingerprint(cfg *Config, settings *ProjectSettings) string {
	payload := map[string]any{
		"story": map[string]any{
			"type":                     cfg.Story.Type,
			"chapter_count":            cfg.Story.ChapterCount,
			"target_words_per_chapter": cfg.Story.TargetWordsPerChapter,
			"writing_style":            cfg.Story.WritingStyle,
			"writing_pov":              cfg.Story.WritingPOV,
			"story_synopsis":           cfg.Story.StorySynopsis,
		},
		"prompts": map[string]any{
			"outline_generation":              cfg.Prompts.OutlineGeneration,
			"continuation_outline_generation": cfg.Prompts.ContinuationOutlineGeneration,
		},
		"characters": settingsFingerprintData(settings),
	}
	return hashPayload(payload)
}

func checkpointInfoFrom(cp *OutlineCheckpoint) *OutlineCheckpointInfo {
	if cp == nil {
		return &OutlineCheckpointInfo{Exists: false}
	}
	return &OutlineCheckpointInfo{
		Exists:               true,
		Mode:                 cp.Mode,
		CompletedCount:       len(cp.CompletedChapters),
		TotalChapters:        cp.TotalChapters,
		RequestedNewChapters: cp.RequestedNewChapters,
		NextStartNum:         cp.NextStartNum,
	}
}

func BuildContinuationOutlineFingerprint(cfg *Config, state *Progress, settings *ProjectSettings, requestedNewChapters int, userRequirements string) string {
	payload := map[string]any{
		"story": map[string]any{
			"type":           cfg.Story.Type,
			"writing_style":  cfg.Story.WritingStyle,
			"writing_pov":    cfg.Story.WritingPOV,
			"story_synopsis": cfg.Story.StorySynopsis,
		},
		"state": map[string]any{
			"phase":          state.Phase,
			"title":          state.Title,
			"core_prompt":    state.CorePrompt,
			"story_synopsis": state.StorySynopsis,
			"chapters":       continuationStateFingerprintData(state.Chapters),
		},
		"prompts": map[string]any{
			"continuation_outline_generation": cfg.Prompts.ContinuationOutlineGeneration,
		},
		"characters":             settingsFingerprintData(settings),
		"requested_new_chapters": requestedNewChapters,
		"user_requirements":      strings.TrimSpace(userRequirements),
	}
	return hashPayload(payload)
}

func ValidateOutlineCheckpoint(cp *OutlineCheckpoint, cfg *Config, state *Progress, settings *ProjectSettings) bool {
	if cp == nil {
		return false
	}
	switch cp.Mode {
	case outlineCheckpointModeInitial:
		if cp.TotalChapters != cfg.Story.ChapterCount || cp.NextStartNum < 1 {
			return false
		}
		return cp.Fingerprint == BuildInitialOutlineFingerprint(cfg, settings)
	case outlineCheckpointModeContinuation:
		if cp.RequestedNewChapters <= 0 || cp.NextStartNum < 1 {
			return false
		}
		return cp.Fingerprint == BuildContinuationOutlineFingerprint(cfg, state, settings, cp.RequestedNewChapters, "")
	default:
		return false
	}
}

func hashPayload(payload any) string {
	data, _ := json.Marshal(payload)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func settingsFingerprintData(settings *ProjectSettings) []map[string]string {
	if settings == nil || len(settings.Characters) == 0 {
		return nil
	}
	out := make([]map[string]string, 0, len(settings.Characters))
	for _, c := range settings.Characters {
		out = append(out, map[string]string{
			"name":        c.Name,
			"personality": c.Personality,
			"background":  c.Background,
		})
	}
	return out
}

func continuationStateFingerprintData(chapters []ChapterState) []map[string]any {
	out := make([]map[string]any, 0, len(chapters))
	for _, ch := range chapters {
		out = append(out, map[string]any{
			"num":     ch.Num,
			"title":   ch.Title,
			"outline": ch.Outline,
			"status":  ch.Status,
		})
	}
	return out
}
