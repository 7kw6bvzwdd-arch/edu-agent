// Match the existing save API's title/bullet/focus limits; rendering still
// decides whether the actual text fits without silently removing content.
export function canvasTextIssue(slide){
 const length=text=>Array.from(text||'').length
 if(!slide.title?.trim())return '标题不能为空。'
 if(length(slide.title)>70)return '标题最多70字，请缩短后保存。'
 if((slide.bullets||[]).length>6)return '正文最多6条要点，请合并或拆分页面。'
 if((slide.bullets||[]).some(line=>length(line)>160))return '每条正文最多160字，详细推导可移到讲解备注。'
 if(length(slide.focus)>60)return '课堂提问最多60字，请缩短后保存。'
 for(const object of slide.canvas_objects||[]){
  if(object.kind==='text'&&(!object.text?.trim()||length(object.text)>600))return '新增文本框不能为空，最多600字。'
  if(object.kind==='image'&&length(object.caption)>60)return '图片说明最多60字。'
 }
 return ''
}
