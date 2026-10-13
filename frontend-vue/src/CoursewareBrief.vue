<script setup>
import {ref,computed,watch,nextTick} from 'vue'
const props=defineProps({form:Object,api:Function,mode:String,disabled:Boolean,ready:Boolean,qualityReady:{type:Boolean,default:true},requirements:String})
const emit=defineEmits(['update:requirements','confirmed'])
const text=ref(''),messages=ref([]),busy=ref(false),error=ref(''),summary=ref(''),accepted=ref(false),chat=ref(null)
const context=computed(()=>`课题：${props.form.topic}\n对象：${props.form.grade} · ${props.form.subject} · ${props.form.textbook}\n内容详略：${({concise:'精简',default:'默认（约15页参考）',detailed:'详细'})[props.form.detail]||'默认'}，页数按教学需要安排\n课时：${props.form.lesson_minutes||45}分钟\n学生基础：${props.form.student_level||'按当前年级常规基础'}\n参考用途：${({content:'依据资料内容',sequence:'参考教学顺序',style:'参考视觉风格'})[props.form.reference_mode]||'依据资料内容'}\n教学目标：${props.form.objectives||'围绕核心概念、典型例题与课堂练习'}\n教材章节：${props.form.chapter||'未指定'}${props.form.material_id?` · 文件第${props.form.material_start}–${props.form.material_end}页`: ''}\n参考课件：${props.form.reference_id?'已选择画面分析记录':'未选择'}\n资料：${[props.form.use_kb?'企业知识库':'',props.form.use_web?'联网检索':''].filter(Boolean).join(' + ')}`)
watch(context,(value,previous)=>{
 if(!summary.value)summary.value=value+'\n教师补充：\n以核心概念讲解、必要推导、例题和练习为主；教师可在此修改。'
 else if(previous&&summary.value.startsWith(previous))summary.value=value+summary.value.slice(previous.length)
},{immediate:true})
watch(()=>props.form.topic,()=>{messages.value=[]})
watch(()=>props.disabled,value=>{if(!value)accepted.value=false})
watch(summary,()=>{accepted.value=false;emit('update:requirements','')})
async function send(){if(busy.value||props.disabled||!text.value.trim())return;busy.value=true;error.value='';const content=text.value.trim();try{
 const snapshot=JSON.stringify(props.form);const recent=[...messages.value,{role:'user',content}].slice(-11);const v=await props.api('/courseware/brief','POST',{input:{...props.form,type:'courseware'},messages:recent});if(snapshot!==JSON.stringify(props.form))throw Error('教学参数已改变，本次回复未应用；请重新发送需求。');messages.value=[...messages.value,{role:'user',content},{role:'assistant',content:v.reply}];summary.value+='\n'+content;text.value='';await nextTick();chat.value?.scrollTo({top:chat.value.scrollHeight,behavior:'smooth'})
 }catch(e){error.value=e.message}finally{busy.value=false}}
