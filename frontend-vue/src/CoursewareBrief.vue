<script setup>
import {ref,toRef,computed,watch,nextTick,onUnmounted,useId} from 'vue'
import {coursewareSubjects} from './courseware-subjects.js'
import WorkspaceIcon from './WorkspaceIcon.vue'
import HomeToolResult from './HomeToolResult.vue'
const props=defineProps({form:Object,api:Function,mode:String,disabled:Boolean,ready:Boolean,qualityReady:{type:Boolean,default:true},requirements:String,generator:Boolean,workspace:Boolean,session:Object,token:String,health:Object,available:Number,canCourseware:Boolean,canAnimation:Boolean})
const emit=defineEmits(['update:requirements','confirmed','changed'])
const subjectList=useId()
const text=props.session?toRef(props.session,'text'):ref(''),messages=props.session?toRef(props.session,'messages'):ref([]),summary=props.session?toRef(props.session,'summary'):ref('')
const busy=ref(false),error=ref(''),accepted=ref(false),chat=ref(null)
const additionalRequirements=ref('')
if(props.session&&props.session.understanding===undefined)props.session.understanding=''
const understanding=props.session?toRef(props.session,'understanding'):ref('')
if(props.session&&!props.session.contextMemory)props.session.contextMemory={text:'',through:0}
const contextMemory=props.session?toRef(props.session,'contextMemory'):ref({text:'',through:0})
const selectedTool=props.session?toRef(props.session,'selectedTool'):ref(''),suggestedTool=props.session?toRef(props.session,'suggestedTool'):ref('courseware')
const toolName=tool=>tool==='animation'?'教学动画':'课件制作'
const currentTool=computed(()=>selectedTool.value||suggestedTool.value)
let controller
onUnmounted(()=>controller?.abort())
// The server's display labels map only to the settings this conversation may edit.
const rowFields={'课题':'topic','学科':'subject','年级':'grade','课时（分钟）':'lesson_minutes','教材版本':'textbook','学生基础':'student_level','教学目标':'objectives','内容详略':'detail'}
const fieldLimits={topic:100,subject:40,grade:40,textbook:100,student_level:400,objectives:1000}
const rowOptions=computed(()=>({detail:[['auto','按教学需求安排'],['concise','精简'],['default','适中'],['detailed','详细']]}))
const editableMessage=computed(()=>{const index=messages.value.findLastIndex(m=>m.role==='assistant'&&!m.pending&&!m.failed&&(m.blocks?.length||m.requirements||m.questions?.length));return messages.value[index]?.execution?-1:index})
function fieldError(field){
 const value=props.form[field]
 if(field==='lesson_minutes'&&value!==''&&value!==0&&(!Number.isInteger(value)||value<10||value>180))return '课时请输入10–180之间的整数。'
 if(fieldLimits[field]&&[...String(value||'')].length>fieldLimits[field])return `此项最多${fieldLimits[field]}字。`
 if(rowOptions.value[field]&&!rowOptions.value[field].some(([key])=>key===value))return '请选择有效的教学设置。'
 return ''
}
function editRow(row,value){
 const field=rowFields[row.label]
 if(!field||busy.value||props.disabled)return
 props.form[field]=field==='lesson_minutes'?(value===''?'':Number(value)):value
 row.value=rowOptions.value[field]?.find(([key])=>key===value)?.[1]??String(value)
 accepted.value=false;error.value='';emit('update:requirements','')
}
function validateSettings(){
 error.value=Object.values(rowFields).map(fieldError).find(Boolean)||''
 return !error.value
}
const context=computed(()=>`课题：${props.form.topic}\n对象：${props.form.grade||'学习阶段未指定'} · ${props.form.subject||'学科未指定'} · ${props.form.textbook||'教材未指定'}\n内容详略：${({auto:'按教学需求安排',concise:'精简',default:'适中',detailed:'详细'})[props.form.detail]||'按教学需求安排'}，页数按教学需要安排\n课时：${props.form.lesson_minutes?props.form.lesson_minutes+'分钟':'未指定'}\n学生基础：${props.form.student_level||'未指定'}\n参考用途：${({content:'依据资料内容',sequence:'参考教学顺序',style:'参考视觉风格'})[props.form.reference_mode]||'依据资料内容'}\n教学目标：${props.form.objectives||'未指定，按实际需求沟通'}\n教材章节：${props.form.chapter||'未指定'}${props.form.material_id?` · 文件第${props.form.material_start}–${props.form.material_end}页`: ''}\n参考课件：${props.form.reference_id?'已选择画面分析记录':'未选择'}\n资料：${[props.form.use_kb?'企业知识库':'',props.form.use_web?'联网检索':''].filter(Boolean).join(' + ')}`)
watch(context,(value,previous)=>{
 if(!summary.value)summary.value=value+'\n教师补充：\n'
 else if(previous&&summary.value.startsWith(previous))summary.value=value+summary.value.slice(previous.length)
},{immediate:true})
const requirementsText=computed(()=>(summary.value.startsWith(context.value)?summary.value:context.value+'\n教师补充：\n'+summary.value)+(understanding.value.trim()?'\n已整理的教学要求（最新修正优先）：\n'+understanding.value.trim():'')+(props.generator&&additionalRequirements.value.trim()?'\n其他要求：'+additionalRequirements.value.trim():''))
watch(()=>props.disabled,value=>{if(!value)accepted.value=false})
watch(understanding,()=>{accepted.value=false;emit('update:requirements','')})
watch(summary,()=>{accepted.value=false;emit('update:requirements','')})
async function scrollChat(){await nextTick();if(!chat.value)return;const nearEnd=chat.value.scrollHeight-chat.value.scrollTop-chat.value.clientHeight<200;if(nearEnd||busy.value)chat.value.scrollTo({top:chat.value.scrollHeight,behavior:'instant'})}
async function send(intent='discuss'){
 if(busy.value||props.disabled||!text.value.trim())return false
 if(!validateSettings())return false
 const content=text.value.trim(),snapshot=JSON.stringify(props.form)
 const history=messages.value.filter(m=>!m.failed&&!m.pending).map(m=>({role:m.role,content:(m.content+(m.questions?.length?'\n待确认问题：'+m.questions.map(q=>q.question).join('；'):'')).slice(0,1600)}))
 const recent=[...history.slice(contextMemory.value.through),{role:'user',content}]
 const compactContext=props.mode==='live'&&(recent.length>=25||recent.reduce((size,m)=>size+[...m.content].length,0)>12000)
 const user={role:'user',content},assistant={role:'assistant',content:'',blocks:[],pending:true}
 busy.value=true;error.value='';controller=new AbortController()
 const append=async()=>{messages.value.push(user,assistant);await nextTick()}
 if(props.workspace&&!messages.value.length&&document.startViewTransition&&!matchMedia('(prefers-reduced-motion: reduce)').matches){await document.startViewTransition(append).updateCallbackDone}else await append()
 const current=messages.value.at(-1),currentUser=messages.value.at(-2)
 await scrollChat()
 try{
  if(controller.signal.aborted)throw new DOMException('Stopped','AbortError')
  const v=await props.api(props.workspace?'/workspace/brief':'/courseware/brief','POST',{stream:true,intent,compact_context:compactContext,context_summary:contextMemory.value.text,...(props.workspace?{tool:selectedTool.value}:{}),input:{...props.form,lesson_minutes:Number(props.form.lesson_minutes)||0,type:'courseware',requirements:requirementsText.value},messages:recent.slice(-31)},{signal:controller.signal,onEvent:(event,value)=>{
   if(event==='delta')current.content+=value.text||''
   if(event==='block')current.blocks.push(value)
   void scrollChat()
  }})
  if(controller.signal.aborted)throw new DOMException('Stopped','AbortError')
  if(snapshot!==JSON.stringify(props.form))throw Error('教学设置已改变，请重新发送；本次需求保留。')
  if(compactContext&&(!v.context_summary?.trim()||[...v.context_summary].length>3200))throw Error('对话摘要未完整返回，已保留本轮输入，请重试。')
  if(props.workspace){if(!['courseware','animation'].includes(v.tool)||!['discuss','generate'].includes(v.action))throw Error('工具选择未完成，请重新发送需求。');current.tool=v.tool;suggestedTool.value=v.tool}
  // Only the completed, validated reply can change teaching settings.
  const fields=['topic','subject','grade','textbook','lesson_minutes','student_level','objectives','detail']
  for(const key of fields)if(v.settings?.[key]!==undefined)props.form[key]=v.settings[key]
  current.content=v.reply;current.blocks=v.blocks||current.blocks;current.requirements=v.requirements||'';current.questions=v.questions||[];current.assumptions=v.assumptions||[];current.mode=v.mode;current.pending=false
  if(typeof v.requirements==='string')understanding.value=v.requirements.trim()
  if(compactContext)contextMemory.value={text:v.context_summary.trim(),through:Math.max(0,history.length-6)}
  await nextTick();if(!v.requirements?.trim())summary.value+='\n'+content;if(!props.generator)text.value='';if(props.workspace&&v.action==='generate'&&!current.questions.length&&intent==='discuss')launch(current,v.tool);await scrollChat();if(props.workspace&&intent==='generate'&&v.action!=='generate'){error.value='请先确认助手提出的必要信息，再开始制作。';return false}return true
 }catch(e){
  current.failed=true;current.pending=false;currentUser.failed=true
  error.value=e.name==='AbortError'?'已停止回复，你的需求已保留，可以继续修改或重新发送。':e.message
  current.error=error.value
  return false
 }finally{busy.value=false;controller=null}
}
async function confirm(){
 if(busy.value||props.disabled)return
 if(!validateSettings())return
 if(!props.workspace&&!props.ready){error.value='请先开通基础套餐并确保至少20可用积分，或检查资料来源与服务配置。';return}
 if(text.value.trim()&&!await send('generate'))return
 if(!props.form.topic.trim()){error.value='先告诉我课题，例如“制作九年级二次函数课件”。';await nextTick();document.querySelector('[aria-label="生成前需求输入"]')?.focus();return}
 await nextTick()
 if([...requirementsText.value].length>8000){error.value='需求摘要不能超过8000字，请精简后制作。';return}
 if(props.workspace){let message=messages.value.findLast(m=>m.role==='assistant'&&!m.failed);if(!message){messages.value.push({role:'assistant',content:'按当前需求开始制作。',blocks:[],tool:currentTool.value});message=messages.value.at(-1)}launch(message,currentTool.value);await scrollChat();return}
 accepted.value=true;emit('update:requirements',requirementsText.value.trim());emit('confirmed')
}
function launch(message,tool){
 if(message.execution){error.value='这条需求已开始制作，请在上方结果中继续；新作品请发送新的需求。';return}
 if(!props.form.topic.trim()){error.value='请先告诉我教学主题。';return}
 if(tool==='courseware'&&!props.ready){error.value='制作课件需开通基础套餐、至少20可用积分及可用资料来源。需求已保留。';return}
 if(tool==='animation'&&(!props.canAnimation||!props.health?.animation_configured)){error.value='动画制作权限或服务尚未就绪，需求已保留。';return}
 if([...requirementsText.value].length> (tool==='animation'?4000:8000)){error.value='需求过长，请精简后再制作。';return}
 const input=JSON.parse(JSON.stringify(props.form));delete input.pages
 Object.assign(input,{type:tool,lesson_minutes:Number(input.lesson_minutes)||0,requirements:requirementsText.value.trim(),source_task_id:''})
 if(tool==='animation')input.animation_engine='html';else input.preview_first=true
 message.execution={input,key:crypto.randomUUID(),taskId:''};error.value='';emit('changed')
}
function newResultWork(){selectedTool.value='';text.value='';nextTick(()=>document.querySelector('.brief-composer textarea')?.focus())}
function editUnderstanding(message,value){if(busy.value||props.disabled)return;message.requirements=value;understanding.value=value}
function answerQuestion(question,option){if(busy.value||props.disabled)return;const answer=`关于“${question}”，我选择：${option}`;text.value=text.value.trim()?text.value.trim()+'\n'+answer:answer;nextTick(()=>document.querySelector('.brief-composer textarea')?.focus())}
function animationFromResult({task,slide}){
 const input={type:'animation',animation_engine:'html',topic:task.input.topic,subject:task.input.subject,grade:task.input.grade,source_task_id:task.id,requirements:[slide.title,...(slide.bullets||[]),slide.notes||''].join('；').slice(0,4000)}
 if(!props.canAnimation||!props.health?.animation_configured){error.value='动画制作权限或服务尚未就绪。';return}
 messages.value.push({role:'assistant',content:'根据这页课件制作教学动画方案，确认方案前不预留积分。',blocks:[],tool:'animation',execution:{input,key:crypto.randomUUID(),taskId:''}});void scrollChat()
}

