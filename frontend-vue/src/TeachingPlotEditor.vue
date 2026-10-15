<script setup>
import {computed} from 'vue'
const props=defineProps({slide:Object,editable:Boolean})
const chart=computed(()=>props.slide.chart)
const numeric=computed(()=>['scatter','errorbar'].includes(chart.value?.style))
const free=computed(()=>!['graph','scientific','chart','table','figure','picture','diagram'].some(k=>props.slide[k]))
function addExperiment(){
 if(!props.editable||!free.value)return
 props.slide.chart={title:'电流与电压（示意数据，非实测）',style:'scatter',labels:['点1','点2','点3'],x_values:[1,2,5],values:[.1,.2,.5],x_label:'电压 U / V',y_label:'电流 I / A',source_label:'示意数据，非实测'}
}
function changeStyle(){
 if(chart.value.style==='errorbar')chart.value.y_errors=chart.value.values.map(()=> '')
 else delete chart.value.y_errors
}
function changeRows(add){
 const c=chart.value
 if(add){c.labels.push('点'+(c.labels.length+1));c.x_values.push('');c.values.push('');if(c.style==='errorbar')c.y_errors.push('')}
 else{c.labels.pop();c.x_values.pop();c.values.pop();c.y_errors?.pop()}
}
const issue=computed(()=>{
 const c=chart.value
 if(!numeric.value)return ''
 if(!c.x_label?.trim()||!c.y_label?.trim()||!c.source_label?.trim())return '请填写横纵轴名称与单位，以及数据来源。'
 if(![...c.x_values,...c.values].every(v=>Number.isFinite(v)&&Math.abs(v)<=1000000))return '请填写有效的横纵坐标，数值绝对值不超过1000000。'
 if(c.style==='errorbar'&&(!c.y_errors?.length||!c.y_errors.every(v=>Number.isFinite(v)&&v>=0&&v<=1000000)))return '请填写每个数据点的非负误差大小，并在来源说明中注明误差含义。'
 return ''
})
</script>

<template>
 <section class="teaching-plot-editor" aria-label="教学图表编辑">
  <button v-if="editable&&free" type="button" class="add-experiment-plot" @click="addExperiment">＋ 实验数据图</button>
  <template v-if="chart">
   <h4>{{numeric?'实验数据图':'统计图表'}}</h4>
   <p v-if="numeric" class="fine">横坐标按真实数值间距绘制。可在此改数据并重新出图，PPT中呈现为高清图片。初始数据仅为示意。</p>
   <label>图表标题<input v-model="chart.title" maxlength="80" :readonly="!editable"></label>
   <label>图表类型<select v-model="chart.style" :disabled="!editable" aria-label="实验图类型" @change="changeStyle"><template v-if="numeric"><option value="scatter">散点图</option><option value="errorbar">散点与纵向误差线</option></template><template v-else><option value="bar">柱状图 · PPT内可编辑</option><option value="line">折线图 · 等间隔，PPT内可编辑</option></template></select></label>
   <div v-if="numeric" class="plot-axis-fields"><label>横轴名称与单位<input v-model="chart.x_label" aria-label="实验图横轴" maxlength="40" :readonly="!editable"></label><label>纵轴名称与单位<input v-model="chart.y_label" aria-label="实验图纵轴" maxlength="40" :readonly="!editable"></label></div>
   <div class="plot-data-rows">
    <div v-for="(_,i) in chart.labels" :key="i" :class="['plot-data-row',{'with-errors':numeric&&chart.style==='errorbar'}]">
     <label v-if="!numeric">标签<input v-model="chart.labels[i]" maxlength="18" :readonly="!editable"></label>
     <label v-if="numeric">x<input v-model.number="chart.x_values[i]" type="number" step="any" min="-1000000" max="1000000" :aria-label="'数据点'+(i+1)+'横坐标'" :readonly="!editable"></label>
     <label>{{numeric?'y':'数值'}}<input v-model.number="chart.values[i]" type="number" step="any" :aria-label="'数据点'+(i+1)+'纵坐标'" :readonly="!editable"></label>
     <label v-if="numeric&&chart.style==='errorbar'">± 误差<input v-model.number="chart.y_errors[i]" type="number" step="any" min="0" max="1000000" :aria-label="'数据点'+(i+1)+'误差'" :readonly="!editable"></label>
    </div>
   </div>
   <template v-if="numeric"><label>数据来源与误差说明<input v-model="chart.source_label" aria-label="实验图数据来源" maxlength="100" :readonly="!editable" placeholder="示意数据，非实测；或真实数据出处及误差含义"></label><p v-if="issue" role="alert" class="plot-error">{{issue}}</p><div v-if="editable" class="plot-controls"><button type="button" :disabled="chart.labels.length>=8" @click="changeRows(true)">添加数据点</button><button type="button" :disabled="chart.labels.length<=1" @click="changeRows(false)">删除末点</button></div></template>
   <button v-if="editable" type="button" class="remove-experiment-plot" @click="slide.chart=null">移除图表</button>
  </template>
 </section>
</template>

<style scoped>
.teaching-plot-editor{margin:14px 0}.teaching-plot-editor h4{font-size:14px;margin-bottom:8px}.teaching-plot-editor .fine{line-height:1.8;font-size:12px}.plot-axis-fields,.plot-data-row{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:10px}.plot-data-row.with-errors{grid-template-columns:repeat(3,minmax(0,1fr))}.plot-data-rows{display:grid;gap:8px;margin:12px 0}.plot-data-row input{min-width:0}.plot-controls{display:flex;gap:8px;flex-wrap:wrap;margin:10px 0}.plot-error{font-size:12px;color:#a13939}.remove-experiment-plot{margin-top:10px}
@media(max-width:560px){.plot-axis-fields{grid-template-columns:1fr}.plot-data-row.with-errors{gap:6px}}
</style>
