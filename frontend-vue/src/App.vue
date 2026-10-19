<script setup>
import { ref, reactive, computed, onMounted, onUnmounted, watch, nextTick } from 'vue';
import { templates, layouts, slideLayout } from './templates.js';
import CoursewareEvidence from './CoursewareEvidence.vue';
import TeachingClaimSources from './TeachingClaimSources.vue';
import EmphasisText from './EmphasisText.vue';
import AccountSettings from './AccountSettings.vue';
import BillingPage from './BillingPage.vue';
import CoursewareStudio from './CoursewareStudio.vue';
import AnimationStudio from './AnimationStudio.vue';
import AuthPanel from './AuthPanel.vue';
import CoursewareSetup from './CoursewareSetup.vue';
import HomeWorkspace from './HomeWorkspace.vue';
import WorkspaceIcon from './WorkspaceIcon.vue';
import UsageRecords from './UsageRecords.vue';
import TeachingMaterials from './TeachingMaterials.vue';
import { restoreWorkspace } from './workspace-session.js';
import { readEventStream } from './event-stream.js';
const token = ref(sessionStorage.getItem('edu_token') || ''),
  me = ref(null),
  health = ref({}),
  tenants = ref([]),
  tenant = ref(0);
const restoringSession = ref(!!token.value);
let sessionGeneration = 0;
const page = ref('home'),
  tab = ref('outline'),
  busy = ref(false),
  error = ref(''),
  notice = ref(''),
  loginForm = reactive({ username: 'teacher', password: '' });
const billing = ref({ available: 0, reserved: 0, products: [], orders: [], ledger: [] }),
  tasks = ref([]),
  docs = ref([]),
  members = ref([]),
  dashboard = ref({ tasks: [], usage: [] });
const task = ref(null),
  draft = ref(null),
  accept = ref(false),
  selectedSlide = ref(0),
  sources = ref([]),
  query = ref(''),
  url = ref(''),
  webPreview = ref(null),
  videoURL = ref(''),
  showProcess = ref(false),
  history = ref([]),
  historyOpen = ref(false),
  historyDraft = ref(null);
const historyOffset = ref(0),
  historyMore = ref(false);
const processGroups = computed(() =>
  [...new Set((task.value?.events || []).map((e) => e.stage))].map((stage) => ({
    stage,
    title:
      {
        queued: '任务预留',
        edited: '内容修改',
        restored: '版本恢复',
        reviewed: '辅助评审',
        repairing: '自动修复与排版检查',
        auto_recovering: '自动恢复制作'
      }[stage] ||
      labels[stage] ||
      stage,
    events: task.value.events.filter((e) => e.stage === stage)
  }))
);
const animationBrief = ref(null);
const emptyCourseware = () => ({
  topic: '',
  subject: '',
  grade: '',
  textbook: '',
  objectives: '',
  detail: 'auto',
  lesson_minutes: '',
  student_level: '',
  reference_mode: 'content',
  material_id: '',
  material_start: 1,
  material_end: 3,
  chapter: '',
  reference_id: '',
  theme: 'ocean',
  use_kb: true,
  use_web: true,
  source_task_id: ''
});
const form = reactive(emptyCourseware());
const briefConfirmed = ref(false),
  requirements = ref(''),
  sidebarCollapsed = ref(localStorage.getItem('edu_sidebar_collapsed') === '1');
const briefVersion = ref(0);
const homeForm = reactive({ ...form });
const homeSession = reactive({
  messages: [],
  text: '',
  summary: '',
  understanding: '',
  selectedTool: '',
  suggestedTool: 'courseware',
  contextMemory: { text: '', through: 0 }
});

watch(sidebarCollapsed, (value) =>
  localStorage.setItem('edu_sidebar_collapsed', value ? '1' : '0')
);
const submitting = ref(false),
  outputPanel = ref(null),
  worksQuery = ref(''),
  worksType = ref(''),
  worksStatus = ref(''),
  worksOffset = ref(0),
  worksMore = ref(false),
  worksLoading = ref(false),
  workItems = ref([]),
  renameItem = ref(null),
  renameTitle = ref(''),
  deleteItem = ref(null);
const terminal = ['succeeded', 'degraded', 'failed', 'cancelled'];
const activeStates = [
  'anim_plan_pending',
  'anim_plan_running',
  'anim_rep_pending',
  'anim_rep_running',
  'anim_full_pending',
  'anim_full_running',
  'anim_edit_pending',
  'anim_edit_running',
  'pending',
  'researching',
  'confirmed',
  'rendering',
  'expand_pending',
  'expanding'
];
const selectedWorks = ref([]),
  bulkDeleteOpen = ref(false);
const deletableWorks = computed(() => workItems.value.filter((t) => terminal.includes(t.status)));
const allWorksSelected = computed(
  () =>
    deletableWorks.value.length > 0 &&
    deletableWorks.value.every((t) => selectedWorks.value.includes(t.id))
);
const generating = computed(
  () =>
    submitting.value ||
    (!!task.value &&
      !terminal.includes(task.value.status) &&
      ![
        'awaiting_preview',
        'awaiting_confirmation',
        'awaiting_acceptance',
        'anim_plan',
        'anim_representative',
        'anim_acceptance',
        'anim_edit_acceptance'
      ].includes(task.value.status))
);
const studioMode = computed(
  () =>
    page.value === 'courseware' &&
    task.value?.type === 'courseware' &&
    task.value?.input?.preview_first === true
);
const progressStep = computed(() =>
  finished.value
    ? 3
    : task.value?.status === 'awaiting_confirmation'
      ? 1
      : ['confirmed', 'rendering'].includes(task.value?.status)
        ? 2
        : 0
);
const currentSlide = computed(() => draft.value?.slides[selectedSlide.value]);
const newMember = reactive({ username: '', name: '', password: '' });
const nav = [
  ['home', 'home', '首页'],
  ['tools', 'tools', '所有工具'],
  ['tasks', 'works', '我的作品'],
  ['knowledge', 'library', '知识与来源']
];
const accountPages = [
  ['account', '个人设置'],
  ['billing', '套餐与账单'],
  ['dashboard', '用量与评测']
];
const labels = {
  anim_plan_pending: '等待方案',
  anim_plan_running: '生成方案中',
  anim_plan: '待确认方案',
  anim_rep_pending: '等待代表幕',
  anim_rep_running: '制作代表幕中',
  anim_representative: '待代表幕审核',
  anim_full_pending: '等待完整制作',
  anim_full_running: '制作其余幕中',
  anim_acceptance: '待成品验收',
  anim_edit_pending: '等待局部修改',
  anim_edit_running: '局部修改中',
  anim_edit_acceptance: '待修改验收',
  pending: '等待研究',
  researching: '研究资料中',
  awaiting_preview: '待审核三页质量预览',
  expand_pending: '等待生成剩余页',
  expanding: '生成剩余课件中',
  awaiting_confirmation: '待教师确认',
  confirmed: '等待制作',
  rendering: '制作文件中',
  awaiting_acceptance: '待成品验收',
  succeeded: '已完成',
  degraded: '简化交付',
  failed: '待处理',
  cancelled: '已取消'
};
const worksView = ref('works'),
  cleanupPending = ref(0);
const resourceActions = computed(() => health.value.works_resource_version === 'resources-v1');
const worksStatusOptions = computed(() =>
  Object.fromEntries(
    Object.entries(labels).filter(
      ([state]) =>
        worksView.value === 'history' ||
        (worksView.value === 'works' && resourceActions.value) ||
        ['failed', 'cancelled'].includes(state) === (worksView.value === 'interrupted')
    )
  )
);
const finished = computed(() => ['succeeded', 'degraded'].includes(task.value?.status));
const canEdit = computed(
    () =>
      !busy.value &&
      task.value?.type === 'courseware' &&
      (task.value?.status === 'awaiting_confirmation' || finished.value)
  ),
  title = computed(
    () =>
      nav.find((x) => x[0] === page.value)?.[2] ||
      {
        account: '个人账户',
        billing: '账户 · 套餐与账单',
        dashboard: '账户 · 用量与评测',
        courseware: '课件工作台',
        animation: '教学动画'
      }[page.value] ||
      '企业管理'
  );
const can = (p) => me.value?.role === 'admin' || me.value?.permissions?.includes(p);
const coursewarePaused = computed(
  () =>
    task.value?.type === 'courseware' &&
    (task.value.status === 'failed' ||
      (task.value.status === 'cancelled' && task.value.restored_at))
);
const statusText = (t) =>
  t?.type === 'animation' && t.has_delivery && ['failed', 'cancelled'].includes(t.status)
    ? t.status === 'failed'
      ? '已交付 · 修改待恢复'
      : '已交付 · 修改已取消'
    : t?.capacity_hold
      ? '草稿已保存 · 待清理容量'
      : t?.restored_at && ['failed', 'cancelled'].includes(t.status)
        ? '已还原 · 待继续'
        : t?.status === 'failed' && t?.type === 'courseware'
          ? '待继续制作'
          : labels[t?.status] || t?.status;
function passwordChanged() {
  sessionGeneration++;
  token.value = '';
  me.value = null;
  sessionStorage.removeItem('edu_token');
  clearTask();
  loginForm.password = '';
  notice.value = '密码已更新，请使用新密码重新登录。';
  page.value = 'home';
}
async function retryCourseware() {
  await run(async () => {
    const v = await api('/tasks/' + task.value.id + '/retry', 'POST', {});
    await selectTask(v.id);
    notice.value = '已继续制作；原任务未扣款，本次重新预留20积分，成功交付后结算。';
    await focusOutput();
  });
  if (error.value) {
    await nextTick();
    document
      .querySelector('.content>.alert.error')
      ?.scrollIntoView({ behavior: 'smooth', block: 'center' });
  }
}
let sessionRefresh = null,
  sessionRefreshToken = '',
  refreshRetryAfter = 0;
