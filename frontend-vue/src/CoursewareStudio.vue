<script setup>
import {ref,computed,watch,onMounted,onUnmounted,nextTick} from 'vue'
import {templates,layouts} from './templates.js'
import CoursewareEvidence from './CoursewareEvidence.vue'
import ClassroomPresentation from './ClassroomPresentation.vue'
const presenting=ref(false)

const props=defineProps({task:Object,api:Function,token:String,available:Number,refresh:Function,retrying:Boolean})
const emit=defineEmits(['process','new-work','animation','retry'])
const local=ref(null),messages=ref([]),working=ref(false),error=ref(''),notice=ref(''),selected=ref(''),editing=ref(''),chatText=ref(''),chatMode=ref('discuss'),feedback=ref(null),reason=ref(''),consent=ref(false),imageURLs=ref({}),history=ref([]),showHistory=ref(false)
const historyOffset=ref(0),historyMore=ref(false),historyDraft=ref(null)
const editBaseline=ref(''),editRevision=ref(0)
const canvas=ref(null)
const waitStates=['awaiting_preview','awaiting_confirmation','succeeded']
const ready=computed(()=>waitStates.includes(props.task?.status))
const finished=computed(()=>props.task?.status==='succeeded')
const prefix=computed(()=>props.task?.status==='awaiting_preview')
const dirty=computed(()=>local.value&&JSON.stringify(local.value)!==editBaseline.value)
const pendingMessage=computed(()=>messages.value.some(m=>m.status==='pending'))
const editable=computed(()=>ready.value&&!working.value&&!pendingMessage.value)
const approvedCount=computed(()=>(props.task?.slide_reviews||[]).filter(m=>m.approved&&m.vote>=0).length)
const resumeNotice='已恢复逐页检查点，继续只处理问题页和未完成页；未变更页面的审核保留。'
const deckNotice=computed(()=>{const text=local.value?.notice||'';return text.includes(resumeNotice)?(text.replaceAll(resumeNotice,'').trim()+' '+resumeNotice).trim():text})
const allApproved=computed(()=>!!local.value?.slides?.length&&approvedCount.value===local.value.slides.length&&!dirty.value)
const title=computed(()=>props.task?.stage==='auto_recovering'?'正在自动恢复制作':({awaiting_preview:'三页质量预览',expand_pending:'等待生成剩余页',expanding:'正在制作剩余课件',awaiting_confirmation:'逐页审核，准备交付',succeeded:'课件已完成',failed:'制作暂停',cancelled:'已取消'})[props.task?.status]||'正在研究教学资料')
const current=computed(()=>local.value?.slides.find(sl=>sl.id===selected.value))
const generationProgress=computed(()=>{
 const status=props.task?.status,total=props.task?.result?.plan?.pages?.length||props.task?.input?.pages||15,events=props.task?.events||[]
 let completed=Math.min(total,props.task?.result?.slides?.length||0),batch=''
 for(const event of events){
  const done=/^batch_complete_(\d+)$/.exec(event.stage||''),start=/^batch_start_(\d+)_(\d+)$/.exec(event.stage||'')
  if(done){completed=Math.max(completed,Math.min(total,Number(done[1])));batch=''}
  if(start)batch=(completed>=total?'正在检查并修复第':'正在制作第')+start[1]+'至'+start[2]+'页'
  if(event.stage==='repairing')batch='正在自动修复，完成后继续制作'
 }
 const checkpoint=props.task?.result?.generation
 if(checkpoint){
  completed=Math.min(total,props.task?.result?.slides?.length||0)
  const page=checkpoint.repair_page
  if(page&&page<=completed)completed--
  batch=page?(checkpoint.issue||page<=(props.task?.result?.slides?.length||0)?'正在自动修复第':'正在制作第')+page+'页 · 其他页面已保留':''
 }
 const delivered=status==='succeeded',review=['awaiting_preview','awaiting_confirmation'].includes(status),stopped=['failed','cancelled'].includes(status)
 const paused=checkpoint?.repair_page?'制作已暂停 · 待处理第'+checkpoint.repair_page+'页，已保存页面保留':'制作已暂停'
 return {percent:Math.floor(completed/total*100),text:delivered?'已交付':stopped?paused:review?'等待审核':props.task?.stage==='auto_recovering'?'已保存进度，稍后自动继续 · 无需操作':batch||(completed>=total?'正在检查和排版':'正在整理资料与教学方案'),completed,total}
})
const meta=id=>(props.task?.slide_reviews||[]).find(m=>m.id===id)||{}
function qualityStatus(sl){const check=props.task?.result?.page_checks?.[sl.id];if(!check)return '';if(check.content==='sample')return '样例检查，未调用真实模型';if(check.issues?.some(v=>v.severity==='error'))return '内容或画面需修复';if(check.content!=='checked'||check.visual!=='checked')return '内容或画面检查尚未完成';return '内容与实际画面已检查，仍需教师核对'}
const changed=sl=>JSON.stringify(sl)!==JSON.stringify(props.task?.result?.slides?.find(s=>s.id===sl.id))||local.value?.theme!==props.task?.result?.theme||local.value?.title!==props.task?.result?.title
let imageVersion=0,conversationVersion=0,pollTimer,disposed=false,imageTask='',imageKeys={}
function clearImages(){for(const url of Object.values(imageURLs.value))URL.revokeObjectURL(url);imageURLs.value={};imageKeys={}}
async function loadImages(){
 const version=++imageVersion,id=props.task?.id,revision=props.task?.revision
 if(imageTask!==id){clearImages();imageTask=id}
 const deck=props.task?.result,slides=deck?.slides||[]
 const key=sl=>JSON.stringify([deck.title,deck.theme,sl])
 const available=props.task?.preview_images_available?(props.task.preview_slide_ids||slides.map(sl=>sl.id)):[]
 for(const slideId of Object.keys(imageURLs.value)){
  const sl=slides.find(sl=>sl.id===slideId)
  if(!sl||imageKeys[slideId]!==key(sl)||!available.includes(slideId)){URL.revokeObjectURL(imageURLs.value[slideId]);delete imageURLs.value[slideId];delete imageKeys[slideId]}
 }
 for(const sl of slides){
  if(!available.includes(sl.id)||imageURLs.value[sl.id])continue
  if(version!==imageVersion||disposed)return
  try{
   const r=await fetch('/api/tasks/'+id+'/slides/'+encodeURIComponent(sl.id)+'/preview?revision='+revision,{headers:{Authorization:'Bearer '+props.token}})
   if(!r.ok)continue
   const blob=await r.blob()
   if(version!==imageVersion||disposed)return
   imageURLs.value[sl.id]=URL.createObjectURL(blob)
   imageKeys[sl.id]=key(sl)
  }catch{}
 }
}
async function loadMessages(){
 const version=++conversationVersion,id=props.task?.id
 if(!id)return
 const rows=await props.api('/tasks/'+id+'/conversation')
 if(version===conversationVersion&&props.task?.id===id&&!disposed)messages.value=rows
}
watch([()=>props.task?.id,()=>props.task?.revision],async([id,revision],[oldId,oldRevision]=[])=>{
 if(id!==oldId){messages.value=[];selected.value='';editing.value='';history.value=[];historyDraft.value=null;showHistory.value=false;consent.value=false;error.value='';notice.value='';feedback.value=null;chatText.value=''}
 if(id===oldId&&dirty.value){notice.value='后台版本已更新，当前未保存修改已保留。保存前请核对最新版本。'}
 else{local.value=props.task?.result?.slides?JSON.parse(JSON.stringify(props.task.result)):null;editBaseline.value=JSON.stringify(local.value);editRevision.value=revision||0}
 if(!local.value?.slides.some(sl=>sl.id===selected.value))selected.value=local.value?.slides[0]?.id||''
 if(revision!==oldRevision)consent.value=false
 await Promise.allSettled([loadImages(),loadMessages()])
},{immediate:true})
watch(()=>props.task?.events?.length,()=>{if(!working.value)loadMessages().catch(()=>{})})
watch(()=>props.task?.preview_slide_ids?.join(','),()=>loadImages())
watch(()=>props.task?.result?.evaluation,v=>{if(local.value){if(v!==undefined)local.value.evaluation=JSON.parse(JSON.stringify(v));else delete local.value.evaluation}},{deep:true})
async function reveal(selector){
 await nextTick()
 const panel=canvas.value,target=panel?.querySelector(selector)
 if(target)panel.scrollTo({top:panel.scrollTop+target.getBoundingClientRect().top-panel.getBoundingClientRect().top-16,behavior:'smooth'})
}
function editSlide(id){selected.value=id;editing.value=editing.value===id?'':id;if(editing.value)void reveal('.studio-slide-editor')}
async function run(fn){if(working.value)return;working.value=true;error.value='';notice.value='';try{await fn()}catch(e){error.value=e.message;void reveal('.alert.error')}finally{working.value=false}}
async function persist(){
 if(!dirty.value)return
 if(editRevision.value!==props.task.revision)throw Error('后台版本已变化，当前修改仍保留；请核对最新版本后再保存，避免覆盖其他修改。')
 const path='/tasks/'+props.task.id+(finished.value?'/edit':'/draft')
 await props.api(path,'POST',{result:local.value,revision:editRevision.value})
 editBaseline.value=JSON.stringify(local.value)
 await props.refresh();await nextTick()
 notice.value='内容已保存，变更的页面需要重新审核。'
}
async function approve(id,approved){
 await run(async()=>{
  await persist()
  const m=meta(id)
  if(!m.hash)throw Error('页面已更新，请刷新后审核')
  await props.api('/tasks/'+props.task.id+'/slides/'+id+'/review','POST',{revision:props.task.revision,hash:m.hash,approved})
  await props.refresh()
})
}
async function approveAll(){
 if(!editable.value||allApproved.value)return
 const id=props.task.id
 await run(async()=>{
  await persist()
  if(disposed||props.task?.id!==id)return
  await props.refresh();await nextTick()
  if(disposed||props.task?.id!==id)return
  if(!ready.value||pendingMessage.value)throw Error('课件正在生成或修订，请稍后审核')
  const revision=props.task.revision
  const rows=props.task.result.slides.map((sl,i)=>{
   const m=meta(sl.id)
   if(!m.hash)throw Error('页面已更新，请刷新后审核')
   if(m.vote<0)throw Error('第'+(i+1)+'页仍有点踩，请先修改或撤销反馈后再一键审核')
   return {...m}
  })
  try{
   for(const m of rows.filter(m=>!m.approved)){
    if(disposed||props.task?.id!==id)return
    if(props.task.revision!==revision)throw Error('当前版本已变化，请审核最新内容')
    await props.api('/tasks/'+id+'/slides/'+encodeURIComponent(m.id)+'/review','POST',{revision,hash:m.hash,approved:true})
   }
  }catch(e){
   if(disposed||props.task?.id!==id)return
   await props.refresh().catch(()=>{})
   if(disposed||props.task?.id!==id)return
   throw Error('一键审核未全部完成，已记录的审核会保留。'+e.message+'；请刷新后重试以继续审核未通过的页面。')
  }
  if(disposed||props.task?.id!==id)return
  await props.refresh();await nextTick()
  if(disposed||props.task?.id!==id)return
  if(props.task.revision!==revision||!allApproved.value)throw Error('页面或审核状态已变化，请刷新后重新审核')
  notice.value='当前'+rows.length+'页已全部审核通过。'+(prefix.value?'请另行确认生成剩余页。':finished.value?'':'完整交付仍需单独确认。')
 })
}
function vote(id,value,message=false){
 const row=message?messages.value.find(m=>m.id===id):meta(id)
 if(row?.vote===value){void recordVote(id,0,'',message);return}
 if(value<0){feedback.value={id,message};reason.value='';void reveal('.feedback-form');return}
 void recordVote(id,value,'',message)
}
async function recordVote(id,value,text,message=false){
 await run(async()=>{
  if(!message)await persist()
  const path='/tasks/'+props.task.id+(message?'/conversation/'+id+'/feedback':'/slides/'+id+'/feedback')
  await props.api(path,'POST',{revision:props.task.revision,hash:message?undefined:meta(id).hash,vote:value,reason:text})
  feedback.value=null
  await props.refresh();await loadMessages()
  notice.value=value<0?'点踩已记录。页面需修改后重新审核，反馈会带入后续修订。':'反馈已保存。不会扣教师积分。'
 })
}
async function send(){
 if(!chatText.value.trim())return
 const content=chatText.value.trim(),mode=chatMode.value,target=selected.value
 await run(async()=>{
  if(mode==='revise'&&!target)throw Error('先选择需要修改的页面')
  if(mode==='revise'&&finished.value&&props.available<5)throw Error('单页AI改写需5积分，目前可用额度不足')
  await persist()
  await props.api('/tasks/'+props.task.id+'/conversation','POST',{content,mode,slide_id:target,revision:props.task.revision,idempotency_key:crypto.randomUUID()})
  chatText.value=''
  await props.refresh();await loadMessages()
 })
}
async function expand(){await run(async()=>{await props.api('/tasks/'+props.task.id+'/expand','POST',{revision:props.task.revision});await props.refresh()})}
async function finish(){await run(async()=>{await props.api('/tasks/'+props.task.id+'/confirm','POST',{revision:props.task.revision,result:local.value,accept_review:consent.value});await props.refresh()})}
async function cancel(){await run(async()=>{await props.api('/tasks/'+props.task.id+'/cancel','POST',{});await props.refresh()})}
async function download(preview=false,version,format='pptx'){
 await run(async()=>{
  const path='/api/tasks/'+props.task.id+(preview?'/preview-pptx':version?'/revisions/'+version+'/download/'+format:'/download/'+format)
  const r=await fetch(path,{headers:{Authorization:'Bearer '+props.token}})
  if(!r.ok)throw Error('文件尚不可用')
  const url=URL.createObjectURL(await r.blob()),a=document.createElement('a')
  a.href=url;a.download=(local.value?.title||'课件')+(preview?'-预览':'')+'.'+format;a.click();setTimeout(()=>URL.revokeObjectURL(url),2000)
 })
}
function addSlide(index){
 if(!editable.value||prefix.value)return
 const slide={id:crypto.randomUUID(),title:'新页面',layout:'concept',bullets:['填写具体教学问题'],notes:'请补充讲解依据与答案。'}
 local.value.slides.splice(index+1,0,slide);selected.value=slide.id;editing.value=slide.id
}
function duplicateSlide(index){
 if(!editable.value||prefix.value)return
 const slide=JSON.parse(JSON.stringify(local.value.slides[index]));slide.id=crypto.randomUUID();slide.title+=' · 副本'
 local.value.slides.splice(index+1,0,slide);selected.value=slide.id;editing.value=slide.id
}
function removeSlide(index){
 if(!editable.value||prefix.value||local.value.slides.length<=(local.value.plan?3:8))return
 if(window.confirm('删除本页？保存后会形成新版本，已保存的旧版本保留。'))local.value.slides.splice(index,1)
}
function moveSlide(index,step){
 if(!editable.value||prefix.value||index+step<0||index+step>=local.value.slides.length)return
 const [sl]=local.value.slides.splice(index,1);local.value.slides.splice(index+step,0,sl)
}
async function versions(more=false){await run(async()=>{
 const offset=more?historyOffset.value:0
 const rows=await props.api('/tasks/'+props.task.id+'/revisions?offset='+offset)
 history.value=more?history.value.concat(rows.slice(0,50)):rows.slice(0,50)
 historyMore.value=rows.length>50;historyOffset.value=offset+Math.min(rows.length,50);showHistory.value=true;await reveal('.studio-history')
})}
async function viewVersion(version){await run(async()=>{historyDraft.value=await props.api('/tasks/'+props.task.id+'/revisions/'+version);await reveal('.studio-history details')})}
async function restore(version){
 if(!window.confirm('恢复为新版本，替换当前未保存修改。旧版本保留，恢复免费。继续吗？'))return
 await run(async()=>{await props.api('/tasks/'+props.task.id+'/revisions/'+version+'/restore','POST',{revision:props.task.revision});await props.refresh();await loadMessages();showHistory.value=false})
}
onMounted(()=>{pollTimer=setInterval(async()=>{if(working.value||!pendingMessage.value)return;try{await loadMessages();await props.refresh()}catch(e){error.value=e.message}},2000)})
onUnmounted(()=>{disposed=true;imageVersion++;conversationVersion++;clearInterval(pollTimer);clearImages()})
</script>

