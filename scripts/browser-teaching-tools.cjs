// Intercept every API request: no production account, task or model writes.
const fs=require('node:fs/promises'),path=require('node:path'),assert=require('node:assert/strict');
const {spawn}=require('node:child_process');
const root=path.resolve(__dirname,'..'),sleep=ms=>new Promise(r=>setTimeout(r,ms));
let browser,socket;
(async()=>{
 const output=path.join(root,'work/teaching-tools/browser');await fs.mkdir(output,{recursive:true});
 const deck=JSON.parse(await fs.readFile(path.join(root,'fixtures/courseware-quality-v2.json'),'utf8'));
 deck.slides=deck.slides.slice(0,3);deck.slides.forEach((s,i)=>s.id='slide-'+i);
 const png=Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+jRZkAAAAASUVORK5CYII=','base64');
 const task={id:'fixture-progress',type:'courseware',status:'expanding',mode:'sample',revision:1,cost:20,input:{topic:'浮力',pages:12,preview_first:true,theme:'classroom'},result:deck,events:[],sources:[],preview_images_available:true,slide_reviews:[]};
 const resume='已恢复逐页检查点，继续只处理问题页和未完成页；未变更页面的审核保留。';deck.notice='请教师核验。 '+resume.repeat(3);
 let previews=0,polls=0,eventID=0,failBillingOnce=false;const errors=[],checks=[];
 function event(stage){task.events.push({id:++eventID,stage,message:stage})}
 event('batch_start_4_6');
 browser=spawn('C:/Program Files (x86)/Microsoft/Edge/Application/msedge.exe',['--headless=new','--disable-gpu','--no-first-run','--disable-background-networking','--remote-debugging-port=9277','--remote-debugging-address=127.0.0.1','--user-data-dir='+path.join(output,'browser-profile'),'about:blank'],{windowsHide:true,stdio:'ignore'});
 let page;
 for(let i=0;i<40;i++){try{page=(await fetch('http://127.0.0.1:9277/json/list').then(r=>r.json())).find(p=>p.type==='page');if(page)break}catch{}await sleep(200)}
 assert.ok(page,'Edge starts');socket=new WebSocket(page.webSocketDebuggerUrl);
 await new Promise((resolve,reject)=>{socket.onopen=resolve;socket.onerror=reject});
 let seq=0;const pending=new Map();
 const call=(method,params={})=>new Promise((resolve,reject)=>{const id=++seq,timer=setTimeout(()=>reject(Error('CDP timeout: '+method)),15000);pending.set(id,{resolve,reject,timer});socket.send(JSON.stringify({id,method,params}))});
 socket.onmessage=e=>{
  const m=JSON.parse(e.data);
  if(m.id){const p=pending.get(m.id);if(p){clearTimeout(p.timer);pending.delete(m.id);m.error?p.reject(Error(JSON.stringify(m.error))):p.resolve(m.result)}}
  if(m.method==='Runtime.exceptionThrown')errors.push(m.params.exceptionDetails.text);
  if(m.method!=='Fetch.requestPaused')return;
  const {requestId,request}=m.params,p=new URL(request.url).pathname;
  let reply,body,contentType='application/json',code=200;
  if(p==='/api/courseware/materials/preview'){reply={pages:[{page:1,text:'第十九章 一次函数'},{page:2,text:'y=kx+b，k不为零'}]}}
  else if(p==='/api/courseware/materials')reply=[{id:'textbook1',name:'人教版数学八年级',pages:2,curriculum:{subject:'物理',grade:'八年级',edition:'人教版'},chapters:[{title:'第十九章 一次函数',start:1,end:2}]}];
  else if(p==='/api/courseware/references')reply=[];
  else if(request.method!=='GET'){errors.push('Unexpected write: '+p);code=400;reply={message:'No writes allowed'}}
  else if(p==='/api/health')reply={status:'ok',mode:'sample',courseware_studio_version:'studio-v1',courseware_media_version:'media-v1',teaching_features_version:'teaching-v1',works_view_version:'views-v1'};
  else if(p==='/api/auth/me')reply={tenant_id:1,member_id:1,name:'测试教师',role:'teacher',permissions:['courseware.generate']};
  else if(p==='/api/tenants')reply=[{id:1,name:'测试'}];
  else if(p==='/api/billing'){if(failBillingOnce){failBillingOnce=false;code=503;reply={message:'测试瞬时网络错误'}}else reply={balance:100,reserved:task.status==='failed'?0:20,available:task.status==='failed'?100:80,plan:'basic',ledger:[]}}
  else if(p==='/api/tasks')reply=[task];
  else if(p==='/api/tasks/'+task.id){polls++;reply=task}
  else if(p.endsWith('/conversation'))reply=[];
  else if(p.endsWith('/events')){contentType='text/event-stream';body=Buffer.from('id: '+eventID+'\ndata: '+JSON.stringify(task.events.at(-1))+'\n\n')}
  else if(p.endsWith('/preview')){previews++;contentType='image/png';body=png}
  else{errors.push('Unexpected API: '+p);code=404;reply={}}
  call('Fetch.fulfillRequest',{requestId,responseCode:code,responseHeaders:[{name:'Content-Type',value:contentType}],body:(body||Buffer.from(JSON.stringify(reply))).toString('base64')}).catch(e=>errors.push(String(e)));
 };
 const evaluate=async expression=>{const r=await call('Runtime.evaluate',{expression,returnByValue:true,awaitPromise:true});if(r.exceptionDetails)throw Error(r.exceptionDetails.exception?.description||r.exceptionDetails.text);return r.result.value};
 const wait=async expression=>{for(let i=0;i<100;i++){if(await evaluate(expression))return;await sleep(100)}throw Error('UI timeout: '+expression+'\n'+await evaluate('document.body.innerText.slice(-2000)')+'\n'+JSON.stringify(errors))};
 const click=label=>evaluate(`(()=>{const b=[...(document.querySelector('.classroom-presentation')||document).querySelectorAll('button')].find(b=>b.textContent.includes(${JSON.stringify(label)}));if(!b||b.disabled)throw Error('Missing button');b.click()})()`);
 const srcs=()=>evaluate("[...document.querySelectorAll('.studio-slide-picture img')].map(i=>i.src)");
 await call('Runtime.enable');await call('Page.enable');await call('Fetch.enable',{patterns:[{urlPattern:'*/api/*'}]});
 await call('Page.addScriptToEvaluateOnNewDocument',{source:"sessionStorage.setItem('edu_token','fixture-token')"});
 await call('Emulation.setDeviceMetricsOverride',{width:1440,height:1080,deviceScaleFactor:1,mobile:false});
 await call('Page.navigate',{url:process.env.TEST_APP_URL||'http://127.0.0.1:8080'});
 await wait("[...document.querySelectorAll('nav button')].some(b=>b.textContent.includes('我的作品'))");
 await click('我的作品');await wait("!!document.querySelector('.task-open')");await evaluate("document.querySelector('.task-open').click()");
 await wait("document.querySelectorAll('.studio-slide-picture img').length===3");
 task.status='succeeded';task.revision++;
 deck.slides[0]={...deck.slides[0],layout:'exercise',title:'计算与解释',bullets:['计算2+3','解释你的计算方法'],answer:'答案是5',notes:'TEACHER_ONLY_HINT'};
 deck.slides[1]={...deck.slides[1],title:'三角形',scientific:{kind:'triangle',title:'三边关系',sides:[3,4,5]},graph:null,diagram:'',chart:null,table:null,figure:null};
 deck.slides[2]={...deck.slides[2],title:'断路电流',scientific:{kind:'circuit',title:'断开的串联电路',voltage:6,resistances:[10,20],connection:'series',closed:false},graph:null,diagram:'',chart:null,table:null,figure:null};
 await evaluate("document.querySelector('.studio-header-actions button').click()");
 await click('我的作品');await evaluate("document.querySelector('.task-open').click()");
 await wait("[...document.querySelectorAll('button')].some(b=>b.textContent==='课堂演示')");
 await click('课堂演示');await wait("!!document.querySelector('.classroom-presentation')");
 assert.equal(await evaluate("document.querySelector('.classroom-presentation').innerText.includes('TEACHER_ONLY_HINT')"),false);
 assert.equal(await evaluate("document.querySelector('.classroom-presentation').innerText.includes('答案是5')"),false);
 assert.equal(await evaluate("getComputedStyle(document.querySelector('.presentation-body li')).visibility"),'hidden');
 await click('下一步');assert.equal(await evaluate("getComputedStyle(document.querySelector('.presentation-body li')).visibility"),'visible');
 await click('揭示答案');await wait("document.querySelector('.presentation-answer')?.textContent==='答案是5'");
 await click('下一步');await click('下一步');await wait("!!document.querySelector('.presentation-page .scientific-preview svg')");
 assert.equal(await evaluate("!!document.querySelector('.presentation-answer')"),false);
 await call('Page.captureScreenshot',{format:'png'}).then(r=>fs.writeFile(path.join(output,'classroom.png'),Buffer.from(r.data,'base64')));
 for(let i=0;i<10&&!await evaluate("document.querySelector('.presentation-page h1')?.textContent==='断路电流'");i++)await click('下一步');
 await wait("document.querySelector('.scientific-preview').innerText.includes('总电流 0 A')");
 assert.equal(await evaluate("document.querySelector('.scientific-switch')?.getAttribute('d')"),'M35 75 V98 M35 128 V180 M35 98 L53 117');
 await click('退出');await wait("!document.querySelector('.classroom-presentation')");
 await click('新建课件');await wait("!!document.querySelector('.teaching-materials')");
 const material=await evaluate("(()=>{const s=[...document.querySelectorAll('.teaching-materials label')].find(l=>l.textContent.startsWith('教材原文')).querySelector('select');s.value='textbook1';s.dispatchEvent(new Event('change',{bubbles:true}));return s.value})()");assert.equal(material,'textbook1');
 await click('查看匹配原文');await wait("document.querySelector('.teaching-materials').innerText.includes('y=kx+b，k不为零')");
 assert.ok(await evaluate("document.querySelector('.config-panel').innerText.includes('人教版')"));
 await call('Emulation.setDeviceMetricsOverride',{width:390,height:844,deviceScaleFactor:1,mobile:true});
 assert.ok(await evaluate('document.documentElement.scrollWidth<=innerWidth'));
 checks.push('Step reveal and hidden teacher notes; explicit answer reveal; scientific classroom preview; textbook original page selection; mobile width');
 assert.deepEqual(errors,[]);await fs.writeFile(path.join(output,'checks.json'),JSON.stringify({checks,errors},null,2));
 console.log(JSON.stringify({checks}));await call('Browser.close');socket.close();
})().catch(e=>{console.error(e.stack);process.exitCode=1;socket?.close();browser?.kill()});
