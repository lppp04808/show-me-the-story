package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSaveLoadProgressWithSplitSidecars(t *testing.T) {
	projectDir := t.TempDir()
	path := filepath.Join(projectDir, "progress.json")
	state := &Progress{
		Phase:               "writing",
		Title:               "Book",
		CorePrompt:          "cp",
		StorySynopsis:       "syn",
		CurrentChapterIndex: 1,
		Chapters: []ChapterState{
			{Num: 1, Title: "c1", Outline: "o1", Content: "body1", Summary: "s1", Status: StatusAccepted},
			{Num: 2, Title: "c2", Outline: "o2", Content: "", Summary: "", Status: StatusPending},
		},
		Foreshadows: []Foreshadow{{ID: 1, Name: "fs", Description: "d", PlantChapter: 1, TargetChapter: 2, Status: ForeshadowPlanted}},
		LastForeshadowOutlineReport: &ForeshadowOutlineReport{HasConflicts: true, Summary: "r"},
		LastOutlineCharacterReport:  &OutlineCharacterReport{HasSuggestions: true, Summary: "ocr"},
		MemoryEntries:               []MemoryEntry{{ID: 1, Chapter: 1, Content: "m"}},
		MemoryMaxTokens:             123,
		StageSummaries:              []StageSummary{{StartChapter: 1, EndChapter: 10, Summary: "arc"}},
	}

	if err := SaveProgress(path, state); err != nil {
		t.Fatalf("SaveProgress() err = %v", err)
	}

	mustExist := []string{
		filepath.Join(projectDir, "progress.json"),
		ChapterContentPath(projectDir, 1),
		ChapterMetaPath(projectDir, 1),
		ChapterMetaPath(projectDir, 2),
		foreshadowsPath(projectDir),
		memoryEntriesPath(projectDir),
		stageSummariesPath(projectDir),
		foreshadowOutlineReportPath(projectDir),
		outlineCharacterReportPath(projectDir),
	}
	for _, p := range mustExist {
		if _, err := os.Stat(p); err != nil {
			t.Fatalf("expected file %s to exist: %v", p, err)
		}
	}

	loaded, err := LoadProgress(path)
	if err != nil {
		t.Fatalf("LoadProgress() err = %v", err)
	}
	if loaded.Title != state.Title || loaded.CorePrompt != state.CorePrompt {
		t.Fatalf("loaded progress mismatch: %+v", loaded)
	}
	if got := loaded.Chapters[0].Content; got != "body1" {
		t.Fatalf("chapter content = %q, want body1", got)
	}
	if got := loaded.Chapters[0].Outline; got != "o1" {
		t.Fatalf("chapter outline = %q, want o1", got)
	}
	if len(loaded.Foreshadows) != 1 || loaded.Foreshadows[0].Name != "fs" {
		t.Fatalf("foreshadows not loaded: %+v", loaded.Foreshadows)
	}
	if loaded.LastOutlineCharacterReport == nil || loaded.LastOutlineCharacterReport.Summary != "ocr" {
		t.Fatalf("outline report not loaded: %+v", loaded.LastOutlineCharacterReport)
	}
	if len(loaded.MemoryEntries) != 1 || loaded.MemoryEntries[0].Content != "m" {
		t.Fatalf("memory entries not loaded: %+v", loaded.MemoryEntries)
	}
	if len(loaded.StageSummaries) != 1 || loaded.StageSummaries[0].Summary != "arc" {
		t.Fatalf("stage summaries not loaded: %+v", loaded.StageSummaries)
	}
}

func TestProgressWithoutContentKeepsMetadata(t *testing.T) {
	p := &Progress{
		Title: "Book",
		Chapters: []ChapterState{{Num: 1, Title: "c1", Outline: "o1", Content: "body", Summary: "s1", Status: StatusAccepted}},
		Foreshadows: []Foreshadow{{ID: 1, Name: "fs"}},
	}

	lite := ProgressWithoutContent(p)
	if lite == p {
		t.Fatal("expected clone, got same pointer")
	}
	if lite.Chapters[0].Content != "" {
		t.Fatalf("lite chapter content = %q, want empty", lite.Chapters[0].Content)
	}
	if lite.Chapters[0].Outline != "o1" || lite.Chapters[0].Summary != "s1" {
		t.Fatalf("lite chapter metadata lost: %+v", lite.Chapters[0])
	}
	if len(lite.Foreshadows) != 1 || lite.Foreshadows[0].Name != "fs" {
		t.Fatalf("lite foreshadows lost: %+v", lite.Foreshadows)
	}
}
