const createCDP = require('./browser-cdp.cjs');
// Opens a built offline animation package in headless Edge with the network
// disabled and checks playback behaviour. Usage: node scripts/browser-animation-package.cjs <package.html>
const fs=require('node:fs/promises'),path=require('node:path'),assert=require('node:assert/strict');
const {spawn}=require('node:child_process'),{pathToFileURL}=require('node:url');
const root=path.resolve(__dirname,'..'),sleep=ms=>new Promise(r=>setTimeout(r,ms));
const file=path.resolve(process.argv[2]||path.join(root,'work/anim-v2/linear-k-tone.html'));
let browser,socket;
(async()=>{
 const output=path.resolve(process.argv[3]||path.join(root,'work/anim-v2/browser'));await fs.mkdir(output,{recursive:true});
 browser=spawn('C:/Program Files (x86)/Microsoft/Edge/Application/msedge.exe',['--headless=new','--disable-gpu','--no-first-run','--disable-background-networking','--autoplay-policy=no-user-gesture-required','--remote-debugging-port=9271','--remote-debugging-address=127.0.0.1','--user-data-dir='+path.join(output,'profile'),'about:blank'],{windowsHide:true,stdio:'ignore'});
 let page;
 for(let i=0;i<40;i++){try{page=(await fetch('http://127.0.0.1:9271/json/list').then(r=>r.json())).find(p=>p.type==='page');if(page)break}catch{}await sleep(200)}
 assert.ok(page,'Edge starts');socket=new WebSocket(page.webSocketDebuggerUrl);
 await new Promise((resolve,reject)=>{socket.onopen=resolve;socket.onerror=reject});
 const errors=[],requests=[],checks=[];
 const { call, evaluate: ev } = createCDP(socket, m => {
  if(m.method==='Runtime.exceptionThrown')errors.push(m.params.exceptionDetails.exception?.description||m.params.exceptionDetails.text);
  if(m.method==='Log.entryAdded'&&m.params.entry.level==='error')errors.push(m.params.entry.text);
  if(m.method==='Network.requestWillBeSent'){const u=m.params.request.url;if(!u.startsWith('data:')&&!u.startsWith('file:'))requests.push(u)}
 });
 const ok=(name,cond,detail)=>{checks.push({name,pass:!!cond,detail});if(!cond)console.error('FAIL',name,detail??'')};
 await call('Runtime.enable');await call('Log.enable');await call('Network.enable');await call('Page.enable');
 await call('Network.emulateNetworkConditions',{offline:true,latency:0,downloadThroughput:0,uploadThroughput:0});
 await call('Emulation.setDeviceMetricsOverride',{width:1440,height:900,deviceScaleFactor:1,mobile:false});
 await call('Page.navigate',{url:pathToFileURL(file).href});
 for(let i=0;i<50&&!(await ev('!!window.__anim').catch(()=>false));i++)await sleep(100);
 ok('player boots offline under CSP',await ev('!!window.__anim'),errors);
 const palette=await ev(`(()=>{__anim.render(1000);const rgb=getComputedStyle(document.body).backgroundColor.match(/[0-9]+/g).map(Number);return{rgb,stage:document.querySelector('#stage>rect')?.getAttribute('fill')}})()`);
 ok('bright classroom background',palette.rgb.slice(0,3).every(n=>n>=235)&&palette.stage==='#f5f8fc',palette);
 const illustrated=await ev(`JSON.parse(document.getElementById('anim-data').textContent).script.scenes.some(s=>s.objects.some(o=>o.type==='illustration'))`);
 if(illustrated){const diagrams=await ev(`(()=>{const d=JSON.parse(document.getElementById('anim-data').textContent),bad=[];let count=0;for(const s of d.script.scenes){const ts=d.timeline.scenes.find(t=>t.id===s.id);__anim.render(ts.end-1);for(const o of s.objects.filter(o=>o.type==='illustration')){const g=document.querySelector('[data-illustration='+o.id+']');if(!g||g.querySelectorAll('path,circle,ellipse,rect,line,polygon,image').length<1)bad.push(s.id);else count++}}return{count,bad}})()`);ok('illustrations draw real teaching shapes across scenes',diagrams.count>=1&&diagrams.bad.length===0,diagrams)}
 const T=await ev('__anim.timeline');
 const total=T.total_ms;ok('timeline length 30s–3min',total>=30000&&total<=180000,total);

 // Text stays inside the 1280x720 stage and inside the side column, sampled across every scene.
 const overflow=await ev(`(()=>{const bad=[],svg=document.getElementById('stage');
  for(const s of __anim.timeline.scenes)for(let f=0;f<=10;f++){const t=s.start+(s.end-s.start-1)*f/10;__anim.render(t);
   const side=(svg.getAttribute('data-side')||'').split(',').map(Number);
   for(const el of svg.querySelectorAll('text')){const b=el.getBBox();
    if(b.x<0||b.y<0||b.x+b.width>1280||b.y+b.height>720)bad.push([s.id,el.textContent,'stage']);
    if(el.dataset.box==='side'&&side.length===4&&(b.x+b.width>side[0]+side[2]+1||b.y+b.height>side[1]+side[3]+1))bad.push([s.id,el.textContent,'side'])}}
  return bad})()`);
 ok('no text overflow at 1440x900 across all scenes',overflow.length===0,overflow.slice(0,5));
 await call('Emulation.setDeviceMetricsOverride',{width:1920,height:1080,deviceScaleFactor:1,mobile:false});
 const sub=await ev(`(()=>{__anim.seek(${(T.scenes[1].subtitles||T.scenes[1].sentences).at(-1).start+50});const f=document.querySelector('.frame').getBoundingClientRect(),s=document.getElementById('subtitle').getBoundingClientRect();return{inside:s.left>=f.left&&s.right<=f.right&&s.bottom<=f.bottom,lines:Math.round(s.height/parseFloat(getComputedStyle(document.getElementById('subtitle')).lineHeight))}})()`);
 ok('subtitle inside frame at 1920x1080',sub.inside&&sub.lines<=2,sub);

 // Determinism: render(t) after an unrelated seek equals a direct render(t).
 const rep=T.scenes.find(s=>(s.actions||[]).some(a=>a.type==='tween'))||T.scenes[1],tw=rep.actions.find(a=>a.type==='tween')||rep.actions[0];
 const mid=Math.round((tw.start+tw.end)/2);
 const same=await ev(`(()=>{__anim.seek(${mid});const a=document.getElementById('stage').innerHTML;__anim.seek(${total-10});__anim.seek(0);__anim.seek(${mid});return a===document.getElementById('stage').innerHTML})()`);
 ok('state is a pure function of t (seek/replay restore)',same);
 const geometry=await ev('JSON.parse(document.getElementById("anim-data").textContent).script.archetype==="geo.transform@1"');
 if(geometry){
  const result=await ev(`(()=>{const D=JSON.parse(document.getElementById('anim-data').textContent),bad=[];let moved=false;
   for(const span of D.timeline.scenes){const sc=D.script.scenes.find(s=>s.id===span.id),shape=sc.objects.find(o=>['rectangle','right_triangle'].includes(o.type)),ax=sc.objects.find(o=>o.type==='axes').params;
    const expectedArea=shape.params.width*shape.params.height/(shape.type==='right_triangle'?2:1);let before;
    for(let f=0;f<=40;f++){const t=span.start+(span.end-span.start-1)*f/40;__anim.seek(t);const polygon=document.querySelector('[data-shape]'),v=JSON.parse(polygon.dataset.vertices);
     let area=0;for(let i=0;i<v.length;i++){const b=v[(i+1)%v.length];area+=v[i][0]*b[1]-b[0]*v[i][1]}
     if(Math.abs(Math.abs(area/2)-expectedArea)>1e-7)bad.push([span.id,'area']);
     const points=Array.from(polygon.points,p=>[p.x,p.y]);let scale;for(let i=0;i<v.length;i++){const a=v[i],b=v[(i+1)%v.length],pa=points[i],pb=points[(i+1)%v.length];const ratio=Math.hypot(pb[0]-pa[0],pb[1]-pa[1])/Math.hypot(b[0]-a[0],b[1]-a[1]);if(i===0)scale=ratio;else if(Math.abs(ratio-scale)>1e-4)bad.push([span.id,'unequal axis scale'])}
     if(v.some(([x,y])=>x<ax.xmin||x>ax.xmax||y<ax.ymin||y>ax.ymax))bad.push([span.id,'clipped']);
     if(!document.querySelector('[data-original]')||!document.querySelector('[data-pivot]'))bad.push([span.id,'missing reference']);
     const labels=Array.from(document.querySelectorAll('#stage text')).filter(e=>['A','B','C','D','O','A / O','B / O','C / O','D / O'].includes(e.textContent));
     for(let i=0;i<labels.length;i++)for(let j=i+1;j<labels.length;j++){const a=labels[i].getBBox(),b=labels[j].getBBox();if(a.x<b.x+b.width&&a.x+a.width>b.x&&a.y<b.y+b.height&&a.y+a.height>b.y)bad.push([span.id,'overlapping labels',labels[i].textContent,labels[j].textContent])}

     const side=Array.from(document.querySelectorAll('[data-box=side]')).map(e=>e.textContent).join(' ');if(!side.includes('面积 '+String(Number(expectedArea.toFixed(5)))+' 平方单位'))bad.push([span.id,'measurement']);
     const now=JSON.stringify(v);if(before&&before!==now)moved=true;before=now;
    }}return{bad,moved}})()`);
  ok('geometry has actual motion',result.moved);ok('geometry preserves area, scale and bounds throughout playback',result.bad.length===0,result.bad.slice(0,5));
  const motion=await ev(`(()=>{const d=JSON.parse(document.getElementById('anim-data').textContent),bad=[];let count=0;for(const sc of d.script.scenes){const span=d.timeline.scenes.find(s=>s.id===sc.id);for(const a of sc.actions.filter(a=>a.op==='tween')){const ts=span.actions.find(x=>x.id===a.id);__anim.seek(ts.start);const before=JSON.parse(document.querySelector('[data-shape]').dataset.vertices),beforeParams=__anim.stateAt(ts.start).objects[a.object].params;__anim.seek(ts.end);const after=JSON.parse(document.querySelector('[data-shape]').dataset.vertices);
   if(a.param==='tx'||a.param==='ty'){const k=a.param==='tx'?0:1,delta=a.to-beforeParams[a.param];if(before.some((v,i)=>Math.abs(after[i][k]-v[k]-delta)>1e-6||Math.abs(after[i][1-k]-v[1-k])>1e-6))bad.push('translation')}
   if(a.param==='angle'){const delta=(a.to-beforeParams.angle)*Math.PI/180,c=Math.cos(delta),s=Math.sin(delta),x=beforeParams.cx+beforeParams.tx,y=beforeParams.cy+beforeParams.ty;if(before.some((v,i)=>Math.abs(after[i][0]-(x+(v[0]-x)*c-(v[1]-y)*s))>1e-6||Math.abs(after[i][1]-(y+(v[0]-x)*s+(v[1]-y)*c))>1e-6))bad.push('rotation')}count++;}}return{count,bad}})()`);
  ok('geometry translation and rotation match independent expectations',motion.count>=1&&motion.bad.length===0,motion);
 }else if(await ev('JSON.parse(document.getElementById("anim-data").textContent).script.archetype==="lesson.explain@1"')){
  ok('general lesson uses visible teaching components',await ev('(__anim.seek('+mid+'),document.querySelectorAll("[data-lesson]").length>0)'));
 const layout=await ev(`(()=>{const bad=[];for(const sc of __anim.timeline.scenes){__anim.render(sc.start+1000);for(const group of document.querySelectorAll('[data-lesson]')){const r=group.querySelector('rect').getBBox(),texts=Array.from(group.querySelectorAll(':scope > text, [data-kind=uploaded] text'));for(let i=0;i<texts.length;i++){const a=texts[i].getBBox();if(a.x<r.x||a.y<r.y||a.x+a.width>r.x+r.width||a.y+a.height>r.y+r.height)bad.push([sc.id,'outside']);for(let j=0;j<i;j++){const b=texts[j].getBBox();if(a.x<b.x+b.width&&a.x+a.width>b.x&&a.y<b.y+b.height&&a.y+a.height>b.y)bad.push([sc.id,'overlap'])}}}}return bad})()`);
 ok('lesson text stays inside each card without overlapping',layout.length===0,layout.slice(0,5));
 }else{
  const ks=await ev(`[${tw.start-1},${mid},${tw.end+1}].map(t=>{__anim.seek(t);return Number(document.querySelector('[data-line]').dataset.k)})`);
  ok('representative tween changes k monotonically',ks[0]<ks[1]&&ks[1]<ks[2],ks);
 }

 // Subtitles: shown during a sentence, empty during a silent hold.
 const h=T.scenes.flatMap(s=>s.holds||[]).find(h=>h.type==='observe'),sen=(T.scenes[0].subtitles||T.scenes[0].sentences)[0];
 const subs=await ev(`(()=>{const r=[];__anim.seek(${sen.start+100});r.push(document.getElementById('subtitle').textContent);__anim.seek(${h.start+100});r.push(document.getElementById('subtitle').textContent);return r})()`);
 ok('subtitle follows sentence and clears in silent hold',subs[0].length>0&&subs[1]==='',subs);
 if(T.scenes[0].subtitles){const first=T.scenes[0].subtitles[0];const edges=await ev(`(()=>{const out=[];for(const t of [${first.start-1},${first.start+1},${first.end+1}]){__anim.seek(Math.max(0,t));out.push(document.getElementById('subtitle').textContent)}return out})()`);ok('subtitle excludes leading and trailing speech silence',edges[0]===''&&edges[1].length>0&&edges[2]==='',edges)}

 // Step mode freezes time at an await until 继续; auto mode passes through it.
 const aw=T.scenes.flatMap(s=>s.holds||[]).find(h=>h.type==='await');
 await ev(`(__anim.step(true),__anim.seek(${aw.start-300}),__anim.toggle(),0)`);await sleep(1500);
 const w=await ev(`({t:__anim.player.t,waiting:!!__anim.player.waiting,btn:!document.getElementById('continue').hidden})`);
 ok('step mode waits at await',w.waiting&&w.btn&&Math.abs(w.t-aw.start)<1,w);
 await sleep(700);ok('time frozen while waiting',Math.abs((await ev('__anim.player.t'))-aw.start)<1);
 await ev('(document.getElementById("continue").click(),0)');await sleep(300);
 const after=await ev('__anim.player.t');ok('继续 resumes after the hold',after>=aw.end,{after,end:aw.end});
 await ev(`(__anim.toggle(),__anim.step(false),__anim.seek(${aw.start-300}),__anim.toggle(),0)`);await sleep(1200);
 const auto=await ev('({t:__anim.player.t,waiting:!!__anim.player.waiting})');
 ok('auto mode does not wait',!auto.waiting&&auto.t>aw.start,auto);
 await ev('(__anim.toggle(),0)');

 // Pause freezes time and audio.
 const p0=await ev('__anim.player.t');await sleep(500);
 ok('pause freezes time',Math.abs((await ev('__anim.player.t'))-p0)<1&&(await ev('__anim.player.audio===null')));

 // Audio drives time inside a sentence (only meaningful when the package has audio).
 const hasAudio=await ev('Object.values(JSON.parse(document.getElementById("anim-data").textContent).audio).length>0');
 if(hasAudio){
  const s2=T.scenes[1].sentences[0];
  await ev(`(__anim.seek(${s2.start+200}),__anim.toggle(),0)`);await sleep(1200);
  const a=await ev(`(()=>{const p=__anim.player;return{t:p.t,audio:p.audio&&p.audio.currentTime*1000,playing:p.audio&&!p.audio.paused,start:${s2.start}}})()`);
  ok('sentence audio plays and drives time',a.playing&&Math.abs(a.start+a.audio-a.t)<120,a);
  await ev('(__anim.toggle(),0)');
  ok('pause stops audio',await ev('__anim.player.audio===null'));
 }

 const snapshots=[['tween-start',tw.start],['tween-mid',mid],['tween-end',tw.end+200],['summary',T.scenes.at(-1).sentences[1].start+500]];
 if(geometry){const times=await ev(`(()=>{const d=JSON.parse(document.getElementById('anim-data').textContent),out=[];for(const sc of d.script.scenes)for(const a of sc.actions)if(a.op==='tween'&&a.param==='angle'){const span=d.timeline.scenes.find(s=>s.id===sc.id).actions.find(x=>x.id===a.id);out.push(['rotation-'+sc.id+'-mid',(span.start+span.end)/2],['rotation-'+sc.id+'-end',span.end])}return out})()`);snapshots.push(...times)}
 for(const [name,t] of snapshots){
  await ev(`(__anim.seek(${t}),0)`);await sleep(50);
  const shot=await call('Page.captureScreenshot',{format:'png'});await fs.writeFile(path.join(output,name+'.png'),Buffer.from(shot.data,'base64'));
 }
 if(hasAudio){
  await ev(`(()=>{window.__originalPlay=HTMLMediaElement.prototype.play;HTMLMediaElement.prototype.play=()=>Promise.reject(new Error('test playback blocked'));__anim.seek(${T.scenes[0].sentences[0].start+500});__anim.toggle()})()`);await sleep(600);
  ok('blocked audio stops playback with an explicit retry message',await ev("!__anim.player.playing&&!!document.getElementById('audio-error')?.textContent"));
  await ev('HTMLMediaElement.prototype.play=window.__originalPlay');
 }
 ok('no network requests',requests.length===0,requests);
 ok('no console errors or CSP violations',errors.length===0,errors);
 await fs.writeFile(path.join(output,'report.json'),JSON.stringify({file:path.basename(file),total_ms:total,checks},null,1));
 await ev('__anim.player.playing=false;__anim.render(__anim.timeline.scenes[0].start+2000);document.getElementById("stage").setAttribute("xmlns","http://www.w3.org/2000/svg")');await fs.writeFile(path.join(output,'preview.svg'),await ev('document.getElementById("stage").outerHTML'));
 const failed=checks.filter(c=>!c.pass);
 console.log(`${checks.length-failed.length}/${checks.length} checks passed; screenshots in ${path.relative(root,output)}`);
 process.exitCode=failed.length?1:0;
})().catch(e=>{console.error(e);process.exitCode=1}).finally(()=>{try{socket?.close()}catch{}browser?.kill()});
