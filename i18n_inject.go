package main

import (
	"fmt"
	"strings"
)

// Helper functions that produce language-specific text fragments injected
// into prompt templates. These are NOT the prompt templates themselves
// (those live in prompts.go / prompts_en.go) — these are the runtime
// context blocks built from project state.

// buildOutlineConstraintsForLang returns the "全书章节脉络" reverse-constraint block
// in the requested language.
func buildOutlineConstraintsForLang(state *Progress, idx int, lang string) string {
	var past, future strings.Builder
	for i := 0; i < idx && i < len(state.Chapters); i++ {
		ch := state.Chapters[i]
		if strings.TrimSpace(ch.Outline) == "" {
			continue
		}
		past.WriteString(formatChapterLine(ch.Num, ch.Title, ch.Outline, lang))
	}
	end := idx + 1 + futureOutlineWindow
	if end > len(state.Chapters) {
		end = len(state.Chapters)
	}
	for i := idx + 1; i < end; i++ {
		ch := state.Chapters[i]
		if strings.TrimSpace(ch.Outline) == "" {
			continue
		}
		future.WriteString(formatChapterLine(ch.Num, ch.Title, ch.Outline, lang))
	}
	if past.Len() == 0 && future.Len() == 0 {
		return ""
	}
	var sb strings.Builder
	if NormalizeLanguage(lang) == LangEN {
		sb.WriteString("[Full-novel chapter arc (reverse constraint, must be obeyed strictly)]\n")
		if future.Len() > 0 {
			sb.WriteString("- Upcoming chapters — the following character debuts, first meetings, identity reveals etc. are already assigned to specific later chapters. This chapter MUST NOT make them happen early, nor hint at or spoil them:\n")
			sb.WriteString(future.String())
		}
		if past.Len() > 0 {
			sb.WriteString("- Already happened — the events below have already occurred. This chapter must not re-enact them as new events (especially one-time events like first meetings or identity reveals — only continue them as established facts):\n")
			sb.WriteString(past.String())
		}
	} else {
		sb.WriteString("【全书章节脉络（反向约束，必须严格遵守）】\n")
		if future.Len() > 0 {
			sb.WriteString("◆ 后续章节安排——以下人物登场、初遇、身份揭示等事件已安排在对应章节，本章严禁提前发生，也不得以任何形式暗示或剧透：\n")
			sb.WriteString(future.String())
		}
		if past.Len() > 0 {
			sb.WriteString("◆ 前文已发生——以下事件已经发生，本章不得将其作为新事件重复发生（尤其是初次见面、身份揭示等一次性事件，只能作为既成事实延续）：\n")
			sb.WriteString(past.String())
		}
	}
	sb.WriteString("\n")
	return sb.String()
}

func formatChapterLine(num int, title, outline, lang string) string {
	if NormalizeLanguage(lang) == LangEN {
		return fmt.Sprintf("Chapter %d \"%s\": %s\n", num, title, outline)
	}
	return fmt.Sprintf("第%d章《%s》：%s\n", num, title, outline)
}

func buildPreviousChapterTailForLang(state *Progress, idx int, lang string) string {
	if idx <= 0 || idx >= len(state.Chapters) {
		return ""
	}
	prev := state.Chapters[idx-1]
	if prev.Content == "" {
		return ""
	}
	tail := tailAtParagraph(prev.Content, prevTailMaxRunes)
	if tail == "" {
		return ""
	}
	if NormalizeLanguage(lang) == LangEN {
		return fmt.Sprintf("[Previous chapter ending (for seamless scene/mood continuation only — do NOT recap or rewrite)]\n%s\n\n", tail)
	}
	return fmt.Sprintf("【上一章结尾原文（仅供无缝承接场景与情绪，禁止复述或改写）】\n%s\n\n", tail)
}

func buildHistorySummaryForLang(state *Progress, idx int, lang string) string {
	history, _, _ := buildHistoryContextForLang(state, idx, lang)
	return history
}

