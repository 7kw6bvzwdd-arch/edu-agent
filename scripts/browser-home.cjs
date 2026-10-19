const createCDP = require('./browser-cdp.cjs');
// Fixture APIs only. No production login, task creation or paid model calls.
const fs=require('node:fs/promises'),path=require('node:path'),assert=require('node:assert/strict');
const {spawn}=require('node:child_process');
const root=path.resolve(__dirname,'..'),sleep=ms=>new Promise(r=>setTimeout(r,ms));
let browser,socket;
(async()=>{
 const output=path.join(root,'work/home-workspace');await fs.mkdir(output,{recursive:true});
 const deck=JSON.parse(await fs.readFile(path.join(root,'fixtures/courseware-quality-v2.json'),'utf8'));
 deck.slides=deck.slides.slice(0,3);deck.slides.forEach((s,i)=>s.id='slide-'+i);
 const works=[
  {id:'fixture-paused',type:'courseware',status:'failed',charge_state:'released',mode:'sample',revision:1,cost:20,created_at:'2026-10-09T10:00:00Z',input:{topic:'浮力与阿基米德原理',pages:12,preview_first:true,theme:'classroom'},result:deck,events:[],sources:[],error:'模型服务暂时不可用',slide_reviews:[]},
  {id:'fixture-done',type:'courseware',status:'succeeded',charge_state:'settled',mode:'sample',revision:1,cost:20,created_at:'2026-10-08T09:00:00Z',input:{topic:'勾股定理的生活应用',pages:12,theme:'ocean'},result:{...deck,title:'勾股定理的生活应用'},events:[],sources:[]},
  {id:'fixture-animation',type:'animation',status:'awaiting_confirmation',mode:'sample',revision:1,cost:40,created_at:'2026-10-07T09:00:00Z',input:{topic:'一次函数的动态演示',pages:8,theme:'violet'},result:deck,events:[],sources:[]},
 ];
 const errors=[],checks=[],writes=[];
 browser=spawn('C:/Program Files (x86)/Microsoft/Edge/Application/msedge.exe',['--headless=new','--disable-gpu','--no-first-run','--disable-background-networking','--remote-debugging-port=9263','--remote-debugging-address=127.0.0.1','--user-data-dir='+path.join(output,'browser-profile'),'about:blank'],{windowsHide:true,stdio:'ignore'});
 let page;
 for(let i=0;i<40;i++){try{page=(await fetch('http://127.0.0.1:9263/json/list').then(r=>r.json())).find(p=>p.type==='page');if(page)break}catch{}await sleep(200)}
 assert.ok(page,'Edge starts');socket=new WebSocket(page.webSocketDebuggerUrl);
 await new Promise((resolve,reject)=>{socket.onopen=resolve;socket.onerror=reject});

 const { call, evaluate } = createCDP(socket, m => {
  if(m.method==='Runtime.exceptionThrown')errors.push(m.params.exceptionDetails.text);
  if(m.method!=='Fetch.requestPaused')return;
  const {requestId,request}=m.params,p=new URL(request.url).pathname;
  let reply,body,contentType='application/json',code=200;
  if(request.method!=='GET')writes.push(p);
  if(p==='/api/health')reply={status:'ok',mode:'sample',courseware_media_version:'media-v1',courseware_studio_version:'studio-v1',works_view_version:'views-v1'};
  else if(p==='/api/auth/me')reply={tenant_id:1,member_id:1,name:'林老师',role:'teacher',permissions:['courseware.generate','animation.render','kb.manage']};
  else if(p==='/api/tenants')reply=[{id:1,name:'启明教研'}];
  else if(p==='/api/billing')reply={balance:100,reserved:0,available:100,plan:'basic',products:[],ledger:[],orders:[]};
  else if(p==='/api/tasks')reply=works;
  else if(p==='/api/documents')reply=[];
  else if(p==='/api/dashboard')reply={tasks:[],usage:[]};
  else if(p.endsWith('/conversation'))reply=[];
  else if(p.endsWith('/events')){contentType='text/event-stream';body=Buffer.from(': fixture\n\n')}
  else if(p.endsWith('/retry')&&request.method==='POST'){
   const retry={...works[0],id:'fixture-retry',status:'awaiting_preview',error:'',charge_state:'reserved'};works.unshift(retry);reply={id:retry.id};code=202;
  }else if(p.startsWith('/api/tasks/'))reply=works.find(w=>w.id===p.split('/').at(-1));
  else{errors.push('Unexpected API: '+p);code=404;reply={}}
  if(!reply&&!body){errors.push('Missing fixture: '+p);reply={};code=404}
  call('Fetch.fulfillRequest',{requestId,responseCode:code,responseHeaders:[{name:'Content-Type',value:contentType}],body:(body||Buffer.from(JSON.stringify(reply))).toString('base64')}).catch(e=>errors.push(String(e)));
 });
 const wait=async expression=>{for(let i=0;i<100;i++){if(await evaluate(expression))return;await sleep(100)}throw Error('UI timeout: '+expression+'\n'+await evaluate('document.body.innerText.slice(-1800)')+'\n'+JSON.stringify(errors))};
 const click=label=>evaluate(`(()=>{const b=[...document.querySelectorAll('button')].find(b=>b.textContent.includes(${JSON.stringify(label)}));if(!b||b.disabled)throw Error('Missing button '+${JSON.stringify(label)});b.click()})()`);
 const shot=async name=>fs.writeFile(path.join(output,name),Buffer.from((await call('Page.captureScreenshot',{format:'png'})).data,'base64'));
 await call('Runtime.enable');await call('Page.enable');await call('Fetch.enable',{patterns:[{urlPattern:'*/api/*'}]});
 await call('Page.addScriptToEvaluateOnNewDocument',{source:"sessionStorage.setItem('edu_token','fixture-token')"});
 await call('Emulation.setDeviceMetricsOverride',{width:1440,height:1080,deviceScaleFactor:1,mobile:false});
 await call('Page.navigate',{url:process.env.TEST_APP_URL||'http://127.0.0.1:8080'});
 await wait("document.querySelectorAll('.home-tool').length===6&&document.querySelectorAll('.home-recent-work').length===3");
 assert.equal(await evaluate("document.querySelector('.sidebar [aria-current]').textContent.trim()"),'首页');
 assert.equal(await evaluate("document.querySelector('.home-prompt button').disabled"),true);
 assert.ok(await evaluate("document.querySelector('[data-tool=billing]').textContent.includes('100')"));
 await shot('home-desktop.png');
 const contrast=await evaluate(`(()=>{
  const lum=c=>{const v=c.match(/[\\d.]+/g).slice(0,3).map(Number).map(x=>{x/=255;return x<=.04045?x/12.92:((x+.055)/1.055)**2.4});return .2126*v[0]+.7152*v[1]+.0722*v[2]};
  const ratio=(a,b)=>{const x=lum(a),y=lum(b);return (Math.max(x,y)+.05)/(Math.min(x,y)+.05)};
  const selectors=['.home-prompt label','.home-prompt-actions>span','.home-tool-copy strong','.home-tool-copy small','.home-tool-copy em','.home-recent-work strong','.home-recent-work small','.home-work-state','.home-recent-all','.home-suggestions button','.sidebar nav button'];
  return selectors.flatMap(sel=>[...document.querySelectorAll(sel)].map(n=>{let p=n,bg;while(p){bg=getComputedStyle(p).backgroundColor;if(bg!=='rgba(0, 0, 0, 0)')break;p=p.parentElement}return {selector:sel,text:n.textContent.slice(0,24),ratio:ratio(getComputedStyle(n).color,bg||'rgb(255,255,255)')}}));
 })()`);
 assert.deepEqual(contrast.filter(x=>x.ratio<4.5),[],'Homepage text passes WCAG AA contrast');
 await call('Emulation.setDeviceMetricsOverride',{width:390,height:844,deviceScaleFactor:1,mobile:true});
 await shot('home-mobile.png');
 await evaluate("document.querySelector('.home-content').scrollIntoView()");await sleep(150);await shot('home-tools-mobile.png');
 await call('Emulation.setDeviceMetricsOverride',{width:1440,height:1080,deviceScaleFactor:1,mobile:false});await evaluate('window.scrollTo(0,0)');

 checks.push('Default home shows six working tools, live account balance and three recent works');
 await click('浮力与阿基米德原理');await click('开始备课');
 await wait("!!document.querySelector('.brief-panel')");
 assert.equal(await evaluate("document.querySelector('.config-panel input').value"),'浮力与阿基米德原理');
 assert.equal(writes.length,0,'Starting an idea only opens demand confirmation');
 await shot('courseware-desktop.png');checks.push('Prompt carries the topic into the existing requirement confirmation without creating a task');
 await click('首页');await wait("!!document.querySelector('.home-workspace')");
 await click('勾股定理的生活应用');await click('开始备课');await wait("!!document.querySelector('.brief-panel')");
 assert.equal(await evaluate("document.querySelector('.config-panel select').value"),'数学');
 await click('首页');await wait("!!document.querySelector('.home-workspace')");
 await evaluate("document.querySelector('[data-tool=knowledge]').click()");await wait("!!document.querySelector('.document-row')||document.body.innerText.includes('还没有资料')");
 await click('首页');await wait("!!document.querySelector('.home-workspace')");
 await evaluate("document.querySelector('[data-tool=animation]').click()");await wait("!!document.querySelector('.animation-studio')");
 await click('首页');await wait("!!document.querySelector('.home-workspace')");
 await evaluate("document.querySelector('.home-recent-work').click()");await wait("!!document.querySelector('.courseware-studio')");
 await click('继续制作');await wait("document.querySelector('.studio-stage-banner')?.textContent.includes('先看看前三页')");
 assert.deepEqual(writes,['/api/tasks/fixture-paused/retry']);checks.push('Tool cards open existing workflows; recent paused work reaches the retry action and restored preview');
 await click('首页');await wait("!!document.querySelector('.home-workspace')");
 await call('Emulation.setDeviceMetricsOverride',{width:390,height:844,deviceScaleFactor:1,mobile:true});
 await evaluate('window.scrollTo(0,0)');await sleep(200);
 assert.ok(await evaluate('document.documentElement.scrollWidth<=innerWidth'),'Mobile home has no horizontal overflow');

 await evaluate("document.querySelector('[data-tool=courseware]').click()");await wait("!!document.querySelector('.brief-panel')");
 assert.ok(await evaluate('document.documentElement.scrollWidth<=innerWidth'),'Mobile requirement workflow has no overflow');
 checks.push('Mobile home, cards and existing requirement workflow remain accessible without page overflow');
 assert.deepEqual(errors,[]);await fs.writeFile(path.join(output,'checks.json'),JSON.stringify({checks,errors},null,2));
 console.log(JSON.stringify({checks}));await call('Browser.close');socket.close();
})().catch(e=>{console.error(e.stack);process.exitCode=1;socket?.close();browser?.kill()});
