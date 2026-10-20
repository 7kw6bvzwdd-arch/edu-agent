<script setup>
import {ref,computed,onUnmounted,inject} from 'vue'
import MediaImage from './MediaImage.vue'
import CoursewareImageSearch from './CoursewareImageSearch.vue'
import {uploadTeachingImage} from './media.js'
const mediaToken=inject('mediaToken',ref(''))
import EmphasisText from './EmphasisText.vue'
import ScientificDiagram from './ScientificDiagram.vue'
import FunctionPlot from './FunctionPlot.vue'
import TeachingChart from './TeachingChart.vue'
import {showGraphCoordinates} from './classroom.js'
const props=defineProps({slide:{type:Object,required:true},editable:Boolean,preview:Boolean,taskId:String,revision:Number,useWeb:Boolean})
const imageError=ref(''),uploading=ref(false)
let uploadVersion=0
onUnmounted(()=>uploadVersion++)
async function uploadPicture(event){
 const file=event.target.files?.[0];event.target.value='';if(!file)return
 const version=++uploadVersion,target=props.slide;imageError.value='';uploading.value=true
 try{
  const asset=await uploadTeachingImage(file,mediaToken.value)
  if(version!==uploadVersion||target!==props.slide||!props.editable)return
  if(target.scientific||target.graph||target.chart||target.diagram||(target.table&&target.figure))throw Error('此页图形过多，请先移除冲突图形。')
  target.picture={asset_id:asset.asset_id,caption:target.picture?.caption||'',source:''}
  target.image_search={status:'selected',message:'教师已上传替换图片，请填写新图片出处并保存。'}
 }catch(e){if(version===uploadVersion)imageError.value=e.message||'图片无法上传，请重试。'}
 finally{if(version===uploadVersion)uploading.value=false}
}
function routePoints(items){return items.map((_,i)=>`${(i+.5)*100/items.length},${i%2?5:21}`).join(' ')}
function removePicture(){uploadVersion++;uploading.value=false;props.slide.picture=null;props.slide.image_search={status:'removed',message:'教师已移除配图，保存后生效。'}}
function addTable(slide){slide.chart=null;slide.diagram='';slide.table={title:'',headers:['条件','观察结果'],rows:[['',''],['','']]}}
function addFigure(slide){slide.chart=null;slide.diagram='';slide.figure={kind:'flow',title:'',items:[{label:'',detail:''},{label:'',detail:''}]}}
const intervalValid=computed(()=>{
 const f=props.slide.figure,v=f?.values
 return f?.unit?.trim()&&Array.isArray(v)&&v.length===f.items.length&&v.every((n,i)=>typeof n==='number'&&Number.isFinite(n)&&(!i||n>v[i-1]))&&Number.isFinite(v.at(-1)-v[0])
})
function intervalPoint(i){const v=props.slide.figure.values;return 2+96*(v[i]-v[0])/(v.at(-1)-v[0])}
function intervalValue(n){return Number(n.toPrecision(8))+'\u00a0'+props.slide.figure.unit}
function changeFigureKind(){
 const f=props.slide.figure
 if(f.kind==='interval'){f.unit||='min';if(f.values?.length!==f.items.length)f.values=f.items.map((_,i)=>i*10)}
 else{delete f.unit;delete f.values}
}
function changeNodes(add){
 const f=props.slide.figure
 if(add){f.items.push({label:'',detail:''});if(f.kind==='interval')f.values.push((Number(f.values.at(-1))||0)+10)}
 else{f.items.pop();if(f.kind==='interval')f.values.pop()}
}
</script>