async function renewSession(background = false) {
  const current = token.value;
  if (!current || (background && Date.now() < refreshRetryAfter)) return;
  let expires;
  try {
    expires =
      JSON.parse(atob(current.split('.')[1].replace(/-/g, '+').replace(/_/g, '/'))).exp * 1000;
  } catch {
    return;
  }
  if (!Number.isFinite(expires) || expires - Date.now() > 30 * 60 * 1000) return;
  if (sessionRefresh && sessionRefreshToken === current) return sessionRefresh;
  sessionRefreshToken = current;
  const pending = (async () => {
    try {
      const response = await fetch('/api/auth/refresh', {
        method: 'POST',
        headers: { Authorization: 'Bearer ' + current },
        signal: AbortSignal.timeout(10000)
      });
      if (!response.ok) {
        refreshRetryAfter = Date.now() + 30000;
        return;
      }
      const value = await response.json();
      if (value.token && token.value === current) {
        token.value = value.token;
        sessionStorage.setItem('edu_token', value.token);
        refreshRetryAfter = 0;
      }
    } catch {
      refreshRetryAfter = Date.now() + 30000;
    }
  })();
  sessionRefresh = pending;
  try {
    await pending;
  } finally {
    if (sessionRefresh === pending) {
      sessionRefresh = null;
      sessionRefreshToken = '';
    }
  }
}
async function requireCoursewareService(staged = false) {
  const ready = () =>
    health.value.courseware_media_version === 'media-v1' &&
    (!staged || health.value.courseware_studio_version === 'studio-v1');
  if (ready()) return;
  // A tab can outlive a service restart; refresh capabilities before blocking a write.
  try {
    health.value = await api('/health', 'GET', undefined, { signal: AbortSignal.timeout(8000) });
  } catch {
    throw Error('暂时无法连接课件服务，请稍后点击重试。当前对话与编辑内容已保留。');
  }
  if (!ready())
    throw Error('课件服务版本暂不兼容，请联系管理员更新后点击重试。当前对话与编辑内容已保留。');
}
async function api(path, method = 'GET', data, { signal, onEvent } = {}) {
  const writesCourseware =
    (path === '/tasks' && data?.type === 'courseware') ||
    (task.value?.type === 'courseware' &&
      /^\/tasks\/[^/]+\/(confirm|edit|rewrite|retry|revisions\/\d+\/restore)$/.test(path));
  if (method === 'POST' && writesCourseware)
    await requireCoursewareService(path === '/tasks' && data?.preview_first === true);
  if (
    ![
      '/health',
      '/auth/login',
      '/auth/register',
      '/auth/reset-password',
      '/auth/logout',
      '/account/password'
    ].includes(path)
  ) {
    const generation = sessionGeneration;
    await renewSession();
    if (generation !== sessionGeneration) throw Error('登录状态已变化，请重新执行当前操作。');
  }
  const requestToken = token.value,
    requestGeneration = sessionGeneration,
    opts = { method, headers: {}, signal };
  if (requestToken) opts.headers.Authorization = 'Bearer ' + requestToken;
  if (onEvent) opts.headers.Accept = 'text/event-stream';
  if (data instanceof FormData) opts.body = data;
  else if (data !== undefined) {
    opts.headers['Content-Type'] = 'application/json';
    opts.body = JSON.stringify(data);
  }
  const r = await fetch('/api' + path, opts);
  if (!r.ok) {
    const v = await r.json().catch(() => ({}));
    if (
      r.status === 401 &&
      requestToken &&
      requestToken === token.value &&
      !['/auth/login', '/auth/register', '/auth/reset-password'].includes(path)
    ) {
      let expired = path === '/auth/me';
      if (!expired) {
        try {
          const check = await fetch('/api/auth/me', {
            headers: { Authorization: 'Bearer ' + requestToken }
          });
          expired = check.status === 401;
        } catch {}
      }
      if (expired && requestToken === token.value) {
        sessionGeneration++;
        token.value = '';
        me.value = null;
        sessionStorage.removeItem('edu_token');
        clearTask();
        eventController?.abort();
        notice.value = '登录已失效，请重新登录。';
      }
    }
    throw Error(v.message || '请求失败：' + r.status);
  }
  if (onEvent)
    return readEventStream(r, (event, value) => {
      if (requestGeneration !== sessionGeneration || requestToken !== token.value)
        throw Error('企业或登录状态已变化，请重新发送需求。');
      onEvent(event, value);
    });
  const result = r.status === 204 ? null : await r.json();
  if (requestGeneration !== sessionGeneration || requestToken !== token.value)
    throw Error('企业或登录状态已变化，请重新执行当前操作。');
  return result;
}
async function run(fn) {
  if (busy.value) return;
  busy.value = true;
  error.value = '';
  notice.value = '';
  try {
    await fn();
  } catch (e) {
    error.value = e.message;
  } finally {
    busy.value = false;
  }
}
async function refreshAccount() {
  const generation = sessionGeneration,
    actor = await api('/auth/me');
  if (generation !== sessionGeneration) return;
  me.value = actor;
  tenant.value = actor.tenant_id;
  const [account, works] = await Promise.all([api('/billing'), api('/tasks')]);
  if (generation !== sessionGeneration) return;
  billing.value = account;
  tasks.value = works;
}
async function restoreSession() {
  if (busy.value) return;
  restoringSession.value = true;
  await run(async () => {
    await refreshAccount();
    tenants.value = await api('/tenants');
  });
  restoringSession.value = false;
}
async function finishLogin(v) {
  await run(async () => {
    sessionGeneration++;
    token.value = v.token;
    sessionStorage.setItem('edu_token', v.token);
    tenants.value = v.tenants;
    await refreshAccount();
    loginForm.password = '';
  });
}
async function logout() {
  await run(async () => {
    await api('/auth/logout', 'POST', {});
    sessionGeneration++;
    token.value = '';
    me.value = null;
    clearTask();
    sessionStorage.removeItem('edu_token');
  });
}
async function switchTenant() {
  await run(async () => {
    const v = await api('/auth/switch-tenant', 'POST', { tenant_id: Number(tenant.value) });
    sessionGeneration++;
    token.value = v.token;
    sessionStorage.setItem('edu_token', v.token);
    clearTask();
    docs.value = [];
    sources.value = [];
    workItems.value = [];
    worksRequest++;
    page.value = 'home';
    await refreshAccount();
  });
}
let selectionVersion = 0,
  worksRequest = 0,
  polling = false,
  billingNeedsRefresh = false,
  billingRefreshing = false;
let worksRefreshing = false,
  lastWorksRefresh = 0,
  worksFilters = null;
