const createCDP = require('./browser-cdp.cjs');
// Test built UI against intercepted fixture APIs; no login, generation or writes reach the backend.
const fs = require('node:fs/promises');
const path = require('node:path');
const assert = require('node:assert/strict');
const {spawn} = require('node:child_process');
const root = path.resolve(__dirname, '..');
const sleep = ms => new Promise(resolve => setTimeout(resolve, ms));
let browser, socket;
(async () => {
  const deck = JSON.parse(await fs.readFile(path.join(root, 'fixtures/courseware-quality-v2.json'), 'utf8'));
  const media = JSON.parse(await fs.readFile(path.join(root, 'fixtures/courseware-teaching-media.json'), 'utf8'));
  deck.theme='classroom';deck.slides[1]=media.slides[1];deck.slides[4]=media.slides[2];deck.slides[6]=media.slides[3];
  const output=path.join(root,'work/courseware-teaching-media');await fs.mkdir(output,{recursive:true});
  const task = {id:'fixture-ppt',type:'courseware',status:'awaiting_confirmation',revision:1,mode:'sample',result:deck,sources:[],events:[],input:{topic:deck.title,subject:'物理',grade:'八年级',pages:8,theme:'forest',use_kb:true}};
  const actor = {tenant_id:1,member_id:1,name:'测试教师',role:'teacher',permissions:['courseware.generate']};
  const errors=[], checks=[];
  let confirmed, mediaVersion='media-v1';
  browser = spawn('C:/Program Files (x86)/Microsoft/Edge/Application/msedge.exe', ['--headless=new','--disable-gpu','--no-first-run','--disable-background-networking','--remote-debugging-port=9246','--remote-debugging-address=127.0.0.1','--user-data-dir='+path.join(root,'work','ppt-ui-profile'),'about:blank'],{windowsHide:true,stdio:'ignore'});
  let page;
  for(let i=0;i<40;i++){try{page=(await fetch('http://127.0.0.1:9246/json/list').then(r=>r.json())).find(p=>p.type==='page');if(page)break}catch{}await sleep(200)}
  assert.ok(page,'browser started');
  socket=new WebSocket(page.webSocketDebuggerUrl);
  await new Promise((resolve,reject)=>{socket.onopen=resolve;socket.onerror=reject});

  const { call, evaluate } = createCDP(socket, m => {
    if(m.method==='Runtime.exceptionThrown')errors.push(m.params.exceptionDetails.text);
    if(m.method==='Fetch.requestPaused'){
      const {requestId,request}=m.params;const url=new URL(request.url);let reply,code=200;
      if(url.pathname==='/api/tasks/fixture-ppt/events'){
        call('Fetch.fulfillRequest',{requestId,responseCode:200,responseHeaders:[{name:'Content-Type',value:'text/event-stream'}],body:Buffer.from(': fixture\n\n').toString('base64')}).catch(e=>errors.push(String(e)));return;
      }
      if(url.pathname==='/api/health')reply={status:'ok',mode:'sample',courseware_media_version:mediaVersion};
      else if(url.pathname==='/api/auth/me')reply=actor;
      else if(url.pathname==='/api/tenants')reply=[{id:1,name:'排版测试'}];
      else if(url.pathname==='/api/billing')reply={balance:100,reserved:20,available:80,plan:'basic',ledger:[]};
      else if(url.pathname==='/api/tasks'&&request.method==='GET')reply=[task];
      else if(url.pathname==='/api/tasks/fixture-ppt'&&request.method==='GET')reply=task;
      else if(url.pathname==='/api/tasks/fixture-ppt/confirm'&&request.method==='POST'){confirmed=JSON.parse(request.postData);reply={ok:true}}
      else {code=400;reply={message:'Unexpected fixture request'};errors.push(request.method+' '+url.pathname)}
      call('Fetch.fulfillRequest',{requestId,responseCode:code,responseHeaders:[{name:'Content-Type',value:'application/json'}],body:Buffer.from(JSON.stringify(reply)).toString('base64')}).catch(e=>errors.push(String(e)));
    }
  });
  const wait=async expression=>{for(let i=0;i<50;i++){if(await evaluate(expression))return;await sleep(100)}throw Error('UI timeout: '+expression)};
  const click=async label=>evaluate(`(()=>{const b=[...document.querySelectorAll('button')].find(b=>b.textContent.includes(${JSON.stringify(label)}));if(!b)throw Error('Button missing');b.click()})()`);
  await call('Runtime.enable');await call('Page.enable');
  await call('Fetch.enable',{patterns:[{urlPattern:'*/api/*'}]});
  await call('Page.addScriptToEvaluateOnNewDocument',{source:"sessionStorage.setItem('edu_token','fixture-token')"});
  await call('Emulation.setDeviceMetricsOverride',{width:1440,height:1080,deviceScaleFactor:1,mobile:false});
  await call('Page.navigate',{url:'http://127.0.0.1:8080'});
  await wait("!!document.querySelector('.workbench')");
  await click('我的作品');await wait("!!document.querySelector('.task-open')");
  await evaluate("document.querySelector('.task-open').click()");await wait("document.querySelectorAll('.courseware-media-editor').length===8");
  await evaluate("document.querySelectorAll('.courseware-media-editor').forEach(d=>d.open=true)");
  assert.equal(await evaluate("[...document.querySelectorAll('.courseware-media-editor select')].some(s=>s.value==='line')"),true);
  checks.push('Generated line chart and formula controls appear');
  await evaluate("(()=>{const d=document.querySelectorAll('.courseware-media-editor');const f=d[3].querySelector('input');f.value='F=mg';f.dispatchEvent(new Event('input',{bubbles:true}));const n=d[2].querySelector('input[type=number]');n.value='1.5';n.dispatchEvent(new Event('input',{bubbles:true}));const s=[...d[2].querySelectorAll('select')].find(x=>[...x.options].some(o=>o.value==='line'));s.value='bar';s.dispatchEvent(new Event('change',{bubbles:true}))})()");
  await evaluate("document.querySelectorAll('.courseware-evidence-editor').forEach(d=>d.open=true)");
  assert.equal(await evaluate("document.querySelector('.outline-toolbar select').value"),'classroom');
  assert.equal(await evaluate("document.querySelectorAll('.courseware-evidence-editor')[1].querySelector('input[type=file]').disabled"),true);
  await evaluate('('+(function(){
    const d=document.querySelectorAll('.courseware-evidence-editor')[1];
    const set=(n,v)=>{n.value=v;n.dispatchEvent(new Event('input',{bubbles:true}))};
    set(d.querySelector('input'),'08:30与2 min的含义相同吗？');
    set(d.querySelector('textarea'),'08:30\n2 min');
    set(d.querySelector('[aria-label="第2行第2列"]'),'08:31');
    set([...d.querySelectorAll('label')].find(l=>l.textContent.startsWith('节点1')).querySelector('input'),'起点甲站');
  }).toString()+')()');
  checks.push('Classroom theme and teaching table/figure/focus controls work; conflicting media is disabled');
  await evaluate('('+(function(){
    const d=document.querySelectorAll('.courseware-evidence-editor')[6];
    const n=d.querySelectorAll('input[type=number]')[2];n.value='33';n.dispatchEvent(new Event('input',{bubbles:true}));
    document.querySelector('[aria-label="预览第7页"]').click();
  }).toString()+')()');
  await wait("document.querySelectorAll('.interval-segment').length===3");
  const widths=await evaluate("[...document.querySelectorAll('.interval-segment')].map(s=>Number(s.getAttribute('width')))");
  assert.ok(Math.abs(widths[0]/widths[1]-10)<.001&&Math.abs(widths[2]/widths[1]-9)<.001);
  assert.ok(await evaluate("document.querySelector('.interval-values').textContent.includes('3\\u00a0min')"));
  await call('Emulation.setDeviceMetricsOverride',{width:390,height:844,deviceScaleFactor:1,mobile:true});
  assert.equal(await evaluate('document.documentElement.scrollWidth<=390'),true);
  await fs.writeFile(path.join(output,'browser-interval-mobile.png'),Buffer.from((await call('Page.captureScreenshot',{format:'png'})).data,'base64'));
  checks.push('Interval editor preserves numeric scale and readable labels on mobile');
  await call('Emulation.setDeviceMetricsOverride',{width:1440,height:1080,deviceScaleFactor:1,mobile:false});
  await click('大纲与编辑');await wait("document.querySelectorAll('.courseware-evidence-editor').length===8");
  await evaluate("document.querySelectorAll('.courseware-evidence-editor').forEach(d=>d.open=true)");
  await evaluate('('+(async function(){
    const canvas=document.createElement('canvas');canvas.width=400;canvas.height=200;
    const c=canvas.getContext('2d');c.fillStyle='#fff';c.fillRect(0,0,400,200);c.fillStyle='#009b51';c.fillRect(50,80,300,12);
    const blob=await new Promise(resolve=>canvas.toBlob(resolve,'image/png'));
    const transfer=new DataTransfer();transfer.items.add(new File([blob],'teacher-diagram.png',{type:'image/png'}));
    const input=document.querySelectorAll('.courseware-evidence-editor')[4].querySelector('input[type=file]');
    input.files=transfer.files;input.dispatchEvent(new Event('change',{bubbles:true}));
  }).toString()+')()');
  await wait("!!document.querySelectorAll('.courseware-evidence-editor')[4].querySelector('.image-thumbnail')");
  await evaluate('('+(function(){
    const d=document.querySelectorAll('.courseware-evidence-editor')[4];
    for(const [label,value] of [['图片说明','教师绘制的示意图'],['图片出处','教师自绘，测试素材']]){
      const n=[...d.querySelectorAll('label')].find(l=>l.textContent.startsWith(label)).querySelector('input');n.value=value;n.dispatchEvent(new Event('input',{bubbles:true}));
    }
    document.querySelector('[aria-label="预览第5页"]').click();
  }).toString()+')()');
  await wait("!!document.querySelector('.evidence-picture img')");
  assert.equal(await evaluate("document.querySelector('.evidence-picture img').alt"),'教师绘制的示意图');
  assert.ok(await evaluate("document.querySelectorAll('.evidence-table .teaching-emphasis').length>0"));
  await fs.writeFile(path.join(output,'browser-preview.png'),Buffer.from((await call('Page.captureScreenshot',{format:'png'})).data,'base64'));
  await call('Emulation.setDeviceMetricsOverride',{width:390,height:844,deviceScaleFactor:1,mobile:true});
  assert.equal(await evaluate('document.documentElement.scrollWidth<=390'),true);
  checks.push('Image upload, attribution and table/image preview work on desktop and mobile');
  await click('大纲与编辑');await wait("document.querySelectorAll('.courseware-evidence-editor').length===8");
  await evaluate("document.querySelectorAll('.courseware-evidence-editor').forEach(d=>d.open=true)");
  await evaluate("document.querySelector('.consent input').click()");await click('确认方案，开始制作');
  for(let i=0;i<30&&!confirmed;i++)await sleep(100);
  assert.ok(confirmed,'Confirmation was submitted');
  assert.equal(confirmed.result.theme,'classroom');
  assert.equal(confirmed.result.slides[1].focus,'08:30与2 min的含义相同吗？');
  assert.deepEqual(confirmed.result.slides[1].emphasis,['08:30','2 min']);
  assert.equal(confirmed.result.slides[1].table.rows[1][1],'08:31');
  assert.equal(confirmed.result.slides[1].figure.items[0].label,'起点甲站');
  assert.deepEqual(confirmed.result.slides[6].figure.values,[0,30,33,60]);
  assert.equal(confirmed.result.slides[6].figure.unit,'min');
  assert.equal(confirmed.result.slides[4].picture.source,'教师自绘，测试素材');
  assert.match(confirmed.result.slides[4].picture.data_url,/^data:image\/png;base64,/);
  checks.push('Teaching media and uploaded image reach the confirmation payload');
  const rendered=await fetch((process.env.TEST_RENDER_URL||'http://127.0.0.1:8001')+'/render/pptx',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify(confirmed.result)});
  if(!rendered.ok)throw Error('Renderer failed: '+await rendered.text());
  await fs.writeFile(path.join(output,'browser-edited.pptx'),Buffer.from(await rendered.arrayBuffer()));
  checks.push('Browser-edited deck exports through the actual PPTX renderer');
  assert.equal(confirmed.result.slides[3].formula,'F=mg');
  assert.equal(confirmed.result.slides[2].chart.values[0],1.5);
  assert.equal(confirmed.result.slides[2].chart.style,'bar');
  checks.push('Edited formula, numeric data and chart style reach confirmation payload');
  await call('Emulation.setDeviceMetricsOverride',{width:390,height:844,deviceScaleFactor:1,mobile:true});
  assert.equal(await evaluate('document.documentElement.scrollWidth<=390'),true);
  checks.push('Mobile editor has no horizontal overflow');
  mediaVersion='';confirmed=undefined;
  await call('Page.reload');await wait("!!document.querySelector('.workbench')");
  await click('我的作品');await wait("!!document.querySelector('.task-open')");
  await evaluate("document.querySelector('.task-open').click()");await wait("!!document.querySelector('.consent input')");
  await evaluate("document.querySelector('.consent input').click()");await click('确认方案，开始制作');
  await wait("document.querySelector('.alert.error')?.textContent.includes('课件服务尚未更新')");
  assert.equal(confirmed,undefined,'Stale backend must not receive unsupported content');
  checks.push('Outdated backend blocks submission without dropping edited media');
  assert.deepEqual(errors,[]);
  await fs.writeFile(path.join(output,'browser.json'),JSON.stringify({checks,errors,scope:'Fixture-only API interception; zero production writes'},null,2));
  console.log(JSON.stringify({checks}));
  await call('Browser.close');socket.close();
})().catch(e=>{console.error(e.stack);process.exitCode=1;socket?.close();browser?.kill()});
