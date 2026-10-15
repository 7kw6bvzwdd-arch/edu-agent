<script setup>
import {computed,ref,nextTick} from 'vue'
import WorkspaceIcon from './WorkspaceIcon.vue'

const props=defineProps({tasks:Array,available:Number,canCreate:Boolean,statusText:Function,loading:Boolean,toolsOnly:Boolean})
const emit=defineEmits(['start','navigate','open'])
const home=ref(null)
async function prepare(materials=false){
 if(materials){const settings=home.value?.querySelector('.brief-parameters');if(settings)settings.open=true}
 await nextTick()
 const target=home.value?.querySelector(materials?'.brief-parameters':'.brief-composer textarea')
 target?.scrollIntoView({block:'nearest',behavior:matchMedia('(prefers-reduced-motion: reduce)').matches?'instant':'smooth'})
 if(!materials)target?.focus({preventScroll:true})
}
const tools=[
 {id:'courseware',title:'课件工作台',description:'确认需求，审核三页预览，再完成课堂课件。',icon:'slides',note:'20积分 · 成功交付后结算'},
 {id:'animation',title:'教学动画',description:'新引擎重构中，暂不开放生成。历史作品保留。',icon:'animation',note:'暂时停用'},
]
const recent=computed(()=>[...(props.tasks||[])].filter(t=>t.type==='courseware'||t.type==='animation').sort((a,b)=>(Date.parse(b.created_at)||0)-(Date.parse(a.created_at)||0)).slice(0,3))
const shortDate=value=>Number.isNaN(Date.parse(value))?'':new Date(value).toLocaleDateString('zh-CN',{month:'short',day:'numeric'})
</script>

<template>
 <div v-if="!toolsOnly" ref="home" class="home-workspace conversation-home" :aria-busy="loading">
  <header class="home-welcome">
   <h1>今天，想准备哪一堂课？</h1>
   <p>从一个想法开始，和课件助手一起把课堂准备好。</p>
  </header>
  <div class="home-main-grid">
   <section class="home-dialogue" aria-label="和课件助手一起备课">
    <slot/>
   </section>
  </div>
  <section class="home-preparation" aria-labelledby="preparation-heading">
   <div class="home-section-title"><h2 id="preparation-heading">开始备课</h2><span>需求聊清楚，课件更贴近你的课堂</span></div>
   <div class="home-preparation-tools">
    <button type="button" class="home-prepare-courseware" data-home-action="courseware" :disabled="!canCreate" @click="prepare()"><span class="home-prepare-icon"><WorkspaceIcon name="slides"/></span><span><strong>课堂课件</strong><small>描述教学想法，先看三页真实预览</small></span><WorkspaceIcon name="arrow"/></button>
    <button type="button" data-home-action="materials" @click="prepare(true)"><span class="home-prepare-icon"><WorkspaceIcon name="library"/></span><span><strong>教材与参考</strong><small>按需添加资料，设置年级与课时</small></span><WorkspaceIcon name="arrow"/></button>
   </div>
  </section>
    <section class="home-recent" aria-labelledby="recent-heading">
     <div class="home-recent-head"><h2 id="recent-heading">最近作品</h2><button type="button" class="home-recent-all" @click="emit('navigate','tasks')">查看全部<WorkspaceIcon name="arrow"/></button></div>
     <div v-if="loading" class="home-recent-loading" role="status"><span>正在加载作品</span><i v-for="n in 3" :key="n"></i></div>
     <div v-else-if="!recent.length" class="home-recent-empty"><WorkspaceIcon name="works"/><p>你的第一份课件，从这里开始。</p><span>完成的作品会保存在这里，方便随时继续。</span></div>
     <div v-else class="home-recent-list"><button v-for="work in recent" :key="work.id" type="button" class="home-recent-work" @click="emit('open',work)"><span class="home-recent-file"><WorkspaceIcon :name="work.type==='animation'?'animation':'slides'"/></span><span class="home-recent-copy"><strong>{{work.display_title||work.input?.topic||'未命名作品'}}</strong><small>{{statusText(work)}}<template v-if="shortDate(work.created_at)"> · {{shortDate(work.created_at)}}</template></small></span><WorkspaceIcon name="arrow"/></button></div>
    </section>
 </div>
 <div v-else class="home-workspace" :aria-busy="loading">
  <section class="home-content" aria-labelledby="tools-heading">
   <div class="home-tools-panel">
    <div class="home-section-heading"><div><h1 id="tools-heading">所有工具</h1><p>选择适合这堂课的起点，之后可以随时回到作品继续。</p></div></div>
    <div class="home-tool-grid"><button v-for="tool in tools" :key="tool.id" type="button" class="home-tool" :data-tool="tool.id" @click="emit('navigate',tool.id)"><span class="home-tool-icon"><WorkspaceIcon :name="tool.icon"/></span><span class="home-tool-copy"><strong>{{tool.title}}</strong><small>{{tool.description}}</small><em>{{tool.note}}</em></span><WorkspaceIcon class="home-tool-arrow" name="arrow"/></button></div>
   </div>
  </section>
 </div>
</template>
