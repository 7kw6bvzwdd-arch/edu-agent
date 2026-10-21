<script setup>
import {ref,computed,onMounted,onUnmounted,watch} from 'vue'
import CoursewareEvidence from './CoursewareEvidence.vue'
import {answerSteps} from './classroom.js'
const props=defineProps({deck:Object,images:Object,saved:Boolean,token:String})
const emit=defineEmits(['close'])
const index=ref(0),step=ref(0),fullPage=ref(false),stage=ref(null),error=ref(''),answerStep=ref(0),formulaURLs=ref({})
let formulaVersion=0,formulaController=null
let notesWindow=null,previousFocus
const slide=computed(()=>props.deck.slides[index.value])
const count=computed(()=>slide.value.html?0:slide.value.bullets?.length||0)
const actualPage=computed(()=>!!slide.value.html||fullPage.value)
const answers=computed(()=>answerSteps(slide.value.answer))
const formulaVisible=computed(()=>fullPage.value||!['steps','example'].includes(slide.value.layout)||step.value===count.value)
function revealAnswer(){fullPage.value=false;answerStep.value=Math.min(answerStep.value+1,answers.value.length)}
function advance(){if(fullPage.value){nextPage();return}if(step.value<count.value)step.value++;else nextPage()}
function nextPage(){if(index.value<props.deck.slides.length-1){index.value++;step.value=0}}
function previous(){if(step.value&&!actualPage.value)step.value--;else if(index.value>0){index.value--;step.value=props.deck.slides[index.value].html?0:props.deck.slides[index.value].bullets?.length||0}}
function close(){if(document.fullscreenElement===stage.value)document.exitFullscreen().catch(()=>{});emit('close')}
function key(event){if(event.key==='Tab'){const buttons=[...stage.value.querySelectorAll('button:not(:disabled)')],position=buttons.indexOf(document.activeElement);event.preventDefault();buttons[(position+(event.shiftKey?-1:1)+buttons.length)%buttons.length]?.focus();return}if(event.key===' '&&document.activeElement?.tagName==='BUTTON')return;if(['ArrowRight','ArrowDown','PageDown',' '].includes(event.key)){event.preventDefault();advance()}else if(['ArrowLeft','ArrowUp','PageUp'].includes(event.key)){event.preventDefault();previous()}else if(event.key==='Escape')close()}
async function fullscreen(){try{if(document.fullscreenElement)await document.exitFullscreen();else await stage.value.requestFullscreen()}catch{error.value='当前浏览器无法全屏，可使用浏览器全屏快捷键。'}}
function updateNotes(){if(!notesWindow||notesWindow.closed)return;notesWindow.document.title='教师讲解提示';const title=notesWindow.document.getElementById('title'),notes=notesWindow.document.getElementById('notes');if(title)title.textContent=`第${index.value+1}页：${slide.value.title}`;if(notes)notes.textContent=(slide.value.notes||'本页暂无讲解备注。')+(slide.value.answer?'\n课堂答案：'+slide.value.answer:'')}
function openNotes(){notesWindow=window.open('','edu-teacher-notes','popup,width=600,height=700');if(!notesWindow){error.value='请允许弹出教师提示窗口后重试。';return}notesWindow.document.body.innerHTML='<main style="font:20px/1.8 sans-serif;padding:28px"><p>教师提示 · 请将课件演示窗口投屏给学生</p><h1 id="title" style="font-size:24px"></h1><pre id="notes" style="font:inherit;white-space:pre-wrap"></pre></main>';updateNotes()}
watch([index,()=>props.deck],updateNotes)
watch(()=>slide.value,()=>{answerStep.value=0})
async function loadFormula(){const text=slide.value.formula;if(!text||formulaURLs.value[text])return;const version=++formulaVersion;formulaController?.abort();formulaController=new AbortController();try{const response=await fetch('/api/courseware/formula',{method:'POST',headers:{Authorization:'Bearer '+props.token,'Content-Type':'application/json'},body:JSON.stringify({formula:text}),signal:formulaController.signal});if(!response.ok)throw Error('公式画面未能加载，可切换实际PPT整页查看。');const blob=await response.blob();if(version!==formulaVersion)return;formulaURLs.value[text]=URL.createObjectURL(blob)}catch(e){if(e.name!=='AbortError')error.value=e.message}}
watch(()=>slide.value.formula,loadFormula,{immediate:true})
onMounted(()=>{previousFocus=document.activeElement;window.addEventListener('keydown',key);document.body.style.overflow='hidden';stage.value.focus()})
onUnmounted(()=>{document.body.style.overflow='';formulaVersion++;formulaController?.abort();Object.values(formulaURLs.value).forEach(URL.revokeObjectURL);window.removeEventListener('keydown',key);notesWindow?.close();previousFocus?.focus()})
</script>
<template>
 <section ref="stage" class="classroom-presentation" role="dialog" aria-modal="true" aria-label="课堂演示" tabindex="-1">
  <header><b>{{deck.title}}</b><div><button v-if="!slide.html" @click="fullPage=!fullPage">{{fullPage?'切换逐步讲解':'显示实际PPT整页'}}</button><button @click="openNotes">教师提示窗口</button><button @click="fullscreen">全屏</button><button @click="close" aria-label="退出课堂演示">退出</button></div></header>
  <p v-if="!saved" class="present-notice">演示当前未保存内容；实际PPT截图仍为已保存版本。</p><p v-if="!fullPage&&(slide.canvas_objects?.length||Object.keys(slide.canvas_positions||{}).length)" class="present-notice">逐步讲解按教学内容展示；查看画布布局请切换“显示实际PPT整页”。</p><p v-if="error" role="alert">{{error}}</p>
  <div :class="['presentation-page',deck.theme]">
   <template v-if="actualPage&&images?.[slide.id]"><img :src="images[slide.id]" :alt="slide.title"></template>
   <p v-else-if="slide.html" class="present-notice">实际生成页面尚未加载，请保存后重新打开课堂演示。</p>
   <template v-else><p v-if="fullPage" class="present-notice">此页没有实际PPT截图，显示内容演示。</p><small>{{slide.section}}</small><h1>{{slide.title}}</h1><div class="presentation-body"><ol><li v-for="(text,i) in slide.bullets" :key="i" :style="{visibility:fullPage||i<step?'visible':'hidden'}">{{text}}</li></ol><div v-if="slide.chart||slide.diagram||slide.graph||slide.scientific||slide.table||slide.figure||slide.picture" class="presentation-media"><CoursewareEvidence :slide="{...slide,focus:''}" preview/></div></div><p v-if="slide.formula&&formulaVisible" class="presentation-formula"><img v-if="formulaURLs[slide.formula]" :src="formulaURLs[slide.formula]" alt="本页公式"><span v-else>公式画面加载中…</span></p><div v-if="answerStep" class="presentation-answer" aria-live="polite"><p v-for="(text,i) in answers.slice(0,answerStep)" :key="i">{{text}}</p></div><p v-if="slide.focus&&(fullPage||step===count)&&slide.layout!=='exercise'" class="presentation-focus">{{slide.focus}}</p><p v-if="slide.layout==='exercise'" class="presentation-question">{{slide.focus}}</p></template>
  </div>
  <footer><button @click="previous" :disabled="index===0&&step===0">上一步</button><span>{{index+1}} / {{deck.slides.length}} 页 · {{actualPage?'实际页面':`已展开 ${step} / ${count} 步`}}</span><button v-if="!slide.html&&answers.length&&answerStep<answers.length" @click="revealAnswer">{{answerStep?'揭示下一步答案':'揭示答案'}}</button><button v-if="answerStep" @click="answerStep=0">收起答案</button><button @click="advance" :disabled="index===deck.slides.length-1&&(actualPage||step===count)">下一步</button></footer>
 </section>