func buildHistoryContextForLang(state *Progress, idx int, lang string) (string, int, int) {
	startIdx := 0
	if idx > 5 {
		startIdx = idx - 5
	}
	var history strings.Builder
	recentCount := 0
	for i := startIdx; i < idx; i++ {
		if state.Chapters[i].Summary != "" {
			if NormalizeLanguage(lang) == LangEN {
				history.WriteString(fmt.Sprintf("[Chapter %d summary]: %s\n", state.Chapters[i].Num, state.Chapters[i].Summary))
			} else {
				history.WriteString(fmt.Sprintf("[第%d章摘要]: %s\n", state.Chapters[i].Num, state.Chapters[i].Summary))
			}
			recentCount++
		}
	}
	stageBlock, stageCount := buildStageSummaryContextForLang(state, idx, lang)
	if stageBlock != "" {
		history.WriteString("\n")
		history.WriteString(stageBlock)
	}
	if history.Len() == 0 {
		if NormalizeLanguage(lang) == LangEN {
			return "This is the opening of the story; no prior context.", 0, 0
		}
		return "当前为故事开端，无历史前情。", 0, 0
	}
	return history.String(), recentCount, stageCount
}

func buildStageSummaryContextForLang(state *Progress, idx int, lang string) (string, int) {
	if len(state.StageSummaries) == 0 || idx <= 0 {
		return "", 0
	}
	currentChapterNum := state.Chapters[idx].Num
	var sb strings.Builder
	added := 0
	for i := len(state.StageSummaries) - 1; i >= 0; i-- {
		ss := state.StageSummaries[i]
		if ss.EndChapter >= currentChapterNum {
			continue
		}
		if NormalizeLanguage(lang) == LangEN {
			sb.WriteString(fmt.Sprintf("[Stage summary Ch.%d-%d]: %s\n", ss.StartChapter, ss.EndChapter, ss.Summary))
		} else {
			sb.WriteString(fmt.Sprintf("[阶段摘要 第%d-%d章]: %s\n", ss.StartChapter, ss.EndChapter, ss.Summary))
		}
		added++
		if added >= 2 {
			break
		}
	}
	if sb.Len() == 0 {
		return "", 0
	}
	if NormalizeLanguage(lang) == LangEN {
		return "[Stage context — medium-range story progression]\n" + sb.String(), added
	}
	return "【阶段上下文——中程剧情推进】\n" + sb.String(), added
}

// buildCharacterContextForLang returns structured character details injected into writing prompts.
func buildCharacterContextForLang(settings *ProjectSettings, chapterOutline, lang string) string {
	var sb strings.Builder

	if settings != nil && len(settings.Characters) > 0 {
		relevant := selectRelevantCharacters(settings, chapterOutline, 6)
		en := NormalizeLanguage(lang) == LangEN
		for _, c := range relevant {
			sb.WriteString(fmt.Sprintf("【%s】", c.Name))
			if c.Age != "" {
				if en {
					sb.WriteString(fmt.Sprintf(" Age: %s", c.Age))
				} else {
					sb.WriteString(fmt.Sprintf(" 年龄:%s", c.Age))
				}
			}
			sb.WriteString("\n")
			write := func(label, val string) {
				if val == "" {
					return
				}
				sb.WriteString(fmt.Sprintf("  %s: %s\n", label, val))
			}
			if en {
				write("Appearance", c.Appearance)
				write("Personality", c.Personality)
				write("Background", c.Background)
				write("Motivation", c.Motivation)
				write("Abilities", c.Abilities)
				write("Notes", c.Notes)
			} else {
				write("外貌", c.Appearance)
				write("性格", c.Personality)
				write("背景", c.Background)
				write("动机", c.Motivation)
				write("能力", c.Abilities)
				write("备注", c.Notes)
			}
			sb.WriteString("\n")
		}
	}

	if derived := buildOutlineDerivedCharacterContext(chapterOutline, settings, lang); derived != "" {
		sb.WriteString(derived)
	}
	return sb.String()
}

func buildWorldviewContextForLang(settings *ProjectSettings, chapterOutline, lang string) string {
	if settings == nil {
		return ""
	}

	en := NormalizeLanguage(lang) == LangEN
	var sb strings.Builder

	if len(settings.Worldview) > 0 {
		for _, w := range selectRelevantWorldviewEntries(settings, chapterOutline, 5) {
			sb.WriteString(fmt.Sprintf("【%s】(%s)\n  %s\n\n", w.Name, w.Category, w.Description))
		}
	}

	if len(settings.Organizations) > 0 {
		for _, o := range selectRelevantOrganizations(settings, chapterOutline, 4) {
			if en {
				sb.WriteString(fmt.Sprintf("[Organization: %s] (%s)\n  %s\n", o.Name, o.Type, o.Description))
				if len(o.Members) > 0 {
					sb.WriteString(fmt.Sprintf("  Member IDs: %s\n", strings.Join(o.Members, ", ")))
				}
			} else {
				sb.WriteString(fmt.Sprintf("【组织:%s】(%s)\n  %s\n", o.Name, o.Type, o.Description))
				if len(o.Members) > 0 {
					sb.WriteString(fmt.Sprintf("  成员IDs: %s\n", strings.Join(o.Members, ", ")))
				}
			}
			sb.WriteString("\n")
		}
	}

	return sb.String()
}