<template>
 <div v-if="preview" class="evidence-preview"><FunctionPlot v-if="slide.graph" :graph="slide.graph" :show-coordinates="showGraphCoordinates(slide)"/><ScientificDiagram :slide="slide" preview/><TeachingChart v-if="slide.chart||slide.diagram" :chart="slide.chart" :diagram="slide.diagram"/>
  <div :class="['evidence-pair',{'has-both':slide.table&&slide.figure||slide.picture&&(slide.table||slide.figure)}]">
   <div v-if="slide.table" class="evidence-table"><h3 v-if="slide.table.title">{{slide.table.title}}</h3><table><thead><tr><th v-for="(h,i) in slide.table.headers" :key="i" scope="col"><EmphasisText :text="h" :terms="slide.emphasis||[]"/></th></tr></thead><tbody><tr v-for="(row,i) in slide.table.rows" :key="i"><td v-for="(value,j) in row" :key="j"><EmphasisText :text="value" :terms="slide.emphasis||[]"/></td></tr></tbody></table></div>
   <div v-if="slide.figure" class="evidence-figure">
    <h3 v-if="slide.figure.title">{{slide.figure.title}}</h3>
    <template v-if="slide.figure.kind==='interval'">
     <div v-if="intervalValid" class="interval-preview">
      <ol class="interval-labels" :style="{'--nodes':slide.figure.items.length}"><li v-for="(item,i) in slide.figure.items" :key="i"><b><EmphasisText :text="item.label" :terms="slide.emphasis||[]"/></b><span>{{item.detail}}</span></li></ol>
      <svg viewBox="0 0 100 20" role="img" aria-label="按数值比例绘制的区间，下面标注各段长度">
       <line v-for="(_,i) in slide.figure.items" :key="'label'+i" :x1="(i+.5)*100/slide.figure.items.length" y1="0" :x2="intervalPoint(i)" y2="8" stroke="currentColor" stroke-width=".18"/>
       <g v-for="(_,i) in slide.figure.values.slice(1)" :key="i">
        <rect class="interval-segment" :x="intervalPoint(i)" y="8" :width="intervalPoint(i+1)-intervalPoint(i)" height="2" fill="currentColor" :opacity="i%2?.5:1"/>
        <line :x1="(intervalPoint(i)+intervalPoint(i+1))/2" y1="10" :x2="(i+.5)*100/(slide.figure.items.length-1)" y2="20" stroke="currentColor" stroke-width=".18"/>
       </g>
       <line v-for="(_,i) in slide.figure.items" :key="'tick'+i" :x1="intervalPoint(i)" y1="7" :x2="intervalPoint(i)" y2="11" stroke="currentColor" stroke-width=".3"/>
      </svg>
      <div class="interval-values" :style="{'--nodes':slide.figure.items.length-1}"><b v-for="(v,i) in slide.figure.values.slice(1)" :key="i">{{intervalValue(v-slide.figure.values[i])}}</b></div>
      <small>总跨度 {{intervalValue(slide.figure.values.at(-1)-slide.figure.values[0])}} · 线段按数值比例绘制</small>
     </div>
     <p v-else class="image-error" role="alert">请填写单位与各节点数值，数值须严格递增。</p>
    </template>
    <template v-else>
     <small v-if="slide.figure.kind==='timeline'">顺序示意 · 不按时间比例</small>
     <template v-if="slide.figure.kind==='route'"><small>路线示意 · 不表示实际距离或方向</small><svg class="route-path" viewBox="0 0 100 26" role="img" aria-label="位置连接示意"><polyline :points="routePoints(slide.figure.items)" fill="none" stroke="currentColor" stroke-width="1.2"/><circle v-for="(_,i) in slide.figure.items" :key="i" :cx="(i+.5)*100/slide.figure.items.length" :cy="i%2?5:21" r="1.8" fill="currentColor"/></svg></template>
     <ol :class="slide.figure.kind" :style="{'--nodes':slide.figure.items.length}"><li v-for="(item,i) in slide.figure.items" :key="i"><b><EmphasisText :text="item.label" :terms="slide.emphasis||[]"/></b><span><EmphasisText :text="item.detail" :terms="slide.emphasis||[]"/></span></li></ol>
    </template>
   </div>
   <figure v-if="slide.picture" class="evidence-picture"><MediaImage :picture="slide.picture" :alt="slide.picture.caption"/><figcaption>{{slide.picture.caption}}</figcaption></figure>
  </div>
  <p v-if="slide.focus" class="evidence-focus"><EmphasisText :text="slide.focus" :terms="slide.emphasis||[]"/></p>
 </div>
 <details v-else class="courseware-evidence-editor">
  <summary>专业图示、配图、课堂图解与表格</summary><ScientificDiagram :slide="slide" :editable="editable"/>
  <section v-if="slide.graph" class="evidence-edit-section">
   <label>函数绘图方式<select :value="slide.graph.renderer||'native'" @change="slide.graph.renderer=$event.target.value" :disabled="!editable" aria-label="函数绘图方式"><option value="native">PPT可编辑线条</option><option value="matplotlib">高清函数图 · 网页参数可修改</option></select></label>
   <p v-if="slide.graph.renderer==='matplotlib'" class="fine">修改函数参数并保存后重新绘制。PPT中呈现为图片。</p>
   <label>坐标图标题<input v-model="slide.graph.title" :readonly="!editable" maxlength="60"></label>
   <div class="evidence-grid" style="--columns:2"><label v-for="(label,key) in {x_min:'x 最小值',x_max:'x 最大值',y_min:'y 最小值',y_max:'y 最大值'}" :key="key">{{label}}<input type="number" step="any" v-model.number="slide.graph[key]" :readonly="!editable"></label></div>
   <label><input type="checkbox" :checked="!!slide.graph.animation" :disabled="!editable" @change="slide.graph.animation=$event.target.checked?{series:0,param:slide.graph.series[0].kind==='quadratic'?'a':'k',to:2,duration_ms:2000}:null">导出参数变化GIF</label>
   <div v-if="slide.graph.animation" class="evidence-grid" style="--columns:2">
    <label>变化曲线<select v-model.number="slide.graph.animation.series" :disabled="!editable" @change="slide.graph.animation.param=slide.graph.series[slide.graph.animation.series].kind==='quadratic'?'a':'k'"><option v-for="(_,i) in slide.graph.series" :value="i">{{i+1}}</option></select></label>
    <label>变化参数<select v-model="slide.graph.animation.param" :disabled="!editable"><option v-for="p in (slide.graph.series[slide.graph.animation.series]?.kind==='quadratic'?['a','b','c']:slide.graph.series[slide.graph.animation.series]?.kind==='inverse'?['k']:['k','b'])" :value="p">{{p}}</option></select></label>
    <label>目标值<input type="number" step="any" v-model.number="slide.graph.animation.to" :readonly="!editable"></label><label>时长（毫秒）<input type="number" min="1000" max="8000" step="100" v-model.number="slide.graph.animation.duration_ms" :readonly="!editable"></label>
   </div><p v-if="slide.graph.animation" class="fine">PPT放映查看动图，编辑和截图显示首帧；a经过0时为退化函数，反比例系数不能跨零。</p>
   <div v-for="(series,i) in slide.graph.series" :key="i" class="evidence-edit-section">
    <label>函数类型<select :value="series.kind||'linear'" @change="series.kind=$event.target.value;series.a??=1;series.c??=0;slide.graph.animation=null" :disabled="!editable"><option value="linear">一次/常函数 y=kx+b</option><option value="quadratic">二次函数 y=ax²+bx+c</option><option value="inverse">反比例 y=k/x</option></select></label><div v-if="series.kind==='quadratic'" class="evidence-grid" style="--columns:2"><label>a<input type="number" step="any" v-model.number="series.a" :readonly="!editable"></label><label>c<input type="number" step="any" v-model.number="series.c" :readonly="!editable"></label></div>
    <div class="evidence-grid" style="--columns:2"><label v-if="series.kind!=='quadratic'">k<input type="number" step="any" v-model.number="series.k" :readonly="!editable"></label><label v-if="series.kind!=='inverse'">b<input type="number" step="any" v-model.number="series.b" :readonly="!editable"></label></div>
    <label v-for="(_,j) in series.mark_x||[]" :key="j">{{'标记点 '+(j+1)+' 的 x'}}<input type="number" step="any" v-model.number="series.mark_x[j]" :readonly="!editable"></label>
    <div v-if="editable" class="evidence-actions"><button type="button" :disabled="(series.mark_x||[]).length>=2" @click="(series.mark_x||=[]).push(0)">添加标记点</button><button type="button" :disabled="!series.mark_x?.length" @click="series.mark_x.pop()">删除末点</button><button type="button" :disabled="slide.graph.series.length<=1" @click="slide.graph.animation=null;slide.graph.series.splice(i,1)">移除此函数</button></div>
   </div>
   <div v-if="editable" class="evidence-actions"><button type="button" :disabled="slide.graph.series.length>=4" @click="slide.graph.series.push({kind:'linear',k:1,b:0,a:1,c:0,mark_x:[]})">添加函数</button><button type="button" @click="slide.graph=null">移除坐标图</button></div>
  </section>
  <label>核心问题或结论<input v-model="slide.focus" :readonly="!editable" maxlength="60" placeholder="练习页写提示，不提前透露答案"></label>
  <label>重点词（每行一个，最多6个，每词20字）<textarea :value="(slide.emphasis||[]).join('\n')" @input="slide.emphasis=$event.target.value.split('\n').map(x=>x.trim()).filter(Boolean)" :readonly="!editable" rows="2" placeholder="填写正文或表格中已有的关键词、数值"></textarea></label>
  <div class="evidence-actions" v-if="editable"><button type="button" v-if="!slide.table" :disabled="!!slide.scientific||!!slide.chart||!!slide.diagram||!!slide.picture&&!!slide.figure" @click="addTable(slide)">＋ 教学表格</button><button type="button" v-if="!slide.figure" :disabled="!!slide.scientific||!!slide.graph||!!slide.chart||!!slide.diagram||!!slide.picture&&!!slide.table" @click="addFigure(slide)">＋ 流程/时间线/路线</button><button type="button" v-if="!slide.scientific&&!slide.graph&&!slide.figure&&!slide.picture&&!slide.chart&&!slide.diagram" @click="slide.graph={title:'函数图像',x_min:-3,x_max:3,y_min:-3,y_max:3,series:[{kind:'linear',k:1,b:0,a:1,c:0,mark_x:[]}]}">＋ 函数坐标图</button></div>
  <section class="evidence-edit-section">
   <CoursewareImageSearch :slide="slide" :task-id="taskId" :revision="revision" :editable="editable" :use-web="useWeb"/>
   <label v-if="editable">{{uploading?'正在处理图片…':'上传或替换配图'}}<input type="file" accept="image/png,image/jpeg,image/gif" :disabled="uploading||!!slide.scientific||!!slide.graph||!!slide.chart||!!slide.diagram||!!slide.table&&!!slide.figure" @change="uploadPicture"></label>
   <p v-if="imageError" role="alert" class="image-error">{{imageError}}</p>
   <template v-if="slide.picture"><MediaImage class="image-thumbnail" :picture="slide.picture" :alt="slide.picture.caption||'上传的配图'"/><label>图片说明（必填）<input v-model="slide.picture.caption" :readonly="!editable" maxlength="60" placeholder="说明图片与知识点的关系"></label><label>图片出处（必填）<input v-model="slide.picture.source" :readonly="!editable" maxlength="300" placeholder="如：本人拍摄；或作者、来源网址及许可"></label><button v-if="editable" type="button" @click="removePicture">移除图片</button></template>
   <p class="fine">PNG/JPEG/GIF，8MB以内，保留原图与动图；请使用有权使用的图片。可与表格或流程图组合，图片来源写入PPT备注。单页AI改写保留原图，不读取图像内容。</p>
  </section>
  <p class="fine" v-if="slide.chart||slide.diagram">此页已有图表或固定示意图；课堂表格和流程图请放到其他页。</p>
  <section v-if="slide.table" class="evidence-edit-section">
   <label>表格标题<input v-model="slide.table.title" :readonly="!editable" maxlength="60"></label>
   <div class="evidence-grid" :style="{'--columns':slide.table.headers.length}"><label v-for="(_,j) in slide.table.headers" :key="j">{{'第'+(j+1)+'列表头'}}<input v-model="slide.table.headers[j]" :readonly="!editable" maxlength="24"></label></div>
   <div class="evidence-grid" v-for="(row,i) in slide.table.rows" :key="i" :style="{'--columns':row.length}"><input v-for="(_,j) in row" :key="j" v-model="row[j]" :readonly="!editable" maxlength="60" :aria-label="'第'+(i+1)+'行第'+(j+1)+'列'"></div>
   <div v-if="editable" class="evidence-actions"><button type="button" :disabled="slide.table.rows.length>=5" @click="slide.table.rows.push(slide.table.headers.map(()=>''))">加行</button><button type="button" :disabled="slide.table.rows.length<=1" @click="slide.table.rows.pop()">删末行</button><button type="button" :disabled="slide.table.headers.length>=4" @click="slide.table.headers.push('');slide.table.rows.forEach(r=>r.push(''))">加列</button><button type="button" :disabled="slide.table.headers.length<=2" @click="slide.table.headers.pop();slide.table.rows.forEach(r=>r.pop())">删末列</button><button type="button" @click="slide.table=null">移除表格</button></div>
   <p class="fine">2–4列、1–5行数据；不要留空，未知值填“待测”。示例数据请在标题中注明。</p>
  </section>
  <section v-if="slide.figure" class="evidence-edit-section">
   <div class="media-fields"><label>图解标题<input v-model="slide.figure.title" :readonly="!editable" maxlength="60"></label><label>图解类型<select v-model="slide.figure.kind" @change="changeFigureKind" :disabled="!editable"><option value="flow">流程推理</option><option value="timeline">时间顺序（不按比例）</option><option value="route">路线示意（不按比例）</option><option value="interval">数值区间（按比例）</option></select></label></div>
   <label v-if="slide.figure.kind==='interval'">区间单位<input v-model="slide.figure.unit" :readonly="!editable" maxlength="12" placeholder="如 min、s、m"></label>
   <div class="media-fields" v-for="(item,i) in slide.figure.items" :key="i"><label>{{'节点'+(i+1)}}<input v-model="item.label" :readonly="!editable" maxlength="24"></label><label>说明<input v-model="item.detail" :readonly="!editable" maxlength="60"></label><label v-if="slide.figure.kind==='interval'">{{'节点'+(i+1)+'数值'}}<input type="number" step="any" v-model.number="slide.figure.values[i]" :readonly="!editable"></label></div>
   <p v-if="slide.figure.kind==='interval'" class="fine">使用同一起点与单位的递增数值，如 0、30、32、60 min。实际时刻写在节点标签；完整讲解放备注。</p>
   <div v-if="editable" class="evidence-actions"><button type="button" :disabled="slide.figure.items.length>=4" @click="changeNodes(true)">加节点</button><button type="button" :disabled="slide.figure.items.length<=2" @click="changeNodes(false)">删末节点</button><button type="button" @click="slide.figure=null">移除图解</button></div>
  </section>
  <p class="fine" v-if="slide.table||slide.figure">图解页正文保留1–2条短问题，完整讲解放备注。图表可在导出的PPT中继续编辑。</p>
 </details>
