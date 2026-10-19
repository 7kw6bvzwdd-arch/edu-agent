const createCDP = require('./browser-cdp.cjs');
const fs = require('node:fs/promises');
const path = require('node:path');
const {spawn} = require('node:child_process');
const {pathToFileURL} = require('node:url');
const assert = require('node:assert/strict');
const root = path.resolve(__dirname, '..');
const outputs = path.join(root, 'work');
const errors = [];
const checks = [];
const browser = spawn('C:/Program Files (x86)/Microsoft/Edge/Application/msedge.exe', [
  '--headless=new', '--disable-gpu', '--no-first-run', '--no-default-browser-check',
  '--disable-extensions', '--disable-background-networking',
  '--remote-debugging-port=9238', '--remote-debugging-address=127.0.0.1',
  '--user-data-dir='+path.join(root,'work','browser-profile'), 'about:blank'
], {windowsHide:true, stdio:'ignore'});
browser.on('error', e=>errors.push(String(e)));
let socket;
(async()=>{
  let pages;
  for(let i=0;i<50;i++){
    try { pages=await fetch('http://127.0.0.1:9238/json/list').then(r=>r.json()); if(pages.some(p=>p.type==='page'))break; }catch{}
    await new Promise(r=>setTimeout(r,200));
  }
  if(!pages)throw Error('Headless browser did not expose its debugging endpoint. '+errors.join('; '));
  socket = new WebSocket(pages.find(p=>p.type==='page').webSocketDebuggerUrl);
  await new Promise((resolve,reject)=>{socket.onopen=resolve;socket.onerror=reject});

  const { call, evaluate } = createCDP(socket, m => {
    if(m.method==='Runtime.exceptionThrown')errors.push(m.params.exceptionDetails.text+': '+(m.params.exceptionDetails.exception?.description||''));
  }, 10000);
  await call('Runtime.enable');await call('Page.enable');
  await call('Emulation.setDeviceMetricsOverride',{width:1440,height:1080,deviceScaleFactor:1,mobile:false});
  await call('Page.navigate',{url:'http://127.0.0.1:8080'});
  for(let i=0;i<30;i++){if(await evaluate("document.readyState==='complete' && !!document.querySelector('.login-form')"))break;await new Promise(r=>setTimeout(r,100));}
  const check=async(name,expr)=>{assert.equal(await evaluate(expr),true,name);checks.push(name)};

  const waitFor=async expr=>{for(let i=0;i<120;i++){if(await evaluate(expr))return;await new Promise(r=>setTimeout(r,250));}throw Error('Timed out waiting for '+expr)};
  const click=async text=>evaluate(`(()=>{const b=[...document.querySelectorAll('button')].find(x=>x.textContent.includes(${JSON.stringify(text)}));if(!b)throw Error('Missing button');b.click()})()`);
  const env=await fs.readFile(path.join(root,'.env'),'utf8');const password=env.match(/^SEED_PASSWORD=(.*)$/m)[1].trim();
  await evaluate(`(()=>{const p=document.querySelector('input[type=password]');p.value=${JSON.stringify(password)};p.dispatchEvent(new Event('input',{bubbles:true}));document.querySelector('.login-form').dispatchEvent(new Event('submit',{bubbles:true,cancelable:true}))})()`);
  await waitFor("!!document.querySelector('.workbench')");
  await check('Authenticated workspace renders','document.body.textContent.includes("成员独立额度")');
  await check('Desktop has no horizontal overflow','document.documentElement.scrollWidth <= innerWidth');
  const screenshot=await call('Page.captureScreenshot',{format:'png',captureBeyondViewport:false});
  await fs.writeFile(path.join(outputs,'workspace-desktop.png'),Buffer.from(screenshot.data,'base64'));
  await click('开始研究');
  await waitFor("document.body.textContent.includes('确认方案，开始制作')");
  await check('Unverified draft requires explicit consent',"[...document.querySelectorAll('button')].find(b=>b.textContent.includes('确认方案，开始制作')).disabled");
  await evaluate("document.querySelector('.consent input').click()");
  await click('确认方案，开始制作');
  await waitFor("document.body.textContent.includes('下载 PPTX')");
  await check('Eight editable slides after real generation',"document.querySelectorAll('.outline-slide').length===8");
  await evaluate("var x=document.querySelector('.outline-slide input');x.value='<img src=x onerror=alert(1)>';x.dispatchEvent(new Event('input',{bubbles:true}))");
  await click('课件预览');
  await check('Teacher input remains text and cannot execute HTML',"!document.querySelector('.slide-preview img') && document.querySelector('.slide-preview').textContent.includes('<img')");
  await click('大纲与编辑');
  await evaluate("var x=document.querySelector('.outline-slide input');x.value='教师修订 · 学习目标';x.dispatchEvent(new Event('input',{bubbles:true}))");
  await click('免费保存并重导出');
  await waitFor("document.body.textContent.includes('新修订已保存')");
  await click('课件预览');
  const readyShot=await call('Page.captureScreenshot',{format:'png',captureBeyondViewport:false});await fs.writeFile(path.join(outputs,'workspace-courseware.png'),Buffer.from(readyShot.data,'base64'));
  await click('查看制作过程');
  await waitFor("document.activeElement?.getAttribute('aria-label')==='关闭过程'");
  await check('Process dialog receives keyboard focus',"document.activeElement.getAttribute('aria-label')==='关闭过程'");
  await check('Persisted process tree is visible',"document.querySelector('[role=dialog]').textContent.includes('课件已生成并结算')");
  await evaluate("document.querySelector('[aria-label=关闭过程]').click()");
  await click('套餐与账单');await waitFor("document.body.textContent.includes('模拟购买（不真实扣款）')");
  await check('Simple member billing page exists',"document.body.textContent.includes('无周期清零') && document.querySelectorAll('.product').length===2");
  await click('知识与来源');await waitFor("document.body.textContent.includes('检索测试')");
  await check('Knowledge uploads accept agreed formats',"document.querySelector('input[type=file]').accept==='.pdf,.docx,.pptx,.txt,.md'");
  await click('课件工作台');await waitFor("!!document.querySelector('.workbench')");
  await call('Emulation.setDeviceMetricsOverride',{width:390,height:844,deviceScaleFactor:1,mobile:true});
  await check('Mobile workspace has no horizontal overflow','innerWidth===390 && document.documentElement.scrollWidth<=390');
  const mobile=await call('Page.captureScreenshot',{format:'png',captureBeyondViewport:false});await fs.writeFile(path.join(outputs,'workspace-mobile.png'),Buffer.from(mobile.data,'base64'));
  assert.deepEqual(errors,[],'No browser runtime errors');checks.push('No browser runtime errors');
  await fs.writeFile(path.join(outputs,'browser-validation.json'),JSON.stringify({checks,errors,scope:'Real local HTTP app, PostgreSQL accounting, Python PPTX; explicit sample content, no paid APIs.'},null,2));
  console.log(JSON.stringify({passed:checks.length,checks}));
  await call('Browser.close');socket.close();
})().catch(e=>{console.error(e.stack||e);process.exitCode=1;try{socket?.close()}catch{};browser.kill()});
