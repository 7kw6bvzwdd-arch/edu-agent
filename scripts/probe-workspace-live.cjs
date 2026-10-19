// Explicit live homepage smoke check. Uses a separate browser and own login.
// Never clicks generation or creates a work; credentials/responses stay private.
const fs=require('node:fs/promises'),path=require('node:path'),assert=require('node:assert/strict'),{spawn}=require('node:child_process');
const root=path.resolve(__dirname,'..'),base='http://127.0.0.1:8080',sleep=ms=>new Promise(r=>setTimeout(r,ms));
let browser,socket,token,serial=0,stage='login';const pending=new Map();
(async()=>{
 const env=Object.fromEntries((await fs.readFile(path.join(root,'.env'),'utf8')).split(/\r?\n/).filter(s=>/^[^#=]+=/.test(s)).map(s=>{const i=s.indexOf('=');return[s.slice(0,i).trim(),s.slice(i+1).trim()]}));
 async function api(url,method='GET',body){const r=await fetch(base+'/api'+url,{method,headers:{'Content-Type':'application/json',...(token?{Authorization:'Bearer '+token}:{})},body:body===undefined?undefined:JSON.stringify(body)});assert.ok(r.ok,'API HTTP '+r.status);return r.status===204?null:r.json()}
 try {
  token=(await api('/auth/login','POST',{username:'teacher',password:env.SEED_PASSWORD})).token;assert.ok(token);
  const before=await api('/billing'),tasks=await api('/tasks'),out=path.join(root,'work/conversation-first/connection');await fs.mkdir(out,{recursive:true});
  browser=spawn('C:/Program Files (x86)/Microsoft/Edge/Application/msedge.exe',['--headless=new','--disable-gpu','--no-first-run','--disable-background-networking','--remote-debugging-port=9397','--remote-debugging-address=127.0.0.1','--user-data-dir='+path.join(out,'profile'),'about:blank'],{windowsHide:true,stdio:'ignore'});
  let page;for(let i=0;i<60;i++){try{page=(await fetch('http://127.0.0.1:9397/json/list').then(r=>r.json())).find(p=>p.type==='page');if(page)break}catch{}await sleep(100)}assert.ok(page,'Browser starts');
  socket=new WebSocket(page.webSocketDebuggerUrl);await new Promise((resolve,reject)=>{socket.onopen=resolve;socket.onerror=reject});
  const call=(method,params={})=>new Promise((resolve,reject)=>{const id=++serial,timer=setTimeout(()=>{pending.delete(id);reject(Error('Browser command timed out'))},20000);pending.set(id,{resolve,reject,timer});socket.send(JSON.stringify({id,method,params}))});
  const errors=[];let requestCount=0,started;
  socket.onmessage=e=>{const m=JSON.parse(e.data);if(m.id){const p=pending.get(m.id);if(p){clearTimeout(p.timer);pending.delete(m.id);m.error?p.reject(Error('Browser command failed')):p.resolve(m.result)}return}if(m.method==='Runtime.exceptionThrown')errors.push('browser_exception');if(m.method==='Network.requestWillBeSent'&&new URL(m.params.request.url).pathname==='/api/workspace/brief')requestCount++};
  const evaluate=async expression=>{const v=await call('Runtime.evaluate',{expression,returnByValue:true,awaitPromise:true});assert.ok(!v.exceptionDetails,'Browser evaluation completes');return v.result.value};
  const wait=async expression=>{for(let i=0;i<1900;i++){if(await evaluate(expression))return;await sleep(100)}throw Error('Live homepage did not complete')};
  await call('Runtime.enable');await call('Page.enable');await call('Network.enable');
  await call('Emulation.setDeviceMetricsOverride',{width:1440,height:1000,deviceScaleFactor:1,mobile:false});await call('Page.navigate',{url:base});
  // Edge can open a first-run sign-in page after the first navigation.
  await sleep(1000);await call('Page.navigate',{url:base});
  await wait('document.title.includes("备课工坊")');
  await evaluate('sessionStorage.setItem("edu_token",'+JSON.stringify(token)+')');
  await call('Page.reload',{ignoreCache:true});
  stage='homepage_ready';await wait('!!document.querySelector(".brief-composer textarea")&&!document.querySelector(".brief-composer textarea").disabled');started=performance.now();
  await evaluate('(()=>{const input=document.querySelector(".brief-composer textarea");input.value="制作二次函数课件";input.dispatchEvent(new Event("input",{bubbles:true}));})()');
  await wait('!!document.querySelector(".brief-composer button[type=submit]")&&!document.querySelector(".brief-composer button[type=submit]").disabled');
  await evaluate('document.querySelector(".brief-composer button[type=submit]").click()');
  await wait('document.querySelectorAll(".brief-bubble.assistant").length>0&&!document.querySelector(".brief-messages").getAttribute("aria-busy").includes("true")');
  stage='rendered_response';const state=await evaluate('(()=>{const reply=document.querySelector(".brief-bubble.assistant");return{failed:reply.classList.contains("incomplete"),reply:!!reply.querySelector("p")?.textContent.trim(),blocks:document.querySelectorAll(".brief-current-requirements .brief-requirement-block").length,compact:!document.querySelector(".brief-current-requirements")?.open,repeated_tables:document.querySelectorAll(".brief-messages table").length,genericError:document.body.innerText.includes("暂时无法连接模型服务"),emptyInput:document.querySelector(".brief-composer textarea").value===""}})()');
  await fs.writeFile(path.join(out,'homepage-live.png'),Buffer.from((await call('Page.captureScreenshot',{format:'png'})).data,'base64'));
  assert.equal(state.compact,true);assert.equal(state.repeated_tables,0);assert.equal(state.failed,false);assert.equal(state.genericError,false);assert.ok(state.reply&&state.blocks>=2&&state.emptyInput);assert.equal(requestCount,1,'Single homepage request');assert.deepEqual(errors,[]);
  const after=await api('/billing'),afterTasks=await api('/tasks');for(const key of ['balance','reserved','available'])assert.equal(after[key],before[key]);assert.deepEqual(afterTasks,tasks);
  const health=await api('/health');assert.equal(health.mode,'live');const report={at:new Date().toISOString(),mode:health.mode,model:health.courseware_generation_model,request_count:requestCount,done_ms:Math.round(performance.now()-started),...state,task_list_unchanged:true,billing_unchanged:true};
  await fs.writeFile(path.join(out,'homepage-live.json'),JSON.stringify(report,null,2));process.stdout.write(JSON.stringify(report)+'\n');
  await call('Browser.close');
 } finally {if(token)await api('/auth/logout','POST',{}).catch(()=>{});socket?.close();browser?.kill();for(const p of pending.values())clearTimeout(p.timer)}
})().catch(e=>{process.stderr.write('Live homepage check failed at '+stage+' ('+String(e.code||e.name)+'); credentials and response suppressed.\n');process.stderr.write(String(e.stack).split('\n').filter(s=>s.includes('probe-workspace-live.cjs:')).join('\n')+'\n');process.exitCode=1});
