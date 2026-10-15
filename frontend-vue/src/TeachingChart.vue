<script setup>
import {computed} from 'vue'
const props=defineProps({chart:Object,diagram:String})
const numeric=computed(()=>['scatter','errorbar'].includes(props.chart?.style))
const format=v=>Number(v.toPrecision(4)).toString()
const dataPlot=computed(()=>{
 const c=props.chart
 if(!numeric.value||!c.values?.length||c.values.length>8||c.x_values?.length!==c.values.length)return null
 if(![...c.x_values,...c.values].every(v=>Number.isFinite(v)&&Math.abs(v)<=1000000))return null
 const errors=c.style==='errorbar'?c.y_errors:c.values.map(()=>0)
 if(errors?.length!==c.values.length||!errors.every(v=>Number.isFinite(v)&&v>=0&&v<=1000000))return null
 if(!c.x_label?.trim()||!c.y_label?.trim()||!c.source_label?.trim())return null
 const range=values=>{const lo=Math.min(...values),hi=Math.max(...values),pad=(hi-lo||Math.max(1,Math.abs(lo)))*.12;return [lo-pad,hi+pad]}
 const [left,right]=range(c.x_values),[low,high]=range(c.values.flatMap((v,i)=>[v-errors[i],v+errors[i]]))
 const px=v=>62+(v-left)/(right-left)*292,py=v=>202-(v-low)/(high-low)*154
 return {px,py,errors,xTicks:Array.from({length:4},(_,i)=>left+(right-left)*i/3),yTicks:Array.from({length:4},(_,i)=>low+(high-low)*i/3)}
})
const plot=computed(()=>{const c=props.chart;if(!c||c.labels?.length!==c.values?.length||!c.values?.length||!c.values.every(Number.isFinite))return null;const lo=Math.min(0,...c.values),hi=Math.max(1,...c.values),py=v=>20+(hi-v)/(hi-lo)*150;return {py,baseline:py(0),width:270/c.values.length,points:c.values.map((v,i)=>`${40+(i+.5)*270/c.values.length},${py(v)}`).join(' '),lo,hi}})
</script>
<template>
<figure v-if="numeric" class="teaching-chart numeric-chart">
 <h3>{{chart.title}}</h3>
 <svg v-if="dataPlot" viewBox="0 0 380 260" role="img" :aria-label="chart.title+'：横坐标按真实数值间距'">
  <text x="8" y="18">{{chart.y_label}}</text>
  <g v-for="tick in dataPlot.yTicks" :key="tick"><line x1="62" :y1="dataPlot.py(tick)" x2="354" :y2="dataPlot.py(tick)" stroke="currentColor" opacity=".15"/><text x="55" :y="dataPlot.py(tick)+4" text-anchor="end">{{format(tick)}}</text></g>
  <path d="M62 48V202H354" fill="none" stroke="currentColor"/>
  <g v-for="tick in dataPlot.xTicks" :key="tick"><line :x1="dataPlot.px(tick)" y1="202" :x2="dataPlot.px(tick)" y2="207" stroke="currentColor"/><text :x="dataPlot.px(tick)" y="221" text-anchor="middle">{{format(tick)}}</text></g>
  <g v-for="(v,i) in chart.values" :key="i">
   <g v-if="chart.style==='errorbar'" class="numeric-error" stroke="currentColor"><line :x1="dataPlot.px(chart.x_values[i])" :x2="dataPlot.px(chart.x_values[i])" :y1="dataPlot.py(v-dataPlot.errors[i])" :y2="dataPlot.py(v+dataPlot.errors[i])"/><line v-for="sign in [-1,1]" :key="sign" :x1="dataPlot.px(chart.x_values[i])-4" :x2="dataPlot.px(chart.x_values[i])+4" :y1="dataPlot.py(v+sign*dataPlot.errors[i])" :y2="dataPlot.py(v+sign*dataPlot.errors[i])"/></g>
   <circle class="numeric-point" :cx="dataPlot.px(chart.x_values[i])" :cy="dataPlot.py(v)" r="3.5" fill="currentColor"><title>{{chart.labels[i]}}：({{chart.x_values[i]}}, {{v}}){{chart.style==='errorbar'?' ± '+dataPlot.errors[i]:''}}</title></circle>
  </g>
  <text x="208" y="246" text-anchor="middle">{{chart.x_label}}</text>
 </svg>
 <p v-else role="alert">请补全有效坐标、轴名称、来源和误差后预览。</p>
 <figcaption>{{chart.source_label}}</figcaption>
</figure>
<figure v-else-if="chart&&plot" class="teaching-chart"><h3>{{chart.title}}</h3><svg viewBox="0 0 340 235" role="img" :aria-label="chart.title"><line x1="40" :y1="plot.baseline" x2="310" :y2="plot.baseline" stroke="currentColor"/><text x="0" y="25">{{plot.hi}}</text><text x="0" y="170">{{plot.lo}}</text><polyline v-if="chart.style==='line'" :points="plot.points" fill="none" stroke="currentColor" stroke-width="2"/><g v-for="(v,i) in chart.values" :key="i"><rect v-if="chart.style!=='line'" :x="40+i*plot.width+8" :y="Math.min(plot.py(v),plot.baseline)" :width="Math.max(3,plot.width-16)" :height="Math.max(1,Math.abs(plot.py(v)-plot.baseline))" fill="currentColor"/><circle v-else :cx="40+(i+.5)*plot.width" :cy="plot.py(v)" r="3" fill="currentColor"/><text :x="40+(i+.5)*plot.width" :y="plot.py(v)-6" text-anchor="middle">{{v}}</text><text :x="40+(i+.5)*plot.width" y="193" text-anchor="middle">{{chart.labels[i]}}</text></g></svg></figure><figure v-else-if="diagram" class="teaching-chart"><svg v-if="diagram==='triangle'" viewBox="0 0 340 235" role="img" aria-label="直角三角形示意"><path d="M70 35 V190 H265 Z M70 170 H90 V190" fill="none" stroke="currentColor" stroke-width="2"/><text x="50" y="110">a</text><text x="155" y="215">b</text><text x="185" y="105">c</text></svg><svg v-else-if="diagram==='buoyancy'" viewBox="0 0 340 235" role="img" aria-label="浮力方向竖直向上"><rect x="40" y="95" width="260" height="110" fill="#e8eff8"/><rect x="130" y="75" width="80" height="70" fill="#d5ad7d" stroke="currentColor"/><path d="M170 110 V25 M158 40 L170 25 L182 40" fill="none" stroke="currentColor" stroke-width="3"/><text x="190" y="50">F浮</text><text x="80" y="230">浮力方向：竖直向上</text></svg></figure></template>
<style scoped>.teaching-chart{margin:0}.teaching-chart svg{display:block;width:100%;max-height:300px}.numeric-chart figcaption{font-size:12px;overflow-wrap:anywhere}.teaching-chart text{fill:currentColor;font-size:11px}</style>
