// Trusted deterministic renderer. All geometry comes from validated data; no author code.
const TeachingDrawing = (() => {
  const clamp = v => Math.max(0, Math.min(1, v));
  const smooth = v => {v=clamp(v);return v*v*(3-2*v)};
  function along(points,u) {
    const lengths=points.slice(1).map((p,i)=>Math.hypot(p[0]-points[i][0],p[1]-points[i][1]));
    let d=lengths.reduce((a,b)=>a+b,0)*u;
    for(let i=0;i<lengths.length;i++){if(d<=lengths[i]&&lengths[i]){const t=d/lengths[i];return points[i].map((v,k)=>v+(points[i+1][k]-v)*t)}d-=lengths[i]}
    return points.at(-1);
  }
  function states(scene,progress,time,span) {
    const objects=Object.fromEntries(scene.objects.map(o=>[o.id,o])),state={};
    for(const o of scene.objects){
      const center=o.points?.length?[0,1].map(k=>o.points.reduce((v,p)=>v+p[k],0)/o.points.length):[o.x??.5,o.y??.5];
      state[o.id]={x:center[0],y:center[1],center,angle:0,scale:1,opacity:1,emphasis:0,reveal:1,slide:0};
      if(scene.actions?.some(a=>a.object===o.id&&a.op==='reveal'))state[o.id].reveal=0;
    }
    const seen=new Set();
    for(const a of scene.actions||[])if(['show','hide'].includes(a.op)&&!seen.has(a.object)){state[a.object].opacity=a.op==='show'?0:1;seen.add(a.object)}
    const timed=(scene.actions||[]).map((a,i)=>{
      const cue=a.sentence&&(span.subtitles||span.sentences).find(s=>s.id===a.sentence);
      return {a,i,t:cue?time-cue.start:progress*scene.duration_ms-a.start_ms,duration:cue?Math.min(a.duration_ms,cue.end-cue.start):a.duration_ms,start:cue?cue.start:span.start+a.start_ms};
    }).sort((a,b)=>a.start-b.start||a.i-b.i);
    for(const {a,t,duration} of timed){
      if(t<0)continue;const s=state[a.object],u=smooth(t/Math.max(1,duration));
      if(a.op==='move'){s.x+=(a.x-s.x)*u;s.y+=(a.y-s.y)*u}
      else if(a.op==='follow_path')[s.x,s.y]=along(objects[a.path].points,u);
      else if(['rotate','scale','reveal','slide'].includes(a.op)){const key=a.op==='rotate'?'angle':a.op;s[key]+=(a.to-s[key])*u}
      else if(a.op==='show'||a.op==='hide')s.opacity+=((a.op==='show'?1:0)-s.opacity)*u;
      else if(a.op==='emphasize'&&t<duration)s.emphasis=Math.sin(Math.PI*u);
    }
    for(const o of scene.objects)if(o.attach_to){const s=state[o.id],target=state[o.attach_to];s.x+=target.x-target.center[0];s.y+=target.y-target.center[1];s.opacity*=target.opacity}
    return state;
  }
  function carbon(el,g,o,s,model,box){
    if(!model?.points||!model.edges)throw Error('Missing frozen carbon geometry');
    const points=model.points,kind=o.allotrope,angle=.52+s.angle*Math.PI/180,c=Math.cos(angle),sn=Math.sin(angle);
    const ex=Math.max(...points.map(([x,y,z])=>Math.hypot(x,y)+(kind==='graphite'?.9*Math.abs(z):0)));
    const ey=Math.max(...points.map(([x,y,z])=>Math.hypot(x,y)*(kind==='graphite'?.34:.45)+Math.abs(z)*(kind==='graphite'?1.65:.89)));
    const width=(o.width||.12)*box.w*s.scale,height=(o.height||.12)*box.h*s.scale;
    const scale=Math.min(width/(2*ex+.25),height/(2*ey+.25));
    const projected=points.map(([x,y,z])=>{const a=x*c+y*sn,b=-x*sn+y*c;return kind==='graphite'?[a+z*s.slide*.9,b*.34-z*1.65,z+b*.01]:[a,b*.45-z*.89,b*.89+z*.45]}).map(([x,y,z])=>[box.x+s.x*box.w+x*scale,box.y+s.y*box.h+y*scale,z]);
    const order=points.map((p,i)=>({i,d:Math.hypot(...p.map((v,k)=>v-points[0][k]))})).sort((a,b)=>a.d-b.d||a.i-b.i),rank={};order.forEach((p,i)=>rank[p.i]=i);
    const alpha=i=>clamp(s.reveal*points.length-rank[i]);
    [...model.edges].sort((a,b)=>a.reduce((n,i)=>n+projected[i][2],0)-b.reduce((n,i)=>n+projected[i][2],0)).forEach(([i,j])=>{
      const p=projected[i],q=projected[j];el('line',{x1:p[0],y1:p[1],x2:q[0],y2:q[1],stroke:s.emphasis>.05?'#bc582b':kind==='diamond'&&(i===0||j===0)?'#dc8b29':'#6885a0','stroke-width':Math.max(2,scale*.045)+3*s.emphasis,opacity:Math.min(alpha(i),alpha(j)),'data-bond':i+'-'+j},g);
    });
    projected.map((p,i)=>({p,i})).sort((a,b)=>a.p[2]-b.p[2]).forEach(({p,i})=>el('circle',{cx:p[0],cy:p[1],r:Math.max(3,Math.min(13,scale*(kind==='fullerene'?.055:.075))),fill:kind==='diamond'&&i===0?'#de932e':'#197bad',stroke:'white','stroke-width':1,opacity:alpha(i),'data-atom':i},g));
  }
  function draw(el,svg,diagram,bundle,area,outer,time,span){
    const scene=diagram.scene,ss=states(scene,outer.params.progress||0,time,span);
    const root=el('g',{'data-scientific':'scene','data-native':'true',opacity:outer.opacity,'data-box':'science'},svg);
    // Keep the same normalized educational canvas as the preview/PPTX renderer.
    const composed=scene.objects.some(o=>o.type==='carbon'),height=composed?400:620;
    const factor=Math.min(area.w/1000,area.h/height),left=area.x+(area.w-1000*factor)/2,top=area.y+(area.h-height*factor)/2;
    const box={x:left+30*factor,y:top+(composed?65:85)*factor,w:940*factor,h:(composed?270:470)*factor};
    el('text',{x:left+30*factor,y:top+40*factor,'font-size':30*factor,fill:'#1767bf'},root,diagram.title||'');
    for(const o of [...scene.objects].sort((a,b)=>(a.type==='text')-(b.type==='text'))){
      const s=ss[o.id],g=el('g',{'data-object':o.id,'data-kind':o.type,'data-state':JSON.stringify(s),opacity:s.opacity},root);
      if(o.type==='carbon'){carbon(el,g,o,s,bundle.metadata.models[o.id],box);continue}
      const cx=box.x+s.x*box.w,cy=box.y+s.y*box.h,stroke=s.emphasis>.05?'#bc582b':o.color||'#1767bf';
      const attrs={stroke,'stroke-width':(o.stroke_width||3)+4*s.emphasis,fill:o.fill||'none',transform:`translate(${cx} ${cy}) rotate(${s.angle}) scale(${s.scale})`};
      if(o.type==='text'){el('text',{x:cx,y:cy,'text-anchor':'middle','dominant-baseline':'central','font-size':(o.font_size||26)*factor*s.scale,fill:stroke,transform:`rotate(${s.angle} ${cx} ${cy})`},g,o.text);continue}
      const width=(o.width||.12)*box.w,height=(o.height||.12)*box.h;
      if(o.type==='rect')el('rect',{...attrs,x:-width/2,y:-height/2,width,height},g);
      else if(o.type==='ellipse')el('ellipse',{...attrs,cx:0,cy:0,rx:width/2,ry:height/2},g);
      else {
        const pts=o.points.map(p=>[(p[0]-s.center[0])*box.w,(p[1]-s.center[1])*box.h]);
        el(o.type==='polygon'?'polygon':'polyline',{...attrs,points:pts.map(p=>p.join(',')).join(' '),'stroke-linejoin':'round'},g);
        if(o.type==='arrow'){const p=pts.at(-2),q=pts.at(-1),a=Math.atan2(q[1]-p[1],q[0]-p[0]),r=12+(o.stroke_width||3);el('polygon',{...attrs,fill:stroke,points:[q,[q[0]-r*Math.cos(a-.45),q[1]-r*Math.sin(a-.45)],[q[0]-r*Math.cos(a+.45),q[1]-r*Math.sin(a+.45)]].map(p=>p.join(',')).join(' ')},g)}
      }
    }
    el('text',{x:area.x+30,y:area.y+area.h-14,'font-size':19,fill:'#60758a'},root,'教学示意 · 非真实比例；结构变化与观察视角按旁白区分');
  }
  return {draw,states};
})();
