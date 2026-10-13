<script setup>
import {ref,computed} from 'vue'
import WorkspaceIcon from './WorkspaceIcon.vue'

const props=defineProps({tasks:Array,available:Number,canCreate:Boolean,statusText:Function,loading:Boolean})
const emit=defineEmits(['start','navigate','open'])
const idea=ref('')
const suggestions=[{topic:'浮力与阿基米德原理',subject:'物理'},{topic:'勾股定理的生活应用',subject:'数学'},{topic:'一次函数的图像与性质',subject:'数学'}]
const tools=computed(()=>[
 {id:'courseware',title:'课件制作',description:'先确认前三页，再逐页审核和导出 PPTX。',icon:'slides',note:'20 积分 · 成功交付后结算'},
 {id:'animation',title:'教学动画',description:'把抽象概念变成可审核的动态演示。',icon:'animation',note:'分镜 → 审核 → 生成'},
 {id:'knowledge',title:'知识与来源',description:'检索公开资料，管理企业共享教研材料。',icon:'library',note:'来源可追溯'},
 {id:'tasks',title:'我的作品',description:'查看进度、继续制作和管理历史版本。',icon:'works',note:'只显示自己的作品'},
 {id:'billing',title:'套餐与账单',description:'查看独立积分、预留和模拟支付记录。',icon:'billing',note:props.loading?'正在读取账户':'可用 '+props.available+' 积分'},
 {id:'dashboard',title:'用量与评测',description:'回看使用情况和检索效果。',icon:'dashboard',note:'教学过程可复盘'},
])
const recent=computed(()=>[...(props.tasks||[])].filter(t=>t.type==='courseware'||t.type==='animation').sort((a,b)=>(Date.parse(b.created_at)||0)-(Date.parse(a.created_at)||0)).slice(0,3))
const shortDate=value=>Number.isNaN(Date.parse(value))?'':new Date(value).toLocaleDateString('zh-CN')
function start(){const topic=idea.value.trim();if(topic&&props.canCreate&&!props.loading)emit('start',{topic,subject:suggestions.find(s=>s.topic===topic)?.subject})}
</script>

<template>
 <div class="home-workspace" :aria-busy="loading">
  <section class="home-hero" aria-labelledby="home-heading">
   <div class="home-hero-inner">
    <h1 id="home-heading">你好，今天想准备什么课？</h1>
    <p>从一个想法开始，和课件助手一起把内容整理成可审核的课堂作品。</p>
    <form class="home-prompt" @submit.prevent="start">
     <label for="home-idea">输入课题或备课想法</label>
     <textarea id="home-idea" v-model="idea" rows="3" maxlength="100" placeholder="例如：浮力，让八年级学生通过实验理解浮力的方向和大小"></textarea>
     <div class="home-prompt-actions"><span><WorkspaceIcon name="check"/> 先确认需求，再生成前三页</span><button class="primary" :disabled="!idea.trim()||!canCreate||loading">开始备课 <WorkspaceIcon name="arrow"/></button></div>
    </form>
    <div class="home-suggestions" aria-label="备课灵感"><span>试试这些课题</span><button v-for="s in suggestions" :key="s.topic" type="button" @click="idea=s.topic">{{s.topic}} <WorkspaceIcon name="arrow"/></button></div>
   </div>
  </section>

  <section class="home-content" aria-labelledby="tools-heading">
   <div class="home-section-label"><span>为你的课堂准备</span></div>
   <div class="home-dashboard">
    <div class="home-tools-panel">
     <div class="home-section-heading"><div><h2 id="tools-heading">常用教学工具</h2><p>选择适合这堂课的起点，之后可以随时回到作品继续。</p></div></div>
     <div class="home-tool-grid"><button v-for="tool in tools" :key="tool.id" type="button" class="home-tool" :data-tool="tool.id" @click="emit('navigate',tool.id)"><span class="home-tool-icon"><WorkspaceIcon :name="tool.icon"/></span><span class="home-tool-copy"><strong>{{tool.title}}</strong><small>{{tool.description}}</small><em>{{tool.note}}</em></span><WorkspaceIcon class="home-tool-arrow" name="arrow"/></button></div>
    </div>
    <aside class="home-recent" aria-labelledby="recent-heading"><div class="home-recent-head"><span class="home-recent-icon"><WorkspaceIcon name="clock"/></span><h2 id="recent-heading">最近作品</h2></div><div v-if="loading" class="home-recent-loading" role="status"><span>正在加载作品</span><i v-for="n in 3" :key="n"></i></div><div v-else-if="!recent.length" class="home-recent-empty">第一份作品会出现在这里。<button @click="emit('navigate','courseware')">开始制作课件 →</button></div><div v-else class="home-recent-list"><button v-for="work in recent" :key="work.id" class="home-recent-work" @click="emit('open',work)"><span :class="['home-work-state',work.status]">{{statusText(work)}}</span><strong>{{work.display_title||work.input?.topic||'未命名作品'}}</strong><small>{{work.type==='animation'?'教学动画':'可编辑课件'}}<template v-if="shortDate(work.created_at)"> · {{shortDate(work.created_at)}}</template></small><WorkspaceIcon name="arrow"/></button></div><button class="home-recent-all" @click="emit('navigate','tasks')">打开我的作品 <WorkspaceIcon name="arrow"/></button></aside>
   </div>
  </section>
 </div>
</template>
