// Node 22+, Edge. Use an isolated sample database on :8081; never reads .env.
const assert = require('node:assert/strict');
const fs = require('node:fs/promises');
const path = require('node:path');
const os = require('node:os');
const {spawn, spawnSync} = require('node:child_process');
const {randomUUID} = require('node:crypto');
const base = new URL(process.env.EDU_TEST_URL || 'http://127.0.0.1:8081').origin;
const output = path.resolve(__dirname, '../work');
const checks = [], errors = [], pending = new Map();
let browser, socket, profile, sequence = 0, token = '', failure;
const sleep = ms => new Promise(resolve => setTimeout(resolve, ms));
async function request(route, method='GET', body, expected=200, bearer=token) {
  const r = await fetch(base+'/api'+route, {method, signal:AbortSignal.timeout(15000),
    headers:{'Content-Type':'application/json', ...(bearer ? {Authorization:'Bearer '+bearer} : {})},
    ...(body === undefined ? {} : {body:JSON.stringify(body)})});
  assert.equal(r.status, expected, method+' '+route+' HTTP status');
  return r.headers.get('content-type')?.includes('json') ? r.json() : r.arrayBuffer();
}
async function waitFor(name, check, timeout=30000) {
  const end=Date.now()+timeout;
  do {if(await check())return;await sleep(150)} while(Date.now()<end);
  throw Error('Timed out: '+name);
}
function call(method, params={}) {
  return new Promise((resolve,reject)=>{
    const id=++sequence, timer=setTimeout(()=>{pending.delete(id);reject(Error('CDP timeout: '+method))},15000);
    pending.set(id,{resolve,reject,timer});socket.send(JSON.stringify({id,method,params}));
  });
}
async function evaluate(expression) {
  const r=await call('Runtime.evaluate',{expression,returnByValue:true,awaitPromise:true});
  if(r.exceptionDetails)throw Error(r.exceptionDetails.exception?.description||r.exceptionDetails.text);
  return r.result.value;
}
async function uiWait(name, expression, timeout=30000) {
  await waitFor(name,async()=>{
    const result=await evaluate(`({ok:!!(${expression}),error:document.querySelector('.alert.error')?.textContent})`);
    assert.ok(!result.error,name+': '+result.error);return result.ok;
  },timeout);
}
function passed(name){checks.push(name);console.log('PASS '+name)}
async function check(name,expression){assert.equal(await evaluate(expression),true,name);passed(name)}
async function setValue(selector,value,index=0) {
  await evaluate(`(()=>{const x=document.querySelectorAll(${JSON.stringify(selector)})[${index}];
    if(!x||x.disabled)throw Error('Missing or disabled field');x.value=${JSON.stringify(value)};
    x.dispatchEvent(new Event(x.tagName==='SELECT'?'change':'input',{bubbles:true}))})()`);
}
async function click(text,scope='document') {
  await evaluate(`(()=>{const b=[...${scope}.querySelectorAll('button')].find(x=>x.textContent.includes(${JSON.stringify(text)}));
    if(!b||b.disabled)throw Error('Missing or disabled button: '+${JSON.stringify(text)});b.click()})()`);
}
async function screenshot(name) {
  const shot=await call('Page.captureScreenshot',{format:'png',captureBeyondViewport:false});
  await fs.writeFile(path.join(output,'onboarding-'+name+'.png'),Buffer.from(shot.data,'base64'));
}
async function main() {
  assert.notEqual(base,new URL('http://127.0.0.1:8080').origin,'Use the isolated test server');
  let health;
  await waitFor('sample server readiness',async()=>{try{health=await request('/health');assert.equal(health.mode,'sample','Refusing live server');return true}catch(e){if(e instanceof assert.AssertionError)throw e;return false}});
  await fs.mkdir(output,{recursive:true});profile=await fs.mkdtemp(path.join(os.tmpdir(),'edu-onboarding-'));
  browser=spawn('C:/Program Files (x86)/Microsoft/Edge/Application/msedge.exe',[
    '--headless=new','--disable-gpu','--no-first-run','--no-default-browser-check','--disable-extensions',
    '--remote-debugging-port=0','--remote-debugging-address=127.0.0.1','--user-data-dir='+profile,'about:blank'
  ],{windowsHide:true,stdio:'ignore'});
  browser.on('error',e=>errors.push(e.message));
  let debugPort;
  await waitFor('isolated browser',async()=>{try{debugPort=(await fs.readFile(path.join(profile,'DevToolsActivePort'),'utf8')).split('\n')[0];return !!debugPort}catch{return false}});
  const pages=await fetch('http://127.0.0.1:'+debugPort+'/json/list').then(r=>r.json());
  socket=new WebSocket(pages.find(p=>p.type==='page').webSocketDebuggerUrl);
  await new Promise((resolve,reject)=>{socket.onopen=resolve;socket.onerror=reject});
  socket.onmessage=event=>{const m=JSON.parse(event.data);
    if(m.method==='Runtime.exceptionThrown')errors.push(m.params.exceptionDetails.text);
    const p=pending.get(m.id);if(p){pending.delete(m.id);clearTimeout(p.timer);m.error?p.reject(Error(JSON.stringify(m.error))):p.resolve(m.result)}
  };
  await call('Runtime.enable');await call('Page.enable');
  await call('Emulation.setDeviceMetricsOverride',{width:1440,height:1080,deviceScaleFactor:1,mobile:false});
  await call('Page.navigate',{url:base});await uiWait('login panel',"!!document.querySelector('.auth-panel')");
  await check('Login includes registration and recovery',"document.body.textContent.includes('注册账号')&&document.body.textContent.includes('忘记密码？')");
  await screenshot('login-desktop');
  await call('Emulation.setDeviceMetricsOverride',{width:390,height:844,deviceScaleFactor:1,mobile:true});
  await check('Login fits mobile',"document.documentElement.scrollWidth<=390");await screenshot('login-mobile');
  await call('Emulation.setDeviceMetricsOverride',{width:1440,height:1080,deviceScaleFactor:1,mobile:false});
  const username='onboard-'+randomUUID().slice(0,12),password='test-secure-pass-123',newPassword='test-new-pass-456';
  await click('注册账号',"document.querySelector('.auth-tabs')");
  await setValue('[autocomplete=username]',username);await setValue('[autocomplete=name]','验收教师');await setValue('[autocomplete=organization]','验收独立空间');
  await setValue('[autocomplete=new-password]',password);await setValue('[autocomplete=new-password]',password,1);
  await evaluate("document.querySelector('form[aria-label=注册账号]').requestSubmit()");
  await uiWait('registered recovery code',"!!document.querySelector('.recovery-card code')");
  const code=await evaluate("document.querySelector('.recovery-card code').textContent");assert.ok(code.length>=32);
  const oldSession=await request('/auth/login','POST',{username,password},200,'');
  const oldToken=oldSession.token;
  await evaluate("document.querySelector('.inline-check input').click()");await click('返回登录');await click('忘记密码？');
  await setValue('[autocomplete=username]',username);await setValue('[autocomplete=off]',code);
  await setValue('[autocomplete=new-password]',newPassword);await setValue('[autocomplete=new-password]',newPassword,1);
  await evaluate("document.querySelector('form[aria-label=找回密码]').requestSubmit()");
  await uiWait('reset completed',"document.body.textContent.includes('密码重置成功')&&!!document.querySelector('.recovery-card code')");
  assert.notEqual(await evaluate("document.querySelector('.recovery-card code').textContent"),code);
  await request('/auth/me','GET',undefined,401,oldToken);
  await request('/auth/login','POST',{username,password},401,'');
  await request('/auth/reset-password','POST',{username,password:'test-another-pass-789',recovery_code:code},400,'');
  passed('Registration and reset work; old password, session and recovery code fail');
  await evaluate("document.querySelector('.inline-check input').click()");await click('返回登录');
  await setValue('[autocomplete=username]',username);await setValue('[autocomplete=current-password]',newPassword);
  await evaluate("document.querySelector('form[aria-label=登录]').requestSubmit()");
  await uiWait('workspace',"!!document.querySelector('.home-workspace')");await click('课件工作台');await uiWait('briefing',"!!document.querySelector('.brief-output')");token=await evaluate("sessionStorage.getItem('edu_token')");
  const initial=await request('/billing');assert.equal(initial.available,0);assert.equal(initial.plan,'');assert.deepEqual(await request('/tasks'),[]);assert.deepEqual(await request('/documents'),[]);
  passed('New registration has its own space and zero points');
  const order=await request('/billing/orders','POST',{sku:'basic'},201);await request('/billing/orders/'+order.id+'/pay','POST',{});
  await call('Page.reload');await uiWait('home',"!!document.querySelector('.home-workspace')");await click('课件工作台');await uiWait('ready briefing',"!!document.querySelector('.brief-summary button')&&!document.querySelector('.brief-summary button').disabled");
  await setValue('.config-panel select',8,3);
  await evaluate("const x=document.querySelectorAll('.check-card input')[1];if(x.checked)x.click()");
  const marker='只使用课堂实验观察，不引入高中公式。';
  const summary=await evaluate("document.querySelector('.brief-summary textarea').value");
  await setValue('.brief-summary textarea',summary+'\n'+marker);
  await setValue('[aria-label=生成前需求输入]','45分钟，学生基础偏弱，三道由浅入深的练习。');
  const before=await request('/billing');
  await click('发送',"document.querySelector('.brief-composer')");await uiWait('brief reply',"document.querySelectorAll('.brief-bubble.assistant').length===2");
  assert.deepEqual(await request('/billing'),before);assert.deepEqual(await request('/tasks'),[]);
  passed('Requirement discussion creates no task and reserves no points');
  await check('Further discussion preserves manual summary',`document.querySelector('.brief-summary textarea').value.includes(${JSON.stringify(marker)})`);
  await evaluate("document.querySelector('.horizontal-themes input[value=classroom]').click()");
  await check('Template selection preserves manual summary',`document.querySelector('.brief-summary textarea').value.includes(${JSON.stringify(marker)})`);
  await check('Templates follow the briefing in one horizontal row',`(()=>{const a=document.querySelector('.brief-output'),b=document.querySelector('.template-shelf'),t=document.querySelector('.horizontal-themes');return !!(a.compareDocumentPosition(b)&Node.DOCUMENT_POSITION_FOLLOWING)&&getComputedStyle(t).display==='flex'&&getComputedStyle(t).flexWrap==='nowrap'&&t.querySelectorAll('.theme').length===7})()`);
  await evaluate("document.querySelector('.brief-output').scrollIntoView({block:'start'})");await screenshot('brief-desktop');
  await call('Emulation.setDeviceMetricsOverride',{width:390,height:844,deviceScaleFactor:1,mobile:true});
  await check('Briefing fits mobile',"document.documentElement.scrollWidth<=390");await screenshot('brief-mobile');
  await call('Emulation.setDeviceMetricsOverride',{width:1440,height:1080,deviceScaleFactor:1,mobile:false});
  await setValue('[aria-label=生成前需求输入]','最后补充：先提问，再解释浮力方向。');
  await click('确认需求，生成前三页',"document.querySelector('.brief-summary')");
  await uiWait('three-slide preview',"document.querySelectorAll('.studio-slide').length===3&&document.querySelector('.studio-stage-banner').textContent.includes('先看看前三页')",180000);
  const works=await request('/tasks');assert.equal(works.length,1);const task=await request('/tasks/'+works[0].id);
  assert.equal(task.input.theme,'classroom');assert.ok(task.input.requirements.includes(marker));assert.ok(task.input.requirements.includes('最后补充'));assert.equal(task.status,'awaiting_preview');assert.equal(task.result.slides.length,3);assert.equal(task.charge_state,'reserved');
  await check('Unapproved preview cannot expand',"[...document.querySelectorAll('.studio-completion button')].find(b=>b.textContent.includes('确认前三页')).disabled");
  const file=Buffer.from(await request('/tasks/'+task.id+'/preview-pptx'));assert.ok(file.length>1000&&file.subarray(0,2).toString()==='PK');
  passed('Confirmed requirements persist; exactly three real PPTX pages await review');
  await check('Horizontal templates remain available below preview pages',"document.querySelectorAll('.studio-canvas .studio-templates .theme').length===7 && getComputedStyle(document.querySelector('.studio-templates .horizontal-themes')).display==='flex'");
  await evaluate("document.querySelector('.studio-templates input[value=coral]').click()");
  await check('Preview swatches update the editable template',"document.querySelector('.studio-toolbar select').value==='coral'&&!!document.querySelector('.unsaved-note')");
  await click('保存修改',"document.querySelector('.studio-toolbar')");
  await uiWait('template saved',"!document.querySelector('.unsaved-note')&&!document.querySelector('.studio-toolbar select').disabled",120000);
  assert.equal((await request('/tasks/'+task.id)).result.theme,'coral');
  assert.equal((await request('/billing')).balance,before.balance);
  passed('Changing preview template saves a real revision without charging');
  await evaluate("document.querySelector('.studio-templates').scrollIntoView({block:'center'})");
  await screenshot('preview-desktop');
  await call('Emulation.setDeviceMetricsOverride',{width:390,height:844,deviceScaleFactor:1,mobile:true});
  await check('Preview and horizontal template strip fit mobile',"document.documentElement.scrollWidth<=390 && document.querySelector('.studio-templates .horizontal-themes').getBoundingClientRect().right<=390");
  await evaluate("document.querySelector('.studio-templates').scrollIntoView({block:'center'})");await screenshot('preview-mobile');
  await call('Emulation.setDeviceMetricsOverride',{width:1440,height:1080,deviceScaleFactor:1,mobile:false});
  await click('一键审核通过',"document.querySelector('.studio-completion')");
  await uiWait('three slides approved',"[...document.querySelectorAll('.studio-completion button')].find(b=>b.textContent.includes('确认前三页'))?.disabled===false");
  const quota=await request('/billing');assert.equal(quota.balance,before.balance);assert.equal(quota.reserved,20);
  await request('/tasks/'+task.id+'/cancel','POST',{});assert.equal((await request('/billing')).reserved,0);assert.equal((await request('/billing')).balance,before.balance);
  passed('Review gate opens only after approval; cancellation releases all 20 points');
  assert.deepEqual(errors,[]);passed('No browser runtime errors');
}
(async()=>{
  try{await main()}catch(e){failure=e.stack||String(e);console.error(failure)}
  finally{
    try{if(socket?.readyState===WebSocket.OPEN)await call('Browser.close')}catch{}
    socket?.close();for(const p of pending.values())clearTimeout(p.timer);
    if(browser?.pid&&browser.exitCode===null){await Promise.race([new Promise(r=>browser.once('exit',r)),sleep(3000)]);if(browser.exitCode===null)spawnSync('taskkill',['/PID',String(browser.pid),'/T','/F'],{windowsHide:true,stdio:'ignore'})}
    if(profile){assert.equal(path.dirname(path.resolve(profile)),path.resolve(os.tmpdir()));assert.ok(path.basename(profile).startsWith('edu-onboarding-'));await fs.rm(profile,{recursive:true,force:true,maxRetries:5,retryDelay:300})}
    await fs.mkdir(output,{recursive:true});await fs.writeFile(path.join(output,'onboarding-validation.json'),JSON.stringify({url:base,checks,errors,failure:failure||null,scope:'Independent sample database; real HTTP registration/reset, summary confirmation, real PPTX preview; no paid providers.'},null,2));
    if(failure)process.exitCode=1;
  }
})().catch(e=>{console.error(e);process.exitCode=1});
