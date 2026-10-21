<script setup>
import {ref,computed,watch,onUnmounted,nextTick} from 'vue'
import MediaImage from './MediaImage.vue'
import ScienceVisual from './ScienceVisual.vue'
import AuthoredScenePreview from './AuthoredScenePreview.vue'
import {uploadTeachingImage} from './media.js'
import WorkspaceIcon from './WorkspaceIcon.vue'
import examples from './animation-examples.json'
const props=defineProps({initialBrief:Object,task:Object,api:Function,token:String,health:Object,available:Number,canCreate:Boolean,refresh:Function})
const emit=defineEmits(['created','new-work'])
const state=ref(null),busy=ref(false),error=ref(''),topic=ref(''),requirements=ref(''),instruction=ref(''),selected=ref(''),consent=ref(false),preview=ref(''),planJSON=ref('')
const picture=ref(null),pictureCaption=ref(''),pictureSource=ref('教师自备素材'),pictureBusy=ref(false),example=ref('')
const grade=ref('八年级'),subject=ref('数学'),opening=ref('natural'),durationRange=ref('60-90')
watch(()=>props.initialBrief,brief=>{if(brief&&!props.task){topic.value=brief.topic;requirements.value=brief.requirements;grade.value=brief.grade||'八年级';subject.value=brief.subject||'数学'}},{immediate:true})
let requestVersion=0,previewHash='',controller=null,createKey=crypto.randomUUID(),pendingRequest=null
const labels={anim_plan_pending:'等待生成方案',anim_plan_running:'正在生成方案',anim_plan:'请审核分镜方案',anim_rep_pending:'等待制作代表幕',anim_rep_running:'正在制作代表幕',anim_representative:'请观看并确认代表幕',anim_full_pending:'等待生成其余幕',anim_full_running:'正在生成其余幕',anim_acceptance:'请完整播放并验收',anim_edit_pending:'等待局部修改',anim_edit_running:'正在修改指定幕',anim_edit_acceptance:'请验收局部修改',succeeded:'已交付',failed:'制作待恢复',cancelled:'已取消'}
const working=computed(()=>/_pending$|_running$/.test(state.value?.status||''))
const ready=computed(()=>props.health?.animation_configured===true&&props.canCreate)
const phases=['分镜方案','代表幕审核','完整动画','交付与修改']
const phase=computed(()=>{const status=state.value?.status||'';if(status==='succeeded'||status.startsWith('anim_edit'))return 3;if(status.startsWith('anim_full')||status==='anim_acceptance')return 2;if(status.startsWith('anim_rep'))return 1;return 0})
const selectedScene=computed(()=>state.value?.script?.scenes?.find(scene=>scene.id===selected.value))
const canRepairPlan=computed(()=>['anim_plan','anim_representative','anim_acceptance'].includes(state.value?.status)||(['failed','cancelled'].includes(state.value?.status)&&state.value?.script?.scenes?.length&&!state.value?.has_delivery))
const canRevise=computed(()=>canRepairPlan.value||state.value?.status==='succeeded')
const actionsPanel=ref(null)
async function editScene(scene){selected.value=scene.id;if(state.value.status!=='succeeded')instruction.value='只修改“'+scene.title+'”这一幕：';await nextTick();actionsPanel.value?.scrollIntoView({behavior:'smooth',block:'nearest'});actionsPanel.value?.querySelector('textarea')?.focus({preventScroll:true})}
const guidance=computed(()=>state.value?.has_delivery&&['failed','cancelled'].includes(state.value?.status)?'本次修改未完成，已交付版本仍可下载。可恢复当前阶段，或放弃修改。':({anim_plan:'先核对每幕讲解与旁白，确认后制作代表幕。',anim_representative:'先播放代表幕，核对声音、字幕与动作，再继续制作。',anim_acceptance:'完整播放当前版本，核对无误后验收交付。',anim_edit_acceptance:'播放局部修改预览，接受后才使用本幕免费修改次数。',succeeded:'作品已交付，可以下载离线播放，也可以按幕局部修改。',failed:'制作已暂停，方案与音频检查点已保留。内容或时长问题可修改方案，临时服务故障可恢复当前阶段。',cancelled:'制作已取消，可按当前阶段恢复。'})[state.value?.status]||'正在处理当前阶段，进度与已生成内容会保留。')
const path=computed(()=>'/tasks/'+props.task?.id+'/animation')
async function run(fn){if(busy.value)return;busy.value=true;error.value='';try{await fn()}catch(e){error.value=e.message}finally{busy.value=false}}
async function file(url,signal){await props.api('/auth/me');const token=props.token;const r=await fetch('/api'+url,{headers:{Authorization:'Bearer '+token},signal});if(!r.ok){const body=await r.json().catch(()=>({}));throw Error(body.message||'文件暂不可用')}if(token!==props.token)throw Error('登录状态已变化');return r.blob()}
async function load(){
 const id=props.task?.id,version=++requestVersion;if(!id){state.value=null;return}
 const data=await props.api('/tasks/'+id+'/animation');if(version!==requestVersion||id!==props.task?.id)return
 if(state.value?.revision!==data.revision){consent.value=false;planJSON.value=JSON.stringify(data.script,null,2)}
 state.value=data;if(!selected.value&&data.script?.scenes?.length)selected.value=data.script.scenes[0].id
 if(data.has_preview&&previewHash!==data.build_hash){controller?.abort();controller=new AbortController();const blob=await file('/tasks/'+id+'/animation/file',controller.signal);if(version!==requestVersion||id!==props.task?.id)return;if(preview.value)URL.revokeObjectURL(preview.value);preview.value=URL.createObjectURL(blob);previewHash=data.build_hash}
 if(!data.has_preview&&preview.value){URL.revokeObjectURL(preview.value);preview.value='';previewHash=''}
}
watch(()=>[props.task?.id,props.task?.revision,props.task?.status],()=>{load().catch(e=>{if(e.name!=='AbortError')error.value=e.message})},{immediate:true})
watch(()=>props.task?.id,()=>{picture.value=props.task?.input?.animation_picture||null;pictureCaption.value=props.task?.input?.animation_picture?.caption||'';pictureSource.value=props.task?.input?.animation_picture?.source||'教师自备素材';controller?.abort();if(preview.value)URL.revokeObjectURL(preview.value);preview.value='';previewHash='';state.value=null;selected.value='';pendingRequest=null},{immediate:true})
watch(()=>props.token,()=>{picture.value=null;pictureCaption.value='';pictureSource.value='教师自备素材';requestVersion++;controller?.abort();if(preview.value)URL.revokeObjectURL(preview.value);preview.value='';previewHash='';state.value=null;load().catch(()=>{})})
const exportTimer=setInterval(()=>{if(props.task&&['pending','running'].includes(state.value?.export?.status)&&!busy.value&&!document.hidden)load().catch(e=>{if(e.name!=='AbortError')error.value=e.message})},2000)
onUnmounted(()=>{clearInterval(exportTimer);requestVersion++;controller?.abort();if(preview.value)URL.revokeObjectURL(preview.value)})
async function pickPicture(event){
 const target=props.task?.id,token=props.token,file=event.target.files?.[0];if(!file)return;pictureBusy.value=true;error.value=''
 try{
  if(!['image/png','image/jpeg'].includes(file.type))throw Error('教学动画配图请选择PNG或JPEG；GIF可用于课件放映。')
  const asset=await uploadTeachingImage(file,token)
  if(target!==props.task?.id||token!==props.token)return
  picture.value={asset_id:asset.asset_id}
 }catch(e){error.value=e.message}finally{pictureBusy.value=false;event.target.value=''}
}
function picturePayload(){if(!picture.value)return null;if(!pictureCaption.value.trim()||!pictureSource.value.trim())throw Error('请填写配图说明与出处。');return{...picture.value,caption:pictureCaption.value.trim(),source:pictureSource.value.trim()}}

