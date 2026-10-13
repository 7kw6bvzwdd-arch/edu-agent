// Isolated fixture APIs: no real task deletion, account changes, or paid calls.
const fs=require('node:fs/promises'),path=require('node:path'),assert=require('node:assert/strict');
const {spawn}=require('node:child_process');
const root=path.resolve(__dirname,'..'),sleep=ms=>new Promise(r=>setTimeout(r,ms));
let browser,socket;
(async()=>{
 const output=path.join(root,'work/works-resources');await fs.mkdir(output,{recursive:true});
 const items=[['failed','中断的浮力课件'],['cancelled','已取消的函数课件'],['succeeded','已完成的勾股定理']].map(([status,topic],i)=>({id:'work-'+i,type:'courseware',status,display_title:topic,mode:'sample',cost:20,input:{topic,pages:12,theme:'forest',preview_first:true},result:{},revision:1,events:[],sources:[],created_at:'2026-10-10T01:00:00Z'}));
 let cleanup=0,restoreFail=false,deleteFail=false;const writes=[],errors=[],checks=[];
 let usage=Array.from({length:120},(_,i)=>({id:String(i+1),operation:'courseware.page',model:'fixture',input_tokens:10,output_tokens:5,latency_ms:100,success:true,created_at:new Date(Date.parse('2026-10-10T01:00:00Z')+i*60000).toISOString()}));
 browser=spawn('C:/Program Files (x86)/Microsoft/Edge/Application/msedge.exe',['--headless=new','--disable-gpu','--no-first-run','--disable-background-networking','--remote-debugging-port=9267','--remote-debugging-address=127.0.0.1','--user-data-dir='+path.join(output,'browser-profile'),'about:blank'],{windowsHide:true,stdio:'ignore'});
 let page;for(let i=0;i<50;i++){try{page=(await fetch('http://127.0.0.1:9267/json/list').then(r=>r.json())).find(p=>p.type==='page');if(page)break}catch{}await sleep(150)}
 assert.ok(page);socket=new WebSocket(page.webSocketDebuggerUrl);await new Promise((r,j)=>{socket.onopen=r;socket.onerror=j});
 let seq=0;const pending=new Map();
 const call=(method,params={})=>new Promise((resolve,reject)=>{const id=++seq,timer=setTimeout(()=>reject(Error('CDP timeout '+method)),15000);pending.set(id,{resolve,reject,timer});socket.send(JSON.stringify({id,method,params}))});
 socket.onmessage=e=>{
  const m=JSON.parse(e.data);if(m.id){const p=pending.get(m.id);if(p){clearTimeout(p.timer);pending.delete(m.id);m.error?p.reject(Error(JSON.stringify(m.error))):p.resolve(m.result)}}
  if(m.method==='Runtime.exceptionThrown')errors.push(m.params.exceptionDetails.text);if(m.method!=='Fetch.requestPaused')return;
  const {requestId,request}=m.params,u=new URL(request.url),p=u.pathname;let result,code=200;
  if(request.method!=='GET')writes.push([request.method,p]);
  if(p==='/api/health')result={status:'ok',mode:'sample',works_view_version:'views-v1',works_resource_version:'resources-v1',courseware_media_version:'media-v1'};
  else if(p==='/api/dashboard')result={totals:{calls:120,input_tokens:1200,output_tokens:600,failures:0},p95_ms:100};
  else if(p==='/api/usage'){
   const rows=usage.filter(l=>(!u.searchParams.get('from')||l.created_at>=u.searchParams.get('from'))&&(!u.searchParams.get('to')||l.created_at<u.searchParams.get('to')));
   const offset=Number(u.searchParams.get('offset')||0),limit=Number(u.searchParams.get('limit')||50);result={total:rows.length,items:rows.slice(offset,offset+limit),offset,limit};
  }else if(p==='/api/usage/delete'){
   const body=JSON.parse(request.postData);const before=usage.length;
   usage=usage.filter(l=>body.all?!(l.created_at>=body.from&&l.created_at<body.to):!body.ids.includes(Number(l.id)));result={deleted:before-usage.length};
  }
  else if(p==='/api/auth/me')result={tenant_id:1,member_id:1,name:'测试教师',role:'teacher',permissions:['courseware.generate']};
  else if(p==='/api/tenants')result=[{id:1,name:'测试空间'}];
  else if(p==='/api/billing')result={available:100,reserved:0,balance:100,ledger:[]};
  else if(p==='/api/tasks/cleanup-status')result={pending:cleanup};
  else if(p==='/api/tasks'){
   const view=u.searchParams.get('view');result=items.filter(t=>!t.deleted&&(view==='works'?(!['failed','cancelled'].includes(t.status)||t.restored_at):view==='interrupted'?(['failed','cancelled'].includes(t.status)&&!t.restored_at):true));
  }else if(p.endsWith('/restore')){
   if(restoreFail){code=503;result={message:'测试：暂时无法还原'}}else{items.find(t=>p.includes(t.id)).restored_at='2026-10-10T02:00:00Z';result={ok:true}}
  }else if(request.method==='DELETE'){
   if(deleteFail){code=409;result={message:'测试：仍有关联制作，请稍后删除'}}else{items.find(t=>p.endsWith(t.id)).deleted=true;cleanup=1;result={ok:true,cleanup_pending:false}}
  }else if(p.endsWith('/conversation'))result=[];
  else if(items.some(t=>p==='/api/tasks/'+t.id))result=items.find(t=>p.endsWith(t.id));
  else{code=404;result={message:'Unexpected API'};errors.push(request.method+' '+p)}
  call('Fetch.fulfillRequest',{requestId,responseCode:code,responseHeaders:[{name:'Content-Type',value:'application/json'}],body:Buffer.from(JSON.stringify(result)).toString('base64')}).catch(e=>errors.push(String(e)));
 };
 const evaluate=async expression=>{const r=await call('Runtime.evaluate',{expression,returnByValue:true,awaitPromise:true});if(r.exceptionDetails)throw Error(r.exceptionDetails.exception?.description||r.exceptionDetails.text);return r.result.value};
 const wait=async expression=>{for(let i=0;i<100;i++){if(await evaluate(expression))return;await sleep(100)}throw Error('UI timeout '+expression)};
 const click=label=>evaluate(`(()=>{const b=[...document.querySelectorAll('button')].find(b=>b.textContent.includes(${JSON.stringify(label)}));if(!b||b.disabled)throw Error('Missing or disabled button '+${JSON.stringify(label)});b.click()})()`);
 const view=async name=>{await evaluate(`document.querySelector('[data-view=${name}]').click()`);await wait('!document.querySelector(".works-toolbar .primary").disabled')};
 await call('Runtime.enable');await call('Page.enable');await call('Fetch.enable',{patterns:[{urlPattern:'*/api/*'}]});
 await call('Page.addScriptToEvaluateOnNewDocument',{source:"sessionStorage.setItem('edu_token','fixture-token')"});
 await call('Emulation.setDeviceMetricsOverride',{width:1440,height:1000,deviceScaleFactor:1,mobile:false});
 await call('Page.navigate',{url:process.env.TEST_APP_URL||'http://127.0.0.1:8080'});await wait('!!document.querySelector(".home-workspace")');
 await click('我的作品');await wait('document.querySelectorAll(".task-card").length===1');await wait('!document.querySelector(".works-toolbar .primary").disabled');
 await view('interrupted');await wait('document.querySelectorAll(".task-card").length===2');
 assert.equal(await evaluate('document.querySelectorAll(".restore-work").length'),2);assert.equal(writes.length,0);
 await fs.writeFile(path.join(output,'interrupted-desktop.png'),Buffer.from((await call('Page.captureScreenshot',{format:'png'})).data,'base64'));
 checks.push('Visible collection controls show failed and cancelled works with restore and delete actions');
 restoreFail=true;await evaluate('document.querySelector(".restore-work").click()');await wait('document.body.textContent.includes("暂时无法还原")');assert.equal(await evaluate('document.querySelectorAll(".task-card").length'),2);restoreFail=false;
 await evaluate('document.querySelector(".restore-work").click()');await wait('document.querySelector("[data-view=works]").getAttribute("aria-pressed")==="true"&&document.querySelectorAll(".task-card").length===2');
 assert.equal(await evaluate('document.body.textContent.includes("未启动制作、未预留或扣除积分")'),true);
 assert.ok(writes.every(([method,p])=>method==='POST'&&p.endsWith('/restore')));
 await call('Page.reload');await wait('!!document.querySelector(".home-workspace")');await click('我的作品');await wait('document.querySelectorAll(".task-card").length===2');
 checks.push('Restore survives reload, changes collection only, makes no generation or billing request, and failures keep the card');
 await evaluate('document.querySelector("[data-task-id=work-0] .task-open").click()');await wait('!!document.querySelector(".courseware-studio")');await sleep(300);
 assert.equal(errors.filter(e=>e.includes('/events')).length,0,'Terminal works must not open an event stream');
 await click('我的作品');await wait('!!document.querySelector(".works-views")&&!document.querySelector(".works-toolbar .primary").disabled');await view('interrupted');await wait('document.querySelectorAll(".task-card").length===1');
 await call('Emulation.setDeviceMetricsOverride',{width:390,height:844,deviceScaleFactor:1,mobile:true});
 assert.equal(await evaluate('document.documentElement.scrollWidth<=innerWidth'),true);
 await evaluate('document.querySelector(".work-actions .danger-link").click()');await wait('!!document.querySelector(".work-dialog")');
 assert.equal(await evaluate('document.querySelector(".work-dialog").textContent.includes("删除后不可还原")'),true);
 await fs.writeFile(path.join(output,'delete-mobile.png'),Buffer.from((await call('Page.captureScreenshot',{format:'png'})).data,'base64'));
 await click('保留作品');assert.equal(writes.filter(([m])=>m==='DELETE').length,0);
 await evaluate('document.querySelector(".work-actions .danger-link").click()');deleteFail=true;await click('确认删除');await wait('document.body.textContent.includes("仍有关联制作")');assert.equal(await evaluate('!!document.querySelector(".work-dialog")'),true);
 deleteFail=false;await click('确认删除');await wait('!document.querySelector(".work-dialog")&&document.querySelectorAll(".task-card").length===0');
 await wait('!!document.querySelector(".cleanup-status")');await wait('document.body.textContent.includes("作品已删除，资源正在清理")');cleanup=0;await wait('!document.querySelector(".cleanup-status")');
 checks.push('Mobile delete confirmation is explicit; cancellation sends no delete; failures retain the dialog; completed deletion removes the card and polls pending cleanup');
 await call('Emulation.setDeviceMetricsOverride',{width:1440,height:1000,deviceScaleFactor:1,mobile:false});
 await click('用量与评测');await wait('document.querySelectorAll("[data-usage-id]").length===50');
 await evaluate('document.querySelector("[aria-label=下一页调用记录]").click()');await wait('document.querySelector("[data-usage-id]").dataset.usageId==="51"');
 await evaluate('document.querySelector("[aria-label=全选本页调用记录]").click()');
 await wait('document.querySelector(".selection-bar").textContent.includes("已选 50 条")');
 await evaluate('document.querySelector(".usage-records .danger-link").click()');await wait('!!document.querySelector("[aria-label=删除调用记录]")');
 assert.equal(await evaluate('document.activeElement.textContent'), '取消');
 await click('确认删除');await wait('!document.querySelector(".modal-backdrop")&&document.querySelector(".usage-records h2").textContent.includes("70")');
 assert.equal(usage.length,70);assert.equal(await evaluate('document.querySelector(".stat-grid .stat strong").textContent'), '120');
 await evaluate(`(()=>{const inputs=document.querySelectorAll('.usage-filters input');for(const [i,value] of ['2026-10-10T08:00','2026-10-10T12:00'].entries()){inputs[i].value=value;inputs[i].dispatchEvent(new Event('input',{bubbles:true}))}document.querySelector('.usage-filters button').click()})()`);
 await wait('document.querySelectorAll("[data-usage-id]").length===50');
 await click('选择该时间范围全部');await wait('document.querySelector(".selection-bar").textContent.includes("已选 70 条")');
 await evaluate('document.querySelector(".usage-records .danger-link").click()');await wait('!!document.querySelector("[aria-label=删除调用记录]")');
 assert.equal(await evaluate('document.querySelector("[aria-label=删除调用记录]").textContent.includes("不含结束时间")'),true);
 await click('确认删除');await wait('!document.querySelector(".modal-backdrop")&&document.querySelectorAll("[data-usage-id]").length===0');assert.equal(usage.length,0);
 assert.equal(await evaluate('document.querySelector(".stat-grid .stat strong").textContent'), '120');
 await call('Emulation.setDeviceMetricsOverride',{width:390,height:844,deviceScaleFactor:1,mobile:true});
 assert.equal(await evaluate('document.documentElement.scrollWidth<=innerWidth'),true);
 checks.push('Usage pagination, current-page selection, date-range cross-page deletion, dialog focus and retained totals work on desktop and mobile');
 assert.deepEqual(errors,[]);await fs.writeFile(path.join(output,'browser.json'),JSON.stringify({checks,writes,errors,scope:'Fixture APIs only'},null,2));console.log(JSON.stringify({checks}));
 await call('Browser.close');socket.close();
})().catch(e=>{console.error(e.stack);process.exitCode=1;socket?.close();browser?.kill()});
