<script setup>
import { ref, computed, watch, onMounted, onUnmounted, nextTick } from 'vue';
import { templates, layouts } from './templates.js';
import CoursewareEvidence from './CoursewareEvidence.vue';
import TeachingPlotEditor from './TeachingPlotEditor.vue';
import CoursewareReviewDetails from './CoursewareReviewDetails.vue';
import SlideObjectCanvas from './SlideObjectCanvas.vue';
import { canvasTextIssue } from './canvas-text.js';
import ClassroomPresentation from './ClassroomPresentation.vue';
import WorkspaceIcon from './WorkspaceIcon.vue';
const presenting = ref(false),
  objectEditing = ref(''),
  objectMaps = ref({});
const deckSettings = ref(false),
  discussionOpen = ref(false);

const props = defineProps({
  task: Object,
  api: Function,
  token: String,
  available: Number,
  refresh: Function,
  retrying: Boolean,
  canvasAvailable: Boolean,
  canvasComplete: Boolean
});
const emit = defineEmits(['process', 'new-work', 'animation', 'retry']);
const local = ref(null),
  messages = ref([]),
  working = ref(false),
  error = ref(''),
  notice = ref(''),
  selected = ref(''),
  editing = ref(''),
  chatText = ref(''),
  chatMode = ref('revise'),
  consent = ref(false),
  imageURLs = ref({}),
  history = ref([]),
  showHistory = ref(false);
const historyOffset = ref(0),
  historyMore = ref(false),
  historyDraft = ref(null);
const editBaseline = ref(''),
  editRevision = ref(0);
const canvas = ref(null);
const waitStates = ['awaiting_preview', 'awaiting_confirmation', 'succeeded'];
const ready = computed(() => waitStates.includes(props.task?.status));
const finished = computed(() => props.task?.status === 'succeeded');
const prefix = computed(() => props.task?.status === 'awaiting_preview');
const dirty = computed(() => local.value && JSON.stringify(local.value) !== editBaseline.value);
const textIssue = computed(() => {
  let imageSize = 0;
  for (const [i, sl] of (local.value?.slides || []).entries()) {
    const issue = canvasTextIssue(sl);
    if (issue) return '第' + (i + 1) + '页：' + issue;
    imageSize +=
      (sl.picture?.data_url?.length || 0) +
      (sl.canvas_objects || []).reduce((n, o) => n + (o.data_url?.length || 0), 0);
  }
  return imageSize > 1200000 ? '整套课件图片总量过大，请减少图片或降低分辨率后再保存。' : '';
});
const pendingMessage = computed(() => messages.value.some((m) => m.status === 'pending'));
const paused = computed(
  () =>
    props.task?.status === 'failed' &&
    props.task?.input?.preview_first &&
    !!props.task?.result?.generation &&
    !!props.task?.result?.slides?.length
);
const structureLocked = computed(() => prefix.value || paused.value);
const editable = computed(
  () => (ready.value || paused.value) && !working.value && !pendingMessage.value
);
const approvedCount = computed(
  () => (props.task?.slide_reviews || []).filter((m) => m.approved && m.vote >= 0).length
);
const resumeNotice = '已恢复逐页检查点，继续只处理问题页和未完成页；未变更页面的审核保留。';
const deckNotice = computed(() => {
  const text = (local.value?.notice || '').replaceAll(
    '内容模型审核已停用；保留程序校验与截图审核，请教师核对知识、题设、计算和答案。',
    '生成时曾停用内容模型审核；请以各页最新核验状态为准。'
  );
  return text.includes(resumeNotice)
    ? (text.replaceAll(resumeNotice, '').trim() + ' ' + resumeNotice).trim()
    : text;
});
const allApproved = computed(
  () =>
    !!local.value?.slides?.length &&
    approvedCount.value === local.value.slides.length &&
    !dirty.value
);
const title = computed(() =>
  props.task?.stage === 'auto_recovering'
    ? '正在自动恢复制作'
    : {
        researching: props.task?.result?.plan ? '正在制作三页预览' : '正在整理资料与教学方案',
        awaiting_preview: '三页质量预览',
        expand_pending: '等待生成剩余页',
        expanding: '正在制作剩余课件',
        awaiting_confirmation: '逐页审核，准备交付',
        succeeded: '课件已完成',
        failed: '制作暂停',
        cancelled: '已取消'
      }[props.task?.status] || '正在研究教学资料'
);
const reviewPhase = computed(() =>
  finished.value
    ? 3
    : props.task?.status === 'awaiting_confirmation'
      ? 2
      : ['expand_pending', 'expanding', 'confirmed', 'rendering'].includes(props.task?.status)
        ? 1
        : 0
);
const reviewNext = computed(() =>
  finished.value
    ? '已完成交付，可以下载课件或继续调整页面。'
    : prefix.value
      ? '下一步：核对三页内容与风格，确认后制作其余页面。'
      : props.task?.status === 'awaiting_confirmation'
        ? '下一步：逐页核对知识、题设与答案，确认后完整交付。'
        : '页面会随制作进度更新，已有内容和制作进度会保留。'
);
const current = computed(() => local.value?.slides.find((sl) => sl.id === selected.value));
const currentIndex = computed(() => {
  const index = local.value?.slides.findIndex((sl) => sl.id === selected.value) ?? -1;
  return index < 0 ? 0 : index;
});
const currentPosition = computed(() => current.value?.order || currentIndex.value + 1);
function syncSelected(slides = local.value?.slides || []) {
  if (!slides.length) {
    selected.value = '';
    return;
  }
  if (!slides.some((sl) => sl.id === selected.value)) selected.value = slides[0].id;
}
async function selectSlide(id) {
  selected.value = id;
  await nextTick();
  const panel = canvas.value,
    target = panel?.querySelector('.studio-slide.selected');
  if (!panel || !target) return;
  if (getComputedStyle(panel).overflowY === 'visible') {
    const rect = target.getBoundingClientRect();
    if (rect.top < 44 || rect.top >= innerHeight)
      target.scrollIntoView({ block: 'start', behavior: 'instant' });
  } else panel.scrollTo({ top: 0, behavior: 'instant' });
}
function discussSlide(id) {
  void selectSlide(id);
  chatMode.value = 'revise';
  nextTick(() => canvas.value?.parentElement.querySelector('.chat-composer textarea')?.focus());
}
const generationProgress = computed(() => {
  const task = props.task,
    status = task?.status,
    plan = task?.result?.plan,
    total = plan?.pages?.length || task?.input?.pages || 15,
    events = task?.events || [],
    slides = task?.result?.slides || [];
  let completed = Math.min(total, props.task?.result?.slides?.length || 0),
    batch = '';
  for (const event of events) {
    const done = /^batch_complete_(\d+)$/.exec(event.stage || ''),
      start = /^batch_start_(\d+)_(\d+)$/.exec(event.stage || '');
    if (done) {
      completed = Math.max(completed, Math.min(total, Number(done[1])));
      batch = '';
    }
    if (start)
      batch =
        (completed >= total ? '正在检查并修复第' : '正在制作第') +
        start[1] +
        '至' +
        start[2] +
        '页';
    if (event.stage === 'repairing') batch = '正在自动修复，完成后继续制作';
  }
  const checkpoint = props.task?.result?.generation;
  if (checkpoint) {
    completed = Math.min(total, props.task?.result?.slides?.length || 0);
    const page = checkpoint.repair_page;
    if (page && page <= completed) completed--;
    const schedule = plan?.pages
      ? [...plan.pages.filter((p) => p.preview), ...plan.pages.filter((p) => !p.preview)]
      : [];
    const position = slides[page - 1]?.order || schedule[page - 1]?.position || page;
    batch = page
      ? (checkpoint.issue || page <= slides.length ? '正在修复' : '正在制作') +
        '教学位置第' +
        position +
        '页 · 其他页面已保留'
      : '';
  }
  const passed = (sl) => {
    const c = task?.result?.page_checks?.[sl.id];
    return (
      c &&
      ((c.content === 'sample' && c.visual === 'sample') ||
        (['checked', 'disabled', 'skipped'].includes(c.content) &&
          c.visual === 'checked' &&
          !c.issues?.some((v) => v.severity === 'error')))
    );
  };
  const drafts = plan ? slides.filter((sl) => !passed(sl)).length : 0;
  if (plan) completed = slides.length - drafts;
  const latest = events.at(-1);
  if (['page_generation', 'page_render', 'page_review', 'page_draft_saved'].includes(latest?.stage))
    batch = latest.message;
  const detail =
    (plan ? '已规划' + total + '页' : '正在规划') +
    ' · 已保存' +
    slides.length +
    '页' +
    (drafts ? ' · 待修复草稿' + drafts + '页' : '') +
    (task?.courseware_provider_calls
      ? ' · 已调用' + task.courseware_provider_calls + '次服务'
      : '');
  const delivered = status === 'succeeded',
    review = ['awaiting_preview', 'awaiting_confirmation'].includes(status),
    stopped = ['failed', 'cancelled'].includes(status);
  const paused = checkpoint?.repair_page
    ? '制作已暂停 · 待处理第' + checkpoint.repair_page + '页，已保存页面保留'
    : '制作已暂停';
  return {
    percent: Math.floor((completed / total) * 100),
    text: delivered
      ? '已交付'
      : stopped
        ? paused
        : review
          ? '等待审核'
          : props.task?.stage === 'auto_recovering'
            ? '已保存进度，稍后自动继续 · 无需操作'
            : batch || (completed >= total ? '正在检查和排版' : '正在整理资料与教学方案'),
    detail,
    completed,
    total
  };
});
const meta = (id) => (props.task?.slide_reviews || []).find((m) => m.id === id) || {};
function qualityStatus(sl) {
  const check = props.task?.result?.page_checks?.[sl.id];
  if (!check) return '';
  if (check.content === 'sample') return '样例检查，未调用真实模型';
  if (check.issues?.some((v) => v.severity === 'error')) return '内容或画面需修复';
  if (check.content === 'skipped')
    return check.visual === 'checked'
      ? '内容未核验（普通页）；实际画面已检查，请教师核对'
      : '内容未核验；实际画面待检查';
  if (check.content === 'disabled')
    return check.visual === 'checked'
      ? '本页生成时未做内容核验；实际画面已检查，可重新检查后确认'
      : '本页生成时未做内容核验；实际画面检查尚未完成';
  if (check.content !== 'checked' || check.visual !== 'checked') return '内容或画面检查尚未完成';
  return '内容已核验 · 实际画面已检查，仍需教师确认';
}
function qualityLabel(sl) {
  if (changed(sl)) return '修改后待检查';
  const check = props.task?.result?.page_checks?.[sl.id];
  if (!check) return '内容未核验';
  if (check.issues?.some((issue) => issue.severity === 'error')) return '需修复';
  return (
    { checked: '内容已核验', sample: '样例检查', skipped: '普通页未核验' }[check.content] ||
    '内容未核验'
  );
}
async function recheck() {
  await run(async () => {
    notice.value = '正在检查现有页面，完成后更新核验结果；已有内容会保留。';
    await persist(true);
    notice.value = '已重新检查并保存。请查看各页核验状态与问题，确认后再审核；未重新生成课件内容。';
  });
}
const changed = (sl) => {
  const saved = props.task?.result;
  return (
    JSON.stringify(sl) !== JSON.stringify(saved?.slides?.find((s) => s.id === sl.id)) ||
    local.value?.theme !== saved?.theme ||
    local.value?.title !== saved?.title ||
    JSON.stringify(local.value?.plan?.pages?.find((p) => p.position === sl.order)) !==
      JSON.stringify(saved?.plan?.pages?.find((p) => p.position === sl.order))
  );
};
let imageVersion = 0,
  conversationVersion = 0,
  pollTimer,
  disposed = false,
  imageTask = '',
  imageRevision = '',
  imageKeys = {};
