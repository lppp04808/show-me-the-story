<script>
  import { api } from '../lib/api.js';
  import { fetchProgressLite } from '../lib/sse.js';
  import { progress, config, streamingContent, streamingChapterIdx, taskRunning, addToast, showConfirm, continueAnalysis, outlineCharacterSuggestions, outlineCharacterShowSuggestions, settings } from '../lib/stores.js';
  import { t } from '../lib/i18n/index.js';
  import ConfigChangePanel from '../components/ConfigChangePanel.svelte';

  $: p = $progress;
  $: displayTitle = $config?.story?.title || p?.title || '';
  $: displaySynopsis = $config?.story?.story_synopsis || p?.story_synopsis || '';
  $: chapters = p?.chapters || [];
  $: hasOutline = chapters.length > 0;
  $: hasAccepted = chapters.some(c => c.status === 'accepted');
  $: inOutlinePhase = p?.phase === 'outline';
  $: pendingCount = chapters.filter(c => c.status === 'pending').length;

  $: pendingChapterNums = chapters.filter(c => c.status === 'pending').map(c => c.num);
  $: selectedPendingChapters = selectedPendingChapters.filter(num => pendingChapterNums.includes(num));

  $: statusMeta = {
    pending:  { label: $t('outline.status.pending'),  cls: 'badge-ghost' },
    writing:  { label: $t('outline.status.writing'),  cls: 'badge-warning' },
    review:   { label: $t('outline.status.review'),   cls: 'badge-info' },
    accepted: { label: $t('outline.status.accepted'), cls: 'badge-success' },
  };

  let reviseFeedback = '';
  let showRevise = false;

  // 内联编辑
  let editingNum = -1;
  let editTitle = '';
  let editOutline = '';

  // 导入续写
  let showImport = false;
  let importContent = '';
  let continuationCount = 5;
  let continuationRequirements = '';
  let showContinuationModal = false;
  let showManualOutlineModal = false;
  let showAppendManualModal = false;
  let manualOutlineChapterCount = 10;
  let appendManualChapterCount = 1;
  let appendManualContent = '';
  let selectedPendingChapters = [];

  async function maybeResumeCheckpoint(mode, startFn) {
    try {
      const info = await api('GET', `/api/outline/checkpoint?mode=${mode}`);
      if (!info?.exists) {
        await startFn();
        return;
      }
      const msg = mode === 'continuation'
        ? $t('outline.toasts.continuationResumeAsk', { completed: info.completed_count, total: info.requested_new_chapters, next: info.next_start_num })
        : $t('outline.toasts.resumeAsk', { completed: info.completed_count, total: info.total_chapters, next: info.next_start_num });
      showConfirm(msg, async () => {
        try {
          await startFn();
        } catch (e) {
          addToast(e.message, 'error');
        }
      }, {
        confirmLabel: $t('outline.resume.continue'),
        cancelLabel: $t('outline.resume.restart'),
        onCancel: async () => {
          try {
            await api('DELETE', '/api/outline/checkpoint');
            await startFn();
          } catch (e) {
            addToast(e.message, 'error');
          }
        }
      });
    } catch (e) {
      addToast(e.message, 'error');
    }
  }

  async function generateOutline() {
    await maybeResumeCheckpoint('initial', async () => {
      await api('POST', '/api/outline/generate');
      addToast($t('outline.toasts.outlineStarted'), 'info');
    });
  }

  async function createManualOutline() {
    const count = Number(manualOutlineChapterCount) || 0;
    if (count < 1) {
      addToast($t('outline.toasts.manualCountRequired'), 'error');
      return;
    }
    try {
      await api('POST', '/api/outline/manual', { chapter_count: count });
      progress.set(await fetchProgressLite());
      showManualOutlineModal = false;
      addToast($t('outline.toasts.manualCreated', { n: count }), 'success');
    } catch (e) {
      addToast(e.message, 'error');
    }
  }

  async function appendManualOutline() {
    const count = Number(appendManualChapterCount) || 0;
    const content = appendManualContent.trim();
    if (!content && count < 1) {
      addToast($t('outline.toasts.manualCountRequired'), 'error');
      return;
    }
    try {
      await api('POST', '/api/outline/manual-append', {
        chapter_count: content ? 0 : count,
        content,
      });
      progress.set(await fetchProgressLite());
      showAppendManualModal = false;
      appendManualContent = '';
      addToast(content ? $t('outline.toasts.manualBatchAppended') : $t('outline.toasts.manualAppended', { n: count }), 'success');
    } catch (e) {
      addToast(e.message, 'error');
    }
  }

  async function confirmOutline() {
    showConfirm($t('outline.toasts.confirmAsk'), async () => {
      try {
        await api('POST', '/api/outline/confirm');
        progress.set(await fetchProgressLite());
        addToast($t('outline.toasts.outlineConfirmed'), 'success');
        window.location.hash = '#writing';
      } catch (e) { addToast(e.message, 'error'); }
    });
  }

  async function reviseOutline() {
    const fb = reviseFeedback.trim();
    if (!fb) { addToast($t('outline.toasts.reviseFeedbackRequired'), 'error'); return; }
    try {
      await api('POST', '/api/outline/revise', { feedback: fb });
      addToast($t('outline.toasts.reviseStarted'), 'info');
      reviseFeedback = '';
      showRevise = false;
    } catch (e) { addToast(e.message, 'error'); }
  }

  async function deleteOutline() {
    showConfirm($t('outline.toasts.deleteConfirm', { n: chapters.length }), async () => {
      try {
        await api('DELETE', '/api/outline');
        progress.set(await fetchProgressLite());
        addToast($t('outline.toasts.deleted'), 'success');
      } catch (e) { addToast(e.message, 'error'); }
    });
  }

  async function generateContinuation() {
    const count = Number(continuationCount) || 0;
    if (count < 1) {
      addToast($t('outline.toasts.continuationCountRequired'), 'error');
      return;
    }
    await maybeResumeCheckpoint('continuation', async () => {
      await api('POST', '/api/outline/generate-continuation', {
        chapter_count: count,
        user_requirements: continuationRequirements.trim(),
      });
      addToast($t('outline.toasts.continuationStarted'), 'info');
      continuationRequirements = '';
      showContinuationModal = false;
    });
  }

  function startEdit(ch) {
    editingNum = ch.num;
    editTitle = ch.title;
    editOutline = ch.outline;
  }

  function cancelEdit() {
    editingNum = -1;
  }

  async function saveEdit() {
    if (!editTitle.trim() || !editOutline.trim()) { addToast($t('outline.toasts.editRequired'), 'error'); return; }
    try {
      await api('PUT', '/api/outline/' + editingNum, { title: editTitle.trim(), outline: editOutline.trim() });
      progress.set(await fetchProgressLite());
      addToast($t('outline.toasts.editSaved', { num: editingNum }), 'success');
      editingNum = -1;
    } catch (e) { addToast(e.message, 'error'); }
  }

  function askDeletePendingChapter(num) {
    showConfirm($t('outline.toasts.chapterDeleteConfirm', { num }), () => deletePendingChapter(num), {
      confirmLabel: $t('common.delete'),
    });
  }

  async function deletePendingChapter(num) {
    try {
      await api('DELETE', '/api/outline/' + num);
      progress.set(await fetchProgressLite());
      selectedPendingChapters = selectedPendingChapters.filter(n => n !== num);
      if (editingNum === num) editingNum = -1;
      addToast($t('outline.toasts.chapterDeleted', { num }), 'success');
    } catch (e) { addToast(e.message, 'error'); }
  }

  function togglePendingSelection(num, checked) {
    if (checked) {
      if (!selectedPendingChapters.includes(num)) selectedPendingChapters = [...selectedPendingChapters, num];
      return;
    }
    selectedPendingChapters = selectedPendingChapters.filter(n => n !== num);
  }

  function selectAllPending() {
    selectedPendingChapters = [...pendingChapterNums];
  }

  function clearPendingSelection() {
    selectedPendingChapters = [];
  }

  function askDeleteSelectedPending() {
    const count = selectedPendingChapters.length;
    if (!count) {
      addToast($t('outline.toasts.noPendingSelection'), 'error');
      return;
    }
    showConfirm($t('outline.toasts.batchDeleteConfirm', { n: count }), deleteSelectedPending, {
      confirmLabel: $t('common.delete'),
    });
  }

  async function deleteSelectedPending() {
    try {
      await api('POST', '/api/outline/delete-batch', { chapter_nums: selectedPendingChapters });
      progress.set(await fetchProgressLite());
      selectedPendingChapters = [];
      editingNum = -1;
      addToast($t('outline.toasts.batchDeleted'), 'success');
    } catch (e) { addToast(e.message, 'error'); }
  }

  function askDeletePendingFrom(num) {
    showConfirm($t('outline.toasts.deleteFromConfirm', { num }), () => deletePendingFrom(num), {
      confirmLabel: $t('common.delete'),
    });
  }

  async function deletePendingFrom(num) {
    try {
      await api('DELETE', '/api/outline/from/' + num);
      progress.set(await fetchProgressLite());
      selectedPendingChapters = selectedPendingChapters.filter(n => n < num);
      editingNum = -1;
      addToast($t('outline.toasts.deletedFrom', { num }), 'success');
    } catch (e) { addToast(e.message, 'error'); }
  }

  async function importExisting() {
    const content = importContent.trim();
    if (!content) { addToast($t('outline.toasts.importContentRequired'), 'error'); return; }
    try {
      await api('POST', '/api/continue/import', { content });
      addToast($t('outline.toasts.importStarted'), 'info');
    } catch (e) { addToast(e.message, 'error'); }
  }

  async function confirmImport() {
    if (!$continueAnalysis) return;
    try {
      await api('POST', '/api/continue/confirm', $continueAnalysis);
      progress.set(await fetchProgressLite());
      continueAnalysis.set(null);
      showImport = false;
      importContent = '';
      addToast($t('outline.toasts.importDone'), 'success');
    } catch (e) { addToast(e.message, 'error'); }
  }

  async function confirmCharacterSuggestions() {
    const selected = $outlineCharacterSuggestions.filter(s => s._selected !== false);
    if (selected.length === 0) {
      addToast($t('outline.charSuggestions.noneSelected'), 'error');
      return;
    }
    try {
      await api('POST', '/api/outline/characters/confirm', { characters: selected });
      settings.set(await api('GET', '/api/settings'));
      outlineCharacterSuggestions.set([]);
      outlineCharacterShowSuggestions.set(false);
      addToast($t('outline.charSuggestions.adopted', { n: selected.length }), 'success');
    } catch (e) { addToast(e.message, 'error'); }
  }

  function dismissCharacterSuggestions() {
    outlineCharacterSuggestions.set([]);
    outlineCharacterShowSuggestions.set(false);
  }
