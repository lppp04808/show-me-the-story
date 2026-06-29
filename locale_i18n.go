package main

import (
	"fmt"
	"net/http"
	"strings"
)

// localeFromRequest extracts the UI locale to use for response messages.
// Priority: X-UI-Locale header > locale query param > Accept-Language > "zh".
func localeFromRequest(r *http.Request) string {
	if r == nil {
		return LangZH
	}
	if v := strings.TrimSpace(r.Header.Get("X-UI-Locale")); v != "" {
		return NormalizeLanguage(v)
	}
	if v := strings.TrimSpace(r.URL.Query().Get("locale")); v != "" {
		return NormalizeLanguage(v)
	}
	if v := strings.TrimSpace(r.Header.Get("Accept-Language")); v != "" {
		first := strings.SplitN(v, ",", 2)[0]
		return NormalizeLanguage(first)
	}
	return LangZH
}

// errorCatalog maps a stable error key to its zh/en messages.
// Messages may contain %s for args.
var errorCatalog = map[string]map[string]string{
	"missing_project_name": {LangZH: "缺少项目名称", LangEN: "Project name is required"},
	"project_name_invalid_chars": {LangZH: "项目名称包含非法字符", LangEN: "Project name contains invalid characters"},
	"project_exists": {LangZH: "项目已存在", LangEN: "Project already exists"},
	"create_project_dir_failed": {LangZH: "创建项目目录失败: %s", LangEN: "Failed to create project directory: %s"},
	"init_project_config_failed": {LangZH: "初始化项目配置失败: %s", LangEN: "Failed to initialise project config: %s"},
	"select_project_first": {LangZH: "请先选择一个项目", LangEN: "Please select a project first"},
	"task_running_locked": {LangZH: "有AI任务正在运行，暂不能修改，请等待任务完成或先停止任务", LangEN: "An AI task is running; please wait or stop it before editing"},
	"task_running_wait": {LangZH: "有任务正在运行，请等待完成", LangEN: "A task is running; please wait until it finishes"},
	"no_task_running": {LangZH: "没有正在运行的任务", LangEN: "No task is currently running"},
	"invalid_json": {LangZH: "无效的JSON: %s", LangEN: "Invalid JSON: %s"},
	"missing_feedback": {LangZH: "缺少 feedback 字段", LangEN: "feedback field is required"},
	"missing_content": {LangZH: "缺少 content 字段", LangEN: "content field is required"},
	"missing_fields": {LangZH: "缺少 fields 字段", LangEN: "fields array is required"},
	"no_pending_changes": {LangZH: "没有待确认的配置变更", LangEN: "No pending config changes"},
	"load_pending_config_failed": {LangZH: "加载待确认配置失败: %s", LangEN: "Failed to load pending config changes: %s"},
	"save_pending_config_failed": {LangZH: "保存待确认配置失败: %s", LangEN: "Failed to save pending config changes: %s"},
	"delete_pending_config_failed": {LangZH: "清除待确认配置失败: %s", LangEN: "Failed to clear pending config changes: %s"},
	"invalid_chapter_num": {LangZH: "无效的章节编号", LangEN: "Invalid chapter number"},
	"chapter_not_found": {LangZH: "章节不存在", LangEN: "Chapter not found"},
	"chapter_n_not_found": {LangZH: "章节 %s 不存在", LangEN: "Chapter %s not found"},
	"phase_not_outline": {LangZH: "当前不在大纲阶段", LangEN: "Not in outline phase"},
	"phase_not_writing": {LangZH: "当前不在写作阶段", LangEN: "Not in writing phase"},
	"outline_empty": {LangZH: "大纲为空，请先生成大纲", LangEN: "Outline is empty; generate an outline first"},
	"outline_incomplete_confirm": {LangZH: "第 %s 章的大纲还没填完，请先补全后再确认", LangEN: "Chapter %s is still incomplete; finish its outline before confirming"},
	"outline_delete_pending_only": {LangZH: "只能删除待写作章节", LangEN: "Only pending outline chapters can be deleted"},
	"outline_no_selection": {LangZH: "请至少选择一章", LangEN: "Select at least one chapter"},
	"outline_delete_range_pending_only": {LangZH: "只能从待写作章节开始删除到末尾", LangEN: "Can only delete from a pending chapter through the end"},
	"outline_confirm_failed": {LangZH: "确认大纲失败: %s", LangEN: "Failed to confirm outline: %s"},
	"writing_chapter_present": {LangZH: "有正在写作/审核中的章节，请先处理后再重新生成大纲", LangEN: "There are chapters in writing/review; finish them before regenerating the outline"},
	"accepted_chapter_present": {LangZH: "存在已确认章节，无法整体重新生成大纲。如需追加章节请使用「生成后续大纲」", LangEN: "Confirmed chapters exist; cannot regenerate the full outline. Use \"Generate Continuation Outline\" to append."},
	"writing_chapter_present_delete": {LangZH: "有正在写作/审核中的章节，请先处理后再删除大纲", LangEN: "There are chapters in writing/review; finish them before deleting the outline"},
	"reset_progress_locked": {LangZH: "有任务正在运行，无法重置进度", LangEN: "A task is running; cannot reset progress"},
	"delete_chapter_locked": {LangZH: "有任务正在运行，无法删除章节", LangEN: "A task is running; cannot delete chapter"},
	"delete_outline_locked": {LangZH: "有任务正在运行，无法删除大纲", LangEN: "A task is running; cannot delete outline"},
	"delete_project_locked": {LangZH: "有任务正在运行，无法删除项目", LangEN: "A task is running; cannot delete project"},
	"cannot_delete_current_project": {LangZH: "不能删除当前正在使用的项目", LangEN: "Cannot delete the currently active project"},
	"project_not_found": {LangZH: "项目不存在", LangEN: "Project not found"},
	"delete_project_failed": {LangZH: "删除项目失败: %s", LangEN: "Failed to delete project: %s"},
	"delete_progress_failed": {LangZH: "删除进度文件失败: %s", LangEN: "Failed to delete progress file: %s"},
	"no_chapters_to_delete": {LangZH: "没有可删除的章节", LangEN: "No chapters to delete"},
	"writing_chapter_cannot_delete": {LangZH: "正在写作中的章节无法删除", LangEN: "Cannot delete a chapter that is being written"},
	"delete_frontier_unavailable": {LangZH: "当前写作前沿没有可删除的章节正文", LangEN: "No chapter content at the writing frontier to delete"},
	"writing_range_has_writing": {LangZH: "删除范围内有正在写作中的章节，无法删除", LangEN: "Delete range contains a chapter being written; cannot delete"},
	"save_progress_failed": {LangZH: "保存进度失败: %s", LangEN: "Failed to save progress: %s"},
	"save_failed": {LangZH: "保存失败: %s", LangEN: "Save failed: %s"},
	"save_config_failed": {LangZH: "保存配置失败: %s", LangEN: "Failed to save config: %s"},
	"save_api_config_failed": {LangZH: "保存API配置失败: %s", LangEN: "Failed to save API config: %s"},
	"serialize_config_failed": {LangZH: "序列化配置失败: %s", LangEN: "Failed to serialise config: %s"},
	"serialize_api_config_failed": {LangZH: "序列化API配置失败: %s", LangEN: "Failed to serialise API config: %s"},
	"api_test_timeout": {LangZH: "连接超时（15秒）", LangEN: "Connection timed out (15s)"},
	"api_test_failed": {LangZH: "测试失败: %s", LangEN: "Test failed: %s"},
	"api_test_success": {LangZH: "连接成功", LangEN: "Connection succeeded"},
	"character_name_empty": {LangZH: "角色名不能为空", LangEN: "Character name is required"},
	"character_not_found": {LangZH: "角色不存在", LangEN: "Character not found"},
	"worldview_field_empty": {LangZH: "名称和描述不能为空", LangEN: "Name and description are required"},
	"worldview_not_found": {LangZH: "世界观条目不存在", LangEN: "Worldview entry not found"},
	"organization_name_empty": {LangZH: "组织名不能为空", LangEN: "Organization name is required"},
	"organization_not_found": {LangZH: "组织不存在", LangEN: "Organization not found"},
	"relation_endpoints_empty": {LangZH: "源和目标不能为空", LangEN: "Source and target are required"},
	"relation_not_found": {LangZH: "关系不存在", LangEN: "Relation not found"},
	"foreshadow_name_required": {LangZH: "缺少 name", LangEN: "name field is required"},
	"foreshadow_desc_required": {LangZH: "缺少 description", LangEN: "description field is required"},
	"foreshadow_not_found": {LangZH: "伏笔不存在", LangEN: "Foreshadow not found"},
	"invalid_foreshadow_id": {LangZH: "无效的伏笔ID", LangEN: "Invalid foreshadow id"},
	"need_generate_outline_first": {LangZH: "请先生成大纲", LangEN: "Generate an outline first"},
	"continue_reset_first": {LangZH: "续写前请先重置进度", LangEN: "Reset progress before importing continuation"},
	"continue_analyze_first": {LangZH: "请先分析内容", LangEN: "Analyse the content first"},
	"analysis_no_chapters": {LangZH: "分析结果中没有任何章节", LangEN: "Analysis result contains no chapters"},
	"continue_import_failed": {LangZH: "导入续写失败: %s", LangEN: "Failed to import continuation: %s"},
	"manual_outline_chapter_count_required": {LangZH: "请填写正确的章节数", LangEN: "Please enter a valid chapter count"},
	"manual_outline_create_failed": {LangZH: "创建手动大纲失败: %s", LangEN: "Failed to create manual outline: %s"},
	"manual_outline_append_failed": {LangZH: "追加手动章节失败: %s", LangEN: "Failed to append manual outline chapters: %s"},
	"manual_outline_parse_failed": {LangZH: "批量追加章节失败: %s", LangEN: "Failed to append chapters in batch: %s"},
	"book_not_complete": {LangZH: "全书尚未完成（需所有章节已确认）", LangEN: "Book is not yet complete (all chapters must be confirmed)"},
	"need_polish_skill": {LangZH: "没有启用的润色技能，请先在技能管理页启用 polish 类技能", LangEN: "No polish skill enabled; enable a polish-type skill on the Skills page first"},
	"chapter_content_empty": {LangZH: "章节内容为空，无法润色", LangEN: "Chapter content is empty; cannot polish"},
	"chapter_edit_op_required": {LangZH: "缺少 operation 参数，必须为 replace_lines / replace_text / insert_after_line / append 之一", LangEN: "Missing operation parameter; must be one of: replace_lines / replace_text / insert_after_line / append"},
	"chapter_edit_text_required": {LangZH: "new_text 不能为空", LangEN: "new_text must not be empty"},
	"chapter_edit_failed": {LangZH: "章节编辑失败: %s", LangEN: "Chapter edit failed: %s"},
	"chapter_in_writing": {LangZH: "章节正在写作中，无法润色", LangEN: "Chapter is being written; cannot polish"},
	"chapter_num_required": {LangZH: "请指定章节编号", LangEN: "Chapter number is required"},
	"no_transitions_to_optimize": {LangZH: "没有可优化的章节（需要至少两个相邻的已确认章节）", LangEN: "No transitions to optimise (need at least two adjacent confirmed chapters)"},
	"missing_diagnosis_or_consistency": {LangZH: "缺少诊断或核查报告，请先运行全书诊断", LangEN: "Diagnosis or consistency report is missing; run book diagnosis first"},
	"no_roadmap_items": {LangZH: "没有可执行的优化工单", LangEN: "No roadmap items to execute"},
	"select_at_least_one_item": {LangZH: "请至少勾选一条待执行的工单", LangEN: "Select at least one pending roadmap item"},
	"clear_postprocess_failed": {LangZH: "清空失败: %s", LangEN: "Failed to clear: %s"},
	"chat_session_not_found": {LangZH: "会话不存在", LangEN: "Chat session not found"},
	"load_session_list_failed": {LangZH: "加载会话列表失败: %s", LangEN: "Failed to load chat sessions: %s"},
	"create_session_failed": {LangZH: "创建会话失败: %s", LangEN: "Failed to create chat session: %s"},
	"save_session_failed": {LangZH: "保存会话失败: %s", LangEN: "Failed to save chat session: %s"},
	"delete_session_failed": {LangZH: "删除会话失败: %s", LangEN: "Failed to delete chat session: %s"},
	"skill_not_found": {LangZH: "技能不存在", LangEN: "Skill not found"},
	"settings_ai_generate_moved": {LangZH: "此功能已移至 LLM 对话中，请通过聊天让 AI 帮你生成设定", LangEN: "This action has moved into the LLM chat; ask the assistant to generate settings for you"},
	"settings_polish_moved": {LangZH: "此功能已移至 LLM 对话中，请通过聊天让 AI 帮你润色", LangEN: "This action has moved into the LLM chat; ask the assistant to polish for you"},
	"writing_conflict_none": {LangZH: "当前没有待处理的写作冲突", LangEN: "No pending writing conflict to resolve"},
	"missing_action": {LangZH: "缺少 action 字段", LangEN: "action field is required"},
	"invalid_conflict_chapter_idx": {LangZH: "冲突章节索引无效", LangEN: "Invalid conflict chapter index"},
	"unsupported_action": {LangZH: "不支持的 action: %s", LangEN: "Unsupported action: %s"},
	"no_foreshadows_to_check": {LangZH: "当前没有伏笔，无需检查", LangEN: "No foreshadows to check"},
}

