<script setup>
import {computed} from 'vue';
const props=defineProps({modelValue:String,readonly:Boolean});
const emit=defineEmits(['update:modelValue']);
const labels=computed(()=>{
 const doc=new DOMParser().parseFromString(props.modelValue||'','text/html');
 return [...doc.querySelectorAll('[data-editable]')].map(el=>({id:el.getAttribute('data-editable'),text:el.textContent}));
});
function change(id,text){
 const doc=new DOMParser().parseFromString(props.modelValue||'','text/html');
 const el=[...doc.querySelectorAll('[data-editable]')].find(el=>el.getAttribute('data-editable')===id);
 if(!el)return;el.textContent=text;
 emit('update:modelValue','<!doctype html>'+doc.documentElement.outerHTML);
}
</script>
<template>
 <div class="authored-page-editor">
  <p>页面由 AI 直接绘制。可在这里改画面文字，改动的标注会合并为普通文字；调整图形、布局或保留混合文字样式，请向助手描述需要修改的这一页。保存后更新实际 PPT 预览。</p>
  <label v-for="label in labels" :key="label.id">画面文字 · {{label.id}}
   <textarea :value="label.text" :readonly="readonly" rows="2" @input="change(label.id,$event.target.value)"></textarea>
  </label>
 </div>
</template>
<style scoped>
.authored-page-editor{display:grid;gap:12px}.authored-page-editor p{line-height:1.7}.authored-page-editor label{display:grid;gap:6px}.authored-page-editor textarea{font:inherit;min-height:60px;width:100%}
</style>
