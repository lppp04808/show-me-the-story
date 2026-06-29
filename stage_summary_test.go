package main

import (
	"strings"
	"testing"
)

func TestBuildHistorySummaryForLangIncludesStageSummary(t *testing.T) {
	chapters := make([]ChapterState, 25)
	for i := range chapters {
		chapters[i] = ChapterState{Num: i + 1, Summary: strings.Repeat("s", i+1)}
	}
	state := &Progress{
		Chapters:       chapters,
		StageSummaries: []StageSummary{{StartChapter: 1, EndChapter: 20, Summary: "stage-1"}},
	}
	got := buildHistorySummaryForLang(state, 24, LangZH)
	if got == "" {
		t.Fatal("buildHistorySummaryForLang() = empty")
	}
	if !containsAll(got, []string{"[第20章摘要]:", "stage-1"}) {
		t.Fatalf("buildHistorySummaryForLang() = %q, want recent summary and stage summary", got)
	}
}

func TestFormatExistingStageSummaries(t *testing.T) {
	got := formatExistingStageSummaries([]StageSummary{{StartChapter: 1, EndChapter: 20, Summary: "arc"}}, LangEN)
	if !containsAll(got, []string{"Stage Ch.1-20", "arc"}) {
		t.Fatalf("formatExistingStageSummaries() = %q", got)
	}
}

func containsAll(s string, subs []string) bool {
	for _, sub := range subs {
		if !strings.Contains(s, sub) {
			return false
		}
	}
	return true
}
