<script setup>
import {inject,ref,watch,onUnmounted} from 'vue'
const props=defineProps({picture:Object,alt:String})
const token=inject('mediaToken',ref('')),src=ref(''),error=ref('')
let version=0,url=''
function clear(){if(url)URL.revokeObjectURL(url);url='';src.value=''}
watch(()=>[props.picture?.asset_id,props.picture?.data_url,token.value],async()=>{
 const current=++version;clear();error.value=''
 if(!props.picture?.asset_id){src.value=props.picture?.data_url||'';return}
 try{
  const response=await fetch('/api/media/'+encodeURIComponent(props.picture.asset_id),{headers:{Authorization:'Bearer '+token.value}})
  if(!response.ok)throw Error('图片暂时无法加载')
  const blob=await response.blob();if(current!==version)return
  url=URL.createObjectURL(blob);src.value=url
 }catch(e){if(current===version)error.value=e.message}
},{immediate:true})
onUnmounted(()=>{version++;clear()})
</script>
<template><img v-if="src" :src="src" :alt="alt||picture?.caption||'教师配图'"><span v-else role="status">{{error||'图片加载中…'}}</span></template>
