(() => {
'use strict';
// Offline teaching-animation player. Scene state is a pure function of global
// teaching time t (ms); audio only drives t while a sentence with audio plays.
const D = JSON.parse(document.getElementById('anim-data').textContent);
const S = D.script, T = D.timeline, svg = document.getElementById('stage'), NS = svg.namespaceURI;
const W = 1280, H = 720, HEAD = 96, SUB = 112, COLORS = ['#1767bf', '#b95b1c', '#138360', '#a34987'];
const $ = id => document.getElementById(id), clamp = (v, a, b) => Math.min(b, Math.max(a, v));
const ease = u => u < .5 ? 2 * u * u : 1 - 2 * (1 - u) * (1 - u);
const num = v => { const r = Math.round(v * 10) / 10; return String(Object.is(r, -0) ? 0 : r).replace('-', '−'); };
const featureNum = v => String(Number(v.toPrecision(4))).replace('-', '−');
const roundedFeature = v => Math.abs(v-Number(v.toPrecision(4)))>1e-7;
const sentences = [], subtitles = T.scenes.flatMap(s => s.subtitles || s.sentences), display = {};
T.scenes.forEach(s => s.sentences.forEach(x => sentences.push(x)));
S.scenes.forEach(s => s.narration.forEach(n => { display[n.id] = n.display; }));

function sceneAt(t) { const i = T.scenes.findIndex(s => t < s.end); return i < 0 ? T.scenes.length - 1 : i; }
function sentenceAt(t) { return sentences.find(s => t >= s.start && t < s.end) || null; }
function stateAt(t) {
  const i = sceneAt(t), sc = S.scenes[i], sp = T.scenes[i], st = { scene: i, objects: {}, sentence: sentenceAt(t), time: t };
  for (const o of sc.objects||[]) st.objects[o.id] = { params: { ...(o.params || {}) }, opacity: 1, emph: 0 };
  const order = (sc.actions||[]).map((a, j) => [a, sp.actions[j]]).sort((x, y) => x[1].start - y[1].start);
  const seen = {};
  for (const [a] of order) if ((a.op === 'show' || a.op === 'hide') && !seen[a.object]) {
    seen[a.object] = 1; if (st.objects[a.object]) st.objects[a.object].opacity = a.op === 'show' ? 0 : 1;
  }
  for (const [a, s] of order) {
    const o = st.objects[a.object]; if (!o || t < s.start) continue;
    const u = clamp((t - s.start) / Math.max(1, s.end - s.start), 0, 1), e = ease(u);
    if (a.op === 'show' || a.op === 'hide') o.opacity += ((a.op === 'show' ? 1 : 0) - o.opacity) * e;
    else if (a.op === 'tween') o.params[a.param] += (a.to - o.params[a.param]) * e;
    else if (a.op === 'emphasize' && t < s.end) o.emph = Math.max(o.emph, Math.sin(Math.PI * u));
  }
  return st;
}

function el(tag, attrs, parent, text) {
  const e = document.createElementNS(NS, tag);
  for (const k in attrs) e.setAttribute(k, attrs[k]);
  if (text != null) e.textContent = text;
  (parent || svg).appendChild(e); return e;
}
function formula(k, b) {
  const kr = Math.round(k * 10) / 10, br = Math.round(b * 10) / 10;
  let s = 'y = ' + (kr === 1 ? '' : kr === -1 ? '−' : num(kr)) + 'x';
  return br ? s + (br > 0 ? ' + ' + num(br) : ' − ' + num(-br)) : s;
}
function wrap(text, max) {
  const out = []; let line = '', w = 0;
  for (const ch of text) { const cw = /[\x00-\xff]/.test(ch) ? .56 : 1; if (w + cw > max && line) { out.push(line); line = ''; w = 0; } line += ch; w += cw; }
  if (line) out.push(line); return out;
}
function drawAxes(o, box, s) {
  const p = s.params, u = Math.min(box.w / (p.xmax - p.xmin), box.h / (p.ymax - p.ymin));
  const w = (p.xmax - p.xmin) * u, h = (p.ymax - p.ymin) * u, x0 = box.x + (box.w - w) / 2, y0 = box.y + (box.h - h) / 2;
  const X = x => x0 + (x - p.xmin) * u, Y = y => y0 + (p.ymax - y) * u, g = el('g', { opacity: s.opacity });
  // Tick labels go in their own group, re-appended above the lines with a halo so lines never hide them.
  const lab = el('g', { opacity: s.opacity, stroke: '#f5f8fc', 'stroke-width': 5, 'paint-order': 'stroke', 'stroke-linejoin': 'round' });
  const step = Math.max(p.xmax - p.xmin, p.ymax - p.ymin) > 12 ? 2 : 1;
  for (let x = Math.ceil(p.xmin); x <= p.xmax; x++) el('line', { x1: X(x), y1: y0, x2: X(x), y2: y0 + h, stroke: '#e1e8f1' }, g);
  for (let y = Math.ceil(p.ymin); y <= p.ymax; y++) el('line', { x1: x0, y1: Y(y), x2: x0 + w, y2: Y(y), stroke: '#e1e8f1' }, g);
  const ox = clamp(X(0), x0, x0 + w), oy = clamp(Y(0), y0, y0 + h);
  el('line', { x1: x0, y1: oy, x2: x0 + w, y2: oy, stroke: '#536d85', 'stroke-width': 2 }, g);
  el('line', { x1: ox, y1: y0, x2: ox, y2: y0 + h, stroke: '#536d85', 'stroke-width': 2 }, g);
  for (let x = Math.ceil(p.xmin / step) * step; x <= p.xmax; x += step) if (x) el('text', { x: X(x), y: Math.min(oy + 22, y0 + h - 4), 'font-size': 16, 'text-anchor': 'middle', fill: '#536d85' }, lab, num(x));
  for (let y = Math.ceil(p.ymin / step) * step; y <= p.ymax; y += step) if (y) el('text', { x: Math.max(ox - 10, x0 + 18), y: Y(y) + 6, 'font-size': 16, 'text-anchor': 'end', fill: '#536d85' }, lab, num(y));
  el('text', { x: x0 + w - 6, y: oy - 10, 'font-size': 20, 'text-anchor': 'end', fill: '#28445e', 'font-style': 'italic' }, g, 'x');
  el('text', { x: ox + 12, y: y0 + 22, 'font-size': 20, fill: '#28445e', 'font-style': 'italic' }, g, 'y');
  el('rect', { x: x0, y: y0, width: w, height: h }, el('clipPath', { id: 'plot' }));
  return { X, Y, p, lab };
}
const isShape = type => type === 'rectangle' || type === 'right_triangle';
const geoNum = v => String(Number(v.toFixed(2))).replace('-', '−');
function geometryPoints(type, p) {
  const points = type === 'rectangle' ? [[0,0],[p.width,0],[p.width,p.height],[0,p.height]] : [[0,0],[p.width,0],[0,p.height]];
  const angle = p.angle * Math.PI / 180, c = Math.cos(angle), s = Math.sin(angle);
  return points.map(([x,y]) => [p.cx+(x-p.cx)*c-(y-p.cy)*s+p.tx, p.cy+(x-p.cx)*s+(y-p.cy)*c+p.ty]);
}
function drawGeometry(o, m, state, color) {
  const p=state.params, points=geometryPoints(o.type,p), initial=geometryPoints(o.type,o.params);
  const pointsString=points=>points.map(([x,y])=>m.X(x)+','+m.Y(y)).join(' ');
  const g=el('g',{opacity:state.opacity}),labels=[];
  const label=(text,x,y,anchor,fill)=>{
    const node=el('text',{x,y,'text-anchor':anchor,'font-size':18,fill,stroke:'#f5f8fc','stroke-width':3,'paint-order':'stroke'},g,text);
    for(let attempt=0;attempt<12;attempt++){
      node.setAttribute('y',y+(attempt===0?0:Math.ceil(attempt/2)*24*(attempt%2?1:-1)));
      const box=node.getBBox(),left=m.X(m.p.xmin)-20,right=m.X(m.p.xmax)+20;
      if(box.x<left)node.setAttribute('x',Number(node.getAttribute('x'))+left-box.x);
      else if(box.x+box.width>right)node.setAttribute('x',Number(node.getAttribute('x'))+right-box.x-box.width);
      const b=node.getBBox();
      if(b.y>=HEAD&&b.y+b.height<=H-SUB&&!labels.some(a=>b.x<a.x+a.width+3&&b.x+b.width+3>a.x&&b.y<a.y+a.height+3&&b.y+b.height+3>a.y)){labels.push(b);return}
    }
    labels.push(node.getBBox());
  };
  el('polygon',{points:pointsString(initial),fill:'none',stroke:'#8298b0','stroke-width':2,'stroke-dasharray':'7 6','data-original':o.id},g);
  el('polygon',{points:pointsString(points),fill:color,'fill-opacity':.2,stroke:color,'stroke-width':3+3*state.emph,'stroke-linejoin':'round','data-shape':o.id,'data-vertices':JSON.stringify(points),'data-area':p.width*p.height/(o.type==='right_triangle'?2:1)},g);
  const center=points.reduce((a,v)=>[a[0]+v[0]/points.length,a[1]+v[1]/points.length],[0,0]);
  const ox=m.X(p.cx+p.tx),oy=m.Y(p.cy+p.ty),shared=points.findIndex(([x,y])=>Math.hypot(m.X(x)-ox,m.Y(y)-oy)<1);
  points.forEach(([x,y],i)=>{
    const right=x>=center[0],up=y>=center[1];
    el('circle',{cx:m.X(x),cy:m.Y(y),r:4,fill:color},g);
    label(String.fromCharCode(65+i)+(i===shared?' / O':''),m.X(x)+(right?10:-10),m.Y(y)+(up?-10:22),right?'start':'end',color);
  });

  el('circle',{cx:ox,cy:oy,r:5,fill:'#b8790c','data-pivot':o.id},g);
  if(shared<0)label('O',ox+10,oy+23,'start','#b8790c');
}
function drawSide(objs, box, st, colorOf) {
  let y = box.y + 8;
  for (const o of objs) {
    const s = st.objects[o.id], g = el('g', { opacity: s.opacity }); let lines, size;
    if (o.type === 'equation') {
      const obj=S.scenes[st.scene].objects.find(x=>x.id===o.line),l=st.objects[o.line].params,f={...l,kind:obj.kind};
      lines=wrap(obj.type==='function'?TeachingFunctions.expression(f):formula(l.k,l.b),22);size=obj.type==='function'?24:40;
      if(obj.type==='function') {
        const features=TeachingFunctions.features(f);
        if(Number.isFinite(features.axis))lines.push('对称轴 x '+(roundedFeature(features.axis)?'≈ ':'= ')+featureNum(features.axis));
        features.points.forEach(p=>lines.push(p.label+(roundedFeature(p.x)||roundedFeature(p.y)?' ≈ ':' ')+'('+featureNum(p.x)+', '+featureNum(p.y)+')'));
        if(obj.kind==='inverse')lines.push('x ≠ 0；与坐标轴无交点');
      }
    }
    else if (o.type === 'measure') { const shape=S.scenes[st.scene].objects.find(x=>x.id===o.shape),p=st.objects[o.shape].params,triangle=shape.type==='right_triangle';size=22;const area=Number((p.width*p.height/(triangle?2:1)).toFixed(5));lines=[triangle?'直角三角形':'矩形','底 '+geoNum(p.width)+' · 高 '+geoNum(p.height),'S = '+geoNum(p.width)+' × '+geoNum(p.height)+(triangle?' ÷ 2':'')+' = '+String(area),'面积 '+String(area)+' 平方单位','平移 ('+geoNum(p.tx)+', '+geoNum(p.ty)+') · 角度 '+geoNum(p.angle)+'°','中心 O ('+geoNum(p.cx+p.tx)+', '+geoNum(p.cy+p.ty)+')','虚线：本幕初始 · 实线：当前']; }
    else if (o.type === 'text') { size = 28; lines = wrap(o.text, Math.floor((box.w - 28) / size)); }
    else { size = 26; lines = []; o.items.forEach(it => wrap('• ' + it, Math.floor((box.w - 28) / size)).forEach(x => lines.push(x))); }
    const height = size * 1.3 * lines.length + 22;
    if (s.emph > 0) el('rect', { x: box.x, y, width: box.w, height, rx: 10, fill: '#fff4d6', stroke: '#b8790c', 'stroke-width': 3, opacity: s.emph }, g);
    lines.forEach((tx, i) => el('text', { x: box.x + 14, y: y + 11 + size * (1.3 * i + 1), 'font-size': size, fill: o.type === 'equation' ? colorOf[o.line] : '#28445e', 'data-box': 'side' }, g, tx));
    y += height + 16;
  }
}
// Layout is trusted code; model text is assigned only with textContent.
function drawIllustration(o, b, parent, st) {
  const g=el('g',{'data-illustration':o.id,'data-kind':o.kind},parent);
  if(o.kind==='uploaded'){
    if(D.picture){
      el('image',{x:b.x,y:b.y,width:b.w,height:b.h-28,href:D.picture.data_url,preserveAspectRatio:'xMidYMid meet',role:'img','aria-label':D.picture.caption},g);
      const source=Array.from('教师配图 · '+D.picture.source),limit=Math.max(5,Math.floor(b.w/14));
      const label=source.length>limit?source.slice(0,limit-1).join('')+'…':source.join('');
      const note=el('text',{x:b.x+b.w/2,y:b.y+b.h-6,'text-anchor':'middle','font-size':14,fill:'#536d85'},g,label);
      el('title',{},note,D.picture.caption+'；出处：'+D.picture.source);
    }return;
  }
  const v=el('svg',{x:b.x,y:b.y,width:b.w,height:b.h,viewBox:'0 0 620 280',preserveAspectRatio:'xMidYMid meet',role:'img','aria-label':o.text+'，教学示意，非真实比例'},g);
  const text=(x,y,s,c='#28445e')=>el('text',{x,y,'text-anchor':'middle','font-size':22,fill:c},v,s);
  const arrow=(d,x,y,r=0)=>{el('path',{d,fill:'none',stroke:'#1767bf','stroke-width':5,'stroke-linecap':'round'},v);el('path',{d:'M -9 -8 L 0 0 L -9 8',fill:'none',stroke:'#1767bf','stroke-width':5,transform:'translate('+x+' '+y+') rotate('+r+')'},v)};
  if(o.kind==='water_cycle'){
    el('rect',{x:0,y:0,width:620,height:280,rx:18,fill:'#ecf7ff'},v);
    el('circle',{cx:58,cy:48,r:25,fill:'#f3bf39'},v);
    el('path',{d:'M 0 223 Q 82 198 166 223 T 340 223 L 340 280 L 0 280 Z',fill:'#64b5e5'},v);
    el('path',{d:'M 300 240 L 448 110 L 620 240 L 620 280 L 300 280 Z',fill:'#b8d8bb'},v);
    el('path',{d:'M 412 143 L 448 110 L 490 146 L 462 136 L 447 145 L 434 133 Z',fill:'#fff'},v);
    [[260,65,42],[310,48,44],[357,67,40]].forEach(([cx,cy,r])=>el('circle',{cx,cy,r,fill:'#fff',stroke:'#b9d5e8','stroke-width':2},v));
    el('rect',{x:256,y:65,width:105,height:30,fill:'#fff'},v);
    [[373,105],[397,120],[420,100]].forEach(([x,y])=>el('path',{d:'M '+x+' '+y+' l -9 25',stroke:'#318ecc','stroke-width':5,'stroke-linecap':'round'},v));
    arrow('M 110 190 Q 100 110 218 75',218,75,-16);
    arrow('M 432 204 Q 392 249 280 245',280,245,180);
    text(90,139,'蒸发');text(305,30,'凝结');text(478,95,'降水');text(390,272,'汇流');text(117,264,'地表水');
    text(218,179,'水蒸气不可见','#536d85');
  }else if(o.kind==='water_states'){
    ['固态','液态','气态'].forEach((s,i)=>{const x=12+i*205;el('rect',{x,y:42,width:185,height:180,rx:16,fill:['#e7f1fd','#e5f5f1','#fff4dc'][i],stroke:'#bfd0e2','stroke-width':2},v);text(x+92,28,s);for(let j=0;j<12;j++){const cx=x+(i===2?22:42)+(j%4)*(i===2?47:27)+(i===1?Math.sin(j)*7:0),cy=(i===2?60:92)+Math.floor(j/4)*(i===2?65:29)+(i===1?Math.cos(j)*8:0);el('circle',{cx,cy,r:8,fill:['#377bc8','#178c99','#be881b'][i]},v)}});
    text(310,262,'结构示意 · 圆点不代表真实分子外观','#536d85');
  }else if(o.kind==='plant'){
    el('rect',{x:0,y:218,width:620,height:62,rx:10,fill:'#f1e8d8'},v);
    el('path',{d:'M 310 235 L 310 62',stroke:'#3d8d4c','stroke-width':12,'stroke-linecap':'round'},v);
    el('path',{d:'M 310 140 Q 202 134 212 66 Q 300 62 310 140 M 310 188 Q 420 174 418 112 Q 328 108 310 188',fill:'#6abd79',stroke:'#3d8d4c','stroke-width':3},v);
    ['M 310 229 L 275 267','M 310 232 L 338 269','M 310 231 L 308 277'].forEach(d=>el('path',{d,stroke:'#95744c','stroke-width':5,fill:'none'},v));
    [[225,95,117,75,'叶'],[312,170,480,165,'茎'],[301,251,145,251,'根']].forEach(([x,y,x2,y2,label])=>{el('line',{x1:x,y1:y,x2,y2,stroke:'#718c78','stroke-width':2},v);text(x2,y2-10,label)});
  }
}
function drawLesson(sc, st, area) {
  if(sc.objects.length===1&&sc.objects[0].type==='scientific'){
    const o=sc.objects[0],state=st.objects[o.id],bundle=D.science?.[sc.id+'/'+o.id];
    if(!bundle){el('text',{x:area.x+30,y:area.y+60,fill:'#a22','font-size':26},svg,'科学图示数据缺失，请重新生成');return}
    if(bundle.native&&o.scientific.kind==='scene'){TeachingDrawing.draw(el,svg,o.scientific,bundle,area,state,st.time,T.scenes[st.scene]);return}
    const frames=bundle.frames?.length?bundle.frames:[bundle.image],index=Math.round(clamp(state.params.progress||0,0,1)*(frames.length-1));
    el('image',{x:area.x,y:area.y,width:area.w,height:area.h,href:frames[index],preserveAspectRatio:'xMidYMid meet',opacity:state.opacity,'data-scientific':o.scientific.kind,'data-frame':index,'data-box':'science'},svg);
    return;
  }
  const count=sc.objects.length, gap=22, width=(area.w-gap*(count-1))/count;
  sc.objects.forEach((o,j)=>{
    const state=st.objects[o.id], x=area.x+j*(width+gap), color=COLORS[j%COLORS.length];
    const g=el('g',{opacity:state.opacity,'data-lesson':o.id,'data-layout':o.type,'data-bounds':[x,area.y,width,area.h].join(',')},svg);
    const title=wrap(o.text,Math.max(1,Math.floor((width-40)/28)));
    title.forEach((line,i)=>el('text',{x:x+20,y:area.y+42+i*44,'font-size':28,'font-weight':600,fill:'#18324c'},g,line));
    const top=area.y+54+title.length*44, bottom=area.y+area.h-20;
    const span=T.scenes[st.scene], cues=(o.item_sentences||[]).map(id=>span.sentences.find(n=>n.id===id));
    let box={x:x+16,y:top,w:width-32,h:bottom-top};
    if(o.type==='illustration'){
      if(count===1){drawIllustration(o,{x:x+12,y:top,w:width*.62,h:bottom-top},g,st);box={x:x+width*.65,y:top,w:width*.35-20,h:bottom-top}}
      else {const h=Math.min(145,(bottom-top)*.42);drawIllustration(o,{x:x+12,y:top,w:width-24,h},g,st);box.y+=h+14;box.h-=h+14}
    }
    const horizontal=(o.type==='timeline'||o.type==='relationship')&&count===1;
    const columns=count===1&&(o.type==='card'||o.type==='comparison')?Math.min(o.type==='comparison'?4:2,o.items.length):horizontal?o.items.length:1;
    const rows=Math.ceil(o.items.length/columns), gapX=horizontal?40:22, gapY=18;
    const w=(box.w-gapX*(columns-1))/columns, h=(box.h-gapY*(rows-1))/rows;
    o.items.forEach((item,i)=>{
      const col=i%columns,row=Math.floor(i/columns),ix=box.x+col*(w+gapX),cellY=box.y+row*(h+gapY);
      const tileH=Math.min(h,o.type==='comparison'?260:130),iy=cellY+(h-tileH)/2;
      const cue=cues[i], visible=!cue?1:ease(clamp((st.time-cue.start)/350,0,1)), focused=cue&&st.time>=cue.start&&st.time<cue.end;
      const node=el('g',{opacity:visible,'data-item':i,'data-active':!!focused,'data-sentence':cue?.id||'',transform:'translate(0 '+((1-visible)*10)+')'},g);
      if(horizontal&&i>0){
        const cy=iy+tileH/2;el('line',{x1:ix-gapX+5,y1:cy,x2:ix-8,y2:cy,stroke:color,'stroke-width':3},node);
        if(o.type==='relationship')el('path',{d:'M '+(ix-15)+' '+(cy-6)+' L '+(ix-8)+' '+cy+' L '+(ix-15)+' '+(cy+6),fill:'none',stroke:color,'stroke-width':3},node);
      }
      if(focused)el('line',{x1:ix+18,y1:iy+tileH-4,x2:ix+Math.min(w-18,150),y2:iy+tileH-4,stroke:color,'stroke-width':3},node);
      const ordered=o.type==='timeline'||o.type==='relationship';
      const stacked=ordered&&!horizontal;
      if(ordered){const cy=stacked?iy+tileH/2:iy+25;el('circle',{cx:ix+24,cy,r:13,fill:color},node);el('text',{x:ix+24,y:cy+5,'text-anchor':'middle','font-size':15,fill:'#fff'},node,String(i+1))}
      if(stacked&&i>0)el('line',{x1:ix+24,y1:iy-gapY,x2:ix+24,y2:iy,stroke:color,'stroke-width':2},node);
      const pad=stacked?50:18, availableH=tileH-(ordered&&!stacked?76:16), availableW=w-pad-18;let size=count===1?30:24;
      while(size>14&&wrap(item,Math.max(1,Math.floor(availableW/size))).length*(size*1.5)>availableH)size--;
      const lines=wrap(item,Math.max(1,Math.floor(availableW/size))),lineH=size*1.5;
      const y=ordered&&!stacked?iy+79:iy+(tileH-lines.length*lineH)/2+size;
      lines.forEach((line,k)=>el('text',{x:ix+pad,y:y+k*lineH,'font-size':size,'font-weight':focused?600:400,fill:focused?color:'#28445e','data-box':'lesson'},node,line));
      if(state.emph>0&&(!cue||focused))el('line',{x1:ix+14,y1:iy+tileH-6,x2:ix+w-14,y2:iy+tileH-6,stroke:'#b8790c','stroke-width':3,opacity:state.emph},node);
    });
  });
}
function render(t) {
  const st = stateAt(t), sc = S.scenes[st.scene]; svg.textContent = '';
  if(sc.html){svg.hidden=true;svg.style.display='none';TeachingDirect.render(sc,T.scenes[st.scene],t);return st}
  el('title', {}, svg, sc.title);
  el('rect', { x: 0, y: 0, width: W, height: H, fill: '#f5f8fc' });
  el('text', { x: 48, y: 62, 'font-size': 34, 'font-weight': 600, fill: '#18324c', 'data-box': 'head' }, svg, sc.title);
  const main = sc.objects.filter(o => o.slot === 'main'), side = sc.objects.filter(o => o.slot === 'side');
  const area = { x: 40, y: HEAD, w: W - 80, h: H - HEAD - SUB }; let mainBox = area, sideBox = null;
  if (main.length && side.length) { mainBox = { ...area, w: area.w * .64 }; sideBox = { x: area.x + area.w * .67, y: area.y, w: area.w * .33, h: area.h }; }
  else if (!main.length) sideBox = { x: area.x + area.w * .2, y: area.y + 40, w: area.w * .6, h: area.h - 40 };
  svg.setAttribute('data-side', sideBox ? [sideBox.x, sideBox.y, sideBox.w, sideBox.h].join(',') : '');
  const colorOf = {}; main.filter(o => o.type === 'line' || o.type === 'function' || isShape(o.type)).forEach((o, i) => { colorOf[o.id] = COLORS[i % COLORS.length]; });
  if(S.archetype==='lesson.explain@1')drawLesson(sc,st,area);
  const ax = main.find(o => o.type === 'axes'), m = ax && drawAxes(ax, mainBox, st.objects[ax.id]);
  if (m) for (const o of main) {
    const s = st.objects[o.id];
    if (isShape(o.type)) drawGeometry(o,m,s,colorOf[o.id]);
    else if (o.type === 'line') {
      const { k, b } = s.params;
      el('line', { x1: m.X(m.p.xmin), y1: m.Y(k * m.p.xmin + b), x2: m.X(m.p.xmax), y2: m.Y(k * m.p.xmax + b), stroke: colorOf[o.id], 'stroke-width': 4 + 4 * s.emph, 'stroke-linecap': 'round', 'clip-path': 'url(#plot)', opacity: s.opacity, 'data-line': o.id, 'data-k': k.toFixed(3) });
    } else if (o.type === 'function') {
      const f={...s.params,kind:o.kind},features=TeachingFunctions.features(f),color=colorOf[o.id];
      const g=el('g',{opacity:s.opacity,'clip-path':'url(#plot)'});
      el('path',{d:TeachingFunctions.path(f,m.p,m.X,m.Y),fill:'none',stroke:color,'stroke-width':4+4*s.emph,'data-function':o.id,'data-kind':o.kind,'data-params':JSON.stringify(s.params)},g);
      if(Number.isFinite(features.axis)) el('line',{x1:m.X(features.axis),x2:m.X(features.axis),y1:m.Y(m.p.ymin),y2:m.Y(m.p.ymax),stroke:color,'stroke-dasharray':'7 6','data-symmetry':features.axis},g);
      for(const pt of features.points) {
        if(pt.x<m.p.xmin||pt.x>m.p.xmax||pt.y<m.p.ymin||pt.y>m.p.ymax)continue;
        const dot=el('circle',{cx:m.X(pt.x),cy:m.Y(pt.y),r:5,fill:color,'data-feature':pt.label,'data-x':pt.x,'data-y':pt.y},g);
        el('title',{},dot,pt.label+' ('+num(pt.x)+', '+num(pt.y)+')');
      }
    } else if (o.type === 'point') {
      const l = st.objects[o.line],obj=sc.objects.find(x=>x.id===o.line),x = s.params.x, y = TeachingFunctions.value({...l.params,kind:obj.kind||'linear'},x),op = Math.min(s.opacity, l.opacity);
      if (!Number.isFinite(y)||x<m.p.xmin||x>m.p.xmax||y < m.p.ymin || y > m.p.ymax) continue;
      el('circle', { cx: m.X(x), cy: m.Y(y), r: 7 + 5 * s.emph, fill: colorOf[o.line], stroke: '#ffffff', 'stroke-width': 2, opacity: op });
      if (o.text) el('text', { x: m.X(x) + 14, y: m.Y(y) - 12, 'font-size': 20, fill: '#18324c', opacity: op }, svg, o.text);
    }
  }
  if (m) svg.appendChild(m.lab);
  if (sideBox) drawSide(side, sideBox, st, colorOf);
  return st;
}
// Controls. Auto play: await holds last their fixed ms. Step mode: time freezes at each await until 继续.
const P = { t: 0, playing: false, step: false, waiting: null, subs: true, last: 0, audio: null, audioFor: null, stall: 0 };
const awaits = []; T.scenes.forEach(s => (s.holds||[]).forEach(h => { if (h.type === 'await') awaits.push(h); }));
const fmt = ms => { const s = Math.floor(ms / 1000); return Math.floor(s / 60) + ':' + String(s % 60).padStart(2, '0'); };
function stopAudio() { if (P.audio) P.audio.pause(); P.audio = P.audioFor = null; }
function audioFailure(){
 P.playing=false;stopAudio();let error=$('audio-error');if(!error){error=document.createElement('p');error.id='audio-error';error.setAttribute('role','alert');document.querySelector('header').appendChild(error)}
 error.textContent='语音暂时无法播放，已暂停。请点击播放重试。';draw();
}
function syncAudio(sen) {
  // Inside a sentence with audio, its playback position defines t; elsewhere the monotonic clock does.
  if (!P.playing || !sen || !D.audio[sen.id]) { stopAudio(); return false; }
  if (P.audioFor !== sen.id) {
    stopAudio(); const a = new Audio(D.audio[sen.id]); P.audio = a; P.audioFor = sen.id; P.stall = 0;
    a.currentTime = Math.max(0, (P.t - sen.start) / 1000);
    a.addEventListener('ended', () => { if (P.audioFor === sen.id) { P.t = Math.max(P.t, sen.end); stopAudio(); } });
    a.addEventListener('error',()=>{if(P.audio===a)audioFailure()});
    a.play().then(()=>{if(P.audio===a)$('audio-error')?.remove()}).catch(()=>{if(P.audio===a)audioFailure()});
  }
  return true;
}
function frame(now) {
  const dt = Math.min(250, now - P.last); P.last = now;
  if (P.playing && !P.waiting) {
    const sen = sentenceAt(P.t);
    if (syncAudio(sen) && P.audio) {
      const at = sen.start + P.audio.currentTime * 1000;
      // Audio remains the only clock inside a narrated sentence, including buffering.
      if (at > P.t) { P.t = at; P.stall = 0; } else if ((P.stall += dt) > 5000) audioFailure();
    } else P.t += dt;
    if (P.step) {
      const h = awaits.find(h => P.t >= h.start && P.t - dt < h.start + 1);
      if (h) { P.t = h.start; P.waiting = h; stopAudio(); }
    }
    if (P.t >= T.total_ms) { P.t = T.total_ms; P.playing = false; stopAudio(); }
  }
  draw(); requestAnimationFrame(frame);
}
function draw() {
  const st = render(Math.min(P.t, T.total_ms - 1)), sub = $('subtitle');
  sub.textContent = ''; const sen = P.t < T.total_ms ? subtitles.find(s => P.t >= s.start && P.t < s.end) : null;
  if (sen) { const span = document.createElement('span'); span.textContent = display[sen.id]; sub.appendChild(span); }
  sub.classList.toggle('off', !P.subs);
  $('seek').value = String(Math.round(P.t)); $('seek').setAttribute('aria-valuetext', fmt(P.t) + '，第' + (st.scene + 1) + '幕');
  $('time').textContent = fmt(P.t) + ' / ' + fmt(T.total_ms);
  $('play').textContent = P.playing ? '暂停' : '播放'; $('play').setAttribute('aria-pressed', String(P.playing));
  $('continue').hidden = !P.waiting; $('scene').textContent = '第 ' + (st.scene + 1) + ' / ' + T.scenes.length + ' 幕';
}
function seek(t) { stopAudio(); P.waiting = null; P.t = clamp(t, 0, T.total_ms); draw(); }
function toggle() {
  if (P.waiting) return resume();
  if (!P.playing && P.t >= T.total_ms) P.t = 0;
  P.playing = !P.playing; if (!P.playing) stopAudio(); draw();
}
function resume() { if (!P.waiting) return; P.t = P.waiting.end; P.waiting = null; P.playing = true; draw(); }
function goScene(d) { const i = clamp(sceneAt(P.t) + d, 0, T.scenes.length - 1); seek(T.scenes[i].start); }
$('seek').max = String(T.total_ms);
$('seek').addEventListener('input', e => seek(Number(e.target.value)));
$('play').addEventListener('click', toggle);
$('continue').addEventListener('click', resume);
$('prev').addEventListener('click', () => goScene(-1));
$('next').addEventListener('click', () => goScene(1));
$('replay').addEventListener('click', () => { seek(0); P.playing = true; draw(); });
$('mode').addEventListener('click', () => { P.step = !P.step; if (!P.step && P.waiting) resume(); $('mode').setAttribute('aria-pressed', String(P.step)); });
$('subs').addEventListener('click', () => { P.subs = !P.subs; $('subs').setAttribute('aria-pressed', String(P.subs)); draw(); });
document.addEventListener('keydown', e => {
  if (e.target.closest && e.target.closest('button,input')) return;
  if (e.key === ' ') { e.preventDefault(); toggle(); }
  else if (e.key === 'ArrowLeft') seek(P.t - 5000); else if (e.key === 'ArrowRight') seek(P.t + 5000);
});
window.__anim = { stateAt, render, sentenceAt, seek, toggle, resume, timeline: T, player: P, step: v => { P.step = v; $('mode').setAttribute('aria-pressed', String(v)); } };
addEventListener('teaching-direct-error',()=>{P.playing=false;stopAudio()});
window.__anim.ready=()=>TeachingDirect.wait();window.__anim.authoredState=()=>TeachingDirect.state();
window.__hfWaitForSeekCompletion=()=>TeachingDirect.wait();
draw(); P.last = performance.now(); requestAnimationFrame(frame);
})();