function clearImages() {
  for (const url of Object.values(imageURLs.value)) URL.revokeObjectURL(url);
  imageURLs.value = {};
  imageKeys = {};
}
async function loadImages() {
  const version = ++imageVersion,
    id = props.task?.id,
    revision = props.task?.revision;
  if (imageTask !== id || imageRevision !== String(revision ?? '')) {
    clearImages();
    imageTask = id;
    imageRevision = String(revision ?? '');
  }
  const deck = props.task?.result,
    slides = deck?.slides || [];
  // A rendered page is position-sensitive: the same slide id can point to a
  // different PNG after a reorder or a newly rendered artifact revision.
  const key = (sl, index) => JSON.stringify([id, revision, index, deck.title, deck.theme, sl]);
  const available = props.task?.preview_images_available
    ? props.task.preview_slide_ids || slides.map((sl) => sl.id)
    : [];
  for (const slideId of Object.keys(imageURLs.value)) {
    const index = slides.findIndex((sl) => sl.id === slideId),
      sl = index < 0 ? null : slides[index];
    if (!sl || imageKeys[slideId] !== key(sl, index) || !available.includes(slideId)) {
      URL.revokeObjectURL(imageURLs.value[slideId]);
      delete imageURLs.value[slideId];
      delete imageKeys[slideId];
    }
  }
  for (const [index, sl] of slides.entries()) {
    if (!available.includes(sl.id) || imageURLs.value[sl.id]) continue;
    if (version !== imageVersion || disposed) return;
    try {
      const r = await fetch(
        '/api/tasks/' +
          id +
          '/slides/' +
          encodeURIComponent(sl.id) +
          '/preview?revision=' +
          revision,
        { headers: { Authorization: 'Bearer ' + props.token } }
      );
      if (!r.ok) continue;
      const blob = await r.blob();
      if (version !== imageVersion || disposed) return;
      imageURLs.value[sl.id] = URL.createObjectURL(blob);
      imageKeys[sl.id] = key(sl, index);
    } catch {}
  }
}
async function openObjectCanvas(slide) {
  if (objectEditing.value === slide.id) {
    objectEditing.value = '';
    return;
  }
  await run(async () => {
    const id = props.task.id,
      revision = props.task.revision;
    let result = await props.api(
      '/tasks/' + id + '/slides/' + encodeURIComponent(slide.id) + '/objects?revision=' + revision
    );
    if (props.task.id !== id || props.task.revision !== revision) return;
    if (!result.objects.length) {
      await persist(true);
      if (props.task.id !== id) return;
      result = await props.api(
        '/tasks/' +
          id +
          '/slides/' +
          encodeURIComponent(slide.id) +
          '/objects?revision=' +
          props.task.revision
      );
      if (!result.objects.length) throw Error('本页暂时没有可移动对象，请使用内容编辑。');
    }
    const saved = props.task.result.slides.find((s) => s.id === slide.id)?.canvas_positions || {};
    objectMaps.value[slide.id] = result.objects.map((o) => ({
      ...o,
      offset: saved[o.id] || { dx: 0, dy: 0 }
    }));
    editing.value = '';
    objectEditing.value = slide.id;
    await reveal('.object-editor');
  });
}
async function previewCanvas(slide) {
  const id = props.task.id,
    revision = editRevision.value;
  if (revision !== props.task.revision)
    throw Error('后台版本已变化，修改仍保留；请先核对最新版本。');
  const result = await props.api(
    '/tasks/' + id + '/slides/' + encodeURIComponent(slide.id) + '/canvas-preview',
    'POST',
    { revision, result: JSON.parse(JSON.stringify(local.value)) }
  );
  if (disposed || props.task.id !== id || props.task.revision !== revision)
    throw Error('预览版本已变化，修改仍保留。');
  return result;
}
async function loadMessages() {
  const version = ++conversationVersion,
    id = props.task?.id;
  if (!id) return;
  const rows = await props.api('/tasks/' + id + '/conversation');
  if (version === conversationVersion && props.task?.id === id && !disposed) messages.value = rows;
}
watch(
  [() => props.task?.id, () => props.task?.revision],
  async ([id, revision], [oldId, oldRevision] = []) => {
    if (id !== oldId) {
      objectEditing.value = '';
      objectMaps.value = {};
      messages.value = [];
      selected.value = '';
      editing.value = '';
      history.value = [];
      historyDraft.value = null;
      showHistory.value = false;
      consent.value = false;
      error.value = '';
      notice.value = '';
      chatText.value = '';
    }
    if (id === oldId && dirty.value) {
      notice.value = '后台版本已更新，当前未保存修改已保留。保存前请核对最新版本。';
    } else {
      local.value = props.task?.result?.slides
        ? JSON.parse(JSON.stringify(props.task.result))
        : null;
      editBaseline.value = JSON.stringify(local.value);
      editRevision.value = revision || 0;
    }
    syncSelected();
    if (revision !== oldRevision) consent.value = false;
    await Promise.allSettled([loadImages(), loadMessages()]);
  },
  { immediate: true }
);
watch(
  () => props.task?.events?.length,
  () => {
    if (!working.value) loadMessages().catch(() => {});
  }
);
watch(
  () => props.task?.preview_slide_ids?.join(','),
  () => loadImages()
);
watch(
  () => props.task?.result?.evaluation,
  (v) => {
    if (local.value) {
      if (v !== undefined) local.value.evaluation = JSON.parse(JSON.stringify(v));
      else delete local.value.evaluation;
    }
  },
  { deep: true }
);
async function reveal(selector) {
  await nextTick();
  const panel = canvas.value,
    target = panel?.querySelector(selector);
  if (!target) return;
  const behavior = window.matchMedia('(prefers-reduced-motion: reduce)').matches
    ? 'instant'
    : 'smooth';
  if (getComputedStyle(panel).overflowY === 'visible')
    target.scrollIntoView({ block: 'start', behavior });
  else
    panel.scrollTo({
      top:
        panel.scrollTop +
        target.getBoundingClientRect().top -
        panel.getBoundingClientRect().top -
        16,
      behavior
    });
}
async function editSlide(id) {
  await selectSlide(id);
  editing.value = editing.value === id ? '' : id;
  if (editing.value) {
    await nextTick();
    const search = canvas.value?.querySelector('.studio-slide.selected .commons-image-search');
    if (search)
      for (let node = search; node && node !== canvas.value; node = node.parentElement)
        if (node.tagName === 'DETAILS') node.open = true;
    await reveal('.studio-slide.selected .studio-slide-editor');
    canvas.value
      ?.querySelector('.studio-slide.selected .studio-slide-editor input')
      ?.focus({ preventScroll: true });
  }
}
async function run(fn) {
  if (working.value) return;
  working.value = true;
  error.value = '';
  notice.value = '';
  try {
    await fn();
  } catch (e) {
    error.value = e.message;
    void reveal('.alert.error');
  } finally {
    working.value = false;
  }
}
async function persist(force = false) {
  if (!dirty.value && !force) return;
  if (textIssue.value) throw Error(textIssue.value);
  if (editRevision.value !== props.task.revision)
    throw Error('后台版本已变化，当前修改仍保留；请核对最新版本后再保存，避免覆盖其他修改。');
  const path = '/tasks/' + props.task.id + (finished.value ? '/edit' : '/draft');
  await props.api(path, 'POST', { result: local.value, revision: editRevision.value });
  editBaseline.value = JSON.stringify(local.value);
  objectEditing.value = '';
  objectMaps.value = {};
  await props.refresh();
  await nextTick();
  notice.value = '内容已保存，变更的页面需要重新审核。';
}
async function approve(id, approved) {
  await run(async () => {
    await persist();
    const m = meta(id);
    if (!m.hash) throw Error('页面已更新，请刷新后审核');
    // Explicit teacher approval supersedes the old feedback mechanism.
    if (approved && m.vote < 0)
      await props.api('/tasks/' + props.task.id + '/slides/' + id + '/feedback', 'POST', {
        revision: props.task.revision,
        hash: m.hash,
        vote: 0,
        reason: ''
      });
    await props.api('/tasks/' + props.task.id + '/slides/' + id + '/review', 'POST', {
      revision: props.task.revision,
      hash: m.hash,
      approved
    });
    await props.refresh();
  });
}
async function approveAll() {
  if (!editable.value || allApproved.value) return;
  const id = props.task.id;
  await run(async () => {
    await persist();
    if (disposed || props.task?.id !== id) return;
    await props.refresh();
    await nextTick();
    if (disposed || props.task?.id !== id) return;
    if (!ready.value || pendingMessage.value) throw Error('课件正在生成或修订，请稍后审核');
    const revision = props.task.revision;
    const rows = props.task.result.slides.map((sl, i) => {
      const m = meta(sl.id);
      if (!m.hash) throw Error('页面已更新，请刷新后审核');
      return { ...m };
    });
    try {
      for (const m of rows.filter((m) => !m.approved)) {
        if (disposed || props.task?.id !== id) return;
        if (props.task.revision !== revision) throw Error('当前版本已变化，请审核最新内容');
        if (m.vote < 0)
          await props.api(
            '/tasks/' + id + '/slides/' + encodeURIComponent(m.id) + '/feedback',
            'POST',
            { revision, hash: m.hash, vote: 0, reason: '' }
          );
        await props.api(
          '/tasks/' + id + '/slides/' + encodeURIComponent(m.id) + '/review',
          'POST',
          { revision, hash: m.hash, approved: true }
        );
      }
    } catch (e) {
      if (disposed || props.task?.id !== id) return;
      await props.refresh().catch(() => {});
      if (disposed || props.task?.id !== id) return;
      throw Error(
        '一键审核未全部完成，已记录的审核会保留。' +
          e.message +
          '；如核验未完成，请点击“重新检查”，查看本页核对信息后再审核。'
      );
    }
    if (disposed || props.task?.id !== id) return;
    await props.refresh();
    await nextTick();
    if (disposed || props.task?.id !== id) return;
    if (props.task.revision !== revision || !allApproved.value)
      throw Error('页面或审核状态已变化，请刷新后重新审核');
    notice.value =
      '当前' +
      rows.length +
      '页已全部审核通过。' +
      (prefix.value ? '请另行确认生成剩余页。' : finished.value ? '' : '完整交付仍需单独确认。');
  });
}
async function send() {
  if (!chatText.value.trim()) return;
  discussionOpen.value = true;
  const content = chatText.value.trim(),
    mode = chatMode.value,
    target = selected.value;
  await run(async () => {
    if (mode === 'revise' && !target) throw Error('先选择需要修改的页面');
    if (mode === 'revise' && finished.value && props.available < 5)
      throw Error('单页AI改写需5积分，目前可用额度不足');
    await persist();
    await props.api('/tasks/' + props.task.id + '/conversation', 'POST', {
      content,
      mode,
      slide_id: target,
      revision: props.task.revision,
      idempotency_key: crypto.randomUUID()
    });
    chatText.value = '';
    await props.refresh();
    await loadMessages();
  });
}
async function expand() {
  await run(async () => {
    await props.api('/tasks/' + props.task.id + '/expand', 'POST', {
      revision: props.task.revision
    });
    await props.refresh();
  });
}
async function finish() {
  await run(async () => {
    await props.api('/tasks/' + props.task.id + '/confirm', 'POST', {
      revision: props.task.revision,
      result: local.value,
      accept_review: consent.value
    });
    await props.refresh();
  });
}
async function cancel() {
  await run(async () => {
    await props.api('/tasks/' + props.task.id + '/cancel', 'POST', {});
    await props.refresh();
  });
}
async function download(preview = false, version, format = 'pptx') {
  await run(async () => {
    if (!version) await persist();
    const path =
      '/api/tasks/' +
      props.task.id +
      (preview
        ? '/preview-pptx'
        : version
          ? '/revisions/' + version + '/download/' + format
          : '/download/' + format);
    const r = await fetch(path, { headers: { Authorization: 'Bearer ' + props.token } });
    if (!r.ok) throw Error('文件尚不可用');
    const url = URL.createObjectURL(await r.blob()),
      a = document.createElement('a');
    a.href = url;
    a.download = (local.value?.title || '课件') + (preview ? '-预览' : '') + '.' + format;
    a.click();
    setTimeout(() => URL.revokeObjectURL(url), 2000);
  });
}
function addSlide(index) {
  if (!editable.value || structureLocked.value) return;
  const slide = {
    id: crypto.randomUUID(),
    title: '新页面',
    layout: 'concept',
    bullets: ['填写具体教学问题'],
    notes: '请补充讲解依据与答案。'
  };
  local.value.slides.splice(index + 1, 0, slide);
  void selectSlide(slide.id);
  editing.value = slide.id;
}
function duplicateSlide(index) {
  if (!editable.value || structureLocked.value) return;
  const slide = JSON.parse(JSON.stringify(local.value.slides[index]));
  slide.id = crypto.randomUUID();
  slide.title += ' · 副本';
  local.value.slides.splice(index + 1, 0, slide);
  void selectSlide(slide.id);
  editing.value = slide.id;
}
function removeSlide(index) {
  if (
    !editable.value ||
    structureLocked.value ||
    local.value.slides.length <= (local.value.plan ? 3 : 8)
  )
    return;
  if (!window.confirm('删除本页？保存后会形成新版本，已保存的旧版本保留。')) return;
  const [removed] = local.value.slides.splice(index, 1);
  if (selected.value === removed.id)
    selectSlide(local.value.slides[Math.min(index, local.value.slides.length - 1)].id);
  if (editing.value === removed.id) editing.value = '';
  if (objectEditing.value === removed.id) objectEditing.value = '';
}
function moveSlide(index, step) {
  if (
    !editable.value ||
    structureLocked.value ||
    index + step < 0 ||
    index + step >= local.value.slides.length
  )
    return;
  const [sl] = local.value.slides.splice(index, 1);
  local.value.slides.splice(index + step, 0, sl);
}
async function versions(more = false) {
  await run(async () => {
    const offset = more ? historyOffset.value : 0;
    const rows = await props.api('/tasks/' + props.task.id + '/revisions?offset=' + offset);
    history.value = more ? history.value.concat(rows.slice(0, 50)) : rows.slice(0, 50);
    historyMore.value = rows.length > 50;
    historyOffset.value = offset + Math.min(rows.length, 50);
    showHistory.value = true;
    await reveal('.studio-history');
  });
}
async function viewVersion(version) {
  await run(async () => {
    historyDraft.value = await props.api('/tasks/' + props.task.id + '/revisions/' + version);
    await reveal('.studio-history details');
  });
}
async function restore(version) {
  if (!window.confirm('恢复为新版本，替换当前未保存修改。旧版本保留，恢复免费。继续吗？')) return;
  await run(async () => {
    await props.api('/tasks/' + props.task.id + '/revisions/' + version + '/restore', 'POST', {
      revision: props.task.revision
    });
    await props.refresh();
    await loadMessages();
    showHistory.value = false;
  });
}
onMounted(() => {
  pollTimer = setInterval(async () => {
    if (working.value || !pendingMessage.value) return;
    try {
      await loadMessages();
      await props.refresh();
    } catch (e) {
      error.value = e.message;
    }
  }, 2000);
});
onUnmounted(() => {
  disposed = true;
  imageVersion++;
  conversationVersion++;
  clearInterval(pollTimer);
  clearImages();
});
</script>

