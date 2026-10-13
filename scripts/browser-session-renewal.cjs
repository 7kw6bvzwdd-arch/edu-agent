// Every API call is intercepted. No real credentials, tasks, or model calls.
const fs=require('node:fs/promises'),path=require('node:path'),assert=require('node:assert/strict');
const {spawn}=require('node:child_process');
const root=path.resolve(__dirname,'..'),sleep=ms=>new Promise(r=>setTimeout(r,ms));
let browser,socket;
(async()=>{
 const output=path.join(root,'work/auth-session');await fs.mkdir(output,{recursive:true});
 const deck=JSON.parse(await fs.readFile(path.join(root,'fixtures/courseware-quality-v2.json'),'utf8'));
 deck.slides=deck.slides.slice(0,3);deck.slides.forEach((sl,i)=>sl.id='slide-'+i);
 const task={id:'session-test',type:'courseware',status:'awaiting_preview',mode:'sample',revision:1,cost:20,input:{topic:'浮力',pages:12,preview_first:true},result:deck,events:[],sources:[],slide_reviews:[]};
 const start=Date.now(),expiry=start+12*60*60*1000;
 const jwt=expires=>Buffer.from('{"alg":"HS256"}').toString('base64url')+'.'+Buffer.from(JSON.stringify({exp:Math.floor(expires/1000),uid:1,tid:1,sid:'test-session'})).toString('base64url')+'.fixture';
 const original=jwt(expiry),fresh=jwt(expiry+11*60*60*1000);
 let elapsed=0,refreshes=0,unavailable=false,revoked=false,heldRefresh=null,holdRefresh=false;
 const checks=[],errors=[],requests=[];
 browser=spawn('C:/Program Files (x86)/Microsoft/Edge/Application/msedge.exe',['--headless=new','--disable-gpu','--no-first-run','--disable-background-networking','--remote-debugging-port=9264','--remote-debugging-address=127.0.0.1','--user-data-dir='+path.join(output,'browser-profile'),'about:blank'],{windowsHide:true,stdio:'ignore'});
 let page;for(let i=0;i<50;i++){try{page=(await fetch('http://127.0.0.1:9264/json/list').then(r=>r.json())).find(p=>p.type==='page');if(page)break}catch{}await sleep(100)}
 assert.ok(page,'browser startup');socket=new WebSocket(page.webSocketDebuggerUrl);await new Promise((r,j)=>{socket.onopen=r;socket.onerror=j});
 let seq=0;const pending=new Map();
 const call=(method,params={})=>new Promise((resolve,reject)=>{const id=++seq,timer=setTimeout(()=>reject(Error('CDP timeout '+method)),15000);pending.set(id,{resolve,reject,timer});socket.send(JSON.stringify({id,method,params}))});
 const reply=(requestId,code,data)=>call('Fetch.fulfillRequest',{requestId,responseCode:code,responseHeaders:[{name:'Content-Type',value:'application/json'}],body:Buffer.from(JSON.stringify(data)).toString('base64')}).catch(e=>errors.push(String(e)));
 socket.onmessage=e=>{
  const m=JSON.parse(e.data);
  if(m.id){const p=pending.get(m.id);if(p){clearTimeout(p.timer);pending.delete(m.id);m.error?p.reject(Error(JSON.stringify(m.error))):p.resolve(m.result)}}
  if(m.method==='Runtime.exceptionThrown')errors.push(m.params.exceptionDetails.text);
  if(m.method!=='Fetch.requestPaused')return;
  const {requestId,request}=m.params,p=new URL(request.url).pathname,credential=request.headers.Authorization||request.headers.authorization;
  requests.push({path:p,method:request.method});
  if(p==='/api/health'){reply(requestId,200,{status:'ok',mode:'sample',courseware_media_version:'media-v1',works_view_version:'views-v1',session_refresh_version:'session-v1'});return}
  if(p==='/api/auth/logout'){revoked=true;reply(requestId,200,{ok:true});return}
  if(p==='/api/auth/refresh'){
   refreshes++;
   if(holdRefresh){heldRefresh=requestId;return}
   if(unavailable){reply(requestId,503,{message:'登录验证服务暂不可用，请稍后重试'});return}
   if(!revoked&&start+elapsed<expiry){reply(requestId,200,{token:fresh});return}
  }
  if(revoked||(credential==='Bearer '+original&&start+elapsed>=expiry)){reply(requestId,401,{message:'登录已失效'});return}
  if(p==='/api/auth/me')reply(requestId,200,{tenant_id:1,member_id:1,name:'测试教师',role:'teacher',permissions:['courseware.generate']});
  else if(p==='/api/tenants')reply(requestId,200,[{id:1,name:'测试空间'}]);
  else if(p==='/api/billing')reply(requestId,200,{available:80,reserved:20,products:[],orders:[],ledger:[]});
  else if(p==='/api/tasks')reply(requestId,200,[task]);
  else if(p==='/api/tasks/'+task.id)reply(requestId,200,task);
  else if(p.endsWith('/conversation'))reply(requestId,200,[]);
  else if(p.endsWith('/revisions'))reply(requestId,200,[]);
  else if(p.endsWith('/events'))call('Fetch.fulfillRequest',{requestId,responseCode:200,responseHeaders:[{name:'Content-Type',value:'text/event-stream'}],body:Buffer.from(': fixture\n\n').toString('base64')}).catch(e=>errors.push(String(e)));
  else{errors.push('Unexpected API '+p);reply(requestId,404,{message:'Unexpected API'})}
 };
 const evaluate=async expression=>{const r=await call('Runtime.evaluate',{expression,returnByValue:true,awaitPromise:true});if(r.exceptionDetails)throw Error(r.exceptionDetails.exception?.description||r.exceptionDetails.text);return r.result.value};
 const wait=async expression=>{for(let i=0;i<80;i++){if(await evaluate(expression))return;await sleep(100)}throw Error('UI timeout '+expression)};
 const click=label=>evaluate(`(()=>{const b=[...document.querySelectorAll('button')].find(b=>b.textContent.includes(${JSON.stringify(label)}));if(!b)throw Error('Missing button');b.click()})()`);
 const clock=async value=>{elapsed=value;await evaluate('window.sessionTestClock='+value)};
 await call('Runtime.enable');await call('Page.enable');await call('Fetch.enable',{patterns:[{urlPattern:'*/api/*'}]});
 await call('Page.addScriptToEvaluateOnNewDocument',{source:`sessionStorage.setItem('edu_token',${JSON.stringify(original)});window.sessionTestClock=0;const realDateNow=Date.now;Date.now=()=>realDateNow()+window.sessionTestClock;`});
 await call('Page.navigate',{url:process.env.TEST_APP_URL||'http://127.0.0.1:8080'});await wait('!!document.querySelector(".home-workspace")');
 await click('我的作品');await wait('!!document.querySelector(".task-open")');await evaluate('document.querySelector(".task-open").click()');await wait('!!document.querySelector(".courseware-studio")');
 const editor=await evaluate(`(()=>{const b=document.querySelector('.courseware-studio button[title="编辑本页"]')||[...document.querySelectorAll('.courseware-studio button')].find(b=>b.textContent.includes('编辑'));if(b)b.click();return !!b})()`);
 assert.equal(editor,true,'Editable courseware is present for the unsaved-edit regression');
 if(editor){await wait('!!document.querySelector(".studio-slide-editor input")');await evaluate(`(()=>{const e=document.querySelector('.studio-slide-editor input');e.value='未保存的教学标题';e.dispatchEvent(new Event('input',{bubbles:true}))})()`)}
 await clock(12*60*60*1000-2*60*1000);
 // Keep the actual workspace open across the original token deadline.
 await sleep(2500);await clock(12*60*60*1000+1000);await click('版本历史');await sleep(700);
 assert.equal(await evaluate('!!document.querySelector(".auth-panel")'),false,'Active courseware must not be replaced by the login form at the original expiry');
 assert.equal(await evaluate('!!document.querySelector(".courseware-studio")'),true,'Courseware stays mounted');
 assert.equal(await evaluate('sessionStorage.getItem("edu_token")'),fresh,'Active browser renews its token');
 assert.equal(refreshes,1,'Concurrent pollers share one refresh');
 if(editor)assert.equal(await evaluate('document.querySelector(".studio-slide-editor input").value'),'未保存的教学标题','Renewal preserves unsaved edits');
 checks.push('Active courseware survives the original 12-hour expiry with one refresh and unchanged edits');
 await call('Page.reload');elapsed=0;await wait('!!document.querySelector(".home-workspace")');
 unavailable=true;await clock(12*60*60*1000-2*60*1000);await click('套餐与账单');await sleep(700);
 assert.equal(await evaluate('sessionStorage.getItem("edu_token")'),original,'Temporary refresh outage keeps valid credentials');
 assert.equal(await evaluate('!!document.querySelector(".auth-panel")'),false);
 unavailable=false;await click('首页');await wait('sessionStorage.getItem("edu_token")==='+JSON.stringify(fresh));
 checks.push('Temporary refresh outage retains the current session and recovers on the next request');
 elapsed=0;await call('Page.reload');await wait('!!document.querySelector(".home-workspace")');
 holdRefresh=true;await clock(12*60*60*1000-2*60*1000);await click('套餐与账单');
 for(let i=0;i<80&&!heldRefresh;i++)await sleep(100);assert.ok(heldRefresh,'Refresh started');
 await evaluate('document.querySelector("button[aria-label=退出登录]").click()');await wait('!!document.querySelector(".auth-panel")');
 await reply(heldRefresh,200,{token:fresh});heldRefresh=null;await sleep(300);
 assert.equal(await evaluate('sessionStorage.getItem("edu_token")'),null,'Delayed renewal cannot restore an explicitly logged-out session');
 assert.equal(await evaluate('!!document.querySelector(".auth-panel")'),true);
 checks.push('Explicit logout is not delayed by renewal and a late renewal cannot sign the user back in');
 assert.deepEqual(errors,[]);assert.equal(requests.filter(r=>r.method==='POST'&&!['/api/auth/refresh','/api/auth/logout'].includes(r.path)).length,0);
 await fs.writeFile(path.join(output,'browser.json'),JSON.stringify({checks,errors,scope:'All API calls intercepted; no production writes'},null,2));console.log(JSON.stringify({checks}));
 await call('Browser.close');socket.close();
})().catch(e=>{console.error(e.stack);process.exitCode=1;socket?.close();browser?.kill()});
