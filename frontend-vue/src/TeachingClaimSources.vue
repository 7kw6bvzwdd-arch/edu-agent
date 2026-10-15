<script setup>
import {computed} from 'vue'
const props=defineProps({slide:Object,sources:Array,editable:Boolean})
const entries=computed(()=>(props.slide?.evidence||[]).map(e=>({e,source:props.sources?.find(s=>s.id===e.source_id)})))
function remove(index){
 if(!props.editable)return
 const e=props.slide.evidence[index];props.slide.evidence.splice(index,1)
 if(!props.slide.evidence.some(v=>v.source_id===e.source_id)){
  const marker='['+e.source_id+']'
  function clear(value){if(Array.isArray(value)){for(let i=0;i<value.length;i++)value[i]=typeof value[i]==='string'?value[i].replaceAll(marker,''):(clear(value[i]),value[i])}else if(value&&typeof value==='object'){for(const key of Object.keys(value)){if(['evidence','data_url','id'].includes(key))continue;if(typeof value[key]==='string')value[key]=value[key].replaceAll(marker,'');else clear(value[key])}}}
  clear(props.slide)
 }
 if(!props.slide.notes?.includes('移除的原文关联仍待核验'))props.slide.notes=(props.slide.notes||'')+'\n本页移除的原文关联仍待核验。'
}
</script>
<template>
 <details class="claim-sources"><summary>本页结论与原文 · {{entries.length}} 项关联</summary>
  <p class="fine">原文摘录可追溯，结论是否成立仍由文字审核与教师核对。未关联内容仍待核验。</p>
  <p v-if="!entries.length">本页尚未保存具体结论的原文关联；资料列表不等于每句话都有依据。</p>
  <article v-for="({e,source},i) in entries" :key="i">
   <b>{{e.claim}}</b>
   <template v-if="source"><p>[{{source.id}}] {{source.textbook?.name||source.title}} · {{source.location}}</p><p v-if="source.textbook">{{source.textbook.edition||'版次未标注'}} · {{source.textbook.chapter||'章节未标注'}}（教师标注）</p><p v-if="source.file_page">文件第 {{source.file_page}} 页，可能与书中印刷页码不同。</p><blockquote>{{e.quote}}</blockquote><p v-if="e.source_hash!==source.hash" class="error">原文快照不匹配，请移除旧关联后重新核对。</p><details><summary>查看保存的上下文</summary><p class="original">{{source.content}}</p></details></template>
   <p v-else class="error">该来源不在当前作品保存的资料中。</p>
   <button v-if="editable" type="button" @click="remove(i)">移除本项关联，作为待核验补充</button>
  </article>
 </details>
</template>
<style scoped>
.claim-sources{margin:12px 0;padding:12px;border:1px solid var(--line);border-radius:8px;font-size:13px;line-height:1.7}.claim-sources summary{cursor:pointer}.claim-sources article{padding:12px 0;border-top:1px solid var(--line);margin-top:12px}.claim-sources p{margin:6px 0}.claim-sources blockquote{margin:10px 0;padding:8px 12px;border-left:3px solid var(--line);white-space:pre-wrap}.original{white-space:pre-wrap;max-height:260px;overflow:auto}
</style>
