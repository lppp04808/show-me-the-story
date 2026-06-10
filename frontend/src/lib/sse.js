import { addLog, addToast, config, progress, taskRunning, streamingContent, streamingChapterIdx, streamCharCount, continueAnalysis, currentChatSession, settings, chatSessions, lastFailedTask, currentTaskName, logEntries } from './stores.js';
import { api } from './api.js';

let eventSource = null;
let reconnectTimer = null;

// —— 流式输出节流缓冲 ——
// 每个 token 都直接更新 store 会导致整页高频重渲染（长文本时 O(n²)），页面卡死。
// 这里将 chunk 先累积到缓冲区，按固定间隔批量刷入 store。
const FLUSH_INTERVAL = 150;

let contentBuf = '';
let contentIdx = -1;
let contentTimer = null;

function flushContentBuf() {
  if (contentTimer) { clearTimeout(contentTimer); contentTimer = null; }
  if (!contentBuf) return;
  const text = contentBuf;
  contentBuf = '';
  streamingChapterIdx.set(contentIdx);
  streamingContent.update(v => v + text);
  streamCharCount.update(n => n + Array.from(text).length);
}

function resetContentStream(idx) {
  contentBuf = '';
  if (contentTimer) { clearTimeout(contentTimer); contentTimer = null; }
  contentIdx = idx;
  streamingChapterIdx.set(idx);
  streamingContent.set('');
  streamCharCount.set(0);
}

let chatBuf = '';
let chatSessionId = null;
let chatTimer = null;

function flushChatBuf() {
  if (chatTimer) { clearTimeout(chatTimer); chatTimer = null; }
  if (!chatBuf) return;
  const text = chatBuf;
  const sid = chatSessionId;
  chatBuf = '';
  streamCharCount.update(n => n + Array.from(text).length);
  currentChatSession.update(s => {
    if (!s || s.id !== sid) return s;
    return { ...s, streaming_text: (s.streaming_text || '') + text };
  });
}

function clearChatBuf() {
  chatBuf = '';
  if (chatTimer) { clearTimeout(chatTimer); chatTimer = null; }
}