<template>
  <section class="courseware-studio" aria-label="对话课件工作台">
    <ClassroomPresentation
      v-if="presenting && local"
      :deck="local"
      :images="imageURLs"
      :saved="!dirty"
      :token="token"
      @close="presenting = false"
    />
    <header class="studio-header">
      <div class="studio-document-title">
        <span class="studio-document-icon"><WorkspaceIcon name="slides" /></span>
        <div>
          <h2>{{ local?.title || task.input?.topic }}</h2>
          <p>
            {{ title }}<template v-if="local"> · {{ local.slides.length }} 页</template>
          </p>
        </div>
      </div>
      <div class="studio-header-actions">
        <button v-if="finished && local" @click="presenting = true">
          <WorkspaceIcon name="slides" />课堂演示
        </button>
        <details class="studio-more-actions">
          <summary>更多<WorkspaceIcon name="menu" /></summary>
          <div>
            <button @click="emit('process')">查看过程</button
            ><button v-if="local" @click="versions(false)" :disabled="working">版本历史</button
            ><button
              @click="emit('new-work')"
              :disabled="working || !['succeeded', 'failed', 'cancelled'].includes(task.status)"
            >
              新建课件</button
            ><button v-if="finished" @click="download(false, undefined, 'md')" :disabled="working">
              下载教案 MD
            </button>
          </div>
        </details>
        <button
          v-if="local && (ready || paused)"
          @click="download(!finished)"
          :disabled="working"
          class="primary"
        >
          <WorkspaceIcon name="download" />{{ finished ? '导出 PPTX' : '下载预览 PPTX' }}
        </button>
      </div>
    </header>
    <p class="studio-work-next">{{ reviewNext }}</p>
    <p class="studio-saved-note">已保存至“我的作品” · 已提交的调整与制作进度会保留。</p>
    <ol class="studio-phase-list" aria-label="课件制作阶段">
      <li
        v-for="(phase, index) in ['三页预览', '完整制作', '逐页审核', '交付与调整']"
        :key="phase"
        :class="{ current: reviewPhase === index, complete: reviewPhase > index }"
        :aria-current="reviewPhase === index ? 'step' : undefined"
      >
        <span
          ><WorkspaceIcon v-if="reviewPhase > index" name="check" /><template v-else>{{
            index + 1
          }}</template></span
        >{{ phase }}
      </li>
    </ol>
    <div class="studio-body">
      <nav v-if="local" class="studio-page-rail" aria-label="课件页面导航">
        <div class="studio-pages-heading">
          <b>页面</b><span>{{ local.slides.length }} 页</span>
        </div>
        <div class="studio-page-thumbnails">
          <button
            v-for="(sl, i) in local.slides"
            :key="sl.id"
            type="button"
            class="studio-page-thumbnail"
            :class="{ active: selected === sl.id }"
            :aria-label="'跳转第' + (i + 1) + '页：' + sl.title"
            :aria-current="selected === sl.id ? 'page' : undefined"
            @click="selectSlide(sl.id)"
          >
            <span class="studio-thumbnail-art"
              ><img v-if="imageURLs[sl.id]" :src="imageURLs[sl.id]" alt="" /><span
                v-else
                class="studio-thumbnail-fallback"
                ><WorkspaceIcon name="slides" /><strong>{{ sl.title }}</strong></span
              ><span v-if="meta(sl.id).approved && !changed(sl)" class="studio-thumbnail-approved"
                ><WorkspaceIcon name="check" /></span></span
            ><span class="studio-thumbnail-label"
              ><b>{{ i + 1 }}</b
              ><span>{{ changed(sl) ? '待保存' : meta(sl.id).approved ? '已确认' : '待审核' }}</span
              ><small :title="qualityStatus(sl)">{{ qualityLabel(sl) }}</small></span
            >
          </button>
        </div>
      </nav>
      <aside class="studio-conversation" aria-label="课件审核与调整">
        <header class="studio-adjust-heading" :data-current-slide-id="current?.id || ''">
          <h2>{{ finished ? '交付与调整' : '审核与调整' }}</h2>
          <small v-if="current"
            >教学位置 {{ currentPosition }} · 预览第 {{ currentIndex + 1 }} /
            {{ local.slides.length }} 页</small
          >
        </header>
        <div class="studio-chat-panel">
          <form class="chat-composer" @submit.prevent="send">
            <div class="chat-mode">
              <button
                type="button"
                :class="{ active: chatMode === 'discuss' }"
                @click="chatMode = 'discuss'"
                :disabled="working"
              >
                讨论课件</button
              ><button
                type="button"
                :class="{ active: chatMode === 'revise' }"
                @click="chatMode = 'revise'"
                :disabled="working"
              >
                修改本页
              </button>
            </div>
            <small v-if="chatMode === 'revise'"
              >{{
                current
                  ? '当前教学位置' +
                    currentPosition +
                    ' · 第' +
                    (currentIndex + 1) +
                    '页：' +
                    current.title
                  : '在左侧选择需要修改的页'
              }}
              · {{ finished ? '成功改写5积分' : '草稿阶段不另扣业务积分' }}</small
            >
            <label class="studio-adjust-label"
              >{{ chatMode === 'revise' ? '告诉 AI 怎样调整当前页' : '与 AI 讨论课件'
              }}<textarea
                v-model="chatText"
                rows="3"
                maxlength="600"
                :disabled="!ready || working || pendingMessage"
                aria-label="继续课件对话"
                :placeholder="
                  chatMode === 'revise'
                    ? '例如：把这一页改成实验探究，补充控制变量和安全提示。'
                    : '例如：这三页是否适合八年级学生？有哪些概念需要先解释？'
                "
              ></textarea>
            </label>
            <div class="chat-send-row">
              <span
                >{{ task.mode === 'sample' ? '样例模式，不调用真实模型' : '对话请求会调用文本模型'
                }}<br />只修改选中的页面</span
              ><button
                class="primary"
                :disabled="!ready || working || pendingMessage || !chatText.trim()"
              >
                {{
                  working || pendingMessage
                    ? '处理中…'
                    : chatMode === 'revise'
                      ? '修改当前页'
                      : '发送讨论'
                }}
              </button>
            </div>
          </form>
          <details
            :open="discussionOpen"
            @toggle="discussionOpen = $event.target.open"
            class="studio-discussion-history"
          >
            <summary>
              查看调整与讨论记录<span v-if="messages.length"> · {{ messages.length }}</span>
            </summary>
            <div class="conversation-log" aria-live="polite">
              <div class="chat-bubble assistant">
                <p>
                  {{
                    prefix
                      ? '先看看这三页。有什么需要调整的，可以直接告诉我。'
                      : '对当前页面有新的教学想法？可以讨论，也可以指定修改。'
                  }}
                </p>
                <small>{{
                  task.mode === 'sample' ? '本地样例，不调用真实模型' : '讨论与修改会调用真实模型'
                }}</small>
              </div>
              <template v-for="m in messages" :key="m.id">
                <div class="chat-bubble user">
                  <small>{{ m.mode === 'revise' ? '修改指定页' : '对话讨论' }}</small>
                  <p>{{ m.content }}</p>
                </div>
                <div class="chat-bubble assistant">
                  <p v-if="m.status === 'failed'" class="chat-failure">
                    {{ m.error || '这次回复未完成，原页面保留。' }}
                  </p>
                  <p v-else>
                    {{ m.reply || (m.status === 'pending' ? '正在理解你的要求…' : '回复已完成') }}
                  </p>
                  <small v-if="m.child_id && m.status === 'completed'"
                    >单页AI修改已写入新版本，请审核更新后的页面。</small
                  >
                </div>
              </template>
            </div>
          </details>
        </div>
        <div v-if="current" class="inspector-edit-actions">
          <button type="button" @click="editSlide(current.id)" :disabled="!editable">
            <WorkspaceIcon name="edit" />手动编辑内容</button
          ><button
            type="button"
            @click="approve(current.id, !meta(current.id).approved)"
            :disabled="!editable || !ready"
            :class="{ approved: meta(current.id).approved }"
          >
            <WorkspaceIcon name="check" />{{
              meta(current.id).approved ? '撤销本页确认' : '本页审核通过'
            }}
          </button>
        </div>
        <p class="inspector-review-foot">修改后需要重新审核当前页。</p>
        <footer v-if="local" class="studio-completion">
          <div>
            <b>{{ approvedCount }} / {{ local.slides.length }} 页已审核</b>
            <p>
              {{
                prefix
                  ? '确认三页质量预览后，再制作其余页面；仍只预留20积分。'
                  : finished
                    ? '已交付。免费手改与模板重导出保留；AI单页改写成功后5积分。'
                    : '所有页面审核通过后才完整交付，成功后结算20积分。'
              }}
            </p>
          </div>
          <button
            v-if="ready"
            class="studio-approve-all"
            @click="approveAll"
            :disabled="!editable || allApproved"
            aria-label="一键审核通过"
            title="确认当前已生成页面"
          >
            {{ working ? '处理中…' : allApproved ? '全部已审核' : '一键审核通过' }}</button
          ><button
            v-if="prefix"
            class="primary"
            @click="expand"
            :disabled="working || pendingMessage || !allApproved"
          >
            确认预览，制作其余 {{ generationProgress.total - 3 }} 页 →</button
          ><template v-else-if="task.status === 'awaiting_confirmation'"
            ><label class="consent"
              ><input
                v-model="consent"
                type="checkbox"
              />我已核对内容与引用，知悉待核验内容及20积分费用</label
            ><button
              class="primary"
              @click="finish"
              :disabled="working || pendingMessage || !allApproved || !consent"
            >
              全部审核通过，完整交付 →
            </button></template
          ><button
            v-if="!['succeeded', 'failed', 'cancelled'].includes(task.status)"
            @click="cancel"
            :disabled="working"
          >
            取消并释放预留
          </button>
        </footer>
        <button
          v-if="ready && local?.plan"
          class="studio-recheck"
          @click="recheck"
          :disabled="!editable"
        >
          {{ working ? '处理中…' : '重新检查内容与画面' }}
        </button>
        <small v-if="ready && local?.plan" class="studio-recheck-help">
          按当前核验设置检查，保留已有页面；不扣积分，也不会自动审核或继续制作。
        </small>
        <CoursewareReviewDetails
          :current="current"
          :current-position="currentPosition"
          :current-index="currentIndex"
          :local="local"
          :task="task"
          :editable="editable"
          :deck-notice="deckNotice"
          :meta="meta"
          :changed="changed"
          :quality-status="qualityStatus"
        />
      </aside>
      <div
        ref="canvas"
        class="studio-canvas"
        role="region"
        aria-label="课件页面预览"
        :data-current-slide-id="current?.id || ''"
        tabindex="0"
      >
        <div v-if="!finished && !ready" class="studio-stage-banner" role="status">
          <b>{{ title }}</b>
          <p v-if="prefix">确认这三页的内容与风格后，再制作其余页面。</p>
          <p v-else-if="task.status === 'awaiting_confirmation'">核对每页内容后，确认完整交付。</p>
          <p v-else-if="task.status === 'failed'">
            {{ task.error
            }}<template v-if="paused">
              可先手动修改内容、确认配图或上传替换，保存后再继续制作；保存不重启任务、不重新预留积分。</template
            >
          </p>
          <p v-else>{{ task.events?.at(-1)?.message || '正在准备课件，页面会自动更新。' }}</p>
          <div v-if="!ready" class="studio-progress">
            <div>
              <b>{{ generationProgress.text }}</b
              ><span>内容生成 {{ generationProgress.percent }}%</span>
            </div>
            <progress
              :value="generationProgress.percent"
              max="100"
              aria-label="内容生成进度"
            ></progress
            ><small
              >{{ generationProgress.detail }} · 已通过检查 {{ generationProgress.completed }} /
              {{ generationProgress.total }} 页 · 教师确认后交付</small
            >
          </div>
        </div>
        <p v-if="error" class="alert error" role="alert">{{ error }}</p>
        <p v-if="notice" class="alert success" role="status">{{ notice }}</p>
        <button
          v-if="task.status === 'failed' || (task.status === 'cancelled' && task.restored_at)"
          class="primary"
          @click="emit('retry')"
          :disabled="working || retrying || available < 20"
        >
          {{ retrying ? '正在恢复…' : '继续制作 · 重新预留20积分' }}
        </button>
        <p
          v-if="
            (task.status === 'failed' || (task.status === 'cancelled' && task.restored_at)) &&
            available < 20
          "
          class="fine"
        >
          继续制作需要20可用积分，请先到套餐与账单补充积分。
        </p>
        <template v-if="local">
          <details class="studio-deck-settings" :open="deckSettings">
            <summary @click.prevent="deckSettings = !deckSettings">
              <WorkspaceIcon name="settings" />课件设置与模板<span>{{
                templates.find((t) => t.id === local.theme)?.name
              }}</span>
            </summary>
            <div class="studio-toolbar">
              <label
                >课件标题<input
                  v-model="local.title"
                  :readonly="!editable"
                  maxlength="100" /></label
              ><label
                >视觉模板<select v-model="local.theme" :disabled="!editable">
                  <option v-for="t in templates" :key="t.id" :value="t.id">{{ t.name }}</option>
                </select></label
              ><button @click="run(persist)" :disabled="!editable || !dirty || !!textIssue">
                保存修改{{ finished ? '并免费重导出' : '' }}
              </button>
            </div>
            <section class="studio-templates" aria-label="横向课件模板">
              <div class="template-heading">
                <h3>选择课件模板</h3>
                <small>{{ templates.find((t) => t.id === local.theme)?.name }}</small>
              </div>
              <div class="themes horizontal-themes">
                <label
                  v-for="t in templates"
                  :key="t.id"
                  :class="['theme', t.id, { chosen: local.theme === t.id }]"
                  :title="t.description"
                  ><input
                    type="radio"
                    v-model="local.theme"
                    :value="t.id"
                    name="studio-theme"
                    :disabled="!editable"
                  /><span class="theme-mini" aria-hidden="true"
                    ><b>{{ t.mark }}</b
                    ><i></i><i></i><em>课堂探索</em></span
                  ><strong>{{ t.name }}</strong
                  ><small>{{ t.tag }}</small></label
                >
              </div>
            </section>
          </details>
          <p v-if="textIssue" class="canvas-text-warning alert error" role="alert">
            {{ textIssue }}
          </p>
          <p v-if="dirty" class="unsaved-note" role="status">
            存在未保存修改：截图仍为已保存版本，变更页面需重新审核。<button
              v-if="editRevision !== task.revision"
              @click="
                local = JSON.parse(JSON.stringify(task.result));
                editBaseline = JSON.stringify(local);
                editRevision = task.revision;
              "
              :disabled="working"
            >
              放弃本地修改，查看最新版本
            </button>
          </p>
          <article
            v-for="(sl, i) in local.slides"
            v-show="selected === sl.id"
            :key="sl.id"
            :class="['studio-slide', { selected: selected === sl.id }]"
            :data-slide-id="sl.id"
          >
            <div class="studio-slide-top">
              <button
                class="slide-select"
                @click="selectSlide(sl.id)"
                :aria-pressed="selected === sl.id"
              >
                教学位置 {{ sl.order || i + 1 }} · 预览第 {{ i + 1 }} / {{ local.slides.length }}
                <span>{{
                  changed(sl) ? '有修改，待保存' : meta(sl.id).approved ? '审核通过' : '待审核'
                }}</span>
              </button>
              <div class="slide-tools">
                <button
                  v-if="canvasAvailable && !paused && imageURLs[sl.id]"
                  @click="openObjectCanvas(sl)"
                  :disabled="!editable"
                  :aria-label="'移动第' + (i + 1) + '页对象'"
                  title="编辑文字、图片、图形与图层"
                  :aria-expanded="objectEditing === sl.id"
                >
                  {{ objectEditing === sl.id ? '返回预览' : '编辑画布' }}</button
                ><button
                  v-else
                  @click="editSlide(sl.id)"
                  :disabled="!editable"
                  :aria-label="'编辑第' + (i + 1) + '页'"
                >
                  <WorkspaceIcon name="edit" />编辑</button
                ><button
                  @click="moveSlide(i, -1)"
                  :disabled="!editable || structureLocked || i === 0"
                  :aria-label="'上移第' + (i + 1) + '页'"
                >
                  <WorkspaceIcon name="up" /></button
                ><button
                  @click="moveSlide(i, 1)"
                  :disabled="!editable || structureLocked || i === local.slides.length - 1"
                  :aria-label="'下移第' + (i + 1) + '页'"
                >
                  <WorkspaceIcon name="down" /></button
                ><button
                  @click="addSlide(i)"
                  :disabled="!editable || structureLocked || local.slides.length >= 100"
                  :aria-label="'在第' + (i + 1) + '页后添加'"
                >
                  <WorkspaceIcon name="plus" /></button
                ><button
                  @click="duplicateSlide(i)"
                  :disabled="!editable || structureLocked || local.slides.length >= 100"
                  :aria-label="'复制第' + (i + 1) + '页'"
                >
                  <WorkspaceIcon name="copy" /></button
                ><button
                  @click="removeSlide(i)"
                  :disabled="
                    !editable || structureLocked || local.slides.length <= (local.plan ? 3 : 8)
                  "
                  :aria-label="'删除第' + (i + 1) + '页'"
                >
                  <WorkspaceIcon name="trash" />
                </button>
              </div>
            </div>
            <div
              v-if="
                !sl.picture &&
                sl.image_search &&
                ['needs_confirmation', 'unavailable', 'not_found'].includes(sl.image_search.status)
              "
              class="studio-image-attention"
              role="status"
            >
              <p>{{ sl.image_search.message }}</p>
              <button type="button" :disabled="!editable" @click="editSlide(sl.id)">
                {{
                  sl.image_search.candidates?.length
                    ? '查看候选图并处理配图'
                    : '检索、上传或跳过配图'
                }}
              </button>
            </div>
            <button
              v-if="objectEditing !== sl.id"
              class="studio-slide-picture"
              @click="selectSlide(sl.id)"
              :aria-label="'选择第' + (i + 1) + '页预览'"
            >
              <img
                v-if="imageURLs[sl.id]"
                :src="imageURLs[sl.id]"
                :alt="'第' + (i + 1) + '页实际PPT截图：' + sl.title"
              />
              <div v-else :class="['slide-fallback', local.theme]">
                <small>布局示意 · 可下载实际PPTX</small>
                <h3>{{ sl.title }}</h3>
                <ul>
                  <li v-for="(b, j) in sl.bullets" :key="j">{{ b }}</li>
                </ul>
                <CoursewareEvidence :slide="sl" preview />
                <p v-if="sl.formula">{{ sl.formula }}</p>
              </div>
            </button>
            <small v-if="objectEditing !== sl.id" class="actual-preview-caption">{{
              imageURLs[sl.id]
                ? '实际PPTX渲染截图 · 修改保存后更新'
                : paused
                  ? '待修复草稿，当前版本暂无真实PPT截图；上方为布局示意'
                  : '当前版本暂无真实PPT截图；上方为布局示意'
            }}</small>
            <template
              v-if="objectEditing === sl.id && objectMaps[sl.id]?.length && imageURLs[sl.id]"
              ><SlideObjectCanvas
                :key="task.id + ':' + editRevision + ':' + sl.id"
                :slide="sl"
                :objects="objectMaps[sl.id]"
                :image="imageURLs[sl.id]"
                :editable="editable"
                :complete="canvasComplete"
                :preview="previewCanvas"
                :preview-key="
                  JSON.stringify([local.title, local.theme, local.slides.map((s) => s.id)])
                "
              /><button
                @click="run(persist)"
                :disabled="!editable || !dirty || !!textIssue"
                class="primary"
              >
                保存画布及其他修改
              </button></template
            >
            <details
              v-if="objectEditing === sl.id || editing === sl.id"
              :open="editing === sl.id"
              class="studio-content-settings"
            >
              <summary>教学内容与讲解设置</summary>
              <div class="studio-slide-editor">
                <label>标题<input v-model="sl.title" maxlength="70" :readonly="!editable" /></label>
                <div class="editor-row">
                  <label
                    >版式<select v-model="sl.layout" :disabled="!editable">
                      <option v-for="l in layouts" :key="l[0]" :value="l[0]">{{ l[1] }}</option>
                    </select></label
                  ><label
                    >章节<input v-model="sl.section" maxlength="80" :readonly="!editable"
                  /></label>
                </div>
                <label
                  >正文（每行一条要点）<textarea
                    :value="sl.bullets.join('\n')"
                    @input="sl.bullets = $event.target.value.split('\n').filter((s) => s.trim())"
                    rows="4"
                    :readonly="!editable"
                  ></textarea></label
                ><label
                  >课堂答案（每行一步，默认隐藏）<textarea
                    v-model="sl.answer"
                    :readonly="!editable"
                    maxlength="2000"
                    rows="3"
                  ></textarea></label
                ><label
                  >讲解备注与答案<textarea
                    v-model="sl.notes"
                    rows="5"
                    :readonly="!editable"
                  ></textarea>
                </label>
                <div class="editor-row">
                  <label>LaTeX公式<input v-model="sl.formula" :readonly="!editable" /></label
                  ><label
                    >固定示意图<select
                      v-model="sl.diagram"
                      :disabled="!editable || !!sl.graph || !!sl.scientific"
                    >
                      <option value="">不使用</option>
                      <option value="triangle">直角三角形</option>
                      <option value="buoyancy">浮力方向</option>
                    </select></label
                  >
                </div>
                <TeachingPlotEditor :slide="sl" :editable="editable" />
                <CoursewareEvidence
                  :slide="sl"
                  :editable="editable"
                  :task-id="task.id"
                  :revision="editRevision"
                  :use-web="task.input?.use_web === true"
                />
                <button
                  @click="run(persist)"
                  :disabled="!editable || !dirty || !!textIssue"
                  class="primary"
                >
                  保存本页及其他修改
                </button>
              </div>
            </details>
            <div class="studio-slide-bottom">
              <div class="studio-page-turn">
                <button
                  type="button"
                  @click="selectSlide(local.slides[i - 1].id)"
                  :disabled="i === 0"
                  aria-label="上一页"
                >
                  <WorkspaceIcon name="left" /></button
                ><span>{{ i + 1 }} / {{ local.slides.length }}</span
                ><button
                  type="button"
                  @click="selectSlide(local.slides[i + 1].id)"
                  :disabled="i === local.slides.length - 1"
                  aria-label="下一页"
                >
                  <WorkspaceIcon name="arrow" />
                </button>
              </div>
              <div>
                <button @click="discussSlide(sl.id)" :disabled="!editable || !ready">
                  对话修改本页</button
                ><button
                  class="approve-page"
                  @click="approve(sl.id, !meta(sl.id).approved)"
                  :disabled="!editable || !ready"
                  :class="{ approved: meta(sl.id).approved }"
                >
                  <WorkspaceIcon name="check" />{{
                    meta(sl.id).approved ? '撤销本页确认' : '本页审核通过'
                  }}
                </button>
              </div>
              <div v-if="meta(sl.id).vote < 0" class="legacy-review-note">
                <small>旧改进意见：{{ meta(sl.id).reason }}。修改后请重新审核。</small>
              </div>
            </div>
          </article>
          <section v-if="showHistory" class="studio-history">
            <h3>版本历史</h3>
            <p>保留最近20个版本及首个版本，当前成果保留；已交付课件可以免费恢复完整版本。</p>
            <div v-for="v in history" :key="v.revision">
              <span>v{{ v.revision }} · {{ v.pages }}页 · {{ v.title }}</span
              ><button @click="viewVersion(v.revision)" :disabled="working">查看内容</button
              ><button @click="download(false, v.revision)" :disabled="working">下载旧版</button
              ><button
                v-if="finished && v.pages >= (local.plan ? 3 : 8)"
                @click="restore(v.revision)"
                :disabled="!editable"
              >
                免费恢复
              </button>
            </div>
            <button v-if="historyMore" @click="versions(true)" :disabled="working">
              加载更多版本
            </button>
            <details v-if="historyDraft" open>
              <summary>
                v{{ historyDraft.revision }} · {{ historyDraft.result.title }}（只读）
              </summary>
              <p class="history-text">{{ historyDraft.result.lesson }}</p>
              <article v-for="(sl, i) in historyDraft.result.slides" :key="i">
                <h4>{{ i + 1 }} · {{ sl.title }}</h4>
                <ul>
                  <li v-for="(b, j) in sl.bullets" :key="j">{{ b }}</li>
                </ul>
                <CoursewareEvidence :slide="sl" preview />
                <p v-if="sl.formula">{{ sl.formula }}</p>
                <p class="history-text">{{ sl.notes }}</p>
              </article>
            </details>
            <button
              @click="
                showHistory = false;
                historyDraft = null;
              "
            >
              关闭
            </button>
          </section>
        </template>
        <div v-else class="studio-wait">
          <template v-if="['failed', 'cancelled'].includes(task.status)"
            ><h3>{{ task.status === 'failed' ? '本次尚无可用预览' : '任务已取消' }}</h3>
            <p>原预留已释放，未扣生成积分。可以继续制作或新建课件。</p></template
          ><template v-else
            ><span class="spinner"></span>
            <h3>整理资料与整课规划</h3>
            <p>正在准备三页质量预览。</p></template
          >
        </div>
      </div>
    </div>
  </section>
