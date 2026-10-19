<script setup>
import {ref,computed,watch,nextTick,onMounted,onUnmounted,inject} from 'vue'
import MediaImage from './MediaImage.vue'
import {uploadTeachingImage} from './media.js'
const mediaToken=inject('mediaToken',ref(''))
import {canvasTextIssue} from './canvas-text.js'
const props=defineProps({slide:Object,objects:Array,image:String,editable:Boolean,complete:Boolean,preview:Function,previewKey:String})
const stageWidth=ref(960)
const selected=ref([]),stage=ref(null),textEditing=ref(''),textInput=ref(null),textDraft=ref('')
const undo=ref([]),redo=ref([]),message=ref(''),zoom=ref(100),grid=ref(false),snap=ref(true)
const fileInput=ref(null),uploading=ref(false),previewImage=ref(''),previewHash=ref(''),previewBusy=ref(false),autoPreview=ref(false)
const labels={title:'标题',body:'正文',visual:'图示',formula:'公式',focus:'课堂提问'}
const clone=x=>JSON.parse(JSON.stringify(x)),snapshot=()=>JSON.stringify(props.slide)
const hash=computed(()=>JSON.stringify([props.previewKey,props.slide]))
const actual=computed(()=>previewHash.value===hash.value&&!!previewImage.value)
const positions=computed(()=>props.slide.canvas_positions||{})
const customs=computed(()=>props.slide.canvas_objects||[])
const textIssue=computed(()=>canvasTextIssue(props.slide))
let drag=null,textOriginal='',dragMoved=false,replaceImage='',disposed=false,previewTimer=null,stageObserver=null
const geometryMap=ref(null)
const baseSizes=new Map()
function restore(value){const restored=JSON.parse(value);for(const key of Object.keys(props.slide))if(!(key in restored))delete props.slide[key];Object.assign(props.slide,restored)}
function commit(before){if(before!==snapshot()){undo.value.push(before);if(undo.value.length>30)undo.value.shift();redo.value=[]}}
function change(fn){if(!props.editable||!finishText())return;const before=snapshot();message.value='';fn();commit(before)}
function undoEdit(){if(!props.editable)return;if(textEditing.value){restore(textOriginal);textEditing.value='';return}const prev=undo.value.pop();if(prev){redo.value.push(snapshot());restore(prev)}}
function redoEdit(){if(!props.editable||textEditing.value)return;const next=redo.value.pop();if(next){undo.value.push(snapshot());restore(next)}}
function exists(id){return id==='title'||id==='body'&&(props.slide.bullets||[]).length||id==='focus'&&props.slide.focus||id==='formula'&&props.slide.formula||id==='visual'&&['graph','scientific','picture','table','figure','chart','diagram'].some(k=>props.slide[k])}
const objects=computed(()=>{
 const native=(geometryMap.value||props.objects||[]).filter(o=>!o.id.startsWith('object-')&&exists(o.id)).map(o=>{
  const now=positions.value[o.id]||{},saved=o.offset||{},ratio=(now.scale??1)/(saved.scale??1)
  return {...o,x:o.x+(now.dx||0)-(saved.dx||0),y:o.y+(now.dy||0)-(saved.dy||0),width:o.width*ratio,height:o.height*ratio,rotation:now.rotation||0,locked:!!now.locked,label:labels[o.id],kind:'native'}
 })
 const all=[...native,...customs.value.map((o,i)=>({...o,rotatable:true,label:o.kind==='text'?'文本框 '+(i+1):o.kind==='image'?'图片 '+(i+1):'图形 '+(i+1)}))]
 const ids=props.slide.canvas_order||[]
 return [...all.filter(o=>!ids.includes(o.id)),...ids.map(id=>all.find(o=>o.id===id)).filter(Boolean)]
})
const active=computed(()=>selected.value.length===1?objects.value.find(o=>o.id===selected.value[0]):null)
const chosen=computed(()=>objects.value.filter(o=>selected.value.includes(o.id)))
const unlocked=computed(()=>chosen.value.filter(o=>!o.locked))
const textObject=computed(()=>objects.value.find(o=>o.id===textEditing.value))
const percent=computed(()=>{const o=active.value;if(!o)return 100;if(o.kind==='native')return Math.round((positions.value[o.id]?.scale??1)*100);return Math.round(o.width/(baseSizes.get(o.id)?.width||o.width)*100)})
const limits=computed(()=>{
 const o=active.value;if(!o)return {min:100,max:100}
 const available=Math.min((1-o.x)/o.width,(1-o.y)/o.height)
 if(o.kind==='native')return {min:Math.ceil(Math.max(.8,(o.offset?.scale??1)*(o.min_scale||0))*100-1e-6),max:Math.floor(Math.min(1.5,(positions.value[o.id]?.scale??1)*available)*100)}
 return {min:Math.ceil(Math.max(.01/o.width,.005/o.height)*percent.value),max:Math.floor(percent.value*available)}
})
function extents(o){const a=(o.rotation||0)*Math.PI/180,c=Math.abs(Math.cos(a)),s=Math.abs(Math.sin(a)),w=o.width*c+o.height*s*9/16,h=o.width*s*16/9+o.height*c;return {x:o.x+(o.width-w)/2,y:o.y+(o.height-h)/2,width:w,height:h}}
function inside(o){const b=extents(o);return b.x>=-1e-6&&b.y>=-1e-6&&b.x+b.width<=1.000001&&b.y+b.height<=1.000001}
function nativeOffset(id){props.slide.canvas_positions||={};return props.slide.canvas_positions[id]||={dx:0,dy:0,scale:1}}
function objectData(id){return customs.value.find(o=>o.id===id)||nativeOffset(id)}
function setPosition(o,x,y){if(o.kind==='native'){const original=(geometryMap.value||props.objects).find(v=>v.id===o.id),p=nativeOffset(o.id);p.dx=+(x-original.x+(original.offset?.dx||0)).toFixed(5);p.dy=+(y-original.y+(original.offset?.dy||0)).toFixed(5)}else Object.assign(objectData(o.id),{x:+x.toFixed(5),y:+y.toFixed(5)})}
function moveGroup(originals,dx,dy){
 let lowX=-1,highX=1,lowY=-1,highY=1
 for(const o of originals){const b=extents(o);lowX=Math.max(lowX,-b.x);highX=Math.min(highX,1-b.x-b.width);lowY=Math.max(lowY,-b.y);highY=Math.min(highY,1-b.y-b.height)
  if(o.kind==='native'){const p=positions.value[o.id]||{};lowX=Math.max(lowX,-.8-(p.dx||0));highX=Math.min(highX,.8-(p.dx||0));lowY=Math.max(lowY,-.8-(p.dy||0));highY=Math.min(highY,.8-(p.dy||0))}}
 dx=Math.max(lowX,Math.min(highX,dx));dy=Math.max(lowY,Math.min(highY,dy));for(const o of originals)setPosition(o,o.x+dx,o.y+dy)
}
function select(event,o){if(event.shiftKey||event.ctrlKey||event.metaKey){selected.value=selected.value.includes(o.id)?selected.value.filter(id=>id!==o.id):[...selected.value,o.id]}else selected.value=[o.id]}
function start(event,o,mode='move'){
 if(!props.editable||event.button!==0||!finishText())return
 event.preventDefault();if(event.shiftKey||event.ctrlKey||event.metaKey||!selected.value.includes(o.id))select(event,o);stage.value.focus({preventScroll:true});dragMoved=false
 if(o.locked||!selected.value.includes(o.id))return
 const rect=stage.value.getBoundingClientRect();drag={x:event.clientX,y:event.clientY,width:rect.width,height:rect.height,before:snapshot(),objects:clone(unlocked.value),mode,object:clone(o),percent:percent.value}
 event.currentTarget.setPointerCapture(event.pointerId)
}
function update(event){
 if(!drag||Math.hypot(event.clientX-drag.x,event.clientY-drag.y)<3&&!dragMoved)return
 dragMoved=true;let dx=(event.clientX-drag.x)/drag.width,dy=(event.clientY-drag.y)/drag.height
 if(snap.value&&!event.altKey){dx=Math.round(dx*100)/100;dy=Math.round(dy*100)/100}
 restore(drag.before)
 if(drag.mode==='resize'){const ratio=Math.max(.01,1+Math.max(dx/drag.object.width,dy/drag.object.height));resizeTo(drag.percent*ratio,drag.object)}else moveGroup(drag.objects,dx,dy)
}
function stop(){if(drag){commit(drag.before);drag=null}}
function cancelDrag(){if(drag){restore(drag.before);drag=null}}
function resizeTo(value,o=active.value){
 if(!o||o.locked||!Number.isFinite(value))return
 const current=o.kind==='native'?(positions.value[o.id]?.scale??1)*100:o.width/(baseSizes.get(o.id)?.width||o.width)*100
 const ratio=value/current,candidate={...o,width:o.width*ratio,height:o.height*ratio}
 if(!inside(candidate)){message.value='调整后的对象会超出页面，请先移动或减小尺寸。';return}
 if(o.kind==='native'){if(value<limits.value.min||value>150)return;nativeOffset(o.id).scale=+(value/100).toFixed(5)}else if(candidate.width>=.01&&candidate.height>=.005)Object.assign(objectData(o.id),{width:+candidate.width.toFixed(5),height:+candidate.height.toFixed(5)})
}
function resize(event){change(()=>resizeTo(Number(event.target.value)))}
function rotate(event){change(()=>{const o=active.value,v=Number(event.target.value);if(!o||o.locked||!o.rotatable||!Number.isFinite(v)||Math.abs(v)>180)return;if(!inside({...o,rotation:v})){message.value='旋转后会超出页面，请先将对象向页面中间移动。';return}objectData(o.id).rotation=v})}
function geometry(key,event){change(()=>{
 const o=active.value,value=Number(event.target.value)/100;if(!o||o.kind==='native'||o.locked||!Number.isFinite(value))return
 const candidate={...o,[key]:value}
 if(o.kind==='image'&&key==='width')candidate.height=o.height*value/o.width
 if(o.kind==='image'&&key==='height')candidate.width=o.width*value/o.height
 if(candidate.width<.01||candidate.height<.005||!inside(candidate)){message.value='位置或尺寸超出页面，请调整后重试。';return}
 Object.assign(objectData(o.id),{x:candidate.x,y:candidate.y,width:candidate.width,height:candidate.height})
})}
function property(key,event){change(()=>{if(!active.value||active.value.locked)return;const value=event.target.type==='number'?Number(event.target.value):event.target.type==='checkbox'?event.target.checked:event.target.type==='color'?event.target.value.slice(1):event.target.value;if(key==='font_size'&&(!Number.isFinite(value)||value<16||value>60)||key==='line_width'&&(!Number.isFinite(value)||value<.5||value>8)){message.value='请输入允许范围内的数值。';return}objectData(active.value.id)[key]=value})}
function nativeText(id){return id==='body'?(props.slide.bullets||[]).join('\n'):props.slide[id]||''}
function setText(id,value){
 if(id.startsWith('object-'))objectData(id).text=value;else if(id==='body')props.slide.bullets=value.split('\n').filter(line=>line.trim());else props.slide[id]=value
 if((id==='focus'&&!value.trim())||(id==='body'&&!props.slide.bullets.length)){delete props.slide.canvas_positions?.[id];if(props.slide.canvas_positions&&!Object.keys(props.slide.canvas_positions).length)delete props.slide.canvas_positions;props.slide.canvas_order=props.slide.canvas_order?.filter(x=>x!==id)}
}
function finishText(){if(!textEditing.value)return true;if(textIssue.value)return false;commit(textOriginal);textEditing.value='';schedulePreview();return true}
async function editText(event,o){
 if(dragMoved){dragMoved=false;return}
 if(event.shiftKey||event.ctrlKey||event.metaKey||!props.editable||o.locked||!['title','body','focus'].includes(o.id)&&o.kind!=='text')return
 if(textEditing.value===o.id)return;if(!finishText()){textInput.value?.focus();return}
 selected.value=[o.id];textEditing.value=o.id;textOriginal=snapshot();textDraft.value=o.kind==='text'?objectData(o.id).text:nativeText(o.id);await nextTick();textInput.value?.focus()
}
function textKey(event){if(event.isComposing)return;if(event.key==='Escape'){event.preventDefault();undoEdit()}if(event.key==='Enter'&&(event.ctrlKey||event.metaKey)){event.preventDefault();finishText()}}
function add(kind,shape='rect'){change(()=>{
 if(customs.value.length>=24){message.value='本页最多添加24个对象。';return}
 const o={id:'object-'+crypto.randomUUID(),kind,x:.35,y:.4,width:.3,height:.14,rotation:0,color:'263B36',background:'',locked:false}
 if(kind==='text')Object.assign(o,{text:'输入教学内容',font_size:24,bold:false,align:'left'})
 if(kind==='shape')Object.assign(o,{shape,line_width:2,height:shape==='line'?.015:.18})
 props.slide.canvas_objects||=[];props.slide.canvas_objects.push(o);baseSizes.set(o.id,{width:o.width,height:o.height});selected.value=[o.id]
})}
function chooseImage(replace=false){replaceImage=replace?active.value?.id||'':'';fileInput.value?.click()}
async function upload(event){
 const file=event.target.files?.[0];event.target.value='';if(!file||!props.editable)return
 const target=props.slide,id=target.id,before=snapshot(),replace=replaceImage;uploading.value=true;message.value=''
 try{
  const asset=await uploadTeachingImage(file,mediaToken.value)
  const imageWidth=asset.width,imageHeight=asset.height
   if(disposed||props.slide!==target||props.slide.id!==id||!props.editable)return
   if(snapshot()!==before)throw Error('读取图片期间页面已修改，请重新添加图片。')
   const existing=customs.value.find(o=>o.id===replace)
   if(existing?.locked)throw Error('图片已锁定，请先解锁。')
   if(!existing&&customs.value.length>=24)throw Error('本页最多添加24个对象。')
   const width=Math.min(.3,.3*imageWidth/imageHeight*9/16),height=width*16/9*imageHeight/imageWidth
   const o=existing||{id:'object-'+crypto.randomUUID(),kind:'image',x:.35,y:.4,width,height,rotation:0,locked:false,caption:''}
   const nextWidth=existing?existing.width:width,nextHeight=nextWidth*16/9*imageHeight/imageWidth
   if(!inside({...o,width:nextWidth,height:nextHeight}))throw Error('替换图片会超出页面，请先缩小图片。')
   delete o.data_url;o.asset_id=asset.asset_id;o.width=nextWidth;o.height=nextHeight
   if(!existing){props.slide.canvas_objects||=[];props.slide.canvas_objects.push(o)}
   baseSizes.set(o.id,{width:o.width,height:o.height});selected.value=[o.id];commit(before)
 }catch(e){message.value=e.message}finally{uploading.value=false}
}
function duplicate(){change(()=>{
 const items=chosen.value.filter(o=>o.kind!=='native'&&!o.locked)
 if(customs.value.length+items.length>24){message.value='复制后超过每页24个新增对象。';return}
 const copies=items.map(o=>{const c=clone(objectData(o.id));c.id='object-'+crypto.randomUUID();const b=extents(c);c.x+=Math.min(.02,1-b.x-b.width);c.y+=Math.min(.02,1-b.y-b.height);return c})
 props.slide.canvas_objects||=[];props.slide.canvas_objects.push(...copies);selected.value=copies.map(o=>o.id);for(const o of copies)baseSizes.set(o.id,{width:o.width,height:o.height})
})}
function remove(){change(()=>{
 const ids=unlocked.value.filter(o=>o.id!=='title').map(o=>o.id)
 if(selected.value.includes('title'))message.value='页面标题需保留，可直接编辑文字。'
 for(const id of ids){if(id==='body')props.slide.bullets=[];else if(id==='visual'){if(props.slide.picture)props.slide.image_search={status:'removed',message:'教师已在画布移除配图，保存后生效。'};for(const k of ['graph','scientific','picture','table','figure','chart'])delete props.slide[k];props.slide.diagram=''}else if(!id.startsWith('object-'))props.slide[id]='';delete props.slide.canvas_positions?.[id]}
 props.slide.canvas_objects=customs.value.filter(o=>!ids.includes(o.id));props.slide.canvas_order=(props.slide.canvas_order||[]).filter(id=>!ids.includes(id));selected.value=selected.value.filter(id=>!ids.includes(id))
})}
function lock(){change(()=>{const value=chosen.value.some(o=>!o.locked);for(const o of chosen.value)objectData(o.id).locked=value})}
function layer(direction){change(()=>{
 const order=objects.value.map(o=>o.id),ids=new Set(unlocked.value.map(o=>o.id))
 if(direction==='front'||direction==='back'){const moving=order.filter(id=>ids.has(id)),rest=order.filter(id=>!ids.has(id));props.slide.canvas_order=direction==='front'?[...rest,...moving]:[...moving,...rest];return}
 const loop=direction==='up'?[...order.keys()].reverse():[...order.keys()]
 for(const i of loop){const j=direction==='up'?i+1:i-1;if(ids.has(order[i])&&j>=0&&j<order.length&&!ids.has(order[j]))[order[i],order[j]]=[order[j],order[i]]}
 props.slide.canvas_order=order
})}
function align(mode){change(()=>{
 const items=unlocked.value;if(!items.length)return
 const boxes=items.map(extents),left=Math.min(...boxes.map(b=>b.x)),top=Math.min(...boxes.map(b=>b.y)),right=Math.max(...boxes.map(b=>b.x+b.width)),bottom=Math.max(...boxes.map(b=>b.y+b.height))
 for(let i=0;i<items.length;i++){const o=items[i],b=boxes[i];let dx=0,dy=0
  if(mode==='left')dx=(items.length===1?0:left)-b.x;if(mode==='center')dx=(items.length===1?.5:(left+right)/2)-b.x-b.width/2;if(mode==='right')dx=(items.length===1?1:right)-b.x-b.width
  if(mode==='top')dy=(items.length===1?0:top)-b.y;if(mode==='middle')dy=(items.length===1?.5:(top+bottom)/2)-b.y-b.height/2;if(mode==='bottom')dy=(items.length===1?1:bottom)-b.y-b.height
  moveGroup([o],dx,dy)
 }
})}
function distribute(axis){change(()=>{
 const items=[...unlocked.value].sort((a,b)=>extents(a)[axis]-extents(b)[axis]);if(items.length<3)return
 const size=axis==='x'?'width':'height',first=extents(items[0]),last=extents(items.at(-1)),gap=(last[axis]+last[size]-first[axis]-items.reduce((n,o)=>n+extents(o)[size],0))/(items.length-1)
 if(gap<0){message.value='对象过密，请先拉开两端对象再均匀分布。';return}
 let cursor=first[axis];for(const o of items){const b=extents(o),delta=cursor-b[axis];moveGroup([o],axis==='x'?delta:0,axis==='y'?delta:0);cursor+=b[size]+gap}
})}
function key(event){
 if(event.isComposing||['INPUT','TEXTAREA','SELECT'].includes(event.target.tagName)||!props.editable)return
 if((event.ctrlKey||event.metaKey)&&event.key.toLowerCase()==='z'){event.preventDefault();event.shiftKey?redoEdit():undoEdit();return}
 if((event.ctrlKey||event.metaKey)&&event.key.toLowerCase()==='y'){event.preventDefault();redoEdit();return}
 if((event.ctrlKey||event.metaKey)&&event.key.toLowerCase()==='a'){event.preventDefault();selected.value=objects.value.map(o=>o.id);return}
 if(event.key==='Escape'){event.preventDefault();cancelDrag();selected.value=[];return}
 if(event.key==='Delete'||event.key==='Backspace'){event.preventDefault();remove();return}
 const step=event.shiftKey?.01:.002,delta={ArrowLeft:[-step,0],ArrowRight:[step,0],ArrowUp:[0,-step],ArrowDown:[0,step]}[event.key]
 if(delta){event.preventDefault();change(()=>moveGroup(clone(unlocked.value),...delta))}
}
function reset(){change(()=>delete props.slide.canvas_positions)}
async function refreshPreview(){
 if(!props.complete||!props.preview||previewBusy.value||!props.editable||!finishText()||textIssue.value)return
 const expected=hash.value;previewBusy.value=true;message.value='';let failed=false
 try{const result=await props.preview(props.slide);if(!disposed&&hash.value===expected){previewImage.value=result.image;previewHash.value=expected;if(result.objects?.length)geometryMap.value=result.objects.map(o=>({...o,offset:clone(positions.value[o.id]||{dx:0,dy:0,scale:1})}))}else if(!disposed)message.value='排版期间有新修改，请更新实际预览。'}catch(e){failed=true;if(!disposed)message.value=e.message+'；当前修改仍保留。'}finally{previewBusy.value=false;if(!disposed&&!failed&&hash.value!==expected)schedulePreview()}
}
function schedulePreview(){clearTimeout(previewTimer);if(autoPreview.value)previewTimer=setTimeout(()=>{if(!previewBusy.value&&!textEditing.value&&!actual.value)void refreshPreview()},2000)}
watch(hash,schedulePreview)
watch(autoPreview,schedulePreview)
watch(()=>props.slide.id,()=>{selected.value=[];textEditing.value='';undo.value=[];redo.value=[];drag=null;previewImage.value='';previewHash.value='';geometryMap.value=null;baseSizes.clear();for(const o of customs.value)baseSizes.set(o.id,{width:o.width,height:o.height})},{immediate:true})
watch(objects,()=>{selected.value=selected.value.filter(id=>objects.value.some(o=>o.id===id))})
onMounted(()=>{stageObserver=new ResizeObserver(entries=>{stageWidth.value=entries[0].contentRect.width});stageObserver.observe(stage.value)})
onUnmounted(()=>{disposed=true;clearTimeout(previewTimer);stageObserver?.disconnect();drag=null})
function style(o){return {left:o.x*100+'%',top:o.y*100+'%',width:o.width*100+'%',height:o.height*100+'%',transform:`rotate(${o.rotation||0}deg)`}}
const newObject=o=>o.kind!=='native'&&!(props.objects||[]).some(saved=>saved.id===o.id)
</script>
<template>
 <section class="object-editor" aria-label="页面对象画布" @keydown="key">
  <header class="canvas-heading"><div><strong>页面画布</strong><p>点击文字编辑，拖动调整位置。按住 Shift 多选，方向键微调。</p></div><span>{{selected.length?selected.length+' 个对象已选':'选择页面中的对象'}}</span></header>
  <div class="canvas-toolbar" role="toolbar" aria-label="画布工具">
   <template v-if="complete"><button @click="add('text')" :disabled="!editable">添加文本</button><button @click="chooseImage()" :disabled="!editable||uploading">{{uploading?'读取图片…':'添加图片'}}</button><select aria-label="添加图形" :disabled="!editable" @change="add('shape',$event.target.value);$event.target.value='' "><option value="">添加图形</option><option value="rect">矩形</option><option value="ellipse">椭圆</option><option value="line">直线</option><option value="arrow">箭头</option></select></template>
   <button @click="undoEdit" :disabled="!editable||(!textEditing&&!undo.length)">撤销</button><button @click="redoEdit" :disabled="!editable||!!textEditing||!redo.length">重做</button><label><input type="checkbox" v-model="grid">网格</label><label><input type="checkbox" v-model="snap">吸附</label><label>视图 <select v-model.number="zoom" aria-label="画布视图缩放"><option :value="100">适应</option><option :value="125">125%</option><option :value="150">150%</option><option :value="200">200%</option></select></label>
  </div>
  <input ref="fileInput" class="canvas-file" type="file" accept="image/png,image/jpeg,image/gif" aria-label="上传画布图片" @change="upload">
  <div class="canvas-scroll"><div ref="stage" :class="['object-stage',{'show-grid':grid}]" :style="{width:zoom+'%'}" tabindex="0" aria-label="课件画布，方向键移动选中对象" @pointerdown.self="finishText()&&(selected=[])" >
   <img :src="actual?previewImage:image" :alt="actual?'当前修改的实际PPT画面':'已保存版本的实际PPT画面'">
   <button v-for="o in objects" :key="o.id" type="button" :data-object-id="o.id" :data-kind="o.kind" :disabled="!editable" :aria-label="'移动'+o.label" :aria-pressed="selected.includes(o.id)" :class="{locked:o.locked}" :style="style(o)" @pointerdown="start($event,o)" @pointermove="update" @pointerup="stop" @pointercancel="cancelDrag" @lostpointercapture="stop" @focus="!selected.includes(o.id)&&(selected=[o.id])" @click="editText($event,o)">
    <template v-if="!actual&&newObject(o)"><span v-if="o.kind==='text'" class="new-text" :style="{fontSize:o.font_size*stageWidth/960+'px',color:'#'+o.color,fontWeight:o.bold?'bold':'normal',textAlign:o.align,background:o.background?'#'+o.background:'transparent'}">{{o.text}}</span><MediaImage v-else-if="o.kind==='image'" class="new-image" :picture="o" :alt="o.caption||'待保存图片'"/><span v-else :class="['new-shape',o.shape]" :style="{borderColor:'#'+o.color,background:o.background?'#'+o.background:'transparent'}"></span></template>
    <span class="object-label">{{o.label}}{{o.locked?' · 锁定':''}}</span>
   </button>
   <button v-if="active&&!active.locked&&!textEditing" class="resize-handle" aria-label="拖动等比例缩放对象" :style="{left:(active.x+active.width)*100+'%',top:(active.y+active.height)*100+'%'}" :disabled="!editable" @pointerdown.stop="start($event,active,'resize')" @pointermove="update" @pointerup="stop" @pointercancel="cancelDrag" @lostpointercapture="stop">↘</button>
   <textarea v-if="textObject" ref="textInput" :class="['canvas-text-input',{'body-text':textEditing==='body'}]" :aria-label="'画布编辑'+textObject.label" :value="textDraft" :readonly="!editable" :aria-invalid="!!textIssue" :style="{...style(textObject),top:textObject.y>.65?undefined:textObject.y*100+'%',bottom:textObject.y>.65?(1-textObject.y-textObject.height)*100+'%':undefined}" @input="textDraft=$event.target.value;setText(textEditing,textDraft)" @keydown.stop="textKey"></textarea>
  </div></div>
  <div class="canvas-preview-row"><span role="status">{{actual?'当前修改已完成实际排版':'底图为已保存画面，选框和新增对象表示待保存修改。'}}</span><template v-if="complete"><button @click="refreshPreview" :disabled="!editable||previewBusy||!!textIssue">{{previewBusy?'正在实际排版…':'更新实际预览'}}</button><label><input type="checkbox" v-model="autoPreview">编辑后自动预览</label></template></div>
  <div class="text-actions"><button v-if="textEditing" @click="finishText" :disabled="!editable||!!textIssue">完成输入</button><button @click="undoEdit" :disabled="!editable||(!textEditing&&!undo.length)">撤销文字修改</button><span v-if="textEditing">{{textEditing==='body'?'每行一条要点。':''}}Esc撤销本次输入，Ctrl+Enter完成。</span></div>
  <p v-if="textIssue||message" class="canvas-text-warning" role="alert">{{textIssue||message}}</p>
  <div class="canvas-inspector">
   <div class="object-actions"><strong>{{active?.label|| (selected.length?'多个对象':'对象属性')}}</strong><label v-if="active">等比例缩放 <input type="range" aria-label="对象缩放" :min="limits.min" :max="Math.max(limits.min,limits.max)" :value="percent" :disabled="!editable||active.locked||limits.min>=limits.max" @input="resize"><output>{{percent}}%</output></label><label v-if="complete&&active?.rotatable">旋转 <input type="number" aria-label="对象旋转角度" min="-180" max="180" step="5" :value="active.rotation||0" :disabled="!editable||active.locked" @change="rotate">°</label><span v-if="complete&&active&&!active.rotatable" class="fine">学科图示和公式保持原方向</span></div>
   <div v-if="complete&&active&&active.kind!=='native'" class="custom-properties">
    <label v-for="[id,label] in [['x','水平位置'],['y','垂直位置'],['width','宽度'],['height','高度']]" :key="id">{{label}} <input type="number" :aria-label="'对象'+label" min="0" max="100" step="0.1" :value="+(active[id]*100).toFixed(2)" :disabled="!editable||active.locked" @change="geometry(id,$event)">%</label>
    <template v-if="active.kind==='text'"><label>字号 <input type="number" aria-label="文本字号" min="16" max="60" :value="active.font_size" :disabled="!editable||active.locked" @change="property('font_size',$event)"></label><label><input type="checkbox" aria-label="文本粗体" :checked="active.bold" :disabled="!editable||active.locked" @change="property('bold',$event)">粗体</label><select aria-label="文本对齐" :value="active.align" :disabled="!editable||active.locked" @change="property('align',$event)"><option value="left">左对齐</option><option value="center">居中</option><option value="right">右对齐</option></select></template>
    <template v-if="active.kind!=='image'"><label>{{active.kind==='text'?'文字颜色':'线条颜色'}} <input type="color" aria-label="对象颜色" :value="'#'+(active.color||'263B36')" :disabled="!editable||active.locked" @change="property('color',$event)"></label><label>填充 <input type="color" aria-label="对象填充颜色" :value="'#'+(active.background||'FFFFFF')" :disabled="!editable||active.locked" @change="property('background',$event)"></label><button @click="change(()=>objectData(active.id).background='')" :disabled="!editable||active.locked||!active.background">无填充</button></template>
    <template v-if="active.kind==='shape'"><label>线宽 <input type="number" aria-label="图形线宽" min="0.5" max="8" step="0.5" :value="active.line_width" :disabled="!editable||active.locked" @change="property('line_width',$event)"></label></template>
    <template v-if="active.kind==='image'"><button @click="chooseImage(true)" :disabled="!editable||active.locked||uploading">替换图片</button><label>图片说明 <input aria-label="图片说明" maxlength="60" :value="active.caption" :disabled="!editable||active.locked" @change="property('caption',$event)"></label></template>
   </div>
   <div v-if="complete" class="canvas-arrange"><button @click="duplicate" :disabled="!editable||!chosen.some(o=>o.kind!=='native'&&!o.locked)">复制对象</button><button @click="remove" :disabled="!editable||!unlocked.some(o=>o.id!=='title')">删除对象</button><button @click="lock" :disabled="!editable||!chosen.length">{{chosen.length&&chosen.every(o=>o.locked)?'解锁对象':'锁定对象'}}</button><button v-for="[id,label] in [['back','置底'],['down','下移一层'],['up','上移一层'],['front','置顶']]" :key="id" @click="layer(id)" :disabled="!editable||!unlocked.length">{{label}}</button></div>
   <div v-if="complete" class="canvas-arrange"><button v-for="[id,label] in [['left','左对齐'],['center','水平居中'],['right','右对齐'],['top','顶端对齐'],['middle','垂直居中'],['bottom','底端对齐']]" :key="id" @click="align(id)" :disabled="!editable||!unlocked.length">{{label}}</button><button @click="distribute('x')" :disabled="!editable||unlocked.length<3">横向均匀分布</button><button @click="distribute('y')" :disabled="!editable||unlocked.length<3">纵向均匀分布</button></div>
   <div class="canvas-bottom"><button @click="reset" :disabled="!editable||!slide.canvas_positions">恢复默认位置与大小</button><span>保存后更新课件与PPTX；改动页需重新审核。撤销记录最多30步。</span></div>
   <details v-if="complete"><summary>图层 · {{objects.length}}个对象</summary><div class="canvas-layers"><button v-for="o in [...objects].reverse()" :key="o.id" :aria-pressed="selected.includes(o.id)" :disabled="!editable" @click="finishText()&&select($event,o)">{{o.label}} <span>{{o.locked?'已锁定':''}}</span></button></div></details>
  </div>
 </section>
