package main

import "testing"

func TestCalcOutlineLengthRange(t *testing.T) {
	tests := []struct {
		perChapter int
		wantMin    int
		wantMax    int
	}{
		{2500, 125, 312},
		{1000, 80, 150},
		{0, 125, 312},
	}
	for _, tt := range tests {
		minLen, maxLen := calcOutlineLengthRange(tt.perChapter)
		if minLen != tt.wantMin || maxLen != tt.wantMax {
			t.Errorf("calcOutlineLengthRange(%d) = (%d,%d), want (%d,%d)",
				tt.perChapter, minLen, maxLen, tt.wantMin, tt.wantMax)
		}
	}
}

func TestExtractFirstAppearanceStubs(t *testing.T) {
	outline := "张三与李四在码头会面。王五（首次登场）是码头管事，告知密信下落。"
	stubs := extractFirstAppearanceStubs(outline)
	if len(stubs) != 1 || stubs[0].Name != "王五" {
		t.Fatalf("extractFirstAppearanceStubs() = %+v, want 王五", stubs)
	}
}

func TestValidateOutlineChapterLengths(t *testing.T) {
	chapters := []OutlineChapter{
		{Num: 1, Outline: stringsRepeat("情节", 50)},
		{Num: 2, Outline: "太短"},
	}
	short := validateOutlineChapterLengths(chapters, 80)
	if len(short) != 1 || short[0] != 2 {
		t.Fatalf("validateOutlineChapterLengths() = %v, want [2]", short)
	}
}

func TestPlanOutlineBatchSizes(t *testing.T) {
	got := planOutlineBatchSizes(45, outlineBatchSizeDefault)
	want := []int{20, 20, 5}
	if len(got) != len(want) {
		t.Fatalf("planOutlineBatchSizes() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("planOutlineBatchSizes() = %v, want %v", got, want)
		}
	}
}

func TestValidateOutlineBatch(t *testing.T) {
	valid := []OutlineChapter{
		{Num: 11, Title: "A", Outline: "outline-a"},
		{Num: 12, Title: "B", Outline: "outline-b"},
	}
	if err := validateOutlineBatch(valid, 11, 2); err != nil {
		t.Fatalf("validateOutlineBatch(valid) error = %v", err)
	}

	cases := []struct {
		name     string
		chapters []OutlineChapter
	}{
		{"missing", valid[:1]},
		{"wrong-num", []OutlineChapter{{Num: 11, Title: "A", Outline: "outline-a"}, {Num: 13, Title: "B", Outline: "outline-b"}}},
		{"empty-title", []OutlineChapter{{Num: 11, Title: "", Outline: "outline-a"}, {Num: 12, Title: "B", Outline: "outline-b"}}},
		{"empty-outline", []OutlineChapter{{Num: 11, Title: "A", Outline: ""}, {Num: 12, Title: "B", Outline: "outline-b"}}},
	}
	for _, tc := range cases {
		if err := validateOutlineBatch(tc.chapters, 11, 2); err == nil {
			t.Fatalf("validateOutlineBatch(%s) error = nil, want non-nil", tc.name)
		}
	}
}

func TestBuildOutlineBatchHint(t *testing.T) {
	hint := buildOutlineBatchHint(21, 20, 300, LangZH)
	if hint == "" {
		t.Fatal("buildOutlineBatchHint() = empty, want content")
	}
}

func TestDeletePendingOutlineChaptersRejectsEmptySelection(t *testing.T) {
	state := &Progress{Chapters: []ChapterState{{Num: 1, Status: StatusPending}}}
	if _, err := DeletePendingOutlineChapters(state, nil); err != ErrOutlineNoChaptersSelected {
		t.Fatalf("DeletePendingOutlineChapters() error = %v, want %v", err, ErrOutlineNoChaptersSelected)
	}
}

func TestDeletePendingOutlineChaptersFromRejectsNonPendingTail(t *testing.T) {
	state := &Progress{Chapters: []ChapterState{{Num: 1, Status: StatusAccepted}, {Num: 2, Status: StatusPending}, {Num: 3, Status: StatusAccepted}}}
	if _, err := DeletePendingOutlineChaptersFrom(state, 2); err != ErrOutlineDeleteRangeNotPending {
		t.Fatalf("DeletePendingOutlineChaptersFrom() error = %v, want %v", err, ErrOutlineDeleteRangeNotPending)
	}
}

func TestDeletePendingOutlineChapterRejectsNonPending(t *testing.T) {
	state := &Progress{Chapters: []ChapterState{{Num: 1, Status: StatusAccepted}}}
	if err := DeletePendingOutlineChapter(state, 1); err != ErrOutlineChapterNotPending {
		t.Fatalf("DeletePendingOutlineChapter() error = %v, want %v", err, ErrOutlineChapterNotPending)
	}
}

func TestExtractManualOutlineTitle(t *testing.T) {
	if got := extractManualOutlineTitle("第66章《返程结算》", 66, LangZH); got != "返程结算" {
		t.Fatalf("extractManualOutlineTitle zh = %q, want 返程结算", got)
	}
	if got := extractManualOutlineTitle("Chapter 67: The New Door", 67, LangEN); got != "The New Door" {
		t.Fatalf("extractManualOutlineTitle en = %q, want The New Door", got)
	}
}

func TestFirstIncompleteOutlineChapter(t *testing.T) {
	state := &Progress{Chapters: []ChapterState{
		{Num: 1, Title: "第一章", Outline: "已完成"},
		{Num: 2, Title: "", Outline: "缺标题"},
		{Num: 3, Title: "第三章", Outline: ""},
	}}
	if got := firstIncompleteOutlineChapter(state); got != 2 {
		t.Fatalf("firstIncompleteOutlineChapter() = %d, want 2", got)
	}

	state.Chapters[1].Title = "第二章"
	state.Chapters[2].Outline = "已完成"
	if got := firstIncompleteOutlineChapter(state); got != 0 {
		t.Fatalf("firstIncompleteOutlineChapter() = %d, want 0", got)
	}
}

func stringsRepeat(s string, n int) string {
	out := ""
	for i := 0; i < n; i++ {
		out += s
	}
	return out
}