</script>
<template>
 <section :class="['brief-panel',{'has-messages':messages.length}]" aria-label="生成前需求对话">
  <datalist :id="subjectList"><option v-for="subject in coursewareSubjects" :key="subject" :value="subject"/></datalist>
  <template v-if="generator">
   <header class="generator-heading"><h1>课件制作</h1><p>把教学主题与想法写下来，先看三页课件预览，再继续完成整堂课。</p></header>
   <form class="generator-form" @submit.prevent="confirm">
    <fieldset class="generator-fields" :disabled="busy||disabled">

     <div class="generator-topic-group">
      <label>教学主题与目标 <span class="generator-required">必填</span><textarea v-model="text" required maxlength="1200" rows="4" aria-label="课件教学主题" placeholder="例如：九年级数学《二次函数的图像与性质》。从生活情境引入，讲清顶点与对称轴，再通过例题和练习巩固。"></textarea></label>
      <slot name="materials"/>
     </div>
     <label>其他要求 <span class="generator-optional">选填</span><textarea v-model="additionalRequirements" maxlength="1000" rows="3" aria-label="课件其他要求" placeholder="例如：至少安排三个课堂问题；推导分步展示，答案放在讲解备注中。"></textarea></label>
     <slot name="basics"/>
     <slot name="appearance" :busy="busy"/>
     <slot name="options"/>
    </fieldset>
    <p v-if="error" class="alert error" role="alert">{{error}}</p>
    <p v-if="!qualityReady" class="alert" role="status">真实画面审核尚未配置完成，请联系管理员准备好后制作。</p>
    <p v-else-if="!ready" class="fine" role="status">制作需开通基础套餐、至少20可用积分，并保留至少一种资料来源。</p>
    <button class="primary generator-submit" type="submit" :disabled="busy||disabled||!ready||!text.trim()" :aria-busy="busy||disabled"><WorkspaceIcon name="sparkle"/>{{busy?'正在整理教学需求…':disabled?'正在开始制作…':'生成三页预览'}}</button>
    <div class="generator-submit-note"><p>{{mode==='sample'?'本地示例模式 · 不调用真实模型':'先预留20积分，成功交付后结算'}}<span>逐页审核后继续制作，取消或失败释放预留积分。</span></p><button v-if="busy" type="button" @click="controller?.abort()">停止</button></div>
   </form>
  </template>
  <template v-else>
  <div class="brief-heading"><h2>说说你想做的课件</h2><p>先聊一聊，也可以按当前需求直接制作。</p></div>
  <div class="brief-grid">
   <div class="brief-conversation">
    <div class="brief-messages" ref="chat" role="log" aria-label="需求对话记录" aria-live="polite" :aria-busy="busy">
     <div v-if="!messages.length" class="brief-welcome"><p>从一个课题开始，任何学科都可以；教学资料与设置按需补充。</p></div>
     <article v-for="(m,i) in messages" :key="i" :class="['brief-bubble',m.role,{incomplete:m.failed}]">
      <b>{{m.role==='user'?'我的要求':workspace?'备课助手':'课件助手'}}<span v-if="m.role==='assistant'&&m.mode==='sample'"> · 本地示例</span></b>
      <p v-if="m.content">{{m.content}}<span v-if="m.pending" class="stream-cursor" aria-hidden="true">▍</span></p><p v-else-if="m.pending" class="brief-thinking" role="status">正在理解你的需求…</p>
      <section v-if="m.role==='assistant'&&!m.pending&&!m.failed&&m.requirements&&!m.execution" class="brief-understanding" aria-label="已理解的教学要求"><h3>已理解的教学要求</h3><textarea v-if="i===editableMessage" :value="m.requirements" aria-label="修改已理解的教学要求" maxlength="2400" rows="4" :disabled="busy||disabled" @input="editUnderstanding(m,$event.target.value)"></textarea><p v-else>{{m.requirements}}</p></section>
      <section v-if="m.role==='assistant'&&!m.pending&&!m.failed&&m.assumptions?.length&&!m.execution" class="brief-assumptions"><h3>暂定安排 · 可继续调整</h3><ul><li v-for="item in m.assumptions" :key="item">{{item}}</li></ul></section>
      <section v-if="m.role==='assistant'&&!m.pending&&!m.failed&&m.questions?.length&&!m.execution" class="brief-questions"><h3>还需要你确认<small>可选择、自由补充，也可直接制作</small></h3><div v-for="question in m.questions" :key="question.question" class="brief-question"><p>{{question.question}}</p><div class="brief-question-options"><button v-for="option in question.options" :key="option" type="button" :disabled="busy||disabled||i!==editableMessage" @click="answerQuestion(question.question,option)">{{option}}</button></div></div></section>
      <details v-if="m.execution&&m.blocks?.length" class="home-result-requirements"><summary>查看本次制作需求</summary><p v-for="block in m.blocks" :key="block.title"><strong>{{block.title}}</strong><br>{{block.rows.map(row=>row.label+'：'+row.value).join(' · ')}}</p></details>
      <section v-for="(block,j) in m.blocks" v-show="!m.execution" :key="j" class="brief-requirement-block" :aria-label="block.title">
       <h3>{{block.title}}<small v-if="i===editableMessage">点击内容即可修改</small></h3>
       <table><caption class="sr-only">{{block.title}}</caption><colgroup><col class="brief-row-label"><col></colgroup><tbody>
        <tr v-for="(row,k) in block.rows" :key="k"><th scope="row">{{row.label}}</th><td>
         <template v-if="i===editableMessage&&rowFields[row.label]">
          <select v-if="rowOptions[rowFields[row.label]]" :value="form[rowFields[row.label]]" :aria-label="'修改'+row.label" :disabled="busy||disabled" @change="editRow(row,$event.target.value)"><option v-for="[value,label] in rowOptions[rowFields[row.label]]" :key="value" :value="value">{{label}}</option></select>
          <textarea v-else-if="['student_level','objectives'].includes(rowFields[row.label])" :value="form[rowFields[row.label]]" :maxlength="fieldLimits[rowFields[row.label]]" :aria-label="'修改'+row.label" :disabled="busy||disabled" rows="2" placeholder="未指定，采用当前年级常规安排" @input="editRow(row,$event.target.value)"></textarea>
          <input v-else :type="rowFields[row.label]==='lesson_minutes'?'number':'text'" :list="rowFields[row.label]==='subject'?subjectList:undefined" :placeholder="rowFields[row.label]==='subject'?'选择或输入学科，例如历史':undefined" :value="form[rowFields[row.label]]" :maxlength="fieldLimits[rowFields[row.label]]" :min="rowFields[row.label]==='lesson_minutes'?10:undefined" :max="rowFields[row.label]==='lesson_minutes'?180:undefined" :step="rowFields[row.label]==='lesson_minutes'?1:undefined" :aria-label="'修改'+row.label" :aria-invalid="!!fieldError(rowFields[row.label])" :disabled="busy||disabled" @input="editRow(row,$event.target.value)">
          <small v-if="fieldError(rowFields[row.label])" class="brief-field-error">{{fieldError(rowFields[row.label])}}</small>
         </template><template v-else>{{row.value}}</template>
        </td></tr>
       </tbody></table>
      </section>
      <p v-if="m.failed&&m.role==='assistant'" class="fine">{{m.error||'本次回复未完成，尚未应用到制作需求。'}}</p>
      <div v-if="i===editableMessage&&!m.execution" class="brief-block-actions"><small>修改立即用于后续对话和制作，也可以继续补充。</small><button type="button" @click="confirm" :disabled="busy||disabled||(!workspace&&!ready)" class="primary">{{workspace?`制作${toolName(currentTool)}`:'按这个需求制作'}}</button></div>
      <HomeToolResult v-if="m.execution" :execution="m.execution" :api="api" :token="token" :health="health" :available="available" :can-animation="canAnimation" @changed="emit('changed')" @new-work="newResultWork" @animation="animationFromResult"/>
     </article>
    </div>
    <form @submit.prevent="send('discuss')" class="brief-composer"><div v-if="workspace" class="home-tool-switch" aria-label="选择对话工具"><button v-for="option in [{id:'',name:'自动选择'},{id:'courseware',name:'课件制作'},{id:'animation',name:'教学动画'}]" :key="option.id" type="button" :aria-pressed="selectedTool===option.id" :disabled="busy||disabled||(option.id==='courseware'&&!canCourseware)||(option.id==='animation'&&!canAnimation)" @click="selectedTool=option.id"><WorkspaceIcon v-if="option.id" :name="option.id==='animation'?'animation':'slides'"/>{{option.name}}</button></div><label>提出你的需求<textarea v-model="text" rows="3" maxlength="1200" :disabled="busy||disabled" :placeholder="workspace?'告诉我想做的课件或教学动画，例如：制作一次函数课件，再用动画演示斜率变化。':'告诉我想做哪一课，例如：九年级二次函数，45分钟。'" aria-label="生成前需求输入" @keydown.ctrl.enter.prevent="send('discuss')"></textarea></label><div class="brief-actions"><small>{{mode==='sample'?'本地需求引导，不调用模型':'沟通需求不扣制作积分'}}<br>{{workspace?'课件预留20积分；动画先出方案，确认后预留40积分。':'直接制作将预留20积分，先生成三页供你审核。'}}</small><div class="brief-action-buttons"><button v-if="busy" type="button" @click="controller?.abort()">停止回复</button><template v-else><button type="button" class="brief-direct" @click="confirm" :disabled="disabled||(!workspace&&!ready)||(!text.trim()&&!form.topic.trim())">{{accepted?'正在开始制作…':workspace?'开始制作':'直接制作三页预览'}}</button><button type="submit" :disabled="disabled||!text.trim()" class="primary">发送 ↑</button></template></div></div><p v-if="error" class="alert error" role="alert">{{error}}</p></form>
   </div>
   <div class="brief-summary"><slot name="appearance" :busy="busy"/><small v-if="contextMemory.text" class="brief-memory-note" role="status">较早对话已压缩，聊天记录保留，最新修改优先。</small><slot name="parameters" :busy="busy"/><details class="brief-summary-editor"><summary>查看或手动修改需求摘要</summary><label>可编辑摘要<textarea v-model="summary" rows="12" maxlength="8000" :disabled="busy||disabled" aria-label="课件需求摘要"></textarea></label></details><p v-if="!qualityReady" class="alert" role="status">真实画面审核尚未配置完成，请联系管理员准备好后制作。</p><p v-else-if="!ready&&!workspace" class="fine">制作需开通基础套餐、至少20可用积分，并保留至少一种资料来源。可以先继续聊需求。</p></div>
  </div>
  </template>
 </section>