</template>
<style scoped>
.object-editor{padding:16px;border:1px solid #d9dce1;border-radius:12px;margin:14px 0;background:#f8f9fb;min-width:0;color:#323c45}.canvas-heading{display:flex;justify-content:space-between;gap:12px;align-items:start}.canvas-heading strong{font-size:15px}.canvas-heading p{margin:5px 0 12px;font-size:12px;color:#65717b}.canvas-heading>span{font-size:11px;color:#65717b;white-space:nowrap}.canvas-toolbar,.canvas-arrange,.custom-properties,.object-actions,.canvas-preview-row,.text-actions,.canvas-bottom{display:flex;align-items:center;gap:8px;flex-wrap:wrap}.object-editor button,.object-editor select{font-size:12px;padding:7px 9px;border-radius:5px}.object-editor button:focus-visible,.object-editor input:focus-visible,.object-editor select:focus-visible,.object-stage:focus-visible{outline:2px solid #7551b4;outline-offset:2px}.object-editor label{display:flex;align-items:center;gap:5px;font-size:12px}.object-editor input[type=checkbox]{width:15px;min-width:15px;margin:0}.object-editor input[type=number]{width:70px;padding:5px;min-width:0}.object-editor input[type=color]{width:32px;height:28px;padding:2px;min-width:0}.object-editor select{width:auto;min-width:0}.object-editor .canvas-file{display:none}.canvas-scroll{overflow:auto;margin:12px 0;border:1px solid #d9dce1;background:#e7e9ee}.object-stage{position:relative;aspect-ratio:16/9;background:white;overflow:hidden;min-width:0}.object-stage>img{display:block;width:100%;height:100%;object-fit:contain;pointer-events:none}.object-stage.show-grid:after{content:'';position:absolute;inset:0;pointer-events:none;z-index:1;background-image:linear-gradient(to right,#56668825 1px,transparent 1px),linear-gradient(to bottom,#56668825 1px,transparent 1px);background-size:10% 10%}.object-stage>button:not(.resize-handle){position:absolute;padding:0;border:1px dashed #8462b6;background:#8655c006;border-radius:1px;cursor:move;touch-action:none;min-width:0;white-space:normal}.object-stage>button[aria-pressed=true]{border:2px solid #7551b4;background:#8655c012;z-index:2}.object-stage>button.locked{border-color:#8b929a;cursor:default}.object-stage .object-label{white-space:nowrap;position:absolute;left:0;top:0;font-size:10px;background:#7551b4;color:white;padding:1px 4px;pointer-events:none}.object-stage .locked .object-label{background:#717983}.object-stage .resize-handle{position:absolute;z-index:3;width:24px;height:24px;min-width:0;padding:0;transform:translate(-50%,-50%);background:white;color:#7551b4;border:2px solid #7551b4;cursor:nwse-resize;touch-action:none}.new-text{display:block;white-space:pre-wrap;font-size:clamp(9px,1.3vw,20px);text-align:left;height:100%;overflow:hidden;padding:2px;pointer-events:none}.new-image{width:100%;height:100%;object-fit:fill;pointer-events:none}.new-shape{display:block;width:100%;height:100%;border:2px solid;box-sizing:border-box;pointer-events:none}.new-shape.ellipse{border-radius:50%}.new-shape.line{height:0;position:absolute;top:50%;border-width:2px 0 0}.new-shape.arrow{clip-path:polygon(0 25%,65% 25%,65% 0,100% 50%,65% 100%,65% 75%,0 75%)}.canvas-preview-row{font-size:11px;color:#65717b;justify-content:space-between;margin:8px 0}.canvas-inspector{border-top:1px solid #dfe2e7;padding-top:12px;margin-top:12px}.canvas-inspector>div{margin:9px 0}.object-actions strong{font-size:12px;min-width:70px}.object-actions input[type=range]{width:120px;min-width:0}.object-actions output{font-size:12px;min-width:38px}.custom-properties input:not([type=checkbox]):not([type=color]):not([type=number]){width:160px;min-width:0}.fine,.canvas-bottom span{font-size:11px;color:#65717b}.canvas-bottom{justify-content:space-between}.canvas-layers{display:grid;gap:4px;padding-top:8px;max-height:220px;overflow:auto}.canvas-layers button{display:flex;justify-content:space-between;text-align:left}.canvas-layers button[aria-pressed=true]{background:#ede7f6;border-color:#7551b4}.canvas-layers span{color:#65717b;font-size:11px}.canvas-inspector summary{font-size:12px;cursor:pointer;padding:8px 0}.canvas-text-input{position:absolute;z-index:4;box-sizing:border-box;margin:0;min-width:0;min-height:44px;resize:none;border:2px solid #7551b4;border-radius:2px;background:white;color:#263B36;padding:5px;font-family:inherit;font-size:14px;line-height:1.5;box-shadow:0 0 0 2px white;overflow:auto}.canvas-text-input.body-text{min-height:72px}.text-actions{font-size:11px;color:#65717b}.canvas-text-warning{color:#9b321f;background:#fff1e7;padding:8px;border-radius:5px;font-size:12px;line-height:1.6}@media(max-width:600px){.object-editor{padding:10px}.canvas-heading{display:block}.canvas-heading>span{display:block;margin:8px 0}.object-editor button,.object-editor select{min-height:34px}.object-stage .resize-handle{min-height:24px}.canvas-toolbar{gap:6px}.canvas-bottom{align-items:start}.canvas-preview-row{justify-content:flex-start}}
</style>
