<script setup>
import TeachingClaimSources from './TeachingClaimSources.vue';
defineProps({
  current: Object,
  currentPosition: Number,
  currentIndex: Number,
  local: Object,
  task: Object,
  editable: Boolean,
  deckNotice: String,
  meta: Function,
  changed: Function,
  qualityStatus: Function
});
</script>
<template>
  <details class="studio-reference-details">
    <summary>本页核对信息与资料来源</summary>
    <div class="studio-review-panel">
      <template v-if="current"
        ><div class="studio-inspector-heading">
          <small
            >教学位置 {{ currentPosition }} · 预览第 {{ currentIndex + 1 }} /
            {{ local.slides.length }} 页</small
          ><span
            :class="[
              'inspector-review-state',
              { approved: meta(current.id).approved && !changed(current) }
            ]"
            >{{
              changed(current) ? '有修改，待保存' : meta(current.id).approved ? '已确认' : '待审核'
            }}</span
          >
        </div>
        <h3 class="studio-inspector-title">{{ current.title }}</h3>
        <p class="inspector-review-help">
          请核对左侧同一页 PPT 的知识、题设、计算与答案，确认这页适合你的课堂。
        </p>
        <p v-if="qualityStatus(current)" class="inspector-quality-note" role="status">
          {{ qualityStatus(current) }}
        </p>
        <ul
          v-if="task.result?.page_checks?.[current.id]?.issues?.length"
          class="inspector-quality-issues"
        >
          <li v-for="(issue, index) in task.result.page_checks[current.id].issues" :key="index">
            <b>{{ issue.severity === 'error' ? '需修复' : '建议' }} · {{ issue.object }}</b>
            <p>{{ issue.evidence }}</p>
            <p>{{ issue.repair }}</p>
          </li>
        </ul>
        <details class="inspector-notes">
          <summary>教师讲解备注</summary>
          <p>{{ current.notes || '本页尚无讲解备注。' }}</p>
          <p v-if="current.answer"><b>课堂答案</b><br />{{ current.answer }}</p>
        </details>
        <TeachingClaimSources :slide="current" :sources="task.sources" :editable="editable"
      /></template>
      <details v-if="deckNotice" class="inspector-deck-notice">
        <summary>课件校验说明</summary>
        <p>{{ deckNotice }}</p>
      </details>
      <details v-if="local && task.result?.evaluation" class="studio-lesson">
        <summary>独立模型辅助评审 · 不替代教师逐页审核</summary>
        <p>
          {{
            task.result.evaluation.status === 'completed'
              ? '评审已完成，仍需教师核验'
              : task.result.evaluation.reason
          }}
        </p>
        <pre v-if="task.result.evaluation.assessment" class="assessment">{{
          JSON.stringify(task.result.evaluation.assessment, null, 2)
        }}</pre>
      </details>
      <details v-if="local" class="studio-lesson">
        <summary>教案与资料来源</summary>
        <label
          >教案<textarea v-model="local.lesson" rows="8" :readonly="!editable"></textarea>
        </label>
        <article v-for="s in task.sources" :key="s.id">
          <b>[{{ s.id }}] {{ s.title }}</b
          ><small>{{ s.location }} · {{ s.kind }}</small>
          <p v-if="s.textbook">
            {{ s.textbook.edition || '版次未标注' }} ·
            {{ s.textbook.chapter || '章节未标注' }}（教师标注；文件页序不等于印刷页码）
          </p>
          <p v-if="s.evidence_kind === 'catalogue'" class="alert">
            目录用于定位章节，不作为教学结论依据。
          </p>
          <p v-if="s.evidence_kind === 'unreliable_extraction'" class="alert">
            原文字形或公式提取异常，需要核对原文件画面；本页不能作为已验证依据。
          </p>
          <p>{{ s.content }}</p>
          <a
            v-if="s.url && /^https?:\/\//.test(s.url)"
            :href="s.url"
            target="_blank"
            rel="noopener noreferrer"
            >查看来源 ↗</a
          >
        </article>
        <p v-if="!task.sources?.length">未找到可引用资料，需教师核验；不编造出处。</p>
      </details>
      <p v-if="!current" class="inspector-review-help">页面生成后，可在这里逐页核对并确认。</p>
    </div>
  </details>
</template>
<style scoped>
.studio-lesson {
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
</style>
