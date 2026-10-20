const createCDP = require('./browser-cdp.cjs');
// Browser fixtures only: actual incremental ReadableStream, no model or user data writes.
const fs=require('node:fs/promises'),path=require('node:path'),assert=require('node:assert/strict'),{spawn}=require('node:child_process'),http=require('node:http');
const root=path.resolve(__dirname,'..'),sleep=ms=>new Promise(r=>setTimeout(r,ms));let browser,socket,server;
(async()=>{
 const out=path.join(root,'work/workspace-history-20261020/browser');await fs.mkdir(out,{recursive:true});
 server=http.createServer(async(req,res)=>{try{const name=new URL(req.url,'http://localhost').pathname;const file=path.resolve(root,'frontend-vue/dist','.'+(name==='/'?'/index.html':name));assert.ok(file.startsWith(path.join(root,'frontend-vue/dist')));res.setHeader('Content-Type',file.endsWith('.js')?'text/javascript':file.endsWith('.css')?'text/css':'text/html');res.end(await fs.readFile(file))}catch{res.writeHead(404);res.end()}});await new Promise(r=>server.listen(8205,'127.0.0.1',r));
 const writes=[],errors=[],checks=[],histories=new Map();
 let memberID=1,failSave=false,deleteDelay=0;
 const saved=()=>[...histories.entries()].filter(([k,v])=>k.startsWith(memberID+':')&&!v.deleted).map(([,v])=>v);
 browser=spawn(process.env.TEST_BROWSER_PATH||(process.platform==='win32'?'C:/Program Files (x86)/Microsoft/Edge/Application/msedge.exe':'google-chrome'),['--headless=new','--disable-gpu','--no-first-run','--disable-background-networking','--remote-debugging-port=9305','--remote-debugging-address=127.0.0.1','--user-data-dir='+path.join(out,'profile'),'about:blank'],{windowsHide:true,stdio:['ignore','ignore','inherit']});
 browser.on('error',error=>{console.error('Cannot launch test browser:',error.message);socket?.close();server?.close();process.exitCode=1});
 let page;for(let i=0;i<300;i++){try{page=(await fetch('http://127.0.0.1:9305/json/list').then(r=>r.json())).find(p=>p.type==='page');if(page)break}catch{}await sleep(100)}assert.ok(page);
 socket=new WebSocket(page.webSocketDebuggerUrl);await new Promise((resolve,reject)=>{socket.onopen=resolve;socket.onerror=reject});
 const { call, evaluate } = createCDP(socket, m => {  if(m.method==='Runtime.exceptionThrown')errors.push(m.params.exceptionDetails.text);
  if(m.method!=='Fetch.requestPaused')return;const {requestId,request}=m.params,p=new URL(request.url).pathname;let reply,code=200,contentType='application/json';
  const data=JSON.parse(request.postData||'{}'),method=request.method;
  if(method!=='GET')writes.push({path:p,method,data});
  if(p==='/api/health')reply={status:'ok',mode:'live',animation_configured:true,animation_html_version:'workflow-v1',courseware_quality_configured:true,courseware_content_review_enabled:false,courseware_media_version:'media-v1',courseware_studio_version:'studio-v1',workspace_history_version:'history-v1'};
  else if(p==='/api/workspace/conversations'||p.startsWith('/api/workspace/conversations/')){
   const id=p.split('/')[4],key=memberID+':'+id,row=histories.get(key);
   if(method==='PUT'){
    if(failSave){code=503;reply={message:'测试保存失败'};}
    else if(row?.deleted||(row?.revision||0)!==data.revision){code=409;reply={message:'对话已清除或更新'};}
    else {reply={revision:(row?.revision||0)+1};histories.set(key,{id,title:data.state.session.messages.find(m=>m.role==='user')?.content||'备课对话',revision:reply.revision,updated_at:new Date().toISOString(),state:data.state});}
   }else if(method==='DELETE'){
    if(id){if(!row||row.deleted){code=404;reply={message:'不存在'};}else {histories.set(key,{...row,state:{},deleted:true});reply={ok:true};}}
    else {for(const [k,v]of histories)if(k.startsWith(memberID+':'))histories.set(k,{...v,state:{},deleted:true});reply={ok:true};}
   }else if(id){if(!row||row.deleted){code=404;reply={message:'不存在'};}else reply=row;}
   else reply=saved().map(({state,...row})=>row);
  }
  else if(p==='/api/auth/me')reply={tenant_id:1,member_id:memberID,name:'林老师',role:'teacher',permissions:['courseware.generate','animation.render']};
  else if(p==='/api/tenants')reply=[{id:1,name:'测试教研空间'}];
  else if(p==='/api/billing')reply={balance:100,reserved:0,available:100,plan:'basic',products:[],orders:[],ledger:[]};
  else if(p==='/api/tasks'){if(method!=='GET')errors.push('History created a task');reply=[];}
  else if(p==='/api/tasks/existing-task')reply={id:'existing-task',type:'animation',status:'failed',mode:'sample',input:{type:'animation',topic:'旧作品'},events:[]};
  else if(p==='/api/tasks/existing-task/animation'){code=404;reply={message:'测试作品暂无预览'};}
  else {errors.push('Unexpected API '+p);code=404;reply={message:'Missing fixture'}}
  // Reload cancels an intercepted old-document request before its fixture reply arrives.
  const fulfill=()=>call('Fetch.fulfillRequest',{requestId,responseCode:code,responseHeaders:[{name:'Content-Type',value:contentType}],body:Buffer.from(typeof reply==='string'?reply:JSON.stringify(reply)).toString('base64')}).catch(e=>{if(!String(e).includes('Invalid InterceptionId'))errors.push(String(e))});
  if(method==='DELETE'&&deleteDelay)setTimeout(fulfill,deleteDelay);else void fulfill();
 });
 const wait=async expression=>{for(let i=0;i<100;i++){if(await evaluate(expression))return;await sleep(50)}throw Error('UI timeout '+expression)};
 const fill=(selector,value)=>evaluate(`(()=>{const e=document.querySelector(${JSON.stringify(selector)});e.value=${JSON.stringify(value)};e.dispatchEvent(new Event('input',{bubbles:true}))})()`);
 const click=selector=>evaluate(`document.querySelector(${JSON.stringify(selector)}).click()`);
 const shot=async name=>{await fs.writeFile(path.join(out,name+'.png'),Buffer.from((await call('Page.captureScreenshot',{format:'png',captureBeyondViewport:false})).data,'base64'))};
 await call('Page.enable');await call('Runtime.enable');await call('Fetch.enable',{patterns:[{urlPattern:'*/api/*'}]});
 await call('Page.addScriptToEvaluateOnNewDocument',{source:"sessionStorage.setItem('edu_token','fixture-token');localStorage.setItem('edu_sidebar_collapsed','0')"});
 await call('Emulation.setDeviceMetricsOverride',{width:1440,height:900,deviceScaleFactor:1,mobile:false});await call('Page.navigate',{url:process.env.TEST_APP_URL||'http://127.0.0.1:8205'});
 await wait("!!document.querySelector('.brief-composer textarea')");
 await wait("!!document.querySelector('[data-history-action=panel]')");
 await click('[data-history-action=panel]');await wait("!!document.querySelector('.workspace-history-empty')");
 checks.push('History entry and empty list');
 const installStream=()=>evaluate("(()=>{const original=fetch.bind(window);window.__requests=[];window.fetch=(url,options={})=>{if(!String(url).endsWith('/workspace/brief'))return original(url,options);window.__requests.push(JSON.parse(options.body));return Promise.resolve(new Response(new ReadableStream({start(c){const enc=new TextEncoder();window.__event=(event,value)=>c.enqueue(enc.encode('event: '+event+'\\ndata: '+JSON.stringify(value)+'\\n\\n'));window.__end=()=>c.close();window.__event('delta',{text:'正在讨论'});}}),{headers:{'Content-Type':'text/event-stream'}}));};})()");
 const finish=topic=>evaluate('window.__event("done",'+JSON.stringify({tool:'courseware',action:'discuss',reply:'已整理'+topic,settings:{topic,subject:'科学'},requirements:'保留观察与讨论',blocks:[],questions:[],mode:'live'})+');window.__end()');
 await click('[data-history-action=panel]');await installStream();
 await fill('.brief-composer textarea','讨论光的折射');await click('.brief-composer button[type=submit]');await wait("!!document.querySelector('.stream-cursor')");await finish('光的折射');
 await wait("!document.querySelector('.brief-composer textarea').disabled");await sleep(1000);assert.equal(saved().length,1);const first=saved()[0].id;
 await click('[data-history-action=new]');await wait("document.querySelectorAll('.brief-bubble').length===0");assert.equal(saved().length,1);
 await click('[data-history-action=panel]');await wait("document.querySelectorAll('.workspace-history-item').length===1");await shot('history-desktop');
 await click('.workspace-history-open');await wait("document.querySelector('.brief-topic input').value==='光的折射'");assert.ok(await evaluate("document.querySelector('.courseware-setup').textContent.includes('已整理光的折射')"));
 checks.push('Saved history opens with previous requirements; new conversation keeps history');
 failSave=true;await fill('.brief-composer textarea','新补充');await sleep(850);await click('[data-history-action=new]');await wait("!!document.querySelector('.workspace-history-error')");assert.ok(await evaluate("document.querySelector('.courseware-setup').textContent.includes('已整理光的折射')"));failSave=false;
 checks.push('Save failure preserves the current conversation');
 await fill('.brief-composer textarea','迟到的回答');await click('.brief-composer button[type=submit]');await wait("!!document.querySelector('.stream-cursor')");deleteDelay=900;await click('[data-history-action=clear]');await finish('不应该回填');await wait("document.querySelectorAll('.brief-bubble').length===0&&!document.querySelector('[data-history-action=new]').disabled");deleteDelay=0;
 await sleep(1000);assert.equal(saved().length,0);assert.ok(histories.get('1:'+first).deleted);
 assert.equal(await evaluate("document.querySelector('.brief-composer textarea').value"),'');assert.equal(await evaluate("document.querySelector('.brief-topic input').value"),'');
 assert.ok(await evaluate("(()=>{const s=JSON.parse(sessionStorage.getItem('edu_workspace_v1:1:1')).session;return s.understanding===''&&s.contextMemory.text===''&&!s.summary.includes('保留观察')&&!s.summary.includes('不应该回填')})()"));
 await call('Page.reload',{ignoreCache:true});await wait("!!document.querySelector('[data-history-action=panel]')");assert.equal(await evaluate("document.querySelectorAll('.brief-bubble').length"),0);await installStream();
 checks.push('Clearing aborts stale replies and resets input, summary, memory and settings; refresh stays empty');
 await fill('.brief-composer textarea','细胞结构');await click('.brief-composer button[type=submit]');await wait("!!document.querySelector('.stream-cursor')");await finish('细胞结构');await wait("!document.querySelector('.brief-composer textarea').disabled");await click('[data-history-action=new]');await wait("document.querySelectorAll('.brief-bubble').length===0");
 const row=saved()[0];row.state.session.messages.push({role:'assistant',content:'已制作',execution:{taskId:'existing-task',key:'old-key',input:{type:'animation',topic:'旧作品'}}},{role:'assistant',content:'未开始',execution:{taskId:'',key:'never-restart',input:{type:'courseware',topic:'不得重建'}}});
 await click('[data-history-action=panel]');await wait("document.querySelectorAll('.workspace-history-item').length===1");await click('.workspace-history-open');await wait("document.querySelectorAll('.home-tool-result').length===1");await sleep(700);assert.equal(writes.filter(w=>w.path==='/api/tasks').length,0);
 checks.push('Restoring task links only reads existing works and cannot create tasks');
 await click('[data-history-action=panel]');await wait("document.querySelectorAll('.workspace-history-item').length===1");await click('.workspace-history-delete');await wait("document.querySelectorAll('.brief-bubble').length===0&&document.querySelectorAll('.workspace-history-item').length===0");
 checks.push('Deleting current history clears its on-screen conversation');
 await fill('.brief-composer textarea','光合作用');await click('.brief-composer button[type=submit]');await wait("!!document.querySelector('.stream-cursor')");await finish('光合作用');await wait("!document.querySelector('.brief-composer textarea').disabled");await click('[data-history-action=new]');await wait("document.querySelectorAll('.brief-bubble').length===0");await click('[data-history-action=panel]');await wait("document.querySelectorAll('.workspace-history-item').length===1");
 await evaluate('window.confirm=()=>true');await click('[data-history-action=clear-all]');await wait("document.querySelectorAll('.workspace-history-item').length===0");assert.equal(saved().length,0);
 checks.push('Clear-all removes saved conversations');
 await call('Emulation.setDeviceMetricsOverride',{width:390,height:844,deviceScaleFactor:1,mobile:true});await sleep(200);assert.ok(await evaluate('document.documentElement.scrollWidth<=innerWidth'),'Mobile history cannot overflow');await shot('history-mobile');
 memberID=2;await call('Page.reload',{ignoreCache:true});await wait("!!document.querySelector('[data-history-action=panel]')");await click('[data-history-action=panel]');await wait("!!document.querySelector('.workspace-history-empty')");assert.equal(await evaluate("document.querySelectorAll('.brief-bubble').length"),0);
 checks.push('Mobile layout and member isolation');
 assert.deepEqual(errors,[]);await fs.writeFile(path.join(out,'checks.json'),JSON.stringify({checks,errors,task_creates:writes.filter(w=>w.path==='/api/tasks').length},null,2));console.log(JSON.stringify({checks}));
 await call('Browser.close');socket.close();await new Promise(r=>server.close(r));
})().catch(e=>{console.error(e.stack);process.exitCode=1;socket?.close();browser?.kill();server?.close()});