export function connectSSE() {
  if (eventSource) eventSource.close();
  eventSource = new EventSource('/api/events');

  eventSource.addEventListener('log', e => {
    const d = JSON.parse(e.data);
    addLog(d);
  });

  eventSource.addEventListener('progress_update', () => {
    api('GET', '/api/progress').then(p => progress.set(p)).catch(() => {});
  });

  eventSource.addEventListener('task_start', e => {
    const d = JSON.parse(e.data);
    taskRunning.set(true);
    resetContentStream(-1);
    clearChatBuf();
    streamCharCount.set(0);
    currentTaskName.set(taskNames[d.task] || d.task);
    logEntries.set([]);
    lastFailedTask.set(null);
  });

  const taskNames = {
    'outline_generation': '大纲生成',
    'outline_revision': '大纲修订',
    'chapter_generation': '章节创作',
    'chapter_revision': '章节修订',
    'foreshadow_suggest': '伏笔建议',
    'continue_analysis': '内容分析',
    'continuation_outline': '续写大纲',
    'settings_reconciliation': '设定协调',
    'chat_message': '助理对话',
  };

  eventSource.addEventListener('task_end', e => {
    const d = JSON.parse(e.data);
    taskRunning.set(false);
    resetContentStream(-1);
    clearChatBuf();
    streamCharCount.set(0);
    currentTaskName.set(null);
    api('GET', '/api/progress').then(p => progress.set(p)).catch(() => {});

    if (d.success) {
      const name = taskNames[d.task] || d.task;
      addToast(`✓ ${name}已完成`, 'success');
    } else {
      // 任务失败时记录重试信息
      lastFailedTask.set({ task: d.task, taskName: taskNames[d.task] || d.task });
    }

    if (d.task === 'chat_message') {
      let sessionId = null;
      currentChatSession.update(s => {
        if (!s) return s;
        sessionId = s.id;
        return { ...s, streaming_text: '' };
      });
      if (sessionId) {
        api('GET', '/api/chat/sessions/' + sessionId).then(s => {
          currentChatSession.set(s);
        }).catch(() => {});
      }
      api('GET', '/api/chat/sessions').then(s => chatSessions.set(s)).catch(() => {});
      api('GET', '/api/config').then(c => config.set(c)).catch(() => {});
      api('GET', '/api/settings').then(s => settings.set(s)).catch(() => {});
    }
  });

  // 一次新的流式输出开始（章节生成/修订/润色），清空旧缓冲，
  // 避免事实核查重试或自动连写时新旧内容叠加。
  eventSource.addEventListener('stream_start', e => {
    const d = JSON.parse(e.data);
    resetContentStream(d.chapter_idx);
  });

  eventSource.addEventListener('content_chunk', e => {
    const d = JSON.parse(e.data);
    if (d.chapter_idx !== contentIdx) {
      flushContentBuf();
      resetContentStream(d.chapter_idx);
    }
    contentBuf += d.text;
    if (!contentTimer) contentTimer = setTimeout(flushContentBuf, FLUSH_INTERVAL);
  });

  eventSource.addEventListener('stream_progress', e => {
    const d = JSON.parse(e.data);
    addLog({ level: 'info', msg: `正在生成中... 已写 ${d.char_count} 字`, time: new Date().toLocaleTimeString('zh-CN', { hour12: false }) });
  });

  eventSource.addEventListener('continue_analysis', e => {
    const d = JSON.parse(e.data);
    continueAnalysis.set(d);
  });

  eventSource.addEventListener('settings_reconciled', e => {
    const d = JSON.parse(e.data);
    api('GET', '/api/config').then(c => {
      config.set(c);
    }).catch(() => {});
    api('GET', '/api/progress').then(p => progress.set(p)).catch(() => {});
    addToast('设定协调完成：' + (d.explanation || ''), 'success');
  });

  eventSource.addEventListener('settings_updated', () => {
    api('GET', '/api/settings').then(s => settings.set(s)).catch(() => {});
    api('GET', '/api/config').then(c => config.set(c)).catch(() => {});
  });

  eventSource.addEventListener('foreshadow_suggestions', e => {
    const d = JSON.parse(e.data);
    addToast(`伏笔建议已生成，共 ${d.length} 条`, 'info');
  });

  eventSource.addEventListener('chat_chunk', e => {
    const d = JSON.parse(e.data);
    if (d.session_id !== chatSessionId) {
      flushChatBuf();
      chatSessionId = d.session_id;
    }
    chatBuf += d.text;
    if (!chatTimer) chatTimer = setTimeout(flushChatBuf, FLUSH_INTERVAL);
  });

  eventSource.addEventListener('tool_call_start', e => {
    const d = JSON.parse(e.data);
    flushChatBuf();
    currentChatSession.update(s => {
      if (!s) return s;
      const toolCalls = [...(s.pending_tool_calls || []), { name: d.tool_name, status: 'running', args: d.args }];
      return { ...s, pending_tool_calls: toolCalls };
    });
  });

  eventSource.addEventListener('tool_call_end', e => {
    const d = JSON.parse(e.data);
    currentChatSession.update(s => {
      if (!s) return s;
      const toolCalls = (s.pending_tool_calls || []).map(tc =>
        tc.name === d.tool_name && tc.status === 'running'
          ? { ...tc, status: 'done', result: d.result }
          : tc
      );
      return { ...s, pending_tool_calls: toolCalls };
    });
    api('GET', '/api/config').then(c => config.set(c)).catch(() => {});
    api('GET', '/api/settings').then(s => settings.set(s)).catch(() => {});
    api('GET', '/api/progress').then(p => progress.set(p)).catch(() => {});
  });

  eventSource.onerror = () => {
    eventSource.close();
    clearTimeout(reconnectTimer);
    reconnectTimer = setTimeout(connectSSE, 3000);
  };
}