</template>
<style scoped>
.classroom-presentation{position:fixed;inset:0;z-index:1000;background:#10251c;color:white;display:flex;flex-direction:column;padding:16px;outline:none}.classroom-presentation header,.classroom-presentation footer{display:flex;align-items:center;justify-content:space-between;gap:12px;padding:8px}.classroom-presentation header div{display:flex;gap:8px;flex-wrap:wrap}.classroom-presentation button{background:#fff;color:#173b2c}.presentation-page{margin:auto;width:min(100%,1280px);aspect-ratio:16/9;max-height:calc(100vh - 160px);overflow:auto;padding:4%;background:#fff;color:#182820}.presentation-page img{display:block;width:100%;height:100%;object-fit:contain}.presentation-page:has(>img){padding:0}.presentation-page h1{font-size:clamp(24px,3.4vw,44px);line-height:1.3;margin:0 0 28px}.presentation-body{display:flex;gap:4%;align-items:center}.presentation-body ol{flex:1;font-size:clamp(20px,2.4vw,32px);line-height:1.65;padding-left:32px;margin:0}.presentation-body li{margin-bottom:16px}.presentation-media{width:46%;min-width:0}.presentation-focus,.presentation-question{font-size:24px;font-weight:700}.presentation-answer p{margin:0 0 12px}.presentation-answer{font-size:24px;white-space:pre-wrap;background:#fff2b3;padding:16px}.presentation-formula img{max-width:100%;height:auto;max-height:90px}.presentation-formula{font-size:28px;text-align:center;white-space:pre-wrap}.present-notice{font-size:13px;margin:4px 8px}.presentation-page .present-notice{color:#53685b}.classroom-presentation footer{justify-content:center}@media(max-width:650px){.classroom-presentation header b{display:none}.presentation-body{flex-direction:column;align-items:stretch}.presentation-media{width:100%}.presentation-page{aspect-ratio:auto;flex:1}.classroom-presentation header button{font-size:11px}.presentation-body ol{font-size:22px}}
</style>