<template>
 <section class="courseware-studio" aria-label="对话课件工作台"><ClassroomPresentation v-if="presenting&&local" :deck="local" :images="imageURLs" :saved="!dirty" :token="token" @close="presenting=false"/>
  <header class="studio-header"><div><span class="eyebrow">CO-CREATE · REVIEW · TEACH</span><h2>{{local?.title||task.input?.topic}}</h2><p>{{title}} · {{task.input?.pages}}页教学目标</p></div><div class="studio-header-actions"><button v-if="finished&&local" @click="presenting=true">课堂演示</button><button @click="emit('process')">查看过程</button><button v-if="local" @click="versions(false)" :disabled="working">版本历史</button><button @click="emit('new-work')" :disabled="working||!['succeeded','failed','cancelled'].includes(task.status)">新建课件</button><button v-if="local&&ready" @click="download(!finished)" :disabled="working" class="primary">{{finished?'导出 PPTX':'下载预览 PPTX'}}</button><button v-if="finished" @click="download(false,undefined,'md')" :disabled="working">教案 MD</button></div></header>
  <div class="studio-body">
   <aside class="studio-conversation" aria-label="课件对话">
    <div class="conversation-heading"><span class="studio-avatar" aria-hidden="true">✦</span><div><b>课件协作助手</b><small>与你一起，把每一页讲清楚</small></div></div>
    <div class="conversation-log" aria-live="polite">
     <div class="chat-bubble user"><small>你的教学目标</small><p>{{task.input?.topic}} · {{task.input?.grade}} {{task.input?.subject}}</p><p>{{task.input?.objectives}}</p></div>
     <div class="chat-bubble assistant"><p>先制作三页质量预览，确认内容、图示与教学方向后，再完成其余页面。</p><div class="chat-artifact"><span>▤</span><div><b>{{local?.title||task.input?.topic}}</b><small>{{local?local.slides.length+'页已制作':'正在研究资料'}} · {{task.mode==='sample'?'内置样例':'AI协作'}}</small></div></div></div>
     <template v-for="m in messages" :key="m.id">
      <div class="chat-bubble user"><small>{{m.mode==='revise'?'修改指定页':'对话讨论'}}</small><p>{{m.content}}</p></div>
      <div class="chat-bubble assistant"><p v-if="m.status==='failed'" class="chat-failure">{{m.error||'这次回复未完成，原页面保留。'}}</p><p v-else>{{m.reply||(m.status==='pending'?'正在理解你的要求…':'回复已完成')}}</p><small v-if="m.child_id&&m.status==='completed'">单页AI修改已写入新版本，请审核更新后的页面。</small><div v-if="m.status==='completed'" class="vote-actions"><button @click="vote(m.id,1,true)" :aria-pressed="m.vote===1" :aria-label="'点赞回复 '+m.id" :disabled="working">👍</button><button @click="vote(m.id,-1,true)" :aria-pressed="m.vote===-1" :aria-label="'点踩回复 '+m.id" :disabled="working">👎</button><small v-if="m.vote<0">已记录改进意见</small></div></div>
     </template>
    </div>
    <form class="chat-composer" @submit.prevent="send">
     <div class="chat-mode"><button type="button" :class="{active:chatMode==='discuss'}" @click="chatMode='discuss'" :disabled="working">讨论课件</button><button type="button" :class="{active:chatMode==='revise'}" @click="chatMode='revise'" :disabled="working">修改本页</button></div>
     <small v-if="chatMode==='revise'">{{current?'当前第'+(local.slides.findIndex(s=>s.id===selected)+1)+'页：'+current.title:'在右侧选择需要修改的页'}} · {{finished?'成功改写5积分':'草稿阶段不另扣业务积分'}}</small>
     <textarea v-model="chatText" rows="3" maxlength="600" :disabled="!ready||working||pendingMessage" aria-label="继续课件对话" :placeholder="chatMode==='revise'?'例如：把这一页改成实验探究，补充控制变量和安全提示。':'例如：这三页是否适合八年级学生？有哪些概念需要先解释？'"></textarea>
     <div class="chat-send-row"><span>{{task.mode==='sample'?'样例模式，不调用真实模型':'对话请求会调用文本模型'}}<br>反馈不代表已训练模型</span><button class="primary" :disabled="!ready||working||pendingMessage||!chatText.trim()">{{working||pendingMessage?'处理中…':'发送 ↑'}}</button></div>
    </form>
   </aside>
   <div ref="canvas" class="studio-canvas" role="region" aria-label="课件页面滚动区域" tabindex="0">
    <div class="studio-stage-banner" role="status"><b>{{title}}</b><p v-if="prefix">当前为三页质量预览。逐页审核并确认后制作其余内容，完成后按教学顺序排列。</p><p v-else-if="task.status==='awaiting_confirmation'">已确认的预览页保留；全部页面审核后完整导出。</p><p v-else-if="task.status==='failed'">{{task.error}}</p><p v-else>{{task.events?.at(-1)?.message||'正在准备课件，页面会自动更新。'}}</p><span v-if="local">{{approvedCount}} / {{local.slides.length}} 页审核通过 · 点踩撤销本页确认</span><div class="studio-progress"><div><b>{{generationProgress.text}}</b><span>内容生成 {{generationProgress.percent}}%</span></div><progress :value="generationProgress.percent" max="100" aria-label="内容生成进度"></progress><small>本次已生成 {{generationProgress.completed}} / {{generationProgress.total}} 页内容 · 排版与审核后交付</small></div></div>
    <p v-if="error" class="alert error" role="alert">{{error}}</p><p v-if="notice" class="alert success" role="status">{{notice}}</p>
    <button v-if="task.status==='failed'||task.status==='cancelled'&&task.restored_at" class="primary" @click="emit('retry')" :disabled="working||retrying||available<20">{{retrying?'正在恢复…':'继续制作 · 重新预留20积分'}}</button><p v-if="(task.status==='failed'||task.status==='cancelled'&&task.restored_at)&&available<20" class="fine">继续制作需要20可用积分，请先到套餐与账单补充积分。</p>
    <div v-if="feedback" class="feedback-form" role="region" aria-label="点踩原因"><h3>哪里需要改进？</h3><p>明确的原因会作为下次修订约束。它不会扣你的积分或自动训练模型。</p><textarea v-model="reason" rows="3" maxlength="400" aria-label="点踩原因" placeholder="例如：公式条件不完整、例题太简单、文字过密、配图不准确。"></textarea><div><button class="primary" @click="recordVote(feedback.id,-1,reason,feedback.message)" :disabled="working||!reason.trim()">提交点踩原因</button><button @click="feedback=null" :disabled="working">取消</button></div></div>
    <template v-if="local">
     <div class="studio-toolbar"><label>课件标题<input v-model="local.title" :readonly="!editable" maxlength="100"></label><label>视觉模板<select v-model="local.theme" :disabled="!editable"><option v-for="t in templates" :key="t.id" :value="t.id">{{t.name}}</option></select></label><button @click="run(persist)" :disabled="!editable||!dirty">保存修改{{finished?'并免费重导出':''}}</button></div>
     <p class="studio-note">{{deckNotice}}</p><p v-if="dirty" class="unsaved-note" role="status">存在未保存修改：截图仍为已保存版本，变更页面需重新审核。<button v-if="editRevision!==task.revision" @click="local=JSON.parse(JSON.stringify(task.result));editBaseline=JSON.stringify(local);editRevision=task.revision" :disabled="working">放弃本地修改，查看最新版本</button></p>
     <details v-if="task.result?.evaluation" class="studio-lesson"><summary>独立模型辅助评审 · 不替代教师逐页审核</summary><p>{{task.result.evaluation.status==='completed'?'评审已完成，仍需教师核验':task.result.evaluation.reason}}</p><pre v-if="task.result.evaluation.assessment" class="assessment">{{JSON.stringify(task.result.evaluation.assessment,null,2)}}</pre></details>
     <details class="studio-lesson"><summary>教案、教学路线与资料来源（保留原有功能）</summary><label>教案<textarea v-model="local.lesson" rows="8" :readonly="!editable"></textarea></label><article v-for="s in task.sources" :key="s.id"><b>[{{s.id}}] {{s.title}}</b><small>{{s.location}} · {{s.kind}}</small><p>{{s.content}}</p><a v-if="s.url&&/^https?:\/\//.test(s.url)" :href="s.url" target="_blank" rel="noopener noreferrer">查看来源 ↗</a></article><p v-if="!task.sources?.length">未找到可引用资料，需教师核验；不编造出处。</p></details>
     <section v-if="showHistory" class="studio-history"><h3>版本历史</h3><p>保留最近20个版本及首个版本，当前成果保留；已交付课件可以免费恢复完整版本。</p><div v-for="v in history" :key="v.revision"><span>v{{v.revision}} · {{v.pages}}页 · {{v.title}}</span><button @click="viewVersion(v.revision)" :disabled="working">查看内容</button><button @click="download(false,v.revision)" :disabled="working">下载旧版</button><button v-if="finished&&v.pages>=(local.plan?3:8)" @click="restore(v.revision)" :disabled="!editable">免费恢复</button></div><button v-if="historyMore" @click="versions(true)" :disabled="working">加载更多版本</button><details v-if="historyDraft" open><summary>v{{historyDraft.revision}} · {{historyDraft.result.title}}（只读）</summary><p class="history-text">{{historyDraft.result.lesson}}</p><article v-for="(sl,i) in historyDraft.result.slides" :key="i"><h4>{{i+1}} · {{sl.title}}</h4><ul><li v-for="(b,j) in sl.bullets" :key="j">{{b}}</li></ul><CoursewareEvidence :slide="sl" preview/><p v-if="sl.formula">{{sl.formula}}</p><p class="history-text">{{sl.notes}}</p></article></details><button @click="showHistory=false;historyDraft=null">关闭</button></section>
     <article v-for="(sl,i) in local.slides" :key="sl.id" :class="['studio-slide',{selected:selected===sl.id}]" :data-slide-id="sl.id">
      <div class="studio-slide-top"><button class="slide-select" @click="selected=sl.id" :aria-pressed="selected===sl.id">教学位置 {{sl.order||i+1}} · 当前 {{i+1}} / {{local.slides.length}} <span>{{changed(sl)?'有修改，待保存':meta(sl.id).approved?'审核通过':'待审核'}}</span></button><div class="slide-tools"><button @click="editSlide(sl.id)" :disabled="!editable" :aria-label="'编辑第'+(i+1)+'页'">✎ 编辑</button><button @click="moveSlide(i,-1)" :disabled="!editable||prefix||i===0" :aria-label="'上移第'+(i+1)+'页'">↑</button><button @click="moveSlide(i,1)" :disabled="!editable||prefix||i===local.slides.length-1" :aria-label="'下移第'+(i+1)+'页'">↓</button><button @click="addSlide(i)" :disabled="!editable||prefix||local.slides.length>=100" :aria-label="'在第'+(i+1)+'页后添加'">＋</button><button @click="duplicateSlide(i)" :disabled="!editable||prefix||local.slides.length>=100" :aria-label="'复制第'+(i+1)+'页'">▣</button><button @click="removeSlide(i)" :disabled="!editable||prefix||local.slides.length<=(local.plan?3:8)" :aria-label="'删除第'+(i+1)+'页'">⌫</button></div></div>
      <button class="studio-slide-picture" @click="selected=sl.id" :aria-label="'选择第'+(i+1)+'页预览'"><img v-if="imageURLs[sl.id]" :src="imageURLs[sl.id]" :alt="'第'+(i+1)+'页实际PPT截图：'+sl.title"><div v-else :class="['slide-fallback',local.theme]"><small>布局示意 · 可下载实际PPTX</small><h3>{{sl.title}}</h3><ul><li v-for="(b,j) in sl.bullets" :key="j">{{b}}</li></ul><CoursewareEvidence :slide="sl" preview/><p v-if="sl.formula">{{sl.formula}}</p></div></button>
      <small class="actual-preview-caption">{{imageURLs[sl.id]?'实际PPTX渲染截图 · 修改保存后更新':'当前环境无实际截图；上方是布局示意，不冒充PPT截图'}}</small>
      <p v-if="qualityStatus(sl)" class="studio-note" role="status">{{qualityStatus(sl)}}</p>
      <div v-if="editing===sl.id" class="studio-slide-editor">
       <label>标题<input v-model="sl.title" maxlength="70" :readonly="!editable"></label><div class="editor-row"><label>版式<select v-model="sl.layout" :disabled="!editable"><option v-for="l in layouts" :key="l[0]" :value="l[0]">{{l[1]}}</option></select></label><label>章节<input v-model="sl.section" maxlength="80" :readonly="!editable"></label></div>
       <label>正文（每行一条要点）<textarea :value="sl.bullets.join('\n')" @input="sl.bullets=$event.target.value.split('\n').filter(s=>s.trim())" rows="4" :readonly="!editable"></textarea></label><label>课堂答案（默认隐藏，教师点击后展示）<textarea v-model="sl.answer" :readonly="!editable" maxlength="2000" rows="3"></textarea></label><label>讲解备注与答案<textarea v-model="sl.notes" rows="5" :readonly="!editable"></textarea></label>
       <div class="editor-row"><label>LaTeX公式<input v-model="sl.formula" :readonly="!editable"></label><label>固定示意图<select v-model="sl.diagram" :disabled="!editable||!!sl.graph||!!sl.scientific"><option value="">不使用</option><option value="triangle">直角三角形</option><option value="buoyancy">浮力方向</option></select></label></div>
       <template v-if="sl.chart"><label>图表标题<input v-model="sl.chart.title" :readonly="!editable"></label><label>图表类型<select v-model="sl.chart.style" :disabled="!editable"><option value="bar">柱状</option><option value="line">折线 · 等间隔</option></select></label><div class="editor-row" v-for="(_,j) in sl.chart.labels" :key="j"><label>标签<input v-model="sl.chart.labels[j]" :readonly="!editable"></label><label>数值<input v-model.number="sl.chart.values[j]" type="number" step="any" :readonly="!editable"></label></div></template>
       <CoursewareEvidence :slide="sl" :editable="editable"/>
       <button @click="run(persist)" :disabled="!editable||!dirty" class="primary">保存本页及其他修改</button>
      </div>
      <div class="studio-slide-bottom"><div class="vote-actions"><button @click="vote(sl.id,1)" :aria-pressed="meta(sl.id).vote===1" :disabled="!editable" :aria-label="'点赞第'+(i+1)+'页'">👍 满意</button><button @click="vote(sl.id,-1)" :aria-pressed="meta(sl.id).vote===-1" :disabled="!editable" :aria-label="'点踩第'+(i+1)+'页'">👎 需改进</button><small v-if="meta(sl.id).vote<0">{{meta(sl.id).reason}}</small></div><div><button @click="selected=sl.id;chatMode='revise'" :disabled="!editable">对话修改本页</button><button @click="approve(sl.id,!meta(sl.id).approved)" :disabled="!editable||meta(sl.id).vote<0" :class="{approved:meta(sl.id).approved}">{{meta(sl.id).approved?'撤销本页确认':'✓ 本页审核通过'}}</button><button v-if="finished" @click="emit('animation',sl)">设计动画</button></div></div>
     </article>
     <section class="studio-templates" aria-label="横向课件模板"><div class="template-heading"><h3>选择课件模板</h3><small>{{templates.find(t=>t.id===local.theme)?.name}}</small></div><div class="themes horizontal-themes"><label v-for="t in templates" :key="t.id" :class="['theme',t.id,{chosen:local.theme===t.id}]" :title="t.description"><input type="radio" v-model="local.theme" :value="t.id" name="studio-theme" :disabled="!editable"><span class="theme-mini" aria-hidden="true"><b>{{t.mark}}</b><i></i><i></i><em>课堂探索</em></span><strong>{{t.name}}</strong><small>{{t.tag}}</small></label></div></section>
     <footer class="studio-completion"><div><b>{{approvedCount}} / {{local.slides.length}} 页已审核</b><p>{{prefix?'确认三页质量预览后，再制作其余页面；仍只预留20积分。':finished?'已交付。免费手改与模板重导出保留；AI单页改写成功后5积分。':'所有页面审核通过后才完整交付，成功后结算20积分。'}}</p></div><button v-if="ready" class="studio-approve-all" @click="approveAll" :disabled="!editable||allApproved" aria-label="一键审核通过" title="确认当前已生成页面">{{working?'处理中…':allApproved?'全部已审核':'一键审核通过'}}</button><button v-if="prefix" class="primary" @click="expand" :disabled="working||pendingMessage||!allApproved">确认预览，制作其余 {{generationProgress.total-3}} 页 →</button><template v-else-if="task.status==='awaiting_confirmation'"><label class="consent"><input v-model="consent" type="checkbox">我已核对内容与引用，知悉待核验内容及20积分费用</label><button class="primary" @click="finish" :disabled="working||pendingMessage||!allApproved||!consent">全部审核通过，完整交付 →</button></template><button v-if="!['succeeded','failed','cancelled'].includes(task.status)" @click="cancel" :disabled="working">取消并释放预留</button></footer>
    </template>
    <div v-else class="studio-wait"><template v-if="['failed','cancelled'].includes(task.status)"><h3>{{task.status==='failed'?'本次尚无可用预览':'任务已取消'}}</h3><p>原预留已释放，未扣生成积分。可以继续制作或新建课件。</p></template><template v-else><span class="spinner"></span><h3>整理资料与整课规划</h3><p>正在准备三页质量预览。</p></template></div>
   </div>
  </div>
 </section>
