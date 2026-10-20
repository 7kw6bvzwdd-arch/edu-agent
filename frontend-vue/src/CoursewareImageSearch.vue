<script setup>
import { computed, inject, onUnmounted, ref, watch } from 'vue';
import MediaImage from './MediaImage.vue';
const props = defineProps({
  slide: { type: Object, required: true },
  taskId: String,
  revision: Number,
  editable: Boolean,
  useWeb: Boolean
});
const token = inject('mediaToken', ref(''));
const allowWeb = ref(false);
const purpose = ref(''),
  queries = ref(''),
  constraints = ref(''),
  busy = ref(false),
  message = ref(''),
  error = ref(''),
  candidates = ref([]),
  rightsNotes = ref({}),
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
    candidates.value =
      props.slide.picture || props.slide.image_search?.status === 'removed'
        ? []
        : JSON.parse(JSON.stringify(props.slide.image_search?.candidates || []));
    rightsNotes.value = {};
    message.value = '';
    error.value = '';
    allowWeb.value = false;
    const need = props.slide.image_need;
    purpose.value = need?.purpose || '';
    queries.value = (need?.queries || []).join('\n');
    constraints.value = need?.constraints || '';
  },
  { immediate: true }
);
watch(
  () => props.slide.picture,
  () => {
    cancel();
    candidates.value = [];
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
  if (!props.useWeb && !allowWeb.value) {
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
  candidates.value = [];
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
    candidates.value = result.candidates || [];
    slide.image_need = need;
    slide.image_search = {
      status: result.status,
      message: result.message,
      candidates: JSON.parse(JSON.stringify(candidates.value))
    };
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
function choose(candidate) {
  if (
    !props.editable ||
    !compatible.value ||
    busy.value ||
    candidate.provenance.rights_status === 'pending_confirmation'
  )
    return;
  props.slide.picture = { ...candidate.picture };
  props.slide.image_search = {
    status: 'selected',
    message: '教师已选择网络配图，保存后需重新审核本页。'
  };
}
function skip() {
  if (!props.editable || busy.value) return;
  cancel();
  candidates.value = [];
  rightsNotes.value = {};
  props.slide.picture = null;
  props.slide.image_search = {
    status: 'removed',
    message: '已选择跳过本页配图，保存后生效；继续制作不会自动重新检索。'
  };
  message.value = '已跳过配图，请保存本页。';
  error.value = '';
}
async function confirmRights(candidate) {
  if (!props.editable || busy.value || !compatible.value) return;
  const note = (rightsNotes.value[candidate.picture.asset_id] || '').trim();
  if (note.length < 5) {
    error.value = '请填写图片使用权限依据，例如已获得作者许可及许可范围。';
    return;
  }
  cancel();
  const current = version,
    slide = props.slide,
    revision = props.revision;
  const request = new AbortController();
  controller = request;
  const timeout = setTimeout(() => request.abort('timeout'), 20000);
  busy.value = true;
  error.value = '';
  try {
    const response = await fetch(
      '/api/tasks/' + encodeURIComponent(props.taskId) + '/images/confirm',
      {
        method: 'POST',
        headers: { Authorization: 'Bearer ' + token.value, 'Content-Type': 'application/json' },
        body: JSON.stringify({
          revision,
          slide_id: slide.id,
          asset_id: candidate.picture.asset_id,
          rights_note: note
        }),
        signal: request.signal
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
    if (!response.ok) throw Error(result.message || '图片权限确认失败，请重试。');
    Object.assign(candidate, result);
    slide.image_search = {
      ...slide.image_search,
      candidates: JSON.parse(JSON.stringify(candidates.value))
    };
    message.value = '使用权限依据已记录，可选择图片并保存本页。';
  } catch (e) {
    if (current === version) {
      if (request.signal.reason === 'timeout') error.value = '确认超时，请重试；当前课件未改变。';
      else if (e.name !== 'AbortError') error.value = e.message;
    }
  } finally {
    clearTimeout(timeout);
    if (current === version) busy.value = false;
  }
}
</script>
<template>
  <section class="commons-image-search" aria-label="检索真实配图">
    <p v-if="slide.image_search" role="status">{{ slide.image_search.message }}</p>
    <details v-if="taskId && editable" :open="candidates.length > 0">
      <summary>检索真实配图 · 网页优先，Commons 备用</summary>
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
      <p v-if="!compatible" class="fine">当前页面已有冲突图形，请先调整视觉内容再检索配图。</p>
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
        :disabled="busy || !compatible || (!useWeb && !allowWeb)"
        @click="search"
      >
        {{ busy ? '正在检索并审核图片…' : '检索并审核候选图片' }}
      </button>
      <button v-if="busy" type="button" @click="cancel">停止等待</button>
      <button type="button" :disabled="busy" @click="skip">跳过本页配图</button>
      <p v-if="message" role="status">{{ message }}</p>
      <p v-if="error" role="alert" class="image-error">{{ error }}</p>
      <article
        v-for="candidate in candidates"
        :key="candidate.picture.asset_id"
        class="commons-candidate"
      >
        <MediaImage :picture="candidate.picture" :alt="candidate.picture.caption" />
        <b>{{ candidate.provenance.title }}</b>
        <p>{{ candidate.reason }}</p>
        <p>{{ candidate.provenance.author }} · {{ candidate.provenance.license }}</p>
        <a :href="candidate.provenance.page_url" target="_blank" rel="noopener noreferrer"
          >查看来源页</a
        >
        <div
          v-if="candidate.provenance.rights_status === 'pending_confirmation'"
          class="image-rights"
        >
          <p>
            该图使用权限尚不明确。请核对来源及许可，取得使用权限后填写依据；确认仅记录教师声明。
          </p>
          <label
            >使用权限依据<textarea
              v-model="rightsNotes[candidate.picture.asset_id]"
              :disabled="busy"
              maxlength="300"
              rows="2"
              placeholder="如：已取得作者许可，允许用于教学课件及下载分发"
            ></textarea>
          </label>
          <button
            type="button"
            :disabled="busy || !editable || !compatible"
            @click="confirmRights(candidate)"
          >
            记录使用权限依据
          </button>
        </div>
        <button
          type="button"
          :disabled="
            busy ||
            !editable ||
            !compatible ||
            candidate.provenance.rights_status === 'pending_confirmation'
          "
          @click="choose(candidate)"
        >
          使用此图
        </button>
      </article>
      <p class="fine">
        优先搜索网页图片，无可用图片时尝试
        Commons。自动配图仅采用明确声明开放许可且相关性审核通过的图片；许可不明的候选需教师确认使用权限。不扣制作积分，搜索及视觉服务调用仍产生供应商用量。选择后需保存本页。
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
      <a :href="provenance.page_url" target="_blank" rel="noopener noreferrer">来源页</a>
      <a
        v-if="provenance.license_url"
        :href="provenance.license_url"
        target="_blank"
        rel="noopener noreferrer"
        >许可说明</a
      >
      <p>抓取时间：{{ provenance.retrieved_at }}</p>
      <p class="commons-hash">SHA256：{{ provenance.sha256 }}</p>
      <small
        >元数据按抓取时记录，并随图片写入 PPT
        备注。相关性筛选不等于史实或授权认证，请教师核对。</small
      >
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
.commons-candidate {
  margin-top: 14px;
  padding: 12px;
  border: 1px solid #dedbe8;
  border-radius: 8px;
}
.commons-candidate img {
  width: 100%;
  max-height: 230px;
  object-fit: contain;
  display: block;
  margin-bottom: 8px;
}
.commons-hash {
  overflow-wrap: anywhere;
  font-size: 12px;
}
</style>
