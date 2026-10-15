export function answerSteps(answer='') {
 return answer.split(/\r?\n/).map(s=>s.trim()).filter(Boolean)
}
export function showGraphCoordinates(slide) {
 return slide.layout!=='exercise'||!/(?:读出|求出|求|确定)[^。！？?\n]{0,12}坐标/.test([slide.title,...(slide.bullets||[])].join('。'))
}
