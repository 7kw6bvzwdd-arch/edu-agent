<script setup>
import {useId} from 'vue'
import {coursewareSubjects} from './courseware-subjects.js'
import CoursewareBrief from './CoursewareBrief.vue'
import TeachingMaterials from './TeachingMaterials.vue'
import {templates} from './templates.js'
import WorkspaceIcon from './WorkspaceIcon.vue'
defineProps({form:Object,api:Function,health:Object,disabled:Boolean,ready:Boolean,tagging:Boolean,requirements:String,generator:Boolean,workspace:Boolean,session:Object,token:String,available:Number,canCourseware:Boolean,canAnimation:Boolean})
defineEmits(['update:requirements','confirmed','changed'])
const subjectList=useId()
</script>
<template>
 <section :class="['courseware-setup','panel',{'courseware-generator':generator}]" :aria-label="generator?'课件制作设置':'首页课件需求确认'">
  <datalist :id="subjectList"><option v-for="subject in coursewareSubjects" :key="subject" :value="subject"/></datalist>
  <CoursewareBrief :workspace="workspace" :session="session" :token="token" :health="health" :available="available" :can-courseware="canCourseware" :can-animation="canAnimation" @changed="$emit('changed')" :generator="generator" :form="form" :api="api" :mode="health.mode" :quality-ready="health.mode!=='live'||health.courseware_quality_configured!==false" :disabled="disabled" :ready="ready" :requirements="requirements" @update:requirements="$emit('update:requirements',$event)" @confirmed="$emit('confirmed')">
   <template #appearance="{busy}"><label v-if="!workspace||canCourseware" class="courseware-appearance">课件配色<select v-model="form.theme" aria-label="课件配色" :disabled="disabled||busy"><option v-for="theme in templates" :key="theme.id" :value="theme.id">{{theme.name}}</option></select><small>仅调整PPT配色，随本次制作保存</small></label></template>
   <template #basics>
    <details class="generator-options"><summary>学科与教学安排<small>选填，也可以直接在需求中说明</small></summary><div class="generator-options-body">
     <div class="generator-row"><label>学习阶段<input v-model="form.grade" maxlength="40" placeholder="未指定，例如小学五年级或高一" aria-label="课件年级"></label><label>学科<input v-model="form.subject" :list="subjectList" maxlength="40" placeholder="选择或输入学科，可留空" aria-label="课件学科"></label></div>
     <div class="generator-row"><label>内容详略<select v-model="form.detail" aria-label="课件内容详略"><option value="auto">按教学需求安排</option><option value="concise">精简</option><option value="default">适中</option><option value="detailed">详细</option></select></label><label>课时（分钟）<input v-model.number="form.lesson_minutes" type="number" min="10" max="180" placeholder="未指定" aria-label="课件课时"></label></div>
    </div></details>
   </template>
   <template #materials>
    <details v-if="health.teaching_features_version==='teaching-v1'" class="generator-materials"><summary><WorkspaceIcon name="library"/>添加教材与参考课件<span>选填</span></summary><TeachingMaterials :form="form" :api="api" :disabled="disabled" :tagging="tagging" :page-images="health.courseware_material_images_version==='page-images-v1'"/></details>
   </template>
   <template #options>
    <details class="generator-options"><summary>更多教学设置<small>{{form.textbook}} · {{templates.find(t=>t.id===form.theme)?.name}}</small></summary>
     <div class="generator-options-body">
      <label>教材版本<input v-model="form.textbook" maxlength="100" placeholder="未指定，可填写实际教材与版次"></label>
      <label>学生基础<input v-model="form.student_level" maxlength="400" placeholder="例如：已学一次函数，需要加强图像理解"></label>
      <label>教学目标细化<textarea v-model="form.objectives" maxlength="1000" rows="2" placeholder="可补充具体的知识、能力与课堂目标"></textarea></label>
      <label>参考资料用途<select v-model="form.reference_mode"><option value="content">依据资料内容</option><option value="sequence">参考教学顺序</option><option value="style">参考视觉风格</option></select></label>
      <div class="brief-source-options"><label><input v-model="form.use_kb" type="checkbox">企业知识库</label><label><input v-model="form.use_web" type="checkbox">联网检索</label></div>
      <p class="fine">页数按教学内容安排，先生成三页供你审核。课件主题仅影响PPT配色。</p>
     </div>
    </details>
   </template>
   <template #parameters="{busy}">
    <details class="brief-parameters">
     <summary>教学资料与设置（选填） <small>{{form.grade||'学习阶段未指定'}} · {{form.subject||'学科未指定'}} · {{form.lesson_minutes?form.lesson_minutes+'分钟':'课时未指定'}}</small></summary>
     <fieldset :disabled="disabled||busy">
      <p class="fine">这些设置可以直接在对话中告诉助手。未提供的信息保持未指定，教材文件与参考课件都可以不选。</p>
      <label class="brief-topic">课题名称<input v-model="form.topic" maxlength="100" aria-label="课题名称" placeholder="也可以直接在对话中说课题"></label>
      <div class="field-row"><label>学科<input v-model="form.subject" :list="subjectList" maxlength="40" placeholder="选择或输入学科，可留空" aria-label="教学设置学科"></label><label>年级<input v-model="form.grade" maxlength="40" placeholder="未指定，可填写实际学习阶段"></label></div>
      <div class="field-row"><label>教材版本<input v-model="form.textbook" maxlength="100" placeholder="未指定，可填写实际教材与版次"></label><label>内容详略<select v-model="form.detail"><option value="auto">按教学需求安排</option><option value="concise">精简</option><option value="default">适中</option><option value="detailed">详细</option></select></label></div>
      <div class="field-row"><label>课时（分钟）<input v-model.number="form.lesson_minutes" type="number" min="10" max="180"></label><label>参考资料用途<select v-model="form.reference_mode"><option value="content">依据资料内容</option><option value="sequence">参考教学顺序</option><option value="style">参考视觉风格</option></select></label></div>
      <label>学生基础<input v-model="form.student_level" maxlength="400" placeholder="例如：已学正比例函数，基础偏弱"></label>
      <label>教学目标<textarea v-model="form.objectives" maxlength="1000" rows="2"></textarea></label>
      <div class="brief-source-options"><label><input v-model="form.use_kb" type="checkbox">企业知识库</label><label><input v-model="form.use_web" type="checkbox">联网检索</label></div>
      <TeachingMaterials v-if="health.teaching_features_version==='teaching-v1'&&(!workspace||canCourseware)" :form="form" :api="api" :disabled="disabled||busy" :tagging="tagging" :page-images="health.courseware_material_images_version==='page-images-v1'"/>
     </fieldset>
    </details>
   </template>
  </CoursewareBrief>
 </section>
</template>
<style scoped>
.courseware-appearance{display:flex;align-items:center;flex-wrap:wrap;gap:8px;font-size:12px;margin:0 0 8px}.courseware-appearance select{width:auto;min-width:130px;margin:0;padding:5px 9px;min-height:32px}.courseware-appearance small{font-size:10px;color:var(--muted)}
.brief-topic{padding:18px 22px 0}.courseware-setup{text-align:left;max-width:1100px;margin:24px auto;overflow:hidden}.brief-parameters{margin:0 22px 18px;border-top:1px solid var(--line);padding-top:16px}.brief-parameters summary{cursor:pointer;font-size:13px;font-weight:600}.brief-parameters summary small{display:block;color:var(--muted);font-weight:400;margin-top:6px}.brief-parameters fieldset{border:0;padding:18px 0 0;margin:0}.brief-source-options{display:flex;gap:20px;flex-wrap:wrap}.brief-source-options label{display:flex;align-items:center;gap:6px}.brief-source-options input{width:auto;margin:0}
</style>