async function refreshTaskBilling() {
  if (!billingNeedsRefresh || billingRefreshing || !token.value) return;
  billingRefreshing = true;
  const generation = sessionGeneration;
  try {
    const account = await api('/billing');
    if (generation === sessionGeneration) {
      billing.value = account;
      billingNeedsRefresh = false;
    }
  } finally {
    billingRefreshing = false;
  }
}
function clearTask() {
  selectedWorks.value = [];
  bulkDeleteOpen.value = false;
  briefConfirmed.value = false;
  requirements.value = '';
  selectionVersion++;
  task.value = null;
  draft.value = null;
  history.value = [];
  historyOpen.value = false;
  historyDraft.value = null;
  showProcess.value = false;
  renameItem.value = null;
  deleteItem.value = null;
  if (videoURL.value) {
    URL.revokeObjectURL(videoURL.value);
    videoURL.value = '';
  }
}
async function selectTask(id, { poll = false } = {}) {
  if (poll && polling) return;
  const version = poll ? selectionVersion : ++selectionVersion;
  if (poll) polling = true;
  let v;
  try {
    v = await api('/tasks/' + id);
  } finally {
    if (poll) polling = false;
  }
  if (
    version !== selectionVersion ||
    (poll && task.value?.id !== id) ||
    (task.value?.id === id && v.revision < task.value.revision)
  )
    return;
  if (task.value?.id !== id) {
    if (videoURL.value) URL.revokeObjectURL(videoURL.value);
    videoURL.value = '';
  }
  if (task.value?.id !== id) {
    history.value = [];
    historyOpen.value = false;
    historyDraft.value = null;
  }
  if (
    task.value?.id !== id ||
    task.value?.revision !== v.revision ||
    (!draft.value && v.result?.slides)
  ) {
    draft.value = v.result?.slides ? structuredClone(v.result) : null;
    selectedSlide.value = 0;
    accept.value = false;
  }
  if (draft.value && v.result?.evaluation) draft.value.evaluation = v.result.evaluation;
  task.value = v;
  if (v.type === 'animation' && v.status === 'awaiting_acceptance' && !videoURL.value)
    await previewAnimation(id, version);
  if (!poll) await refreshAccount();
  else if (terminal.includes(v.status)) {
    billingNeedsRefresh = true;
    await refreshTaskBilling();
  }
}
async function focusOutput() {
  await nextTick();
  const panel = outputPanel.value?.$el || outputPanel.value;
  panel?.scrollIntoView({ behavior: 'smooth', block: 'start' });
  panel?.focus({ preventScroll: true });
}
function newWork(kind) {
  clearTask();
  animationBrief.value = null;
  briefVersion.value++;
  page.value = kind;
  form.source_task_id = '';
  form.material_id = '';
  form.reference_id = '';
  form.chapter = '';
  if (kind === 'courseware') Object.assign(form, emptyCourseware());
}
function openHomeTool(kind) {
  if (kind === 'courseware' || kind === 'animation') {
    newWork(kind);
    page.value = kind;
  } else page.value = kind;
}
async function startFromHome({ topic, subject }) {
  newWork('courseware');
  page.value = 'home';
  form.topic = topic;
  form.objectives = '';
  if (subject) form.subject = subject;
  await nextTick();
  const input = document.querySelector('[aria-label="生成前需求输入"]');
  if (input) {
    input.value = `帮我制作《${topic}》课件。`;
    input.dispatchEvent(new Event('input', { bubbles: true }));
    input.focus();
    input.scrollIntoView({ block: 'nearest', behavior: 'smooth' });
  }
}
async function openWork(t) {
  await run(async () => {
    tab.value = 'outline';
    page.value = t.type === 'animation' ? 'animation' : 'courseware';
    await nextTick();
    await selectTask(t.id);
    const v = task.value;
    if (v?.id === t.id) {
      for (const k of [
        'topic',
        'subject',
        'grade',
        'textbook',
        'objectives',
        'use_kb',
        'use_web',
        'detail',
        'lesson_minutes',
        'student_level',
        'reference_mode',
        'material_id',
        'material_start',
        'material_end',
        'chapter',
        'reference_id'
      ])
        if (v.input[k] !== undefined) form[k] = v.input[k];
      form.source_task_id = v.input.source_task_id || '';
      form.pages = v.input.pages || 8;
      form.theme = v.result?.theme || v.input.theme || 'forest';
    }
    await focusOutput();
  });
}
async function loadWorks(reset = false, { background = false } = {}) {
  if (background && (worksRefreshing || worksLoading.value || !worksFilters)) return;
  if (health.value.works_view_version !== 'views-v1') worksView.value = 'history';
  if (reset) worksOffset.value = 0;
  const request = ++worksRequest,
    generation = sessionGeneration;
  if (background) worksRefreshing = true;
  else {
    worksLoading.value = true;
    worksFilters = {
      view: worksView.value,
      q: worksQuery.value,
      type: worksType.value,
      status: worksStatus.value,
      offset: String(worksOffset.value),
      limit: '51'
    };
  }
  lastWorksRefresh = Date.now();
  try {
    const rows = await api('/tasks?' + new URLSearchParams(worksFilters), 'GET', undefined, {
      signal: AbortSignal.timeout(15000)
    });
    if (request === worksRequest && generation === sessionGeneration) {
      workItems.value = rows.slice(0, 50);
      worksMore.value = rows.length > 50;
      selectedWorks.value = background
        ? selectedWorks.value.filter((id) => deletableWorks.value.some((t) => t.id === id))
        : [];
    }
  } finally {
    if (background) worksRefreshing = false;
    else if (request === worksRequest) worksLoading.value = false;
  }
}
async function deleteSelectedWorks() {
  await run(async () => {
    const ids = [...selectedWorks.value];
    await api('/tasks/delete', 'POST', { ids });
    bulkDeleteOpen.value = false;
    if (ids.includes(task.value?.id)) clearTask();
    await loadWorks(true);
    await refreshAccount();
    await loadCleanupStatus();
    notice.value = '所选作品已删除，独占资源将在后台清理，积分账单保留。';
  });
}
async function renameWork() {
  await run(async () => {
    await api('/tasks/' + renameItem.value.id, 'PATCH', { title: renameTitle.value.trim() });
    renameItem.value = null;
    await loadWorks();
    await refreshAccount();
    notice.value = '作品名称已更新。';
  });
}
async function loadCleanupStatus() {
  if (!resourceActions.value) return;
  const generation = sessionGeneration,
    value = await api('/tasks/cleanup-status');
  if (generation === sessionGeneration) cleanupPending.value = value.pending;
}
async function changeWorksView(view) {
  worksView.value = view;
  worksStatus.value = '';
  await run(() => loadWorks(true));
}
async function restoreWork(item) {
  await run(async () => {
    await api('/tasks/' + item.id + '/restore', 'POST', {});
    worksView.value = 'works';
    worksQuery.value = '';
    worksType.value = '';
    worksStatus.value = '';
    await loadWorks(true);
    await refreshAccount();
    notice.value = '已还原到当前作品。草稿已保留，未启动制作、未预留或扣除积分。';
  });
}
async function deleteWork() {
  await run(async () => {
    const id = deleteItem.value.id,
      result = await api('/tasks/' + id, 'DELETE');
    deleteItem.value = null;
    if (task.value?.id === id) clearTask();
    await loadWorks();
    if (!workItems.value.length && worksOffset.value) {
      worksOffset.value -= 50;
      await loadWorks();
    }
    await refreshAccount();
    await loadCleanupStatus();
    notice.value = resourceActions.value
      ? result.cleanup_pending || cleanupPending.value > 0
        ? '作品已删除，资源正在清理；后台会自动重试。历史账单保留。'
        : '作品已删除，独占文件已清理；其他作品仍引用的文件和历史账单保留。'
      : '作品已删除，历史账单保留。';
  });
}
async function create() {
  if (!briefConfirmed.value) {
    await focusOutput();
    return;
  }
  const confirmedRequirements = requirements.value;
  await run(async () => {
    await requireCoursewareService(true);
    submitting.value = true;
    if (task.value) clearTask();
    tab.value = 'outline';
    await focusOutput();
    try {
      const v = await api('/tasks', 'POST', {
        ...form,
        lesson_minutes: Number(form.lesson_minutes) || 0,
        requirements: confirmedRequirements,
        pages: undefined,
        source_task_id: '',
        type: 'courseware',
        preview_first: true,
        idempotency_key: crypto.randomUUID()
      });
      await selectTask(v.id);
      notice.value = '任务已提交，正在规划教学内容与三页质量预览。';
    } finally {
      submitting.value = false;
      briefConfirmed.value = false;
    }
  });
}
async function previewAnimation(id = task.value?.id, version = selectionVersion) {
  const r = await fetch('/api/tasks/' + id + '/download/mp4', {
    headers: { Authorization: 'Bearer ' + token.value }
  });
  if (!r.ok) throw Error('预览暂不可用');
  const blob = await r.blob();
  if (version !== selectionVersion || task.value?.id !== id) return;
  if (videoURL.value) URL.revokeObjectURL(videoURL.value);
  videoURL.value = URL.createObjectURL(blob);
  tab.value = 'preview';
}
async function viewFrame(index) {
  await run(async () => {
    const r = await fetch('/api/tasks/' + task.value.id + '/frames/' + index, {
      headers: { Authorization: 'Bearer ' + token.value }
    });
    if (!r.ok) throw Error('关键帧不可用');
    const u = URL.createObjectURL(await r.blob());
    const a = document.createElement('a');
    a.href = u;
    a.download = 'frame-' + index + '.jpg';
    a.click();
    setTimeout(() => URL.revokeObjectURL(u), 2000);
  });
}
async function confirmGeneration() {
  await run(async () => {
    await api('/tasks/' + task.value.id + '/confirm', 'POST', {
      result: draft.value,
      revision: task.value.revision,
      accept_review: accept.value
    });
    await selectTask(task.value.id);
    await focusOutput();
  });
}
async function cancel() {
  await run(async () => {
    await api('/tasks/' + task.value.id + '/cancel', 'POST', {});
    await selectTask(task.value.id);
  });
}
async function save() {
  await run(async () => {
    await api('/tasks/' + task.value.id + '/edit', 'POST', {
      result: draft.value,
      revision: task.value.revision
    });
    await selectTask(task.value.id);
    notice.value = '新修订已保存，PPTX已重新导出；未扣积分。';
  });
}
async function loadHistory(offset = 0) {
  const id = task.value?.id,
    rows = await api('/tasks/' + id + '/revisions?offset=' + offset);
  if (task.value?.id !== id) return;
  history.value = rows.slice(0, 50);
  historyMore.value = rows.length > 50;
  historyOffset.value = offset;
  historyOpen.value = true;
  historyDraft.value = null;
}
async function viewHistory(version) {
  await run(async () => {
    const id = task.value.id,
      v = await api('/tasks/' + id + '/revisions/' + version);
    if (task.value?.id === id) {
      historyDraft.value = v;
      historyOpen.value = true;
    }
  });
}
async function restoreHistory(version) {
  if (
    !window.confirm(
      '恢复将替换当前编辑内容（含未保存修改），并保存为新版本。已保存的旧版仍保留，恢复免费。继续吗？'
    )
  )
    return;
  await run(async () => {
    await api('/tasks/' + task.value.id + '/revisions/' + version + '/restore', 'POST', {
      revision: task.value.revision
    });
    await selectTask(task.value.id);
    await loadHistory();
    notice.value = '历史版本已恢复并免费重新导出。';
  });
}
async function rewrite() {
  const instruction = prompt(
    '说明这一页的修改要求（成功后扣5积分）：',
    '让例题更贴近生活，并给出清晰解答'
  );
  if (!instruction) return;
  await run(async () => {
    const v = await api('/tasks/' + task.value.id + '/rewrite', 'POST', {
      instruction,
      slide: selectedSlide.value,
      revision: task.value.revision,
      idempotency_key: crypto.randomUUID()
    });
    notice.value = '单页修改任务已提交，成功后更新原课件。';
    await selectTask(v.id);
  });
}
async function download(format, version) {
  await run(async () => {
    const r = await fetch(
      '/api/tasks/' +
        task.value.id +
        (version ? '/revisions/' + version : '') +
        '/download/' +
        format,
      { headers: { Authorization: 'Bearer ' + token.value } }
    );
    if (!r.ok) throw Error('文件尚不可用');
    const blob = await r.blob(),
      u = URL.createObjectURL(blob);
    if (format === 'mp4') {
      if (videoURL.value) URL.revokeObjectURL(videoURL.value);
      videoURL.value = u;
      tab.value = 'preview';
    }
    const a = document.createElement('a');
    a.href = u;
    a.download = (draft.value?.title || 'lesson') + (version ? '-v' + version : '') + '.' + format;
    a.click();
    if (format !== 'mp4') setTimeout(() => URL.revokeObjectURL(u), 2000);
  });
}
async function buy(sku) {
  if (billing.value.simulated_payment_enabled === false) {
    notice.value = '当前服务已关闭自助模拟充值，请联系空间管理员分配积分。';
    return;
  }
  await run(async () => {
    const order = await api('/billing/orders', 'POST', { sku });
    await api('/billing/orders/' + order.id + '/pay', 'POST', {});
    await refreshAccount();
    notice.value = '模拟支付成功，积分已到账；没有发生真实扣款。';
  });
}
async function pay(id) {
  if (billing.value.simulated_payment_enabled === false) {
    notice.value = '当前服务已关闭自助模拟充值，请联系空间管理员分配积分。';
    return;
  }
  await run(async () => {
    await api('/billing/orders/' + id + '/pay', 'POST', {});
    await refreshAccount();
  });
}
async function upload(ev) {
  const f = ev.target.files[0];
  if (!f) return;
  await run(async () => {
    if (f.size > 100 * 1024 * 1024) throw Error('请选择100MB以内的文档');
    const data = new FormData();
    data.append('file', f);
    await api('/documents', 'POST', data);
    docs.value = await api('/documents');
    notice.value = '文档已上传，正在后台解析；离开页面不会中断。';
  });
  ev.target.value = '';
}
async function importPage(confirm = false) {
  await run(async () => {
    const v = await api('/documents/url', 'POST', {
      url: url.value,
      confirm,
      hash: webPreview.value?.hash || ''
    });
    if (confirm) {
      webPreview.value = null;
      docs.value = await api('/documents');
      notice.value = '网页正文已入库';
    } else webPreview.value = v;
  });
}
async function search() {
  await run(async () => {
    const v = await api('/retrieval/test', 'POST', { query: query.value, web: false });
    sources.value = v.sources;
    if (!sources.value.length) notice.value = '未找到相关资料。';
  });
}
async function reparseDoc(d) {
  if (
    !window.confirm(
      '重新解析这份资料？完成后替换资料库中的文字与切片，已有作品的来源快照保持原样。'
    )
  )
    return;
  await run(async () => {
    await api('/documents/' + d.id + '/reparse', 'POST', {});
    docs.value = await api('/documents');
    notice.value = '已安排重新解析，完成后更新资料内容；离开页面不会中断。';
  });
}
async function removeDoc(id) {
  if (!window.confirm('删除此企业共享文档及其切片？已生成作品的来源快照会保留。')) return;
  await run(async () => {
    await api('/documents/' + id, 'DELETE');
    docs.value = await api('/documents');
  });
}
async function addMember() {
  await run(async () => {
    await api('/admin/members', 'POST', newMember);
    newMember.username = '';
    newMember.name = '';
    newMember.password = '';
    members.value = await api('/admin/members');
    notice.value = '成员已创建；开通基础套餐后即可生成。';
  });
}
async function assignPackage(m, sku) {
  await run(async () => {
    await api('/admin/members/' + m.id + '/package', 'POST', {
      sku,
      idempotency_key: crypto.randomUUID()
    });
    members.value = await api('/admin/members');
    await refreshAccount();
    notice.value = '套餐已分配到该成员独立账户，操作已记录。';
  });
}
async function updateMember(m) {
  await run(async () => {
    await api('/admin/members/' + m.id, 'PATCH', { active: m.active, permissions: m.permissions });
    notice.value = '成员状态与权限已保存';
  });
}
async function evaluate() {
  await run(async () => {
    const v = await api('/evaluations', 'POST', { query: query.value || form.topic });
    notice.value = '检索评测已完成：' + JSON.stringify(v.metrics);
    dashboard.value = await api('/dashboard');
  });
}
function animationFromSlide() {
  const sl = draft.value?.slides[selectedSlide.value];
  if (!sl) return;
  animationBrief.value = {
    topic: draft.value.title,
    requirements: sl.title + '：' + sl.bullets.join('；'),
    source_task_id: finished.value ? task.value.id : '',
    subject: task.value.input?.subject || '数学',
    grade: task.value.input?.grade || '八年级'
  };
  page.value = 'animation';
  clearTask();
}
function date(v) {
  return v
    ? new Date(v).toLocaleString('zh-CN', {
        month: '2-digit',
        day: '2-digit',
        hour: '2-digit',
        minute: '2-digit'
      })
    : '—';
}
watch([() => me.value?.tenant_id, () => me.value?.member_id], () => {
  animationBrief.value = null;
});
let timer, previousFocus, eventController;
watch(
  [() => task.value?.id, () => terminal.includes(task.value?.status), page],
  ([id, ended, p]) => {
    eventController?.abort();
    if (id && !ended && ['courseware', 'animation'].includes(p)) {
      eventController = new AbortController();
      followEvents(id, eventController.signal);
    }
  }
);
async function followEvents(id, signal) {
  let last = '0';
  while (!signal.aborted && task.value?.id === id) {
    try {
      const response = await fetch('/api/tasks/' + id + '/events', {
        headers: { Authorization: 'Bearer ' + token.value, 'Last-Event-ID': last },
        signal
      });
      if (!response.ok) {
        if ([401, 403, 404].includes(response.status)) return;
        throw new Error('events ' + response.status);
      }
      const reader = response.body.getReader(),
        decoder = new TextDecoder();
      let buffer = '';
      while (!signal.aborted) {
        const { value, done } = await reader.read();
        if (done) break;
        buffer += decoder.decode(value, { stream: true });
        let end;
        while ((end = buffer.indexOf('\n\n')) >= 0) {
          const frame = buffer.slice(0, end);
          buffer = buffer.slice(end + 2);
          const line = frame.split('\n').find((x) => x.startsWith('data: '));
          const serial = frame.split('\n').find((x) => x.startsWith('id: '));
          if (serial) last = serial.slice(4);
          if (line && task.value?.id === id) {
            const ev = JSON.parse(line.slice(6));
            if (!task.value.events.some((x) => x.id === ev.id)) {
              task.value.events.push(ev);
              if (!busy.value)
                void selectTask(id, { poll: true }).catch((e) => (error.value = e.message));
            }
          }
        }
      }
    } catch (e) {
      if (signal.aborted) return;
    }
    if (!signal.aborted) await new Promise((r) => setTimeout(r, 2000));
  }
}