</template>
<style scoped>
.brief-heading{padding:26px 28px 18px;border-bottom:1px solid var(--line)}.brief-heading h2{font-size:22px;margin:10px 0}.brief-heading p{color:var(--muted);font-size:12px}.brief-grid{display:flex;flex-direction:column;min-height:470px}.brief-conversation{display:flex;flex-direction:column;min-width:0;border-right:0}.brief-messages{padding:22px;max-height:340px;min-height:220px;overflow-y:auto;overscroll-behavior:contain}.brief-bubble{padding:14px 16px;border-radius:12px;background:#f3f6ee;margin:0 0 12px;max-width:96%;font-size:12px;line-height:1.8}.brief-bubble b{font-size:10px;color:#70886e}.brief-bubble p{white-space:pre-wrap;overflow-wrap:anywhere}.brief-bubble.user{margin-left:20px;background:#e4eee0}.brief-composer{padding:18px 22px;border-top:1px solid var(--line);margin-top:auto}.brief-actions{display:flex;align-items:center;justify-content:space-between;gap:10px;margin-top:12px}.brief-summary{padding:22px;background:#fafbf7;border-top:1px solid var(--line)}.brief-summary summary{cursor:pointer;font-weight:600;margin-bottom:12px}.brief-summary h3{margin:0 0 18px}.brief-summary textarea{min-height:285px;line-height:1.8;font-size:12px}.brief-summary button{margin-top:18px}@media(max-width:1050px){.brief-grid{grid-template-columns:1fr}.brief-conversation{border-right:0}.brief-messages{max-height:290px;min-height:150px}.brief-summary textarea{min-height:210px}.brief-summary{border-top:1px solid var(--line)}}@media(max-width:650px){.brief-heading,.brief-summary,.brief-messages,.brief-composer{padding:18px}}
</style>
