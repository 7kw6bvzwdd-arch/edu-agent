<script setup>
import TeachingClaimSources from './TeachingClaimSources.vue';
const props = defineProps({
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
function toggleGoal(id, check, enabled) {
  const page = props.local?.plan?.pages?.[props.currentPosition - 1];
  if (!page || !props.editable) return;
  page.goal_refs ||= [];
  page.checks_goals ||= [];
  const field = check ? 'checks_goals' : 'goal_refs';
  page[field] = page[field].filter((ref) => ref !== id);
  if (enabled) page[field].push(id);
  if (check && enabled && !page.goal_refs.includes(id)) page.goal_refs.push(id);
  if (!check && !enabled) page.checks_goals = page.checks_goals.filter((ref) => ref !== id);
}
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
      <details v-if="local?.plan" class="studio-lesson">
        <summary>学习目标与全课跨页核验</summary>
        <p v-if="!local.plan.learning_goals?.length">
          历史课件尚未建立结构化目标映射，不能据此认定核心目标已达标。
        </p>
        <article v-for="goal in local.plan.learning_goals || []" :key="goal.id">
          <b>{{ goal.core ? '核心目标' : '拓展目标' }} {{ goal.id }} · {{ goal.text }}</b>
          <p>
            讲解页：{{
              local.plan.pages
                .filter((p) => p.goal_refs?.includes(goal.id))
                .map((p) => p.position)
                .join('、') || '未覆盖'
            }}；考查页：{{
              local.plan.pages
                .filter((p) => p.checks_goals?.includes(goal.id))
                .map((p) => p.position)
                .join('、') || '未安排'
            }}
          </p>
        </article>
        <p>目标映射只检查引用与任务安排，不代表学生已经掌握。</p>
        <label
          v-if="
            current &&
            editable &&
            local.plan.learning_goals?.length &&
            local.plan.pages[currentPosition - 1]
          "
        >
          必要前置页（用逗号分隔，只能填写早于本页的页号；删除前置页前请先调整此处）
          <input
            :value="(local.plan.pages[currentPosition - 1].depends_on || []).join(',')"
            @change="
              local.plan.pages[currentPosition - 1].depends_on = $event.target.value.trim()
                ? $event.target.value
                    .split(/[,，、\s]+/)
                    .filter(Boolean)
                    .map(Number)
                : []
            "
          />
        </label>
        <template
          v-if="
            current &&
            editable &&
            local.plan.learning_goals?.length &&
            local.plan.pages[currentPosition - 1]
          "
        >
          <p>本页目标关系（保存后重新核验）：</p>
          <div v-for="goal in local.plan.learning_goals" :key="goal.id">
            <label
              ><input
                type="checkbox"
                :checked="local.plan.pages[currentPosition - 1].goal_refs?.includes(goal.id)"
                @change="toggleGoal(goal.id, false, $event.target.checked)"
              />讲解 {{ goal.id }}</label
            >
            <label
              ><input
                type="checkbox"
                :checked="local.plan.pages[currentPosition - 1].checks_goals?.includes(goal.id)"
                @change="toggleGoal(goal.id, true, $event.target.checked)"
              />用具体任务考查 {{ goal.id }}</label
            >
          </div>
        </template>
        <p v-if="task.result?.deck_check" role="status">
          {{
            {
              checked: '全课模型核验完成',
              blocked: '全课核验未通过',
              unavailable: '全课核验未完成',
              sample: '固定样例程序检查',
              disabled: '内容核验已关闭',
              unmapped: '尚无目标映射'
            }[task.result.deck_check.status] || '待核验'
          }}：{{ task.result.deck_check.summary }}
        </p>
        <p v-else>全课生成完成后检查；内容修改后须保存并重新核验。</p>
        <article v-for="(issue, index) in task.result?.deck_check?.issues || []" :key="index">
          <b
            >{{
              issue.severity === 'missing_evidence'
                ? '缺少依据'
                : issue.severity === 'error'
                  ? '需修复'
                  : '建议'
            }}
            · 第{{ issue.pages.join('、') }}页 · {{ issue.object }}</b
          >
          <p>{{ issue.evidence }}</p>
          <p>{{ issue.repair }}</p>
        </article>
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