async function savePicture(){await run(async()=>{await props.api(path.value+'/set-picture','POST',{revision:state.value.revision,picture:picturePayload()});await props.refresh();await load()})}
function useExample(item){topic.value=item.topic;subject.value=item.subject;requirements.value=item.requirements;example.value=''}
async function create(){if(!ready.value||!topic.value.trim()||!requirements.value.trim()||pictureBusy.value)return;await run(async()=>{const v=await props.api('/tasks','POST',{type:'animation',animation_engine:'html',animation_opening:opening.value,animation_min_seconds:Number(durationRange.value.split('-')[0]),animation_max_seconds:Number(durationRange.value.split('-')[1]),subject:subject.value,grade:grade.value,source_task_id:props.initialBrief?.source_task_id||'',topic:topic.value.trim(),requirements:requirements.value,animation_picture:picturePayload(),idempotency_key:createKey});createKey=crypto.randomUUID();emit('created',v.id)})}
async function action(op,extra={}){await run(async()=>{let body={revision:state.value.revision,...extra};if(op==='edit'||op==='revise-plan'){const fingerprint=JSON.stringify({op,...body});if(pendingRequest?.fingerprint!==fingerprint)pendingRequest={fingerprint,key:crypto.randomUUID()};body.request_key=pendingRequest.key}await props.api(path.value+'/'+op,'POST',body);pendingRequest=null;instruction.value='';await props.refresh();await load()})}
async function savePlan(){await run(async()=>{let script;try{script=JSON.parse(planJSON.value)}catch{throw Error('方案JSON格式不正确')}await props.api(path.value+'/revise-plan','POST',{revision:state.value.revision,request_key:crypto.randomUUID(),script});await props.refresh();await load()})}
async function exportAction(op){await run(async()=>{await props.api(path.value+'/'+op,'POST',{revision:state.value.accepted_revision});await load()})}
async function download(format='html'){await run(async()=>{const blob=await file('/tasks/'+props.task.id+'/download/'+format);const url=URL.createObjectURL(blob),a=document.createElement('a');a.href=url;a.download='教学动画.'+format;a.click();setTimeout(()=>URL.revokeObjectURL(url),1000)})}
</script>

