// Opens a built offline animation package in headless Edge with the network
// disabled and checks playback behaviour. Usage: node scripts/browser-animation-package.cjs <package.html>
const fs=require('node:fs/promises'),path=require('node:path'),assert=require('node:assert/strict');
const {spawn}=require('node:child_process'),{pathToFileURL}=require('node:url');
const root=path.resolve(__dirname,'..'),sleep=ms=>new Promise(r=>setTimeout(r,ms));
const file=path.resolve(process.argv[2]||path.join(root,'work/anim-v2/linear-k-tone.html'));
let browser,socket;
(async()=>{
 const output=path.join(root,'work/anim-v2/browser');await fs.mkdir(output,{recursive:true});
 browser=spawn('C:/Program Files (x86)/Microsoft/Edge/Application/msedge.exe',['--headless=new','--disable-gpu','--no-first-run','--disable-background-networking','--autoplay-policy=no-user-gesture-required','--remote-debugging-port=9271','--remote-debugging-address=127.0.0.1','--user-data-dir='+path.join(output,'profile'),'about:blank'],{windowsHide:true,stdio:'ignore'});
 let page;
 for(let i=0;i<40;i++){try{page=(await fetch('http://127.0.0.1:9271/json/list').then(r=>r.json())).find(p=>p.type==='page');if(page)break}catch{}await sleep(200)}
 assert.ok(page,'Edge starts');socket=new WebSocket(page.webSocketDebuggerUrl);
 await new Promise((resolve,reject)=>{socket.onopen=resolve;socket.onerror=reject});
 let seq=0;const pending=new Map(),errors=[],requests=[],checks=[];
 const call=(method,params={})=>new Promise((resolve,reject)=>{const id=++seq,timer=setTimeout(()=>reject(Error('CDP timeout: '+method)),15000);pending.set(id,{resolve,reject,timer});socket.send(JSON.stringify({id,method,params}))});
 socket.onmessage=e=>{
  const m=JSON.parse(e.data);
  if(m.id){const p=pending.get(m.id);if(p){clearTimeout(p.timer);pending.delete(m.id);m.error?p.reject(Error(JSON.stringify(m.error))):p.resolve(m.result)}}
  if(m.method==='Runtime.exceptionThrown')errors.push(m.params.exceptionDetails.exception?.description||m.params.exceptionDetails.text);
  if(m.method==='Log.entryAdded'&&m.params.entry.level==='error')errors.push(m.params.entry.text);
  if(m.method==='Network.requestWillBeSent'){const u=m.params.request.url;if(!u.startsWith('data:')&&!u.startsWith('file:'))requests.push(u)}
 };
 const ev=async(expr)=>{const r=await call('Runtime.evaluate',{expression:expr,returnByValue:true,awaitPromise:true});if(r.exceptionDetails)throw Error(r.exceptionDetails.exception?.description||r.exceptionDetails.text);return r.result.value};
 const ok=(name,cond,detail)=>{checks.push({name,pass:!!cond,detail});if(!cond)console.error('FAIL',name,detail??'')};
 await call('Runtime.enable');await call('Log.enable');await call('Network.enable');await call('Page.enable');
 await call('Network.emulateNetworkConditions',{offline:true,latency:0,downloadThroughput:0,uploadThroughput:0});
 await call('Emulation.setDeviceMetricsOverride',{width:1440,height:900,deviceScaleFactor:1,mobile:false});
 await call('Page.navigate',{url:pathToFileURL(file).href});
 for(let i=0;i<50&&!(await ev('!!window.__anim').catch(()=>false));i++)await sleep(100);
 ok('player boots offline under CSP',await ev('!!window.__anim'),errors);
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
 const sub=await ev(`(()=>{__anim.seek(${T.scenes[1].sentences[2].start+50});const f=document.querySelector('.frame').getBoundingClientRect(),s=document.getElementById('subtitle').getBoundingClientRect();return{inside:s.left>=f.left&&s.right<=f.right&&s.bottom<=f.bottom,lines:Math.round(s.height/parseFloat(getComputedStyle(document.getElementById('subtitle')).lineHeight))}})()`);
 ok('subtitle inside frame at 1920x1080',sub.inside&&sub.lines<=2,sub);

 // Determinism: render(t) after an unrelated seek equals a direct render(t).
 const rep=T.scenes.find(s=>s.actions.some(a=>a.type==='tween')),tw=rep.actions.find(a=>a.type==='tween');
 const mid=Math.round((tw.start+tw.end)/2);
 const same=await ev(`(()=>{__anim.seek(${mid});const a=document.getElementById('stage').innerHTML;__anim.seek(${total-10});__anim.seek(0);__anim.seek(${mid});return a===document.getElementById('stage').innerHTML})()`);
 ok('state is a pure function of t (seek/replay restore)',same);
 const ks=await ev(`[${tw.start-1},${mid},${tw.end+1}].map(t=>{__anim.seek(t);return Number(document.querySelector('[data-line]').dataset.k)})`);
 ok('representative tween changes k monotonically',ks[0]<ks[1]&&ks[1]<ks[2],ks);

 // Subtitles: shown during a sentence, empty during a silent hold.
 const h=T.scenes.flatMap(s=>s.holds).find(h=>h.type==='observe'),sen=T.scenes[0].sentences[0];
 const subs=await ev(`(()=>{const r=[];__anim.seek(${sen.start+100});r.push(document.getElementById('subtitle').textContent);__anim.seek(${h.start+100});r.push(document.getElementById('subtitle').textContent);return r})()`);
 ok('subtitle follows sentence and clears in silent hold',subs[0].length>0&&subs[1]==='',subs);

 // Step mode freezes time at an await until 继续; auto mode passes through it.
 const aw=T.scenes.flatMap(s=>s.holds).find(h=>h.type==='await');
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

 for(const [name,t] of [['tween-start',tw.start],['tween-mid',mid],['tween-end',tw.end+200],['summary',T.scenes.at(-1).sentences[1].start+500]]){
  await ev(`(__anim.seek(${t}),0)`);await sleep(50);
  const shot=await call('Page.captureScreenshot',{format:'png'});await fs.writeFile(path.join(output,name+'.png'),Buffer.from(shot.data,'base64'));
 }
 ok('no network requests',requests.length===0,requests);
 ok('no console errors or CSP violations',errors.length===0,errors);
 await fs.writeFile(path.join(output,'report.json'),JSON.stringify({file:path.basename(file),total_ms:total,checks},null,1));
 const failed=checks.filter(c=>!c.pass);
 console.log(`${checks.length-failed.length}/${checks.length} checks passed; screenshots in ${path.relative(root,output)}`);
 process.exitCode=failed.length?1:0;
})().catch(e=>{console.error(e);process.exitCode=1}).finally(()=>{try{socket?.close()}catch{}browser?.kill()});