</template>

<style scoped>
.studio-templates {
  padding: 20px 0;
  margin-top: 22px;
  border-top: 1px solid #e8ebf1;
  min-width: 0;
}
.studio-templates h3 {
  font-size: 16px;
  margin: 0;
}
.studio-templates small {
  font-size: 11px;
  color: #70836d;
}
.courseware-studio {
  background: #fff;
  border: 1px solid #e5e9ed;
  border-radius: 20px;
  overflow: hidden;
  box-shadow: 0 12px 50px #27304b08;
  min-width: 0;
}
.studio-header {
  padding: 24px 28px;
  border-bottom: 1px solid #e8ebf1;
  display: flex;
  justify-content: space-between;
  gap: 20px;
  align-items: center;
}
.studio-header h2 {
  font-size: 25px;
  margin: 7px 0;
}
.studio-header p {
  margin: 0;
  color: #74808d;
}
.studio-header-actions {
  display: flex;
  gap: 7px;
  flex-wrap: wrap;
  justify-content: flex-end;
}
.studio-header-actions button {
  padding: 9px 12px;
  font-size: 12px;
}
.studio-body {
  display: grid;
  grid-template-columns: minmax(270px, 340px) minmax(0, 1fr);
}
.studio-conversation {
  background: #f8f9fd;
  border-right: 1px solid #e8ebf1;
  padding: 22px;
  display: flex;
  flex-direction: column;
  position: sticky;
  top: 0;
  align-self: start;
  height: calc(100vh - 150px);
  min-height: 540px;
  max-height: 940px;
}
.conversation-heading {
  display: flex;
  gap: 11px;
  align-items: center;
  margin-bottom: 20px;
}
.conversation-heading b {
  font-size: 15px;
}
.conversation-heading small {
  display: block;
  margin-top: 4px;
  color: #8490a1;
  font-size: 11px;
}
.studio-avatar {
  width: 36px;
  height: 36px;
  display: grid;
  place-items: center;
  background: #ece5ff;
  color: #7548c4;
  border-radius: 12px;
  font-size: 22px;
}
.conversation-log {
  flex: 1;
  overflow-y: auto;
  padding-right: 4px;
  scrollbar-width: thin;
  min-height: 180px;
}
.chat-bubble {
  padding: 15px 17px;
  border-radius: 14px;
  margin-bottom: 17px;
  font-size: 13px;
  line-height: 1.7;
  overflow-wrap: anywhere;
  white-space: pre-wrap;
}
.chat-bubble p {
  margin: 5px 0;
}
.chat-bubble.user {
  background: #efedf8;
  margin-left: 12px;
  color: #35334a;
}
.chat-bubble.assistant {
  background: #fff;
  border: 1px solid #e7e7f1;
  margin-right: 5px;
}
.chat-bubble small {
  font-size: 10px;
  color: #8990a3;
}
.chat-artifact {
  display: flex;
  gap: 12px;
  align-items: center;
  border: 1px solid #ddd1fa;
  border-radius: 10px;
  padding: 12px;
  margin-top: 14px;
  color: #7044b9;
}
.chat-artifact > span {
  font-size: 24px;
}
.chat-artifact small {
  display: block;
  margin-top: 4px;
}
.chat-composer {
  padding: 15px;
  background: #fff;
  border: 1px solid #ded9ef;
  border-radius: 14px;
  margin-top: 12px;
  box-shadow: 0 6px 18px #5d399906;
}
.chat-composer textarea {
  border: 0;
  box-shadow: none !important;
  resize: vertical;
  padding: 10px 0;
  font-size: 12px;
  background: transparent;
  min-height: 88px;
}
.chat-mode {
  display: flex;
  gap: 6px;
  margin-bottom: 8px;
}
.chat-mode button {
  padding: 6px 9px;
  font-size: 11px;
  border: 0;
  background: #f2f4f8;
}
.chat-mode button.active {
  background: #e9e1fb;
  color: #7044b9;
}
.chat-composer > small {
  font-size: 10px;
  color: #7c7b8a;
  display: block;
}
.chat-send-row {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 6px;
}
.chat-send-row span {
  font-size: 9px;
  color: #9096a4;
}
.chat-send-row button {
  padding: 8px 13px;
  font-size: 12px;
}
.studio-canvas {
  padding: 24px 28px 0;
  min-width: 0;
}
.studio-stage-banner {
  padding: 17px 21px;
  border-radius: 12px;
  background: #f1f7f4;
  border: 1px solid #d8e8df;
  margin-bottom: 20px;
  font-size: 13px;
}
.studio-stage-banner p {
  margin: 7px 0;
  line-height: 1.6;
  color: #62756b;
}
.studio-stage-banner span {
  font-size: 11px;
  color: #547565;
}
.studio-toolbar {
  display: flex;
  align-items: end;
  gap: 12px;
  margin: 15px 0;
}
.studio-toolbar label {
  font-size: 10px;
  flex: 1;
}
.studio-toolbar label:first-child {
  flex: 2;
}
.studio-toolbar input,
.studio-toolbar select {
  margin-top: 5px;
}
.studio-toolbar button {
  flex-shrink: 0;
  padding: 11px 13px;
  font-size: 11px;
}
.studio-note {
  font-size: 11px;
  color: #8a94a2;
  line-height: 1.7;
}
.unsaved-note {
  font-size: 12px;
  color: #977023;
  background: #fff7e2;
  padding: 12px;
  border-radius: 9px;
}
.studio-lesson,
.studio-history {
  font-size: 12px;
  margin: 20px 0;
  border: 1px solid #e8ebf1;
  border-radius: 12px;
  padding: 15px;
}
.studio-lesson summary {
  cursor: pointer;
  color: #697488;
}
.studio-lesson label {
  margin-top: 16px;
}
.studio-lesson article {
  border-top: 1px solid #e9edf1;
  padding: 12px 0;
}
.studio-lesson article small {
  display: block;
  margin-top: 5px;
  color: #8993a0;
}
.studio-lesson article p {
  white-space: pre-wrap;
}
.studio-history > div {
  display: flex;
  flex-wrap: wrap;
  gap: 10px;
  margin: 8px 0;
  align-items: center;
}
.studio-history button {
  padding: 7px 12px;
}
.studio-slide {
  margin-bottom: 30px;
  scroll-margin-top: 30px;
}
.studio-slide-top {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 8px;
  margin-bottom: 10px;
}
.slide-select {
  border: none;
  background: transparent;
  padding: 0;
  color: #8a91a4;
  font-size: 11px;
  text-align: left;
}
.slide-select span {
  margin-left: 10px;
  font-size: 10px;
  background: #f2f4f8;
  border-radius: 6px;
  padding: 3px 6px;
}
.slide-tools {
  display: flex;
  gap: 3px;
}
.slide-tools button {
  padding: 4px 7px;
  font-size: 12px;
  border: 0;
  background: transparent;
  color: #8490a3;
}
.studio-slide-picture {
  display: block;
  width: 100%;
  padding: 0;
  border: 2px solid #e3e6f0;
  border-radius: 15px;
  overflow: hidden;
  background: #fff;
  box-shadow: 0 5px 18px #25375909;
  text-align: left;
}
.studio-slide.selected .studio-slide-picture {
  border-color: #9b7bdb;
  box-shadow: 0 0 0 3px #9b7bdb15;
}
.studio-slide-picture img {
  display: block;
  width: 100%;
  aspect-ratio: 16/9;
  object-fit: contain;
}
.actual-preview-caption {
  display: block;
  font-size: 9px;
  color: #99a1b0;
  margin: 7px 2px;
}
.slide-fallback {
  aspect-ratio: 16/9;
  padding: 7%;
  display: flex;
  flex-direction: column;
  justify-content: center;
  overflow: hidden;
  background: #f1f7f4;
  color: #163e30;
}
.slide-fallback h3 {
  font-size: clamp(20px, 2.3vw, 38px);
  margin: 10px 0;
}
.slide-fallback ul {
  font-size: clamp(12px, 1.2vw, 19px);
  line-height: 1.7;
  padding-left: 22px;
}
.slide-fallback small {
  font-size: 10px;
  color: #8e98a7;
}
.slide-fallback.midnight {
  background: #101b2c;
  color: #e7ebf3;
}
.slide-fallback.violet {
  background: #f3effb;
  color: #3c2167;
}
.slide-fallback.ocean {
  background: #edf4ff;
  color: #1f4c8b;
}
.slide-fallback.paper {
  background: #faf4e8;
  color: #52442b;
}
.slide-fallback.coral {
  background: #fff0e8;
  color: #9a3e2a;
}
.studio-slide-bottom {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
  flex-wrap: wrap;
  margin-top: 12px;
}
.studio-slide-bottom > div {
  display: flex;
  gap: 6px;
  align-items: center;
  flex-wrap: wrap;
}
.studio-slide-bottom button {
  font-size: 11px;
  padding: 7px 10px;
}
.studio-slide-bottom .approved {
  color: #32745a;
  background: #e8f5ed;
  border-color: #b6ddc8;
}
.studio-slide-editor {
  background: #fafbfe;
  border: 1px solid #e6eaf3;
  border-radius: 12px;
  margin: 14px 0;
  padding: 20px;
  font-size: 12px;
}
.studio-slide-editor label {
  margin-bottom: 12px;
}
.editor-row {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 12px;
}
.studio-slide-editor button {
  padding: 9px 12px;
  font-size: 12px;
}
.studio-completion {
  margin-top: 20px;
  padding: 22px 0 25px;
  border-top: 1px solid #e6eaef;
  display: flex;
  flex-wrap: wrap;
  gap: 12px;
  align-items: center;
  background: #fff;
  position: sticky;
  bottom: 0;
  box-shadow: 0 -10px 20px #ffffffc4;
}
.studio-completion > div {
  flex: 1;
  min-width: 200px;
}
.studio-completion b {
  font-size: 14px;
}
.studio-completion p {
  font-size: 10px;
  color: #80908b;
  line-height: 1.6;
  max-width: 440px;
}
.studio-completion button {
  font-size: 11px;
  padding: 12px 16px;
}
.studio-completion .consent {
  font-size: 11px;
  max-width: 330px;
}
.studio-wait {
  text-align: center;
  padding: 100px 20px;
  color: #7a8998;
}
.studio-wait h3 {
  font-size: 20px;
}
.studio-wait p {
  font-size: 12px;
}
.chat-failure {
  color: #a77554;
}
/* Bound the shared workspace, not each slide: chat and PPT scroll independently. */
.courseware-studio {
  display: flex;
  flex-direction: column;
  height: calc(100dvh - 96px);
  min-height: 480px;
}
.studio-header {
  flex-shrink: 0;
}
.studio-body {
  flex: 1;
  min-height: 0;
  overflow: hidden;
}
.studio-conversation {
  position: static;
  align-self: stretch;
  height: auto;
  min-height: 0;
  max-height: none;
}
.conversation-heading,
.chat-composer {
  flex-shrink: 0;
}
.conversation-log {
  min-height: 0;
  overscroll-behavior-y: contain;
}
.studio-canvas {
  min-height: 0;
  overflow-y: auto;
  overscroll-behavior-y: contain;
  scrollbar-gutter: stable;
  scrollbar-width: thin;
}
.studio-canvas:focus-visible {
  outline: 2px solid #9b7bdb;
  outline-offset: -3px;
}
@media (max-width: 1150px) {
  .studio-body {
    grid-template-columns: 270px minmax(0, 1fr);
  }
  .studio-conversation {
    padding: 16px;
  }
  .studio-canvas {
    padding: 20px 18px 0;
  }
  .studio-toolbar {
    flex-wrap: wrap;
  }
  .studio-slide-bottom > div {
    width: 100%;
  }
  .studio-header {
    padding: 20px;
  }
  .studio-header-actions {
    max-width: 230px;
  }
  .slide-tools button {
    padding: 4px 5px;
  }
}
@media (max-width: 760px) {
  .studio-body {
    display: flex;
    flex-direction: column;
  }
  .studio-header {
    align-items: start;
    flex-direction: column;
    padding: 17px;
  }
  .studio-header-actions {
    max-width: none;
    justify-content: start;
  }
  .studio-conversation {
    position: static;
    height: auto;
    max-height: 580px;
    min-height: 360px;
    border-right: 0;
    border-bottom: 1px solid #e8ebf1;
  }
  .conversation-log {
    max-height: 240px;
  }
  .studio-canvas {
    padding: 18px 13px 0;
  }
  .studio-slide-top {
    align-items: start;
    flex-direction: column;
  }
  .slide-tools {
    align-self: end;
  }
  .studio-toolbar label {
    min-width: 110px;
  }
  .studio-slide-editor {
    padding: 13px;
  }
  .editor-row {
    grid-template-columns: 1fr;
  }
  .studio-completion {
    position: static;
  }
  .studio-completion .consent {
    max-width: none;
  }
  .studio-completion button {
    width: 100%;
  }
  .slide-fallback {
    min-height: 240px;
    aspect-ratio: auto;
  }
}
.history-text {
  white-space: pre-wrap;
  line-height: 1.8;
  overflow-wrap: anywhere;
}
.studio-thumbnail-label {
  flex-wrap: wrap;
  gap: 3px;
}
.studio-thumbnail-label small {
  flex-basis: 100%;
  text-align: left;
  overflow-wrap: anywhere;
}
.studio-history article {
  padding: 10px 0;
  border-top: 1px solid #e8ebf1;
}
@media (max-width: 760px) {
  .courseware-studio {
    height: auto;
    min-height: 0;
  }
  .studio-body {
    flex: none;
    overflow: visible;
  }
  .studio-canvas {
    max-height: 75dvh;
    min-height: 320px;
  }
}
</style>