<template>
 <section :class="['animation-studio',{'is-creating':!task}]" aria-label="HTML教学动画工作台">
  <form v-if="!task" class="panel anim-brief animation-generator" @submit.prevent="create">
   <header class="generator-heading"><h1>教学动画</h1><p>把一个知识点讲清楚，让图形变化、语音与字幕一起呈现。</p></header>
       <section class="anim-examples" aria-label="动画示例"><div class="anim-examples-heading"><h2>先看看画面，再开始设计</h2><span>固定示例 · 非实时 AI 生成 · 无真实语音</span></div><div class="anim-example-grid"><article v-for="item in examples" :key="item.id"><button type="button" class="anim-example-preview" @click="example=item.id" :aria-label="'预览示例：'+item.title"><img :src="item.image" :alt="item.alt"><span>播放示例 →</span></button><div><h3>{{item.title}}</h3><p>{{item.description}}</p><button type="button" @click="useExample(item)">用这个课题</button></div></article></div><section v-if="example" class="anim-example-player"><button type="button" @click="example=''">收起示例</button><iframe :src="'/animation-examples/'+example+'.html'" sandbox="allow-scripts" title="固定动画示例，无真实语音"></iframe></section></section>
<div class="generator-form">
    <div class="anim-scope-note"><WorkspaceIcon name="animation"/><p>各学科通用讲解 · 明亮课堂与示意配图<span>30秒–3分钟；模型直接创作画面，按学科演示结构、运动与关系。</span></p></div>
    <fieldset class="generator-fields" :disabled="busy">
     <label>动画主题 <span class="generator-required">必填</span><input v-model="topic" maxlength="100" required aria-label="动画主题" placeholder="例如：矩形平移后面积是否改变"></label>
     <div class="generator-row"><label>年级<select v-model="grade" aria-label="动画年级"><option>七年级</option><option>八年级</option><option>九年级</option></select></label><label>学科<select v-model="subject" aria-label="动画学科" aria-describedby="anim-scope-help"><option v-for="name in ['语文','数学','英语','物理','化学','生物','历史','地理','道德与法治','信息科技','其他']" :key="name">{{name}}</option></select></label></div>
     <label>自动播放目标时长<select v-model="durationRange" aria-label="动画目标时长"><option value="30-60">30–60 秒</option><option value="60-90">60–90 秒（默认）</option><option value="90-120">90–120 秒</option><option value="120-180">120–180 秒</option></select></label>
     <label>开场方式<select v-model="opening" aria-label="动画开场方式"><option value="natural">自然引出本课问题</option><option value="direct">直接进入讲解（也可在要求中指定开场）</option></select></label>
     <label>演示目标与教学要求 <span class="generator-required">必填</span><textarea v-model="requirements" maxlength="4000" rows="5" required aria-label="动画教学要求" placeholder="希望学生观察什么变化？哪些地方需要停下来思考？例如：固定 b，改变 k，对比直线方向与陡缓，并在结尾安排一道判断题。"></textarea></label>
     <p v-if="initialBrief?.source_task_id" class="anim-source-note"><WorkspaceIcon name="slides"/>已关联原课件，沿用本次页面的教学要求与资料快照。</p>
    </fieldset>
         <details class="anim-picture-upload"><summary>添加教学配图（可选）</summary><p>上传与课题相关的图片，分镜根据说明选用。图片不会交给文本模型识图，请准确说明内容；仅使用你有权使用的素材。</p><label>PNG / JPEG<input type="file" accept="image/png,image/jpeg" @change="pickPicture" :disabled="busy||pictureBusy"></label><template v-if="picture"><MediaImage :picture="picture" alt="待上传教学配图"/><label>配图说明<input v-model="pictureCaption" maxlength="60" placeholder="例如：叶片结构，标注叶脉与叶肉"></label><label>出处<input v-model="pictureSource" maxlength="300"></label><button type="button" @click="picture=null">移除配图</button></template></details>