watch(
  () => !!renameItem.value || !!deleteItem.value || bulkDeleteOpen.value,
  async (open) => {
    if (open) {
      previousFocus = document.activeElement;
      await nextTick();
      document.querySelector('.work-dialog input,.work-dialog button')?.focus();
    } else previousFocus?.focus();
  }
);
watch(showProcess, async (open) => {
  if (open) {
    previousFocus = document.activeElement;
    await nextTick();
    document.querySelector('[aria-label="关闭过程"]')?.focus();
  } else previousFocus?.focus();
});
function trapDialog(event) {
  if (event.key !== 'Tab') return;
  const nodes = [
    ...event.currentTarget.querySelectorAll('button,summary,a[href],input,textarea,select')
  ].filter((x) => !x.disabled && x.offsetParent !== null);
  if (!nodes.length) return;
  const first = nodes[0],
    last = nodes.at(-1);
  if (event.shiftKey && document.activeElement === first) {
    event.preventDefault();
    last.focus();
  } else if (!event.shiftKey && document.activeElement === last) {
    event.preventDefault();
    first.focus();
  }
}
watch(page, async (p) => {
  error.value = '';
  if (
    task.value &&
    ((p === 'animation' && task.value.type !== 'animation') ||
      (p === 'courseware' && task.value.type === 'animation'))
  ) {
    clearTask();
  }
  try {
    if (p === 'home' && token.value) await refreshAccount();
    if (p === 'knowledge') docs.value = await api('/documents');
    if (p === 'admin') members.value = await api('/admin/members');
    if (p === 'dashboard') dashboard.value = await api('/dashboard');
    if (p === 'tasks') {
      await refreshAccount();
      await loadWorks();
      await loadCleanupStatus();
    }
    if (p === 'billing') await refreshAccount();
  } catch (e) {
    error.value = e.message;
  }
});
onMounted(async () => {
  try {
    health.value = await api('/health');
  } catch (e) {
    error.value = e.message;
  }
  if (token.value) await restoreSession();
  restoringSession.value = false;
  timer = setInterval(async () => {
    if (!token.value || document.hidden) return;
    void renewSession(true);
    if (billingNeedsRefresh) {
      try {
        await refreshTaskBilling();
      } catch {}
    }
    if (busy.value) return;
    if (
      page.value === 'knowledge' &&
      docs.value.some((d) => ['pending', 'indexing'].includes(d.status))
    ) {
      try {
        docs.value = await api('/documents');
      } catch {}
    }
    const t = task.value;
    if (page.value === 'tasks' && cleanupPending.value > 0) {
      try {
        await loadCleanupStatus();
      } catch {}
    }
    if (
      page.value === 'tasks' &&
      Date.now() - lastWorksRefresh >= 10000 &&
      workItems.value.some((x) => activeStates.includes(x.status))
    ) {
      try {
        await loadWorks(false, { background: true });
      } catch {}
    }
    if (
      t &&
      ['courseware', 'animation'].includes(page.value) &&
      (activeStates.includes(t.status) ||
        (t.status === 'succeeded' && !t.result?.evaluation && t.type === 'courseware'))
    ) {
      try {
        await selectTask(t.id, { poll: true });
      } catch (e) {
        error.value = e.message;
      }
    }
  }, 2000);
});
const stopWorkspacePersistence = restoreWorkspace({
  me,
  homeSession,
  homeForm,
  form,
  page,
  task,
  selectTask,
  clearTask,
  defaults: emptyCourseware
});
onUnmounted(stopWorkspacePersistence);
onUnmounted(() => {
  selectionVersion++;
  worksRequest++;
  eventController?.abort();
  clearInterval(timer);
  if (videoURL.value) URL.revokeObjectURL(videoURL.value);
});
</script>

