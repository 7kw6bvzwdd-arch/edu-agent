const fs=require('node:fs/promises'),path=require('node:path'),assert=require('node:assert/strict');
const {spawn}=require('node:child_process'),{pathToFileURL}=require('node:url');
const sleep=ms=>new Promise(r=>setTimeout(r,ms));
function checkLayout(){
 const d=JSON.parse(document.getElementById('anim-data').textContent),bad=[],layouts=[];let checked=0;
 for(const sc of d.script.scenes){const span=d.timeline.scenes.find(s=>s.id===sc.id);
  for(const o of sc.objects.filter(o=>o.items?.length)){
   const inspect=t=>{__anim.seek(t);return Array.from(document.querySelectorAll('[data-lesson="'+o.id+'"] [data-item]')).map(g=>({index:Number(g.dataset.item),opacity:Number(g.getAttribute('opacity')),active:g.dataset.active}))};
   const early=inspect(span.sentences[0].start+700),late=inspect(span.sentences.at(-1).start+700);
   if(early.length!==o.items.length||early[0]?.opacity!==1||early.at(-1)?.opacity!==0)bad.push([sc.id,'not independent',early]);
   if(late.some(g=>g.opacity!==1)||late.at(-1)?.active!=='true')bad.push([sc.id,'last cue',late]);
   if(JSON.stringify(inspect(span.sentences[0].start+700))!==JSON.stringify(early))bad.push([sc.id,'seek restoration']);
   __anim.seek(span.end-1);const group=document.querySelector('[data-lesson="'+o.id+'"]');
   if(!group?.dataset.layout)bad.push([sc.id,'missing layout']);else layouts.push(group.dataset.layout);
   if(group?.querySelector(':scope > rect'))bad.push([sc.id,'one enclosing text box']);
   if(group?.querySelector('[data-item-box]'))bad.push([sc.id,'repeated text tiles']);
   checked++;
  }
  for(let f=0;f<=8;f++){__anim.seek(span.start+(span.end-span.start-1)*f/8);const texts=Array.from(document.querySelectorAll('#stage text'));for(let i=0;i<texts.length;i++){const a=texts[i].getBoundingClientRect(),stage=document.getElementById('stage').getBoundingClientRect();if(a.left<stage.left||a.top<stage.top||a.right>stage.right+1||a.bottom>stage.top+stage.height*608/720+1)bad.push([sc.id,'overflow',texts[i].textContent]);for(let j=0;j<i;j++){const b=texts[j].getBoundingClientRect();if(a.x<b.x+b.width-1&&a.x+a.width>b.x+1&&a.y<b.y+b.height-1&&a.y+a.height>b.y+1)bad.push([sc.id,'overlap',texts[i].textContent,texts[j].textContent])}}}
 }
 return{checked,layouts,bad};
}
(async()=>{
 const file=path.resolve(process.argv[2]),out=path.dirname(file);
 const browser=spawn('C:/Program Files (x86)/Microsoft/Edge/Application/msedge.exe',['--headless=new','--disable-gpu','--no-first-run','--disable-background-networking','--remote-debugging-port=9283','--user-data-dir='+path.join(out,'text-browser-profile'),'about:blank'],{windowsHide:true,stdio:'ignore'});
 let socket;
 try{
  let page;for(let i=0;i<50;i++){try{page=(await fetch('http://127.0.0.1:9283/json/list').then(r=>r.json())).find(p=>p.type==='page');if(page)break}catch{}await sleep(100)}
  assert.ok(page,'browser starts');socket=new WebSocket(page.webSocketDebuggerUrl);await new Promise((r,j)=>{socket.onopen=r;socket.onerror=j});
  const errors=[],{call,evaluate:ev}=require('./browser-cdp.cjs')(socket,m=>{if(m.method==='Runtime.exceptionThrown')errors.push(m.params.exceptionDetails.exception?.description||m.params.exceptionDetails.text);if(m.method==='Log.entryAdded'&&m.params.entry.level==='error')errors.push(m.params.entry.text)});
  await call('Runtime.enable');await call('Log.enable');await call('Page.enable');await call('Network.enable');
  await call('Network.emulateNetworkConditions',{offline:true,latency:0,downloadThroughput:0,uploadThroughput:0});
  await call('Emulation.setDeviceMetricsOverride',{width:1440,height:960,deviceScaleFactor:1,mobile:false});
  await call('Page.navigate',{url:pathToFileURL(file).href});
  for(let i=0;i<50&&!(await ev('!!window.__anim').catch(()=>false));i++)await sleep(100);
  assert.ok(await ev('!!window.__anim'),'offline CSP player boots: '+JSON.stringify({errors,state:await ev('({url:location.href,body:document.body?.textContent.slice(0,150)})')}));
  const result=await ev('('+checkLayout.toString()+')()');
  await fs.writeFile(path.join(out,path.basename(file,'.html')+'-check.json'),JSON.stringify({result,errors},null,2));
  assert.ok(result.checked>=5);assert.equal(result.bad.length,0,JSON.stringify(result.bad.slice(0,8)));assert.deepEqual(errors,[]);
  const shots=await ev('__anim.timeline.scenes.map(s=>[s.id,s.sentences[1].start+700])');
  for(const [name,t]of shots){await ev('__anim.seek('+t+')');const shot=await call('Page.captureScreenshot',{format:'png'});await fs.writeFile(path.join(out,name+'.png'),Buffer.from(shot.data,'base64'))}
  console.log(JSON.stringify(result));
 }finally{socket?.close();browser.kill()}
})().catch(e=>{console.error(e);process.exitCode=1});
