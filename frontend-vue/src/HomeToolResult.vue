<script setup>
import {ref,onMounted,onUnmounted,watch} from 'vue'
import CoursewareStudio from './CoursewareStudio.vue'
import AnimationStudio from './AnimationStudio.vue'
import WorkspaceIcon from './WorkspaceIcon.vue'
const props=defineProps({execution:Object,api:Function,token:String,health:Object,available:Number,canAnimation:Boolean})
const emit=defineEmits(['changed','animation','new-work'])
const task=ref(null),error=ref(''),busy=ref(false),process=ref(false)
let timer,version=0,stopped=false
const active=['pending','researching','confirmed','rendering','expand_pending','expanding','anim_plan_pending','anim_plan_running','anim_rep_pending','anim_rep_running','anim_full_pending','anim_full_running','anim_edit_pending','anim_edit_running']
async function refresh(){
 const generation=version,id=props.execution.taskId
 if(!id)return
 const value=await props.api('/tasks/'+id)
 if(stopped||generation!==version)return
 if(task.value?.id===id&&value.revision<task.value.revision)return
 const changed=task.value?.status!==value.status
 task.value=value
 if(changed)emit('changed')
}
async function create(){
 if(busy.value)return
 busy.value=true;error.value=''
 try{
  if(!props.execution.taskId){const result=await props.api('/tasks','POST',{...props.execution.input,idempotency_key:props.execution.key});props.execution.taskId=result.id;if(stopped)return;emit('changed')}
  await refresh()
 }catch(e){if(!stopped)error.value=e.message}finally{busy.value=false}
}
async function retry(){if(busy.value)return;busy.value=true;error.value='';try{const value=await props.api('/tasks/'+task.value.id+'/retry','POST',{});props.execution.taskId=value.id;await refresh()}catch(e){error.value=e.message}finally{busy.value=false}}
watch(()=>props.token,()=>{version++;task.value=null;refresh().catch(e=>{error.value=e.message})})
onMounted(()=>{void create();timer=setInterval(async()=>{if(stopped||busy.value||document.hidden||!task.value||!active.includes(task.value.status))return;busy.value=true;try{await refresh()}catch(e){error.value=e.message}finally{busy.value=false}},2000)})
onUnmounted(()=>{stopped=true;version++;clearInterval(timer)})
</script>
<template>
 <section class="home-tool-result" :data-result-tool="execution.input.type" aria-label="对话中的工具结果">
  <header class="home-result-heading"><span><WorkspaceIcon :name="execution.input.type==='animation'?'animation':'slides'"/>{{execution.input.type==='animation'?'教学动画':'课件制作'}}</span><small>{{execution.input.topic}}</small></header>
  <div v-if="error" class="alert error" role="alert">{{error}}<button v-if="!task" @click="create" :disabled="busy">重试</button></div>
  <p v-if="!task&&!error" class="home-tool-pending" role="status"><span class="spinner" aria-hidden="true"></span>正在启动{{execution.input.type==='animation'?'分镜方案':'三页课件预览'}}…</p>
  <CoursewareStudio v-if="task?.type==='courseware'" :task="task" :api="api" :token="token" :available="available" :retrying="busy" :refresh="refresh" :canvas-available="['regions-v2','canvas-v3'].includes(health.courseware_canvas_version)" :canvas-complete="health.courseware_canvas_version==='canvas-v3'" @retry="retry" @process="process=!process" @new-work="$emit('new-work')" @animation="slide=>$emit('animation',{task,slide})"/>
  <AnimationStudio v-else-if="task?.type==='animation'" :task="task" :api="api" :token="token" :health="health" :available="available" :can-create="canAnimation" :refresh="refresh" @new-work="$emit('new-work')"/>
  <details v-if="process&&task" open class="home-result-process"><summary>制作过程</summary><p v-for="(event,index) in task.events||[]" :key="index">{{event.message||event.detail||event.stage}}</p></details>
 </section>
</template>