</template>

<style scoped>
.studio-templates{padding:20px 0;margin-top:22px;border-top:1px solid #e8ebf1;min-width:0}.studio-templates h3{font-size:16px;margin:0}.studio-templates small{font-size:11px;color:#70836d}
.courseware-studio{background:#fff;border:1px solid #e5e9ed;border-radius:20px;overflow:hidden;box-shadow:0 12px 50px #27304b08;min-width:0}.studio-header{padding:24px 28px;border-bottom:1px solid #e8ebf1;display:flex;justify-content:space-between;gap:20px;align-items:center}.studio-header h2{font-size:25px;margin:7px 0}.studio-header p{margin:0;color:#74808d}.studio-header-actions{display:flex;gap:7px;flex-wrap:wrap;justify-content:flex-end}.studio-header-actions button{padding:9px 12px;font-size:12px}.studio-body{display:grid;grid-template-columns:minmax(270px,340px) minmax(0,1fr)}.studio-conversation{background:#f8f9fd;border-right:1px solid #e8ebf1;padding:22px;display:flex;flex-direction:column;position:sticky;top:0;align-self:start;height:calc(100vh - 150px);min-height:540px;max-height:940px}.conversation-heading{display:flex;gap:11px;align-items:center;margin-bottom:20px}.conversation-heading b{font-size:15px}.conversation-heading small{display:block;margin-top:4px;color:#8490a1;font-size:11px}.studio-avatar{width:36px;height:36px;display:grid;place-items:center;background:#ece5ff;color:#7548c4;border-radius:12px;font-size:22px}.conversation-log{flex:1;overflow-y:auto;padding-right:4px;scrollbar-width:thin;min-height:180px}.chat-bubble{padding:15px 17px;border-radius:14px;margin-bottom:17px;font-size:13px;line-height:1.7;overflow-wrap:anywhere;white-space:pre-wrap}.chat-bubble p{margin:5px 0}.chat-bubble.user{background:#efedf8;margin-left:12px;color:#35334a}.chat-bubble.assistant{background:#fff;border:1px solid #e7e7f1;margin-right:5px}.chat-bubble small{font-size:10px;color:#8990a3}.chat-artifact{display:flex;gap:12px;align-items:center;border:1px solid #ddd1fa;border-radius:10px;padding:12px;margin-top:14px;color:#7044b9}.chat-artifact>span{font-size:24px}.chat-artifact small{display:block;margin-top:4px}.chat-composer{padding:15px;background:#fff;border:1px solid #ded9ef;border-radius:14px;margin-top:12px;box-shadow:0 6px 18px #5d399906}.chat-composer textarea{border:0;box-shadow:none!important;resize:vertical;padding:10px 0;font-size:12px;background:transparent;min-height:88px}.chat-mode{display:flex;gap:6px;margin-bottom:8px}.chat-mode button{padding:6px 9px;font-size:11px;border:0;background:#f2f4f8}.chat-mode button.active{background:#e9e1fb;color:#7044b9}.chat-composer>small{font-size:10px;color:#7c7b8a;display:block}.chat-send-row{display:flex;justify-content:space-between;align-items:center;gap:6px}.chat-send-row span{font-size:9px;color:#9096a4}.chat-send-row button{padding:8px 13px;font-size:12px}.studio-canvas{padding:24px 28px 0;min-width:0}.studio-stage-banner{padding:17px 21px;border-radius:12px;background:#f1f7f4;border:1px solid #d8e8df;margin-bottom:20px;font-size:13px}.studio-stage-banner p{margin:7px 0;line-height:1.6;color:#62756b}.studio-stage-banner span{font-size:11px;color:#547565}.studio-toolbar{display:flex;align-items:end;gap:12px;margin:15px 0}.studio-toolbar label{font-size:10px;flex:1}.studio-toolbar label:first-child{flex:2}.studio-toolbar input,.studio-toolbar select{margin-top:5px}.studio-toolbar button{flex-shrink:0;padding:11px 13px;font-size:11px}.studio-note{font-size:11px;color:#8a94a2;line-height:1.7}.unsaved-note{font-size:12px;color:#977023;background:#fff7e2;padding:12px;border-radius:9px}.studio-lesson,.studio-history{font-size:12px;margin:20px 0;border:1px solid #e8ebf1;border-radius:12px;padding:15px}.studio-lesson summary{cursor:pointer;color:#697488}.studio-lesson label{margin-top:16px}.studio-lesson article{border-top:1px solid #e9edf1;padding:12px 0}.studio-lesson article small{display:block;margin-top:5px;color:#8993a0}.studio-lesson article p{white-space:pre-wrap}.studio-history>div{display:flex;flex-wrap:wrap;gap:10px;margin:8px 0;align-items:center}.studio-history button{padding:7px 12px}.studio-slide{margin-bottom:30px;scroll-margin-top:30px}.studio-slide-top{display:flex;justify-content:space-between;align-items:center;gap:8px;margin-bottom:10px}.slide-select{border:none;background:transparent;padding:0;color:#8a91a4;font-size:11px;text-align:left}.slide-select span{margin-left:10px;font-size:10px;background:#f2f4f8;border-radius:6px;padding:3px 6px}.slide-tools{display:flex;gap:3px}.slide-tools button{padding:4px 7px;font-size:12px;border:0;background:transparent;color:#8490a3}.studio-slide-picture{display:block;width:100%;padding:0;border:2px solid #e3e6f0;border-radius:15px;overflow:hidden;background:#fff;box-shadow:0 5px 18px #25375909;text-align:left}.studio-slide.selected .studio-slide-picture{border-color:#9b7bdb;box-shadow:0 0 0 3px #9b7bdb15}.studio-slide-picture img{display:block;width:100%;aspect-ratio:16/9;object-fit:contain}.actual-preview-caption{display:block;font-size:9px;color:#99a1b0;margin:7px 2px}.slide-fallback{aspect-ratio:16/9;padding:7%;display:flex;flex-direction:column;justify-content:center;overflow:hidden;background:#f1f7f4;color:#163e30}.slide-fallback h3{font-size:clamp(20px,2.3vw,38px);margin:10px 0}.slide-fallback ul{font-size:clamp(12px,1.2vw,19px);line-height:1.7;padding-left:22px}.slide-fallback small{font-size:10px;color:#8e98a7}.slide-fallback.midnight{background:#101b2c;color:#e7ebf3}.slide-fallback.violet{background:#f3effb;color:#3c2167}.slide-fallback.ocean{background:#edf4ff;color:#1f4c8b}.slide-fallback.paper{background:#faf4e8;color:#52442b}.slide-fallback.coral{background:#fff0e8;color:#9a3e2a}.studio-slide-bottom{display:flex;align-items:center;justify-content:space-between;gap:10px;flex-wrap:wrap;margin-top:12px}.studio-slide-bottom>div{display:flex;gap:6px;align-items:center;flex-wrap:wrap}.studio-slide-bottom button{font-size:11px;padding:7px 10px}.vote-actions{display:flex;gap:5px;align-items:center;flex-wrap:wrap}.vote-actions button{border:0;background:#f4f5f9;color:#6b7686;font-size:11px;padding:6px 9px}.vote-actions button[aria-pressed=true]{background:#e6def8;color:#7146b7}.vote-actions small{font-size:10px;color:#a3724b;max-width:280px}.studio-slide-bottom .approved{color:#32745a;background:#e8f5ed;border-color:#b6ddc8}.studio-slide-editor{background:#fafbfe;border:1px solid #e6eaf3;border-radius:12px;margin:14px 0;padding:20px;font-size:12px}.studio-slide-editor label{margin-bottom:12px}.editor-row{display:grid;grid-template-columns:1fr 1fr;gap:12px}.studio-slide-editor button{padding:9px 12px;font-size:12px}.studio-completion{margin-top:20px;padding:22px 0 25px;border-top:1px solid #e6eaef;display:flex;flex-wrap:wrap;gap:12px;align-items:center;background:#fff;position:sticky;bottom:0;box-shadow:0 -10px 20px #ffffffc4}.studio-completion>div{flex:1;min-width:200px}.studio-completion b{font-size:14px}.studio-completion p{font-size:10px;color:#80908b;line-height:1.6;max-width:440px}.studio-completion button{font-size:11px;padding:12px 16px}.studio-completion .consent{font-size:11px;max-width:330px}.studio-wait{text-align:center;padding:100px 20px;color:#7a8998}.studio-wait h3{font-size:20px}.studio-wait p{font-size:12px}.feedback-form{padding:20px;border:1px solid #ead1bc;background:#fff7f0;border-radius:12px;margin:15px 0}.feedback-form h3{font-size:16px;margin:0}.feedback-form p{font-size:12px;color:#987256}.feedback-form>div{display:flex;gap:9px;margin-top:12px}.chat-failure{color:#a77554}
/* Bound the shared workspace, not each slide: chat and PPT scroll independently. */
.courseware-studio{display:flex;flex-direction:column;height:calc(100dvh - 96px);min-height:480px}
.studio-header{flex-shrink:0}
.studio-body{flex:1;min-height:0;overflow:hidden}
.studio-conversation{position:static;align-self:stretch;height:auto;min-height:0;max-height:none}
.conversation-heading,.chat-composer{flex-shrink:0}
.conversation-log{min-height:0;overscroll-behavior-y:contain}
.studio-canvas{min-height:0;overflow-y:auto;overscroll-behavior-y:contain;scrollbar-gutter:stable;scrollbar-width:thin}
.studio-canvas:focus-visible{outline:2px solid #9b7bdb;outline-offset:-3px}
@media(max-width:1150px){.studio-body{grid-template-columns:270px minmax(0,1fr)}.studio-conversation{padding:16px}.studio-canvas{padding:20px 18px 0}.studio-toolbar{flex-wrap:wrap}.studio-slide-bottom>div{width:100%}.studio-header{padding:20px}.studio-header-actions{max-width:230px}.slide-tools button{padding:4px 5px}}
@media(max-width:760px){.studio-body{display:flex;flex-direction:column}.studio-header{align-items:start;flex-direction:column;padding:17px}.studio-header-actions{max-width:none;justify-content:start}.studio-conversation{position:static;height:auto;max-height:580px;min-height:360px;border-right:0;border-bottom:1px solid #e8ebf1}.conversation-log{max-height:240px}.studio-canvas{padding:18px 13px 0}.studio-slide-top{align-items:start;flex-direction:column}.slide-tools{align-self:end}.studio-toolbar label{min-width:110px}.studio-slide-editor{padding:13px}.editor-row{grid-template-columns:1fr}.studio-completion{position:static}.studio-completion .consent{max-width:none}.studio-completion button{width:100%}.slide-fallback{min-height:240px;aspect-ratio:auto}}
.history-text{white-space:pre-wrap;line-height:1.8;overflow-wrap:anywhere}.studio-history article{padding:10px 0;border-top:1px solid #e8ebf1}
@media(max-width:760px){
 .courseware-studio{height:auto;min-height:0}
 .studio-body{flex:none;overflow:visible}
 .studio-canvas{max-height:75dvh;min-height:320px}
}
</style>
