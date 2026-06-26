package main

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"
)

const (
	outlineGenMaxAttempts   = 2
	outlineBatchSizeDefault = 20
	outlineBatchSizeReduced = 10
)

var errOutlineBatchMalformed = errors.New("outline batch malformed")

// calcOutlineLengthRange returns recommended per-chapter outline length bounds (characters)
// scaled to planned chapter prose length. ponytail: linear heuristic; tune divisors if field feels too short/long.
func calcOutlineLengthRange(targetWordsPerChapter int) (minLen, maxLen int) {
	if targetWordsPerChapter < 1 {
		targetWordsPerChapter = 2500
	}
	minLen = targetWordsPerChapter / 20
	if minLen < 80 {
		minLen = 80
	}
	maxLen = targetWordsPerChapter / 8
	if maxLen < 150 {
		maxLen = 150
	}
	if maxLen < minLen+20 {
		maxLen = minLen + 20
	}
	return minLen, maxLen
}

func outlineRuneLen(s string) int {
	return utf8.RuneCountInString(s)
}

func validateOutlineChapterLengths(chapters []OutlineChapter, minLen int) []int {
	var short []int
	for _, ch := range chapters {
		if outlineRuneLen(strings.TrimSpace(ch.Outline)) < minLen {
			short = append(short, ch.Num)
		}
	}
	return short
}

func formatCharacterListForOutline(settings *ProjectSettings, lang string) string {
	en := NormalizeLanguage(lang) == LangEN
	if settings == nil || len(settings.Characters) == 0 {
		if en {
			return "(No characters registered yet. You may introduce new characters only when necessary; mark each debut with \"first appearance\" and include a one-line role or relationship to the protagonist.)"
		}
		return "（尚未在角色管理中登记角色。仅因剧情需要方可引入新人物；须在其首次出场章节标注「首次登场」，并附一行身份或与主角关系说明。）"
	}

	var sb strings.Builder
	if en {
		sb.WriteString("Registered characters (prefer these; avoid duplicates):\n")
	} else {
		sb.WriteString("已登记角色（优先使用，避免功能重复）：\n")
	}
	for _, c := range settings.Characters {
		sb.WriteString(fmt.Sprintf("- %s", c.Name))
		if c.Personality != "" {
			if en {
				sb.WriteString(fmt.Sprintf(" — personality: %s", c.Personality))
			} else {
				sb.WriteString(fmt.Sprintf(" — 性格：%s", c.Personality))
			}
		} else if c.Background != "" {
			if en {
				sb.WriteString(fmt.Sprintf(" — background: %s", truncateRunes(c.Background, 40)))
			} else {
				sb.WriteString(fmt.Sprintf(" — 背景：%s", truncateRunes(c.Background, 40)))
			}
		}
		sb.WriteString("\n")
	}
	if en {
		sb.WriteString("\nOnly add unlisted characters when the plot requires it; mark \"first appearance\" in their debut chapter with a one-line description.")
	} else {
		sb.WriteString("\n仅当剧情需要时方可新增未登记角色；在其首次出场章节标注「首次登场」并附一行说明。")
	}
	return sb.String()
}

func formatOutlineLengthRequirementBlock(minLen, maxLen int, lang string) string {
	if NormalizeLanguage(lang) == LangEN {
		return fmt.Sprintf("Each chapter outline must be %d–%d characters (not counting the chapter title). Outlines shorter than %d characters are unacceptable.", minLen, maxLen, minLen)
	}
	return fmt.Sprintf("每章 outline 字段正文须为 %d–%d 字（不含章节标题）。低于 %d 字视为不合格。", minLen, maxLen, minLen)
}

func formatOutlineStructureRequirementBlock(lang string) string {
	if NormalizeLanguage(lang) == LangEN {
		return "Each chapter outline must cover, in order: opening scene/location; core conflict or goal; key turning point or revelation; characters appearing (with roles); how the chapter ends or what hook it leaves."
	}
	return "每章大纲须依次包含：开场场景/地点；本章核心冲突或目标；关键转折或信息点；出场人物（及作用）；章末走向或悬念钩子。"
}

func buildOutlinePromptExtras(cfg *Config, settings *ProjectSettings) map[string]string {
	minLen, maxLen := calcOutlineLengthRange(cfg.Story.TargetWordsPerChapter)
	return map[string]string{
		"OutlineMinWords": fmt.Sprintf("%d", minLen),
		"OutlineMaxWords": fmt.Sprintf("%d", maxLen),
		"CharacterList":   formatCharacterListForOutline(settings, cfg.Language),
	}
}

func mergeOutlinePromptData(base map[string]string, cfg *Config, settings *ProjectSettings) map[string]string {
	merged := make(map[string]string, len(base)+3)
	for k, v := range base {
		merged[k] = v
	}
	for k, v := range buildOutlinePromptExtras(cfg, settings) {
		merged[k] = v
	}
	return merged
}

