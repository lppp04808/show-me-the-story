package main

import "strings"

func selectRelevantCharacters(settings *ProjectSettings, chapterOutline string, limit int) []Character {
	if settings == nil || len(settings.Characters) == 0 {
		return nil
	}
	if limit <= 0 {
		limit = 6
	}
	selected := make([]Character, 0, minInt(limit, len(settings.Characters)))
	seen := make(map[string]bool, len(settings.Characters))
	for _, c := range settings.Characters {
		if len(selected) >= limit {
			break
		}
		if strings.Contains(chapterOutline, stripNameMarks(c.Name)) {
			selected = append(selected, c)
			seen[c.ID] = true
		}
	}
	for _, c := range settings.Characters {
		if len(selected) >= limit {
			break
		}
		if seen[c.ID] {
			continue
		}
		selected = append(selected, c)
	}
	return selected
}

func selectRelevantWorldviewEntries(settings *ProjectSettings, chapterOutline string, limit int) []WorldviewEntry {
	if settings == nil || len(settings.Worldview) == 0 {
		return nil
	}
	if limit <= 0 {
		limit = 5
	}
	selected := make([]WorldviewEntry, 0, minInt(limit, len(settings.Worldview)))
	seen := make(map[string]bool, len(settings.Worldview))
	keywords := memoryKeywords(chapterOutline)
	for _, w := range settings.Worldview {
		if len(selected) >= limit {
			break
		}
		if strings.Contains(chapterOutline, w.Name) || strings.Contains(chapterOutline, w.Category) || textHitsAny(w.Description, keywords) {
			selected = append(selected, w)
			seen[w.ID] = true
		}
	}
	for _, w := range settings.Worldview {
		if len(selected) >= limit {
			break
		}
		if seen[w.ID] {
			continue
		}
		selected = append(selected, w)
	}
	return selected
}

func selectRelevantOrganizations(settings *ProjectSettings, chapterOutline string, limit int) []Organization {
	if settings == nil || len(settings.Organizations) == 0 {
		return nil
	}
	if limit <= 0 {
		limit = 4
	}
	selected := make([]Organization, 0, minInt(limit, len(settings.Organizations)))
	seen := make(map[string]bool, len(settings.Organizations))
	keywords := memoryKeywords(chapterOutline)
	for _, o := range settings.Organizations {
		if len(selected) >= limit {
			break
		}
		if strings.Contains(chapterOutline, o.Name) || textHitsAny(o.Description, keywords) {
			selected = append(selected, o)
			seen[o.ID] = true
		}
	}
	for _, o := range settings.Organizations {
		if len(selected) >= limit {
			break
		}
		if seen[o.ID] {
			continue
		}
		selected = append(selected, o)
	}
	return selected
}

func textHitsAny(text string, keywords []string) bool {
	if text == "" || len(keywords) == 0 {
		return false
	}
	for _, kw := range keywords {
		if kw != "" && strings.Contains(text, kw) {
			return true
		}
	}
	return false
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