// buildMemoryForLang renders the memory block for injection into writing/fact-check prompts.
func buildMemoryForLang(state *Progress, idx int, lang string) string {
	block, _, _ := buildMemoryContextForLang(state, idx, lang)
	return block
}

func buildMemoryContextForLang(state *Progress, idx int, lang string) (string, int, int) {
	selected := selectRelevantMemories(state, idx)
	if len(selected) == 0 {
		return "", 0, len(state.MemoryEntries)
	}
	en := NormalizeLanguage(lang) == LangEN
	var sb strings.Builder
	if en {
		sb.WriteString("【Story Memory — long-term narrative details from earlier chapters】\n")
	} else {
		sb.WriteString("【叙事记忆——早期章节的关键叙事细节】\n")
	}
	for _, m := range selected {
		snippet := extractSnippet(state, m.Chapter, m.Position, 100)
		if snippet != "" {
			if en {
				sb.WriteString(fmt.Sprintf("[Ch.%d] %s (original: \"%s\")\n", m.Chapter, m.Content, snippet))
			} else {
				sb.WriteString(fmt.Sprintf("[第%d章] %s（原文：「%s」）\n", m.Chapter, m.Content, snippet))
			}
		} else {
			if en {
				sb.WriteString(fmt.Sprintf("[Ch.%d] %s\n", m.Chapter, m.Content))
			} else {
				sb.WriteString(fmt.Sprintf("[第%d章] %s\n", m.Chapter, m.Content))
			}
		}
	}
	return sb.String(), len(selected), len(state.MemoryEntries)
}

func selectRelevantMemories(state *Progress, idx int) []MemoryEntry {
	if len(state.MemoryEntries) == 0 {
		return nil
	}
	chapterNum := 0
	outline := ""
	if idx >= 0 && idx < len(state.Chapters) {
		chapterNum = state.Chapters[idx].Num
		outline = state.Chapters[idx].Outline
	}
	keywords := memoryKeywords(outline)
	var strong, recent, backlog []MemoryEntry
	seen := make(map[int]bool)
	for _, m := range state.MemoryEntries {
		if keywordHit(m, keywords) {
			strong = append(strong, m)
			seen[m.ID] = true
			continue
		}
		if chapterNum > 0 && chapterNum-m.Chapter <= 20 && chapterNum-m.Chapter > 0 {
			recent = append(recent, m)
			seen[m.ID] = true
			continue
		}
		switch m.Category {
		case "promise", "event", "item":
			backlog = append(backlog, m)
		}
	}
	limit := 12
	selected := append([]MemoryEntry(nil), strong...)
	for _, bucket := range [][]MemoryEntry{recent, backlog} {
		for _, m := range bucket {
			if len(selected) >= limit {
				break
			}
			if seen[m.ID] && !keywordHit(m, keywords) && chapterNum > 0 && chapterNum-m.Chapter > 20 {
				continue
			}
			selected = append(selected, m)
			seen[m.ID] = true
		}
		if len(selected) >= limit {
			break
		}
	}
	if len(selected) == 0 {
		if len(state.MemoryEntries) <= limit {
			return append([]MemoryEntry(nil), state.MemoryEntries...)
		}
		return append([]MemoryEntry(nil), state.MemoryEntries[len(state.MemoryEntries)-limit:]...)
	}
	return selected
}

func memoryKeywords(outline string) []string {
	outline = strings.TrimSpace(outline)
	if outline == "" {
		return nil
	}
	replacer := strings.NewReplacer("，", " ", "。", " ", "：", " ", ":", " ", "、", " ", "（", " ", "）", " ", "\n", " ", "\t", " ")
	parts := strings.Fields(replacer.Replace(outline))
	seen := make(map[string]bool)
	var out []string
	for _, p := range parts {
		p = strings.TrimSpace(stripNameMarks(p))
		if len([]rune(p)) < 2 || seen[p] {
			continue
		}
		seen[p] = true
		out = append(out, p)
		if len(out) >= 12 {
			break
		}
	}
	return out
}