func finalizeOutlinePrompt(template, rendered string, cfg *Config, settings *ProjectSettings, batchHint string) string {
	lang := cfg.Language
	minLen, maxLen := calcOutlineLengthRange(cfg.Story.TargetWordsPerChapter)
	rendered = appendIfMissingPlaceholder(template, rendered, "{{.CharacterList}}", formatCharacterListForOutline(settings, lang))
	if !strings.Contains(template, "{{.OutlineMinWords}}") {
		block := formatOutlineLengthRequirementBlock(minLen, maxLen, lang) + "\n" + formatOutlineStructureRequirementBlock(lang)
		rendered += "\n\n" + block
	}
	if strings.TrimSpace(batchHint) != "" {
		rendered += "\n\n" + strings.TrimSpace(batchHint)
	}
	return rendered
}

func buildOutlineBatchHint(startNum, batchCount, totalChapterCount int, lang string) string {
	if batchCount <= 0 || totalChapterCount <= 0 {
		return ""
	}
	endNum := startNum + batchCount - 1
	if NormalizeLanguage(lang) == LangEN {
		return fmt.Sprintf("IMPORTANT BATCHING RULES:\n- This is only one batch of a longer outline generation. Generate chapter %d through chapter %d only.\n- The full novel has %d chapters in total, so do not rush the ending into this batch unless chapter %d is also the final chapter.\n- Keep pacing and escalation coherent with earlier chapters, while leaving room for later chapters to continue the story naturally.\n- Return exactly this batch range in order, with no missing or extra chapters.", startNum, endNum, totalChapterCount, endNum)
	}
	return fmt.Sprintf("重要分批规则：\n- 这只是一次更长大纲生成任务中的当前批次，只生成第 %d 章到第 %d 章。\n- 全书总章节数为 %d 章，因此除非第 %d 章正好是全书结局，否则不要提前把结尾压缩到这一批里。\n- 节奏与冲突升级要承接前文，同时为后续章节自然推进预留空间。\n- 仅按顺序返回这一批章节，不要缺章，也不要多返回额外章节。", startNum, endNum, totalChapterCount, endNum)
}

func planOutlineBatchSizes(total int, batchSize int) []int {
	if total <= 0 {
		return nil
	}
	if batchSize <= 0 {
		batchSize = outlineBatchSizeDefault
	}
	var batches []int
	for total > 0 {
		n := batchSize
		if total < n {
			n = total
		}
		batches = append(batches, n)
		total -= n
	}
	return batches
}

func validateOutlineBatch(chapters []OutlineChapter, startNum, wantCount int) error {
	if wantCount <= 0 {
		return nil
	}
	if len(chapters) != wantCount {
		return fmt.Errorf("%w: want %d chapters, got %d", errOutlineBatchMalformed, wantCount, len(chapters))
	}
	for i, ch := range chapters {
		wantNum := startNum + i
		if ch.Num != wantNum {
			return fmt.Errorf("%w: chapter num %d at index %d, want %d", errOutlineBatchMalformed, ch.Num, i, wantNum)
		}
		if strings.TrimSpace(ch.Title) == "" {
			return fmt.Errorf("%w: chapter %d title is empty", errOutlineBatchMalformed, wantNum)
		}
		if strings.TrimSpace(ch.Outline) == "" {
			return fmt.Errorf("%w: chapter %d outline is empty", errOutlineBatchMalformed, wantNum)
		}
	}
	return nil
}

func formatOutlineContext(chapters []OutlineChapter, lang string) string {
	var sb strings.Builder
	for _, ch := range chapters {
		sb.WriteString(formatChapterLine(ch.Num, ch.Title, ch.Outline, lang))
	}
	return sb.String()
}

func formatOutlineBatchProgress(startNum, batchCount, done, total int, lang string) string {
	endNum := startNum + batchCount - 1
	if NormalizeLanguage(lang) == LangEN {
		return fmt.Sprintf("Generating outline batch: chapters %d-%d (completed %d/%d before this batch)", startNum, endNum, done, total)
	}
	return fmt.Sprintf("正在生成大纲批次：第 %d-%d 章（本批前已完成 %d/%d 章）", startNum, endNum, done, total)
}

func formatOutlineBatchDone(startNum, batchCount, done, total int, lang string) string {
	endNum := startNum + batchCount - 1
	if NormalizeLanguage(lang) == LangEN {
		return fmt.Sprintf("Outline batch complete: chapters %d-%d (now %d/%d complete)", startNum, endNum, done, total)
	}
	return fmt.Sprintf("大纲批次完成：第 %d-%d 章（当前已完成 %d/%d 章）", startNum, endNum, done, total)
}

func formatOutlineBatchReduceLog(startNum, batchCount int, lang string) string {
	endNum := startNum + batchCount - 1
	if NormalizeLanguage(lang) == LangEN {
		return fmt.Sprintf("Outline batch chapters %d-%d looks truncated or malformed; retrying this range with smaller batches of %d chapters.", startNum, endNum, outlineBatchSizeReduced)
	}
	return fmt.Sprintf("第 %d-%d 章这一批大纲疑似被截断或结构不完整；将该范围改为每批 %d 章重试。", startNum, endNum, outlineBatchSizeReduced)
}

