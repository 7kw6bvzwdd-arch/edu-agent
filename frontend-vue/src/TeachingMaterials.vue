<script setup>
import {ref,computed,onMounted,watch} from 'vue'
const props=defineProps({form:Object,api:Function,disabled:Boolean,tagging:Boolean})
const materials=ref([]),references=ref([]),error=ref(''),busy=ref(false),preview=ref(null),tag=ref({subject:'数学',grade:'八年级',edition:'人教版'}),tagID=ref('')
const selected=computed(()=>materials.value.find(m=>m.id===props.form.material_id))
const matching=computed(()=>materials.value.filter(m=>!m.curriculum?.subject||(m.curriculum.subject===props.form.subject&&m.curriculum.grade===props.form.grade&&(m.curriculum.edition===props.form.textbook||m.curriculum.edition.startsWith(props.form.textbook+' ')))))
const reference=computed(()=>references.value.find(m=>m.id===props.form.reference_id))
async function run(fn){busy.value=true;error.value='';try{await fn()}catch(e){error.value=e.message}finally{busy.value=false}}
async function load(){const [m,r]=await Promise.all([props.api('/courseware/materials'),props.api('/courseware/references')]);materials.value=m;references.value=r}
function selectMaterial(){preview.value=null;props.form.material_start=1;props.form.material_end=Math.min(3,selected.value?.pages||1);props.form.chapter='';if(selected.value)props.form.use_kb=true}
function chooseChapter(event){const c=selected.value?.chapters?.[Number(event.target.value)];if(!c)return;props.form.chapter=c.title;props.form.material_start=c.start;props.form.material_end=Math.min(c.end,c.start+19);preview.value=null}
async function showPreview(){await run(async()=>preview.value=await props.api('/courseware/materials/preview','POST',{subject:props.form.subject,grade:props.form.grade,textbook:props.form.textbook,material_id:props.form.material_id,material_start:props.form.material_start,material_end:props.form.material_end}))}
async function uploadReference(event){const file=event.target.files?.[0];event.target.value='';if(!file)return;await run(async()=>{if(file.size>20*1024*1024)throw Error('请选择20MB以内的PDF/PPTX');const body=new FormData();body.append('file',file);const value=await props.api('/courseware/references','POST',body);await load();props.form.reference_id=value.id;props.form.reference_mode='style';if(value.profile?.theme)props.form.theme=value.profile.theme})}
async function removeReference(){if(!reference.value||!window.confirm('删除这份参考分析？已有作品保存的参考内容会保留。'))return;await run(async()=>{await props.api('/courseware/references/'+reference.value.id,'DELETE');props.form.reference_id='';await load()})}
async function saveTag(){await run(async()=>{await props.api('/documents/'+tagID.value+'/curriculum','PATCH',tag.value);await load()})}
watch(()=>[props.form.subject,props.form.grade,props.form.textbook],()=>{if(props.form.material_id&&!matching.value.some(m=>m.id===props.form.material_id)){props.form.material_id='';props.form.chapter='';preview.value=null}})
onMounted(()=>run(load))
</script>
<template>
 <fieldset class="teaching-materials" :disabled="disabled||busy">
  <legend>教材章节与参考课件</legend>
  <p v-if="error" class="alert error" role="alert">{{error}}</p>
  <label>教材原文<select v-model="form.material_id" @change="selectMaterial"><option value="">不指定教材文件</option><option v-for="m in matching" :value="m.id" :key="m.id">{{m.name}}{{m.curriculum?.subject?'':'（未标注版本）'}}</option></select></label>
  <p class="field-help">从“知识与来源”导入教材。章节由原文标题识别，可手动核对页码；版本标签由教师填写。</p>
  <template v-if="selected">
   <label v-if="selected.chapters?.length">选择原文章节<select @change="chooseChapter"><option value="">手动指定页码</option><option v-for="(c,i) in selected.chapters" :key="i" :value="i">{{c.title}} · 第{{c.start}}页</option></select></label>
   <label>章节名称<input v-model="form.chapter" maxlength="100" placeholder="如 第十九章 一次函数"></label>
   <div class="page-range"><label>起始页<input type="number" v-model.number="form.material_start" min="1" :max="selected.pages"></label><label>结束页<input type="number" v-model.number="form.material_end" :min="form.material_start" :max="selected.pages"></label></div>
   <button type="button" @click="showPreview">查看匹配原文</button>
   <details v-if="preview?.pages" open><summary>实际采用的教材原文</summary><article v-for="p in preview.pages" :key="p.page"><b>文件第{{p.page}}页</b><p>{{p.text}}</p></article></details>
  </template>
  <label>参考课件<select v-model="form.reference_id"><option value="">不使用参考课件</option><option v-for="r in references" :key="r.id" :value="r.id">{{r.name}}</option></select></label>
  <label class="reference-upload">{{busy?'处理中…':'上传参考PDF / PPTX'}}<input type="file" accept=".pdf,.pptx" @change="uploadReference"></label>
  <p class="field-help">最多80页、20MB。抽样分析实际画面，保留分析文字，原文件与画面处理后释放。会调用视觉模型，不预留业务积分。</p>
  <details v-if="reference" open><summary>已观察的参考风格</summary><p>共{{reference.profile.total_pages}}页 · 分析页码：{{reference.profile.observed_pages?.join('、')}}</p><p v-if="reference.profile.text_truncated">原文超出容量，仅保留前部文字；画面仍按所列页码抽样。</p><p>{{reference.profile.layout}}</p><p>{{reference.profile.typography}}</p><p>{{reference.profile.rhythm}}</p><p>{{reference.profile.diagram_style}}</p><button type="button" @click="removeReference">删除此参考</button></details>
  <details v-if="tagging"><summary>标注资料的教材版本</summary><label>资料<select v-model="tagID"><option value="">选择已就绪的资料</option><option v-for="m in materials" :key="m.id" :value="m.id">{{m.name}}</option></select></label><label>学科<select v-model="tag.subject"><option>数学</option><option>物理</option></select></label><label>年级<input v-model="tag.grade" maxlength="30"></label><label>教材版本<input v-model="tag.edition" maxlength="70" placeholder="包含册次与出版年份，便于准确匹配"></label><button type="button" @click="saveTag" :disabled="!tagID">保存版本标注</button></details>
 </fieldset>
</template>
<style scoped>
.teaching-materials{margin-top:20px}.teaching-materials p{font-size:12px;line-height:1.7}.page-range{display:flex;gap:12px}.page-range label{flex:1;min-width:0}.teaching-materials details{margin:12px 0}.teaching-materials summary{cursor:pointer;font-size:13px}.teaching-materials article{max-height:220px;overflow:auto;border-bottom:1px solid var(--line);padding:12px 0}.teaching-materials article p{white-space:pre-wrap}.reference-upload input{font-size:12px}
</style>
