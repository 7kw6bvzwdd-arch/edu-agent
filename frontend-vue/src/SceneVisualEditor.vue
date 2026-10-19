<script setup>
const props=defineProps({scene:Object,editable:Boolean})
const types={rect:'矩形',ellipse:'椭圆',text:'文字标注',path:'路径',polygon:'多边形',arrow:'箭头'}
const ops={move:'移动',follow_path:'沿路径移动',rotate:'旋转',scale:'缩放',show:'出现',hide:'消失',emphasize:'强调'}
function addObject(type){props.scene.objects.push({id:'shape_'+crypto.randomUUID().replaceAll('-','').slice(0,12),type,x:.5,y:.5,width:.12,height:.12,color:'#1767bf',...(type==='text'?{text:'教学标注',font_size:28}:{}),...(['path','polygon','arrow'].includes(type)?{points:type==='polygon'?[[.3,.3],[.6,.3],[.5,.6]]:[[.2,.5],[.5,.3],[.8,.5]]}:{})})}
function removeObject(id){props.scene.objects=props.scene.objects.filter(o=>o.id!==id);props.scene.actions=props.scene.actions.filter(a=>a.object!==id&&a.path!==id)}
function actionChanged(a){delete a.x;delete a.y;delete a.to;delete a.path;if(a.op==='move'){a.x=.7;a.y=.5}else if(a.op==='follow_path'){a.path=props.scene.objects.find(o=>o.type==='path'&&o.id!==a.object)?.id||''}else if(a.op==='rotate')a.to=90;else if(a.op==='scale')a.to=1.3}
function addAction(){props.scene.actions.push({object:props.scene.objects[0].id,op:'emphasize',start_ms:0,duration_ms:Math.min(1000,props.scene.duration_ms)})}
</script>
<template>
 <section class="scene-editor">
  <p>图形中心与路径坐标均为0–1，x向右、y向下。按结构化描述演示，不代替学科仿真。</p>
  <label>整段时长 / 毫秒<input type="number" min="1000" max="8000" step="100" v-model.number="scene.duration_ms" :readonly="!editable"></label>
  <div class="scene-toolbar"><button v-for="(label,type) in types" :key="type" type="button" :disabled="!editable||scene.objects.length>=24" @click="addObject(type)">＋ {{label}}</button></div>
  <details v-for="(o,index) in scene.objects" :key="o.id" class="scene-object"><summary>{{index+1}} · {{types[o.type]}} · {{o.text||o.id}}</summary>
   <label>对象编号<input :value="o.id" readonly></label>
   <label>线条/文字颜色<input type="color" v-model="o.color" :disabled="!editable"></label>
   <template v-if="!o.points"><label>中心 x<input type="number" min="0" max="1" step="0.01" v-model.number="o.x" :readonly="!editable"></label><label>中心 y<input type="number" min="0" max="1" step="0.01" v-model.number="o.y" :readonly="!editable"></label></template>
   <template v-if="['rect','ellipse'].includes(o.type)"><label>宽度比例<input type="number" min="0.005" max="1" step="0.01" v-model.number="o.width" :readonly="!editable"></label><label>高度比例<input type="number" min="0.005" max="1" step="0.01" v-model.number="o.height" :readonly="!editable"></label></template>
   <template v-if="['rect','ellipse','polygon'].includes(o.type)"><label><input type="checkbox" :checked="!!o.fill" :disabled="!editable" @change="$event.target.checked?o.fill=o.color:delete o.fill">填充</label><label v-if="o.fill">填充颜色<input type="color" v-model="o.fill" :disabled="!editable"></label></template>
   <template v-if="o.type==='text'"><label>标注文字<input v-model="o.text" maxlength="60" :readonly="!editable"></label><label>字号<input type="number" min="22" max="60" v-model.number="o.font_size" :readonly="!editable"></label></template>
   <template v-if="o.points"><div class="point-row" v-for="(p,i) in o.points" :key="i"><span>路径点 {{i+1}}</span><label>x<input type="number" min="0" max="1" step="0.01" v-model.number="p[0]" :readonly="!editable"></label><label>y<input type="number" min="0" max="1" step="0.01" v-model.number="p[1]" :readonly="!editable"></label><button type="button" :disabled="!editable||o.points.length<=(o.type==='polygon'?3:2)" @click="o.points.splice(i,1)">删点</button></div><button type="button" :disabled="!editable||o.points.length>=32" @click="o.points.push([.7,.5])">加路径点</button></template>
   <button type="button" :disabled="!editable||scene.objects.length<=1" @click="removeObject(o.id)">删除对象及关联动作</button>
  </details>
  <h5>动画动作</h5>
  <details v-for="(a,i) in scene.actions" :key="i" class="scene-action"><summary>{{i+1}} · {{ops[a.op]}} · {{a.object}}</summary>
   <label>作用对象<select v-model="a.object" :disabled="!editable"><option v-for="o in scene.objects" :key="o.id" :value="o.id">{{o.text||o.id}}</option></select></label>
   <label>动作<select v-model="a.op" :disabled="!editable" @change="actionChanged(a)"><option v-for="(label,op) in ops" :key="op" :value="op">{{label}}</option></select></label>
   <label>开始 / 毫秒<input type="number" min="0" :max="scene.duration_ms" step="100" v-model.number="a.start_ms" :readonly="!editable"></label><label>持续 / 毫秒<input type="number" min="100" :max="scene.duration_ms" step="100" v-model.number="a.duration_ms" :readonly="!editable"></label>
   <template v-if="a.op==='move'"><label>终点 x<input type="number" min="0" max="1" step="0.01" v-model.number="a.x" :readonly="!editable"></label><label>终点 y<input type="number" min="0" max="1" step="0.01" v-model.number="a.y" :readonly="!editable"></label></template>
   <label v-if="a.op==='follow_path'">沿哪条路径<select v-model="a.path" :disabled="!editable"><option v-for="o in scene.objects.filter(o=>o.type==='path'&&o.id!==a.object)" :key="o.id" :value="o.id">{{o.id}}</option></select></label>
   <label v-if="['rotate','scale'].includes(a.op)">{{a.op==='rotate'?'终点角度（顺时针）':'终点缩放倍数'}}<input type="number" step="any" v-model.number="a.to" :readonly="!editable"></label>
   <button type="button" :disabled="!editable" @click="scene.actions.splice(i,1)">删除动作</button>
  </details>
  <button type="button" :disabled="!editable||scene.actions.length>=48" @click="addAction">＋ 动画动作</button>
  <p>有动作时自动生成循环GIF并插入PPT；清空动作后导出静态图。图形在PPT中为图片，结构参数可回到此处编辑。</p>
 </section>
</template>
<style scoped>.scene-toolbar,.point-row{display:flex;flex-wrap:wrap;gap:6px}.scene-object,.scene-action{border:1px solid #d8dfeb;border-radius:6px;margin:8px 0;padding:8px}.scene-editor summary{cursor:pointer;font-size:12px}.scene-editor .point-row input{width:72px}.scene-editor h5{font-size:14px}.scene-editor label{font-size:12px}.scene-editor input[type=color]{width:48px}.scene-editor input[type=checkbox]{width:auto}</style>
