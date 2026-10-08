<script setup>
import {computed} from 'vue'
const props=defineProps({text:{type:String,default:''},terms:{type:Array,default:()=>[]}})
const keepUnits=text=>text.replace(/(\d(?:\.\d+)?)[ \u00a0]+((?:km\/h|m\/s[²2]?|mm|cm|km|ms|min|kg|mL|mol|Hz|Pa|s|h|m[²³]?|g|L|N|J|W|K|V|A|°C|℃|%)(?![A-Za-z]))/g,'$1\u00a0$2')
const pieces=computed(()=>{
 const text=keepUnits(props.text)
 const words=[...new Set(props.terms.filter(t=>typeof t==='string'&&t.trim()&&t.length<=40).map(keepUnits))].slice(0,6).sort((a,b)=>b.length-a.length)
 if(!words.length)return [{text,strong:false}]
 // Split as text nodes; highlighted teaching content never becomes raw HTML.
 const parts=[];let cursor=0
 while(cursor<text.length){
  let start=text.length,word=''
  for(const term of words){const i=text.indexOf(term,cursor);if(i>=0&&i<start){start=i;word=term}}
  if(start>cursor)parts.push({text:text.slice(cursor,start),strong:false})
  if(!word)break
  parts.push({text:word,strong:true});cursor=start+word.length
 }
 return parts
})
</script>
<template><template v-for="(part,i) in pieces" :key="i"><strong v-if="part.strong" class="teaching-emphasis">{{part.text}}</strong><template v-else>{{part.text}}</template></template></template>
<style>.teaching-emphasis{color:#b54822;font-weight:750}.midnight .teaching-emphasis{color:#ffd166}</style>