func keywordHit(m MemoryEntry, keywords []string) bool {
	if len(keywords) == 0 {
		return false
	}
	for _, kw := range keywords {
		if strings.Contains(m.Content, kw) {
			return true
		}
	}
	return false
}

// extractSnippet extracts approximately maxRunes characters from the chapter content
// starting at the given paragraph position (1-indexed, split by double newlines).
func extractSnippet(state *Progress, chapterNum, position, maxRunes int) string {
	if position <= 0 || chapterNum <= 0 {
		return ""
	}
	for i := range state.Chapters {
		if state.Chapters[i].Num == chapterNum {
			content := state.Chapters[i].Content
			if content == "" {
				return ""
			}
			paragraphs := strings.Split(content, "\n\n")
			idx := position - 1
			if idx < 0 || idx >= len(paragraphs) {
				return ""
			}
			para := strings.TrimSpace(paragraphs[idx])
			runes := []rune(para)
			if len(runes) > maxRunes {
				return string(runes[:maxRunes]) + "…"
			}
			return para
		}
	}
	return ""
}

// formatMemoryForUpdatePrompt renders the existing memory list for the memory update prompt.
func formatMemoryForUpdatePrompt(entries []MemoryEntry, lang string) string {
	if len(entries) == 0 {
		if NormalizeLanguage(lang) == LangEN {
			return "(empty — no memories yet)"
		}
		return "（空——尚无记忆）"
	}
	en := NormalizeLanguage(lang) == LangEN
	var sb strings.Builder
	for _, m := range entries {
		if en {
			sb.WriteString(fmt.Sprintf("#%d [%s] Ch.%d: %s\n", m.ID, m.Category, m.Chapter, m.Content))
		} else {
			sb.WriteString(fmt.Sprintf("#%d [%s] 第%d章: %s\n", m.ID, m.Category, m.Chapter, m.Content))
		}
	}
	return sb.String()
}

// formatSelectedForeshadowsForChapterLang renders only the foreshadows explicitly selected for a chapter.
func formatSelectedForeshadowsForChapterLang(foreshadows []Foreshadow, selectedIDs []int, chapterNum int, lang string) string {
	if len(selectedIDs) == 0 {
		return ""
	}
	selected := make([]Foreshadow, 0, len(selectedIDs))
	selectedSet := make(map[int]bool, len(selectedIDs))
	for _, id := range selectedIDs {
		selectedSet[id] = true
	}
	for _, fs := range foreshadows {
		if selectedSet[fs.ID] {
			selected = append(selected, fs)
		}
	}
	if len(selected) == 0 {
		return ""
	}
	base := formatActiveForeshadowsForChapterLang(selected, chapterNum, lang)
	if base == "" {
		return ""
	}
	if NormalizeLanguage(lang) == LangEN {
		return base + "\n[Foreshadow integration requirement]\nMerge the selected foreshadows naturally into this chapter's outline and scene progression. They are writing guidance, not a checklist to be copied verbatim."
	}
	return base + "\n【伏笔融合要求】\n若已为本章勾选伏笔，请将这些伏笔自然融入本章大纲与场景推进中；它们是写作指导，不是逐条照抄的清单。"
}

