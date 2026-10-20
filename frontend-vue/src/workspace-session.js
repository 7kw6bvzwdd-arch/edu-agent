import { watch } from 'vue';

export const emptyCourseware = () => ({
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
export const emptyHomeSession = () => ({
  messages: [],
  text: '',
  summary: '',
  understanding: '',
  selectedTool: '',
  suggestedTool: 'courseware',
  contextMemory: { text: '', through: 0 },
  conversationId: '',
  conversationRevision: 0
});

// Restoring history must never re-submit a task whose creation was interrupted.
export function restoreHomeMessages(messages) {
  return (Array.isArray(messages) ? messages : []).map((message) => {
    const copy = { ...message };
    if (copy.pending) {
      copy.pending = false;
      copy.failed = true;
      copy.error = '连接已中断，可继续补充需求。';
    }
    if (copy.execution && !copy.execution.taskId) {
      delete copy.execution;
      copy.failed = true;
      copy.error = '上次制作未取得任务编号，请先在我的作品核对，避免重复制作。';
    }
    return copy;
  });
}

const pages = new Set([
  'home',
  'tools',
  'tasks',
  'knowledge',
  'account',
  'billing',
  'dashboard',
  'admin',
  'courseware',
  'animation'
]);
export function restoreWorkspace({
  me,
  homeSession,
  homeForm,
  form,
  page,
  task,
  selectTask,
  clearTask,
  defaults
}) {
  let identity = '',
    restoring = false,
    routeVersion = 0;
  const key = (id) => 'edu_workspace_v1:' + id;
  const empty = emptyHomeSession;
  const route = () => {
    try {
      const [name, search = ''] = location.hash.slice(1).split('?');
      return { page: pages.has(name) ? name : 'home', id: new URLSearchParams(search).get('task') };
    } catch {
      return { page: 'home' };
    }
  };
  async function openRoute() {
    if (!identity) return;
    const owner = identity,
      version = ++routeVersion;
    const r = route();
    restoring = true;
    try {
      if (
        r.id &&
        /^[a-zA-Z0-9_-]{8,64}$/.test(r.id) &&
        ['home', 'courseware', 'animation'].includes(r.page)
      )
        await selectTask(r.id);
      else if (task.value) clearTask();
      if (owner === identity && version === routeVersion) page.value = r.page;
    } catch {
      if (owner !== identity || version !== routeVersion) return;
      clearTask();
      page.value = 'home';
      history.replaceState(null, '', '#home');
    } finally {
      if (owner === identity && version === routeVersion) restoring = false;
    }
  }
  watch(
    () => [me.value?.tenant_id, me.value?.member_id].filter(Boolean).join(':'),
    async (next) => {
      const previous = identity;
      routeVersion++;
      if (previous && previous !== next) {
        try {
          sessionStorage.removeItem(key(previous));
        } catch {}
      }
      identity = next;
      restoring = true;
      Object.assign(homeSession, empty());
      Object.assign(form, defaults());
      Object.assign(homeForm, defaults());
      if (!next) {
        clearTask();
        restoring = false;
        return;
      }
      try {
        const saved = JSON.parse(sessionStorage.getItem(key(next)) || 'null');
        if (saved?.version === 1) {
          Object.assign(homeSession, empty(), saved.session);
          homeSession.messages = restoreHomeMessages(homeSession.messages);
          Object.assign(homeForm, defaults(), saved.homeForm);
          Object.assign(form, defaults(), saved.form);
        }
      } catch {
        try {
          sessionStorage.removeItem(key(next));
        } catch {}
      }
      if (previous && previous !== next) {
        clearTask();
        history.replaceState(null, '', '#home');
      }
      restoring = false;
      await openRoute();
    },
    { flush: 'pre' }
  );
  watch(
    [homeSession, homeForm, form],
    () => {
      if (!identity || restoring) return;
      try {
        sessionStorage.setItem(
          key(identity),
          JSON.stringify({ version: 1, session: homeSession, homeForm, form })
        );
      } catch {}
    },
    { deep: true }
  );
  watch(
    [page, () => task.value?.id],
    () => {
      if (restoring || !identity) return;
      const id = task.value?.id,
        hash =
          '#' +
          page.value +
          (id && ['home', 'courseware', 'animation'].includes(page.value)
            ? '?task=' + encodeURIComponent(id)
            : '');
      if (location.hash !== hash) history.replaceState(null, '', hash);
    },
    { flush: 'post' }
  );
  window.addEventListener('hashchange', openRoute);
  return () => window.removeEventListener('hashchange', openRoute);
}
