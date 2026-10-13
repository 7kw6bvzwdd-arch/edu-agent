<script setup>
import {ref,computed,onMounted,onUnmounted,watch,nextTick} from 'vue'
import {IconChevronLeft,IconChevronRight,IconTrash,IconSearch} from '@tabler/icons-vue'
const props=defineProps({api:Function})
const emit=defineEmits(['changed'])
const rows=ref([]),total=ref(0),offset=ref(0),busy=ref(false),error=ref(''),notice=ref(''),from=ref(''),to=ref(''),selection=ref([]),allRange=ref(false),confirmDelete=ref(false)
const applied=ref({from:'',to:''}),limit=50
let previousFocus,disposed=false
const dialog=ref(null)
watch([from,to],()=>{selection.value=[];allRange.value=false})
watch(confirmDelete,async open=>{if(open){previousFocus=document.activeElement;await nextTick();dialog.value?.querySelector('button')?.focus()}else previousFocus?.focus()})
function trapDialog(event){if(event.key!=='Tab')return;const buttons=[...event.currentTarget.querySelectorAll('button')].filter(b=>!b.disabled);if(!buttons.length){event.preventDefault();return}const first=buttons[0],last=buttons.at(-1);if(event.shiftKey&&document.activeElement===first){event.preventDefault();last.focus()}else if(!event.shiftKey&&document.activeElement===last){event.preventDefault();first.focus()}}
const count=computed(()=>allRange.value?total.value:selection.value.length)
const allPage=computed(()=>rows.value.length>0&&rows.value.every(r=>selection.value.includes(r.id)))
function times(){const value={from:from.value?new Date(from.value).toISOString():'',to:to.value?new Date(to.value).toISOString():''};if(value.from&&value.to&&value.from>=value.to)throw Error('结束时间必须晚于开始时间');return value}
async function load(reset=false){if(busy.value)return;busy.value=true;error.value='';try{if(reset){applied.value=times();offset.value=0}selection.value=[];allRange.value=false;const q=new URLSearchParams({...applied.value,limit:String(limit),offset:String(offset.value)});let v=await props.api('/usage?'+q);if(disposed)return;if(!v.items.length&&offset.value){offset.value=Math.max(0,(Math.ceil(v.total/limit)-1)*limit);q.set('offset',String(offset.value));v=await props.api('/usage?'+q)}if(disposed)return;rows.value=v.items;total.value=v.total}catch(e){if(!disposed)error.value=e.message}finally{busy.value=false}}
function togglePage(event){allRange.value=false;selection.value=event.target.checked?rows.value.map(r=>r.id):[]}
function changeSelection(){allRange.value=false}
async function remove(){if(busy.value)return;busy.value=true;error.value='';try{const result=await props.api('/usage/delete','POST',allRange.value?{all:true,...applied.value}:{ids:selection.value.map(Number)});confirmDelete.value=false;notice.value='已删除 '+result.deleted+' 条调用明细，累计用量与积分账单保留。';emit('changed')}catch(e){error.value=e.message}finally{busy.value=false}if(!error.value)await load()}
const date=v=>new Date(v).toLocaleString('zh-CN',{hour12:false})
onMounted(()=>load())
onUnmounted(()=>{disposed=true})
</script>
<template>
 <section class="usage-records" aria-label="调用记录">
  <h2>调用记录 <small>{{total}} 条</small></h2>
  <form class="usage-filters" @submit.prevent="load(true)">
   <label>开始时间<input type="datetime-local" v-model="from" aria-label="调用开始时间" :disabled="busy"></label>
   <label>结束时间（不含）<input type="datetime-local" v-model="to" aria-label="调用结束时间" :disabled="busy"></label>
   <button class="primary" :disabled="busy"><IconSearch :size="17"/>查询</button>
   <button type="button" :disabled="busy" @click="from='';to='';load(true)">重置</button>
  </form>
  <div class="selection-bar">
   <label class="inline-check"><input type="checkbox" :checked="allPage||allRange" :indeterminate="!allPage&&!allRange&&selection.length>0" @change="togglePage" :disabled="busy||!rows.length" aria-label="全选本页调用记录">全选本页</label>
   <button v-if="applied.from&&applied.to&&total>rows.length" :disabled="busy||!total" @click="allRange=true;selection=rows.map(r=>r.id)">选择该时间范围全部 {{total}} 条</button>
   <span>已选 {{count}} 条</span>
   <button class="danger-link" :disabled="busy||!count" @click="confirmDelete=true"><IconTrash :size="17"/>删除所选</button>
  </div>
  <p v-if="error" class="alert error" role="alert">{{error}}</p><p v-if="notice" class="alert" role="status">{{notice}}</p>
  <div class="table-scroll"><table><thead><tr><th>选择</th><th>时间</th><th>操作</th><th>模型</th><th>输入 / 输出Token</th><th>耗时</th><th>结果</th></tr></thead><tbody><tr v-for="l in rows" :key="l.id" :data-usage-id="l.id"><td><input type="checkbox" v-model="selection" :value="l.id" @change="changeSelection" :disabled="busy" :aria-label="'选择调用 '+l.id"></td><td>{{date(l.created_at)}}</td><td>{{l.operation}}</td><td class="usage-model">{{l.model}}</td><td>{{l.input_tokens}} / {{l.output_tokens}}</td><td>{{l.latency_ms}}ms</td><td>{{l.success?'成功':'失败'}}</td></tr></tbody></table></div>
  <p v-if="!rows.length&&!busy" class="empty-small">{{applied.from||applied.to?'该时间范围没有调用记录。':'暂无保留的调用明细。'}}</p>
  <div class="works-pagination"><button :disabled="busy||!offset" @click="offset-=limit;load()" aria-label="上一页调用记录" title="上一页"><IconChevronLeft :size="19"/></button><span>第 {{offset/limit+1}} / {{Math.max(1,Math.ceil(total/limit))}} 页</span><button :disabled="busy||offset+limit>=total" @click="offset+=limit;load()" aria-label="下一页调用记录" title="下一页"><IconChevronRight :size="19"/></button></div>
  <div v-if="confirmDelete" class="modal-backdrop" @click.self="!busy&&(confirmDelete=false)"><section ref="dialog" class="work-dialog" role="dialog" aria-modal="true" aria-label="删除调用记录" @keydown="trapDialog" @keydown.esc="!busy&&(confirmDelete=false)"><h2>删除 {{count}} 条调用明细？</h2><p>{{allRange?'所选时间范围内所有页的调用明细':'当前页勾选的调用明细'}}将被删除，累计用量和积分账单保留。</p><p v-if="allRange">{{date(applied.from)}} 至 {{date(applied.to)}}（不含结束时间）</p><div class="action-row"><button :disabled="busy" @click="confirmDelete=false">取消</button><button class="danger" :disabled="busy" @click="remove"><IconTrash :size="17"/>确认删除</button></div></section></div>
 </section>
</template>
<style scoped>
.usage-filters,.selection-bar{display:flex;align-items:center;flex-wrap:wrap;gap:12px;margin:16px 0}.usage-filters{align-items:flex-end}.usage-filters label{flex:1 1 210px;min-width:0}.selection-bar span{font-size:12px;color:var(--muted)}.usage-model{max-width:230px;overflow-wrap:anywhere}.inline-check{display:inline-flex;align-items:center;gap:8px}.inline-check input{margin:0}.work-dialog p{margin:14px 0}@media(max-width:650px){.usage-filters label{flex-basis:100%}.selection-bar button{white-space:normal}}
</style>
