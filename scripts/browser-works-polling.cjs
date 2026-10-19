const createCDP = require('./browser-cdp.cjs');
// Isolated fixture APIs: no real task deletion, account changes, or paid calls.
const fs=require('node:fs/promises'),path=require('node:path'),assert=require('node:assert/strict');
const {spawn}=require('node:child_process');
const root=path.resolve(__dirname,'..'),sleep=ms=>new Promise(r=>setTimeout(r,ms));
let browser,socket;
(async()=>{
 const output=path.join(root,'work/works-polling');await fs.mkdir(output,{recursive:true});
 const items=[['failed','中断的浮力课件'],['cancelled','已取消的函数课件'],['succeeded','已完成的勾股定理']].map(([status,topic],i)=>({id:'work-'+i,type:'courseware',status,display_title:topic,mode:'sample',cost:20,input:{topic,pages:12,theme:'forest',preview_first:true},result:{},revision:1,events:[],sources:[],created_at:'2026-10-10T01:00:00Z'}));
 items[0].status='awaiting_preview';items[2].status='succeeded';
 let worksRequests=0,detailRequests=0,delayed=false,inFlight=0,maxInFlight=0,requestedQueries=[];
 let cleanup=0,restoreFail=false,deleteFail=false;const writes=[],errors=[],checks=[];
 let usage=Array.from({length:120},(_,i)=>({id:String(i+1),operation:'courseware.page',model:'fixture',input_tokens:10,output_tokens:5,latency_ms:100,success:true,created_at:new Date(Date.parse('2026-10-10T01:00:00Z')+i*60000).toISOString()}));
 browser=spawn('C:/Program Files (x86)/Microsoft/Edge/Application/msedge.exe',['--headless=new','--disable-gpu','--no-first-run','--disable-background-networking','--remote-debugging-port=9273','--remote-debugging-address=127.0.0.1','--user-data-dir='+path.join(output,'browser-profile'),'about:blank'],{windowsHide:true,stdio:'ignore'});
 let page;for(let i=0;i<50;i++){try{page=(await fetch('http://127.0.0.1:9273/json/list').then(r=>r.json())).find(p=>p.type==='page');if(page)break}catch{}await sleep(150)}
 assert.ok(page);socket=new WebSocket(page.webSocketDebuggerUrl);await new Promise((r,j)=>{socket.onopen=r;socket.onerror=j});

 const { call, evaluate } = createCDP(socket, m => {  if(m.method==='Runtime.exceptionThrown')errors.push(m.params.exceptionDetails.text);if(m.method!=='Fetch.requestPaused')return;
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
   if(u.searchParams.has('view')){worksRequests++;requestedQueries.push(u.searchParams.get('q'));inFlight++;maxInFlight=Math.max(maxInFlight,inFlight)}
   const view=u.searchParams.get('view');result=items.filter(t=>!t.deleted&&(view==='works'?(!['failed','cancelled'].includes(t.status)||t.restored_at):view==='interrupted'?(['failed','cancelled'].includes(t.status)&&!t.restored_at):true));
  }else if(p.endsWith('/restore')){
   if(restoreFail){code=503;result={message:'测试：暂时无法还原'}}else{items.find(t=>p.includes(t.id)).restored_at='2026-10-10T02:00:00Z';result={ok:true}}
  }else if(request.method==='DELETE'){
   if(deleteFail){code=409;result={message:'测试：仍有关联制作，请稍后删除'}}else{items.find(t=>p.endsWith(t.id)).deleted=true;cleanup=1;result={ok:true,cleanup_pending:false}}
  }else if(p.endsWith('/conversation'))result=[];
  else if(items.some(t=>p==='/api/tasks/'+t.id)){detailRequests++;result=items.find(t=>p.endsWith(t.id))}
  else{code=404;result={message:'Unexpected API'};errors.push(request.method+' '+p)}
  const fulfill=()=>call('Fetch.fulfillRequest',{requestId,responseCode:code,responseHeaders:[{name:'Content-Type',value:'application/json'}],body:Buffer.from(JSON.stringify(result)).toString('base64')}).then(()=>{if(p==='/api/tasks'&&u.searchParams.has('view'))inFlight--}).catch(e=>errors.push(String(e)));
  if(delayed&&p==='/api/tasks'&&u.searchParams.has('view'))setTimeout(fulfill,3000);else fulfill();
 });
 const wait=async expression=>{for(let i=0;i<100;i++){if(await evaluate(expression))return;await sleep(100)}throw Error('UI timeout '+expression)};
 const click=label=>evaluate(`(()=>{const b=[...document.querySelectorAll('button')].find(b=>b.textContent.includes(${JSON.stringify(label)}));if(!b||b.disabled)throw Error('Missing or disabled button '+${JSON.stringify(label)});b.click()})()`);
 const view=async name=>{await evaluate(`document.querySelector('[data-view=${name}]').click()`);await wait('!document.querySelector(".works-toolbar .primary").disabled')};
 await call('Runtime.enable');await call('Page.enable');await call('Fetch.enable',{patterns:[{urlPattern:'*/api/*'}]});
 await call('Page.addScriptToEvaluateOnNewDocument',{source:"sessionStorage.setItem('edu_token','fixture-token')"});
 await call('Emulation.setDeviceMetricsOverride',{width:1440,height:1000,deviceScaleFactor:1,mobile:false});
 await call('Page.navigate',{url:process.env.TEST_APP_URL||'http://127.0.0.1:8080'});await wait('!!document.querySelector(".home-workspace")');
 await click('我的作品');await wait('document.querySelectorAll(".task-card").length===2');await wait('!document.querySelector(".works-toolbar .primary").disabled');
 const idle=worksRequests;await sleep(4500);
 assert.equal(worksRequests,idle,'Waiting for teacher preview must not repeatedly query My Works');
 checks.push('Teacher review and completed works stay idle');
 items[0].status='expanding';await click('查询');await wait('!document.querySelector(".works-toolbar .primary").disabled');
 await evaluate('document.querySelector("[data-task-id=work-2] .work-checkbox input").click()');
 await evaluate('(()=>{const n=document.querySelector(".works-search input");n.value="尚未提交的搜索";n.dispatchEvent(new Event("input",{bubbles:true}))})()');
 await evaluate('window.loadingFlashes=0;new MutationObserver(()=>{if(document.querySelector(".works-toolbar .primary")?.textContent.includes("查询中"))window.loadingFlashes++}).observe(document.querySelector(".works-toolbar .primary"),{childList:true,subtree:true,characterData:true})');
 delayed=true;const active=worksRequests;for(let i=0;worksRequests===active&&i<140;i++)await sleep(100);
 assert.ok(worksRequests>active,'Generating works still refresh');await sleep(3400);
 assert.equal(maxInFlight,1,'Slow queries must not overlap');
 assert.equal(await evaluate('window.loadingFlashes'),0,'Background updates must not flash querying');
 assert.equal(await evaluate('document.querySelector("[data-task-id=work-2] .work-checkbox input").checked'),true,'Background refresh must preserve selected works');
 assert.equal(requestedQueries.at(-1),'','Background polling must not submit unfinished search text');
 checks.push('Generating works update quietly without overlaps, selection loss or submitting draft search');
 items[0].status='awaiting_preview';delayed=false;const before=worksRequests;for(let i=0;worksRequests===before&&i<140;i++)await sleep(100);await sleep(400);
 const stopped=worksRequests;await sleep(4500);assert.equal(worksRequests,stopped,'Polling stops after generation reaches review');
 checks.push('Background polling stops when work reaches teacher review');
 await evaluate('document.querySelector("[data-task-id=work-2] .task-open").click()');await wait('!!document.querySelector(".courseware-studio")');
 await click('我的作品');await wait('!!document.querySelector(".works-toolbar")&&!document.querySelector(".works-toolbar .primary").disabled');
 const hiddenDetail=detailRequests;await sleep(4500);assert.equal(detailRequests,hiddenDetail,'My Works must not continue polling the previously opened detail');
 checks.push('Returning to My Works stops polling the previous detail');
 assert.deepEqual(errors,[]);await fs.writeFile(path.join(output,'browser.json'),JSON.stringify({checks,worksRequests,detailRequests,maxInFlight},null,2));console.log(JSON.stringify({checks,worksRequests,detailRequests,maxInFlight}));
 await call('Browser.close');socket.close();
})().catch(e=>{console.error(e.stack);process.exitCode=1;socket?.close();browser?.kill()});
