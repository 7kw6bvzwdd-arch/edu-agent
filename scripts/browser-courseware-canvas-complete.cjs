// Intercept every API request: no production account, task or model writes.
const fs=require('node:fs/promises'),path=require('node:path'),assert=require('node:assert/strict');
const {spawn}=require('node:child_process');
const root=path.resolve(__dirname,'..'),sleep=ms=>new Promise(r=>setTimeout(r,ms));
let browser,socket;
(async()=>{
 const output=path.join(root,'work/canvas-complete/browser');await fs.mkdir(output,{recursive:true});
 const deck=JSON.parse(await fs.readFile(path.join(root,'fixtures/courseware-quality-v2.json'),'utf8'));
 deck.slides[0].scientific={kind:'forces',title:'受力示意',object:'木块',unit:'N',equilibrium:true,forces:[{label:'重力',x:0,y:-5},{label:'支持力',x:0,y:5}]};deck.slides=deck.slides.slice(0,3);deck.slides.forEach((s,i)=>s.id='slide-'+i);
 const png=Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+jRZkAAAAASUVORK5CYII=','base64');
 const task={id:'fixture-progress',type:'courseware',status:'awaiting_preview',mode:'sample',revision:1,cost:20,input:{topic:'浮力',pages:12,preview_first:true,theme:'classroom'},result:deck,events:[],sources:[{id:'S1',hash:'fixture-hash',location:'文件第7页',file_page:7,content:'构造样例原文：浮力方向竖直向上。',textbook:{name:'原文证据样例.pdf',edition:'人教版 教师标注样例版次',chapter:'样例章节'}}],preview_images_available:true,slide_reviews:[]};
 deck.slides[0].evidence=[{claim:'浮力方向竖直向上',source_id:'S1',source_hash:'fixture-hash',quote:'浮力方向竖直向上。'}];
 task.slide_reviews=deck.slides.map(s=>({id:s.id,hash:'hash-'+s.id,approved:false,vote:0}));
 const resume='已恢复逐页检查点，继续只处理问题页和未完成页；未变更页面的审核保留。';deck.notice='请教师核验。 '+resume.repeat(3);
 let previewDelay=0,previewFailure=false;let previews=0,polls=0,eventID=0,failBillingOnce=false,created=0,creation=null,saved=0,failSaveOnce=false,downloads=0; const reviews=new Map();const errors=[],checks=[];let reparsed=0;const documents=[{id:'document-fixture',name:'教材样例.pdf',original_available:true,status:'ready',chunks:3,created_at:'2026-10-13T12:00:00Z'}];
 function event(stage){task.events.push({id:++eventID,stage,message:stage})}
 event('batch_start_4_6');
 browser=spawn('C:/Program Files (x86)/Microsoft/Edge/Application/msedge.exe',['--headless=new','--disable-gpu','--no-first-run','--disable-background-networking','--remote-debugging-port=9286','--remote-debugging-address=127.0.0.1','--user-data-dir='+path.join(output,'browser-profile'),'about:blank'],{windowsHide:true,stdio:'ignore'});
 let page;
 for(let i=0;i<40;i++){try{page=(await fetch('http://127.0.0.1:9286/json/list').then(r=>r.json())).find(p=>p.type==='page');if(page)break}catch{}await sleep(200)}
 assert.ok(page,'Edge starts');socket=new WebSocket(page.webSocketDebuggerUrl);
 await new Promise((resolve,reject)=>{socket.onopen=resolve;socket.onerror=reject});
 let seq=0;const pending=new Map();
 const call=(method,params={})=>new Promise((resolve,reject)=>{const id=++seq,timer=setTimeout(()=>reject(Error('CDP timeout: '+method)),15000);pending.set(id,{resolve,reject,timer});socket.send(JSON.stringify({id,method,params}))});
 socket.onmessage=e=>{
  const m=JSON.parse(e.data);
  if(m.id){const p=pending.get(m.id);if(p){clearTimeout(p.timer);pending.delete(m.id);m.error?p.reject(Error(JSON.stringify(m.error))):p.resolve(m.result)}}
  if(m.method==='Page.javascriptDialogOpening')call('Page.handleJavaScriptDialog',{accept:true}).catch(e=>errors.push(String(e)));
  if(m.method==='Runtime.exceptionThrown')errors.push(m.params.exceptionDetails.text);
  if(m.method!=='Fetch.requestPaused')return;
  const {requestId,request}=m.params,p=new URL(request.url).pathname;
  let reply,body,contentType='application/json',code=200;
  if(p.endsWith('/canvas-preview')){if(previewFailure){previewFailure=false;code=422;reply={message:'文字过密，当前修改保留'}}else reply={image:'data:image/png;base64,'+png.toString('base64'),revision:task.revision};previews++}
  else if(p.endsWith('/objects')){const sl=task.result.slides[0],offset=sl.canvas_positions||{};reply={version:'canvas-v3',objects:[{id:'title',x:.06+(offset.title?.dx||0),y:.2+(offset.title?.dy||0),width:.8*(offset.title?.scale??1),height:.1*(offset.title?.scale??1),rotatable:true,min_scale:.6,rotation:offset.title?.rotation||0},{id:'body',x:.06,y:.35,width:.5,height:.35,rotatable:true},{id:'focus',x:.06,y:.76,width:.85,height:.1,rotatable:true},{id:'visual',x:.65,y:.4,width:.25,height:.3,rotatable:false},...(sl.canvas_objects||[]).map(o=>({...o,rotatable:true}))]}}
  else if(p==='/api/documents/document-fixture/reparse'&&request.method==='POST'){reparsed++;documents[0].status='pending';reply={ok:true}}
  else if(p==='/api/documents'&&request.method==='GET')reply=documents;
  else if(p==='/api/tasks'&&request.method==='POST'){created++;creation=JSON.parse(request.postData);task.status='awaiting_preview';task.input=creation;task.result=deck;deck.slides[0].focus='课堂问题';task.revision=3;task.slide_reviews=deck.slides.map(s=>({id:s.id,hash:'hash-'+s.id,approved:false,vote:s.id==='slide-0'?-1:0,reason:'历史改进意见'}));reply={id:task.id}}
  else if(p==='/api/courseware/brief'&&request.method==='POST')reply={reply:'请补充课堂探究活动。',mode:'sample'};
  else if(p==='/api/tasks/'+task.id+'/draft'&&request.method==='POST'){const b=JSON.parse(request.postData);if(failSaveOnce){failSaveOnce=false;code=422;reply={message:'文字过密，请缩短要点；原版本保留'}}else if(b.revision!==task.revision){code=409;reply={message:'当前版本已变化'}}else{const old=task.result;task.result=b.result;task.revision++;saved++;task.slide_reviews=task.result.slides.map(s=>JSON.stringify(s)===JSON.stringify(old.slides.find(o=>o.id===s.id))?task.slide_reviews.find(m=>m.id===s.id):({id:s.id,hash:'saved-'+s.id,approved:false,vote:0}));reply={ok:true}}}
  else if(p.endsWith('/feedback')&&request.method==='POST'){const b=JSON.parse(request.postData),id=p.split('/').at(-2);assert.equal(b.vote,0);task.slide_reviews.find(s=>s.id===id).vote=0;reply={ok:true}}
  else if(p.endsWith('/review')&&request.method==='POST'){const b=JSON.parse(request.postData),id=p.split('/').at(-2);assert.equal(b.revision,task.revision);const m=task.slide_reviews.find(s=>s.id===id);assert.equal(b.hash,m.hash);assert.equal(m.vote,0);m.approved=b.approved;reviews.set(id,b);reply={ok:true}}
  else if(p==='/api/courseware/materials/preview'){reply={pages:[{page:1,text:'第十九章 一次函数'},{page:2,text:'y=kx+b，k不为零'}],material_snapshot:{name:'教材样例.pdf',edition:'教师标注测试版次',chapter:'原文章节'},page_warnings:[{file_page:1,kind:'catalogue'}]}}
  else if(p==='/api/courseware/materials')reply=[{id:'textbook1',name:'人教版数学八年级.pdf',pages:2,curriculum:{subject:'物理',grade:'八年级',edition:'人教版'},chapters:[{title:'第十九章 一次函数',start:1,end:2}]}];
  else if(/\/materials\/[^/]+\/pages\/\d+\/image$/.test(p))reply={file_page:2,data_url:'data:image/png;base64,'+png.toString('base64'),name:'原始教材.pdf',original_hash:'fixture-original-hash'};
  else if(p==='/api/courseware/references')reply=[];
  else if(request.method!=='GET'){errors.push('Unexpected write: '+p);code=400;reply={message:'No writes allowed'}}
  else if(p==='/api/health')reply={status:'ok',mode:'sample',courseware_canvas_version:'canvas-v3',courseware_studio_version:'studio-v1',courseware_media_version:'media-v1',courseware_material_images_version:'page-images-v1',document_reparse_version:'reparse-v1',teaching_features_version:'teaching-v1',works_view_version:'views-v1'};
  else if(p==='/api/account')reply={username:'teacher',name:'测试教师'};
  else if(p==='/api/dashboard')reply={totals:{},tasks:[],usage:[]};
  else if(p==='/api/usage')reply={items:[],total:0};
  else if(p==='/api/auth/me')reply={tenant_id:1,member_id:1,name:'测试教师',role:'teacher',permissions:['courseware.generate','kb.manage']};
  else if(p==='/api/tenants')reply=[{id:1,name:'测试'}];
  else if(p==='/api/billing'){if(failBillingOnce){failBillingOnce=false;code=503;reply={message:'测试瞬时网络错误'}}else reply={balance:100,reserved:task.status==='failed'?0:20,available:task.status==='failed'?100:80,plan:'basic',ledger:[]}}
  else if(p==='/api/tasks')reply=[task];
  else if(p==='/api/tasks/'+task.id){polls++;reply=task}
  else if(p.endsWith('/conversation'))reply=[];
  else if(p.endsWith('/events')){contentType='text/event-stream';body=Buffer.from('id: '+eventID+'\ndata: '+JSON.stringify(task.events.at(-1))+'\n\n')}
  else if(p.endsWith('/preview-pptx')||p.endsWith('/download/pptx')){downloads++;contentType='application/vnd.openxmlformats-officedocument.presentationml.presentation';body=Buffer.from('fixture-pptx')}
  else if(p.endsWith('/preview')){previews++;contentType='image/png';body=png}
  else{errors.push('Unexpected API: '+p);code=404;reply={}}
  const deliver=()=>call('Fetch.fulfillRequest',{requestId,responseCode:code,responseHeaders:[{name:'Content-Type',value:contentType}],body:(body||Buffer.from(JSON.stringify(reply))).toString('base64')}).catch(e=>errors.push(String(e)));
  if(p.endsWith('/canvas-preview'))setTimeout(deliver,previewDelay);else deliver();
 };
 const evaluate=async expression=>{const r=await call('Runtime.evaluate',{expression,returnByValue:true,awaitPromise:true});if(r.exceptionDetails)throw Error(r.exceptionDetails.exception?.description||r.exceptionDetails.text);return r.result.value};
 const wait=async expression=>{for(let i=0;i<100;i++){if(await evaluate(expression))return;await sleep(100)}throw Error('UI timeout: '+expression+'\n'+await evaluate('document.body.innerText.slice(-2000)')+'\n'+JSON.stringify(errors))};
 const click=label=>evaluate(`(()=>{const b=[...(document.querySelector('.classroom-presentation')||document).querySelectorAll('button')].find(b=>b.textContent.includes(${JSON.stringify(label)}));if(!b||b.disabled)throw Error('Missing button');b.click()})()`);
 const srcs=()=>evaluate("[...document.querySelectorAll('.studio-slide-picture img')].map(i=>i.src)");
 await call('Runtime.enable');await call('Page.enable');await call('Emulation.setFocusEmulationEnabled',{enabled:true});await call('Fetch.enable',{patterns:[{urlPattern:'*/api/*'}]});
 await call('Page.addScriptToEvaluateOnNewDocument',{source:"sessionStorage.setItem('edu_token','fixture-token')"});
 await call('Emulation.setDeviceMetricsOverride',{width:1440,height:1080,deviceScaleFactor:1,mobile:false});
 await call('Page.navigate',{url:process.env.TEST_APP_URL||'http://127.0.0.1:8080'});
 await wait("[...document.querySelectorAll('nav button')].some(b=>b.textContent.includes('我的作品'))");
 await click('我的作品');await wait("!!document.querySelector('.task-open')");await evaluate("document.querySelector('.task-open').click()");
 await wait("!!document.querySelector('[aria-label=\"移动第1页对象\"]')");await evaluate("document.querySelector('[aria-label=\"移动第1页对象\"]').click()");await wait("!!document.querySelector('.object-stage')");
 const button=label=>`[...document.querySelectorAll('.object-editor button')].find(b=>b.textContent.trim()===${JSON.stringify(label)})`;
 const action=label=>evaluate(`${button(label)}.click()`);
 const input=async(label,value,type='change')=>evaluate(`(()=>{const x=document.querySelector('[aria-label="'+${JSON.stringify(label)}+'"]');x.value=${JSON.stringify(value)};x.dispatchEvent(new Event(${JSON.stringify(type)},{bubbles:true}))})()`);
 const choose=id=>evaluate(`(()=>{const b=document.querySelector('[data-object-id="'+${JSON.stringify(id)}+'"]');b.blur();b.focus()})()`);
 const layers=()=>evaluate("document.querySelector('.canvas-inspector details').open=true");
 const layerSelect=async(label,shift=false)=>{await layers();await evaluate(`[...document.querySelectorAll('.canvas-layers button')].find(b=>b.textContent.includes(${JSON.stringify(label)})).dispatchEvent(new MouseEvent('click',{bubbles:true,shiftKey:${shift}}))`)};
 await action('添加文本');await wait("document.querySelectorAll('[data-kind=text]').length===1");
 const textID=await evaluate("document.querySelector('[data-kind=text]').dataset.objectId");
 await evaluate("document.querySelector('[data-kind=text]').click()");await wait("!!document.querySelector('.canvas-text-input')");
 await input('画布编辑文本框 1','观察并解释图像','input');await action('完成输入');
 await input('文本字号','28');await input('对象颜色','#175b40');await input('文本对齐','center');
 await input('对象旋转角度','12');assert.equal(await evaluate("document.querySelector('[data-kind=text]').style.transform"),'rotate(12deg)');
 checks.push('Text entry, type size, colour, alignment and rotation update native object data');
 await action('复制对象');assert.equal(await evaluate("document.querySelectorAll('[data-kind=text]').length"),2);
 await action('删除对象');assert.equal(await evaluate("document.querySelectorAll('[data-kind=text]').length"),1);
 await action('撤销');assert.equal(await evaluate("document.querySelectorAll('[data-kind=text]').length"),2);
 await action('重做');assert.equal(await evaluate("document.querySelectorAll('[data-kind=text]').length"),1);
 await choose(textID);await action('锁定对象');assert.equal(await evaluate("document.querySelector('[aria-label=\"对象缩放\"]').disabled"),true);
 const lockedPosition=await evaluate("document.querySelector('[data-kind=text]').style.left");
 await evaluate("document.querySelector('.object-stage').focus();document.querySelector('.object-stage').dispatchEvent(new KeyboardEvent('keydown',{key:'ArrowRight',bubbles:true}))");
 assert.equal(await evaluate("document.querySelector('[data-kind=text]').style.left"),lockedPosition);
 await action('解锁对象');await choose(textID);
 await call('Input.dispatchKeyEvent',{type:'keyDown',key:'ArrowRight',code:'ArrowRight'});await call('Input.dispatchKeyEvent',{type:'keyUp',key:'ArrowRight',code:'ArrowRight'});
 assert.notEqual(await evaluate("document.querySelector('[data-kind=text]').style.left"),lockedPosition);
 checks.push('Duplicate, delete, undo/redo, locking and real keyboard nudge');
 await input('添加图形','ellipse');await wait("document.querySelectorAll('[data-kind=shape]').length===1");
 const shapeID=await evaluate("document.querySelector('[data-kind=shape]').dataset.objectId");
 await input('对象填充颜色','#dceee4');await input('图形线宽','3');
 await input('对象水平位置','60');await input('对象垂直位置','55');
 await layerSelect('文本框 1');await layerSelect('图形 2',true);assert.equal(await evaluate("document.querySelectorAll('.object-stage [aria-pressed=true]').length"),2);
 await action('左对齐');const lefts=await evaluate("[...document.querySelectorAll('.object-stage [aria-pressed=true]')].map(b=>({left:parseFloat(b.style.left),rot:b.style.transform}))");assert.equal(lefts.length,2);
 await action('置底');assert.ok(await evaluate("document.querySelector('.object-stage button').dataset.kind==='text'"));
 await layerSelect('图形 2');await action('置顶');assert.equal(await evaluate("[...document.querySelectorAll('.object-stage button[data-object-id]')].at(-1).dataset.objectId"),shapeID);
 await input('对象旋转角度','20');await input('对象宽度','20');await input('对象高度','15');
 await evaluate("document.querySelector('.object-stage').scrollIntoView({block:'center',behavior:'instant'})");
 const handle=await evaluate("(()=>{const r=document.querySelector('.resize-handle').getBoundingClientRect();return {x:r.x+r.width/2,y:r.y+r.height/2}})()");
 const widthBefore=await evaluate("document.querySelector('[data-kind=shape]').style.width");
 await call('Input.dispatchMouseEvent',{type:'mousePressed',button:'left',clickCount:1,...handle});await call('Input.dispatchMouseEvent',{type:'mouseMoved',button:'left',x:handle.x+25,y:handle.y+12});await call('Input.dispatchMouseEvent',{type:'mouseReleased',button:'left',clickCount:1,x:handle.x+25,y:handle.y+12});
 assert.notEqual(await evaluate("document.querySelector('[data-kind=shape]').style.width"),widthBefore);await action('撤销');assert.equal(await evaluate("document.querySelector('[data-kind=shape]').style.width"),widthBefore);
 checks.push('Multi-selection, alignment, layer ordering, numeric geometry and real pointer resize');
 await evaluate("document.querySelector('[aria-label=\"移动图示\"]').focus()");assert.equal(await evaluate("!!document.querySelector('[aria-label=\"对象旋转角度\"]')"),false);
 checks.push('Scientific region rotation protected in UI');
 const imagePath=path.join(output,'test-image.png');
 // Locally generated fixture, never a production upload.
 const made=await evaluate("(()=>{const c=document.createElement('canvas');c.width=96;c.height=96;const x=c.getContext('2d');x.fillStyle='#156791';x.fillRect(0,0,96,96);return c.toDataURL('image/png').split(',')[1]})()");await fs.writeFile(imagePath,Buffer.from(made,'base64'));
 await action('添加图片');await call('DOM.enable');const doc=await call('DOM.getDocument');const node=await call('DOM.querySelector',{nodeId:doc.root.nodeId,selector:'.canvas-file'});await call('DOM.setFileInputFiles',{nodeId:node.nodeId,files:[imagePath]});
 await wait("document.querySelectorAll('[data-kind=image]').length===1");await input('图片说明','教师添加示意图');await input('对象旋转角度','15');
 const imageWidth=await evaluate("document.querySelector('[data-kind=image]').style.width"),imageHeight=await evaluate("parseFloat(document.querySelector('[data-kind=image]').style.height)");
 const replacement=await evaluate("(()=>{const c=document.createElement('canvas');c.width=96;c.height=48;const x=c.getContext('2d');x.fillStyle='#1b7858';x.fillRect(0,0,96,48);return c.toDataURL('image/png').split(',')[1]})()");await fs.writeFile(imagePath,Buffer.from(replacement,'base64'));
 await action('替换图片');await call('DOM.setFileInputFiles',{nodeId:node.nodeId,files:[imagePath]});await wait("parseFloat(document.querySelector('[data-kind=image]').style.height)<"+imageHeight);
 assert.equal(await evaluate("document.querySelector('[data-kind=image]').style.width"),imageWidth);assert.ok(Math.abs(await evaluate("parseFloat(document.querySelector('[data-kind=image]').style.height)")-imageHeight/2)<.001);
 checks.push('Local image decoding, bounded compression, aspect ratio, replacement and caption');
 const savesBefore=saved;previewDelay=300;await action('更新实际预览');await wait("document.querySelector('.canvas-preview-row').innerText.includes('当前修改已完成实际排版')");assert.equal(saved,savesBefore);
 assert.equal(await evaluate("document.querySelectorAll('.object-stage .new-image').length"),0);
 await input('图片说明','新说明');assert.ok(await evaluate("document.querySelector('.canvas-preview-row').innerText.includes('底图为已保存画面')"));
 await action('更新实际预览');await input('图片说明','排版期间又修改');await wait("document.querySelector('.canvas-text-warning')?.innerText.includes('排版期间')");assert.equal(await evaluate("document.querySelector('.canvas-preview-row').innerText.includes('当前修改已完成实际排版')"),false);
 previewFailure=true;await action('更新实际预览');await wait("document.querySelector('.canvas-text-warning')?.innerText.includes('文字过密')");const count=previews;await sleep(2400);assert.equal(previews,count);
 const autoStart=previews;await evaluate("(()=>{const x=[...document.querySelectorAll('.canvas-preview-row input')][0];x.checked=true;x.dispatchEvent(new Event('change',{bubbles:true}))})()");
 await input('图片说明','自动预览的新说明');await wait("document.querySelector('.canvas-preview-row').innerText.includes('当前修改已完成实际排版')");assert.equal(previews,autoStart+1);await sleep(2300);assert.equal(previews,autoStart+1);
 await evaluate("(()=>{const x=[...document.querySelectorAll('.canvas-preview-row input')][0];x.checked=false;x.dispatchEvent(new Event('change',{bubbles:true}))})()");await input('图片说明','排版期间又修改');
 checks.push('Temporary actual preview is read-only; stale image discarded; automatic preview coalesces changes without a polling loop');
 failSaveOnce=true;await click('保存画布及其他修改');await wait("document.querySelector('.alert.error')?.innerText.includes('文字过密')");assert.equal(saved,savesBefore);assert.equal(await evaluate("document.querySelectorAll('[data-kind=image]').length"),1);
 await click('保存画布及其他修改');await wait("!document.querySelector('.unsaved-note')");assert.equal(task.result.slides[0].canvas_objects.length,3);assert.equal(task.result.slides[0].canvas_objects.find(o=>o.kind==='image').caption,'排版期间又修改');
 await wait("!!document.querySelector('[aria-label=\"移动第1页对象\"]')"); await evaluate("document.querySelector('[aria-label=\"移动第1页对象\"]').click()");await wait("document.querySelectorAll('[data-kind=image]').length===1");assert.equal(await evaluate("document.querySelectorAll('.object-stage .new-image').length"),0);
 assert.ok(task.result.slides[0].canvas_objects.find(o=>o.kind==='image').data_url.startsWith('data:image/png;base64,'));
 checks.push('Failed save keeps all objects; successful save/reopen retains geometry, layers and embedded image once');
 await layerSelect('图片 3');await action('删除对象');await action('撤销');assert.equal(await evaluate("document.querySelectorAll('[data-kind=image]').length"),1);await action('重做');assert.equal(await evaluate("document.querySelectorAll('[data-kind=image]').length"),0);
 await click('保存画布及其他修改');await wait("!document.querySelector('.unsaved-note')");assert.equal(task.result.slides[0].canvas_objects.length,2);assert.equal(JSON.stringify(task.result.slides[0]).includes('data:image/'),false);
 checks.push('Image deletion removes current payload after save; reversible before save');
 await wait("!!document.querySelector('[aria-label=\"移动第1页对象\"]')"); await evaluate("document.querySelector('[aria-label=\"移动第1页对象\"]').click()");await wait("!!document.querySelector('.object-stage')");
 await action('添加文本');await action('复制对象');await action('复制对象');
 await layerSelect('文本框 3');await input('对象水平位置','5');await input('对象宽度','8');
 await layerSelect('文本框 4');await input('对象水平位置','30');await input('对象宽度','8');
 await layerSelect('文本框 5');await input('对象水平位置','65');await input('对象宽度','8');
 await layerSelect('文本框 3');await layerSelect('文本框 4',true);await layerSelect('文本框 5',true);await action('横向均匀分布');
 const xs=await evaluate("[...document.querySelectorAll('.object-stage [aria-pressed=true]')].map(b=>parseFloat(b.style.left)).sort((a,b)=>a-b)");assert.ok(Math.abs((xs[1]-xs[0])-(xs[2]-xs[1]))<.001);
 checks.push('Three-object distribution keeps endpoint positions and equal spacing');
 await call('Emulation.setDeviceMetricsOverride',{width:390,height:844,deviceScaleFactor:1,mobile:true});await sleep(300);assert.ok(await evaluate('document.documentElement.scrollWidth<=innerWidth'));
 await evaluate("document.querySelector('.object-editor').scrollIntoView({block:'start',behavior:'instant'})");await call('Page.captureScreenshot',{format:'png'}).then(r=>fs.writeFile(path.join(output,'canvas-mobile.png'),Buffer.from(r.data,'base64')));
 await call('Emulation.setDeviceMetricsOverride',{width:1440,height:1080,deviceScaleFactor:1,mobile:false});await sleep(200);await call('Page.captureScreenshot',{format:'png'}).then(r=>fs.writeFile(path.join(output,'canvas-desktop.png'),Buffer.from(r.data,'base64')));
 const geometryBefore=await evaluate("[...document.querySelectorAll('.object-stage [data-object-id]')].map(b=>b.style.cssText).join('|')");await input('画布视图缩放','150');
 assert.equal(await evaluate("[...document.querySelectorAll('.object-stage [data-object-id]')].map(b=>b.style.cssText).join('|')"),geometryBefore);await input('画布视图缩放','100');
 checks.push('390px responsive layout; view zoom does not alter saved geometry');
 assert.deepEqual(errors,[]);await fs.writeFile(path.join(output,'checks.json'),JSON.stringify({checks,errors},null,2));
 console.log(JSON.stringify({checks}));await call('Browser.close');socket.close();
})().catch(e=>{console.error(e.stack);process.exitCode=1;socket?.close();browser?.kill()});
