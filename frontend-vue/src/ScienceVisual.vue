<script setup>
import { inject, ref, watch, onUnmounted } from 'vue'
const props=defineProps({diagram:Object,endpoint:{type:String,default:'/api/courseware/scientific'},request:Object})
const token=inject('mediaToken',ref('')),image=ref(''),summary=ref(''),error=ref(''),metadata=ref(null)
let generation=0,timer,controller
watch(()=>[JSON.stringify(props.diagram),props.endpoint,JSON.stringify(props.request),token.value],()=>{
 const current=++generation
 clearTimeout(timer);controller?.abort();image.value='';error.value='';metadata.value=null;summary.value=''
 timer=setTimeout(async()=>{
  if(!token.value){error.value='请登录后查看科学图示';return}
  controller=new AbortController()
  try{
   const response=await fetch(props.endpoint,{method:'POST',headers:{'Content-Type':'application/json',Authorization:'Bearer '+token.value},body:JSON.stringify(props.request||{diagram:props.diagram}),signal:controller.signal})
   const result=await response.json()
   if(!response.ok)throw Error(result.message||'科学图示暂时无法绘制，请核对参数')
   if(current!==generation)return
   image.value=result.animation||result.image;summary.value=result.summary;metadata.value=result.metadata
  }catch(e){if(current===generation&&e.name!=='AbortError')error.value=e.message}
 },300)
},{immediate:true})
onUnmounted(()=>{generation++;clearTimeout(timer);controller?.abort()})
</script>
<template>
 <div class="science-visual">
  <img v-if="image" :src="image" :alt="diagram.title" :data-science-kind="diagram.kind">
  <p v-else role="status">{{error||'正在绘制科学图示…'}}</p>
  <p v-if="summary">{{summary}}</p>
  <p v-if="metadata&&diagram.kind==='thin_lens'">{{metadata.image_distance_cm===null?'不成有限远像':`像距 ${Number(metadata.image_distance_cm.toPrecision(4))} cm · 放大率 ${Number(metadata.magnification.toPrecision(4))}`}}</p>
 </div>
</template>
<style scoped>.science-visual img{display:block;width:100%;height:auto}.science-visual p{font-size:12px;line-height:1.6}</style>
