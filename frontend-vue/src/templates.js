export const templates = [
  {id:'classroom',name:'明亮课堂',tag:'情境 · 精讲',description:'白底绿标题、黄色问题带与情境图表',mark:'07'},
  {id:'forest',name:'松间课堂',tag:'自然 · 探究',description:'开放留白与实验图解',mark:'01'},
  {id:'violet',name:'灵感研学',tag:'思辨 · 对比',description:'紫色分区与双栏对照',mark:'02'},
  {id:'ocean',name:'晴空理科',tag:'清晰 · 推导',description:'蓝色信息条与步骤讲解',mark:'03'},
  {id:'midnight',name:'深空实验室',tag:'科技 · 物理',description:'深色底与高对比重点',mark:'04'},
  {id:'paper',name:'书页讲堂',tag:'人文 · 数学',description:'暖纸底与编辑式排版',mark:'05'},
  {id:'coral',name:'活力互动',tag:'活泼 · 练习',description:'暖色题卡与互动提问',mark:'06'},
]
export const layouts = [['cover','课程导入'],['concept','核心概念'],['steps','步骤推导'],['comparison','双栏对比'],['example','例题解析'],['exercise','课堂练习'],['summary','知识总结']]
export function slideLayout(slide,index){return slide?.layout||(index===0?'cover':slide?.diagram||slide?.chart?'concept':'steps')}
