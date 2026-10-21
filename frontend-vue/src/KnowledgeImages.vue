<script setup>
import { inject, onUnmounted, ref, watch } from 'vue';
import MediaImage from './MediaImage.vue';
const props = defineProps({ documentId: String });
const token = inject('mediaToken', ref(''));
const images = ref([]),
  warnings = ref([]),
  busy = ref(false),
  error = ref('');
let request;
watch(
  () => [props.documentId, token.value],
  () => {
    request?.abort();
    images.value = [];
    warnings.value = [];
    error.value = '';
    busy.value = false;
  }
);
onUnmounted(() => request?.abort());
async function load() {
  request?.abort();
  const current = new AbortController();
  request = current;
  busy.value = true;
  error.value = '';
  try {
    const response = await fetch(
      '/api/documents/' + encodeURIComponent(props.documentId) + '/images',
      { headers: { Authorization: 'Bearer ' + token.value }, signal: current.signal }
    );
    const result = await response.json();
    if (!response.ok) throw Error(result.message || '资料图片读取失败');
    if (current !== request || current.signal.aborted) return;
    images.value = result.images || [];
    warnings.value = result.warnings || [];
  } catch (e) {
    if (!current.signal.aborted) error.value = e.message;
  } finally {
    if (current === request) busy.value = false;
  }
}
</script>
<template>
  <details class="knowledge-images" @toggle="$event.target.open && load()">
    <summary>查看资料配图与页码</summary>
    <p v-if="busy" role="status">正在读取资料配图…</p>
    <p v-if="error" role="alert">{{ error }}</p>
    <p v-for="warning in warnings" class="fine">{{ warning }}</p>
    <p v-if="!busy && !error && !images.length" class="fine">
      未提取到可复用图片；旧资料可点击重新解析。
    </p>
    <figure v-for="image in images" :key="image.asset_id + ':' + image.page">
      <MediaImage :picture="{ asset_id: image.asset_id }" :alt="'第' + image.page + '页资料配图'" />
      <figcaption>第{{ image.page }}页 · {{ image.caption || '原文图片，无文字说明' }}</figcaption>
    </figure>
    <p class="fine">
      图片保留原文出处；提图不代表已核实事实或获得许可。制作课件时仍按页面用途筛选。
    </p>
  </details>
</template>
<style scoped>
.knowledge-images {
  margin-top: 10px;
}
.knowledge-images summary {
  cursor: pointer;
  font-size: 12px;
}
.knowledge-images figure {
  margin: 12px 0;
}
.knowledge-images :deep(img) {
  max-width: 100%;
  max-height: 200px;
  object-fit: contain;
}
.knowledge-images figcaption {
  font-size: 12px;
  overflow-wrap: anywhere;
}
</style>
