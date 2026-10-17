// Real app with isolated HTTP fixtures: never calls models or writes user data.
const fs=require('node:fs/promises'),path=require('node:path'),assert=require('node:assert/strict'),{spawn}=require('node:child_process'),http=require('node:http');
const root=path.resolve(__dirname,'..'),sleep=ms=>new Promise(r=>setTimeout(r,ms));let browser,socket,server;
(async()=>{
 const out=path.join(root,'work/service-readiness/browser');await fs.mkdir(out,{recursive:true});
 server=http.createServer(async(req,res)=>{try{const name=new URL(req.url,'http://localhost').pathname;const file=path.resolve(root,'frontend-vue/dist','.'+(name==='/'?'/index.html':name));assert.ok(file.startsWith(path.join(root,'frontend-vue/dist')));res.setHeader('Content-Type',file.endsWith('.js')?'text/javascript':file.endsWith('.css')?'text/css':'text/html');res.end(await fs.readFile(file))}catch{res.writeHead(404);res.end()}});await new Promise(r=>server.listen(8201,'127.0.0.1',r));
 const writes=[],errors=[],checks=[];const deck=JSON.parse(await fs.readFile(path.join(root,'fixtures/courseware-quality-v2.json'),'utf8'));deck.slides=deck.slides.slice(0,3);deck.slides.forEach((s,i)=>s.id='slide-'+i);
 const animationScript=JSON.parse(await fs.readFile(path.join(root,'fixtures/animation-v2-linear-k.json'),'utf8'));let animationTask=null;
 const task={id:'conversation-fixture',type:'courseware',status:'pending',mode:'live',revision:1,cost:20,input:{topic:'二次函数',subject:'数学',grade:'九年级',preview_first:true},result:null,events:[],sources:[],slide_reviews:[]};
 let animationOnly=false,healthRequests=0;const scenario=process.env.TEST_READINESS_CASE||'recovered';
 browser=spawn('C:/Program Files (x86)/Microsoft/Edge/Application/msedge.exe',['--headless=new','--disable-gpu','--no-first-run','--disable-background-networking','--remote-debugging-port=9299','--remote-debugging-address=127.0.0.1','--user-data-dir='+path.join(out,'profile'),'about:blank'],{windowsHide:true,stdio:'ignore'});
 let page;for(let i=0;i<50;i++){try{page=(await fetch('http://127.0.0.1:9299/json/list').then(r=>r.json())).find(p=>p.type==='page');if(page)break}catch{}await sleep(100)}assert.ok(page);
 socket=new WebSocket(page.webSocketDebuggerUrl);await new Promise((resolve,reject)=>{socket.onopen=resolve;socket.onerror=reject});let serial=0;const pending=new Map();
 const call=(method,params={})=>new Promise((resolve,reject)=>{const id=++serial,timer=setTimeout(()=>reject(Error('CDP timeout '+method)),15000);pending.set(id,{resolve,reject,timer});socket.send(JSON.stringify({id,method,params}))});
 socket.onmessage=e=>{const m=JSON.parse(e.data);if(m.id){const p=pending.get(m.id);if(p){clearTimeout(p.timer);pending.delete(m.id);m.error?p.reject(Error(m.error.message)):p.resolve(m.result)}return}
  if(m.method==='Runtime.exceptionThrown')errors.push(m.params.exceptionDetails.text);
  if(m.method!=='Fetch.requestPaused')return;const {requestId,request}=m.params,p=new URL(request.url).pathname;let reply,code=200,contentType='application/json';
  if(request.method!=='GET')writes.push({path:p,data:JSON.parse(request.postData||'{}')});
  if(p==='/api/health'){
   healthRequests++;reply={status:'ok',mode:'live',animation_configured:true,animation_html_version:'workflow-v1',courseware_quality_configured:true,courseware_content_review_enabled:false,courseware_media_version:healthRequests===1||scenario==='outdated'&&healthRequests===2?'media-v0':'media-v1',courseware_studio_version:'studio-v1'};
   if(scenario==='offline'&&healthRequests===2){code=503;reply={message:'fixture temporary outage'}}
  }
  else if(p==='/api/workspace/brief')reply={tool:'courseware',action:'generate',reply:'按当前需求制作光的折射课件。',mode:'live',settings:{topic:'光的折射',subject:'物理',grade:'八年级'},blocks:[]};
  else if(p==='/api/auth/me')reply={tenant_id:1,member_id:1,name:'林老师',role:'teacher',permissions:['courseware.generate','animation.render']};
  else if(p==='/api/tenants')reply=[{id:1,name:'测试教研空间'}];
  else if(p==='/api/billing')reply={balance:100,reserved:0,available:100,plan:'basic',products:[],orders:[],ledger:[]};
  else if(p==='/api/tasks'){if(request.method==='POST'){const data=JSON.parse(request.postData);if(data.type==='animation'){animationTask={id:'inline-animation',type:'animation',mode:'live',status:'anim_plan',revision:1,cost:40,input:data,result:null,events:[],sources:[]};reply={id:animationTask.id}}else reply={id:task.id}}else reply=[];}
  else if(p==='/api/tasks/inline-animation')reply=animationTask;
  else if(p==='/api/tasks/inline-animation/animation')reply={id:animationTask.id,status:animationTask.status,revision:1,script:animationScript,charge_state:'unreserved',free_used:{},has_preview:false,has_delivery:false};
  else if(p.includes('/slides/')&&p.endsWith('/preview')){contentType='image/svg+xml';reply='<svg xmlns="http://www.w3.org/2000/svg" width="1280" height="720"><rect width="1280" height="720" fill="#faf6ff"/><text x="90" y="150" fill="#59358b" font-size="50">PPT preview fixture</text></svg>';}
  else if(p==='/api/tasks/'+task.id)reply=task;
  else if(p.endsWith('/conversation'))reply=[];
  else if(p.endsWith('/events')){reply=': fixture\n\n';contentType='text/event-stream'}
  else {errors.push('Unexpected API '+p);code=404;reply={message:'Missing fixture'}}
  if(animationOnly&&p==='/api/health')reply.teaching_features_version='teaching-v1';
  if(animationOnly&&p==='/api/auth/me')reply.permissions=['animation.render'];
  call('Fetch.fulfillRequest',{requestId,responseCode:code,responseHeaders:[{name:'Content-Type',value:contentType}],body:Buffer.from(typeof reply==='string'?reply:JSON.stringify(reply)).toString('base64')}).catch(e=>{if(!animationOnly||!String(e).includes('Invalid InterceptionId'))errors.push(String(e))});
 };
 const evaluate=async expression=>{const r=await call('Runtime.evaluate',{expression,returnByValue:true,awaitPromise:true});if(r.exceptionDetails)throw Error(r.exceptionDetails.exception?.description||r.exceptionDetails.text);return r.result.value};
 const wait=async expression=>{for(let i=0;i<100;i++){if(await evaluate(expression))return;await sleep(50)}throw Error('UI timeout '+expression+'; UI: '+await evaluate('document.body.innerText.slice(-1500)')+'; errors: '+JSON.stringify(errors))};
 const fill=(selector,value)=>evaluate(`(()=>{const e=document.querySelector(${JSON.stringify(selector)});e.value=${JSON.stringify(value)};e.dispatchEvent(new Event('input',{bubbles:true}))})()`);
 const click=selector=>evaluate(`document.querySelector(${JSON.stringify(selector)}).click()`);
 const shot=async name=>{await fs.writeFile(path.join(out,name+'.png'),Buffer.from((await call('Page.captureScreenshot',{format:'png',captureBeyondViewport:false})).data,'base64'))};
 await call('Page.enable');await call('Runtime.enable');await call('Emulation.setEmulatedMedia',{features:[{name:'prefers-reduced-motion',value:'reduce'}]});await call('Fetch.enable',{patterns:[{urlPattern:'*/api/*'}]});
 await call('Page.addScriptToEvaluateOnNewDocument',{source:"sessionStorage.setItem('edu_token','fixture-token');localStorage.setItem('edu_sidebar_collapsed','0')"});
 await call('Emulation.setDeviceMetricsOverride',{width:1440,height:900,deviceScaleFactor:1,mobile:false});await call('Page.navigate',{url:process.env.TEST_APP_URL||'http://127.0.0.1:8201'});
 await wait("!!document.querySelector('.home-recent-empty')&&!document.querySelector('.brief-composer textarea').disabled");
 await fill('.brief-composer textarea','做一个讲解光的折射的课件');
 await click('.brief-composer button[type=submit]');
 await wait("!!document.querySelector('.home-tool-result')||!!document.querySelector('.brief-composer .error')");await sleep(500);
 if(!['recovered','progress'].includes(scenario)){
  await wait("!!document.querySelector('.home-tool-result .alert.error button')");
  assert.equal(writes.filter(w=>w.path==='/api/tasks').length,0,'Unready service must not create or reserve a task');
  assert.ok(await evaluate("document.querySelector('.home-tool-result .alert.error').innerText.includes('内容已保留')"));
  await click('.home-tool-result .alert.error button');
  await wait("!!document.querySelector('.home-tool-result .courseware-studio')");
 }
 await shot('courseware-create-readiness');
 const creations=writes.filter(w=>w.path==='/api/tasks');
 assert.equal(creations.length,1,'Recovered health must allow exactly one task; UI: '+await evaluate("document.querySelector('.home-tool-result')?.innerText||document.querySelector('.brief-composer .error')?.innerText"));
 assert.equal(creations[0].data.topic,'光的折射');
 assert.ok(healthRequests>=2,'Courseware creation rechecks server health without refreshing away the conversation');
 await wait("!!document.querySelector('.home-tool-result .courseware-studio')");
 assert.equal(await evaluate("document.querySelectorAll('article.brief-bubble.user').length"),1);
 assert.equal(await evaluate("!!document.querySelector('.home-tool-result .alert.error')"),false);
 assert.deepEqual(errors,[]);
 if(scenario==='progress'){
  task.status='researching';task.revision=2;task.result={title:'光的折射',slides:[],plan:{pages:Array.from({length:14},(_,i)=>({position:i+1,preview:[2,4,6].includes(i)}))},generation:{phase:'preview',repair_page:1,page_attempts:{1:2}}};
  task.courseware_provider_calls=5;task.automatic_failures=1;
  task.events=[{stage:'page_review',message:'教学位置第3页正在审核实际PPT画面',created_at:new Date().toISOString()}];
  await wait("document.querySelector('.studio-progress')?.innerText.includes('已规划14页')");
  assert.ok(await evaluate("document.querySelector('.studio-progress').innerText.includes('5次')"));
  assert.ok(await evaluate("document.querySelector('.studio-progress').innerText.includes('教学位置第3页')"));
  assert.equal(await evaluate("document.querySelector('.studio-progress progress').value"),0);
  task.revision=3;task.result.slides=[{...deck.slides[0],order:3}];task.result.page_checks={[deck.slides[0].id]:{content:'disabled',visual:'checked',issues:[{severity:'error'}]}};
  await wait("document.querySelector('.studio-progress')?.innerText.includes('待修复草稿1页')");
  assert.equal(await evaluate("document.querySelector('.studio-progress progress').value"),0,'Draft must not count as passed');
  await shot('courseware-workflow-progress');
 }
 if(scenario==='progress')checks.push('Planned page count and provider calls are visible','Preview slot maps to teaching position','Saved failed draft does not count as passed');
 const result={scenario,checks:['Stale startup health recovers before courseware creation','Conversation and requirements survive recheck','Exactly one task is created',...checks],healthRequests,task_creates:creations.length};
 await fs.writeFile(path.join(out,'checks-'+scenario+'.json'),JSON.stringify(result,null,2));console.log(JSON.stringify(result));
 await call('Browser.close');socket.close();await new Promise(r=>server.close(r));
})().catch(e=>{console.error(e.stack);process.exitCode=1;socket?.close();browser?.kill();server?.close()});

