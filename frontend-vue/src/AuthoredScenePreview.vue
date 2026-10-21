<script setup>
import {ref,watch,onUnmounted} from 'vue'
const props=defineProps({endpoint:String,revision:Number,sceneId:String,token:String})
const url=ref(''),busy=ref(false),error=ref('')
let controller,version=0
function reset(){version++;controller?.abort();if(url.value)URL.revokeObjectURL(url.value);url.value='';error.value='';busy.value=false}
watch(()=>[props.endpoint,props.revision,props.sceneId,props.token],reset)
onUnmounted(reset)
async function load(){
 reset();const current=version;controller=new AbortController();busy.value=true
 try{
  const r=await fetch(props.endpoint,{method:'POST',headers:{'Content-Type':'application/json',Authorization:'Bearer '+props.token},body:JSON.stringify({revision:props.revision,scene_id:props.sceneId}),signal:controller.signal})
  const data=await r.json();if(!r.ok)throw Error(data.message||'画面预览暂不可用')
  if(!data.html)throw Error('请更新后端以支持直接生成画面')
  if(current===version)url.value=URL.createObjectURL(new Blob([data.html],{type:'text/html'}))
 }catch(e){if(current===version&&e.name!=='AbortError')error.value=e.message}
 finally{if(current===version)busy.value=false}
}
</script>
<template>
 <div class="authored-scene-preview">
  <button type="button" :disabled="busy" @click="load">{{busy?'正在检查画面…':url?'重新预览这一幕':'预览这一幕画面'}}</button>
  <p v-if="error" role="alert">{{error}}</p>
  <iframe v-if="url" :src="url" sandbox="allow-scripts" title="分镜源码预览，无真实语音"></iframe>
  <p v-if="url">源码画面预览 · 无真实语音，时长为估算；正式审核以代表幕为准。</p>
 </div>
</template>
<style scoped>.authored-scene-preview iframe{width:100%;height:440px;border:0}.authored-scene-preview p{font-size:13px;line-height:1.6}</style>
