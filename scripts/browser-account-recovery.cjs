const createCDP = require('./browser-cdp.cjs');
// Intercept every API call: no real passwords, tasks or balances are changed.
const fs=require('node:fs/promises'),path=require('node:path'),assert=require('node:assert/strict');
const {spawn}=require('node:child_process');
const root=path.resolve(__dirname,'..'),sleep=ms=>new Promise(r=>setTimeout(r,ms));
let browser,socket;
(async()=>{
 const output=path.join(root,'work/account-recovery');await fs.mkdir(output,{recursive:true});
 const deck=JSON.parse(await fs.readFile(path.join(root,'fixtures/courseware-teaching-media.json'),'utf8'));
 const task={id:'paused-courseware',type:'courseware',status:'failed',cost:20,revision:1,charge_state:'released',error:'表格/流程图与图表/固定示意图请分开放到不同页面',result:{},input:{topic:'浮力',pages:8,theme:'forest',use_kb:true},events:[],sources:[]};
 const next={...task,id:'continued-courseware',status:'awaiting_confirmation',charge_state:'reserved',error:'',result:deck};
 const actor={tenant_id:1,member_id:1,name:'测试教师',role:'teacher',permissions:['courseware.generate']};
 let updatedName,passwordCalls=0,retries=0,worksVersion='views-v1',sessionUnavailable=false,sessionExpired=false,billingUnauthorized=false,holdBilling=false,heldBilling;const errors=[],checks=[],worksQueries=[];
 browser=spawn('C:/Program Files (x86)/Microsoft/Edge/Application/msedge.exe',['--headless=new','--disable-gpu','--no-first-run','--disable-background-networking','--remote-debugging-port=9250','--remote-debugging-address=127.0.0.1','--user-data-dir='+path.join(output,'browser-profile'),'about:blank'],{windowsHide:true,stdio:'ignore'});
 let page;for(let i=0;i<40;i++){try{page=(await fetch('http://127.0.0.1:9250/json/list').then(r=>r.json())).find(p=>p.type==='page');if(page)break}catch{}await sleep(200)}
 assert.ok(page,'browser startup');socket=new WebSocket(page.webSocketDebuggerUrl);await new Promise((r,j)=>{socket.onopen=r;socket.onerror=j});

 const { call, evaluate } = createCDP(socket, m => {
  if(m.method==='Runtime.exceptionThrown')errors.push(m.params.exceptionDetails.text);
  if(m.method==='Fetch.requestPaused'){
   const {requestId,request}=m.params,u=new URL(request.url);let reply,code=200;
   if(u.pathname==='/api/billing'&&holdBilling){heldBilling=requestId;holdBilling=false;return;}
   if(u.pathname.endsWith('/events')){call('Fetch.fulfillRequest',{requestId,responseCode:200,responseHeaders:[{name:'Content-Type',value:'text/event-stream'}],body:Buffer.from(': fixture\n\n').toString('base64')}).catch(e=>errors.push(String(e)));return}
   if(u.pathname==='/api/health')reply={status:'ok',mode:'sample',courseware_media_version:'media-v1',works_view_version:worksVersion};
   else if(u.pathname==='/api/auth/me'){if(sessionExpired){code=401;reply={message:'登录已失效'}}else if(sessionUnavailable){code=503;reply={message:'登录验证服务暂不可用，请稍后重试'}}else reply=actor;}
   else if(u.pathname==='/api/auth/switch-tenant')reply={token:'fixture-new-token'};
   else if(u.pathname==='/api/tenants')reply=[{id:1,name:'隔离测试'}];
   else if(u.pathname==='/api/billing'){if(billingUnauthorized){code=401;reply={message:'临时验证失败'}}else reply={balance:100,reserved:retries?20:0,available:retries?80:100,plan:'basic',ledger:[]};}
   else if(u.pathname==='/api/account'&&request.method==='GET')reply={username:'teacher',name:actor.name};
   else if(u.pathname==='/api/account'&&request.method==='PATCH'){updatedName=JSON.parse(request.postData).name;actor.name=updatedName;reply={name:updatedName}}
   else if(u.pathname==='/api/account/password'&&request.method==='POST'){
    passwordCalls++;const data=JSON.parse(request.postData);assert.equal(data.current_password,'current-test-password');assert.equal(data.new_password,'new-test-password');reply={reauthenticate:true};
   }
   else if(u.pathname==='/api/tasks'){
    const view=u.searchParams.get('view');worksQueries.push(Object.fromEntries(u.searchParams));
    reply=view==='works'?(retries?[next]:[]):view==='interrupted'?(retries?[]:[task]):(retries?[next,task]:[task]);
    if(u.searchParams.get('status'))reply=reply.filter(t=>t.status===u.searchParams.get('status'));
   }
   else if(u.pathname==='/api/tasks/'+task.id)reply=task;
   else if(u.pathname==='/api/tasks/'+next.id)reply=next;
   else if(u.pathname==='/api/tasks/'+task.id+'/retry'){retries++;reply={id:next.id}}
   else{code=400;reply={message:'Unexpected request'};errors.push(request.method+' '+u.pathname)}
   call('Fetch.fulfillRequest',{requestId,responseCode:code,responseHeaders:[{name:'Content-Type',value:'application/json'}],body:Buffer.from(JSON.stringify(reply)).toString('base64')}).catch(e=>errors.push(String(e)));
  }
 });
 const run=fn=>evaluate('('+fn.toString()+')()');
 const wait=async expression=>{for(let i=0;i<60;i++){if(await evaluate(expression))return;await sleep(100)}console.error(await evaluate('JSON.stringify({heading:document.querySelector("h1")?.textContent,alerts:[...document.querySelectorAll("[role=alert]")].map(x=>x.textContent),account:document.querySelector(".account-settings")?.textContent,focused:document.activeElement?.outerHTML})'));throw Error('UI timeout '+expression)};
 const click=label=>evaluate('(()=>{const b=[...document.querySelectorAll("button")].find(b=>b.textContent.includes('+JSON.stringify(label)+'));if(!b)throw Error("Missing button");b.click()})()');
 await call('Runtime.enable');await call('Page.enable');await call('Fetch.enable',{patterns:[{urlPattern:'*/api/*'}]});
 await call('Page.addScriptToEvaluateOnNewDocument',{source:"sessionStorage.setItem('edu_token','fixture-token')"});
 await call('Emulation.setDeviceMetricsOverride',{width:1440,height:1000,deviceScaleFactor:1,mobile:false});
 await call('Page.navigate',{url:process.env.TEST_APP_URL||'http://127.0.0.1:8080'});await wait('!!document.querySelector(".home-workspace")');
 await evaluate('document.querySelector(".profile-open").focus()');
 await call('Input.dispatchKeyEvent',{type:'keyDown',key:'Enter',code:'Enter',windowsVirtualKeyCode:13,text:'\r'});await call('Input.dispatchKeyEvent',{type:'keyUp',key:'Enter',code:'Enter',windowsVirtualKeyCode:13});
 await wait('document.querySelector(".account-settings input")?.value==="teacher"');
 await run(()=>{const form=document.querySelector('[aria-label="修改账户名称"]'),input=form.querySelector('input[autocomplete=name]');input.value='林老师·教研';input.dispatchEvent(new Event('input',{bubbles:true}));form.requestSubmit()});
 await wait('document.querySelector(".profile-open")?.textContent.includes("林老师·教研")');assert.equal(updatedName,'林老师·教研');
 checks.push('Keyboard-accessible profile entry and display-name update');
 billingUnauthorized=true;await click('套餐与账单');await wait('!!document.querySelector("[role=alert]")');
 assert.equal(await evaluate('sessionStorage.getItem("edu_token")'),'fixture-token');
 assert.equal(await evaluate('document.querySelector(".auth-panel")===null'),true);
 billingUnauthorized=false;checks.push('A resource 401 does not discard a session that auth/me still validates');
 sessionUnavailable=true;await call('Page.reload');await wait('!!document.querySelector(".session-recovery")');
 assert.equal(await evaluate('sessionStorage.getItem("edu_token")'),'fixture-token');
 assert.equal(await evaluate('document.querySelector(".auth-panel")===null'),true);
 sessionUnavailable=false;await wait('!!document.querySelector(".session-recovery button:not(:disabled)")');await click('重试');await wait('!!document.querySelector(".account-navigation")');await click('首页');await wait('!!document.querySelector(".home-workspace")');
 checks.push('Temporary startup authentication outage keeps credentials and offers a working retry');
 await evaluate('document.querySelector(".profile-open").click()');await wait('!!document.querySelector(".account-settings")');
 await fs.writeFile(path.join(output,'account-desktop.png'),Buffer.from((await call('Page.captureScreenshot',{format:'png'})).data,'base64'));
 await click('首页');await call('Emulation.setDeviceMetricsOverride',{width:390,height:844,deviceScaleFactor:1,mobile:true});
 await evaluate('document.querySelector(".mobile-account").click()');await wait('!!document.querySelector(".account-settings")');
 assert.equal(await evaluate('document.documentElement.scrollWidth<=390'),true);
 await fs.writeFile(path.join(output,'account-mobile.png'),Buffer.from((await call('Page.captureScreenshot',{format:'png'})).data,'base64'));
 checks.push('Mobile account entry and responsive settings page');
 await click('我的作品');await wait('!!document.querySelector(".works-toolbar")&&!document.querySelector(".works-toolbar .primary").disabled');
 for(let i=0;i<60&&worksQueries.at(-1)?.view!=='works';i++)await sleep(100);
 assert.equal(await evaluate('document.querySelector(".works-views button[aria-pressed=true]")?.dataset.view'),'works');
 assert.equal(worksQueries.at(-1).view,'works');assert.equal(await evaluate('document.querySelectorAll(".task-card").length'),0);
 assert.ok(await evaluate('document.querySelector(".empty-small").textContent.includes("待处理")'));
 await run(()=>{document.querySelector('[data-view=interrupted]').click()});
 await wait('!!document.querySelector(".task-open")');
 assert.equal(worksQueries.at(-1).view,'interrupted');assert.equal(worksQueries.at(-1).offset,'0');
 assert.equal(await evaluate('document.documentElement.scrollWidth<=390'),true);
 await fs.writeFile(path.join(output,'works-interrupted-mobile.png'),Buffer.from((await call('Page.captureScreenshot',{format:'png'})).data,'base64'));
 checks.push('Default works view hides interrupted records; accessible view selector reveals them without mobile overflow');
 await evaluate('document.querySelector(".task-open").click()');await wait('!!document.querySelector(".recovery-actions")');
 assert.equal(await evaluate('document.querySelector(".output-head .status").textContent'),'待继续制作');
 assert.equal(await evaluate('document.querySelector(".generation-progress .spinner")===null'),true);
 assert.equal(await evaluate('document.querySelector(".output-panel").textContent.includes("未完成")'),false);
 assert.equal(await evaluate('document.querySelector(".recovery-actions details").open'),false);
 await evaluate('document.querySelector(".output-panel").scrollIntoView({behavior:"instant",block:"start"})');
 await fs.writeFile(path.join(output,'recovery-mobile.png'),Buffer.from((await call('Page.captureScreenshot',{format:'png'})).data,'base64'));
 await click('继续制作');await wait('document.querySelectorAll(".outline-slide").length===8');assert.equal(retries,1);
 checks.push('Interrupted courseware exposes recovery and restores editable draft');
 await click('我的作品');await wait('!!document.querySelector(".works-toolbar")&&!document.querySelector(".works-toolbar .primary").disabled&&document.querySelectorAll(".task-card").length===0');
 assert.equal(worksQueries.at(-1).view,'interrupted','Old failure disappears from interrupted view after retry');
 await run(()=>{document.querySelector('[data-view=history]').click()});
 await wait('document.querySelectorAll(".task-card").length===2');
 assert.equal(worksQueries.at(-1).view,'history');
 await run(()=>{const select=document.querySelector('[aria-label="制作状态"]');select.value='failed';select.dispatchEvent(new Event('change',{bubbles:true}))});
 await wait('document.querySelectorAll(".task-card").length===1&&document.querySelector(".task-card").dataset.taskId==="paused-courseware"');
 await run(()=>{document.querySelector('[data-view=works]').click()});
 await wait('document.querySelectorAll(".task-card").length===1&&document.querySelector(".task-card").dataset.taskId==="continued-courseware"');
 assert.equal(worksQueries.at(-1).status,'','View switch resets incompatible status filters');assert.equal(retries,1);
 assert.equal(await evaluate('document.querySelector("[aria-label=制作状态] option[value=failed]")===null'),true);
 await click('重置');await wait('!document.querySelector(".works-toolbar .primary").disabled');
 assert.equal(worksQueries.at(-1).view,'works');assert.equal(worksQueries.at(-1).q,'');
 await call('Emulation.setDeviceMetricsOverride',{width:1440,height:1000,deviceScaleFactor:1,mobile:false});
 await fs.writeFile(path.join(output,'works-current-desktop.png'),Buffer.from((await call('Page.captureScreenshot',{format:'png'})).data,'base64'));
 checks.push('Retry shows only the latest work; all-records view retains old attempts, filters reset across views, and listing triggers no extra retries');
 await evaluate('document.querySelector(".mobile-account").click()');await wait('document.querySelector(".account-settings input")?.value==="teacher"');
 await run(()=>{const form=document.querySelector('[aria-label="修改登录密码"]'),inputs=form.querySelectorAll('input');['current-test-password','new-test-password','mismatch-password'].forEach((value,i)=>{inputs[i].value=value;inputs[i].dispatchEvent(new Event('input',{bubbles:true}))});form.requestSubmit()});
 await wait('document.querySelector(".account-settings [role=alert]")?.textContent.includes("不一致")');assert.equal(passwordCalls,0);
 await run(()=>{const form=document.querySelector('[aria-label="修改登录密码"]'),input=form.querySelectorAll('input')[2];input.value='new-test-password';input.dispatchEvent(new Event('input',{bubbles:true}));form.requestSubmit()});
 await wait('!!document.querySelector(".login-form")');assert.equal(passwordCalls,1);assert.equal(await evaluate('sessionStorage.getItem("edu_token")'),null);
 assert.ok(await evaluate('document.querySelector(".login-form").textContent.includes("密码已更新")'));
 checks.push('Password confirmation prevents bad submission; success clears session and returns to login');
 await call('Page.navigate',{url:process.env.TEST_APP_URL||'http://127.0.0.1:8080'});await wait('!!document.querySelector(".home-workspace")');
 holdBilling=true;await click('套餐与账单');for(let i=0;i<60&&!heldBilling;i++)await sleep(100);assert.ok(heldBilling);
 await run(()=>document.querySelector('.tenant-select select').dispatchEvent(new Event('change',{bubbles:true})));
 await wait('sessionStorage.getItem("edu_token")==="fixture-new-token"');
 await call('Fetch.fulfillRequest',{requestId:heldBilling,responseCode:401,responseHeaders:[{name:'Content-Type',value:'application/json'}],body:Buffer.from(JSON.stringify({message:'旧请求登录失效'})).toString('base64')});heldBilling=null;
 await wait('!!document.querySelector("[role=alert]")');
 assert.equal(await evaluate('sessionStorage.getItem("edu_token")'),'fixture-new-token');assert.equal(await evaluate('document.querySelector(".auth-panel")===null'),true);
 checks.push('Delayed 401 from a previous credential cannot log out the new session');
 worksVersion='';await call('Page.navigate',{url:process.env.TEST_APP_URL||'http://127.0.0.1:8080'});await wait('!!document.querySelector(".home-workspace")');
 await click('我的作品');await wait('document.querySelectorAll(".task-card").length===2');
 assert.equal(worksQueries.at(-1).view,'history');
 assert.equal(await evaluate('document.querySelector(".works-views button").disabled'),true);
 assert.ok(await evaluate('document.body.textContent.includes("作品收纳功能需要重启后端后生效")'));
 checks.push('Outdated backend explicitly falls back to all records, keeping old works accessible without pretending grouping works');
 sessionExpired=true;billingUnauthorized=true;await click('套餐与账单');await wait('!!document.querySelector(".auth-panel")');
 assert.equal(await evaluate('sessionStorage.getItem("edu_token")'),null);
 assert.ok(await evaluate('document.querySelector(".auth-panel").textContent.includes("登录已失效")'));
 checks.push('Confirmed session expiry clears credentials and explicitly requests login');
 assert.deepEqual(errors,[]);await fs.writeFile(path.join(output,'browser.json'),JSON.stringify({checks,errors,scope:'All API requests intercepted; zero production writes'},null,2));console.log(JSON.stringify({checks}));
 await call('Browser.close');socket.close();
})().catch(e=>{console.error(e.stack);process.exitCode=1;socket?.close();browser?.kill()});
