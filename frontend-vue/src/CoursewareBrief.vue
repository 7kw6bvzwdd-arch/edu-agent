<script setup>
import {ref,computed,watch,nextTick,onUnmounted} from 'vue'
const props=defineProps({form:Object,api:Function,mode:String,disabled:Boolean,ready:Boolean,qualityReady:{type:Boolean,default:true},requirements:String})
const emit=defineEmits(['update:requirements','confirmed'])
const text=ref(''),messages=ref([]),busy=ref(false),error=ref(''),summary=ref(''),accepted=ref(false),chat=ref(null)
let controller
onUnmounted(()=>controller?.abort())
// The server's display labels map only to the settings this conversation may edit.
const rowFields={'课题':'topic','学科':'subject','年级':'grade','课时（分钟）':'lesson_minutes','教材版本':'textbook','学生基础':'student_level','教学目标':'objectives','内容详略':'detail'}
const fieldLimits={topic:100,textbook:100,student_level:400,objectives:1000}
const rowOptions={subject:[['数学','数学'],['物理','物理']],grade:[['七年级','七年级'],['八年级','八年级'],['九年级','九年级']],detail:[['concise','精简'],['default','适中'],['detailed','详细']]}
const editableMessage=computed(()=>messages.value.findLastIndex(m=>m.role==='assistant'&&!m.pending&&!m.failed&&m.blocks?.length))
function fieldError(field){
 const value=props.form[field]
 if(field==='lesson_minutes'&&(!Number.isInteger(value)||value<10||value>180))return '课时请输入10–180之间的整数。'
 if(fieldLimits[field]&&[...String(value||'')].length>fieldLimits[field])return `此项最多${fieldLimits[field]}字。`
 if(rowOptions[field]&&!rowOptions[field].some(([key])=>key===value))return '请选择有效的教学设置。'
 return ''
}
function editRow(row,value){
 const field=rowFields[row.label]
 if(!field||busy.value||props.disabled)return
 props.form[field]=field==='lesson_minutes'?(value===''?'':Number(value)):value
 row.value=rowOptions[field]?.find(([key])=>key===value)?.[1]??String(value)
 accepted.value=false;error.value='';emit('update:requirements','')
}
function validateSettings(){
 error.value=Object.values(rowFields).map(fieldError).find(Boolean)||''
 return !error.value
}
const context=computed(()=>`课题：${props.form.topic}\n对象：${props.form.grade} · ${props.form.subject} · ${props.form.textbook}\n内容详略：${({concise:'精简',default:'默认（约15页参考）',detailed:'详细'})[props.form.detail]||'默认'}，页数按教学需要安排\n课时：${props.form.lesson_minutes||45}分钟\n学生基础：${props.form.student_level||'按当前年级常规基础'}\n参考用途：${({content:'依据资料内容',sequence:'参考教学顺序',style:'参考视觉风格'})[props.form.reference_mode]||'依据资料内容'}\n教学目标：${props.form.objectives||'围绕核心概念、典型例题与课堂练习'}\n教材章节：${props.form.chapter||'未指定'}${props.form.material_id?` · 文件第${props.form.material_start}–${props.form.material_end}页`: ''}\n参考课件：${props.form.reference_id?'已选择画面分析记录':'未选择'}\n资料：${[props.form.use_kb?'企业知识库':'',props.form.use_web?'联网检索':''].filter(Boolean).join(' + ')}`)
watch(context,(value,previous)=>{
 if(!summary.value)summary.value=value+'\n教师补充：\n以核心概念讲解、必要推导、例题和练习为主；教师可在此修改。'
 else if(previous&&summary.value.startsWith(previous))summary.value=value+summary.value.slice(previous.length)
},{immediate:true})
const requirementsText=computed(()=>summary.value.startsWith(context.value)?summary.value:context.value+'\n教师补充：\n'+summary.value)
watch(()=>props.disabled,value=>{if(!value)accepted.value=false})
watch(summary,()=>{accepted.value=false;emit('update:requirements','')})
async function scrollChat(){await nextTick();chat.value?.scrollTo({top:chat.value.scrollHeight,behavior:'instant'})}
async function send(intent='discuss'){
 if(busy.value||props.disabled||!text.value.trim())return false
 if(!validateSettings())return false
 const content=text.value.trim(),snapshot=JSON.stringify(props.form)
 const recent=[...messages.value.filter(m=>!m.failed).map(({role,content})=>({role,content})),{role:'user',content}].slice(-11)
 const user={role:'user',content},assistant={role:'assistant',content:'',blocks:[],pending:true}
 messages.value.push(user,assistant);const current=messages.value.at(-1),currentUser=messages.value.at(-2)
 busy.value=true;error.value='';controller=new AbortController();await scrollChat()
 try{
  const v=await props.api('/courseware/brief','POST',{stream:true,intent,input:{...props.form,type:'courseware',requirements:requirementsText.value},messages:recent},{signal:controller.signal,onEvent:(event,value)=>{
   if(event==='delta')current.content+=value.text||''
   if(event==='block')current.blocks.push(value)
   void scrollChat()
  }})
  if(controller.signal.aborted)throw new DOMException('Stopped','AbortError')
  if(snapshot!==JSON.stringify(props.form))throw Error('教学设置已改变，请重新发送；本次需求保留。')
  // Only the completed, validated reply can change teaching settings.
  const fields=['topic','subject','grade','textbook','lesson_minutes','student_level','objectives','detail']
  for(const key of fields)if(v.settings?.[key]!==undefined)props.form[key]=v.settings[key]
  current.content=v.reply;current.blocks=v.blocks||current.blocks;current.mode=v.mode;current.pending=false
  await nextTick();summary.value+='\n'+content;text.value='';await scrollChat();return true
 }catch(e){
  current.failed=true;current.pending=false;currentUser.failed=true
  error.value=e.name==='AbortError'?'已停止回复，你的需求已保留，可以继续修改或重新发送。':e.message
  return false
 }finally{busy.value=false;controller=null}
}
async function confirm(){
 if(busy.value||props.disabled)return
 if(!validateSettings())return
 if(!props.ready){error.value='请先开通基础套餐并确保至少20可用积分，或检查资料来源与服务配置。';return}
 if(text.value.trim()&&!await send('generate'))return
 if(!props.form.topic.trim()){error.value='先告诉我课题，例如“制作九年级二次函数课件”。';await nextTick();document.querySelector('[aria-label="生成前需求输入"]')?.focus();return}
 await nextTick()
 if([...requirementsText.value].length>8000){error.value='需求摘要不能超过8000字，请精简后制作。';return}
 accepted.value=true;emit('update:requirements',requirementsText.value.trim());emit('confirmed')
}
</script>
<template>
 <section :class="['brief-panel',{'has-messages':messages.length}]" aria-label="生成前需求对话">
  <div class="brief-heading"><h2>说说你想做的课件</h2><p>先聊一聊，也可以采用默认设置直接制作。</p></div>
  <div class="brief-grid">
   <div class="brief-conversation">
    <div class="brief-messages" ref="chat" role="log" aria-label="需求对话记录" aria-live="polite" :aria-busy="busy">
     <div v-if="!messages.length" class="brief-welcome"><p>从一个课题开始，也可以告诉我要画的函数或实验数据图。</p></div>
     <article v-for="(m,i) in messages" :key="i" :class="['brief-bubble',m.role,{incomplete:m.failed}]">
      <b>{{m.role==='user'?'我的要求':'课件助手'}}<span v-if="m.role==='assistant'&&m.mode==='sample'"> · 本地示例</span></b>
      <p v-if="m.content">{{m.content}}<span v-if="m.pending" class="stream-cursor" aria-hidden="true">▍</span></p><p v-else-if="m.pending" class="brief-thinking" role="status">正在理解你的需求…</p>
      <section v-for="(block,j) in m.blocks" :key="j" class="brief-requirement-block" :aria-label="block.title">
       <h3>{{block.title}}<small v-if="i===editableMessage">点击内容即可修改</small></h3>
       <table><caption class="sr-only">{{block.title}}</caption><colgroup><col class="brief-row-label"><col></colgroup><tbody>
        <tr v-for="(row,k) in block.rows" :key="k"><th scope="row">{{row.label}}</th><td>
         <template v-if="i===editableMessage&&rowFields[row.label]">
          <select v-if="rowOptions[rowFields[row.label]]" :value="form[rowFields[row.label]]" :aria-label="'修改'+row.label" :disabled="busy||disabled" @change="editRow(row,$event.target.value)"><option v-for="[value,label] in rowOptions[rowFields[row.label]]" :key="value" :value="value">{{label}}</option></select>
          <textarea v-else-if="['student_level','objectives'].includes(rowFields[row.label])" :value="form[rowFields[row.label]]" :maxlength="fieldLimits[rowFields[row.label]]" :aria-label="'修改'+row.label" :disabled="busy||disabled" rows="2" placeholder="未指定，采用当前年级常规安排" @input="editRow(row,$event.target.value)"></textarea>
          <input v-else :type="rowFields[row.label]==='lesson_minutes'?'number':'text'" :value="form[rowFields[row.label]]" :maxlength="fieldLimits[rowFields[row.label]]" :min="rowFields[row.label]==='lesson_minutes'?10:undefined" :max="rowFields[row.label]==='lesson_minutes'?180:undefined" :step="rowFields[row.label]==='lesson_minutes'?1:undefined" :aria-label="'修改'+row.label" :aria-invalid="!!fieldError(rowFields[row.label])" :disabled="busy||disabled" @input="editRow(row,$event.target.value)">
          <small v-if="fieldError(rowFields[row.label])" class="brief-field-error">{{fieldError(rowFields[row.label])}}</small>
         </template><template v-else>{{row.value}}</template>
        </td></tr>
       </tbody></table>
      </section>
      <p v-if="m.failed&&m.role==='assistant'" class="fine">本次回复未完成，尚未应用到制作需求。</p>
      <div v-if="i===editableMessage" class="brief-block-actions"><small>修改立即用于后续对话和制作，也可以继续补充。</small><button type="button" @click="confirm" :disabled="busy||disabled||!ready" class="primary">按这个需求制作</button></div>
     </article>
    </div>
    <form @submit.prevent="send('discuss')" class="brief-composer"><label>提出你的需求<textarea v-model="text" rows="3" maxlength="1200" :disabled="busy||disabled" placeholder="告诉我想做哪一课，例如：九年级二次函数，45分钟，先讲概念，再安排练习。" aria-label="生成前需求输入" @keydown.ctrl.enter.prevent="send('discuss')"></textarea></label><div class="brief-actions"><small>{{mode==='sample'?'本地需求引导，不调用模型':'沟通需求不扣制作积分'}}<br>直接制作将预留20积分，先生成三页供你审核。</small><div class="brief-action-buttons"><button v-if="busy" type="button" @click="controller?.abort()">停止回复</button><template v-else><button type="button" class="brief-direct" @click="confirm" :disabled="disabled||!ready||(!text.trim()&&!form.topic.trim())">{{accepted?'正在开始制作…':'直接制作三页预览'}}</button><button type="submit" :disabled="disabled||!text.trim()" class="primary">发送 ↑</button></template></div></div><p v-if="error" class="alert error" role="alert">{{error}}</p></form>
   </div>
   <div class="brief-summary"><slot name="parameters" :busy="busy"/><details class="brief-summary-editor"><summary>查看或手动修改需求摘要</summary><label>可编辑摘要<textarea v-model="summary" rows="12" maxlength="8000" :disabled="busy||disabled" aria-label="课件需求摘要"></textarea></label></details><p v-if="!qualityReady" class="alert" role="status">真实画面审核尚未配置完成，请联系管理员准备好后制作。</p><p v-else-if="!ready" class="fine">制作需开通基础套餐、至少20可用积分，并保留至少一种资料来源。可以先继续聊需求。</p></div>
  </div>
 </section>
