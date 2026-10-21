<script setup>
import { computed, inject, onUnmounted, ref, watch } from 'vue';
const props = defineProps({
  slide: { type: Object, required: true },
  taskId: String,
  revision: Number,
  editable: Boolean,
  useWeb: Boolean,
  useKB: Boolean
});
const token = inject('mediaToken', ref(''));
const allowWeb = ref(false);
const purpose = ref(''),
  queries = ref(''),
  constraints = ref(''),
  busy = ref(false),
  message = ref(''),
  error = ref(''),
  provenance = ref(null);
let version = 0,
  controller;
const compatible = computed(
  () =>
    !props.slide.scientific &&
    !props.slide.graph &&
    !props.slide.chart &&
    !props.slide.diagram &&
    !(props.slide.table && props.slide.figure)
);
function cancel() {
  version++;
  controller?.abort();
  busy.value = false;
}
watch(
  () => [props.slide.id, props.taskId, props.revision, token.value],
  () => {
    cancel();
    message.value = '';
    error.value = '';
    allowWeb.value = false;
    const need = props.slide.image_need;
    purpose.value = need?.purpose || props.slide.title || '';
    queries.value = (need?.queries || [props.slide.title]).filter(Boolean).join('\n');
    constraints.value = need?.constraints || '';
  },
  { immediate: true }
);
watch(
  () => props.slide.picture,
  () => {
    cancel();
    message.value = '';
  },
  { deep: true }
);
watch(
  () => [props.slide.picture?.asset_id, token.value],
  async ([asset, key], _, cleanup) => {
    provenance.value = null;
    if (!asset || !key) return;
    const request = new AbortController();
    cleanup(() => request.abort());
    try {
      const response = await fetch('/api/media/' + encodeURIComponent(asset) + '/metadata', {
        headers: { Authorization: 'Bearer ' + key },
        signal: request.signal
      });
      if (!response.ok) return;
      const result = await response.json();
      if (!request.signal.aborted) provenance.value = result.provenance || null;
    } catch {
      /* A failed attribution read must never invent source details. */
    }
  },
  { immediate: true }
);
onUnmounted(cancel);
async function search() {
  if (!props.editable || busy.value || !props.taskId || !compatible.value) return;
  if (!props.useWeb && !props.useKB && !allowWeb.value) {
    error.value = '请先同意本次联网检索，或上传图片、跳过配图。';
    return;
  }
  cancel();
  const current = version,
    slide = props.slide,
    revision = props.revision;
  const need = {
    purpose: purpose.value.trim(),
    queries: queries.value
      .split('\n')
      .map((q) => q.trim())
      .filter(Boolean),
    constraints: constraints.value.trim()
  };
  message.value = '';
  error.value = '';
  if (!need.purpose || !need.queries.length || need.queries.length > 2) {
    error.value = '请填写教学用途和1至2组检索词，每行一组。';
    return;
  }
  controller = new AbortController();
  const request = controller;
  const timeout = setTimeout(() => request.abort('timeout'), 95000);
  busy.value = true;
  try {
    const response = await fetch(
      '/api/tasks/' + encodeURIComponent(props.taskId) + '/images/search',
      {
        method: 'POST',
        headers: { Authorization: 'Bearer ' + token.value, 'Content-Type': 'application/json' },
        body: JSON.stringify({ revision, slide_id: slide.id, need, allow_web: allowWeb.value }),
        signal: controller.signal
      }
    );
    const result = await response.json();
    if (
      current !== version ||
      slide !== props.slide ||
      revision !== props.revision ||
      !props.editable
    )
      return;
    if (!response.ok) throw Error(result.message || '图片检索失败，请稍后重试。');
    message.value = result.message;
    const selected = result.status === 'found' ? result.candidates?.[0] : null;
    slide.image_need = need;
    slide.image_search = {
      status: result.status,
      message: result.message,
      candidates: []
    };
    if (selected) {
      slide.picture = { ...selected.picture };
      slide.image_search = {
        status: 'selected',
        message: '已自动配图，保存后生效；可替换或移除。'
      };
    }
  } catch (e) {
    if (current === version) {
      if (request.signal.reason === 'timeout')
        error.value = '图片检索超时，课件内容已保留，可修改检索词后重试。';
      else if (e.name !== 'AbortError') error.value = e.message;
    }
  } finally {
    clearTimeout(timeout);
    if (current === version) busy.value = false;
  }
}
function skip() {
  if (!props.editable || busy.value) return;
  cancel();
  props.slide.picture = null;
  props.slide.image_search = {
    status: 'removed',
    message: '已选择跳过本页配图，保存后生效；继续制作不会自动重新检索。'
  };
  message.value = '已跳过配图，请保存本页。';
  error.value = '';
}
</script>
<template>
  <section class="commons-image-search" aria-label="检索真实配图">
    <p v-if="slide.image_search" role="status">{{ slide.image_search.message }}</p>
    <details v-if="taskId && editable">
      <summary>更换或补充配图</summary>
      <details class="image-search-options">
        <summary>调整配图要求（可选）</summary>
        <label
          >配图教学用途<input
            v-model="purpose"
            maxlength="60"
            :disabled="busy"
            placeholder="如：观察长城的城墙与敌台"
        /></label>
        <label
          >检索词（每行一组，最多两组）<textarea
            v-model="queries"
            rows="2"
            :disabled="busy"
            placeholder="长城 八达岭 建筑照片"
          ></textarea>
        </label>
        <label
          >人物、地点、年代或视角要求<textarea
            v-model="constraints"
            maxlength="300"
            rows="2"
            :disabled="busy"
            placeholder="如：实际建筑照片，不采用现代复原插画"
          ></textarea>
        </label>
      </details>
      <p v-if="!compatible" class="fine">当前页面已有冲突图形，请先调整视觉内容再检索配图。</p>
      <p v-if="useKB && !useWeb" class="fine">
        当前可直接匹配企业资料中的图片；无需启用公开网页检索。图片仍会交给已配置的视觉模型筛选。
      </p>
      <label v-if="!useWeb"
        ><input
          v-model="allowWeb"
          type="checkbox"
          :disabled="busy"
        />本课件已关闭联网；同意仅本次配图检索连接公开网页及备用
        Commons，并将候选图片交给视觉模型审核。</label
      >
      <p class="fine">
        仅发送填写的检索词，不发送整课私有资料；每次检索各来源合计最多审核3张候选图。上传和跳过不联网。
      </p>
      <button
        type="button"
        :disabled="busy || !compatible || (!useWeb && !useKB && !allowWeb)"
        @click="search"
      >
        {{ busy ? '正在自动匹配图片…' : slide.picture ? '重新自动配图' : '自动配图' }}
      </button>
      <button v-if="busy" type="button" @click="cancel">停止等待</button>
      <button type="button" :disabled="busy" @click="skip">跳过本页配图</button>
      <p v-if="message" role="status">{{ message }}</p>
      <p v-if="error" role="alert" class="image-error">{{ error }}</p>
      <p class="fine">
        启用知识库时优先匹配资料图片；允许联网后才搜索网页，无可用图片时尝试
        Commons。系统根据教学用途筛选并自动插入；来源未标明许可时如实记录，不代表已获授权。不扣制作积分，搜索及视觉服务调用仍产生供应商用量。修改后保存本页。
      </p>
    </details>
    <details v-if="provenance" class="commons-attribution">
      <summary>配图来源与许可 · {{ provenance.license }}</summary>
      <p>{{ provenance.title }}</p>
      <p>作者：{{ provenance.author }}</p>
      <p>{{ provenance.description }}</p>
      <p v-if="provenance.date">来源日期：{{ provenance.date }}</p>
      <p v-if="provenance.rights_status === 'teacher_confirmed'">
        教师使用权限声明：{{ provenance.rights_note }}（{{ provenance.rights_confirmed_at }}）
      </p>
      <p v-if="provenance.provider === 'knowledge_document'">
        企业资料 · 第{{ provenance.page }}页 · 文档编号 {{ provenance.document_id }}
      </p>
      <a v-else :href="provenance.page_url" target="_blank" rel="noopener noreferrer">来源页</a>
      <a
        v-if="provenance.license_url"
        :href="provenance.license_url"
        target="_blank"
        rel="noopener noreferrer"
        >许可说明</a
      >
      <p>抓取时间：{{ provenance.retrieved_at }}</p>
      <p class="commons-hash">SHA256：{{ provenance.sha256 }}</p>
      <small>元数据按抓取时记录，并随图片写入 PPT 备注。相关性筛选不等于史实或授权认证。</small>
    </details>
  </section>
</template>
<style scoped>
.commons-image-search {
  margin-block: 12px;
}
.commons-image-search details {
  border: 1px solid #dedbe8;
  border-radius: 8px;
  padding: 12px;
  margin-block: 8px;
}
.commons-image-search label {
  display: grid;
  gap: 6px;
  margin-block: 12px;
}
.commons-image-search a,
.commons-image-search button {
  margin-right: 12px;
}
.commons-hash {
  overflow-wrap: anywhere;
  font-size: 12px;
}
</style>