</template>

<style scoped>
.interval-preview svg{display:block;width:100%;color:var(--green,#009b51)}.interval-preview .interval-labels,.interval-values{display:grid;grid-template-columns:repeat(var(--nodes),minmax(0,1fr));gap:8px;text-align:center;margin:12px 0 0;padding:0;border:0}.interval-labels li{overflow-wrap:anywhere}.interval-values{margin:0 0 18px;font-size:16px}.interval-values b{white-space:nowrap}
.route-path{display:block;width:100%;max-height:150px;margin:10px 0;color:#009b51}.evidence-figure ol.route{display:grid;grid-template-columns:repeat(var(--nodes),minmax(0,1fr));gap:10px;border:0;padding:0;margin:4px 0}.route li{overflow-wrap:anywhere}.evidence-preview th :deep(.teaching-emphasis){color:inherit;text-decoration:underline}:global(.classroom){background:#fff;color:#182820}:global(.classroom .preview-kicker){color:#009b51}:global(.classroom .theme-mini){border-top:6px solid #009b51;border-right:8px solid #ffd539}:global(.classroom .evidence-focus){background:#fff2b3;border-color:#009b51;padding:10px 14px}:global(.classroom .evidence-preview th){background:#ffe99b}:global(.classroom .evidence-preview tbody tr:nth-child(even)){background:#fff9de}
.evidence-picture{margin:0;display:flex;flex-direction:column;justify-content:center;gap:10px}.evidence-picture img{width:100%;height:auto;max-height:300px;object-fit:contain}.evidence-picture figcaption{font-size:13px}.image-thumbnail{max-width:100%;max-height:160px;object-fit:contain;margin-top:10px}.image-error{color:#a44d3d;font-size:13px}
.courseware-evidence-editor{margin:14px 0;border-top:1px solid var(--line);padding-top:12px}.courseware-evidence-editor summary{font-size:12px;cursor:pointer;color:var(--green)}.courseware-evidence-editor label{margin-top:10px}.evidence-actions{display:flex;flex-wrap:wrap;gap:8px;margin-top:10px}.evidence-actions button{font-size:11px;padding:6px 10px}.evidence-grid{display:grid;grid-template-columns:repeat(var(--columns),minmax(0,1fr));gap:8px;margin-top:8px}.evidence-grid input{min-width:0}.evidence-edit-section{margin-top:14px;padding-top:8px;border-top:1px solid var(--line)}.evidence-pair{display:grid;gap:22px}.evidence-pair.has-both{grid-template-columns:1.15fr 1fr}.evidence-pair>*{min-width:0}.evidence-preview h3{font-size:16px;margin:10px 0 12px}.evidence-preview table{width:100%;border-collapse:collapse;table-layout:fixed;font-size:14px}.evidence-preview td,.evidence-preview th{border-bottom:1px solid currentColor;padding:10px 8px;overflow-wrap:anywhere;text-align:left}.evidence-preview th{background:#88888818}.evidence-figure ol{list-style:none;margin:14px 0;padding:0 0 0 16px;border-left:2px solid currentColor;display:grid;gap:18px}.evidence-figure li{display:flex;flex-direction:column;gap:5px;padding:0;background:none;border:0;font-size:15px}.evidence-figure li span{font-size:13px;opacity:.8}.evidence-focus{border-left:4px solid currentColor;padding:8px 0 8px 14px;font-size:19px;font-weight:700;margin:22px 0 0}.evidence-preview small{font-size:11px;opacity:.75}@media(max-width:650px){.evidence-pair.has-both{grid-template-columns:1fr}.evidence-grid{grid-template-columns:repeat(2,minmax(0,1fr))}.evidence-preview td,.evidence-preview th{padding:8px 4px}.evidence-focus{font-size:16px}}
</style>