<p id="anim-scope-help" class="anim-field-note">先生成可修改的分镜方案，再审核带语音的代表幕。AI 合成语音需核对发音与知识内容。</p>
    <details class="generator-options anim-delivery-info"><summary>交付与积分说明<small>离线 HTML / MP4 · 验收后结算</small></summary><div class="generator-options-body"><p>生成和修改方案暂不预留积分。确认方案时预留40积分，成品验收后结算；取消或失败释放预留。</p><p>交付后每幕一次免费局部修改，接受修改才计次。下载的 HTML 包含动画、语音与字幕，可离线播放。</p></div></details>
    <p v-if="error" class="alert error" role="alert">{{error}}</p>
    <p v-if="!ready" class="alert anim-unavailable" role="status">{{!canCreate?'当前成员没有动画制作权限，请联系管理员。':'动画模型或语音尚未就绪，请联系管理员检查服务配置。该功能需要真实接口模式。'}}</p>
    <button class="primary generator-submit" :disabled="busy||pictureBusy||!ready||!topic.trim()||!requirements.trim()" :aria-busy="busy"><WorkspaceIcon name="sparkle"/>{{busy?'正在提交教学要求…':'生成分镜方案'}}</button>
    <div class="generator-submit-note"><p>这一步不预留积分<span>确认方案后预留40积分，完整动画验收后结算。</span></p></div>
   </div>
  </form>
  <template v-else>
   <header class="anim-work-header"><div><h1>{{state?.script?.title||task.input?.topic||'教学动画'}}</h1><p aria-label="下一步操作">下一步：{{guidance}}</p><p>已保存至“我的作品” · 已提交的方案与制作进度会保留。</p></div><button type="button" @click="emit('new-work')"><WorkspaceIcon name="plus"/>新建动画</button></header>
   <p v-if="error" class="alert error" role="alert">{{error}}</p>
   <template v-if="state">
    <ol class="anim-phase-list" aria-label="动画制作阶段"><li v-for="(name,i) in phases" :key="name" :class="{current:i===phase,complete:i<phase}" :aria-current="i===phase?'step':undefined"><span><WorkspaceIcon v-if="i<phase" name="check"/><template v-else>{{i+1}}</template></span>{{name}}</li></ol>
    <section class="anim-stage" aria-label="当前动画状态"><div class="anim-state-line"><strong role="status"><span v-if="working" class="spinner" aria-hidden="true"></span>{{labels[state.status]||state.status}}</strong><small>修订 {{state.revision}}<template v-if="task.input?.animation_max_seconds"> · 目标 {{task.input.animation_min_seconds}}–{{task.input.animation_max_seconds}} 秒</template><template v-if="state.duration_ms"> · 自动播放 {{(state.duration_ms/1000).toFixed(1)}} 秒</template></small></div><span class="anim-charge">{{({unreserved:'尚未预留积分',reserved:'已预留40积分 · 未结算',settled:'已结算40积分',released:'预留已释放'})[state.charge_state]}}</span></section>
    <ul v-if="state.problems?.length" class="alert error"><li v-for="problem in state.problems" :key="problem">{{problem}}</li></ul>
    <div class="anim-workspace">
     <div class="anim-main">
      <section v-if="preview" class="panel anim-preview" aria-label="动画播放区"><header class="anim-section-heading"><h2>{{state.status==='anim_representative'?'代表幕预览':'动画预览'}}</h2><span>语音 · 字幕 · 动画</span></header><iframe :src="preview" sandbox="allow-scripts" title="当前动画修订预览" allow="autoplay"></iframe><p class="anim-preview-note">请实际播放并核对发音、字幕、公式与动作。保留程序校验与教师审核，尚未启用模型截图审核。</p></section>
      <div v-if="working" class="anim-working-note" role="status"><span class="spinner" aria-hidden="true"></span><p>正在处理，可以离开页面，稍后从“我的作品”继续。<span>逐句语音生成可能需要数分钟，已完成的进度和音频会保留。</span></p></div>
      <section v-if="state.script?.scenes?.length" class="panel anim-scenes" aria-label="动画分镜方案"><header class="anim-section-heading"><h2>分镜方案</h2><span>{{state.script.scenes.length}} 幕 · 按教学顺序</span></header>
       <details v-for="(scene,i) in state.script.scenes" :key="scene.id" class="anim-scene" :open="!preview||scene.id===state.script.representative"><summary><span class="anim-scene-number">{{String(i+1).padStart(2,'0')}}</span><span class="anim-scene-title">{{scene.title}}<small>{{scene.intent}}</small></span><span v-if="scene.id===state.script.representative" class="anim-representative-tag">代表幕</span><WorkspaceIcon name="down"/></summary><div class="anim-scene-body"><button v-if="canRevise" :data-edit-scene="scene.id" :disabled="busy||(state.status==='succeeded'&&state.free_used[scene.id])" @click="editScene(scene)">{{state.status==='succeeded'&&state.free_used[scene.id]?'本幕免费修改已用':'修改这一幕'}}</button><AuthoredScenePreview v-if="scene.html" :endpoint="'/api/tasks/'+state.id+'/animation/preview-visual'" :revision="state.revision" :scene-id="scene.id" :token="token"/><template v-for="object in (scene.objects||[]).filter(o=>o.type==='scientific')" :key="object.id"><h3>教学画面预览</h3><ScienceVisual :diagram="object.scientific" :endpoint="'/api/tasks/'+state.id+'/animation/preview-visual'" :request="{revision:state.revision,scene_id:scene.id}"/><p class="anim-preview-note">结构示意预览；正式时间轴、语音和字幕请在代表幕与成品中审核。</p></template><h3>讲解与字幕</h3><p v-for="n in scene.narration" :key="n.id">{{n.display}}</p><details class="anim-technical"><summary>旁白读法与动画动作</summary><p v-for="n in scene.narration" :key="n.id">{{n.speech}}</p><pre>{{JSON.stringify({objects:scene.objects,actions:scene.actions,holds:scene.holds},null,2)}}</pre></details></div></details>
      </section>
     </div>
     <aside ref="actionsPanel" class="panel anim-actions" aria-label="动画审核与修改">
      <header class="anim-section-heading"><h2>{{state.status==='succeeded'?'交付与修改':'审核与调整'}}</h2></header>
             <details v-if="state.status==='anim_plan'" class="anim-picture-upload"><summary>上传或替换方案配图</summary><p>先保存配图，再说明需要在哪一幕使用。更换后须按当前修订重新确认。</p><input type="file" accept="image/png,image/jpeg" @change="pickPicture" :disabled="busy||pictureBusy"><template v-if="picture"><MediaImage :picture="picture" alt="方案教学配图"/><label>说明<input v-model="pictureCaption" maxlength="60"></label><label>出处<input v-model="pictureSource" maxlength="300"></label><button type="button" @click="savePicture" :disabled="busy||pictureBusy">保存当前配图</button></template></details>