<template>
  <div v-if="!me" class="login-shell">
    <section class="login-story">
      <div class="brand"><span class="brand-mark">研</span>备课工坊</div>
      <span class="eyebrow">TEACH LESS PREP. INSPIRE MORE.</span>
      <h1>把时间留给<br /><em>真正的教学。</em></h1>
      <p>从可信资料到课堂表达。<br />让每一个好想法，成为一堂好课。</p>
      <div class="orb one"></div>
      <div class="orb two"></div>
      <div class="story-caption">教案 · 可编辑课件 · 概念动画</div>
    </section>
    <section v-if="token" class="login-form session-recovery" aria-label="恢复登录状态">
      <h2>{{ restoringSession ? '正在恢复登录' : '暂时无法连接工作台' }}</h2>
      <p v-if="error" role="alert">{{ error }}</p>
      <button v-if="!restoringSession" class="primary" :disabled="busy" @click="restoreSession">
        重试
      </button>
    </section>
    <AuthPanel v-else :api="api" :health="health" :notice="notice" @authenticated="finishLogin" />
  </div>
  <div
    v-else
    :class="['app-shell', { 'sidebar-collapsed': sidebarCollapsed, 'review-page': studioMode }]"
    :data-page="page"
  >
    <a
      class="skip-link"
      href="#main-content"
      @click.prevent="$event.currentTarget.ownerDocument.getElementById('main-content')?.focus()"
      >跳到主要内容</a
    >
    <aside class="sidebar">
      <div class="sidebar-heading">
        <button
          class="sidebar-toggle"
          @click="sidebarCollapsed = !sidebarCollapsed"
          :aria-label="sidebarCollapsed ? '展开导航栏' : '收起导航栏'"
          :title="sidebarCollapsed ? '展开导航栏' : '收起导航栏'"
          :aria-expanded="!sidebarCollapsed"
        >
          <WorkspaceIcon name="menu" />
        </button>
        <div class="brand">
          <span class="brand-mark">研</span>
          <div>备课工坊<small>EDUCATOR STUDIO</small></div>
        </div>
      </div>
      <label class="tenant-select"
        ><span>企业空间</span
        ><select v-model="tenant" @change="switchTenant" :disabled="busy">
          <option v-for="t in tenants" :key="t.id" :value="t.id">{{ t.name }}</option>
        </select></label
      >
      <div class="nav-caption">教学创作</div>
      <nav aria-label="主导航">
        <button
          v-for="n in nav"
          :key="n[0]"
          @click="page = n[0]"
          :title="n[2]"
          :aria-label="n[2]"
          :class="{
            active:
              page === n[0] || (n[0] === 'tools' && ['courseware', 'animation'].includes(page))
          }"
          :aria-current="page === n[0] ? 'page' : undefined"
        >
          <WorkspaceIcon :name="n[1]" /><span class="nav-label">{{ n[2] }}</span
          ><i v-if="n[0] === 'courseware'">主打</i></button
        ><button
          v-if="can('user.manage')"
          :class="{ active: page === 'admin' }"
          title="企业管理"
          aria-label="企业管理"
          @click="page = 'admin'"
        >
          <WorkspaceIcon name="settings" /><span class="nav-label">企业管理</span>
        </button>
      </nav>
      <div class="sidebar-bottom">
        <div class="quota-card">
          <span>我的可用积分</span><strong>{{ billing.available }}<small> 积分</small></strong>
          <p>预留 {{ billing.reserved }} · 成员独立额度</p>
          <button @click="page = 'billing'">查看套餐与账单 ↗</button>
        </div>
        <div class="profile">
          <button
            type="button"
            class="profile-open"
            @click="page = 'account'"
            aria-label="个人账户设置"
            title="个人账户设置"
          >
            <span class="avatar">{{ me.name?.slice(-2) }}</span
            ><span
              ><b>{{ me.name }}</b
              ><small>{{ me.role === 'admin' ? '企业管理员' : '教研成员' }}</small></span
            ></button
          ><button @click="logout" title="退出登录" aria-label="退出登录">
            <WorkspaceIcon name="logout" />
          </button>
        </div>
      </div>
    </aside>
    <main>
      <header class="topbar">
        <span
          ><template v-if="['courseware', 'animation'].includes(page) && !task"
            ><button type="button" class="generator-back" @click="page = 'tools'">所有工具</button
            ><span class="crumb" aria-hidden="true">/</span
            ><b>{{ page === 'animation' ? '教学动画' : '课件制作' }}</b></template
          ><template v-else
            >工作空间 <span class="crumb" aria-hidden="true">/</span> <b>{{ title }}</b></template
          ></span
        >
        <div>
          <span
            :class="['mode-tag', health.mode === 'sample' ? 'sample' : 'live']"
            :title="
              health.mode === 'sample' ? '内置样例，不调用真实模型' : '调用已配置的真实模型接口'
            "
            >{{ health.mode === 'sample' ? '本地示例模式' : '真实接口模式' }}</span
          ><span class="private-tag"><WorkspaceIcon name="lock" />个人作品私有</span
          ><button class="mobile-account" @click="page = 'account'" aria-label="个人账户设置">
            账户</button
          ><button class="mobile-logout" @click="logout">退出</button>
        </div>
      </header>
      <div
        id="main-content"
        tabindex="-1"
        :class="[
          'content',
          {
            'home-page': page === 'home',
            'courseware-create-page': page === 'courseware' && !task,
            'animation-page': page === 'animation',
            'works-page': page === 'tasks'
          }
        ]"
      >
        <div v-if="error" class="alert error" role="alert">
          {{ error }}<button @click="error = ''" aria-label="关闭错误">×</button>
        </div>
        <div v-if="notice" class="alert success" role="status">{{ notice }}</div>
        <p
          v-if="health.status === 'ok' && health.courseware_media_version !== 'media-v1'"
          class="alert"
          role="status"
        >
          课件服务版本信息待确认，制作前会重新检查。可保留当前需求，稍后点击重试。
        </p>
        <p
          v-if="
            health.mode === 'live' &&
            health.courseware_content_review_enabled === false &&
            ['home', 'tools', 'courseware'].includes(page)
          "
          class="alert"
          role="status"
        >
          内容模型审核已停用；保留程序校验与截图审核。请逐页核对知识、题设、计算和答案后确认交付。
        </p>
        <nav
          v-if="accountPages.some((n) => n[0] === page)"
          class="account-navigation"
          aria-label="账户页面"
        >
          <button
            v-for="n in accountPages"
            :key="n[0]"
            :aria-current="page === n[0] ? 'page' : undefined"
            :class="{ active: page === n[0] }"
            @click="page = n[0]"
          >
            {{ n[1] }}
          </button>
        </nav>
        <HomeWorkspace
          :animation-ready="health.animation_configured === true"
          v-if="['home', 'tools'].includes(page)"
          :tools-only="page === 'tools'"
          :loading="restoringSession"
          :tasks="tasks"
          :available="billing.available"
          :can-create="can('courseware.generate') || can('animation.render')"
          :status-text="statusText"
          @start="startFromHome"
          @navigate="openHomeTool"
          @open="openWork"
          ><CoursewareSetup
            workspace
            :session="homeSession"
            :token="token"
            :available="billing.available"
            :can-courseware="can('courseware.generate')"
            :can-animation="can('animation.render')"
            @changed="refreshAccount().catch((e) => (error = e.message))"
            :key="String(me?.tenant_id) + ':' + String(me?.member_id)"
            :form="homeForm"
            :api="api"
            :health="health"
            :disabled="busy"
            :tagging="can('kb.manage')"
            :ready="
              (health.mode !== 'live' || health.courseware_quality_configured !== false) &&
              !!billing.plan &&
              billing.available >= 20 &&
              can('courseware.generate') &&
              (homeForm.use_kb || homeForm.use_web)
            "
            v-model:requirements="requirements"
            @confirmed="
              briefConfirmed = true;
              create();
            "
        /></HomeWorkspace>
        <AnimationStudio
          :initial-brief="animationBrief"
          v-else-if="page === 'animation' && (!task || task.input?.animation_engine === 'html')"
          :key="String(me?.tenant_id) + ':' + String(me?.member_id) + ':' + (task?.id || 'new')"
          :task="task"
          :api="api"
          :token="token"
          :health="health"
          :available="billing.available"
          :can-create="can('animation.render')"
          :refresh="() => selectTask(task.id)"
          @created="(id) => run(() => selectTask(id))"
          @new-work="newWork('animation')"
        />
        <template v-else-if="['courseware', 'animation'].includes(page)">
          <div v-if="page !== 'courseware' || task" class="page-heading">
            <div>
              <div class="eyebrow">
                {{
                  page === 'courseware' ? 'FROM KNOWLEDGE TO CLASSROOM' : 'MAKE CONCEPTS VISIBLE'
                }}
              </div>
              <h1>{{ page === 'courseware' ? '让备课，多一点从容。' : '让抽象知识，动起来。' }}</h1>
              <p>
                {{
                  page === 'courseware'
                    ? '找资料、理思路、做课件。在一个工作台完成。'
                    : '历史动画可查看和下载；重新制作请新建 HTML 动画。'
                }}
              </p>
            </div>
            <span class="step-pill">01 研究 <i>→</i> 02 确认 <i>→</i> 03 交付</span>
          </div>
          <div
            :class="[
              'workbench',
              { 'studio-mode': studioMode, 'courseware-workbench': page === 'courseware' }
            ]"
          >
            <CoursewareSetup
              :key="String(me?.tenant_id) + ':' + String(me?.member_id) + ':' + briefVersion"
              v-if="page === 'courseware' && !task"
              generator
              :form="form"
              :api="api"
              :health="health"
              :disabled="busy || generating"
              :tagging="can('kb.manage')"
              :ready="
                (health.mode !== 'live' || health.courseware_quality_configured !== false) &&
                !!billing.plan &&
                billing.available >= 20 &&
                can('courseware.generate') &&
                (form.use_kb || form.use_web)
              "
              v-model:requirements="requirements"
              @confirmed="
                briefConfirmed = true;
                create();
              "
            />
            <CoursewareStudio
              v-if="studioMode"
              ref="outputPanel"
              :task="task"
              :api="api"
              :token="token"
              :available="billing.available"
              :retrying="busy"
              :canvas-available="
                ['regions-v2', 'canvas-v3'].includes(health.courseware_canvas_version)
              "
              :canvas-complete="health.courseware_canvas_version === 'canvas-v3'"
              :refresh="() => selectTask(task.id)"
              @process="showProcess = true"
              @retry="retryCourseware"
              @new-work="newWork('courseware')"
              @animation="
                (sl) => {
                  selectedSlide = draft.slides.findIndex((s) => s.id === sl.id);
                  animationFromSlide();
                }
              "
            />
            <section
              v-else-if="task || page === 'animation'"
              class="panel output-panel"
              ref="outputPanel"
              tabindex="-1"
              aria-label="当前制作任务"
              :aria-busy="generating"
            >
              <div class="output-head">
                <div>
                  <span class="tiny-label">{{ task ? '当前创作' : '创作预览' }}</span>
                  <h2>
                    {{
                      task?.display_title ||
                      draft?.title ||
                      task?.input?.topic ||
                      '一堂好课，从这里展开'
                    }}
                  </h2>
                </div>
                <span v-if="task" :class="['status', task.status]">{{ statusText(task) }}</span
                ><span v-else class="muted">等待开始</span>
              </div>
              <div
                v-if="task || submitting"
                :class="['generation-progress', { working: generating }]"
                role="status"
                aria-live="polite"
              >
                <div class="progress-heading">
                  <span v-if="generating" class="spinner" aria-hidden="true"></span
                  ><strong>{{ submitting ? '正在提交制作任务' : statusText(task) }}</strong
                  ><small v-if="task"
                    >{{ task.type === 'animation' ? '教学动画' : '课件' }} ·
                    {{ task.cost }} 积分</small
                  >
                </div>
                <ol class="progress-steps">
                  <li
                    v-for="(step, i) in ['检索与构思', '教师审核', '制作文件', '交付完成']"
                    :key="step"
                    :class="{ current: i === progressStep, done: i < progressStep }"
                  >
                    <span>{{ i < progressStep ? '✓' : i + 1 }}</span
                    >{{ step }}
                  </li>
                </ol>
                <p v-if="!coursewarePaused">
                  {{
                    task?.error ||
                    (submitting
                      ? '正在创建任务，请稍候…'
                      : task?.status === 'awaiting_confirmation'
                        ? '方案已准备好。可编辑教案、大纲和备注，审核后点击下方“确认方案”。'
                        : finished
                          ? '作品已完成，可以直接预览、编辑或下载。'
                          : task?.status === 'cancelled'
                            ? '任务已取消，预留积分已释放。'
                            : task?.events?.at(-1)?.message ||
                              '正在检索资料并组织教学内容，本页面将自动更新。')
                  }}
                </p>
                <div v-if="coursewarePaused" class="recovery-actions">
                  <p>
                    {{
                      draft
                        ? '上次制作已暂停，草稿已保存，可继续审核并制作。'
                        : '上次制作已暂停，课题设置已保留，可继续整理方案。'
                    }}预留积分已释放，本次未扣款。
                  </p>
                  <button
                    class="primary"
                    @click="retryCourseware"
                    :disabled="busy || billing.available < 20 || !can('courseware.generate')"
                  >
                    继续制作</button
                  ><small>重新预留20积分，成功交付后结算。</small>
                  <details>
                    <summary>查看原因与处理建议</summary>
                    <p>{{ task.error }}</p>
                  </details>
                </div>
              </div>
              <div class="tabs" role="tablist">
                <button
                  v-for="t in [
                    ['lesson', '教案'],
                    ['outline', '大纲与编辑'],
                    ['preview', '课件预览'],
                    ['sources', '资料来源']
                  ]"
                  :key="t[0]"
                  role="tab"
                  :aria-selected="tab === t[0]"
                  :class="{ selected: tab === t[0] }"
                  @click="tab = t[0]"
                >
                  {{ t[1]
                  }}<span v-if="t[0] === 'sources' && task">{{ task.sources?.length || 0 }}</span>
                </button>
              </div>
              <div v-if="!draft" class="empty-work">
                <div class="paper-stack">
                  <div></div>
                  <div>
                    <span>LESSON PLAN</span
                    ><b>{{
                      coursewarePaused
                        ? '可以继续制作'
                        : task
                          ? '正在准备教学方案'
                          : '留出思考的空间'
                    }}</b
                    ><i></i><i></i><i></i><em>✦</em>
                  </div>
                </div>
                <h3>
                  {{
                    page === 'animation'
                      ? task
                        ? '资料 → 演示目标 → 动画方案'
                        : '从一个概念，到一段教学演示'
                      : task
                        ? '资料 → 教学目标 → 页面大纲'
                        : '从一个课题，到一份完整课件'
                  }}
                </h3>
                <p>
                  {{
                    coursewarePaused
                      ? '课题设置已保留。点击上方“继续制作”恢复流程。'
                      : task?.error ||
                        '设定教学目标，选择资料来源。我们先整理方案，由你把握课堂的方向。'
                  }}
                </p>
                <div class="feature-tags">
                  <span>{{ page === 'animation' ? '静音 MP4' : '可编辑 PPTX' }}</span
                  ><span>引用可追溯</span><span>教师先确认</span>
                </div>
              </div>
              <div v-else class="output-body">
                <div class="review-note">
                  {{ draft.notice }}<span v-if="task.mode === 'sample'"> · 内置样例</span>
                </div>
                <div v-if="draft.evaluation" class="fine">
                  {{
                    draft.evaluation.status === 'completed'
                      ? '辅助模型评审已完成，仍需教师核验'
                      : draft.evaluation.reason
                  }}
                  <pre v-if="draft.evaluation.assessment" class="assessment">{{
                    JSON.stringify(draft.evaluation.assessment, null, 2)
                  }}</pre>
                </div>
                <section
                  v-if="task.type === 'animation' && draft.animation_quality"
                  class="quality-panel"
                >
                  <b>动画质量检查</b>
                  <p v-for="check in draft.animation_quality.checks" :key="check">{{ check }}</p>
                  <p>{{ draft.animation_quality.visual?.reason || '尚未进行成品画面检查' }}</p>
                  <p
                    v-for="issue in draft.animation_quality.visual?.issues || []"
                    :key="issue"
                    class="error"
                  >
                    {{ issue }}
                  </p>
                  <details v-if="draft.animation_quality.frames?.length">
                    <summary>查看动作关键帧（不代替完整播放）</summary>
                    <button
                      v-for="f in draft.animation_quality.frames"
                      :key="f.index"
                      @click="viewFrame(f.index)"
                      :disabled="busy"
                    >
                      {{ f.second.toFixed(1) }}秒 ↓
                    </button>
                  </details>
                </section>
                <template v-if="tab === 'lesson'"
                  ><label
                    >教案内容<textarea
                      class="lesson-editor"
                      v-model="draft.lesson"
                      :readonly="!canEdit"
                      aria-label="教案内容"
                    ></textarea></label></template
                ><template v-if="tab === 'outline'"
                  ><label class="deck-name"
                    >课件标题<input
                      v-model="draft.title"
                      :readonly="!canEdit"
                      maxlength="100"
                      aria-label="课件标题"
                  /></label>
                  <div class="outline-toolbar">
                    <span>{{ draft.slides.length }} 页 · 点击文字即可编辑</span
                    ><label
                      >模板<select v-model="draft.theme" :disabled="!canEdit">
                        <option v-for="t in templates" :key="t.id" :value="t.id">
                          {{ t.name }}
                        </option>
                      </select></label
                    ><button
                      v-if="finished && task.type === 'courseware'"
                      type="button"
                      @click="run(() => loadHistory())"
                      :disabled="busy"
                    >
                      版本历史
                    </button>
                  </div>
                  <section v-if="historyOpen" class="panel padded history-panel">
                    <div class="panel-title">
                      <div>
                        <h3>版本历史</h3>
                        <p class="fine">手工编辑、单页改写和恢复都会保留旧版；恢复不额外扣积分。</p>
                      </div>
                      <button type="button" @click="historyOpen = false" aria-label="关闭版本历史">
                        ×
                      </button>
                    </div>
                    <p v-if="!history.length" class="empty-small">暂无可恢复版本。</p>
                    <article
                      v-for="v in history"
                      :key="v.revision"
                      class="history-row"
                      :data-revision="v.revision"
                    >
                      <div>
                        <b
                          >修订 {{ v.revision
                          }}{{ v.revision === task.revision ? '（当前）' : '' }} ·
                          {{ v.title || '未命名课件' }}</b
                        ><small class="block">{{ v.pages }} 页 · {{ date(v.created_at) }}</small>
                      </div>
                      <div>
                        <button type="button" @click="viewHistory(v.revision)" :disabled="busy">
                          查看内容</button
                        ><button :disabled="busy" @click="download('pptx', v.revision)">
                          下载旧版</button
                        ><button
                          v-if="can('courseware.generate')"
                          type="button"
                          class="primary"
                          :disabled="busy || v.revision === task.revision"
                          @click="restoreHistory(v.revision)"
                        >
                          恢复此版
                        </button>
                      </div>
                    </article>
                    <div class="works-pagination">
                      <button
                        :disabled="busy || !historyOffset"
                        @click="run(() => loadHistory(historyOffset - 50))"
                      >
                        上一页版本</button
                      ><span>第 {{ historyOffset / 50 + 1 }} 页</span
                      ><button
                        :disabled="busy || !historyMore"
                        @click="run(() => loadHistory(historyOffset + 50))"
                      >
                        下一页版本
                      </button>
                    </div>
                    <details v-if="historyDraft" open class="history-preview">
                      <summary>历史修订 {{ historyDraft.revision }} 的内容</summary>
                      <h4>{{ historyDraft.result.title }}</h4>
                      <p class="fine">{{ historyDraft.result.lesson }}</p>
                      <ol>
                        <li v-for="sl in historyDraft.result.slides" :key="sl.title">
                          <b>{{ sl.title }}</b
                          ><span v-if="sl.bullets?.length">：{{ sl.bullets.join('；') }}</span>
                          <p class="fine history-notes">{{ sl.notes }}</p>
                        </li>
                      </ol>
                    </details>
                  </section>
                  <article class="outline-slide" v-for="(sl, i) in draft.slides" :key="i">
                    <span class="page-number">{{ String(i + 1).padStart(2, '0') }}</span>
                    <div>
                      <input
                        v-model="sl.title"
                        :readonly="!canEdit"
                        :aria-label="'第' + (i + 1) + '页标题'"
                        maxlength="70"
                      /><textarea
                        :value="sl.bullets.join('\n')"
                        @input="sl.bullets = $event.target.value.split('\n').filter(Boolean)"
                        :readonly="!canEdit"
                        :aria-label="'第' + (i + 1) + '页内容'"
                        rows="3"
                      ></textarea>
                      <div v-if="task.type === 'animation' && sl.scene" class="scene-editor">
                        <label
                          >本幕目标<input
                            v-model="sl.scene.goal"
                            :readonly="!canEdit"
                            maxlength="60" /></label
                        ><label
                          >时长（秒）<input
                            type="number"
                            v-model.number="sl.scene.duration_sec"
                            :readonly="!canEdit"
                            min="0.1"
                            max="6"
                            step="0.1" /></label
                        ><label
                          >{{
                            task.input.animation_engine === 'ark' && i > 0
                              ? '动作节点参考文字（不叠加到视频）'
                              : '实际视频字幕'
                          }}<textarea
                            v-model="sl.scene.caption"
                            :readonly="!canEdit"
                            maxlength="40"
                            rows="2"
                          ></textarea></label
                        ><label
                          >{{
                            task.input.animation_engine === 'ark' && i > 0
                              ? '节点参考公式（不叠加到视频）'
                              : '实际视频公式（Unicode）'
                          }}<input v-model="sl.scene.formula" :readonly="!canEdit" maxlength="40"
                        /></label>
                        <p class="fine">
                          证据类型：{{
                            { illustration: '教学示意', numeric: '数值验证', proof: '拼图证明' }[
                              sl.scene.evidence
                            ]
                          }}
                          ·
                          {{
                            task.input.animation_engine === 'ark'
                              ? '情境视频仅叠加首幕字幕与公式，其余节点用于动作参考。'
                              : '视频逐幕使用以上字幕和公式；讲解备注供备课参考。'
                          }}
                        </p>
                        <details>
                          <summary>图形、动作与约束</summary>
                          <pre>{{
                            JSON.stringify(
                              { elements: sl.scene.elements, checks: sl.scene.checks },
                              null,
                              2
                            )
                          }}</pre>
                        </details>
                      </div>
                      <details v-if="task.type === 'courseware'" class="courseware-media-editor">
                        <summary>公式与图表（实际导出内容）</summary>
                        <div class="media-fields">
                          <label
                            >展示公式（LaTeX）<input
                              v-model="sl.formula"
                              :readonly="!canEdit"
                              maxlength="250"
                              placeholder="例如 a^2+b^2=c^2" /></label
                          ><label
                            >可编辑示意图<select
                              :value="sl.diagram || ''"
                              @change="sl.diagram = $event.target.value"
                              :disabled="!canEdit || !!sl.table || !!sl.figure || !!sl.picture"
                            >
                              <option value="">无示意图</option>
                              <option value="triangle">直角三角形（非拼图证明）</option>
                              <option value="buoyancy">浮力方向示意</option>
                            </select></label
                          >
                        </div>
                        <template
                          v-if="
                            sl.chart &&
                            Array.isArray(sl.chart.labels) &&
                            Array.isArray(sl.chart.values)
                          "
                          ><div class="media-fields">
                            <label
                              >图表标题及单位<input
                                v-model="sl.chart.title"
                                :readonly="!canEdit"
                                maxlength="80" /></label
                            ><label
                              >图表类型<select
                                :value="sl.chart.style || 'bar'"
                                @change="sl.chart.style = $event.target.value"
                                :disabled="!canEdit"
                              >
                                <option value="bar">柱状图 · 类别比较</option>
                                <option value="line">折线图 · 等间隔变化</option>
                              </select></label
                            >
                          </div>
                          <div class="media-fields" v-for="(_, j) in sl.chart.labels" :key="j">
                            <label
                              >{{ '第' + (j + 1) + '项标签'
                              }}<input
                                v-model="sl.chart.labels[j]"
                                :readonly="!canEdit"
                                maxlength="18" /></label
                            ><label
                              >数值<input
                                type="number"
                                step="any"
                                v-model.number="sl.chart.values[j]"
                                :readonly="!canEdit"
                            /></label>
                          </div>
                          <p class="fine">
                            折线图使用等距类别轴；不等间隔数值不能按真实距离解读。示例数据须在标题和备注中标明，不能冒充实测。
                          </p></template
                        >
                        <p class="fine">
                          文字、图表和示意图可在PPT中继续编辑；LaTeX公式以清晰图片导出。网页预览不是实际PPT截图。
                        </p>
                      </details>
                      <CoursewareEvidence
                        v-if="task.type === 'courseware'"
                        :slide="sl"
                        :editable="canEdit"
                      />
                      <div class="slide-edit-options">
                        <label
                          >版式<select v-model="sl.layout" :disabled="!canEdit">
                            <option value="">自动编排</option>
                            <option v-for="l in layouts" :key="l[0]" :value="l[0]">
                              {{ l[1] }}
                            </option>
                          </select></label
                        >
                        <details>
                          <summary>讲解备注与答案</summary>
                          <textarea
                            v-model="sl.notes"
                            :readonly="!canEdit"
                            :aria-label="'第' + (i + 1) + '页讲解备注'"
                            rows="5"
                          ></textarea>
                        </details>
                      </div>
                    </div>
                    <button
                      @click="
                        selectedSlide = i;
                        tab = 'preview';
                      "
                      :aria-label="'预览第' + (i + 1) + '页'"
                    >
                      ↗
                    </button>
                  </article></template
                ><template v-if="tab === 'preview'"
                  ><video
                    v-if="task.type === 'animation' && videoURL"
                    :src="videoURL"
                    controls
                    class="animation-player"
                  ></video>
                  <div
                    v-else
                    :class="[
                      'slide-preview',
                      draft.theme,
                      'layout-' + slideLayout(currentSlide, selectedSlide)
                    ]"
                  >
                    <small>备课工坊 · {{ draft.title }}</small
                    ><span class="preview-kicker"
                      >{{
                        layouts.find((l) => l[0] === slideLayout(currentSlide, selectedSlide))?.[1]
                      }}
                      / {{ String(selectedSlide + 1).padStart(2, '0') }}</span
                    >
                    <h2>
                      <EmphasisText
                        :text="currentSlide?.title || ''"
                        :terms="currentSlide?.emphasis || []"
                      />
                    </h2>
                    <ul>
                      <li v-for="(b, i) in currentSlide?.bullets" :key="i">
                        <span class="bullet-index">{{ String(i + 1).padStart(2, '0') }}</span
                        ><EmphasisText :text="b" :terms="currentSlide?.emphasis || []" />
                      </li>
                    </ul>
                    <CoursewareEvidence v-if="currentSlide" :slide="currentSlide" preview />
                    <p v-if="currentSlide?.formula" class="preview-formula">
                      {{ currentSlide.formula }}
                    </p>
                    <div class="preview-footer">
                      <span>布局示意 · 以导出PPTX为准</span
                      ><span>{{ selectedSlide + 1 }} / {{ draft.slides.length }}</span>
                    </div>
                  </div>
                  <div class="slide-controls">
                    <button @click="selectedSlide = Math.max(0, selectedSlide - 1)">← 上一页</button
                    ><button v-if="task.type === 'courseware'" @click="animationFromSlide">
                      ◈ 以此页设计动画</button
                    ><button
                      @click="selectedSlide = Math.min(draft.slides.length - 1, selectedSlide + 1)"
                    >
                      下一页 →
                    </button>
                  </div>
                  <button
                    v-if="finished && task.type === 'courseware'"
                    @click="rewrite"
                    :disabled="busy || billing.available < 5"
                  >
                    ✦ 单页AI改写 · 5积分
                  </button></template
                ><template v-if="tab === 'sources'"
                  ><TeachingClaimSources
                    v-for="(sl, i) in draft.slides"
                    :key="sl.id || i"
                    :slide="sl"
                    :sources="task.sources"
                    :editable="canEdit"
                  />
                  <p v-if="!task.sources?.length" class="muted">
                    未找到可引用资料。当前内容需教师核验；确认交付后按普通价格结算。
                  </p>
                  <article class="source-card" v-for="s in task.sources" :key="s.id">
                    <span class="source-id">{{ s.id }}</span>
                    <div>
                      <h3>{{ s.title }}</h3>
                      <small
                        >{{
                          s.kind === 'search_summary'
                            ? '搜索摘要，未抓取正文'
                            : s.kind === 'knowledge'
                              ? '企业知识库'
                              : s.kind === 'web_extracted'
                                ? '搜索服务提取正文'
                                : '公开网页'
                        }}
                        · {{ s.location }}</small
                      >
                      <p v-if="s.filter" class="fine">
                        {{
                          s.filter.method.includes('vector-candidate')
                            ? '向量候选，未按关键词删段'
                            : `相关段落保留 ${s.filter.kept} / ${s.filter.total}`
                        }}
                        ·
                        {{
                          s.filter.method.includes('rerank')
                            ? '已语义重排'
                            : s.filter.method.includes('vector-candidate')
                              ? '未启用语义重排'
                              : '关键词与上下文筛选'
                        }}
                        · 非事实可信度评分<span v-if="s.filter.truncated">
                          · 篇幅受限，请查看原文</span
                        >
                      </p>
                      <p>{{ s.content }}</p>
                      <a
                        v-if="s.url && /^https?:\/\//.test(s.url)"
                        :href="s.url"
                        target="_blank"
                        rel="noopener noreferrer"
                        >查看来源 ↗</a
                      >
                    </div>
                  </article></template
                >
              </div>
              <div v-if="task" class="output-actions">
                <label
                  v-if="
                    task.type === 'courseware' &&
                    task.status === 'awaiting_confirmation' &&
                    draft?.needs_review
                  "
                  class="consent"
                  ><input type="checkbox" v-model="accept" />我已知悉资料需核验，确认{{
                    task.type === 'animation' ? '成品验收' : '完整交付'
                  }}后扣 {{ task.cost }} 积分</label
                >
                <div class="action-row">
                  <button
                    v-if="task.status === 'awaiting_acceptance'"
                    @click="run(() => previewAnimation())"
                    :disabled="busy"
                  >
                    播放历史视频</button
                  ><button v-if="task.type === 'animation'" @click="newWork('animation')">
                    新建 HTML 动画</button
                  ><button @click="showProcess = true">◷ 查看制作过程</button
                  ><button
                    v-if="!['succeeded', 'degraded', 'failed', 'cancelled'].includes(task.status)"
                    @click="cancel"
                    :disabled="busy"
                  >
                    取消并释放</button
                  ><button
                    v-if="task.type === 'courseware' && task.status === 'awaiting_confirmation'"
                    class="primary"
                    @click="confirmGeneration"
                    :disabled="busy || (draft.needs_review && !accept)"
                  >
                    确认方案，开始制作 →</button
                  ><template v-if="finished"
                    ><button @click="download('md')" :disabled="busy">教案 MD ↓</button
                    ><button v-if="task.type !== 'animation'" @click="save" :disabled="busy">
                      免费保存并重导出</button
                    ><button
                      class="primary"
                      @click="download(task.type === 'animation' ? 'mp4' : 'pptx')"
                      :disabled="busy"
                    >
                      {{ task.type === 'animation' ? '预览 / 下载 MP4' : '下载 PPTX' }} ↓
                    </button></template
                  >
                </div>
              </div>
            </section>
            <section
              v-if="page === 'courseware' && task"
              class="panel template-shelf"
              aria-label="横向课件模板"
            >
              <div class="template-heading">
                <h2>选择课件模板</h2>
                <small
                  >横向浏览 · 当前：{{ templates.find((t) => t.id === form.theme)?.name }}</small
                >
              </div>
              <div class="themes horizontal-themes">
                <label
                  v-for="t in templates"
                  :key="t.id"
                  :class="['theme', t.id, { chosen: form.theme === t.id }]"
                  :title="t.description"
                  ><input type="radio" v-model="form.theme" :value="t.id" name="theme" /><span
                    class="theme-mini"
                    aria-hidden="true"
                    ><b>{{ t.mark }}</b
                    ><i></i><i></i><em>课堂探索</em></span
                  ><strong>{{ t.name }}</strong
                  ><small>{{ t.tag }}</small></label
                >
              </div>
            </section>
          </div>
        </template>
        <template v-else-if="page === 'knowledge'"
          ><div class="page-heading">
            <div>
              <div class="eyebrow">A SHARED FOUNDATION</div>
              <h1>好课，有据可依。</h1>
              <p>企业共享知识库。上传原文，保留出处，按需检索。</p>
            </div>
            <label v-if="can('kb.manage')" class="button primary"
              >＋ 导入文档<input
                type="file"
                accept=".pdf,.docx,.pptx,.txt,.md"
                @change="upload"
                :disabled="busy"
                class="file-input"
            /></label>
          </div>
          <div class="two-columns">
            <section class="panel padded">
              <h2>
                资料库 <small>{{ docs.length }} 份文档</small>
              </h2>
              <p class="fine">PDF（文字版）/ DOCX / PPTX / TXT / MD · 单份不超过100MB</p>
              <div v-if="!docs.length" class="empty-small">还没有资料，先上传一份教研文档。</div>
              <article v-for="d in docs" :key="d.id" class="document-row">
                <span class="doc-icon">▤</span>
                <div>
                  <h3>{{ d.name }}</h3>
                  <small
                    >{{
                      {
                        ready: '已就绪',
                        pending: '排队中',
                        indexing: '解析入库中',
                        failed: '解析失败'
                      }[d.status] || d.status
                    }}
                    · {{ d.chunks }} 个切片 · {{ date(d.created_at) }}</small
                  >
                </div>
                <button
                  v-if="
                    can('kb.manage') &&
                    d.status === 'ready' &&
                    d.original_available &&
                    health.document_reparse_version === 'reparse-v1'
                  "
                  @click="reparseDoc(d)"
                  :disabled="busy"
                >
                  重新解析</button
                ><button
                  v-if="can('kb.manage') && d.status === 'failed'"
                  @click="
                    run(async () => {
                      await api('/documents/' + d.id + '/retry', 'POST', {});
                      docs = await api('/documents');
                    })
                  "
                  :disabled="busy"
                >
                  重试</button
                ><button v-if="can('kb.manage')" @click="removeDoc(d.id)" :disabled="busy">
                  删除
                </button>
              </article>
            </section>
            <section class="panel padded">
              <h2>检索测试</h2>
              <form class="inline-form" @submit.prevent="search">
                <input v-model="query" placeholder="输入知识点，如：浮力" required /><button
                  class="primary"
                  :disabled="busy"
                >
                  检索
                </button>
              </form>
              <article v-for="s in sources" :key="s.id" class="source-card">
                <div>
                  <h3>{{ s.title }}</h3>
                  <small>{{ s.location }}</small>
                  <p v-if="s.filter" class="fine">
                    {{
                      s.filter.method.includes('vector-candidate')
                        ? '向量候选，未按关键词删段'
                        : `相关段落保留 ${s.filter.kept} / ${s.filter.total}`
                    }}
                    ·
                    {{
                      s.filter.method.includes('rerank')
                        ? '已语义重排'
                        : s.filter.method.includes('vector-candidate')
                          ? '未启用语义重排'
                          : '关键词与上下文筛选'
                    }}
                    · 非事实可信度评分<span v-if="s.filter.truncated"> · 篇幅受限，请查看原文</span>
                  </p>
                  <p>{{ s.content }}</p>
                </div>
              </article>
              <hr />
              <h2>公开网页入库</h2>
              <p class="fine">先抓取预览，再确认加入企业知识库。</p>
              <form class="inline-form" @submit.prevent="importPage(false)">
                <input type="url" v-model="url" placeholder="https://…" required /><button
                  :disabled="busy || !can('kb.manage')"
                >
                  预览
                </button>
              </form>
              <div v-if="webPreview" class="web-preview">
                <h3>{{ webPreview.title }}</h3>
                <pre>{{ webPreview.preview }}</pre>
                <button class="primary" @click="importPage(true)" :disabled="busy">确认入库</button>
              </div>
            </section>
          </div></template
        >
        <template v-else-if="page === 'tasks'">
          <div class="page-heading">
            <div>
              <div class="eyebrow">YOUR TEACHING COLLECTION</div>
              <h1>每次思考，都有留存。</h1>
              <p>查找、编辑与管理你的课件和教学动画。作品仅本人可见。</p>
            </div>
            <div class="collection-create">
              <button @click="newWork('animation')">＋ 新建动画</button
              ><button class="primary" @click="newWork('courseware')">＋ 新建课件</button>
            </div>
          </div>
          <div class="works-views" role="group" aria-label="作品视图">
            <button
              v-for="v in [
                ['works', '当前作品'],
                ['interrupted', '待处理与已取消'],
                ['history', '全部记录']
              ]"
              :key="v[0]"
              type="button"
              :data-view="v[0]"
              :aria-pressed="worksView === v[0]"
              :disabled="busy || worksLoading || health.works_view_version !== 'views-v1'"
              @click="changeWorksView(v[0])"
            >
              {{ v[1] }}
            </button>
          </div>
          <p v-if="cleanupPending" class="alert cleanup-status" role="status">
            {{ cleanupPending }}
            项已删除记录的资源正在清理。后台会等待制作退出并在连接恢复后自动重试；账单不受影响。
          </p>
          <form class="panel works-toolbar" @submit.prevent="run(() => loadWorks(true))">
            <label class="works-search"
              >搜索作品<input
                v-model="worksQuery"
                type="search"
                placeholder="搜索作品名称或课题"
                maxlength="100" /></label
            ><label
              >作品类型<select v-model="worksType" @change="run(() => loadWorks(true))">
                <option value="">全部类型</option>
                <option value="courseware">课件</option>
                <option value="animation">动画</option>
                <option value="revision">单页修改</option>
              </select></label
            ><label
              >制作状态<select
                v-model="worksStatus"
                aria-label="制作状态"
                @change="run(() => loadWorks(true))"
              >
                <option value="">全部状态</option>
                <option v-for="(label, key) in worksStatusOptions" :key="key" :value="key">
                  {{ label }}
                </option>
              </select></label
            ><button class="primary" :disabled="worksLoading || busy">
              {{ worksLoading ? '查询中…' : '查询' }}</button
            ><button
              type="button"
              :disabled="busy"
              @click="
                worksView = 'works';
                worksQuery = '';
                worksType = '';
                worksStatus = '';
                run(() => loadWorks(true));
              "
            >
              重置
            </button>
          </form>
          <p v-if="health.works_view_version !== 'views-v1'" class="alert" role="status">
            作品收纳功能需要重启后端后生效，目前仍展示全部记录，不会删除作品。重启并刷新后可按视图收纳。
          </p>
          <p class="fine works-view-note">
            {{
              worksView === 'history'
                ? '这里保留历次制作记录，可查看原因或手动删除；积分明细仍保留在账单中。'
                : worksView === 'interrupted'
                  ? '中断或取消的作品集中在这里。还原只移回当前作品，不会自动制作或扣积分；删除会清理独占资源，删除后不可还原。'
                  : '展示制作中、待审核、已完成及手动还原的作品。同一课件续作只显示最新一次；中断和取消记录可在「待处理与已取消」还原或删除。'
            }}
          </p>
          <p v-if="workItems.some((t) => t.capacity_hold)" class="alert" role="status">
            待处理记录已达到十二条，额外草稿已保存在当前作品。请清理不再需要的记录，已有页面不会自动删除。
          </p>
          <div v-if="workItems.length" class="work-selection">
            <label class="inline-check"
              ><input
                type="checkbox"
                :checked="allWorksSelected"
                :indeterminate="selectedWorks.length > 0 && !allWorksSelected"
                :disabled="busy || worksLoading || !deletableWorks.length"
                @change="
                  selectedWorks = $event.target.checked ? deletableWorks.map((t) => t.id) : []
                "
                aria-label="全选本页可删除作品"
              />全选本页</label
            ><span>已选 {{ selectedWorks.length }} 项</span
            ><button
              class="danger-link"
              :disabled="busy || worksLoading || !selectedWorks.length"
              @click="bulkDeleteOpen = true"
            >
              <WorkspaceIcon name="trash" />删除所选</button
            ><span v-if="worksView === 'interrupted'"
              >最多保留十二条；已有页面的草稿不会自动删除</span
            >
          </div>
          <div class="task-grid">
            <article
              v-for="t in workItems"
              :key="t.id"
              class="panel task-card"
              :data-task-id="t.id"
            >
              <label class="work-checkbox"
                ><input
                  type="checkbox"
                  v-model="selectedWorks"
                  :value="t.id"
                  :disabled="busy || worksLoading || !terminal.includes(t.status)"
                  :aria-label="'选择作品 ' + (t.display_title || t.input.topic)" /></label
              ><button
                class="task-open"
                @click="openWork(t)"
                :aria-label="'打开作品 ' + (t.display_title || t.input.topic)"
              >
                <div :class="['task-art', t.input.theme || 'forest']">
                  <span>{{ t.type === 'animation' ? '◈' : '▤' }}</span
                  ><small>{{ t.mode === 'sample' ? '本地示例' : 'AI生成' }}</small
                  ><b>{{ t.display_title || t.input.topic }}</b>
                </div>
                <div class="task-card-body">
                  <span :class="['status', t.status]">{{ statusText(t) }}</span>
                  <h3>{{ t.display_title || t.input.topic }}</h3>
                  <p>
                    {{
                      t.type === 'animation'
                        ? '教学动画'
                        : t.type === 'revision'
                          ? '单页修改'
                          : '可编辑课件'
                    }}
                    · {{ t.cost }} 积分
                  </p>
                  <small>{{ date(t.created_at) }}</small>
                </div>
              </button>
              <div class="work-actions">
                <button
                  v-if="
                    resourceActions &&
                    ['failed', 'cancelled'].includes(t.status) &&
                    !t.restored_at &&
                    !t.has_delivery
                  "
                  class="restore-work"
                  @click="restoreWork(t)"
                  :disabled="busy"
                  :aria-label="'还原作品 ' + (t.display_title || t.input.topic)"
                >
                  还原</button
                ><button @click="openWork(t)">
                  {{
                    t.type === 'animation' && t.has_delivery
                      ? '查看 / 修改'
                      : ['awaiting_confirmation', 'anim_plan'].includes(t.status)
                        ? '审核方案'
                        : t.type === 'courseware' && t.status === 'succeeded'
                          ? '查看 / 编辑'
                          : '查看进度'
                  }}</button
                ><button
                  @click="
                    renameItem = t;
                    renameTitle = t.display_title || t.input.topic;
                  "
                  :disabled="busy"
                >
                  重命名</button
                ><button
                  class="danger-link"
                  @click="deleteItem = t"
                  :disabled="busy || !terminal.includes(t.status)"
                  :title="
                    terminal.includes(t.status) ? '删除此作品' : '请先打开作品并取消任务，再删除'
                  "
                >
                  删除
                </button>
              </div>
            </article>
          </div>
          <p v-if="!workItems.length && !worksLoading" class="empty-small">
            {{
              worksQuery || worksType || worksStatus
                ? '没有找到匹配的作品，试试其他筛选条件。'
                : worksView === 'works'
                  ? '当前没有制作中、待审核或已完成的作品。中断的作品可切换至「待处理与已取消」查看。'
                  : worksView === 'interrupted'
                    ? '没有待处理或已取消的作品。'
                    : '暂无制作记录。'
            }}
          </p>
          <div class="works-pagination">
            <button
              :disabled="!worksOffset || worksLoading"
              @click="
                worksOffset -= 50;
                run(() => loadWorks());
              "
            >
              上一页</button
            ><span>第 {{ worksOffset / 50 + 1 }} 页</span
            ><button
              :disabled="!worksMore || worksLoading"
              @click="
                worksOffset += 50;
                run(() => loadWorks());
              "
            >
              下一页
            </button>
          </div>
        </template>
        <BillingPage
          v-else-if="page === 'billing'"
          :billing="billing"
          :busy="busy"
          :date="date"
          @buy="buy"
          @pay="pay"
        />
        <AccountSettings
          v-else-if="page === 'account'"
          :api="api"
          @renamed="me.name = $event"
          @password-changed="passwordChanged"
        />
        <template v-else-if="page === 'admin'"
          ><div class="page-heading">
            <div>
              <div class="eyebrow">GROW TOGETHER</div>
              <h1>一起教研，各自从容。</h1>
              <p>管理企业成员与权限；个人作品仍归创作者私有。</p>
            </div>
          </div>
          <section class="panel padded">
            <h2>新增成员</h2>
            <form class="member-form" @submit.prevent="addMember">
              <label
                >账号<input
                  v-model="newMember.username"
                  name="member-username"
                  required
                  placeholder="3–40位字母数字"
                  autocomplete="off"
                  spellcheck="false"
                  autocapitalize="off" /></label
              ><label
                >姓名<input
                  v-model="newMember.name"
                  name="member-name"
                  required
                  maxlength="30"
                  autocomplete="off" /></label
              ><label
                >初始密码<input
                  v-model="newMember.password"
                  type="password"
                  required
                  minlength="10"
                  autocomplete="new-password" /></label
              ><button class="primary" :disabled="busy">创建成员</button>
            </form>
            <div class="table-scroll">
              <table>
                <thead>
                  <tr>
                    <th>成员</th>
                    <th>角色</th>
                    <th>余额 / 预留</th>
                    <th>可用功能</th>
                    <th>启用</th>
                    <th></th>
                  </tr>
                </thead>
                <tbody>
                  <tr v-for="m in members" :key="m.id">
                    <td>
                      {{ m.display_name }}<small class="block">{{ m.username }}</small>
                    </td>
                    <td>{{ m.role === 'admin' ? '管理员' : '教师' }}</td>
                    <td>{{ m.balance }} / {{ m.reserved }}</td>
                    <td>
                      <label
                        v-for="p in [
                          ['courseware.generate', '课件'],
                          ['animation.render', '动画'],
                          ['kb.manage', '知识库']
                        ]"
                        :key="p[0]"
                        class="inline-check"
                        ><input
                          type="checkbox"
                          v-model="m.permissions"
                          :value="p[0]"
                          :disabled="m.role === 'admin'"
                        />{{ p[1] }}</label
                      >
                    </td>
                    <td>
                      <input
                        type="checkbox"
                        v-model="m.active"
                        :disabled="m.role === 'admin'"
                        :aria-label="'启用' + m.display_name"
                      />
                    </td>
                    <td>
                      <button v-if="m.role !== 'admin'" @click="updateMember(m)" :disabled="busy">
                        保存</button
                      ><button
                        v-if="can('billing.manage')"
                        @click="assignPackage(m, 'basic')"
                        :disabled="busy"
                      >
                        分配基础套餐
                      </button>
                    </td>
                  </tr>
                </tbody>
              </table>
            </div>
          </section></template
        >
        <template v-else-if="page === 'dashboard'"
          ><div class="page-heading">
            <div>
              <div class="eyebrow">OBSERVE & IMPROVE</div>
              <h1>看见过程，持续精进。</h1>
              <p>
                {{ me.role === 'admin' ? '当前企业汇总' : '仅本人成员数据' }} ·
                调用用量与业务积分分开记录
              </p>
            </div>
          </div>
          <div class="stat-grid">
            <div class="panel stat">
              <span>累计调用</span><strong>{{ dashboard.totals?.calls || 0 }}</strong
              ><small>删除明细后保留累计用量</small>
            </div>
            <div class="panel stat">
              <span>模型调用P95</span
              ><strong>{{ (dashboard.p95_ms / 1000 || 0).toFixed(1) }}<small> 秒</small></strong
              ><small>当前保留明细范围；无明细时为0</small>
            </div>
            <div class="panel stat">
              <span>累计Token</span
              ><strong>{{
                (dashboard.totals?.input_tokens || 0) + (dashboard.totals?.output_tokens || 0)
              }}</strong
              ><small>失败调用 {{ dashboard.totals?.failures || 0 }} 次</small>
            </div>
          </div>
          <UsageRecords
            :api="api"
            @changed="run(async () => (dashboard = await api('/dashboard')))"
          />
          <section v-if="can('eval.run')" class="panel padded spaced">
            <h2>知识检索评测</h2>
            <p class="fine">使用固定标注集检查来源召回；不将模型自评当作事实证明。</p>
            <button @click="evaluate" :disabled="busy">运行小样本评测</button>
          </section></template
        >
        <footer class="page-footer">
          <span>备课工坊 · 为真正的教学而设计</span><span>AI内容需教师审核 · 当前版本 0.1</span>
        </footer>
      </div>
    </main>
    <div
      v-if="renameItem || deleteItem"
      class="modal-backdrop"
      @click.self="!busy && ((renameItem = null), (deleteItem = null))"
    >
      <section
        class="work-dialog"
        role="dialog"
        aria-modal="true"
        :aria-label="renameItem ? '重命名作品' : '删除作品'"
        @keydown="trapDialog"
        @keydown.esc="!busy && ((renameItem = null), (deleteItem = null))"
      >
        <form v-if="renameItem" @submit.prevent="renameWork">
          <h2>重命名作品</h2>
          <p class="fine">修改作品列表名称；课件内的页面内容可在编辑器中修改。</p>
          <label>作品名称<input v-model="renameTitle" maxlength="100" required autofocus /></label>
          <div class="action-row">
            <button type="button" @click="renameItem = null" :disabled="busy">取消</button
            ><button class="primary" :disabled="busy || !renameTitle.trim()">保存名称</button>
          </div>
        </form>
        <template v-else
          ><h2>删除“{{ deleteItem.display_title || deleteItem.input.topic }}”？</h2>
          <p v-if="resourceActions">
            将永久删除该作品的草稿、历史版本及独占的文件和预览，删除后不可还原。其他作品仍引用的文件会保留。已结算积分不退回，历史账单保留。
          </p>
          <p v-else>删除后无法再从作品列表查看或下载。已结算积分不退回，历史账单保留。</p>
          <div class="action-row">
            <button @click="deleteItem = null" :disabled="busy">保留作品</button
            ><button class="danger" @click="deleteWork" :disabled="busy">确认删除</button>
          </div></template
        >
      </section>
    </div>
    <div
      v-if="bulkDeleteOpen"
      class="modal-backdrop"
      @click.self="!busy && (bulkDeleteOpen = false)"
    >
      <section
        class="work-dialog"
        role="dialog"
        aria-modal="true"
        aria-label="批量删除作品"
        @keydown="trapDialog"
        @keydown.esc="!busy && (bulkDeleteOpen = false)"
      >
        <h2>删除 {{ selectedWorks.length }} 个作品？</h2>
        <p>
          草稿、历史版本及独占文件将被清理，删除后不可还原。仍被其他作品引用的文件及积分账单保留。
        </p>
        <div class="action-row">
          <button :disabled="busy" @click="bulkDeleteOpen = false">保留作品</button
          ><button class="danger" :disabled="busy" @click="deleteSelectedWorks">
            <WorkspaceIcon name="trash" />确认删除
          </button>
        </div>
      </section>
    </div>
    <div v-if="showProcess" class="modal-backdrop" @click.self="showProcess = false">
      <section
        class="process-dialog"
        role="dialog"
        aria-modal="true"
        aria-label="制作过程"
        @keydown.esc="showProcess = false"
        @keydown="trapDialog"
      >
        <div class="panel-title">
          <div>
            <span class="eyebrow">BEHIND THE LESSON</span>
            <h2>每一步，都有迹可循。</h2>
          </div>
          <button autofocus @click="showProcess = false" aria-label="关闭过程">×</button>
        </div>
        <details v-for="group in processGroups" :key="group.stage" open>
          <summary>{{ group.title }}</summary>
          <div v-for="ev in group.events" :key="ev.id" class="event">
            <span>{{ date(ev.created_at) }}</span>
            <p>{{ ev.message }}</p>
          </div>
        </details>
        <details v-if="task.result?.script">
          <summary>动画代码</summary>
          <pre class="code-preview">{{ task.result.script }}</pre>
        </details>
        <p class="fine">
          流水号 {{ task.id }} · 修订 {{ task.revision }} ·
          {{
            task.charge_state === 'settled'
              ? '已结算'
              : task.charge_state === 'released'
                ? '已释放'
                : '预留中'
          }}
        </p>
      </section>
    </div>
  </div>
</template>
