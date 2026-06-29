package main

import "testing"

func TestInitialOutlineCheckpointInfo(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Story.ChapterCount = 30
	path := t.TempDir() + "/progress.json"
	cpPath := OutlineCheckpointPath(path)
	cp := &OutlineCheckpoint{
		Mode:              outlineCheckpointModeInitial,
		Fingerprint:       BuildInitialOutlineFingerprint(cfg, nil),
		Title:             "Book",
		CorePrompt:        "cp",
		StorySynopsis:     "syn",
		TotalChapters:     30,
		NextStartNum:      11,
		CurrentBatchSize:  10,
		CompletedChapters: []OutlineChapter{{Num: 1, Title: "A", Outline: "B"}},
	}
	if err := SaveOutlineCheckpoint(cpPath, cp); err != nil {
		t.Fatalf("SaveOutlineCheckpoint() error = %v", err)
	}
	info := initialOutlineCheckpointInfo(cfg, path, nil)
	if !info.Exists || info.NextStartNum != 11 {
		t.Fatalf("initialOutlineCheckpointInfo() = %+v, want checkpoint", info)
	}
}

func TestContinuationOutlineCheckpointInfo(t *testing.T) {
	cfg := DefaultConfig()
	state := &Progress{Phase: "outline", Title: "Book", CorePrompt: "cp", StorySynopsis: "syn", Chapters: []ChapterState{{Num: 1, Title: "A", Outline: "B", Status: StatusAccepted}}}
	path := t.TempDir() + "/progress.json"
	cpPath := OutlineCheckpointPath(path)
	cp := &OutlineCheckpoint{
		Mode:                 outlineCheckpointModeContinuation,
		Fingerprint:          BuildContinuationOutlineFingerprint(cfg, state, nil, 5, ""),
		Title:                "Book",
		CorePrompt:           "cp",
		StorySynopsis:        "syn",
		TotalChapters:        6,
		RequestedNewChapters: 5,
		NextStartNum:         3,
		CurrentBatchSize:     10,
		CompletedChapters:    []OutlineChapter{{Num: 2, Title: "C", Outline: "D"}},
	}
	if err := SaveOutlineCheckpoint(cpPath, cp); err != nil {
		t.Fatalf("SaveOutlineCheckpoint() error = %v", err)
	}
	info := continuationOutlineCheckpointInfo(cfg, state, path, nil)
	if !info.Exists || info.NextStartNum != 3 {
		t.Fatalf("continuationOutlineCheckpointInfo() = %+v, want checkpoint", info)
	}
}
