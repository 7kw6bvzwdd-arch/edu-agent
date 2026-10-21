// Offline HTML boundary: visible state, continuous motion, geometry and narration.
const fs=require('node:fs/promises'),path=require('node:path'),assert=require('node:assert/strict');
const {pathToFileURL}=require('node:url');
const puppeteer=require('../animation-video/node_modules/puppeteer-core');
(async()=>{
 const file=path.resolve(process.argv[2]),out=path.resolve(process.argv[3]);await fs.mkdir(out,{recursive:true});
 const browser=await puppeteer.launch({executablePath:'C:/Program Files (x86)/Microsoft/Edge/Application/msedge.exe',headless:true,args:['--disable-background-networking'],defaultViewport:{width:1440,height:1000}});
 try{
  const page=await browser.newPage(),errors=[],network=[];page.on('pageerror',e=>errors.push(e.message));page.on('request',r=>{if(!/^(file|data|blob):/.test(r.url()))network.push(r.url())});await page.setOfflineMode(true);await page.goto(pathToFileURL(file).href);await page.waitForFunction(()=>!!window.__anim);
  const result=await page.evaluate(()=>{
   const d=JSON.parse(document.getElementById('anim-data').textContent),checks=[],shots=[],bad=[];
   const capture=(t,id)=>{__anim.seek(t);const e=document.querySelector('[data-object="'+id+'"]');if(!e)return null;const clone=e.cloneNode(true);clone.removeAttribute('data-state');return {html:clone.outerHTML,state:JSON.parse(e.dataset.state),atoms:[...e.querySelectorAll('[data-atom]')].map(a=>({id:a.dataset.atom,x:+a.getAttribute('cx'),y:+a.getAttribute('cy'),opacity:+a.getAttribute('opacity')}))}};
   for(const sc of d.script.scenes){const span=d.timeline.scenes.find(s=>s.id===sc.id);
    for(const o of sc.objects){if(o.scientific?.kind!=='scene'||!d.science[sc.id+'/'+o.id]?.native)continue;
     const diagram=o.scientific,outer=sc.actions.find(a=>a.object===o.id&&a.op==='tween'),outerSpan=span.actions.find(a=>a.id===outer?.id);
     for(const [i,a]of diagram.scene.actions.entries()){
      const cue=(span.subtitles||span.sentences).find(n=>n.id===a.sentence),start=cue?cue.start:outerSpan.start+a.start_ms/diagram.scene.duration_ms*(outerSpan.end-outerSpan.start),duration=cue?Math.min(a.duration_ms,cue.end-cue.start):a.duration_ms/diagram.scene.duration_ms*(outerSpan.end-outerSpan.start);
      const times=Array.from({length:17},(_,j)=>Math.min(span.end-1,start+duration*j/16)),states=times.map(t=>capture(t,a.object));
      if(states.some(s=>!s)){bad.push([sc.id,a.object,'missing object']);continue}
      const first=states[0],last=states.at(-1),changed=new Set(states.map(s=>s.html)).size;
      checks.push({scene:sc.id,object:a.object,op:a.op,sentence:a.sentence||null,distinctFrames:changed,reverseSeek:capture(times[0],a.object).html===first.html});
      if(!checks.at(-1).reverseSeek)bad.push([sc.id,a.object,'backward seek']);
      if(['reveal','slide','move','follow_path','rotate','scale'].includes(a.op)&&changed<12)bad.push([sc.id,a.object,'motion quantized or absent',changed]);
      if(cue&&a.op==='reveal'&&capture(Math.max(span.start,start-1),a.object).atoms.some(p=>p.opacity>0))bad.push([sc.id,a.object,'revealed before sentence']);
      if(a.op==='reveal'&&!(last.atoms.filter(p=>p.opacity>.9).length>first.atoms.filter(p=>p.opacity>.9).length))bad.push([sc.id,a.object,'no progressive structure']);
      if(a.op==='slide'){
       const geometry=d.science[sc.id+'/'+o.id].metadata.models[a.object],atom=(s,id)=>s.atoms.find(p=>+p.id===id);
       for(const [i,j]of geometry.edges){const dist=s=>{const p=atom(s,i),q=atom(s,j);return Math.hypot(p.x-q.x,p.y-q.y)};if(Math.abs(dist(first)-dist(last))>1e-6){bad.push([sc.id,'intralayer bond changed']);break}}
       const deltas=geometry.points.map((p,i)=>{const f=atom(first,i),l=atom(last,i);return[p[2],l.x-f.x]});
       if(!(Math.min(...deltas.map(p=>p[1]))<0&&Math.max(...deltas.map(p=>p[1]))>0))bad.push([sc.id,'layers did not move relatively']);
      }
      for(const [j,t]of [times[0],times[8],times[16]].entries())shots.push({name:sc.id+'-'+i+'-'+j,time:t});
     }
    }
    for(let j=0;j<=16;j++){__anim.seek(span.start+(span.end-span.start-1)*j/16);const texts=[...document.querySelectorAll('[data-native] text')].filter(e=>+e.parentNode.getAttribute('opacity')!==0);
     for(let i=0;i<texts.length;i++){const r=texts[i].getBBox();if(r.x<40||r.x+r.width>1240||r.y<96||r.y+r.height>608)bad.push([sc.id,'text outside',texts[i].textContent]);for(let k=0;k<i;k++){const b=texts[k].getBBox();if(r.x<b.x+b.width-1&&r.x+r.width>b.x+1&&r.y<b.y+b.height-1&&r.y+r.height>b.y+1)bad.push([sc.id,'text overlap',texts[i].textContent,texts[k].textContent])}}
     for(const atom of document.querySelectorAll('[data-atom]')){if(+atom.getAttribute('opacity')===0)continue;const r=atom.getBBox();if(r.x<40||r.x+r.width>1240||r.y<96||r.y+r.height>608)bad.push([sc.id,'atom outside'])}
    }
   }
   return {mode:d.voice,renderer:d.renderer,totalMS:d.timeline.total_ms,audioTracks:Object.keys(d.audio).length,checks,shots,bad};
  });
  for(const shot of result.shots){await page.evaluate(t=>__anim.seek(t),shot.time);await (await page.$('.frame')).screenshot({path:path.join(out,shot.name+'.png')})}
  await fs.writeFile(path.join(out,'report.json'),JSON.stringify({...result,errors,network},null,2));console.log(JSON.stringify({...result,shots:result.shots.length}));
  assert.ok(result.checks.length,'native animations were actually exercised');assert.deepEqual(result.bad,[]);assert.deepEqual(errors,[]);assert.deepEqual(network,[]);
 }finally{await browser.close()}
})().catch(e=>{console.error(e);process.exitCode=1});