// formatActiveForeshadowsForChapterLang renders the "active foreshadows" block in the requested language.
func formatActiveForeshadowsForChapterLang(foreshadows []Foreshadow, chapterNum int, lang string) string {
	var active []Foreshadow
	var overdue []Foreshadow

	for _, fs := range foreshadows {
		if fs.Status == ForeshadowPlanted || fs.Status == ForeshadowProgressing {
			active = append(active, fs)
			if fs.TargetChapter > 0 && chapterNum >= fs.TargetChapter {
				overdue = append(overdue, fs)
			}
		}
	}
	if len(active) == 0 {
		return ""
	}

	en := NormalizeLanguage(lang) == LangEN
	var sb strings.Builder
	if en {
		sb.WriteString("[Active foreshadows (you must advance or pay them off when writing)]\n")
	} else {
		sb.WriteString("【活跃伏笔（写作时必须注意推进或回收）】\n")
	}

	for _, fs := range active {
		if en {
			sb.WriteString(fmt.Sprintf("#%d \"%s\" [planted in chapter %d", fs.ID, fs.Name, fs.PlantChapter))
			if fs.TargetChapter > 0 {
				sb.WriteString(fmt.Sprintf(", expected payoff chapter %d", fs.TargetChapter))
			}
			sb.WriteString("]\n")
			sb.WriteString(fmt.Sprintf("   Description: %s\n", fs.Description))
		} else {
			sb.WriteString(fmt.Sprintf("#%d \"%s\" [第%d章埋设", fs.ID, fs.Name, fs.PlantChapter))
			if fs.TargetChapter > 0 {
				sb.WriteString(fmt.Sprintf("，预计第%d章回收", fs.TargetChapter))
			}
			sb.WriteString("]\n")
			sb.WriteString(fmt.Sprintf("   描述: %s\n", fs.Description))
		}

		if len(fs.Events) > 0 {
			if en {
				sb.WriteString("   Progress so far:\n")
				for _, ev := range fs.Events {
					sb.WriteString(fmt.Sprintf("   - Chapter %d: %s\n", ev.Chapter, ev.Note))
				}
			} else {
				sb.WriteString("   已有进展:\n")
				for _, ev := range fs.Events {
					sb.WriteString(fmt.Sprintf("   - 第%d章: %s\n", ev.Chapter, ev.Note))
				}
			}
		}

		isOverdue := false
		for _, od := range overdue {
			if od.ID == fs.ID {
				isOverdue = true
				break
			}
		}

		if isOverdue {
			if en {
				sb.WriteString(fmt.Sprintf("   ⚠️ This foreshadow is past its expected payoff chapter (%d); this chapter should prioritise paying it off.\n", fs.TargetChapter))
			} else {
				sb.WriteString(fmt.Sprintf("   ⚠️ 该伏笔已超过预计回收章节（第%d章），本章应优先考虑回收\n", fs.TargetChapter))
			}
		} else if fs.TargetChapter > 0 && chapterNum >= fs.TargetChapter-2 {
			if en {
				sb.WriteString(fmt.Sprintf("   → Approaching the expected payoff (chapter %d); start closing it.\n", fs.TargetChapter))
			} else {
				sb.WriteString(fmt.Sprintf("   → 接近预计回收节点（第%d章），可开始收束\n", fs.TargetChapter))
			}
		}

		sb.WriteString("\n")
	}

	return sb.String()
}

// formatForeshadowsForPromptLang renders the foreshadow list given to the update tracker.
func formatForeshadowsForPromptLang(foreshadows []Foreshadow, lang string) string {
	if len(foreshadows) == 0 {
		if NormalizeLanguage(lang) == LangEN {
			return "(none)"
		}
		return "无"
	}

	en := NormalizeLanguage(lang) == LangEN
	var sb strings.Builder
	for _, fs := range foreshadows {
		sb.WriteString(fmt.Sprintf("#%d [%s] %s\n", fs.ID, fs.Status, fs.Name))
		if en {
			sb.WriteString(fmt.Sprintf("   Description: %s\n", fs.Description))
			sb.WriteString(fmt.Sprintf("   Planted at: chapter %d", fs.PlantChapter))
			if fs.TargetChapter > 0 {
				sb.WriteString(fmt.Sprintf(", expected payoff: chapter %d", fs.TargetChapter))
			}
		} else {
			sb.WriteString(fmt.Sprintf("   描述: %s\n", fs.Description))
			sb.WriteString(fmt.Sprintf("   埋设于: 第%d章", fs.PlantChapter))
			if fs.TargetChapter > 0 {
				sb.WriteString(fmt.Sprintf("，预计回收: 第%d章", fs.TargetChapter))
			}
		}
		sb.WriteString("\n")

		if len(fs.Events) > 0 {
			if en {
				sb.WriteString("   Progress so far:\n")
				for _, ev := range fs.Events {
					sb.WriteString(fmt.Sprintf("   - Chapter %d: %s\n", ev.Chapter, ev.Note))
				}
			} else {
				sb.WriteString("   已有进展:\n")
				for _, ev := range fs.Events {
					sb.WriteString(fmt.Sprintf("   - 第%d章: %s\n", ev.Chapter, ev.Note))
				}
			}
		}

		if fs.Resolution != "" {
			if en {
				sb.WriteString(fmt.Sprintf("   Resolution: %s\n", fs.Resolution))
			} else {
				sb.WriteString(fmt.Sprintf("   回收方式: %s\n", fs.Resolution))
			}
		}

		sb.WriteString("\n")
	}

	return sb.String()
}