</template>
<style scoped>
.brief-heading{padding:26px 28px 18px;border-bottom:1px solid var(--line)}.brief-heading h2{font-size:22px;margin:10px 0}.brief-heading p{color:var(--muted);font-size:12px}.brief-grid{display:flex;flex-direction:column;min-height:470px}.brief-conversation{display:flex;flex-direction:column;min-width:0;border-right:0}.brief-messages{padding:22px;max-height:340px;min-height:220px;overflow-y:auto;overscroll-behavior:contain}.brief-bubble{padding:14px 16px;border-radius:12px;background:#f3f6ee;margin:0 0 12px;max-width:96%;font-size:12px;line-height:1.8}.brief-bubble b{font-size:10px;color:#70886e}.brief-bubble p{white-space:pre-wrap;overflow-wrap:anywhere}.brief-bubble.user{margin-left:20px;background:#e4eee0}.brief-composer{padding:18px 22px;border-top:1px solid var(--line);margin-top:auto}.brief-actions{display:flex;align-items:center;justify-content:space-between;gap:10px;margin-top:12px}.brief-summary{padding:22px;background:#fafbf7;border-top:1px solid var(--line)}.brief-summary summary{cursor:pointer;font-weight:600;margin-bottom:12px}.brief-summary h3{margin:0 0 18px}.brief-summary textarea{min-height:285px;line-height:1.8;font-size:12px}.brief-summary button{margin-top:18px}@media(max-width:1050px){.brief-grid{grid-template-columns:1fr}.brief-conversation{border-right:0}.brief-messages{max-height:290px;min-height:150px}.brief-summary textarea{min-height:210px}.brief-summary{border-top:1px solid var(--line)}}@media(max-width:650px){.brief-heading,.brief-summary,.brief-messages,.brief-composer{padding:18px}}
</style>