func lookupCatalog(lang, key string) (string, bool) {
	lang = NormalizeLanguage(lang)
	for _, catalog := range []map[string]map[string]string{messageCatalog, errorCatalog} {
		entry, ok := catalog[key]
		if !ok {
			continue
		}
		tpl := entry[lang]
		if tpl == "" {
			tpl = entry[LangZH]
		}
		if tpl != "" {
			return tpl, true
		}
	}
	return "", false
}

// T returns a localized message for the given key and args; falls back to zh, then key.
func T(lang, key string, args ...any) string {
	tpl, ok := lookupCatalog(lang, key)
	if !ok {
		return key
	}
	if len(args) == 0 {
		return tpl
	}
	return fmt.Sprintf(tpl, args...)
}

func msgArgsToStrings(args ...any) []string {
	if len(args) == 0 {
		return nil
	}
	out := make([]string, len(args))
	for i, a := range args {
		out[i] = fmt.Sprint(a)
	}
	return out
}

// writeErrorReq writes a JSON error response, picking message language from the request.
func (h *Handlers) writeErrorReq(w http.ResponseWriter, r *http.Request, code int, key string, args ...any) {
	lang := localeFromRequest(r)
	h.writeJSON(w, code, map[string]string{"error": T(lang, key, args...)})
}