func parseOutlineBatchMeta(data map[string]string) (startNum, batchCount, totalChapterCount int) {
	startNum = atoiOrDefault(data["StartNum"], 1)
	batchCount = atoiOrDefault(data["NewChapterCount"], 0)
	totalChapterCount = atoiOrDefault(data["TotalChapterCount"], 0)
	if batchCount == 0 {
		batchCount = atoiOrDefault(data["BatchCount"], 0)
	}
	if totalChapterCount == 0 {
		totalChapterCount = atoiOrDefault(data["ChapterCount"], 0)
	}
	if totalChapterCount == 0 && batchCount > 0 {
		totalChapterCount = startNum + batchCount - 1
	}
	return startNum, batchCount, totalChapterCount
}

func atoiOrDefault(s string, fallback int) int {
	if strings.TrimSpace(s) == "" {
		return fallback
	}
	var n int
	if _, err := fmt.Sscanf(strings.TrimSpace(s), "%d", &n); err != nil || n <= 0 {
		return fallback
	}
	return n
}

func formatShortOutlineRetryFeedback(shortNums []int, minLen int, lang string) string {
	nums := make([]string, len(shortNums))
	for i, n := range shortNums {
		nums[i] = fmt.Sprintf("%d", n)
	}
	joined := strings.Join(nums, ", ")
	if NormalizeLanguage(lang) == LangEN {
		return fmt.Sprintf("\n\nIMPORTANT: Chapters %s have outlines shorter than %d characters. Expand them with concrete plot beats (scene, conflict, turning point, characters, ending hook) and resubmit the full JSON.", joined, minLen)
	}
	return fmt.Sprintf("\n\n重要：第 %s 章大纲不足 %d 字。请补充具体情节（场景、冲突、转折、人物、章末钩子）后重新输出完整 JSON。", joined, minLen)
}

func truncateRunes(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen]) + "..."
}

func registeredCharacterNameSet(settings *ProjectSettings) map[string]bool {
	names := make(map[string]bool)
	if settings == nil {
		return names
	}
	for _, c := range settings.Characters {
		name := strings.TrimSpace(stripNameMarks(c.Name))
		if name != "" {
			names[name] = true
		}
	}
	return names
}

type outlineCharacterStub struct {
	Name        string
	Description string
}

var (
	firstAppearanceZH = regexp.MustCompile(`([^，。；：\n（(【\[]+?)[（(【\[]?首次登场[）)】\]]?`)
	firstAppearanceEN = regexp.MustCompile(`(?i)([\p{Han}A-Za-z·\s]{2,20}?)\s*[\(（]?\s*first appearance\s*[\)）]?`)
)

func extractFirstAppearanceStubs(outline string) []outlineCharacterStub {
	outline = strings.TrimSpace(outline)
	if outline == "" {
		return nil
	}
	var stubs []outlineCharacterStub
	seen := make(map[string]bool)

	addStub := func(name string) {
		name = strings.TrimSpace(stripNameMarks(name))
		name = strings.Trim(name, "：:、 ")
		if name == "" || seen[name] {
			return
		}
		seen[name] = true
		desc := extractStubDescription(outline, name)
		stubs = append(stubs, outlineCharacterStub{Name: name, Description: desc})
	}

	for _, m := range firstAppearanceZH.FindAllStringSubmatch(outline, -1) {
		if len(m) > 1 {
			addStub(m[1])
		}
	}
	for _, m := range firstAppearanceEN.FindAllStringSubmatch(outline, -1) {
		if len(m) > 1 {
			addStub(m[1])
		}
	}
	return stubs
}

func extractStubDescription(outline, name string) string {
	idx := strings.Index(outline, name)
	if idx < 0 {
		return ""
	}
	rest := outline[idx+len(name):]
	rest = strings.TrimLeft(rest, "（(【[")
	if cut := strings.IndexAny(rest, "。.\n"); cut >= 0 && cut < 120 {
		return strings.TrimSpace(rest[:cut])
	}
	return truncateRunes(strings.TrimSpace(rest), 80)
}

func buildOutlineDerivedCharacterContext(chapterOutline string, settings *ProjectSettings, lang string) string {
	registered := registeredCharacterNameSet(settings)
	stubs := extractFirstAppearanceStubs(chapterOutline)
	if len(stubs) == 0 {
		return ""
	}

	en := NormalizeLanguage(lang) == LangEN
	var sb strings.Builder
	for _, stub := range stubs {
		if registered[stub.Name] {
			continue
		}
		if en {
			sb.WriteString(fmt.Sprintf("[Outline-only character: %s]\n", stub.Name))
			if stub.Description != "" {
				sb.WriteString(fmt.Sprintf("  From outline: %s\n", stub.Description))
			}
		} else {
			sb.WriteString(fmt.Sprintf("【大纲人物：%s】\n", stub.Name))
			if stub.Description != "" {
				sb.WriteString(fmt.Sprintf("  大纲描述：%s\n", stub.Description))
			}
		}
		sb.WriteString("\n")
	}
	return sb.String()
}