</script>

<div class="space-y-3">
  {#if !hasOutline}
    <!-- 空状态 -->
    <div class="text-center py-14 text-base-content/50">
      <div class="text-5xl mb-3">📝</div>
      <p class="text-base mb-1">{$t('outline.empty.title')}</p>
      <p class="text-sm text-base-content/35 mb-6">{$t('outline.empty.hint')}</p>
      <div class="flex justify-center gap-2">
        <button class="btn btn-primary btn-sm" on:click={generateOutline} disabled={$taskRunning}>{$t('outline.btn.generate')}</button>
        <button class="btn btn-secondary btn-sm" on:click={() => showManualOutlineModal = true} disabled={$taskRunning}>{$t('outline.btn.manual')}</button>
        <button class="btn btn-ghost btn-sm" on:click={() => showImport = !showImport} disabled={$taskRunning}>{$t('outline.btn.import')}</button>
      </div>
    </div>

    {#if showImport}
      <div class="card bg-base-200 shadow-sm">
        <div class="card-body p-4 gap-2">
          <h3 class="card-title text-base">{$t('outline.import.title')}</h3>
          <p class="text-xs text-base-content/50">{$t('outline.import.hint')}</p>
          <textarea class="textarea w-full h-48 text-sm font-serif" bind:value={importContent} placeholder={$t('outline.import.placeholder')} disabled={$taskRunning}></textarea>
          <div class="flex justify-end gap-2">
            <button class="btn btn-ghost btn-xs" on:click={() => { showImport = false; importContent = ''; }}>{$t('common.cancel')}</button>
            <button class="btn btn-primary btn-xs" on:click={importExisting} disabled={$taskRunning || !importContent.trim()}>{$t('outline.import.start')}</button>
          </div>
        </div>
      </div>
    {/if}

    {#if $continueAnalysis}
      <div class="card bg-base-200 shadow-sm border border-primary/30">
        <div class="card-body p-4 gap-2">
          <h3 class="card-title text-base">{$t('outline.analysis.title')}</h3>
          <div class="grid grid-cols-2 gap-2">
            <div>
              <span class="text-xs text-base-content/50 mb-0.5 block">{$t('outline.analysis.fields.title')}</span>
              <input type="text" class="input input-sm w-full" bind:value={$continueAnalysis.title} disabled={$taskRunning} />
            </div>
            <div>
              <span class="text-xs text-base-content/50 mb-0.5 block">{$t('outline.analysis.fields.type')}</span>
              <input type="text" class="input input-sm w-full" bind:value={$continueAnalysis.story_type} disabled={$taskRunning} />
            </div>
          </div>
          <div>
            <span class="text-xs text-base-content/50 mb-0.5 block">{$t('outline.analysis.fields.synopsis')}</span>
            <textarea class="textarea textarea-sm w-full h-20 text-sm" bind:value={$continueAnalysis.story_synopsis} disabled={$taskRunning}></textarea>
          </div>
          <div>
            <span class="text-xs text-base-content/50 mb-0.5 block">{$t('outline.analysis.fields.style')}</span>
            <textarea class="textarea textarea-sm w-full h-16 text-sm" bind:value={$continueAnalysis.writing_style} disabled={$taskRunning}></textarea>
          </div>
          <div>
            <span class="text-xs text-base-content/50 mb-0.5 block">{$t('outline.analysis.fields.pov')}</span>
            <textarea class="textarea textarea-sm w-full h-16 text-sm" bind:value={$continueAnalysis.writing_pov} disabled={$taskRunning}></textarea>
          </div>
          <div class="text-xs text-base-content/50">{$t('outline.analysis.detected', { n: $continueAnalysis.chapters?.length || 0 })}</div>
          <div class="max-h-48 overflow-y-auto space-y-1">
            {#each ($continueAnalysis.chapters || []) as ch}
              <div class="bg-base-300 rounded p-2 text-xs">
                <span class="font-medium">{$t('outline.analysis.chapter', { num: ch.num, title: ch.title })}</span>
                <span class="text-base-content/50">{ch.outline || ch.summary || ''}</span>
              </div>
            {/each}
          </div>
          <div class="flex justify-end gap-2">
            <button class="btn btn-ghost btn-xs" on:click={() => continueAnalysis.set(null)}>{$t('outline.analysis.abandon')}</button>
            <button class="btn btn-success btn-xs" on:click={confirmImport} disabled={$taskRunning}>{$t('outline.analysis.confirm')}</button>
          </div>
        </div>
      </div>
    {/if}
  {:else}
    <ConfigChangePanel />

    {#if $outlineCharacterShowSuggestions && $outlineCharacterSuggestions.length > 0}
      <div class="card bg-base-200 border border-primary/30 shadow-sm">
        <div class="card-body py-4 gap-3">
          <h3 class="font-semibold">{$t('outline.charSuggestions.title', { n: $outlineCharacterSuggestions.length })}</h3>
          <p class="text-sm text-base-content/60">{$t('outline.charSuggestions.hint')}</p>
          <div class="space-y-2 max-h-72 overflow-y-auto">
            {#each $outlineCharacterSuggestions as s}
              <label class="flex gap-3 p-3 rounded-lg bg-base-300/50 cursor-pointer">
                <input type="checkbox" class="checkbox checkbox-sm mt-1" bind:checked={s._selected} />
                <div class="min-w-0 flex-1">
                  <div class="font-medium">{s.name}</div>
                  {#if s.description}
                    <div class="text-sm text-base-content/70 mt-1">{s.description}</div>
                  {/if}
                  <div class="text-xs text-base-content/50 mt-1">
                    {$t('outline.charSuggestions.line', { chapter: s.chapter_num, role: s.role || $t('outline.charSuggestions.noRole') })}
                  </div>
                </div>
              </label>
            {/each}
          </div>
          <div class="flex gap-2">
            <button class="btn btn-primary btn-sm" disabled={$taskRunning} on:click={confirmCharacterSuggestions}>{$t('outline.charSuggestions.adopt')}</button>
            <button class="btn btn-ghost btn-sm" on:click={dismissCharacterSuggestions}>{$t('outline.charSuggestions.dismiss')}</button>
          </div>
        </div>
      </div>
    {/if}

    <!-- 操作栏 -->
    <div class="card bg-base-200 shadow-sm">
      <div class="card-body p-4 gap-2">
        <div class="flex items-center gap-2 flex-wrap">
          <h3 class="text-base font-semibold flex-1 min-w-0 truncate">📖 {displayTitle || $t('common.untitled')}</h3>
          {#if inOutlinePhase && !hasAccepted}
            <button class="btn btn-secondary btn-xs" on:click={() => showManualOutlineModal = true} disabled={$taskRunning}>{$t('outline.btn.manual')}</button>
          {/if}
          {#if hasOutline}
            <button class="btn btn-secondary btn-xs" on:click={() => showAppendManualModal = true} disabled={$taskRunning}>{$t('outline.btn.appendManual')}</button>
          {/if}
          {#if inOutlinePhase}
            <button class="btn btn-success btn-xs" on:click={confirmOutline} disabled={$taskRunning || chapters.length === 0}>{$t('outline.btn.confirm')}</button>
          {/if}
          <button class="btn btn-ghost btn-xs" on:click={() => showRevise = !showRevise} disabled={$taskRunning}>{$t('outline.btn.revise')}</button>
          {#if hasAccepted}
            <button class="btn btn-primary btn-xs" on:click={() => showContinuationModal = true} disabled={$taskRunning}>{$t('outline.btn.continuation')}</button>
          {:else if inOutlinePhase}
            <button class="btn btn-ghost btn-xs" on:click={generateOutline} disabled={$taskRunning}>{$t('outline.btn.regenerate')}</button>
          {/if}
          {#if !hasAccepted}
            <button class="btn btn-ghost btn-xs text-error" on:click={deleteOutline} disabled={$taskRunning}>{$t('outline.btn.deleteOutline')}</button>
          {/if}
        </div>

        {#if showRevise}
          <div class="bg-base-300 rounded-lg p-3 space-y-2">
            <textarea class="textarea textarea-sm w-full h-20 text-sm" bind:value={reviseFeedback} placeholder={$t('outline.revise.placeholder')} disabled={$taskRunning}></textarea>
            <div class="flex justify-between items-center">
              <span class="text-xs text-base-content/40">{$t('outline.revise.hint')}</span>
              <div class="flex gap-2">
                <button class="btn btn-ghost btn-xs" on:click={() => { showRevise = false; reviseFeedback = ''; }}>{$t('common.cancel')}</button>
                <button class="btn btn-primary btn-xs" on:click={reviseOutline} disabled={$taskRunning || !reviseFeedback.trim()}>{$t('outline.revise.submit')}</button>
              </div>
            </div>
          </div>
        {/if}

        {#if p.core_prompt}
          <div>
            <span class="text-xs text-base-content/50">{$t('outline.corePrompt')}</span>
            <div class="bg-base-300 rounded p-2 text-sm mt-0.5 max-h-24 overflow-y-auto">{p.core_prompt}</div>
          </div>
        {/if}
        {#if displaySynopsis}
          <div>
            <span class="text-xs text-base-content/50">{$t('outline.synopsis')}</span>
            <div class="bg-base-300 rounded p-2 text-sm mt-0.5 max-h-24 overflow-y-auto">{displaySynopsis}</div>
          </div>
        {/if}
      </div>
    </div>

    <!-- 章节大纲列表 -->
    <div class="card bg-base-200 shadow-sm">
      <div class="card-body p-4 gap-2">
        <div class="flex items-center justify-between">
          <h4 class="text-sm font-semibold text-base-content/60">{$t('outline.chapterList')} <span class="font-normal text-base-content/35">{$t('outline.chapterList.summary', { total: chapters.length, suffix: pendingCount ? $t('outline.chapterList.pendingSuffix', { n: pendingCount }) : '' })}</span></h4>
          <div class="flex items-center gap-2 flex-wrap justify-end">
            <span class="text-xs text-base-content/35">{$t('outline.chapterList.editHint')}</span>
            {#if pendingCount > 0}
              <button class="btn btn-ghost btn-xs" on:click={selectAllPending} disabled={$taskRunning}>{$t('outline.chapterList.selectAll')}</button>
              <button class="btn btn-ghost btn-xs" on:click={clearPendingSelection} disabled={$taskRunning || selectedPendingChapters.length === 0}>{$t('outline.chapterList.clearSelection')}</button>
              <button class="btn btn-ghost btn-xs text-error" on:click={askDeleteSelectedPending} disabled={$taskRunning || selectedPendingChapters.length === 0}>{$t('outline.chapterList.deleteSelected', { n: selectedPendingChapters.length })}</button>
            {/if}
          </div>
        </div>
        <div class="space-y-1.5">
          {#each chapters as ch (ch.num)}
            {#if editingNum === ch.num}
              <div class="bg-base-300 rounded-lg p-3 space-y-2 ring-1 ring-primary/50">
                <div class="flex items-center gap-2">
                  <span class="text-sm font-bold text-base-content/50 shrink-0">{$t('outline.chapter.chapterLabel', { num: ch.num })}</span>
                  <input type="text" class="input input-sm flex-1" bind:value={editTitle} placeholder={$t('outline.chapter.titlePlaceholder')} disabled={$taskRunning} />
                  <button class="btn btn-ghost btn-xs" on:click={() => askDeletePendingFrom(ch.num)} disabled={$taskRunning}>{$t('outline.chapter.deleteFrom')}</button>
                  <button class="btn btn-ghost btn-xs text-error" on:click={() => askDeletePendingChapter(ch.num)} disabled={$taskRunning}>{$t('common.delete')}</button>
                </div>
                <textarea class="textarea textarea-sm w-full h-24 text-sm" bind:value={editOutline} placeholder={$t('outline.chapter.outlinePlaceholder')} disabled={$taskRunning}></textarea>
                <div class="flex justify-end gap-2">
                  <button class="btn btn-ghost btn-xs" on:click={cancelEdit}>{$t('common.cancel')}</button>
                  <button class="btn btn-success btn-xs" on:click={saveEdit} disabled={$taskRunning}>{$t('common.save')}</button>
                </div>
              </div>
            {:else}
              <!-- svelte-ignore a11y-click-events-have-key-events -->
              <!-- svelte-ignore a11y-no-static-element-interactions -->
              <div
                class="bg-base-300 rounded-lg p-2.5 group {ch.status === 'pending' && !$taskRunning ? 'cursor-pointer hover:ring-1 hover:ring-primary/40' : ''} transition-shadow"
                on:click={() => ch.status === 'pending' && !$taskRunning && startEdit(ch)}
              >
                <div class="flex items-center gap-2">
                  {#if ch.status === 'pending'}
                    <input type="checkbox" class="checkbox checkbox-xs" checked={selectedPendingChapters.includes(ch.num)} on:click|stopPropagation on:change={(e) => togglePendingSelection(ch.num, e.currentTarget.checked)} disabled={$taskRunning} />
                  {:else}
                    <span class="w-4 shrink-0"></span>
                  {/if}
                  <span class="text-sm font-bold text-base-content/40 w-12 shrink-0">{ch.num}</span>
                  <span class="text-sm font-medium flex-1 min-w-0 truncate">{ch.title}</span>
                  <span class="badge badge-xs {statusMeta[ch.status]?.cls || 'badge-ghost'}">{statusMeta[ch.status]?.label || ch.status}</span>
                  {#if ch.status === 'pending'}
                    <button class="btn btn-ghost btn-xs opacity-0 group-hover:opacity-100 transition-opacity shrink-0" on:click|stopPropagation={() => askDeletePendingFrom(ch.num)} disabled={$taskRunning}>{$t('outline.chapter.deleteFrom')}</button>
                    <button class="btn btn-ghost btn-xs text-error opacity-0 group-hover:opacity-100 transition-opacity shrink-0" on:click|stopPropagation={() => askDeletePendingChapter(ch.num)} disabled={$taskRunning}>{$t('common.delete')}</button>
                    <span class="text-xs text-primary opacity-0 group-hover:opacity-100 transition-opacity shrink-0">{$t('outline.chapter.editTag')}</span>
                  {/if}
                </div>
                <p class="text-xs text-base-content/50 mt-1 ml-14 line-clamp-2">{ch.outline}</p>
              </div>
            {/if}
          {/each}
        </div>

        {#if $streamingChapterIdx >= 0 && $streamingContent}
          <div class="bg-base-300 rounded p-3 mt-1 text-sm max-h-48 overflow-y-auto chapter-content">
            <div class="text-xs text-base-content/40 mb-1 flex items-center gap-1">
              <span class="loading loading-dots loading-xs"></span> {$t('outline.streamHint')}
            </div>
            {$streamingContent}
          </div>
        {/if}
      </div>
    </div>
  {/if}

  {#if showManualOutlineModal}
    <div class="modal modal-open">
      <div class="modal-box max-w-lg">
        <h3 class="font-bold text-lg">{$t('outline.manual.title')}</h3>
        <p class="text-sm text-base-content/60 mt-2">{$t('outline.manual.hint')}</p>
        <div class="form-control gap-3 mt-4">
          <div>
            <span class="label py-0"><span class="label-text">{$t('outline.manual.count')}</span></span>
            <input type="number" min="1" max="200" class="input input-bordered input-sm w-28" bind:value={manualOutlineChapterCount} disabled={$taskRunning} />
          </div>
        </div>
        <div class="modal-action">
          <button class="btn btn-ghost btn-sm" on:click={() => showManualOutlineModal = false}>{$t('common.cancel')}</button>
          <button class="btn btn-primary btn-sm" on:click={createManualOutline} disabled={$taskRunning}>{$t('outline.manual.submit')}</button>
        </div>
      </div>
      <div class="modal-backdrop" on:click={() => showManualOutlineModal = false} on:keydown={() => {}} role="presentation"></div>
    </div>
  {/if}

  {#if showAppendManualModal}
    <div class="modal modal-open">
      <div class="modal-box max-w-2xl">
        <h3 class="font-bold text-lg">{$t('outline.appendManual.title')}</h3>
        <p class="text-sm text-base-content/60 mt-2">{$t('outline.appendManual.hint')}</p>
        <div class="form-control gap-3 mt-4">
          <div>
            <span class="label py-0"><span class="label-text">{$t('outline.appendManual.count')}</span></span>
            <input type="number" min="1" max="200" class="input input-bordered input-sm w-28" bind:value={appendManualChapterCount} disabled={$taskRunning || !!appendManualContent.trim()} />
          </div>
          <div>
            <span class="label py-0"><span class="label-text">{$t('outline.appendManual.batchLabel')}</span></span>
            <textarea class="textarea textarea-bordered text-sm w-full h-72 font-serif" bind:value={appendManualContent} placeholder={$t('outline.appendManual.batchPlaceholder')} disabled={$taskRunning}></textarea>
            <div class="text-xs text-base-content/50 mt-2">{$t('outline.appendManual.batchHint')}</div>
          </div>
        </div>
        <div class="modal-action">
          <button class="btn btn-ghost btn-sm" on:click={() => { showAppendManualModal = false; appendManualContent = ''; }}>{$t('common.cancel')}</button>
          <button class="btn btn-primary btn-sm" on:click={appendManualOutline} disabled={$taskRunning}>{$t('outline.appendManual.submit')}</button>
        </div>
      </div>
      <div class="modal-backdrop" on:click={() => showAppendManualModal = false} on:keydown={() => {}} role="presentation"></div>
    </div>
  {/if}

  {#if showContinuationModal}
    <div class="modal modal-open">
      <div class="modal-box max-w-xl">
        <h3 class="font-bold text-lg">{$t('outline.continuation.modal.title')}</h3>
        <p class="text-sm text-base-content/60 mt-2">{$t('outline.continuation.modal.hint')}</p>
        <div class="form-control gap-3 mt-4">
          <div>
            <span class="label py-0"><span class="label-text">{$t('outline.continuation.modal.count')}</span></span>
            <input type="number" min="1" max="50" class="input input-bordered input-sm w-24" bind:value={continuationCount} disabled={$taskRunning} />
          </div>
          <div>
            <span class="label py-0"><span class="label-text">{$t('outline.continuation.modal.requirements')}</span></span>
            <textarea class="textarea textarea-bordered text-sm w-full h-36" bind:value={continuationRequirements} placeholder={$t('outline.continuation.placeholder')} disabled={$taskRunning}></textarea>
          </div>
        </div>
        <div class="modal-action">
          <button class="btn btn-ghost btn-sm" on:click={() => showContinuationModal = false}>{$t('common.cancel')}</button>
          <button class="btn btn-primary btn-sm" on:click={generateContinuation} disabled={$taskRunning}>{$t('outline.continuation.modal.submit')}</button>
        </div>
      </div>
      <div class="modal-backdrop" on:click={() => showContinuationModal = false} on:keydown={() => {}} role="presentation"></div>
    </div>
  {/if}
</div>
