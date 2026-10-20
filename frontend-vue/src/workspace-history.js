import { ref, watch, onMounted, onUnmounted, nextTick } from 'vue';
import { emptyHomeSession, emptyCourseware, restoreHomeMessages } from './workspace-session.js';

// Saves only conversation data. Existing task IDs are links, never new generation requests.
export function useWorkspaceHistory({ enabled, api, session, form, blocked, beforeReplace }) {
  const opened = ref(false),
    items = ref([]),
    loading = ref(false),
    saving = ref(false);
  const error = ref(''),
    notice = ref(''),
    more = ref(false),
    acting = ref(false);
  let timer,
    disposed = false,
    replacing = false,
    epoch = 0,
    saved = '',
    tail = Promise.resolve();
  const route = '/workspace/conversations';
  const snapshot = () => {
    const { conversationId, conversationRevision, ...data } = session;
    return JSON.stringify({ version: 1, session: data, form });
  };
  function save() {
    clearTimeout(timer);
    const version = epoch;
    const run = async () => {
      if (
        disposed ||
        version !== epoch ||
        !enabled() ||
        replacing ||
        blocked() ||
        !session.messages.length
      )
        return;
      const state = snapshot();
      if (state === saved) return;
      session.conversationId ||= crypto.randomUUID();
      const id = session.conversationId;
      saving.value = true;
      try {
        const result = await api(route + '/' + id, 'PUT', {
          revision: session.conversationRevision || 0,
          state: JSON.parse(state)
        });
        if (disposed || version !== epoch) return;
        session.conversationRevision = result.revision;
        saved = state;
        error.value = '';
      } catch (e) {
        if (!disposed && version === epoch) {
          error.value = '历史记录未保存：' + e.message;
          throw e;
        }
      } finally {
        if (!disposed && version === epoch) saving.value = false;
      }
    };
    tail = tail.catch(() => {}).then(run);
    return tail;
  }
  function schedule() {
    clearTimeout(timer);
    if (!disposed && !replacing && !acting.value && enabled() && !blocked())
      timer = setTimeout(() => save().catch(() => {}), 600);
  }
  watch([session, form, blocked], schedule, { deep: true });
  async function list(reset = true) {
    const version = epoch;
    loading.value = true;
    try {
      const rows = await api(route + (reset ? '' : '?offset=' + items.value.length));
      if (disposed || version !== epoch) return;
      items.value = reset ? rows : items.value.concat(rows);
      more.value = rows.length === 50;
    } catch (e) {
      if (!disposed) error.value = e.message;
    } finally {
      if (!disposed) loading.value = false;
    }
  }
  async function panel() {
    opened.value = !opened.value;
    if (opened.value) {
      error.value = '';
      await save().catch(() => {});
      if (!disposed) await list();
    }
  }
  async function replace(state, id = '', revision = 0) {
    if (disposed) return;
    replacing = true;
    epoch++;
    clearTimeout(timer);
    beforeReplace(true);
    Object.assign(form, emptyCourseware(), state?.form);
    Object.assign(session, emptyHomeSession(), state?.session, {
      conversationId: id,
      conversationRevision: revision
    });
    session.messages = restoreHomeMessages(session.messages);
    await nextTick();
    saved = snapshot();
    replacing = false;
    saving.value = false;
  }
  async function action(fn) {
    if (acting.value || !enabled()) return;
    acting.value = true;
    clearTimeout(timer);
    notice.value = '';
    error.value = '';
    try {
      await fn();
    } catch (e) {
      if (!disposed) error.value = e.message;
    } finally {
      await nextTick();
      if (!disposed) {
        acting.value = false;
        schedule();
      }
    }
  }
  const newConversation = () =>
    action(async () => {
      beforeReplace();
      await nextTick();
      await save();
      if (disposed) return;
      await replace();
      opened.value = false;
      notice.value = '已开始新对话，上一段可在历史消息中查看。';
    });
  const restore = (id) =>
    action(async () => {
      beforeReplace();
      await nextTick();
      await save();
      if (disposed) return;
      const row = await api(route + '/' + id);
      if (disposed) return;
      await replace(row.state, row.id, row.revision);
      opened.value = false;
      notice.value = '历史对话已打开，可以继续沟通。';
    });
  async function remove(id) {
    await tail.catch(() => {});
    if (disposed) return;
    await api(route + '/' + id, 'DELETE');
    if (disposed) return;
    if (id === session.conversationId) await replace();
    await list();
    notice.value = '对话已清除，已生成作品仍在“我的作品”。';
  }
  const clearCurrent = () =>
    action(async () => {
      beforeReplace();
      clearTimeout(timer);
      await tail.catch(() => {});
      if (disposed) return;
      const id = session.conversationId;
      if (id) await remove(id);
      else await replace();
      notice.value = '当前对话、需求摘要和对话记忆已清空，已生成作品保留。';
    });
  const deleteConversation = (id) =>
    action(async () => {
      if (id === session.conversationId) beforeReplace();
      await remove(id);
    });
  const clearAll = () => {
    if (!window.confirm('清除全部历史对话和当前对话？已生成作品会保留，此操作无法撤销。')) return;
    return action(async () => {
      beforeReplace();
      clearTimeout(timer);
      await tail.catch(() => {});
      if (disposed) return;
      await api(route, 'DELETE');
      if (disposed) return;
      await replace();
      items.value = [];
      more.value = false;
      notice.value = '全部历史对话已清除，已生成作品保留。';
    });
  };
  const pageHide = () => {
    if (!acting.value) void save().catch(() => {});
  };
  onMounted(() => {
    if (enabled()) {
      schedule();
      window.addEventListener('pagehide', pageHide);
    }
  });
  onUnmounted(() => {
    disposed = true;
    epoch++;
    clearTimeout(timer);
    window.removeEventListener('pagehide', pageHide);
  });
  return {
    opened,
    items,
    loading,
    saving,
    error,
    notice,
    more,
    acting,
    panel,
    list,
    save,
    newConversation,
    restore,
    clearCurrent,
    deleteConversation,
    clearAll
  };
}