<template v-if="canRepairPlan"><label>告诉 AI 怎样调整方案<textarea v-model="instruction" rows="3" maxlength="2000" placeholder="例如：先平移矩形，再旋转直角三角形，比较面积。" :disabled="busy"></textarea></label><button class="anim-revise-button" :disabled="busy||!instruction.trim()" @click="action('revise-plan',{instruction})"><WorkspaceIcon name="edit"/>修改方案</button><p v-if="state.status!=='anim_plan'" class="anim-action-note">修改会释放当前预留并使成品确认失效。代表幕内容及依赖变化时，需重新审核。</p><details class="anim-json-editor"><summary>直接编辑结构化分镜</summary><textarea v-model="planJSON" rows="14" aria-label="结构化分镜JSON" :disabled="busy"></textarea><button @click="savePlan" :disabled="busy">校验并保存方案</button></details><div v-if="state.status==='anim_plan'" class="anim-confirm-block"><p>确认教学顺序与旁白后，先制作一幕供你试听、试看。</p><button class="primary" @click="action('confirm-plan')" :disabled="busy||available<40">确认方案，预留40积分并制作代表幕</button><p v-if="available<40" role="status">当前可用积分不足40，方案已保留。</p></div></template>
      <div v-if="state.status==='anim_representative'" class="anim-confirm-block"><label class="anim-consent"><input v-model="consent" type="checkbox" :disabled="busy"><span>我已播放当前代表幕，确认讲解、声音与动作。</span></label><button class="primary" :disabled="busy||!consent" @click="action('confirm-representative',{hash:state.representative_hash})">确认代表幕，生成其余幕</button></div>
      <div v-if="['anim_acceptance','anim_edit_acceptance'].includes(state.status)" class="anim-confirm-block"><label class="anim-consent"><input v-model="consent" type="checkbox" :disabled="busy"><span>我已完整观看当前版本并核对知识、发音、字幕和动作。</span></label><button class="primary" :disabled="busy||!consent" @click="action('accept',{hash:state.build_hash})">{{state.status==='anim_acceptance'?'验收并结算40积分':'接受修改，使用本幕免费修改次数'}}</button><button v-if="state.status==='anim_edit_acceptance'" :disabled="busy" @click="action('discard-edit')">放弃修改，保留已交付版本</button></div>
      <template v-if="state.has_delivery"><button class="primary anim-download" @click="download('html')" :disabled="busy"><WorkspaceIcon name="download"/>下载已验收 HTML</button><p class="anim-action-note">包含动画、语音和字幕，下载后可离线播放。MP4 导出沿用已验收版本，不再次扣积分。</p><template v-if="health?.animation_video_configured"><button v-if="state.export?.status==='succeeded'&&state.export.revision===state.accepted_revision" @click="download('mp4')" :disabled="busy">下载已验收 MP4</button><template v-else-if="['pending','running'].includes(state.export?.status)"><p role="status">{{state.export.status==='pending'?'视频等待导出':'正在导出视频'}}，可从“我的作品”回来下载。</p><button @click="exportAction('cancel-export')" :disabled="busy">取消视频导出</button></template><button v-else @click="exportAction('export')" :disabled="busy">{{state.export?.status==='failed'?'重试 MP4 导出':'导出已验收 MP4'}}</button><p v-if="state.export?.error" class="anim-action-note" role="status">{{state.export.error}}</p></template></template>
      <template v-if="state.status==='succeeded'"><div class="anim-edit-heading"><h3>局部修改</h3><span>每幕一次免费</span></div><label>选择分镜<select v-model="selected" :disabled="busy"><option v-for="scene in state.script.scenes" :key="scene.id" :value="scene.id" :disabled="state.free_used[scene.id]">{{scene.title}}{{state.free_used[scene.id]?'（已用）':'（一次免费）'}}</option></select></label><p v-if="selectedScene" class="anim-action-note">{{selectedScene.intent}}</p><label>修改要求<textarea v-model="instruction" rows="3" maxlength="2000" placeholder="描述这一幕需要调整的讲解或动作" :disabled="busy"></textarea></label><button :disabled="busy||!instruction.trim()||!selected||state.free_used[selected]" @click="action('edit',{scene_id:selected,instruction})">生成此幕修改预览</button><p class="anim-action-note">首期不支持拆幕、合并幕。接受修改前，下载仍为上次交付版本。</p></template>
      <template v-if="['failed','cancelled'].includes(state.status)"><p class="anim-action-note">方案与音频检查点已保留。恢复时重新检查权限和积分，保留已用调用次数。</p><button class="primary" :disabled="busy" @click="action('resume')">恢复当前阶段</button><button v-if="state.has_delivery" :disabled="busy" @click="action('discard-edit')">放弃修改</button></template>
      <p v-if="working" class="anim-action-note">制作完成后，这里会显示当前阶段的确认操作。</p>
      <button v-if="!['succeeded','cancelled','failed'].includes(state.status)" class="danger-link anim-cancel" :disabled="busy" @click="action('cancel')">取消当前制作{{state.charge_state==='reserved'?'并释放预留':''}}</button>
     </aside>
    </div>
   </template><p v-else class="anim-loading" role="status"><span class="spinner" aria-hidden="true"></span>正在读取动画状态…</p>
  </template>
 </section>
</template>
