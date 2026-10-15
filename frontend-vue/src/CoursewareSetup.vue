<script setup>
import CoursewareBrief from './CoursewareBrief.vue'
import TeachingMaterials from './TeachingMaterials.vue'
import {templates} from './templates.js'
defineProps({form:Object,api:Function,health:Object,disabled:Boolean,ready:Boolean,tagging:Boolean,requirements:String})
defineEmits(['update:requirements','confirmed'])
</script>
<template>
 <section class="courseware-setup panel" aria-label="首页课件需求确认">
  <CoursewareBrief :form="form" :api="api" :mode="health.mode" :quality-ready="health.mode!=='live'||health.courseware_quality_configured!==false" :disabled="disabled" :ready="ready" :requirements="requirements" @update:requirements="$emit('update:requirements',$event)" @confirmed="$emit('confirmed')">
   <template #parameters="{busy}">
    <details class="brief-parameters">
     <summary>教学资料与设置（选填） <small>{{form.grade}} · {{form.subject}} · {{form.lesson_minutes}}分钟</small></summary>
     <fieldset :disabled="disabled||busy">
      <p class="fine">这些设置可以直接在对话中告诉助手。没有特别要求时沿用当前值，教材文件与参考课件都可以不选。</p>
      <label class="brief-topic">课题名称<input v-model="form.topic" maxlength="100" aria-label="课题名称" placeholder="也可以直接在对话中说课题"></label>
      <div class="field-row"><label>学科<select v-model="form.subject"><option>数学</option><option>物理</option></select></label><label>年级<select v-model="form.grade"><option>七年级</option><option>八年级</option><option>九年级</option></select></label></div>
      <div class="field-row"><label>教材版本<select v-model="form.textbook"><option>人教版</option></select></label><label>内容详略<select v-model="form.detail"><option value="concise">精简</option><option value="default">默认</option><option value="detailed">详细</option></select></label></div>
      <div class="field-row"><label>课时（分钟）<input v-model.number="form.lesson_minutes" type="number" min="10" max="180"></label><label>参考资料用途<select v-model="form.reference_mode"><option value="content">依据资料内容</option><option value="sequence">参考教学顺序</option><option value="style">参考视觉风格</option></select></label></div>
      <label>学生基础<input v-model="form.student_level" maxlength="400" placeholder="例如：已学正比例函数，基础偏弱"></label>
      <label>教学目标<textarea v-model="form.objectives" maxlength="1000" rows="2"></textarea></label>
      <div class="brief-source-options"><label><input v-model="form.use_kb" type="checkbox">企业知识库</label><label><input v-model="form.use_web" type="checkbox">联网检索</label></div>
      <TeachingMaterials v-if="health.teaching_features_version==='teaching-v1'" :form="form" :api="api" :disabled="disabled||busy" :tagging="tagging" :page-images="health.courseware_material_images_version==='page-images-v1'"/>
      <label>课件视觉主题<select v-model="form.theme"><option v-for="theme in templates" :key="theme.id" :value="theme.id">{{theme.name}}</option></select></label>
     </fieldset>
    </details>
   </template>
  </CoursewareBrief>
 </section>
</template>
<style scoped>
.brief-topic{padding:18px 22px 0}.courseware-setup{text-align:left;max-width:1100px;margin:24px auto;overflow:hidden}.brief-parameters{margin:0 22px 18px;border-top:1px solid var(--line);padding-top:16px}.brief-parameters summary{cursor:pointer;font-size:13px;font-weight:600}.brief-parameters summary small{display:block;color:var(--muted);font-weight:400;margin-top:6px}.brief-parameters fieldset{border:0;padding:18px 0 0;margin:0}.brief-source-options{display:flex;gap:20px;flex-wrap:wrap}.brief-source-options label{display:flex;align-items:center;gap:6px}.brief-source-options input{width:auto;margin:0}
</style>