async function confirm(){if(busy.value||props.disabled||!props.ready)return;if(text.value.trim()){summary.value+='\n'+text.value.trim();text.value='';await nextTick()}if(!summary.value.trim())return;if(summary.value.length>8000){error.value='需求摘要不能超过8000字，请精简后确认。';return}accepted.value=true;emit('update:requirements',summary.value.trim());emit('confirmed')}
</script>
<template>
 <section class="brief-panel" aria-label="生成前需求对话">
  <div class="brief-heading"><span class="eyebrow">UNDERSTAND BEFORE CREATING</span><h2>先聊清楚，再做三页质量预览。</h2><p>补充教学需求，确认摘要后才开始检索与预览。这里不会创建作品或预留积分。</p></div>
  <div class="brief-grid">
   <div class="brief-conversation">
    <div class="brief-messages" ref="chat" role="log" aria-label="需求对话记录" aria-live="polite"><div class="brief-bubble assistant"><b>课件助手</b><p>这节《{{form.topic}}》准备讲多久？学生有哪些基础？你更希望突出概念讲解、实验探究，还是例题练习？也可以告诉我你想要的视觉风格。</p></div><div v-for="(m,i) in messages" :key="i" :class="['brief-bubble',m.role]"><b>{{m.role==='user'?'我的要求':'课件助手'}}</b><p>{{m.content}}</p></div></div>
    <form @submit.prevent="send" class="brief-composer"><label>补充你的需求<textarea v-model="text" rows="3" maxlength="1200" :disabled="busy||disabled" placeholder="例如：45分钟，学生基础偏弱；先做生活情境导入，重点讲实验过程，安排3道由浅入深的练习。" aria-label="生成前需求输入"></textarea></label><div class="brief-actions"><small>{{mode==='sample'?'本地需求引导，不调用模型':'对话调用课件文本模型；不扣业务积分'}}<br>也可直接修改右侧摘要</small><button :disabled="busy||disabled||!text.trim()" class="primary">{{busy?'正在理解…':'发送 ↑'}}</button></div><p v-if="error" class="alert error" role="alert">{{error}}</p></form>
   </div>
   <div class="brief-summary"><h3>将用于生成的需求摘要</h3><label>可编辑摘要<textarea v-model="summary" rows="12" maxlength="8000" :disabled="busy||disabled" aria-label="课件需求摘要"></textarea></label><p class="fine">确认后生成三页质量预览。预览阶段仍可逐页修改、点赞点踩和审核；不会直接生成整份课件。</p><button class="primary wide" @click="confirm" :disabled="disabled||busy||!ready||!summary.trim()">{{accepted?'需求已确认，开始生成…':'确认需求，生成三页质量预览 →'}}</button><p v-if="!qualityReady" class="alert" role="status">内容与真实画面审核尚未配置完成。需求可先保存；联系管理员准备好审核后开始制作，不会预留积分。</p><p v-else-if="!ready" class="fine">请先填写课题、选择资料来源并开通基础套餐，确保至少20可用积分。</p></div>
  </div>
 </section>
</template>
<style scoped>
.brief-heading{padding:26px 28px 18px;border-bottom:1px solid var(--line)}.brief-heading h2{font-size:22px;margin:10px 0}.brief-heading p{color:var(--muted);font-size:12px}.brief-grid{display:grid;grid-template-columns:minmax(0,1.1fr) minmax(0,1fr);gap:0;min-height:470px}.brief-conversation{display:flex;flex-direction:column;min-width:0;border-right:1px solid var(--line)}.brief-messages{padding:22px;max-height:340px;min-height:220px;overflow-y:auto;overscroll-behavior:contain}.brief-bubble{padding:14px 16px;border-radius:12px;background:#f3f6ee;margin:0 0 12px;max-width:96%;font-size:12px;line-height:1.8}.brief-bubble b{font-size:10px;color:#70886e}.brief-bubble p{white-space:pre-wrap;overflow-wrap:anywhere}.brief-bubble.user{margin-left:20px;background:#e4eee0}.brief-composer{padding:18px 22px;border-top:1px solid var(--line);margin-top:auto}.brief-actions{display:flex;align-items:center;justify-content:space-between;gap:10px;margin-top:12px}.brief-summary{padding:22px;background:#fafbf7}.brief-summary h3{margin:0 0 18px}.brief-summary textarea{min-height:285px;line-height:1.8;font-size:12px}.brief-summary button{margin-top:18px}@media(max-width:1050px){.brief-grid{grid-template-columns:1fr}.brief-conversation{border-right:0}.brief-messages{max-height:290px;min-height:150px}.brief-summary textarea{min-height:210px}.brief-summary{border-top:1px solid var(--line)}}@media(max-width:650px){.brief-heading,.brief-summary,.brief-messages,.brief-composer{padding:18px}}
</style>
